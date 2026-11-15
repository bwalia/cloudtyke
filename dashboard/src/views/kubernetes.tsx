"use client";

import { ArrowUpCircle, Container, Download, Play, Power, RefreshCw, Server, Terminal } from "lucide-react";
import { useState } from "react";
import useSWR from "swr";

import { JsonView } from "@/components/json";
import { Dialog, useToast } from "@/components/overlay";
import { Badge, Button, Card, CopyButton, EmptyState, ErrorState, Field, Mono, PageHeader, Select, Skeleton, StatusDot, Tabs, cx, statusTone } from "@/components/ui";
import { API, ApiError, armPath, call, waitAsync } from "@/lib/api";
import type { Subscription } from "@/lib/azure";
import { useSubscriptions } from "@/lib/hooks";

interface Pool {
  name: string;
  mode: string;
  vmSize: string;
  count: number;
  enableAutoScaling?: boolean;
  minCount?: number;
  maxCount?: number;
  currentOrchestratorVersion: string;
  osSKU?: string;
  availabilityZones?: string[];
  vnetSubnetID?: string;
  powerState?: { code: string };
  nodeLabels?: Record<string, string>;
  nodeTaints?: string[];
}
interface Cluster {
  id: string;
  name: string;
  location: string;
  tags?: Record<string, string>;
  sku?: { tier: string };
  identity?: { type: string; userAssignedIdentities?: Record<string, unknown>; principalId?: string };
  properties: {
    currentKubernetesVersion: string;
    kubernetesVersion: string;
    fqdn: string;
    nodeResourceGroup: string;
    powerState?: { code: string };
    agentPoolProfiles: Pool[];
    networkProfile: { networkPlugin: string; networkPluginMode?: string; networkPolicy?: string; outboundType: string; podCidr?: string; serviceCidr: string; dnsServiceIP: string };
    oidcIssuerProfile?: { enabled: boolean; issuerURL?: string };
    securityProfile?: { workloadIdentity?: { enabled: boolean } };
    aadProfile?: { managed: boolean; enableAzureRBAC: boolean; adminGroupObjectIDs?: string[] };
    addonProfiles?: Record<string, { enabled: boolean; config?: Record<string, string> }>;
    identityProfile?: { kubeletidentity?: { clientId: string; objectId: string; resourceId: string } };
    disableLocalAccounts?: boolean;
  };
  subscription: Subscription;
}

const rgOf = (id: string) => id.split("/")[4] ?? "";

function useClusters(subs: Subscription[]) {
  const key = subs.map((s) => s.subscriptionId).join(",");
  return useSWR(key ? ["aks", key] : null, async () => {
    const lists = await Promise.all(
      subs.map((s) =>
        call<{ value: Omit<Cluster, "subscription">[] }>("arm", armPath(`/subscriptions/${s.subscriptionId}/providers/Microsoft.ContainerService/managedClusters`, API.aks))
          .then((r) => r.value.map((c) => ({ ...c, subscription: s }) as Cluster))
          .catch(() => [] as Cluster[]),
      ),
    );
    return lists.flat();
  });
}

export function Kubernetes() {
  const subs = useSubscriptions();
  const clusters = useClusters(subs.items);
  const [sel, setSel] = useState<string | null>(null);
  const list = clusters.data ?? [];
  const current = list.find((c) => c.id === sel) ?? list[0];

  return (
    <>
      <PageHeader
        icon={<Container className="size-5" />}
        title="Kubernetes"
        description="AKS clusters across your subscriptions: node pools, networking, identities and add-ons, plus a live view of the cluster's Kubernetes API through its own credentials."
        actions={
          <Button icon={<RefreshCw className="size-4" />} loading={clusters.isValidating} onClick={() => clusters.mutate()}>
            Refresh
          </Button>
        }
      />
      {clusters.error ? (
        <Card padded={false}>
          <ErrorState error={clusters.error} />
        </Card>
      ) : !clusters.data ? (
        <Skeleton className="h-80 rounded-xl" />
      ) : !list.length ? (
        <Card>
          <EmptyState
            icon={<Container className="size-5" />}
            title="No AKS clusters yet"
            description={
              <>
                Apply <code className="font-mono text-xs">examples/terraform/aks-platform</code> or <code className="font-mono text-xs">landing-zone</code>, then come back.
              </>
            }
          />
        </Card>
      ) : (
        <div className="grid gap-4 lg:grid-cols-[280px_1fr]">
          <Card padded={false} className="self-start">
            {list.map((c) => (
              <button
                key={c.id}
                onClick={() => setSel(c.id)}
                className={cx("flex w-full items-center gap-3 border-l-2 px-3 py-2.5 text-left", current?.id === c.id ? "border-accent bg-accent-soft/50" : "border-transparent hover:bg-surface-2/60")}
              >
                <StatusDot tone={c.properties.powerState?.code === "Stopped" ? "neutral" : "ok"} pulse={c.properties.powerState?.code !== "Stopped"} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{c.name}</span>
                  <span className="block truncate text-[11px] text-subtle">
                    v{c.properties.currentKubernetesVersion} · {c.subscription.displayName}
                  </span>
                </span>
              </button>
            ))}
          </Card>
          {current && <ClusterView key={current.id} c={current} reload={() => clusters.mutate()} />}
        </div>
      )}
    </>
  );
}

type Tab = "overview" | "nodes" | "connect" | "json";

function ClusterView({ c, reload }: { c: Cluster; reload: () => void }) {
  const [tab, setTab] = useState<Tab>("overview");
  const [busy, setBusy] = useState<string | null>(null);
  const [upgrade, setUpgrade] = useState(false);
  const toast = useToast();
  const p = c.properties;
  const stopped = p.powerState?.code === "Stopped";
  const nodes = p.agentPoolProfiles.reduce((n, x) => n + (x.count ?? 0), 0);

  const act = async (name: string, path: string, done: string) => {
    setBusy(name);
    try {
      const r = await call("arm", armPath(`${c.id}/${path}`, API.aks), { method: "POST" });
      await waitAsync(r.__info);
      toast.push({ tone: "ok", title: done });
      reload();
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(null);
    }
  };

  const download = async () => {
    setBusy("kubeconfig");
    try {
      const r = await call<{ kubeconfigs: { value: string }[] }>("arm", armPath(`${c.id}/listClusterAdminCredential`, API.aks), { method: "POST" });
      const blob = new Blob([atob(r.kubeconfigs[0].value)], { type: "text/yaml" });
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = `${c.name}-admin.kubeconfig`;
      a.click();
      URL.revokeObjectURL(a.href);
    } catch (e) {
      toast.error(e, "Could not get credentials");
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="min-w-0 space-y-4">
      <section className="rounded-xl border border-border bg-surface shadow-card">
        <div className="flex flex-wrap items-start gap-4 p-5 pb-0">
          <span className="grid size-11 place-items-center rounded-xl bg-accent-soft text-accent">
            <Container className="size-5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-[11px] font-semibold uppercase tracking-wider text-subtle">
              AKS · {c.subscription.displayName} · {rgOf(c.id)}
            </div>
            <h2 className="text-xl font-semibold">{c.name}</h2>
            <Mono copy>{p.fqdn}</Mono>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone={stopped ? "neutral" : "ok"} dot>
              {stopped ? "Stopped" : "Running"}
            </Badge>
            <Button size="sm" icon={<ArrowUpCircle className="size-3.5" />} onClick={() => setUpgrade(true)}>
              Upgrade
            </Button>
            {stopped ? (
              <Button size="sm" icon={<Play className="size-3.5" />} loading={busy === "start"} onClick={() => act("start", "start", `${c.name} started`)}>
                Start
              </Button>
            ) : (
              <Button size="sm" icon={<Power className="size-3.5" />} loading={busy === "stop"} onClick={() => act("stop", "stop", `${c.name} stopped`)}>
                Stop
              </Button>
            )}
            <Button size="sm" variant="primary" icon={<Download className="size-3.5" />} loading={busy === "kubeconfig"} disabled={p.disableLocalAccounts} onClick={download} title={p.disableLocalAccounts ? "Local accounts are disabled on this cluster" : undefined}>
              Admin kubeconfig
            </Button>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3 p-5 sm:grid-cols-4">
          <Fact label="Kubernetes" value={`v${p.currentKubernetesVersion}`} />
          <Fact label="Nodes" value={`${nodes} in ${p.agentPoolProfiles.length} pool${p.agentPoolProfiles.length === 1 ? "" : "s"}`} />
          <Fact label="Tier" value={c.sku?.tier ?? "Free"} />
          <Fact label="Network" value={`${p.networkProfile.networkPlugin}${p.networkProfile.networkPluginMode ? " " + p.networkProfile.networkPluginMode : ""}`} />
        </div>
        <div className="px-5">
          <Tabs<Tab>
            value={tab}
            onChange={setTab}
            tabs={[
              { id: "overview", label: "Overview" },
              { id: "nodes", label: "Live nodes", icon: <Server className="size-3.5" /> },
              { id: "connect", label: "Connect", icon: <Terminal className="size-3.5" /> },
              { id: "json", label: "JSON" },
            ]}
          />
        </div>
        <div className="p-5">
          {tab === "overview" && <Overview c={c} />}
          {tab === "nodes" && <LiveNodes c={c} />}
          {tab === "connect" && <Connect c={c} />}
          {tab === "json" && <JsonView value={c} collapsedDepth={2} />}
        </div>
      </section>
      {upgrade && <UpgradeDialog c={c} onClose={() => setUpgrade(false)} onDone={reload} />}
    </div>
  );
}

function Fact({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="rounded-lg bg-surface-2/60 px-3 py-2">
      <div className="text-[11px] text-muted">{label}</div>
      <div className="truncate text-sm font-semibold">{value}</div>
    </div>
  );
}

function Overview({ c }: { c: Cluster }) {
  const p = c.properties;
  const addons = Object.entries(p.addonProfiles ?? {}).filter(([, a]) => a.enabled);
  return (
    <div className="space-y-5">
      <div>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-subtle">Node pools</h3>
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs text-muted">
                {["Pool", "Mode", "VM size", "Nodes", "Version", "OS", "Zones"].map((h) => (
                  <th key={h} className="px-3 py-2 font-medium">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {p.agentPoolProfiles.map((x) => (
                <tr key={x.name} className="border-b border-border last:border-0">
                  <td className="px-3 py-2 font-medium">{x.name}</td>
                  <td className="px-3 py-2">
                    <Badge tone={x.mode === "System" ? "accent" : "neutral"}>{x.mode}</Badge>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs">{x.vmSize}</td>
                  <td className="px-3 py-2 tabular-nums">
                    {x.count}
                    {x.enableAutoScaling && (
                      <span className="ml-1 text-xs text-muted">
                        (auto {x.minCount}–{x.maxCount})
                      </span>
                    )}
                  </td>
                  <td className="px-3 py-2 tabular-nums">{x.currentOrchestratorVersion}</td>
                  <td className="px-3 py-2 text-muted">{x.osSKU}</td>
                  <td className="px-3 py-2 text-muted">{[...(x.availabilityZones ?? [])].sort().join(", ") || "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <dl className="grid grid-cols-[130px_minmax(0,1fr)] gap-y-2 text-sm">
          <dt className="text-muted">Outbound</dt>
          <dd>{p.networkProfile.outboundType}</dd>
          <dt className="text-muted">Network policy</dt>
          <dd>{p.networkProfile.networkPolicy || "none"}</dd>
          <dt className="text-muted">Pod CIDR</dt>
          <dd className="font-mono text-xs">{p.networkProfile.podCidr || "from subnet"}</dd>
          <dt className="text-muted">Service CIDR</dt>
          <dd className="font-mono text-xs">
            {p.networkProfile.serviceCidr} (DNS {p.networkProfile.dnsServiceIP})
          </dd>
          <dt className="text-muted">Node RG</dt>
          <dd className="truncate text-xs">{p.nodeResourceGroup}</dd>
        </dl>
        <dl className="grid grid-cols-[130px_minmax(0,1fr)] gap-y-2 text-sm">
          <dt className="text-muted">Identity</dt>
          <dd>{c.identity?.type ?? "service principal"}</dd>
          <dt className="text-muted">Kubelet identity</dt>
          <dd className="truncate font-mono text-xs">{p.identityProfile?.kubeletidentity?.resourceId.split("/").pop() ?? "—"}</dd>
          <dt className="text-muted">Entra ID</dt>
          <dd>{p.aadProfile ? `managed${p.aadProfile.enableAzureRBAC ? " + Azure RBAC" : ""}` : "local accounts"}</dd>
          <dt className="text-muted">Workload identity</dt>
          <dd>{p.securityProfile?.workloadIdentity?.enabled ? "enabled" : "off"}</dd>
          <dt className="text-muted">OIDC issuer</dt>
          <dd className="truncate font-mono text-xs">{p.oidcIssuerProfile?.issuerURL ?? "off"}</dd>
        </dl>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {addons.map(([k]) => (
          <Badge key={k} tone="ok">
            {k}
          </Badge>
        ))}
        {!addons.length && <span className="text-xs text-subtle">No add-ons enabled</span>}
      </div>
    </div>
  );
}

interface NodeList {
  items: { metadata: { name: string; labels: Record<string, string> }; status: { nodeInfo?: { kubeletVersion: string; osImage: string }; conditions?: { type: string; status: string }[] } }[];
}

function LiveNodes({ c }: { c: Cluster }) {
  const r = useSWR<NodeList>(["kube-nodes", c.id], () => call<NodeList>("raw", `/api/kube?cluster=${encodeURIComponent(c.id)}&path=/api/v1/nodes`), { refreshInterval: 10_000 });
  if (r.error) return <ErrorState error={r.error as ApiError} retry={() => r.mutate()} />;
  if (!r.data) return <Skeleton className="h-40" />;
  return (
    <div>
      <p className="mb-3 text-xs text-muted">
        <code className="font-mono">kubectl get nodes</code>, fetched with the cluster&apos;s own admin credential.
      </p>
      <div className="overflow-x-auto rounded-lg border border-border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-xs text-muted">
              {["Name", "Status", "Pool", "Instance type", "Zone", "Version"].map((h) => (
                <th key={h} className="px-3 py-2 font-medium">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {r.data.items.map((n) => {
              const ready = n.status.conditions?.find((x) => x.type === "Ready")?.status === "True";
              const l = n.metadata.labels;
              return (
                <tr key={n.metadata.name} className="border-b border-border last:border-0">
                  <td className="px-3 py-2 font-mono text-xs">{n.metadata.name}</td>
                  <td className="px-3 py-2">
                    <Badge tone={statusTone(ready ? "ok" : "failed")} dot>
                      {ready ? "Ready" : "NotReady"}
                    </Badge>
                  </td>
                  <td className="px-3 py-2">{l["agentpool"] ?? l["kubernetes.azure.com/agentpool"]}</td>
                  <td className="px-3 py-2 font-mono text-xs">{l["node.kubernetes.io/instance-type"] ?? "—"}</td>
                  <td className="px-3 py-2 text-muted">{l["topology.kubernetes.io/zone"] ?? "—"}</td>
                  <td className="px-3 py-2 tabular-nums">{n.status.nodeInfo?.kubeletVersion}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function Connect({ c }: { c: Cluster }) {
  const rg = rgOf(c.id);
  const inToolbox = `# in the directory you applied from (resource group ${rg})
tofu output -raw kube_admin_config > ~/.kube/config
kubectl get nodes`;
  const onHost = `kubectl --kubeconfig ${c.name}-admin.kubeconfig \\
  --server https://localhost --tls-server-name ${c.properties.fqdn} get nodes`;
  return (
    <div className="space-y-4 text-sm">
      <div>
        <div className="mb-1 flex items-center justify-between">
          <span className="font-semibold">Inside the toolbox (make shell)</span>
          <CopyButton value={inToolbox} label="Copy" />
        </div>
        <pre className="overflow-x-auto rounded-lg border border-border bg-surface-2/60 p-3 font-mono text-xs">{inToolbox}</pre>
      </div>
      <div>
        <div className="mb-1 flex items-center justify-between">
          <span className="font-semibold">From your machine, with the downloaded kubeconfig</span>
          <CopyButton value={onHost} label="Copy" />
        </div>
        <pre className="overflow-x-auto rounded-lg border border-border bg-surface-2/60 p-3 font-mono text-xs">{onHost}</pre>
      </div>
      <p className="text-xs text-muted">
        The emulator answers with a built-in Kubernetes API by default. Start it with <code className="font-mono">make up-k3s</code> to give every cluster a real k3s control plane.
      </p>
    </div>
  );
}

function UpgradeDialog({ c, onClose, onDone }: { c: Cluster; onClose: () => void; onDone: () => void }) {
  const prof = useSWR(["upgrades", c.id], () => call<{ properties: { controlPlaneProfile: { upgrades: { kubernetesVersion: string }[] } } }>("arm", armPath(`${c.id}/upgradeProfiles/default`, API.aks)));
  const options = prof.data?.properties.controlPlaneProfile.upgrades ?? [];
  const [v, setV] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const target = v || options[0]?.kubernetesVersion || "";
  return (
    <Dialog
      open
      onClose={onClose}
      title={`Upgrade ${c.name}`}
      description={`Currently v${c.properties.currentKubernetesVersion}. AKS allows one minor version at a time.`}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!target}
            onClick={async () => {
              setBusy(true);
              try {
                const body = await call<Record<string, unknown> & { properties: Record<string, unknown> }>("arm", armPath(c.id, API.aks));
                body.properties.kubernetesVersion = target;
                const r = await call("arm", armPath(c.id, API.aks), { method: "PUT", body });
                await waitAsync(r.__info);
                toast.push({ tone: "ok", title: `${c.name} upgraded to v${target}` });
                onDone();
                onClose();
              } catch (e) {
                toast.error(e, "Upgrade failed");
              } finally {
                setBusy(false);
              }
            }}
          >
            Upgrade control plane
          </Button>
        </>
      }
    >
      {prof.error ? (
        <ErrorState error={prof.error as ApiError} />
      ) : !prof.data ? (
        <Skeleton className="h-9" />
      ) : options.length ? (
        <Field label="Target version">
          <Select value={target} onChange={(e) => setV(e.target.value)}>
            {options.map((o) => (
              <option key={o.kubernetesVersion} value={o.kubernetesVersion}>
                {o.kubernetesVersion}
              </option>
            ))}
          </Select>
        </Field>
      ) : (
        <EmptyState title="Already on the newest version" />
      )}
    </Dialog>
  );
}
