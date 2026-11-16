/* Chapter 4: the club safe. Find the password in the config, swap it for a wristband, test the lockers. */
(function () {
  "use strict";
  var C = window.LZS;
  LZ.boot();

  /* ---------- the strong room drawing ---------- */
  var room = document.getElementById("room");
  var fixedSecret = false;
  function drawRoom() {
    var s = '<rect x="0" y="0" width="640" height="400" rx="18" class="land"/>';
    s += '<g data-part="v-room"><rect x="20" y="20" width="600" height="300" rx="16" class="room-wall"/><text x="40" y="48" class="svg-text">Strong room</text></g>';
    // lockers per event
    var ev = [["Derby", "derby"], ["Summer", "summer"], ["Neon", "neon"]];
    s += '<g data-part="v-secrets">' + ev.map(function (e, i) {
      var x = 50 + i * 82;
      return '<rect x="' + x + '" y="70" width="70" height="140" rx="6" class="locker"/><circle cx="' + (x + 56) + '" cy="140" r="4" class="knob2"/>' +
        '<text x="' + (x + 35) + '" y="96" text-anchor="middle" class="svg-small" style="fill:#fff">' + e[0] + "</text>";
    }).join("") + '<text x="160" y="232" text-anchor="middle" class="svg-small">lockers: one per event</text></g>';
    s += '<g data-part="v-keys"><rect x="320" y="70" width="130" height="110" rx="8" class="cabinet"/><path d="M345 150c0-20 20-20 20-40h20c0 20 20 20 20 40z" class="trophy"/><rect x="368" y="150" width="14" height="14" class="trophy"/>' +
      '<text x="385" y="196" text-anchor="middle" class="svg-small">trophy cabinet key</text></g>';
    s += '<g data-part="v-certs"><circle cx="530" cy="115" r="42" class="seal"/><circle cx="530" cy="115" r="30" class="seal-in"/><text x="530" y="120" text-anchor="middle" class="svg-small" style="fill:#fff">SEAL</text></g>';
    s += '<g data-part="v-book"><rect x="470" y="200" width="110" height="74" rx="6" class="book"/><path d="M525 200v74" class="book-line"/><path d="M482 220h32M482 234h32M536 220h32M536 234h32" class="book-line"/><text x="525" y="292" text-anchor="middle" class="svg-small">visitor book</text></g>';
    s += '<g data-part="v-purge"><rect x="330" y="230" width="60" height="64" rx="6" class="bin"/><rect x="324" y="222" width="72" height="12" rx="4" class="bin"/><rect x="348" y="250" width="24" height="20" rx="3" class="lock-fg2"/>' +
      '<text x="360" y="310" text-anchor="middle" class="svg-small">no purging</text></g>';
    // door with the taped key, or a wristband
    s += fixedSecret
      ? '<g data-part="v-identity"><rect x="250" y="340" width="140" height="44" rx="22" class="band2"/><text x="320" y="368" text-anchor="middle" class="svg-text">wristband</text></g>'
      : '<g data-part="v-tape"><rect x="270" y="326" width="100" height="60" rx="6" class="door2"/><path d="M300 352h40" class="tape"/><circle cx="314" cy="356" r="7" class="key"/><path d="M320 356h16v6" class="key-s"/><text x="320" y="398" text-anchor="middle" class="svg-small">key taped to the door</text></g>';
    room.innerHTML = s;
    LZ.refreshParts(room);
  }
  drawRoom();

  /* ---------- Step 1: the settings file ---------- */
  var LINES = [
    ["{", null],
    ['  "EventName": "Saturday Derby",', "Not a secret. The event name is safe to show."],
    ['  "TicketApi": "https://tickets.derby.internal",', "Not a secret. An address is fine to show (and it is private)."],
    ['  "Database": {', null],
    ['    "Host": "psql-derby.postgres.database.azure.com",', "Not a secret. Knowing the host does not let you in."],
    ['    "User": "derbyapp",', "Not really a secret. A user name without the password is no use."],
    ['    "Password": "Derby-Kickoff-2026!"', "BAD"],
    ["  },", null],
    ['  "Logging": { "Level": "Information" }', "Not a secret. Just how much to log."],
    ["}", null]
  ];
  var FIXED = [
    '    "Password": "@Microsoft.KeyVault(SecretUri=https://kv-derby.vault.azure.net/secrets/db-password/)"',
    '    "PasswordSecretArn": "arn:aws:secretsmanager:eu-west-2:111122223333:secret:derby/db-password"',
    '    "PasswordSecret": "projects/events-derby/secrets/db-password/versions/latest"'
  ];
  var found = false;
  function drawCfg() {
    var cfg = document.getElementById("cfg");
    cfg.innerHTML = LINES.map(function (l, i) {
      var txt = i === 6 && fixedSecret ? FIXED[LZ.ci()] : l[0];
      var cls = i === 6 ? (fixedSecret ? "fixed" : found ? "bad-line" : "") : "";
      return '<button type="button" role="listitem" class="' + cls + '" data-line="' + i + '">' + LZ.esc(txt) + "</button>";
    }).join("");
    cfg.querySelectorAll("[data-line]").forEach(function (b) {
      b.addEventListener("click", function () {
        var l = LINES[+b.getAttribute("data-line")];
        var fb = document.getElementById("find-fb");
        if (l[1] === "BAD") {
          if (fixedSecret) { fb.innerHTML = '<p class="note good">Fixed. This line now points to the vault. The password itself is not in the file.</p>'; return; }
          found = true; drawCfg();
          fb.innerHTML = '<p class="note bad"><strong>Found it.</strong> A real password in a settings file is a key taped to the door. Anyone who sees the code, a log, a backup or a screenshot now has it. It also lives for ever in git history.</p>';
          document.getElementById("swap").hidden = false;
          var t = room.querySelector('[data-part="v-tape"]');
          if (t) { t.classList.add("pulse"); setTimeout(function () { t.classList.remove("pulse"); }, 2400); }
        } else if (l[1]) {
          fb.innerHTML = '<p class="note">' + LZ.esc(l[1]) + " Keep looking.</p>";
        }
      });
    });
  }
  drawCfg();

  /* ---------- Step 2: the swap ---------- */
  var CHOICES = [
    { t: "Move it to an environment variable", ok: false, why: "Better than git, but it is still a password. It shows up in process lists, crash dumps and deploy logs, and someone must rotate it by hand." },
    { t: "Scramble it with base64", ok: false, why: "base64 is not encryption. Anyone can turn it back in one line." },
    { t: null, ok: true, why: "" },
    { t: "Email it to the team so it is not in the code", ok: false, why: "Now it is in many inboxes for ever. The key is taped to many doors." }
  ];
  function rightChoice() {
    return ["Store it in Key Vault. Give the app a managed identity with the Key Vault Secrets User role on its own vault.",
      "Store it in Secrets Manager. Give the app an IAM role that may read only its own secret.",
      "Store it in Secret Manager. Give the app a service account with Secret Accessor on its own secret only."][LZ.ci()];
  }
  function drawChoices() {
    var box = document.getElementById("swap-choices");
    box.innerHTML = CHOICES.map(function (c, i) {
      return '<button type="button" class="btn" data-c="' + i + '">' + LZ.esc(c.t || rightChoice()) + "</button>";
    }).join("");
    box.querySelectorAll("[data-c]").forEach(function (b) {
      b.addEventListener("click", function () {
        var c = CHOICES[+b.getAttribute("data-c")], fb = document.getElementById("swap-fb");
        if (!c.ok) { fb.innerHTML = '<p class="note bad"><strong>Not quite.</strong> ' + LZ.esc(c.why) + "</p>"; return; }
        fixedSecret = true; drawCfg(); drawRoom();
        fb.innerHTML = '<p class="note good"><strong>Swapped.</strong> The app now wears a wristband (' + LZ.esc(LZ.cloudName("v-identity")) +
          "). It proves who it is with a token, so there is no password to steal or rotate. Then change the old database password, because it was exposed.</p>";
      });
    });
  }
  drawChoices();

  /* ---------- Step 3: locker tests ---------- */
  var TESTS = {
    own: { ok: true, msg: ["GET https://kv-derby.vault.azure.net/secrets/db-password?api-version=7.4\nHTTP/1.1 200 OK\n{ \"value\": \"********\", \"id\": \".../secrets/db-password/1a2b\" }",
      "aws secretsmanager get-secret-value --secret-id derby/db-password\n{ \"Name\": \"derby/db-password\", \"SecretString\": \"********\" }",
      "gcloud secrets versions access latest --secret=db-password --project=events-derby\n********"],
      why: "The Derby wristband opens the Derby locker. The visitor book records it." },
    other: { ok: false, msg: ["GET https://kv-summer.vault.azure.net/secrets/db-password?api-version=7.4\nHTTP/1.1 403 Forbidden\n{ \"error\": { \"code\": \"Forbidden\", \"message\": \"Caller is not authorized to perform action on resource.\\nAction: 'Microsoft.KeyVault/vaults/secrets/getSecret/action'\", \"innererror\": { \"code\": \"ForbiddenByRbac\" } } }",
      "An error occurred (AccessDeniedException) when calling the GetSecretValue operation:\nUser: arn:aws:sts::111122223333:assumed-role/derby-app/... is not authorized to perform: secretsmanager:GetSecretValue on resource: summer/db-password",
      "ERROR: (gcloud.secrets.versions.access) PERMISSION_DENIED: Permission 'secretmanager.versions.access' denied for resource 'projects/events-summer/secrets/db-password/versions/latest'."],
      why: "Each app opens only its own locker. If the Derby app is ever broken into, Summer stays safe." },
    purge: { ok: false, msg: ["DELETE .../vaults/kv-derby  → 200 OK (soft-deleted, kept for 90 days)\nPOST .../deletedVaults/kv-derby/purge\nHTTP/1.1 409 Conflict\n{ \"error\": { \"code\": \"Conflict\", \"message\": \"Vault 'kv-derby' has purge protection enabled and cannot be purged until the retention period ends.\" } }",
      "aws kms schedule-key-deletion --key-id derby-key --pending-window-in-days 7\nKey is now PendingDeletion. It can be cancelled for 7 days (default 30).",
      "gcloud kms keys versions destroy 1 --key=derby-key ...\nState: DESTROY_SCHEDULED. The key is kept and can be restored during the destroy delay (default 30 days)."],
      why: "Purge protection: even by mistake, nobody can throw the trophies away for good. There is always time to get them back." }
  };
  document.querySelectorAll("[data-t]").forEach(function (b) {
    b.addEventListener("click", function () {
      var t = TESTS[b.getAttribute("data-t")];
      document.getElementById("test-out").innerHTML = '<div class="err' + (t.ok ? " okay" : "") + '">' + LZ.esc(t.msg[LZ.ci()]) + '</div><p class="note ' + (t.ok ? "good" : "warn") + '">' + LZ.esc(t.why) + "</p>";
    });
  });

  LZ.on("cloud", function () { drawCfg(); drawChoices(); });
  LZ.quiz(document.getElementById("quiz"), C.chapters[3].quiz, "ch4");
})();
