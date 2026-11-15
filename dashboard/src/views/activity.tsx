"use client";

import { Activity as ActivityIcon, Pause, Play } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";

import { JsonView } from "@/components/json";
import { Drawer } from "@/components/overlay";
import { DataTable } from "@/components/table";
import { Badge, Button, Mono, PageHeader, Select, StatusDot, Tabs, cx, statusTone } from "@/components/ui";
import { API, armPath, useList } from "@/lib/api";
import { ago, type ActivityEvent, type RequestLogEntry } from "@/lib/azure";
import { useRequests, useSubscriptions } from "@/lib/hooks";
import type { ServicePrincipal, User } from "@/lib/azure";

const VERBS: Record<string, string> = { write: "Create or update", delete: "Delete", read: "Read", action: "Run action" };

/** "Microsoft.Resources/subscriptions/resourceGroups/write" -> "Create or update resource group". */
export function humanOp(op?: string) {
  if (!op) return "Event";
  const parts = op.split("/");
  const verb = parts.pop()!;
  const noun = (parts.pop() ?? "")
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .toLowerCase()
    .replace(/s$/, "");
  return `${VERBS[verb] ?? verb} ${noun}`.trim();
}

/** Activity-log callers are app IDs (service principals) or user principal names. */
function useCallerNames() {
  const sps = useList<ServicePrincipal>("graph", "/v1.0/servicePrincipals?$top=999");
  const users = useList<User>("graph", "/v1.0/users?$top=999");
  return useMemo(() => {
    const m: Record<string, string> = {};
    for (const s of sps.items) if (s.displayName) m[s.appId] = s.displayName;
    for (const u of users.items) if (u.displayName) m[u.userPrincipalName] = m[u.id] = u.displayName;
    return m;
  }, [sps.items, users.items]);
}

type Tab = "activity" | "http";

export function Activity() {
  const params = useSearchParams();
  const [tab, setTab] = useState<Tab>((params.get("tab") as Tab) || "activity");
  return (
    <>
      <PageHeader
        icon={<ActivityIcon className="size-5" />}
        title="Activity"
        description="The Azure Activity Log for control-plane writes, and every HTTP request the emulator has served: Terraform, the SDKs, the CLI and this dashboard."
      />
      <div className="mb-4">
        <Tabs<Tab>
          value={tab}
          onChange={setTab}
          tabs={[
            { id: "activity", label: "Activity log" },
            { id: "http", label: "Live HTTP stream" },
          ]}
        />
      </div>
      {tab === "activity" ? <ActivityLog /> : <HttpStream />}
    </>
  );
}

function ActivityLog() {
  const subs = useSubscriptions();
  const [sub, setSub] = useState<string>("");
  const s = sub || subs.items.find((x) => x.displayName === "sub-sandbox")?.subscriptionId || subs.items[0]?.subscriptionId;
  const ev = useList<ActivityEvent>("arm", s ? armPath(`/subscriptions/${s}/providers/Microsoft.Insights/eventtypes/management/values`, API.activity) : null, { refreshInterval: 5000 });
  const callers = useCallerNames();
  const [open, setOpen] = useState<ActivityEvent | null>(null);
  const rows = useMemo(() => [...ev.items].sort((a, b) => b.eventTimestamp.localeCompare(a.eventTimestamp)), [ev.items]);
  return (
    <>
      <DataTable
        rows={rows}
        loading={ev.isLoading}
        error={ev.error}
        retry={() => ev.mutate()}
        rowKey={(e) => e.eventDataId}
        onRowClick={setOpen}
        searchText={(e) => `${e.operationName?.value} ${humanOp(e.operationName?.value)} ${e.resourceGroupName} ${e.resourceId} ${e.status?.value} ${callers[e.caller] ?? e.caller}`}
        searchPlaceholder="Filter by operation, resource, caller, status…"
        toolbar={
          <Select value={s ?? ""} onChange={(e) => setSub(e.target.value)} className="h-8 w-44 text-xs">
            {subs.items.map((x) => (
              <option key={x.subscriptionId} value={x.subscriptionId}>
                {x.displayName}
              </option>
            ))}
          </Select>
        }
        columns={[
          {
            key: "op",
            header: "Operation",
            render: (e) => (
              <span className="flex flex-col">
                <span className="font-medium">{humanOp(e.operationName?.value)}</span>
                <span className="font-mono text-[11px] text-subtle">{e.operationName?.value}</span>
              </span>
            ),
          },
          { key: "st", header: "Status", render: (e) => <Badge tone={statusTone(e.status?.value)}>{e.status?.value}</Badge> },
          { key: "rg", header: "Resource group", render: (e) => <span className="text-muted">{e.resourceGroupName || "—"}</span> },
          { key: "c", header: "Caller", render: (e) => <span className="text-xs">{callers[e.caller] ?? <Mono>{e.caller}</Mono>}</span> },
          { key: "t", header: "Time", sort: (e) => e.eventTimestamp, render: (e) => <span className="whitespace-nowrap text-muted" title={e.eventTimestamp}>{ago(e.eventTimestamp)}</span> },
        ]}
      />
      <Drawer open={!!open} onClose={() => setOpen(null)} title={humanOp(open?.operationName?.value)} subtitle={open?.eventTimestamp} wide>
        {open && (
          <div className="p-5">
            <JsonView value={open} collapsedDepth={2} maxHeight="none" />
          </div>
        )}
      </Drawer>
    </>
  );
}

const SVC_TONE: Record<string, string> = { arm: "text-accent", graph: "text-ok", login: "text-warn", admin: "text-subtle" };

function HttpStream() {
  const [live, setLive] = useState(true);
  const [svc, setSvc] = useState("all");
  const [hideDash, setHideDash] = useState(true);
  const r = useRequests(500, live ? 1500 : 0);
  const rows = useMemo(
    () =>
      [...r.items]
        .reverse()
        .filter((x) => (svc === "all" || x.service === svc) && (!hideDash || !x.path.startsWith("/_cloudtyke"))),
    [r.items, svc, hideDash],
  );
  const stats = useMemo(() => {
    const n = r.items.length || 1;
    const errs = r.items.filter((x) => x.status >= 400).length;
    const sorted = r.items.map((x) => x.durationMs).sort((a, b) => a - b);
    return { total: r.items.length, errRate: Math.round((errs / n) * 100), p95: sorted[Math.floor(sorted.length * 0.95)] ?? 0 };
  }, [r.items]);
  const [open, setOpen] = useState<RequestLogEntry | null>(null);
  return (
    <>
      <div className="mb-3 grid grid-cols-3 gap-3 sm:w-[480px]">
        <Mini label="Requests (last 500)" value={stats.total} />
        <Mini label="Error rate" value={`${stats.errRate}%`} tone={stats.errRate > 10 ? "text-bad" : undefined} />
        <Mini label="p95 latency" value={`${stats.p95} ms`} />
      </div>
      <DataTable
        rows={rows}
        loading={r.isLoading}
        error={r.error}
        rowKey={(x) => x.requestId}
        onRowClick={setOpen}
        searchText={(x) => `${x.method} ${x.path} ${x.status} ${x.host} ${x.error ?? ""}`}
        searchPlaceholder="Filter by path, method, status…"
        toolbar={
          <>
            <Select value={svc} onChange={(e) => setSvc(e.target.value)} className="h-8 w-28 text-xs">
              <option value="all">All services</option>
              <option value="arm">ARM</option>
              <option value="graph">Graph</option>
              <option value="login">Login</option>
            </Select>
            <label className="flex items-center gap-1.5 text-xs">
              <input type="checkbox" checked={hideDash} onChange={(e) => setHideDash(e.target.checked)} className="accent-[var(--accent)]" />
              hide admin calls
            </label>
            <Button size="sm" variant={live ? "secondary" : "primary"} icon={live ? <Pause className="size-3.5" /> : <Play className="size-3.5" />} onClick={() => setLive(!live)}>
              {live ? (
                <span className="flex items-center gap-1.5">
                  <StatusDot tone="ok" pulse /> Live
                </span>
              ) : (
                "Resume"
              )}
            </Button>
          </>
        }
        columns={[
          { key: "s", header: "Status", render: (x) => <Badge tone={statusTone(x.status)}>{x.status}</Badge> },
          { key: "m", header: "Method", render: (x) => <span className="font-mono text-xs font-bold">{x.method}</span> },
          { key: "svc", header: "Service", render: (x) => <span className={cx("text-xs font-semibold", SVC_TONE[x.service])}>{x.service}</span> },
          {
            key: "p",
            header: "Path",
            render: (x) => (
              <span className="flex min-w-0 flex-col">
                <span className="max-w-[520px] truncate font-mono text-xs">{x.path}</span>
                {x.error && <span className="truncate text-[11px] text-bad">{x.error}</span>}
              </span>
            ),
          },
          { key: "d", header: "Duration", sort: (x) => x.durationMs, render: (x) => <span className="tabular-nums text-muted">{x.durationMs} ms</span> },
          { key: "t", header: "When", render: (x) => <span className="whitespace-nowrap text-muted">{ago(x.time)}</span> },
        ]}
      />
      <Drawer open={!!open} onClose={() => setOpen(null)} title={open && `${open.method} ${open.status}`} subtitle={open?.path}>
        {open && (
          <div className="p-5">
            <JsonView value={open} />
          </div>
        )}
      </Drawer>
    </>
  );
}

function Mini({ label, value, tone }: { label: string; value: React.ReactNode; tone?: string }) {
  return (
    <div className="rounded-xl border border-border bg-surface px-3 py-2 shadow-card">
      <div className="text-[11px] text-muted">{label}</div>
      <div className={cx("text-lg font-semibold tabular-nums", tone)}>{value}</div>
    </div>
  );
}
