/* CloudTyke Arena core: state, switches, part tooltips and side panel, quizzes, drag helper.
   Classic script (no modules) so pages open from file://. */
(function () {
  "use strict";
  var C = window.LZS;
  var KEY = "lzs:v1";
  var LZ = (window.LZ = {});
  var listeners = {};

  /* ---------- state, saved in localStorage when allowed ---------- */
  LZ.state = { cloud: "azure", venue: "football", labels: false, theme: "auto", progress: {} };
  try {
    var saved = JSON.parse(localStorage.getItem(KEY) || "{}");
    for (var k in saved) if (saved.hasOwnProperty(k)) LZ.state[k] = saved[k];
  } catch (e) { /* storage blocked: keep defaults */ }
  if (!C.venues[LZ.state.venue]) LZ.state.venue = "football";
  if (C.ui.cloudKeys.indexOf(LZ.state.cloud) < 0) LZ.state.cloud = "azure";

  LZ.save = function () {
    try { localStorage.setItem(KEY, JSON.stringify(LZ.state)); } catch (e) { /* ignore */ }
  };
  LZ.on = function (name, fn) { (listeners[name] = listeners[name] || []).push(fn); };
  LZ.emit = function (name, val) { (listeners[name] || []).forEach(function (fn) { fn(val); }); };

  LZ.reduced = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  LZ.ci = function () { return C.ui.cloudKeys.indexOf(LZ.state.cloud); };
  LZ.venue = function () { return C.venues[LZ.state.venue]; };
  LZ.esc = function (s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, function (ch) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch];
    });
  };

  /* ---------- part lookup (venue aware) ---------- */
  var extra = {};
  LZ.registerPart = function (id, info) { extra[id] = info; };
  LZ.part = function (id) {
    var base = extra[id] || C.parts[id];
    if (!base) return null;
    var v = LZ.venue();
    var p = {};
    for (var k in base) if (base.hasOwnProperty(k)) p[k] = base[k];
    if (v.names && v.names[id]) p.name = v.names[id];
    if (v.team && v.team[id]) { p.name = v.team[id][0]; p.what = v.team[id][1]; }
    p.id = id;
    return p;
  };
  LZ.cloudName = function (id) { var p = LZ.part(id); return p && p.c ? p.c[LZ.ci()] : ""; };
  LZ.label = function (id) { var p = LZ.part(id); if (!p) return ""; return p.l ? p.l[LZ.ci()] : (p.c ? p.c[LZ.ci()] : p.name); };

  LZ.setCloud = function (c) { LZ.state.cloud = c; LZ.save(); LZ.refreshParts(); LZ.syncControls(); LZ.emit("cloud", c); };
  LZ.setVenue = function (v) { LZ.state.venue = v; LZ.save(); LZ.refreshParts(); LZ.syncControls(); LZ.emit("venue", v); };
  LZ.setLabels = function (on) {
    LZ.state.labels = !!on; LZ.save();
    document.body.classList.toggle("show-labels", LZ.state.labels);
    LZ.syncControls();
  };
  LZ.applyTheme = function () {
    var t = LZ.state.theme;
    if (t === "auto") document.documentElement.removeAttribute("data-theme");
    else document.documentElement.setAttribute("data-theme", t);
  };
  LZ.applyTheme();

  /* ---------- progress ---------- */
  LZ.done = function (key) { LZ.state.progress[key] = true; LZ.save(); LZ.emit("progress", key); };
  LZ.isDone = function (key) { return !!LZ.state.progress[key]; };

  /* ---------- live region ---------- */
  LZ.say = function (msg) {
    var r = document.getElementById("lz-live");
    if (!r) return;
    r.textContent = "";
    setTimeout(function () { r.textContent = msg; }, 30);
  };

  LZ.badge = function (perm) {
    return perm
      ? '<span class="badge perm" title="Built once by the platform team">' + LOCK + " " + C.ui.perm + "</span>"
      : '<span class="badge change" title="Each event brings its own">' + SWAP + " " + C.ui.change + "</span>";
  };
  var LOCK = '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 12 12"><rect x="2" y="5.5" width="8" height="5.5" rx="1" fill="currentColor"/><path d="M4 5.5V4a2 2 0 0 1 4 0v1.5" stroke="currentColor" stroke-width="1.4" fill="none"/></svg>';
  var SWAP = '<svg aria-hidden="true" width="12" height="12" viewBox="0 0 12 12"><path d="M2 4h7l-2-2M10 8H3l2 2" stroke="currentColor" stroke-width="1.5" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>';
  LZ.LOCK = LOCK;

  /* ---------- top bar ---------- */
  LZ.root = function () { return document.body.getAttribute("data-root") || "."; };
  LZ.topbar = function () {
    var el = document.getElementById("topbar");
    if (!el) return;
    var root = LZ.root();
    var vopts = C.venues.order.map(function (v) {
      return '<option value="' + v + '">' + LZ.esc(C.venues[v].label) + "</option>";
    }).join("");
    var cbtn = C.ui.cloudKeys.map(function (c, i) {
      return '<button type="button" role="radio" data-cloud="' + c + '" class="seg-btn">' + C.ui.clouds[i] + "</button>";
    }).join("");
    el.innerHTML =
      '<a class="brand" href="' + root + '/index.html" aria-label="CloudTyke Arena home">' + LOGO + "<span>" + C.ui.brand + "</span></a>" +
      '<div class="controls">' +
      '<a class="navlink" href="' + root + '/azure/index.html"' + (document.body.classList.contains("alz") || /\/azure\//.test(location.pathname) ? ' aria-current="page"' : "") + ">Azure landing zones</a>" +
      '<div class="seg" role="radiogroup" aria-label="Cloud">' + cbtn + "</div>" +
      '<label class="venue-pick"><span class="sr-only">Venue</span><select id="venue-select" aria-label="Venue">' + vopts + "</select></label>" +
      '<button type="button" class="chip" id="labels-toggle" aria-pressed="false">' + C.ui.showLabels + "</button>" +
      '<button type="button" class="chip icon" id="theme-toggle" aria-label="Colour theme"></button>' +
      "</div>";
    el.querySelectorAll("[data-cloud]").forEach(function (b) {
      b.addEventListener("click", function () { LZ.setCloud(b.getAttribute("data-cloud")); });
    });
    el.querySelector("#venue-select").addEventListener("change", function (e) { LZ.setVenue(e.target.value); });
    el.querySelector("#labels-toggle").addEventListener("click", function () { LZ.setLabels(!LZ.state.labels); });
    el.querySelector("#theme-toggle").addEventListener("click", function () {
      var order = ["auto", "light", "dark"];
      LZ.state.theme = order[(order.indexOf(LZ.state.theme) + 1) % 3];
      LZ.save(); LZ.applyTheme(); LZ.syncControls();
    });
    LZ.syncControls();
  };
  var LOGO = '<svg aria-hidden="true" width="30" height="30" viewBox="0 0 32 32"><ellipse cx="16" cy="16" rx="14" ry="11" fill="none" stroke="currentColor" stroke-width="3"/><rect x="10" y="11" width="12" height="10" rx="1.5" fill="#4caf6a"/><line x1="16" y1="11" x2="16" y2="21" stroke="#fff" stroke-width="1"/></svg>';
  LZ.syncControls = function () {
    document.querySelectorAll("[data-cloud]").forEach(function (b) {
      var on = b.getAttribute("data-cloud") === LZ.state.cloud;
      b.setAttribute("aria-checked", on ? "true" : "false");
      b.tabIndex = on ? 0 : -1;
    });
    var vs = document.getElementById("venue-select");
    if (vs) vs.value = LZ.state.venue;
    document.querySelectorAll("[data-venue-pick]").forEach(function (b) {
      b.setAttribute("aria-checked", b.getAttribute("data-venue-pick") === LZ.state.venue ? "true" : "false");
    });
    var lt = document.getElementById("labels-toggle");
    if (lt) lt.setAttribute("aria-pressed", LZ.state.labels ? "true" : "false");
    var tt = document.getElementById("theme-toggle");
    if (tt) {
      var t = LZ.state.theme;
      tt.textContent = t === "auto" ? "Theme: auto" : t === "dark" ? "Theme: dark" : "Theme: light";
    }
    document.querySelectorAll("[data-cloud-note]").forEach(function (n) {
      n.textContent = LZ.state.cloud === "azure" ? "Azure names match the CloudTyke dashboard." : C.ui.teachingOnly;
    });
  };

  /* ---------- parts: attributes, labels, tooltip, panel ---------- */
  LZ.refreshParts = function (root) {
    (root || document).querySelectorAll("[data-part]").forEach(function (el) {
      var id = el.getAttribute("data-part"), p = LZ.part(id);
      if (!p) return;
      el.setAttribute("role", "button");
      if (!el.hasAttribute("tabindex")) el.setAttribute("tabindex", "0");
      el.setAttribute("aria-label", (el.getAttribute("data-label") ? el.getAttribute("data-label") + ". " : "") + p.name + ". " + (p.c ? "In the cloud: " + LZ.cloudName(id) : p.what));
      el.querySelectorAll(".plabel").forEach(function (t) {
        if (t.closest("[data-part]") === el) t.textContent = LZ.label(id);
      });
    });
  };

  var tip, panel, tipFor = null, lastPointer = "mouse", lastFocus = null;
  function ensureUi() {
    if (tip) return;
    tip = document.createElement("div");
    tip.id = "lz-tip"; tip.setAttribute("role", "tooltip"); tip.hidden = true;
    document.body.appendChild(tip);
    panel = document.createElement("aside");
    panel.id = "lz-panel"; panel.setAttribute("aria-label", "Part details"); panel.hidden = true;
    panel.setAttribute("tabindex", "-1");
    document.body.appendChild(panel);
    var live = document.createElement("div");
    live.id = "lz-live"; live.className = "sr-only"; live.setAttribute("aria-live", "polite");
    document.body.appendChild(live);
  }

  function related(el) {
    var id = el.getAttribute("data-part"), p = LZ.part(id), svg = el.ownerSVGElement || el.closest("svg") || document;
    var out = [];
    ((p && p.related) || []).forEach(function (r) {
      svg.querySelectorAll('[data-part="' + r + '"]').forEach(function (n) { out.push(n); });
    });
    return out;
  }
  function clearGlow() {
    document.querySelectorAll(".lit, .glow").forEach(function (n) { n.classList.remove("lit"); n.classList.remove("glow"); });
  }
  LZ.showTip = function (el) {
    ensureUi();
    var id = el.getAttribute("data-part"), p = LZ.part(id);
    if (!p) return;
    clearGlow();
    el.classList.add("lit");
    related(el).forEach(function (n) { n.classList.add("glow"); });
    var line2 = p.c ? "→ " + LZ.cloudName(id) + (p.ct && LZ.state.cloud === "azure" ? " (" + p.ct + ")" : "") : p.what;
    tip.innerHTML = "<strong>" + LZ.esc(p.name) + "</strong><span>" + LZ.esc(line2) + "</span>" +
      (lastPointer === "touch" ? '<em>' + C.ui.tapAgain + "</em>" : "");
    tip.hidden = false;
    var r = el.getBoundingClientRect(), tw = tip.offsetWidth, th = tip.offsetHeight;
    var x = r.left + r.width / 2 - tw / 2, y = r.top - th - 10;
    if (y < 8) y = r.bottom + 10;
    if (y + th > window.innerHeight - 8) y = Math.max(8, r.top + r.height / 2 - th / 2);
    x = Math.max(8, Math.min(window.innerWidth - tw - 8, x));
    tip.style.left = x + window.scrollX + "px";
    tip.style.top = y + window.scrollY + "px";
    tipFor = el;
  };
  LZ.hideTip = function () {
    if (tip) tip.hidden = true;
    tipFor = null;
    clearGlow();
  };

  LZ.openPanel = function (id, opener) {
    ensureUi();
    var p = LZ.part(id);
    if (!p) return;
    lastFocus = opener || document.activeElement;
    var ci = LZ.ci();
    var rows = p.c ? C.ui.clouds.map(function (n, i) {
      return "<tr" + (i === ci ? ' class="cur"' : "") + "><th scope=\"row\">" + n + "</th><td>" + LZ.esc(p.c[i]) + "</td></tr>";
    }).join("") : "";
    var ct = p.ct ? '<div class="pblock"><h3>' + C.ui.panelCt + "</h3><p><code>" + LZ.esc(p.ct) + "</code>" +
      (ci > 0 ? ' <span class="muted">(runnable in Azure only)</span>' : "") + "</p></div>" : "";
    panel.innerHTML =
      '<button type="button" class="panel-close" aria-label="Close details">×</button>' +
      '<p class="kicker">' + LZ.esc(LZ.venue().label) + "</p>" +
      "<h2 id=\"lz-panel-h\">" + LZ.esc(p.name) + "</h2>" +
      (typeof p.perm === "boolean" ? "<p>" + LZ.badge(p.perm) + "</p>" : "") +
      '<p class="lead">' + LZ.esc(p.what) + "</p>" +
      (p.c ? '<div class="pblock"><h3>' + C.ui.panelCloud + " (" + C.ui.clouds[ci] + ')</h3><p class="cloudname">' + LZ.esc(p.c[ci]) + "</p></div>" : "") +
      ct +
      (p.why ? '<div class="pblock why"><h3>' + C.ui.panelWhy + "</h3><p>" + LZ.esc(p.why) + "</p></div>" : "") +
      (rows ? '<table class="mini"><caption>All three clouds</caption>' + rows + "</table>" : "") +
      (ci > 0 ? '<p class="muted small">' + C.ui.teachingOnly + "</p>" : "");
    panel.setAttribute("aria-labelledby", "lz-panel-h");
    panel.hidden = false;
    document.body.classList.add("panel-open");
    panel.querySelector(".panel-close").addEventListener("click", LZ.closePanel);
    panel.focus();
    LZ.hideTip();
    LZ.emit("panel", id);
  };
  LZ.closePanel = function () {
    if (!panel || panel.hidden) return;
    panel.hidden = true;
    document.body.classList.remove("panel-open");
    if (lastFocus && lastFocus.focus) lastFocus.focus();
  };

  /* A page can claim clicks on parts (puzzles). Return true to stop the panel opening. */
  LZ.clickHandlers = [];
  function activate(el, how) {
    var id = el.getAttribute("data-part");
    for (var i = 0; i < LZ.clickHandlers.length; i++) if (LZ.clickHandlers[i](id, el, how) === true) return;
    if (el.hasAttribute("data-nopanel")) return;
    if (how === "touch" && tipFor !== el) { LZ.showTip(el); return; }
    LZ.openPanel(id, el);
  }

  document.addEventListener("pointerdown", function (e) { lastPointer = e.pointerType || "mouse"; }, true);
  document.addEventListener("pointerover", function (e) {
    if (e.pointerType === "touch") return;
    var el = e.target.closest && e.target.closest("[data-part]");
    if (el && el !== tipFor) LZ.showTip(el);
  });
  document.addEventListener("pointerout", function (e) {
    if (e.pointerType === "touch") return;
    var el = e.target.closest && e.target.closest("[data-part]");
    if (!el) return;
    var to = e.relatedTarget && e.relatedTarget.closest && e.relatedTarget.closest("[data-part]");
    if (to !== el && tipFor === el && document.activeElement !== el) LZ.hideTip();
  });
  document.addEventListener("focusin", function (e) {
    var el = e.target.closest && e.target.closest("[data-part]");
    if (el && el === e.target) { lastPointer = "key"; LZ.showTip(el); }
  });
  document.addEventListener("focusout", function (e) {
    if (e.target === tipFor) LZ.hideTip();
  });
  document.addEventListener("click", function (e) {
    var el = e.target.closest && e.target.closest("[data-part]");
    if (!el || e.defaultPrevented) {
      if (lastPointer === "touch" && tipFor && !el) LZ.hideTip();
      return;
    }
    activate(el, lastPointer === "touch" ? "touch" : "mouse");
  });
  document.addEventListener("keydown", function (e) {
    var cb = e.target.closest && e.target.closest("[data-cloud]");
    if (!cb) return;
    var keys = C.ui.cloudKeys, i = keys.indexOf(LZ.state.cloud);
    if (e.key === "ArrowRight" || e.key === "ArrowDown") i = (i + 1) % 3;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") i = (i + 2) % 3;
    else return;
    e.preventDefault(); LZ.setCloud(keys[i]);
    var grp = cb.parentNode.querySelector('[data-cloud="' + keys[i] + '"]');
    if (grp) grp.focus();
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape") { LZ.closePanel(); LZ.hideTip(); return; }
    if (e.key !== "Enter" && e.key !== " ") return;
    var el = e.target.closest && e.target.closest("[data-part]");
    if (!el || el !== e.target) return;
    e.preventDefault();
    activate(el, "key");
  });
  window.addEventListener("scroll", function () { if (tipFor && lastPointer !== "key") LZ.hideTip(); }, { passive: true });

  /* ---------- quizzes ---------- */
  LZ.quiz = function (box, questions, key, opts) {
    opts = opts || {};
    var name = "q" + Math.random().toString(36).slice(2, 7);
    box.classList.add("quiz");
    box.innerHTML = (opts.title === false ? "" : "<h2>" + (opts.title || C.ui.check) + "</h2><p>" + (opts.intro || C.ui.checkIntro) + "</p>") +
      questions.map(function (q, i) {
        return '<fieldset class="q" data-i="' + i + '"><legend><span class="qn">' + (i + 1) + ".</span> " + LZ.esc(q.q) + "</legend>" +
          q.a.map(function (a, j) {
            return '<label class="opt"><input type="radio" name="' + name + i + '" value="' + j + '"> <span>' + LZ.esc(a) + "</span></label>";
          }).join("") + '<p class="qfb" aria-live="polite"></p></fieldset>';
      }).join("") +
      '<div class="quiz-foot"><button type="button" class="btn primary quiz-check">Check answers</button> <span class="quiz-result" role="status"></span></div>';
    box.querySelector(".quiz-check").addEventListener("click", function () {
      var right = 0;
      questions.forEach(function (q, i) {
        var fs = box.querySelector('.q[data-i="' + i + '"]');
        var pick = fs.querySelector("input:checked");
        var fb = fs.querySelector(".qfb");
        fs.classList.remove("ok", "bad");
        if (!pick) { fb.textContent = "Pick an answer."; fs.classList.add("bad"); return; }
        if (+pick.value === q.ok) { right++; fs.classList.add("ok"); fb.textContent = "Right. " + q.why; }
        else { fs.classList.add("bad"); fb.textContent = "Not quite. Hint: " + q.why; }
      });
      var res = box.querySelector(".quiz-result");
      res.textContent = right + " of " + questions.length + " right. " + (right === questions.length ? C.ui.checkDone : C.ui.checkTry);
      if (right === questions.length && key) { LZ.done(key); if (opts.onPass) opts.onPass(); }
    });
  };

  /* ---------- story cards and chapter footer ---------- */
  LZ.icons = {
    colosseum: '<ellipse cx="24" cy="30" rx="20" ry="11" fill="none" stroke="currentColor" stroke-width="2.5"/><path d="M6 30V20c6-5 30-5 36 0v10" fill="none" stroke="currentColor" stroke-width="2.5"/><path d="M12 18v8M19 16v8M26 16v8M33 16v8M39 18v8" stroke="currentColor" stroke-width="2"/><rect x="18" y="33" width="12" height="4" fill="currentColor"/>',
    awning: '<path d="M4 20 24 8l20 12" fill="none" stroke="currentColor" stroke-width="2.5"/><path d="M8 20h32v4H8z" fill="currentColor"/><path d="M10 24v14M38 24v14M24 8v30" stroke="currentColor" stroke-width="2"/><circle cx="24" cy="40" r="3" fill="currentColor"/>',
    dropin: '<rect x="8" y="30" width="32" height="10" rx="2" fill="none" stroke="currentColor" stroke-width="2.5"/><rect x="17" y="8" width="14" height="10" rx="1" fill="currentColor"/><path d="M24 4v4M24 18v8M20 22l4 4 4-4" stroke="currentColor" stroke-width="2.2" fill="none"/>',
    retract: '<rect x="4" y="14" width="22" height="20" rx="2" fill="currentColor" opacity=".35"/><rect x="18" y="14" width="22" height="20" rx="2" fill="none" stroke="currentColor" stroke-width="2.5"/><path d="M30 24h14M40 20l4 4-4 4" stroke="currentColor" stroke-width="2.2" fill="none"/>',
    ice: '<rect x="6" y="28" width="36" height="8" rx="2" fill="currentColor" opacity=".35"/><rect x="6" y="18" width="36" height="8" rx="2" fill="none" stroke="currentColor" stroke-width="2.5"/><path d="M24 6v8M20 10l4 4 4-4" stroke="currentColor" stroke-width="2.2" fill="none"/>',
    ramoji: '<rect x="5" y="20" width="16" height="18" fill="none" stroke="currentColor" stroke-width="2.5"/><rect x="27" y="20" width="16" height="18" fill="none" stroke="currentColor" stroke-width="2.5"/><path d="M5 20l8-8 8 8M27 20l8-8 8 8" fill="none" stroke="currentColor" stroke-width="2.2"/><rect x="10" y="28" width="6" height="10" fill="currentColor"/>',
    hengdian: '<path d="M6 22h36M10 22l14-12 14 12" fill="none" stroke="currentColor" stroke-width="2.5"/><path d="M4 22c4 0 6-3 6-3M44 22c-4 0-6-3-6-3" stroke="currentColor" stroke-width="2" fill="none"/><rect x="10" y="22" width="28" height="14" fill="none" stroke="currentColor" stroke-width="2.5"/><rect x="21" y="27" width="6" height="9" fill="currentColor"/>'
  };
  LZ.storyCard = function (key) {
    var s = C.stories[key];
    if (!s) return "";
    return '<article class="story"><svg aria-hidden="true" viewBox="0 0 48 48" width="44" height="44">' + (LZ.icons[s.icon] || "") +
      "</svg><div><h3>" + LZ.esc(s.title) + "</h3><p>" + LZ.esc(s.line) + '</p><p class="lesson"><strong>Lesson:</strong> ' + LZ.esc(s.lesson) + "</p></div></article>";
  };
  LZ.chapterFoot = function (n) {
    var el = document.getElementById("chapter-foot");
    if (!el) return;
    var ch = C.chapters[n - 1], prev = C.chapters[n - 2], next = C.chapters[n];
    el.innerHTML =
      (ch.story ? '<section class="story-wrap" aria-label="A real venue">' + '<h2 class="eyebrow">A real venue</h2>' + LZ.storyCard(ch.story) + "</section>" : "") +
      '<nav class="chapnav" aria-label="Chapters">' +
      (prev ? '<a class="btn" href="' + prev.file + '">← ' + prev.n + ". " + LZ.esc(prev.title) + "</a>" : '<a class="btn" href="../index.html">← ' + C.ui.home + "</a>") +
      '<a class="btn ghost" href="../index.html">' + C.ui.home + "</a>" +
      (next ? '<a class="btn primary" href="' + next.file + '">' + next.n + ". " + LZ.esc(next.title) + " →</a>" : "") +
      "</nav>";
  };
  LZ.chapterHead = function (n) {
    var el = document.getElementById("chapter-head");
    if (!el) return;
    var ch = C.chapters[n - 1];
    el.innerHTML = '<p class="eyebrow">Chapter ' + n + " of 9" + (LZ.isDone("ch" + n) ? ' <span class="tick" aria-label="done">✓ done</span>' : "") + "</p>" +
      "<h1>" + LZ.esc(ch.title) + "</h1><p class=\"lead\">" + LZ.esc(ch.puzzle) + ' <span class="muted">Ideas: ' + LZ.esc(ch.ideas) + "</span></p>" +
      '<p class="cloud-note muted small" data-cloud-note></p>';
  };

  /* ---------- drag or tap helper ----------
     makeDraggable(el, onDrop): drag el with mouse, pen or touch. On release, onDrop(targetElementUnderPointer).
     A plain click is left alone, so the same element also works by tap or keyboard. */
  LZ.makeDraggable = function (el, onDrop) {
    var start = null, ghost = null;
    el.style.touchAction = "none";
    el.addEventListener("pointerdown", function (e) {
      if (e.button !== 0) return;
      start = { x: e.clientX, y: e.clientY, id: e.pointerId };
    });
    el.addEventListener("pointermove", function (e) {
      if (!start || e.pointerId !== start.id) return;
      if (!ghost) {
        if (Math.abs(e.clientX - start.x) + Math.abs(e.clientY - start.y) < 8) return;
        try { el.setPointerCapture(e.pointerId); } catch (x) { /* ignore */ }
        ghost = el.cloneNode(true);
        ghost.className += " drag-ghost";
        ghost.removeAttribute("id");
        document.body.appendChild(ghost);
        el.classList.add("dragging");
      }
      ghost.style.left = e.clientX + window.scrollX - 20 + "px";
      ghost.style.top = e.clientY + window.scrollY - 20 + "px";
      var under = document.elementFromPoint(e.clientX, e.clientY);
      document.querySelectorAll(".drop-hover").forEach(function (n) { n.classList.remove("drop-hover"); });
      var dz = under && under.closest && under.closest("[data-drop]");
      if (dz) dz.classList.add("drop-hover");
    });
    function end(e) {
      if (!start) return;
      var wasDrag = !!ghost;
      if (ghost) { ghost.remove(); ghost = null; }
      el.classList.remove("dragging");
      document.querySelectorAll(".drop-hover").forEach(function (n) { n.classList.remove("drop-hover"); });
      start = null;
      if (wasDrag) {
        el.setAttribute("data-just-dragged", "1");
        setTimeout(function () { el.removeAttribute("data-just-dragged"); }, 50);
        var under = document.elementFromPoint(e.clientX, e.clientY);
        onDrop(under && under.closest ? under.closest("[data-drop]") : null, under);
      }
    }
    el.addEventListener("pointerup", end);
    el.addEventListener("pointercancel", function () { if (ghost) ghost.remove(); ghost = null; start = null; el.classList.remove("dragging"); });
    el.addEventListener("click", function (e) {
      if (el.getAttribute("data-just-dragged")) { e.preventDefault(); e.stopImmediatePropagation(); }
    }, true);
  };

  /* ---------- hierarchy tree ---------- */
  LZ.renderTree = function (box, opts) {
    opts = opts || {};
    function draw() {
      var ci = LZ.ci();
      box.innerHTML = '<ul class="tree" role="list">' + C.tree.map(function (t) {
        var lit = opts.lit ? opts.lit.indexOf(t.id) >= 0 : true;
        return '<li class="d' + t.depth + (lit ? "" : " dim") + '" data-tree="' + t.id + '"><button type="button" data-tree-part="' + t.part + '">' +
          '<span class="tname">' + LZ.esc(ci === 0 ? t.label : t.c[ci]) + "</span>" +
          '<span class="tsub">' + LZ.esc(ci === 0 ? t.sub : C.ui.clouds[ci] + " teaching map") + "</span></button></li>";
      }).join("") + "</ul>";
      box.querySelectorAll("[data-tree-part]").forEach(function (b) {
        b.addEventListener("click", function () {
          var part = b.getAttribute("data-tree-part");
          var targets = document.querySelectorAll('svg [data-part="' + part + '"]');
          document.querySelectorAll(".pulse").forEach(function (n) { n.classList.remove("pulse"); });
          targets.forEach(function (n) { n.classList.add("pulse"); });
          setTimeout(function () { targets.forEach(function (n) { n.classList.remove("pulse"); }); }, 2400);
          var p = LZ.part(part);
          LZ.say("Highlighted " + (p ? p.name : part) + " in the drawing.");
          if (opts.onPick) opts.onPick(part);
        });
      });
    }
    draw();
    LZ.on("cloud", draw);
    return { redraw: function (lit) { if (lit) opts.lit = lit; draw(); } };
  };

  /* ---------- boot ---------- */
  LZ.boot = function () {
    ensureUi();
    LZ.topbar();
    document.body.classList.toggle("show-labels", !!LZ.state.labels);
    var n = +document.body.getAttribute("data-chapter");
    if (n) { LZ.chapterHead(n); LZ.chapterFoot(n); }
    LZ.refreshParts();
    LZ.syncControls();
  };
})();
