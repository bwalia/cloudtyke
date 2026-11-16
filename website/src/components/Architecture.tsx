// "How it works" diagram: your tools on the left, the CloudTyke container in
// the middle with the emulated services, and what backs them on the right.
const C = {
  ink: "#0f172a", muted: "#64748b", line: "#cbd5e1",
  tools: "#334155", entra: "#0078d4", arm: "#7c3aed", kv: "#b45309", blob: "#0e7490",
  aks: "#326ce5", policy: "#be123c", net: "#15803d",
};

function Box({ x, y, w, h, title, sub, color, fill = "#fff" }: {
  x: number; y: number; w: number; h: number; title: string; sub?: string; color: string; fill?: string;
}) {
  return (
    <g>
      <rect x={x} y={y} width={w} height={h} rx={10} fill={fill} stroke={color} strokeWidth={1.5} />
      <rect x={x} y={y} width={5} height={h} rx={2} fill={color} />
      <text x={x + 16} y={y + (sub ? 24 : h / 2 + 5)} fontSize={14} fontWeight={600} fill={color}>{title}</text>
      {sub && <text x={x + 16} y={y + 43} fontSize={12} fill={C.muted} fontFamily="var(--ct-stack-mono)">{sub}</text>}
    </g>
  );
}

export default function Architecture() {
  const services: [string, string, string][] = [
    ["Azure Resource Manager", "management.  · 88 resource types", C.arm],
    ["Entra ID", "login.  · tokens, OIDC, workload identity", C.entra],
    ["Microsoft Graph", "graph.  · users, groups, apps", C.entra],
    ["Azure RBAC + Policy", "every call authorised, deny/modify", C.policy],
    ["Key Vault", "*.vault.  · secrets data plane", C.kv],
    ["Storage", "*.blob / *.file  · Entra or keys", C.blob],
    ["AKS", "kubectl, Azure RBAC, OIDC issuer", C.aks],
  ];
  return (
    <svg viewBox="0 0 1100 520" role="img" aria-label="How CloudTyke works" style={{ fontFamily: "var(--ct-stack-body)" }}>
      <defs>
        <marker id="arw" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill={C.ink} />
        </marker>
        <marker id="arwm" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill={C.muted} />
        </marker>
      </defs>

      {/* left: tools */}
      <text x={20} y={34} fontSize={12} fontWeight={600} fill={C.muted} letterSpacing="0.1em">YOUR TOOLS</text>
      <Box x={20} y={56} w={220} h={58} title="OpenTofu / Terraform" sub="azurerm · azuread" color={C.tools} />
      <Box x={20} y={130} w={220} h={58} title="kubectl" sub="Entra token → AKS" color={C.tools} />
      <Box x={20} y={204} w={220} h={58} title="Your apps & SDKs" sub="DefaultAzureCredential" color={C.tools} />
      <Box x={20} y={278} w={220} h={58} title="CI pipelines" sub="GitHub Actions, OIDC" color={C.tools} />
      <Box x={20} y={352} w={220} h={58} title="Dashboard" sub="resources, RBAC, tokens" color={C.tools} />

      {/* arrows into the box */}
      {[85, 159, 233, 307, 381].map((y) => (
        <path key={y} d={`M240,${y} C300,${y} 300,262 352,262`} fill="none" stroke={C.ink} strokeWidth={1.6} markerEnd="url(#arw)" opacity={0.75} />
      ))}
      <text x={20} y={448} fontSize={12} fill={C.muted}>HTTPS to Azure-shaped hostnames,</text>
      <text x={20} y={466} fontSize={12} fill={C.muted}>Entra tokens, Azure&apos;s own errors</text>

      {/* middle: CloudTyke */}
      <rect x={356} y={20} width={430} height={482} rx={18} fill="#f8fafc" stroke={C.ink} strokeWidth={1.5} />
      <text x={378} y={52} fontSize={17} fontWeight={700} fill={C.ink} fontFamily="var(--ct-stack-display)">CloudTyke</text>
      <text x={378} y={72} fontSize={12} fill={C.muted} fontFamily="var(--ct-stack-mono)">one Docker container · Go · bbolt state</text>
      {services.map(([t, s, c], i) => (
        <Box key={t} x={376} y={88 + i * 58} w={390} h={50} title={t} sub={s} color={c} />
      ))}

      {/* right: what backs it */}
      <text x={850} y={34} fontSize={12} fontWeight={600} fill={C.muted} letterSpacing="0.1em">BEHIND THE SCENES</text>
      <Box x={850} y={56} w={230} h={58} title="State & activity log" sub="snapshots, reset, chaos" color={C.muted} />
      <Box x={850} y={150} w={230} h={58} title="CA + DNS sidecar" sub="*.cloudtyke.local" color={C.muted} />
      <Box x={850} y={262} w={230} h={58} title="Real Kubernetes" sub="one k3s per AKS cluster" color={C.aks} />
      <Box x={850} y={374} w={230} h={58} title="Observability" sub="OTel → Grafana, Jaeger" color={C.muted} />
      <path d="M766,85 L846,85" stroke={C.muted} strokeWidth={1.5} strokeDasharray="5 4" markerEnd="url(#arwm)" />
      <path d="M766,180 L846,180" stroke={C.muted} strokeWidth={1.5} strokeDasharray="5 4" markerEnd="url(#arwm)" />
      <path d="M766,461 C810,461 806,291 846,291" fill="none" stroke={C.aks} strokeWidth={1.5} strokeDasharray="5 4" markerEnd="url(#arwm)" />
      <path d="M766,403 C806,403 806,403 846,403" stroke={C.muted} strokeWidth={1.5} strokeDasharray="5 4" markerEnd="url(#arwm)" />
    </svg>
  );
}
