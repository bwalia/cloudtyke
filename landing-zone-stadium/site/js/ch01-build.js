/* Chapter 1: build the stadium, piece by piece, with a riddle for each piece. */
(function () {
  "use strict";
  var C = window.LZS, B = C.build;
  LZ.boot();
  var svg = document.getElementById("stadium");
  var SW = { land: "--land", shell: "--shell", gates: "--gate", stewards: "--steward", rules: "--board", control: "--control", safe: "--safe",
    kitroom: "--kit", stands: "--stand", pitch: "--grass", training: "--train" };
  var FOLLOW = { gates: ["firewall", "staffdoor"], pitch: ["boxoffice", "crew"] };
  var placed = Math.min(+(LZ.state.build1 || 0), B.length);
  var shuffled = B.map(function (b) { return b.part; }).sort(function (a, b) { return (a.charCodeAt(1) * 7) % 11 - (b.charCodeAt(1) * 7) % 11; });
  var eventKit = LZ.venue().kit, lastKit = eventKit;
  var tree = LZ.renderTree(document.getElementById("tree"), { lit: [] });

  function isPlaced(part) {
    for (var i = 0; i < placed; i++) {
      if (B[i].part === part) return true;
      if ((FOLLOW[B[i].part] || []).indexOf(part) >= 0) return true;
    }
    return part === "road";
  }
  function draw() {
    LZ.drawStadium(svg, { event: eventKit, lot: false });
    svg.querySelectorAll("[data-part]").forEach(function (el) {
      var id = el.getAttribute("data-part");
      el.removeAttribute("data-drop");
      if (!isPlaced(id)) {
        el.classList.add("ghost-part");
        if (B[placed] && B[placed].part === id) { el.classList.add("active"); el.setAttribute("data-drop", "slot"); }
      }
    });
    if (placed >= B.length) LZ.markLocks(svg, true, eventKit !== lastKit);
  }
  function render() {
    draw();
    var lit = [];
    for (var i = 0; i < placed; i++) lit = lit.concat(B[i].tree);
    tree.redraw(lit);
    document.getElementById("count").textContent = placed >= B.length ? "All 11 pieces placed" : "Piece " + (placed + 1) + " of " + B.length;
    document.getElementById("bar").style.width = (placed / B.length) * 100 + "%";
    document.getElementById("riddle").textContent = placed >= B.length ? "Done! Every part of the landing zone is in place." : "Riddle: " + B[placed].riddle;
    var tray = document.getElementById("tray");
    tray.innerHTML = "";
    shuffled.forEach(function (part) {
      if (isPlaced(part)) return;
      var p = LZ.part(part);
      var b = document.createElement("button");
      b.type = "button"; b.className = "piece"; b.setAttribute("role", "listitem");
      b.setAttribute("data-piece", part);
      b.innerHTML = '<span class="sw" style="background:var(' + SW[part] + ')"></span>' + LZ.esc(p.name);
      b.setAttribute("aria-label", "Place " + p.name);
      b.addEventListener("click", function () { attempt(part, b); });
      LZ.makeDraggable(b, function (zone, under) {
        if (zone) return attempt(part, b);
        var other = under && under.closest && under.closest(".ghost-part");
        if (other) feedback("bad", "That spot comes later. Build in order: the glowing outline is next.");
      });
      tray.appendChild(b);
    });
    document.getElementById("after").hidden = placed < B.length;
    if (placed >= B.length) renderEvents();
  }
  function feedback(kind, html) {
    var fb = document.getElementById("fb");
    fb.className = "fb note " + (kind === "ok" ? "good" : "bad");
    fb.innerHTML = html;
  }
  function attempt(part, btn) {
    if (placed >= B.length) return;
    var want = B[placed].part, p = LZ.part(part);
    if (part !== want) {
      btn.classList.remove("wrong"); void btn.offsetWidth; btn.classList.add("wrong");
      feedback("bad", "<strong>Not this one.</strong> " + LZ.esc(p.name) + ": " + LZ.esc(p.what) + " Read the riddle again.");
      return;
    }
    placed++;
    LZ.state.build1 = placed; LZ.save();
    render();
    var el = svg.querySelector('[data-part="' + part + '"]');
    if (el && !LZ.reduced) el.classList.add("placed-pop");
    feedback("ok", "<strong>Right: " + LZ.esc(p.name) + ".</strong> " + LZ.badge(p.perm) + "<br>In the cloud: " + LZ.esc(LZ.cloudName(part)) +
      (p.ct ? " (<code>" + LZ.esc(p.ct) + "</code>)" : "") + ". " + LZ.esc(p.why));
    var next = document.querySelector(".piece");
    if (next && btn === document.activeElement) next.focus();
    if (placed >= B.length) LZ.say("Stadium complete. Try switching the event below.");
  }
  LZ.clickHandlers.push(function (id, el) {
    if (el.classList.contains("ghost-part")) { LZ.say("Not built yet. Place pieces from the tray."); return true; }
  });

  function renderEvents() {
    var box = document.getElementById("events");
    box.innerHTML = ["football", "cricket", "concert"].map(function (k) {
      return '<button type="button" role="radio" class="btn" data-kit="' + k + '" aria-checked="' + (k === eventKit) + '">' + C.kits[k].name + ": " + C.kits[k].event + "</button>";
    }).join("");
    box.querySelectorAll("[data-kit]").forEach(function (b) {
      b.addEventListener("click", function () {
        var k = b.getAttribute("data-kit");
        if (k === eventKit) return;
        lastKit = eventKit; eventKit = k;
        render();
        document.getElementById("report").innerHTML = LZ.changeReport(lastKit, eventKit) +
          '<p class="note good">One part changed. Fourteen stayed exactly the same. That is a landing zone.</p>';
        var nb = document.querySelector('[data-kit="' + k + '"]');
        if (nb) nb.focus();
      });
    });
  }

  document.getElementById("restart").addEventListener("click", function () {
    placed = 0; LZ.state.build1 = 0; LZ.save(); eventKit = LZ.venue().kit; lastKit = eventKit;
    document.getElementById("fb").className = "fb"; document.getElementById("fb").textContent = "";
    document.getElementById("report").innerHTML = "";
    render();
  });

  var out = document.getElementById("test-out");
  var fanSteps = ["Internet: the fan walks along the public road.", "Hub firewall: the gate and bag check look at everything.", "Peering: the walkway joins the concourse to the stand.",
    "Spoke subnet: the event's own stand.", "NSG: the door rules for this block say yes to ticket holders.", "The app: the fan is in their seat."];
  document.getElementById("t-fan").addEventListener("click", function () {
    out.innerHTML = '<ol class="steps-list" id="fan-steps">' + fanSteps.map(function (s) { return "<li>" + s + "</li>"; }).join("") + "</ol>";
    var lis = out.querySelectorAll("li");
    LZ.walkDot(svg, LZ.fanPath, function (s) {
      lis.forEach(function (li, i) { li.className = i < s ? "past" : i === s ? "now" : ""; });
    }, function () { out.insertAdjacentHTML("beforeend", '<p class="note good">Seated. One way in, every step checked.</p>'); });
  });
  document.getElementById("t-side").addEventListener("click", function () {
    var pts = [{ x: 150, y: 650, step: 0 }, { x: 150, y: 560, step: 0 }, { x: 160, y: 500, step: 1 }];
    LZ.walkDot(svg, pts, null, function () {
      out.innerHTML = '<div class="err">' + LZ.esc(sideDoorError()) + '</div><p class="note">There is no side door. The safety code says: no public IPs in events. The only way in is through the gates and the bag check.</p>';
    });
  });
  document.getElementById("t-safe").addEventListener("click", function () {
    out.innerHTML = '<p class="note">The <strong>' + LZ.esc(C.kits[eventKit].event) + "</strong> app wears its own wristband (" + LZ.esc(LZ.cloudName("v-identity")) +
      "). The safe opens <em>its</em> locker only. Nobody can use a key taped to the door, because there isn't one. Chapter 4 shows how.</p>";
    var s = svg.querySelector('[data-part="safe"]');
    if (s) { s.classList.add("pulse"); setTimeout(function () { s.classList.remove("pulse"); }, 2400); }
  });
  function sideDoorError() {
    var c = LZ.state.cloud;
    if (c === "aws") return "An error occurred (UnauthorizedOperation) when calling the AllocateAddress operation:\nYou are not authorized to perform this operation with an explicit deny in a service control policy.";
    if (c === "gcp") return "ERROR: (gcloud.compute.addresses.create) Constraint constraints/compute.vmExternalIpAccess violated for project events-derby.";
    return 'Code: RequestDisallowedByPolicy\nMessage: Resource \'pip-sidedoor\' was disallowed by policy.\n  Policy: "No public IPs in events" (assigned at ct-landingzones)';
  }

  LZ.quiz(document.getElementById("quiz"), C.chapters[0].quiz, "ch1");
  LZ.on("venue", function () { eventKit = LZ.venue().kit; lastKit = eventKit; render(); });
  LZ.on("cloud", function () { render(); });
  render();
})();
