/* Azure landing zones, scenario B: migrate on-premises workloads. Flow, the 5 Rs, and a target decision tree. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;

  /* ---------------- figure 1: the migration journey ---------------- */
  var N = {
    dc: [16, 130, 190, 150, ["Fabrikam data", "centre"], "VMs, SQL, SAP, file shares", "building", "onprem"],
    assess: [250, 30, 200, 64, "Discover and assess", "Azure Migrate appliance", "audit", "dev"],
    link: [250, 130, 200, 64, "Connect to the hub", "ExpressRoute or VPN", "gw", "net"],
    waves: [250, 230, 200, 64, "Plan waves", "groups that move together", "layers", "dev"],
    hub: [490, 130, 160, 64, "Connectivity hub", "firewall, DNS", "net", "net"],
    repl: [490, 230, 160, 64, "Replicate and test", "Azure Migrate", "refresh", "dev"],
    corp: [700, 30, 170, 56, "Corp landing zones", "internal apps", "sub", "lz"],
    online: [700, 120, 170, 56, "Online landing zones", "public apps", "globe", "lz"],
    sap: [700, 210, 170, 56, "SAP landing zone", "SAP estate", "sap", "lz"],
    cut: [910, 120, 174, 64, "Cutover", "switch DNS, go live", "flag", "dev"],
    decom: [910, 240, 174, 64, ["Decommission", "on-premises"], null, "trash", "decom"]
  };
  var D1 = {
    dc: { k: "Source", t: "Fabrikam's data centre", d: "The estate to move: about 300 VMs (Windows and Linux), SQL Server, an SAP ERP system, file shares and a few web apps. The lease ends in 18 months." },
    assess: { k: "Step 1", t: "Discover and assess", d: "The Azure Migrate appliance discovers servers, software and dependencies. Assessments give right-sized VM or PaaS targets, readiness and a cost estimate. Business owners confirm which apps matter most." },
    link: { k: "Step 2", t: "Connect to the hub", d: "The platform landing zone comes first. ExpressRoute (or a VPN to start) joins the data centre to the hub, so migrated servers keep talking to what is still on-premises." },
    waves: { k: "Step 5", t: "Plan waves", d: "Group servers that depend on each other into waves (dependency analysis helps). Start with a low-risk pilot wave, then grow. Each wave has an owner, a date, a test plan and a rollback plan." },
    hub: { k: "Platform", t: "Connectivity hub", d: "Every wave lands in a spoke peered to the hub. The firewall allows traffic back to on-premises during the move, and DNS resolves both worlds." },
    repl: { k: "Step 6", t: "Replicate and test", d: "Azure Migrate replicates the servers into the target landing zone. Run a test migration into an isolated network first, then fix what breaks." },
    corp: { k: "Step 4: target", t: "Corp landing zones", d: "Most internal apps go here, one subscription per app or app group per environment." },
    online: { k: "Step 4: target", t: "Online landing zones", d: "Public web apps, behind Front Door or Application Gateway with WAF." },
    sap: { k: "Step 4: target", t: "SAP landing zone", d: "The SAP estate, sized from the assessment, close to on-premises over ExpressRoute." },
    cut: { k: "Step 7", t: "Cutover", d: "Freeze changes, do a final sync, switch DNS or load balancer targets, run smoke tests, then hand over to operations. Keep the source ready for rollback for a short time." },
    decom: { k: "Step 8", t: "Decommission on-premises", d: "Turn the old servers off, keep backups as policy requires, then remove them and release licences. Track it: savings only come when the old kit is gone." }
  };
  D1.assess.ct = D1.repl.ct = "Azure Migrate is not emulated. CloudTyke is useful for the landing zone side: build and test the target spokes, policy and RBAC before a wave moves.";
  var s = "";
  Object.keys(N).forEach(function (k) { var n = N[k]; s += A.node({ id: k, x: n[0], y: n[1], w: n[2], h: n[3], t: n[4], s: n[5] || undefined, ic: n[6], k: n[7] }); });
  var E = [
    ["dc", "assess", [[206, 160], [228, 160], [228, 62], [250, 62]]],
    ["dc", "link", [[206, 205], [250, 162]]],
    ["dc", "waves", [[206, 250], [228, 250], [228, 262], [250, 262]]],
    ["link", "hub", [[450, 162], [490, 162]]],
    ["waves", "repl", [[450, 262], [490, 262]]],
    ["hub", "corp", [[650, 150], [675, 150], [675, 58], [700, 58]]],
    ["hub", "online", [[650, 162], [675, 162], [675, 148], [700, 148]]],
    ["repl", "sap", [[650, 262], [675, 262], [675, 238], [700, 238]]],
    ["repl", "online", [[650, 250], [668, 250], [668, 172], [700, 172]]],
    ["online", "cut", [[870, 148], [910, 148]]],
    ["corp", "cut", [[870, 58], [890, 58], [890, 140], [910, 140]]],
    ["sap", "cut", [[870, 238], [890, 238], [890, 168], [910, 168]]],
    ["cut", "decom", [[997, 184], [997, 240]]]
  ];
  var es = E.map(function (e) { return A.path(e[2], "thin", ' data-e="' + e[0] + "-" + e[1] + '"'); }).join("");
  var svg1 = document.getElementById("journey");
  svg1.innerHTML = '<g id="j-edges">' + es + "</g>" + s;
  var api1 = A.bind(svg1, D1, document.getElementById("journey-panel"), { empty: '<p class="hint">Press Next step, or click any box.</p>' });
  var JS = [
    { t: "Discover and assess", d: "Find everything, map dependencies, size it, price it.", on: ["dc", "assess"] },
    { t: "Build the platform and connect", d: "Platform landing zone first. Then ExpressRoute or VPN into the hub.", on: ["link", "hub"] },
    { t: "Choose a strategy per app", d: "Rehost, refactor, rearchitect, rebuild or replace. See figure 2.", on: ["dc"] },
    { t: "Choose the target landing zone", d: "Corp, Online or SAP, by what each app needs. See figure 3.", on: ["corp", "online", "sap"] },
    { t: "Plan waves", d: "Pilot first, then waves of servers that depend on each other.", on: ["waves"] },
    { t: "Replicate and test", d: "Replicate into the landing zone and run test migrations.", on: ["repl", "corp", "online", "sap"] },
    { t: "Cut over", d: "Final sync, switch DNS, smoke test, hand over.", on: ["cut"] },
    { t: "Decommission", d: "Turn off and remove the old servers. Repeat for the next wave.", on: ["decom"] }
  ];
  A.stepper(document.getElementById("journey-steps"), JS, function (i) {
    A.mark(svg1, i >= 0 ? JS[i].on : [], "on");
    svg1.classList.toggle("dimming", i >= 0);
    svg1.querySelectorAll("#j-edges .edge").forEach(function (p) {
      var ids = p.getAttribute("data-e").split("-");
      p.classList.toggle("hot", i >= 0 && JS[i].on.indexOf(ids[1]) >= 0);
    });
    if (i >= 0) api1.select(JS[i].on[JS[i].on.length - 1], true); else api1.clear();
  });

  /* ---------------- figure 2: the 5 Rs ---------------- */
  var R = {
    rehost: { t: "Rehost", sub: "lift and shift", eff: 1, gain: 1, from: ["Windows Server VM", "IIS web app", "SQL Server on a VM"], to: [["Azure VM", "vm"], ["Azure VM", "vm"], ["SQL Server on Azure VM", "db"]], lz: "Corp",
      d: "Move the servers as they are. Fastest way out of a data centre with a deadline. Cloud gains come later, through modernisation (scenario D).", ex: "Fabrikam's HR system: two VMs moved by Azure Migrate in one weekend." },
    refactor: { t: "Refactor", sub: "small changes, move to PaaS", eff: 2, gain: 2, from: ["IIS web app", "SQL Server database"], to: [["App Service", "globe"], ["Azure SQL Database", "db"]], lz: "Corp",
      d: "Change configuration and a little code so the app runs on platform services. No servers to patch.", ex: "The expenses app moves to App Service and Azure SQL Database, with private endpoints." },
    rearchitect: { t: "Rearchitect", sub: "change the design", eff: 3, gain: 3, from: ["Monolith on 4 VMs"], to: [["AKS or Container Apps", "wheel"], ["Service Bus", "queue"], ["Azure SQL", "db"]], lz: "Corp or Online",
      d: "Split or redesign the app to scale and release independently. More work, more benefit.", ex: "The order system becomes containers on AKS, with Service Bus between the parts." },
    rebuild: { t: "Rebuild", sub: "write it again, cloud-native", eff: 4, gain: 3, from: ["Old VB6 desktop app"], to: [["Azure Functions", "fn"], ["Cosmos DB", "layers"], ["Static web front end", "globe"]], lz: "Online",
      d: "Throw the old code away and build new. Right when the old app cannot change and the business needs something new anyway.", ex: "Customer self-service is rebuilt on Functions and Cosmos DB." },
    replace: { t: "Replace", sub: "buy SaaS instead", eff: 2, gain: 2, from: ["On-premises CRM"], to: [["SaaS product (for example Dynamics 365)", "cloud"]], lz: "none (SaaS), integration in Corp",
      d: "Retire the app and use a SaaS product. Little to run in Azure, apart from integration and identity.", ex: "The CRM moves to a SaaS product. Data integration runs from a Corp landing zone." }
  };
  A.icons.cloud = '<path d="M7 18h10.5a4 4 0 0 0 .5-8 6 6 0 0 0-11.5 1.5A3.3 3.3 0 0 0 7 18z"/>';
  var svg2 = document.getElementById("rs");
  function drawR(k) {
    var r = R[k], s = "";
    s += A.text(24, 30, "Today, on-premises", "t-band") + A.text(560, 30, "In Azure", "t-band");
    r.from.forEach(function (f, i) { s += A.node({ id: "f" + i, x: 24, y: 46 + i * 62, w: 300, h: 50, t: f, ic: i === r.from.length - 1 && /SQL|database/i.test(f) ? "db" : "server", k: "onprem" }); });
    r.to.forEach(function (f, i) { s += A.node({ id: "t" + i, x: 560, y: 46 + i * 62, w: 300, h: 50, t: f[0], ic: f[1], k: "lz" }); });
    s += A.path([[340, 100], [540, 100]], "hot") + A.arrow(546, 100, "right", "hot");
    s += A.text(440, 88, r.t, "t-hl", "middle") + A.text(440, 122, r.sub, "t-xs", "middle");
    var by = 250;
    [["Effort and risk", r.eff, "bar-c"], ["Cloud benefit", r.gain, "bar-b"]].forEach(function (b, i) {
      var y = by + i * 34;
      s += A.text(24, y + 14, b[0], "t-s t-ink");
      for (var q = 0; q < 4; q++) s += '<rect x="' + (160 + q * 54) + '" y="' + y + '" width="48" height="18" rx="5" class="' + (q < b[1] ? b[2] : "bar-bg") + '"/>';
    });
    s += A.text(560, by + 14, "Target landing zone: " + r.lz, "t-s t-ink", null, 14, 320);
    svg2.innerHTML = s;
    A.fit(svg2);
    document.getElementById("rs-out").innerHTML = "<h3>" + r.t + "</h3><p>" + r.d + "</p><p><strong>Example.</strong> " + r.ex + '</p><p class="small muted">The Cloud Adoption Framework also lists replatform, retain and retire as options in some guides. The bars are a rough comparison.</p>';
  }
  A.seg(document.getElementById("rs-pick"), "Strategy:", Object.keys(R).map(function (k) { return [k, R[k].t]; }), drawR);

  /* ---------------- figure 3: which landing zone? ---------------- */
  var T = {
    q1: [380, 20, "Is it an experiment or a proof of concept?"],
    q2: [380, 120, "Is it part of an SAP estate?"],
    q3: [380, 220, "Do internet users reach it directly?"],
    q4: [380, 320, "Does it need private links to on-premises or other internal apps?"],
    sandbox: [40, 20, "Sandbox"], sap: [40, 120, "SAP landing zone"], online: [40, 220, "Online landing zone"],
    corp: [40, 320, "Corp landing zone"], corp2: [760, 420, "Corp landing zone (default: private unless it must be public)"]
  };
  var PATH = { yes: { q1: "sandbox", q2: "sap", q3: "online", q4: "corp" }, no: { q1: "q2", q2: "q3", q3: "q4", q4: "corp2" } };
  var APPS = {
    hr: { t: "HR portal for staff", a: { q1: "no", q2: "no", q3: "no", q4: "yes" } },
    shop: { t: "Customer web shop", a: { q1: "no", q2: "no", q3: "yes" } },
    s4: { t: "SAP S/4HANA", a: { q1: "no", q2: "yes" } },
    poc: { t: "AI proof of concept", a: { q1: "yes" } },
    batch: { t: "Nightly report job", a: { q1: "no", q2: "no", q3: "no", q4: "no" } }
  };
  var D3 = {
    sandbox: { k: "Result", t: "Sandbox", d: "Isolated and cheap to throw away. When the idea proves itself, vend a real landing zone and redeploy from code (see scenario E)." },
    sap: { k: "Result", t: "SAP landing zone", d: "Under the SAP management group. Large VM sizes, ExpressRoute to on-premises, SAP-specific guardrails." },
    online: { k: "Result", t: "Online landing zone", d: "Published through Front Door or Application Gateway with WAF. It can still be peered to the hub for private calls to internal APIs." },
    corp: { k: "Result", t: "Corp landing zone", d: "Private only, peered to the hub, reachable from on-premises." },
    corp2: { k: "Result", t: "Corp landing zone", d: "Nothing needs to be public, so keep it private. Corp is the safe default." }
  };
  ["q1", "q2", "q3", "q4"].forEach(function (q) {
    D3[q + "y"] = { k: "Answer", t: "Yes: " + T[q][2], d: "Follow the yes branch." };
    D3[q + "n"] = { k: "Answer", t: "No: " + T[q][2], d: "Follow the no branch." };
  });
  var svg3 = document.getElementById("tree"), ans = {};
  function drawT() {
    var s = "", path = [], at = "q1";
    while (at && at.charAt(0) === "q") { path.push(at); var a = ans[at]; if (!a) break; at = PATH[a][at]; }
    var result = at && at.charAt(0) !== "q" ? at : null;
    ["q1", "q2", "q3", "q4"].forEach(function (q, i) {
      var t = T[q], y = t[1], yes = PATH.yes[q], on = path.indexOf(q) >= 0;
      s += '<g class="' + (on ? "" : "faded") + '">';
      s += '<rect x="' + t[0] + '" y="' + y + '" width="340" height="60" rx="12" class="band" style="' + (on ? "stroke:var(--az-flow);stroke-width:2" : "") + '"/>' + A.text(t[0] + 16, y + 26, A.wrap(t[2], 42), "t-s t-ink", null, 15, 310);
      s += A.path([[t[0], y + 30], [T[yes][0] + 240, y + 30]], ans[q] === "yes" ? "hot" : "");
      if (i < 3) s += A.path([[t[0] + 170, y + 60], [t[0] + 170, y + 100]], ans[q] === "no" ? "hot" : "");
      else s += A.path([[t[0] + 340, y + 30], [910, y + 30], [910, 420]], ans[q] === "no" ? "hot" : "");
      s += A.node({ id: q + "y", x: t[0] - 70, y: y + 18, w: 52, h: 24, t: "Yes", k: ans[q] === "yes" ? "lz" : "chip", center: true, tc: "t-s t-ink", rx: 12 });
      s += A.node({ id: q + "n", x: i < 3 ? t[0] + 182 : t[0] + 360, y: i < 3 ? y + 68 : y + 18, w: 52, h: 24, t: "No", k: ans[q] === "no" ? "decom" : "chip", center: true, tc: "t-s t-ink", rx: 12 });
      s += "</g>";
    });
    ["sandbox", "sap", "online", "corp"].forEach(function (r) {
      var t = T[r];
      s += A.node({ id: r, x: t[0], y: t[1] + 6, w: 240, h: 48, t: t[2], k: r === "sandbox" ? "sand" : "lz", ic: r === "sandbox" ? "flask" : r === "sap" ? "sap" : r === "online" ? "globe" : "sub", cls: result === r ? "on" : result ? "off" : "" });
    });
    s += A.node({ id: "corp2", x: 760, y: 420, w: 300, h: 56, t: ["Corp landing zone"], s: "private unless it must be public", k: "lz", ic: "sub", cls: result === "corp2" ? "on" : result ? "off" : "" });
    svg3.innerHTML = s;
    api3.refresh();
    var out = document.getElementById("tree-out");
    out.innerHTML = result ? A.panelHtml(D3[result]) : '<p class="hint">Answer the next question: click Yes or No in the drawing, or pick an example app.</p>';
  }
  var api3 = { refresh: function () {} };
  svg3.addEventListener("click", function (e) {
    var n = e.target.closest && e.target.closest("[data-node]");
    if (!n) return;
    var id = n.getAttribute("data-node"), m = id.match(/^(q\d)([yn])$/);
    if (!m) return;
    choose(m[1], m[2] === "y" ? "yes" : "no", id);
  });
  svg3.addEventListener("keydown", function (e) {
    if (e.key !== "Enter" && e.key !== " ") return;
    var n = e.target.closest && e.target.closest("[data-node]");
    if (!n || n !== e.target) return;
    e.preventDefault();
    var m = n.getAttribute("data-node").match(/^(q\d)([yn])$/);
    if (m) choose(m[1], m[2] === "y" ? "yes" : "no", n.getAttribute("data-node"));
  });
  function choose(q, a, focusId) {
    ans[q] = a;
    var order = ["q1", "q2", "q3", "q4"];
    for (var k = order.indexOf(q) + 1; k < 4; k++) if (a === "yes") delete ans[order[k]];
    drawT();
    var f = svg3.querySelector('[data-node="' + focusId + '"]');
    if (f) f.focus();
  }
  var tpanel = document.getElementById("tree-out");
  api3 = A.bind(svg3, D3, document.createElement("div"), {});
  api3.refresh = (function (orig) { return function () { orig(); }; })(api3.refresh);
  drawT();
  var ab = document.getElementById("tree-apps");
  ab.innerHTML = '<span class="lbl">Example apps:</span>' + Object.keys(APPS).map(function (k) { return '<button type="button" class="btn" data-app="' + k + '">' + A.esc(APPS[k].t) + "</button>"; }).join("") + '<button type="button" class="btn ghost" data-app="">Start again</button>';
  ab.addEventListener("click", function (e) {
    var b = e.target.closest("[data-app]");
    if (!b) return;
    var k = b.getAttribute("data-app");
    ans = k ? JSON.parse(JSON.stringify(APPS[k].a)) : {};
    drawT();
    tpanel.focus && tpanel.setAttribute("tabindex", "-1");
  });
})();
