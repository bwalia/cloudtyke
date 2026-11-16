/* Azure landing zones: management and security services, where each lives and what it watches. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  var ALL = ["sc-mgmt", "sc-conn", "sc-ident", "sc-a1", "sc-online", "sc-sandbox"];
  var SV = {
    law: { t: "Log Analytics workspace", ic: "log", dep: ["sc-mgmt"], watch: ALL, k: "Management",
      d: "The central store for logs. Activity logs, resource diagnostics and VM data from every landing zone land here, sent by DeployIfNotExists policies.",
      s: [["Access", "App teams read their own resources' logs with resource-context access, without seeing everyone else's."]],
      ct: "Log Analytics workspaces are emulated (retention, daily cap, keys). The Activity Log is queryable. Diagnostic settings that ship data are not." },
    monitor: { t: "Azure Monitor", ic: "chart", dep: ["sc-mgmt", "sc-contoso"], watch: ALL, k: "Management",
      d: "Metrics, alerts and dashboards. The Azure Monitor Agent with data collection rules gathers VM data. Azure Monitor Baseline Alerts (AMBA) gives a ready set of platform and landing zone alerts, deployed by policy.",
      ct: "Not emulated beyond Log Analytics and the Activity Log." },
    defender: { t: "Microsoft Defender for Cloud", ic: "shieldcheck", dep: ["sc-contoso"], watch: ALL.concat(["sc-onprem"]), k: "Security",
      d: "Security posture (secure score, recommendations) and workload protection plans for servers, containers, databases, storage, Key Vault and more. Turned on for every subscription by a policy at Contoso. Covers on-premises servers through Azure Arc.",
      ct: "Not emulated." },
    sentinel: { t: "Microsoft Sentinel", ic: "radar", dep: ["sc-mgmt"], watch: ALL.concat(["sc-onprem"]), k: "Security",
      d: "SIEM and SOAR on top of the Log Analytics workspace. Data connectors bring in Entra ID sign-ins, Defender alerts, Azure Activity, firewall logs and on-premises sources. The security operations team runs it.",
      ct: "Not emulated." },
    update: { t: "Azure Update Manager", ic: "refresh", dep: ["sc-contoso"], watch: ["sc-ident", "sc-a1", "sc-onprem"], k: "Management",
      d: "Assess and install updates on Windows and Linux VMs in Azure and on Arc-enabled servers on-premises. No agent or workspace to deploy. A policy turns on periodic assessment for every VM.",
      ct: "Not emulated. VMs arrive in a later CloudTyke phase." },
    backup: { t: "Azure Backup", ic: "backup", dep: ["sc-a1", "sc-ident"], watch: ["sc-a1", "sc-ident"], k: "Management",
      d: "A Recovery Services vault in each landing zone subscription, in the same region as the VMs. A DeployIfNotExists policy at Landing zones turns on backup for new VMs.",
      ct: "Not emulated." },
    asr: { t: "Azure Site Recovery", ic: "gw", dep: ["sc-a1"], watch: ["sc-a1", "sc-onprem"], k: "Management",
      d: "Replicates VMs to a second region for disaster recovery. It can also replicate on-premises servers into Azure. The vault sits in the target region.",
      ct: "Not emulated." },
    kv: { t: "Azure Key Vault", ic: "vault", dep: ["sc-mgmt", "sc-conn", "sc-ident", "sc-a1", "sc-online"], watch: [], k: "Security",
      d: "One vault per app, per environment, per region. Platform subscriptions have their own. Use Azure RBAC, soft delete and purge protection, and private endpoints in Corp. Logs go to the central workspace.",
      ct: "Emulated: vaults with RBAC or access policies, soft delete, purge protection and a secrets data plane. The Arena's <code>seats.tf</code> gives every event a vault." },
    policy: { t: "Azure Policy", ic: "policy", dep: ["sc-contoso"], watch: ALL, k: "Governance",
      d: "Definitions and initiatives stored at Contoso. Assignments at Contoso, Platform, Landing zones and their children. It denies bad changes, audits drift and deploys missing settings such as diagnostics.",
      ct: "Emulated in depth, enforced on every write. DeployIfNotExists never deploys and remediation tasks are not run." },
    cost: { t: "Cost Management and budgets", ic: "coin", dep: ["sc-contoso", "sc-a1", "sc-sandbox"], watch: ALL, k: "Governance",
      d: "Cost analysis by management group, subscription and tag. Budgets with alerts on each landing zone and sandbox. A budget warns; it does not stop spending. Tags such as cost centre (enforced by policy) make the split easy.",
      ct: "Not emulated yet. Synthetic costs are a planned CloudTyke phase." }
  };
  var SC = {
    "sc-contoso": [176, 20, 190, 64, "Contoso", "management group", "folder", "root"],
    "sc-mgmt": [16, 150, 150, 70, "Management", "subscription", "chart", "plat"],
    "sc-conn": [176, 150, 150, 70, "Connectivity", "subscription", "net", "plat"],
    "sc-ident": [336, 150, 150, 70, "Identity", "subscription", "user", "plat"],
    "sc-a1": [16, 290, 150, 70, "Landing zone A1", "Corp", "sub", "lz"],
    "sc-online": [176, 290, 150, 70, "Online LZ", "internet-facing", "globe", "lz"],
    "sc-sandbox": [336, 290, 150, 70, "Sandbox", "subscription", "flask", "sand"],
    "sc-onprem": [16, 430, 310, 62, "On-premises servers", "connected with Azure Arc", "building", "onprem"]
  };
  var DD = {};
  Object.keys(SV).forEach(function (k) { DD[k] = SV[k]; });
  Object.keys(SC).forEach(function (k) {
    var c = SC[k], deps = [], w = [];
    Object.keys(SV).forEach(function (s) { if (SV[s].dep.indexOf(k) >= 0) deps.push(SV[s].t); if (SV[s].watch.indexOf(k) >= 0) w.push(SV[s].t); });
    DD[k] = { k: "Scope", t: c[4] + (c[5] === "subscription" ? " subscription" : ""), d: "What is deployed or configured here, and what keeps an eye on it.", s: [["Deployed or configured here", deps.length ? deps : ["Nothing from this list"]], ["Watched or protected by", w.length ? w : ["Only its own diagnostics"]]] };
  });
  var s = "";
  s += A.zone(8, 120, 490, 254, "--az-plat", "");
  s += A.text(16, 140, "Platform", "t-band") + A.text(16, 280, "Landing zones and sandbox", "t-band");
  s += A.path([[271, 84], [271, 112]], "") + A.arrow(271, 118, "down");
  s += '<text x="271" y="104" class="t-xs" dx="8">policy, Defender plans, budgets flow down</text>';
  Object.keys(SC).forEach(function (k) {
    var c = SC[k];
    s += A.node({ id: k, x: c[0], y: c[1], w: c[2], h: c[3], t: c[4], s: c[5], ic: c[6], k: c[7],
      extra: '<text class="t-xs scb" data-badge="' + k + '" x="' + (c[0] + 38) + '" y="' + (c[1] + c[3] - 9) + '" style="font-weight:600"></text>' });
  });
  s += A.text(16, 520, "Pick a service on the right.", "t-s");
  s += A.text(16, 538, "Orange solid = deployed or configured here. Orange dashed = watched or protected.", "t-xs");
  var keys = Object.keys(SV);
  s += A.text(560, 14, "Services", "t-band");
  keys.forEach(function (k, i) {
    var col = i % 2, row = Math.floor(i / 2), x = 560 + col * 216, y = 24 + row * 98;
    s += A.node({ id: k, x: x, y: y, w: 204, h: 84, t: A.wrap(SV[k].t, 18), s: SV[k].k, ic: SV[k].ic,
      k: SV[k].k === "Security" ? "sec" : SV[k].k === "Governance" ? "gov" : "plat" });
  });
  var svg = document.getElementById("ops");
  svg.innerHTML = s;
  A.bind(svg, DD, document.getElementById("ops-panel"), { first: "law", dim: true, dimAlways: true, onSelect: function (id) {
    svg.querySelectorAll(".scb").forEach(function (t) { t.textContent = ""; });
    A.mark(svg, [], "on");
    var v = SV[id];
    if (!v) { svg.classList.remove("dimming"); return; }
    A.mark(svg, v.dep, "on");
    v.watch.forEach(function (w) { if (v.dep.indexOf(w) < 0) svg.querySelectorAll('[data-node="' + w + '"]').forEach(function (n) { n.classList.add("rel"); }); });
    v.dep.forEach(function (w) { var t = svg.querySelector('[data-badge="' + w + '"]'); if (t) t.textContent = "deployed here"; });
    v.watch.forEach(function (w) { var t = svg.querySelector('[data-badge="' + w + '"]'); if (t && !t.textContent) t.textContent = "watched"; });
  } });
})();
