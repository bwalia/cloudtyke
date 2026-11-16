/* Azure landing zones: benefits, shown as sprawl vs governed, with the evidence for each. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  var SPRAWL = [
    ["sw1", 40, 40, "web-prod", "subscription", "pip", "Public IP on a VM, RDP open"],
    ["sw2", 330, 24, "team-x-dev", "subscription", "owner", "Owner: 40 people"],
    ["sw3", 640, 56, "finance", "subscription", "overlap", "10.0.0.0/16, again"],
    ["sw4", 100, 214, "test-old", "subscription", "logs", "Logs: none"],
    ["sw5", 410, 196, "data-lake", "subscription", "cost", "Cost owner: unknown"],
    ["sw6", 720, 232, "new-app", "subscription", "ticket", "Waiting weeks for a network"]
  ];
  var D = {
    sw1: { k: "Sprawl", t: "web-prod", d: "Built by hand two years ago. Nobody is sure what else runs in it." },
    sw2: { k: "Sprawl", t: "team-x-dev", d: "A dev subscription that slowly became production." },
    sw3: { k: "Sprawl", t: "finance", d: "Its own VNet with the same range as two others, so it can never be joined to them." },
    sw4: { k: "Sprawl", t: "test-old", d: "Forgotten. Still billing." },
    sw5: { k: "Sprawl", t: "data-lake", d: "Storage with public access, created for a one-off export." },
    sw6: { k: "Sprawl", t: "new-app", d: "A new team waits while network, security and access are built by hand, again." },
    pip: { k: "Problem", t: "Public IP, RDP open to the internet", d: "One of the most common ways in for attackers. Nothing stopped it being created." },
    owner: { k: "Problem", t: "Owner for 40 people", d: "Anyone can change anything, including access and logs." },
    overlap: { k: "Problem", t: "Overlapping address space", d: "VNets with the same range cannot be peered. Joining them later means re-addressing." },
    logs: { k: "Problem", t: "No logs", d: "When something goes wrong, there is no record of who did what." },
    cost: { k: "Problem", t: "No cost owner", d: "No tags, no budget. The bill grows and nobody can say why." },
    ticket: { k: "Problem", t: "Waiting weeks", d: "Every new app rebuilds the same plumbing, by hand, a little differently each time." },
    nopol: { k: "Problem", t: "No shared rules", d: "Each team sets its own rules, or none. Auditors find a different answer in every subscription." },
    "g-mg": { k: "Governed", t: "Management groups and Azure Policy", d: "One tree, one set of rules, checked by Azure on every change.", s: [["Evidence", '<a href="hierarchy.html">Management groups, figures 1 and 2</a>']] },
    "g-comp": { k: "Governed", t: "Compliance dashboard", d: "Azure Policy compliance across the whole estate, plus regulatory initiatives in Defender for Cloud.", s: [["Evidence", '<a href="brownfield.html">Scenario C: audit, then enforce</a>']] },
    "g-cost": { k: "Governed", t: "Budgets and cost-centre tags", d: "Every landing zone has an owner, a cost centre tag (required by policy) and a budget with alerts.", s: [["Evidence", '<a href="management-security.html">Management and security</a>']] },
    "g-hub": { k: "Governed", t: "The hub", d: "One way in and out, inspected by the firewall. No public IPs in Corp. Admin access through Bastion.", s: [["Evidence", '<a href="network.html">Network topology: packet paths</a>']] },
    "g-lz1": { k: "Governed", t: "Landing zone", d: "Same baseline as every other: spoke, peering, routes, RBAC groups, policy, logs.", s: [["Evidence", '<a href="platform-vs-app.html">Platform vs application</a>']] },
    "g-logs": { k: "Governed", t: "Central logs and security", d: "Every resource logs to one workspace. Defender for Cloud and Sentinel watch everything.", s: [["Evidence", '<a href="management-security.html">Management and security</a>']] },
    "g-vend": { k: "Governed", t: "Subscription vending", d: "A new, compliant landing zone from a request, in hours, from code.", s: [["Evidence", '<a href="vending.html">Subscription vending</a>, <a href="greenfield.html">Scenario A</a>']] }
  };
  D["g-lz2"] = D["g-lz1"]; D["g-lz3"] = D["g-lz1"];
  var BEN = {
    sec: { t: "Security by default", sprawl: ["pip", "owner"], gov: ["g-hub", "g-logs"], d: "Guardrails stop risky builds before they happen, and every landing zone is watched from day one. Before: an open RDP port and 40 owners. After: a hub firewall, no public IPs in Corp, central monitoring." },
    gov: { t: "Consistent governance", sprawl: ["nopol"], gov: ["g-mg"], d: "One set of rules, written once, inherited everywhere. Teams do not have to remember them." },
    speed: { t: "Faster delivery", sprawl: ["ticket"], gov: ["g-vend"], d: "Vending turns a request into a ready landing zone in hours. Teams spend their time on the app, not the plumbing." },
    cost: { t: "Cost visibility", sprawl: ["cost", "sw4"], gov: ["g-cost"], d: "Tags, budgets and a clear owner for every subscription. Forgotten subscriptions are found and decommissioned." },
    comp: { t: "Compliance", sprawl: ["nopol", "logs"], gov: ["g-comp", "g-mg", "g-logs"], d: "Policy compliance and audit logs give auditors one answer for the whole estate." },
    scale: { t: "Scale", sprawl: ["overlap", "ticket"], gov: ["g-lz1", "g-lz2", "g-lz3", "g-vend"], d: "Identical landing zones from code, with a planned IP space, so the hundredth is as easy as the second." }
  };
  var svg = document.getElementById("ben"), view = "gov", api, cur = null;
  function draw() {
    var s = "";
    if (view === "sprawl") {
      SPRAWL.forEach(function (n) {
        s += A.node({ id: n[0], x: n[1], y: n[2], w: 230, h: 62, t: n[3], s: n[4], ic: "sub", k: "mute", cls: "sprawl" });
        s += A.node({ id: n[5], x: n[1], y: n[2] + 70, w: 230, h: 32, t: n[6], ic: "x", isz: 14, k: "chip", tc: "t-s t-ink", rx: 8, cls: "prob" });
      });
      s += A.node({ id: "nopol", x: 40, y: 360, w: 910, h: 40, t: "No shared policy: every team sets its own rules, or none", ic: "x", k: "chip", tc: "t-s t-ink", cls: "prob" });
    } else {
      s += A.node({ id: "g-mg", x: 40, y: 24, w: 290, h: 60, t: "Management groups + Policy", s: "one tree, one set of rules", ic: "policy", k: "gov" });
      s += A.node({ id: "g-comp", x: 350, y: 24, w: 290, h: 60, t: "Compliance: 96%", s: "across the estate", ic: "check", k: "gov" });
      s += A.node({ id: "g-cost", x: 660, y: 24, w: 290, h: 60, t: "Budgets and tags", s: "an owner for every subscription", ic: "coin", k: "gov" });
      s += A.path([[330, 165], [360, 165], [360, 222], [870, 222]], "") + A.path([[470, 222], [470, 190]], "") + A.path([[670, 222], [670, 190]], "") + A.path([[870, 222], [870, 190]], "");
      s += A.text(520, 238, "hub-to-spoke peering", "t-xs", "middle");
      s += A.node({ id: "g-hub", x: 40, y: 110, w: 290, h: 110, t: "Connectivity hub", s: ["firewall, DNS, Bastion,", "DDoS protection"], ic: "net", k: "net" });
      ["g-lz1", "g-lz2", "g-lz3"].forEach(function (id, i) { s += A.node({ id: id, x: 380 + i * 200, y: 120, w: 180, h: 70, t: "Landing zone", s: "same baseline", ic: "sub", k: "lz" }); });
      s += A.node({ id: "g-logs", x: 40, y: 270, w: 290, h: 64, t: "Central logs and security", s: "Monitor, Defender, Sentinel", ic: "shieldcheck", k: "sec" });
      s += A.node({ id: "g-vend", x: 380, y: 270, w: 570, h: 64, t: "Subscription vending", s: "a new, compliant landing zone from a request, in hours", ic: "vend", k: "dev" });
      s += A.path([[665, 270], [665, 252]], "dash") + A.arrow(665, 246, "up");
      s += A.text(40, 380, "Same company, same apps. Built on a landing zone.", "t-s");
    }
    svg.innerHTML = s;
    if (api) api.refresh();
    if (cur) highlight(cur);
  }
  function highlight(b) {
    A.mark(svg, BEN[b][view], "on");
    svg.classList.add("dimming");
  }
  draw();
  api = A.bind(svg, D, document.getElementById("ben-panel"), { empty: '<p class="hint">Pick a benefit above, or click any box.</p>' });
  var seg = A.seg(document.getElementById("ben-view"), "Show:", [["sprawl", "Before: sprawl"], ["gov", "After: governed"]], function (v) { view = v; draw(); }, "gov");
  var bb = document.getElementById("ben-btns");
  bb.innerHTML = Object.keys(BEN).map(function (k) { return '<button type="button" class="btn" aria-pressed="false" data-b="' + k + '">' + A.esc(BEN[k].t) + "</button>"; }).join("");
  bb.addEventListener("click", function (e) {
    var b = e.target.closest("[data-b]");
    if (!b) return;
    cur = b.getAttribute("data-b");
    bb.querySelectorAll("[data-b]").forEach(function (x) { x.setAttribute("aria-pressed", x === b ? "true" : "false"); });
    api.clear();
    highlight(cur);
    var ben = BEN[cur], ev = ben.gov.map(function (id) { return D[id].s ? D[id].s[0][1] : ""; }).filter(function (x, i, arr) { return x && arr.indexOf(x) === i; });
    document.getElementById("ben-panel").innerHTML = '<p class="kicker">Benefit</p><h3>' + A.esc(ben.t) + "</h3><p>" + ben.d + '</p><div class="pb"><h4>Before</h4><ul>' + ben.sprawl.map(function (id) { return "<li>" + A.esc(D[id].t) + "</li>"; }).join("") + '</ul></div><div class="pb"><h4>After</h4><ul>' + ben.gov.filter(function (id, i, arr) { return D[id] !== D[arr[i - 1]]; }).map(function (id) { return "<li>" + A.esc(D[id].t) + "</li>"; }).join("") + '</ul></div><div class="pb"><h4>Evidence in this section</h4><ul>' + ev.map(function (x) { return "<li>" + x + "</li>"; }).join("") + "</ul></div>" +
      '<p class="small"><button type="button" class="btn" id="ben-flip">Show ' + (view === "gov" ? "before" : "after") + "</button></p>";
    document.getElementById("ben-flip").addEventListener("click", function () { seg.set(view === "gov" ? "sprawl" : "gov"); document.getElementById("ben-flip").textContent = "Show " + (view === "gov" ? "before" : "after"); });
  });
})();
