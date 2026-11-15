/* The top-down stadium (and studio lot) drawing. Every part is a <g data-part="..."> so core.js
   gives it a tooltip, a panel, keyboard support and a label. viewBox is 1000 x 680. */
(function () {
  "use strict";
  var NS = "http://www.w3.org/2000/svg";
  var CX = 400, CY = 300;

  function ell(rx, ry, attrs) { return '<ellipse cx="' + CX + '" cy="' + CY + '" rx="' + rx + '" ry="' + ry + '" ' + (attrs || "") + "/>"; }
  function lab(x, y, anchor) { return '<text class="plabel" x="' + x + '" y="' + y + '" text-anchor="' + (anchor || "middle") + '"></text>'; }
  function g(part, inner, extra) { return '<g class="part part-' + part + '" data-part="' + part + '" ' + (extra || "") + ">" + inner + "</g>"; }

  /* ---------- pitch art for each event ---------- */
  var PX = 260, PY = 212, PW = 280, PH = 176;
  function pitchArt(eventKind) {
    var s = "";
    if (eventKind === "cricket") {
      s += '<rect x="' + PX + '" y="' + PY + '" width="' + PW + '" height="' + PH + '" rx="8" class="grass-dark"/>';
      s += '<ellipse cx="400" cy="300" rx="134" ry="84" class="grass"/>';
      s += '<ellipse cx="400" cy="300" rx="126" ry="78" fill="none" class="rope"/>';
      s += '<ellipse cx="400" cy="300" rx="60" ry="40" fill="none" class="line" opacity=".6"/>';
      s += '<rect x="391" y="268" width="18" height="64" rx="2" class="strip"/>';
      s += '<path d="M388 276h24M388 324h24" class="line"/>';
      s += '<g class="dots">' + [[352, 262], [448, 262], [330, 310], [470, 300], [400, 360], [360, 345], [445, 345], [415, 255], [380, 250]].map(function (p) {
        return '<circle cx="' + p[0] + '" cy="' + p[1] + '" r="4.5"/>';
      }).join("") + "</g>";
    } else if (eventKind === "concert") {
      s += '<rect x="' + PX + '" y="' + PY + '" width="' + PW + '" height="' + PH + '" rx="8" class="floor"/>';
      s += '<rect x="300" y="218" width="200" height="44" rx="4" class="stage"/>';
      s += '<path d="M310 222h180" class="truss"/>';
      s += [325, 360, 400, 440, 475].map(function (x) { return '<circle cx="' + x + '" cy="230" r="5" class="light"/>'; }).join("");
      s += '<path d="M290 272h220" class="barrier"/>';
      var crowd = "";
      for (var r = 0; r < 5; r++) for (var c = 0; c < 15; c++) {
        if ((r * 7 + c * 3) % 5 === 0) continue;
        crowd += '<circle cx="' + (288 + c * 16 + (r % 2) * 8) + '" cy="' + (286 + r * 16) + '" r="4"/>';
      }
      s += '<g class="crowd">' + crowd + "</g>";
      s += '<rect x="384" y="362" width="32" height="18" rx="3" class="desk"/>';
    } else {
      for (var i = 0; i < 7; i++) s += '<rect x="' + (PX + i * 40) + '" y="' + PY + '" width="40" height="' + PH + '" class="' + (i % 2 ? "grass" : "grass-dark") + '"/>';
      s += '<rect x="' + (PX + 10) + '" y="' + (PY + 10) + '" width="' + (PW - 20) + '" height="' + (PH - 20) + '" fill="none" class="line"/>';
      s += '<path d="M400 ' + (PY + 10) + "V" + (PY + PH - 10) + '" class="line"/><circle cx="400" cy="300" r="24" fill="none" class="line"/>';
      s += '<rect x="270" y="262" width="34" height="76" fill="none" class="line"/><rect x="496" y="262" width="34" height="76" fill="none" class="line"/>';
      s += '<rect x="262" y="286" width="8" height="28" class="goal"/><rect x="530" y="286" width="8" height="28" class="goal"/>';
    }
    return s;
  }

  /* ---------- "How is the show built?" overlay ---------- */
  function buildArt(kind) {
    if (kind === "vm") {
      return g("b-vm",
        '<rect x="330" y="250" width="140" height="100" rx="4" class="set-hand"/>' +
        '<path d="M330 250l20-22h100l20 22" class="set-hand"/>' +
        '<path d="M345 260v80M360 260v80M440 260v80M455 260v80M330 300h140" class="scaffold"/>' +
        '<rect x="385" y="310" width="30" height="40" class="door"/>' + lab(400, 242));
    }
    if (kind === "docker") {
      var crates = "";
      for (var i = 0; i < 6; i++) {
        var x = 316 + (i % 3) * 58, y = 252 + Math.floor(i / 3) * 52;
        crates += '<rect x="' + x + '" y="' + y + '" width="50" height="42" rx="3" class="crate"/><path d="M' + (x + 6) + " " + (y + 14) + "h38M" + (x + 6) + " " + (y + 28) + 'h38" class="crate-line"/>';
      }
      return g("b-docker", crates + lab(400, 246));
    }
    if (kind === "k8s") {
      var f = [[290, 300], [320, 250], [320, 280], [320, 320], [320, 350], [370, 262], [370, 300], [370, 338], [430, 270], [430, 330], [470, 300]];
      return g("b-k8s", f.map(function (p) { return '<circle cx="' + p[0] + '" cy="' + p[1] + '" r="8" class="player"/>'; }).join("") +
        '<rect x="388" y="378" width="24" height="16" rx="3" class="manager"/>' + lab(400, 244));
    }
    return "";
  }

  /* ---------- the stadium ---------- */
  function stadiumCore(opts) {
    var s = "";
    // shell (walls and roof)
    s += g("shell", ell(330, 255, 'class="shell-ring"') + ell(330, 255, 'class="hit-stroke"') + lab(400, 36));
    // security ring (bag check)
    s += g("firewall", ell(300, 228, 'class="fw-ring"') + ell(300, 228, 'class="hit-stroke"') + lab(170, 120));
    // stands with seat rows and aisles
    var rows = "";
    for (var i = 0; i < 5; i++) {
      var t = i / 4, rx = 276 - t * 64, ry = 206 - t * 56;
      rows += ell(rx, ry, 'class="seat-row"');
    }
    var aisles = "";
    for (var a = 0; a < 12; a++) {
      var ang = (a / 12) * Math.PI * 2 + 0.26;
      aisles += '<path d="M' + (CX + 205 * Math.cos(ang)).toFixed(1) + " " + (CY + 145 * Math.sin(ang)).toFixed(1) + "L" +
        (CX + 285 * Math.cos(ang)).toFixed(1) + " " + (CY + 213 * Math.sin(ang)).toFixed(1) + '" class="aisle"/>';
    }
    s += g("stands",
      '<path fill-rule="evenodd" class="stand" d="M' + (CX - 286) + " " + CY + "a286 214 0 1 0 572 0a286 214 0 1 0 -572 0Z M" + (CX - 202) + " " + CY + 'a202 142 0 1 0 404 0a202 142 0 1 0 -404 0Z"/>' +
      rows + aisles + lab(400, 108));
    // pitch
    s += g("pitch", '<g class="pitch-art">' + pitchArt(opts.event) + "</g>" +
      '<rect x="' + PX + '" y="' + PY + '" width="' + PW + '" height="' + PH + '" rx="8" class="pitch-edge"/>' +
      (opts.build ? '<rect x="' + PX + '" y="' + PY + '" width="' + PW + '" height="' + PH + '" rx="8" class="pitch-veil"/>' + buildArt(opts.build) : "") +
      lab(400, 206));
    // gates N S W E
    var gates = [[400, 45, 0], [400, 555, 0], [70, 300, 1], [730, 300, 1]].map(function (p) {
      var w = p[2] ? 30 : 54, h = p[2] ? 54 : 30;
      return '<rect x="' + (p[0] - w / 2) + '" y="' + (p[1] - h / 2) + '" width="' + w + '" height="' + h + '" rx="5" class="gate"/>' +
        (p[2] ? '<path d="M' + (p[0] - 7) + " " + (p[1] - 14) + "v28M" + (p[0] + 7) + " " + (p[1] - 14) + 'v28" class="turnstile"/>'
              : '<path d="M' + (p[0] - 14) + " " + (p[1] - 7) + "h28M" + (p[0] - 14) + " " + (p[1] + 7) + 'h28" class="turnstile"/>');
    }).join("");
    s += g("gates", gates + lab(470, 600, "start"));
    // staff door
    s += g("staffdoor", '<circle cx="612" cy="105" r="26" class="hit"/><rect x="600" y="93" width="24" height="24" rx="4" class="staff"/><circle cx="618" cy="105" r="2" class="knob"/>' + lab(640, 84, "start"));
    return s;
  }

  function lotCore(opts) {
    var s = "";
    s += g("shell", '<rect x="60" y="40" width="680" height="530" rx="22" class="shell-ring"/><rect x="60" y="40" width="680" height="530" rx="22" class="hit-stroke"/>' + lab(400, 30));
    s += g("firewall", '<rect x="84" y="64" width="632" height="482" rx="14" class="fw-ring"/><rect x="84" y="64" width="632" height="482" rx="14" class="hit-stroke"/>' + lab(150, 84, "start"));
    var stages = [[118, 96], [422, 96], [118, 330], [422, 330]];
    var shells = "", sets = "";
    var setKinds = ["palace", "station", "street", "ship"];
    stages.forEach(function (p, i) {
      shells += '<rect x="' + p[0] + '" y="' + p[1] + '" width="260" height="190" rx="10" class="stage-shed"/>' +
        '<path d="M' + (p[0] + 14) + " " + (p[1] + 12) + "h232" + '" class="rig"/>' +
        [40, 100, 160, 220].map(function (dx) { return '<circle cx="' + (p[0] + dx) + '" cy="' + (p[1] + 12) + '" r="4" class="light"/>'; }).join("") +
        '<rect x="' + (p[0] + 236) + '" y="' + (p[1] + 160) + '" width="16" height="22" class="exit"/>';
      var cx = p[0] + 130, cy = p[1] + 105, k = setKinds[i];
      if (k === "palace") sets += '<path d="M' + (cx - 70) + " " + (cy + 40) + "V" + (cy - 10) + "l70-40 70 40V" + (cy + 40) + 'Z" class="set"/><rect x="' + (cx - 12) + '" y="' + (cy + 10) + '" width="24" height="30" class="door"/>';
      if (k === "station") sets += '<rect x="' + (cx - 80) + '" y="' + (cy - 20) + '" width="160" height="50" class="set"/><path d="M' + (cx - 90) + " " + (cy + 44) + "h180M" + (cx - 90) + " " + (cy + 52) + 'h180" class="rail"/><circle cx="' + cx + '" cy="' + (cy - 36) + '" r="12" class="set"/>';
      if (k === "street") sets += [-60, -10, 40].map(function (dx, j) { return '<rect x="' + (cx + dx - 20) + '" y="' + (cy - 30 + j * 6) + '" width="44" height="' + (70 - j * 6) + '" class="set"/>'; }).join("");
      if (k === "ship") sets += '<path d="M' + (cx - 80) + " " + (cy + 10) + "h160l-26 30H" + (cx - 54) + 'Z" class="set"/><path d="M' + cx + " " + (cy + 10) + "V" + (cy - 50) + "l40 40H" + cx + '" class="set"/>';
    });
    s += g("stands", shells + lab(400, 314));
    s += g("pitch", sets + lab(248, 200));
    var gates = '<rect x="350" y="556" width="100" height="28" rx="5" class="gate"/><path d="M370 562v16M400 562v16M430 562v16" class="turnstile"/>' +
      '<rect x="46" y="270" width="28" height="70" rx="5" class="gate"/>';
    s += g("gates", gates + lab(470, 600, "start"));
    s += g("staffdoor", '<circle cx="612" cy="40" r="26" class="hit"/><rect x="600" y="28" width="24" height="24" rx="4" class="staff"/>' + lab(640, 22, "start"));
    if (opts.build) s += '<g class="lot-build" transform="translate(0,0)">' + "" + "</g>";
    return s;
  }

  LZ.drawStadium = function (svg, opts) {
    opts = opts || {};
    var v = LZ.venue();
    var lot = opts.lot != null ? opts.lot : !!v.lot;
    var ev = opts.event || v.kit;
    svg.setAttribute("viewBox", "0 0 1000 680");
    svg.setAttribute("class", (svg.getAttribute("class") || "").replace(/\bstadium\b/, "").trim() + " stadium");
    var s = "";
    // land with a few trees
    var trees = [[30, 60], [52, 90], [28, 420], [960, 560], [930, 600], [985, 520], [820, 560], [760, 600], [700, 590]].map(function (t) {
      return '<circle cx="' + t[0] + '" cy="' + t[1] + '" r="11" class="tree"/>';
    }).join("");
    s += g("land", '<rect x="0" y="0" width="1000" height="680" rx="26" class="land"/>' + trees + lab(16, 26, "start"));
    // the road and the path to the main gate
    s += g("road", '<rect x="0" y="612" width="1000" height="52" class="road"/><path d="M0 638H1000" class="road-line"/>' + lab(24, 656, "start"));
    s += '<rect x="386" y="568" width="28" height="46" class="path" aria-hidden="true"/>';
    // service lanes from the right-hand buildings to the shell
    s += '<path d="M770 80H700M770 190H735M770 300H741M770 410H712" class="lane" aria-hidden="true"/>';
    s += lot ? lotCore({ build: opts.build }) : stadiumCore({ event: ev, build: opts.build });
    // right-hand buildings
    s += g("control", '<rect x="780" y="30" width="200" height="100" rx="10" class="bld control"/>' +
      [0, 1, 2, 3].map(function (i) { return '<rect x="' + (796 + i * 44) + '" y="48" width="36" height="24" rx="3" class="screen"/>'; }).join("") +
      '<rect x="796" y="84" width="168" height="10" rx="3" class="desk2"/>' + lab(880, 120));
    s += g("safe", '<rect x="780" y="145" width="95" height="95" rx="10" class="bld safe"/><circle cx="827" cy="190" r="22" class="vault"/><path d="M827 174v32M811 190h32" class="vault-x"/>' + lab(827, 232));
    s += g("kitroom", '<rect x="885" y="145" width="95" height="95" rx="10" class="bld kit"/>' +
      [0, 1, 2].map(function (i) { return '<path d="M897 ' + (168 + i * 18) + 'h71" class="shelf"/><rect x="' + (902 + i * 18) + '" y="' + (156 + i * 18) + '" width="14" height="11" rx="2" class="box"/>'; }).join("") + lab(932, 232));
    s += g("crew", '<rect x="780" y="255" width="200" height="70" rx="10" class="bld crew"/><rect x="796" y="272" width="96" height="34" rx="4" class="van"/><path d="M892 280h26l14 14v12h-40z" class="van"/><circle cx="820" cy="310" r="7" class="wheel"/><circle cx="910" cy="310" r="7" class="wheel"/>' + lab(880, 268));
    s += g("training", '<rect x="780" y="340" width="200" height="150" rx="10" class="training-bg"/><rect x="800" y="360" width="160" height="110" rx="4" class="grass-light"/><path d="M880 360v110" class="line"/><circle cx="880" cy="415" r="14" fill="none" class="line"/>' +
      [[820, 380], [850, 450], [930, 390], [910, 440]].map(function (c) { return '<path d="M' + c[0] + " " + (c[1] + 6) + "l6-12 6 12z\" class=\"cone\"/>"; }).join("") + lab(880, 482));
    // outside the main gate
    s += g("boxoffice", '<rect x="20" y="500" width="130" height="86" rx="10" class="bld box-office"/><rect x="38" y="520" width="40" height="26" rx="3" class="window"/><rect x="90" y="520" width="40" height="26" rx="3" class="window"/><path d="M30 500l10-14h90l10 14" class="awning"/>' + lab(85, 576));
    s += g("stewards", '<rect x="282" y="568" width="84" height="40" rx="8" class="hit"/><circle cx="300" cy="578" r="7" class="steward"/><rect x="294" y="586" width="12" height="16" rx="4" class="steward"/><circle cx="326" cy="578" r="7" class="steward"/><rect x="320" y="586" width="12" height="16" rx="4" class="steward"/><rect x="340" y="584" width="20" height="8" rx="4" class="band"/>' + lab(250, 600, "end"));
    s += g("rules", '<rect x="434" y="566" width="84" height="40" rx="8" class="hit"/><rect x="440" y="570" width="72" height="30" rx="3" class="board"/><path d="M450 580h52M450 588h40" class="board-line"/><path d="M458 600v8M494 600v8" class="post"/>' + lab(530, 586, "start"));
    // where the follow-a-fan dot goes
    s += '<g class="fan-layer" aria-hidden="true"></g>';
    svg.innerHTML = s;
    LZ.refreshParts(svg);
  };

  /* Padlocks on parts that never change, and a "changed" tag on the pitch. */
  var LOCKS = { shell: [548, 58], firewall: [128, 168], stands: [236, 140], gates: [446, 540], staffdoor: [636, 120], control: [964, 44], safe: [862, 160],
    kitroom: [966, 160], crew: [966, 268], training: [966, 356], boxoffice: [136, 512], stewards: [284, 560], rules: [520, 562] };
  LZ.markLocks = function (svg, on, changed) {
    var old = svg.querySelector(".lock-layer");
    if (old) old.remove();
    if (!on) return;
    var layer = document.createElementNS(NS, "g");
    layer.setAttribute("class", "lock-layer");
    layer.setAttribute("aria-hidden", "true");
    var s = "";
    Object.keys(LOCKS).forEach(function (k) {
      if (!svg.querySelector('[data-part="' + k + '"]')) return;
      var p = LOCKS[k];
      s += '<g transform="translate(' + (p[0] - 11) + "," + (p[1] - 11) + ')"><circle cx="11" cy="11" r="12" class="lock-bg"/>' +
        '<rect x="5.5" y="10" width="11" height="8" rx="1.5" class="lock-fg"/><path d="M8 10V7.5a3 3 0 0 1 6 0V10" class="lock-sh"/></g>';
    });
    if (changed) s += '<g transform="translate(330,190)"><rect width="140" height="26" rx="13" class="changed-bg"/><text x="70" y="18" text-anchor="middle" class="changed-tx">changed tonight</text></g>';
    layer.innerHTML = s;
    svg.appendChild(layer);
  };

  /* "What changed / What stayed the same" after an event switch. */
  LZ.changeReport = function (from, to) {
    var K = window.LZS.kits, a = K[from], b = K[to];
    var stay = ["land", "shell", "gates", "firewall", "staffdoor", "stands", "control", "safe", "kitroom", "rules", "stewards", "boxoffice", "crew", "training"];
    return '<div class="split even"><div><h3>What changed (1 part)</h3><ul><li><strong>' + LZ.esc(LZ.part("pitch").name) + "</strong>: " +
      LZ.esc(a.event + " (" + a.name + ")") + " → " + LZ.esc(b.event + " (" + b.name + ")") + "</li>" +
      b.needs.map(function (n) { return "<li>" + LZ.esc(n) + "</li>"; }).join("") + "</ul></div>" +
      "<div><h3>What stayed the same (" + stay.length + " parts)</h3><p>" + stay.map(function (k) {
        return '<span class="badge unch">' + LZ.LOCK + " " + LZ.esc(LZ.part(k).name) + "</span>";
      }).join(" ") + "</p></div></div>";
  };

  /* Points for "Follow a fan": road -> gate -> bag check -> walkway -> stand block -> door -> seat. */
  LZ.fanPath = [
    { x: 400, y: 650, step: 0 },
    { x: 400, y: 556, step: 1 },
    { x: 400, y: 528, step: 1 },
    { x: 360, y: 506, step: 2 },
    { x: 330, y: 470, step: 3 },
    { x: 322, y: 452, step: 4 },
    { x: 318, y: 428, step: 5 }
  ];

  /* Walk a dot along points. onStep(stepIndex). Returns a stop function. */
  LZ.walkDot = function (svg, points, onStep, done) {
    var layer = svg.querySelector(".fan-layer");
    layer.innerHTML = "";
    var dot = document.createElementNS(NS, "circle");
    dot.setAttribute("r", "10"); dot.setAttribute("class", "fan-dot");
    var trail = document.createElementNS(NS, "polyline");
    trail.setAttribute("class", "fan-trail");
    layer.appendChild(trail); layer.appendChild(dot);
    var i = 0, t = 0, stopped = false, lastStep = -1, pts = [];
    function place(x, y) {
      dot.setAttribute("cx", x); dot.setAttribute("cy", y);
      pts.push(x.toFixed(1) + "," + y.toFixed(1));
      trail.setAttribute("points", pts.join(" "));
    }
    place(points[0].x, points[0].y);
    if (onStep) { onStep(points[0].step); lastStep = points[0].step; }
    if (LZ.reduced) {
      points.forEach(function (p) { place(p.x, p.y); });
      if (onStep) onStep(points[points.length - 1].step);
      if (done) done();
      return function () {};
    }
    function frame() {
      if (stopped) return;
      if (i >= points.length - 1) { if (done) done(); return; }
      var a = points[i], b = points[i + 1];
      var len = Math.hypot(b.x - a.x, b.y - a.y);
      t += 2.2 / Math.max(len, 1);
      if (t >= 1) { t = 0; i++; place(b.x, b.y); if (onStep && b.step !== lastStep) { lastStep = b.step; onStep(b.step); } }
      else place(a.x + (b.x - a.x) * t, a.y + (b.y - a.y) * t);
      requestAnimationFrame(frame);
    }
    requestAnimationFrame(frame);
    return function () { stopped = true; };
  };
})();
