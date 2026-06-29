import React, { useEffect, useMemo, useRef, useState } from "react";

// Atlas — The Benchmark Instrument
// Interactive deterministic force-directed code graph on a vanilla <canvas>.
// No external libraries / CDN. Self-fetches data/graph.json on mount.

// Community accent hues, matching the locked design brief g0..g5.
const COMMUNITY_COLORS = [
  "#5EE6C4", // g0 mint
  "#7AA2FF", // g1 blue
  "#C792EA", // g2 violet
  "#F2B43A", // g3 amber
  "#FF8FA3", // g4 pink
  "#67E8F9", // g5 cyan
];

const PALETTE = {
  bg: "#08090C",
  surface: "#10131A",
  line: "#1E2430",
  lineStrong: "#2A323F",
  text: "#E8ECF2",
  muted: "#8A93A3",
  faint: "#5A6373",
  primary: "#5EE6C4",
};

// ----- deterministic PRNG -------------------------------------------------
// FNV-1a 32-bit hash of a string seed → mulberry32 PRNG. Stable across reloads.
function fnv1a(str) {
  let h = 0x811c9dc5;
  for (let i = 0; i < str.length; i += 1) {
    h ^= str.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}

function mulberry32(seed) {
  let a = seed >>> 0;
  return function next() {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function languageLabel(value) {
  if (value === "cpp") return "C++";
  if (value === "csharp") return "C#";
  if (value === "objc") return "Objective-C";
  if (!value) return "unknown";
  return String(value).replace(/\b\w/g, (c) => c.toUpperCase());
}

const fmt = new Intl.NumberFormat("en-US");

// Hubs called out in the right-rail focus chips (verbatim from the brief).
const TOP_HUBS = [
  "String",
  "get",
  "run",
  "Parse",
  "Close",
  "Context",
  "run_smoke",
  "contains",
];

// ----- layout simulation --------------------------------------------------
// Bounded O(n^2) force-directed layout: repulsion + spring on edges + gentle
// centering. Capped iterations, fully deterministic from the seeded PRNG.
function computeLayout(nodes, edges) {
  const n = nodes.length;
  if (n === 0) return [];

  const rand = mulberry32(fnv1a("atlas-benchmark-instrument-v1"));

  // Domain is roughly a unit-ish square centred on origin; the renderer maps
  // this to screen via a fit transform, so absolute scale here is arbitrary.
  const SPREAD = 600;
  const pos = nodes.map((node) => {
    // Seed each node from its stable id so positions never depend on array order.
    const r = mulberry32(fnv1a(`node-${node.id}`));
    const angle = r() * Math.PI * 2;
    const radius = Math.sqrt(rand()) * SPREAD;
    return {
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius,
      vx: 0,
      vy: 0,
    };
  });

  const degMax = nodes.reduce((m, node) => Math.max(m, node.deg || 0), 1);

  // Build adjacency for spring forces (dedupe self loops / out-of-range).
  const springs = [];
  for (const e of edges) {
    if (e.s === e.t) continue;
    if (e.s < 0 || e.s >= n || e.t < 0 || e.t >= n) continue;
    springs.push([e.s, e.t]);
  }

  const ITERS = 320;
  const REPULSION = 5200;
  const SPRING_LEN = 56;
  const SPRING_K = 0.012;
  const CENTER_K = 0.0016;
  const DAMPING = 0.86;
  const MAX_DISP = 90;

  for (let iter = 0; iter < ITERS; iter += 1) {
    // Cooling factor so the system settles instead of oscillating.
    const cool = 1 - iter / ITERS;

    // Repulsion: every pair pushes apart (n=280 → ~39k pairs/iter, fine).
    for (let i = 0; i < n; i += 1) {
      const pi = pos[i];
      for (let j = i + 1; j < n; j += 1) {
        const pj = pos[j];
        let dx = pi.x - pj.x;
        let dy = pi.y - pj.y;
        let d2 = dx * dx + dy * dy;
        if (d2 < 0.01) {
          // Coincident: nudge deterministically by index so it stays stable.
          dx = (i - j) * 0.01 + 0.001;
          dy = (i + j) * 0.01 + 0.001;
          d2 = dx * dx + dy * dy;
        }
        const inv = 1 / d2;
        const force = REPULSION * inv;
        const dist = Math.sqrt(d2);
        const fx = (dx / dist) * force;
        const fy = (dy / dist) * force;
        pi.vx += fx;
        pi.vy += fy;
        pj.vx -= fx;
        pj.vy -= fy;
      }
    }

    // Springs: edges pull connected nodes toward a rest length.
    for (let s = 0; s < springs.length; s += 1) {
      const a = pos[springs[s][0]];
      const b = pos[springs[s][1]];
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 0.01;
      const force = (dist - SPRING_LEN) * SPRING_K;
      const fx = (dx / dist) * force;
      const fy = (dy / dist) * force;
      a.vx += fx;
      a.vy += fy;
      b.vx -= fx;
      b.vy -= fy;
    }

    // Centering: pull everything gently toward origin so the layout stays framed.
    for (let i = 0; i < n; i += 1) {
      const p = pos[i];
      p.vx -= p.x * CENTER_K;
      p.vy -= p.y * CENTER_K;

      // Integrate with damping + cooling, clamped to avoid explosions.
      p.vx *= DAMPING;
      p.vy *= DAMPING;
      let dispx = p.vx * cool;
      let dispy = p.vy * cool;
      const disp = Math.sqrt(dispx * dispx + dispy * dispy);
      if (disp > MAX_DISP) {
        dispx = (dispx / disp) * MAX_DISP;
        dispy = (dispy / disp) * MAX_DISP;
      }
      p.x += dispx;
      p.y += dispy;
    }
  }

  return pos.map((p, i) => ({
    x: p.x,
    y: p.y,
    deg: nodes[i].deg || 0,
    degNorm: (nodes[i].deg || 0) / degMax,
  }));
}

// ----- component ----------------------------------------------------------
export default function GraphExplorer({ className = "" }) {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);
  const [hoverId, setHoverId] = useState(null);
  const [focusName, setFocusName] = useState(null);

  const canvasRef = useRef(null);
  const wrapRef = useRef(null);
  const rafRef = useRef(0);

  // Mutable view + interaction state kept in a ref so the render loop reads it
  // without re-subscribing effects on every pointer move.
  const viewRef = useRef({
    scale: 1,
    tx: 0,
    ty: 0,
    fitted: false,
  });
  const stateRef = useRef({
    nodes: [],
    edges: [],
    layout: [],
    adjacency: new Map(),
    hoverId: null,
    focusId: null,
    dragId: null,
    dragMoved: false,
    panning: false,
    lastX: 0,
    lastY: 0,
    pointerX: 0,
    pointerY: 0,
    dpr: 1,
    width: 0,
    height: 0,
    settleStart: 0,
    visible: false,
    needsPaint: true,
    reducedMotion: false,
  });

  // ---- fetch graph data --------------------------------------------------
  useEffect(() => {
    let alive = true;
    fetch("data/graph.json")
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((json) => {
        if (!alive) return;
        if (!json || !Array.isArray(json.nodes) || !Array.isArray(json.edges)) {
          throw new Error("malformed graph payload");
        }
        setData(json);
      })
      .catch((err) => {
        if (alive) setError(err);
      });
    return () => {
      alive = false;
    };
  }, []);

  // Derived layout + adjacency, computed once per dataset.
  const prepared = useMemo(() => {
    if (!data) return null;
    const nodes = data.nodes;
    const edges = data.edges;
    const layout = computeLayout(nodes, edges);

    const adjacency = new Map();
    for (let i = 0; i < nodes.length; i += 1) adjacency.set(i, new Set());
    for (const e of edges) {
      if (e.s === e.t) continue;
      if (e.s < 0 || e.s >= nodes.length || e.t < 0 || e.t >= nodes.length) continue;
      adjacency.get(e.s).add(e.t);
      adjacency.get(e.t).add(e.s);
    }

    // id → index map (ids happen to be 0..n-1 but never assume it).
    const idToIndex = new Map();
    nodes.forEach((node, i) => idToIndex.set(node.id, i));

    return { nodes, edges, layout, adjacency, idToIndex };
  }, [data]);

  // Community counts for the legend, derived from real data.
  const communityCounts = useMemo(() => {
    if (!data) return [];
    const counts = {};
    for (const node of data.nodes) counts[node.c] = (counts[node.c] || 0) + 1;
    return Object.keys(counts)
      .map((k) => ({ c: Number(k), count: counts[k] }))
      .sort((a, b) => a.c - b.c);
  }, [data]);

  const caption = useMemo(() => {
    if (!data || !data.meta) return "";
    const m = data.meta;
    const shownNodes = fmt.format(m.shown_nodes ?? data.nodes.length);
    const total = fmt.format(m.nodes_total ?? data.nodes.length);
    const shownEdges = fmt.format(m.shown_edges ?? data.edges.length);
    const communities = m.communities ?? communityCounts.length;
    return `${shownNodes} of ${total} symbols · ${shownEdges} edges · ${communities} communities · ${m.source || "atlas export --all"}`;
  }, [data, communityCounts]);

  const ariaSummary = useMemo(() => {
    if (error) {
      return "Code graph could not be loaded. The interactive visualization is unavailable.";
    }
    if (!data || !data.meta) return "Loading the Atlas code graph.";
    const m = data.meta;
    const langs = Array.from(new Set(data.nodes.map((n) => n.lang))).map(languageLabel);
    const topHubs = [...data.nodes]
      .sort((a, b) => (b.deg || 0) - (a.deg || 0))
      .slice(0, 6)
      .map((n) => `${n.name} (degree ${n.deg})`);
    return [
      `Interactive force-directed code graph from ${m.repo || "the Atlas repository"}.`,
      `Showing ${fmt.format(m.shown_nodes ?? data.nodes.length)} of ${fmt.format(m.nodes_total ?? data.nodes.length)} symbols`,
      `and ${fmt.format(m.shown_edges ?? data.edges.length)} of ${fmt.format(m.edges_total ?? data.edges.length)} call edges,`,
      `grouped into ${m.communities ?? communityCounts.length} communities.`,
      `Languages present: ${langs.join(", ")}.`,
      `Highest-degree symbols (hubs): ${topHubs.join("; ")}.`,
      `Source command: ${m.source || "atlas export --all"}.`,
    ].join(" ");
  }, [data, error, communityCounts]);

  // ---- canvas render loop ------------------------------------------------
  useEffect(() => {
    if (!prepared) return undefined;
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return undefined;

    const st = stateRef.current;
    st.nodes = prepared.nodes;
    st.edges = prepared.edges;
    st.layout = prepared.layout.map((p) => ({ ...p })); // own mutable copy for drag
    st.adjacency = prepared.adjacency;
    st.idToIndex = prepared.idToIndex;
    st.reducedMotion =
      typeof window !== "undefined" &&
      window.matchMedia &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    st.settleStart = 0;
    st.needsPaint = true;

    const ctx = canvas.getContext("2d");

    // ---- fit transform: frame all node positions in the viewport --------
    function fitView() {
      const layout = st.layout;
      if (!layout.length || !st.width || !st.height) return;
      let minX = Infinity;
      let minY = Infinity;
      let maxX = -Infinity;
      let maxY = -Infinity;
      for (const p of layout) {
        if (p.x < minX) minX = p.x;
        if (p.y < minY) minY = p.y;
        if (p.x > maxX) maxX = p.x;
        if (p.y > maxY) maxY = p.y;
      }
      const pad = 48;
      const w = Math.max(maxX - minX, 1);
      const h = Math.max(maxY - minY, 1);
      const scale = Math.min(
        (st.width - pad * 2) / w,
        (st.height - pad * 2) / h
      );
      const view = viewRef.current;
      view.scale = scale;
      view.tx = st.width / 2 - ((minX + maxX) / 2) * scale;
      view.ty = st.height / 2 - ((minY + maxY) / 2) * scale;
      view.fitted = true;
      st.needsPaint = true;
    }

    // ---- coordinate helpers --------------------------------------------
    function worldToScreen(x, y) {
      const v = viewRef.current;
      return { sx: x * v.scale + v.tx, sy: y * v.scale + v.ty };
    }
    function screenToWorld(sx, sy) {
      const v = viewRef.current;
      return { x: (sx - v.tx) / v.scale, y: (sy - v.ty) / v.scale };
    }

    function radiusFor(p) {
      // 3.2..11px by degree, scaled by current zoom but bounded so hubs read.
      const base = 3.2 + p.degNorm * 7.8;
      return base;
    }

    function nodeAt(sx, sy) {
      // Hit-test in reverse paint order; generous hit slop for small nodes.
      const layout = st.layout;
      for (let i = layout.length - 1; i >= 0; i -= 1) {
        const p = layout[i];
        const scr = worldToScreen(p.x, p.y);
        const r = Math.max(radiusFor(p) * viewRef.current.scale, 4) + 4;
        const dx = sx - scr.sx;
        const dy = sy - scr.sy;
        if (dx * dx + dy * dy <= r * r) return i;
      }
      return null;
    }

    // ---- paint ----------------------------------------------------------
    function paint() {
      const v = viewRef.current;
      const layout = st.layout;
      const nodes = st.nodes;
      const W = st.width;
      const H = st.height;
      ctx.save();
      ctx.scale(st.dpr, st.dpr);
      ctx.clearRect(0, 0, W, H);

      // background + soft primary radial glow at ~6%
      ctx.fillStyle = PALETTE.bg;
      ctx.fillRect(0, 0, W, H);
      const glow = ctx.createRadialGradient(
        W * 0.5,
        H * 0.42,
        0,
        W * 0.5,
        H * 0.42,
        Math.max(W, H) * 0.6
      );
      glow.addColorStop(0, "rgba(94,230,196,0.06)");
      glow.addColorStop(1, "rgba(94,230,196,0)");
      ctx.fillStyle = glow;
      ctx.fillRect(0, 0, W, H);

      const hoverIdx = st.hoverId;
      const focusIdx = st.focusId;
      const activeIdx = hoverIdx != null ? hoverIdx : focusIdx;
      const neighbors = activeIdx != null ? st.adjacency.get(activeIdx) : null;

      // ---- edges --------------------------------------------------------
      ctx.lineWidth = 1;
      for (const e of st.edges) {
        const a = layout[e.s];
        const b = layout[e.t];
        if (!a || !b) continue;
        const incident =
          activeIdx != null && (e.s === activeIdx || e.t === activeIdx);
        if (activeIdx != null && !incident) {
          ctx.strokeStyle = "rgba(120,130,150,0.04)";
        } else if (incident) {
          ctx.strokeStyle = "rgba(94,230,196,0.45)";
        } else {
          ctx.strokeStyle = "rgba(120,130,150,0.10)";
        }
        const sa = worldToScreen(a.x, a.y);
        const sb = worldToScreen(b.x, b.y);
        ctx.beginPath();
        ctx.moveTo(sa.sx, sa.sy);
        ctx.lineTo(sb.sx, sb.sy);
        ctx.stroke();
      }

      // ---- nodes --------------------------------------------------------
      for (let i = 0; i < layout.length; i += 1) {
        const p = layout[i];
        const node = nodes[i];
        const scr = worldToScreen(p.x, p.y);
        const r = Math.max(radiusFor(p) * v.scale, 1.4);
        const color = COMMUNITY_COLORS[node.c % COMMUNITY_COLORS.length];

        let alpha = 1;
        if (activeIdx != null) {
          const isActive = i === activeIdx;
          const isNeighbor = neighbors && neighbors.has(i);
          alpha = isActive ? 1 : isNeighbor ? 0.95 : 0.12;
        }

        ctx.globalAlpha = alpha;
        ctx.beginPath();
        ctx.arc(scr.sx, scr.sy, r, 0, Math.PI * 2);
        ctx.fillStyle = color;
        ctx.fill();

        // thin ring on the active node + neighbors for legibility
        if (activeIdx != null && (i === activeIdx || (neighbors && neighbors.has(i)))) {
          ctx.globalAlpha = 1;
          ctx.lineWidth = i === activeIdx ? 2 : 1;
          ctx.strokeStyle = i === activeIdx ? PALETTE.text : "rgba(232,236,242,0.5)";
          ctx.stroke();
        }
        ctx.globalAlpha = 1;
      }

      // ---- hub labels (only the largest, to avoid clutter) --------------
      ctx.globalAlpha = 1;
      ctx.font = "600 11px ui-monospace, 'SF Mono', Menlo, monospace";
      ctx.textBaseline = "middle";
      for (let i = 0; i < layout.length; i += 1) {
        const p = layout[i];
        if (p.degNorm < 0.55 && i !== activeIdx) continue;
        const node = nodes[i];
        const scr = worldToScreen(p.x, p.y);
        const r = Math.max(radiusFor(p) * v.scale, 1.4);
        let labelAlpha = 0.85;
        if (activeIdx != null) {
          const visible = i === activeIdx || (neighbors && neighbors.has(i));
          labelAlpha = visible ? 1 : 0.0;
        }
        if (labelAlpha <= 0) continue;
        ctx.globalAlpha = labelAlpha;
        ctx.fillStyle = PALETTE.text;
        ctx.fillText(node.name, scr.sx + r + 4, scr.sy);
      }
      ctx.globalAlpha = 1;
      ctx.restore();
    }

    // ---- animation control ---------------------------------------------
    // Layout is precomputed (settled). We only need to repaint when something
    // changes (hover/drag/pan/zoom/resize) or for a brief intro fade-in.
    function frame() {
      if (st.needsPaint || st.dragId != null || st.panning) {
        paint();
        st.needsPaint = false;
      }
      rafRef.current = requestAnimationFrame(frame);
    }

    // ---- sizing ---------------------------------------------------------
    function resize() {
      const rect = wrap.getBoundingClientRect();
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      st.dpr = dpr;
      st.width = Math.max(rect.width, 1);
      st.height = Math.max(rect.height, 1);
      canvas.width = Math.round(st.width * dpr);
      canvas.height = Math.round(st.height * dpr);
      canvas.style.width = `${st.width}px`;
      canvas.style.height = `${st.height}px`;
      if (!viewRef.current.fitted) fitView();
      st.needsPaint = true;
    }

    const ro =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(() => resize())
        : null;
    if (ro) ro.observe(wrap);
    else window.addEventListener("resize", resize);
    resize();

    // ---- pointer interactions ------------------------------------------
    function localPoint(evt) {
      const rect = canvas.getBoundingClientRect();
      return { sx: evt.clientX - rect.left, sy: evt.clientY - rect.top };
    }

    function onPointerDown(evt) {
      canvas.setPointerCapture?.(evt.pointerId);
      const { sx, sy } = localPoint(evt);
      st.lastX = sx;
      st.lastY = sy;
      const idx = nodeAt(sx, sy);
      if (idx != null) {
        st.dragId = idx;
        st.dragMoved = false;
      } else {
        st.panning = true;
      }
    }

    function onPointerMove(evt) {
      const { sx, sy } = localPoint(evt);
      st.pointerX = sx;
      st.pointerY = sy;

      if (st.dragId != null) {
        const world = screenToWorld(sx, sy);
        const p = st.layout[st.dragId];
        p.x = world.x;
        p.y = world.y;
        st.dragMoved = true;
        st.needsPaint = true;
        return;
      }
      if (st.panning) {
        const v = viewRef.current;
        v.tx += sx - st.lastX;
        v.ty += sy - st.lastY;
        st.lastX = sx;
        st.lastY = sy;
        st.needsPaint = true;
        return;
      }
      // hover hit-test
      const idx = nodeAt(sx, sy);
      if (idx !== st.hoverId) {
        st.hoverId = idx;
        st.needsPaint = true;
        setHoverId(idx);
      }
    }

    function onPointerUp(evt) {
      canvas.releasePointerCapture?.(evt.pointerId);
      st.dragId = null;
      st.panning = false;
    }

    function onPointerLeave() {
      if (st.hoverId != null) {
        st.hoverId = null;
        st.needsPaint = true;
        setHoverId(null);
      }
    }

    function onWheel(evt) {
      evt.preventDefault();
      const { sx, sy } = localPoint(evt);
      const v = viewRef.current;
      const factor = Math.exp(-evt.deltaY * 0.0015);
      const newScale = Math.min(Math.max(v.scale * factor, 0.15), 6);
      // zoom toward the cursor
      const wx = (sx - v.tx) / v.scale;
      const wy = (sy - v.ty) / v.scale;
      v.scale = newScale;
      v.tx = sx - wx * v.scale;
      v.ty = sy - wy * v.scale;
      st.needsPaint = true;
    }

    canvas.addEventListener("pointerdown", onPointerDown);
    canvas.addEventListener("pointermove", onPointerMove);
    canvas.addEventListener("pointerup", onPointerUp);
    canvas.addEventListener("pointerleave", onPointerLeave);
    canvas.addEventListener("wheel", onWheel, { passive: false });

    rafRef.current = requestAnimationFrame(frame);

    // expose control hooks for the toolbar buttons
    canvas._atlasControls = {
      zoom(factor) {
        const v = viewRef.current;
        const cx = st.width / 2;
        const cy = st.height / 2;
        const wx = (cx - v.tx) / v.scale;
        const wy = (cy - v.ty) / v.scale;
        v.scale = Math.min(Math.max(v.scale * factor, 0.15), 6);
        v.tx = cx - wx * v.scale;
        v.ty = cy - wy * v.scale;
        st.needsPaint = true;
      },
      fit() {
        viewRef.current.fitted = false;
        fitView();
      },
    };

    return () => {
      cancelAnimationFrame(rafRef.current);
      if (ro) ro.disconnect();
      else window.removeEventListener("resize", resize);
      canvas.removeEventListener("pointerdown", onPointerDown);
      canvas.removeEventListener("pointermove", onPointerMove);
      canvas.removeEventListener("pointerup", onPointerUp);
      canvas.removeEventListener("pointerleave", onPointerLeave);
      canvas.removeEventListener("wheel", onWheel);
      delete canvas._atlasControls;
    };
  }, [prepared]);

  // Reflect external focus chip selection into the render state.
  useEffect(() => {
    const st = stateRef.current;
    if (!prepared) return;
    if (focusName == null) {
      st.focusId = null;
      st.needsPaint = true;
      return;
    }
    const idx = prepared.nodes.findIndex((n) => n.name === focusName);
    st.focusId = idx >= 0 ? idx : null;
    // recenter on the focused node
    if (idx >= 0) {
      const p = st.layout[idx];
      const v = viewRef.current;
      if (p) {
        v.tx = st.width / 2 - p.x * v.scale;
        v.ty = st.height / 2 - p.y * v.scale;
      }
    }
    st.needsPaint = true;
  }, [focusName, prepared]);

  // ---- hovered node detail for the tooltip -------------------------------
  const hoverNode =
    hoverId != null && prepared ? prepared.nodes[hoverId] : null;
  const tooltipPos = stateRef.current;

  // ---- fallback panel (fetch failed) -------------------------------------
  if (error) {
    return (
      <div
        className={className}
        data-testid="graph-canvas"
        role="img"
        aria-label={ariaSummary}
        style={{
          minHeight: 280,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          textAlign: "center",
          padding: 24,
          background: PALETTE.surface,
          border: `1px solid ${PALETTE.line}`,
          borderRadius: 12,
          color: PALETTE.muted,
        }}
      >
        <div style={{ maxWidth: 420 }}>
          <div
            style={{
              fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace",
              fontSize: 11,
              letterSpacing: "0.16em",
              textTransform: "uppercase",
              color: PALETTE.faint,
              marginBottom: 10,
            }}
          >
            graph unavailable
          </div>
          <p style={{ fontSize: 14, lineHeight: 1.6, color: PALETTE.muted, margin: 0 }}>
            The interactive code graph could not be loaded. Atlas builds a
            deterministic symbol/call graph; this exhibit renders it from
            <code style={{ fontFamily: "ui-monospace, monospace", color: PALETTE.text }}>
              {" "}data/graph.json
            </code>
            , which did not load in this environment.
          </p>
        </div>
      </div>
    );
  }

  // ---- main render -------------------------------------------------------
  return (
    <div
      className={className}
      data-testid="graph-canvas"
      style={{
        position: "relative",
        display: "flex",
        flexDirection: "row",
        flexWrap: "wrap",
        gap: 16,
        width: "100%",
      }}
    >
      {/* Accessible text summary, visually hidden but exposed to AT. */}
      <p
        style={{
          position: "absolute",
          width: 1,
          height: 1,
          padding: 0,
          margin: -1,
          overflow: "hidden",
          clip: "rect(0 0 0 0)",
          whiteSpace: "nowrap",
          border: 0,
        }}
      >
        {ariaSummary}
      </p>

      {/* Canvas stage */}
      <div
        ref={wrapRef}
        role="img"
        aria-label={ariaSummary}
        style={{
          position: "relative",
          flex: "1 1 420px",
          minWidth: 280,
          minHeight: 360,
          height: "100%",
          background: PALETTE.bg,
          border: `1px solid ${PALETTE.line}`,
          borderRadius: 12,
          overflow: "hidden",
        }}
      >
        <canvas
          ref={canvasRef}
          style={{
            display: "block",
            position: "absolute",
            inset: 0,
            width: "100%",
            height: "100%",
            cursor: hoverNode ? "pointer" : "grab",
            touchAction: "none",
          }}
        />

        {/* Tooltip */}
        {hoverNode && (
          <div
            style={{
              position: "absolute",
              left: Math.min(tooltipPos.pointerX + 14, (tooltipPos.width || 0) - 240),
              top: Math.min(tooltipPos.pointerY + 14, (tooltipPos.height || 0) - 110),
              pointerEvents: "none",
              maxWidth: 240,
              background: "rgba(16,19,26,0.96)",
              border: `1px solid ${PALETTE.lineStrong}`,
              borderRadius: 8,
              padding: "8px 10px",
              fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace",
              fontSize: 12,
              lineHeight: 1.45,
              color: PALETTE.text,
              boxShadow: "0 8px 24px rgba(0,0,0,0.4)",
              zIndex: 5,
            }}
          >
            <div style={{ fontWeight: 600, color: PALETTE.primary, wordBreak: "break-all" }}>
              {hoverNode.name}
            </div>
            <div style={{ color: PALETTE.muted, marginTop: 2 }}>
              {hoverNode.kind} · {languageLabel(hoverNode.lang)} · deg {hoverNode.deg}
            </div>
            <div style={{ color: PALETTE.faint, marginTop: 2, wordBreak: "break-all" }}>
              {hoverNode.path}
            </div>
          </div>
        )}

        {/* Zoom / reset / fit controls (bottom-left) */}
        <div
          style={{
            position: "absolute",
            left: 12,
            bottom: 12,
            display: "flex",
            gap: 6,
            zIndex: 4,
          }}
        >
          {[
            { label: "−", title: "Zoom out", fn: () => canvasRef.current?._atlasControls?.zoom(0.8) },
            { label: "+", title: "Zoom in", fn: () => canvasRef.current?._atlasControls?.zoom(1.25) },
            { label: "Fit", title: "Reset & fit", fn: () => canvasRef.current?._atlasControls?.fit() },
          ].map((b) => (
            <button
              key={b.title}
              type="button"
              title={b.title}
              aria-label={b.title}
              onClick={b.fn}
              style={{
                minWidth: 30,
                height: 30,
                padding: "0 8px",
                background: PALETTE.surface,
                border: `1px solid ${PALETTE.line}`,
                borderRadius: 6,
                color: PALETTE.text,
                fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace",
                fontSize: 13,
                cursor: "pointer",
                lineHeight: 1,
              }}
            >
              {b.label}
            </button>
          ))}
        </div>

        {/* Caption */}
        {caption && (
          <div
            style={{
              position: "absolute",
              right: 12,
              bottom: 12,
              maxWidth: "70%",
              textAlign: "right",
              fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace",
              fontSize: 11,
              letterSpacing: "0.01em",
              color: PALETTE.faint,
              zIndex: 4,
              pointerEvents: "none",
            }}
          >
            {caption}
          </div>
        )}
      </div>

      {/* Right-rail legend + hubs */}
      <div
        data-testid="graph-legend"
        style={{
          flex: "0 0 200px",
          minWidth: 180,
          display: "flex",
          flexDirection: "column",
          gap: 18,
          fontFamily:
            "Inter, ui-sans-serif, system-ui, -apple-system, sans-serif",
        }}
      >
        <div>
          <div
            style={{
              fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace",
              fontSize: 11,
              letterSpacing: "0.16em",
              textTransform: "uppercase",
              color: PALETTE.faint,
              marginBottom: 10,
            }}
          >
            Communities
          </div>
          <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 7 }}>
            {communityCounts.map((entry) => (
              <li
                key={entry.c}
                style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 12.5, color: PALETTE.text }}
              >
                <span
                  style={{
                    width: 10,
                    height: 10,
                    borderRadius: "50%",
                    background: COMMUNITY_COLORS[entry.c % COMMUNITY_COLORS.length],
                    flex: "0 0 auto",
                  }}
                />
                <span style={{ color: PALETTE.muted }}>c{entry.c}</span>
                <span
                  style={{
                    marginLeft: "auto",
                    fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace",
                    fontVariantNumeric: "tabular-nums",
                    color: PALETTE.text,
                  }}
                >
                  {fmt.format(entry.count)}
                </span>
              </li>
            ))}
          </ul>
        </div>

        <div>
          <div
            style={{
              fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace",
              fontSize: 11,
              letterSpacing: "0.16em",
              textTransform: "uppercase",
              color: PALETTE.faint,
              marginBottom: 10,
            }}
          >
            Top hubs
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
            {TOP_HUBS.filter((name) =>
              prepared ? prepared.nodes.some((n) => n.name === name) : true
            ).map((name) => {
              const active = focusName === name;
              return (
                <button
                  key={name}
                  type="button"
                  onClick={() => setFocusName(active ? null : name)}
                  aria-pressed={active}
                  style={{
                    fontFamily: "ui-monospace, 'SF Mono', Menlo, monospace",
                    fontSize: 12,
                    padding: "3px 8px",
                    borderRadius: 6,
                    cursor: "pointer",
                    border: `1px solid ${active ? PALETTE.primary : PALETTE.line}`,
                    background: active ? "rgba(94,230,196,0.12)" : PALETTE.surface,
                    color: active ? PALETTE.primary : PALETTE.muted,
                  }}
                >
                  {name}
                </button>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}
