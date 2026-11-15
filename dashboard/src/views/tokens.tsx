"use client";

import { KeyRound } from "lucide-react";
import { useState } from "react";

import { JsonView } from "@/components/json";
import { Badge, Card, CopyButton, ErrorState, Mono, PageHeader, Skeleton, Tabs } from "@/components/ui";
import { ApiError, call, useApi } from "@/lib/api";
import useSWR from "swr";

interface TokenInfo {
  persona: string;
  resource: string;
  jwt: string;
  header: Record<string, unknown>;
  claims: Record<string, unknown>;
  signatureBytes: number;
}

const CLAIMS: Record<string, string> = {
  aud: "Audience: the API this token is for",
  iss: "Issuer: the emulator's login endpoint for this tenant",
  iat: "Issued at",
  nbf: "Not valid before",
  exp: "Expires",
  tid: "Tenant ID",
  oid: "Object ID of the service principal",
  appid: "Client (application) ID that asked for the token",
  azp: "Authorized party (v2 tokens)",
  sub: "Subject",
  roles: "Graph app roles granted to the app: these drive permission checks",
  groups: "Group memberships",
  idtyp: "Identity type",
  ver: "Token version",
};

export function Tokens() {
  const [resource, setResource] = useState<"management" | "graph">("management");
  const t = useSWR<TokenInfo>(["token", resource], () => call<TokenInfo>("raw", `/api/token?resource=${resource}`));
  const oidc = useApi<Record<string, unknown>>("login", "/7c10d7e4-0000-4000-8000-000000000001/v2.0/.well-known/openid-configuration");
  const jwksUri = typeof oidc.data?.jwks_uri === "string" ? new URL(oidc.data.jwks_uri).pathname : null;
  const jwks = useApi<{ keys: Record<string, unknown>[] }>("login", jwksUri);

  return (
    <>
      <PageHeader
        icon={<KeyRound className="size-5" />}
        title="Tokens & OIDC"
        description="The access token the dashboard is using right now, decoded claim by claim, and the emulator's OpenID Connect discovery and signing keys."
      />
      <div className="mb-4">
        <Tabs
          value={resource}
          onChange={setResource}
          tabs={[
            { id: "management", label: "ARM token" },
            { id: "graph", label: "Microsoft Graph token" },
          ]}
        />
      </div>
      {t.error ? (
        <Card padded={false}>
          <ErrorState error={t.error as ApiError} retry={() => t.mutate()} />
        </Card>
      ) : !t.data ? (
        <Skeleton className="h-72 rounded-xl" />
      ) : (
        <div className="grid gap-4 xl:grid-cols-[1fr_1fr]">
          <Card title={<>Encoded · {t.data.persona}</>} actions={<CopyButton value={t.data.jwt} label="Copy JWT" />}>
            <p className="break-all font-mono text-[12px] leading-relaxed">
              <span className="text-bad">{t.data.jwt.split(".")[0]}</span>.<span className="text-accent">{t.data.jwt.split(".")[1]}</span>.
              <span className="text-ok">{t.data.jwt.split(".")[2]}</span>
            </p>
            <div className="mt-3 flex flex-wrap gap-2 text-xs">
              <Badge tone="bad">header</Badge>
              <Badge tone="accent">payload</Badge>
              <Badge tone="ok">signature · {t.data.header.alg as string} · {t.data.signatureBytes * 8} bits</Badge>
            </div>
            <div className="mt-4">
              <div className="mb-1 text-xs font-semibold uppercase tracking-wider text-subtle">Header</div>
              <JsonView value={t.data.header} />
            </div>
          </Card>
          <Card title="Claims">
            <div className="divide-y divide-border">
              {Object.entries(t.data.claims).map(([k, v]) => (
                <div key={k} className="grid grid-cols-[132px_minmax(0,1fr)] gap-3 py-2 text-sm">
                  <span className="break-all font-mono text-xs font-semibold text-accent">{k}</span>
                  <span className="min-w-0">
                    <span className="block break-all font-mono text-xs">
                      {["iat", "nbf", "exp"].includes(k) && typeof v === "number" ? `${new Date(v * 1000).toLocaleString()}  (${v})` : Array.isArray(v) ? v.join(", ") || "[]" : String(v)}
                    </span>
                    {CLAIMS[k] && <span className="block text-[11px] text-subtle">{CLAIMS[k]}</span>}
                  </span>
                </div>
              ))}
            </div>
          </Card>
        </div>
      )}
      <div className="mt-4 grid gap-4 xl:grid-cols-2">
        <Card title="OpenID configuration" actions={<Mono>/.well-known/openid-configuration</Mono>}>
          {oidc.error ? <ErrorState error={oidc.error} /> : <JsonView value={oidc.data ?? {}} collapsedDepth={1} />}
        </Card>
        <Card title={`Signing keys (JWKS) · ${jwks.data?.keys?.length ?? "…"}`}>
          {jwks.error ? <ErrorState error={jwks.error} /> : <JsonView value={jwks.data ?? {}} collapsedDepth={3} />}
        </Card>
      </div>
    </>
  );
}
