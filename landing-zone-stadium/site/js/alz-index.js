/* Azure landing zones: overview page. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  var more = function (f, t) { return 'Read more: <a href="' + f + '">' + t + "</a>"; };
  var D = {
    gov: { k: "Governance", t: "Management groups, Azure Policy and RBAC", d: "One tree of management groups holds the rules. Policy and role assignments set high up reach every subscription below. Nobody has to remember the rules: Azure checks them on every change.", s: [["Go deeper", more("hierarchy.html", "Management groups")]], ct: "Policy at management group scope is enforced. Management groups come from the seed and are read-only." },
    entra: { k: "Identity", t: "Microsoft Entra ID", d: "One tenant for people and workloads. Groups get roles. Pipelines and apps use managed identities or workload identity federation, so there are no secrets to leak.", s: [["Go deeper", more("hierarchy.html", "Identity and access")]], ct: "Users, groups, service principals, managed identities and federated credentials are emulated." },
    idsub: { k: "Platform landing zone", t: "Identity subscription", d: "Domain controllers or Microsoft Entra Domain Services for apps that still need Active Directory.", s: [["Go deeper", more("hierarchy.html", "Management groups")]] },
    mgmt: { k: "Platform landing zone", t: "Management subscription", d: "Central Log Analytics workspace, Azure Monitor, Microsoft Sentinel. Every landing zone sends its logs here by policy.", s: [["Go deeper", more("management-security.html", "Management and security")]], ct: "<code>sub-management</code> with a Log Analytics workspace." },
    hub: { k: "Platform landing zone", t: "Connectivity subscription (the hub)", d: "Azure Firewall, VPN and ExpressRoute gateways, Azure Bastion, DDoS protection and private DNS zones. All traffic between spokes, on-premises and the internet passes here.", s: [["Go deeper", more("network.html", "Network topology")]], ct: "<code>sub-connectivity</code>: hub VNet, subnets, private DNS zones. No firewall or gateway appliances." },
    onprem: { k: "Outside Azure", t: "On-premises data centre", d: "Joined to the hub by ExpressRoute (private circuit) or site-to-site VPN. Apps being migrated come from here.", s: [["Go deeper", more("migrate.html", "Scenario B: migrate")]] },
    users: { k: "Outside Azure", t: "Internet users", d: "Customers reach Online apps through Azure Front Door or Application Gateway with a Web Application Firewall. They never reach Corp apps.", s: [["Go deeper", more("network.html", "Network topology")]] },
    corp1: { k: "Application landing zone", t: "Corp: Payroll (production)", d: "An internal app. Its spoke VNet is peered to the hub. No public IPs. Databases use private endpoints.", s: [["Go deeper", more("services.html", "Services catalogue")]], ct: "Like the Arena's <code>sub-events</code>, built by <code>terraform/seats.tf</code>." },
    corp2: { k: "Application landing zone", t: "Corp: Payroll (non-production)", d: "Same baseline as production, separate subscription. Dev and test cannot touch production by accident.", s: [["Go deeper", more("vending.html", "Subscription vending")]] },
    online: { k: "Application landing zone", t: "Online: customer portal", d: "Internet-facing. The front end is published through a WAF. The back end stays private.", s: [["Go deeper", more("network.html", "Network topology")]] },
    sap: { k: "Application landing zone", t: "SAP: finance system", d: "SAP workloads with their own archetype and larger VM sizes, peered to the hub and close to on-premises.", s: [["Go deeper", more("hierarchy.html", "Management groups")]] },
    sandbox: { k: "Sandbox", t: "Sandbox", d: "Try new services here. No link to the hub or on-premises. A budget alert keeps spend honest.", s: [["Go deeper", more("modernise.html", "Sandbox to production")]], ct: "<code>ct-sandbox</code> with <code>sub-sandbox</code>." },
    vend: { k: "Platform automation", t: "Subscription vending", d: "A request turns into a ready landing zone in hours: subscription, management group, spoke network, peering, groups, policy and budget, all from code.", s: [["Go deeper", more("vending.html", "Subscription vending")]], ct: "Subscriptions come from the seed (no creation API). The steps after creation are practised with <code>for_each</code> in <code>terraform/seats.tf</code>." }
  };
  var s = "";
  s += A.bandBox(16, 16, 968, 58, "");
  s += A.node({ id: "gov", x: 28, y: 26, w: 600, h: 38, k: "gov", t: "Governance: management groups, Azure Policy, Azure RBAC", ic: "policy" });
  s += A.node({ id: "entra", x: 640, y: 26, w: 332, h: 38, k: "id", t: "Microsoft Entra ID", ic: "users" });
  /* platform */
  s += A.zone(16, 100, 330, 330, "--az-plat", "Platform landing zone");
  s += A.node({ id: "idsub", x: 32, y: 136, w: 298, h: 52, k: "plat", t: "Identity", s: "domain services", ic: "user" });
  s += A.node({ id: "mgmt", x: 32, y: 200, w: 298, h: 52, k: "plat", t: "Management", s: "logs, monitoring, Sentinel", ic: "chart" });
  s += A.node({ id: "hub", x: 32, y: 264, w: 298, h: 150, k: "net", t: "Connectivity hub", s: ["firewall, gateways, Bastion,", "DDoS protection, private DNS"], ic: "net" });
  /* app landing zones */
  s += A.zone(420, 100, 564, 330, "--az-lz", "Application landing zones");
  var apps = [["corp1", "Corp: Payroll", "production", 436, 136, "sub"], ["corp2", "Corp: Payroll", "non-production", 708, 136, "sub"], ["online", "Online: portal", "public via WAF", 436, 254, "globe"], ["sap", "SAP: finance", "SAP archetype", 708, 254, "sap"]];
  apps.forEach(function (a) { s += A.node({ id: a[0], x: a[3], y: a[4], w: 260, h: 60, k: "lz", t: a[1], s: a[2], ic: a[5] }); });
  /* peering: hub to each spoke along one bus (spokes are not peered to each other) */
  var bus = A.path([[330, 300], [380, 300], [380, 225], [838, 225]], "") +
    A.path([[566, 225], [566, 196]], "") + A.path([[838, 225], [838, 196]], "") + A.path([[566, 225], [566, 254]], "") + A.path([[838, 225], [838, 254]], "");
  s = s.replace('<g class="nd k-lz"', '<g id="peer">' + bus + '</g><g class="nd k-lz"');
  s += '<text x="702" y="219" class="t-xs" text-anchor="middle">hub-to-spoke peering</text>';
  /* outside */
  s += A.node({ id: "onprem", x: 32, y: 470, w: 230, h: 56, k: "onprem", t: "On-premises", s: "ExpressRoute or VPN", ic: "building" });
  s += A.path([[147, 470], [147, 414]], "dash") + A.arrow(147, 418, "up");
  s += A.node({ id: "users", x: 436, y: 470, w: 230, h: 56, k: "onprem", t: "Internet users", s: "Front Door or App Gateway + WAF", ic: "globe" });
  s += A.path([[551, 470], [551, 314]], "dash") + A.arrow(551, 318, "up");
  s += A.node({ id: "sandbox", x: 708, y: 360, w: 260, h: 52, k: "sand", t: "Sandbox", s: "isolated, budget alerts", ic: "flask" });
  s += A.node({ id: "vend", x: 708, y: 470, w: 260, h: 56, k: "dev", t: "Subscription vending", s: "new landing zones from code", ic: "vend" });
  s += A.path([[838, 470], [838, 436]], "dash") + A.arrow(838, 440, "up");
  var svg = document.getElementById("glance");
  svg.innerHTML = s;
  A.bind(svg, D, document.getElementById("glance-panel"), { first: "hub" });

  document.getElementById("alz-cards").innerHTML = A.pages.slice(1).map(function (p, i) {
    return '<li><a href="' + p.f + '">' + A.iconSvg(p.ic, 34) + '<span><span class="num">' + (i < 7 ? "Topic " + (i + 1) : i < 11 ? "Scenario" : "Why") + "</span><h3>" + A.esc(p.t) + "</h3><p>" + A.esc(p.d) + "</p></span></a></li>";
  }).join("");
})();
