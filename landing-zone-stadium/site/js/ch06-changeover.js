/* Chapter 6: event kits, the changeover, native / Docker / Kubernetes, storage, stateless vs stateful. */
(function () {
  "use strict";
  var C = window.LZS, K = C.kits;
  LZ.boot();
  var svg = document.getElementById("stadium");
  var kit = LZ.venue().kit, build = "k8s", swaps = 0, ended = false;
  var TF = { football: "football", cricket: "cricket", concert: "concert" };

  function draw() {
    LZ.drawStadium(svg, { event: kit, build: ended ? null : build, lot: false });
    LZ.markLocks(svg, true, swaps > 0);
    drawShelf();
    drawTf();
  }
  function radios(boxId, items, cur, onPick) {
    var box = document.getElementById(boxId);
    box.innerHTML = items.map(function (it) {
      return '<button type="button" role="radio" class="btn" data-v="' + it[0] + '" aria-checked="' + (it[0] === cur) + '">' + it[1] + "</button>";
    }).join("");
    box.querySelectorAll("[data-v]").forEach(function (b) { b.addEventListener("click", function () { onPick(b.getAttribute("data-v")); }); });
  }
  function controls() {
    radios("events", ["football", "cricket", "concert"].map(function (k) { return [k, K[k].name + ": " + K[k].event]; }), kit, function (k) {
      if (k === kit) return;
      var from = kit; kit = k; swaps++; ended = false;
      controls(); draw();
      document.getElementById("m-rebuilt").textContent = "1";
      document.getElementById("m-kept").textContent = "14";
      document.getElementById("m-swaps").textContent = swaps;
      document.getElementById("report").innerHTML = LZ.changeReport(from, k) +
        '<p class="note good">The crew swapped the pitch overnight. Fourteen parts have padlocks: nobody touched them. Like the indoor arena: the ice stays, the court goes on top.</p>';
      LZ.say("Changed to " + K[k].event + ". Only the pitch was rebuilt.");
    });
    radios("builds", [["vm", "Native app"], ["docker", "Docker"], ["k8s", "Kubernetes"]], build, function (b) {
      build = b; ended = false; controls(); draw();
      var id = "b-" + b, p = LZ.part(id);
      LZ.say(p.name + ": " + LZ.cloudName(id));
    });
  }

  /* storage shelf */
  function drawShelf() {
    var items = [["s-object", "Vault", "obj"], ["s-files", "Script library", "files"], ["s-disk", "Lock-ups", "disk"], ["s-db", "Score book", "db"]];
    var s = '<rect x="0" y="0" width="420" height="210" rx="14" class="land"/><path d="M10 160H410" class="shelf-plank"/>';
    items.forEach(function (it, i) {
      var x = 14 + i * 101, need = storageNeeds().indexOf(it[2]) >= 0;
      s += '<g data-part="' + it[0] + '"><rect x="' + x + '" y="30" width="90" height="128" rx="10" class="sh-' + it[2] + (need ? "" : " sh-off") + '"/>' +
        '<text x="' + (x + 45) + '" y="56" text-anchor="middle" class="svg-text" style="fill:#fff">' + it[1] + "</text>" +
        '<text x="' + (x + 45) + '" y="186" text-anchor="middle" class="svg-small">' + LZ.esc(shortName(it[0])) + "</text>" +
        '<text x="' + (x + 45) + '" y="204" text-anchor="middle" class="svg-small">' + (need ? "used tonight" : "not needed") + "</text></g>";
    });
    var sh = document.getElementById("shelf");
    sh.innerHTML = s;
    LZ.refreshParts(sh);
  }
  function shortName(id) { return LZ.label(id); }
  function storageNeeds() {
    var n = ["obj"];
    if (K[kit].db) n.push("db");
    if (build === "vm") n.push("disk");
    if (build === "k8s" && K[kit].db) n.push("disk");
    return n;
  }

  /* terraform view */
  function drawTf() {
    document.getElementById("tf").textContent =
      '# terraform/variables.tf\nevents = {\n  derby = {\n    kit   = "' + TF[kit] + '"   # ← the only line you change\n    title = "Saturday Derby"\n    ...\n  }\n  summer = { kit = "cricket", ... }\n}';
    document.getElementById("plan").textContent = swaps === 0
      ? "$ tofu plan\nNo changes. Your infrastructure matches the configuration."
      : "$ tofu plan\n  ~ pitch resources for \"derby\" (AKS pool, storage rules" + (K[kit].db ? ", score book" : "") + ")\n\n  untouched: gates, firewall, Bastion, DNS, kit room,\n             control room, seats (spoke, subnets, NSGs, routes),\n             safety code\n\nOnly derby's pitch changes.";
  }

  /* kit puzzle */
  var ANSWER = {
    football: { comp: "k8s", scale: "big", st: ["object"], life: "short" },
    cricket: { comp: "k8s", scale: "steady", st: ["object", "db"], life: "tiers" },
    concert: { comp: "k8s", scale: "auto", st: ["object"], life: "d30" }
  };
  function brief() {
    var k = K[kit];
    document.getElementById("brief").innerHTML = "<strong>Tonight: " + LZ.esc(k.event) + " (" + k.name + ").</strong> " + LZ.esc(k.shape) +
      (kit === "cricket" ? " Scorecards and stats must be kept for years. Ball-tracking replays are watched a lot at first, then rarely." :
       kit === "concert" ? " Stage media is only needed for a month. There is nothing to keep score of." :
       " The ticket and fan apps get hammered at kick-off. Highlight clips are only kept a short time.") + " Pick the kit.";
  }
  document.getElementById("kit-check").addEventListener("click", function () {
    var a = ANSWER[kit], msgs = [], ok = 0;
    var comp = (document.querySelector('input[name="k-comp"]:checked') || {}).value;
    var scale = (document.querySelector('input[name="k-scale"]:checked') || {}).value;
    var life = (document.querySelector('input[name="k-life"]:checked') || {}).value;
    var st = Array.prototype.map.call(document.querySelectorAll('input[name="k-st"]:checked'), function (i) { return i.value; }).sort();
    if (comp === a.comp) { ok++; msgs.push("✅ Kubernetes: many small apps that must scale and heal."); }
    else msgs.push(comp === "vm" ? "❌ Hand-built VMs are slow to change and slow to scale for a rush." : comp === "docker" ? "❌ One container is fine for a tiny site, but tickets plus the fan app plus a rush need many crews and a floor manager." : "❌ Pick how the show is built.");
    if (scale === a.scale) { ok++; msgs.push("✅ Scaling fits the shape of the event."); }
    else msgs.push("❌ Scaling: " + (kit === "cricket" ? "five slow days need a small steady pool, not a big one that sits idle." : kit === "football" ? "kick-off is a huge rush: a big autoscaling pool, min 3, max 30." : "doors-open is one rush: an autoscaling pool."));
    if (st.join() === a.st.slice().sort().join()) { ok++; msgs.push("✅ Storage: " + (a.st.indexOf("db") >= 0 ? "a database for the score book, plus object storage for replays." : "object storage only, and no database.")); }
    else msgs.push("❌ Storage: " + (kit === "cricket" ? "stats kept for years need a database. Replays go in object storage." : "media goes in object storage. " + (st.indexOf("db") >= 0 ? "There is no score to keep, so skip the database: less to secure and pay for." : "")) + (st.indexOf("files") >= 0 ? " A file share is not needed here." : ""));
    if (life === a.life) { ok++; msgs.push("✅ Lifecycle rules match how long the media matters."); }
    else msgs.push("❌ Lifecycle: " + (kit === "cricket" ? "hot, then cool, then archive keeps old replays cheap." : kit === "concert" ? "stage media is deleted after 30 days." : "highlight clips are kept a short time."));
    document.getElementById("kit-fb").innerHTML = '<ul style="list-style:none;padding:0">' + msgs.map(function (m) { return "<li>" + LZ.esc(m) + "</li>"; }).join("") + "</ul>" +
      (ok === 4 ? '<p class="note good">A perfect kit for ' + LZ.esc(K[kit].event) + ". Try another event: the kit changes, the stadium does not.</p>" : '<p class="note warn">' + ok + " of 4 right.</p>");
  });

  /* final whistle and booking */
  document.getElementById("end").addEventListener("click", function () {
    ended = true; draw();
    document.getElementById("end-fb").innerHTML = '<p class="note">The players, crews and containers have gone home: the pitch is empty. That is fine. They are <strong>stateless</strong> and can come back any time.</p>' +
      '<p class="note good">The vault' + (K[kit].db ? ", the score book" : "") + ' and the tapes in the control room are still there. They are <strong>stateful</strong>. Data lives in storage that outlasts the pods.</p>';
  });
  document.getElementById("book").addEventListener("click", function () {
    ["boxoffice", "crew"].forEach(function (id) {
      var el = svg.querySelector('[data-part="' + id + '"]');
      if (el) { el.classList.add("pulse"); setTimeout(function () { el.classList.remove("pulse"); }, 2400); }
    });
    document.getElementById("end-fb").innerHTML = '<p class="note">A new team books at the box office (' + LZ.esc(LZ.cloudName("boxoffice")) + '). They get a ready pitch with seats, gates and rules. ' +
      "The changeover crew (" + LZ.esc(LZ.cloudName("crew")) + ") builds it from the same template every time, and signs in with a short-lived token, not a stored password.</p>";
  });

  LZ.on("cloud", draw);
  LZ.on("venue", function () { kit = LZ.venue().kit; controls(); draw(); brief(); });
  controls(); draw(); brief();
  LZ.quiz(document.getElementById("quiz"), C.chapters[5].quiz, "ch6");
})();
