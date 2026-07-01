import React, { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { createRoot } from "react-dom/client";
import {
  ArrowRight,
  ArrowUpDown,
  Check,
  Copy,
  Download,
  ExternalLink,
} from "lucide-react";
import GraphExplorer from "./GraphExplorer";
import LanguagesExplorer from "./LanguagesExplorer";

/* ============================================================
   Atlas — The Benchmark Instrument
   Dark instrument console. Every numeral traces to
   data/benchmark-data.json. No chart CDN — inline SVG/canvas/CSS.
   ============================================================ */

const fmt = new Intl.NumberFormat("en-US");

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
  if (v === "javascript") return "JavaScript";
  if (v === "typescript") return "TypeScript";
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
function tenXModel(data) {
  const summary = data.summary?.live || {};
  const liveRows = (data.liveBenchmarks || []).filter(
    (r) => r.coverage && typeof r.coverage.ratio === "number"
  );
  const parityRows = liveRows.filter((r) => r.coverage.ratio <= 1.0001);
  const parityComparable = parityRows.filter(
    (r) =>
      r.querySummary &&
      r.querySummary.equivalentRows > 0 &&
      r.querySummary.tokenRatio != null &&
      r.querySummary.latencyRatio != null
  );
  const liveComparable = liveRows.filter(
    (r) =>
      r.querySummary &&
      r.querySummary.equivalentRows > 0 &&
      r.querySummary.tokenRatio != null &&
      r.querySummary.latencyRatio != null
  );
  const count = (rows, pred) => rows.filter(pred).length;
  return {
    liveTotal: summary.artifacts ?? liveRows.length,
    liveComparable: summary.withComparableRows ?? liveComparable.length,
    liveCoverageExceed: summary.coverageExceedLanguages ?? count(liveRows, (r) => r.coverage.ratio > 1.0001),
    liveToken10: summary.token10xComparable ?? count(liveComparable, (r) => r.querySummary.tokenRatio >= 10),
    liveLatency10: summary.latency10xComparable ?? count(liveComparable, (r) => r.querySummary.latencyRatio >= 10),
    livePerformance10: summary.tenXComparable ?? count(liveComparable, (r) => r.querySummary.tokenRatio >= 10 && r.querySummary.latencyRatio >= 10),
    parityTotal: summary.coverageParityLanguages ?? parityRows.length,
    parityComparable: summary.parityComparable ?? parityComparable.length,
    parityCoverageExceed: 0,
    parityToken10: summary.parityToken10x ?? count(parityComparable, (r) => r.querySummary.tokenRatio >= 10),
    parityLatency10: summary.parityLatency10x ?? count(parityComparable, (r) => r.querySummary.latencyRatio >= 10),
    parityPerformance10: summary.parityTenX ?? count(parityComparable, (r) => r.querySummary.tokenRatio >= 10 && r.querySummary.latencyRatio >= 10),
    parityNonComparable: parityRows.length - parityComparable.length,
  };
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

/* ========================== CONSOLE BAR ================================== */

const NAV_ITEMS = [
  ["Coverage", "vs-native"],
  ["Comparison", "vs-graphify"],
  ["Graph", "graph"],
  ["Languages", "matrix"],
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
  const liveLangs = data.summary.live.artifacts;
  const supported = data.summary.supported || {};
  const supportedFamilies = supported.families ?? core.languages + liveLangs;
  const supportedAtlasOk = supported.atlasOk ?? supportedFamilies;
  const liveSummary = data.summary.live;
  const parityCoverage = liveSummary.coverageParityLanguages ?? 0;
  const exceedCoverage = liveSummary.coverageExceedLanguages ?? 0;
  // Atlas's OWN per-query response size, straight from coreMatrix atlasTokens —
  // the hero identity. graphify's ratio is a supporting comparison, not this.
  const atlasTokensList = data.coreMatrix.map((r) => r.querySummary.atlasTokens).filter((v) => v != null);
  const tokMin = Math.min(...atlasTokensList);
  const tokMax = Math.max(...atlasTokensList);
  const fastestIndex = Math.min(...data.coreMatrix.map((r) => r.atlas.metrics.cold_seconds).filter((v) => v != null));
  const toolCount = data.provenance.tools.coreCount;
  // Native-parity result the ladder PROVES: count of live languages whose
  // coverage ratio is at or above ×1.0 — every benchmarked live language.
  // (Distinct from the 39/39 comparable deterministic-row figure, which counts
  // query rows, not languages — they are reported as separate facts, never one
  // mixed denominator.)
  const parityLangs = data.liveBenchmarks.filter(
    (r) => r.coverage && typeof r.coverage.ratio === "number" && r.coverage.ratio >= 1.0
  ).length;
  const liveCovered = data.liveBenchmarks.filter(
    (r) => r.coverage && typeof r.coverage.ratio === "number"
  ).length;
  // comparable deterministic-row universe — the data key was renamed off the
  // graphify-anchored `graphifyRows`; read the new name with a back-compat fallback.
  const cov = data.summary.coverage;
  const comparableRows = cov.comparableRows ?? cov.graphifyRows;
  // Per-query answer-size spread, derived from coreMatrix atlasTokens so the
  // inline annotation can never drift from the figure: median + the language
  // that owns each end of the 14–42 range.
  const sortedTokens = [...atlasTokensList].sort((a, b) => a - b);
  const tokMedian = sortedTokens.length
    ? sortedTokens.length % 2
      ? sortedTokens[(sortedTokens.length - 1) / 2]
      : Math.round((sortedTokens[sortedTokens.length / 2 - 1] + sortedTokens[sortedTokens.length / 2]) / 2)
    : tokMin;
  const tokMinLang = data.coreMatrix.find((r) => r.querySummary.atlasTokens === tokMin)?.language;
  const tokMaxLang = data.coreMatrix.find((r) => r.querySummary.atlasTokens === tokMax)?.language;
  // The row that ACTUALLY owns the fastest cold index — its symbol/edge caption
  // must come from this row, not a hardcoded language. (Go cold-indexes far
  // slower than the fastest slice, so a fixed Go caption mislabels the figure.)
  const fastestRow = data.coreMatrix.find((r) => r.atlas.metrics.cold_seconds === fastestIndex);
  // The supported-family count comes from parser.Supported fixture evidence.
  // Public-repo matrix/live artifacts remain separate, stronger evidence tiers.
  const nativeLangCount = supportedFamilies;
  const derivedCount = data.derivedArtifacts?.length || 0;
  return (
    <section
      id="hero"
      data-testid="hero"
      className="measure-grid"
      aria-labelledby="hero-title"
      style={{ borderBottom: "1px solid var(--line)" }}
    >
      <div className="shell grid grid-cols-1 gap-10 py-14 lg:grid-cols-[minmax(0,1fr)_minmax(380px,0.82fr)] lg:py-20">
        {/* LEFT — Atlas-first thesis + its OWN headline numbers */}
        <div className="flex min-w-0 flex-col justify-center">
          <div className="kicker" style={{ color: "var(--primary)" }}>
            Deterministic · LLM-free · local
          </div>
          <h1
            id="hero-title"
            className="mt-4 max-w-2xl text-balance font-semibold"
            style={{ fontSize: "clamp(34px,5vw,56px)", lineHeight: 1.02, letterSpacing: "-0.025em" }}
          >
            Your agent reads a sentence, not the whole file.
          </h1>
          <p className="mt-5 max-w-xl" style={{ fontSize: 15, lineHeight: 1.6, color: "var(--muted)" }}>
            Atlas indexes a repo into a local symbol and call graph, then answers any context query — a symbol&rsquo;s
            definition, callers, callees and imports — in a couple dozen tokens. That precise slice goes to your coding,
            refactoring, debugging or review agent over CLI, MCP or SDK. No whole-file dumps flooding the context window,
            no model in the loop, no code leaving the machine.
          </p>

          {/* PRIMARY — the answer size Atlas hands an agent, the hero's center of
              gravity. Full width, dominant, with the self-referential anchor that
              finally gives the 14–42 figure meaning, plus the measured spread. */}
          <div className="mt-10 min-w-0">
            <div className="kicker">The whole answer · per context query</div>
            <div className="mt-2 flex items-baseline gap-2.5">
              <span
                data-testid="ratio-tokens"
                className="mono tnum"
                aria-label={`${tokMin} to ${tokMax} response tokens per context query`}
                style={{ fontSize: "clamp(46px,7vw,82px)", lineHeight: 0.92, letterSpacing: "-0.035em", fontWeight: 600, color: "var(--primary)" }}
              >
                {tokMin}–{tokMax}
              </span>
              <span className="mono" style={{ fontSize: 16, color: "var(--muted)" }}>tok</span>
            </div>
            <p className="mt-2.5 max-w-lg" style={{ fontSize: 13.5, lineHeight: 1.5, color: "var(--text)" }}>
              One symbol&rsquo;s full neighborhood — defs, callers, callees, imports — in fewer tokens than this sentence.
            </p>
            <div className="mono mt-1.5" style={{ fontSize: 11.5, color: "var(--faint)", letterSpacing: "0.01em" }}>
              median {tokMedian} · {langLabel(tokMinLang)} floor {tokMin} · {langLabel(tokMaxLang)} ceiling {tokMax}
            </div>
          </div>

          {/* SECONDARY — cold index that stands the answers up. Caption derived
              from the row that ACTUALLY owns the fastest index, never hardcoded. */}
          <div className="mt-8 min-w-0">
            <div className="kicker">Cold index · ready to answer</div>
            <div className="mt-2 flex items-baseline gap-2">
              <span
                data-testid="ratio-latency"
                className="mono tnum"
                aria-label={`fastest cold index ${fastestIndex} seconds`}
                style={{ fontSize: "clamp(34px,4.5vw,54px)", lineHeight: 0.95, letterSpacing: "-0.03em", fontWeight: 600, color: "var(--secondary)" }}
              >
                {fastestIndex.toFixed(2)}
              </span>
              <span className="mono" style={{ fontSize: 14, color: "var(--muted)" }}>s</span>
            </div>
            <div className="mono mt-1" style={{ fontSize: 12, color: "var(--faint)" }}>
              fastest cold index · {num(fastestRow?.atlas.metrics.symbols)} symbols, {num(fastestRow?.atlas.metrics.edges)} edges ({langLabel(fastestRow?.language)})
            </div>
          </div>

          {/* ALL-NATIVE breadth — one story, no core/live/detector split. The
              language count and the comparable-row count are kept as two clearly
          distinct denominators, never blurred into one. */}
          <div className="mt-7 hairline" style={{ paddingTop: 18 }}>
            <p className="max-w-xl" style={{ fontSize: 13, lineHeight: 1.55, color: "var(--text)" }}>
              All{" "}
              <span className="num" style={{ color: "var(--success)", fontWeight: 600 }}>{nativeLangCount}</span>{" "}
              Atlas-supported parser families have live fixture evidence — Atlas indexed {supportedAtlasOk}/{supportedFamilies}, with zero hidden regex
              fallback. The public-repo ladder remains {parityCoverage} exactly at parity + {exceedCoverage} above native;{" "}
              <span className="num" style={{ color: "var(--success)", fontWeight: 600 }}>{parityLangs}/{liveCovered}</span>{" "}
              live languages are ≥ ×1.0 across{" "}
              <span className="num" style={{ color: "var(--success)", fontWeight: 600 }}>{cov.deterministicRowsCovered}/{comparableRows}</span>{" "}
              comparable deterministic rows.
            </p>
          </div>

          {/* SUPPORTING stat — clearly scoped to graphify, one of many tools */}
          <div
            className="mt-5 flex flex-wrap items-center gap-x-5 gap-y-2 rounded-lg px-4 py-3"
            style={{ background: "var(--bg2)", border: "1px solid var(--line)" }}
            data-testid="vs-graphify-support"
          >
            <span className="mono" style={{ fontSize: 11, color: "var(--faint)", letterSpacing: "0.08em", textTransform: "uppercase" }}>
              vs graphify
            </span>
            <span style={{ fontSize: 13, color: "var(--muted)" }}>
              the same answer,{" "}
              <span className="num" style={{ color: "var(--primary)", fontWeight: 600 }}>{core.tokenRatio.toFixed(2)}×</span> lighter ·{" "}
              <span className="num" style={{ color: "var(--secondary)", fontWeight: 600 }}>{core.latencyRatio.toFixed(2)}×</span> faster
            </span>
            <span className="mono" style={{ fontSize: 11, color: "var(--faint)" }}>
              1 of {toolCount} tools benchmarked · over {core.equivalentRows} comparable query rows
            </span>
          </div>

          {/* instrument rail of plain mono stat ticks */}
          <div
            className="mt-7 grid grid-cols-2 gap-x-6 gap-y-5 sm:grid-cols-5"
            style={{ borderTop: "1px solid var(--line)", paddingTop: 20 }}
          >
            <StatTick label="Languages" value={`${nativeLangCount} supported`} sub={`${supportedAtlasOk}/${supportedFamilies} Atlas fixture ok`} />
            <StatTick label="Coverage split" value={`${parityCoverage} + ${exceedCoverage}`} sub="parity + exceed in live ladder" />
            <StatTick label="10x target" value={`${liveSummary.tenXComparable}/${liveSummary.withComparableRows}`} sub="token+latency comparable live" />
            <StatTick label="Tools benchmarked" value={toolCount} sub="incl. SCIP / LSP / graphify" />
            <StatTick label="Evidence" value={data.sourceArtifacts.length + derivedCount} sub="raw + derived artifacts" />
          </div>

          <div className="mt-9 flex flex-wrap gap-3">
            <a href="#vs-graphify" className="btn btn-primary focusring" style={{ textDecoration: "none" }}>
              See the proof <ArrowRight className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/benchmark-data.json" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              Download evidence <Download className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/tenx-gap-report.md" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              10x gap report <Download className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/final-benchmark-audit-report.md" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              Final audit <Download className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/raw/SUPPORTED_LANGUAGE_BENCHMARK.json" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              Supported sweep <Download className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/public-repo-validation-manifest.md" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              Validation manifest <Download className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/validation-remeasurement-manifest.md" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              Remeasurement readiness <Download className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/precision-evidence-manifest.md" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              Precision evidence <Download className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/call-edge-evidence-manifest.md" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              Call-edge evidence <Download className="h-4 w-4" aria-hidden />
            </a>
            <a href="data/graphify-support-manifest.md" download data-source-artifact className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
              Graphify support <Download className="h-4 w-4" aria-hidden />
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
        kicker={`One comparison · vs graphify (1 of ${data.provenance.tools.coreCount} tools)`}
        title="Against the closest portable code-graph tool, Atlas returns far less"
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
        graphify is the closest portable code-graph tool to Atlas, so it makes the most honest head-to-head. Native
        SCIP/LSP indexers are compared separately on coverage above. Shown only where both tools answered the same
        query; rows with no graphify equivalent render as a hollow “no equivalent” tick, never a zero.
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

const NATIVE_TOOL_ORDER = [
  "scip-go",
  "scip-python",
  "scip-typescript",
  "scip-java",
  "gopls",
  "pyright",
  "tsc",
  "jdtls",
  "clangd",
  "rust-analyzer",
  "sourcekit-lsp",
  "dotnet",
  "ruby",
  "php",
  "pwsh",
];

function toolStatusColor(tool) {
  if (tool.ok) return "var(--success)";
  if (tool.status === "missing") return "var(--danger)";
  return "var(--warning)";
}

function toolVersionLabel(tool) {
  if (!tool) return "unknown";
  return tool.version || tool.status || "unknown";
}

function buildNativeToolManifest(data) {
  const byName = new Map();
  for (const tool of data.provenance.tools.core || []) {
    if (NATIVE_TOOL_ORDER.includes(tool.tool)) byName.set(tool.tool, tool);
  }
  for (const tool of data.provenance.tools.liveBenchmarkTools || []) {
    if (!byName.has(tool.tool)) byName.set(tool.tool, tool);
  }
  return [...byName.values()].sort((a, b) => {
    const ai = NATIVE_TOOL_ORDER.indexOf(a.tool);
    const bi = NATIVE_TOOL_ORDER.indexOf(b.tool);
    if (ai !== -1 || bi !== -1) return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi);
    return a.tool.localeCompare(b.tool);
  });
}

/* The native-parity LADDER — ONE unified visual.
   A single horizontal coverage-ratio axis (×1.0 → ×1.84). The ×1.0 spine is
   the native-parity reference: nothing in the data falls below it. Two honest
   treatments share that one axis:
     · PARITY COLUMN — the 29 languages that sit EXACTLY at ×1.0 are not faked
       into 29 identical bars (that was the old scatter's mush). They are a
       single counted stack pinned on the spine; click/Enter expands the roster
       of chips so the cluster is inspectable, never a black box.
     · STANDOUTS — the 7 languages that EXCEED native render as real bars
       growing rightward from the spine, ordered by ratio, the bar's THICKNESS
       (and the trailing dot's area) encoding defs indexed. Raw Atlas-vs-native
       defs surface on hover/focus + inline.
   Detector-only languages (ejs/ets/r) live in the parity column flagged with a
   hollow ▱ glyph — present, reachable, never counted as a coverage "win".
   No graphify anywhere on the axes — coverage is Atlas defs ÷ native defs. */

const DETECTOR_LANGS = new Set(["ejs", "ets", "r"]);

function buildParityModel(data) {
  const rows = data.liveBenchmarks
    .filter((r) => r.coverage && typeof r.coverage.ratio === "number")
    .map((r) => ({
      lang: r.language,
      ratio: r.coverage.ratio,
      atlasDefs: r.coverage.atlasDefinitions,
      nativeDefs: r.coverage.nativeDefinitions,
      tool: r.native?.tool || "—",
      detector: DETECTOR_LANGS.has(r.language) || !!r.detectorOnly,
      artifact: r.artifact,
    }));
  const atParity = rows
    .filter((r) => r.ratio <= 1.0)
    .sort((a, b) => b.atlasDefs - a.atlasDefs);
  const standouts = rows
    .filter((r) => r.ratio > 1.0)
    .sort((a, b) => b.ratio - a.ratio);
  const maxRatio = Math.max(1.0, ...rows.map((r) => r.ratio));
  const maxDefs = Math.max(1, ...standouts.map((r) => r.atlasDefs));
  const minRatio = Math.min(...rows.map((r) => r.ratio));
  return { rows, atParity, standouts, maxRatio, maxDefs, minRatio };
}

function NativeParityLadder({ data }) {
  const reduced = usePrefersReducedMotion();
  const [ref, inView] = useInView({ threshold: 0.18 });
  const animate = !reduced && inView;
  const [expanded, setExpanded] = useState(false);
  const [active, setActive] = useState(null); // hovered/focused standout lang

  const model = useMemo(() => buildParityModel(data), [data]);
  const { atParity, standouts, maxRatio, maxDefs, minRatio } = model;
  const liveTotal = atParity.length + standouts.length;
  const coreTotal = data.summary.core.languages;
  const publicRepoTotal = coreTotal + liveTotal;
  const supportedTotal = data.summary.supported?.families ?? publicRepoTotal;

  // ---- one shared horizontal ratio scale, used by BOTH zones --------------
  // Domain starts a hair below 1.0 so the spine has air to its left; it ends a
  // ROUND tick ABOVE the top standout (ceil to the next 0.2 step) so the
  // longest bar never reaches the axis frame and its ×ratio + raw-defs label
  // always has a gutter to live in. Ticks are honest ratio marks, never
  // graphify. For data topping at ×1.84 this yields a ×2.0 domain.
  const DOM_MIN = 0.96;
  const DOM_MAX = Math.max(1.2, Math.ceil(maxRatio * 5 + 0.001) / 5); // ceil → next .2
  const AXIS_LEFT = 21; // % — where ×1.0 spine sits (parity column to its left)
  const AXIS_RIGHT = 97; // %
  const ratioToPct = (r) =>
    AXIS_LEFT + ((Math.max(DOM_MIN, r) - 1.0) / (DOM_MAX - 1.0)) * (AXIS_RIGHT - AXIS_LEFT);
  // Ladder-x (full width) → standouts-zone-x. The zone is the right grid cell
  // spanning ladder-x AXIS_LEFT→100, so bars, gridlines and labels inside it
  // share ONE coordinate space and stay aligned with the axis ticks above.
  const ZONE_SPAN = 100 - AXIS_LEFT;
  const ladderToZone = (p) => ((p - AXIS_LEFT) / ZONE_SPAN) * 100;
  const ticks = [1.0, 1.2, 1.4, 1.6, 1.8, 2.0].filter((t) => t <= DOM_MAX + 0.001);
  const defsToThickness = (d) => 8 + Math.sqrt(d / maxDefs) * 16; // 8..24px bar

  const detectorCount = atParity.filter((r) => r.detector).length;

  return (
    <div ref={ref} data-testid="parity-ladder" className="min-w-0 overflow-hidden">
      {/* TRUTH STRIP — the headline THIS chart proves, in plain numerals. The
          ladder is a per-LANGUAGE visual (parity column + standout bars), so the
          headline counts languages; the deterministic-row figure is reported
          separately in the hero to avoid mixing denominators. */}
      <div className="mb-5 flex min-w-0 flex-wrap items-center gap-x-6 gap-y-2">
        <div className="flex min-w-0 flex-wrap items-baseline gap-2">
          <span className="num" style={{ fontSize: 26, fontWeight: 600, color: "var(--success)" }}>
            {atParity.length + standouts.length}/{atParity.length + standouts.length}
          </span>
          <span style={{ fontSize: 13, color: "var(--muted)" }}>live languages at or above native parity</span>
        </div>
        <span className="mono min-w-0" style={{ fontSize: 12, color: "var(--faint)", maxWidth: "100%", overflowWrap: "anywhere" }}>
          {atParity.length} at parity · {standouts.length} exceed · none below ×1.0 · {data.summary.coverage.detectorOnlyRowsCovered} detector-only
        </span>
        <span className="mono min-w-0" style={{ fontSize: 12, color: "var(--primary)", maxWidth: "100%", overflowWrap: "anywhere" }}>
          {supportedTotal} supported families · {publicRepoTotal} public-repo code surfaces
        </span>
      </div>

      {/* ===================== THE LADDER ===================== */}
      <div className="relative">
        {/* axis ticks header */}
        <div className="relative mb-2" style={{ height: 16 }} aria-hidden>
          {ticks.map((t) => (
            <span
              key={t}
              className="mono absolute"
              style={{ left: `${ratioToPct(t)}%`, transform: "translateX(-50%)", fontSize: 10.5, color: t === 1.0 ? "var(--warning)" : "var(--faint)" }}
            >
              ×{t.toFixed(1)}
            </span>
          ))}
          <span
            className="kicker absolute"
            style={{ left: 0, top: 0, fontSize: 10, color: "var(--faint)" }}
          >
            at parity
          </span>
        </div>

        {/* the ×1.0 parity spine, spanning both zones */}
        <div
          className="pointer-events-none absolute"
          style={{ left: `${AXIS_LEFT}%`, top: 18, bottom: 26, width: 2, background: "var(--warning)", opacity: 0.85, boxShadow: "0 0 10px rgba(242,180,58,0.35)" }}
          aria-hidden
        />

        <div className="grid items-stretch gap-0" style={{ gridTemplateColumns: `${AXIS_LEFT}% 1fr` }}>
          {/* -------- ZONE A: PARITY COLUMN (the 29 exactly-at-1.0) -------- */}
          <button
            type="button"
            data-testid="parity-column"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            aria-label={`${atParity.length} of ${liveTotal} live languages exactly at native coverage parity. ${expanded ? "Collapse" : "Expand"} the roster.`}
            className="focusring relative flex flex-col items-center justify-end pr-3 text-center"
            style={{ background: "transparent", border: "none", cursor: "pointer", minHeight: 200 }}
          >
            <div
              className="relative flex w-full max-w-[150px] flex-col items-center justify-end overflow-hidden rounded-t-md"
              style={{
                height: animate ? 168 : 0,
                background: "linear-gradient(180deg, rgba(82,217,139,0.16), rgba(82,217,139,0.05))",
                border: "1px solid var(--success)",
                borderBottom: "none",
                transition: reduced ? "none" : "height 620ms cubic-bezier(0.22,1,0.36,1)",
              }}
            >
              <span className="num" style={{ fontSize: 34, fontWeight: 600, color: "var(--success)", lineHeight: 1 }}>
                {atParity.length}/{liveTotal}
              </span>
              <span className="mono mt-1" style={{ fontSize: 10.5, color: "var(--muted)" }}>
                of {liveTotal} live ladder
              </span>
              <span className="mono mt-3 mb-2" style={{ fontSize: 10, color: "var(--faint)" }}>
                {expanded ? "hide roster ▴" : "show roster ▾"}
              </span>
            </div>
            <span className="num mt-2" style={{ fontSize: 12, color: "var(--warning)" }}>×1.00</span>
            <span className="mono" style={{ fontSize: 10, color: "var(--faint)" }}>coverage parity</span>
          </button>

          {/* -------- ZONE B: STANDOUTS LADDER (the 7 above parity) -------- */}
          <div className="relative" role="list" aria-label="Languages that exceed native definition coverage">
            {/* faint ratio gridlines behind the bars */}
            {ticks.filter((t) => t > 1.0).map((t) => (
              <div
                key={t}
                className="pointer-events-none absolute"
                style={{ left: `${ladderToZone(ratioToPct(t))}%`, top: 0, bottom: 26, width: 1, background: "var(--grid)" }}
                aria-hidden
              />
            ))}
            <div className="flex flex-col justify-end gap-2.5 pl-3" style={{ minHeight: 200 }}>
              {standouts.map((r, i) => {
                // zone-relative bar end (0–100 within the standouts column)
                const endZone = ladderToZone(ratioToPct(r.ratio));
                const widthPct = endZone;
                const th = defsToThickness(r.atlasDefs);
                const isActive = active === r.lang;
                // If the bar end leaves too little zone width for the outside
                // ×ratio + defs label (which is right-anchored in the gutter),
                // flip the label INSIDE the bar end so it is never clipped by the
                // frame and never collides with the bar. The top ×1.84 bar ends
                // ~81% of zone → inside; everything shorter labels outside.
                const labelInside = endZone > 72;
                // Short bars can't contain their own name in dark ink — render
                // the name in light ink so it stays legible spilling onto the bg.
                const nameLight = widthPct < 18;
                return (
                  <div
                    key={r.lang}
                    role="listitem"
                    className="relative"
                    style={{ height: Math.max(th, 22) }}
                    onMouseEnter={() => setActive(r.lang)}
                    onMouseLeave={() => setActive((a) => (a === r.lang ? null : a))}
                  >
                    {/* the bar grows rightward from the spine */}
                    <div
                      tabIndex={0}
                      role="img"
                      data-testid={`standout-${r.lang}`}
                      aria-label={`${langLabel(r.lang)} ${r.ratio.toFixed(2)} times native coverage — Atlas ${num(r.atlasDefs)} definitions versus ${num(r.nativeDefs)} from ${r.tool}`}
                      onFocus={() => setActive(r.lang)}
                      onBlur={() => setActive((a) => (a === r.lang ? null : a))}
                      className="focusring absolute flex items-center"
                      style={{
                        left: 0,
                        top: "50%",
                        transform: "translateY(-50%)",
                        height: th,
                        width: `${animate ? widthPct : 0}%`,
                        background: isActive
                          ? "linear-gradient(90deg, var(--secondary), #9db8ff)"
                          : "linear-gradient(90deg, rgba(122,162,255,0.85), rgba(122,162,255,0.45))",
                        borderRadius: "0 4px 4px 0",
                        transition: reduced ? "none" : `width 640ms cubic-bezier(0.22,1,0.36,1) ${i * 60}ms, background 160ms`,
                        outlineOffset: 3,
                      }}
                    >
                      {/* trailing dot — area also encodes defs, reinforcing thickness */}
                      <span
                        className="absolute"
                        style={{
                          right: -5,
                          top: "50%",
                          width: 10,
                          height: 10,
                          marginTop: -5,
                          borderRadius: "50%",
                          background: "var(--secondary)",
                          boxShadow: "0 0 8px rgba(122,162,255,0.55)",
                        }}
                        aria-hidden
                      />
                    </div>
                    {/* language label, anchored inside-left of the bar */}
                    <span
                      className="num pointer-events-none absolute"
                      style={{ left: 10, top: "50%", transform: "translateY(-50%)", fontSize: 12.5, color: nameLight ? "var(--text)" : "#061021", fontWeight: 600 }}
                    >
                      {langLabel(r.lang)}
                    </span>
                    {/* ratio + raw defs. Outside labels are RIGHT-ANCHORED to
                        the zone edge (right:0), so they always sit in the gutter
                        past the bar end and can never overrun the axis frame at
                        any width or bar length. On the rare bar long enough that
                        the gutter would collide with the bar end, the label
                        flips INSIDE the bar end instead. */}
                    <span
                      className="num pointer-events-none absolute whitespace-nowrap"
                      style={
                        labelInside
                          ? { left: `calc(${animate ? endZone : 0}% - 12px)`, top: "50%", transform: "translate(-100%,-50%)", fontSize: 12, color: "#061021", fontWeight: 600, transition: reduced ? "none" : `left 640ms cubic-bezier(0.22,1,0.36,1) ${i * 60}ms` }
                          : { right: 0, top: "50%", transform: "translateY(-50%)", fontSize: 12, color: "var(--text)", textAlign: "right" }
                      }
                    >
                      <span style={{ color: labelInside ? "#061021" : "var(--secondary)", fontWeight: 600 }}>×{r.ratio.toFixed(2)}</span>
                      <span className="hidden sm:inline" style={{ color: labelInside ? "rgba(6,16,33,0.7)" : "var(--faint)", marginLeft: 8, fontSize: 11 }}>
                        {num(r.atlasDefs)}/{num(r.nativeDefs)} defs
                      </span>
                    </span>
                  </div>
                );
              })}
            </div>
            {/* exceeds-native caption pinned to the standouts zone */}
            <div className="mono mt-2 pl-3" style={{ fontSize: 10.5, color: "var(--faint)" }}>
              exceeds native — bar thickness ∝ definitions indexed
            </div>
          </div>
        </div>

        {/* axis baseline + label */}
        <div className="relative mt-1" style={{ height: 14 }} aria-hidden>
          <div className="absolute" style={{ left: `${AXIS_LEFT}%`, right: `${100 - AXIS_RIGHT}%`, top: 0, height: 1, background: "var(--line)" }} />
        </div>
        <div className="mono mt-1 text-center" style={{ fontSize: 10.5, color: "var(--muted)" }}>
          coverage ratio — Atlas definitions ÷ native definitions (range ×{minRatio.toFixed(2)}–×{maxRatio.toFixed(2)})
        </div>
      </div>

      {/* expandable roster of the at-parity cluster — honest inspectability */}
      <div
        data-testid="parity-roster"
        className="grid overflow-hidden transition-all"
        style={{ gridTemplateRows: expanded ? "1fr" : "0fr", transition: reduced ? "none" : "grid-template-rows 360ms ease" }}
      >
        <div className="min-h-0">
          <div className="mt-5 rounded-lg p-4" style={{ background: "var(--bg2)", border: "1px solid var(--line)" }}>
            <div className="kicker mb-3">
              {atParity.length} languages exactly at parity · {detectorCount} detector-only
            </div>
            <ul className="flex flex-wrap gap-2" aria-label="Languages at native parity">
              {atParity.map((r) => (
                <li key={r.lang}>
                  <span
                    className="mono inline-flex items-center gap-1.5 rounded-md px-2 py-1"
                    title={`${langLabel(r.lang)} · ${num(r.atlasDefs)} Atlas defs vs ${num(r.nativeDefs)} ${r.tool} · ×${r.ratio.toFixed(2)}`}
                    style={{
                      fontSize: 11.5,
                      border: `1px solid ${r.detector ? "var(--not-comparable)" : "var(--line-strong)"}`,
                      background: "var(--surface)",
                      color: r.detector ? "var(--not-comparable)" : "var(--text)",
                    }}
                  >
                    {r.detector && <span aria-hidden style={{ fontSize: 11 }}>▱</span>}
                    {langLabel(r.lang)}
                    <span style={{ color: "var(--faint)" }}>{num(r.atlasDefs)}</span>
                  </span>
                </li>
              ))}
            </ul>
            <p className="mt-3" style={{ fontSize: 11.5, lineHeight: 1.5, color: "var(--faint)" }}>
              ▱ detector-only languages (ejs · ets · r) are reached through a scriptable source-counter proxy and shown
              here for completeness — they are never counted as a coverage win.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}

function TenXTargetReadout({ data }) {
  const m = tenXModel(data);
  return (
    <div
      data-testid="tenx-target"
      className="mb-7 grid gap-3 rounded-lg px-5 py-4 sm:grid-cols-3"
      style={{ border: "1px solid var(--line-strong)", background: "var(--bg2)" }}
      aria-label="10x target progress for at-parity live languages"
    >
      <div className="sm:col-span-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="kicker">10x target tracker</div>
          <SourceLink href="data/tenx-gap-report.md" download>
            <Download className="h-3 w-3" aria-hidden /> tenx-gap-report.md
          </SourceLink>
        </div>
        <p className="mt-2 max-w-3xl" style={{ fontSize: 13, lineHeight: 1.5, color: "var(--muted)" }}>
          Goal: move the {m.parityTotal} live languages that are exactly at native coverage parity into the exceed
          column, while also proving at least 10x token and latency ratios on comparable rows. Current data keeps the
          axes separate: coverage proves parity/exceed, while token and latency carry the 10x ratio target.
        </p>
      </div>
      <StatTick
        label="Coverage exceed"
        value={`${m.parityCoverageExceed}/${m.parityTotal}`}
        sub={`${m.liveCoverageExceed}/${m.liveTotal} live already > native`}
      />
      <StatTick
        label="Token ≥10x"
        value={`${m.parityToken10}/${m.parityComparable}`}
        sub={`${m.liveToken10}/${m.liveComparable} comparable live rows`}
      />
      <StatTick
        label="Latency ≥10x"
        value={`${m.parityLatency10}/${m.parityComparable}`}
        sub={`${m.parityNonComparable} parity languages not comparable`}
      />
      <div className="sm:col-span-3">
        <p className="mono" style={{ fontSize: 12, lineHeight: 1.5, color: "var(--warning)" }}>
          Strict token+latency 10x now: {m.livePerformance10}/{m.liveComparable} comparable live languages. Accuracy is
          tracked as definition coverage versus native, so the honest target is moving {m.parityTotal} parity languages
          above ×1.0 native coverage, not inventing a 10x accuracy ratio from a bounded denominator.
        </p>
      </div>
    </div>
  );
}

function VsNative({ data }) {
  // The ladder's universe is the LIVE public-repo rows with native/proxy
  // coverage ratios. The supported-family fixture sweep is broader and lives in
  // data/raw/SUPPORTED_LANGUAGE_BENCHMARK.json.
  const liveLangs = data.summary.live.artifacts;
  const supportedLangs = data.summary.supported?.families ?? liveLangs;
  const toolManifest = useMemo(() => buildNativeToolManifest(data), [data]);
  return (
    <section id="vs-native" data-testid="vs-native" className="shell py-16" aria-labelledby="vsn-title">
      <SectionHeader
        id="vsn-title"
        kicker="Coverage · alongside native SCIP / LSP"
        title="Atlas meets or beats native definition coverage, language by language"
      >
        Coverage is Atlas definitions ÷ the best native indexer for each public-repo row — no graphify on this axis.
        The ×1.0 spine is native parity: each of the {liveLangs} live languages here sits on it or to its right. The
        broader supported-family sweep covers {supportedLangs} Atlas parser families in the raw evidence bundle.
      </SectionHeader>

      {/* peer-framed standfirst — native indexers are the bar Atlas stands with */}
      <div
        data-testid="native-callout"
        className="mb-7 rounded-lg px-5 py-4"
        style={{ border: "1px solid var(--line-strong)", background: "var(--surface)" }}
      >
        <p className="text-balance" style={{ fontSize: 15, lineHeight: 1.5, color: "var(--text)" }}>
          <span style={{ color: "var(--secondary)", fontWeight: 600 }}>Native indexers are the peer bar</span> — SCIP and
          LSP servers define ground truth for definition coverage; this is shown on its own coverage axis and is never
          averaged into any efficiency ratio.
        </p>
      </div>

      <TenXTargetReadout data={data} />

      <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1.55fr)_minmax(260px,0.9fr)]">
        <div className="panel min-w-0 p-5 sm:p-6">
          <NativeParityLadder data={data} />
        </div>
        <div className="panel min-w-0 p-5 sm:p-6">
          <div className="kicker">Native baselines</div>
          <ul className="mt-4 flex min-w-0 flex-col gap-2.5" aria-label="Native tool manifest">
            {toolManifest.map((tool) => (
              <li
                key={tool.tool}
                className="grid min-w-0 items-center gap-3"
                style={{ gridTemplateColumns: "minmax(0,0.72fr) minmax(0,1.28fr) 6px" }}
              >
                <span className="num truncate" style={{ fontSize: 12.5, color: "var(--text)" }}>
                  {tool.tool}
                </span>
                <span
                  className="num tnum min-w-0"
                  style={{ fontSize: 12, color: "var(--muted)", textAlign: "right", fontVariantNumeric: "tabular-nums", overflowWrap: "anywhere" }}
                  title={tool.note || tool.command || toolVersionLabel(tool)}
                >
                  {toolVersionLabel(tool)}
                </span>
                <span
                  style={{ width: 6, height: 6, borderRadius: "50%", background: toolStatusColor(tool) }}
                  aria-label={tool.status || (tool.ok ? "ok" : "unknown")}
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
  const saturatedRows = data.saturation || [];
  const saturatedNames = saturatedRows.map((row) => langLabel(row.language)).join(", ");
  return (
    <section id="not-comparable" data-testid="not-comparable" className="shell py-16" aria-labelledby="nc-title">
      <SectionHeader
        id="nc-title"
        kicker="Honesty · the edge of our scope"
        title={saturatedRows.length ? "Where Atlas doesn’t claim an efficiency win" : "No current zero-equivalent Graphify rows"}
      >
        {saturatedRows.length
          ? `${saturatedNames} held native coverage at or above 1.0 across repeated iterations but produced no comparable query rows, so no token or latency ratio is claimed.`
          : "The final benchmark pass has at least one Graphify-equivalent query row for every live language. Historical no-equivalent saturation rows are not folded into current headline ratios."}
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
        {saturatedRows.length === 0 && (
          <div className="panel p-5 lg:col-span-3" style={{ borderColor: "rgba(82,217,139,0.35)" }}>
            <div className="kicker" style={{ color: "var(--success)" }}>current final pass</div>
            <p className="mt-3" style={{ fontSize: 13, lineHeight: 1.5, color: "var(--muted)" }}>
              `SATURATION_REPORT.json` is still published for auditability, but it now records no active zero-equivalent
              saturation rows. Detector-only and source-counter proxy caveats remain in the final audit report.
            </p>
            <div className="mt-4">
              <SourceLink href="data/raw/SATURATION_REPORT.json" download>
                <Download className="h-3 w-3" aria-hidden /> SATURATION_REPORT.json
              </SourceLink>
            </div>
          </div>
        )}
        {saturatedRows.map((row) => (
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
    ["MCP", "dev or agent"],
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
            title="Retrieve code context"
            line={`atlas context --paths path/to/changed-file.go --query "review risk" --format json`}
            copy="Returns a compact context bundle around the changed files for any developer or agent — coding, refactoring, debugging, review."
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
            <SourceLink href="data/tenx-gap-report.md" download>
              <Download className="h-3 w-3" aria-hidden /> tenx-gap-report.md
            </SourceLink>
            <SourceLink href="data/final-benchmark-audit-report.md" download>
              <Download className="h-3 w-3" aria-hidden /> final audit report
            </SourceLink>
            <SourceLink href="data/public-repo-validation-manifest.md" download>
              <Download className="h-3 w-3" aria-hidden /> validation manifest
            </SourceLink>
            <SourceLink href="data/validation-remeasurement-manifest.md" download>
              <Download className="h-3 w-3" aria-hidden /> remeasurement readiness
            </SourceLink>
            <SourceLink href="data/precision-evidence-manifest.md" download>
              <Download className="h-3 w-3" aria-hidden /> precision evidence
            </SourceLink>
            <SourceLink href="data/call-edge-evidence-manifest.md" download>
              <Download className="h-3 w-3" aria-hidden /> call-edge evidence
            </SourceLink>
            <SourceLink href="data/graphify-support-manifest.md" download>
              <Download className="h-3 w-3" aria-hidden /> graphify support
            </SourceLink>
          </div>
        </div>
        <div className="mt-4">
          <TermBlock
            lines={[
              "curl -LO https://aziron-ai.github.io/atlas/data/benchmark-data.json",
              "curl -LO https://aziron-ai.github.io/atlas/data/raw/MATRIX_REPORT.json",
              "curl -LO https://aziron-ai.github.io/atlas/data/tenx-gap-report.md",
              "curl -LO https://aziron-ai.github.io/atlas/data/final-benchmark-audit-report.md",
              "curl -LO https://aziron-ai.github.io/atlas/data/public-repo-validation-manifest.md",
              "curl -LO https://aziron-ai.github.io/atlas/data/validation-remeasurement-manifest.md",
              "curl -LO https://aziron-ai.github.io/atlas/data/precision-evidence-manifest.md",
              "curl -LO https://aziron-ai.github.io/atlas/data/call-edge-evidence-manifest.md",
              "curl -LO https://aziron-ai.github.io/atlas/data/graphify-support-manifest.md",
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
    const wanted = [
      "atlas",
      "graphify",
      "scip-go",
      "scip-python",
      "scip-typescript",
      "scip-java",
      "gopls",
      "pyright",
      "tsc",
      "jdtls",
      "clangd",
      "rust-analyzer",
      "sourcekit-lsp",
    ];
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
        <a href="data/tenx-gap-report.md" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> tenx-gap-report.md
        </a>
        <a href="data/final-benchmark-audit-report.md" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> final audit report
        </a>
        <a href="data/public-repo-validation-manifest.md" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> validation manifest
        </a>
        <a href="data/validation-remeasurement-manifest.md" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> remeasurement readiness
        </a>
        <a href="data/precision-evidence-manifest.md" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> precision evidence
        </a>
        <a href="data/call-edge-evidence-manifest.md" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> call-edge evidence
        </a>
        <a href="data/graphify-support-manifest.md" download data-source-artifact data-testid="download-link" className="btn btn-ghost focusring" style={{ textDecoration: "none" }}>
          <Download className="h-4 w-4" aria-hidden /> graphify support
        </a>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        {/* LEFT — all source artifacts */}
        <div className="panel min-w-0 p-5">
          <div className="kicker mb-4">Source artifacts · {data.sourceArtifacts.length}</div>
          <div className="tablewrap" style={{ maxHeight: 480, overflowY: "auto" }}>
            <table className="dtable dtable-compact">
              <colgroup>
                <col style={{ width: "46%" }} />
                <col style={{ width: "17%" }} />
                <col style={{ width: "24%" }} />
                <col style={{ width: "13%" }} />
              </colgroup>
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
                <span className="truncate" style={{ color: t.ok ? "var(--text)" : "var(--danger)", maxWidth: 180, textAlign: "right" }} title={t.note || t.version || t.status}>
                  {toolVersionLabel(t)}
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
  const active = useScrollSpy(["vs-native", "vs-graphify", "graph", "matrix", "install"]);

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
        <VsNative data={data} />
        <VsGraphify data={data} />
        <NotComparable data={data} />
        <GraphSection data={enriched} />
        <SignalPath data={enriched} />
        <LanguagesExplorer data={data} />
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
