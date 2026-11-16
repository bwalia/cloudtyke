/* Chapter 2: follow a fan, fix hub-and-spoke walkways, write door rules. */
(function () {
  "use strict";
  var C = window.LZS;
  LZ.boot();

  /* ---------- Step 1: follow a fan ---------- */
  var svg = document.getElementById("stadium");
  LZ.drawStadium(svg, { lot: false });
  var STEPS = [
    { s: "The public road", c: ["Internet", "Internet", "Internet"], d: "Anyone can be on the road. Nobody is trusted yet." },
    { s: "Gate and bag check", c: ["Hub firewall: Azure Firewall in sub-connectivity", "Network Firewall in the network account", "Cloud NGFW in the Shared VPC host"], d: "Every bag is checked in one place. Bad traffic stops here." },
    { s: "The walkway to the stand", c: ["VNet peering from the hub to the event's spoke", "Transit Gateway attachment", "Shared VPC: the event uses a subnet of the host network"], d: "A walkway joins two places only. It does not pass you on to a third." },
    { s: "The event's stand", c: ["Spoke subnet in sub-events", "Subnet in the event's VPC", "Subnet shared to the event project"], d: "Each event has its own stand, kept apart from the others." },
    { s: "The door to the block", c: ["NSG on the subnet", "Security group on the app", "VPC firewall rule"], d: "A list on the door. Ticket holders only." },
    { s: "The seat", c: ["The app (for example on AKS)", "The app (for example on EKS)", "The app (for example on GKE)"], d: "The fan reaches the app. Every step on the way was checked." }
  ];
  var cur = -1;
  function drawSteps(n) {
    var ci = LZ.ci();
    document.getElementById("fan-steps").innerHTML = STEPS.map(function (st, i) {
      return '<li class="' + (i < n ? "past" : i === n ? "now" : "") + '"><strong>' + st.s + "</strong> → " + LZ.esc(st.c[ci]) + "</li>";
    }).join("");
    document.getElementById("fan-detail").textContent = n >= 0 ? STEPS[n].d : "";
  }
  drawSteps(-1);
  document.getElementById("fan-go").addEventListener("click", function () {
    cur = -1;
    LZ.walkDot(svg, LZ.fanPath, function (s) { cur = s; drawSteps(s); });
  });
  document.getElementById("fan-next").addEventListener("click", function () {
    cur = (cur + 1) % STEPS.length;
    var pts = LZ.fanPath.filter(function (p) { return p.step <= cur; });
    var save = LZ.reduced; LZ.reduced = true;
    LZ.walkDot(svg, pts, null);
    LZ.reduced = save;
    drawSteps(cur);
    LZ.say("Step " + (cur + 1) + ": " + STEPS[cur].s);
  });

  /* ---------- Step 2: hub and spoke ---------- */
  var hub = document.getElementById("hub");
  var fixed = {};
  var SP = { derby: [110, 80, "Derby"], summer: [530, 80, "Summer"], neon: [320, 340, "Neon"] };
  var HUB = [320, 175];
  function mid(a, b) { return [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2]; }
  function drawHub(extra) {
    var s = '<rect x="0" y="0" width="640" height="400" rx="18" class="land"/>';
    if (extra) s += extra;
    Object.keys(SP).forEach(function (k) {
      var p = SP[k], m = mid(HUB, p);
      var spokeSideOk = k !== "derby" || fixed["peer-derby"];
      s += '<g data-part="n-peer"><line x1="' + HUB[0] + '" y1="' + HUB[1] + '" x2="' + m[0] + '" y2="' + m[1] + '" class="walk ok"/></g>';
      s += '<g data-part="n-peer"' + (spokeSideOk ? "" : ' data-fix="peer-derby"') + '><line x1="' + m[0] + '" y1="' + m[1] + '" x2="' + p[0] + '" y2="' + p[1] + '" class="walk ' + (spokeSideOk ? "ok" : "broken") + '"/>' +
        '<line x1="' + m[0] + '" y1="' + m[1] + '" x2="' + p[0] + '" y2="' + p[1] + '" class="hit-line"/></g>';
    });
    s += '<g data-part="n-hub"><circle cx="' + HUB[0] + '" cy="' + HUB[1] + '" r="64" class="hub-c"/><text x="' + HUB[0] + '" y="' + (HUB[1] - 18) + '" text-anchor="middle" class="svg-text">Hub</text></g>';
    s += '<g data-part="firewall"><rect x="' + (HUB[0] - 34) + '" y="' + (HUB[1] - 4) + '" width="68" height="28" rx="6" class="fw-box"/><text x="' + HUB[0] + '" y="' + (HUB[1] + 15) + '" text-anchor="middle" class="svg-small" fill="#fff" style="fill:#fff">bag check</text></g>';
    Object.keys(SP).forEach(function (k) {
      var p = SP[k];
      s += '<g data-part="n-spoke"><rect x="' + (p[0] - 70) + '" y="' + (p[1] - 36) + '" width="140" height="72" rx="12" class="spoke-box"/>' +
        '<text x="' + p[0] + '" y="' + (p[1] - 6) + '" text-anchor="middle" class="svg-text">' + p[2] + "</text>" +
        '<text x="' + p[0] + '" y="' + (p[1] + 14) + '" text-anchor="middle" class="svg-small">spoke</text></g>';
      var needs = k === "derby" ? "route-derby" : k === "summer" ? "route-summer" : null;
      var ok = !needs || fixed[needs];
      var sx = p[0] + (k === "neon" ? 130 : 0), sy = p[1] + (k === "neon" ? -10 : 58);
      s += '<g data-part="n-route"' + (ok ? "" : ' data-fix="' + needs + '"') + '><rect x="' + (sx - 46) + '" y="' + (sy - 14) + '" width="92" height="30" rx="6" class="sign ' + (ok ? "ok" : "broken") + '"/>' +
        '<text x="' + sx + '" y="' + (sy + 5) + '" text-anchor="middle" class="svg-small">' + (ok ? "→ via bag check" : "no sign") + "</text></g>";
    });
    s += '<g class="hub-dot"></g>';
    hub.innerHTML = s;
    LZ.refreshParts(hub);
  }
  var FIX = {
    "peer-derby": "Fixed: the walkway now opens at both ends. A peering needs two halves: hub to Derby and Derby to hub. One half on its own shows as \"Initiated\" and carries nothing.",
    "route-derby": "Fixed: Derby now has a sign (a route table) that says 0.0.0.0/0 goes to the bag check. Derby only knows the hub. Peering is not transitive, so it needs the sign to reach Summer.",
    "route-summer": "Fixed: Summer's replies now go back via the bag check too. Without this, the reply skips the firewall, which sees half a conversation and drops it."
  };
  LZ.clickHandlers.push(function (id, el) {
    var f = el.getAttribute("data-fix");
    if (!f) return;
    fixed[f] = true;
    drawHub();
    out("good", FIX[f] + " (" + Object.keys(fixed).length + " of 3 fixed)");
    return true;
  });
  function out(kind, html) { var o = document.getElementById("hub-out"); o.className = "fb note " + kind; o.innerHTML = html; }
  function moveDot(pts, done) {
    var layer = hub.querySelector(".hub-dot");
    layer.innerHTML = '<circle r="11" class="fan-dot" cx="' + pts[0][0] + '" cy="' + pts[0][1] + '"/>';
    var dot = layer.firstChild, i = 0, t = 0;
    if (LZ.reduced) { dot.setAttribute("cx", pts[pts.length - 1][0]); dot.setAttribute("cy", pts[pts.length - 1][1]); return done(); }
    (function f() {
      if (i >= pts.length - 1) return done();
      var a = pts[i], b = pts[i + 1];
      t += 4 / Math.max(Math.hypot(b[0] - a[0], b[1] - a[1]), 1);
      if (t >= 1) { t = 0; i++; }
      var q = i < pts.length - 1 ? [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t] : b;
      dot.setAttribute("cx", q[0]); dot.setAttribute("cy", q[1]);
      requestAnimationFrame(f);
    })();
  }
  document.getElementById("hub-test").addEventListener("click", function () {
    var D = SP.derby, S = SP.summer, m = mid(HUB, D);
    if (!fixed["peer-derby"]) return moveDot([D, m], function () { out("bad", "<strong>Stuck at Derby.</strong> The walkway is only open at the hub end. Peering state: Initiated. Fix the red walkway."); });
    if (!fixed["route-derby"]) return moveDot([D, [D[0] + 60, D[1] + 30]], function () { out("bad", "<strong>Lost.</strong> Derby is joined to the hub, but not to Summer. Peering is not transitive. Derby needs a sign that says: go via the bag check."); });
    if (!fixed["route-summer"]) return moveDot([D, HUB, S, HUB], function () { out("bad", "<strong>The reply was dropped.</strong> The fan got there, but Summer's answer tried to skip the bag check. The firewall only saw half the conversation. Give Summer the same sign."); });
    moveDot([D, HUB, S, HUB, D], function () { out("good", "<strong>It works.</strong> Derby → bag check → Summer, and the reply comes back the same way. Every event uses the same pattern, so the firewall sees everything."); });
  });
  document.getElementById("hub-direct").addEventListener("click", function () {
    var D = SP.derby, S = SP.summer;
    drawHub('<line x1="' + D[0] + '" y1="' + D[1] + '" x2="' + S[0] + '" y2="' + S[1] + '" class="walk nope"/>');
    out("warn", "<strong>That would work, but don't.</strong> A direct walkway skips the bag check. And it grows fast: 10 events need 45 direct walkways, 50 events need 1,225. Hub and spoke needs one per event.");
    setTimeout(function () { drawHub(); }, 2600);
  });
  document.getElementById("hub-reset").addEventListener("click", function () { fixed = {}; drawHub(); out("", ""); });
  drawHub();

  /* ---------- Step 3: door rules ---------- */
  var RULES = [
    { id: "r1", name: "Allow HTTPS from the bag check", src: "fw", port: "443", act: "Allow", pri: 300, on: true },
    { id: "r2", name: "Deny everything", src: "any", port: "any", act: "Deny", pri: 100, on: true },
    { id: "r3", name: "Allow SSH from the staff door", src: "bastion", port: "22", act: "Allow", pri: 400, on: true },
    { id: "r4", name: "Allow SSH from anywhere", src: "any", port: "22", act: "Allow", pri: 200, on: true }
  ];
  var SRC = { fw: "Bag check (hub firewall)", bastion: "Staff door (Bastion subnet)", internet: "Internet", any: "Any" };
  var TESTS = [
    { name: "A fan via the bag check, HTTPS", src: "fw", port: "443", want: "Allow" },
    { name: "A stranger straight from the internet, HTTPS", src: "internet", port: "443", want: "Deny" },
    { name: "An admin via the staff door, SSH", src: "bastion", port: "22", want: "Allow" },
    { name: "A stranger from the internet, SSH", src: "internet", port: "22", want: "Deny" }
  ];
  function priLabel() { return LZ.state.cloud === "aws" ? "Rule number" : "Priority"; }
  function drawRules() {
    var t = document.getElementById("rules");
    t.innerHTML = "<thead><tr><th scope=\"col\">" + priLabel() + '</th><th scope="col">Rule</th><th scope="col">From</th><th scope="col">Port</th><th scope="col">Action</th><th scope="col">On</th></tr></thead><tbody>' +
      RULES.map(function (r) {
        return "<tr><td><input type=\"number\" min=\"100\" max=\"4096\" step=\"1\" value=\"" + r.pri + "\" data-r=\"" + r.id + "\" data-k=\"pri\" aria-label=\"" + priLabel() + " for " + r.name + "\" style=\"width:5.5em\"></td>" +
          "<td>" + r.name + "</td><td>" + SRC[r.src] + "</td><td>" + r.port + "</td>" +
          "<td><select data-r=\"" + r.id + "\" data-k=\"act\" aria-label=\"Action for " + r.name + "\"><option" + (r.act === "Allow" ? " selected" : "") + ">Allow</option><option" + (r.act === "Deny" ? " selected" : "") + ">Deny</option></select></td>" +
          "<td><input type=\"checkbox\" data-r=\"" + r.id + "\" data-k=\"on\"" + (r.on ? " checked" : "") + " aria-label=\"Rule on: " + r.name + "\"></td></tr>";
      }).join("") + "</tbody>";
    t.querySelectorAll("[data-r]").forEach(function (inp) {
      inp.addEventListener("change", function () {
        var r = RULES.filter(function (x) { return x.id === inp.getAttribute("data-r"); })[0], k = inp.getAttribute("data-k");
        r[k] = k === "on" ? inp.checked : k === "pri" ? +inp.value : inp.value;
      });
    });
    var ci = LZ.ci();
    document.getElementById("rules-intro").textContent = "These are the door rules for the Derby seats. Rules are read from the lowest number up. The first match wins. Fix them so the four tests pass.";
    document.getElementById("cloud-rules-note").innerHTML = [
      "<strong>Azure NSG:</strong> priorities 100 to 4096, lowest first, first match wins. Stateful: a reply to an allowed request is allowed. Azure adds a hidden <code>DenyAllInBound</code> at 65500.",
      "<strong>AWS:</strong> a <em>security group</em> has no numbers and only Allow rules; anything not allowed is denied, and it is stateful. A <em>network ACL</em> has rule numbers, lowest first, Allow and Deny, and is stateless. This puzzle uses the numbered style.",
      "<strong>GCP firewall rules:</strong> priority 0 to 65535, lower wins, Allow and Deny, stateful. Rules target VMs by network tag or service account."
    ][ci];
  }
  function evalRules(src, port) {
    var list = RULES.filter(function (r) { return r.on; }).sort(function (a, b) { return a.pri - b.pri; });
    for (var i = 0; i < list.length; i++) {
      var r = list[i];
      var srcOk = r.src === "any" || r.src === src, portOk = r.port === "any" || r.port === port;
      if (srcOk && portOk) return { act: r.act, by: r };
    }
    return { act: "Deny", by: { name: "the default deny at the bottom", pri: 65500 } };
  }
  document.getElementById("stateless").addEventListener("change", function (e) { document.getElementById("outbound").hidden = !e.target.checked; });
  document.getElementById("rules-test").addEventListener("click", function () {
    var pass = 0, rows = TESTS.map(function (t) {
      var r = evalRules(t.src, t.port), ok = r.act === t.want;
      if (ok) pass++;
      return "<li>" + (ok ? "✅" : "❌") + " " + t.name + ": <strong>" + r.act + "</strong> by \"" + LZ.esc(r.by.name) + "\" (" + r.by.pri + ")" + (ok ? "" : ". Wanted " + t.want + ".") + "</li>";
    });
    var total = TESTS.length;
    if (document.getElementById("stateless").checked) {
      total++;
      var rep = document.getElementById("out-eph").checked;
      if (rep) pass++;
      rows.push("<li>" + (rep ? "✅" : "❌") + " The reply to the fan: " + (rep ? "allowed by the outbound rule." : "<strong>blocked</strong>. A stateless door forgets the request, so the reply needs its own outbound rule.") + "</li>");
    }
    var hint = "";
    if (pass < total) {
      var deny = RULES[1];
      if (deny.on && deny.pri < 300) hint = "Hint: \"Deny everything\" is read first, so nothing gets past it. Give it a bigger number than the Allow rules.";
      else if (RULES[3].on && RULES[3].act === "Deny" && RULES[3].pri < RULES[2].pri) hint = "Hint: \"Deny\" SSH from anywhere is now read before the staff door rule, so admins are blocked too. Turn it off instead, or give it a bigger number than the staff door rule.";
      else if (RULES[3].on && RULES[3].act === "Allow" && RULES[3].pri < (deny.on ? deny.pri : 99999)) hint = "Hint: \"Allow SSH from anywhere\" lets strangers in. Turn it off.";
    }
    document.getElementById("rules-out").innerHTML = '<ul class="steps-list" style="list-style:none;padding:0">' + rows.join("") + "</ul>" +
      (pass === total ? '<p class="note good">All tests pass. Lowest number first, first match wins, and a deny-all at the bottom.</p>' : '<p class="note warn">' + pass + " of " + total + " pass. " + hint + "</p>");
  });
  drawRules();

  LZ.on("cloud", function () { drawSteps(cur); drawRules(); drawHub(); });
  LZ.on("venue", function () { LZ.drawStadium(svg, { lot: false }); });
  LZ.quiz(document.getElementById("quiz"), C.chapters[1].quiz, "ch2");
})();
