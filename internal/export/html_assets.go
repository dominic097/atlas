package export

// htmlCSS, htmlBody, and htmlJS are the inlined, dependency-free assets for the
// self-contained graph page. They reference no network resource. The JS uses a
// seeded PRNG and a fixed iteration count, so layout is deterministic per graph.

const htmlCSS = `
:root { color-scheme: dark; }
* { box-sizing: border-box; }
html, body { margin: 0; height: 100%; }
body {
  font: 13px/1.4 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  background: #0d1117; color: #c9d1d9; overflow: hidden;
}
#title {
  position: fixed; top: 0; left: 0; right: 0; z-index: 5;
  padding: 10px 14px; background: rgba(13,17,23,.86);
  border-bottom: 1px solid #21262d; font-weight: 600; font-size: 14px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
#canvas { position: fixed; inset: 0; width: 100%; height: 100%; cursor: grab; }
#canvas.grabbing { cursor: grabbing; }
#canvas svg { width: 100%; height: 100%; display: block; }
.edge { stroke: #30363d; stroke-width: 1; }
.node { stroke: #0d1117; stroke-width: 1.5; cursor: pointer; }
.node:hover { stroke: #f0f6fc; stroke-width: 2.5; }
.nlabel { fill: #8b949e; font-size: 9px; pointer-events: none; }
#tooltip {
  position: fixed; z-index: 10; pointer-events: none; display: none;
  max-width: 360px; padding: 8px 10px; border-radius: 6px;
  background: #161b22; border: 1px solid #30363d; box-shadow: 0 6px 20px rgba(0,0,0,.5);
}
#tooltip .tname { font-weight: 600; color: #f0f6fc; margin-bottom: 3px; }
#tooltip .trow { color: #8b949e; font-size: 12px; }
#tooltip .trow b { color: #c9d1d9; font-weight: 600; }
#legend {
  position: fixed; bottom: 12px; left: 12px; z-index: 5; max-height: 42%;
  overflow: auto; padding: 8px 10px; border-radius: 6px;
  background: rgba(22,27,34,.92); border: 1px solid #30363d; font-size: 12px;
}
#legend .lhead { font-weight: 600; color: #f0f6fc; margin-bottom: 6px; }
#legend .lrow { display: flex; align-items: center; gap: 6px; margin: 2px 0; }
#legend .swatch { width: 11px; height: 11px; border-radius: 3px; flex: none; }
#hint {
  position: fixed; bottom: 12px; right: 12px; z-index: 5;
  padding: 6px 9px; border-radius: 6px; color: #8b949e;
  background: rgba(22,27,34,.92); border: 1px solid #30363d; font-size: 11px;
}
`

const htmlBody = `<div id="title">Atlas graph</div>
<div id="canvas"></div>
<div id="tooltip"></div>
<div id="legend"></div>
<div id="hint">drag node · drag bg to pan · scroll to zoom</div>
`

const htmlJS = `
(function () {
  "use strict";
  var SVGNS = "http://www.w3.org/2000/svg";
  var data = JSON.parse(document.getElementById("atlas-graph").textContent);
  document.getElementById("title").textContent = data.title;
  document.title = data.title;

  var nodes = data.nodes, edges = data.edges;

  // ---- deterministic seeded PRNG (mulberry32) -------------------------------
  function hash32(str) {
    var h = 2166136261 >>> 0;            // FNV-1a
    for (var i = 0; i < str.length; i++) {
      h ^= str.charCodeAt(i);
      h = Math.imul(h, 16777619) >>> 0;
    }
    return h >>> 0;
  }
  function mulberry32(a) {
    return function () {
      a |= 0; a = (a + 0x6D2B79F5) | 0;
      var t = Math.imul(a ^ (a >>> 15), 1 | a);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }

  // ---- categorical palette (deterministic by community id) ------------------
  var PALETTE = [
    "#58a6ff","#3fb950","#d29922","#f85149","#bc8cff","#39c5cf","#db61a2","#ff7b72",
    "#a5d6ff","#7ee787","#e3b341","#ffa657","#d2a8ff","#56d4dd","#f778ba","#ffab70"
  ];
  function colorFor(c) { return PALETTE[((c % PALETTE.length) + PALETTE.length) % PALETTE.length]; }

  // ---- layout: seeded init + fixed-iteration force simulation ---------------
  var W = 1200, H = 800;
  var index = {};
  nodes.forEach(function (n, i) { index[n.id] = i; });

  nodes.forEach(function (n) {
    var rnd = mulberry32(hash32(n.id));
    n.x = (rnd() - 0.5) * W;
    n.y = (rnd() - 0.5) * H;
    n.vx = 0; n.vy = 0;
    // radius by degree (sqrt scale keeps hubs from dominating).
    n.r = 4 + Math.sqrt(n.degree) * 2.4;
  });

  var links = [];
  edges.forEach(function (e) {
    var a = index[e.from], b = index[e.to];
    if (a === undefined || b === undefined || a === b) return;
    links.push([a, b]);
  });

  var ITER = 300, K = Math.sqrt((W * H) / Math.max(1, nodes.length));
  for (var step = 0; step < ITER; step++) {
    var t = 1 - step / ITER;          // cooling
    // repulsion (O(n^2); fine within the top-N cap).
    for (var i = 0; i < nodes.length; i++) {
      var ni = nodes[i], fx = 0, fy = 0;
      for (var j = 0; j < nodes.length; j++) {
        if (i === j) continue;
        var nj = nodes[j];
        var dx = ni.x - nj.x, dy = ni.y - nj.y;
        var d2 = dx * dx + dy * dy; if (d2 < 0.01) d2 = 0.01;
        var d = Math.sqrt(d2);
        var rep = (K * K) / d;
        fx += (dx / d) * rep; fy += (dy / d) * rep;
      }
      ni.vx = fx; ni.vy = fy;
    }
    // attraction along links.
    for (var l = 0; l < links.length; l++) {
      var a = nodes[links[l][0]], b = nodes[links[l][1]];
      var dx = a.x - b.x, dy = a.y - b.y;
      var d = Math.sqrt(dx * dx + dy * dy) || 0.01;
      var att = (d * d) / K;
      var ox = (dx / d) * att, oy = (dy / d) * att;
      a.vx -= ox; a.vy -= oy; b.vx += ox; b.vy += oy;
    }
    // integrate with capped displacement + mild gravity toward origin.
    var maxDisp = 30 * t + 1;
    for (var m = 0; m < nodes.length; m++) {
      var n = nodes[m];
      n.vx -= n.x * 0.012; n.vy -= n.y * 0.012;
      var dl = Math.sqrt(n.vx * n.vx + n.vy * n.vy) || 0.01;
      var f = Math.min(dl, maxDisp) / dl;
      n.x += n.vx * f; n.y += n.vy * f;
    }
  }

  // ---- build SVG ------------------------------------------------------------
  var svg = document.createElementNS(SVGNS, "svg");
  var root = document.createElementNS(SVGNS, "g");
  svg.appendChild(root);
  var gEdges = document.createElementNS(SVGNS, "g");
  var gNodes = document.createElementNS(SVGNS, "g");
  root.appendChild(gEdges); root.appendChild(gNodes);

  links.forEach(function (lk) {
    var a = nodes[lk[0]], b = nodes[lk[1]];
    var line = document.createElementNS(SVGNS, "line");
    line.setAttribute("class", "edge");
    line.setAttribute("x1", a.x); line.setAttribute("y1", a.y);
    line.setAttribute("x2", b.x); line.setAttribute("y2", b.y);
    gEdges.appendChild(line);
    lk.el = line;
  });

  var showLabels = nodes.length <= 120;
  nodes.forEach(function (n) {
    var c = document.createElementNS(SVGNS, "circle");
    c.setAttribute("class", "node");
    c.setAttribute("cx", n.x); c.setAttribute("cy", n.y);
    c.setAttribute("r", n.r);
    c.setAttribute("fill", colorFor(n.community));
    gNodes.appendChild(c);
    n.el = c;
    if (showLabels) {
      var tx = document.createElementNS(SVGNS, "text");
      tx.setAttribute("class", "nlabel");
      tx.setAttribute("x", n.x + n.r + 2); tx.setAttribute("y", n.y + 3);
      tx.textContent = n.name;
      gNodes.appendChild(tx);
      n.label = tx;
    }
    c.addEventListener("mouseenter", function (ev) { showTip(n, ev); });
    c.addEventListener("mousemove", function (ev) { moveTip(ev); });
    c.addEventListener("mouseleave", hideTip);
    c.addEventListener("mousedown", function (ev) { startDrag(n, ev); });
  });

  // fit viewBox to content.
  var minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  nodes.forEach(function (n) {
    minX = Math.min(minX, n.x - n.r); minY = Math.min(minY, n.y - n.r);
    maxX = Math.max(maxX, n.x + n.r); maxY = Math.max(maxY, n.y + n.r);
  });
  if (!isFinite(minX)) { minX = -W / 2; minY = -H / 2; maxX = W / 2; maxY = H / 2; }
  var pad = 40;
  svg.setAttribute("viewBox", (minX - pad) + " " + (minY - pad) + " " +
    (maxX - minX + 2 * pad) + " " + (maxY - minY + 2 * pad));
  svg.setAttribute("preserveAspectRatio", "xMidYMid meet");
  document.getElementById("canvas").appendChild(svg);

  // ---- pan / zoom -----------------------------------------------------------
  var view = { x: 0, y: 0, k: 1 };
  function applyView() {
    root.setAttribute("transform", "translate(" + view.x + "," + view.y + ") scale(" + view.k + ")");
  }
  var canvas = document.getElementById("canvas");
  canvas.addEventListener("wheel", function (ev) {
    ev.preventDefault();
    var rect = svg.getBoundingClientRect();
    var mx = ev.clientX - rect.left, my = ev.clientY - rect.top;
    var factor = Math.pow(1.0015, -ev.deltaY);
    var nk = Math.max(0.1, Math.min(8, view.k * factor));
    var r = nk / view.k;
    view.x = mx - r * (mx - view.x);
    view.y = my - r * (my - view.y);
    view.k = nk; applyView();
  }, { passive: false });

  var panning = false, panStart = null;
  canvas.addEventListener("mousedown", function (ev) {
    if (ev.target !== svg && ev.target.tagName !== "line" && ev.target.tagName !== "g" && ev.target !== canvas) return;
    panning = true; canvas.classList.add("grabbing");
    panStart = { x: ev.clientX - view.x, y: ev.clientY - view.y };
  });

  // ---- node drag (in graph coordinates) -------------------------------------
  var drag = null;
  function svgPoint(ev) {
    var rect = svg.getBoundingClientRect();
    var vb = svg.viewBox.baseVal;
    var sx = vb.width / rect.width, sy = vb.height / rect.height;
    var gx = vb.x + (ev.clientX - rect.left) * sx;
    var gy = vb.y + (ev.clientY - rect.top) * sy;
    // undo the pan/zoom transform.
    return { x: (gx - view.x) / view.k, y: (gy - view.y) / view.k };
  }
  function startDrag(n, ev) {
    ev.stopPropagation();
    drag = n; hideTip();
  }
  window.addEventListener("mousemove", function (ev) {
    if (drag) {
      var p = svgPoint(ev);
      drag.x = p.x; drag.y = p.y; redraw(drag);
    } else if (panning) {
      view.x = ev.clientX - panStart.x; view.y = ev.clientY - panStart.y; applyView();
    }
  });
  window.addEventListener("mouseup", function () {
    drag = null; panning = false; canvas.classList.remove("grabbing");
  });

  function redraw(n) {
    n.el.setAttribute("cx", n.x); n.el.setAttribute("cy", n.y);
    if (n.label) { n.label.setAttribute("x", n.x + n.r + 2); n.label.setAttribute("y", n.y + 3); }
    links.forEach(function (lk) {
      var a = nodes[lk[0]], b = nodes[lk[1]];
      if (a === n) { lk.el.setAttribute("x1", n.x); lk.el.setAttribute("y1", n.y); }
      if (b === n) { lk.el.setAttribute("x2", n.x); lk.el.setAttribute("y2", n.y); }
    });
  }

  // ---- tooltip --------------------------------------------------------------
  var tip = document.getElementById("tooltip");
  function esc(s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }
  function showTip(n, ev) {
    var loc = n.path ? n.path + (n.line ? ":" + n.line : "") : "(no path)";
    tip.innerHTML =
      '<div class="tname">' + esc(n.name) + '</div>' +
      '<div class="trow"><b>' + esc(n.kind || "symbol") + '</b>' + (n.language ? ' · ' + esc(n.language) : '') + '</div>' +
      '<div class="trow">' + esc(loc) + '</div>' +
      '<div class="trow">degree <b>' + n.degree + '</b> · community <b>' + n.community + '</b></div>';
    tip.style.display = "block"; moveTip(ev);
  }
  function moveTip(ev) {
    var x = ev.clientX + 14, y = ev.clientY + 14;
    var w = tip.offsetWidth, h = tip.offsetHeight;
    if (x + w > window.innerWidth) x = ev.clientX - w - 14;
    if (y + h > window.innerHeight) y = ev.clientY - h - 14;
    tip.style.left = x + "px"; tip.style.top = y + "px";
  }
  function hideTip() { tip.style.display = "none"; }

  // ---- legend ---------------------------------------------------------------
  var present = {};
  nodes.forEach(function (n) { present[n.community] = (present[n.community] || 0) + 1; });
  var cids = Object.keys(present).map(Number).sort(function (a, b) { return a - b; });
  var legend = document.getElementById("legend");
  var html = '<div class="lhead">Communities (' + cids.length + ')</div>';
  cids.forEach(function (c) {
    html += '<div class="lrow"><span class="swatch" style="background:' + colorFor(c) + '"></span>' +
      'community ' + c + ' · ' + present[c] + '</div>';
  });
  legend.innerHTML = html;
})();
`
