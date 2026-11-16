/* Azure landing zones: the eight CAF design areas. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  var D = {
    a: { k: "Design area A · Environment", t: "Azure billing and Microsoft Entra tenant", rel: ["b", "c"],
      d: "Who pays, and which directory everything trusts. Decide this first: it is hard to change later.",
      s: [["It decides", ["Agreement type: Enterprise Agreement, Microsoft Customer Agreement or Cloud Solution Provider", "Billing accounts, billing profiles and invoice sections (or EA departments and accounts)", "How many Entra tenants, and which one is production", "Who may create subscriptions"]],
        ["Azure services", ["Microsoft Cost Management and Billing", "Microsoft Entra ID tenant", "Subscription creation (the Microsoft.Subscription alias API)"]]],
      ct: "One tenant (<code>cloudtyke.local</code>) and subscriptions from the seed. There is no billing model yet (FinOps is a planned phase)." },
    b: { k: "Design area B · Environment", t: "Identity and access management", rel: ["a", "e", "c"],
      d: "Who can do what, where. People and workloads both need an identity.",
      s: [["It decides", ["Roles for the platform team, app teams, network and security operations", "Just-in-time access for powerful roles", "Whether apps need Active Directory Domain Services in Azure", "Managed identities and workload identity federation for pipelines"]],
        ["Azure services", ["Microsoft Entra ID, Conditional Access, Privileged Identity Management", "Azure RBAC and custom roles", "Managed identities", "Microsoft Entra Domain Services or AD DS VMs in the identity subscription"]]],
      ct: "Azure RBAC with inheritance and custom roles, Entra users and groups, managed identities and GitHub OIDC all work. PIM and Conditional Access are out of scope." },
    c: { k: "Design area C · Environment", t: "Resource organization", rel: ["a", "g", "b"],
      d: "How to group and name things so that rules, access and costs follow the structure.",
      s: [["It decides", ["The management group tree (keep it flat: three or four levels)", "One subscription per workload and environment, as the unit of scale and isolation", "Naming and tagging standards (owner, cost centre, environment)"]],
        ["Azure services", ["Management groups", "Subscriptions and resource groups", "Tags, enforced or inherited with Azure Policy"]]],
      ct: "Management groups are read-only and come from the seed. Tag rules work: the Arena's <code>safety-code.tf</code> requires tags on resource groups." },
    d: { k: "Design area D · Network", t: "Network topology and connectivity", rel: ["e", "c"],
      d: "How landing zones talk to each other, to on-premises and to the internet, and how traffic is inspected.",
      s: [["It decides", ["Hub and spoke, or Azure Virtual WAN", "IP address plan with no overlaps", "Hybrid links: ExpressRoute or VPN", "Inspection and egress through a firewall", "Private endpoints and DNS for PaaS services"]],
        ["Azure services", ["Virtual networks, peering, Azure Virtual Network Manager", "Azure Firewall, Azure Bastion, Azure DDoS Protection", "VPN Gateway, ExpressRoute, Azure Virtual WAN", "Private Link, Azure Private DNS, DNS Private Resolver", "Application Gateway, Azure Front Door, Web Application Firewall"]]],
      ct: "VNets, subnets, peering, NSGs, route tables, private DNS zones and Virtual Network Manager are emulated. Firewall, Bastion, gateways and Virtual WAN are not." },
    e: { k: "Design area E · Compliance", t: "Security", rel: ["b", "f", "g"],
      d: "Protect, detect and respond, across every landing zone, from day one.",
      s: [["It decides", ["Security baseline and benchmark (Microsoft cloud security benchmark)", "Threat protection plans to turn on", "Where security logs go and who watches them", "Key and secret management, encryption"]],
        ["Azure services", ["Microsoft Defender for Cloud", "Microsoft Sentinel", "Azure Key Vault", "Azure DDoS Protection, Azure Firewall Premium"]]],
      ct: "Key Vault (secrets, RBAC, purge protection) is emulated. Defender for Cloud and Sentinel are not." },
    f: { k: "Design area F · Compliance", t: "Management", rel: ["e", "h"],
      d: "Keep the estate healthy: see it, patch it, back it up, recover it.",
      s: [["It decides", ["One central Log Analytics workspace, or a few", "Monitoring baseline and alerts for platform and apps", "Patching, backup and disaster recovery", "Operational compliance and drift checks"]],
        ["Azure services", ["Azure Monitor and Log Analytics", "Azure Monitor Agent and data collection rules", "Azure Update Manager", "Azure Backup and Azure Site Recovery", "Azure Monitor baseline alerts (AMBA)"]]],
      ct: "Log Analytics workspaces and the Activity Log are emulated. Update Manager, Backup and Site Recovery are not." },
    g: { k: "Design area G · Compliance", t: "Governance", rel: ["c", "e"],
      d: "Turn rules into code that Azure checks for you, and keep costs visible.",
      s: [["It decides", ["Which policies, at which scope, with which effect (Audit first, then Deny)", "Exemptions and who approves them", "Budgets and cost alerts", "Regulatory compliance initiatives"]],
        ["Azure services", ["Azure Policy (definitions, initiatives, assignments, exemptions)", "Microsoft Cost Management budgets", "Resource locks", "Azure Resource Graph for inventory"]]],
      ct: "Azure Policy is emulated in depth: definitions, initiatives, assignments, exemptions and compliance state. Built-in definitions are not seeded." },
    h: { k: "Design area H · Operations", t: "Platform automation and DevOps", rel: ["f", "c"],
      d: "Build and change the platform from code, through pipelines, with reviews.",
      s: [["It decides", ["Infrastructure as code tool: Bicep or Terraform", "Repositories, branching and approvals", "Pipeline identities with no secrets", "Subscription vending as code"]],
        ["Azure services", ["Azure landing zones accelerator (Bicep and Terraform modules)", "Azure Verified Modules", "GitHub Actions or Azure Pipelines", "Workload identity federation"]]],
      ct: "This is what CloudTyke is for: run Terraform against the emulator. Start with <code>azure-emulator/examples/terraform/landing-zone/</code>." }
  };
  var cols = [
    { x: 16, t: "Environment", c: "--az-plat", items: [["a", "A", "Billing and Entra tenant", "card"], ["b", "B", "Identity and access", "key"], ["c", "C", "Resource organization", "folder"]] },
    { x: 262, t: "Network", c: "--az-net", items: [["d", "D", "Network topology and connectivity", "net"]] },
    { x: 508, t: "Compliance", c: "--az-sec", items: [["e", "E", "Security", "shieldcheck"], ["f", "F", "Management", "chart"], ["g", "G", "Governance", "policy"]] },
    { x: 754, t: "Operations", c: "--az-dev", items: [["h", "H", "Platform automation and DevOps", "pipe"]] }
  ];
  var K = { "--az-plat": "plat", "--az-net": "net", "--az-sec": "sec", "--az-dev": "dev" };
  var s = "";
  cols.forEach(function (c) {
    s += A.zone(c.x, 16, 230, 340, c.c, c.t);
    c.items.forEach(function (it, i) {
      var y = 46 + i * 100;
      s += A.node({ id: it[0], x: c.x + 12, y: y, w: 206, h: 88, k: K[c.c], t: A.wrap(it[2], 20), s: "Design area " + it[1], ic: it[3] });
    });
    s += A.path([[c.x + 115, 356], [c.x + 115, 394]], "") + A.arrow(c.x + 115, 400, "down");
  });
  s += '<rect x="16" y="404" width="968" height="58" rx="14" class="band"/>' + A.icon("stadium", 44, 433, 22, "") +
    A.text(66, 429, "Your Azure landing zone", "t-hl") + A.text(66, 448, "Every design area feeds one implementation: the platform landing zone plus the application landing zones it serves.", "t-s", null, 14, 900);
  var svg = document.getElementById("areas");
  svg.innerHTML = s;
  A.bind(svg, D, document.getElementById("areas-panel"), { first: "d", dim: true });
})();
