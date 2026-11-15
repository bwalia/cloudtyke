"use client";

import { KeyRound, Plus, Save, ShieldCheck, Trash2, UserMinus } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useSWRConfig } from "swr";

import { JsonView } from "@/components/json";
import { ConfirmDialog, Dialog, Drawer, useToast } from "@/components/overlay";
import { Avatar, Badge, Button, CopyButton, EmptyState, ErrorState, Field, Input, Mono, Select, SkeletonRows, Tabs, cx } from "@/components/ui";
import { API, armPath, call, useApi, useList } from "@/lib/api";
import { BUILTIN_ROLES, ago, roleId, type Application, type DirectoryRole, type GraphObject, type Group, type RoleAssignment, type ServicePrincipal, type User } from "@/lib/azure";

const graphKeys = (k: unknown) => Array.isArray(k) && k[0] === "graph";
const kindOf = (o: GraphObject) => {
  const t = (o["@odata.type"] ?? "").replace("#microsoft.graph.", "");
  return t === "group" ? "group" : t === "servicePrincipal" ? "sp" : t === "application" ? "app" : "user";
};

function useDelete(path: string, label: string, onDone: () => void) {
  const toast = useToast();
  const { mutate } = useSWRConfig();
  return async () => {
    try {
      await call("graph", path, { method: "DELETE" });
      toast.push({ tone: "ok", title: `Deleted ${label}`, body: "Soft-deleted: restore it from Deleted items." });
      await mutate(graphKeys);
      onDone();
    } catch (e) {
      toast.error(e, "Delete failed");
    }
  };
}

function Props({ rows }: { rows: [string, ReactNode][] }) {
  return (
    <dl className="grid grid-cols-[150px_1fr] gap-x-4 gap-y-2.5 text-sm">
      {rows.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-muted">{k}</dt>
          <dd className="min-w-0 break-words">{v ?? <span className="text-subtle">—</span>}</dd>
        </div>
      ))}
    </dl>
  );
}

function ObjectList({ items, loading, empty, action }: { items: GraphObject[]; loading?: boolean; empty: string; action?: (o: GraphObject) => ReactNode }) {
  if (loading) return <SkeletonRows rows={3} />;
  if (!items.length) return <div className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-sm text-subtle">{empty}</div>;
  return (
    <div className="divide-y divide-border rounded-lg border border-border">
      {items.map((o) => (
        <div key={o.id} className="flex items-center gap-3 px-3 py-2">
          <Avatar name={o.displayName ?? o.id} kind={kindOf(o)} />
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{o.displayName ?? o.id}</div>
            <div className="truncate text-xs text-muted">{(o.userPrincipalName as string) || (o["@odata.type"] ?? "").replace("#microsoft.graph.", "")}</div>
          </div>
          {action?.(o)}
        </div>
      ))}
    </div>
  );
}

/** ARM role assignments held by a principal, at any scope. */
function PrincipalRoles({ id }: { id: string }) {
  const f = encodeURIComponent(`principalId eq '${id}'`);
  const ras = useList<RoleAssignment>("arm", armPath(`/providers/Microsoft.Authorization/roleAssignments?$filter=${f}`, API.auth));
  if (ras.error) return <ErrorState error={ras.error} />;
  if (ras.isLoading) return <SkeletonRows rows={2} />;
  if (!ras.items.length) return <div className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-sm text-subtle">No Azure role assignments</div>;
  return (
    <div className="divide-y divide-border rounded-lg border border-border">
      {ras.items.map((r) => (
        <div key={r.id} className="flex items-center gap-3 px-3 py-2 text-sm">
          <ShieldCheck className="size-4 text-accent" />
          <Badge tone="accent">{BUILTIN_ROLES[roleId(r.properties.roleDefinitionId)] ?? roleId(r.properties.roleDefinitionId)}</Badge>
          <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted">{r.properties.scope}</span>
        </div>
      ))}
    </div>
  );
}

function Section({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) {
  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-subtle">{title}</h3>
        {action}
      </div>
      {children}
    </section>
  );
}

// ------------------------------------------------------------------ user

export function UserDrawer({ id, onClose }: { id: string | null; onClose: () => void }) {
  const u = useApi<User>("graph", id ? `/v1.0/users/${id}` : null);
  const memberOf = useList<GraphObject>("graph", id ? `/v1.0/users/${id}/memberOf` : null);
  const manager = useApi<GraphObject>("graph", id ? `/v1.0/users/${id}/manager` : null, { shouldRetryOnError: false });
  const [tab, setTab] = useState<"overview" | "access" | "json">("overview");
  const [del, setDel] = useState(false);
  const remove = useDelete(`/v1.0/users/${id}`, u.data?.displayName ?? "user", onClose);
  const user = u.data;
  return (
    <Drawer
      open={!!id}
      onClose={onClose}
      icon={<Avatar name={user?.displayName ?? "?"} />}
      title={user?.displayName ?? "…"}
      subtitle={user?.userPrincipalName}
      actions={<Button size="sm" variant="ghost" className="text-bad" icon={<Trash2 className="size-3.5" />} onClick={() => setDel(true)} aria-label="Delete" />}
    >
      <div className="px-5 pt-2">
        <Tabs value={tab} onChange={setTab} tabs={[{ id: "overview", label: "Overview" }, { id: "access", label: "Groups & roles" }, { id: "json", label: "JSON" }]} />
      </div>
      <div className="space-y-6 p-5">
        {u.error && <ErrorState error={u.error} />}
        {user && tab === "overview" && (
          <>
            <EditUser key={JSON.stringify([user.displayName, user.jobTitle, user.department, user.accountEnabled])} user={user} onSaved={() => u.mutate()} />
            <Props
              rows={[
                ["Object ID", <Mono key="i" copy>{user.id}</Mono>],
                ["User type", user.userType],
                ["Manager", manager.data ? manager.data.displayName : "—"],
                ["Created", user.createdDateTime ? `${new Date(user.createdDateTime).toLocaleString()} (${ago(user.createdDateTime)})` : "—"],
              ]}
            />
          </>
        )}
        {user && tab === "access" && (
          <>
            <Section title={`Member of (${memberOf.items.length})`}>
              <ObjectList items={memberOf.items} loading={memberOf.isLoading} empty="Not a member of any group" />
            </Section>
            <Section title="Azure role assignments">
              <PrincipalRoles id={user.id} />
            </Section>
          </>
        )}
        {user && tab === "json" && <JsonView value={user} />}
      </div>
      <ConfirmDialog open={del} onClose={() => setDel(false)} title={`Delete ${user?.displayName}?`} description="The user moves to Deleted items and can be restored for 30 days." onConfirm={remove} />
    </Drawer>
  );
}

function EditUser({ user, onSaved }: { user: User; onSaved: () => void }) {
  const [f, setF] = useState({ displayName: user.displayName ?? "", jobTitle: user.jobTitle ?? "", department: user.department ?? "", accountEnabled: user.accountEnabled !== false });
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const { mutate } = useSWRConfig();
  const dirty = f.displayName !== (user.displayName ?? "") || f.jobTitle !== (user.jobTitle ?? "") || f.department !== (user.department ?? "") || f.accountEnabled !== (user.accountEnabled !== false);
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Display name">
          <Input value={f.displayName} onChange={(e) => setF({ ...f, displayName: e.target.value })} />
        </Field>
        <Field label="Job title">
          <Input value={f.jobTitle} onChange={(e) => setF({ ...f, jobTitle: e.target.value })} />
        </Field>
        <Field label="Department">
          <Input value={f.department} onChange={(e) => setF({ ...f, department: e.target.value })} />
        </Field>
        <Field label="Sign-in">
          <button
            type="button"
            onClick={() => setF({ ...f, accountEnabled: !f.accountEnabled })}
            className={cx("flex h-9 w-full items-center gap-2 rounded-lg border px-3 text-sm", f.accountEnabled ? "border-ok/40 text-ok" : "border-bad/40 text-bad")}
          >
            <span className={cx("relative h-4 w-7 rounded-full transition-colors", f.accountEnabled ? "bg-ok" : "bg-bad")}>
              <span className={cx("absolute top-0.5 size-3 rounded-full bg-white transition-all", f.accountEnabled ? "left-3.5" : "left-0.5")} />
            </span>
            {f.accountEnabled ? "Allowed" : "Blocked"}
          </button>
        </Field>
      </div>
      {dirty && (
        <div className="flex justify-end gap-2 animate-in">
          <Button size="sm" variant="ghost" onClick={() => setF({ displayName: user.displayName ?? "", jobTitle: user.jobTitle ?? "", department: user.department ?? "", accountEnabled: user.accountEnabled !== false })}>
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
                await call("graph", `/v1.0/users/${user.id}`, { method: "PATCH", body: { displayName: f.displayName, jobTitle: f.jobTitle || null, department: f.department || null, accountEnabled: f.accountEnabled } });
                toast.push({ tone: "ok", title: "User updated" });
                onSaved();
                mutate(graphKeys);
              } catch (e) {
                toast.error(e, "Update failed");
              } finally {
                setBusy(false);
              }
            }}
          >
            Save
          </Button>
        </div>
      )}
    </div>
  );
}

// ------------------------------------------------------------------ group

export function GroupDrawer({ id, onClose }: { id: string | null; onClose: () => void }) {
  const g = useApi<Group>("graph", id ? `/v1.0/groups/${id}` : null);
  const members = useList<GraphObject>("graph", id ? `/v1.0/groups/${id}/members` : null);
  const owners = useList<GraphObject>("graph", id ? `/v1.0/groups/${id}/owners` : null);
  const transitive = useList<GraphObject>("graph", id ? `/v1.0/groups/${id}/transitiveMembers` : null);
  const [tab, setTab] = useState<"members" | "owners" | "roles" | "json">("members");
  const [adding, setAdding] = useState<"members" | "owners" | null>(null);
  const [del, setDel] = useState(false);
  const toast = useToast();
  const { mutate } = useSWRConfig();
  const remove = useDelete(`/v1.0/groups/${id}`, g.data?.displayName ?? "group", onClose);
  const group = g.data;

  const unlink = (rel: "members" | "owners") =>
    function Unlink(o: GraphObject) {
      return (
        <Button
          size="sm"
          variant="ghost"
          icon={<UserMinus className="size-3.5" />}
          aria-label="Remove"
          onClick={async () => {
            try {
              await call("graph", `/v1.0/groups/${id}/${rel}/${o.id}/$ref`, { method: "DELETE" });
              toast.push({ tone: "ok", title: `Removed ${o.displayName}` });
              mutate(graphKeys);
            } catch (e) {
              toast.error(e, "Remove failed");
            }
          }}
        />
      );
    };

  const nested = transitive.items.length - members.items.length;
  return (
    <Drawer
      open={!!id}
      onClose={onClose}
      icon={<Avatar name={group?.displayName ?? "?"} kind="group" />}
      title={group?.displayName ?? "…"}
      subtitle={group?.description || group?.mailNickname}
      actions={<Button size="sm" variant="ghost" className="text-bad" icon={<Trash2 className="size-3.5" />} onClick={() => setDel(true)} aria-label="Delete" />}
    >
      <div className="px-5 pt-2">
        <Tabs
          value={tab}
          onChange={setTab}
          tabs={[
            { id: "members", label: "Members", count: members.items.length },
            { id: "owners", label: "Owners", count: owners.items.length },
            { id: "roles", label: "Azure roles" },
            { id: "json", label: "JSON" },
          ]}
        />
      </div>
      <div className="space-y-5 p-5">
        {g.error && <ErrorState error={g.error} />}
        {tab === "members" && (
          <Section
            title="Direct members"
            action={
              <Button size="sm" icon={<Plus className="size-3.5" />} onClick={() => setAdding("members")}>
                Add members
              </Button>
            }
          >
            <ObjectList items={members.items} loading={members.isLoading} empty="No members yet" action={unlink("members")} />
            {nested > 0 && <div className="mt-2 text-xs text-muted">+ {nested} more through nested groups (transitive members)</div>}
          </Section>
        )}
        {tab === "owners" && (
          <Section
            title="Owners"
            action={
              <Button size="sm" icon={<Plus className="size-3.5" />} onClick={() => setAdding("owners")}>
                Add owners
              </Button>
            }
          >
            <ObjectList items={owners.items} loading={owners.isLoading} empty="No owners" action={unlink("owners")} />
          </Section>
        )}
        {tab === "roles" && id && <PrincipalRoles id={id} />}
        {tab === "json" && group && <JsonView value={group} />}
      </div>
      {adding && id && (
        <AddMembers
          rel={adding}
          groupId={id}
          exclude={[id, ...(adding === "members" ? members.items : owners.items).map((m) => m.id)]}
          onClose={() => setAdding(null)}
        />
      )}
      <ConfirmDialog open={del} onClose={() => setDel(false)} title={`Delete ${group?.displayName}?`} description="The group moves to Deleted items." onConfirm={remove} />
    </Drawer>
  );
}

function AddMembers({ rel, groupId, exclude, onClose }: { rel: "members" | "owners"; groupId: string; exclude: string[]; onClose: () => void }) {
  const users = useList<User>("graph", "/v1.0/users?$top=999");
  const groups = useList<Group>("graph", "/v1.0/groups?$top=999");
  const sps = useList<ServicePrincipal>("graph", "/v1.0/servicePrincipals?$top=999");
  const [q, setQ] = useState("");
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const { mutate } = useSWRConfig();
  const all: (GraphObject & { kind: "user" | "group" | "sp"; sub: string })[] = [
    ...users.items.map((u) => ({ ...u, kind: "user" as const, sub: u.userPrincipalName })),
    ...(rel === "members" ? groups.items.map((g) => ({ ...g, kind: "group" as const, sub: "group" })) : []),
    ...sps.items.map((s) => ({ ...s, kind: "sp" as const, sub: "service principal" })),
  ].filter((o) => !exclude.includes(o.id) && `${o.displayName} ${o.sub}`.toLowerCase().includes(q.toLowerCase()));
  return (
    <Dialog
      open
      onClose={onClose}
      title={`Add ${rel}`}
      description={rel === "members" ? "Users, groups (nesting) and service principals." : "Users and service principals."}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={busy}
            disabled={!picked.size}
            onClick={async () => {
              setBusy(true);
              try {
                for (const oid of picked) {
                  await call("graph", `/v1.0/groups/${groupId}/${rel}/$ref`, { method: "POST", body: { "@odata.id": `https://graph.microsoft.com/v1.0/directoryObjects/${oid}` } });
                }
                toast.push({ tone: "ok", title: `Added ${picked.size} ${rel}` });
                mutate(graphKeys);
                onClose();
              } catch (e) {
                toast.error(e, "Add failed");
              } finally {
                setBusy(false);
              }
            }}
          >
            Add {picked.size || ""}
          </Button>
        </>
      }
    >
      <Input autoFocus value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search…" />
      <div className="max-h-72 divide-y divide-border overflow-y-auto rounded-lg border border-border">
        {!all.length && <div className="p-6 text-center text-sm text-subtle">Nobody left to add</div>}
        {all.map((o) => (
          <label key={o.id} className="flex cursor-pointer items-center gap-3 px-3 py-2 hover:bg-surface-2">
            <input
              type="checkbox"
              className="size-4 accent-[var(--accent)]"
              checked={picked.has(o.id)}
              onChange={(e) => {
                const n = new Set(picked);
                if (e.target.checked) n.add(o.id);
                else n.delete(o.id);
                setPicked(n);
              }}
            />
            <Avatar name={o.displayName ?? "?"} kind={o.kind} />
            <span className="min-w-0">
              <span className="block truncate text-sm font-medium">{o.displayName}</span>
              <span className="block truncate text-xs text-muted">{o.sub}</span>
            </span>
          </label>
        ))}
      </div>
    </Dialog>
  );
}

// ------------------------------------------------------------------ application

export function AppDrawer({ id, onClose }: { id: string | null; onClose: () => void }) {
  const a = useApi<Application>("graph", id ? `/v1.0/applications/${id}` : null);
  const fics = useList<{ id: string; name: string; issuer: string; subject: string; audiences: string[] }>("graph", id ? `/v1.0/applications/${id}/federatedIdentityCredentials` : null);
  const [tab, setTab] = useState<"creds" | "fic" | "json">("creds");
  const [secret, setSecret] = useState<{ text: string; hint: string } | null>(null);
  const [addFic, setAddFic] = useState(false);
  const [del, setDel] = useState(false);
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  const remove = useDelete(`/v1.0/applications/${id}`, a.data?.displayName ?? "application", onClose);
  const app = a.data;
  return (
    <Drawer
      open={!!id}
      onClose={onClose}
      wide
      icon={<Avatar name={app?.displayName ?? "?"} kind="app" />}
      title={app?.displayName ?? "…"}
      subtitle={app && <>Client ID {app.appId}</>}
      actions={<Button size="sm" variant="ghost" className="text-bad" icon={<Trash2 className="size-3.5" />} onClick={() => setDel(true)} aria-label="Delete" />}
    >
      <div className="px-5 pt-2">
        <Tabs
          value={tab}
          onChange={setTab}
          tabs={[
            { id: "creds", label: "Client secrets", count: app?.passwordCredentials?.length },
            { id: "fic", label: "Federated credentials", count: fics.items.length },
            { id: "json", label: "Manifest" },
          ]}
        />
      </div>
      <div className="space-y-5 p-5">
        {a.error && <ErrorState error={a.error} />}
        {app && (
          <Props
            rows={[
              ["Application (client) ID", <Mono key="a" copy>{app.appId}</Mono>],
              ["Object ID", <Mono key="o" copy>{app.id}</Mono>],
              ["Audience", app.signInAudience],
            ]}
          />
        )}
        {app && tab === "creds" && (
          <Section
            title="Client secrets"
            action={
              <Button
                size="sm"
                loading={busy}
                icon={<KeyRound className="size-3.5" />}
                onClick={async () => {
                  setBusy(true);
                  try {
                    const r = await call<{ secretText: string; hint: string }>("graph", `/v1.0/applications/${app.id}/addPassword`, {
                      method: "POST",
                      body: { passwordCredential: { displayName: `dashboard ${new Date().toISOString().slice(0, 10)}` } },
                    });
                    setSecret({ text: r.secretText, hint: r.hint });
                    a.mutate();
                  } catch (e) {
                    toast.error(e, "Could not add a secret");
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                New secret
              </Button>
            }
          >
            {secret && (
              <div className="mb-3 rounded-lg border border-ok/30 bg-ok-soft p-3 animate-in">
                <div className="text-sm font-semibold text-ok">Copy this secret now. It won&apos;t be shown again.</div>
                <div className="mt-2 flex items-center gap-2 rounded-md bg-surface px-2 py-1.5 font-mono text-xs">
                  <span className="flex-1 break-all">{secret.text}</span>
                  <CopyButton value={secret.text} />
                </div>
                <div className="mt-2 text-xs text-muted">It authenticates for real: client_id {app.appId} + this secret gets a token from the emulator&apos;s login endpoint.</div>
              </div>
            )}
            {!app.passwordCredentials?.length ? (
              <div className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-sm text-subtle">No client secrets</div>
            ) : (
              <div className="divide-y divide-border rounded-lg border border-border">
                {app.passwordCredentials.map((p) => (
                  <div key={p.keyId} className="flex items-center gap-3 px-3 py-2 text-sm">
                    <KeyRound className="size-4 text-subtle" />
                    <div className="min-w-0 flex-1">
                      <div className="font-medium">{p.displayName || "secret"}</div>
                      <div className="text-xs text-muted">
                        {p.hint ? `${p.hint}••• · ` : ""}expires {p.endDateTime ? new Date(p.endDateTime).toLocaleDateString() : "never"}
                      </div>
                    </div>
                    <Button
                      size="sm"
                      variant="ghost"
                      icon={<Trash2 className="size-3.5" />}
                      aria-label="Remove secret"
                      onClick={async () => {
                        try {
                          await call("graph", `/v1.0/applications/${app.id}/removePassword`, { method: "POST", body: { keyId: p.keyId } });
                          toast.push({ tone: "ok", title: "Secret removed" });
                          a.mutate();
                        } catch (e) {
                          toast.error(e, "Remove failed");
                        }
                      }}
                    />
                  </div>
                ))}
              </div>
            )}
          </Section>
        )}
        {app && tab === "fic" && (
          <Section
            title="Federated identity credentials (workload identity)"
            action={
              <Button size="sm" icon={<Plus className="size-3.5" />} onClick={() => setAddFic(true)}>
                Add
              </Button>
            }
          >
            {!fics.items.length ? (
              <div className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-sm text-subtle">
                None. Add one to let GitHub Actions or Kubernetes sign in without a secret.
              </div>
            ) : (
              <div className="divide-y divide-border rounded-lg border border-border">
                {fics.items.map((f) => (
                  <div key={f.id} className="px-3 py-2 text-sm">
                    <div className="font-medium">{f.name}</div>
                    <div className="mt-0.5 font-mono text-xs text-muted">
                      {f.issuer} → {f.subject}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </Section>
        )}
        {app && tab === "json" && <JsonView value={app} />}
      </div>
      {addFic && app && <AddFic appId={app.id} onClose={() => setAddFic(false)} onDone={() => fics.mutate()} />}
      <ConfirmDialog open={del} onClose={() => setDel(false)} title={`Delete ${app?.displayName}?`} description="The app moves to Deleted items." onConfirm={remove} />
    </Drawer>
  );
}

function AddFic({ appId, onClose, onDone }: { appId: string; onClose: () => void; onDone: () => void }) {
  const [f, setF] = useState({ name: "github-main", issuer: "https://token.actions.githubusercontent.com", subject: "repo:org/repo:ref:refs/heads/main" });
  const [busy, setBusy] = useState(false);
  const toast = useToast();
  return (
    <Dialog
      open
      onClose={onClose}
      title="Add federated credential"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await call("graph", `/v1.0/applications/${appId}/federatedIdentityCredentials`, { method: "POST", body: { ...f, audiences: ["api://AzureADTokenExchange"] } });
                toast.push({ tone: "ok", title: "Federated credential added" });
                onDone();
                onClose();
              } catch (e) {
                toast.error(e, "Add failed");
              } finally {
                setBusy(false);
              }
            }}
          >
            Add
          </Button>
        </>
      }
    >
      <Field label="Name">
        <Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
      </Field>
      <Field label="Issuer">
        <Select value={f.issuer} onChange={(e) => setF({ ...f, issuer: e.target.value })}>
          <option value="https://token.actions.githubusercontent.com">GitHub Actions</option>
          <option value="https://gitlab.com">GitLab</option>
          <option value="https://kubernetes.default.svc.cluster.local">Kubernetes</option>
        </Select>
      </Field>
      <Field label="Subject">
        <Input value={f.subject} onChange={(e) => setF({ ...f, subject: e.target.value })} className="font-mono" />
      </Field>
    </Dialog>
  );
}

// ------------------------------------------------------------------ service principal

export function SpDrawer({ id, onClose }: { id: string | null; onClose: () => void }) {
  const s = useApi<ServicePrincipal>("graph", id ? `/v1.0/servicePrincipals/${id}` : null);
  const assignments = useList<{ id: string; appRoleId: string; resourceDisplayName: string; resourceId: string; createdDateTime?: string }>(
    "graph",
    id ? `/v1.0/servicePrincipals/${id}/appRoleAssignments` : null,
  );
  const graphSp = useList<ServicePrincipal>("graph", id ? `/v1.0/servicePrincipals?$filter=${encodeURIComponent("displayName eq 'Microsoft Graph'")}` : null);
  const [tab, setTab] = useState<"perm" | "roles" | "exposed" | "json">("perm");
  const [del, setDel] = useState(false);
  const remove = useDelete(`/v1.0/servicePrincipals/${id}`, s.data?.displayName ?? "service principal", onClose);
  const sp = s.data;
  const roleName = (rid: string) => graphSp.items[0]?.appRoles?.find((r) => r.id === rid)?.value ?? rid;
  return (
    <Drawer
      open={!!id}
      onClose={onClose}
      wide
      icon={<Avatar name={sp?.displayName ?? "?"} kind="sp" />}
      title={sp?.displayName ?? "…"}
      subtitle={sp && <>App ID {sp.appId}</>}
      actions={<Button size="sm" variant="ghost" className="text-bad" icon={<Trash2 className="size-3.5" />} onClick={() => setDel(true)} aria-label="Delete" />}
    >
      <div className="px-5 pt-2">
        <Tabs
          value={tab}
          onChange={setTab}
          tabs={[
            { id: "perm", label: "API permissions", count: assignments.items.length },
            { id: "roles", label: "Azure roles" },
            { id: "exposed", label: "App roles", count: sp?.appRoles?.length },
            { id: "json", label: "JSON" },
          ]}
        />
      </div>
      <div className="space-y-5 p-5">
        {s.error && <ErrorState error={s.error} />}
        {tab === "perm" &&
          (assignments.error ? (
            <ErrorState error={assignments.error} />
          ) : !assignments.items.length ? (
            <EmptyState title="No application permissions granted" description="Grant a Graph app role (e.g. User.Read.All) with an appRoleAssignment and it is enforced on every Graph call." />
          ) : (
            <div className="divide-y divide-border rounded-lg border border-border">
              {assignments.items.map((r) => (
                <div key={r.id} className="flex items-center gap-3 px-3 py-2 text-sm">
                  <ShieldCheck className="size-4 text-ok" />
                  <span className="font-mono font-medium">{roleName(r.appRoleId)}</span>
                  <span className="text-muted">on {r.resourceDisplayName}</span>
                  <span className="ml-auto text-xs text-subtle">{ago(r.createdDateTime)}</span>
                </div>
              ))}
            </div>
          ))}
        {tab === "roles" && id && <PrincipalRoles id={id} />}
        {tab === "exposed" &&
          (!sp?.appRoles?.length ? (
            <EmptyState title="Exposes no app roles" />
          ) : (
            <div className="divide-y divide-border rounded-lg border border-border">
              {sp.appRoles.map((r) => (
                <div key={r.id} className="px-3 py-2 text-sm">
                  <div className="font-mono font-medium">{r.value}</div>
                  <div className="text-xs text-muted">{r.description || r.displayName}</div>
                </div>
              ))}
            </div>
          ))}
        {tab === "json" && sp && <JsonView value={sp} />}
      </div>
      <ConfirmDialog open={del} onClose={() => setDel(false)} title={`Delete ${sp?.displayName}?`} onConfirm={remove} />
    </Drawer>
  );
}

// ------------------------------------------------------------------ directory role

export function RoleDrawer({ id, onClose }: { id: string | null; onClose: () => void }) {
  const r = useApi<DirectoryRole>("graph", id ? `/v1.0/directoryRoles/${id}` : null);
  const members = useList<GraphObject>("graph", id ? `/v1.0/directoryRoles/${id}/members` : null);
  return (
    <Drawer open={!!id} onClose={onClose} icon={<Avatar name={r.data?.displayName ?? "?"} kind="role" />} title={r.data?.displayName ?? "…"} subtitle="Directory role">
      <div className="space-y-5 p-5">
        {r.error && <ErrorState error={r.error} />}
        {r.data && <p className="text-sm text-muted">{r.data.description}</p>}
        <Section title={`Members (${members.items.length})`}>
          <ObjectList items={members.items} loading={members.isLoading} empty="No members" />
        </Section>
        {r.data && <JsonView value={r.data} collapsedDepth={1} />}
      </div>
    </Drawer>
  );
}
