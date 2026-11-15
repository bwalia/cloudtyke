"use client";

import { AppWindow, ArrowRight, Boxes, Compass, FolderTree, KeyRound, Network, Play, ShieldCheck, UserRound, UsersRound } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import { CopyButton, Badge, Button, Card, EmptyState, PageHeader, Skeleton, Stat, StatusDot, cx, statusTone } from "@/components/ui";
import { API, armPath, useList } from "@/lib/api";
import { SECTION_LABEL, ago, type Application, type Group, type RoleAssignment, type ServicePrincipal, type User } from "@/lib/azure";
import { selfTestReport, useAllResourceGroups, useHierarchy, useRequests, useSelfTest, useSubscriptions, type MgNode } from "@/lib/hooks";
import { useSession } from "@/components/session";

export function Overview() {
  const st = useSelfTest();
  const report = selfTestReport(st);
  const subs = useSubscriptions();
  const rgs = useAllResourceGroups(subs.items);
  const users = useList<User>("graph", "/v1.0/users?$top=999");
  const groups = useList<Group>("graph", "/v1.0/groups?$top=999");
  const apps = useList<Application>("graph", "/v1.0/applications?$top=999");
  const sps = useList<ServicePrincipal>("graph", "/v1.0/servicePrincipals?$top=999");
  const ras = useList<RoleAssignment>("arm", armPath("/providers/Microsoft.Authorization/roleAssignments", API.auth));
  const { data: session } = useSession();

  const count = (r: { items: unknown[]; isLoading: boolean; error?: unknown }) => (r.error ? "—" : r.isLoading && !r.items.length ? <Skeleton className="h-7 w-10" /> : r.items.length);

  return (
    <>
      <PageHeader
        title="Overview"
        description={
          <>
            Tenant <span className="font-mono text-xs">{session?.tenantId ?? "…"}</span> on <span className="font-mono text-xs">{session?.domain ?? "…"}</span>. Everything below is
            read live from the emulator&apos;s real APIs.
          </>
        }
        actions={
          <>
            <Link href="/tour">
              <Button icon={<Compass className="size-4" />}>Take the tour</Button>
            </Link>
            <Button variant="primary" icon={<Play className="size-4" />} loading={st.isValidating} onClick={() => st.mutate()}>
              Run self-test
            </Button>
          </>
        }
      />

      <div className="space-y-4">
        <HealthHero report={report} loading={st.isLoading} error={!report ? st.error : undefined} />
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 2xl:grid-cols-8">
          <Stat label="Subscriptions" value={count(subs)} href="/resources" icon={<KeyRound className="size-4" />} />
          <Stat label="Resource groups" value={rgs.error ? "—" : rgs.data ? rgs.data.length : <Skeleton className="h-7 w-10" />} href="/resources" icon={<FolderTree className="size-4" />} />
          <Stat label="Users" value={count(users)} href="/identity?tab=users" icon={<UserRound className="size-4" />} />
          <Stat label="Groups" value={count(groups)} href="/identity?tab=groups" icon={<UsersRound className="size-4" />} />
          <Stat label="App registrations" value={count(apps)} href="/identity?tab=apps" icon={<AppWindow className="size-4" />} />
          <Stat label="Enterprise apps" value={count(sps)} href="/identity?tab=sps" icon={<Boxes className="size-4" />} />
          <Stat label="Role assignments" value={count(ras)} hint="visible to you" href="/access" icon={<ShieldCheck className="size-4" />} />
          <Stat label="Services tested" value={report ? report.results.length : <Skeleton className="h-7 w-10" />} href="/services" icon={<Network className="size-4" />} />
        </div>
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[1.25fr_1fr]">
        <ServiceGrid report={report} />
        <Hierarchy />
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[1.25fr_1fr]">
        <LiveTraffic />
        <ConnectTools domain={session?.domain} tenant={session?.tenantId} />
      </div>
    </>
  );
}

function Ring({ value, total, size = 112 }: { value: number; total: number; size?: number }) {
  const r = size / 2 - 8;
  const c = 2 * Math.PI * r;
  const pct = total ? value / total : 0;
  const ok = value === total && total > 0;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="shrink-0 -rotate-90">
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--border)" strokeWidth="9" />
      <circle
        cx={size / 2}
        cy={size / 2}
        r={r}
        fill="none"
        stroke={ok ? "var(--ok)" : "var(--bad)"}
        strokeWidth="9"
        strokeLinecap="round"
        strokeDasharray={`${c * pct} ${c}`}
        style={{ transition: "stroke-dasharray .6s ease" }}
      />
    </svg>
  );
}

function HealthHero({ report, loading, error }: { report?: import("@/lib/azure").SelfTestReport; loading: boolean; error?: Error }) {
  if (error)
    return (
      <Card>
        <EmptyState title="Self-test unavailable" description={error.message} />
      </Card>
    );
  const total = report ? report.passed + report.failed : 0;
  const sections = report ? [...new Set(report.results.map((r) => r.section))] : [];
  return (
    <section className="relative overflow-hidden rounded-xl border border-border bg-surface p-5 shadow-card">
      <div className="pointer-events-none absolute -right-20 -top-24 size-72 rounded-full bg-accent/10 blur-3xl" />
      <div className="relative flex flex-wrap items-center gap-6">
        <div className="relative">
          <Ring value={report?.passed ?? 0} total={total} />
          <div className="absolute inset-0 grid place-items-center text-center">
            {report ? (
              <div>
                <div className="text-2xl font-bold tabular-nums">{Math.round((report.passed / Math.max(1, total)) * 100)}%</div>
                <div className="text-[10px] uppercase tracking-wider text-subtle">passing</div>
              </div>
            ) : (
              <Skeleton className="h-6 w-10" />
            )}
          </div>
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            {report ? <StatusDot tone={report.failed ? "bad" : "ok"} pulse /> : null}
            <h2 className="text-lg font-semibold">
              {loading && !report ? "Testing every service…" : report?.failed ? `${report.failed} service${report.failed > 1 ? "s" : ""} failing` : `All ${total} services working`}
            </h2>
          </div>
          <p className="mt-1 text-sm text-muted">
            Each service is exercised end-to-end over its real HTTP API against a throwaway in-memory copy of the emulator, so your data is never touched.
          </p>
          <div className="mt-4 flex flex-wrap gap-2">
            {sections.map((s) => {
              const rs = report!.results.filter((r) => r.section === s);
              const p = rs.filter((r) => r.status === "pass").length;
              return (
                <Link key={s} href={`/services?section=${s}`} className="rounded-lg border border-border bg-surface-2/60 px-3 py-2 transition-colors hover:border-border-strong">
                  <div className="text-[11px] text-muted">{SECTION_LABEL[s] ?? s}</div>
                  <div className={cx("text-sm font-semibold tabular-nums", p === rs.length ? "text-ok" : "text-bad")}>
                    {p}/{rs.length}
                  </div>
                </Link>
              );
            })}
            {report && (
              <div className="ml-auto self-end text-xs text-subtle">
                {report.results.reduce((n, r) => n + (r.calls?.length ?? 0), 0)} HTTP calls in {report.durationMs} ms · {ago(report.started)}
              </div>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}

function ServiceGrid({ report }: { report?: import("@/lib/azure").SelfTestReport }) {
  return (
    <Card
      title="Service health"
      actions={
        <Link href="/services" className="flex items-center gap-1 text-xs font-medium text-accent hover:underline">
          All services <ArrowRight className="size-3" />
        </Link>
      }
    >
      {!report ? (
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
          {Array.from({ length: 9 }).map((_, i) => (
            <Skeleton key={i} className="h-14" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 2xl:grid-cols-3">
          {report.results.map((r) => (
            <Link
              key={r.service}
              href={`/services?id=${encodeURIComponent(r.service)}`}
              className={cx(
                "group flex items-start gap-2.5 rounded-lg border px-3 py-2.5 transition-all hover:-translate-y-px hover:shadow-card",
                r.status === "pass" ? "border-border hover:border-ok/40" : "border-bad/40 bg-bad-soft/40",
              )}
            >
              <span className="mt-1">
                <StatusDot tone={statusTone(r.status)} />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block break-all font-mono text-[12px] font-medium leading-snug">{r.service.replace(/^Microsoft\./, "")}</span>
                <span className="block truncate text-[11px] text-subtle">
                  {r.section} · {r.calls?.length ?? 0} calls · {r.durationMs} ms
                </span>
              </span>
            </Link>
          ))}
        </div>
      )}
    </Card>
  );
}

function MgTree({ node, depth = 0 }: { node: MgNode; depth?: number }) {
  return (
    <div className={cx(depth > 0 && "ml-4 border-l border-border pl-3")}>
      <div className="flex items-center gap-2 py-1">
        <span className="grid size-6 place-items-center rounded-md bg-accent-soft text-accent">
          <Network className="size-3.5" />
        </span>
        <span className="text-sm font-medium">{node.displayName}</span>
        <span className="font-mono text-[10px] text-subtle">{depth === 0 ? "tenant root" : node.name}</span>
      </div>
      {node.subscriptions.map((s) => (
        <Link key={s.subscriptionId} href={`/resources?sub=${s.subscriptionId}`} className="ml-4 flex items-center gap-2 border-l border-border py-1 pl-3 hover:text-accent">
          <span className="grid size-6 place-items-center rounded-md bg-warn-soft text-warn">
            <KeyRound className="size-3.5" />
          </span>
          <span className="text-sm">{s.displayName}</span>
          <Badge tone={statusTone(s.state)}>{s.state}</Badge>
        </Link>
      ))}
      {node.children.map((c) => (
        <MgTree key={c.name} node={c} depth={depth + 1} />
      ))}
    </div>
  );
}

function Hierarchy() {
  const { tree, isLoading, error } = useHierarchy();
  return (
    <Card title="Management hierarchy" actions={<span className="text-xs text-subtle">management groups → subscriptions</span>}>
      {error ? (
        <EmptyState title="Hierarchy unavailable" description={error.message} />
      ) : isLoading || !tree ? (
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-6" />
          ))}
        </div>
      ) : (
        <MgTree node={tree} />
      )}
    </Card>
  );
}

function useNow(ms: number) {
  const [now, setNow] = useState(0);
  useEffect(() => {
    const tick = () => setNow(Date.now());
    const t = setInterval(tick, ms);
    const first = setTimeout(tick, 0);
    return () => {
      clearInterval(t);
      clearTimeout(first);
    };
  }, [ms]);
  return now;
}

function LiveTraffic() {
  const { items, error } = useRequests(500, 2500);
  const now = useNow(2500);
  const buckets = useMemo(() => {
    const n = 30;
    const span = 10_000;
    const b = Array.from({ length: n }, () => ({ ok: 0, err: 0 }));
    for (const r of items) {
      const i = n - 1 - Math.floor((now - new Date(r.time).getTime()) / span);
      if (i >= 0 && i < n) b[i][r.status >= 400 ? "err" : "ok"]++;
    }
    return b;
  }, [items, now]);
  const max = Math.max(1, ...buckets.map((b) => b.ok + b.err));
  const recent = [...items].reverse().slice(0, 8);
  return (
    <Card
      title={
        <span className="flex items-center gap-2">
          <StatusDot tone="ok" pulse /> Live traffic
        </span>
      }
      actions={
        <Link href="/activity?tab=http" className="flex items-center gap-1 text-xs font-medium text-accent hover:underline">
          Open stream <ArrowRight className="size-3" />
        </Link>
      }
    >
      {error ? (
        <EmptyState title="Request log unavailable" description={error.message} />
      ) : (
        <>
          <div className="flex h-20 items-end gap-[3px]" aria-label="Requests per 10 seconds, last 5 minutes">
            {buckets.map((b, i) => (
              <div key={i} className="flex flex-1 flex-col justify-end" title={`${b.ok + b.err} requests (${b.err} errors)`}>
                <div className="rounded-t-[2px] bg-bad/80" style={{ height: `${(b.err / max) * 72}px` }} />
                <div className={cx("bg-accent/70", !b.err && "rounded-t-[2px]")} style={{ height: `${(b.ok / max) * 72}px`, minHeight: b.ok ? 2 : 0 }} />
              </div>
            ))}
          </div>
          <div className="mt-1 flex justify-between text-[10px] text-subtle">
            <span>5 min ago</span>
            <span>now</span>
          </div>
          <div className="mt-3 divide-y divide-border">
            {recent.length === 0 && <div className="py-4 text-center text-sm text-muted">No requests yet. Run Terraform or click around.</div>}
            {recent.map((r) => (
              <div key={r.requestId} className="flex items-center gap-3 py-1.5 text-xs">
                <Badge tone={statusTone(r.status)}>{r.status}</Badge>
                <span className="w-12 font-mono font-semibold text-muted">{r.method}</span>
                <span className="w-14 text-subtle">{r.service}</span>
                <span className="min-w-0 flex-1 truncate font-mono">{r.path}</span>
                <span className="tabular-nums text-subtle">{ago(r.time)}</span>
              </div>
            ))}
          </div>
        </>
      )}
    </Card>
  );
}

function ConnectTools({ domain = "cloudtyke.local", tenant = "" }: { domain?: string; tenant?: string }) {
  const tf = `provider "azurerm" {
  features {}
  metadata_host   = "management.${domain}"
  tenant_id       = "${tenant}"
  subscription_id = "5b5c0000-0000-4000-8000-000000000003"
  client_id       = "a99c0000-0000-4000-8000-000000000001"
  # client_secret from ARM_CLIENT_SECRET
}`;
  return (
    <Card title="Point your tools at the emulator" actions={<CopyButton value={tf} label="Copy" />}>
      <pre className="overflow-x-auto rounded-lg border border-border bg-surface-2/60 p-3 font-mono text-[12px] leading-relaxed">{tf}</pre>
      <div className="mt-3 flex flex-wrap gap-2 text-xs text-muted">
        <span>Works with azurerm, azuread, azapi, the az CLI and the Azure SDKs.</span>
        <Link href="/emulator" className="font-medium text-accent hover:underline">
          Environment variables →
        </Link>
      </div>
    </Card>
  );
}

