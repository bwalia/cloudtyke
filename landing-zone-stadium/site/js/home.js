/* Landing page. */
(function () {
  "use strict";
  var C = window.LZS;
  LZ.boot();
  var svg = document.getElementById("stadium");
  var lot = !!LZ.venue().lot;
  function draw() { LZ.drawStadium(svg, { lot: lot }); syncLot(); }
  function syncLot() {
    var b = document.getElementById("lot-toggle");
    b.setAttribute("aria-pressed", lot ? "true" : "false");
    b.textContent = lot ? "Flip back to the stadium" : "Flip to the studio lot";
  }
  document.getElementById("lot-toggle").addEventListener("click", function () {
    lot = !lot; draw();
    LZ.say(lot ? "Now showing the film studio lot. Same landing zone idea, a second picture." : "Now showing the stadium.");
  });

  document.getElementById("one-line").textContent = C.ui.oneLine;
  document.getElementById("b-perm").innerHTML = LZ.LOCK + " " + C.ui.perm;
  document.getElementById("b-change").textContent = "⇄ " + C.ui.change;

  // venue picker
  var vbox = document.getElementById("venues");
  vbox.innerHTML = C.venues.order.map(function (k) {
    var v = C.venues[k];
    return '<button type="button" role="radio" class="venue-btn" data-venue-pick="' + k + '"><strong>' + LZ.esc(v.label) + "</strong><span>" + LZ.esc(v.blurb) + "</span></button>";
  }).join("");
  vbox.querySelectorAll("[data-venue-pick]").forEach(function (b, i, all) {
    b.addEventListener("click", function () { LZ.setVenue(b.getAttribute("data-venue-pick")); });
    b.addEventListener("keydown", function (e) {
      var d = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
      if (!d) return;
      e.preventDefault();
      var n = all[(i + d + all.length) % all.length];
      n.focus(); n.click();
    });
  });
  // hero cloud switch (mirrors the top bar)
  var hc = document.getElementById("hero-cloud");
  hc.innerHTML = C.ui.cloudKeys.map(function (c, i) {
    return '<button type="button" role="radio" class="seg-btn" data-cloud="' + c + '">' + C.ui.clouds[i] + "</button>";
  }).join("");
  hc.querySelectorAll("[data-cloud]").forEach(function (b) {
    b.addEventListener("click", function () { LZ.setCloud(b.getAttribute("data-cloud")); });
  });

  LZ.syncControls();
  LZ.on("venue", function () { lot = !!LZ.venue().lot; draw(); });

  // chapters with progress ticks
  function chapters() {
    document.getElementById("chapters").innerHTML = C.chapters.map(function (ch) {
      var done = LZ.isDone("ch" + ch.n);
      return '<li><a href="chapters/' + ch.file + '"><span class="num">Chapter ' + ch.n + "</span>" +
        '<span class="state' + (done ? " done" : "") + '">' + (done ? "✓ done" : "not yet") + "</span>" +
        "<h3>" + LZ.esc(ch.title) + "</h3><p>" + LZ.esc(ch.puzzle) + "</p></a></li>";
    }).join("");
  }
  chapters();
  document.getElementById("stories").innerHTML = C.storyStrip.map(LZ.storyCard).join("");
  LZ.renderTree(document.getElementById("tree"));

  // follow a fan on the hero drawing
  var steps = ["the public road: the internet", "the gate and bag check: the hub firewall", "the walkway: peering from hub to spoke", "the stand: the event's spoke subnet", "the door to the block: the NSG", "the seat: the app"];
  document.getElementById("fan-btn").addEventListener("click", function () {
    if (lot) { lot = false; draw(); }
    var out = document.getElementById("fan-step");
    LZ.walkDot(svg, LZ.fanPath, function (s) { out.textContent = "Step " + (s + 1) + " of 6: " + steps[s] + "."; },
      function () { out.textContent += " Seated. Chapter 2 explains each step."; });
  });

  document.getElementById("reset").addEventListener("click", function () {
    LZ.state.progress = {}; LZ.save(); chapters(); LZ.say("Progress cleared.");
  });
  draw();
})();
