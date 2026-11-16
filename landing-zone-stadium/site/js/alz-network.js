/* Azure landing zones: network topology. Hub and spoke vs Azure Virtual WAN, packet paths, decision guide. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  var NOFW = "There is no Azure Firewall, gateway or Virtual WAN resource in CloudTyke, so there is no data path. You can build the shape: subnets named <code>AzureFirewallSubnet</code>, <code>GatewaySubnet</code> and <code>AzureBastionSubnet</code>, a public IP and route tables that point at the firewall's private IP.";
  var D = {
    inet: { k: "Outside Azure", t: "Internet", d: "Customers, partners and admins on the public internet." },
    onprem: { k: "Outside Azure", t: "On-premises network", d: "The data centre and offices. Joined to Azure by ExpressRoute (a private circuit through a connectivity provider) or site-to-site VPN over the internet. Many use both: ExpressRoute first, VPN as backup." },
    gw: { k: "Connectivity hub", t: "VPN and ExpressRoute gateways", d: "Terminate the links from on-premises and exchange routes with BGP. They live in the hub, in a subnet named GatewaySubnet. In Virtual WAN they are part of the managed hub.", ct: NOFW },
    fw: { k: "Connectivity hub", t: "Azure Firewall", d: "Inspects traffic between spokes, to and from on-premises, and out to the internet. Rules live in a Firewall Policy that the platform team manages as code. Premium adds TLS inspection and IDPS.", s: [["How traffic gets here", "Each spoke subnet has a route table (UDR) that sends 0.0.0.0/0 and other spokes' ranges to the firewall's private IP. In Virtual WAN, routing intent does this for you."]], ct: NOFW + " The Arena's <code>terraform/platform.tf</code> builds the firewall subnet and public IP." },
    bastion: { k: "Connectivity hub", t: "Azure Bastion", d: "Browser or native-client RDP and SSH to VMs over TLS, with no public IP on any VM. In hub and spoke it can sit in the hub and reach peered spokes. In Virtual WAN it goes in a spoke.", ct: "No Bastion resource in CloudTyke. <code>AzureBastionSubnet</code> and its public IP can be created." },
    ddos: { k: "Connectivity hub", t: "Azure DDoS Network Protection", d: "One DDoS protection plan in the connectivity subscription covers public IPs in the hub and the spokes. Policy can link every VNet to it.", ct: "Not emulated." },
    resolver: { k: "Connectivity hub", t: "DNS Private Resolver", d: "Lets on-premises resolve Azure private names, and Azure resolve on-premises names, without DNS VMs. Inbound and outbound endpoints sit in dedicated subnets.", ct: "Not emulated. Private DNS zones and VNet links are." },
    dns: { k: "Connectivity subscription", t: "Private DNS zones (privatelink.*)", d: "One zone per PaaS service type, such as privatelink.database.windows.net, created once and linked to the hub (or the resolver's VNet). A DeployIfNotExists policy adds a record when an app team creates a private endpoint.", ct: "Private DNS zones, A records and VNet links are emulated. <code>examples/terraform/landing-zone/main.tf</code> creates the zones and links." },
    corp: { k: "Corp landing zone (A1)", t: "Corp app", d: "An internal payroll app on VMs or AKS. No public IP. All its traffic leaves through the hub firewall.", ct: "AKS with UDR egress is emulated: <code>examples/terraform/landing-zone/aks.tf</code>." },
    pe: { k: "Corp landing zone (A1)", t: "Azure SQL through a private endpoint", d: "The database has public network access turned off. A private endpoint gives it a private IP inside the spoke. The name resolves to that IP through the private DNS zone.", ct: "Azure SQL is not emulated. PostgreSQL flexible server with private access (delegated subnet and private DNS zone) is." },
    udr1: { k: "Corp landing zone (A1)", t: "Route table: 0.0.0.0/0 to the firewall", d: "A user-defined route on every subnet in the spoke. It forces internet-bound and spoke-to-spoke traffic through the firewall. Policy or RBAC stops app teams from changing it.", ct: "Route tables with VirtualAppliance next hops are emulated: <code>terraform/seats.tf</code> builds one." },
    appgw: { k: "Online landing zone", t: "Application Gateway with WAF", d: "The public entry point for the online app, with a Web Application Firewall. Azure Front Door is the global alternative, with WAF at the edge.", ct: "Not emulated." },
    web: { k: "Online landing zone", t: "Online web app", d: "The customer portal. Reachable only through the WAF. Its outbound calls still go through the hub firewall." },
    udr2: { k: "Online landing zone", t: "Route table: 0.0.0.0/0 to the firewall", d: "Outbound traffic is still inspected. Inbound web traffic arrives through the WAF, so return traffic must go back the same way: design the routes so it does." },
    sandbox: { k: "Sandbox", t: "Sandbox VNet (not connected)", d: "Not peered to the hub, and a policy denies peering. It reaches the internet directly, and can never reach on-premises or production." },
    rt: { k: "Virtual WAN hub", t: "Routing intent", d: "One setting that sends internet traffic, private traffic or both through the secured hub's firewall. No route tables to maintain in each spoke." },
    router: { k: "Virtual WAN hub", t: "Hub router (managed)", d: "Microsoft runs the hub router. Spokes, branches and other hubs connect to it. Hub-to-hub transit across regions is built in." },
    conns: { k: "Virtual WAN hub", t: "VNet connections", d: "Spokes connect to the Virtual WAN hub with a VNet connection instead of peering you manage. Branch sites connect with VPN or SD-WAN partners." },
    shared: { k: "Shared services spoke", t: "Shared services VNet", d: "With Virtual WAN you cannot put your own VMs, Bastion or DNS resolver inside the hub. They go in a shared services spoke." },
    vbastion: { k: "Shared services spoke", t: "Azure Bastion (in a spoke)", d: "Bastion in a spoke can reach VMs in other spokes through the hub with the Standard SKU and IP-based connection.", ct: "Not emulated." },
    vresolver: { k: "Shared services spoke", t: "DNS Private Resolver (in a spoke)", d: "Same job as in the hub. The private DNS zones are linked to this VNet, and spokes use it as their DNS server.", ct: "Not emulated." }
  };
  var L = {
    hub: {
      inet: [470, 16, 220, 56, "Internet", "users and admins", "globe", "onprem"],
      onprem: [16, 160, 170, 96, "On-premises", "data centre, offices", "building", "onprem"],
      gw: [256, 180, 180, 56, ["VPN and", "ExpressRoute GW"], null, "gw", "net"],
      fw: [456, 180, 190, 56, "Azure Firewall", "Firewall Policy", "fw", "net"],
      bastion: [668, 124, 180, 46, "Azure Bastion", null, "door", "net"],
      ddos: [668, 180, 180, 46, "DDoS Protection", null, "shield", "net"],
      resolver: [668, 236, 180, 46, ["DNS Private", "Resolver"], null, "dns", "net"],
      dns: [880, 150, 204, 140, ["Private DNS zones"], ["privatelink.database.", "windows.net and others"], "dns", "plat"],
      corp: [256, 476, 120, 56, "Corp app", "VMs or AKS", "vm", "lz"],
      pe: [386, 476, 124, 56, "Azure SQL", "private endpoint", "db", "lz"],
      udr1: [256, 546, 254, 34, "UDR 0.0.0.0/0 to firewall", null, "arrow", "chip"],
      appgw: [576, 476, 124, 56, "App Gateway", "with WAF", "waf", "lz"],
      web: [710, 476, 120, 56, "Web app", "customer portal", "globe", "lz"],
      udr2: [576, 546, 254, 34, "UDR 0.0.0.0/0 to firewall", null, "arrow", "chip"],
      sandbox: [890, 476, 186, 56, "Sandbox VNet", "not peered", "flask", "sand"]
    }
  };
  L.vwan = JSON.parse(JSON.stringify(L.hub));
  delete L.vwan.bastion; delete L.vwan.ddos; delete L.vwan.resolver;
  L.vwan.gw[4] = ["VPN, ExpressRoute", "and SD-WAN"];
  L.vwan.fw[4] = ["Azure Firewall"]; L.vwan.fw[5] = "secured hub";
  L.vwan.rt = [668, 124, 180, 46, "Routing intent", null, "arrow", "net"];
  L.vwan.router = [668, 180, 180, 46, "Hub router", null, "net", "net"];
  L.vwan.conns = [668, 236, 180, 46, "VNet connections", null, "gw", "net"];
  L.vwan.vbastion = [28, 476, 176, 46, "Azure Bastion", null, "door", "net"];
  L.vwan.vresolver = [28, 532, 176, 46, ["DNS Private", "Resolver"], null, "dns", "net"];
  L.vwan.shared = null;
  L.vwan.udr1 = [256, 546, 254, 34, "No UDRs: routing intent", null, "check", "chip"];
  L.vwan.udr2 = [576, 546, 254, 34, "No UDRs: routing intent", null, "check", "chip"];
  D.udr1v = D.udr1; D.udr2v = D.udr2;

  var PATHS = {
    online: { t: "Internet user to the Online app", nodes: ["inet", "appgw", "web"],
      hub: [[690, 44], [866, 44], [866, 420], [638, 420], [638, 476], [638, 532], [700, 504], [710, 504]],
      steps: ["A customer opens the portal on the internet.", "The request reaches the public IP of Application Gateway in the Online spoke.", "The WAF checks it (SQL injection, bad bots, OWASP rules).", "Clean requests go to the web app's private IP. The app never had a public IP of its own."] },
    onprem: { t: "Corp app to on-premises", nodes: ["corp", "udr1", "fw", "gw", "onprem"],
      hub: [[316, 476], [316, 420], [551, 420], [551, 236], [551, 208], [436, 208], [256, 208], [186, 208]],
      steps: ["The payroll app calls an on-premises HR database.", "The spoke route table sends the packet to the hub firewall (peering carries it).", "Azure Firewall checks a network rule: payroll subnet to HR database on port 1433, allowed.", "The gateway sends it over ExpressRoute (or VPN) to the data centre."] },
    egress: { t: "Corp app out to the internet", nodes: ["corp", "udr1", "fw", "inet"],
      hub: [[316, 476], [316, 420], [551, 420], [551, 236], [551, 180], [551, 72]],
      steps: ["The app calls a public API to check bank details.", "0.0.0.0/0 points at the firewall, so the packet goes to the hub.", "An application rule allows only the named domain. Everything else is denied and logged.", "The firewall sends it out from its own public IP, so the API sees one known address."] },
    pe: { t: "Corp app to Azure SQL (private endpoint)", nodes: ["corp", "resolver", "dns", "pe"],
      hub: [[316, 476], [316, 420], [980, 420], [980, 290], [980, 420], [448, 420], [448, 476]],
      steps: ["The app looks up contoso-payroll.database.windows.net.", "DNS answers with a CNAME to the privatelink name. The private DNS zone (linked to the hub resolver) returns 10.1.2.5.", "The app connects to 10.1.2.5: a private endpoint in its own spoke. Traffic never leaves the private network.", "Public network access on the server is off, so the public name is useless from outside."] },
    bastion: { t: "Admin to a VM through Bastion", nodes: ["inet", "bastion", "vbastion", "corp"],
      hub: [[580, 72], [580, 98], [758, 98], [758, 124], [758, 170], [658, 170], [658, 420], [316, 420], [316, 476]],
      steps: ["An admin signs in to the Azure portal with MFA.", "Azure Bastion opens an RDP or SSH session over TLS (port 443).", "Bastion reaches the VM's private IP across the peering.", "The VM has no public IP and no open management port to the internet."] }
  };
  PATHS.online.vwan = PATHS.online.hub;
  PATHS.onprem.vwan = PATHS.onprem.hub;
  PATHS.egress.vwan = PATHS.egress.hub;
  PATHS.pe.vwan = [[316, 476], [316, 420], [230, 420], [230, 555], [204, 555], [230, 555], [230, 420], [448, 420], [448, 476]];
  PATHS.bastion.vwan = [[470, 44], [116, 44], [116, 476], [116, 522], [222, 499], [222, 420], [316, 420], [316, 476]];

  var svg = document.getElementById("net"), panel = document.getElementById("net-panel"), mode = "hub", api, curPath = null;
  function draw() {
    var lay = L[mode], s = "";
    s += A.zone(240, 100, 620, 200, "--az-net", mode === "hub" ? "Hub VNet · connectivity subscription" : "Azure Virtual WAN hub · Microsoft-managed");
    s += A.zone(240, 446, 286, 146, "--az-lz", "Corp spoke · Landing zone A1", "t-xs");
    s += A.zone(560, 446, 286, 146, "--az-lz", "Online spoke", "t-xs");
    s += A.zone(880, 446, 204, 100, "--az-sand", "Sandbox", "t-xs");
    if (mode === "vwan") s += A.zone(16, 446, 200, 146, "--az-net", "Shared services spoke", "t-xs");
    /* links */
    var link = mode === "hub" ? "peering" : "VNet connection";
    s += A.path([[383, 300], [383, 446]], "thin") + A.path([[703, 300], [703, 446]], "thin");
    s += A.text(390, 372, link, "t-xs") + A.text(710, 372, link, "t-xs");
    if (mode === "vwan") s += A.path([[116, 446], [116, 380], [300, 380], [300, 300]], "thin") + A.text(124, 372, link, "t-xs");
    s += A.path([[186, 208], [256, 208]], "") + A.text(213, 200, "ER/VPN", "t-xs", "middle");
    if (mode === "hub") s += A.path([[848, 259], [880, 259]], "dash");
    s += '<g style="color:var(--az-no)">' + A.icon("x", 983, 432, 16) + "</g>" + A.text(983, 562, "no peering: denied by policy", "t-xs", "middle");
    Object.keys(lay).forEach(function (id) {
      var n = lay[id];
      if (!n) return;
      s += A.node({ id: id, x: n[0], y: n[1], w: n[2], h: n[3], t: n[4], s: n[5] || undefined, ic: n[6], k: n[7], tc: n[3] < 40 ? "t-s t-ink" : "t-h", isz: n[3] < 40 ? 16 : 20 });
    });
    s += '<g id="pkt-layer"></g>';
    svg.innerHTML = s;
  }
  function render(id, d) { return A.panelHtml(d) + (curPath ? "" : ""); }
  draw();
  api = A.bind(svg, D, panel, { first: "fw", render: render });
  function runPath(key) {
    var p = PATHS[key], pts = p[mode];
    curPath = key;
    var nodes = p.nodes.filter(function (n) { return L[mode][n]; });
    A.mark(svg, [], "on");
    svg.classList.add("dimming");
    var out = document.getElementById("path-out");
    out.innerHTML = "<h3>" + A.esc(p.t) + '</h3><ol class="steps-list">' + p.steps.map(function (st) { return "<li>" + st + "</li>"; }).join("") + "</ol>";
    var lis = out.querySelectorAll("li"), n = 0;
    lis.forEach(function (li) { li.classList.add("past"); });
    A.travel(svg.querySelector("#pkt-layer"), pts, {
      onPoint: function (i) {
        var k = Math.min(lis.length - 1, Math.floor((i / (pts.length - 1)) * lis.length));
        lis.forEach(function (li, j) { li.classList.toggle("now", j === k); li.classList.toggle("past", j > k); });
        var upto = Math.min(nodes.length, Math.ceil(((i + 1) / pts.length) * nodes.length));
        A.mark(svg, nodes.slice(0, upto), "on");
        n = i;
      },
      done: function () { A.mark(svg, nodes, "on"); lis.forEach(function (li) { li.classList.remove("past"); }); }
    });
  }
  function clearPath() { curPath = null; svg.classList.remove("dimming"); A.mark(svg, [], "on"); var l = svg.querySelector("#pkt-layer"); if (l) l.innerHTML = ""; document.getElementById("path-out").innerHTML = '<p class="muted small">Pick a path to watch a packet.</p>'; }
  A.seg(document.getElementById("net-mode"), "Topology:", [["hub", "Hub and spoke"], ["vwan", "Azure Virtual WAN"]], function (v) {
    mode = v; draw();
    if (api) { if (!L[mode][api.current]) api.current = "fw"; api.refresh(); }
    document.getElementById("net-mode-note").innerHTML = v === "hub" ?
      "<strong>Hub and spoke:</strong> you build and run the hub VNet. Full control of routing, firewall and shared services in the hub. You maintain route tables and peerings." :
      "<strong>Azure Virtual WAN:</strong> Microsoft runs the hub router. Spokes and branches connect to it. Routing intent replaces per-spoke route tables. Your own VMs, Bastion and DNS resolvers go in a shared services spoke.";
    if (curPath) runPath(curPath);
  });
  var pb = document.getElementById("path-btns");
  pb.innerHTML = Object.keys(PATHS).map(function (k) { return '<button type="button" class="btn" data-path="' + k + '">' + A.esc(PATHS[k].t) + "</button>"; }).join("") + '<button type="button" class="btn ghost" data-path="">Clear</button>';
  pb.addEventListener("click", function (e) {
    var b = e.target.closest("[data-path]");
    if (!b) return;
    pb.querySelectorAll("[data-path]").forEach(function (x) { x.setAttribute("aria-pressed", x === b && b.getAttribute("data-path") ? "true" : "false"); });
    if (b.getAttribute("data-path")) runPath(b.getAttribute("data-path")); else clearPath();
  });
  clearPath();

  /* ---------------- decision guide ---------------- */
  var Q = [
    { q: "Do you need connectivity across many Azure regions, with traffic between them?", y: 2, n: -1 },
    { q: "Do you connect many branch offices, perhaps with SD-WAN?", y: 2, n: -1 },
    { q: "Do you need full control of hub routing, or a third-party network appliance of your choice in the hub?", y: -2, n: 1 },
    { q: "Do you want to run shared services (Bastion, DNS resolver, jump hosts) inside the hub itself?", y: -2, n: 0 },
    { q: "Would you rather Microsoft ran the hub routing, so your team has less to operate?", y: 2, n: -1 }
  ];
  var ans = Q.map(function () { return null; });
  var qbox = document.getElementById("guide-q"), gsvg = document.getElementById("guide");
  qbox.innerHTML = Q.map(function (q, i) {
    return '<fieldset class="kitset"><legend>' + (i + 1) + ". " + A.esc(q.q) + '</legend><div class="row">' +
      '<label><input type="radio" name="gq' + i + '" value="y"> Yes</label><label><input type="radio" name="gq' + i + '" value="n"> No</label><label><input type="radio" name="gq' + i + '" value="" checked> Not sure</label></div></fieldset>';
  }).join("");
  qbox.addEventListener("change", function (e) {
    var m = e.target.name.match(/gq(\d+)/);
    if (!m) return;
    ans[+m[1]] = e.target.value || null;
    gauge();
  });
  function gauge() {
    var score = 0, why = [];
    ans.forEach(function (a, i) { if (a) { var v = a === "y" ? Q[i].y : Q[i].n; score += v; if (v) why.push([v, Q[i].q]); } });
    var clamp = Math.max(-6, Math.min(6, score)), ang = (clamp / 6) * 80;
    var s = '<path d="M60 200 A170 170 0 0 1 400 200" fill="none" stroke="var(--az-plat)" stroke-width="22" opacity=".25"/>' +
      '<path d="M230 30 A170 170 0 0 1 400 200" fill="none" stroke="var(--az-net)" stroke-width="22" opacity=".35"/>' +
      '<path d="M60 200 A170 170 0 0 1 230 30" fill="none" stroke="var(--az-lz)" stroke-width="22" opacity=".35"/>' +
      A.text(70, 232, "Hub and spoke", "t-h") + A.text(390, 232, "Virtual WAN", "t-h", "end") + A.text(230, 232, "either works", "t-xs", "middle") +
      '<g style="transition:transform .5s" transform="rotate(' + ang.toFixed(1) + ' 230 200)"><line x1="230" y1="200" x2="230" y2="55" stroke="var(--ink)" stroke-width="5" stroke-linecap="round"/></g><circle cx="230" cy="200" r="11" fill="var(--ink)"/>';
    gsvg.innerHTML = s;
    var verdict = score <= -2 ? "Lean to <strong>hub and spoke</strong>." : score >= 2 ? "Lean to <strong>Azure Virtual WAN</strong>." : "No strong pull either way. Hub and spoke is the simpler start; Virtual WAN pays off as regions and branches grow.";
    document.getElementById("guide-out").innerHTML = '<p class="verdict">' + verdict + "</p>" + (why.length ? "<ul>" + why.map(function (w) { return "<li>" + (w[0] > 0 ? "Points to Virtual WAN: " : "Points to hub and spoke: ") + A.esc(w[1].replace(/^Do you |^Would you /, "").replace(/\?$/, "")) + "</li>"; }).join("") + "</ul>" : '<p class="small muted">Answer the questions to move the needle.</p>');
  }
  gauge();
})();
