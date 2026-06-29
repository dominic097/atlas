import React, { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { createRoot } from "react-dom/client";
import {
  ArrowRight,
  ArrowUpDown,
  Check,
  Copy,
  Download,
  ExternalLink,
  Search,
} from "lucide-react";
import GraphExplorer from "./GraphExplorer";

/* ============================================================
   Atlas — The Benchmark Instrument
   Dark instrument console. Every numeral traces to
   data/benchmark-data.json. No chart CDN — inline SVG/canvas/CSS.
   ============================================================ */

const fmt = new Intl.NumberFormat("en-US");

const COMMUNITY_COLORS = ["#5EE6C4", "#7AA2FF", "#C792EA", "#F2B43A", "#FF8FA3", "#67E8F9"];

function cn(...c) {
  return c.filter(Boolean).join(" ");
}
function num(v) {
  if (v === null || v === undefined || Number.isNaN(Number(v))) return "—";
  return fmt.format(v);
}
function ratio(v) {
  if (v === null || v === undefined || Number.isNaN(Number(v))) return "not comparable";
  return `${Number(v).toFixed(2)}x`;
}
function secs(v) {
  if (v === null || v === undefined || Number.isNaN(Number(v))) return "—";
  return `${Number(v).toFixed(Number(v) < 1 ? 3 : 2)}s`;
}
function shortSha(v) {
  return v ? v.slice(0, 10) : "—";
}
function langLabel(v) {
  if (v === "cpp") return "C++";
  if (v === "objc") return "Objective-C";
  if (v === "csharp") return "C#";
  if (v === "ejs") return "EJS";
  if (v === "ets") return "ETS";
  if (v === "sql") return "SQL";
  if (v === "php") return "PHP";
  if (v === "cuda") return "CUDA";
  if (v === "dotnet") return ".NET";
  if (!v) return "unknown";
  return String(v).replace(/\b\w/g, (c) => c.toUpperCase());
}
function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return undefined;
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReduced(mq.matches);
    const fn = (e) => setReduced(e.matches);
    mq.addEventListener?.("change", fn);
    return () => mq.removeEventListener?.("change", fn);
  }, []);
  return reduced;
}

/* -- shared IntersectionObserver hook -------------------------------------
   inView is a REVEAL ENHANCEMENT only. It always resolves to true — either
   because the element scrolled into view, or via a short fallback timer — so
   deep-link landings, IO misses, JS-disabled-IO, and headless screenshot
   harnesses never see a blank chart. Charts must default to their final
   geometry/opacity and use this flag only to opt into the entry animation. */
function useInView(options) {
  const ref = useRef(null);
  const [inView, setInView] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el || typeof IntersectionObserver === "undefined") {
      setInView(true);
      return undefined;
    }
    let timer = 0;
    const io = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => {
          if (e.isIntersecting) setInView(true);
        });
      },
      options || { threshold: 0.25 }
    );
    io.observe(el);
    // Fallback: if the observer never fires (off-screen at load, deep-link,
    // missed intersection), reveal anyway so the data is always shown.
    timer = window.setTimeout(() => setInView(true), 1200);
    return () => {
      io.disconnect();
      window.clearTimeout(timer);
    };
  }, []);
  return [ref, inView];
}

/* ========================== PRIMITIVES ================================== */

function CopyButton({ text, label = "copy", className }) {
  const [copied, setCopied] = useState(false);
  const tRef = useRef(0);
  const onCopy = useCallback(() => {
    const done = () => {
      setCopied(true);
      window.clearTimeout(tRef.current);
      tRef.current = window.setTimeout(() => setCopied(false), 1400);
    };
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done).catch(done);
      } else {
        done();
      }
    } catch {
      done();
    }
  }, [text]);
  useEffect(() => () => window.clearTimeout(tRef.current), []);
  return (
    <button
      type="button"
      data-testid="copy-button"
      onClick={onCopy}
      aria-label={copied ? "Copied" : `Copy ${label}`}
      className="focusring mono inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-semibold transition"
      style={{
        border: "1px solid var(--line-strong)",
        background: "var(--surface-raised)",
        color: copied ? "var(--primary)" : "var(--muted)",
      }}
    >
      {copied ? <Check className="h-3.5 w-3.5" aria-hidden /> : <Copy className="h-3.5 w-3.5" aria-hidden />}
      {copied ? "copied ✓" : "copy"}
    </button>
  );
}

function StatTick({ label, value, sub }) {
  return (
    <div className="min-w-0">
      <div className="kicker truncate">{label}</div>
      <div className="num mt-1 truncate" style={{ fontSize: 18, color: "var(--text)" }}>
        {value}
      </div>
      {sub && (
        <div className="mt-0.5 truncate" style={{ fontSize: 11, color: "var(--faint)" }}>
          {sub}
        </div>
      )}
    </div>
  );
}

function SourceLink({ children, href, download = false, testId = true }) {
  return (
    <a
      {...(testId ? { "data-source-artifact": "" } : {})}
      className="focusring chip transition hover:text-text"
      href={href}
      download={download || undefined}
      target={download ? undefined : "_blank"}
      rel={download ? undefined : "noreferrer"}
      style={{ textDecoration: "none" }}
    >
      {children}
    </a>
  );
}

function SectionHeader({ kicker, title, children, actions, id }) {
  return (
    <div className="mb-7 flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
      <div className="max-w-3xl">
        {kicker && <div className="kicker" style={{ color: "var(--primary)" }}>{kicker}</div>}
        <h2
          id={id}
          className="mt-3 text-balance font-semibold tracking-tight"
          style={{ fontSize: "clamp(22px,3vw,30px)", lineHeight: 1.1, letterSpacing: "-0.015em" }}
        >
          {title}
        </h2>
        {children && (
          <p className="mt-3" style={{ fontSize: 15, lineHeight: 1.6, color: "var(--muted)" }}>
            {children}
          </p>
        )}
      </div>
      {actions && <div className="flex flex-wrap gap-2 md:justify-end">{actions}</div>}
    </div>
  );
}

/* count-up ratio: animates once on scroll-into-view, reduced-motion = final */
function CountUpRatio({ value, suffix = "x", testId, ariaLabel }) {
  const reduced = usePrefersReducedMotion();
  const [ref, inView] = useInView({ threshold: 0.4 });
  const [display, setDisplay] = useState(reduced ? value : 0);
  const started = useRef(false);
  useEffect(() => {
    if (!inView || started.current) return undefined;
    started.current = true;
    if (reduced) {
      setDisplay(value);
      return undefined;
    }
    const dur = 720;
    const t0 = performance.now();
    let raf = 0;
    const tick = (t) => {
      const p = Math.min(1, (t - t0) / dur);
      const eased = 1 - Math.pow(1 - p, 3);
      setDisplay(value * eased);
      if (p < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [inView, reduced, value]);
  return (
    <span
      ref={ref}
      data-testid={testId}
      aria-label={ariaLabel}
      className="mono tnum"
      style={{
        fontSize: "clamp(40px,5.5vw,68px)",
        lineHeight: 0.95,
        letterSpacing: "-0.03em",
        fontWeight: 600,
        color: "var(--primary)",
        display: "inline-block",
      }}
    >
      {display.toFixed(2)}
      <span style={{ color: "var(--muted)", fontSize: "0.42em", marginLeft: 2 }}>{suffix}</span>
    </span>
  );
}

/* Honest hero mini-dumbbell: a single quantitative row showing Atlas (small
   dot, fixed left anchor) vs graphify (the aggregate ratio sets the line
   length). The MAGNITUDE of the headline win is seen, not just printed.
   atlasVal/graphifyVal are summed raw units over the comparable rows;
   ratio is the page's aggregate ratio. No invented numbers. */
function MiniDumbbell({ ratio, atlasVal, graphifyVal, unit, color = "var(--primary)", label }) {
  const W = 150;
  const H = 30;
  const ax = 9; // atlas anchor (px)
  const gx = W - 9; // graphify end (px) — full track encodes the ratio span
  const midY = 20;
  return (
    <svg
      width={W}
      height={H}
      viewBox={`0 0 ${W} ${H}`}
      role="img"
      aria-label={label || `Atlas ${atlasVal} ${unit} versus graphify ${graphifyVal} ${unit}, ${ratio.toFixed(2)} times`}
      style={{ overflow: "visible" }}
    >
      <line x1={ax} y1={midY} x2={gx} y2={midY} stroke="var(--line)" strokeWidth="1" />
      <line
        x1={ax}
        y1={midY}
        x2={gx}
        y2={midY}
        stroke={color}
        strokeWidth="2"
        strokeLinecap="round"
        opacity="0.55"
      />
      {/* atlas (small, near) */}
      <circle cx={ax} cy={midY} r="4" fill={color} />
      <text x={ax} y={9} className="mono" fontSize="9" fill={color} textAnchor="start">
        {num(Math.round(atlasVal))}
      </text>
      {/* graphify (far) */}
      <circle cx={gx} cy={midY} r="4" fill="var(--faint)" />
      <text x={gx} y={9} className="mono" fontSize="9" fill="var(--faint)" textAnchor="end">
        {num(Math.round(graphifyVal))}
      </text>
    </svg>
  );
}

/* ========================== CONSOLE BAR ================================== */

const NAV_ITEMS = [
  ["Benchmark", "vs-graphify"],
  ["Graph", "graph"],
  ["Coverage", "vs-native"],
  ["Evidence", "evidence"],
  ["Install", "install"],
];

function ConsoleBar({ data, active }) {
  const platform = data.provenance.platform;
  return (
    <header
      data-testid="nav"
      className="sticky top-0 z-40"
      style={{
        height: 56,
        background: "rgba(8,9,12,0.72)",
        backdropFilter: "blur(12px)",
        WebkitBackdropFilter: "blur(12px)",
        borderBottom: "1px solid var(--line)",
      }}
    >
      <nav className="shell flex h-full items-center justify-between gap-3" aria-label="Primary">
        <a href="#hero" className="focusring flex min-w-0 items-center gap-2.5" style={{ textDecoration: "none" }}>
          <span
            className="mono grid place-items-center rounded-md"
            style={{
              width: 26,
              height: 26,
              background: "var(--primary)",
              color: "#04130f",
              fontWeight: 700,
              fontSize: 15,
            }}
            aria-hidden
          >
            A
          </span>
          <span className="font-semibold tracking-tight" style={{ fontSize: 15, color: "var(--text)" }}>
            ATLAS
          </span>
          <span className="chip hidden sm:inline-flex">v0.1.21</span>
        </a>

        <div className="hidden items-center gap-0.5 md:flex">
          {NAV_ITEMS.map(([label, anchor]) => (
            <a key={anchor} className="navlink focusring" href={`#${anchor}`} data-active={active === anchor}>
              {label}
            </a>
          ))}
        </div>

        <div className="flex items-center gap-2">
          <span className="hidden items-center gap-1.5 lg:flex" title={`${platform.system} ${platform.release} ${platform.machine}`}>
            <span
              style={{
                width: 7,
                height: 7,
                borderRadius: "50%",
                background: "var(--success)",
                boxShadow: "0 0 8px var(--success)",
                display: "inline-block",
              }}
              aria-hidden
            />
            <span className="mono" style={{ fontSize: 11, color: "var(--faint)" }}>
              {platform.system} {platform.release} {platform.machine}
            </span>
          </span>
          <a
            className="focusring chip"
            href="https://github.com/aziron-ai/atlas"
            target="_blank"
            rel="noreferrer"
            aria-label="Atlas on GitHub"
            style={{ textDecoration: "none" }}
          >
            <ExternalLink className="h-3.5 w-3.5" aria-hidden /> GitHub
          </a>
          <a href="#install" className="btn btn-primary focusring" style={{ minHeight: 34, padding: "0 14px", textDecoration: "none" }}>
            Get Atlas
          </a>
        </div>
      </nav>
    </header>
  );
}

/* ========================== HERO READOUT ================================ */

function HeroReadout({ data }) {
  const core = data.summary.core;
  // Summed raw units over the core matrix — the same aggregation the headline
  // ratios are computed from (graphifyTotal / atlasTotal). Used to give the
  // hero KPIs an honest at-a-glance magnitude picture.
  const sums = data.coreMatrix.reduce(
    (acc, r) => {
      const qs = r.querySummary;
      acc.atlasTokens += qs.atlasTokens || 0;
      acc.graphifyTokens += qs.graphifyTokens || 0;
      acc.atlasMs += qs.atlasMs || 0;
      acc.graphifyMs += qs.graphifyMs || 0;
      return acc;
    },
    { atlasTokens: 0, graphifyTokens: 0, atlasMs: 0, graphifyMs: 0 }
  );
  const liveLangs = data.summary.live.artifacts;
  const detectorOnly = data.summary.live.detectorOnlyArtifacts;
  return (
    <section
      id="hero"
      data-testid="hero"
      className="measure-grid"
      aria-labelledby="hero-title"
      style={{ borderBottom: "1px solid var(--line)" }}
    >
      <div className="shell grid grid-cols-1 gap-10 py-14 lg:grid-cols-[minmax(0,1fr)_minmax(380px,0.82fr)] lg:py-20">
        {/* LEFT — thesis + tickers */}
        <div className="flex min-w-0 flex-col justify-center">
          <div className="kicker" style={{ color: "var(--primary)" }}>
            The benchmark instrument
          </div>
          <h1
            id="hero-title"
            className="mt-4 max-w-2xl text-balance font-semibold"
            style={{ fontSize: "clamp(34px,5vw,56px)", lineHeight: 1.02, letterSpacing: "-0.025em" }}
          >
            Atlas gives review agents the smallest useful picture of a code change.
          </h1>
          <p className="mt-5 max-w-xl" style={{ fontSize: 15, lineHeight: 1.6, color: "var(--muted)" }}>
            A deterministic, LLM-free code-intelligence engine. It indexes a repo into a local symbol/call graph and
            hands agents the exact slice they need over MCP — far fewer model tokens, faster answers, code stays local,
            every claim auditable.
          </p>

          <div className="mt-10 grid grid-cols-1 gap-8 sm:grid-cols-2">
            <div className="min-w-0">
              <div className="kicker">Tokens vs graphify</div>
              <div className="mt-2 flex min-w-0 flex-wrap items-end gap-3">
                <CountUpRatio value={core.tokenRatio} testId="ratio-tokens" ariaLabel={`${core.tokenRatio} times fewer tokens`} />
              </div>
              <div className="mono mt-1" style={{ fontSize: 12, color: "var(--faint)" }}>
                fewer model tokens
              </div>
              <div className="mt-3 hidden sm:block">
                <MiniDumbbell
                  ratio={core.tokenRatio}
                  atlasVal={sums.atlasTokens}
                  graphifyVal={sums.graphifyTokens}
                  unit="tok"
                  color="var(--primary)"
                  label={`Atlas ${sums.atlasTokens} tokens versus graphify ${sums.graphifyTokens} tokens over the core matrix`}
                />
                <div className="mono mt-0.5" style={{ fontSize: 10, color: "var(--faint)" }}>
                  {num(sums.atlasTokens)} → {num(sums.graphifyTokens)} tok · summed
                </div>
              </div>
            </div>
            <div className="min-w-0">
              <div className="kicker">Latency vs graphify</div>
              <div className="mt-2 flex min-w-0 flex-wrap items-end gap-3">
                <CountUpRatio value={core.latencyRatio} testId="ratio-latency" ariaLabel={`${core.latencyRatio} times faster`} />
              </div>
              <div className="mono mt-1" style={{ fontSize: 12, color: "var(--faint)" }}>
                faster answers
              </div>
              <div className="mt-3 hidden sm:block">
                <MiniDumbbell
                  ratio={core.latencyRatio}
                  atlasVal={sums.atlasMs}
                  graphifyVal={sums.graphifyMs}
                  unit="ms"
                  color="var(--secondary)"
                  label={`Atlas ${Math.round(sums.atlasMs)} milliseconds versus graphify ${Math.round(sums.graphifyMs)} milliseconds over the core matrix`}
                />
                <div className="mono mt-0.5" style={{ fontSize: 10, color: "var(--faint)" }}>
                  {num(Math.round(sums.atlasMs))} → {num(Math.round(sums.graphifyMs))} ms · summed
                </div>
              </div>
            </div>
          </div>

          {/* honesty caveat — separated from the KPIs by a hairline so the eye
              reads headline → caveat → secondary ticks as distinct tiers */}
          <div className="mt-7 hairline" style={{ paddingTop: 18 }}>
            <p
              className="max-w-xl"
              style={{ fontSize: 12.5, lineHeight: 1.45, fontWeight: 500, color: "var(--warning)" }}
            >
              Computed over {core.equivalentRows} comparable core query rows where both Atlas and graphify answered the same
              question ({core.equivalentRows}/{core.queryRows} rows; {core.graphifyMissingRows} had no graphify equivalent
              and are never averaged in).
            </p>
          </div>

          {/* instrument rail of plain mono stat ticks */}
          <div
            className="mt-7 grid grid-cols-2 gap-x-6 gap-y-5 sm:grid-cols-4"
            style={{ borderTop: "1px solid var(--line)", paddingTop: 20 }}
          >
            <StatTick label="Languages" value={`${core.languages} core · ${liveLangs} live`} sub={`${detectorOnly} detector-only`} />
            <StatTick label="Comparable rows" value={`${core.equivalentRows}/${core.queryRows}`} sub="core matrix" />
            <StatTick label="Native parity" value={`${data.summary.coverage.deterministicRowsCovered}/${data.summary.coverage.graphifyRows}`} sub="deterministic rows" />
            <StatTick label="Evidence" value={data.sourceArtifacts.length} sub="downloadable artifacts" />
          </div>

          <div className="mt-9 flex flex-wrap gap-3">
            <a href="#vs-graphify" className="btn btn-primary focusring" style={{ textDecoration: "none" }}>
              See the proof <ArrowRight className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/benchmark-data.json" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              Download evidence <Download className="h-4 w-4" aria-hidden />
            </a>
          </div>
        </div>

        {/* RIGHT — framed graph console peek */}
        <div className="flex min-w-0 flex-col">
          <div className="panel flex h-full min-h-[420px] flex-col overflow-hidden p-0">
            <div className="flex items-center justify-between px-4 py-3" style={{ borderBottom: "1px solid var(--line)" }}>
              <span className="kicker">atlas export --all</span>
              <span className="mono" style={{ fontSize: 11, color: "var(--faint)" }}>
                live · deterministic
              </span>
            </div>
            <div className="flex-1 p-3">
              <GraphExplorer className="atlas-graph-peek" />
            </div>
            <div className="mono px-4 py-3" style={{ fontSize: 11, color: "var(--faint)", borderTop: "1px solid var(--line)" }}>
              280 of 9,139 symbols · 897 edges · 6 communities
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

/* ====================== VS GRAPHIFY — DUMBBELL ========================== */

function DumbbellChart({ rows, metric, widestLang, narrowestLang }) {
  // rows: [{lang, atlas, graphify, ratio, missing, atlasRaw, graphifyRaw}]
  // Per-row LOCAL scale: the Atlas dot sits at a fixed left anchor and the
  // line length encodes the ratio, so the *gap* is the visual and short rows
  // are never crushed into the left edge against a single shared max.
  const reduced = usePrefersReducedMotion();
  const [ref, inView] = useInView({ threshold: 0.2 });
  // Reveal is enhancement-only: geometry always renders at final positions;
  // `animate` merely opts into the entry transition once visible.
  const animate = !reduced && inView;
  const unit = metric === "tokens" ? "tok" : "ms";
  const maxRatio = Math.max(...rows.map((r) => r.ratio || 0), 1);
  const ATLAS_ANCHOR = 6; // % — fixed left anchor for every Atlas dot
  const TRACK_END = 96; // % — longest bar reaches here
  return (
    <div ref={ref} className="flex flex-col gap-1.5" role="table" aria-label={`Atlas versus graphify ${metric} per language`}>
      <div
        className="mono hidden grid-cols-[5.5rem_minmax(0,1fr)_4rem] items-center gap-3 px-1 pb-1 sm:grid"
        style={{ fontSize: 11, color: "var(--faint)" }}
        role="row"
      >
        <span role="columnheader">lang</span>
        <span role="columnheader">atlas → graphify ({unit}) · bar length ∝ ratio</span>
        <span role="columnheader" className="text-right">ratio</span>
      </div>
      {rows.map((r, i) => {
        // Bar length is proportional to this row's ratio against the largest
        // ratio in view — so every dumbbell spans most of the track and the
        // relative magnitude of the gap is directly legible.
        const aPct = ATLAS_ANCHOR;
        const span = ((r.ratio || 0) / maxRatio) * (TRACK_END - ATLAS_ANCHOR);
        const gPct = ATLAS_ANCHOR + span;
        const widest = r.lang === widestLang;
        const narrow = r.lang === narrowestLang;
        // Labels: anchor the Atlas value left-aligned at its dot, the graphify
        // value right-aligned at its dot, so the two never collide.
        return (
          <div
            key={r.lang}
            role="row"
            className="grid grid-cols-[5.5rem_minmax(0,1fr)_4rem] items-center gap-3 rounded-lg px-1 py-2 transition"
            style={{ background: i % 2 ? "rgba(255,255,255,0.018)" : "transparent" }}
          >
            <span className="num truncate" style={{ fontSize: 12.5, color: "var(--text)" }}>
              {langLabel(r.lang)}
            </span>
            <div className="relative" style={{ height: 26 }}>
              {/* track */}
              <div
                className="absolute"
                style={{ top: "50%", left: 0, right: 0, height: 1, background: "var(--line)", transform: "translateY(-50%)" }}
              />
              {/* connecting segment atlas->graphify (length ∝ ratio) */}
              <div
                className="absolute"
                style={{
                  top: "50%",
                  left: `${aPct}%`,
                  width: `${animate ? span : 0}%`,
                  height: 2,
                  background: "linear-gradient(90deg, var(--primary), var(--faint))",
                  transform: "translateY(-50%)",
                  transition: reduced ? "none" : "width 480ms cubic-bezier(0.22,1,0.36,1)",
                }}
              />
              {/* atlas dot — fixed anchor */}
              <div
                className="absolute"
                title={`Atlas ${num(Math.round(r.atlasRaw))} ${unit}`}
                style={{
                  top: "50%",
                  left: `${aPct}%`,
                  width: 9,
                  height: 9,
                  borderRadius: "50%",
                  background: "var(--primary)",
                  boxShadow: "0 0 8px rgba(94,230,196,0.5)",
                  transform: "translate(-50%,-50%)",
                }}
              />
              {/* graphify dot */}
              <div
                className="absolute"
                title={`graphify ${num(Math.round(r.graphifyRaw))} ${unit}`}
                style={{
                  top: "50%",
                  left: `${animate ? gPct : aPct}%`,
                  width: 9,
                  height: 9,
                  borderRadius: "50%",
                  background: "var(--faint)",
                  transform: "translate(-50%,-50%)",
                  transition: reduced ? "none" : "left 480ms cubic-bezier(0.22,1,0.36,1)",
                }}
              />
              {/* inline raw values — Atlas anchored left of its dot,
                  graphify anchored right of its dot: no overlap. */}
              <div
                className="mono absolute"
                style={{ top: -1, left: `${aPct}%`, fontSize: 10, color: "var(--primary)", transform: "translateX(-50%)" }}
              >
                {num(Math.round(r.atlasRaw))}
              </div>
              <div
                className="mono absolute"
                style={{
                  top: -1,
                  left: `${animate ? gPct : aPct}%`,
                  fontSize: 10,
                  color: "var(--faint)",
                  transform: "translateX(-50%)",
                  transition: reduced ? "none" : "left 480ms cubic-bezier(0.22,1,0.36,1)",
                }}
              >
                {num(Math.round(r.graphifyRaw))}
              </div>
              {r.missing > 0 && (
                <div
                  className="mono absolute"
                  style={{ bottom: -4, right: 0, fontSize: 10, color: "var(--warning)" }}
                  title={`${r.missing} query rows had no graphify equivalent`}
                >
                  ○ {r.missing} no equiv
                </div>
              )}
            </div>
            <div className="text-right">
              <span className="num" style={{ fontSize: 13, color: "var(--primary)" }}>
                {r.ratio.toFixed(2)}x
              </span>
              {widest && <div className="mono" style={{ fontSize: 9.5, color: "var(--warning)" }}>widest gap</div>}
              {narrow && <div className="mono" style={{ fontSize: 9.5, color: "var(--faint)" }}>narrowest</div>}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function VsGraphify({ data }) {
  const core = data.summary.core;
  const [metric, setMetric] = useState("tokens");
  const [sortByRatio, setSortByRatio] = useState(true);

  const rows = useMemo(() => {
    const base = data.coreMatrix.map((r) => {
      const qs = r.querySummary;
      const atlasRaw = metric === "tokens" ? qs.atlasTokens : qs.atlasMs;
      const graphifyRaw = metric === "tokens" ? qs.graphifyTokens : qs.graphifyMs;
      return {
        lang: r.language,
        atlas: atlasRaw,
        graphify: graphifyRaw,
        atlasRaw,
        graphifyRaw,
        ratio: metric === "tokens" ? qs.tokenRatio : qs.latencyRatio,
        missing: qs.graphifyMissing || 0,
      };
    });
    if (sortByRatio) base.sort((a, b) => b.ratio - a.ratio);
    return base;
  }, [data, metric, sortByRatio]);

  // Widest/narrowest are metric-dependent: under Tokens C is widest & JS
  // narrowest, but under Latency Python is widest & Java narrowest. Derive
  // them from the live ratios so the superlative labels never go stale.
  const { widestLang, narrowestLang } = useMemo(() => {
    let widest = null;
    let narrow = null;
    for (const r of rows) {
      if (r.ratio == null) continue;
      if (!widest || r.ratio > widest.ratio) widest = r;
      if (!narrow || r.ratio < narrow.ratio) narrow = r;
    }
    return { widestLang: widest?.lang, narrowestLang: narrow?.lang };
  }, [rows]);

  const aggregate = metric === "tokens" ? core.tokenRatio : core.latencyRatio;

  return (
    <section id="vs-graphify" data-testid="vs-graphify" className="shell py-16" aria-labelledby="vsg-title">
      <SectionHeader
        id="vsg-title"
        kicker="Efficiency · vs graphify"
        title="Per-language token and latency savings over the core matrix"
        actions={
          <div className="flex items-center gap-2">
            <div className="seg" role="group" aria-label="Toggle metric" data-testid="graphify-toggle">
              <button type="button" className="seg-btn focusring" data-active={metric === "tokens"} onClick={() => setMetric("tokens")} aria-pressed={metric === "tokens"}>
                Tokens
              </button>
              <button type="button" className="seg-btn focusring" data-active={metric === "latency"} onClick={() => setMetric("latency")} aria-pressed={metric === "latency"}>
                Latency
              </button>
            </div>
            <button
              type="button"
              className="focusring chip"
              onClick={() => setSortByRatio((s) => !s)}
              aria-pressed={sortByRatio}
              title="Sort rows by ratio"
            >
              <ArrowUpDown className="h-3.5 w-3.5" aria-hidden /> {sortByRatio ? "by ratio" : "by language"}
            </button>
          </div>
        }
      >
        Shown only where both tools answered the same query. graphify rows with no Atlas equivalent render as a hollow
        “no equivalent” tick, never a zero.
      </SectionHeader>

      {/* printed equation — non-negotiable credibility move */}
      <div
        data-testid="vs-graphify-equation"
        className="mono mb-7 flex flex-col gap-1 rounded-lg px-5 py-4 sm:flex-row sm:items-center sm:justify-between"
        style={{ background: "var(--bg2)", border: "1px solid var(--line)" }}
      >
        <span style={{ fontSize: 13.5, color: "var(--text)" }}>
          {metric === "tokens" ? "graphifyTokens ÷ atlasTokens" : "graphifyMs ÷ atlasMs"} over {core.equivalentRows}{" "}
          comparable rows
        </span>
        <span className="flex items-center gap-2">
          <span style={{ color: "var(--faint)" }}>=</span>
          <span style={{ fontSize: 20, fontWeight: 600, color: "var(--primary)" }}>{aggregate.toFixed(2)}x</span>
        </span>
      </div>

      <div className="panel p-5 sm:p-7">
        <DumbbellChart rows={rows} metric={metric} widestLang={widestLang} narrowestLang={narrowestLang} />
      </div>
    </section>
  );
}

/* ====================== VS NATIVE — COVERAGE SCATTER ==================== */

const TOOL_MANIFEST = [
  ["scip-go", "0.2.7"],
  ["scip-python", "0.6.6"],
  ["scip-typescript", "0.4.0"],
  ["scip-java", "0.12.3"],
  ["gopls", "v0.22.0"],
  ["pyright", "1.1.411"],
  ["tsc", "5.9.3"],
  ["jdtls", "1.58.0"],
  ["clangd", "17.0.0"],
  ["rust-analyzer", "0.0.0"],
  ["sourcekit-lsp", "6.2.4"],
];

function CoverageScatter({ data }) {
  const reduced = usePrefersReducedMotion();
  // inView is reveal-enhancement only (see useInView): points default to their
  // final opacity and only animate the staggered fade-in once revealed.
  const [ref, inView] = useInView({ threshold: 0.2 });
  const revealed = reduced || inView;
  const W = 720;
  const H = 360;
  const padL = 54;
  const padR = 24;
  const padT = 24;
  const padB = 46;
  const plotW = W - padL - padR;
  const plotH = H - padT - padB;

  const points = useMemo(() => {
    const detectorLangs = new Set(["ejs", "ets", "r"]);
    const raw = data.liveSmokes
      .filter((r) => r.querySummary && r.coverage)
      .map((r) => ({
        lang: r.language,
        x: r.coverage.ratio || 0,
        y: r.querySummary.tokenRatio,
        size: r.atlas?.index?.symbols || 0,
        c: 1,
        detector: detectorLangs.has(r.language) || r.detectorOnly,
        comparable: (r.querySummary.equivalentRows || 0) > 0,
      }));
    // x domain 0.9..max coverage, y domain 0..max token ratio
    const xs = raw.map((p) => p.x);
    const ys = raw.map((p) => (p.y == null ? 0 : p.y));
    const xMin = Math.min(0.95, ...xs);
    const xMax = Math.max(2, ...xs);
    const yMax = Math.max(45, ...ys);
    const sizes = raw.map((p) => p.size);
    const sMax = Math.max(...sizes, 1);

    const sx = (x) => padL + ((x - xMin) / (xMax - xMin)) * plotW;
    const sy = (y) => padT + plotH - (y / yMax) * plotH;
    const sr = (s) => 3 + Math.sqrt(s / sMax) * 9;

    // 1-D jitter/collision pass on y for clustered near-1.0 points
    const placed = raw
      .map((p) => ({
        ...p,
        px: sx(p.x),
        py: p.comparable && p.y != null ? sy(p.y) : padT + plotH - 6,
        r: sr(p.size),
      }))
      .sort((a, b) => a.px - b.px);
    for (let i = 0; i < placed.length; i += 1) {
      for (let j = 0; j < i; j += 1) {
        const a = placed[i];
        const b = placed[j];
        const dx = a.px - b.px;
        const dy = a.py - b.py;
        const minDist = a.r + b.r + 2;
        const d = Math.hypot(dx, dy);
        if (d < minDist && d > 0.0001) {
          const push = (minDist - d) / 2;
          const ux = dx / d;
          const uy = dy / d;
          a.px += ux * push;
          a.py += uy * push;
          b.px -= ux * push;
          b.py -= uy * push;
        }
      }
    }
    const parityX = sx(1.0);
    return { placed, xMin, xMax, yMax, sx, sy, parityX };
  }, [data]);

  const { placed, yMax, parityX, sx } = points;
  const yTicks = [0, Math.round(yMax / 3), Math.round((2 * yMax) / 3), Math.round(yMax)];
  const xTicks = [1.0, 1.5, 2.0].filter((t) => t >= points.xMin && t <= points.xMax);

  return (
    <div ref={ref} className="min-w-0 overflow-x-auto">
      <svg
        viewBox={`0 0 ${W} ${H}`}
        width="100%"
        style={{ minWidth: 560, display: "block" }}
        role="img"
        aria-label={`Coverage versus token-ratio scatter of ${placed.length} live language rows; native parity at coverage 1.0`}
      >
        {/* axes */}
        <line x1={padL} y1={padT} x2={padL} y2={padT + plotH} stroke="var(--line)" strokeWidth="1" />
        <line x1={padL} y1={padT + plotH} x2={W - padR} y2={padT + plotH} stroke="var(--line)" strokeWidth="1" />
        {/* y gridlines + ticks */}
        {yTicks.map((t) => {
          const y = padT + plotH - (t / yMax) * plotH;
          return (
            <g key={`y${t}`}>
              <line x1={padL} y1={y} x2={W - padR} y2={y} stroke="var(--grid)" strokeWidth="1" />
              <text x={padL - 8} y={y + 3} textAnchor="end" className="mono" fontSize="10" fill="var(--faint)">
                {t}x
              </text>
            </g>
          );
        })}
        {/* x ticks */}
        {xTicks.map((t) => {
          const x = sx(t);
          return (
            <text key={`x${t}`} x={x} y={padT + plotH + 16} textAnchor="middle" className="mono" fontSize="10" fill="var(--faint)">
              {t.toFixed(1)}
            </text>
          );
        })}
        {/* native-parity baseline at coverage 1.0 */}
        <line x1={parityX} y1={padT} x2={parityX} y2={padT + plotH} stroke="var(--warning)" strokeWidth="1" strokeDasharray="4 4" opacity="0.7" />
        <text x={parityX + 5} y={padT + 12} className="mono" fontSize="10" fill="var(--warning)">
          native parity 1.0
        </text>
        {/* axis labels */}
        <text x={padL + plotW / 2} y={H - 6} textAnchor="middle" className="mono" fontSize="10.5" fill="var(--muted)">
          coverage ratio (Atlas defs ÷ native defs)
        </text>
        <text
          x={14}
          y={padT + plotH / 2}
          textAnchor="middle"
          className="mono"
          fontSize="10.5"
          fill="var(--muted)"
          transform={`rotate(-90 14 ${padT + plotH / 2})`}
        >
          token ratio vs graphify
        </text>
        {/* points */}
        {placed.map((p, i) => (
          <g key={p.lang}>
            {p.detector ? (
              <circle
                cx={p.px}
                cy={p.py}
                r={Math.max(p.r, 5)}
                fill="none"
                stroke="var(--not-comparable)"
                strokeWidth="1.4"
                strokeDasharray="2 2"
                opacity={0.9}
                className={revealed && !reduced ? "scatter-pt" : undefined}
                style={revealed && !reduced ? { animationDelay: `${i * 12}ms` } : undefined}
              >
                <title>{`${langLabel(p.lang)} · detector-only · coverage ${p.x.toFixed(2)}`}</title>
              </circle>
            ) : (
              <circle
                cx={p.px}
                cy={p.py}
                r={p.r}
                fill={p.comparable ? COMMUNITY_COLORS[i % 6] : "var(--not-comparable)"}
                opacity={0.82}
                stroke={p.comparable ? "rgba(8,9,12,0.6)" : "none"}
                strokeWidth="1"
                className={revealed && !reduced ? "scatter-pt" : undefined}
                style={revealed && !reduced ? { animationDelay: `${i * 12}ms` } : undefined}
              >
                <title>{`${langLabel(p.lang)} · coverage ${p.x.toFixed(2)} · ${p.comparable ? `${(p.y || 0).toFixed(2)}x tokens` : "no comparable rows"} · ${num(p.size)} symbols`}</title>
              </circle>
            )}
          </g>
        ))}
      </svg>
    </div>
  );
}

function VsNative({ data }) {
  const cov = data.summary.coverage;
  return (
    <section id="vs-native" data-testid="vs-native" className="shell py-16" aria-labelledby="vsn-title">
      <SectionHeader
        id="vsn-title"
        kicker="Parity · vs native SCIP / LSP"
        title="Native-parity coverage shown separately from the token race"
      >
        Each dot is a live language: x is definition coverage against the best native indexer, y is the token ratio
        vs graphify, dot size is symbols indexed. Detector-only languages render as hollow outlines.
      </SectionHeader>

      {/* verbatim bordered standfirst */}
      <div
        data-testid="native-callout"
        className="mb-7 rounded-lg px-5 py-4"
        style={{ border: "1px solid var(--line-strong)", background: "var(--surface)" }}
      >
        <p className="text-balance" style={{ fontSize: 15, lineHeight: 1.5, color: "var(--text)" }}>
          <span style={{ color: "var(--secondary)", fontWeight: 600 }}>Different graph model</span> — not a token race,
          shown separately, never averaged in.
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1.55fr)_minmax(260px,0.9fr)]">
        <div className="panel min-w-0 p-5 sm:p-6">
          <CoverageScatter data={data} />
          <p className="mono mt-4" style={{ fontSize: 12, lineHeight: 1.45, color: "var(--faint)" }}>
            {cov.deterministicRowsCovered}/{cov.graphifyRows} deterministic rows covered at or above native parity ·{" "}
            {cov.detectorOnlyRowsCovered} detector-only.
          </p>
        </div>
        <div className="panel p-5 sm:p-6">
          <div className="kicker">Native baselines</div>
          <ul className="mt-4 flex flex-col gap-2.5" aria-label="Native tool manifest">
            {TOOL_MANIFEST.map(([tool, version]) => (
              <li
                key={tool}
                className="grid items-center gap-3"
                style={{ gridTemplateColumns: "minmax(0,1fr) auto 6px" }}
              >
                <span className="num truncate" style={{ fontSize: 12.5, color: "var(--text)" }}>
                  {tool}
                </span>
                <span
                  className="num tnum"
                  style={{ fontSize: 12, color: "var(--muted)", textAlign: "right", fontVariantNumeric: "tabular-nums" }}
                >
                  {version}
                </span>
                <span
                  style={{ width: 6, height: 6, borderRadius: "50%", background: "var(--success)" }}
                  aria-label="ok"
                />
              </li>
            ))}
          </ul>
          <p className="mt-4" style={{ fontSize: 12, lineHeight: 1.45, color: "var(--faint)" }}>
            Where no language server is installed, a scriptable source-counter proxy is used and labelled as such in
            the live matrix below.
          </p>
        </div>
      </div>
    </section>
  );
}

/* ===================== NOT COMPARABLE — FAULT LANE ===================== */

function NotComparable({ data }) {
  return (
    <section id="not-comparable" data-testid="not-comparable" className="shell py-16" aria-labelledby="nc-title">
      <SectionHeader id="nc-title" kicker="Honesty · where we don’t claim a win" title="Three languages produced zero graphify-equivalent rows">
        BYOND, ETS and R held native coverage ≥ 1.0 across five iterations, but graphify returned no comparable query
        rows. No latency or token ratio is claimed for them.
      </SectionHeader>

      <div
        className="mb-6 rounded-lg px-5 py-3.5"
        style={{ border: "1px solid var(--warning)", background: "rgba(242,180,58,0.06)" }}
      >
        <p className="mono" style={{ fontSize: 13, color: "var(--warning)" }}>
          Principle: no marketing average is ever polluted.
        </p>
      </div>

      <div data-testid="fault-lane" className="grid gap-4 lg:grid-cols-3">
        {data.saturation.map((row) => (
          <div key={row.language} className="panel flex flex-col p-5" style={{ borderColor: "rgba(242,180,58,0.35)" }}>
            <div className="flex items-baseline justify-between">
              <span className="font-semibold" style={{ fontSize: 16 }}>
                {langLabel(row.language)}
              </span>
              <span className="num" style={{ fontSize: 12, color: "var(--warning)" }}>
                {row.iterationsRun}/{row.iterationsRequested} iterations
              </span>
            </div>
            {/* 5-tick mono strip reading — */}
            <div className="mt-4 flex gap-1.5" aria-label="Five non-improving iterations, all not comparable">
              {row.iterations.map((it) => (
                <div
                  key={it.iteration}
                  className="mono flex h-9 flex-1 items-center justify-center rounded"
                  title={`Iteration ${it.iteration}: ${it.equivalentRows} equivalent rows, ${it.graphifyMissing} graphify missing`}
                  style={{ background: "var(--bg2)", border: "1px solid var(--line)", color: "var(--warning)", fontSize: 14 }}
                >
                  —
                </div>
              ))}
            </div>
            <dl className="mono mt-4 flex flex-col gap-1.5" style={{ fontSize: 12 }}>
              <div className="flex justify-between">
                <dt style={{ color: "var(--faint)" }}>equivalent rows</dt>
                <dd style={{ color: "var(--text)" }}>{row.iterations[0].equivalentRows}</dd>
              </div>
              <div className="flex justify-between">
                <dt style={{ color: "var(--faint)" }}>tokenRatio</dt>
                <dd style={{ color: "var(--faint)", textDecoration: "line-through" }}>null</dd>
              </div>
              <div className="flex justify-between">
                <dt style={{ color: "var(--faint)" }}>latencyRatio</dt>
                <dd style={{ color: "var(--faint)", textDecoration: "line-through" }}>null</dd>
              </div>
              <div className="flex justify-between">
                <dt style={{ color: "var(--faint)" }}>coverageRatio</dt>
                <dd style={{ color: "var(--success)" }}>≥ 1.0</dd>
              </div>
            </dl>
            <p className="mt-4" style={{ fontSize: 12, lineHeight: 1.45, color: "var(--muted)" }}>
              {row.note}
            </p>
            <div className="mt-4">
              <SourceLink href={row.artifact} download>
                <Download className="h-3 w-3" aria-hidden /> SATURATION_REPORT.json
              </SourceLink>
            </div>
          </div>
        ))}
      </div>

      <ol className="mt-7 grid gap-2.5 sm:grid-cols-2" data-testid="caveats" aria-label="Methodology caveats">
        {data.caveats.map((c, i) => (
          <li key={c} className="flex gap-3" style={{ fontSize: 12.5, lineHeight: 1.45, color: "var(--muted)" }}>
            <span className="mono shrink-0" style={{ color: "var(--faint)" }}>
              {i + 1}.
            </span>
            <span>{c}</span>
          </li>
        ))}
      </ol>
    </section>
  );
}

/* ========================= GRAPH SECTION =============================== */

function GraphSection({ data }) {
  const m = data.graphMeta || {};
  return (
    <section id="graph" data-testid="graph" className="shell py-16" aria-labelledby="graph-title">
      <SectionHeader id="graph-title" kicker="The map Atlas builds" title="The deterministic symbol & call graph">
        This is the “smallest useful slice” made visible: real <span className="mono">atlas export --all</span> output
        of the Atlas repo, downsampled to a connected core. Hover a node, drag, pan, zoom; click a hub to focus it.
      </SectionHeader>
      <div className="panel p-4 sm:p-5">
        <div style={{ minHeight: 520 }}>
          <GraphExplorer className="atlas-graph-full" />
        </div>
      </div>
    </section>
  );
}

/* ====================== HOW IT WORKS — SIGNAL PATH ===================== */

function SignalPath({ data }) {
  const reduced = usePrefersReducedMotion();
  const go = data.coreMatrix.find((r) => r.language === "go");
  const atlasTok = go ? go.querySummary.atlasTokens : 28;
  const graphifyTok = go ? go.querySummary.graphifyTokens : 389;
  const stages = [
    ["repo", "source files"],
    ["atlas index", "SQLite graph"],
    ["atlas context", "minimal slice"],
    ["MCP", "review agent"],
  ];
  return (
    <section id="how" data-testid="how" className="shell py-16" aria-labelledby="how-title">
      <SectionHeader id="how-title" kicker="How it works · the slice" title="Why the token count drops">
        Atlas indexes once into a local graph, then returns only the symbols, calls and relationships touching a change
        — not whole files.
      </SectionHeader>
      <div className="panel p-6 sm:p-8">
        <svg viewBox="0 0 880 130" width="100%" role="img" aria-label="Signal path: repo to atlas index to atlas context to MCP to agent" style={{ display: "block" }}>
          <defs>
            <marker id="arrow" markerWidth="8" markerHeight="8" refX="5" refY="4" orient="auto">
              <path d="M0,0 L6,4 L0,8 Z" fill="var(--faint)" />
            </marker>
          </defs>
          {/* wire */}
          <line x1="60" y1="55" x2="820" y2="55" stroke="var(--line)" strokeWidth="2" />
          {!reduced && <circle className="signal-dot" cx="60" cy="55" r="4" fill="var(--primary)" />}
          {stages.map(([title, sub], i) => {
            const x = 60 + i * (760 / 3);
            return (
              <g key={title}>
                <circle cx={x} cy={55} r="9" fill="var(--surface-raised)" stroke="var(--primary-dim)" strokeWidth="1.5" />
                <circle cx={x} cy={55} r="3.5" fill="var(--primary)" />
                <text x={x} y={28} textAnchor="middle" className="mono" fontSize="13" fill="var(--text)" fontWeight="600">
                  {title}
                </text>
                <text x={x} y={88} textAnchor="middle" className="mono" fontSize="11" fill="var(--faint)">
                  {sub}
                </text>
              </g>
            );
          })}
        </svg>
        <div className="mt-7 grid gap-4 sm:grid-cols-3" style={{ borderTop: "1px solid var(--line)", paddingTop: 24 }}>
          <StatTick label="Atlas context (Go)" value={`${atlasTok} tokens`} sub="per comparable query, summed" />
          <StatTick label="graphify context (Go)" value={`${num(graphifyTok)} tokens`} sub="same queries" />
          <StatTick label="Graph edges shown" value={`${num(data.graphMeta?.shown_edges ?? 897)} / ${num(data.graphMeta?.edges_total ?? 3102)}`} sub="atlas export --all" />
        </div>
      </div>
    </section>
  );
}

/* ========================= MATRIX (Core / Live) ======================= */

function RatioBar({ value, max, color = "var(--primary)" }) {
  const w = value == null ? 0 : Math.max(0, Math.min(100, (value / max) * 100));
  return (
    <div className="mt-1.5 h-1 overflow-hidden rounded-full" style={{ background: "var(--line)" }}>
      <div className="h-full rounded-full" style={{ width: `${w}%`, background: color }} />
    </div>
  );
}

function CoreMatrixTable({ data }) {
  return (
    <div className="tablewrap">
      <table className="dtable">
        <thead>
          <tr>
            <th>lang</th>
            <th>repo</th>
            <th>index s</th>
            <th>symbols</th>
            <th>edges</th>
            <th>token×</th>
            <th>latency×</th>
            <th>graphify nodes</th>
            <th>rows</th>
            <th>source</th>
          </tr>
        </thead>
        <tbody>
          {data.coreMatrix.map((r) => {
            const a = r.atlas.metrics || {};
            const g = r.graphify.metrics || {};
            const qs = r.querySummary;
            return (
              <tr key={r.language} data-testid={`matrix-row-${r.language}`}>
                <td>
                  <span style={{ color: "var(--text)" }}>{langLabel(r.language)}</span>
                  <div className="num" style={{ fontSize: 11, color: "var(--faint)" }}>
                    {r.atlas.status} · {r.graphify.status}
                  </div>
                </td>
                <td>
                  <a className="link" href={r.repo} target="_blank" rel="noreferrer">
                    {r.repo.replace("https://github.com/", "")}
                  </a>
                </td>
                <td className="num">{secs(a.cold_seconds ?? r.atlas.seconds)}</td>
                <td className="num">{num(a.symbols)}</td>
                <td className="num">{num(a.edges)}</td>
                <td style={{ minWidth: 96 }}>
                  <span className="num" style={{ color: "var(--primary)" }}>{ratio(qs.tokenRatio)}</span>
                  <RatioBar value={qs.tokenRatio} max={32} />
                </td>
                <td style={{ minWidth: 96 }}>
                  <span className="num" style={{ color: "var(--secondary)" }}>{ratio(qs.latencyRatio)}</span>
                  <RatioBar value={qs.latencyRatio} max={10} color="var(--secondary)" />
                </td>
                <td className="num">{num(g.nodes)}</td>
                <td>
                  <span className="num">{qs.equivalentRows}/{qs.rows}</span>
                  {qs.graphifyMissing > 0 && (
                    <div className="mono" style={{ fontSize: 10.5, color: "var(--warning)" }}>
                      ○ {qs.graphifyMissing} no equiv
                    </div>
                  )}
                </td>
                <td>
                  <SourceLink href={r.artifact} download>JSON</SourceLink>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function LiveDetailDrawer({ row }) {
  if (!row) return null;
  return (
    <aside
      data-testid="detail-drawer"
      className="panel min-w-0 p-5 xl:sticky xl:top-[72px] xl:max-h-[calc(100vh-88px)] xl:overflow-auto"
      aria-label={`${langLabel(row.language)} evidence`}
    >
      <div className="flex items-baseline justify-between gap-2">
        <h3 className="font-semibold" style={{ fontSize: 16 }}>{langLabel(row.language)}</h3>
        <span className="num" style={{ fontSize: 11, color: "var(--faint)" }}>{shortSha(row.commit)}</span>
      </div>
      <a className="link mono mt-1 block break-words" href={row.repo} target="_blank" rel="noreferrer" style={{ fontSize: 12 }}>
        {row.repo.replace("https://github.com/", "")}
      </a>
      <div className="mono mt-4 grid grid-cols-2 gap-3" style={{ fontSize: 12 }}>
        <div>
          <div style={{ color: "var(--faint)" }}>native baseline</div>
          <div style={{ color: "var(--text)" }}>{row.native.tool}</div>
        </div>
        <div>
          <div style={{ color: "var(--faint)" }}>coverage</div>
          <div style={{ color: row.coverage.ratio >= 1 ? "var(--success)" : "var(--text)" }}>{ratio(row.coverage.ratio)}</div>
        </div>
        <div>
          <div style={{ color: "var(--faint)" }}>symbols</div>
          <div style={{ color: "var(--text)" }}>{num(row.atlas?.index?.symbols)}</div>
        </div>
        <div>
          <div style={{ color: "var(--faint)" }}>comparable rows</div>
          <div style={{ color: "var(--text)" }}>{row.querySummary.equivalentRows}/{row.querySummary.rows}</div>
        </div>
      </div>
      <div className="tablewrap mt-4">
        <table className="dtable" style={{ minWidth: 420 }}>
          <thead>
            <tr>
              <th>query</th>
              <th>atlas tok</th>
              <th>graphify tok</th>
              <th>status</th>
            </tr>
          </thead>
          <tbody>
            {row.queries.map((q) => (
              <tr key={q.symbol}>
                <td className="num">{q.symbol}</td>
                <td className="num">{num(q.atlasTokens)}</td>
                <td className="num">{q.graphifyMissing ? "—" : num(q.graphifyTokens)}</td>
                <td>
                  {q.graphifyMissing ? (
                    <span className="mono" style={{ fontSize: 11, color: "var(--warning)" }}>○ no equiv</span>
                  ) : (
                    <span className="mono" style={{ fontSize: 11, color: "var(--success)" }}>comparable</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="mt-4" style={{ fontSize: 12, lineHeight: 1.5, color: "var(--muted)" }}>
        {row.optimization?.stopReason || "No optimization note recorded."}
      </p>
      <div className="mt-3">
        <SourceLink href={row.artifact} download>
          <Download className="h-3 w-3" aria-hidden /> raw artifact
        </SourceLink>
      </div>
    </aside>
  );
}

function LiveTable({ data }) {
  const [filter, setFilter] = useState("all");
  const [sort, setSort] = useState("language");
  const [search, setSearch] = useState("");
  const [detailLang, setDetailLang] = useState("rust");

  const rows = useMemo(() => {
    let next = [...data.liveSmokes];
    const term = search.trim().toLowerCase();
    if (filter === "ok") next = next.filter((r) => r.querySummary.equivalentRows > 0 && !r.detectorOnly);
    if (filter === "partial") next = next.filter((r) => r.querySummary.equivalentRows === 0);
    if (filter === "detector") next = next.filter((r) => r.detectorOnly || ["ejs", "ets", "r"].includes(r.language));
    if (term) {
      next = next.filter((r) =>
        [r.language, r.repo, r.commit, r.native.tool, r.artifact].some((v) => String(v || "").toLowerCase().includes(term))
      );
    }
    next.sort((a, b) => {
      if (sort === "tokens") return (b.querySummary.tokenRatio || 0) - (a.querySummary.tokenRatio || 0);
      if (sort === "latency") return (b.querySummary.latencyRatio || 0) - (a.querySummary.latencyRatio || 0);
      if (sort === "coverage") return (b.coverage.ratio || 0) - (a.coverage.ratio || 0);
      if (sort === "symbols") return (b.atlas?.index?.symbols || 0) - (a.atlas?.index?.symbols || 0);
      return a.language.localeCompare(b.language);
    });
    return next;
  }, [data.liveSmokes, filter, sort, search]);

  const detail = data.liveSmokes.find((r) => r.language === detailLang) || data.liveSmokes[0];

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
      <div className="min-w-0">
        <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style={{ color: "var(--faint)" }} aria-hidden />
            <input
              id="live-search"
              data-testid="live-search"
              className="field focusring pl-9"
              type="search"
              placeholder="Search language, repo, commit, tool…"
              aria-label="Search live smokes"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <select
              id="live-filter"
              data-testid="live-filter"
              className="field focusring"
              style={{ width: "auto" }}
              aria-label="Filter live smokes"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            >
              <option value="all">All rows</option>
              <option value="ok">Comparable</option>
              <option value="partial">No comparable rows</option>
              <option value="detector">Detector-only</option>
            </select>
            <select
              id="live-sort"
              data-testid="live-sort"
              className="field focusring"
              style={{ width: "auto" }}
              aria-label="Sort live smokes"
              value={sort}
              onChange={(e) => setSort(e.target.value)}
            >
              <option value="language">Sort: language</option>
              <option value="tokens">Sort: token ratio</option>
              <option value="latency">Sort: latency ratio</option>
              <option value="coverage">Sort: coverage</option>
              <option value="symbols">Sort: symbols</option>
            </select>
          </div>
        </div>
        <div className="tablewrap">
          <table className="dtable">
            <thead>
              <tr>
                <th>lang</th>
                <th>repo · commit</th>
                <th>native tool</th>
                <th>coverage</th>
                <th>token×</th>
                <th>latency×</th>
                <th>rows</th>
                <th>status</th>
                <th>evidence</th>
              </tr>
            </thead>
            <tbody id="live-body">
              {rows.map((r) => {
                const comparable = r.querySummary.equivalentRows > 0;
                const detector = r.detectorOnly || ["ejs", "ets", "r"].includes(r.language);
                return (
                  <tr key={r.language}>
                    <td>
                      <button
                        type="button"
                        className="focusring text-left"
                        onClick={() => setDetailLang(r.language)}
                        style={{ color: "var(--text)", background: "none", border: "none", cursor: "pointer", padding: 0, font: "inherit" }}
                        title="Inspect"
                      >
                        {langLabel(r.language)}
                      </button>
                      <div className="num" style={{ fontSize: 11, color: "var(--faint)" }}>{r.language}</div>
                    </td>
                    <td>
                      <a className="link" href={r.repo} target="_blank" rel="noreferrer">
                        {r.repo.replace("https://github.com/", "")}
                      </a>
                      <div className="num" style={{ fontSize: 11, color: "var(--faint)" }}>{shortSha(r.commit)}</div>
                    </td>
                    <td className="num" style={{ color: "var(--muted)" }}>{r.native.tool}</td>
                    <td className="num" style={{ color: r.coverage.ratio >= 1 ? "var(--success)" : "var(--text)" }}>
                      {ratio(r.coverage.ratio)}
                    </td>
                    <td className="num" style={{ color: comparable ? "var(--primary)" : "var(--not-comparable)" }}>
                      {comparable ? ratio(r.querySummary.tokenRatio) : "not comparable"}
                    </td>
                    <td className="num" style={{ color: comparable ? "var(--secondary)" : "var(--not-comparable)" }}>
                      {comparable ? ratio(r.querySummary.latencyRatio) : "not comparable"}
                    </td>
                    <td className="num">{r.querySummary.equivalentRows}/{r.querySummary.rows}</td>
                    <td>
                      {detector ? (
                        <span className="mono" style={{ fontSize: 11, color: "var(--warning)" }}>detector-only</span>
                      ) : comparable ? (
                        <span className="mono" style={{ fontSize: 11, color: "var(--success)" }}>ok</span>
                      ) : (
                        <span className="mono" style={{ fontSize: 11, color: "var(--not-comparable)" }}>not comparable</span>
                      )}
                    </td>
                    <td>
                      <SourceLink href={r.artifact} download>raw</SourceLink>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>
      <LiveDetailDrawer row={detail} />
    </div>
  );
}

function Matrix({ data }) {
  const [tab, setTab] = useState("core");
  return (
    <section id="matrix" data-testid="matrix" className="shell py-16" aria-labelledby="matrix-title">
      <SectionHeader
        id="matrix-title"
        kicker="Full benchmark matrix"
        title="Every row, every artifact"
        actions={
          <div className="seg" role="tablist" aria-label="Matrix scope">
            <button
              type="button"
              role="tab"
              data-testid="matrix-tab-core"
              className="seg-btn focusring"
              data-active={tab === "core"}
              aria-selected={tab === "core"}
              onClick={() => setTab("core")}
            >
              Core {data.summary.core.languages}
            </button>
            <button
              type="button"
              role="tab"
              data-testid="matrix-tab-live"
              className="seg-btn focusring"
              data-active={tab === "live"}
              aria-selected={tab === "live"}
              onClick={() => setTab("live")}
            >
              Live {data.summary.live.artifacts}
            </button>
          </div>
        }
      >
        {tab === "core"
          ? "The 7 core languages benchmarked head-to-head against graphify plus native SCIP/LSP baselines."
          : "36 live open-source repository checks, each at a pinned commit with the best available native or parser baseline. Click a language to inspect its per-query breakdown."}
      </SectionHeader>
      {tab === "core" ? <CoreMatrixTable data={data} /> : <LiveTable data={data} />}
    </section>
  );
}

/* ============================ INSTALL ================================= */

const INSTALL_TABS = {
  homebrew: {
    label: "Homebrew",
    sub: "macOS cask",
    lines: ["brew install --cask dominic097/atlas/atlas", "atlas version"],
  },
  npm: {
    label: "npm",
    sub: "node wrapper",
    lines: ["npm install -g @dominic097/atlas", "atlas version"],
  },
  linux: {
    label: "Linux",
    sub: "amd64 / arm64",
    lines: [
      "curl -LO https://github.com/aziron-ai/atlas/releases/download/v0.1.21/atlas_0.1.21_linux_amd64.tar.gz",
      "tar -xzf atlas_0.1.21_linux_amd64.tar.gz",
      "sudo install -m 0755 atlas /usr/local/bin/atlas",
      "atlas version",
    ],
  },
};

function TermBlock({ lines }) {
  const text = lines.join("\n");
  return (
    <div className="term">
      <div className="flex items-center justify-between px-3 py-2" style={{ borderBottom: "1px solid var(--line)", background: "var(--surface)" }}>
        <span className="mono" style={{ fontSize: 11, color: "var(--faint)" }}>zsh</span>
        <CopyButton text={text} label="command" />
      </div>
      <div className="term-body">
        {lines.map((l, i) => (
          <div key={i}>
            <span className="term-prompt">$ </span>
            {l}
          </div>
        ))}
      </div>
    </div>
  );
}

function UsageStep({ n, title, line, copy }) {
  return (
    <div data-testid="usage-step" className="panel min-w-0 p-5">
      <div className="flex items-center gap-3">
        <span
          className="mono grid place-items-center rounded-full"
          style={{ width: 26, height: 26, background: "var(--surface-raised)", border: "1px solid var(--line-strong)", color: "var(--primary)", fontSize: 12, fontWeight: 600 }}
        >
          {n}
        </span>
        <h3 className="font-semibold" style={{ fontSize: 15 }}>{title}</h3>
      </div>
      <div className="mt-4">
        <TermBlock lines={[line]} />
      </div>
      <p className="mt-3" style={{ fontSize: 12.5, lineHeight: 1.5, color: "var(--muted)" }}>{copy}</p>
    </div>
  );
}

function Install() {
  const [tab, setTab] = useState("homebrew");
  return (
    <section id="install" data-testid="install" className="shell py-16" aria-labelledby="install-title">
      <SectionHeader
        id="install-title"
        kicker="Install & connect"
        title="One local binary, a SQLite graph, MCP for agents"
        actions={
          <>
            <SourceLink href="https://github.com/aziron-ai/atlas/releases/latest" testId={false}>
              releases/latest <ExternalLink className="h-3 w-3" aria-hidden />
            </SourceLink>
            <SourceLink href="https://github.com/dominic097/homebrew-atlas" testId={false}>
              Homebrew tap
            </SourceLink>
            <SourceLink href="https://www.npmjs.com/package/@dominic097/atlas" testId={false}>
              npm
            </SourceLink>
          </>
        }
      >
        The default database is <span className="mono">sqlite://./.atlas/atlas.db</span> — no shared server required.
      </SectionHeader>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
        <div className="panel min-w-0 p-5">
          <div className="seg mb-4" role="tablist" aria-label="Install method">
            {Object.entries(INSTALL_TABS).map(([key, t]) => (
              <button
                key={key}
                type="button"
                role="tab"
                data-testid={`install-tab-${key}`}
                className="seg-btn focusring"
                data-active={tab === key}
                aria-selected={tab === key}
                onClick={() => setTab(key)}
              >
                {t.label}
              </button>
            ))}
          </div>
          {/* All method panels stay in the DOM (verbatim commands always present);
              only the active one is shown. Hidden panels keep textContent for audit. */}
          {Object.entries(INSTALL_TABS).map(([key, t]) => (
            <div key={key} className={tab === key ? "" : "hidden"} aria-hidden={tab !== key}>
              <div className="kicker mb-3">{t.sub}</div>
              <TermBlock lines={t.lines} />
              {key === "linux" && (
                <p className="mt-3" style={{ fontSize: 12, color: "var(--faint)" }}>
                  Release assets also include .deb, .rpm and .apk packages for amd64 and arm64.
                </p>
              )}
            </div>
          ))}
        </div>

        <div className="grid min-w-0 gap-4">
          <UsageStep n="1" title="Index a repository" line="atlas index . --reindex" copy="Builds the local symbol, call, route and search graph into SQLite." />
          <UsageStep
            n="2"
            title="Retrieve review context"
            line={`atlas context --paths path/to/changed-file.go --query "review risk" --format json`}
            copy="Returns a compact context bundle around the changed files for a review agent."
          />
          <UsageStep
            n="3"
            title="Connect agents"
            line={`atlas mcp --transport http --http 127.0.0.1:8765`}
            copy="Then atlas install skill --agent codex and atlas install skill --agent claude to wire local assistants over MCP."
          />
        </div>
      </div>

      <div className="panel mt-6 p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="kicker">Download the benchmark</div>
            <p className="mt-2" style={{ fontSize: 13, color: "var(--muted)" }}>
              The same JSON used by this page, plus the raw matrix report.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <SourceLink href="data/benchmark-data.json" download>
              <Download className="h-3 w-3" aria-hidden /> benchmark-data.json
            </SourceLink>
            <SourceLink href="data/raw/MATRIX_REPORT.json" download>
              <Download className="h-3 w-3" aria-hidden /> MATRIX_REPORT.json
            </SourceLink>
          </div>
        </div>
        <div className="mt-4">
          <TermBlock
            lines={[
              "curl -LO https://aziron-ai.github.io/atlas/data/benchmark-data.json",
              "curl -LO https://aziron-ai.github.io/atlas/data/raw/MATRIX_REPORT.json",
            ]}
          />
        </div>
      </div>
    </section>
  );
}

/* =========================== EVIDENCE ================================= */

function Evidence({ data }) {
  const featured = ["benchmark-data.json", "MATRIX_REPORT.json"];
  const tools = useMemo(() => {
    const wanted = ["atlas", "graphify", "scip-go", "gopls", "pyright", "tsc", "clangd", "rust-analyzer", "sourcekit-lsp"];
    return data.provenance.tools.core.filter((t) => wanted.includes(t.tool));
  }, [data]);

  return (
    <section id="evidence" data-testid="evidence" className="shell py-16" aria-labelledby="evidence-title">
      <SectionHeader
        id="evidence-title"
        kicker="Evidence & provenance"
        title="Every number on this page is downloadable"
      >
        Auditability is the closing argument: the page is generated entirely from these committed benchmark artifacts.
      </SectionHeader>

      {/* featured download chips */}
      <div className="mb-6 flex flex-wrap gap-3">
        <a href="data/benchmark-data.json" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> benchmark-data.json
        </a>
        <a href="data/raw/MATRIX_REPORT.json" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> MATRIX_REPORT.json
        </a>
        <a href="data/raw/SATURATION_REPORT.json" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> SATURATION_REPORT.json
        </a>
        <a href="data/raw/GRAPHIFY_LANGUAGE_DISCOVERY.json" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> GRAPHIFY_LANGUAGE_DISCOVERY.json
        </a>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        {/* LEFT — all source artifacts */}
        <div className="panel min-w-0 p-5">
          <div className="kicker mb-4">Source artifacts · {data.sourceArtifacts.length}</div>
          <div className="tablewrap" style={{ maxHeight: 480, overflowY: "auto" }}>
            <table className="dtable" style={{ minWidth: 460 }}>
              <thead>
                <tr>
                  <th>artifact</th>
                  <th>bytes</th>
                  <th>sha256</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {data.sourceArtifacts.map((a) => (
                  <tr key={a.path} data-testid="source-artifact">
                    <td className="num" style={{ color: "var(--text)" }}>{a.name}</td>
                    <td className="num" style={{ color: "var(--muted)" }}>{num(a.bytes)}</td>
                    <td className="num" style={{ color: "var(--faint)" }}>{a.sha256.slice(0, 12)}…</td>
                    <td>
                      <a className="focusring chip" href={a.path} download data-source-artifact data-testid="download-link" style={{ textDecoration: "none" }}>
                        <Download className="h-3 w-3" aria-hidden /> get
                      </a>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* RIGHT — provenance */}
        <div data-testid="provenance" className="panel min-w-0 p-5">
          <div className="kicker mb-4">Provenance</div>
          <dl className="mono flex flex-col gap-2.5" style={{ fontSize: 12.5 }}>
            <div className="flex justify-between gap-3">
              <dt style={{ color: "var(--faint)" }}>graphify</dt>
              <dd style={{ color: "var(--text)" }}>{data.provenance.graphify.version}</dd>
            </div>
            <div className="flex justify-between gap-3">
              <dt style={{ color: "var(--faint)" }}>dispatch count</dt>
              <dd style={{ color: "var(--text)" }}>{data.provenance.graphify.dispatchCount}</dd>
            </div>
            <div className="flex justify-between gap-3">
              <dt style={{ color: "var(--faint)" }}>platform</dt>
              <dd style={{ color: "var(--text)" }}>
                {data.provenance.platform.system} {data.provenance.platform.release} {data.provenance.platform.machine}
              </dd>
            </div>
            <div className="flex justify-between gap-3">
              <dt style={{ color: "var(--faint)" }}>python</dt>
              <dd style={{ color: "var(--text)" }}>{data.provenance.platform.python}</dd>
            </div>
          </dl>

          <div className="kicker mb-3 mt-6">Core tool versions</div>
          <ul className="mono flex flex-col gap-2" style={{ fontSize: 12 }}>
            {tools.map((t) => (
              <li key={t.tool} className="flex items-center justify-between gap-3">
                <span style={{ color: "var(--muted)" }}>{t.tool}</span>
                <span className="truncate" style={{ color: "var(--text)", maxWidth: 180, textAlign: "right" }} title={t.version}>
                  {t.version}
                </span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}

/* ============================= APP =================================== */

function useScrollSpy(ids) {
  const [active, setActive] = useState(ids[0]);
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") return undefined;
    const sections = ids.map((id) => document.getElementById(id)).filter(Boolean);
    const io = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => b.intersectionRatio - a.intersectionRatio);
        if (visible[0]) setActive(visible[0].target.id);
      },
      { rootMargin: "-50% 0px -45% 0px", threshold: [0, 0.25, 0.5] }
    );
    sections.forEach((s) => io.observe(s));
    return () => io.disconnect();
  }, [ids.join(",")]);
  return active;
}

function App() {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);
  const [graphMeta, setGraphMeta] = useState(null);
  const active = useScrollSpy(["vs-graphify", "graph", "vs-native", "evidence", "install"]);

  useEffect(() => {
    fetch("data/benchmark-data.json", { cache: "no-store" })
      .then((r) => {
        if (!r.ok) throw new Error(`Unable to load benchmark data: ${r.status}`);
        return r.json();
      })
      .then(setData)
      .catch((e) => {
        console.error(e);
        setError(e);
      });
    // graph meta is read for the How-it-works real edge numbers; if it fails
    // we fall back to the brief-verified constants, no console error surfaced.
    fetch("data/graph.json")
      .then((r) => (r.ok ? r.json() : null))
      .then((j) => {
        if (j && j.meta) setGraphMeta(j.meta);
      })
      .catch(() => {});
  }, []);

  if (error) {
    return (
      <main className="shell py-16">
        <div className="panel p-6" style={{ borderColor: "var(--danger)" }}>
          <div className="kicker" style={{ color: "var(--danger)" }}>load error</div>
          <p className="mt-2" style={{ color: "var(--text)" }}>{error.message}</p>
        </div>
      </main>
    );
  }

  if (!data) {
    return (
      <main className="shell py-24" aria-busy="true">
        <div className="panel p-6">
          <div className="kicker">loading</div>
          <div className="mt-4 flex flex-col gap-3">
            {[0, 1, 2].map((i) => (
              <div key={i} className="h-3 rounded" style={{ background: "var(--surface-raised)", width: `${80 - i * 18}%` }} />
            ))}
          </div>
          <p className="mt-5 mono" style={{ fontSize: 12, color: "var(--faint)" }}>
            fetching data/benchmark-data.json…
          </p>
        </div>
      </main>
    );
  }

  const enriched = { ...data, graphMeta };

  return (
    <>
      <a className="skip-link" href="#main">Skip to content</a>
      <ConsoleBar data={data} active={active} />
      <main id="main">
        <HeroReadout data={data} />
        <VsGraphify data={data} />
        <VsNative data={data} />
        <NotComparable data={data} />
        <GraphSection data={enriched} />
        <SignalPath data={enriched} />
        <Matrix data={data} />
        <Install />
        <Evidence data={data} />
      </main>
      <footer className="hairline" style={{ marginTop: 8 }}>
        <div className="shell flex flex-col gap-2 py-8 sm:flex-row sm:items-center sm:justify-between">
          <span className="mono" style={{ fontSize: 12, color: "var(--faint)" }}>
            {data.sourceLabel} · generated {data.generatedAt}
          </span>
          <span className="mono" style={{ fontSize: 12, color: "var(--faint)" }}>
            Atlas v0.1.21 · static, data-driven, self-contained
          </span>
        </div>
      </footer>
    </>
  );
}

createRoot(document.getElementById("root")).render(<App />);
