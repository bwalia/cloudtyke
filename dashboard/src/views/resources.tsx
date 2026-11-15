"use client";

import { ChevronRight, Code2, FileJson, FolderClosed, FolderTree, KeyRound, MapPin, Plus, RefreshCw, Save, Tag, Trash2, X } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";
import { useSWRConfig } from "swr";

import { JsonView } from "@/components/json";
import { ScopeCell, usePrincipalNames } from "@/components/principals";
import { ConfirmDialog, Dialog, useToast } from "@/components/overlay";
import { DataTable } from "@/components/table";
import { Badge, Button, Card, CopyButton, EmptyState, ErrorState, Field, Input, Mono, PageHeader, Select, Skeleton, Tabs, cx, statusTone } from "@/components/ui";
import { API, armPath, call, useApi, useList, waitAsync } from "@/lib/api";
import { BUILTIN_ROLES, ago, roleId, type ActivityEvent, type Location, type Resource, type ResourceGroup, type RoleAssignment, type Subscription } from "@/lib/azure";
import { useSubscriptions } from "@/lib/hooks";

export function Resources() {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const subs = useSubscriptions();
  const subId = params.get("sub") ?? subs.items.find((s) => s.displayName === "sub-sandbox")?.subscriptionId ?? subs.items[0]?.subscriptionId;
  const rgName = params.get("rg");
  const [creating, setCreating] = useState(params.get("create") === "1");

  const nav = (sub?: string, rg?: string | null) => {
    const p = new URLSearchParams();
    if (sub) p.set("sub", sub);
    if (rg) p.set("rg", rg);
    router.replace(`${pathname}?${p}`, { scroll: false });
  };

  const sub = subs.items.find((s) => s.subscriptionId === subId);

  return (
    <>
      <PageHeader
        icon={<FolderTree className="size-5" />}
        title="Resources"
        description="Subscriptions and resource groups you can see. The list is filtered by RBAC, exactly like Azure: switch to the Reader identity to compare."
        actions={
          <Button variant="primary" icon={<Plus className="size-4" />} disabled={!sub} onClick={() => setCreating(true)}>
            Resource group
          </Button>
        }
      />
      <div className="grid gap-4 lg:grid-cols-[300px_1fr]">
        <Card padded={false} className="self-start lg:sticky lg:top-20">
          <div className="border-b border-border px-3 py-2 text-[10px] font-semibold uppercase tracking-wider text-subtle">Subscriptions</div>
          {subs.error ? (
            <ErrorState error={subs.error} retry={() => subs.mutate()} />
          ) : subs.isLoading ? (
            <div className="space-y-2 p-3">
              <Skeleton className="h-8" />
              <Skeleton className="h-8" />
              <Skeleton className="h-8" />
            </div>
          ) : subs.items.length === 0 ? (
            <EmptyState title="No subscriptions visible" description="This identity has no role on any subscription." />
          ) : (
            <div className="p-1.5">
              {subs.items.map((s) => (
                <SubTreeNode key={s.subscriptionId} sub={s} open={s.subscriptionId === subId} selectedRg={s.subscriptionId === subId ? rgName : null} onSelect={nav} />
              ))}
            </div>
          )}
        </Card>
        <div className="min-w-0">
          {!sub ? (
            subs.isLoading ? <Skeleton className="h-64 rounded-xl" /> : null
          ) : rgName ? (
            <ResourceGroupView key={sub.subscriptionId + rgName} sub={sub} name={rgName} onDeleted={() => nav(sub.subscriptionId)} />
          ) : (
            <SubscriptionView sub={sub} onOpen={(rg) => nav(sub.subscriptionId, rg)} onCreate={() => setCreating(true)} />
          )}
        </div>
      </div>
      {sub && (
        <CreateResourceGroup
          open={creating}
          sub={sub}
          onClose={() => setCreating(false)}
          onCreated={(name) => {
            setCreating(false);
            nav(sub.subscriptionId, name);
          }}
        />
      )}
    </>
  );
}

const rgListPath = (sub: string) => armPath(`/subscriptions/${sub}/resourcegroups`);

function SubTreeNode({ sub, open, selectedRg, onSelect }: { sub: Subscription; open: boolean; selectedRg: string | null; onSelect: (s: string, rg?: string | null) => void }) {
  const rgs = useList<ResourceGroup>("arm", open ? rgListPath(sub.subscriptionId) : null);
  return (
    <div>
      <button
        onClick={() => onSelect(sub.subscriptionId)}
        className={cx("flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm", open && !selectedRg ? "bg-accent-soft text-accent" : "hover:bg-surface-2")}
      >
        <ChevronRight className={cx("size-3.5 text-subtle transition-transform", open && "rotate-90")} />
        <KeyRound className="size-4 text-warn" />
        <span className="flex-1 truncate font-medium">{sub.displayName}</span>
        {open && rgs.data && <span className="text-[11px] tabular-nums text-subtle">{rgs.items.length}</span>}
      </button>
      {open && (
        <div className="ml-[18px] border-l border-border pl-2">
          {rgs.isLoading && <Skeleton className="my-1 h-6" />}
          {rgs.error && <div className="px-2 py-1 text-xs text-bad">{rgs.error.code}</div>}
          {rgs.data && !rgs.items.length && <div className="px-2 py-1.5 text-xs text-subtle">No resource groups</div>}
          {rgs.items.map((rg) => (
            <button
              key={rg.id}
              onClick={() => onSelect(sub.subscriptionId, rg.name)}
              className={cx(
                "flex w-full items-center gap-2 rounded-lg px-2 py-1 text-left text-[13px]",
                selectedRg?.toLowerCase() === rg.name.toLowerCase() ? "bg-accent-soft text-accent" : "text-muted hover:bg-surface-2 hover:text-fg",
              )}
            >
              <FolderClosed className="size-3.5" />
              <span className="truncate">{rg.name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function SubscriptionView({ sub, onOpen, onCreate }: { sub: Subscription; onOpen: (rg: string) => void; onCreate: () => void }) {
  const rgs = useList<ResourceGroup>("arm", rgListPath(sub.subscriptionId));
  const res = useList<Resource>("arm", armPath(`/subscriptions/${sub.subscriptionId}/resources`));
  const byLoc = useMemo(() => {
    const m = new Map<string, number>();
    for (const r of rgs.items) m.set(r.location, (m.get(r.location) ?? 0) + 1);
    return [...m.entries()].sort((a, b) => b[1] - a[1]);
  }, [rgs.items]);
  return (
    <div className="space-y-4">
      <section className="rounded-xl border border-border bg-surface p-5 shadow-card">
        <div className="flex flex-wrap items-start gap-4">
          <span className="grid size-11 place-items-center rounded-xl bg-warn-soft text-warn">
            <KeyRound className="size-5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-[11px] font-semibold uppercase tracking-wider text-subtle">Subscription</div>
            <h2 className="text-xl font-semibold">{sub.displayName}</h2>
            <Mono copy>{sub.subscriptionId}</Mono>
          </div>
          <Badge tone={statusTone(sub.state)} dot>
            {sub.state}
          </Badge>
        </div>
        <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Fact label="Resource groups" value={rgs.data ? rgs.items.length : "…"} />
          <Fact label="Resources" value={res.error ? "—" : res.data ? res.items.length : "…"} />
          <Fact label="Regions in use" value={byLoc.length} />
          <Fact label="Tenant" value={<Mono copy>{sub.tenantId}</Mono>} />
        </div>
        {byLoc.length > 0 && (
          <div className="mt-4 flex flex-wrap gap-1.5">
            {byLoc.map(([l, n]) => (
              <Badge key={l} tone="info">
                <MapPin className="size-3" /> {l} · {n}
              </Badge>
            ))}
          </div>
        )}
      </section>
      <DataTable
        rows={rgs.items}
        loading={rgs.isLoading}
        error={rgs.error}
        retry={() => rgs.mutate()}
        rowKey={(r) => r.id}
        onRowClick={(r) => onOpen(r.name)}
        searchText={(r) => `${r.name} ${r.location} ${JSON.stringify(r.tags ?? {})}`}
        searchPlaceholder="Filter resource groups by name, region or tag…"
        toolbar={
          <Button size="sm" variant="ghost" icon={<RefreshCw className="size-3.5" />} onClick={() => rgs.mutate()}>
            Refresh
          </Button>
        }
        empty={
          <EmptyState
            icon={<FolderClosed className="size-5" />}
            title="No resource groups yet"
            description="Create one here, or run terraform apply against the emulator and watch it appear."
            action={
              <Button variant="primary" icon={<Plus className="size-4" />} onClick={onCreate}>
                Create resource group
              </Button>
            }
          />
        }
        columns={[
          {
            key: "name",
            header: "Name",
            sort: (r) => r.name.toLowerCase(),
            render: (r) => (
              <span className="flex items-center gap-2 font-medium">
                <FolderClosed className="size-4 text-accent" />
                {r.name}
              </span>
            ),
          },
          { key: "loc", header: "Region", sort: (r) => r.location, render: (r) => <span className="text-muted">{r.location}</span> },
          {
            key: "tags",
            header: "Tags",
            render: (r) => <Tags tags={r.tags} />,
          },
          {
            key: "state",
            header: "State",
            sort: (r) => r.properties?.provisioningState ?? "",
            render: (r) => <Badge tone={statusTone(r.properties?.provisioningState)}>{r.properties?.provisioningState ?? "—"}</Badge>,
          },
        ]}
      />
    </div>
  );
}

function Tags({ tags }: { tags?: Record<string, string> }) {
  const e = Object.entries(tags ?? {});
  if (!e.length) return <span className="text-subtle">—</span>;
  return (
    <span className="flex flex-wrap gap-1">
      {e.slice(0, 3).map(([k, v]) => (
        <span key={k} className="inline-flex items-center rounded-md border border-border text-[11px]">
          <span className="bg-surface-2 px-1.5 py-px text-muted">{k}</span>
          <span className="px-1.5 py-px">{v}</span>
        </span>
      ))}
      {e.length > 3 && <span className="text-[11px] text-subtle">+{e.length - 3}</span>}
    </span>
  );
}

function Fact({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="min-w-0 rounded-lg bg-surface-2/60 px-3 py-2">
      <div className="text-[11px] text-muted">{label}</div>
      <div className="truncate text-base font-semibold tabular-nums">{value}</div>
    </div>
  );
}

type RgTab = "overview" | "resources" | "access" | "activity" | "json" | "iac";

function ResourceGroupView({ sub, name, onDeleted }: { sub: Subscription; name: string; onDeleted: () => void }) {
  const base = `/subscriptions/${sub.subscriptionId}/resourcegroups/${encodeURIComponent(name)}`;
  const rg = useApi<ResourceGroup>("arm", armPath(base));
  const [tab, setTab] = useState<RgTab>("overview");
  const [del, setDel] = useState(false);
  const toast = useToast();
  const { mutate } = useSWRConfig();

  if (rg.error) return <Card padded={false}><ErrorState error={rg.error} retry={() => rg.mutate()} /></Card>;
  if (!rg.data) return <Skeleton className="h-72 rounded-xl" />;
  const r = rg.data;

  return (
    <div className="space-y-4">
      <section className="rounded-xl border border-border bg-surface shadow-card">
        <div className="flex flex-wrap items-start gap-4 p-5 pb-0">
          <span className="grid size-11 place-items-center rounded-xl bg-accent-soft text-accent">
            <FolderClosed className="size-5" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-[11px] font-semibold uppercase tracking-wider text-subtle">Resource group · {sub.displayName}</div>
            <h2 className="text-xl font-semibold">{r.name}</h2>
            <Mono copy className="max-w-full">{r.id}</Mono>
          </div>
          <div className="flex items-center gap-2">
            <Badge tone={statusTone(r.properties?.provisioningState)} dot>
              {r.properties?.provisioningState}
            </Badge>
            <Button size="sm" variant="ghost" icon={<RefreshCw className="size-3.5" />} onClick={() => rg.mutate()} aria-label="Refresh" />
            <Button size="sm" variant="secondary" className="text-bad" icon={<Trash2 className="size-3.5" />} onClick={() => setDel(true)}>
              Delete
            </Button>
          </div>
        </div>
        <div className="px-5 pt-3">
          <Tabs<RgTab>
            value={tab}
            onChange={setTab}
            tabs={[
              { id: "overview", label: "Overview" },
              { id: "resources", label: "Resources" },
              { id: "access", label: "Access (IAM)" },
              { id: "activity", label: "Activity log" },
              { id: "json", label: "JSON", icon: <FileJson className="size-3.5" /> },
              { id: "iac", label: "Terraform", icon: <Code2 className="size-3.5" /> },
            ]}
          />
        </div>
        <div className="p-5">
          {tab === "overview" && <RgOverview rg={r} path={base} onSaved={() => rg.mutate()} />}
          {tab === "resources" && <RgResources path={base} />}
          {tab === "access" && <ScopeAccess scope={r.id} />}
          {tab === "activity" && <RgActivity sub={sub.subscriptionId} rg={r.name} />}
          {tab === "json" && <JsonView value={r} collapsedDepth={4} />}
          {tab === "iac" && <Terraform rg={r} />}
        </div>
      </section>
      <ConfirmDialog
        open={del}
        onClose={() => setDel(false)}
        title={`Delete resource group ${r.name}?`}
        description="Deletes the group and everything in it. The emulator runs this as an async operation, just like Azure; the dashboard polls it to completion."
        typeToConfirm={r.name}
        onConfirm={async () => {
          try {
            const res = await call("arm", armPath(base), { method: "DELETE" });
            toast.push({ tone: "info", title: `Deleting ${r.name}…`, body: "Polling the async operation" });
            await waitAsync(res.__info);
            toast.push({ tone: "ok", title: `Deleted ${r.name}` });
            await mutate((k) => Array.isArray(k) && typeof k[1] === "string" && k[1].includes("resourcegroups"));
            onDeleted();
          } catch (e) {
            toast.error(e, "Delete failed");
          }
        }}
      />
    </div>
  );
}

function RgOverview({ rg, path, onSaved }: { rg: ResourceGroup; path: string; onSaved: () => void }) {
  const [tags, setTags] = useState<[string, string][]>(Object.entries(rg.tags ?? {}));
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const dirty = JSON.stringify(Object.fromEntries(tags.filter(([k]) => k))) !== JSON.stringify(rg.tags ?? {});
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <dl className="grid min-w-0 grid-cols-[110px_minmax(0,1fr)] gap-y-3 text-sm">
        <dt className="text-muted">Region</dt>
        <dd className="flex items-center gap-1.5">
          <MapPin className="size-3.5 text-subtle" />
          {rg.location}
        </dd>
        <dt className="text-muted">Subscription</dt>
        <dd>
          <Mono copy>{rg.id.split("/")[2]}</Mono>
        </dd>
        <dt className="text-muted">Provisioning</dt>
        <dd>{rg.properties?.provisioningState}</dd>
        <dt className="text-muted">Managed by</dt>
        <dd className="text-muted">{rg.managedBy || "—"}</dd>
      </dl>
      <div>
        <div className="mb-2 flex items-center justify-between">
          <span className="flex items-center gap-1.5 text-sm font-semibold">
            <Tag className="size-4 text-subtle" /> Tags
          </span>
          <Button size="sm" variant="ghost" icon={<Plus className="size-3.5" />} onClick={() => setTags([...tags, ["", ""]])}>
            Add tag
          </Button>
        </div>
        <TagEditor tags={tags} onChange={setTags} />
        {dirty && (
          <div className="mt-3 flex justify-end gap-2 animate-in">
            <Button size="sm" variant="ghost" onClick={() => setTags(Object.entries(rg.tags ?? {}))}>
              Discard
            </Button>
            <Button
              size="sm"
              variant="primary"
              loading={busy}
              icon={<Save className="size-3.5" />}
              onClick={async () => {
                setBusy(true);
                try {
                  await call("arm", armPath(path), { method: "PATCH", body: { tags: Object.fromEntries(tags.filter(([k]) => k)) } });
                  toast.push({ tone: "ok", title: "Tags saved", body: "PATCH succeeded" });
                  onSaved();
                } catch (e) {
                  toast.error(e, "Could not save tags");
                } finally {
                  setBusy(false);
                }
              }}
            >
              Save tags
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

function TagEditor({ tags, onChange }: { tags: [string, string][]; onChange: (t: [string, string][]) => void }) {
  if (!tags.length) return <div className="rounded-lg border border-dashed border-border px-3 py-4 text-center text-xs text-subtle">No tags</div>;
  return (
    <div className="space-y-2">
      {tags.map(([k, v], i) => (
        <div key={i} className="flex gap-2">
          <Input value={k} placeholder="name" onChange={(e) => onChange(tags.map((t, j) => (j === i ? [e.target.value, t[1]] : t)))} />
          <Input value={v} placeholder="value" onChange={(e) => onChange(tags.map((t, j) => (j === i ? [t[0], e.target.value] : t)))} />
          <Button variant="ghost" aria-label="Remove tag" icon={<X className="size-4" />} onClick={() => onChange(tags.filter((_, j) => j !== i))} />
        </div>
      ))}
    </div>
  );
}

function RgResources({ path }: { path: string }) {
  const res = useList<Resource>("arm", armPath(`${path}/resources`));
  return (
    <DataTable
      rows={res.items}
      loading={res.isLoading}
      error={res.error}
      rowKey={(r) => r.id}
      searchText={(r) => `${r.name} ${r.type}`}
      empty={<EmptyState title="No resources in this group" description="This emulator phase models the control plane (groups, RBAC, identity). Resource types arrive in later phases; see the coverage matrix." />}
      columns={[
        { key: "n", header: "Name", sort: (r) => r.name, render: (r) => <span className="font-medium">{r.name}</span> },
        { key: "t", header: "Type", sort: (r) => r.type, render: (r) => <Mono>{r.type}</Mono> },
        { key: "l", header: "Region", render: (r) => r.location },
      ]}
    />
  );
}

/** Role assignments that apply at a scope (inherited ones included), with names resolved from Graph. */
export function ScopeAccess({ scope }: { scope: string }) {
  const ras = useList<RoleAssignment>("arm", armPath(`${scope}/providers/Microsoft.Authorization/roleAssignments`, API.auth));
  const resolved = usePrincipalNames(ras.items.map((r) => r.properties.principalId));
  return (
    <DataTable
      rows={ras.items}
      loading={ras.isLoading}
      error={ras.error}
      retry={() => ras.mutate()}
      rowKey={(r) => r.id}
      searchText={(r) => `${resolved[r.properties.principalId]?.name ?? ""} ${BUILTIN_ROLES[roleId(r.properties.roleDefinitionId)] ?? ""} ${r.properties.scope}`}
      columns={[
        {
          key: "p",
          header: "Principal",
          render: (r) => (
            <span className="flex flex-col">
              <span className="font-medium">{resolved[r.properties.principalId]?.name ?? <Mono>{r.properties.principalId}</Mono>}</span>
              <span className="text-[11px] text-subtle">{resolved[r.properties.principalId]?.type || r.properties.principalType}</span>
            </span>
          ),
        },
        {
          key: "r",
          header: "Role",
          render: (r) => <Badge tone="accent">{BUILTIN_ROLES[roleId(r.properties.roleDefinitionId)] ?? roleId(r.properties.roleDefinitionId)}</Badge>,
        },
        {
          key: "s",
          header: "Scope",
          render: (r) => <ScopeCell assignment={r.properties.scope} viewing={scope} />,
        },
      ]}
    />
  );
}

function RgActivity({ sub, rg }: { sub: string; rg: string }) {
  const f = encodeURIComponent(`resourceGroupName eq '${rg}'`);
  const ev = useList<ActivityEvent>("arm", armPath(`/subscriptions/${sub}/providers/Microsoft.Insights/eventtypes/management/values?$filter=${f}`, API.activity));
  return (
    <DataTable
      rows={[...ev.items].sort((a, b) => b.eventTimestamp.localeCompare(a.eventTimestamp))}
      loading={ev.isLoading}
      error={ev.error}
      rowKey={(e) => e.eventDataId}
      columns={[
        { key: "op", header: "Operation", render: (e) => <span className="font-medium">{e.operationName?.localizedValue || e.operationName?.value}</span> },
        { key: "st", header: "Status", render: (e) => <Badge tone={statusTone(e.status?.value)}>{e.status?.value}</Badge> },
        { key: "c", header: "Caller", render: (e) => <Mono>{e.caller}</Mono> },
        { key: "t", header: "When", render: (e) => <span className="text-muted">{ago(e.eventTimestamp)}</span> },
      ]}
    />
  );
}

function Terraform({ rg }: { rg: ResourceGroup }) {
  const ident = rg.name.replace(/[^A-Za-z0-9_]/g, "_").toLowerCase();
  const tags = Object.entries(rg.tags ?? {});
  const hcl = `resource "azurerm_resource_group" "${ident}" {
  name     = "${rg.name}"
  location = "${rg.location}"${
    tags.length
      ? `
  tags = {
${tags.map(([k, v]) => `    ${/^[A-Za-z_][A-Za-z0-9_-]*$/.test(k) ? k : JSON.stringify(k)} = ${JSON.stringify(v)}`).join("\n")}
  }`
      : ""
  }
}

# bring the existing group under Terraform management
import {
  to = azurerm_resource_group.${ident}
  id = "${rg.id}"
}`;
  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <span className="text-sm text-muted">Generated from the live resource. Paste into a configuration that targets the emulator.</span>
        <CopyButton value={hcl} label="Copy HCL" />
      </div>
      <pre className="overflow-x-auto rounded-lg border border-border bg-surface-2/60 p-4 font-mono text-[12.5px] leading-relaxed">{hcl}</pre>
    </div>
  );
}

const RG_NAME = /^[-\w._()]{1,90}$/;

function CreateResourceGroup({ open, sub, onClose, onCreated }: { open: boolean; sub: Subscription; onClose: () => void; onCreated: (name: string) => void }) {
  const locs = useList<Location>("arm", open ? armPath(`/subscriptions/${sub.subscriptionId}/locations`) : null, { revalidateOnFocus: false });
  const [name, setName] = useState("");
  const [loc, setLoc] = useState("uksouth");
  const [tags, setTags] = useState<[string, string][]>([["env", "lab"]]);
  const [busy, setBusy] = useState(false);
  const [showReq, setShowReq] = useState(false);
  const toast = useToast();
  const { mutate } = useSWRConfig();
  const nameError = name && (!RG_NAME.test(name) || name.endsWith(".")) ? "1–90 letters, digits, - _ . ( ); can't end with a period" : undefined;
  const body = { location: loc, tags: Object.fromEntries(tags.filter(([k]) => k)) };
  const path = `/subscriptions/${sub.subscriptionId}/resourcegroups/${name || "{name}"}`;
  const geos = useMemo(() => {
    const m = new Map<string, Location[]>();
    for (const l of locs.items) {
      const g = l.metadata?.geographyGroup || "Other";
      m.set(g, [...(m.get(g) ?? []), l]);
    }
    return [...m.entries()];
  }, [locs.items]);

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Create resource group"
      description={
        <>
          In <b>{sub.displayName}</b>. Sent as a real ARM <code className="font-mono text-xs">PUT</code>, the same call Terraform makes.
        </>
      }
      footer={
        <>
          <Button variant="ghost" className="mr-auto" icon={<Code2 className="size-4" />} onClick={() => setShowReq(!showReq)}>
            {showReq ? "Hide" : "Show"} request
          </Button>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!name || !!nameError}
            onClick={async () => {
              setBusy(true);
              try {
                await call("arm", armPath(path), { method: "PUT", body });
                toast.push({ tone: "ok", title: `Created ${name}`, body: `${loc} · ${sub.displayName}` });
                await mutate((k) => Array.isArray(k) && typeof k[1] === "string" && k[1].includes("resourcegroups"));
                onCreated(name);
                setName("");
              } catch (e) {
                toast.error(e, "Could not create the resource group");
              } finally {
                setBusy(false);
              }
            }}
          >
            Create
          </Button>
        </>
      }
    >
      <Field label="Name" error={nameError} hint="e.g. rg-app-dev-uks">
        <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="rg-my-app" />
      </Field>
      <Field label="Region" hint={locs.isLoading ? "loading regions…" : `${locs.items.length} regions from the locations API`}>
        <Select value={loc} onChange={(e) => setLoc(e.target.value)}>
          {geos.map(([g, ls]) => (
            <optgroup key={g} label={g}>
              {ls.map((l) => (
                <option key={l.name} value={l.name}>
                  {l.displayName} ({l.name})
                </option>
              ))}
            </optgroup>
          ))}
          {!geos.length && <option value={loc}>{loc}</option>}
        </Select>
      </Field>
      <div>
        <div className="mb-1.5 flex items-center justify-between">
          <span className="text-xs font-semibold">Tags</span>
          <Button size="sm" variant="ghost" icon={<Plus className="size-3.5" />} onClick={() => setTags([...tags, ["", ""]])}>
            Add
          </Button>
        </div>
        <TagEditor tags={tags} onChange={setTags} />
      </div>
      {showReq && (
        <div className="animate-in">
          <div className="mb-1 font-mono text-[11px] text-muted">
            PUT https://management.…{path}?api-version={API.resources}
          </div>
          <JsonView value={body} />
        </div>
      )}
    </Dialog>
  );
}
