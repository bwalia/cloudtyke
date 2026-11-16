/* CloudTyke Arena, "Azure landing zones" section: shared helpers.
   SVG builders, a simple icon set, clickable nodes with a details panel, a step-through,
   segmented toggles and a packet animation. Classic script (no modules) so pages open from file://.
   Needs core.js (window.LZ) for the top bar and theme. */
(function () {
  "use strict";
  var A = (window.ALZ = {});
  var esc = function (s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  };
  A.esc = esc;
  A.reduced = !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);

  /* ---------- pages of the section ---------- */
  A.pages = [
    { f: "index.html", t: "Overview", d: "What an Azure landing zone is, and the map of this section.", ic: "map" },
    { f: "hierarchy.html", t: "Management groups", d: "The reference hierarchy, identity and access, and how policy flows down.", ic: "folder" },
    { f: "design-areas.html", t: "8 design areas", d: "The decisions behind every landing zone, and the Azure services for each.", ic: "grid" },
    { f: "platform-vs-app.html", t: "Platform vs application", d: "Who owns what: platform team, app teams and the shared middle.", ic: "users" },
    { f: "network.html", t: "Network topology", d: "Hub and spoke or Virtual WAN, packet paths and a decision guide.", ic: "net" },
    { f: "management-security.html", t: "Management and security", d: "Monitoring, Defender, Sentinel, updates, backup, Key Vault, policy and cost.", ic: "shieldcheck" },
    { f: "services.html", t: "Services catalogue", d: "Where compute, data, integration and AI services land, and what they lean on.", ic: "cube" },
    { f: "vending.html", t: "Subscription vending", d: "From a request to a ready landing zone, step by step.", ic: "vend" },
    { f: "greenfield.html", t: "A. Start fresh", d: "Build the platform first, then the first workload.", ic: "flag" },
    { f: "migrate.html", t: "B. Migrate", d: "Move on-premises workloads: assess, connect, the 5 Rs, waves, cutover.", ic: "truck" },
    { f: "brownfield.html", t: "C. Upgrade an estate", d: "Bring sprawling subscriptions under a landing zone, safely.", ic: "refresh" },
    { f: "modernise.html", t: "D and E. Modernise and lifecycle", d: "VMs to containers, SQL to PaaS, sandbox to production, decommission.", ic: "rocket" },
    { f: "benefits.html", t: "Benefits", d: "Sprawl vs governed, and the evidence for each benefit.", ic: "check" }
  ];

  /* ---------- icons (24 x 24, stroke = currentColor) ---------- */
  var I = {
    user: '<circle cx="12" cy="8" r="4"/><path d="M4 21c0-4.2 3.6-6.5 8-6.5s8 2.3 8 6.5"/>',
    users: '<circle cx="9" cy="8" r="3.4"/><path d="M2.5 20c0-3.6 3-5.6 6.5-5.6s6.5 2 6.5 5.6"/><circle cx="17" cy="7.5" r="2.8"/><path d="M16.5 13.2c3 .2 5 2 5 5"/>',
    robot: '<rect x="5" y="8" width="14" height="11" rx="2.5"/><circle cx="9.5" cy="13" r="1.2"/><circle cx="14.5" cy="13" r="1.2"/><path d="M12 4v4M9 16.5h6"/>',
    key: '<circle cx="8" cy="12" r="4"/><path d="M12 12h9M18 12v3.5M21 12v3"/>',
    shield: '<path d="M12 3l8 3v6c0 5-3.5 8-8 9.2C7.5 20 4 17 4 12V6z"/>',
    shieldcheck: '<path d="M12 3l8 3v6c0 5-3.5 8-8 9.2C7.5 20 4 17 4 12V6z"/><path d="M8.5 12.2l2.4 2.4 4.6-4.8"/>',
    lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>',
    folder: '<path d="M3 6.5h6.5l2 2.2H21V19H3z"/>',
    sub: '<rect x="3" y="5" width="18" height="14" rx="2.5"/><circle cx="9" cy="12" r="2.6"/><path d="M11.6 12H18M16 12v2.4"/>',
    net: '<circle cx="12" cy="12" r="3"/><circle cx="4.5" cy="5" r="2"/><circle cx="19.5" cy="5" r="2"/><circle cx="4.5" cy="19" r="2"/><circle cx="19.5" cy="19" r="2"/><path d="M6 6.4l3.8 3.6M18 6.4l-3.8 3.6M6 17.6l3.8-3.6M18 17.6l-3.8-3.6"/>',
    fw: '<rect x="3" y="5" width="18" height="14" rx="1"/><path d="M3 9.7h18M3 14.3h18M9 5v4.7M15 5v4.7M6 9.7v4.6M12 9.7v4.6M18 9.7v4.6M9 14.3V19M15 14.3V19"/>',
    gw: '<path d="M4 8.5h14l-3.5-3.5M20 15.5H6l3.5 3.5"/>',
    door: '<rect x="6" y="3" width="12" height="18" rx="1"/><circle cx="14.8" cy="12" r="1.1"/>',
    dns: '<path d="M12 3v18M6 5.5h11l2.2 2.2L17 10H6zM18 12.5H7l-2.2 2.2L7 17h11z"/>',
    globe: '<circle cx="12" cy="12" r="9"/><ellipse cx="12" cy="12" rx="3.8" ry="9"/><path d="M3 12h18"/>',
    log: '<rect x="5" y="3" width="14" height="18" rx="2"/><path d="M8.5 8h7M8.5 12h7M8.5 16h4.5"/>',
    chart: '<path d="M4 20V11M10 20V5M16 20v-7M3 20h18"/>',
    eye: '<path d="M2 12s3.8-6.5 10-6.5S22 12 22 12s-3.8 6.5-10 6.5S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
    policy: '<rect x="5" y="3" width="14" height="18" rx="2"/><path d="M8.8 12.3l2.2 2.2 4.2-4.4"/>',
    coin: '<circle cx="12" cy="12" r="8.5"/><path d="M12 6.5v11M14.6 9.3c-.5-.9-1.4-1.4-2.6-1.4-1.5 0-2.6.8-2.6 2 0 2.6 5.3 1.5 5.3 4.2 0 1.2-1.2 2-2.7 2-1.2 0-2.2-.5-2.7-1.5"/>',
    server: '<rect x="4" y="4" width="16" height="7" rx="1.5"/><rect x="4" y="13" width="16" height="7" rx="1.5"/><path d="M7.5 7.5h.01M7.5 16.5h.01"/>',
    vm: '<rect x="3" y="4" width="18" height="12" rx="1.5"/><path d="M8 20h8M12 16v4"/>',
    cube: '<path d="M12 3l8 4.5v9L12 21l-8-4.5v-9z"/><path d="M4 7.5l8 4.5 8-4.5M12 12v9"/>',
    wheel: '<circle cx="12" cy="12" r="8.5"/><circle cx="12" cy="12" r="2.4"/><path d="M12 3.5v6.1M12 14.4v6.1M3.5 12h6.1M14.4 12h6.1M6 6l4.3 4.3M13.7 13.7L18 18M18 6l-4.3 4.3M10.3 13.7L6 18"/>',
    db: '<ellipse cx="12" cy="6" rx="7" ry="3"/><path d="M5 6v12c0 1.7 3.1 3 7 3s7-1.3 7-3V6M5 12c0 1.7 3.1 3 7 3s7-1.3 7-3"/>',
    storage: '<rect x="3" y="4" width="18" height="5" rx="1"/><rect x="3" y="10.5" width="18" height="5" rx="1"/><path d="M3 17h18M6.5 6.5h.01M6.5 13h.01"/>',
    queue: '<path d="M3 7h14M3 12h14M3 17h9"/><path d="M18 14l3 3-3 3"/>',
    bolt: '<path d="M13 2.5L5 14h6l-1 7.5L18 10h-6z"/>',
    api: '<path d="M8 4c-2 0-3 1-3 3v2.5c0 1-.8 2-2 2.5 1.2.5 2 1.5 2 2.5V17c0 2 1 3 3 3M16 4c2 0 3 1 3 3v2.5c0 1 .8 2 2 2.5-1.2.5-2 1.5-2 2.5V17c0 2-1 3-3 3"/>',
    fn: '<path d="M15 4c-3 0-3.4 2-3.9 4.8L9.6 16c-.5 2.8-1.2 4-3.6 4M7.5 10h8"/>',
    flow: '<circle cx="5" cy="6" r="2.2"/><circle cx="19" cy="6" r="2.2"/><circle cx="12" cy="18" r="2.2"/><path d="M7.2 6h9.6M17.8 7.9l-4.6 8.2M6.2 7.9l4.6 8.2"/>',
    ai: '<path d="M11 3l1.8 5.2L18 10l-5.2 1.8L11 17l-1.8-5.2L4 10l5.2-1.8z"/><path d="M18.5 15l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8z"/>',
    layers: '<path d="M12 3l9 5-9 5-9-5z"/><path d="M3 12.5l9 5 9-5M3 16.5l9 5 9-5"/>',
    clock: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7v5.2l3.4 2"/>',
    building: '<rect x="4" y="3" width="10" height="18"/><path d="M14 9h6v12h-6M7 7h4M7 11h4M7 15h4M17 13h.01M17 17h.01"/>',
    archive: '<rect x="3" y="4" width="18" height="4.5" rx="1"/><path d="M5 8.5V20h14V8.5M10 12.5h4"/>',
    trash: '<path d="M4 6.5h16M9 6.5V4h6v2.5M6.5 6.5l1 14h9l1-14M10 10.5v6.5M14 10.5v6.5"/>',
    flask: '<path d="M9 3h6M10 3v6.2L4.8 19c-.5 1 .1 2 1.2 2h12c1.1 0 1.7-1 1.2-2L14 9.2V3M7.5 14h9"/>',
    refresh: '<path d="M20 12a8 8 0 1 1-2.4-5.7M20 4v4.5h-4.5"/>',
    backup: '<path d="M4 12a8 8 0 1 0 2.4-5.7M4 4v4.5h4.5M12 8v4.3l3 1.8"/>',
    radar: '<circle cx="12" cy="12" r="8.5"/><circle cx="12" cy="12" r="4.5"/><path d="M12 12l6-6"/><circle cx="12" cy="12" r="1"/>',
    vault: '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="12" cy="12" r="4"/><path d="M12 8v1.5M12 14.5V16M8 12h1.5M14.5 12H16M6 20v1.5M18 20v1.5"/>',
    card: '<rect x="3" y="5.5" width="18" height="13" rx="2"/><path d="M3 10h18M7 14.5h4"/>',
    lb: '<circle cx="12" cy="4.5" r="2"/><circle cx="5" cy="19" r="2"/><circle cx="12" cy="19" r="2"/><circle cx="19" cy="19" r="2"/><path d="M12 6.5v10.5M12 10l-7 7M12 10l7 7"/>',
    waf: '<path d="M12 3l8 3v6c0 5-3.5 8-8 9.2C7.5 20 4 17 4 12V6z"/><path d="M8 10.5h8M8 14h8M12 10.5V14M10 7.5v3M14 14v3"/>',
    check: '<path d="M5 12.5l4.5 4.5L19 7.5"/>',
    x: '<path d="M6 6l12 12M18 6L6 18"/>',
    arrow: '<path d="M4 12h15M14 7l5 5-5 5"/>',
    tag: '<path d="M3 12V4h8l10 10-8 8z"/><circle cx="7.5" cy="8.5" r="1.4"/>',
    pipe: '<circle cx="4.5" cy="12" r="2.2"/><circle cx="12" cy="12" r="2.2"/><circle cx="19.5" cy="12" r="2.2"/><path d="M6.7 12h3.1M14.2 12h3.1M12 4v5.8M12 14.2V20"/>',
    vend: '<rect x="4" y="3" width="16" height="18" rx="2"/><path d="M8 7h8M8 10.5h8M8 14h3M14 17l2.5-2.5L14 12"/>',
    map: '<path d="M3 6l6-2.5 6 2.5 6-2.5v14.5l-6 2.5-6-2.5-6 2.5z"/><path d="M9 3.5v14.5M15 6v14.5"/>',
    grid: '<rect x="3.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.5"/>',
    flag: '<path d="M5 21V4M5 4h11l-2 4 2 4H5"/>',
    truck: '<rect x="2.5" y="6" width="11" height="10" rx="1"/><path d="M13.5 9.5h4l3.5 3.5v3h-7.5"/><circle cx="7" cy="18" r="2"/><circle cx="17" cy="18" r="2"/>',
    rocket: '<path d="M12 3c3.5 2 5 5.5 5 9.5l-2.5 3h-5L7 12.5C7 8.5 8.5 5 12 3z"/><circle cx="12" cy="10" r="1.8"/><path d="M9.5 15.5L8 20l4-2 4 2-1.5-4.5"/>',
    bell: '<path d="M6 16.5V11a6 6 0 0 1 12 0v5.5l1.5 2h-15z"/><path d="M10 20.5a2 2 0 0 0 4 0"/>',
    mfa: '<rect x="7" y="2.5" width="10" height="19" rx="2"/><path d="M10 18h4M9.5 9.5l2 2 3-3.2"/>',
    audit: '<circle cx="10.5" cy="10.5" r="6"/><path d="M15 15l5.5 5.5M8 10.5h5"/>',
    review: '<rect x="4" y="3" width="16" height="18" rx="2"/><path d="M8 8.5l1.5 1.5L12 7.5M8 14.5l1.5 1.5 2.5-2.5M14 9h3M14 15h3"/>',
    approve: '<circle cx="12" cy="12" r="8.5"/><path d="M8.2 12.3l2.6 2.6 5-5.2"/>',
    stadium: '<ellipse cx="12" cy="12" rx="10" ry="7.5"/><rect x="7.5" y="8.5" width="9" height="7" rx="1"/><path d="M12 8.5v7"/>',
    sap: '<rect x="3" y="6" width="18" height="12" rx="2"/><path d="M7 14.5c.5.6 1.3 1 2.2 1 1 0 1.8-.5 1.8-1.4 0-1.9-3.8-1.1-3.8-3 0-.9.8-1.4 1.8-1.4.8 0 1.5.3 1.9.8M13.5 15.3l1.8-6.2 1.8 6.2M14 13.5h2.6"/>',
    hourglass: '<path d="M6 3h12M6 21h12M7.5 3c0 5 9 6 9 9s-9 4-9 9M16.5 3c0 5-9 6-9 9s9 4 9 9"/>'
  };
  A.icons = I;
  /* icon(name, cx, cy, size): an icon centred on cx, cy */
  A.icon = function (name, cx, cy, size, cls) {
    var s = (size || 20) / 24;
    return '<g class="ic' + (cls ? " " + cls : "") + '" transform="translate(' + (cx - 12 * s).toFixed(1) + " " + (cy - 12 * s).toFixed(1) + ") scale(" + s.toFixed(3) + ')" fill="none" stroke="currentColor" stroke-width="' + (1.9 / Math.max(s, 0.6)).toFixed(2) + '" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + (I[name] || "") + "</g>";
  };
  A.iconSvg = function (name, size) {
    return '<svg aria-hidden="true" width="' + size + '" height="' + size + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">' + (I[name] || "") + "</svg>";
  };

  /* ---------- text ---------- */
  A.wrap = function (s, n) {
    var words = String(s).split(" "), lines = [], cur = "";
    words.forEach(function (w) {
      if (!cur) cur = w;
      else if ((cur + " " + w).length <= n) cur += " " + w;
      else { lines.push(cur); cur = w; }
    });
    if (cur) lines.push(cur);
    return lines;
  };
  /* text(x, y, string or array of lines, class, anchor, line height, max width) */
  A.text = function (x, y, s, cls, anchor, lh, max) {
    var lines = Array.isArray(s) ? s : [s];
    lh = lh || 15;
    return lines.map(function (l, i) {
      return '<text x="' + x + '" y="' + (y + i * lh) + '" class="' + (cls || "t-s") + '"' + (anchor ? ' text-anchor="' + anchor + '"' : "") + (max ? ' data-max="' + max + '"' : "") + ">" + esc(l) + "</text>";
    }).join("");
  };

  /* ---------- shapes ---------- */
  /* node(o): a clickable box. o = { id, x, y, w, h, k (colour family), t (title), s (subtitle or lines),
     ic (icon), center (bool), rx, extra (svg string), label (aria label) } */
  A.node = function (o) {
    var h = o.h || 48, w = o.w || 140, rx = o.rx == null ? 10 : o.rx, out = "";
    out += '<g class="nd k-' + (o.k || "chip") + (o.cls ? " " + o.cls : "") + '" data-node="' + o.id + '"' + (o.label ? ' data-label="' + esc(o.label) + '"' : "") + (o.attrs || "") + ">";
    if (o.stack) out += '<rect class="box" x="' + (o.x + 6) + '" y="' + (o.y - 6) + '" width="' + w + '" height="' + h + '" rx="' + rx + '" opacity=".55"/>';
    out += '<rect class="box" x="' + o.x + '" y="' + o.y + '" width="' + w + '" height="' + h + '" rx="' + rx + '"/>';
    var sub = o.s == null ? [] : Array.isArray(o.s) ? o.s : [o.s];
    var tl = Array.isArray(o.t) ? o.t : [o.t];
    var tlh = o.tlh || 16, slh = o.slh || 14;
    var blockH = tl.length * tlh + sub.length * slh + (sub.length ? 2 : 0);
    if (o.center || o.icTop) {
      var cx = o.x + w / 2, top = o.y + (h - blockH - (o.icTop ? 26 : 0)) / 2;
      if (o.icTop) { out += A.icon(o.ic, cx, top + 10, 20); top += 26; }
      out += A.text(cx, top + 12, tl, o.tc || "t-h", "middle", tlh, w - 12);
      out += A.text(cx, top + 12 + (tl.length - 1) * tlh + slh + 1, sub, o.sc || "t-xs", "middle", slh, w - 12);
    } else {
      var tx = o.x + (o.ic ? 38 : 12), top2 = o.y + (h - blockH) / 2;
      if (o.ic) out += A.icon(o.ic, o.x + 20, o.y + h / 2, o.isz || 20);
      out += A.text(tx, top2 + 12, tl, o.tc || "t-h", null, tlh, w - (tx - o.x) - 8);
      out += A.text(tx, top2 + 12 + (tl.length - 1) * tlh + slh + 1, sub, o.sc || "t-xs", null, slh, w - (tx - o.x) - 8);
    }
    if (o.extra) out += o.extra;
    return out + "</g>";
  };
  A.rect = function (x, y, w, h, cls, rx) {
    return '<rect class="' + cls + '" x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" rx="' + (rx == null ? 14 : rx) + '"/>';
  };
  /* elbow connector from (x1,y1) down to (x2,y2), turning at my */
  A.elbow = function (x1, y1, x2, y2, cls, my, attrs) {
    my = my == null ? (y1 + y2) / 2 : my;
    var d = x1 === x2 ? "M" + x1 + " " + y1 + "V" + y2 : "M" + x1 + " " + y1 + "V" + my + "H" + x2 + "V" + y2;
    return '<path class="edge ' + (cls || "") + '" d="' + d + '"' + (attrs || "") + "/>";
  };
  A.path = function (pts, cls, attrs) {
    return '<path class="edge ' + (cls || "") + '" d="' + pts.map(function (p, i) { return (i ? "L" : "M") + p[0] + " " + p[1]; }).join("") + '"' + (attrs || "") + "/>";
  };
  A.arrow = function (x, y, dir, cls) {
    var r = { down: 0, up: 180, right: -90, left: 90 }[dir || "down"];
    return '<path class="arrowhead ' + (cls || "") + '" d="M-5 -8L0 0L5 -8z" transform="translate(' + x + " " + y + ") rotate(" + r + ')"/>';
  };
  A.bandBox = function (x, y, w, h, title, cls) {
    return '<rect class="band ' + (cls || "") + '" x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" rx="16"/>' +
      (title ? '<text x="' + (x + 16) + '" y="' + (y + 22) + '" class="t-band">' + esc(title) + "</text>" : "");
  };
  A.zone = function (x, y, w, h, color, title, tcls) {
    return '<rect class="zone" x="' + x + '" y="' + y + '" width="' + w + '" height="' + h + '" rx="16" style="stroke:var(' + color + ')"/>' +
      (title ? '<text x="' + (x + 14) + '" y="' + (y + 20) + '" class="' + (tcls || "t-band") + '" style="fill:var(' + color + ')">' + esc(title) + "</text>" : "");
  };

  /* Shrink any text with data-max that is wider than its box, once fonts are ready. */
  A.fit = function (svg) {
    function go() {
      svg.querySelectorAll("text[data-max]").forEach(function (t) {
        var max = +t.getAttribute("data-max"), len;
        try { len = t.getComputedTextLength(); } catch (e) { return; }
        if (len > max && len > 0) {
          var fs = parseFloat(getComputedStyle(t).fontSize) || 12;
          t.style.fontSize = Math.max(8.5, fs * max / len - 0.2).toFixed(2) + "px";
        }
      });
    }
    go();
    if (document.fonts && document.fonts.ready) document.fonts.ready.then(go);
  };

  /* ---------- details panel ---------- */
  A.panelHtml = function (d) {
    if (!d) return "";
    var h = (d.k ? '<p class="kicker">' + esc(d.k) + "</p>" : "") + "<h3>" + esc(d.t) + "</h3>" + (d.d ? "<p>" + d.d + "</p>" : "");
    (d.s || []).forEach(function (sec) {
      var body = sec[1];
      if (Array.isArray(body)) body = "<ul>" + body.map(function (x) { return "<li>" + x + "</li>"; }).join("") + "</ul>";
      else body = "<p>" + body + "</p>";
      h += '<div class="pb"><h4>' + esc(sec[0]) + "</h4>" + body + "</div>";
    });
    if (d.ct) h += '<div class="ct"><h4>Practise in CloudTyke</h4><p>' + d.ct + "</p></div>";
    return h;
  };
  /* bind(svg, data, panel, opts): make every [data-node] a button that fills the panel.
     opts.render(id) returns custom html; opts.onSelect(id); opts.first = id to pre-select; opts.dim = true to dim others */
  A.bind = function (svg, data, panel, opts) {
    opts = opts || {};
    var api = { current: null };
    svg.classList.add("alz-svg");
    function decorate() {
      svg.querySelectorAll("[data-node]").forEach(function (el) {
        var id = el.getAttribute("data-node"), d = data[id];
        if (!d) return;
        el.setAttribute("role", "button");
        el.setAttribute("tabindex", "0");
        el.setAttribute("aria-label", (el.getAttribute("data-label") || d.t) + (d.k ? ", " + d.k : "") + ". Show details.");
        el.setAttribute("aria-controls", panel.id);
      });
    }
    decorate();
    api.refresh = function () { decorate(); A.fit(svg); if (api.current) select(api.current, true); };
    function select(id, quiet) {
      var d = data[id];
      if (!d) return;
      api.current = id;
      svg.querySelectorAll(".sel, .rel").forEach(function (n) { n.classList.remove("sel"); n.classList.remove("rel"); });
      svg.querySelectorAll('[data-node="' + id + '"]').forEach(function (n) { n.classList.add("sel"); n.setAttribute("aria-pressed", "true"); });
      svg.querySelectorAll("[aria-pressed]").forEach(function (n) { if (n.getAttribute("data-node") !== id) n.removeAttribute("aria-pressed"); });
      (d.rel || []).forEach(function (r) { svg.querySelectorAll('[data-node="' + r + '"]').forEach(function (n) { n.classList.add("rel"); }); });
      if (opts.dim) svg.classList.toggle("dimming", !!(d.rel && d.rel.length) || !!opts.dimAlways);
      panel.innerHTML = opts.render ? opts.render(id, d) : A.panelHtml(d);
      if (opts.onSelect) opts.onSelect(id, d);
      if (!quiet && window.innerWidth <= 1180 && panel.getBoundingClientRect().top > window.innerHeight) {
        panel.scrollIntoView({ behavior: A.reduced ? "auto" : "smooth", block: "nearest" });
      }
    }
    api.select = select;
    api.clear = function () {
      api.current = null;
      svg.classList.remove("dimming");
      svg.querySelectorAll(".sel, .rel").forEach(function (n) { n.classList.remove("sel"); n.classList.remove("rel"); });
      svg.querySelectorAll("[aria-pressed]").forEach(function (n) { n.removeAttribute("aria-pressed"); });
      panel.innerHTML = opts.empty || '<p class="hint">Click, tap or Tab to any box and press Enter. Its details show here.</p>';
    };
    svg.addEventListener("click", function (e) {
      var el = e.target.closest && e.target.closest("[data-node]");
      if (el && svg.contains(el)) select(el.getAttribute("data-node"));
    });
    svg.addEventListener("keydown", function (e) {
      if (e.key !== "Enter" && e.key !== " ") return;
      var el = e.target.closest && e.target.closest("[data-node]");
      if (!el || el !== e.target) return;
      e.preventDefault();
      select(el.getAttribute("data-node"));
    });
    panel.setAttribute("aria-live", "polite");
    if (opts.first) select(opts.first, true);
    else panel.innerHTML = opts.empty || '<p class="hint">Click, tap or Tab to any box and press Enter. Its details show here.</p>';
    A.fit(svg);
    return api;
  };

  /* ---------- segmented toggle ---------- */
  A.seg = function (box, label, options, onChange, start) {
    var cur = start || options[0][0];
    box.innerHTML = '<span class="lbl" id="' + box.id + '-l">' + esc(label) + '</span><div class="seg" role="radiogroup" aria-labelledby="' + box.id + '-l">' +
      options.map(function (o) { return '<button type="button" role="radio" class="seg-btn" data-v="' + o[0] + '">' + esc(o[1]) + "</button>"; }).join("") + "</div>";
    var btns = box.querySelectorAll("[data-v]");
    function set(v, focus) {
      cur = v;
      btns.forEach(function (b) {
        var on = b.getAttribute("data-v") === v;
        b.setAttribute("aria-checked", on ? "true" : "false");
        b.tabIndex = on ? 0 : -1;
        if (on && focus) b.focus();
      });
      onChange(v);
    }
    btns.forEach(function (b, i) {
      b.addEventListener("click", function () { set(b.getAttribute("data-v")); });
      b.addEventListener("keydown", function (e) {
        var d = e.key === "ArrowRight" || e.key === "ArrowDown" ? 1 : e.key === "ArrowLeft" || e.key === "ArrowUp" ? -1 : 0;
        if (!d) return;
        e.preventDefault();
        set(btns[(i + d + btns.length) % btns.length].getAttribute("data-v"), true);
      });
    });
    set(cur);
    return { set: set, get: function () { return cur; } };
  };

  /* ---------- step-through ---------- */
  /* stepper(box, steps [{t, d}], apply(i, step)) */
  A.stepper = function (box, steps, apply, opts) {
    opts = opts || {};
    var i = -1, timer = null;
    box.classList.add("stepper");
    box.innerHTML =
      '<div class="row"><button type="button" class="btn" data-a="prev">← Back</button>' +
      '<button type="button" class="btn primary" data-a="next">Next step →</button>' +
      '<button type="button" class="btn ghost" data-a="play" aria-pressed="false">' + (A.reduced ? "Show all" : "Play") + "</button>" +
      '<button type="button" class="btn ghost" data-a="reset">Reset</button></div>' +
      '<div class="dots" role="group" aria-label="Jump to a step">' + steps.map(function (s, k) {
        return '<button type="button" data-k="' + k + '" aria-label="Step ' + (k + 1) + ": " + esc(s.t) + '">' + (k + 1) + "</button>";
      }).join("") + "</div>" +
      '<div aria-live="polite"><p class="st-title"></p><p class="st-text"></p></div>';
    var title = box.querySelector(".st-title"), text = box.querySelector(".st-text");
    function go(k) {
      i = Math.max(-1, Math.min(steps.length - 1, k));
      box.querySelectorAll(".dots button").forEach(function (b, n) {
        b.classList.toggle("past", n < i);
        if (n === i) b.setAttribute("aria-current", "step"); else b.removeAttribute("aria-current");
      });
      if (i < 0) { title.textContent = opts.startTitle || "Ready"; text.innerHTML = opts.startText || "Press Next step to begin."; }
      else { title.textContent = "Step " + (i + 1) + " of " + steps.length + ": " + steps[i].t; text.innerHTML = steps[i].d; }
      box.querySelector('[data-a="prev"]').disabled = i < 0;
      box.querySelector('[data-a="next"]').disabled = i >= steps.length - 1;
      apply(i, steps[i]);
    }
    function stop() { if (timer) { clearInterval(timer); timer = null; } var p = box.querySelector('[data-a="play"]'); p.setAttribute("aria-pressed", "false"); p.textContent = A.reduced ? "Show all" : "Play"; }
    box.addEventListener("click", function (e) {
      var b = e.target.closest("button");
      if (!b) return;
      var a = b.getAttribute("data-a");
      if (b.hasAttribute("data-k")) { stop(); go(+b.getAttribute("data-k")); return; }
      if (a === "prev") { stop(); go(i - 1); }
      else if (a === "next") { stop(); go(i + 1); }
      else if (a === "reset") { stop(); go(-1); }
      else if (a === "play") {
        if (timer) { stop(); return; }
        if (A.reduced) { go(steps.length - 1); return; }
        if (i >= steps.length - 1) go(-1);
        b.setAttribute("aria-pressed", "true"); b.textContent = "Pause";
        go(i + 1);
        timer = setInterval(function () { if (i >= steps.length - 1) stop(); else go(i + 1); }, opts.interval || 2600);
      }
    });
    go(opts.start == null ? -1 : opts.start);
    return { go: go, get: function () { return i; } };
  };

  /* mark(svg, ids, cls): add cls to the nodes with these ids, remove it from all others */
  A.mark = function (svg, ids, cls) {
    svg.querySelectorAll("." + cls).forEach(function (n) { n.classList.remove(cls); });
    (ids || []).forEach(function (id) { svg.querySelectorAll('[data-node="' + id + '"], [data-id="' + id + '"]').forEach(function (n) { n.classList.add(cls); }); });
  };

  /* ---------- packet animation along a polyline ---------- */
  A.travel = function (layer, pts, opts) {
    opts = opts || {};
    if (A._raf) cancelAnimationFrame(A._raf);
    var segs = [], total = 0;
    for (var k = 1; k < pts.length; k++) {
      var dx = pts[k][0] - pts[k - 1][0], dy = pts[k][1] - pts[k - 1][1], L = Math.sqrt(dx * dx + dy * dy);
      segs.push(L); total += L;
    }
    var d = pts.map(function (p, i) { return (i ? "L" : "M") + p[0] + " " + p[1]; }).join("");
    layer.innerHTML = '<path class="pkt-trail" d="' + d + '" pathLength="1000" stroke-dasharray="1000" stroke-dashoffset="1000"/><circle class="pkt" r="8" cx="' + pts[0][0] + '" cy="' + pts[0][1] + '"/>';
    var trail = layer.querySelector(".pkt-trail"), dot = layer.querySelector(".pkt");
    if (A.reduced || opts.instant) {
      trail.setAttribute("stroke-dashoffset", "0");
      dot.setAttribute("cx", pts[pts.length - 1][0]); dot.setAttribute("cy", pts[pts.length - 1][1]);
      if (opts.onPoint) for (var j = 0; j < pts.length; j++) opts.onPoint(j);
      if (opts.done) opts.done();
      return;
    }
    var speed = opts.speed || 0.32, t0 = null, lastIdx = -1;
    function frame(ts) {
      if (!t0) t0 = ts;
      var dist = Math.min(total, (ts - t0) * speed), acc = 0, idx = 0;
      while (idx < segs.length && acc + segs[idx] < dist) { acc += segs[idx]; idx++; }
      var p;
      if (idx >= segs.length) p = pts[pts.length - 1];
      else {
        var f = segs[idx] ? (dist - acc) / segs[idx] : 0;
        p = [pts[idx][0] + (pts[idx + 1][0] - pts[idx][0]) * f, pts[idx][1] + (pts[idx + 1][1] - pts[idx][1]) * f];
      }
      dot.setAttribute("cx", p[0].toFixed(1)); dot.setAttribute("cy", p[1].toFixed(1));
      trail.setAttribute("stroke-dashoffset", (1000 - (1000 * dist) / total).toFixed(1));
      if (opts.onPoint) while (lastIdx < idx) { lastIdx++; opts.onPoint(lastIdx); }
      if (dist < total) A._raf = requestAnimationFrame(frame);
      else { if (opts.onPoint) while (lastIdx < pts.length - 1) { lastIdx++; opts.onPoint(lastIdx); } if (opts.done) opts.done(); }
    }
    A._raf = requestAnimationFrame(frame);
  };

  /* ---------- page chrome ---------- */
  A.boot = function () {
    if (window.LZ && LZ.boot) LZ.boot();
    document.body.classList.add("alz");
    var here = (location.pathname.split("/").pop() || "index.html");
    var nav = document.getElementById("alz-nav");
    if (nav) {
      nav.innerHTML = '<ul class="alz-nav">' + A.pages.map(function (p) {
        return '<li><a href="' + p.f + '"' + (p.f === here ? ' aria-current="page"' : "") + ">" + esc(p.t) + "</a></li>";
      }).join("") + "</ul>";
    }
    var pager = document.getElementById("alz-pager");
    if (pager) {
      var i = -1;
      A.pages.forEach(function (p, k) { if (p.f === here) i = k; });
      var prev = A.pages[i - 1], next = A.pages[i + 1];
      pager.className = "chapnav";
      pager.setAttribute("aria-label", "Azure landing zones pages");
      pager.innerHTML = (prev ? '<a class="btn" href="' + prev.f + '">← ' + esc(prev.t) + "</a>" : '<a class="btn" href="../index.html">← Arena home</a>') +
        '<a class="btn ghost" href="index.html">All Azure pages</a>' +
        (next ? '<a class="btn primary" href="' + next.f + '">' + esc(next.t) + " →</a>" : '<a class="btn primary" href="../index.html">Back to the Arena →</a>');
    }
    document.querySelectorAll("[data-ic]").forEach(function (el) { el.innerHTML = A.iconSvg(el.getAttribute("data-ic"), +(el.getAttribute("data-size") || 40)); });
  };
})();
