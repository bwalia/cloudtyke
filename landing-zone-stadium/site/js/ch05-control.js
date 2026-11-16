/* Chapter 5: the control room. Traces, metrics, logs, OpenTelemetry, alerts. */
(function () {
  "use strict";
  var C = window.LZS;
  LZ.boot();

  /* ---------- the control room drawing ---------- */
  var ctrl = document.getElementById("ctrl");
  function short(id) { return LZ.cloudName(id).split(",")[0]; }
  function box(part, x, y, w, h, cls, title) {
    return '<g data-part="' + part + '"><rect x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" rx="10" class="' + cls + '"/>' +
      '<text x="' + (x + w / 2) + '" y="' + (y + 22) + '" text-anchor="middle" class="svg-text">' + title + "</text>" +
      '<text x="' + (x + w / 2) + '" y="' + (y + 40) + '" text-anchor="middle" class="svg-small">' + LZ.esc(short(part)) + "</text></g>";
  }
  function drawCtrl() {
    var s = '<rect x="0" y="0" width="680" height="380" rx="18" class="land"/>';
    s += '<path d="M150 110H210M150 190H210M150 270H210M390 190H440M440 70V310M440 70H470M440 190H470M440 310H470" class="wire"/>';
    s += '<g data-part="o-otel">' + [70, 150, 230].map(function (y, i) {
      return '<rect x="20" y="' + (y + 10) + '" width="130" height="60" rx="10" class="cam"/><text x="85" y="' + (y + 36) + '" text-anchor="middle" class="svg-text" style="fill:#fff">' +
        ["Derby app", "Summer app", "Neon app"][i] + '</text><text x="85" y="' + (y + 54) + '" text-anchor="middle" class="svg-small" style="fill:#e8f0ff">OTel fitting</text>';
    }).join("") + "</g>";
    s += box("o-collector", 210, 140, 180, 100, "desk3", "Video desk");
    s += box("o-traces", 470, 30, 190, 80, "scr", "Fan camera (traces)");
    s += box("o-metrics", 470, 150, 190, 80, "scr", "Counters (metrics)");
    s += box("o-logs", 470, 270, 190, 80, "scr", "Notebook (logs)");
    s += box("o-alerts", 210, 20, 180, 70, "radio", "Radios (alerts)");
    s += box("o-dash", 210, 290, 180, 70, "scr", "Big screens");
    s += '<g data-part="o-security"><circle cx="40" cy="30" r="16" class="cam"/><path d="M40 30l18-10" class="wire"/><text x="66" y="24" class="svg-small">security cameras</text></g>';
    s += '<g data-part="o-tapes"><rect x="20" y="320" width="140" height="44" rx="8" class="tapes"/><text x="90" y="347" text-anchor="middle" class="svg-small" style="fill:#fff">tapes kept</text></g>';
    ctrl.innerHTML = s;
    LZ.refreshParts(ctrl);
  }
  drawCtrl();

  /* ---------- Step 1: the trace ---------- */
  var SPANS = [
    { n: "gate-3: bag check", s: 0, d: 20, slow: false },
    { n: "ticket-api: check ticket", s: 20, d: 60, slow: false },
    { n: "turnstile-3: open", s: 80, d: 1800, slow: true },
    { n: "seat-service: find seat", s: 1880, d: 30, slow: false },
    { n: "postgres: SELECT seat", s: 1885, d: 20, slow: false }
  ];
  var trace = document.getElementById("trace");
  var total = 1950, X0 = 270, W = 290;
  trace.innerHTML = '<text x="0" y="16" class="svg-small">step</text><text x="' + X0 + '" y="16" class="svg-small">0 ms</text><text x="' + (X0 + W) + '" y="16" text-anchor="end" class="svg-small">' + total + " ms</text>" +
    SPANS.map(function (sp, i) {
      var y = 30 + i * 38, x = X0 + (sp.s / total) * W, w = Math.max(6, (sp.d / total) * W);
      return '<g class="span" role="button" tabindex="0" data-span="' + i + '" aria-label="' + sp.n + ", " + sp.d + ' milliseconds">' +
        '<rect x="0" y="' + y + '" width="640" height="32" rx="6" class="span-row"/>' +
        '<text x="8" y="' + (y + 21) + '" class="svg-mono">' + sp.n + "</text>" +
        '<rect x="' + x + '" y="' + (y + 7) + '" width="' + w + '" height="18" rx="4" class="span-bar"/>' +
        '<text x="632" y="' + (y + 21) + '" text-anchor="end" class="svg-small">' + sp.d + " ms</text></g>";
    }).join("");
  function pickSpan(i) {
    var sp = SPANS[i], fb = document.getElementById("trace-fb");
    trace.querySelectorAll(".span").forEach(function (g) { g.classList.toggle("picked", +g.getAttribute("data-span") === i); });
    if (sp.slow) {
      fb.innerHTML = '<p class="note good"><strong>Found it.</strong> turnstile-3 took 1.8 seconds. Everything else took under 0.1 seconds. A trace times every step of one request, so the slow step stands out.</p>' +
        '<p class="small">Next: open the logs for turnstile-3 at that moment (the steward\'s notebook) to see <em>why</em>. Traces show <em>where</em>, logs show <em>why</em>, metrics show <em>how often</em>.</p>' +
        '<p class="small muted">In ' + C.ui.clouds[LZ.ci()] + ": " + LZ.esc(LZ.cloudName("o-traces")) + ".</p>";
    } else {
      fb.innerHTML = '<p class="note bad">' + LZ.esc(sp.n) + " took only " + sp.d + " ms. That is quick. Look for the longest bar.</p>";
    }
  }
  trace.querySelectorAll(".span").forEach(function (g) {
    g.addEventListener("click", function () { pickSpan(+g.getAttribute("data-span")); });
    g.addEventListener("keydown", function (e) { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); pickSpan(+g.getAttribute("data-span")); } });
  });

  /* ---------- Step 2: pick the right signal ---------- */
  var QS = [
    { q: "How many fans came through the gates each minute today?", a: "metrics", why: "A number over time is a metric. Cheap to keep, great for trends." },
    { q: "Why did fan #4521 wait at gate 3?", a: "traces", why: "One request, step by step: that is a trace." },
    { q: "What exactly did the turnstile-3 software report at 15:02?", a: "logs", why: "The written detail of one moment is a log line." }
  ];
  var sig = document.getElementById("signals");
  sig.innerHTML = QS.map(function (q, i) {
    return '<div><p><strong>' + (i + 1) + ".</strong> " + LZ.esc(q.q) + '</p><div class="row">' +
      ["traces", "metrics", "logs"].map(function (k) { return '<button type="button" class="btn" data-q="' + i + '" data-k="' + k + '">' + k[0].toUpperCase() + k.slice(1) + "</button>"; }).join("") +
      '</div><p class="fb small" id="sig-' + i + '" aria-live="polite"></p></div>';
  }).join("");
  sig.querySelectorAll("[data-q]").forEach(function (b) {
    b.addEventListener("click", function () {
      var q = QS[+b.getAttribute("data-q")], ok = b.getAttribute("data-k") === q.a;
      var o = document.getElementById("sig-" + b.getAttribute("data-q"));
      o.className = "fb small note " + (ok ? "good" : "bad");
      o.textContent = (ok ? "Right. " : "Not that one. ") + q.why;
    });
  });

  /* ---------- Step 3: alert threshold ---------- */
  var Q = [30, 34, 41, 38, 45, 52, 48, 44, 61, 70, 66, 58, 62, 90, 140, 185, 170, 120, 80, 60, 55, 48, 40, 35];
  var thr = document.getElementById("thr");
  function drawAlert() {
    var t = +thr.value, max = 220, svg = document.getElementById("alert");
    document.getElementById("thr-v").textContent = t;
    var s = "", fired = 0, caught = false;
    Q.forEach(function (v, i) {
      var h = (v / max) * 180, x = 20 + i * 25, hit = v > t;
      if (hit) fired++;
      if (hit && i >= 14 && i <= 16) caught = true;
      s += '<rect x="' + x + '" y="' + (200 - h) + '" width="18" height="' + h + '" rx="3" class="' + (hit ? "bar-hot" : "bar") + '"/>';
    });
    var ty = 200 - (t / max) * 180;
    s += '<line x1="10" x2="630" y1="' + ty + '" y2="' + ty + '" class="thr-line"/><text x="626" y="' + (ty - 6) + '" text-anchor="end" class="svg-small">alert line</text>';
    s += '<text x="20" y="216" class="svg-small">minute 0</text><text x="600" y="216" text-anchor="end" class="svg-small">minute 24</text>';
    svg.innerHTML = s;
    var msg, kind;
    if (fired > 8) { kind = "warn"; msg = "The radio calls " + fired + " times. Stewards stop listening. Too noisy: raise the line."; }
    else if (!caught) { kind = "bad"; msg = "The radio stayed quiet during the real crush at minute 15. Too high: lower the line."; }
    else { kind = "good"; msg = "The radio called " + fired + " times, all during the real crush at minutes 14 to 17. A good alert: rare, and always worth acting on."; }
    document.getElementById("alert-fb").innerHTML = '<p class="note ' + kind + '">' + msg + '</p><p class="small muted">In ' + C.ui.clouds[LZ.ci()] + ": " + LZ.esc(LZ.cloudName("o-alerts")) + ".</p>";
  }
  thr.addEventListener("input", drawAlert);
  drawAlert();

  LZ.on("cloud", function () { drawCtrl(); drawAlert(); });
  LZ.quiz(document.getElementById("quiz"), C.chapters[4].quiz, "ch5");
})();
