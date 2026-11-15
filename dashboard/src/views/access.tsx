"use client";

import { Check, Minus, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useSWRConfig } from "swr";

import { JsonView } from "@/components/json";
import { ConfirmDialog, Dialog, Drawer, useToast } from "@/components/overlay";
import { useSession } from "@/components/session";
import { DataTable } from "@/components/table";
import { Avatar, Badge, Button, Card, EmptyState, ErrorState, Field, Input, Mono, PageHeader, Select, Skeleton, Tabs, cx } from "@/components/ui";
import { API, armPath, call, useList } from "@/lib/api";
import { BUILTIN_ROLES, roleId, scopeRelation, type GraphObject, type RoleAssignment, type RoleDefinition, type ServicePrincipal, type User } from "@/lib/azure";
import { useAllResourceGroups, useSubscriptions } from "@/lib/hooks";
import { ScopeCell, usePrincipalNames } from "@/components/principals";

type Tab = "assignments" | "roles" | "check";

export function Access() {
  const subs = useSubscriptions();
  const rgs = useAllResourceGroups(subs.items);
  const [scope, setScope] = useState("/");
  const [tab, setTab] = useState<Tab>("assignments");
  const [adding, setAdding] = useState(false);
  const firstSub = subs.items[0]?.subscriptionId;
  const roleScope = scope === "/" ? (firstSub ? `/subscriptions/${firstSub}` : null) : scope;
  const defs = useList<RoleDefinition>("arm", roleScope ? armPath(`${roleScope}/providers/Microsoft.Authorization/roleDefinitions`, API.auth) : null);

  const scopes = useMemo(() => {
    const out: { value: string; label: string; depth: number }[] = [{ value: "/", label: "Tenant root ( / )", depth: 0 }];
    for (const s of subs.items) {
      out.push({ value: `/subscriptions/${s.subscriptionId}`, label: s.displayName, depth: 1 });
      for (const rg of rgs.data ?? []) if (rg.subscription.subscriptionId === s.subscriptionId) out.push({ value: rg.id, label: rg.name, depth: 2 });
    }
    return out;
  }, [subs.items, rgs.data]);

  return (
    <>
      <PageHeader
        icon={<ShieldCheck className="size-5" />}
        title="Access control (IAM)"
        description="Azure RBAC as the emulator enforces it on every ARM call: assignments inherit down the hierarchy, and the emulator checks them for real."
        actions={
          <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setAdding(true)} disabled={!defs.items.length}>
            Add role assignment
          </Button>
        }
      />
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <Field label="Scope">
          <Select value={scope} onChange={(e) => setScope(e.target.value)} className="min-w-[320px]">
            {scopes.map((s) => (
              <option key={s.value} value={s.value}>
                {"   ".repeat(s.depth)}
                {s.depth === 1 ? "▸ " : s.depth === 2 ? "· " : ""}
                {s.label}
              </option>
            ))}
          </Select>
        </Field>
        <Mono copy className="mb-2">{scope}</Mono>
      </div>
      <div className="mb-4">
        <Tabs<Tab>
          value={tab}
          onChange={setTab}
          tabs={[
            { id: "assignments", label: "Role assignments" },
            { id: "roles", label: "Roles", count: defs.data ? defs.items.length : undefined },
            { id: "check", label: "Check my access" },
          ]}
        />
      </div>
      {tab === "assignments" && <Assignments scope={scope} defs={defs.items} />}
      {tab === "roles" && <Roles defs={defs.items} loading={defs.isLoading} error={defs.error} />}
      {tab === "check" && <CheckAccess scope={scope} />}
      {adding && <AddAssignment scope={scope} defs={defs.items} onClose={() => setAdding(false)} />}
    </>
  );
}

const roleLabel = (defs: RoleDefinition[], id: string) => BUILTIN_ROLES[roleId(id)] ?? defs.find((d) => d.name === roleId(id))?.properties.roleName ?? roleId(id);

function Assignments({ scope, defs }: { scope: string; defs: RoleDefinition[] }) {
  const path = armPath(`${scope === "/" ? "" : scope}/providers/Microsoft.Authorization/roleAssignments`, API.auth);
  const ras = useList<RoleAssignment>("arm", path);
  const names = usePrincipalNames(ras.items.map((r) => r.properties.principalId));
  const [del, setDel] = useState<RoleAssignment | null>(null);
  const toast = useToast();
  const { data: session } = useSession();
  const own = del && del.properties.principalId === session?.claims?.oid;
  const { mutate } = useSWRConfig();
  return (
    <>
      <DataTable
        rows={ras.items}
        loading={ras.isLoading}
        error={ras.error}
        retry={() => ras.mutate()}
        rowKey={(r) => r.id}
        searchText={(r) => `${names[r.properties.principalId]?.name ?? r.properties.principalId} ${roleLabel(defs, r.properties.roleDefinitionId)} ${r.properties.scope}`}
        searchPlaceholder="Search by principal, role or scope…"
        empty={<EmptyState icon={<ShieldCheck className="size-5" />} title="No role assignments at this scope" />}
        columns={[
          {
            key: "p",
            header: "Principal",
            sort: (r) => names[r.properties.principalId]?.name ?? "",
            render: (r) => {
              const n = names[r.properties.principalId];
              return (
                <span className="flex items-center gap-3">
                  <Avatar name={n?.name ?? "?"} kind={n?.type === "group" ? "group" : n?.type === "servicePrincipal" ? "sp" : "user"} />
                  <span className="min-w-0">
                    <span className="block font-medium">{n?.name ?? <Mono>{r.properties.principalId}</Mono>}</span>
                    <span className="block text-xs text-muted">{n?.type || r.properties.principalType}</span>
                  </span>
                </span>
              );
            },
          },
          {
            key: "r",
            header: "Role",
            sort: (r) => roleLabel(defs, r.properties.roleDefinitionId),
            render: (r) => <Badge tone="accent">{roleLabel(defs, r.properties.roleDefinitionId)}</Badge>,
          },
          {
            key: "s",
            header: "Scope",
            render: (r) => <ScopeCell assignment={r.properties.scope} viewing={scope} />,
          },
          {
            key: "x",
            header: "",
            className: "text-right",
            render: (r) =>
              scopeRelation(r.properties.scope, scope) !== "inherited" && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-bad"
                  aria-label="Remove assignment"
                  icon={<Trash2 className="size-3.5" />}
                  onClick={(e) => {
                    e.stopPropagation();
                    setDel(r);
                  }}
                />
              ),
          },
        ]}
      />
      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        title="Remove role assignment?"
        description={
          del && (
            <>
              {names[del.properties.principalId]?.name ?? del.properties.principalId} loses {roleLabel(defs, del.properties.roleDefinitionId)} at {del.properties.scope}.
              {own && <b className="mt-2 block text-bad">This is the identity you are using now: you may lock yourself out. Emulator → Reset to seed restores it.</b>}
            </>
          )
        }
        typeToConfirm={own ? "remove my access" : undefined}
        confirmLabel="Remove"
        onConfirm={async () => {
          try {
            await call("arm", armPath(del!.id, API.auth), { method: "DELETE" });
            toast.push({ tone: "ok", title: "Role assignment removed" });
            await mutate((k) => Array.isArray(k) && String(k[1]).includes("roleAssignments"));
          } catch (e) {
            toast.error(e, "Remove failed");
          }
        }}
      />
    </>
  );
}

function AddAssignment({ scope, defs, onClose }: { scope: string; defs: RoleDefinition[]; onClose: () => void }) {
  const users = useList<User>("graph", "/v1.0/users?$top=999");
  const groups = useList<GraphObject>("graph", "/v1.0/groups?$top=999");
  const sps = useList<ServicePrincipal>("graph", "/v1.0/servicePrincipals?$top=999");
  const sorted = [...defs].sort((a, b) => (a.properties.type === b.properties.type ? a.properties.roleName.localeCompare(b.properties.roleName) : a.properties.type === "BuiltInRole" ? -1 : 1));
  const [role, setRole] = useState(sorted.find((d) => d.properties.roleName === "Reader")?.id ?? sorted[0]?.id ?? "");
  const [principal, setPrincipal] = useState<{ id: string; type: string; name: string } | null>(null);
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const { mutate } = useSWRConfig();
  const candidates = [
    ...users.items.map((u) => ({ id: u.id, type: "User", name: u.displayName ?? u.userPrincipalName, sub: u.userPrincipalName, kind: "user" as const })),
    ...groups.items.map((g) => ({ id: g.id, type: "Group", name: g.displayName ?? g.id, sub: "group", kind: "group" as const })),
    ...sps.items.map((s) => ({ id: s.id, type: "ServicePrincipal", name: s.displayName ?? s.appId, sub: `app ${s.appId}`, kind: "sp" as const })),
  ].filter((c) => `${c.name} ${c.sub}`.toLowerCase().includes(q.toLowerCase()));
  const def = defs.find((d) => d.id === role);
  return (
    <Dialog
      open
      onClose={onClose}
      title="Add role assignment"
      description={
        <>
          At <span className="font-mono text-xs">{scope}</span>
        </>
      }
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!principal || !role}
            onClick={async () => {
              setBusy(true);
              try {
                const roleDefinitionId = scope === "/" ? `/providers/Microsoft.Authorization/roleDefinitions/${roleId(role)}` : `${scope}/providers/Microsoft.Authorization/roleDefinitions/${roleId(role)}`;
                await call("arm", armPath(`${scope === "/" ? "" : scope}/providers/Microsoft.Authorization/roleAssignments/${crypto.randomUUID()}`, API.auth), {
                  method: "PUT",
                  body: { properties: { roleDefinitionId, principalId: principal!.id, principalType: principal!.type } },
                });
                toast.push({ tone: "ok", title: `${principal!.name} is now ${def?.properties.roleName}` });
                await mutate((k) => Array.isArray(k) && String(k[1]).includes("roleAssignments"));
                onClose();
              } catch (e) {
                toast.error(e, "Assignment failed");
              } finally {
                setBusy(false);
              }
            }}
          >
            Assign
          </Button>
        </>
      }
    >
      <Field label="Role" hint={def?.properties.description}>
        <Select value={role} onChange={(e) => setRole(e.target.value)}>
          {sorted.map((d) => (
            <option key={d.id} value={d.id}>
              {d.properties.roleName}
              {d.properties.type === "CustomRole" ? " (custom)" : ""}
            </option>
          ))}
        </Select>
      </Field>
      <Field label="Assign to">
        <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search users, groups, service principals…" />
      </Field>
      <div className="max-h-56 divide-y divide-border overflow-y-auto rounded-lg border border-border">
        {candidates.map((c) => (
          <button
            key={c.id}
            type="button"
            onClick={() => setPrincipal(c)}
            className={cx("flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-surface-2", principal?.id === c.id && "bg-accent-soft")}
          >
            <Avatar name={c.name} kind={c.kind} />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium">{c.name}</span>
              <span className="block truncate text-xs text-muted">{c.sub}</span>
            </span>
            {principal?.id === c.id && <Check className="size-4 text-accent" />}
          </button>
        ))}
      </div>
    </Dialog>
  );
}

function Roles({ defs, loading, error }: { defs: RoleDefinition[]; loading: boolean; error?: Error }) {
  const [open, setOpen] = useState<RoleDefinition | null>(null);
  return (
    <>
      <DataTable
        rows={defs}
        loading={loading}
        error={error}
        rowKey={(d) => d.id}
        onRowClick={setOpen}
        searchText={(d) => `${d.properties.roleName} ${d.properties.description}`}
        searchPlaceholder="Search roles…"
        columns={[
          { key: "n", header: "Role", sort: (d) => d.properties.roleName, render: (d) => <span className="font-medium">{d.properties.roleName}</span> },
          { key: "d", header: "Description", render: (d) => <span className="line-clamp-1 text-muted">{d.properties.description}</span> },
          { key: "t", header: "Type", sort: (d) => d.properties.type, render: (d) => <Badge tone={d.properties.type === "CustomRole" ? "warn" : "neutral"}>{d.properties.type === "CustomRole" ? "Custom" : "Built-in"}</Badge> },
        ]}
      />
      <Drawer open={!!open} onClose={() => setOpen(null)} title={open?.properties.roleName} subtitle={open?.name}>
        {open && (
          <div className="space-y-5 p-5">
            <p className="text-sm text-muted">{open.properties.description}</p>
            {(open.properties.permissions ?? []).map((p, i) => (
              <div key={i} className="space-y-3">
                <PermList title="Allowed actions" items={p.actions} tone="ok" />
                <PermList title="Excluded (notActions)" items={p.notActions} tone="bad" />
                <PermList title="Data actions" items={p.dataActions} tone="ok" />
              </div>
            ))}
            <div>
              <div className="mb-1 text-xs font-semibold uppercase tracking-wider text-subtle">Assignable scopes</div>
              {(open.properties.assignableScopes ?? []).map((s) => (
                <Mono key={s}>{s}</Mono>
              ))}
            </div>
            <JsonView value={open} collapsedDepth={2} />
          </div>
        )}
      </Drawer>
    </>
  );
}

function PermList({ title, items, tone }: { title: string; items?: string[]; tone: "ok" | "bad" }) {
  if (!items?.length) return null;
  return (
    <div>
      <div className="mb-1 text-xs font-semibold uppercase tracking-wider text-subtle">{title}</div>
      <div className="flex flex-wrap gap-1">
        {items.map((a) => (
          <span key={a} className={cx("inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 font-mono text-[11px]", tone === "ok" ? "bg-ok-soft text-ok" : "bg-bad-soft text-bad")}>
            {tone === "ok" ? <Check className="size-3" /> : <Minus className="size-3" />}
            {a}
          </span>
        ))}
      </div>
    </div>
  );
}

const PROBES = [
  "Microsoft.Resources/subscriptions/resourceGroups/read",
  "Microsoft.Resources/subscriptions/resourceGroups/write",
  "Microsoft.Resources/subscriptions/resourceGroups/delete",
  "Microsoft.Authorization/roleAssignments/write",
  "Microsoft.Authorization/roleDefinitions/write",
  "Microsoft.Compute/virtualMachines/write",
];

function matches(pattern: string, action: string) {
  const re = new RegExp("^" + pattern.replace(/[.+?^${}()|[\]\\]/g, "\\$&").replace(/\*/g, ".*") + "$", "i");
  return re.test(action);
}

/** Effective permissions of the current identity (Microsoft.Authorization/permissions) at a scope. */
function CheckAccess({ scope }: { scope: string }) {
  const { data: session } = useSession();
  const target = scope === "/" ? null : scope;
  const perms = useList<{ actions: string[]; notActions: string[] }>("arm", target ? armPath(`${target}/providers/Microsoft.Authorization/permissions`, API.auth) : null);
  if (!target) return <Card><EmptyState title="Pick a subscription or resource group" description="Effective permissions are evaluated at a subscription or below." /></Card>;
  if (perms.error) return <Card padded={false}><ErrorState error={perms.error} /></Card>;
  if (perms.isLoading) return <Skeleton className="h-48 rounded-xl" />;
  const allowed = (a: string) => perms.items.some((p) => p.actions.some((x) => matches(x, a)) && !p.notActions.some((x) => matches(x, a)));
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card title={<>What can <span className="text-accent">{session?.current.name}</span> do here?</>}>
        <div className="divide-y divide-border">
          {PROBES.map((a) => {
            const ok = allowed(a);
            return (
              <div key={a} className="flex items-center gap-3 py-2">
                <span className={cx("grid size-6 place-items-center rounded-full", ok ? "bg-ok-soft text-ok" : "bg-bad-soft text-bad")}>{ok ? <Check className="size-3.5" /> : <Minus className="size-3.5" />}</span>
                <span className="font-mono text-xs">{a}</span>
              </div>
            );
          })}
        </div>
        <p className="mt-3 text-xs text-muted">Switch identity in the top bar and the answers change: the emulator evaluates RBAC for real.</p>
      </Card>
      <Card title="Raw permissions response">
        <JsonView value={perms.data} />
      </Card>
    </div>
  );
}
