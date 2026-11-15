import type { NextRequest } from "next/server";

import { PERSONAS, isPersona, token, type PersonaId } from "@/lib/server/emulator";

/** The current identity's access token for ARM or Graph, decoded, for the Tokens page. Local emulator tokens only. */
export async function GET(req: NextRequest) {
  const c = req.cookies.get("ct-persona")?.value;
  const persona: PersonaId = isPersona(c) ? c : "owner";
  const resource = req.nextUrl.searchParams.get("resource") === "graph" ? "graph" : "management";
  try {
    const jwt = await token(persona, resource);
    const [h, p, s] = jwt.split(".");
    const dec = (x: string) => JSON.parse(Buffer.from(x, "base64url").toString("utf8"));
    return Response.json({ persona: PERSONAS[persona].name, resource, jwt, header: dec(h), claims: dec(p), signatureBytes: Buffer.from(s, "base64url").length }, { headers: { "cache-control": "no-store" } });
  } catch (e) {
    return Response.json({ error: { code: "TokenError", message: e instanceof Error ? e.message : String(e) } }, { status: 502 });
  }
}
