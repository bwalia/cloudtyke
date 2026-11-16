/* Chapter 3: the safety code. Sort rules onto walls, watch inheritance, try to break the rules. */
(function () {
  "use strict";
  var C = window.LZS;
  LZ.boot();

  /* ---------- Step 1: sort rules onto walls ---------- */
  var WALLS = [
    { id: "root", n: ["Tenant Root Group", "Organization root", "Organization node"], d: "Everything in the company", miss: "Too high. This wall covers everything, the platform and the sandbox too. Keep this rule closer to the parts it is for." },
    { id: "platform", n: ["ct-platform", "Infrastructure OU", "platform folder"], d: "Gates, control room, kit room", miss: "This wall only covers the shared platform parts. Events would not get the rule." },
    { id: "lz", n: ["ct-landingzones", "Workloads OU", "landing-zones folder"], d: "Every event", miss: "This wall covers every event. This rule is not about events." },
    { id: "sandbox", n: ["ct-sandbox", "Sandbox OU", "sandbox folder"], d: "The training ground", miss: "This wall only covers the training ground. Real events would not get it." }
  ];
  var CARDS = [
    { id: "c1", t: "No public IPs in events", wall: "lz", why: "Every event must come in through the gates. Assign it at ct-landingzones and every event inherits it." },
    { id: "c2", t: "UK regions only for events", wall: "lz", why: "Events must stay in UK South or UK West. One assignment covers all of them." },
    { id: "c3", t: "Resource groups need tags: event, event-kit, cost-centre", wall: "lz", why: "Every event's spending can be traced. Set once for all events." },
    { id: "c4", t: "Only the platform team may change the hub firewall", wall: "platform", why: "The gates are shared. Lock them where they live, in the platform group." },
    { id: "c5", t: "Sandbox: small machines only, deleted after 30 days", wall: "sandbox", why: "Looser rules for practice, but with a budget and a clean-up." },
    { id: "c6", t: "Every subscription sends its activity log to the control room", wall: "root", why: "This one really is for everything, so it goes at the top." }
  ];
  var placedCards = {}, picked = null;
  function drawSort() {
    var ci = LZ.ci();
    var cards = document.getElementById("cards");
    cards.innerHTML = "";
    CARDS.forEach(function (c) {
      if (placedCards[c.id]) return;
      var b = document.createElement("button");
      b.type = "button"; b.className = "piece"; b.setAttribute("role", "listitem");
      b.setAttribute("aria-pressed", picked === c.id ? "true" : "false");
      b.innerHTML = '<span class="sw" style="background:var(--board)"></span>' + LZ.esc(c.t);
      b.addEventListener("click", function () { picked = picked === c.id ? null : c.id; drawSort(); var n = document.querySelector('.piece[aria-pressed="true"]'); if (n) n.focus(); });
      LZ.makeDraggable(b, function (zone) { if (zone) tryPlace(c.id, zone.getAttribute("data-wall")); });
      cards.appendChild(b);
    });
    if (!cards.children.length) cards.innerHTML = '<p class="note good">All six rules are on the right walls.</p>';
    document.getElementById("walls").innerHTML = WALLS.map(function (w) {
      var on = CARDS.filter(function (c) { return placedCards[c.id] && c.wall === w.id; });
      return '<div class="wall" data-drop="wall" data-wall="' + w.id + '"><button type="button" class="wall-btn" data-wall="' + w.id + '">' +
        "<strong>" + LZ.esc(w.n[ci]) + '</strong><span>' + w.d + "</span></button>" +
        on.map(function (c) { return '<div class="posted">' + LZ.esc(c.t) + "</div>"; }).join("") + "</div>";
    }).join("");
    document.querySelectorAll(".wall-btn").forEach(function (b) {
      b.addEventListener("click", function () {
        if (!picked) { sortFb("warn", "Pick a rule first, then pick a wall."); return; }
        tryPlace(picked, b.getAttribute("data-wall"));
      });
    });
  }
  function sortFb(kind, html) { var f = document.getElementById("sort-fb"); f.className = "fb note " + kind; f.innerHTML = html; }
  function tryPlace(cardId, wallId) {
    var c = CARDS.filter(function (x) { return x.id === cardId; })[0], w = WALLS.filter(function (x) { return x.id === wallId; })[0];
    if (!c || !w) return;
    if (c.wall === wallId) {
      placedCards[cardId] = true; picked = null; drawSort();
      sortFb("good", "<strong>Right.</strong> " + LZ.esc(c.why));
    } else {
      sortFb("bad", "<strong>Not that wall.</strong> " + LZ.esc(w.miss));
    }
  }
  drawSort();

  /* ---------- Step 2: hierarchy and inheritance ---------- */
  var svg = document.getElementById("stadium");
  LZ.drawStadium(svg, {});
  LZ.renderTree(document.getElementById("tree"));
  document.getElementById("flow").addEventListener("click", function () {
    var lines = [
      ["rules", "The safety code is assigned once, at <code>ct-landingzones</code> (assignment <code>arena-safety-code</code>)."],
      ["pitch", "<code>sub-events</code> sits under ct-landingzones, so it inherits every rule."],
      ["pitch", "Each event's resource group (<code>rg-derby</code>, <code>rg-summer</code>) inherits it from sub-events."],
      ["pitch", "Every resource inside inherits it too. Nobody had to copy anything."],
      ["training", "The training ground (<code>ct-sandbox</code>) is a different branch. It does not get these rules."]
    ];
    var out = document.getElementById("flow-out");
    out.innerHTML = "";
    lines.forEach(function (l, i) {
      setTimeout(function () {
        out.insertAdjacentHTML("beforeend", "<li>" + l[1] + "</li>");
        svg.querySelectorAll(".pulse").forEach(function (n) { n.classList.remove("pulse"); });
        var p = svg.querySelector('[data-part="' + l[0] + '"]');
        if (p) p.classList.add("pulse");
      }, LZ.reduced ? 0 : i * 900);
    });
  });

  /* ---------- Step 3: break the rules ---------- */
  var ASSIGN = "/providers/Microsoft.Management/managementGroups/ct-landingzones/providers/Microsoft.Authorization/policyAssignments/arena-safety-code";
  function azErr(target, def, msg) {
    return JSON.stringify({ error: { code: "RequestDisallowedByPolicy", target: target,
      message: "Resource '" + target + "' was disallowed by policy. Reasons: '" + msg + "'. See error details for policy resource IDs.",
      additionalInfo: [{ type: "PolicyViolation", info: { policyDefinitionName: def, policyAssignmentName: "arena-safety-code", policyAssignmentId: ASSIGN } }] } }, null, 2);
  }
  var ATTEMPTS = [
    { t: "Open a side door: a public IP in sub-events", ok: false,
      azure: azErr("pip-sidedoor", "arena-no-public-ips", "No public IP addresses in events"),
      aws: "An error occurred (UnauthorizedOperation) when calling the AllocateAddress operation:\nYou are not authorized to perform this operation with an explicit deny in a service control policy.",
      gcp: "ERROR: Constraint constraints/compute.vmExternalIpAccess violated for project events-derby.",
      why: "There is no side door. Every fan comes in through the gates and the bag check." },
    { t: "Build the Derby in East US", ok: false,
      azure: azErr("rg-derby", "arena-uk-only-rgs", "Resource groups only in UK regions"),
      aws: "An error occurred (AccessDenied) when calling the CreateBucket operation:\nexplicit deny in a service control policy (aws:RequestedRegion not in eu-west-2).",
      gcp: "ERROR: Location us-east1 violates constraint constraints/gcp.resourceLocations on the resource.",
      why: "Our events stay in the UK. The rule is checked before anything is built." },
    { t: "Make a public storage bucket for highlight clips", ok: false,
      azure: azErr("stderbyclips", "arena-storage-secure", "Storage: no public blobs, HTTPS only, TLS 1.2 or newer"),
      aws: "An error occurred (AccessDenied) when calling the PutPublicAccessBlock operation:\nexplicit deny in a service control policy. Account-level Block Public Access is on.",
      gcp: "ERROR: Constraint constraints/storage.publicAccessPrevention violated for bucket derby-clips.",
      why: "The same rules apply to storage. Clips are served through the gates, never from a public bucket." },
    { t: "Create a resource group with no tags", ok: false,
      azure: azErr("rg-pop-up", "arena-rg-event-tags", "Resource groups must carry event, event-kit and cost-centre tags"),
      aws: "An error occurred (AccessDenied): explicit deny in a service control policy (aws:RequestTag/cost-centre is missing).",
      gcp: "ERROR: Constraint custom.requireLabels violated: labels event, event-kit and cost-centre are required.",
      why: "Without tags nobody knows which event owns it or who pays for it." },
    { t: "Build the Derby in UK South, tagged, private only", ok: true,
      azure: "HTTP/1.1 201 Created\n{\n  \"name\": \"rg-derby\",\n  \"location\": \"uksouth\",\n  \"tags\": { \"event\": \"derby\", \"event-kit\": \"football\", \"cost-centre\": \"CC-7100\" }\n}",
      aws: "{\n  \"VpcId\": \"vpc-derby\",\n  \"Region\": \"eu-west-2\",\n  \"Tags\": [ { \"Key\": \"event\", \"Value\": \"derby\" } ]\n}",
      gcp: "Created [https://www.googleapis.com/compute/v1/projects/events-derby/regions/europe-west2/subnetworks/derby].",
      why: "This follows every rule, so the cloud says yes." },
    { t: "A public IP in the training ground (sub-sandbox)", ok: true,
      azure: "HTTP/1.1 201 Created\n{\n  \"name\": \"pip-practice\",\n  \"id\": \"/subscriptions/5b5c0000-0000-4000-8000-000000000003/resourceGroups/rg-practice/providers/Microsoft.Network/publicIPAddresses/pip-practice\"\n}",
      aws: "{\n  \"PublicIp\": \"203.0.113.10\",\n  \"AllocationId\": \"eipalloc-practice\"\n}",
      gcp: "Created [https://www.googleapis.com/compute/v1/projects/sandbox/regions/europe-west2/addresses/practice].",
      why: "Allowed. The safety code is assigned at ct-landingzones, and the training ground is on a different branch. Scope matters." }
  ];
  var box = document.getElementById("attempts");
  box.innerHTML = ATTEMPTS.map(function (a, i) { return '<button type="button" class="btn" data-a="' + i + '">' + LZ.esc(a.t) + "</button>"; }).join("");
  var lastAttempt = -1;
  function showAttempt(i) {
    lastAttempt = i;
    var a = ATTEMPTS[i], txt = a[LZ.state.cloud];
    document.getElementById("attempt-out").innerHTML = '<div class="err' + (a.ok ? " okay" : "") + '">' + LZ.esc(txt) + "</div>" +
      '<p class="note ' + (a.ok ? "good" : "warn") + '">' + (a.ok ? "<strong>Allowed.</strong> " : "<strong>Blocked by the safety code.</strong> ") + LZ.esc(a.why) + "</p>";
  }
  box.querySelectorAll("[data-a]").forEach(function (b) { b.addEventListener("click", function () { showAttempt(+b.getAttribute("data-a")); }); });

  LZ.on("cloud", function () { drawSort(); if (lastAttempt >= 0) showAttempt(lastAttempt); });
  LZ.on("venue", function () { LZ.drawStadium(svg, {}); });
  LZ.quiz(document.getElementById("quiz"), C.chapters[2].quiz, "ch3");
})();
