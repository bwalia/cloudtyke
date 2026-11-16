/* Chapter 7: Kubernetes is the team. Meet the team, Pack the squad, Seat the crowd. */
(function () {
  "use strict";
  var C = window.LZS;
  LZ.boot();
  LZ.registerPart("c-stand", { perm: true, name: "A stand", what: "A block of seats. Opening a stand costs stewards and cleaners.", c: ["A node in an AKS node pool", "A node in an EKS node group", "A node in a GKE node pool"], why: "Fill a stand before opening the next: that is bin packing." });

  /* ---------- Meet the team ---------- */
  var team = document.getElementById("team");
  var ORDER = ["k-control", "k-manifest", "k-scheduler", "k-pod", "k-node", "k-deploy", "k-heal", "k-bench", "k-kubelet", "k-referee", "k-registry", "k-autoscale"];
  function num(n, x, y) { return '<circle cx="' + x + '" cy="' + y + '" r="11" class="num-bg"/><text x="' + x + '" y="' + (y + 4) + '" text-anchor="middle" class="num-tx">' + n + "</text>"; }
  function drawTeam() {
    var v = LZ.state.venue, s = '<rect x="0" y="0" width="640" height="400" rx="18" class="land"/>';
    var bg = v === "cricket" ? '<ellipse cx="320" cy="165" rx="210" ry="135" class="grass"/><rect x="310" y="130" width="20" height="70" class="strip"/>'
      : v === "football" ? '<rect x="110" y="30" width="420" height="270" rx="6" class="grass"/><path d="M320 30v270" class="line"/><circle cx="320" cy="165" r="36" fill="none" class="line"/>'
      : '<rect x="110" y="30" width="420" height="270" rx="6" class="stage"/><path d="M120 44h400" class="truss"/>';
    s += bg;
    s += '<g data-part="k-node">' + [0, 1, 2].map(function (i) { return '<rect x="' + (124 + i * 134) + '" y="44" width="124" height="242" rx="8" class="node-zone"/>'; }).join("") + num(5, 136, 58) + "</g>";
    var P = [[150, 165], [215, 90], [215, 140], [215, 190], [215, 240], [300, 110], [300, 165], [300, 220], [400, 120], [400, 210], [470, 165]];
    s += '<g data-part="k-pod">' + P.map(function (p) { return '<circle cx="' + p[0] + '" cy="' + p[1] + '" r="11" class="player"/>'; }).join("") + num(4, 470, 138) + "</g>";
    s += '<g data-part="k-referee"><circle cx="360" cy="70" r="10" class="ref"/><rect x="354" y="80" width="12" height="16" rx="4" class="ref"/>' + num(10, 382, 64) + "</g>";
    s += '<g data-part="k-control"><rect x="20" y="320" width="70" height="46" rx="8" class="mgr"/><text x="55" y="348" text-anchor="middle" class="svg-small" style="fill:#fff">manager</text>' + num(1, 22, 320) + "</g>";
    s += '<g data-part="k-manifest"><rect x="100" y="318" width="40" height="52" rx="4" class="sheet"/><path d="M108 334h24M108 344h24M108 354h16" class="sheet-l"/>' + num(2, 140, 316) + "</g>";
    s += '<g data-part="k-scheduler"><path d="M70 318L140 250" class="sched"/><path d="M128 252l12-2-2 12" class="sched"/><rect x="60" y="240" width="90" height="90" class="hit"/>' + num(3, 92, 270) + "</g>";
    s += '<g data-part="k-bench"><rect x="220" y="334" width="170" height="36" rx="8" class="bench"/>' + [250, 285, 320, 355].map(function (x) { return '<circle cx="' + x + '" cy="352" r="9" class="player"/>'; }).join("") + num(8, 392, 334) + "</g>";
    s += '<g data-part="k-heal"><path d="M250 330V300" class="sub"/><path d="M243 308l7-10 7 10" class="sub"/><rect x="230" y="290" width="40" height="44" class="hit"/>' + num(7, 270, 304) + "</g>";
    s += '<g data-part="k-kubelet"><circle cx="455" cy="330" r="10" class="coach"/><rect x="449" y="340" width="12" height="18" rx="4" class="coach"/><path d="M462 330q14-8 22 0" class="sched"/>' + num(9, 480, 352) + "</g>";
    s += '<g data-part="k-deploy"><rect x="548" y="40" width="80" height="44" rx="10" class="count"/><text x="588" y="68" text-anchor="middle" class="svg-text">11 / 11</text>' + num(6, 548, 40) + "</g>";
    s += '<g data-part="k-registry"><rect x="556" y="150" width="62" height="50" rx="6" class="kitbox"/><path d="M566 166h42M566 180h42" class="shelf"/>' + num(11, 556, 150) + "</g>";
    s += '<g data-part="k-autoscale"><circle cx="560" cy="330" r="9" class="steward"/><rect x="554" y="339" width="12" height="16" rx="4" class="steward"/><circle cx="590" cy="330" r="9" class="steward"/><rect x="584" y="339" width="12" height="16" rx="4" class="steward"/><text x="618" y="340" class="svg-text">+</text>' + num(12, 600, 312) + "</g>";
    team.innerHTML = s;
    LZ.refreshParts(team);
    var ci = LZ.ci();
    document.getElementById("team-h").textContent = "Meet the team: " + LZ.venue().teamTitle.toLowerCase();
    document.getElementById("team-legend").innerHTML = ORDER.map(function (id) {
      var p = LZ.part(id);
      return '<li><button type="button" class="leg-btn" data-hl="' + id + '"><strong>' + LZ.esc(p.name) + "</strong><span>" + LZ.esc(p.c[ci]) + "</span></button></li>";
    }).join("");
    document.querySelectorAll("[data-hl]").forEach(function (b) {
      b.addEventListener("click", function () { LZ.openPanel(b.getAttribute("data-hl"), b); });
      b.addEventListener("mouseenter", function () { var el = team.querySelector('[data-part="' + b.getAttribute("data-hl") + '"]'); if (el) LZ.showTip(el); });
      b.addEventListener("mouseleave", LZ.hideTip);
    });
  }
  drawTeam();

  /* ---------- tabs ---------- */
  var tabs = [document.getElementById("tab-pack"), document.getElementById("tab-seat")];
  function selectTab(i) {
    tabs.forEach(function (t, j) {
      t.setAttribute("aria-selected", i === j ? "true" : "false");
      t.tabIndex = i === j ? 0 : -1;
      document.getElementById(t.getAttribute("aria-controls")).hidden = i !== j;
    });
    tabs[i].focus();
  }
  tabs.forEach(function (t, i) {
    t.addEventListener("click", function () { selectTab(i); });
    t.addEventListener("keydown", function (e) {
      if (e.key === "ArrowRight" || e.key === "ArrowLeft") { e.preventDefault(); selectTab(1 - i); }
    });
  });

  /* ---------- Pack the squad ---------- */
  var NODES = [["n1", "A"], ["n2", "A"], ["n3", "B"], ["n4", "B"], ["n5", "C"], ["n6", "C"]];
  var CAP = { cpu: 4, mem: 8 };
  var COL = { "ticket-api": "#2f8fd8", "fan-app": "#36b37e", clips: "#e48a3a", scoreboard: "#2f8fd8", "stats-api": "#7a5bd6", replay: "#e48a3a", "doors-app": "#2f8fd8", media: "#e48a3a", merch: "#c2577a" };
  var pods = [], selected = null, zoneDown = null, rushed = false;
  function squad() { return C.squads[LZ.venue().kit]; }
  function addPods(list) {
    list.forEach(function (a) {
      for (var i = 0; i < a[1]; i++) pods.push({ id: a[0] + "-" + (pods.length + 1), app: a[0], cpu: a[2], mem: a[3], imp: a[4], node: null });
    });
  }
  function resetPods() { pods = []; selected = null; zoneDown = null; rushed = false; addPods(squad().pods); }
  function used(n) {
    var u = { cpu: 0, mem: 0, n: 0 };
    pods.forEach(function (p) { if (p.node === n) { u.cpu += p.cpu; u.mem += p.mem; u.n++; } });
    return u;
  }
  function fits(p, n) { var u = used(n); return u.cpu + p.cpu <= CAP.cpu + 1e-9 && u.mem + p.mem <= CAP.mem + 1e-9 && zoneOf(n) !== zoneDown; }
  function zoneOf(n) { for (var i = 0; i < NODES.length; i++) if (NODES[i][0] === n) return NODES[i][1]; }
  var nodesSvg = document.getElementById("nodes");
  function drawPack() {
    var s = "", H = 52, top = 44;
    ["A", "B", "C"].forEach(function (z, i) {
      s += '<text x="' + (20 + i * 208 + 97) + '" y="20" text-anchor="middle" class="svg-text">Zone ' + z + (zoneDown === z ? " (down)" : "") + "</text>";
    });
    NODES.forEach(function (nd, i) {
      var x = 20 + i * 104, down = nd[1] === zoneDown, u = used(nd[0]);
      s += '<g data-part="k-node" data-node="' + nd[0] + '" data-label="Node ' + nd[0] + ' in zone ' + nd[1] + ', ' + u.cpu + ' of 4 CPU used"><rect x="' + x + '" y="' + top + '" width="94" height="' + (CAP.cpu * H) + '" rx="8" class="node' + (down ? " down" : "") + (selected ? " target" : "") + '"/>' +
        '<text x="' + (x + 40) + '" y="' + (top + CAP.cpu * H + 18) + '" text-anchor="middle" class="svg-small">' + nd[0] + " · " + u.cpu + "/4 CPU</text>" +
        '<rect x="' + (x + 80) + '" y="' + top + '" width="10" height="' + (CAP.cpu * H) + '" rx="3" class="membg"/>' +
        '<rect x="' + (x + 80) + '" y="' + (top + CAP.cpu * H - (u.mem / CAP.mem) * CAP.cpu * H) + '" width="10" height="' + ((u.mem / CAP.mem) * CAP.cpu * H) + '" rx="3" class="mem"/></g>';
      var y = top + CAP.cpu * H;
      pods.forEach(function (p) {
        if (p.node !== nd[0]) return;
        var h = p.cpu * H;
        y -= h;
        s += '<g data-part="k-pod" data-pod="' + p.id + '" data-label="Pod ' + p.app + " on " + nd[0] + ", " + p.cpu + " CPU, " + p.mem + ' GiB. Press to send back to the bench."><rect x="' + (x + 4) + '" y="' + (y + 2) + '" width="72" height="' + (h - 4) + '" rx="5" fill="' + COL[p.app] + '" class="pod' + (down ? " lost" : "") + '"/>' +
          '<text x="' + (x + 40) + '" y="' + (y + h / 2 + 4) + '" text-anchor="middle" class="pod-tx">' + (p.imp ? "★ " : "") + p.app + "</text></g>";
      });
    });
    s += '<text x="20" y="296" class="svg-small">★ = important app (keep it alive)</text>';
    nodesSvg.innerHTML = s;
    LZ.refreshParts(nodesSvg);
    var bench = document.getElementById("bench");
    var waiting = pods.filter(function (p) { return !p.node; });
    bench.innerHTML = waiting.length ? "" : '<span class="muted small">Empty. Every pod has a node.</span>';
    waiting.forEach(function (p) {
      var b = document.createElement("button");
      b.type = "button"; b.className = "piece"; b.setAttribute("role", "listitem");
      b.setAttribute("aria-pressed", selected === p.id ? "true" : "false");
      b.innerHTML = '<span class="sw" style="background:' + COL[p.app] + '"></span>' + (p.imp ? "★ " : "") + p.app + ' <span class="muted small">' + p.cpu + " CPU · " + p.mem + " GiB</span>";
      b.addEventListener("click", function () { selected = selected === p.id ? null : p.id; drawPack(); var n = document.querySelector('#bench [aria-pressed="true"]'); if (n) n.focus(); });
      LZ.makeDraggable(b, function (zone, under) {
        var g = under && under.closest && under.closest("[data-node],[data-pod]");
        if (!g) return;
        var n = g.getAttribute("data-node") || (pods.filter(function (q) { return q.id === g.getAttribute("data-pod"); })[0] || {}).node;
        if (n) place(p, n);
      });
      bench.appendChild(b);
    });
    meters();
  }
  function setMeter(id, cls) { var m = document.getElementById(id); m.className = "meter " + cls; }
  function meters() {
    var usedNodes = NODES.filter(function (n) { return used(n[0]).n > 0; });
    var cpu = 0;
    usedNodes.forEach(function (n) { cpu += used(n[0]).cpu; });
    var eff = usedNodes.length ? Math.round((cpu / (usedNodes.length * CAP.cpu)) * 100) : 0;
    var risk = 0;
    NODES.forEach(function (n) { risk = Math.max(risk, used(n[0]).n); });
    document.getElementById("m-nodes").textContent = usedNodes.length + " of 6";
    document.getElementById("m-eff").textContent = eff + "%";
    document.getElementById("m-risk").textContent = risk;
    setMeter("mm-cost", usedNodes.length <= 2 ? "green" : usedNodes.length <= 4 ? "amber" : "red");
    setMeter("mm-eff", eff >= 75 ? "green" : eff >= 50 ? "amber" : "red");
    setMeter("mm-risk", risk >= 6 || stuck().length ? "red" : risk >= 4 ? "amber" : "green");
  }
  /* important apps whose replicas all sit in one zone */
  function stuck() {
    var apps = {};
    pods.forEach(function (p) { if (p.imp && p.node) (apps[p.app] = apps[p.app] || {})[zoneOf(p.node)] = 1; });
    return Object.keys(apps).filter(function (a) { return Object.keys(apps[a]).length < 2; });
  }
  function packFb(kind, html) { var f = document.getElementById("pack-fb"); f.innerHTML = '<p class="note ' + kind + '">' + html + "</p>"; }
  function place(p, n) {
    if (!fits(p, n)) {
      var u = used(n);
      packFb("bad", zoneOf(n) === zoneDown ? "That zone is down. Pick a node in another zone." :
        "It does not fit. " + p.app + " asks for " + p.cpu + " CPU and " + p.mem + " GiB. " + n + " has " + (CAP.cpu - u.cpu) + " CPU and " + (CAP.mem - u.mem) + " GiB free. If no node fits, the pod stays Pending and the cluster autoscaler adds a node.");
      return;
    }
    p.node = n; selected = null; drawPack();
  }
  LZ.clickHandlers.push(function (id, el) {
    if (!nodesSvg.contains(el)) return;
    var podId = el.getAttribute("data-pod"), nodeId = el.getAttribute("data-node");
    var sel = pods.filter(function (q) { return q.id === selected; })[0];
    if (podId) {
      var p = pods.filter(function (q) { return q.id === podId; })[0];
      if (sel) { place(sel, p.node); return true; }
      p.node = null; drawPack(); LZ.say(p.app + " sent back to the bench."); return true;
    }
    if (nodeId) {
      if (!sel) { packFb("", "Pick a pod on the bench first, then pick a node."); return true; }
      place(sel, nodeId); return true;
    }
  });
  function auto(mode) {
    pods.forEach(function (p) { p.node = null; });
    var order = pods.slice().sort(function (a, b) { return (b.imp - a.imp) * (mode === "spread" ? 1 : 0) || b.cpu - a.cpu || b.mem - a.mem; });
    order.forEach(function (p) {
      var best = null, bestScore = Infinity;
      NODES.forEach(function (nd) {
        if (!fits(p, nd[0])) return;
        var u = used(nd[0]), score;
        if (mode === "pack") score = -u.cpu - u.mem / 100;
        else {
          var sameZone = pods.filter(function (q) { return q.node && q.app === p.app && zoneOf(q.node) === nd[1]; }).length;
          var sameNode = pods.filter(function (q) { return q.node === nd[0] && q.app === p.app; }).length;
          score = sameZone * 10 + sameNode * 20 + u.cpu;
        }
        if (score < bestScore) { bestScore = score; best = nd[0]; }
      });
      if (best) p.node = best;
    });
    selected = null; drawPack();
    var pending = pods.filter(function (p) { return !p.node; }).length;
    var nodes = NODES.filter(function (n) { return used(n[0]).n > 0; }).length;
    var s = stuck();
    if (mode === "pack") packFb(s.length ? "warn" : "good", "<strong>Packed onto " + nodes + " nodes.</strong> Cheap. Like filling one stand before opening the next (MostAllocated scoring, or Karpenter consolidation)." +
      (s.length ? " But every " + s.join(", ") + " pod is in one zone. Try Zone failure." : "") + (pending ? " " + pending + " pods are Pending: the cluster autoscaler would add a node." : ""));
    else packFb("good", "<strong>Spread over " + nodes + " nodes in three zones.</strong> It costs more, but one failure cannot take out a whole app (topology spread constraints and pod anti-affinity)." + (pending ? " " + pending + " pods are Pending." : ""));
  }
  document.getElementById("auto-pack").addEventListener("click", function () { zoneDown = null; auto("pack"); });
  document.getElementById("auto-spread").addEventListener("click", function () { zoneDown = null; auto("spread"); });
  document.getElementById("pack-reset").addEventListener("click", function () { resetPods(); drawPack(); document.getElementById("pack-fb").innerHTML = ""; });
  document.getElementById("rush").addEventListener("click", function () {
    if (rushed) { packFb("", "The rush has already arrived. Clear to start again."); return; }
    rushed = true; addPods(squad().rush); drawPack();
    packFb("warn", LZ.esc(squad().rushNote) + " Press Auto: Pack or Auto: Spread to place them.");
  });
  document.getElementById("zone-fail").addEventListener("click", function () {
    zoneDown = zoneDown === null ? "A" : zoneDown === "A" ? "B" : zoneDown === "B" ? "C" : null;
    drawPack();
    if (!zoneDown) { packFb("", "All zones are back up."); return; }
    var apps = {};
    pods.forEach(function (p) {
      if (!p.imp || !p.node) return;
      var a = apps[p.app] = apps[p.app] || { up: 0, all: 0 };
      a.all++; if (zoneOf(p.node) !== zoneDown) a.up++;
    });
    var lost = pods.filter(function (p) { return p.node && zoneOf(p.node) === zoneDown; }).length;
    var dead = Object.keys(apps).filter(function (a) { return apps[a].up === 0; });
    packFb(dead.length ? "bad" : "good", "<strong>Zone " + zoneDown + " is down.</strong> " + lost + " pods were lost. " +
      Object.keys(apps).map(function (a) { return "★ " + a + ": " + apps[a].up + " of " + apps[a].all + " still running."; }).join(" ") +
      (dead.length ? " <strong>" + dead.join(", ") + " is completely down.</strong> Both centre-backs were on one side. Spread the important pods." : " The important apps survived. That is why we spread them.") +
      " Press again to fail the next zone.");
  });
  function drawRushLabel() { document.getElementById("rush").textContent = squad().rushLabel; }
  document.getElementById("pack-table").innerHTML = '<thead><tr><th scope="col">Football / stadium</th><th scope="col">Kubernetes</th><th scope="col">Why it matters</th></tr></thead><tbody>' +
    C.packTable.map(function (r) { return "<tr><td>" + LZ.esc(r[0]) + "</td><td>" + LZ.esc(r[1]) + "</td><td>" + LZ.esc(r[2]) + "</td></tr>"; }).join("") + "</tbody>";
  resetPods(); drawPack(); drawRushLabel();

  /* ---------- Seat the crowd ---------- */
  var GROUPS = [
    { n: "Family of 5", k: 5, t: "home" }, { n: "School trip", k: 12, t: "home" }, { n: "Away fans", k: 6, t: "away" },
    { n: "Wheelchair users", k: 2, t: "W" }, { n: "VIPs", k: 3, t: "V" }, { n: "Friends", k: 3, t: "home" },
    { n: "Couple", k: 2, t: "home" }, { n: "Family of 4", k: 4, t: "home" }, { n: "Single", k: 1, t: "home" },
    { n: "Single", k: 1, t: "home" }, { n: "Couple", k: 2, t: "home" }, { n: "Friends", k: 4, t: "home" }, { n: "Single", k: 1, t: "home" }
  ];
  var GCOL = ["#2f8fd8", "#36b37e", "#d6643b", "#7a5bd6", "#e2b23a", "#179a8f", "#c2577a", "#5a6bd8", "#8d98ab", "#8d98ab", "#c2577a", "#179a8f", "#8d98ab"];
  var ROWS = 4, COLS = 10, seats;
  function special(st, r, c) {
    if (st === 0 && r === 0 && c < 2) return "W";
    if (st === 0 && r === 3 && c >= 7) return "V";
    if (LZ.venue().kit === "concert" && st === 1 && r < 2) return "K";
    return null;
  }
  function emptySeats() {
    seats = [];
    for (var st = 0; st < 3; st++) { seats.push([]); for (var r = 0; r < ROWS; r++) { seats[st].push([]); for (var c = 0; c < COLS; c++) seats[st][r].push({ g: null, t: special(st, r, c) }); } }
  }
  function free(st, r, c) { var s = seats[st][r][c]; return s.g === null && s.t !== "K"; }
  function kindOk(g, s) { return g.t === "W" ? s.t === "W" : g.t === "V" ? s.t === "V" : !s.t; }
  /* place group gi as a run of seats in one row (or consecutive rows for big groups) */
  function tryRun(gi, st, needKind) {
    var g = GROUPS[gi];
    for (var r = 0; r < ROWS; r++) {
      for (var c = 0; c + Math.min(g.k, COLS) <= COLS; c++) {
        var ok = true, cells = [], left = g.k, rr = r, cc = c;
        while (left > 0) {
          if (rr >= ROWS) { ok = false; break; }
          var s = seats[st][rr][cc];
          if (!free(st, rr, cc) || (needKind && !kindOk(g, s))) { ok = false; break; }
          cells.push([rr, cc]); left--; cc++;
          if (cc >= COLS && left > 0) { rr++; cc = 0; }
        }
        if (ok) { cells.forEach(function (x) { seats[st][x[0]][x[1]].g = gi; }); return true; }
      }
    }
    return false;
  }
  function seatBlock() {
    emptySeats();
    var order = GROUPS.map(function (g, i) { return i; }).sort(function (a, b) { return GROUPS[b].k - GROUPS[a].k; });
    order.forEach(function (gi) {
      var g = GROUPS[gi];
      if (g.t === "away") { tryRun(gi, 2, true); return; }
      for (var st = 0; st < 2; st++) if (tryRun(gi, st, true)) return;
    });
  }
  function seatScatter() {
    emptySeats();
    GROUPS.forEach(function (g, gi) {
      var st = gi % 3, r = (gi * 7) % ROWS, c = (gi * 3 + 1) % COLS, left = g.k, guard = 0;
      while (left > 0 && guard++ < 400) {
        if (free(st, r, c)) { seats[st][r][c].g = gi; left--; }
        c++;
        if (c >= COLS) { c = 0; r++; }
        if (r >= ROWS) { r = 0; st = (st + 1) % 3; }
      }
    });
  }
  function score() {
    var orph = 0, open = 0, breaks = [];
    for (var st = 0; st < 3; st++) {
      var any = false;
      for (var r = 0; r < ROWS; r++) for (var c = 0; c < COLS; c++) if (seats[st][r][c].g !== null) any = true;
      if (!any) continue;
      open++;
      for (r = 0; r < ROWS; r++) {
        for (c = 0; c < COLS; c++) {
          if (!free(st, r, c)) continue;
          var L = c === 0 || !free(st, r, c - 1), R = c === COLS - 1 || !free(st, r, c + 1);
          var occL = c > 0 && seats[st][r][c - 1].g !== null, occR = c < COLS - 1 && seats[st][r][c + 1].g !== null;
          if (L && R && (occL || occR)) orph++;
        }
      }
    }
    GROUPS.forEach(function (g, gi) {
      var cells = [];
      for (var st = 0; st < 3; st++) for (var r = 0; r < ROWS; r++) for (var c = 0; c < COLS; c++) if (seats[st][r][c].g === gi) cells.push([st, r, c]);
      var runs = 0;
      cells.forEach(function (x, i) { var p = cells[i - 1]; if (!p || p[0] !== x[0] || !((p[1] === x[1] && p[2] === x[2] - 1) || (p[1] === x[1] - 1 && x[2] === 0 && p[2] === COLS - 1))) runs++; });
      if (runs > 1) breaks.push(g.n + " split up");
      if (cells.length < g.k) breaks.push(g.n + " could not be seated");
      cells.forEach(function (x) {
        var s = seats[x[0]][x[1]][x[2]];
        if (g.t === "W" && s.t !== "W") breaks.push("Wheelchair users not in wheelchair spaces");
        if (g.t !== "W" && s.t === "W") breaks.push(g.n + " took a wheelchair space");
        if (g.t !== "V" && s.t === "V") breaks.push(g.n + " took a VIP seat");
        if (g.t === "away") {
          [-1, 1].forEach(function (d) {
            var n = seats[x[0]][x[1]][x[2] + d];
            if (n && n.g !== null && GROUPS[n.g].t !== "away") breaks.push("Away fans next to home fans");
          });
          if (x[0] !== 2) breaks.push("Away fans outside the away block");
        }
      });
    });
    var uniq = breaks.filter(function (b, i) { return breaks.indexOf(b) === i; });
    return { orph: orph, open: open, breaks: uniq };
  }
  var seatSvg = document.getElementById("seats");
  function drawSeats() {
    var s = "";
    ["Stand A", "Stand B", "Stand C (away block)"].forEach(function (name, st) {
      var x0 = 10 + st * 220;
      s += '<g data-part="c-stand"><rect x="' + x0 + '" y="28" width="210" height="' + (ROWS * 44 + 10) + '" rx="10" class="stand-bg"/><text x="' + (x0 + 105) + '" y="20" text-anchor="middle" class="svg-text">' + name + "</text>";
      for (var r = 0; r < ROWS; r++) for (var c = 0; c < COLS; c++) {
        var cell = seats[st][r][c], cx = x0 + 15 + c * 20, cy = 52 + r * 44;
        if (cell.t === "K") { s += '<path d="M' + (cx - 6) + " " + (cy - 6) + "l12 12M" + (cx + 6) + " " + (cy - 6) + 'l-12 12" class="kill"/>'; continue; }
        s += '<circle cx="' + cx + '" cy="' + cy + '" r="8" ' + (cell.g !== null ? 'fill="' + GCOL[cell.g] + '"' : 'class="seat-empty"') + "/>";
        if (cell.t) s += '<text x="' + cx + '" y="' + (cy + 4) + '" text-anchor="middle" class="seat-tx">' + cell.t + "</text>";
      }
      s += "</g>";
    });
    if (LZ.venue().kit === "concert") s += '<text x="335" y="' + (ROWS * 44 + 60) + '" text-anchor="middle" class="svg-small">✕ = kill seats behind tonight\'s concert stage</text>';
    seatSvg.innerHTML = s;
    LZ.refreshParts(seatSvg);
    document.getElementById("seat-legend").innerHTML = GROUPS.map(function (g, i) {
      return GROUPS.findIndex(function (h) { return h.n === g.n; }) === i ? '<span><i style="background:' + GCOL[i] + '"></i>' + g.n + "</span>" : "";
    }).join("");
  }
  function showSeatScore(mode) {
    drawSeats();
    var sc = score();
    document.getElementById("s-orph").textContent = sc.orph;
    document.getElementById("s-open").textContent = sc.open + " of 3";
    document.getElementById("s-break").textContent = sc.breaks.length;
    setMeter("sm-orph", sc.orph <= 2 ? "green" : sc.orph <= 5 ? "amber" : "red");
    setMeter("sm-open", sc.open <= 2 ? "green" : "amber");
    setMeter("sm-break", sc.breaks.length ? "red" : "green");
    var k8s = mode === "block"
      ? "Kubernetes idea: bin packing. Fill nodes one by one, and respect taints (wheelchair spaces), affinity (families together) and isolation (the away block)."
      : "Kubernetes idea: a scheduler that ignores constraints spreads pods everywhere, leaves fragments no pod can use, and breaks affinity and taint rules.";
    document.getElementById("seat-fb").innerHTML = (sc.breaks.length ? '<ul class="small">' + sc.breaks.map(function (b) { return "<li>" + LZ.esc(b) + "</li>"; }).join("") + "</ul>" : '<p class="note good">No rules broken.</p>') +
      '<p class="note">' + k8s + "</p>";
  }
  document.getElementById("seat-block").addEventListener("click", function () { seatBlock(); showSeatScore("block"); });
  document.getElementById("seat-scatter").addEventListener("click", function () { seatScatter(); showSeatScore("scatter"); });
  document.getElementById("seat-table").innerHTML = '<thead><tr><th scope="col">Seating and box office</th><th scope="col">Kubernetes / cloud</th></tr></thead><tbody>' +
    C.seatTable.map(function (r) { return "<tr><td>" + LZ.esc(r[0]) + "</td><td>" + LZ.esc(r[1]) + "</td></tr>"; }).join("") + "</tbody>";
  emptySeats(); drawSeats();

  LZ.on("cloud", function () { drawTeam(); drawPack(); drawSeats(); });
  LZ.on("venue", function () { drawTeam(); resetPods(); drawPack(); drawRushLabel(); emptySeats(); drawSeats(); });
  LZ.quiz(document.getElementById("quiz"), C.chapters[6].quiz, "ch7");
})();
