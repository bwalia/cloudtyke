/* Chapter 9: design your own event from a brief, get it checked, then the final quiz. */
(function () {
  "use strict";
  var C = window.LZS;
  LZ.boot();
  var BRIEFS = {
    marathon: { title: "Northwind City Marathon", kit: "football", text: "One day. A huge rush at the start gun when every runner's tracker checks in. Finish photos are kept for a year. Results must be searchable for years.",
      comp: ["k8s"], minMax: 20, store: ["db", "object"], life: ["tiers"] },
    film: { title: "Fabrikam Film Night", kit: "concert", text: "About three hours. A rush when the doors open. Trailers and posters are only needed for 30 days. There are no results to keep.",
      comp: ["docker", "k8s"], minMax: 5, store: ["object"], life: ["d30"] },
    chess: { title: "Contoso Chess Weekend", kit: "cricket", text: "Two quiet days, a small steady load. Every game record is kept for years. Nothing big to store.",
      comp: ["vm", "docker", "k8s"], minMax: 1, maxMax: 6, store: ["db"], life: ["short", "tiers", "d30"] }
  };
  var pick = document.getElementById("brief-pick");
  pick.innerHTML = Object.keys(BRIEFS).map(function (k) { return '<option value="' + k + '">' + BRIEFS[k].title + "</option>"; }).join("");
  function showBrief() {
    var b = BRIEFS[pick.value];
    document.getElementById("brief").innerHTML = "<strong>" + LZ.esc(b.title) + ".</strong> " + LZ.esc(b.text);
    document.getElementById("f-key").value = pick.value;
  }
  pick.addEventListener("change", showBrief);
  showBrief();

  function val(id) { return document.getElementById(id).value; }
  function on(id) { return document.getElementById(id).checked; }
  document.getElementById("design").addEventListener("submit", function (e) {
    e.preventDefault();
    var b = BRIEFS[pick.value], key = val("f-key").trim(), region = val("f-region");
    var comp = (document.querySelector('input[name="f-comp"]:checked') || {}).value;
    var min = +val("f-min"), max = +val("f-max");
    var st = Array.prototype.map.call(document.querySelectorAll('input[name="f-st"]:checked'), function (i) { return i.value; }).sort();
    var rows = [], fails = 0;
    function check(group, ok, pass, fail, err) {
      if (!ok) fails++;
      rows.push({ g: group, ok: ok, msg: ok ? pass : fail, err: ok ? null : err });
    }
    // the safety code
    check("Safety code", /^[a-z0-9]{1,10}$/.test(key), "Event key \"" + key + "\" fits the name rules.",
      "Event key must be lowercase letters and numbers, up to 10. Storage account and Key Vault names have length limits.",
      "Error: Invalid value for variable\n  on variables.tf: event keys must be lowercase letters and numbers, max 10.");
    check("Safety code", region === "uksouth" || region === "ukwest", "Region is in the UK.", "Events stay in the UK.",
      "Code: RequestDisallowedByPolicy\nResource 'rg-" + key + "' was disallowed by policy.\nPolicy: arena-uk-only-rgs (assigned at ct-landingzones)");
    check("Safety code", !on("f-pip"), "No side doors: traffic comes in through the gates.", "No public IPs in events.",
      "Code: RequestDisallowedByPolicy\nResource 'pip-" + key + "' was disallowed by policy.\nPolicy: arena-no-public-ips (assigned at ct-landingzones)");
    check("Safety code", on("f-tags"), "Tagged, so everyone knows who owns it and who pays.", "Resource groups need event, event-kit and cost-centre tags.",
      "Code: RequestDisallowedByPolicy\nPolicy: arena-rg-event-tags");
    check("Safety code", val("f-secret") === "vault", "The password is in the vault, read with a wristband.", "A password in a settings file is a key taped to the door.", null);
    // fits the brief
    check("Fits the brief", b.comp.indexOf(comp) >= 0, "The build style suits this event.", comp === "vm" ? "Hand-built VMs are too slow to scale for this rush." : "Pick a build style that suits the brief.", null);
    check("Fits the brief", min >= 1 && max >= min && max >= b.minMax && (!b.maxMax || max <= b.maxMax), "Pool size " + min + " to " + max + " fits the load.",
      max < min ? "The biggest pool must be at least the smallest (AKS needs min_count ≤ count ≤ max_count)." : b.maxMax && max > b.maxMax ? "That pool is far bigger than a small steady event needs. You would pay for empty seats." : "The rush needs room to grow: let the pool reach at least " + b.minMax + ".", null);
    check("Fits the brief", st.join() === b.store.slice().sort().join(), "Storage matches what must be kept.",
      b.store.indexOf("db") >= 0 && st.indexOf("db") < 0 ? "Records kept for years need a database." : st.indexOf("db") >= 0 && b.store.indexOf("db") < 0 ? "There is nothing to keep score of. Skip the database: less to secure and pay for." : "Check which media must be stored.", null);
    var needsMedia = b.store.indexOf("object") >= 0;
    check("Fits the brief", !needsMedia || b.life.indexOf(val("f-life")) >= 0, needsMedia ? "Media lifecycle matches the brief." : "No media rule needed.",
      "Match the lifecycle to how long the media matters. Keeping everything hot for ever wastes money.", null);
    // staying up
    check("Staying up", on("f-zones"), "Spread across zones: one stand can fail and the event goes on.", "Spread across availability zones, so one failure does not stop the event.", null);
    check("Staying up", on("f-otel"), "Watched from minute one through the standard fitting.", "Plug in the OpenTelemetry fitting so the control room can see the event.", null);

    var html = "", last = "";
    rows.forEach(function (r) {
      if (r.g !== last) { html += (last ? "</ul>" : "") + "<h3>" + r.g + '</h3><ul style="list-style:none;padding:0">'; last = r.g; }
      html += "<li>" + (r.ok ? "✅ PASS " : "❌ FAIL ") + LZ.esc(r.msg) + (r.err ? '<div class="err" style="margin:6px 0">' + LZ.esc(r.err) + "</div>" : "") + "</li>";
    });
    html += "</ul>" + (fails ? '<p class="note warn">' + fails + " to fix. Change the design and check again.</p>" :
      '<p class="note good"><strong>Approved.</strong> The box office books it, and the crew builds it from the template. Copy the snippet into <code>terraform/variables.tf</code>.</p>');
    document.getElementById("checks").innerHTML = '<div class="card">' + html + "</div>";
    var sn = document.getElementById("snippet");
    sn.hidden = !!fails;
    sn.textContent = "# terraform/variables.tf: add to events\n" + key + " = {\n  kit           = \"" + b.kit + "\"  # closest kit in the catalogue\n  title         = \"" + b.title + "\"\n  address_space = \"10.34.0.0/16\"\n  cost_centre   = \"CC-7300\"\n  github_repo   = \"cloudtyke-arena/" + key + "\"\n}\n\n$ bash landing-zone-stadium/scripts/up.sh\n$ bash landing-zone-stadium/verify.sh";
    if (!fails) LZ.say("Approved. Your event passed every check.");
  });

  LZ.quiz(document.getElementById("final"), C.finalQuiz, "final", { title: "The final quiz: 12 questions", intro: "About the ideas, not trivia. Take your time." });
  LZ.quiz(document.getElementById("quiz"), C.chapters[8].quiz, "ch9");
})();
