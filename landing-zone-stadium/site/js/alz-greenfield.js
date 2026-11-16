/* Azure landing zones, scenario A: start fresh. Build order (step-through) and an effort model. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  /* [id, title, start, length, lane, colour family, design areas, detail] (time in weeks, illustrative) */
  var BARS = [
    ["b1", "Billing, tenant", 0, 1, 0, "plat", "A", "Pick the agreement, set up billing scopes, confirm the one production tenant, create break-glass accounts."],
    ["b2", "Management groups and policy", 1, 2, 0, "gov", "C, G", "Deploy the hierarchy (Contoso, Platform, Landing zones, Corp, Online, Sandbox, Decommissioned) and the default policy assignments. The Azure landing zones accelerator does this from Bicep or Terraform."],
    ["b3", "Identity and access", 1, 2, 1, "id", "B", "Groups and custom roles, PIM for privileged roles, Conditional Access, identity subscription if apps need AD DS."],
    ["b4", "Management and security", 2, 2, 2, "sec", "E, F", "Log Analytics workspace, Defender for Cloud plans, Sentinel, baseline alerts, update and backup policies."],
    ["b5", "Connectivity hub", 2, 3, 3, "net", "D", "IP plan, hub VNet or Virtual WAN, Azure Firewall, gateways, ExpressRoute or VPN to the office, private DNS zones, Bastion."],
    ["b6", "Vending and pipelines", 4, 2, 1, "dev", "H", "Subscription vending code, pipeline identities with workload identity federation, repository templates for app teams."],
    ["b7", "First workload: Contoso Claims", 6, 3, 4, "lz", "", "Vend a Corp landing zone in a day. The team spends its time on the app, not on networks, logging or security reviews."],
    ["b8", "Next workloads, in parallel", 7, 3, 5, "lz", "", "Each new team gets the same ready landing zone. Several start at once, because nothing waits on hand-built plumbing."]
  ];
  var LANES = ["Platform: governance", "Platform: identity", "Platform: management", "Platform: network", "App team 1", "App teams 2, 3, 4"];
  var D = {};
  BARS.forEach(function (b) { D[b[0]] = { k: (b[6] ? "Design areas " + b[6] + " · " : "") + "weeks " + (b[2] + 1) + " to " + (b[2] + b[3]) + " (example plan)", t: b[1], d: b[7] }; });
  D.b7.ct = "Vend-like steps run on CloudTyke: <code>bash landing-zone-stadium/scripts/up.sh</code> builds the platform, then a landing zone per event.";
  D.b2.ct = "Management groups come from the seed (read-only). Policy at those groups is fully enforced.";
  var X0 = 190, WK = 84, s = "";
  for (var w = 0; w <= 10; w++) {
    s += '<line x1="' + (X0 + w * WK) + '" x2="' + (X0 + w * WK) + '" y1="34" y2="' + (44 + LANES.length * 52) + '" stroke="var(--line)" stroke-width="1"/>';
    if (w < 10) s += A.text(X0 + w * WK + WK / 2, 26, "wk " + (w + 1), "t-xs", "middle");
  }
  LANES.forEach(function (l, i) { s += A.text(16, 74 + i * 52, l, "t-s t-ink", null, 14, 186); });
  BARS.forEach(function (b) {
    s += A.node({ id: b[0], x: X0 + b[2] * WK + 3, y: 50 + b[4] * 52, w: b[3] * WK - 6, h: 44, t: A.wrap(b[1], Math.floor((b[3] * WK - 22) / 6.6)), k: b[5], tc: "t-s t-ink", tlh: 14, rx: 8 });
  });
  s += '<path class="edge dash" d="M' + (X0 + 6 * WK) + ' 40V' + (44 + LANES.length * 52) + '"/>' + A.text(X0 + 6 * WK + 6, 44 + LANES.length * 52 + 16, "platform ready: vend landing zones on demand", "t-xs");
  var svg = document.getElementById("order");
  svg.innerHTML = s;
  var api = A.bind(svg, D, document.getElementById("order-panel"), { dim: true, dimAlways: true });
  A.stepper(document.getElementById("order-steps"), BARS.map(function (b) { return { t: b[1], d: b[7] }; }), function (i) {
    BARS.forEach(function (b, k) {
      var n = svg.querySelector('[data-node="' + b[0] + '"]');
      n.classList.toggle("off", i >= 0 && k > i);
      n.classList.toggle("done", k < i);
    });
    if (i >= 0) api.select(BARS[i][0], true); else { api.clear(); svg.classList.remove("dimming"); }
  }, { startText: "Press Next step to build the platform piece by piece. The timings are an example plan, not a promise." });

  /* ---- effort model ---- */
  var n = document.getElementById("wl"), pf = document.getElementById("pf"), per = document.getElementById("per"), chart = document.getElementById("effort");
  function model() {
    var N = +n.value, P = +pf.value, S = +per.value, V = 0.5;
    document.getElementById("wl-v").textContent = N; document.getElementById("pf-v").textContent = P; document.getElementById("per-v").textContent = S;
    var maxY = Math.max(P + V * 12, S * 12), W = 640, H = 260, x0 = 54, y0 = 290;
    var sx = function (k) { return x0 + (k / 12) * W; }, sy = function (v) { return y0 - (v / maxY) * H; };
    var c = "";
    for (var k = 0; k <= 12; k += 2) c += '<line x1="' + sx(k) + '" x2="' + sx(k) + '" y1="' + (y0 - H) + '" y2="' + y0 + '" stroke="var(--line)"/>' + A.text(sx(k), y0 + 18, String(k), "t-xs", "middle");
    c += '<line x1="' + x0 + '" x2="' + (x0 + W) + '" y1="' + y0 + '" y2="' + y0 + '" stroke="var(--muted)"/>';
    c += A.text(x0 + W / 2, y0 + 38, "number of workloads", "t-xs", "middle");
    c += '<text class="t-xs" transform="translate(16 ' + (y0 - H / 2) + ') rotate(-90)" text-anchor="middle">setup effort (team-weeks)</text>';
    var lz = [], no = [];
    for (k = 0; k <= 12; k++) { lz.push([sx(k), sy(P + V * k)]); no.push([sx(k), sy(S * k)]); }
    c += A.path(no, "", ' style="stroke:var(--az-no);stroke-width:3"') + A.path(lz, "", ' style="stroke:var(--az-plat);stroke-width:3"');
    c += '<line x1="' + sx(N) + '" x2="' + sx(N) + '" y1="' + (y0 - H) + '" y2="' + y0 + '" stroke="var(--az-flow)" stroke-width="2" stroke-dasharray="4 4"/>';
    var a = P + V * N, b = S * N;
    c += '<circle cx="' + sx(N) + '" cy="' + sy(a) + '" r="6" fill="var(--az-plat)"/><circle cx="' + sx(N) + '" cy="' + sy(b) + '" r="6" fill="var(--az-no)"/>';
    c += A.text(x0 + W + 8, sy(P + V * 12) + 4, "with a landing zone", "t-s", null, 14) + A.text(x0 + W + 8, sy(S * 12) + 4, "without", "t-s", null, 14);
    chart.innerHTML = c;
    var be = S > V ? Math.ceil(P / (S - V)) : null;
    document.getElementById("effort-out").innerHTML = "<p>With <strong>" + N + "</strong> workload" + (N === 1 ? "" : "s") + ": about <strong>" + a.toFixed(1) + "</strong> team-weeks with a landing zone, <strong>" + b.toFixed(1) + "</strong> without.</p>" +
      (be ? "<p>On these numbers the landing zone pays back from workload <strong>" + be + "</strong>. " + (N >= be ? "You are past that point." : "You are not there yet: start small and grow the platform.") + "</p>" : "") +
      '<p class="small muted">A teaching model, not a benchmark. Change the assumptions to match your team. It also leaves out what is hard to count: every hand-built environment is a little different, so audits, incidents and support cost more.</p>';
  }
  [n, pf, per].forEach(function (el) { el.addEventListener("input", model); });
  model();
})();
