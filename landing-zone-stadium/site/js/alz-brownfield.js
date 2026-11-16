/* Azure landing zones, scenario C: bring an existing (brownfield) estate under a landing zone. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  var MG = {
    contoso: [475, 86, 150, "Contoso", "root", "root"],
    platform: [130, 156, 150, "Platform", "plat", "contoso"],
    lzs: [430, 156, 150, "Landing zones", "lz", "contoso"],
    sandbox: [760, 156, 140, "Sandbox", "sand", "contoso"],
    decom: [930, 156, 150, "Decommissioned", "decom", "contoso"],
    conn: [40, 226, 130, "Connectivity", "plat", "platform"],
    mgmt: [190, 226, 130, "Management", "plat", "platform"],
    corp: [360, 226, 180, "Corp", "lz", "lzs"],
    online: [560, 226, 130, "Online", "lz", "lzs"]
  };
  /* subscription: [before x, after x, after parent, title, vnet before, vnet after, colour] */
  var SUB = {
    net: [40, 40, "conn", "Network-Shared", "10.100.0.0/16", "hub 10.100.0.0/16", "plat"],
    fin: [190, 330, "corp", "Finance-App", "10.0.0.0/16", "10.20.0.0/16 (re-IP)", "lz"],
    dev: [340, 760, "sandbox", "Dev-Test-1", "10.0.0.0/16", "10.0.0.0/16 (isolated)", "sand"],
    web: [490, 610, "online", "Prod-Web", "10.0.0.0/16", "10.0.0.0/16", "lz"],
    crm: [640, 930, "decom", "Legacy-CRM", "172.16.0.0/16", "172.16.0.0/16", "decom"],
    data: [790, 470, "corp", "Data-Platform", "10.1.0.0/16", "10.1.0.0/16", "lz"]
  };
  var STEPS = [
    { t: "Today: sprawl", d: "Six subscriptions sit straight under the tenant root group. Many people hold Owner. Three VNets use 10.0.0.0/16. Each subscription keeps its own logs, or none. Nobody can say what is compliant." },
    { t: "Build the hierarchy next to it", d: "Deploy the management groups and policy definitions (the Azure landing zones accelerator supports existing estates). Nothing moves yet, so nothing breaks." },
    { t: "Move subscriptions in", d: "Move each subscription under the right group. Resources do not move and apps keep running, but inherited policy and RBAC change at once. Check role assignments and test policy impact first. Legacy-CRM goes straight to Decommissioned." },
    { t: "Policy in audit mode", d: "Assign the guardrails with enforcement mode set to DoNotEnforce, or with Audit effects. Read the compliance report: 58% compliant. Nobody is blocked while you learn what would break." },
    { t: "Remediate, then enforce", d: "Run remediation tasks for DeployIfNotExists and Modify policies (diagnostics, Defender plans, tags). Fix the rest with the app teams, add time-limited exemptions where needed. Then switch to Deny and Default enforcement: 96%." },
    { t: "Bring VNets into the hub", d: "Network-Shared becomes the hub (or a new hub is built). Finance-App overlaps Prod-Web on 10.0.0.0/16, and overlapping VNets cannot both peer to one hub, so Finance-App is re-addressed to 10.20.0.0/16. Spokes peer to the hub and route through the firewall. Dev-Test stays isolated in Sandbox." },
    { t: "Centralise logging and security", d: "A new management subscription hosts the central Log Analytics workspace. Policy points every diagnostic setting at it and turns on Defender for Cloud plans everywhere. Sentinel runs on top. Old per-subscription workspaces are retired after their retention ends." },
    { t: "Retire legacy", d: "Legacy-CRM sat read-only in Decommissioned through the agreed cooling-off period. Now it is cancelled. Owner rights are trimmed to groups with PIM. The estate looks like the reference, and new apps arrive by vending." }
  ];
  var D = {
    root: { k: "Management group", t: "Tenant root group", d: "Before: everything hangs here. After: only Contoso does." },
    contoso: { k: "Management group", t: "Contoso", d: "The new intermediate root. Organisation-wide policy goes here." },
    platform: { k: "Management group", t: "Platform", d: "Shared services." }, lzs: { k: "Management group", t: "Landing zones", d: "Workload guardrails." },
    sandbox: { k: "Management group", t: "Sandbox", d: "Dev-Test-1 lands here: isolated, with a budget." }, decom: { k: "Management group", t: "Decommissioned", d: "Read-only waiting room before cancellation." },
    conn: { k: "Management group", t: "Connectivity", d: "Holds the hub." }, mgmt: { k: "Management group", t: "Management", d: "Holds central logging, created in step 7." },
    corp: { k: "Management group", t: "Corp", d: "Finance-App and Data-Platform: internal, private." }, online: { k: "Management group", t: "Online", d: "Prod-Web: internet-facing behind a WAF." },
    meter: { k: "Azure Policy", t: "Compliance", d: "Azure Policy compliance across all assignments. Audit first shows what would break; enforce once it is fixed." },
    law: { k: "Management subscription", t: "Central Log Analytics", d: "One workspace for the whole estate, with Defender for Cloud and Sentinel." }
  };
  Object.keys(SUB).forEach(function (k) { D[k] = { k: "Subscription", t: SUB[k][3], d: "VNet before: " + SUB[k][4] + ". After: " + SUB[k][5] + "." }; });
  D.fin.d += " Re-addressing means a new VNet and moving the workload into it, or rebuilding subnets during a maintenance window.";
  D.root.ct = "CloudTyke cannot move subscriptions between management groups (read-only). Practise the policy side: assign an initiative with <code>enforcement_mode</code> set to DoNotEnforce, check compliance with <code>policyStates</code>, then enforce. Peering with overlapping address spaces is rejected by the emulator, just as in Azure.";
  D.meter.ct = "<code>Microsoft.PolicyInsights/policyStates</code> summarise and triggerEvaluation are emulated.";
  var svg = document.getElementById("brown"), api, step = -1;
  function x(k) { return step >= 2 ? SUB[k][1] : SUB[k][0]; }
  function draw() {
    var s = "", st = step;
    var showMG = st >= 1;
    /* edges */
    if (showMG) {
      Object.keys(MG).forEach(function (k) {
        var m = MG[k], p = m[5] === "root" ? null : MG[m[5]];
        if (k === "mgmt" && st < 6) return;
        if (p) s += A.elbow(p[0] + p[2] / 2, p[1] + 40, m[0] + m[2] / 2, m[1], "", m[1] - 14);
      });
      s += A.elbow(550, 56, 550, 86, "");
    }
    Object.keys(SUB).forEach(function (k) {
      if (k === "crm" && st >= 7) return;
      if (st >= 2) { var p = MG[SUB[k][2]]; s += A.elbow(p[0] + p[2] / 2, p[1] + 40, x(k) + 65, 320, "", 300); }
      else s += A.elbow(550, 56, x(k) + 65, 320, "dash", 296);
    });
    if (st >= 6) s += A.elbow(MG.mgmt[0] + 65, 266, 255, 320, "");
    /* peering bus */
    if (st >= 5) {
      s += A.path([[105, 410], [105, 432], [675, 432]], "hot");
      ["fin", "data", "web"].forEach(function (k) { s += A.path([[x(k) + 65, 432], [x(k) + 65, 410]], "hot"); });
      s += A.text(130, 448, "hub-to-spoke peering, routed through the firewall", "t-xs");
    }
    s += A.node({ id: "root", x: 470, y: 16, w: 160, h: 40, t: "Tenant root group", k: "root", center: true });
    if (showMG) Object.keys(MG).forEach(function (k) { var m = MG[k]; if (k === "mgmt" && st < 6) return; s += A.node({ id: k, x: m[0], y: m[1], w: m[2], h: 40, t: m[3], k: m[4], center: true, cls: st === 1 ? "on" : "" }); });
    Object.keys(SUB).forEach(function (k) {
      var sb = SUB[k], X = x(k);
      if (k === "crm" && st >= 7) {
        s += '<g class="faded">' + A.rect(X, 320, 130, 56, "band", 10) + A.text(X + 65, 344, "Legacy-CRM", "t-s", "middle") + A.text(X + 65, 362, "cancelled", "t-xs", "middle") + "</g>";
        return;
      }
      var kk = st >= 2 ? sb[6] : "mute";
      s += A.node({ id: k, x: X, y: 320, w: 130, h: 56, t: sb[3], s: st >= 3 && st < 4 && k !== "net" ? "audit: issues found" : "subscription", ic: "sub", k: kk, tc: "t-s t-ink", cls: (st === 2 ? "on" : "") + (st < 2 ? " sprawl" : "") });
      var overlap = st < 5 && sb[4] === "10.0.0.0/16" && k !== "dev";
      var vt = st >= 5 ? sb[5] : sb[4];
      s += '<rect x="' + X + '" y="384" width="130" height="26" rx="6" style="fill:' + (overlap ? "color-mix(in srgb, var(--az-no) 16%, var(--surface))" : "var(--surface2)") + ";stroke:" + (overlap ? "var(--az-no)" : "var(--line)") + '"/>' + A.text(X + 65, 401, vt, "t-m", "middle", 14, 122);
      var central = st >= 6;
      s += '<g style="color:' + (central ? "var(--az-plat)" : "var(--muted)") + '">' + A.icon("log", X + 14, 474, 14) + "</g>" + A.text(X + 26, 478, central ? "logs: central" : k === "web" || k === "dev" ? "logs: none" : "logs: own workspace", "t-xs", null, 14, 102);
    });
    if (st >= 6) s += A.node({ id: "law", x: 190, y: 320, w: 130, h: 56, t: ["Management", "subscription"], s: "Log Analytics", ic: "log", k: "plat", tc: "t-s t-ink", tlh: 14, cls: st === 6 ? "on" : "" });
    if (st < 5) s += A.text(16, 448, "Red: overlapping address spaces (10.0.0.0/16 three times).", "t-xs");
    /* compliance meter */
    if (st >= 3) {
      var pct = st >= 4 ? 96 : 58;
      s += '<g class="nd" data-node="meter"><rect class="box" x="790" y="16" width="290" height="58" rx="10"/>' + A.text(804, 36, "Policy compliance: " + pct + "%" + (st >= 4 ? " (enforced)" : " (audit only)"), "t-s t-ink", null, 14, 270) +
        '<rect x="804" y="48" width="262" height="12" rx="6" class="bar-bg"/><rect x="804" y="48" width="' + (262 * pct / 100) + '" height="12" rx="6" style="fill:' + (st >= 4 ? "var(--az-ok)" : "var(--az-flow)") + '"/></g>';
    }
    if (st < 1) s += A.text(16, 120, "No structure yet. Subscriptions hang off the root.", "t-s");
    svg.innerHTML = s;
    if (api) api.refresh();
  }
  draw();
  api = A.bind(svg, D, document.getElementById("brown-panel"), { empty: '<p class="hint">Press Next step to upgrade the estate. Click any box for details.</p>' });
  A.stepper(document.getElementById("brown-steps"), STEPS, function (i) {
    step = i < 0 ? 0 : i;
    draw();
  }, { start: 0 });
})();
