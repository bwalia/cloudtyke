/* Azure landing zones: platform vs application landing zones, and who owns what. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  /* owner per model: P platform team, S shared, A app team */
  var ROWS = [
    ["tenant", "Entra tenant, identity and PIM", "user", { central: "P", tech: "P", workload: "P" }],
    ["mg", "Management groups and Azure Policy", "policy", { central: "P", tech: "P", workload: "P" }],
    ["hub", "Connectivity hub: firewall, gateways, DNS", "net", { central: "P", tech: "P", workload: "P" }],
    ["logs", "Central logging and security tooling", "chart", { central: "P", tech: "P", workload: "P" }],
    ["vend", "Subscription vending", "vend", { central: "P", tech: "P", workload: "P" }],
    ["spoke", "Spoke VNet, peering and routes", "net", { central: "P", tech: "P", workload: "S" }],
    ["rbac", "Workload RBAC, identities, Key Vault", "key", { central: "P", tech: "S", workload: "A" }],
    ["res", "Workload resources: compute and data", "cube", { central: "P", tech: "S", workload: "A" }],
    ["code", "App code, data and releases", "pipe", { central: "A", tech: "A", workload: "A" }]
  ];
  var MODELS = {
    central: { t: "Central team management", d: "A central IT team runs the application landing zone too. App teams hand over code. Simple and tightly controlled, but the central team becomes a queue as the estate grows." },
    tech: { t: "Technology platform team management", d: "A platform team runs a shared technology, such as an AKS cluster or an App Service Environment, inside the landing zone. Many app teams deploy onto it. Ownership of the middle layers is shared." },
    workload: { t: "Workload team management", d: "The app team owns its landing zone end to end, inside the guardrails. The platform team still owns the hub, the policy and vending. This scales best and is the most common pattern for mature teams." }
  };
  var D = {
    tenant: { k: "Platform landing zone", t: "Entra tenant, identity and PIM", d: "One tenant for everyone. The platform team runs it, sets Conditional Access, manages PIM for privileged roles, and creates the groups that landing zones use." },
    mg: { k: "Platform landing zone", t: "Management groups and Azure Policy", d: "The rules for every landing zone. App teams can ask for an exemption, with an expiry date, but cannot remove a policy." },
    hub: { k: "Platform landing zone", t: "Connectivity hub", d: "Firewall rules, gateways, DNS and Bastion. App teams request firewall rules, often through a pull request to the firewall policy code." },
    logs: { k: "Platform landing zone", t: "Central logging and security tooling", d: "Log Analytics, Defender for Cloud, Sentinel. App teams can read the logs for their own resources (resource-context access) and add their own alerts." },
    vend: { k: "Platform landing zone", t: "Subscription vending", d: "The platform team owns the vending code. App teams fill in a request: name, environment, archetype (Corp or Online), network size, budget, owners." },
    spoke: { k: "Application landing zone", t: "Spoke VNet, peering and routes", d: "Vending builds the spoke and peers it to the hub. The route to the firewall and the peering are locked against the app team. In workload-team management, the app team adds its own subnets and NSG rules inside the spoke." },
    rbac: { k: "Application landing zone", t: "Workload RBAC, identities, Key Vault", d: "Who in the app team can do what, the managed identities the app uses, and its own Key Vault. Owned by whoever runs the landing zone day to day." },
    res: { k: "Application landing zone", t: "Workload resources", d: "VMs, AKS, App Service, databases, storage. With a technology platform team, the shared cluster belongs to that team and each app team owns its namespaces or apps." },
    code: { k: "Application landing zone", t: "App code, data and releases", d: "Always the app team. They own the backlog, the data and the release pipeline." }
  };
  var svg = document.getElementById("own"), panel = document.getElementById("own-panel"), model = "workload", api;
  var X0 = 330, COLW = 316, GAP = 12, RH = 44, TOP = 70;
  function draw() {
    var s = "";
    s += '<text x="' + (X0 + COLW / 2) + '" y="40" text-anchor="middle" class="t-hl" style="fill:var(--az-plat)">Platform team</text>';
    s += '<text x="' + (X0 + COLW + GAP + COLW / 2) + '" y="40" text-anchor="middle" class="t-hl" style="fill:var(--crew)">App team</text>';
    s += '<rect x="' + X0 + '" y="54" width="' + COLW + '" height="' + (ROWS.length * RH + 16) + '" rx="14" class="band"/>';
    s += '<rect x="' + (X0 + COLW + GAP) + '" y="54" width="' + COLW + '" height="' + (ROWS.length * RH + 16) + '" rx="14" class="band"/>';
    /* brackets */
    var py1 = TOP, py2 = TOP + 5 * RH - 8, ay1 = TOP + 5 * RH, ay2 = TOP + 9 * RH - 8;
    s += '<path class="edge" style="stroke:var(--az-plat)" d="M24 ' + py1 + "h-6V" + py2 + 'h6"/>' + '<text class="t-band" style="fill:var(--az-plat)" transform="translate(12 ' + ((py1 + py2) / 2) + ') rotate(-90)" text-anchor="middle">Platform LZ</text>';
    s += '<path class="edge" style="stroke:var(--az-lz)" d="M24 ' + ay1 + "h-6V" + ay2 + 'h6"/>' + '<text class="t-band" style="fill:var(--az-lz)" transform="translate(12 ' + ((ay1 + ay2) / 2) + ') rotate(-90)" text-anchor="middle">Application LZ</text>';
    ROWS.forEach(function (r, i) {
      var y = TOP + i * RH, o = r[3][model];
      s += A.icon(r[2], 44, y + 18, 18) + A.text(62, y + 23, r[1], "t-s t-ink", null, 14, 255);
      var x = o === "A" ? X0 + COLW + GAP + 8 : X0 + 8, w = o === "S" ? COLW * 2 + GAP - 16 : COLW - 16;
      var lab = o === "P" ? "Platform team" : o === "A" ? "App team" : "Shared: platform sets it up, app team works inside it";
      s += '<g class="nd" data-node="' + r[0] + '" data-own="' + (o === "P" ? "plat" : o === "A" ? "app" : "shared") + '"><rect class="box" x="' + x + '" y="' + y + '" width="' + w + '" height="' + (RH - 8) + '" rx="9"/>' +
        A.text(x + w / 2, y + 23, lab, "t-s t-ink", "middle", 14, w - 16) + "</g>";
    });
    svg.innerHTML = s;
    svg.classList.add("show-own");
  }
  draw();
  api = A.bind(svg, D, panel, { first: "spoke", render: function (id, d) {
    var row = ROWS.filter(function (r) { return r[0] === id; })[0], o = row[3][model];
    return A.panelHtml(d) + '<div class="pb"><h4>Owner in "' + MODELS[model].t + '"</h4><p>' + (o === "P" ? '<span class="pill plat">Platform team</span>' : o === "A" ? '<span class="pill app">App team</span>' : '<span class="pill shared">Shared</span>') + "</p></div>";
  } });
  A.seg(document.getElementById("own-model"), "Management model:", [["central", "Central team"], ["tech", "Technology platform team"], ["workload", "Workload team"]], function (v) {
    model = v; draw(); api.refresh();
    document.getElementById("own-desc").innerHTML = "<strong>" + MODELS[v].t + ".</strong> " + MODELS[v].d;
  }, "workload");
})();
