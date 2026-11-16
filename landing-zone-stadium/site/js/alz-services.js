/* Azure landing zones: where common services land, and which platform services each one leans on. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  var P = {
    dns: ["Private DNS", "privatelink zones", "dns"],
    fw: ["Azure Firewall", "egress and east-west", "fw"],
    idp: ["Entra ID", "managed identities", "key"],
    mon: ["Monitoring", "Log Analytics, Monitor", "chart"],
    pol: ["Azure Policy", "guardrails", "policy"],
    def: ["Defender for Cloud", "threat protection", "shieldcheck"]
  };
  var PD = {
    dns: { k: "Platform service", t: "Private DNS zones", d: "Every PaaS service with a private endpoint needs a record in the matching privatelink zone. The platform owns the zones; a policy writes the records." },
    fw: { k: "Platform service", t: "Azure Firewall", d: "Outbound traffic from VMs, AKS nodes and VNet-integrated apps is routed through the hub firewall. AKS needs specific outbound rules (FQDN tags help)." },
    idp: { k: "Platform service", t: "Microsoft Entra ID and managed identities", d: "Apps reach databases, storage and Key Vault with managed identities and Entra auth, not keys or passwords." },
    mon: { k: "Platform service", t: "Monitoring", d: "Diagnostic settings, sent by policy, put every service's logs and metrics in the central workspace." },
    pol: { k: "Platform service", t: "Azure Policy", d: "Inherited guardrails shape how each service may be built: no public access in Corp, TLS versions, allowed SKUs, required tags." },
    def: { k: "Platform service", t: "Microsoft Defender for Cloud", d: "Defender plans cover servers, containers, SQL, storage, Key Vault, APIs and AI workloads, turned on by policy." }
  };
  var CT = { yes: "Emulated in CloudTyke.", cp: "Emulated in CloudTyke (control plane only).", no: "Not emulated in CloudTyke (returns 501).", later: "Not yet: VMs are a planned CloudTyke phase." };
  var G = [
    { t: "Compute", c: "lz", items: [
      ["vm", "Virtual machines", "vm", ["fw", "idp", "mon", "pol", "def"], "later", "Lift-and-shift and anything that needs a full OS. Backup and Update Manager by policy. No public IP; admin through Bastion."],
      ["vmss", "VM Scale Sets", "server", ["fw", "mon", "pol", "def"], "later", "Many identical VMs that scale out and in. Behind a load balancer or Application Gateway."],
      ["aks", "AKS", "wheel", ["fw", "dns", "idp", "mon", "pol", "def"], "yes", "Managed Kubernetes. In a landing zone: private cluster or API server VNet integration, egress through the firewall (outbound type userDefinedRouting), Entra ID with Azure RBAC, workload identity, Azure Policy add-on."],
      ["app", "App Service", "globe", ["dns", "idp", "mon", "pol", "def"], "no", "Web apps and APIs. VNet integration for outbound, private endpoint for inbound in Corp."],
      ["func", "Azure Functions", "fn", ["dns", "idp", "mon", "pol"], "no", "Event-driven code. Flex Consumption or Premium plans support VNet integration."],
      ["aca", "Container Apps", "cube", ["fw", "dns", "idp", "mon", "pol"], "no", "Serverless containers. A workload profiles environment in the spoke VNet can route egress through the firewall."]] },
    { t: "Data", c: "lz", items: [
      ["sql", "Azure SQL", "db", ["dns", "idp", "mon", "pol", "def"], "no", "Managed SQL Server databases. Public network access off in Corp, private endpoint, Entra-only authentication, Defender for SQL."],
      ["pg", "PostgreSQL flexible server", "db", ["dns", "idp", "mon", "pol"], "cp", "Managed PostgreSQL. Private access through a delegated subnet and a private DNS zone, or a private endpoint. The Arena's pitch has one."],
      ["cosmos", "Cosmos DB", "layers", ["dns", "idp", "mon", "pol", "def"], "cp", "Globally distributed NoSQL and MongoDB. Private endpoint and Entra RBAC on the data plane."],
      ["st", "Storage", "storage", ["dns", "idp", "mon", "pol", "def"], "yes", "Blobs, files, queues, tables. No public blob access, no shared keys, private endpoints in Corp. The Arena's <code>safety-code.tf</code> enforces some of this."]] },
    { t: "Integration", c: "lz", items: [
      ["apim", "API Management", "api", ["dns", "fw", "idp", "mon", "pol", "def"], "no", "A gateway for APIs. Often in a shared integration landing zone, in internal VNet mode behind Application Gateway for public APIs."],
      ["sb", "Service Bus", "queue", ["dns", "idp", "mon", "pol"], "no", "Queues and topics for reliable messaging. Premium tier for private endpoints."],
      ["eg", "Event Grid", "bolt", ["dns", "idp", "mon", "pol"], "no", "Event routing between services. Private endpoints for custom topics."],
      ["logic", "Logic Apps", "flow", ["fw", "dns", "idp", "mon", "pol"], "no", "Low-code workflows. Standard plan for VNet integration and private endpoints."]] },
    { t: "AI and analytics", c: "lz", items: [
      ["aoai", "Azure OpenAI in Azure AI Foundry", "ai", ["dns", "fw", "idp", "mon", "pol", "def"], "no", "Generative AI models. Private endpoint, managed identity, no API keys, content filters, and Defender for AI services. Often called from apps behind API Management."],
      ["fabric", "Microsoft Fabric", "layers", ["idp", "mon", "pol"], "no", "SaaS analytics. Capacities are Azure resources in a subscription; workspace access and private links are set in Fabric and Entra ID."],
      ["dbx", "Azure Databricks", "chart", ["fw", "dns", "idp", "mon", "pol"], "no", "Spark analytics. VNet injection into the spoke, egress through the firewall, private link for the workspace."]] },
    { t: "Online edge", c: "lz", items: [
      ["afd", "Azure Front Door", "globe", ["mon", "pol", "def"], "no", "Global entry with CDN and WAF at the edge. Reaches origins privately with Private Link (Premium)."],
      ["agw", "Application Gateway", "lb", ["mon", "pol"], "no", "Regional layer 7 load balancer in the Online spoke."],
      ["waf", "Web Application Firewall", "waf", ["mon", "pol"], "no", "Policies with OWASP rule sets, on Front Door or Application Gateway. Audit or deny policies check it is on."]] }
  ];
  var D = {};
  Object.keys(PD).forEach(function (k) { D[k] = PD[k]; });
  var s = "", lines = "", pos = {}, colW = 206, x0 = 26;
  s += '<rect x="12" y="470" width="1076" height="120" rx="16" class="band"/><g id="dep-lines"></g>';
  s += A.zone(12, 12, 1076, 406, "--az-lz", "Application landing zone · services you build with");
  G.forEach(function (g, gi) {
    var x = x0 + gi * (colW + 8);
    s += A.text(x + 4, 54, g.t, "t-band");
    g.items.forEach(function (it, i) {
      var y = 64 + i * 56;
      pos[it[0]] = [x + colW / 2, y + 46];
      D[it[0]] = { k: g.t, t: it[1], d: it[5], deps: it[3], s: [["Leans on these platform services", it[3].map(function (p) { return P[p][0] + ": " + PD[p].d; })]], ct: CT[it[4]] };
      var badge = it[4] === "yes" || it[4] === "cp" ? '<g><rect x="' + (x + colW - 34) + '" y="' + (y + 6) + '" width="28" height="16" rx="8" style="fill:var(--accent)"/><text x="' + (x + colW - 20) + '" y="' + (y + 17.5) + '" text-anchor="middle" style="font:700 9.5px var(--font);fill:var(--accent-ink)">CT</text></g>' : "";
      s += A.node({ id: it[0], x: x, y: y, w: colW, h: 46, t: A.wrap(it[1], 20), ic: it[2], k: "lz", tc: "t-s t-ink", tlh: 14, extra: badge });
    });
  });
  s += A.text(26, 404, "CT = you can practise it in CloudTyke today.", "t-xs");
  s += A.text(28, 492, "Platform services every workload leans on", "t-band");
  var pk = Object.keys(P);
  pk.forEach(function (k, i) {
    var x = 28 + i * 176;
    pos["p-" + k] = [x + 82, 506];
    s += A.node({ id: k, x: x, y: 506, w: 164, h: 64, t: P[k][0], s: P[k][1], ic: P[k][2], k: k === "def" ? "sec" : k === "pol" ? "gov" : k === "idp" ? "id" : k === "fw" || k === "dns" ? "net" : "plat" });
  });
  var svg = document.getElementById("cat");
  svg.innerHTML = s;
  function drawLines(id) {
    var d = D[id], out = "";
    if (d && d.deps) {
      var from = pos[id];
      d.deps.forEach(function (p, i) {
        var to = pos["p-" + p], busY = 440 + (i % 3) * 8;
        out += A.path([[from[0], from[1]], [from[0], busY], [to[0], busY], [to[0], to[1]]], "hot");
      });
    } else if (PD[id]) {
      Object.keys(D).forEach(function (k) { if (D[k].deps && D[k].deps.indexOf(id) >= 0) svg.querySelector('[data-node="' + k + '"]').classList.add("rel"); });
    }
    svg.querySelector("#dep-lines").innerHTML = out;
  }
  A.bind(svg, D, document.getElementById("cat-panel"), { first: "aks", onSelect: function (id) {
    var d = D[id];
    if (d.deps) d.deps.forEach(function (p) { svg.querySelector('[data-node="' + p + '"]').classList.add("rel"); });
    drawLines(id);
  } });
})();
