/* Azure landing zones, scenarios D and E: modernise inside a landing zone; sandbox to production to decommission. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;

  /* ---------------- figure 1: modernise the workload, keep the landing zone ---------------- */
  var VIEWS = {
    before: { app: [["web1", "Web VM 1", "vm"], ["web2", "Web VM 2", "vm"], ["appvm", "App server VM", "vm"]], data: [["sqlvm", "SQL Server on a VM", "db"]], extra: [["lb", "Internal load balancer", "lb"]] },
    aks: { app: [["aks", "AKS cluster", "wheel"], ["wi", "Workload identity", "key"], ["acr", "Images from the shared registry", "cube"]], data: [["azsql", "Azure SQL Database", "db"]], extra: [["pe", "Private endpoint", "lock"]] },
    aca: { app: [["aca", "Container Apps environment", "cube"], ["mi", "Managed identity", "key"], ["acr", "Images from the shared registry", "cube"]], data: [["azsql", "Azure SQL Database", "db"]], extra: [["pe", "Private endpoint", "lock"]] }
  };
  var STAY = [["peer", "Spoke peered to the hub", "net"], ["udr", "Route to the firewall", "fw"], ["pol", "Inherited policy", "policy"], ["logs", "Diagnostics to central logs", "log"], ["def", "Defender for Cloud", "shieldcheck"], ["rbac", "Team RBAC groups", "users"]];
  var D = {
    web1: { k: "Before", t: "Web VMs", d: "IIS on Windows Server VMs behind a load balancer. The team patches them (Update Manager helps) and scales by hand." },
    web2: { k: "Before", t: "Web VMs", d: "Two copies for availability, in different availability zones." },
    appvm: { k: "Before", t: "App server VM", d: "Business logic as a Windows service. One VM, so one point of failure." },
    sqlvm: { k: "Before", t: "SQL Server on a VM", d: "The team handles patching, backups and high availability. Licence costs sit with the VM." },
    lb: { k: "Before", t: "Internal load balancer", d: "Spreads traffic over the web VMs." },
    aks: { k: "After (option 1)", t: "AKS cluster", d: "Containers on managed Kubernetes. Fits teams that need Kubernetes features and run many services. In a landing zone: private API server, egress through the hub firewall, Entra ID with Azure RBAC, Azure Policy add-on.", ct: "AKS is emulated (k3s or an API stub): <code>examples/terraform/landing-zone/aks.tf</code> builds one with UDR egress." },
    aca: { k: "After (option 2)", t: "Azure Container Apps", d: "Serverless containers with scale to zero, revisions and Dapr. Less to run than AKS. A workload profiles environment sits in the spoke and can route egress through the firewall.", ct: "Not emulated." },
    wi: { k: "After", t: "Workload identity", d: "Pods sign in to Azure SQL and Key Vault with a federated managed identity. No connection-string passwords.", ct: "Federated credentials are emulated." },
    mi: { k: "After", t: "Managed identity", d: "The app signs in to Azure SQL and Key Vault without secrets." },
    acr: { k: "After", t: "Container images", d: "Built by the team's pipeline, scanned, pulled from a registry over a private endpoint.", ct: "Container Registry is emulated; the Arena has a shared kit room registry." },
    azsql: { k: "After", t: "Azure SQL Database", d: "Managed database: patching, backups, point-in-time restore and zone redundancy built in. Entra-only authentication. Public access off in Corp. Use Azure Database Migration Service or Azure Migrate guidance to move the data.", ct: "Azure SQL is not emulated. PostgreSQL flexible server is, if you want to practise private access." },
    pe: { k: "After", t: "Private endpoint", d: "The database gets a private IP in the spoke. The record in the central private DNS zone is written by policy." },
    peer: { k: "Unchanged", t: "Hub peering", d: "The landing zone's network did not change. Only what runs inside it did." },
    udr: { k: "Unchanged", t: "Route to the firewall", d: "Egress still goes through the hub. AKS and Container Apps need firewall rules for their outbound dependencies." },
    pol: { k: "Unchanged", t: "Inherited policy", d: "The same guardrails check the new services: no public endpoints in Corp, TLS settings, Kubernetes rules (no privileged containers)." },
    logs: { k: "Unchanged", t: "Central logs", d: "Diagnostic settings for the new resources are created by the same policy." },
    def: { k: "Unchanged", t: "Defender for Cloud", d: "Defender for Containers and Defender for SQL cover the new services, turned on by policy." },
    rbac: { k: "Unchanged", t: "Team RBAC", d: "Same groups, same subscription. No new access request." }
  };
  var svg1 = document.getElementById("mod"), view = "before", api1;
  function draw1() {
    var v = VIEWS[view], s = "";
    s += A.zone(16, 16, 640, 330, "--az-lz", "Landing zone A1 · Corp · the workload");
    s += A.text(36, 60, "App tier", "t-band") + A.text(36, 200, "Data tier", "t-band");
    v.app.forEach(function (n, i) { s += A.node({ id: n[0], x: 36 + i * 204, y: 72, w: 192, h: 56, t: A.wrap(n[1], 20), ic: n[2], k: view === "before" ? "onprem" : "lz", tc: "t-s t-ink", tlh: 14 }); });
    v.extra.forEach(function (n) { s += A.node({ id: n[0], x: 36, y: 140, w: 192, h: 40, t: n[1], ic: n[2], k: "chip", tc: "t-s t-ink", isz: 16 }); });
    v.data.forEach(function (n) { s += A.node({ id: n[0], x: 36, y: 212, w: 396, h: 56, t: n[1], ic: n[2], k: view === "before" ? "onprem" : "lz" }); });
    s += A.text(36, 300, view === "before" ? "Rehosted as-is in an earlier migration wave." : "Same subscription, same spoke. New services inside it.", "t-xs");
    s += A.zone(680, 16, 404, 330, "--az-plat", "Stays the same");
    STAY.forEach(function (n, i) { s += A.node({ id: n[0], x: 700, y: 50 + i * 48, w: 364, h: 40, t: n[1], ic: n[2], k: "plat", tc: "t-s t-ink", isz: 16 }); });
    svg1.innerHTML = s;
    if (api1) api1.refresh();
  }
  draw1();
  api1 = A.bind(svg1, D, document.getElementById("mod-panel"), { first: "pol" });
  A.seg(document.getElementById("mod-view"), "Show:", [["before", "Before: VMs and SQL Server"], ["aks", "After: AKS and Azure SQL"], ["aca", "After: Container Apps and Azure SQL"]], function (v) {
    view = v;
    if (api1 && api1.current && !D[api1.current]) api1.current = null;
    var ids = VIEWS[v].app.concat(VIEWS[v].data, VIEWS[v].extra).map(function (n) { return n[0]; }).concat(STAY.map(function (n) { return n[0]; }));
    if (api1 && ids.indexOf(api1.current) < 0) api1.current = "pol";
    draw1();
  });

  /* ---------------- figure 2: lifecycle ---------------- */
  var COLS = [["Sandbox", "--az-sand", "sand"], ["Corp: non-production", "--az-lz", "lz"], ["Corp: production", "--az-lz", "lz"], ["Decommissioned", "--az-decom", "decom"], ["Gone", "--az-decom", "mute"]];
  var LIFE = [
    { t: "Prove the idea in a sandbox", d: "A developer tries an AI claims triage idea in Sandbox subscription 1. Loose rules on services, no link to the hub, a small budget with alerts.", at: [[0, "Claims PoC", "budget alert at 80%"]] },
    { t: "Vend a real landing zone", d: "The idea works. The team asks for a Corp non-production landing zone through vending. They do not move the sandbox subscription: it was built under looser rules and may hold anything.", at: [[0, "Claims PoC", "still running"], [1, "Claims non-prod", "vended today"]] },
    { t: "Redeploy from code", d: "The pipeline deploys the app into non-production. Every resource now passes the Corp policy: private endpoints, no public IPs, central logs. Anything that fails policy gets fixed here, not in production.", at: [[0, "Claims PoC", "to be deleted"], [1, "Claims non-prod", "passes policy"]] },
    { t: "Promote to production", d: "A second vended landing zone for production. The same pipeline promotes the same code with approvals. The sandbox is emptied and its subscription reused or cancelled.", at: [[1, "Claims non-prod", "test and staging"], [2, "Claims prod", "live"]] },
    { t: "Years later: retire it", d: "The app is replaced. Its subscriptions move to Decommissioned. Policy denies new resources and shuts VMs down. Access drops to read-only. Data is exported or kept as the retention policy says.", at: [[3, "Claims prod", "read-only"], [3, "Claims non-prod", "read-only"]] },
    { t: "Delete", d: "After the cooling-off period your process sets, the subscriptions are cancelled. Azure keeps a cancelled subscription disabled for a while before it deletes it, so check the current terms before you count on a restore.", at: [[4, "Claims", "cancelled"]] }
  ];
  var DL = {};
  LIFE.forEach(function (l, i) { DL["l" + i] = { k: "Lifecycle step " + (i + 1), t: l.t, d: l.d }; });
  DL.l0.ct = "<code>ct-sandbox</code> and <code>sub-sandbox</code> exist in the seed.";
  DL.l4.ct = "There is no Decommissioned group in the seed, and subscriptions cannot be moved. Practise the guardrail instead: a custom policy at a group that denies every new resource type except a short allow-list.";
  var svg2 = document.getElementById("life"), api2;
  function draw2(i) {
    var s = "";
    COLS.forEach(function (c, k) {
      var x = 16 + k * 216;
      s += A.zone(x, 16, 200, 220, c[1], c[0], "t-xs");
      if (k < COLS.length - 1) s += A.path([[x + 200, 126], [x + 216, 126]], "thin");
    });
    if (i >= 0) LIFE[i].at.forEach(function (a, j) {
      var col = a[0], x = 16 + col * 216 + 12, same = LIFE[i].at.filter(function (b) { return b[0] === col; }).length > 1;
      var y = same ? 50 + j * 86 : 92;
      s += A.node({ id: "l" + i, x: x, y: y, w: 176, h: 70, t: a[1], s: a[2], ic: col === 0 ? "flask" : col >= 3 ? "archive" : "sub", k: COLS[col][2], cls: "on", label: a[1] + ", " + a[2] });
    });
    var lab = ["", "vending", "same code", "", "move group", "cancel"];
    if (i >= 1 && lab[i]) { var from = i === 4 ? 2 : i === 5 ? 3 : i === 3 ? 1 : 0; s += '<g style="color:var(--az-flow)">' + A.icon("arrow", 16 + from * 216 + 208, 112, 18) + "</g>" + A.text(16 + from * 216 + 208, 150, lab[i], "t-xs", "middle"); }
    svg2.innerHTML = s;
    if (api2) api2.refresh();
  }
  draw2(0);
  api2 = A.bind(svg2, DL, document.getElementById("life-panel"), { empty: '<p class="hint">Press Next step.</p>' });
  A.stepper(document.getElementById("life-steps"), LIFE, function (i) { draw2(i); if (i >= 0) api2.select("l" + i, true); else api2.clear(); }, { start: 0 });
})();
