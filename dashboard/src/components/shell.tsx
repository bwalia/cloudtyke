"use client";

import { ChevronDown, Cloud, Menu, Moon, Search, Sun, UserRound } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { SWRConfig } from "swr";

import { useApi } from "@/lib/api";
import { selfTestReport, useSelfTest } from "@/lib/hooks";
import { CommandPalette } from "./command-palette";
import { NAV } from "./nav";
import { ToastProvider, useToast } from "./overlay";
import { useSession, useSwitchPersona } from "./session";
import { Kbd, StatusDot, cx } from "./ui";

export function Shell({ children }: { children: ReactNode }) {
  return (
    <SWRConfig value={{ errorRetryCount: 1, dedupingInterval: 1500 }}>
      <ToastProvider>
        <Frame>{children}</Frame>
      </ToastProvider>
    </SWRConfig>
  );
}

function Frame({ children }: { children: ReactNode }) {
  const [palette, setPalette] = useState(false);
  const [mobileNav, setMobileNav] = useState(false);
  const router = useRouter();
  const pathname = usePathname();
  const pending = useRef<string | null>(null);

  // ⌘K / Ctrl-K palette, "/" to search, and "g then key" to jump between sections.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      const typing = t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable);
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette((p) => !p);
        return;
      }
      if (typing || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "/") {
        e.preventDefault();
        setPalette(true);
        return;
      }
      if (pending.current === "g") {
        pending.current = null;
        const n = NAV.find((x) => x.key === e.key);
        if (n) router.push(n.href);
        return;
      }
      if (e.key === "g") {
        pending.current = "g";
        setTimeout(() => (pending.current = null), 900);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router]);

  return (
    <div className="flex min-h-screen">
      <Sidebar open={mobileNav} onNavigate={() => setMobileNav(false)} />
      {mobileNav && <div className="fixed inset-0 z-30 bg-black/30 lg:hidden" onClick={() => setMobileNav(false)} />}
      <div className="flex min-w-0 flex-1 flex-col lg:pl-60">
        <TopBar onSearch={() => setPalette(true)} onMenu={() => setMobileNav(true)} />
        <main key={pathname} className="mx-auto w-full max-w-[1400px] flex-1 px-4 py-6 sm:px-6 lg:px-8 animate-in">
          {children}
        </main>
      </div>
      {palette && <CommandPalette open onClose={() => setPalette(false)} />}
    </div>
  );
}

function Sidebar({ open, onNavigate }: { open: boolean; onNavigate: () => void }) {
  const pathname = usePathname();
  const groups = [...new Set(NAV.map((n) => n.group))];
  const st = selfTestReport(useSelfTest());
  return (
    <nav
      className={cx(
        "fixed inset-y-0 left-0 z-40 flex w-60 flex-col border-r border-border bg-surface transition-transform lg:translate-x-0",
        open ? "translate-x-0" : "-translate-x-full",
      )}
    >
      <Link href="/" onClick={onNavigate} className="flex h-14 items-center gap-2.5 border-b border-border px-4">
        <span className="grid size-8 place-items-center rounded-lg bg-gradient-to-br from-accent to-info text-white shadow-card">
          <Cloud className="size-4.5" strokeWidth={2.5} />
        </span>
        <span className="leading-tight">
          <span className="block text-sm font-bold tracking-tight">CloudTyke</span>
          <span className="block text-[11px] text-subtle">Azure emulator console</span>
        </span>
      </Link>
      <div className="flex-1 overflow-y-auto px-2 py-3">
        {groups.map((g) => (
          <div key={g} className="mb-4">
            <div className="px-2.5 pb-1 text-[10px] font-semibold uppercase tracking-wider text-subtle">{g}</div>
            {NAV.filter((n) => n.group === g).map((n) => {
              const active = n.href === "/" ? pathname === "/" : pathname.startsWith(n.href);
              const Icon = n.icon;
              return (
                <Link
                  key={n.href}
                  href={n.href}
                  onClick={onNavigate}
                  className={cx(
                    "group mb-0.5 flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm font-medium transition-colors",
                    active ? "bg-accent-soft text-accent" : "text-muted hover:bg-surface-2 hover:text-fg",
                  )}
                >
                  <Icon className="size-4" />
                  <span className="flex-1">{n.label}</span>
                  {n.href === "/services" && st ? (
                    <span className={cx("rounded-full px-1.5 text-[10px] font-bold tabular-nums", st.failed ? "bg-bad-soft text-bad" : "bg-ok-soft text-ok")}>
                      {st.passed}/{st.passed + st.failed}
                    </span>
                  ) : (
                    <span className="hidden text-[10px] text-subtle group-hover:inline">
                      g {n.key}
                    </span>
                  )}
                </Link>
              );
            })}
          </div>
        ))}
      </div>
      <div className="border-t border-border p-3 text-[11px] leading-relaxed text-subtle">
        Local emulator. Not affiliated with or endorsed by Microsoft.
      </div>
    </nav>
  );
}

function TopBar({ onSearch, onMenu }: { onSearch: () => void; onMenu: () => void }) {
  const { data: health, error } = useApi<{ status: string; version: string }>("admin", "/health", { refreshInterval: 5000, shouldRetryOnError: true });
  const up = !error && health?.status === "ok";
  return (
    <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-border bg-bg/80 px-4 backdrop-blur-md sm:px-6 lg:px-8">
      <button className="rounded-md p-1.5 text-muted hover:bg-surface-2 lg:hidden" onClick={onMenu} aria-label="Open navigation">
        <Menu className="size-5" />
      </button>
      <button
        onClick={onSearch}
        className="flex h-9 w-full max-w-md items-center gap-2 rounded-lg border border-border bg-surface px-3 text-sm text-subtle shadow-card transition-colors hover:border-border-strong"
      >
        <Search className="size-4" />
        <span className="flex-1 text-left">Search resources, identities, services…</span>
        <Kbd>⌘K</Kbd>
      </button>
      <div className="ml-auto flex items-center gap-2">
        <Link
          href="/emulator"
          className="hidden items-center gap-2 rounded-lg border border-border bg-surface px-2.5 py-1.5 text-xs font-medium shadow-card sm:flex"
          title={up ? "Emulator healthy" : error?.message}
        >
          <StatusDot tone={health ? (up ? "ok" : "bad") : error ? "bad" : "neutral"} pulse={up} />
          {up ? <>Emulator {health?.version}</> : error ? "Emulator offline" : "Connecting…"}
        </Link>
        <PersonaSwitcher />
        <ThemeToggle />
      </div>
    </header>
  );
}

function PersonaSwitcher() {
  const { data } = useSession();
  const switchTo = useSwitchPersona();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const h = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    window.addEventListener("mousedown", h);
    return () => window.removeEventListener("mousedown", h);
  }, [open]);
  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 rounded-lg border border-border bg-surface py-1 pl-1 pr-2 text-left shadow-card hover:border-border-strong"
      >
        <span className={cx("grid size-7 place-items-center rounded-md", data?.persona === "reader" ? "bg-warn-soft text-warn" : "bg-accent-soft text-accent")}>
          <UserRound className="size-4" />
        </span>
        <span className="hidden leading-tight md:block">
          <span className="block text-xs font-semibold">{data?.current.name ?? "…"}</span>
          <span className="block text-[10px] text-subtle">{data?.persona === "reader" ? "Reader" : "Owner"}</span>
        </span>
        <ChevronDown className="size-3.5 text-subtle" />
      </button>
      {open && data && (
        <div className="absolute right-0 top-11 z-30 w-80 rounded-xl border border-border bg-surface p-1.5 shadow-pop animate-in">
          <div className="px-2.5 pb-1.5 pt-1 text-[11px] text-subtle">Act as a seeded identity. Every call uses its own token, so RBAC applies for real.</div>
          {data.personas.map((p) => (
            <button
              key={p.id}
              onClick={async () => {
                setOpen(false);
                if (p.id === data.persona) return;
                await switchTo(p.id);
                toast.push({ tone: "info", title: `Now acting as ${p.name}`, body: p.role });
              }}
              className={cx("flex w-full items-start gap-2.5 rounded-lg px-2.5 py-2 text-left hover:bg-surface-2", p.id === data.persona && "bg-accent-soft/60")}
            >
              <span className={cx("mt-0.5 grid size-7 place-items-center rounded-md", p.id === "reader" ? "bg-warn-soft text-warn" : "bg-accent-soft text-accent")}>
                <UserRound className="size-4" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-semibold">{p.name}</span>
                <span className="block text-xs text-muted">{p.role}</span>
                <span className="block truncate font-mono text-[10px] text-subtle">{p.clientId}</span>
              </span>
              {p.id === data.persona && <StatusDot tone="accent" />}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function ThemeToggle() {
  const toggle = () => {
    const dark = !document.documentElement.classList.contains("dark");
    document.documentElement.classList.toggle("dark", dark);
    localStorage.setItem("ct-theme", dark ? "dark" : "light");
  };
  return (
    <button onClick={toggle} className="grid size-9 place-items-center rounded-lg border border-border bg-surface text-muted shadow-card hover:text-fg" aria-label="Toggle theme">
      <Sun className="hidden size-4 dark:block" />
      <Moon className="size-4 dark:hidden" />
    </button>
  );
}
