/* Azure landing zones: subscription vending as a step-through. */
(function () {
  "use strict";
  ALZ.boot();
  var A = ALZ;
  var STEPS = [
    { t: "Request", ic: "log", d: "The Contoso Claims team asks for a production landing zone. They fill in a short form, or open a pull request that adds one file to the vending repository: name, archetype (Corp), region, network size, owners and budget.", f: [0, 1, 2, 3, 4, 5] },
    { t: "Approve", ic: "approve", d: "Automated checks run first: naming, a free IP range, a valid cost centre. Then a person approves: the platform team, and the budget owner. With a pull request, approval is the merge.", f: [5] },
    { t: "Create subscription", ic: "sub", d: "A pipeline with its own managed identity calls the subscription alias API under the billing account (an EA enrollment account or an MCA invoice section). A new, empty subscription appears.", f: [0, 2] },
    { t: "Place in group", ic: "folder", d: "The subscription moves under <strong>Corp</strong>. At once it inherits every policy from Contoso, Landing zones and Corp: allowed regions, no public IPs, diagnostics, Defender plans.", f: [1] },
    { t: "Network", ic: "net", d: "A spoke VNet is created from the requested size, with a free range from the IP plan. It is peered to the hub (or connected to the Virtual WAN hub), with a route table to the firewall and the hub DNS servers.", f: [3] },
    { t: "Access", ic: "key", d: "Entra groups for owners, contributors and readers get their roles. A user-assigned managed identity with a federated credential lets the team's GitHub or Azure DevOps pipeline deploy, with no secret.", f: [4] },
    { t: "Policy and budget", ic: "policy", d: "Anything specific to this subscription: a budget with alerts at 80% and 100%, tags such as cost centre, and any extra policy. Everything else came from the management groups.", f: [5] },
    { t: "Hand over", ic: "check", d: "The team gets a message with links: the subscription, the repository and pipeline, the network details. They start building on the same day. Every landing zone looks the same, so support is simpler too." }
  ];
  var FIELDS = [["Name", "Contoso Claims (production)"], ["Archetype", "Corp"], ["Region", "UK South"], ["Network", "/24 from the IP plan"], ["Owners", "grp-claims-owners"], ["Budget", "£2,000 a month, cost centre CC-417"]];
  var D = {};
  STEPS.forEach(function (st, i) { D["st" + i] = { k: "Vending step " + (i + 1), t: st.t, d: st.d }; });
  var s = "";
  STEPS.forEach(function (st, i) {
    var x = 16 + i * 134;
    s += A.node({ id: "st" + i, x: x, y: 20, w: 122, h: 78, t: st.t, s: "step " + (i + 1), ic: st.ic, icTop: true, k: "dev", tc: "t-s t-ink", tlh: 14 });
    if (i < STEPS.length - 1) s += A.path([[x + 122, 59], [x + 134, 59]], "thin");
  });
  /* request card */
  s += '<rect x="16" y="132" width="400" height="300" rx="14" class="band"/>' + A.text(32, 156, "The request", "t-band");
  FIELDS.forEach(function (f, i) {
    var y = 176 + i * 40;
    s += '<g data-id="f' + i + '" class="fld"><rect x="30" y="' + y + '" width="372" height="32" rx="8" class="fld-box"/>' + A.text(42, y + 21, f[0], "t-xs") + A.text(130, y + 21, f[1], "t-s t-ink", null, 14, 262) + "</g>";
  });
  /* tree */
  s += A.zone(450, 132, 634, 300, "--az-lz", "Landing zones management group");
  s += A.node({ id: "corp", x: 610, y: 168, w: 150, h: 42, t: "Corp", k: "lz", ic: "folder" });
  s += A.node({ id: "hub", x: 928, y: 168, w: 140, h: 42, t: "Hub", k: "net", ic: "net" });
  s += A.elbow(685, 210, 540, 258, "", 234) + A.elbow(685, 210, 685, 258, "", 234);
  s += '<g data-s="3">' + A.elbow(685, 210, 830, 258, "hot", 234) + "</g>";
  s += A.node({ id: "a1", x: 470, y: 258, w: 140, h: 56, t: "Landing zone A1", s: "existing", ic: "sub", k: "lz", tc: "t-s t-ink" });
  s += A.node({ id: "a2", x: 615, y: 258, w: 140, h: 56, t: "Landing zone A2", s: "existing", ic: "sub", k: "lz", tc: "t-s t-ink" });
  s += '<g data-s="2">' + A.node({ id: "a3", x: 760, y: 258, w: 140, h: 56, t: "Claims (prod)", s: "new subscription", ic: "sub", k: "dev", tc: "t-s t-ink" }) + "</g>";
  s += '<g data-s="4">' + A.path([[998, 210], [998, 286], [900, 286]], "hot") + A.text(1004, 252, "peering", "t-xs") + "</g>";
  var chips = [[4, "spoke 10.20.3.0/24", "net"], [5, "groups and pipeline identity", "key"], [6, "budget and tags", "coin"], [7, "ready for the team", "check"]];
  chips.forEach(function (c, i) {
    s += '<g data-s="' + c[0] + '">' + A.node({ id: "c" + c[0], x: 470 + (i % 2) * 300, y: 340 + Math.floor(i / 2) * 42, w: 290, h: 34, t: c[1], ic: c[2], isz: 16, k: "chip", tc: "t-s t-ink", rx: 8 }) + "</g>";
    D["c" + c[0]] = D["st" + c[0]];
  });
  D.corp = { k: "Management group", t: "Corp", d: "Archetype for internal apps. Vending places the subscription here, so the Corp policies apply from minute one." };
  D.hub = { k: "Connectivity subscription", t: "Hub", d: "Vending peers the new spoke to the hub in both directions. The hub side needs rights in the connectivity subscription, so the vending identity holds a narrow role there." };
  D.a1 = { k: "Subscription", t: "Landing zone A1", d: "Vended earlier with the same code, so it looks just like the new one." };
  D.a2 = D.a1;
  D.a3 = { k: "Subscription", t: "Contoso Claims (production)", d: "The new landing zone, built only from code.", ct: "CloudTyke cannot create subscriptions (no <code>Microsoft.Subscription/aliases</code>). Add one to a copy of the seed instead, like the Arena's <code>sub-events</code>. Steps 5 to 7 then run for real: <code>landing-zone-stadium/terraform/seats.tf</code> builds the spoke, peering, route table, DNS link, group, Key Vault and crew identity for each event with <code>for_each</code>." };
  var svg = document.getElementById("vend");
  svg.innerHTML = s;
  var api = A.bind(svg, D, document.getElementById("vend-panel"), { empty: '<p class="hint">Press Next step, or click any box.</p>' });
  A.stepper(document.getElementById("vend-steps"), STEPS, function (i) {
    svg.querySelectorAll("[data-s]").forEach(function (g) { g.classList.toggle("hidden-layer", +g.getAttribute("data-s") > i); });
    STEPS.forEach(function (st, k) {
      var n = svg.querySelector('[data-node="st' + k + '"]');
      n.classList.toggle("on", k === i); n.classList.toggle("done", k < i);
    });
    svg.querySelectorAll(".fld").forEach(function (f) { f.classList.remove("hot"); });
    if (i >= 0 && STEPS[i].f) STEPS[i].f.forEach(function (k) { svg.querySelector('[data-id="f' + k + '"]').classList.add("hot"); });
    if (i >= 0) api.select("st" + i, true); else api.clear();
  });
})();
