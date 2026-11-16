/* Azure landing zones: the management group hierarchy (figure 1) and the policy check (figure 2). */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;

  /* ---------------- data ---------------- */
  var CT_MG = "Management groups are read-only in CloudTyke. They come from the seed (<code>azure-emulator/internal/seed/default.yaml</code>). To practise a group that is not there, add it with <code>parent:</code> in a copy of the seed and load it with <code>CLOUDTYKE_SEED_FILE</code>.";
  var POL = {
    root: [],
    contoso: [
      { n: "Allowed locations for resources and resource groups", e: "Deny" },
      { n: "Microsoft cloud security benchmark", e: "Audit" },
      { n: "Configure Microsoft Defender for Cloud plans", e: "DeployIfNotExists" },
      { n: "Send the Activity Log to the central Log Analytics workspace", e: "DeployIfNotExists" },
      { n: "Diagnostic settings to Log Analytics for supported resources", e: "DeployIfNotExists" },
      { n: "Deny classic (ASM) resources", e: "Deny" }
    ],
    platform: [{ n: "Key Vault guardrails (soft delete, purge protection)", e: "Audit" }],
    identity: [
      { n: "Deny public IP addresses", e: "Deny" },
      { n: "Deny management ports (RDP, SSH) from the internet", e: "Deny" },
      { n: "Subnets must have a network security group", e: "Deny" },
      { n: "Configure backup on virtual machines", e: "DeployIfNotExists" }
    ],
    management: [],
    connectivity: [{ n: "Virtual networks protected by Azure DDoS Protection", e: "Modify" }],
    lzs: [
      { n: "Deny management ports (RDP, SSH) from the internet", e: "Deny" },
      { n: "Subnets must have a network security group", e: "Deny" },
      { n: "Deny IP forwarding on network interfaces", e: "Deny" },
      { n: "Storage accounts: secure transfer (HTTPS) only", e: "Deny" },
      { n: "Virtual networks protected by Azure DDoS Protection", e: "Modify" },
      { n: "Kubernetes: no privileged containers", e: "Deny" },
      { n: "Configure backup on virtual machines", e: "DeployIfNotExists" }
    ],
    corp: [
      { n: "Deny public network access on PaaS services", e: "Deny" },
      { n: "Deny public IP addresses on network interfaces", e: "Deny" },
      { n: "Private endpoints register in the central private DNS zones", e: "DeployIfNotExists" },
      { n: "Deny hybrid networking (VPN, ExpressRoute, Virtual WAN) here", e: "Deny" }
    ],
    online: [{ n: "Web Application Firewall enabled on Application Gateway", e: "Audit" }],
    sap: [{ n: "Allowed virtual machine sizes (SAP-certified SKUs)", e: "Audit" }],
    decom: [
      { n: "Decommissioned guardrails: deny new resources", e: "Deny" },
      { n: "Decommissioned guardrails: auto-shutdown for VMs", e: "DeployIfNotExists" }
    ],
    sandbox: [
      { n: "Sandbox guardrails: deny peering to other networks", e: "Deny" },
      { n: "Sandbox guardrails: deny VPN, ExpressRoute and Virtual WAN", e: "Deny" }
    ]
  };
  var PARENT = {
    contoso: "root", platform: "contoso", lzs: "contoso", decom: "contoso", sandbox: "contoso",
    identity: "platform", management: "platform", connectivity: "platform", sap: "lzs", corp: "lzs", online: "lzs",
    "s-identity": "identity", "s-mgmt": "management", "s-conn": "connectivity", "s-sap": "sap", "s-a1": "corp", "s-a2": "corp",
    "s-online": "online", "s-decom": "decom", "s-sb1": "sandbox", "s-sb2": "sandbox"
  };
  var OWN_TXT = {
    plat: '<span class="pill plat">Platform team</span>',
    app: '<span class="pill app">App team</span>',
    shared: '<span class="pill shared">Shared</span>'
  };
  var D = {
    /* identity band */
    entra: { k: "Identity and access", t: "Microsoft Entra ID tenant", own: "plat",
      d: "One directory for the whole organisation. Every person, group and workload identity lives here, and every Azure subscription trusts this one tenant.",
      s: [["What it holds", ["Users (staff, guests)", "Groups (assign roles to groups, not people)", "Service principals and managed identities for apps and pipelines", "Conditional Access, MFA and sign-in logs"]],
        ["Landing zone rule", "Use one tenant for production. Keep separate tenants only for a real reason, such as a regulator or a test of tenant-wide settings."]],
      ct: "CloudTyke models one Entra tenant (<code>cloudtyke.local</code>) through a Microsoft Graph subset: users, groups, applications and service principals. Tokens are app-only (client credentials or a federated assertion)." },
    "id-users": { k: "Microsoft Entra ID", t: "Users", own: "plat", d: "People who sign in. Staff come from HR or on-premises AD (synchronised with Microsoft Entra Connect). Partners come in as guests.",
      s: [["Good practice", ["Give roles to groups, not to users", "Keep two cloud-only break-glass accounts, excluded from normal policies and watched closely"]]],
      ct: "Graph <code>users</code> are supported. Interactive sign-in and MFA are not; use service principals to sign in." },
    "id-groups": { k: "Microsoft Entra ID", t: "Groups", own: "plat", d: "Groups carry role assignments. A landing zone usually gets a small set: owners, contributors, readers.",
      s: [["Example", "<code>grp-lz-a1-contributors</code> gets Contributor on the Landing zone A1 subscription."]],
      ct: "Groups and members work. <code>landing-zone-stadium/terraform/seats.tf</code> creates one group per event and gives it a role. Dynamic membership is stored but has no effect." },
    "id-sp": { k: "Microsoft Entra ID", t: "Service principals and managed identities", own: "shared", d: "Identities for software. A managed identity has no secret to leak. Workload identity federation lets a GitHub or Azure DevOps pipeline sign in with no secret at all.",
      s: [["Use them for", ["Pipelines that deploy into a landing zone", "Apps that read Key Vault or storage", "Azure Policy remediation (DeployIfNotExists and Modify assignments use one)"]]],
      ct: "User-assigned managed identities and federated credentials work. <code>seats.tf</code> gives each event a crew identity that signs in from GitHub with OIDC." },
    pim: { k: "Identity and access", t: "Privileged Identity Management", own: "plat",
      d: "Nobody holds powerful roles all the time. People are <em>eligible</em> and activate a role for a few hours when they need it. Needs Microsoft Entra ID P2 or Microsoft Entra ID Governance.",
      s: [["It gives you", ["Approval workflow", "Notifications", "MFA on activation", "Access reviews", "Audit reports"]], ["Typical use", "Owner on a platform subscription, Global Administrator, User Access Administrator."]],
      ct: "Out of scope in CloudTyke (see <code>azure-emulator/docs/known-gaps.md</code>). Role assignments are permanent there." },
    "pim-approve": { k: "Privileged Identity Management", t: "Approval workflow", own: "plat", d: "Activating a sensitive role can need a yes from named approvers, with a written reason and a time limit.", ct: "Not emulated." },
    "pim-notify": { k: "Privileged Identity Management", t: "Notifications", own: "plat", d: "Emails go out when a role is assigned or activated, so the platform and security teams see privileged use as it happens.", ct: "Not emulated." },
    "pim-mfa": { k: "Privileged Identity Management", t: "MFA on activation", own: "plat", d: "A person must pass multifactor authentication before the role turns on.", ct: "Not emulated." },
    "pim-review": { k: "Privileged Identity Management", t: "Access reviews", own: "plat", d: "Owners or the people themselves confirm, every month or quarter, that access is still needed. Unanswered access can be removed.", ct: "Not emulated." },
    "pim-audit": { k: "Privileged Identity Management", t: "Audit reports", own: "plat", d: "A history of who activated what, when and why. Send it to the central Log Analytics workspace for long retention.", ct: "Not emulated." },
    rbac: { k: "Identity and access", t: "Role-based access control", own: "plat",
      d: "Azure RBAC grants a role to a principal at a scope: management group, subscription, resource group or resource. A role given high up applies to everything below it.",
      s: [["Landing zone roles (from the Cloud Adoption Framework)", ["App owners (DevOps, AppOps)", "Subscription owner (delegated)", "Network management (NetOps)", "Security operations (SecOps)"]]],
      ct: "Azure RBAC is enforced on every ARM call, with built-in roles and custom roles, inherited down management groups." },
    "role-app": { k: "Role-based access control", t: "App owner", own: "app", d: "The app team. Contributor-like rights on their own landing zone, without the right to change the network plumbing, RBAC or policy the platform set up.", s: [["Scope", "One landing zone subscription, or a resource group in it."]],
      ct: "Give a group Contributor on <code>sub-events</code> and try to delete the peering: build a custom role with <code>notActions</code> to stop it." },
    "role-sub": { k: "Role-based access control", t: "Subscription owner", own: "shared", d: "A custom role based on Owner. It can manage the subscription and grant access, but not alter the hub peering, route tables or policy assignments.", s: [["Why custom", "Plain Owner could remove the guardrails the platform team set."]], ct: "Custom role definitions are supported." },
    "role-custom": { k: "Role-based access control", t: "Other custom roles", own: "plat", d: "Network management (NetOps) for the hub and spokes, security operations (SecOps) for Defender and Sentinel, and readers for auditors.", ct: "Custom roles at management group scope are supported." },

    /* management groups */
    root: { k: "Management group", t: "Tenant root group", own: "plat",
      d: "Created by Azure for every tenant. Everything sits under it.",
      s: [["What lives here", "Nothing. Do not assign policy or roles here except for break-glass access."], ["Tip", "Change the default management group for new subscriptions (hierarchy settings) so they land in Sandbox, not here."]],
      ct: "Tenant root group <code>7c10d7e4-0000-4000-8000-000000000001</code>, created automatically. " + CT_MG },
    contoso: { k: "Management group (intermediate root)", t: "Contoso", own: "plat",
      d: "The top of your organisation. Rules that apply to everyone go here, so nothing escapes them.",
      s: [["What lives here", "No resources. It stores custom policy definitions, initiatives and custom roles, and the organisation-wide assignments."]],
      ct: "The CloudTyke seed has no intermediate root: <code>ct-platform</code>, <code>ct-landingzones</code> and <code>ct-sandbox</code> sit straight under the tenant root group. " + CT_MG },
    platform: { k: "Management group", t: "Platform", own: "plat",
      d: "Groups the subscriptions that run shared services for everyone: identity, management and connectivity. Only the platform team deploys here.",
      s: [["Note", "Newer versions of the Azure landing zones reference may add more children here, such as a Security group for security tooling. The idea stays the same."]],
      ct: "<code>ct-platform</code> holds <code>sub-connectivity</code> and <code>sub-management</code>. CloudTyke has no separate Identity, Management and Connectivity groups. " + CT_MG },
    identity: { k: "Management group (platform)", t: "Identity", own: "plat", d: "Holds the identity subscription: anything that extends identity to Azure, such as domain controllers.", ct: "Not in the CloudTyke seed." },
    management: { k: "Management group (platform)", t: "Management", own: "plat", d: "Holds the management subscription: central logging, monitoring and operations tooling.", s: [["Note", "Teams often put a delete lock on the central Log Analytics workspace. A lock is not a policy."]], ct: "Not in the seed as its own group: <code>sub-management</code> sits in <code>ct-platform</code>." },
    connectivity: { k: "Management group (platform)", t: "Connectivity", own: "plat", d: "Holds the connectivity subscription: the hub network that links every landing zone, on-premises and the internet.", ct: "Not in the seed as its own group: <code>sub-connectivity</code> sits in <code>ct-platform</code>." },
    lzs: { k: "Management group", t: "Landing zones", own: "shared",
      d: "The parent of every application landing zone. Guardrails that all workloads share go here. Child groups (archetypes) add stricter or looser rules for each kind of workload.",
      s: [["Children", ["Corp: internal apps, private only", "Online: internet-facing apps", "SAP: SAP estates (optional)"]]],
      ct: "<code>ct-landingzones</code>. The Arena seed (<code>landing-zone-stadium/seed.yaml</code>) adds <code>sub-events</code> here, and <code>terraform/safety-code.tf</code> assigns one policy initiative at this group." },
    sap: { k: "Management group (landing zone archetype)", t: "SAP", own: "shared", d: "Optional archetype for SAP estates: large VMs, SAP HANA, tight links to on-premises. Same guardrails as other landing zones, plus SAP-specific ones.", ct: "Not in the seed. Add it under <code>ct-landingzones</code> in a custom seed." },
    corp: { k: "Management group (landing zone archetype)", t: "Corp", own: "shared", d: "Internal apps that talk to the corporate network and each other through the hub. No direct exposure to the internet.", s: [["Typical apps", "HR, finance, intranet sites, internal APIs, data platforms."]], ct: "Not in the seed. The Arena's <code>sub-events</code> behaves like a Corp landing zone: <code>safety-code.tf</code> denies public IPs." },
    online: { k: "Management group (landing zone archetype)", t: "Online", own: "shared", d: "Apps that serve the public internet. Public endpoints are allowed, but go through Azure Front Door or Application Gateway with a Web Application Firewall.", s: [["Typical apps", "Websites, customer portals, public APIs."]], ct: "Not in the seed. Add it under <code>ct-landingzones</code> in a custom seed." },
    decom: { k: "Management group", t: "Decommissioned", own: "plat", d: "Where subscriptions go to be retired. Policy stops anything new being built and shuts VMs down. Access drops to read-only. After a cooling-off period the subscription is cancelled.", ct: "Not in the CloudTyke seed." },
    sandbox: { k: "Management group", t: "Sandbox", own: "shared", d: "A safe place to try things. Looser rules on services, but no path to the corporate network or production. Budgets keep spend in check.", s: [["Note", "A budget is set with Microsoft Cost Management. It is not an Azure Policy."]], ct: "<code>ct-sandbox</code> with <code>sub-sandbox</code>. Note: <code>azure-emulator/examples/terraform/landing-zone/</code> puts its spokes in <code>sub-sandbox</code> to keep the demo small." },

    /* subscriptions */
    "s-identity": { k: "Subscription (platform)", t: "Identity subscription", own: "plat", d: "Identity services that need to run in Azure.", s: [["What lives here", ["Active Directory Domain Services domain controllers (VMs), or Microsoft Entra Domain Services", "A spoke VNet peered to the hub", "Key Vault for identity secrets, Recovery Services vault"]]], ct: "No identity subscription in the seed. Entra groups and role assignments are built with Terraform instead (<code>examples/terraform/landing-zone/identity.tf</code>)." },
    "s-mgmt": { k: "Subscription (platform)", t: "Management subscription", own: "plat", d: "Central operations for the whole estate.", s: [["What lives here", ["Log Analytics workspace (all logs land here)", "Data collection rules for the Azure Monitor Agent", "Microsoft Sentinel on the workspace", "Action groups and alert rules", "Automation account (older reference versions)"]]], ct: "<code>sub-management</code> (<code>5b5c0000-…0002</code>). <code>examples/terraform/landing-zone/main.tf</code> and the Arena's <code>terraform/platform.tf</code> both build the Log Analytics workspace here." },
    "s-conn": { k: "Subscription (platform)", t: "Connectivity subscription", own: "plat", d: "The hub. Every network path between landing zones, on-premises and the internet goes through here.", s: [["What lives here", ["Hub VNet, or an Azure Virtual WAN hub", "Azure Firewall and a Firewall Policy", "VPN and ExpressRoute gateways", "Azure Bastion", "Azure DDoS Network Protection plan", "Private DNS zones (privatelink.*) and DNS Private Resolver"]]], ct: "<code>sub-connectivity</code> (<code>5b5c0000-…0001</code>): hub VNet and subnets, firewall public IP, private DNS zones and a shared registry. There is no Azure Firewall, Bastion or gateway resource in CloudTyke: the subnets and public IPs are there, the appliances are not. Virtual Network Manager hub-and-spoke configurations are supported." },
    "s-sap": { k: "Subscription (application landing zone)", t: "SAP landing zone", own: "app", d: "An SAP system (for example production S/4HANA) in its own subscription, peered to the hub.", ct: "Not in the seed." },
    "s-a1": { k: "Subscription (application landing zone)", t: "Landing zone A1", own: "app", d: "One workload or one team's environment. The platform hands it over ready: a spoke VNet peered to the hub, routes to the firewall, DNS links, RBAC groups, policy and diagnostics.", s: [["What lives here", ["The app: VMs, AKS, App Service, databases, storage", "Its own Key Vault", "Its spoke VNet and NSGs"]]], ct: "The Arena's <code>sub-events</code> (<code>5b5c0000-…0004</code>, from <code>landing-zone-stadium/seed.yaml</code>) is a landing zone like this. <code>terraform/seats.tf</code> builds the spoke, NSG, route table, peering, DNS link, group and Key Vault." },
    "s-a2": { k: "Subscription (application landing zone)", t: "Landing zone A2", own: "app", d: "A second Corp landing zone. Same baseline as A1, a different app or environment (for example A1 = production, A2 = non-production).", ct: "Add a second subscription under <code>ct-landingzones</code> in a custom seed and reuse <code>seats.tf</code> with <code>for_each</code>." },
    "s-online": { k: "Subscription (application landing zone)", t: "Online landing zone", own: "app", d: "A public-facing workload. Its front end sits behind Front Door or Application Gateway with WAF. Back ends still use private endpoints.", ct: "Not in the seed." },
    "s-decom": { k: "Subscriptions", t: "Decommissioned subscriptions", own: "plat", d: "Subscriptions on their way out. Read-only for the cooling-off period, then cancelled. Cancelled subscriptions keep data for a short time before Azure deletes it.", ct: "Not in the seed." },
    "s-sb1": { k: "Subscription (sandbox)", t: "Sandbox subscription 1", own: "app", d: "A developer or team plays here. Not connected to the hub or on-premises. A budget alert warns before spend runs away.", ct: "<code>sub-sandbox</code> (<code>5b5c0000-…0003</code>). The seed's <code>cloudtyke-reader</code> service principal has Reader on it." },
    "s-sb2": { k: "Subscription (sandbox)", t: "Sandbox subscription 2", own: "app", d: "Another isolated sandbox. One per person or team keeps experiments apart and easy to delete.", ct: "Add another sandbox subscription in a custom seed." }
  };
  Object.keys(POL).forEach(function (k) { if (D[k]) D[k].pol = POL[k]; });

  /* ---------------- figure 1: drawing ---------------- */
  var CX = function (i) { return 88 + 117 * i; };
  var MID = 614, BW = 109;
  var MG = {
    root: { x: MID, y: 290, w: 200, t: "Tenant root group", k: "root", ic: "folder" },
    contoso: { x: MID, y: 372, w: 170, t: "Contoso", k: "root", ic: "folder" },
    platform: { x: CX(1), y: 462, w: 150, t: "Platform", k: "plat", ic: "folder" },
    lzs: { x: MID, y: 462, w: 170, t: "Landing zones", k: "lz", ic: "folder" },
    decom: { x: CX(7), y: 462, w: BW, t: "Decommissioned", k: "decom", ic: null },
    sandbox: { x: CX(8), y: 462, w: BW, t: "Sandbox", k: "sand", ic: "folder" },
    identity: { x: CX(0), y: 556, w: BW, t: "Identity", k: "plat", ic: null },
    management: { x: CX(1), y: 556, w: BW, t: "Management", k: "plat", ic: null },
    connectivity: { x: CX(2), y: 556, w: BW, t: "Connectivity", k: "plat", ic: null },
    sap: { x: CX(3), y: 556, w: BW, t: "SAP", k: "lz", ic: null },
    corp: { x: MID, y: 556, w: 140, t: "Corp", k: "lz", ic: null },
    online: { x: CX(6), y: 556, w: BW, t: "Online", k: "lz", ic: null }
  };
  var SUBS = {
    "s-identity": { x: CX(0), y: 662, t: "Identity", ic: "user", k: "plat" },
    "s-mgmt": { x: CX(1), y: 662, t: "Management", ic: "chart", k: "plat" },
    "s-conn": { x: CX(2), y: 662, t: "Connectivity", ic: "net", k: "plat" },
    "s-sap": { x: CX(3), y: 662, t: ["SAP", "landing zone"], ic: "sap", k: "lz" },
    "s-a1": { x: CX(4), y: 662, t: ["Landing zone", "A1"], ic: "sub", k: "lz" },
    "s-a2": { x: CX(5), y: 662, t: ["Landing zone", "A2"], ic: "sub", k: "lz" },
    "s-online": { x: CX(6), y: 662, t: ["Online", "landing zone"], ic: "globe", k: "lz" },
    "s-decom": { x: CX(7), y: 662, t: ["Decom-", "missioned"], ic: "archive", k: "decom", stack: true },
    "s-sb1": { x: CX(8), y: 662, t: "Sandbox 1", ic: "flask", k: "sand" },
    "s-sb2": { x: CX(8), y: 752, t: "Sandbox 2", ic: "flask", k: "sand" }
  };
  var H_MG = 44, H_SUB = 76;

  function polBadge(id, x, y) {
    var n = (POL[id] || []).length;
    if (!n) return "";
    return '<g class="badge-pol"><rect x="' + (x - 22) + '" y="' + (y - 10) + '" width="44" height="20" rx="10"/>' +
      '<g transform="translate(' + (x - 17) + " " + (y - 7) + ') scale(.58)" fill="none" stroke="#1d1300" stroke-width="2.6">' + A.icons.policy + "</g>" +
      '<text x="' + (x + 7) + '" y="' + (y + 4) + '" text-anchor="middle">' + n + "</text></g>";
  }
  function drawHier() {
    var s = "";
    /* identity band */
    s += A.bandBox(16, 16, 1088, 214, "Identity and access");
    var panels = [
      { id: "entra", x: 32, w: 340, t: "Microsoft Entra ID tenant", ic: "users", chips: [["id-users", "Users", "user"], ["id-groups", "Groups", "users"], ["id-sp", "Service principals, managed identities", "robot"]], cols: 1 },
      { id: "pim", x: 388, w: 344, t: "Privileged Identity Management", ic: "lock", chips: [["pim-approve", "Approval workflow", "approve"], ["pim-notify", "Notifications", "bell"], ["pim-mfa", "MFA", "mfa"], ["pim-review", "Access reviews", "review"], ["pim-audit", "Audit reports", "audit"]], cols: 2 },
      { id: "rbac", x: 748, w: 340, t: "Role-based access control", ic: "key", chips: [["role-app", "App owner", "user"], ["role-sub", "Subscription owner", "sub"], ["role-custom", "Other custom roles", "users"]], cols: 1 }
    ];
    panels.forEach(function (p) {
      s += '<rect x="' + p.x + '" y="42" width="' + p.w + '" height="176" rx="14" fill="none" stroke="var(--az-id)" stroke-width="1.5" stroke-dasharray="4 4"/>';
      s += A.node({ id: p.id, x: p.x + 8, y: 50, w: p.w - 16, h: 40, k: "id", t: p.t, ic: p.ic });
      var cw = p.cols === 2 ? (p.w - 16 - 8) / 2 : p.w - 16;
      p.chips.forEach(function (c, i) {
        var col = p.cols === 2 ? i % 2 : 0, row = p.cols === 2 ? Math.floor(i / 2) : i;
        s += A.node({ id: c[0], x: p.x + 8 + col * (cw + 8), y: 100 + row * 38, w: cw, h: 32, k: "chip", t: c[1], ic: c[2], isz: 16, tc: "t-s t-ink", rx: 8 });
      });
    });
    /* management group band */
    s += A.bandBox(16, 250, 1088, 660, "Management group and subscription organization");
    s += '<text x="1088" y="272" text-anchor="end" class="t-xs">Azure RBAC and Azure Policy assigned at a group apply to everything under it</text>';
    /* identity reaches every scope */
    s += A.path([[918, 218], [918, 240], [918, 252]], "dash thin", ' data-e="rbac"') + A.arrow(918, 258, "down");
    /* edges */
    var edges = "";
    function e(from, to, my) {
      var a = MG[from], b = MG[to] || SUBS[to];
      var y2 = b.y - (b.stack ? 6 : 0);
      edges += A.elbow(a.x, a.y + H_MG, b.x, y2, "", my, ' data-from="' + from + '" data-to="' + to + '"');
    }
    e("root", "contoso"); ["platform", "lzs", "decom", "sandbox"].forEach(function (c) { e("contoso", c, 438); });
    ["identity", "management", "connectivity"].forEach(function (c) { e("platform", c, 530); });
    ["sap", "corp", "online"].forEach(function (c) { e("lzs", c, 530); });
    e("identity", "s-identity"); e("management", "s-mgmt"); e("connectivity", "s-conn"); e("sap", "s-sap");
    e("corp", "s-a1", 632); e("corp", "s-a2", 632); e("online", "s-online"); e("decom", "s-decom"); e("sandbox", "s-sb1");
    edges += '<path class="edge" data-from="sandbox" data-to="s-sb2" d="M' + (CX(8) + 40) + " " + (462 + H_MG) + "V522H1094V" + (752 + H_SUB / 2) + "H" + (CX(8) + BW / 2) + '"/>';
    s += '<g id="h-edges">' + edges + "</g>";
    s += '<g id="h-flow" class="hidden-layer">' + edges.replace(/class="edge /g, 'class="edge flow ') + "</g>";
    /* management groups */
    Object.keys(MG).forEach(function (id) {
      var m = MG[id];
      s += A.node({ id: id, x: m.x - m.w / 2, y: m.y, w: m.w, h: H_MG, k: m.k, t: m.t, ic: m.ic, center: !m.ic, label: m.t + " management group",
        attrs: ' data-own="' + D[id].own + '"', extra: polBadge(id, m.x + m.w / 2 - 6, m.y) });
    });
    /* subscriptions */
    s += '<text x="30" y="644" class="t-band" style="paint-order:stroke;stroke:var(--surface2);stroke-width:6px">Subscriptions</text>';
    Object.keys(SUBS).forEach(function (id) {
      var m = SUBS[id];
      s += A.node({ id: id, x: m.x - BW / 2, y: m.y, w: BW, h: H_SUB, k: m.k, t: m.t, s: "subscription", icTop: true, ic: m.ic, stack: m.stack, tc: "t-s t-ink", sc: "t-xs", tlh: 14,
        label: D[id].t, attrs: ' data-own="' + D[id].own + '"' });
    });
    /* note under the corp group */
    s += A.text(CX(3) - 50, 766, ["SAP and Online hold their own", "landing zone subscriptions too."], "t-xs", null, 14);
    /* legend */
    var lx = 30, ly = 856;
    s += '<text x="' + lx + '" y="' + (ly + 4) + '" class="t-xs" style="font-weight:600">Key</text>';
    [["k-root", "Organisation"], ["k-plat", "Platform"], ["k-lz", "Landing zones"], ["k-sand", "Sandbox"], ["k-decom", "Decommissioned"], ["k-id", "Identity and access"]].forEach(function (k, i) {
      var x = lx + 40 + i * 132;
      s += '<g class="' + k[0] + '"><rect class="box" x="' + x + '" y="' + (ly - 8) + '" width="18" height="16" rx="4"/></g><text x="' + (x + 24) + '" y="' + (ly + 4) + '" class="t-xs">' + k[1] + "</text>";
    });
    s += '<g class="badge-pol"><rect x="830" y="' + (ly + 22) + '" width="22" height="14" rx="7"/></g><text class="t-xs badge-pol" x="858" y="' + (ly + 33) + '" style="font:11px var(--font);fill:var(--muted)">policy assignments at this scope</text>';
    s += '<g class="own-legend">' + [["plat", "Platform team"], ["app", "App team"], ["shared", "Shared: platform sets guardrails, app teams build"]].map(function (k, i) {
      var x = 30 + i * 170;
      return '<g class="nd" data-own="' + k[0] + '" style="cursor:default"><rect class="box" x="' + x + '" y="' + (ly - 52) + '" width="18" height="16" rx="4"/></g><text x="' + (x + 24) + '" y="' + (ly - 40) + '" class="t-xs">' + k[1] + "</text>";
    }).join("") + "</g>";
    s += '<text x="30" y="' + (ly + 33) + '" class="t-xs">Shapes and colours are our own. Group names follow the Azure landing zones reference.</text>';
    return s;
  }

  var svg = document.getElementById("hier");
  svg.innerHTML = drawHier();
  var panel = document.getElementById("hier-panel");

  function chain(id) { var out = []; while (id) { out.unshift(id); id = PARENT[id]; } if (out[0] !== "root" && D[id]) out.unshift("root"); return out; }
  function effPill(e) { var c = e === "Deny" ? "deny" : e === "Audit" ? "audit" : "dine"; return '<span class="pill ' + c + '">' + e + "</span>"; }
  function render(id, d) {
    var h = A.panelHtml({ k: d.k, t: d.t, d: d.d, s: d.s });
    h += '<div class="pb"><h4>Who owns it</h4><p>' + OWN_TXT[d.own] + " " + (d.own === "plat" ? "The platform team builds and runs it." : d.own === "app" ? "The app team builds inside it. The platform team owns the guardrails around it." : "The platform team owns the group, its policy and network links. App teams own what they build in it.") + "</p></div>";
    if (PARENT[id] || id === "root") {
      var c = chain(id), rows = "";
      c.forEach(function (sc) {
        (POL[sc] || []).forEach(function (p) {
          rows += '<li class="' + (sc === id ? "here" : "") + '"><span class="from">' + (sc === id ? "Assigned here" : "Inherited from " + D[sc].t) + "</span>" + A.esc(p.n) + " " + effPill(p.e) + "</li>";
        });
      });
      h += '<div class="pb"><h4>Policy in effect here (' + (rows.match(/<li/g) || []).length + ")</h4>" + (rows ? '<ul class="inh">' + rows + "</ul>" : "<p>None. Keep this scope clean.</p>") + "</div>";
      if (id.indexOf("s-") === 0) h += '<p class="small muted">A subscription can add its own assignments too. Typical examples are named, not exhaustive.</p>';
    }
    if (d.ct) h += '<div class="ct"><h4>Practise in CloudTyke</h4><p>' + d.ct + "</p></div>";
    return h;
  }
  function onSelect(id) {
    svg.querySelectorAll("#h-edges .edge").forEach(function (p) { p.classList.remove("hot"); });
    var c = chain(id);
    for (var i = 1; i < c.length; i++) {
      svg.querySelectorAll('#h-edges [data-from="' + c[i - 1] + '"][data-to="' + c[i] + '"]').forEach(function (p) { p.classList.add("hot"); });
    }
    A.mark(svg, PARENT[id] ? c.slice(0, -1) : [], "rel");
  }
  var api = A.bind(svg, D, panel, { render: render, onSelect: onSelect, first: "s-a1" });

  A.seg(document.getElementById("hier-view"), "View:", [["structure", "Structure"], ["policy", "Policy inheritance"], ["owner", "Ownership"]], function (v) {
    svg.classList.toggle("show-pol", v === "policy");
    svg.classList.toggle("show-own", v === "owner");
    document.getElementById("h-flow").classList.toggle("hidden-layer", v !== "policy");
    var note = document.getElementById("hier-note");
    note.textContent = v === "policy" ? "Orange badges count the assignments at each group. The moving dashes show policy flowing down: a subscription gets everything above it. Pick a box to see its full list." :
      v === "owner" ? "Tags show who owns each box. The platform team owns the structure and the platform subscriptions. App teams own what they build in landing zones and sandboxes." :
        "Pick any box. The orange path shows the chain of groups above it.";
  });

  /* ---------------- figure 2: one request, every assignment above it ---------------- */
  var REQ = [
    { id: "pip", label: "Public IP in Landing zone A1", target: "s-a1", what: "Create a public IP address", where: "Landing zone A1 (Corp), UK South",
      hit: { corp: "Deny public IP addresses on network interfaces" }, note: "Corp landing zones are private. Publish through the hub, or move the app to an Online landing zone." },
    { id: "region", label: "Storage in East US", target: "s-a2", what: "Create a storage account", where: "Landing zone A2 (Corp), East US",
      hit: { contoso: "Allowed locations for resources and resource groups" }, note: "Contoso only allows UK South and UK West. The rule sits at the top, so no subscription escapes it." },
    { id: "web", label: "Public web app in Online", target: "s-online", what: "Create an Application Gateway with WAF and a public IP", where: "Online landing zone, UK South",
      ok: true, pass: { online: "WAF enabled on Application Gateway (Audit: compliant)" }, note: "Allowed. Online landing zones may publish to the internet, behind a Web Application Firewall." },
    { id: "vm-decom", label: "New VM in Decommissioned", target: "s-decom", what: "Create a virtual machine", where: "A decommissioned subscription",
      hit: { decom: "Decommissioned guardrails: deny new resources" }, note: "Nothing new gets built in a subscription that is being retired." },
    { id: "peer", label: "Peer sandbox to the hub", target: "s-sb1", what: "Create a VNet peering to the hub", where: "Sandbox subscription 1",
      hit: { sandbox: "Sandbox guardrails: deny peering to other networks" }, note: "Sandboxes stay isolated from the corporate network. Ask for a landing zone to connect." },
    { id: "sqlpub", label: "Public SQL in Corp", target: "s-a1", what: "Create Azure SQL with public network access on", where: "Landing zone A1 (Corp), UK West",
      hit: { corp: "Deny public network access on PaaS services" }, note: "Turn public network access off and add a private endpoint. The DNS record is created for you by a DeployIfNotExists policy." }
  ];
  var ev = document.getElementById("eval");
  var evOut = document.getElementById("eval-out");
  var timer = null;
  function drawEval(r) {
    var c = chain(r.target), rowH = 52, top = 86, s = "", W = 760;
    s += '<rect x="16" y="14" width="' + (W - 32) + '" height="52" rx="12" class="band"/>';
    s += A.icon("arrow", 40, 40, 20);
    s += A.text(62, 36, "Request: " + r.what, "t-h", null, 16, W - 100) + A.text(62, 54, "Scope: " + r.where, "t-s", null, 14, W - 100);
    c.forEach(function (sc, i) {
      var y = top + i * rowH, pol = POL[sc] || [], d = D[sc];
      var hit = r.hit && r.hit[sc], pass = r.pass && r.pass[sc];
      s += '<g class="ev-row" data-i="' + i + '" style="opacity:.15">';
      s += '<rect x="' + (16 + i * 14) + '" y="' + y + '" width="' + (W - 32 - i * 14) + '" height="' + (rowH - 8) + '" rx="10" class="band" style="' + (hit ? "stroke:var(--az-no);stroke-width:2.5" : pass ? "stroke:var(--az-ok);stroke-width:2" : "") + '"/>';
      s += A.icon(sc.indexOf("s-") === 0 ? "sub" : "folder", 38 + i * 14, y + 22, 18);
      s += A.text(58 + i * 14, y + 19, d.t, "t-h", null, 16, 180) + A.text(58 + i * 14, y + 35, pol.length + " assignment" + (pol.length === 1 ? "" : "s"), "t-xs");
      var msg = hit ? "Deny: " + hit : pass ? pass : pol.length ? "No rule matches this request" : "Nothing assigned here";
      s += A.text(W - 60, y + 27, msg, hit ? "t-s" : "t-xs", "end", 14, W - 330 - i * 14);
      s += hit ? '<g style="color:var(--az-no)">' + A.icon("x", W - 38, y + 22, 20) + "</g>" : '<g style="color:var(--az-ok)">' + A.icon("check", W - 38, y + 22, 20) + "</g>";
      s += "</g>";
    });
    var ry = top + c.length * rowH + 6, ok = !r.hit;
    s += '<g class="ev-row" data-i="' + c.length + '" style="opacity:.15"><rect x="16" y="' + ry + '" width="' + (W - 32) + '" height="50" rx="12" style="fill:color-mix(in srgb, var(' + (ok ? "--az-ok" : "--az-no") + ') 14%, var(--surface));stroke:var(' + (ok ? "--az-ok" : "--az-no") + ');stroke-width:2"/>' +
      A.text(36, ry + 22, ok ? "Result: created" : "Result: RequestDisallowedByPolicy", "t-hl") + A.text(36, ry + 40, ok ? "Every assignment in scope allows it." : "One Deny anywhere above is enough. Nothing is built.", "t-s") + "</g>";
    ev.setAttribute("viewBox", "0 0 " + W + " " + (ry + 64));
    ev.innerHTML = s;
    A.fit(ev);
    var rows = ev.querySelectorAll(".ev-row"), k = 0;
    if (timer) clearInterval(timer);
    function show() { if (k < rows.length) { rows[k].style.opacity = 1; k++; } else { clearInterval(timer); timer = null; } }
    if (A.reduced) { rows.forEach(function (n) { n.style.opacity = 1; }); }
    else { show(); timer = setInterval(show, 380); }
    evOut.innerHTML = '<p class="note ' + (ok ? "good" : "bad") + '"><strong>' + (ok ? "Allowed." : "Blocked.") + "</strong> " + r.note + "</p>" +
      '<p class="small muted">Azure gathers every assignment from the target scope and all the groups above it, then checks them together. The order on the drawing is only for reading: it is not a sequence, and any Deny wins.</p>';
  }
  var cur = REQ[0];
  A.seg(document.getElementById("eval-pick"), "Try a request:", REQ.map(function (r) { return [r.id, r.label]; }), function (v) {
    REQ.forEach(function (r) { if (r.id === v) cur = r; });
    drawEval(cur);
  });
  document.getElementById("eval-replay").addEventListener("click", function () { drawEval(cur); });
  window.ALZ_TEST = { data: D, api: api };
})();
