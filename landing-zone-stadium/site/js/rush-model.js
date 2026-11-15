/* Match day rush: a small, deterministic crowd model (one tick = one minute). No drawing here.
   Controls: auto (turnstile autoscaling), queue (calm waiting queue), limit (weight limit at the bridge
   entrance), jitter (leave stand by stand), over (overflow car park size). */
(function (root) {
  "use strict";
  var M = {
    TOTAL: 1500, MAIN_CAP: 900, SAFE: 300, WALK: 220, STAIRS: 120, TURN: 30, TURN_MAX: 200,
    BARRIER: 200, KICKOFF: 60, WHISTLE: 95, END: 135
  };
  function arrivals(t) {
    if (t >= M.KICKOFF) return 0;
    var g = Math.exp(-Math.pow(t - 42, 2) / (2 * 100));
    return g;
  }
  var norm = 0;
  for (var i = 0; i < M.KICKOFF; i++) norm += arrivals(i);

  M.init = function (ctl) {
    return { t: 0, ctl: ctl, mainOpen: true, overOpen: false, mainUsed: 0, overUsed: 0, wMain: 0, wOver: 0, bin: 0, bout: 0,
      inside: 0, exitWait: 0, exitLeft: 0, away: 0, peak: 0, carry: 0, moving: 0, movePer: 0, downAt: -1, arrived: 0, left: 0, events: [] };
  };
  M.regionDown = function (s) {
    if (!s.mainOpen) return;
    s.mainOpen = false; s.downAt = s.t;
    // fans parked in the main car park must re-park in the overflow: a retry storm unless jittered
    s.moving = s.wMain; s.wMain = 0;
    s.movePer = s.ctl.jitter ? Math.ceil(s.moving / 8) : s.moving;
    s.events.push("down");
  };
  function park(s, n) {
    var toMain = s.mainOpen ? Math.min(n, M.MAIN_CAP - s.mainUsed) : 0;
    s.mainUsed += toMain; s.wMain += toMain; n -= toMain;
    if (n > 0) {
      s.overOpen = true;
      var toOver = Math.min(n, s.ctl.over - s.overUsed);
      s.overUsed += toOver; s.wOver += toOver; n -= toOver;
    }
    s.away += n;
  }
  M.step = function (s) {
    var c = s.ctl, t = s.t;
    // 1. arrivals by road
    var exact = (arrivals(t) / norm) * M.TOTAL + s.carry, a = Math.floor(exact);
    s.carry = exact - a; s.arrived += a;
    park(s, a);
    // 2. after "region down", parked fans drive to the overflow (all at once, or spread out)
    if (s.moving > 0) { var m = Math.min(s.moving, s.movePer); s.moving -= m; park(s, m); }
    // 3. onto the bridge (towards the stadium)
    var room = c.limit ? Math.max(0, Math.floor(0.8 * M.SAFE) - s.bin - s.bout) : M.WALK;
    var W = s.wMain + s.wOver, enter = Math.min(W, room);
    var fromOver = Math.min(s.wOver, enter); s.wOver -= fromOver; s.wMain -= enter - fromOver;
    s.bin += enter;
    var before = s.bin + s.bout;
    // 4. without a calm queue, a crowd that is too big at the barrier gives up and goes home
    W = s.wMain + s.wOver;
    if (!c.queue && W > M.BARRIER) {
      var extra = W - M.BARRIER, fo = Math.min(s.wOver, extra);
      s.wOver -= fo; s.wMain -= extra - fo; s.away += extra;
    }
    // 5. turnstiles let fans off the bridge into the stadium
    var cap = c.auto ? Math.min(M.TURN_MAX, M.TURN + Math.floor((s.bin + W) * 0.6)) : M.TURN;
    var inn = Math.min(s.bin, cap); s.bin -= inn; s.inside += inn;
    s.mid = before;
    // 6. after the final whistle everyone leaves over the same bridge
    if (t === M.WHISTLE) { s.exitLeft = s.inside; }
    if (t >= M.WHISTLE && s.exitLeft > 0) {
      var rel = c.jitter ? Math.min(s.exitLeft, 100) : s.exitLeft;
      s.exitLeft -= rel; s.exitWait += rel; s.inside -= rel;
    }
    if (s.exitWait > 0) {
      var room2 = c.limit ? Math.max(0, Math.floor(0.8 * M.SAFE) - s.bin - s.bout) : M.WALK;
      var go = Math.min(s.exitWait, room2); s.exitWait -= go; s.bout += go;
    }
    s.mid = Math.max(s.mid, s.bin + s.bout);
    s.lastLoad = Math.round((s.mid / M.SAFE) * 100);
    s.peak = Math.max(s.peak, s.lastLoad);
    var off = Math.min(s.bout, M.STAIRS); s.bout -= off; s.left += off;
    s.t++;
    return s;
  };
  M.load = function (s) { return s.lastLoad || 0; };
  M.waiting = function (s) { return s.wMain + s.wOver + s.moving + s.exitWait; };
  M.run = function (ctl, downAt) {
    var s = M.init(ctl);
    while (s.t < M.END) { if (downAt != null && s.t === downAt) M.regionDown(s); M.step(s); }
    return s;
  };
  root.RushModel = M;
  if (typeof module !== "undefined") module.exports = M;
})(typeof window !== "undefined" ? window : this);
