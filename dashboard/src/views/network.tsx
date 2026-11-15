"use client";

import { Network as NetworkIcon, RefreshCw, Shield, Waypoints } from "lucide-react";
import { useMemo, useState } from "react";
import useSWR from "swr";

import { JsonView } from "@/components/json";
import { Drawer } from "@/components/overlay";
import { DataTable } from "@/components/table";
import { Badge, Button, Card, EmptyState, ErrorState, Mono, PageHeader, Skeleton, cx, statusTone } from "@/components/ui";
import { API, armPath, call } from "@/lib/api";
import type { Subscription } from "@/lib/azure";
import { useSubscriptions } from "@/lib/hooks";

interface Ref {
  id: string;
}
interface Subnet {
  id: string;
  name: string;
  properties: { addressPrefix?: string; addressPrefixes?: string[]; networkSecurityGroup?: Ref; routeTable?: Ref; ipConfigurations?: Ref[]; delegations?: unknown[] };
}
interface Peering {
  id: string;
  name: string;
  properties: { peeringState: string; remoteVirtualNetwork: Ref; allowForwardedTraffic?: boolean; allowGatewayTransit?: boolean; remoteAddressSpace?: { addressPrefixes: string[] } };
}
interface VNet {
  id: string;
  name: string;
  location: string;
  tags?: Record<string, string>;
  properties: { addressSpace: { addressPrefixes: string[] }; subnets?: Subnet[]; virtualNetworkPeerings?: Peering[]; dnsServers?: string[]; dhcpOptions?: { dnsServers?: string[] } };
  subscription: Subscription;
}

const lower = (s: string) => s.toLowerCase();
const rgOf = (id: string) => id.split("/")[4] ?? "";

function useVNets(subs: Subscription[]) {
  const key = subs.map((s) => s.subscriptionId).join(",");
  return useSWR(key ? ["vnets", key] : null, async () => {
    const lists = await Promise.all(
      subs.map((s) =>
        call<{ value: Omit<VNet, "subscription">[] }>("arm", armPath(`/subscriptions/${s.subscriptionId}/providers/Microsoft.Network/virtualNetworks`, API.network))
          .then((r) => r.value.map((v) => ({ ...v, subscription: s }) as VNet))
          .catch(() => [] as VNet[]),
      ),
    );
    return lists.flat().sort((a, b) => a.name.localeCompare(b.name));
  });
}

export function Network() {
  const subs = useSubscriptions();
  const vnets = useVNets(subs.items);
  const [open, setOpen] = useState<VNet | null>(null);
  const list = vnets.data ?? [];
  const managed = list.filter((v) => !rgOf(v.id).startsWith("MC_"));

  return (
    <>
      <PageHeader
        icon={<NetworkIcon className="size-5" />}
        title="Network"
        description="Every virtual network you can see across subscriptions, drawn as a live topology: peerings turn green only when both sides are peered, exactly like Azure's peeringState."
        actions={
          <Button icon={<RefreshCw className="size-4" />} loading={vnets.isValidating} onClick={() => vnets.mutate()}>
            Refresh
          </Button>
        }
      />
      {vnets.error ? (
        <Card padded={false}>
          <ErrorState error={vnets.error} retry={() => vnets.mutate()} />
        </Card>
      ) : !vnets.data ? (
        <Skeleton className="h-[460px] rounded-xl" />
      ) : !list.length ? (
        <Card>
          <EmptyState
            icon={<Waypoints className="size-5" />}
            title="No virtual networks yet"
            description={
              <>
                Apply <code className="font-mono text-xs">examples/terraform/landing-zone</code> to see a hub-and-spoke platform across three subscriptions appear here.
              </>
            }
          />
        </Card>
      ) : (
        <Topology vnets={managed.length ? managed : list} onOpen={setOpen} />
      )}

      <div className="mt-4">
        <DataTable
          rows={list}
          loading={!vnets.data && !vnets.error}
          rowKey={(v) => v.id}
          onRowClick={setOpen}
          searchText={(v) => `${v.name} ${v.properties.addressSpace.addressPrefixes.join(" ")} ${v.subscription.displayName} ${rgOf(v.id)}`}
          searchPlaceholder="Filter by name, CIDR, subscription…"
          columns={[
            { key: "n", header: "Virtual network", sort: (v) => v.name, render: (v) => <span className="font-medium">{v.name}</span> },
            { key: "a", header: "Address space", render: (v) => <Mono>{v.properties.addressSpace.addressPrefixes.join(", ")}</Mono> },
            { key: "s", header: "Subnets", sort: (v) => v.properties.subnets?.length ?? 0, render: (v) => <span className="tabular-nums">{v.properties.subnets?.length ?? 0}</span> },
            {
              key: "p",
              header: "Peerings",
              render: (v) => (
                <span className="flex flex-wrap gap-1">
                  {(v.properties.virtualNetworkPeerings ?? []).map((p) => (
                    <Badge key={p.id} tone={p.properties.peeringState === "Connected" ? "ok" : p.properties.peeringState === "Initiated" ? "warn" : "bad"}>
                      {p.name}
                    </Badge>
                  ))}
                  {!v.properties.virtualNetworkPeerings?.length && <span className="text-subtle">—</span>}
                </span>
              ),
            },
            { key: "sub", header: "Subscription", sort: (v) => v.subscription.displayName, render: (v) => <span className="text-muted">{v.subscription.displayName}</span> },
            { key: "rg", header: "Resource group", render: (v) => <span className="text-xs text-muted">{rgOf(v.id)}</span> },
          ]}
        />
      </div>
      <VNetDrawer vnet={open} onClose={() => setOpen(null)} />
    </>
  );
}

function Topology({ vnets, onOpen }: { vnets: VNet[]; onOpen: (v: VNet) => void }) {
  const W = 1000;
  const H = 480;
  const layout = useMemo(() => {
    const degree = new Map<string, number>();
    for (const v of vnets) degree.set(lower(v.id), v.properties.virtualNetworkPeerings?.length ?? 0);
    const sorted = [...vnets].sort((a, b) => (degree.get(lower(b.id)) ?? 0) - (degree.get(lower(a.id)) ?? 0));
    const hub = sorted[0] && (degree.get(lower(sorted[0].id)) ?? 0) >= 2 ? sorted[0] : null;
    const ring = vnets.filter((v) => v !== hub);
    const pos = new Map<string, { x: number; y: number }>();
    const cx = W / 2;
    const cy = H / 2;
    if (hub) pos.set(lower(hub.id), { x: cx, y: cy });
    const rx = hub ? 360 : 380;
    const ry = hub ? 170 : 175;
    ring.forEach((v, i) => {
      const a = -Math.PI / 2 + (2 * Math.PI * i) / Math.max(1, ring.length);
      pos.set(lower(v.id), { x: cx + rx * Math.cos(a), y: cy + ry * Math.sin(a) });
    });
    // one edge per pair, coloured by the combined state
    const edges: { a: string; b: string; state: string }[] = [];
    const seen = new Set<string>();
    for (const v of vnets) {
      for (const p of v.properties.virtualNetworkPeerings ?? []) {
        const a = lower(v.id);
        const b = lower(p.properties.remoteVirtualNetwork.id);
        const key = [a, b].sort().join("|");
        if (seen.has(key) || !pos.has(b)) continue;
        seen.add(key);
        edges.push({ a, b, state: p.properties.peeringState });
      }
    }
    return { pos, edges, hub };
  }, [vnets]);

  const color = (s: string) => (s === "Connected" ? "var(--ok)" : s === "Initiated" ? "var(--warn)" : "var(--bad)");
  return (
    <Card
      padded={false}
      title="Topology"
      actions={
        <span className="flex items-center gap-3 text-xs text-muted">
          <Legend color="var(--ok)" label="Connected" />
          <Legend color="var(--warn)" label="Initiated (one side)" dashed />
          <Legend color="var(--bad)" label="Disconnected" />
        </span>
      }
    >
      <div className="overflow-x-auto">
        <svg viewBox={`0 0 ${W} ${H}`} className="h-[460px] w-full min-w-[760px]" role="img" aria-label="Virtual network topology">
          <defs>
            <pattern id="grid" width="24" height="24" patternUnits="userSpaceOnUse">
              <path d="M 24 0 L 0 0 0 24" fill="none" stroke="var(--border)" strokeWidth="0.5" opacity="0.6" />
            </pattern>
          </defs>
          <rect width={W} height={H} fill="url(#grid)" />
          {layout.edges.map((e) => {
            const a = layout.pos.get(e.a)!;
            const b = layout.pos.get(e.b)!;
            return (
              <g key={e.a + e.b}>
                <line x1={a.x} y1={a.y} x2={b.x} y2={b.y} stroke={color(e.state)} strokeWidth={e.state === "Connected" ? 3 : 2} strokeDasharray={e.state === "Initiated" ? "6 5" : undefined} opacity={0.85} />
                <text x={(a.x + b.x) / 2} y={(a.y + b.y) / 2 - 6} textAnchor="middle" fontSize="10" fill="var(--muted)">
                  {e.state}
                </text>
              </g>
            );
          })}
          {vnets.map((v) => {
            const p = layout.pos.get(lower(v.id));
            if (!p) return null;
            const isHub = layout.hub === v;
            const w = isHub ? 220 : 196;
            const h = 70;
            return (
              <g key={v.id} transform={`translate(${p.x - w / 2},${p.y - h / 2})`} className="cursor-pointer" onClick={() => onOpen(v)}>
                <rect width={w} height={h} rx="12" fill="var(--surface)" stroke={isHub ? "var(--accent)" : "var(--border-strong)"} strokeWidth={isHub ? 2 : 1} />
                <rect width="5" height={h - 20} x="0" y="10" rx="2" fill={isHub ? "var(--accent)" : "var(--info)"} />
                <text x="16" y="22" fontSize="12.5" fontWeight="600" fill="var(--fg)">
                  {v.name.length > 26 ? v.name.slice(0, 25) + "…" : v.name}
                </text>
                <text x="16" y="40" fontSize="11" fontFamily="var(--font-mono)" fill="var(--muted)">
                  {v.properties.addressSpace.addressPrefixes.join(", ")}
                </text>
                <text x="16" y="57" fontSize="10.5" fill="var(--subtle)">
                  {isHub ? "HUB · " : ""}
                  {v.properties.subnets?.length ?? 0} subnets · {v.subscription.displayName}
                </text>
              </g>
            );
          })}
        </svg>
      </div>
    </Card>
  );
}

function Legend({ color, label, dashed }: { color: string; label: string; dashed?: boolean }) {
  return (
    <span className="flex items-center gap-1.5">
      <svg width="22" height="6">
        <line x1="0" y1="3" x2="22" y2="3" stroke={color} strokeWidth="3" strokeDasharray={dashed ? "5 4" : undefined} />
      </svg>
      {label}
    </span>
  );
}

function VNetDrawer({ vnet, onClose }: { vnet: VNet | null; onClose: () => void }) {
  return (
    <Drawer open={!!vnet} onClose={onClose} wide title={vnet?.name} subtitle={vnet && `${vnet.subscription.displayName} · ${rgOf(vnet.id)} · ${vnet.location}`} icon={<span className="grid size-9 place-items-center rounded-lg bg-info-soft text-info"><NetworkIcon className="size-4" /></span>}>
      {vnet && (
        <div className="space-y-6 p-5">
          <div className="flex flex-wrap gap-2">
            {vnet.properties.addressSpace.addressPrefixes.map((a) => (
              <Badge key={a} tone="info">
                {a}
              </Badge>
            ))}
            {(vnet.properties.dhcpOptions?.dnsServers ?? []).map((d) => (
              <Badge key={d}>DNS {d}</Badge>
            ))}
          </div>
          <section>
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-subtle">Subnets</h3>
            <div className="divide-y divide-border rounded-lg border border-border">
              {(vnet.properties.subnets ?? []).map((s) => (
                <div key={s.id} className="grid grid-cols-[1fr_auto] gap-2 px-3 py-2 text-sm">
                  <div className="min-w-0">
                    <div className="font-medium">{s.name}</div>
                    <div className="mt-1 flex flex-wrap gap-1">
                      {s.properties.networkSecurityGroup && (
                        <Badge tone="accent">
                          <Shield className="size-3" /> {s.properties.networkSecurityGroup.id.split("/").pop()}
                        </Badge>
                      )}
                      {s.properties.routeTable && <Badge tone="warn">route table {s.properties.routeTable.id.split("/").pop()}</Badge>}
                      {s.properties.ipConfigurations?.map((c) => (
                        <Badge key={c.id}>used by {c.id.split("/").slice(-2).join("/")}</Badge>
                      ))}
                    </div>
                  </div>
                  <Mono>{(s.properties.addressPrefixes ?? [s.properties.addressPrefix ?? ""]).join(", ")}</Mono>
                </div>
              ))}
              {!vnet.properties.subnets?.length && <div className="px-3 py-4 text-center text-sm text-subtle">No subnets</div>}
            </div>
          </section>
          <section>
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-subtle">Peerings</h3>
            <div className="divide-y divide-border rounded-lg border border-border">
              {(vnet.properties.virtualNetworkPeerings ?? []).map((p) => (
                <div key={p.id} className="flex items-center gap-3 px-3 py-2 text-sm">
                  <Badge tone={statusTone(p.properties.peeringState === "Connected" ? "ok" : p.properties.peeringState === "Initiated" ? "warning" : "failed")} dot>
                    {p.properties.peeringState}
                  </Badge>
                  <span className="font-medium">{p.name}</span>
                  <span className="min-w-0 flex-1 truncate text-xs text-muted">→ {p.properties.remoteVirtualNetwork.id.split("/").pop()}</span>
                  <span className={cx("text-xs", p.properties.allowForwardedTraffic ? "text-ok" : "text-subtle")}>{p.properties.allowForwardedTraffic ? "forwarding" : "no forwarding"}</span>
                </div>
              ))}
              {!vnet.properties.virtualNetworkPeerings?.length && <div className="px-3 py-4 text-center text-sm text-subtle">Not peered</div>}
            </div>
          </section>
          <JsonView value={vnet} collapsedDepth={2} />
        </div>
      )}
    </Drawer>
  );
}
