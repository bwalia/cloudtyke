import Architecture from "@/components/Architecture";

const GH = "https://github.com/bwalia/cloudtyke";
const TREE = `${GH}/tree/main/azure-emulator`;

const features: { ic: string; color: string; title: string; body: React.ReactNode }[] = [
  { ic: "ARM", color: "#7c3aed", title: "Resource Manager", body: <>Subscriptions, resource groups, async operations, paging and ETags. <b>88 resource types</b> across 14 providers, with Azure&apos;s own validation rules.</> },
  { ic: "ID", color: "#0078d4", title: "Entra ID + Graph", body: <>Users, groups, apps and service principals. RS256 tokens with real claims, OIDC discovery, and <b>federated credentials</b> for workload identity.</> },
  { ic: "RB", color: "#be123c", title: "RBAC, enforced", body: <>Built-in roles with their real GUIDs, custom roles, inheritance from management group to resource. A missing role gets <code>AuthorizationFailed</code>.</> },
  { ic: "PO", color: "#b91c1c", title: "Azure Policy", body: <>Deny, Audit, Modify, Append and DenyAction run on every write. Initiatives, exemptions with expiry, compliance results.</> },
  { ic: "NW", color: "#15803d", title: "Networking", body: <>VNets, peering, NSGs, route tables, private DNS, Virtual Network Manager, plus <b>hop-by-hop packet traces</b> to debug a route.</> },
  { ic: "K8", color: "#326ce5", title: "AKS", body: <>Clusters and node pools, Entra-only sign-in, Azure RBAC for Kubernetes, workload identity. Optionally a <b>real k3s</b> per cluster.</> },
  { ic: "KV", color: "#b45309", title: "Key Vault", body: <>Vaults with soft delete and purge protection, and the secrets data plane with <code>ForbiddenByRbac</code> when a role is missing.</> },
  { ic: "ST", color: "#0e7490", title: "Storage", body: <>Accounts, containers, queues, tables, shares and lifecycle rules. Blob and Files data planes with Entra OAuth or shared keys.</> },
  { ic: "DB", color: "#9333ea", title: "Databases", body: <>PostgreSQL flexible servers with private access, and Cosmos DB (NoSQL and MongoDB APIs) as control plane.</> },
];

const examples = [
  { title: "hello-web", tag: "15 resources", path: "examples/terraform/hello-web", body: "One page on AKS that signs in with workload identity, reads Key Vault and counts visits in Blob. The page draws its own Entra ID sequence diagram." },
  { title: "Contoso Pay", tag: "live apps + OTel", path: "scenarios/contoso-pay", body: "A fictional payments company: seven teams, PCI guardrails, hub-and-spoke, private AKS. Live storefront, gateway and risk engine traced into Grafana and Jaeger." },
  { title: "Landing zone", tag: "100 resources", path: "examples/terraform/landing-zone", body: "Cloud Adoption Framework style across three subscriptions: hub and spokes, forced tunnelling, group-based RBAC, GitHub OIDC identities and AKS." },
  { title: "AKS platform", tag: "workload identity", path: "examples/terraform/aks-platform", body: "VNet, Log Analytics, ACR and an AKS cluster with Azure CNI overlay, Azure RBAC, Container insights and a federated service account." },
];

const modules: [string, string, string][] = [
  ["0", "Orientation", "tenant → subscription → resource"],
  ["1", "Identity", "tokens, apps, federation"],
  ["2", "Authorization", "roles, scopes, data actions"],
  ["3", "Infrastructure as Code", "state, drift, idempotency"],
  ["4", "Networking", "hub-and-spoke, packet traces"],
  ["5", "Governance", "Azure Policy guardrails"],
  ["6", "Secrets and data", "Key Vault, Storage, databases"],
  ["7", "AKS", "Entra-only clusters, workload identity"],
  ["8", "CI/CD", "pipelines, OIDC, policy checks"],
  ["9", "Observability", "traces, metrics, logs"],
  ["10", "Operations", "chaos, incidents, restore"],
  ["11", "Capstone", "build a platform from a brief"],
];

const hosts: [string, string, "open" | "gated"][] = [
  ["www.cloudtyke.dev", "this site", "open"],
  ["portal.cloudtyke.dev", "the CloudTyke dashboard: resources, identity, RBAC, activity, tokens", "gated"],
  ["management.cloudtyke.dev", "Azure Resource Manager (the control plane): point metadata_host here", "gated"],
  ["login.cloudtyke.dev · graph.cloudtyke.dev", "Entra ID tokens and Microsoft Graph", "gated"],
  ["*.vault.cloudtyke.dev · *.blob.storage.cloudtyke.dev", "Key Vault and Storage data planes", "gated"],
  ["hello.cloudtyke.dev", "hello-web, the end-to-end demo", "gated"],
  ["pay.cloudtyke.dev · merchant.cloudtyke.dev", "Contoso Pay storefront and merchant portal", "gated"],
  ["grafana.cloudtyke.dev · jaeger.cloudtyke.dev", "dashboards and traces for the live apps", "gated"],
];

export default function Home() {
  return (
    <>
      <nav className="nav">
        <div className="wrap">
          <a className="brand" href="#top" aria-label="CloudTyke home">
            <Logo />
            <b>CloudTyke</b>
          </a>
          <ul>
            <li><a href="#features">What it emulates</a></li>
            <li><a href="#how">How it works</a></li>
            <li><a href="#start">Quickstart</a></li>
            <li><a href="#examples">Examples</a></li>
            <li><a href="#academy">Academy</a></li>
            <li><a href="#hosted">Hosted</a></li>
          </ul>
          <a className="btn small ghost" href={GH}>GitHub</a>
        </div>
      </nav>

      <header className="hero" id="top">
        <div className="wrap grid">
          <div>
            <span className="pill"><i /> Azure APIs, emulated locally</span>
            <h1>A small cloud for <em>big mistakes.</em></h1>
            <p className="sub">
              CloudTyke emulates Azure&apos;s APIs in Docker: Resource Manager, Entra ID, RBAC, Policy, networking, Key Vault,
              Storage and AKS. Point Terraform or OpenTofu at it and build real-looking infrastructure with no subscription,
              no bill and no blast radius.
            </p>
            <div className="ctas">
              <a className="btn primary" href="#start">Start in five minutes →</a>
              <a className="btn ghost" href={GH}>View on GitHub</a>
            </div>
            <p className="note">Free and open source · runs on your laptop · not affiliated with Microsoft</p>
          </div>
          <div className="term" aria-label="Example terminal session">
            <div className="bar"><i /><i /><i /><span>toolbox · examples/terraform/hello-web</span></div>
            <pre>
              <span className="p">$</span> tofu apply{"\n"}
              <span className="c">  # azurerm_key_vault.hello will be created</span>{"\n"}
              <span className="c">  # azurerm_kubernetes_cluster.hello will be created</span>{"\n"}
              <span className="c">  ...</span>{"\n"}
              <span className="g">Apply complete! Resources: 15 added, 0 changed, 0 destroyed.</span>{"\n\n"}
              <span className="p">$</span> TF_VAR_allow_public_blob=true tofu apply{"\n"}
              <span className="r">Error:</span> creating Storage Account &quot;sthelloxb5ip&quot;:{"\n"}
              {"  "}<span className="y">RequestDisallowedByPolicy</span>: Resource &apos;sthelloxb5ip&apos;{"\n"}
              {"  "}was disallowed by policy. hello-web: public blob{"\n"}
              {"  "}access is not allowed.{"\n\n"}
              <span className="p">$</span> curl https://hello.cloudtyke.local{"\n"}
              <span className="b">Hello from CloudTyke!</span> <span className="c">(read from Key Vault</span>{"\n"}
              <span className="c">  with workload identity, no secrets in the pod)</span>
            </pre>
          </div>
        </div>
      </header>

      <div className="strip">
        <div className="wrap">
          <div><b>88</b><span>ARM resource types</span></div>
          <div><b>14</b><span>resource providers</span></div>
          <div><b>2</b><span>IaC tools tested in CI: OpenTofu and Terraform</span></div>
          <div><b>£0</b><span>subscription, credits or cleanup bills</span></div>
        </div>
      </div>

      <section className="section" id="features">
        <div className="wrap">
          <span className="eyebrow">What it emulates</span>
          <h2>The parts of Azure you actually get wrong.</h2>
          <p className="lede">
            Not mocks that say yes to everything: CloudTyke checks names, SKUs, CIDRs, roles and policies the way Azure does,
            and answers with the same status codes and error messages. Your tools and your muscle memory carry straight over.
          </p>
          <div className="cards">
            {features.map((f) => (
              <div className="card" key={f.title}>
                <div className="ic" style={{ background: f.color }}>{f.ic}</div>
                <h3>{f.title}</h3>
                <p>{f.body}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="section dark">
        <div className="wrap">
          <span className="eyebrow">Break it on purpose</span>
          <h2>Real errors are the curriculum.</h2>
          <p className="lede">
            Most cloud learning happens when something is refused. CloudTyke refuses exactly like Azure, so reading the error,
            finding the cause and fixing it becomes a safe, repeatable exercise.
          </p>
          <div className="errors">
            <div className="err">
              <span className="status">403</span><span className="code">AuthorizationFailed</span>
              <pre>The client &apos;…&apos; with object id &apos;…&apos; does not have authorization to perform action &apos;Microsoft.Resources/subscriptions/resourcegroups/write&apos; over scope &apos;/subscriptions/…&apos;</pre>
              <p>A Reader tried to write. Grant a role at the right scope, not Owner everywhere.</p>
            </div>
            <div className="err">
              <span className="status">403</span><span className="code">RequestDisallowedByPolicy</span>
              <pre>Resource &apos;sthello…&apos; was disallowed by policy. Policy identifiers: &apos;[&#123;&quot;policyAssignment&quot;:&#123;&quot;name&quot;:&quot;no-public-blob&quot;…</pre>
              <p>The guardrail did its job. Read the assignment, fix the resource, or request a time-boxed exemption.</p>
            </div>
            <div className="err">
              <span className="status">403</span><span className="code">ForbiddenByRbac</span>
              <pre>Caller is not authorized to perform action on resource. Action: &apos;Microsoft.KeyVault/vaults/secrets/getSecret/action&apos;</pre>
              <p>Owner of the vault, still no secret: control plane and data plane are different permissions.</p>
            </div>
          </div>
        </div>
      </section>

      <section className="section" id="how">
        <div className="wrap">
          <span className="eyebrow">How it works</span>
          <h2>One container that speaks Azure.</h2>
          <p className="lede">
            Your tools talk HTTPS to Azure-shaped hostnames (<code>management.</code>, <code>login.</code>,{" "}
            <code>*.vault.</code>) that a DNS sidecar points at CloudTyke. Every request is authenticated with an Entra
            token, authorised by RBAC, checked by Policy, then stored. AKS clusters can be backed by real Kubernetes.
          </p>
          <div className="diagram"><Architecture /></div>
        </div>
      </section>

      <section className="section alt" id="start">
        <div className="wrap">
          <span className="eyebrow">Quickstart</span>
          <h2>Five minutes, only Docker.</h2>
          <div className="two">
            <ol className="steps">
              <li><b>Start the emulator</b><span>Builds the emulator, the DNS sidecar and a local CA.</span></li>
              <li><b>Open the toolbox</b><span>A shell with OpenTofu, Terraform and kubectl that already trusts the CA and is signed in.</span></li>
              <li><b>Apply an example</b><span>Then plan again: &quot;No changes&quot;. The emulator is idempotent, like Azure.</span></li>
              <li><b>Look inside</b><span>The dashboard shows resources, role assignments, tokens and every API call your tools made.</span></li>
            </ol>
            <pre className="codeblock">
              <span className="c"># 1. start</span>{"\n"}
              git clone {GH}{"\n"}
              cd cloudtyke/azure-emulator && make up{"\n\n"}
              <span className="c"># 2. toolbox shell</span>{"\n"}
              make shell{"\n\n"}
              <span className="c"># 3. inside the toolbox</span>{"\n"}
              cd examples/terraform/quickstart{"\n"}
              tofu init && tofu apply{"\n"}
              tofu plan     <span className="c"># No changes.</span>{"\n\n"}
              <span className="c"># 4. dashboard on http://localhost:4310</span>{"\n"}
              make dashboard{"\n\n"}
              <span className="c"># the whole stack on AKS (k3s), with a web page</span>{"\n"}
              make hello-web
            </pre>
          </div>
        </div>
      </section>

      <section className="section" id="examples">
        <div className="wrap">
          <span className="eyebrow">Examples and scenarios</span>
          <h2>From one page to a whole company.</h2>
          <p className="lede">Every example runs apply → no-change plan → destroy in CI, with both OpenTofu and Terraform.</p>
          <div className="ex">
            {examples.map((e) => (
              <a key={e.title} href={`${TREE}/${e.path}`}>
                <div className="card">
                  <h3>{e.title} <span className="tag">{e.tag}</span></h3>
                  <p>{e.body}</p>
                </div>
              </a>
            ))}
          </div>
        </div>
      </section>

      <section className="section dark" id="academy">
        <div className="wrap">
          <span className="eyebrow">CloudTyke Academy</span>
          <h2>Become an Azure DevOps engineer, safely.</h2>
          <p className="lede">
            A hands-on course for interns and career changers. Each module explains why it matters and how it works, then
            has you build it, break it on purpose and check yourself with an automated grader. In preparation.
          </p>
          <div className="modules">
            {modules.map(([n, t, s]) => (
              <div className="mod" key={n}><small>Module {n}</small><b>{t}</b><span>{s}</span></div>
            ))}
          </div>
        </div>
      </section>

      <section className="section" id="hosted">
        <div className="wrap">
          <span className="eyebrow">Hosted sandbox</span>
          <h2>CloudTyke on cloudtyke.dev.</h2>
          <p className="lede">
            A shared CloudTyke runs on Kubernetes behind two edge locations with DNS failover. The same hostnames as the
            local emulator, with <code>cloudtyke.dev</code> instead of <code>cloudtyke.local</code> and real certificates,
            so tutorials only change the domain. Access is by invitation.
          </p>
          <div className="hosts-wrap"><table className="hosts">
            <thead><tr><th>Hostname</th><th>What</th><th>Access</th></tr></thead>
            <tbody>
              {hosts.map(([h, w, a]) => (
                <tr key={h}><td>{h}</td><td>{w}</td><td><span className={`badge ${a}`}>{a === "open" ? "public" : "invite"}</span></td></tr>
              ))}
            </tbody>
          </table></div>
        </div>
      </section>

      <footer>
        <div className="wrap">
          <div>
            <a className="brand" href="#top"><Logo /><b style={{ color: "#fff" }}>CloudTyke</b></a>
            <p className="disc">
              Not affiliated with or endorsed by Microsoft. Emulation is partial and for learning and testing only; it is never
              a substitute for real Azure. &quot;Azure&quot;, &quot;Entra&quot; and &quot;Microsoft Graph&quot; are used only
              to describe which APIs are emulated.
            </p>
          </div>
          <div>
            <a href={GH}>GitHub</a>
            <a href={`${TREE}/README.md`}>Docs</a>
            <a href={`${TREE}/docs/coverage.md`}>Coverage</a>
            <a href={`${TREE}/docs/known-gaps.md`}>Known gaps</a>
          </div>
        </div>
      </footer>
    </>
  );
}

function Logo() {
  return (
    <svg width="30" height="30" viewBox="0 0 32 32" aria-hidden="true">
      <defs>
        <linearGradient id="lg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#60a5fa" />
          <stop offset="1" stopColor="#2dd4bf" />
        </linearGradient>
      </defs>
      <path d="M9 24h15a6 6 0 0 0 .6-11.97A8 8 0 0 0 9.3 13.2 5.5 5.5 0 0 0 9 24z" fill="url(#lg)" />
      <circle cx="13.5" cy="18" r="1.6" fill="#0b1020" />
      <circle cx="19.5" cy="18" r="1.6" fill="#0b1020" />
      <path d="M14.5 21.2c1.2.9 2.8.9 4 0" stroke="#0b1020" strokeWidth="1.4" fill="none" strokeLinecap="round" />
    </svg>
  );
}
