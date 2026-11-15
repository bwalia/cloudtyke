import type { NextRequest } from "next/server";

import { DOMAIN, PERSONAS, TENANT, claims, isPersona, token, type PersonaId } from "@/lib/server/emulator";

/** GET: who the dashboard is acting as, with the decoded ARM token. POST {persona}: switch identity. */
export async function GET(req: NextRequest) {
  const c = req.cookies.get("ct-persona")?.value;
  const persona: PersonaId = isPersona(c) ? c : "owner";
  const p = PERSONAS[persona];
  let tokenClaims: Record<string, unknown> | null = null;
  let error: string | null = null;
  try {
    tokenClaims = claims(await token(persona, "management"));
  } catch (e) {
    error = e instanceof Error ? e.message : String(e);
  }
  return Response.json(
    {
      persona,
      domain: DOMAIN,
      tenantId: TENANT,
      personas: Object.values(PERSONAS).map(({ id, name, role, clientId }) => ({ id, name, role, clientId })),
      current: { id: p.id, name: p.name, role: p.role, clientId: p.clientId },
      claims: tokenClaims,
      error,
    },
    { headers: { "cache-control": "no-store" } },
  );
}

export async function POST(req: NextRequest) {
  const { persona } = await req.json().catch(() => ({}));
  if (!isPersona(persona)) return Response.json({ error: "persona must be owner or reader" }, { status: 400 });
  const res = Response.json({ persona });
  res.headers.append("set-cookie", `ct-persona=${persona}; Path=/; SameSite=Lax; HttpOnly; Max-Age=31536000`);
  return res;
}
