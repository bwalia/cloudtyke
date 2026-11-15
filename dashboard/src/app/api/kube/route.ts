import type { NextRequest } from "next/server";

import { isPersona, request, requestHost, token, type PersonaId } from "@/lib/server/emulator";

const API = "2025-02-01";

/**
 * GET /api/kube?cluster=<AKS resource id>&path=/api/v1/nodes
 * Gets the cluster's admin kubeconfig through ARM (as the current identity, so
 * RBAC applies), then calls the cluster's Kubernetes API with it, the same
 * way kubectl would.
 */
export async function GET(req: NextRequest) {
  const cluster = req.nextUrl.searchParams.get("cluster") || "";
  const path = req.nextUrl.searchParams.get("path") || "/api/v1/nodes";
  if (!/^\/subscriptions\/[^/]+\/resourceGroups\/[^/]+\/providers\/Microsoft\.ContainerService\/managedClusters\/[^/]+$/i.test(cluster) || !/^\/(api|apis|version)(\/[\w./-]*)?$/.test(path)) {
    return Response.json({ error: { code: "BadRequest", message: "cluster must be an AKS resource id and path a Kubernetes API path" } }, { status: 400 });
  }
  const c = req.cookies.get("ct-persona")?.value;
  const persona: PersonaId = isPersona(c) ? c : "owner";
  try {
    const arm = await token(persona, "management");
    const cred = await request("management", "POST", `${cluster}/listClusterAdminCredential?api-version=${API}`, { headers: { authorization: `Bearer ${arm}` } });
    const body = JSON.parse(cred.body.toString("utf8") || "{}");
    if (cred.status !== 200) return Response.json(body, { status: cred.status });
    const kubeconfig = Buffer.from(body.kubeconfigs?.[0]?.value ?? "", "base64").toString("utf8");
    const server = /server:\s*https:\/\/([^:\s/]+)/.exec(kubeconfig)?.[1];
    const tok = /token:\s*(\S+)/.exec(kubeconfig)?.[1];
    if (!server || !tok) return Response.json({ error: { code: "NoCredential", message: "the cluster returned no token credential" } }, { status: 502 });
    const r = await requestHost(server, "GET", path, { authorization: `Bearer ${tok}` });
    return new Response(new Uint8Array(r.body), { status: r.status, headers: { "content-type": "application/json", "cache-control": "no-store", "x-ct-kube-server": server } });
  } catch (e) {
    return Response.json({ error: { code: "KubeUnreachable", message: e instanceof Error ? e.message : String(e) } }, { status: 502 });
  }
}
