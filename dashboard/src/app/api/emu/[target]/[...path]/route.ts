import type { NextRequest } from "next/server";

import { DOMAIN, PERSONAS, isPersona, request, token, type PersonaId } from "@/lib/server/emulator";

/**
 * Backend-for-frontend proxy: the browser never talks to the emulator or sees a
 * token. /api/emu/{target}/{path} forwards to:
 *
 *   arm    https://management.<domain>/{path}  with a token for the current persona
 *   graph  https://graph.<domain>/{path}       with a Graph token for the persona
 *   admin  https://management.<domain>/_cloudtyke/{path}  (admin API)
 *   login  https://login.<domain>/{path}       GET only (OIDC discovery, JWKS)
 */

const HOP = new Set(["connection", "keep-alive", "transfer-encoding", "content-length", "content-encoding"]);

async function handle(req: NextRequest, ctx: RouteContext<"/api/emu/[target]/[...path]">) {
  const { target, path } = await ctx.params;
  const cookie = req.cookies.get("ct-persona")?.value;
  const persona: PersonaId = isPersona(cookie) ? cookie : "owner";
  const search = req.nextUrl.search;
  const rest = "/" + path.map(encodeURIComponent).join("/");

  let svc: "management" | "graph" | "login";
  let p: string;
  const headers: Record<string, string> = {};
  let admin = false;
  switch (target) {
    case "arm":
      svc = "management";
      p = rest + search;
      break;
    case "graph":
      svc = "graph";
      p = rest + search;
      break;
    case "admin":
      svc = "management";
      p = "/_cloudtyke" + rest + search;
      admin = true;
      break;
    case "login":
      if (req.method !== "GET") return Response.json({ error: { code: "MethodNotAllowed", message: "login is read-only here" } }, { status: 405 });
      svc = "login";
      p = rest + search;
      break;
    default:
      return Response.json({ error: { code: "NotFound", message: `unknown target ${target}` } }, { status: 404 });
  }

  try {
    if (target === "arm" || target === "graph") {
      headers.authorization = "Bearer " + (await token(persona, svc as "management" | "graph"));
    }
    const ct = req.headers.get("content-type");
    if (ct) headers["content-type"] = ct;
    const body = ["GET", "HEAD"].includes(req.method) ? undefined : Buffer.from(await req.arrayBuffer());
    const r = await request(svc, req.method, p, { headers, body: body && body.length ? body : undefined, admin });

    const out = new Headers();
    for (const [k, v] of Object.entries(r.headers)) {
      if (v === undefined || HOP.has(k)) continue;
      out.set(k, Array.isArray(v) ? v.join(", ") : v);
    }
    // Async operations point at the emulator's own host; hand the browser a path it can poll through this proxy.
    for (const h of ["location", "azure-asyncoperation"]) {
      const v = out.get(h);
      if (v) {
        try {
          const u = new URL(v);
          if (u.hostname === `management.${DOMAIN}`) out.set(`x-ct-${h}`, `/api/emu/arm${u.pathname}${u.search}`);
        } catch {
          /* relative or malformed: leave as is */
        }
        out.delete(h);
      }
    }
    out.set("x-ct-duration-ms", String(r.durationMs));
    out.set("x-ct-persona", PERSONAS[persona].name);
    out.set("cache-control", "no-store");
    return new Response(req.method === "HEAD" || r.status === 204 ? null : new Uint8Array(r.body), { status: r.status, headers: out });
  } catch (e) {
    const message = e instanceof Error ? e.message : String(e);
    return Response.json(
      { error: { code: "EmulatorUnreachable", message: `${message}. Is the emulator running? (make -C azure-emulator up)` } },
      { status: 502, headers: { "cache-control": "no-store" } },
    );
  }
}

export { handle as GET, handle as POST, handle as PUT, handle as PATCH, handle as DELETE, handle as HEAD };
