"use client";

import { AppWindow, Boxes, CornerDownLeft, FolderClosed, KeyRound, Moon, Play, Search, UserRound, UsersRound, Wand2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import useSWR from "swr";

import { API, armPath, call } from "@/lib/api";
import type { Application, CoverageRow, Group, ResourceGroup, ServicePrincipal, Subscription, User } from "@/lib/azure";
import { NAV } from "./nav";
import { useSwitchPersona } from "./session";
import { Kbd, cx } from "./ui";

interface Item {
  id: string;
  group: string;
  label: string;
  hint?: string;
  icon: ReactNode;
  run: () => void;
}

type V<T> = { value: T[] };

async function loadIndex() {
  const safe = <T,>(p: Promise<V<T>>) => p.then((r) => r.value ?? []).catch(() => [] as T[]);
  const [subs, users, groups, apps, sps, coverage] = await Promise.all([
    safe(call<V<Subscription>>("arm", armPath("/subscriptions"))),
    safe(call<V<User>>("graph", "/v1.0/users?$top=200")),
    safe(call<V<Group>>("graph", "/v1.0/groups?$top=200")),
    safe(call<V<Application>>("graph", "/v1.0/applications?$top=200")),
    safe(call<V<ServicePrincipal>>("graph", "/v1.0/servicePrincipals?$top=200")),
    safe(call<V<CoverageRow>>("admin", "/coverage")),
  ]);
  const rgs = (
    await Promise.all(subs.map((s) => safe(call<V<ResourceGroup>>("arm", armPath(`/subscriptions/${s.subscriptionId}/resourcegroups`, API.resources))).then((l) => l.map((rg) => ({ rg, s })))))
  ).flat();
  return { subs, rgs, users, groups, apps, sps, coverage };
}

export function CommandPalette({ open, onClose }: { open: boolean; onClose: () => void }) {
  const router = useRouter();
  const switchTo = useSwitchPersona();
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const { data, isLoading } = useSWR(open ? "palette-index" : null, loadIndex, { revalidateOnFocus: false, dedupingInterval: 10_000 });

  const go = (href: string) => () => {
    onClose();
    router.push(href);
  };

  const items = useMemo<Item[]>(() => {
    const out: Item[] = NAV.map((n) => ({ id: "nav" + n.href, group: "Go to", label: n.label, hint: `g ${n.key}`, icon: <n.icon className="size-4" />, run: go(n.href) }));
    out.push(
      { id: "act-selftest", group: "Actions", label: "Run self-test of every service", icon: <Play className="size-4" />, run: go("/services?run=1") },
      { id: "act-rg", group: "Actions", label: "Create a resource group", icon: <Wand2 className="size-4" />, run: go("/resources?create=1") },
      { id: "act-user", group: "Actions", label: "Create a user", icon: <Wand2 className="size-4" />, run: go("/identity?tab=users&create=1") },
      { id: "act-token", group: "Actions", label: "Decode the current access token", icon: <KeyRound className="size-4" />, run: go("/tokens") },
      {
        id: "act-owner",
        group: "Actions",
        label: "Act as cloudtyke-bootstrap (Owner)",
        icon: <UserRound className="size-4" />,
        run: () => {
          onClose();
          switchTo("owner");
        },
      },
      {
        id: "act-reader",
        group: "Actions",
        label: "Act as cloudtyke-reader (Reader)",
        icon: <UserRound className="size-4" />,
        run: () => {
          onClose();
          switchTo("reader");
        },
      },
      {
        id: "act-theme",
        group: "Actions",
        label: "Toggle dark mode",
        icon: <Moon className="size-4" />,
        run: () => {
          const d = !document.documentElement.classList.contains("dark");
          document.documentElement.classList.toggle("dark", d);
          localStorage.setItem("ct-theme", d ? "dark" : "light");
          onClose();
        },
      },
    );
    if (data) {
      for (const s of data.subs)
        out.push({ id: "sub" + s.subscriptionId, group: "Subscriptions", label: s.displayName, hint: s.subscriptionId, icon: <KeyRound className="size-4" />, run: go(`/resources?sub=${s.subscriptionId}`) });
      for (const { rg, s } of data.rgs)
        out.push({ id: "rg" + rg.id, group: "Resource groups", label: rg.name, hint: `${s.displayName} · ${rg.location}`, icon: <FolderClosed className="size-4" />, run: go(`/resources?sub=${s.subscriptionId}&rg=${encodeURIComponent(rg.name)}`) });
      for (const u of data.users)
        out.push({ id: "u" + u.id, group: "Users", label: u.displayName || u.userPrincipalName, hint: u.userPrincipalName, icon: <UserRound className="size-4" />, run: go(`/identity?tab=users&id=${u.id}`) });
      for (const g of data.groups)
        out.push({ id: "g" + g.id, group: "Groups", label: g.displayName || g.id, hint: g.description || g.mailNickname, icon: <UsersRound className="size-4" />, run: go(`/identity?tab=groups&id=${g.id}`) });
      for (const a of data.apps)
        out.push({ id: "a" + a.id, group: "App registrations", label: a.displayName || a.appId, hint: a.appId, icon: <AppWindow className="size-4" />, run: go(`/identity?tab=apps&id=${a.id}`) });
      for (const sp of data.sps)
        out.push({ id: "sp" + sp.id, group: "Enterprise apps", label: sp.displayName || sp.appId, hint: sp.appId, icon: <AppWindow className="size-4" />, run: go(`/identity?tab=sps&id=${sp.id}`) });
      for (const c of data.coverage)
        out.push({ id: "svc" + c.type, group: "Services", label: c.type, hint: c.notes, icon: <Boxes className="size-4" />, run: go(`/services?id=${encodeURIComponent(c.type)}`) });
    }
    return out;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data]);

  const filtered = useMemo(() => {
    if (!q) return items.filter((i) => i.group === "Go to" || i.group === "Actions");
    const terms = q.toLowerCase().split(/\s+/).filter(Boolean);
    return items
      .map((i) => {
        const hay = `${i.label} ${i.hint ?? ""} ${i.group}`.toLowerCase();
        if (!terms.every((t) => hay.includes(t))) return null;
        const score = i.label.toLowerCase().startsWith(terms[0]) ? 0 : i.label.toLowerCase().includes(terms[0]) ? 1 : 2;
        return { i, score };
      })
      .filter((x): x is { i: Item; score: number } => !!x)
      .sort((a, b) => a.score - b.score)
      .slice(0, 60)
      .map((x) => x.i);
  }, [items, q]);

  useEffect(() => {
    listRef.current?.querySelector(`[data-idx="${sel}"]`)?.scrollIntoView({ block: "nearest" });
  }, [sel]);

  if (!open) return null;
  const groups = [...new Set(filtered.map((f) => f.group))];
  let idx = -1;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/35 p-4 pt-[12vh] backdrop-blur-[2px] animate-in" onMouseDown={onClose}>
      <div className="w-full max-w-xl overflow-hidden rounded-2xl border border-border bg-surface shadow-pop" onMouseDown={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-3 border-b border-border px-4">
          <Search className="size-4 text-subtle" />
          <input
            autoFocus
            value={q}
            onChange={(e) => {
              setQ(e.target.value);
              setSel(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setSel((s) => Math.min(filtered.length - 1, s + 1));
              } else if (e.key === "ArrowUp") {
                e.preventDefault();
                setSel((s) => Math.max(0, s - 1));
              } else if (e.key === "Enter") {
                filtered[sel]?.run();
              } else if (e.key === "Escape") onClose();
            }}
            placeholder="Search everything or type a command…"
            className="h-12 flex-1 bg-transparent text-[15px] placeholder:text-subtle focus:outline-none"
          />
          {isLoading && <span className="text-xs text-subtle animate-pulse-soft">indexing…</span>}
        </div>
        <div ref={listRef} className="max-h-[56vh] overflow-y-auto p-1.5">
          {!filtered.length && <div className="px-3 py-10 text-center text-sm text-muted">No results for “{q}”</div>}
          {groups.map((g) => (
            <div key={g} className="mb-1">
              <div className="px-2.5 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-subtle">{g}</div>
              {filtered
                .filter((f) => f.group === g)
                .map((f) => {
                  idx++;
                  const i = idx;
                  return (
                    <button
                      key={f.id}
                      data-idx={i}
                      onMouseMove={() => setSel(i)}
                      onClick={f.run}
                      className={cx("flex w-full items-center gap-3 rounded-lg px-2.5 py-2 text-left text-sm", sel === i ? "bg-accent-soft text-fg" : "text-fg")}
                    >
                      <span className={cx("text-subtle", sel === i && "text-accent")}>{f.icon}</span>
                      <span className="min-w-0 flex-1 truncate font-medium">{f.label}</span>
                      {f.hint && <span className="max-w-[45%] truncate text-xs text-subtle">{f.hint}</span>}
                      {sel === i && <CornerDownLeft className="size-3.5 text-accent" />}
                    </button>
                  );
                })}
            </div>
          ))}
        </div>
        <div className="flex items-center gap-4 border-t border-border bg-surface-2/50 px-4 py-2 text-[11px] text-subtle">
          <span className="flex items-center gap-1">
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd> navigate
          </span>
          <span className="flex items-center gap-1">
            <Kbd>↵</Kbd> open
          </span>
          <span className="flex items-center gap-1">
            <Kbd>esc</Kbd> close
          </span>
        </div>
      </div>
    </div>
  );
}
