import "server-only";

import fs from "node:fs";
import https from "node:https";
import path from "node:path";

/**
 * Server-side client for the CloudTyke emulator.
 *
 * The emulator routes by host name (management. / login. / graph.<domain>), so
 * every request goes to CLOUDTYKE_URL (https://localhost by default) with the
 * Host header and TLS SNI set to the service's host name. TLS is verified
 * against the emulator's own CA: CLOUDTYKE_CA_FILE, ../azure-emulator/certs/ca.pem,
 * or (only if neither exists) the CA published at /_cloudtyke/ca.pem, fetched
 * once on first use.
 */

export const DOMAIN = process.env.CLOUDTYKE_DOMAIN || "cloudtyke.local";
const BASE = new URL(process.env.CLOUDTYKE_URL || "https://localhost");
const ADMIN_TOKEN = process.env.CLOUDTYKE_ADMIN_TOKEN || "";
export const TENANT = process.env.CLOUDTYKE_TENANT_ID || "7c10d7e4-0000-4000-8000-000000000001";

export type Service = "management" | "login" | "graph";

/** Seeded, local-only identities the dashboard can act as (see azure-emulator/README.md). */
export const PERSONAS = {
  owner: {
    id: "owner",
    name: "cloudtyke-bootstrap",
    role: "Owner at / · Graph read/write",
    clientId: process.env.CLOUDTYKE_CLIENT_ID || "a99c0000-0000-4000-8000-000000000001",
    secret: process.env.CLOUDTYKE_CLIENT_SECRET || "cloudtyke-bootstrap-secret",
  },
  reader: {
    id: "reader",
    name: "cloudtyke-reader",
    role: "Reader on sub-sandbox · Graph read",
    clientId: process.env.CLOUDTYKE_READER_CLIENT_ID || "a99c0000-0000-4000-8000-000000000002",
    secret: process.env.CLOUDTYKE_READER_CLIENT_SECRET || "cloudtyke-reader-secret",
  },
} as const;
export type PersonaId = keyof typeof PERSONAS;
export const isPersona = (v: unknown): v is PersonaId => v === "owner" || v === "reader";

export const host = (svc: Service) => `${svc}.${DOMAIN}`;

let agentPromise: Promise<https.Agent> | null = null;

function rawRequest(
  agent: https.Agent | undefined,
  insecure: boolean,
  svcHost: string,
  method: string,
  pathAndQuery: string,
  headers: Record<string, string>,
  body?: Buffer,
): Promise<EmuResponse> {
  return new Promise((resolve, reject) => {
    const started = performance.now();
    const req = https.request(
      {
        protocol: "https:",
        hostname: BASE.hostname,
        port: BASE.port || 443,
        method,
        path: pathAndQuery,
        servername: svcHost,
        agent: insecure ? undefined : agent,
        rejectUnauthorized: !insecure,
        headers: { ...headers, host: svcHost, ...(body ? { "content-length": String(body.length) } : {}) },
        timeout: 30_000,
      },
      (res) => {
        const chunks: Buffer[] = [];
        res.on("data", (c: Buffer) => chunks.push(c));
        res.on("end", () =>
          resolve({
            status: res.statusCode || 0,
            headers: res.headers as Record<string, string | string[] | undefined>,
            body: Buffer.concat(chunks),
            durationMs: Math.round(performance.now() - started),
          }),
        );
      },
    );
    req.on("timeout", () => req.destroy(new Error(`timeout calling ${svcHost}${pathAndQuery}`)));
    req.on("error", reject);
    if (body) req.write(body);
    req.end();
  });
}

async function loadCA(): Promise<string> {
  const candidates = [process.env.CLOUDTYKE_CA_FILE, path.resolve(/*turbopackIgnore: true*/ process.cwd(), "../azure-emulator/certs/ca.pem")];
  for (const f of candidates) {
    if (f && fs.existsSync(/*turbopackIgnore: true*/ f)) return fs.readFileSync(/*turbopackIgnore: true*/ f, "utf8");
  }
  // Trust on first use: the emulator publishes its CA. Only the CA download skips verification.
  const r = await rawRequest(undefined, true, host("management"), "GET", "/_cloudtyke/ca.pem", adminHeaders());
  if (r.status !== 200) throw new Error(`could not fetch the emulator CA (HTTP ${r.status}); set CLOUDTYKE_CA_FILE`);
  console.warn("[cloudtyke-dashboard] trusting the CA from /_cloudtyke/ca.pem (set CLOUDTYKE_CA_FILE to pin it)");
  return r.body.toString("utf8");
}

function agent(): Promise<https.Agent> {
  if (!agentPromise) {
    agentPromise = loadCA()
      .then((ca) => new https.Agent({ ca, keepAlive: true }))
      .catch((e) => {
        agentPromise = null;
        throw e;
      });
  }
  return agentPromise;
}

function adminHeaders(): Record<string, string> {
  return ADMIN_TOKEN ? { "x-cloudtyke-admin-token": ADMIN_TOKEN } : {};
}

export interface EmuResponse {
  status: number;
  headers: Record<string, string | string[] | undefined>;
  body: Buffer;
  durationMs: number;
}

export async function request(
  svc: Service,
  method: string,
  pathAndQuery: string,
  opts: { headers?: Record<string, string>; body?: Buffer; admin?: boolean } = {},
): Promise<EmuResponse> {
  const a = await agent();
  const headers = { accept: "application/json", "user-agent": "cloudtyke-dashboard", ...(opts.admin ? adminHeaders() : {}), ...opts.headers };
  return rawRequest(a, false, host(svc), method, pathAndQuery, headers, opts.body);
}

// ---------------------------------------------------------------- tokens

const tokens = new Map<string, { token: string; exp: number }>();

export async function token(persona: PersonaId, resource: "management" | "graph"): Promise<string> {
  const key = `${persona}|${resource}`;
  const hit = tokens.get(key);
  if (hit && hit.exp - 60_000 > Date.now()) return hit.token;
  const p = PERSONAS[persona];
  const form = new URLSearchParams({
    grant_type: "client_credentials",
    client_id: p.clientId,
    client_secret: p.secret,
    scope: `https://${host(resource)}/.default`,
  });
  const r = await request("login", "POST", `/${TENANT}/oauth2/v2.0/token`, {
    headers: { "content-type": "application/x-www-form-urlencoded" },
    body: Buffer.from(form.toString()),
  });
  const j = JSON.parse(r.body.toString("utf8") || "{}");
  if (r.status !== 200 || !j.access_token) {
    throw new Error(`token request for ${p.name} failed (HTTP ${r.status}): ${j.error_description || j.error || "no token"}`);
  }
  tokens.set(key, { token: j.access_token, exp: Date.now() + (Number(j.expires_in) || 3600) * 1000 });
  return j.access_token;
}

export function clearTokens() {
  tokens.clear();
}

/** Decodes a JWT payload without verifying it (display only). */
export function claims(jwt: string): Record<string, unknown> {
  try {
    return JSON.parse(Buffer.from(jwt.split(".")[1], "base64url").toString("utf8"));
  } catch {
    return {};
  }
}
