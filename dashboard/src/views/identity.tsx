"use client";

import { AppWindow, Building2, Crown, Plus, RefreshCw, RotateCcw, Trash2, UserRound, Users, UsersRound } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { useSWRConfig } from "swr";

import { ConfirmDialog, Dialog, useToast } from "@/components/overlay";
import { DataTable } from "@/components/table";
import { Avatar, Badge, Button, Card, EmptyState, ErrorState, Field, Input, Mono, PageHeader, Select, Skeleton, Tabs, statusTone } from "@/components/ui";
import { call, useApi, useList } from "@/lib/api";
import { ago, type Application, type DirectoryRole, type GraphObject, type Group, type ServicePrincipal, type User } from "@/lib/azure";
import { AppDrawer, GroupDrawer, RoleDrawer, SpDrawer, UserDrawer } from "./identity-detail";

type Tab = "users" | "groups" | "apps" | "sps" | "roles" | "deleted" | "org";

const graphKeys = (k: unknown) => Array.isArray(k) && k[0] === "graph";

export function Identity() {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const tab = (params.get("tab") as Tab) || "users";
  const selected = params.get("id");
  const [creating, setCreating] = useState(params.get("create") === "1");

  const users = useList<User>("graph", "/v1.0/users?$top=999");
  const groups = useList<Group>("graph", "/v1.0/groups?$top=999");
  const apps = useList<Application>("graph", "/v1.0/applications?$top=999");
  const sps = useList<ServicePrincipal>("graph", "/v1.0/servicePrincipals?$top=999");
  const roles = useList<DirectoryRole>("graph", "/v1.0/directoryRoles");

  const set = (next: Partial<{ tab: Tab; id: string | null }>) => {
    const p = new URLSearchParams();
    p.set("tab", next.tab ?? tab);
    const id = "id" in next ? next.id : selected;
    if (id && !next.tab) p.set("id", id);
    router.replace(`${pathname}?${p}`, { scroll: false });
  };
  const close = () => set({ id: null });

  const createLabel: Partial<Record<Tab, string>> = { users: "New user", groups: "New group", apps: "New app registration" };

  return (
    <>
      <PageHeader
        icon={<Users className="size-5" />}
        title="Identity"
        description="Microsoft Entra ID objects served by the emulator's Microsoft Graph. Graph app permissions are enforced: the Reader identity can look but not touch."
        actions={
          createLabel[tab] && (
            <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>
              {createLabel[tab]}
            </Button>
          )
        }
      />
      <div className="mb-4">
        <Tabs<Tab>
          value={tab}
          onChange={(t) => set({ tab: t, id: null })}
          tabs={[
            { id: "users", label: "Users", count: users.data ? users.items.length : undefined, icon: <UserRound className="size-4" /> },
            { id: "groups", label: "Groups", count: groups.data ? groups.items.length : undefined, icon: <UsersRound className="size-4" /> },
            { id: "apps", label: "App registrations", count: apps.data ? apps.items.length : undefined, icon: <AppWindow className="size-4" /> },
            { id: "sps", label: "Enterprise apps", count: sps.data ? sps.items.length : undefined, icon: <AppWindow className="size-4" /> },
            { id: "roles", label: "Directory roles", count: roles.data ? roles.items.length : undefined, icon: <Crown className="size-4" /> },
            { id: "deleted", label: "Deleted items", icon: <Trash2 className="size-4" /> },
            { id: "org", label: "Organization", icon: <Building2 className="size-4" /> },
          ]}
        />
      </div>

      {tab === "users" && (
        <DataTable
          rows={users.items}
          loading={users.isLoading}
          error={users.error}
          retry={() => users.mutate()}
          rowKey={(u) => u.id}
          selectedKey={selected}
          onRowClick={(u) => set({ id: u.id })}
          searchText={(u) => `${u.displayName} ${u.userPrincipalName} ${u.jobTitle ?? ""} ${u.department ?? ""}`}
          searchPlaceholder="Search users by name, UPN, job title…"
          columns={[
            {
              key: "n",
              header: "Name",
              sort: (u) => (u.displayName ?? "").toLowerCase(),
              render: (u) => (
                <span className="flex items-center gap-3">
                  <Avatar name={u.displayName || u.userPrincipalName} />
                  <span className="min-w-0">
                    <span className="block font-medium">{u.displayName}</span>
                    <span className="block truncate text-xs text-muted">{u.userPrincipalName}</span>
                  </span>
                </span>
              ),
            },
            { key: "j", header: "Job title", sort: (u) => u.jobTitle ?? "", render: (u) => <span className="text-muted">{u.jobTitle || "—"}</span> },
            { key: "t", header: "Type", render: (u) => <Badge>{u.userType ?? "Member"}</Badge> },
            { key: "e", header: "Sign-in", render: (u) => <Badge tone={u.accountEnabled === false ? "bad" : "ok"} dot>{u.accountEnabled === false ? "Blocked" : "Enabled"}</Badge> },
            { key: "c", header: "Created", sort: (u) => u.createdDateTime ?? "", render: (u) => <span className="text-muted">{ago(u.createdDateTime)}</span> },
          ]}
        />
      )}

      {tab === "groups" && (
        <DataTable
          rows={groups.items}
          loading={groups.isLoading}
          error={groups.error}
          retry={() => groups.mutate()}
          rowKey={(g) => g.id}
          selectedKey={selected}
          onRowClick={(g) => set({ id: g.id })}
          searchText={(g) => `${g.displayName} ${g.description ?? ""} ${g.mailNickname ?? ""}`}
          searchPlaceholder="Search groups…"
          empty={<EmptyState icon={<UsersRound className="size-5" />} title="No groups yet" description="Create one, or apply an azuread_group with Terraform." action={<Button variant="primary" onClick={() => setCreating(true)}>New group</Button>} />}
          columns={[
            {
              key: "n",
              header: "Name",
              sort: (g) => (g.displayName ?? "").toLowerCase(),
              render: (g) => (
                <span className="flex items-center gap-3">
                  <Avatar name={g.displayName ?? "?"} kind="group" />
                  <span className="min-w-0">
                    <span className="block font-medium">{g.displayName}</span>
                    <span className="block truncate text-xs text-muted">{g.description || g.mailNickname}</span>
                  </span>
                </span>
              ),
            },
            { key: "t", header: "Type", render: (g) => <Badge tone={g.securityEnabled ? "accent" : "info"}>{g.groupTypes?.includes("Unified") ? "Microsoft 365" : g.securityEnabled ? "Security" : "Distribution"}</Badge> },
            { key: "c", header: "Created", sort: (g) => g.createdDateTime ?? "", render: (g) => <span className="text-muted">{ago(g.createdDateTime)}</span> },
          ]}
        />
      )}

      {tab === "apps" && (
        <DataTable
          rows={apps.items}
          loading={apps.isLoading}
          error={apps.error}
          retry={() => apps.mutate()}
          rowKey={(a) => a.id}
          selectedKey={selected}
          onRowClick={(a) => set({ id: a.id })}
          searchText={(a) => `${a.displayName} ${a.appId}`}
          searchPlaceholder="Search by name or client ID…"
          columns={[
            {
              key: "n",
              header: "Display name",
              sort: (a) => (a.displayName ?? "").toLowerCase(),
              render: (a) => (
                <span className="flex items-center gap-3">
                  <Avatar name={a.displayName ?? "?"} kind="app" />
                  <span className="font-medium">{a.displayName}</span>
                </span>
              ),
            },
            { key: "id", header: "Application (client) ID", render: (a) => <Mono copy>{a.appId}</Mono> },
            {
              key: "s",
              header: "Secrets",
              render: (a) => {
                const n = a.passwordCredentials?.length ?? 0;
                const expired = a.passwordCredentials?.some((p) => p.endDateTime && new Date(p.endDateTime) < new Date());
                return n ? <Badge tone={expired ? "warn" : "ok"}>{n} {expired ? "· expired" : "active"}</Badge> : <span className="text-subtle">—</span>;
              },
            },
            { key: "c", header: "Created", sort: (a) => a.createdDateTime ?? "", render: (a) => <span className="text-muted">{ago(a.createdDateTime)}</span> },
          ]}
        />
      )}

      {tab === "sps" && (
        <DataTable
          rows={sps.items}
          loading={sps.isLoading}
          error={sps.error}
          retry={() => sps.mutate()}
          rowKey={(s) => s.id}
          selectedKey={selected}
          onRowClick={(s) => set({ id: s.id })}
          searchText={(s) => `${s.displayName} ${s.appId}`}
          columns={[
            {
              key: "n",
              header: "Name",
              sort: (s) => (s.displayName ?? "").toLowerCase(),
              render: (s) => (
                <span className="flex items-center gap-3">
                  <Avatar name={s.displayName ?? "?"} kind="sp" />
                  <span className="font-medium">{s.displayName}</span>
                </span>
              ),
            },
            { key: "id", header: "Application ID", render: (s) => <Mono copy>{s.appId}</Mono> },
            { key: "t", header: "Type", render: (s) => <Badge>{s.servicePrincipalType ?? "Application"}</Badge> },
            { key: "r", header: "App roles", render: (s) => <span className="tabular-nums text-muted">{s.appRoles?.length ?? 0}</span> },
          ]}
        />
      )}

      {tab === "roles" && (
        <DataTable
          rows={roles.items}
          loading={roles.isLoading}
          error={roles.error}
          rowKey={(r) => r.id}
          selectedKey={selected}
          onRowClick={(r) => set({ id: r.id })}
          searchText={(r) => `${r.displayName} ${r.description}`}
          columns={[
            {
              key: "n",
              header: "Role",
              sort: (r) => r.displayName ?? "",
              render: (r) => (
                <span className="flex items-center gap-3">
                  <Avatar name={r.displayName ?? "?"} kind="role" />
                  <span className="font-medium">{r.displayName}</span>
                </span>
              ),
            },
            { key: "d", header: "Description", render: (r) => <span className="line-clamp-2 text-muted">{r.description}</span> },
          ]}
        />
      )}

      {tab === "deleted" && <DeletedItems />}
      {tab === "org" && <Organization />}

      {tab === "users" && <UserDrawer id={selected} onClose={close} />}
      {tab === "groups" && <GroupDrawer id={selected} onClose={close} />}
      {tab === "apps" && <AppDrawer id={selected} onClose={close} />}
      {tab === "sps" && <SpDrawer id={selected} onClose={close} />}
      {tab === "roles" && <RoleDrawer id={selected} onClose={close} />}

      {tab === "users" && <CreateUser open={creating} onClose={() => setCreating(false)} onCreated={(id) => set({ id })} />}
      {tab === "groups" && <CreateGroup open={creating} onClose={() => setCreating(false)} onCreated={(id) => set({ id })} />}
      {tab === "apps" && <CreateApp open={creating} onClose={() => setCreating(false)} onCreated={(id) => set({ id })} />}
    </>
  );
}

function genPassword() {
  const sets = ["ABCDEFGHJKLMNPQRSTUVWXYZ", "abcdefghijkmnopqrstuvwxyz", "23456789", "!#%+=?@"];
  const rnd = (s: string) => s[crypto.getRandomValues(new Uint32Array(1))[0] % s.length];
  const chars = sets.map(rnd);
  for (let i = 0; i < 12; i++) chars.push(rnd(sets.join("")));
  for (let i = chars.length - 1; i > 0; i--) {
    const j = crypto.getRandomValues(new Uint32Array(1))[0] % (i + 1);
    [chars[i], chars[j]] = [chars[j], chars[i]];
  }
  return chars.join("");
}

function useCreate() {
  const toast = useToast();
  const { mutate } = useSWRConfig();
  const [busy, setBusy] = useState(false);
  const run = async (fn: () => Promise<{ id: string; displayName?: string }>, done: (id: string) => void) => {
    setBusy(true);
    try {
      const o = await fn();
      toast.push({ tone: "ok", title: `Created ${o.displayName ?? o.id}` });
      await mutate(graphKeys);
      done(o.id);
    } catch (e) {
      toast.error(e, "Create failed");
    } finally {
      setBusy(false);
    }
  };
  return { busy, run };
}

function CreateUser({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (id: string) => void }) {
  const domains = useList<{ id: string; isVerified: boolean; isDefault: boolean }>("graph", open ? "/v1.0/domains" : null);
  const [name, setName] = useState("");
  const [alias, setAlias] = useState("");
  const [domain, setDomain] = useState("");
  const [job, setJob] = useState("");
  const [pw, setPw] = useState(genPassword);
  const { busy, run } = useCreate();
  const dom = domain || domains.items.find((d) => d.isDefault)?.id || domains.items[0]?.id || "";
  const nick = alias || name.toLowerCase().replace(/[^a-z0-9]+/g, ".").replace(/^\.|\.$/g, "");
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="New user"
      description="Password complexity and verified-domain rules are enforced by the emulator, just like Entra ID."
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!name || !nick || !dom}
            onClick={() =>
              run(
                () =>
                  call<User>("graph", "/v1.0/users", {
                    method: "POST",
                    body: {
                      accountEnabled: true,
                      displayName: name,
                      mailNickname: nick,
                      userPrincipalName: `${nick}@${dom}`,
                      jobTitle: job || undefined,
                      passwordProfile: { forceChangePasswordNextSignIn: true, password: pw },
                    },
                  }),
                (id) => {
                  onClose();
                  setName("");
                  setAlias("");
                  setJob("");
                  setPw(genPassword());
                  onCreated(id);
                },
              )
            }
          >
            Create user
          </Button>
        </>
      }
    >
      <Field label="Display name">
        <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Grace Hopper" />
      </Field>
      <Field label="User principal name">
        <div className="flex items-center gap-1">
          <Input value={nick} onChange={(e) => setAlias(e.target.value)} placeholder="grace.hopper" />
          <span className="text-muted">@</span>
          <Select value={dom} onChange={(e) => setDomain(e.target.value)}>
            {domains.items.map((d) => (
              <option key={d.id} value={d.id}>
                {d.id}
              </option>
            ))}
          </Select>
        </div>
      </Field>
      <Field label="Job title (optional)">
        <Input value={job} onChange={(e) => setJob(e.target.value)} placeholder="Platform engineer" />
      </Field>
      <Field label="Initial password" hint="Generated to meet complexity rules. The user must change it at first sign-in.">
        <div className="flex gap-2">
          <Input value={pw} onChange={(e) => setPw(e.target.value)} className="font-mono" />
          <Button variant="ghost" icon={<RefreshCw className="size-4" />} onClick={() => setPw(genPassword())} aria-label="Generate" />
        </div>
      </Field>
    </Dialog>
  );
}

function CreateGroup({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (id: string) => void }) {
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const { busy, run } = useCreate();
  const nick = name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="New security group"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!name}
            onClick={() =>
              run(
                () => call<Group>("graph", "/v1.0/groups", { method: "POST", body: { displayName: name, description: desc || undefined, mailEnabled: false, mailNickname: nick || "group", securityEnabled: true } }),
                (id) => {
                  onClose();
                  setName("");
                  setDesc("");
                  onCreated(id);
                },
              )
            }
          >
            Create group
          </Button>
        </>
      }
    >
      <Field label="Group name">
        <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="platform-admins" />
      </Field>
      <Field label="Description (optional)">
        <Input value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="People who run the platform" />
      </Field>
    </Dialog>
  );
}

function CreateApp({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (id: string) => void }) {
  const [name, setName] = useState("");
  const [withSp, setWithSp] = useState(true);
  const { busy, run } = useCreate();
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Register an application"
      description="Creates an application object; optionally its service principal (the enterprise app) so it can sign in and be assigned roles."
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!name}
            onClick={() =>
              run(
                async () => {
                  const app = await call<Application>("graph", "/v1.0/applications", { method: "POST", body: { displayName: name, signInAudience: "AzureADMyOrg" } });
                  if (withSp) await call("graph", "/v1.0/servicePrincipals", { method: "POST", body: { appId: app.appId } });
                  return app;
                },
                (id) => {
                  onClose();
                  setName("");
                  onCreated(id);
                },
              )
            }
          >
            Register
          </Button>
        </>
      }
    >
      <Field label="Name">
        <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="my-deploy-pipeline" />
      </Field>
      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={withSp} onChange={(e) => setWithSp(e.target.checked)} className="size-4 accent-[var(--accent)]" />
        Also create the service principal
      </label>
    </Dialog>
  );
}

function DeletedItems() {
  const [type, setType] = useState<"user" | "group" | "application">("user");
  const items = useList<GraphObject>("graph", `/v1.0/directory/deletedItems/microsoft.graph.${type}`);
  const toast = useToast();
  const { mutate } = useSWRConfig();
  const [purge, setPurge] = useState<GraphObject | null>(null);
  return (
    <>
      <DataTable
        rows={items.items}
        loading={items.isLoading}
        error={items.error}
        retry={() => items.mutate()}
        rowKey={(o) => o.id}
        searchText={(o) => `${o.displayName} ${o.id}`}
        toolbar={
          <Select value={type} onChange={(e) => setType(e.target.value as typeof type)} className="h-8 w-40 text-xs">
            <option value="user">Users</option>
            <option value="group">Groups</option>
            <option value="application">Applications</option>
          </Select>
        }
        empty={<EmptyState icon={<Trash2 className="size-5" />} title="Recycle bin is empty" description="Deleted users, groups and apps stay here (soft delete) until restored or purged." />}
        columns={[
          { key: "n", header: "Name", render: (o) => <span className="font-medium">{o.displayName ?? o.id}</span> },
          { key: "id", header: "Object ID", render: (o) => <Mono copy>{o.id}</Mono> },
          { key: "d", header: "Deleted", render: (o) => <span className="text-muted">{ago(o.deletedDateTime as string)}</span> },
          {
            key: "a",
            header: "",
            className: "text-right",
            render: (o) => (
              <span className="inline-flex gap-1" onClick={(e) => e.stopPropagation()}>
                <Button
                  size="sm"
                  icon={<RotateCcw className="size-3.5" />}
                  onClick={async () => {
                    try {
                      await call("graph", `/v1.0/directory/deletedItems/${o.id}/restore`, { method: "POST" });
                      toast.push({ tone: "ok", title: `Restored ${o.displayName ?? o.id}` });
                      await mutate(graphKeys);
                    } catch (e) {
                      toast.error(e, "Restore failed");
                    }
                  }}
                >
                  Restore
                </Button>
                <Button size="sm" variant="ghost" className="text-bad" icon={<Trash2 className="size-3.5" />} onClick={() => setPurge(o)} aria-label="Delete permanently" />
              </span>
            ),
          },
        ]}
      />
      <ConfirmDialog
        open={!!purge}
        onClose={() => setPurge(null)}
        title={`Permanently delete ${purge?.displayName ?? ""}?`}
        description="This cannot be undone."
        confirmLabel="Delete permanently"
        onConfirm={async () => {
          try {
            await call("graph", `/v1.0/directory/deletedItems/${purge!.id}`, { method: "DELETE" });
            toast.push({ tone: "ok", title: "Permanently deleted" });
            await mutate(graphKeys);
          } catch (e) {
            toast.error(e, "Delete failed");
          }
        }}
      />
    </>
  );
}

function Organization() {
  const org = useApi<{ value: { id: string; displayName: string; createdDateTime?: string; verifiedDomains?: { name: string; isDefault: boolean; isInitial: boolean; type?: string }[] }[] }>("graph", "/v1.0/organization");
  const domains = useList<{ id: string; isVerified: boolean; isDefault: boolean; authenticationType?: string }>("graph", "/v1.0/domains");
  if (org.error) return <Card padded={false}><ErrorState error={org.error} /></Card>;
  const o = org.data?.value?.[0];
  if (!o) return <Skeleton className="h-48 rounded-xl" />;
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card title="Tenant">
        <div className="flex items-center gap-3">
          <span className="grid size-12 place-items-center rounded-xl bg-accent-soft text-accent">
            <Building2 className="size-6" />
          </span>
          <div>
            <div className="text-lg font-semibold">{o.displayName}</div>
            <Mono copy>{o.id}</Mono>
          </div>
        </div>
        <div className="mt-4 text-sm text-muted">Created {o.createdDateTime ? new Date(o.createdDateTime).toLocaleString() : "—"}</div>
      </Card>
      <Card title="Domains">
        <div className="divide-y divide-border">
          {domains.items.map((d) => (
            <div key={d.id} className="flex items-center gap-2 py-2 text-sm">
              <span className="flex-1 font-medium">{d.id}</span>
              {d.isDefault && <Badge tone="accent">default</Badge>}
              <Badge tone={statusTone(d.isVerified ? "ok" : "warning")} dot>
                {d.isVerified ? "verified" : "unverified"}
              </Badge>
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}
