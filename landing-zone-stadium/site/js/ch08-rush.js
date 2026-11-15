/* Chapter 8: match day rush. Uses RushModel (rush-model.js) and draws it. */
(function () {
  "use strict";
  var C = window.LZS, M = window.RushModel;
  LZ.boot();
  var svg = document.getElementById("rush");
  var s, timer = null, arrPeak = 0, exitPeak = 0, pendingDown = false;

  function ctl() {
    return { auto: q("c-auto").checked, queue: q("c-queue").checked, limit: q("c-limit").checked, jitter: q("c-jitter").checked, over: +q("c-over").value };
  }
  function q(id) { return document.getElementById(id); }
  function reset() {
    stop(); s = M.init(ctl()); arrPeak = 0; exitPeak = 0; pendingDown = false;
    q("go").textContent = "Start match day"; q("rush-fb").innerHTML = "";
    draw();
  }
  function stop() { if (timer) clearInterval(timer); timer = null; }
  function tick() {
    if (pendingDown && s.t >= 30) { M.regionDown(s); pendingDown = false; LZ.say("The main car park is closed. Signs now say: use car park B."); }
    M.step(s);
    if (s.t <= M.WHISTLE) arrPeak = Math.max(arrPeak, s.lastLoad); else exitPeak = Math.max(exitPeak, s.lastLoad);
    if (s.t >= M.END) { stop(); q("go").textContent = "Run again"; summary(); }
  }
  function zoneClass(p) { return p >= 100 ? "red" : p >= 70 ? "amber" : "green"; }
  function draw() {
    var load = M.load(s), lc = zoneClass(load);
    var x = "";
    x += '<rect x="0" y="0" width="720" height="320" rx="18" class="land"/>';
    x += '<g data-part="r-zones"><ellipse cx="640" cy="165" rx="64" ry="120" class="shell-ring" style="stroke-width:12"/><ellipse cx="640" cy="165" rx="44" ry="96" class="stand"/>' +
      '<path d="M600 105h80M600 225h80" class="aisle"/><text x="640" y="170" text-anchor="middle" class="svg-text">' + s.inside + "</text><text x=\"640\" y=\"186\" text-anchor=\"middle\" class=\"svg-small\">inside</text></g>";
    x += '<rect x="18" y="0" width="44" height="320" class="road"/><path d="M40 0V320" class="road-line"/>';
    var cars = Math.min(14, Math.round((M.waiting(s) + (s.t < M.KICKOFF ? 30 : 0)) / 40));
    for (var i = 0; i < cars; i++) x += '<rect x="' + (24 + (i % 2) * 18) + '" y="' + (12 + i * 21) + '" width="12" height="16" rx="3" class="car"/>';
    x += '<g data-part="r-dns"><rect x="70" y="8" width="150" height="26" rx="5" class="dns"/><text x="145" y="26" text-anchor="middle" class="svg-small" style="fill:#fff">' + (s.mainOpen ? (s.mainUsed >= M.MAIN_CAP ? "→ use car park B" : "→ car park A") : "→ use car park B") + "</text></g>";
    x += park("r-main", 80, 44, "Car park A (main)", s.mainUsed, M.MAIN_CAP, !s.mainOpen ? "CLOSED (region down)" : "");
    x += park("r-overflow", 80, 182, "Car park B (overflow)", s.overUsed, s.ctl.over, s.overOpen ? "" : "opens when needed");
    x += '<g data-part="r-probes"><rect x="262" y="148" width="26" height="22" rx="4" class="radio2"/><path d="M270 148v-10" class="wire"/><circle cx="275" cy="160" r="4" fill="#fff"/></g>';
    var W = s.wMain + s.wOver + s.moving;
    x += '<g data-part="r-queue"><rect x="298" y="100" width="70" height="128" rx="8" class="queue' + (s.ctl.queue ? " on" : "") + '"/>' +
      '<path d="M306 122h54M306 142h54M306 162h54M306 182h54M306 202h54" class="qline"/><text x="333" y="94" text-anchor="middle" class="svg-small">' + (s.ctl.queue ? "queue" : "crowd") + '</text><text x="333" y="246" text-anchor="middle" class="svg-text">' + W + "</text></g>";
    x += '<g data-part="r-bridge"><rect x="376" y="140" width="170" height="46" rx="6" class="bridge ' + lc + '"/>';
    var on = Math.min(80, Math.round((s.bin + s.bout) / 4));
    for (i = 0; i < on; i++) x += '<circle cx="' + (384 + ((i * 37) % 154)) + '" cy="' + (148 + ((i * 13) % 30)) + '" r="3" class="person"/>';
    x += '<text x="461" y="208" text-anchor="middle" class="svg-text">bridge ' + load + "%</text>" + (s.ctl.limit ? '<rect x="370" y="134" width="8" height="58" class="limit"/>' : "") + "</g>";
    var cap = s.ctl.auto ? Math.min(M.TURN_MAX, M.TURN + Math.floor((s.bin + W) * 0.6)) : M.TURN;
    var lanes = Math.max(1, Math.round(cap / 30));
    x += '<g data-part="r-turnstiles"><rect x="552" y="118" width="22" height="90" rx="4" class="gate"/>' +
      Array.apply(null, Array(Math.min(lanes, 7))).map(function (_, k) { return '<path d="M555 ' + (126 + k * 11) + 'h16" class="turnstile"/>'; }).join("") +
      '<text x="563" y="110" text-anchor="middle" class="svg-small">' + lanes + " open</text></g>";
    x += '<g data-part="r-jitter"><path d="M600 300h-90" class="exit-arrow"/><path d="M516 294l-8 6 8 6" class="exit-arrow"/><text x="500" y="304" text-anchor="end" class="svg-small">' + (s.ctl.jitter ? "leave stand by stand" : "everyone leaves at once") + "</text></g>";
    svg.innerHTML = x;
    LZ.refreshParts(svg);
    q("r-wait").textContent = M.waiting(s);
    q("r-away").textContent = s.away;
    q("r-load").textContent = load + "%";
    q("r-peak").textContent = s.peak + "%";
    q("mm-load").className = "meter " + lc;
    q("mm-away").className = "meter " + (s.away ? "red" : "green");
    q("mm-wait").className = "meter " + (M.waiting(s) > 400 ? "amber" : "");
    var phase = s.t < M.KICKOFF ? "Before kick-off" : s.t < M.WHISTLE ? "Match on" : "After the final whistle";
    q("clock").textContent = "Minute " + s.t + " of " + M.END + ". " + phase + ".";
  }
  function park(part, x0, y0, title, used, cap, note) {
    var f = Math.min(1, used / cap);
    return '<g data-part="' + part + '"><rect x="' + x0 + '" y="' + y0 + '" width="170" height="110" rx="10" class="park' + (note.indexOf("CLOSED") === 0 ? " closed" : "") + '"/>' +
      '<rect x="' + (x0 + 10) + '" y="' + (y0 + 92) + '" width="150" height="8" rx="4" class="membg"/><rect x="' + (x0 + 10) + '" y="' + (y0 + 92) + '" width="' + 150 * f + '" height="8" rx="4" class="mem"/>' +
      '<text x="' + (x0 + 85) + '" y="' + (y0 + 26) + '" text-anchor="middle" class="svg-text">' + title + "</text>" +
      '<text x="' + (x0 + 85) + '" y="' + (y0 + 50) + '" text-anchor="middle" class="svg-small">' + used + " of " + cap + " spaces</text>" +
      (note ? '<text x="' + (x0 + 85) + '" y="' + (y0 + 72) + '" text-anchor="middle" class="svg-small" style="font-weight:600">' + note + "</text>" : "") + "</g>";
  }
  function summary() {
    var c = s.ctl, out = [];
    if (arrPeak >= 100) out.push('<p class="note bad"><strong>The bridge went red before kick-off.</strong> More people stepped on than it can safely hold, because the turnstiles at the far end could not keep up. ' +
      (c.limit ? "" : "Nothing stopped people stepping on. ") + "Try <em>turnstile autoscaling</em> and a <em>weight limit at the entrance</em>.</p>");
    if (exitPeak >= 100) out.push('<p class="note bad"><strong>The bridge went red after the final whistle.</strong> Everyone left at the same moment, like clients that all retry at once after an outage. Try <em>jittered exits</em>.</p>');
    if (s.away) out.push('<p class="note warn"><strong>' + s.away + " fans were turned away.</strong> " +
      (s.ctl.over < M.TOTAL - M.MAIN_CAP + 1 || !s.mainOpen ? "The overflow car park was too small for the crowd it had to take. Reserve enough capacity in the second region. " : "") +
      (!c.queue && c.limit ? "Without a calm queue, people at a crowded barrier give up. A waiting room keeps them safe and in line." : "") + "</p>");
    if (!out.length) out.push('<p class="note good"><strong>Green all day.</strong> Peak bridge load ' + s.peak + "%. Nobody was turned away. Autoscaling, a queue, a limit at the entrance and jittered exits: the dampers on the bridge.</p>");
    if (arrPeak >= 100 || exitPeak >= 100) out.push('<p class="small muted">This is only a drawing and nobody is hurt here. Real bridges have failed this way. The stories below are why engineers limit entry at the door and add dampers.</p>');
    q("rush-fb").innerHTML = out.join("");
  }
  q("go").addEventListener("click", function () {
    if (timer) { stop(); q("go").textContent = "Resume"; return; }
    if (s.t >= M.END) reset();
    s.ctl = ctl();
    if (LZ.reduced) { while (s.t < M.END) tick(); draw(); return; }
    q("go").textContent = "Pause";
    timer = setInterval(function () { tick(); draw(); }, 110);
  });
  q("down").addEventListener("click", function () {
    if (s.t >= 30) { M.regionDown(s); draw(); LZ.say("The main car park is closed."); }
    else { pendingDown = true; LZ.say("The main car park will close at minute 30."); q("rush-fb").innerHTML = '<p class="note">The main car park (the main region) will close at minute 30. Health probes notice, and the motorway signs (DNS) send everyone to car park B.</p>'; }
  });
  q("reset").addEventListener("click", reset);
  ["c-auto", "c-queue", "c-limit", "c-jitter", "c-over"].forEach(function (id) {
    q(id).addEventListener("change", function () { if (s.t === 0) { s.ctl = ctl(); draw(); } else s.ctl = ctl(); });
  });

  q("bridges").innerHTML = C.bridges.map(function (b) {
    return '<article class="story"><div><h3>' + LZ.esc(b.name) + "</h3><p>" + LZ.esc(b.what) + '</p><p class="lesson"><strong>Cloud lesson:</strong> ' + LZ.esc(b.lesson) + "</p></div></article>";
  }).join("");

  reset();
  LZ.on("cloud", draw);
  LZ.quiz(document.getElementById("quiz"), C.chapters[7].quiz, "ch8");
})();
