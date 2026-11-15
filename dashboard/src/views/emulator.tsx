"use client";

import { Camera, Download, Flame, History, RotateCcw, ServerCog, Sprout, Trash2, Zap } from "lucide-react";
import { useState } from "react";
import { useSWRConfig } from "swr";

import { JsonView } from "@/components/json";
import { ConfirmDialog, useToast } from "@/components/overlay";
import { Badge, Button, Card, CopyButton, EmptyState, ErrorState, Field, Input, Mono, PageHeader, Skeleton, StatusDot, cx } from "@/components/ui";
import { call, useApi, useList } from "@/lib/api";
import type { CoverageRow } from "@/lib/azure";

interface Chaos {
  throttleRate: number;
  errorRate: number;
  latencyMs: number;
  asyncDelaySeconds: number;
  writesOnly: boolean;
}

const refreshAll = (k: unknown) => Array.isArray(k) || k === "palette-index";

export function Emulator() {
  const health = useApi<{ status: string; version: string }>("admin", "/health", { refreshInterval: 5000 });
  const version = useApi<{ version: string; commit: string; date: string }>("admin", "/version");
  const env = useApi<{ env: Record<string, string> }>("admin", "/env");
  const coverage = useList<CoverageRow>("admin", "/coverage");
  const metadata = useApi<Record<string, unknown>>("admin", "/metadata");
  const exports = Object.entries(env.data?.env ?? {})
    .map(([k, v]) => `export ${k}=${JSON.stringify(v)}`)
    .join("\n");

  return (
    <>
      <PageHeader
        icon={<ServerCog className="size-5" />}
        title="Emulator"
        description="Health, connection settings, fault injection and state management for the running CloudTyke emulator."
        actions={
          <a href="/api/emu/admin/ca.pem" download="cloudtyke-ca.pem">
            <Button icon={<Download className="size-4" />}>Download CA certificate</Button>
          </a>
        }
      />
      <div className="grid gap-4 lg:grid-cols-3">
        <Card title="Status">
          {health.error ? (
            <ErrorState error={health.error} retry={() => health.mutate()} />
          ) : (
            <div className="space-y-3 text-sm">
              <div className="flex items-center gap-2">
                <StatusDot tone={health.data?.status === "ok" ? "ok" : "neutral"} pulse={health.data?.status === "ok"} />
                <span className="text-lg font-semibold">{health.data?.status === "ok" ? "Healthy" : "…"}</span>
              </div>
              <dl className="grid grid-cols-[90px_1fr] gap-y-1.5">
                <dt className="text-muted">Version</dt>
                <dd className="font-mono text-xs">{version.data?.version}</dd>
                <dt className="text-muted">Commit</dt>
                <dd className="font-mono text-xs">{version.data?.commit}</dd>
                <dt className="text-muted">Built</dt>
                <dd className="font-mono text-xs">{version.data?.date}</dd>
                <dt className="text-muted">Coverage</dt>
                <dd>
                  {coverage.items.length} ARM resource types · <a className="text-accent hover:underline" href="/services">all services</a>
                </dd>
              </dl>
            </div>
          )}
        </Card>
        <div className="lg:col-span-2">
          <ChaosCard />
        </div>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card title="Environment for Terraform, the az CLI and SDKs" actions={exports && <CopyButton value={exports} label="Copy exports" />}>
          {env.error ? (
            <ErrorState error={env.error} />
          ) : !env.data ? (
            <Skeleton className="h-40" />
          ) : (
            <div className="divide-y divide-border">
              {Object.entries(env.data.env).map(([k, v]) => (
                <div key={k} className="flex items-center gap-3 py-1.5">
                  <span className="w-60 shrink-0 font-mono text-xs font-semibold">{k}</span>
                  <Mono copy className="min-w-0 flex-1">{v}</Mono>
                </div>
              ))}
            </div>
          )}
          <p className="mt-3 text-xs text-subtle">Local-only seeded credentials. Never reuse them anywhere real.</p>
        </Card>
        <StateCard />
      </div>

      <div className="mt-4">
        <Card title="Cloud metadata (metadata_host endpoint)">{metadata.data ? <JsonView value={metadata.data} collapsedDepth={1} /> : <Skeleton className="h-24" />}</Card>
      </div>
    </>
  );
}

function Slider({ label, value, min, max, step, unit, onChange, hint }: { label: string; value: number; min: number; max: number; step: number; unit: string; onChange: (v: number) => void; hint: string }) {
  return (
    <Field label={label} hint={hint}>
      <div className="flex items-center gap-3">
        <input type="range" min={min} max={max} step={step} value={value} onChange={(e) => onChange(Number(e.target.value))} className="flex-1 accent-[var(--accent)]" />
        <span className="w-16 text-right font-mono text-sm tabular-nums">
          {unit === "%" ? Math.round(value * 100) : value}
          {unit}
        </span>
      </div>
    </Field>
  );
}

function ChaosCard() {
  const chaos = useApi<Chaos>("admin", "/chaos");
  const [edit, setC] = useState<Chaos | null>(null);
  const c = edit ?? chaos.data ?? null;
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const active = !!chaos.data && (chaos.data.throttleRate > 0 || chaos.data.errorRate > 0 || chaos.data.latencyMs > 0 || chaos.data.asyncDelaySeconds > 0);
  const dirty = c && chaos.data && JSON.stringify(c) !== JSON.stringify(chaos.data);
  const save = async (body: Chaos | null) => {
    setBusy(true);
    try {
      if (body) await call("admin", "/chaos", { method: "PUT", body });
      else await call("admin", "/chaos", { method: "DELETE" });
      toast.push({ tone: body ? "info" : "ok", title: body ? "Chaos settings applied" : "Chaos cleared", body: body ? "Every ARM call now goes through these faults." : undefined });
      setC(null);
      chaos.mutate();
    } catch (e) {
      toast.error(e, "Could not update chaos");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Card
      title={
        <span className="flex items-center gap-2">
          <Flame className={cx("size-4", active ? "text-bad" : "text-subtle")} /> Fault injection
          {active && <Badge tone="bad">active</Badge>}
        </span>
      }
      actions={
        <>
          <Button size="sm" variant="ghost" disabled={!active} loading={busy && !dirty} onClick={() => save(null)}>
            Clear
          </Button>
          <Button size="sm" variant="primary" disabled={!dirty} loading={busy && !!dirty} icon={<Zap className="size-3.5" />} onClick={() => c && save(c)}>
            Apply
          </Button>
        </>
      }
    >
      {chaos.error ? (
        <ErrorState error={chaos.error} />
      ) : !c ? (
        <Skeleton className="h-40" />
      ) : (
        <div className="grid gap-5 sm:grid-cols-2">
          <Slider label="Throttling (429)" value={c.throttleRate} min={0} max={1} step={0.05} unit="%" hint="Tests retry-after handling in Terraform and SDKs" onChange={(v) => setC({ ...c, throttleRate: v })} />
          <Slider label="Server errors (500/503)" value={c.errorRate} min={0} max={1} step={0.05} unit="%" hint="Transient failures" onChange={(v) => setC({ ...c, errorRate: v })} />
          <Slider label="Added latency" value={c.latencyMs} min={0} max={3000} step={50} unit=" ms" hint="Added to every ARM request" onChange={(v) => setC({ ...c, latencyMs: v })} />
          <Slider label="Async operations take" value={c.asyncDelaySeconds} min={0} max={60} step={1} unit=" s" hint="How long deletes stay InProgress" onChange={(v) => setC({ ...c, asyncDelaySeconds: v })} />
          <label className="flex items-center gap-2 text-sm sm:col-span-2">
            <input type="checkbox" checked={c.writesOnly} onChange={(e) => setC({ ...c, writesOnly: e.target.checked })} className="size-4 accent-[var(--accent)]" />
            Only affect writes (PUT, PATCH, DELETE, POST)
          </label>
        </div>
      )}
    </Card>
  );
}

function StateCard() {
  const snaps = useApi<{ value: string[] | null }>("admin", "/snapshots");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [confirm, setConfirm] = useState<null | "reset" | { restore: string } | { del: string }>(null);
  const toast = useToast();
  const { mutate } = useSWRConfig();
  const act = async (key: string, fn: () => Promise<unknown>, ok: string) => {
    setBusy(key);
    try {
      await fn();
      toast.push({ tone: "ok", title: ok });
      await mutate(refreshAll);
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(null);
    }
  };
  const list = snaps.data?.value ?? [];
  return (
    <Card title="State">
      <div className="space-y-4">
        <div className="flex flex-wrap gap-2">
          <Button icon={<Sprout className="size-4" />} loading={busy === "seed"} onClick={() => act("seed", () => call("admin", "/seed", { method: "POST" }), "Seed re-applied")}>
            Re-apply seed
          </Button>
          <Button className="text-bad" icon={<RotateCcw className="size-4" />} onClick={() => setConfirm("reset")}>
            Reset to seed
          </Button>
          <a href="/api/emu/admin/state" download="cloudtyke-state.json">
            <Button variant="ghost" icon={<Download className="size-4" />}>
              Export state
            </Button>
          </a>
        </div>
        <div>
          <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-subtle">Snapshots</div>
          <form
            className="mb-2 flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (name) act("save", () => call("admin", `/snapshots/${encodeURIComponent(name)}`, { method: "POST" }), `Saved snapshot ${name}`).then(() => setName(""));
            }}
          >
            <Input value={name} onChange={(e) => setName(e.target.value.replace(/[^\w.-]/g, "-"))} placeholder="before-demo" />
            <Button type="submit" variant="primary" icon={<Camera className="size-4" />} loading={busy === "save"} disabled={!name}>
              Save
            </Button>
          </form>
          {snaps.error ? (
            <ErrorState error={snaps.error} />
          ) : !list.length ? (
            <EmptyState icon={<History className="size-5" />} title="No snapshots" description="Save one before a demo, then roll back in a click." />
          ) : (
            <div className="divide-y divide-border rounded-lg border border-border">
              {list.map((s) => (
                <div key={s} className="flex items-center gap-2 px-3 py-2">
                  <History className="size-4 text-subtle" />
                  <span className="flex-1 font-mono text-sm">{s}</span>
                  <Button size="sm" icon={<RotateCcw className="size-3.5" />} onClick={() => setConfirm({ restore: s })}>
                    Restore
                  </Button>
                  <Button size="sm" variant="ghost" className="text-bad" icon={<Trash2 className="size-3.5" />} aria-label="Delete snapshot" onClick={() => setConfirm({ del: s })} />
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
      <ConfirmDialog
        open={confirm !== null}
        onClose={() => setConfirm(null)}
        title={confirm === "reset" ? "Reset the emulator?" : confirm && "restore" in confirm ? `Restore snapshot ${confirm.restore}?` : `Delete snapshot ${confirm && "del" in confirm ? confirm.del : ""}?`}
        description={confirm === "reset" ? "Everything you created is wiped and the seed is applied again." : confirm && "restore" in confirm ? "Current state is replaced by the snapshot." : "The snapshot file is removed."}
        confirmLabel={confirm === "reset" ? "Reset" : confirm && "restore" in confirm ? "Restore" : "Delete"}
        onConfirm={async () => {
          if (confirm === "reset") await act("reset", () => call("admin", "/reset", { method: "POST" }), "Emulator reset to seed");
          else if (confirm && "restore" in confirm) await act("restore", () => call("admin", `/snapshots/${confirm.restore}/restore`, { method: "POST" }), `Restored ${confirm.restore}`);
          else if (confirm && "del" in confirm) await act("del", () => call("admin", `/snapshots/${confirm.del}`, { method: "DELETE" }), `Deleted ${confirm.del}`);
        }}
      />
    </Card>
  );
}
