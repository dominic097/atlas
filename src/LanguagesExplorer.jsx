import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowUpDown,
  BarChart3,
  ChevronRight,
  Download,
  ExternalLink,
  FileJson,
  Search,
  Table2,
  X,
} from "lucide-react";

/* ============================================================
   Languages Explorer — ONE unified surface over Atlas's supported
   parser families plus stronger public-repo evidence rows. Two honest lenses
   (Graphical | Table) share a single derived, filtered, sorted
   row set, so toggling the view is a lens change, never a
   context switch. Atlas-centric default (Coverage parity), with
   graphify token/latency ratios available but explicitly scoped
   "vs graphify". Null is never 0; not-comparable is never folded
   into a win.
   ============================================================ */

const fmt = new Intl.NumberFormat("en-US");
const DETECTOR_LANGS = new Set(["ejs", "ets", "r"]);

function num(v) {
  if (v === null || v === undefined || Number.isNaN(Number(v))) return "—";
  return fmt.format(v);
}
function ratioStr(v) {
  if (v === null || v === undefined || Number.isNaN(Number(v))) return "not comparable";
  return `${Number(v).toFixed(2)}×`;
}
function secsStr(v) {
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
function repoShort(repo) {
  return repo ? repo.replace("https://github.com/", "") : "—";
}

/* -------- ONE flat row model, derived once -------------------- */
function buildAllRows(data) {
  const core = data.coreMatrix.map((r) => {
    const m = r.atlas.metrics || {};
    const qs = r.querySummary || {};
    const artifactPath = r.artifact || "data/raw/MATRIX_REPORT.json";
    return {
      language: r.language,
      label: langLabel(r.language),
      tier: "core",
      repo: r.repo,
      repoShort: repoShort(r.repo),
      commit: null,
      indexSeconds: m.cold_seconds ?? r.atlas.seconds ?? null,
      symbols: m.symbols ?? null,
      edges: m.edges ?? null,
      tokenRatio: qs.tokenRatio ?? null,
      latencyRatio: qs.latencyRatio ?? null,
      coverageRatio: null, // core has no native baseline
      atlasDefs: null,
      nativeDefs: null,
      nativeTool: "graphify + SCIP/LSP",
      equivalentRows: qs.equivalentRows ?? 0,
      rows: qs.rows ?? 0,
      detectorOnly: false,
      atlasTokens: qs.atlasTokens ?? null,
      graphifyTokens: qs.graphifyTokens ?? null,
      artifactPath,
      artifactName: artifactPath.split("/").pop(),
    };
  });

  const live = data.liveBenchmarks.map((r) => {
    const idx = r.atlas?.index || {};
    const qs = r.querySummary || {};
    const cov = r.coverage || {};
    const artifactPath = r.artifact || "data/raw/MATRIX_REPORT.json";
    const detectorOnly = !!r.detectorOnly || DETECTOR_LANGS.has(r.language);
    return {
      language: r.language,
      label: langLabel(r.language),
      tier: "live",
      repo: r.repo,
      repoShort: repoShort(r.repo),
      commit: r.commit || null,
      indexSeconds: r.atlas?.coldSeconds ?? null,
      symbols: idx.symbols ?? null,
      edges: idx.edges ?? null,
      tokenRatio: qs.tokenRatio ?? null,
      latencyRatio: qs.latencyRatio ?? null,
      coverageRatio: typeof cov.ratio === "number" ? cov.ratio : null,
      atlasDefs: cov.atlasDefinitions ?? null,
      nativeDefs: cov.nativeDefinitions ?? null,
      nativeTool: r.native?.tool || "—",
      equivalentRows: qs.equivalentRows ?? 0,
      rows: qs.rows ?? 0,
      detectorOnly,
      atlasTokens: qs.atlasTokens ?? null,
      graphifyTokens: qs.graphifyTokens ?? null,
      artifactPath,
      artifactName: artifactPath.split("/").pop(),
    };
  });

  const seen = new Set([...core, ...live].map((row) => row.language));
  const supported = (data.supportedLanguageBenchmark?.rows || [])
    .filter((r) => !seen.has(r.language))
    .map((r) => {
      const metrics = r.atlas?.metrics || {};
      const artifactPath = r.artifact || "data/raw/SUPPORTED_LANGUAGE_BENCHMARK.json";
      return {
        language: r.language,
        label: langLabel(r.language),
        tier: "supported",
        category: r.category,
        repo: "fixture sweep",
        repoShort: r.fixturePath || "fixture",
        commit: null,
        indexSeconds: r.atlas?.seconds ?? null,
        symbols: metrics.symbols ?? null,
        edges: metrics.edges ?? null,
        tokenRatio: null,
        latencyRatio: null,
        coverageRatio: null,
        atlasDefs: metrics.symbols ?? null,
        nativeDefs: null,
        nativeTool: r.native?.tool || r.native?.status || "fixture oracle",
        equivalentRows: 0,
        rows: 0,
        detectorOnly: r.graphify?.support?.support === "detector_only",
        atlasTokens: null,
        graphifyTokens: null,
        artifactPath,
        artifactName: artifactPath.split("/").pop(),
        oracleRecall: r.oracle?.recall ?? null,
        oraclePrecision: r.oracle?.precision ?? null,
      };
    });

  return [...core, ...live, ...supported].map((row) => {
    // STATUS derivation — order matters (verified against the JSON):
    // null token (0/N equivalent) is the stronger truth → not-comparable.
    const status =
      row.tokenRatio == null
        ? "not-comparable"
        : row.detectorOnly
        ? "detector-only"
        : "comparable";
    const exceedsNative =
      row.coverageRatio != null && row.coverageRatio > 1.0001;
    return { ...row, status, exceedsNative };
  });
}

/* metric → row accessor + label + axis caption */
const METRICS = {
  coverage: {
    label: "Coverage parity",
    accessor: (r) => r.coverageRatio,
    axis: "Atlas defs ÷ native defs",
    suffix: "×",
    scoped: false,
  },
  symbols: {
    label: "Symbols",
    accessor: (r) => r.symbols,
    axis: "symbols indexed",
    suffix: "",
    scoped: false,
  },
  index: {
    label: "Index time",
    accessor: (r) => r.indexSeconds,
    axis: "cold index seconds (lower is faster)",
    suffix: "s",
    scoped: false,
  },
  tokens: {
    label: "Token×",
    accessor: (r) => r.tokenRatio,
    axis: "graphify tokens ÷ Atlas tokens",
    suffix: "×",
    scoped: true,
  },
  latency: {
    label: "Latency×",
    accessor: (r) => r.latencyRatio,
    axis: "graphify ms ÷ Atlas ms",
    suffix: "×",
    scoped: true,
  },
};

const SORT_ACCESSORS = {
  language: (r) => r.label.toLowerCase(),
  coverage: (r) => r.coverageRatio,
  symbols: (r) => r.symbols,
  edges: (r) => r.edges,
  index: (r) => r.indexSeconds,
  tokens: (r) => r.tokenRatio,
  latency: (r) => r.latencyRatio,
};

const METRIC_DEFAULT_SORT = {
  coverage: "coverage",
  symbols: "symbols",
  index: "index",
  tokens: "tokens",
  latency: "latency",
};

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

/* status pill */
function StatusPill({ status }) {
  const map = {
    comparable: ["comparable", "var(--success)"],
    "detector-only": ["detector-only", "var(--warning)"],
    "not-comparable": ["not comparable", "var(--not-comparable)"],
  };
  const [text, color] = map[status] || map["not-comparable"];
  return (
    <span
      className="mono inline-flex items-center rounded-full px-2 py-0.5"
      style={{
        fontSize: 10.5,
        fontWeight: 600,
        color,
        border: `1px solid ${color}`,
        background: "transparent",
        whiteSpace: "nowrap",
      }}
    >
      {text}
    </span>
  );
}

/* explicit not-comparable metric cell — an em-dash glyph + a muted label, so
   a null ratio reads unambiguously as "no number here", never as a placeholder
   like xxx. Used wherever a token/latency ratio is absent. */
function NotComparableCell() {
  return (
    <span className="inline-flex items-baseline gap-1.5 whitespace-nowrap">
      <span className="num" aria-hidden style={{ color: "var(--faint)", fontSize: 13 }}>—</span>
      <span className="mono" style={{ fontSize: 10.5, color: "var(--not-comparable)" }}>not comparable</span>
    </span>
  );
}

/* tiny inline ratio bar behind a table number */
function RatioBar({ value, max, color }) {
  const w = value == null ? 0 : Math.max(0, Math.min(100, (value / max) * 100));
  return (
    <div className="mt-1 h-1 overflow-hidden rounded-full" style={{ background: "var(--line)" }}>
      <div className="h-full rounded-full" style={{ width: `${w}%`, background: color }} />
    </div>
  );
}

/* ============================ CONTROLS ================================== */

const CHIPS = [
  ["all", "All"],
  ["comparable", "Comparable"],
  ["not-comparable", "Not comparable"],
  ["detector-only", "Detector-only"],
  ["exceeds-native", "Exceeds native"],
  ["supported", "Supported-only"],
  ["core", "Matrix"],
  ["live", "Live"],
];

function ExplorerControls({
  query,
  onQuery,
  searchRef,
  filter,
  onFilter,
  counts,
  sort,
  onSort,
  dir,
  onDir,
  view,
  onView,
}) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        {/* LEFT cluster — search + chips */}
        <div className="flex min-w-0 flex-1 flex-col gap-3 sm:flex-row sm:items-center">
          <div className="relative min-w-0 flex-1">
            <Search
              className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2"
              style={{ color: "var(--faint)" }}
              aria-hidden
            />
            <input
              ref={searchRef}
              data-testid="lx-search"
              className="field focusring pl-9"
              type="search"
              placeholder="Search language, repo, commit, tool…  (press /)"
              aria-label="Search languages"
              value={query}
              onChange={(e) => onQuery(e.target.value)}
            />
          </div>
        </div>
        {/* RIGHT cluster — sort + view toggle */}
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1.5">
            <select
              data-testid="lx-sort"
              className="field focusring"
              style={{ width: "auto" }}
              aria-label="Sort languages"
              value={sort}
              onChange={(e) => onSort(e.target.value)}
            >
              <option value="language">Sort: language</option>
              <option value="coverage">Sort: coverage</option>
              <option value="symbols">Sort: symbols</option>
              <option value="edges">Sort: edges</option>
              <option value="index">Sort: index time</option>
              <option value="tokens">Sort: token×</option>
              <option value="latency">Sort: latency×</option>
            </select>
            <button
              type="button"
              data-testid="lx-sort-dir"
              className="focusring chip"
              aria-label="Toggle sort direction"
              title={dir === "desc" ? "Descending" : "Ascending"}
              onClick={onDir}
            >
              <ArrowUpDown className="h-3.5 w-3.5" aria-hidden /> {dir}
            </button>
          </div>
          <div className="seg" role="group" aria-label="View" data-testid="lx-view-toggle">
            <button
              type="button"
              data-testid="lx-view-graphical"
              className="seg-btn focusring inline-flex items-center gap-1.5"
              data-active={view === "graphical"}
              aria-pressed={view === "graphical"}
              onClick={() => onView("graphical")}
            >
              <BarChart3 className="h-3.5 w-3.5" aria-hidden /> Graphical
            </button>
            <button
              type="button"
              data-testid="lx-view-table"
              className="seg-btn focusring inline-flex items-center gap-1.5"
              data-active={view === "table"}
              aria-pressed={view === "table"}
              onClick={() => onView("table")}
            >
              <Table2 className="h-3.5 w-3.5" aria-hidden /> Table
            </button>
          </div>
        </div>
      </div>

      {/* filter chips stay in the normal page flow on mobile so they cannot widen the document. */}
      <div
        data-testid="lx-filters"
        role="group"
        aria-label="Filter languages"
        className="flex max-w-full flex-wrap gap-2 pb-1"
      >
        {CHIPS.map(([key, label]) => {
          const active = filter === key;
          return (
            <button
              key={key}
              type="button"
              data-testid={`lx-chip-${key}`}
              className="focusring chip shrink-0"
              aria-pressed={active}
              onClick={() => onFilter(key)}
              style={{
                cursor: "pointer",
                color: active ? "var(--text)" : "var(--muted)",
                borderColor: active ? "var(--primary-dim)" : "var(--line)",
                background: active ? "var(--surface-raised)" : "var(--surface-raised)",
                boxShadow: active ? "inset 0 0 0 1px var(--primary-dim)" : "none",
                whiteSpace: "nowrap",
              }}
            >
              {label}
              <span style={{ color: "var(--faint)", marginLeft: 4 }}>{counts[key]}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

/* ============================ GRAPHICAL VIEW =========================== */

function MetricSwitch({ metric, onMetric }) {
  return (
    <div className="flex items-center justify-end gap-2">
      <span className="kicker" style={{ fontSize: 10 }}>
        metric
      </span>
      <div className="seg" role="group" aria-label="Graphical metric" data-testid="lx-metric">
        {Object.entries(METRICS).map(([key, m]) => (
          <button
            key={key}
            type="button"
            data-testid={`lx-metric-${key}`}
            className="seg-btn focusring"
            data-active={metric === key}
            aria-pressed={metric === key}
            onClick={() => onMetric(key)}
            title={m.scoped ? `${m.label} — vs graphify` : m.label}
          >
            {m.label}
            {m.scoped && (
              <span style={{ color: "var(--faint)", marginLeft: 5, fontSize: 10 }}>vs graphify</span>
            )}
          </button>
        ))}
      </div>
    </div>
  );
}

function MetricBars({ rows, metric, onInspect, inspect, data, reduced }) {
  const m = METRICS[metric];
  const isCoverage = metric === "coverage";

  // values present (non-null) for scaling
  const vals = rows.map((r) => m.accessor(r)).filter((v) => v != null && !Number.isNaN(v));
  const maxVal = vals.length ? Math.max(...vals) : 1;

  // coverage: track domain starts at parity ×1.0 → so the 29 ones are a calm
  // minimum stub and the 7 over-performers visibly extend.
  const COV_MIN = 1.0;
  const COV_MAX = Math.max(1.05, Math.ceil(maxVal * 20) / 20);

  function barPct(r) {
    const v = m.accessor(r);
    if (v == null || Number.isNaN(v)) return null;
    if (isCoverage) {
      const t = (Math.max(COV_MIN, v) - COV_MIN) / (COV_MAX - COV_MIN);
      // floor so ×1.0 rows render as a thin-but-present stub, never width:0
      return 4 + t * 96;
    }
    return Math.max(2, (v / maxVal) * 100);
  }

  function valueLabel(r) {
    const v = m.accessor(r);
    if (v == null || Number.isNaN(v)) return "n/a";
    if (metric === "symbols") return num(v);
    if (metric === "index") return secsStr(v);
    return `${Number(v).toFixed(2)}${m.suffix}`;
  }

  const detCov = data.summary.coverage;
  // Per-language parity split, derived live from the coverage rows actually in
  // the dataset (so the "X at parity · Y exceed · Z below" counts are languages,
  // never silently the row figure). Kept distinct from the deterministic-row
  // figure above to avoid mixing two denominators.
  const covLangs = data.liveBenchmarks.filter((r) => r.coverage && typeof r.coverage.ratio === "number");
  const atParityLangs = covLangs.filter((r) => r.coverage.ratio <= 1.0001).length;
  const exceedLangs = covLangs.filter((r) => r.coverage.ratio > 1.0001).length;
  const belowLangs = covLangs.filter((r) => r.coverage.ratio < 0.9999).length;

  return (
    <div data-testid="lx-graphical">
      {/* big-truth banner (coverage mode only) */}
      {isCoverage && (
        <div
          data-testid="lx-parity-truth"
          className="mb-4 flex flex-wrap items-center gap-x-5 gap-y-1.5 rounded-lg px-4 py-3"
          style={{ background: "var(--bg2)", border: "1px solid var(--line)" }}
        >
          <span className="num" style={{ fontSize: 18, fontWeight: 600, color: "var(--success)" }}>
            {atParityLangs + exceedLangs}/{covLangs.length}
          </span>
          <span style={{ fontSize: 13, color: "var(--muted)" }}>
            live languages at or above native parity
          </span>
          <span className="mono" style={{ fontSize: 11, color: "var(--faint)" }}>
            {atParityLangs} at exactly ×1.0 · {exceedLangs} exceed · {belowLangs} below · {detCov.detectorOnlyRowsCovered} detector-only
          </span>
        </div>
      )}

      {/* axis caption */}
      <div className="mono mb-2 flex items-center justify-between" style={{ fontSize: 10.5, color: "var(--faint)" }}>
        <span>
          {m.label} — {m.axis}
          {m.scoped && <span style={{ color: "var(--warning)" }}> · vs graphify (1 of 21 tools)</span>}
        </span>
        {isCoverage && <span style={{ color: "var(--warning)" }}>×1.0 parity ▏</span>}
      </div>

      <div className="flex flex-col">
        {rows.map((r, i) => {
          const v = m.accessor(r);
          const isNull = v == null || Number.isNaN(v);
          const pct = barPct(r);
          const active = inspect === r.language;
          const isExceed = isCoverage && r.exceedsNative;
          const isParityOne = isCoverage && v != null && Math.abs(v - 1) < 0.0001;
          const isDetectorMetric = !isCoverage && r.detectorOnly && !isNull && (metric === "tokens" || metric === "latency");
          // Group band: in coverage mode (sorted desc) draw ONE labelled divider
          // at the ×1.0 baseline — the boundary between the few that exceed
          // native and the many that sit exactly at parity, so the shape of the
          // story reads without parsing every number.
          const prev = rows[i - 1];
          const showParityBand =
            isCoverage && r.exceedsNative === false && (i === 0 || prev?.exceedsNative === true);

          // fill color encodes tier/status, never decoration
          let fill;
          if (r.tier === "core") fill = "var(--secondary)";
          else fill = "var(--primary)";
          let bg = active
            ? "var(--primary)"
            : `linear-gradient(90deg, ${fill}, ${fill})`;

          const rowEl = (
            <button
              key={r.language}
              type="button"
              data-testid="lx-bar-row"
              onClick={() => onInspect(r.language)}
              className="focusring group grid w-full items-center gap-3 rounded-md px-1.5 py-1.5 text-left"
              style={{
                gridTemplateColumns: "minmax(72px,120px) minmax(0,1fr) auto",
                background: active ? "rgba(94,230,196,0.06)" : i % 2 ? "rgba(255,255,255,0.015)" : "transparent",
                cursor: "pointer",
              }}
              aria-label={`${r.label}: ${valueLabel(r)}${isExceed ? ", exceeds native" : ""}. Inspect.`}
            >
              <span
                className="num truncate"
                style={{ fontSize: 12.5, color: active ? "var(--primary)" : "var(--text)" }}
                title={r.label}
              >
                {r.label}
              </span>

              <div className="relative" style={{ height: 16 }}>
                {isNull ? (
                  /* hollow dashed track — never a colored bar, never width:0 */
                  <div
                    className="absolute inset-0 flex items-center rounded"
                    style={{
                      border: "1px dashed var(--not-comparable)",
                      background: "transparent",
                    }}
                  >
                    <span className="mono px-2" style={{ fontSize: 10, color: "var(--not-comparable)" }}>
                      ▱ {isCoverage && r.tier === "core" ? "n/a · matrix row (no native baseline)" : "not comparable"}
                    </span>
                  </div>
                ) : (
                  <>
                    <div
                      className="absolute inset-y-0 left-0 rounded"
                      style={{ width: "100%", background: "var(--line)", opacity: 0.4, height: 2, top: 7 }}
                      aria-hidden
                    />
                    <div
                      className="absolute left-0 top-0 flex items-center rounded"
                      style={{
                        height: 16,
                        width: `${reduced ? pct : pct}%`,
                        background: isDetectorMetric
                          ? "repeating-linear-gradient(45deg, var(--warning), var(--warning) 4px, rgba(242,180,58,0.35) 4px, rgba(242,180,58,0.35) 8px)"
                          : bg,
                        borderTop: isExceed ? "1px solid var(--success)" : "none",
                        transition: reduced ? "none" : "width .45s cubic-bezier(.2,.7,.2,1)",
                        minWidth: 3,
                      }}
                    >
                      {isExceed && (
                        <span
                          className="absolute"
                          style={{ right: -13, top: "50%", transform: "translateY(-50%)", color: "var(--success)", fontSize: 10 }}
                          aria-hidden
                        >
                          ▲
                        </span>
                      )}
                    </div>
                    {isParityOne && (
                      <span
                        className="mono absolute"
                        style={{ left: "calc(4% + 8px)", top: "50%", transform: "translateY(-50%)", fontSize: 9.5, color: "var(--faint)" }}
                        aria-hidden
                      >
                        = parity
                      </span>
                    )}
                    {isDetectorMetric && (
                      <span
                        className="mono absolute"
                        style={{ left: 6, top: "50%", transform: "translateY(-50%)", fontSize: 9.5, color: "#1a1304", fontWeight: 600 }}
                        aria-hidden
                      >
                        detector-only
                      </span>
                    )}
                  </>
                )}
              </div>

              <span className="flex items-center gap-2 whitespace-nowrap">
                <span
                  className="num"
                  style={{
                    fontSize: 12,
                    color: isNull
                      ? "var(--not-comparable)"
                      : isExceed
                      ? "var(--success)"
                      : r.tier === "core"
                      ? "var(--secondary)"
                      : "var(--text)",
                    fontWeight: isExceed ? 600 : 500,
                  }}
                >
                  {valueLabel(r)}
                </span>
                {isCoverage && r.atlasDefs != null && (
                  <span className="mono hidden sm:inline" style={{ fontSize: 10, color: "var(--faint)" }}>
                    {num(r.atlasDefs)}/{num(r.nativeDefs)}
                  </span>
                )}
                <a
                  href={r.artifactPath}
                  download
                  data-source-artifact
                  data-testid="download-link"
                  className="focusring"
                  aria-label={`Open raw JSON for ${r.label}`}
                  title="Open raw JSON"
                  onClick={(e) => e.stopPropagation()}
                  style={{ color: "var(--faint)", display: "inline-flex" }}
                >
                  <FileJson className="h-3.5 w-3.5" aria-hidden />
                </a>
              </span>
            </button>
          );

          if (showParityBand) {
            return (
              <React.Fragment key={`band-${r.language}`}>
                <div
                  className="mono my-1 flex items-center gap-2"
                  style={{ fontSize: 10, color: "var(--warning)", letterSpacing: "0.08em" }}
                  aria-hidden
                >
                  <span style={{ flex: "0 0 auto" }}>×1.0 parity baseline</span>
                  <span style={{ flex: 1, height: 1, background: "var(--warning)", opacity: 0.4 }} />
                </div>
                {rowEl}
              </React.Fragment>
            );
          }
          return rowEl;
        })}
      </div>
    </div>
  );
}

/* ============================== TABLE VIEW ============================= */

const COLS = [
  ["language", "lang", true],
  ["tier", "tier", false],
  ["repo", "repo · commit", false],
  ["index", "index", true],
  ["symbols", "symbols", true],
  ["edges", "edges", true],
  ["tokens", "token×", true],
  ["latency", "latency×", true],
  ["coverage", "coverage", true],
  ["nativeTool", "native tool", false],
  ["status", "status", false],
  ["evidence", "evidence", false],
];

function MatrixTable({ rows, sort, dir, onSort, onInspect, inspect }) {
  function ariaSort(key) {
    if (sort !== key) return "none";
    return dir === "asc" ? "ascending" : "descending";
  }
  return (
    <div className="tablewrap" data-testid="lx-table">
      <table className="dtable">
        <thead>
          <tr>
            {COLS.map(([key, label, sortable]) => (
              <th key={key} aria-sort={sortable ? ariaSort(key) : undefined}>
                {sortable ? (
                  <button
                    type="button"
                    className="focusring inline-flex items-center gap-1"
                    onClick={() => onSort(key)}
                    style={{ background: "none", border: "none", color: "inherit", font: "inherit", cursor: "pointer", padding: 0, letterSpacing: "inherit", textTransform: "inherit" }}
                  >
                    {label}
                    {sort === key && <span aria-hidden style={{ fontSize: 9 }}>{dir === "asc" ? "▲" : "▼"}</span>}
                  </button>
                ) : (
                  label
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const comparable = r.status === "comparable";
            const active = inspect === r.language;
            return (
              <tr key={r.language} data-testid="lx-row" style={active ? { outline: "1px solid var(--primary-dim)" } : undefined}>
                <td>
                  <button
                    type="button"
                    className="focusring text-left"
                    onClick={() => onInspect(r.language)}
                    style={{ color: "var(--text)", background: "none", border: "none", cursor: "pointer", padding: 0, font: "inherit" }}
                    title="Inspect"
                  >
                    {r.label}
                  </button>
                  <div className="num" style={{ fontSize: 11, color: "var(--faint)" }}>{r.language}</div>
                </td>
                <td>
                  <span className="mono" style={{ fontSize: 11, color: r.tier === "core" ? "var(--secondary)" : "var(--muted)" }}>
                    {r.tier}
                  </span>
                </td>
                <td>
                  {r.tier === "supported" ? (
                    <span style={{ color: "var(--muted)" }}>{r.repoShort}</span>
                  ) : (
                    <a className="link" href={r.repo} target="_blank" rel="noreferrer">
                      {r.repoShort}
                    </a>
                  )}
                  <div className="num" style={{ fontSize: 11, color: "var(--faint)" }}>
                    {r.commit ? shortSha(r.commit) : "—"}
                  </div>
                </td>
                <td className="num">{secsStr(r.indexSeconds)}</td>
                <td className="num">{num(r.symbols)}</td>
                <td className="num">{num(r.edges)}</td>
                <td style={{ minWidth: 96 }}>
                  {comparable ? (
                    <>
                      <span className="num" style={{ color: "var(--primary)" }}>{ratioStr(r.tokenRatio)}</span>
                      {r.tokenRatio != null && <RatioBar value={r.tokenRatio} max={32} color="var(--primary)" />}
                    </>
                  ) : (
                    <NotComparableCell />
                  )}
                </td>
                <td style={{ minWidth: 96 }}>
                  {comparable ? (
                    <span className="num" style={{ color: "var(--secondary)" }}>{ratioStr(r.latencyRatio)}</span>
                  ) : (
                    <NotComparableCell />
                  )}
                </td>
                <td className="num">
                  {r.coverageRatio == null ? (
                    <span
                      style={{ color: "var(--faint)" }}
                      title="Matrix rows benchmark vs graphify; native coverage is a live-suite metric"
                    >
                      —
                    </span>
                  ) : (
                    <span
                      style={{ color: r.coverageRatio >= 1 ? "var(--success)" : "var(--text)", fontWeight: r.exceedsNative ? 600 : 500 }}
                    >
                      {ratioStr(r.coverageRatio)}
                      {r.exceedsNative && <span style={{ color: "var(--success)", marginLeft: 4 }} aria-hidden>▲</span>}
                    </span>
                  )}
                </td>
                <td className="num" style={{ color: "var(--muted)" }}>{r.nativeTool}</td>
                <td><StatusPill status={r.status} /></td>
                <td>
                  <a
                    className="focusring chip"
                    href={r.artifactPath}
                    download
                    data-source-artifact
                    data-testid="download-link"
                    style={{ textDecoration: "none" }}
                  >
                    <Download className="h-3 w-3" aria-hidden /> JSON
                  </a>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

/* ============================== ROW DRAWER ============================= */

function DrawerStat({ label, children }) {
  return (
    <div>
      <div className="mono" style={{ fontSize: 10.5, color: "var(--faint)" }}>{label}</div>
      <div className="num mt-0.5" style={{ fontSize: 12.5, color: "var(--text)" }}>{children}</div>
    </div>
  );
}

function RowDrawer({ row, onClose, asSheet }) {
  const ref = useRef(null);
  useEffect(() => {
    if (!asSheet) return undefined;
    const onKey = (e) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    ref.current?.focus();
    return () => document.removeEventListener("keydown", onKey);
  }, [asSheet, onClose]);

  if (!row) return null;
  const comparable = row.status === "comparable";

  const body = (
    <div
      ref={ref}
      tabIndex={-1}
      data-testid="lx-drawer"
      className="panel min-w-0 p-5"
      aria-label={`${row.label} evidence`}
      style={asSheet ? { borderRadius: "16px 16px 0 0" } : undefined}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="font-semibold" style={{ fontSize: 17 }}>{row.label}</h3>
            <span className="mono" style={{ fontSize: 11, color: row.tier === "core" ? "var(--secondary)" : "var(--muted)" }}>
              {row.tier}
            </span>
            {row.exceedsNative && (
              <span
                className="mono inline-flex items-center rounded-full px-2 py-0.5"
                style={{ fontSize: 10, color: "var(--success)", border: "1px solid var(--success)" }}
              >
                exceeds native
              </span>
            )}
          </div>
          {row.tier === "supported" ? (
            <div className="mono mt-1 block break-words" style={{ fontSize: 12, color: "var(--muted)" }}>
              {row.repoShort}
            </div>
          ) : (
            <a
              className="link mono mt-1 block break-words"
              href={row.repo}
              target="_blank"
              rel="noreferrer"
              style={{ fontSize: 12 }}
            >
              {row.repoShort} <ExternalLink className="inline h-3 w-3" aria-hidden />
            </a>
          )}
          {row.commit && (
            <div className="mono mt-0.5" style={{ fontSize: 11, color: "var(--faint)" }}>{shortSha(row.commit)}</div>
          )}
        </div>
        <div className="flex items-center gap-2">
          <StatusPill status={row.status} />
          {asSheet && (
            <button type="button" className="focusring chip" aria-label="Close" onClick={onClose}>
              <X className="h-3.5 w-3.5" aria-hidden />
            </button>
          )}
        </div>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-3">
        <DrawerStat label="native tool">{row.nativeTool}</DrawerStat>
        <DrawerStat label="coverage vs native">
          {row.coverageRatio == null ? (
            <span style={{ color: "var(--faint)" }}>— (no public-repo coverage row)</span>
          ) : (
            <span style={{ color: row.coverageRatio >= 1 ? "var(--success)" : "var(--text)" }}>
              {ratioStr(row.coverageRatio)}
            </span>
          )}
        </DrawerStat>
        <DrawerStat label="atlas defs / native defs">
          {row.atlasDefs == null ? "—" : `${num(row.atlasDefs)} / ${num(row.nativeDefs)}`}
        </DrawerStat>
        <DrawerStat label="comparable rows">{row.equivalentRows}/{row.rows}</DrawerStat>
        <DrawerStat label="symbols">{num(row.symbols)}</DrawerStat>
        <DrawerStat label="edges">{num(row.edges)}</DrawerStat>
        <DrawerStat label="index time">{secsStr(row.indexSeconds)}</DrawerStat>
        <DrawerStat label="token× / latency×">
          {comparable ? (
            <span>
              <span style={{ color: "var(--primary)" }}>{ratioStr(row.tokenRatio)}</span>
              {" / "}
              <span style={{ color: "var(--secondary)" }}>{ratioStr(row.latencyRatio)}</span>
              <span style={{ color: "var(--faint)", marginLeft: 6, fontSize: 10 }}>vs graphify</span>
            </span>
          ) : (
            <span style={{ color: "var(--not-comparable)" }}>not comparable</span>
          )}
        </DrawerStat>
        {row.tier === "supported" && (
          <>
            <DrawerStat label="fixture recall">
              {row.oracleRecall == null ? "—" : `${Math.round(row.oracleRecall * 100)}%`}
            </DrawerStat>
            <DrawerStat label="fixture precision">
              {row.oraclePrecision == null ? "—" : `${Math.round(row.oraclePrecision * 100)}%`}
            </DrawerStat>
          </>
        )}
      </div>

      <a
        href={row.artifactPath}
        download
        data-source-artifact
        data-testid="download-link"
        className="btn btn-ghost focusring mt-4 w-full"
        style={{ textDecoration: "none" }}
      >
        <Download className="h-4 w-4" aria-hidden /> Open raw JSON · {row.artifactName}
      </a>
    </div>
  );

  if (asSheet) {
    return (
      <div
        className="fixed inset-0 z-50 flex items-end"
        style={{ background: "rgba(4,6,10,0.6)" }}
        onClick={onClose}
        role="dialog"
        aria-modal="true"
      >
        <div className="w-full" onClick={(e) => e.stopPropagation()}>
          {body}
        </div>
      </div>
    );
  }
  return body;
}

/* =============================== EXPLORER ============================== */

export default function LanguagesExplorer({ data }) {
  const reduced = usePrefersReducedMotion();
  const searchRef = useRef(null);
  const [view, setView] = useState("graphical");
  const [metric, setMetric] = useState("coverage");
  const [filter, setFilter] = useState("all");
  const [sort, setSort] = useState("coverage");
  const [dir, setDir] = useState("desc");
  const [query, setQuery] = useState("");
  const [inspect, setInspect] = useState(null);
  const [sortTouched, setSortTouched] = useState(false);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [isNarrow, setIsNarrow] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return undefined;
    const mq = window.matchMedia("(max-width: 1279px)");
    const fn = () => setIsNarrow(mq.matches);
    fn();
    mq.addEventListener?.("change", fn);
    return () => mq.removeEventListener?.("change", fn);
  }, []);

  const allRows = useMemo(() => buildAllRows(data), [data]);

  // live filter counts (recomputed from allRows)
  const counts = useMemo(() => {
    const c = {
      all: allRows.length,
      comparable: allRows.filter((r) => r.status === "comparable").length,
      "not-comparable": allRows.filter((r) => r.status === "not-comparable").length,
      "detector-only": allRows.filter((r) => r.status === "detector-only").length,
      "exceeds-native": allRows.filter((r) => r.exceedsNative).length,
      supported: allRows.filter((r) => r.tier === "supported").length,
      core: allRows.filter((r) => r.tier === "core").length,
      live: allRows.filter((r) => r.tier === "live").length,
    };
    return c;
  }, [allRows]);

  // filtered + sorted rows — derived ONCE, shared by both views
  const rows = useMemo(() => {
    let next = allRows;
    if (filter === "comparable") next = next.filter((r) => r.status === "comparable");
    else if (filter === "not-comparable") next = next.filter((r) => r.status === "not-comparable");
    else if (filter === "detector-only") next = next.filter((r) => r.status === "detector-only");
    else if (filter === "exceeds-native") next = next.filter((r) => r.exceedsNative);
    else if (filter === "supported") next = next.filter((r) => r.tier === "supported");
    else if (filter === "core") next = next.filter((r) => r.tier === "core");
    else if (filter === "live") next = next.filter((r) => r.tier === "live");

    const term = query.trim().toLowerCase();
    if (term) {
      next = next.filter((r) =>
        [r.language, r.label, r.repo, r.commit, r.nativeTool].some((v) =>
          String(v || "").toLowerCase().includes(term)
        )
      );
    }

    const acc = SORT_ACCESSORS[sort] || SORT_ACCESSORS.language;
    const mult = dir === "asc" ? 1 : -1;
    next = [...next].sort((a, b) => {
      const av = acc(a);
      const bv = acc(b);
      // nulls always to the BOTTOM, regardless of direction
      const an = av == null || Number.isNaN(av);
      const bn = bv == null || Number.isNaN(bv);
      if (an && bn) return a.label.localeCompare(b.label);
      if (an) return 1;
      if (bn) return -1;
      if (typeof av === "string") return mult * av.localeCompare(bv);
      return mult * (av - bv);
    });
    return next;
  }, [allRows, filter, query, sort, dir]);

  // inspected row defaults to the top-ranked row so the rail is never empty
  const inspectRow = useMemo(() => {
    const found = rows.find((r) => r.language === inspect);
    return found || rows[0] || null;
  }, [rows, inspect]);

  function handleMetric(key) {
    setMetric(key);
    if (!sortTouched) {
      const s = METRIC_DEFAULT_SORT[key];
      if (s) {
        setSort(s);
        setDir(key === "index" ? "asc" : "desc");
      }
    }
  }
  function handleSort(key) {
    if (key === sort) {
      setDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSort(key);
      setDir(key === "language" || key === "index" ? "asc" : "desc");
    }
    setSortTouched(true);
  }

  function handleInspect(lang) {
    setInspect(lang);
    if (isNarrow) setSheetOpen(true);
  }

  const supportedFamilies = data.summary.supported?.families ?? allRows.length;
  const publicRepoRows = (data.coreMatrix?.length || 0) + (data.liveBenchmarks?.length || 0);

  // keyboard shortcuts
  useEffect(() => {
    const onKey = (e) => {
      const t = e.target;
      const typing =
        t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable);
      if (typing) {
        if (e.key === "Escape") {
          setQuery("");
          t.blur();
        }
        return;
      }
      if (e.key === "g") setView("graphical");
      else if (e.key === "t") setView("table");
      else if (e.key === "/") {
        e.preventDefault();
        searchRef.current?.focus();
      } else if (e.key === "Escape") {
        if (sheetOpen) setSheetOpen(false);
        else setQuery("");
      } else if (["1", "2", "3", "4", "5"].includes(e.key) && view === "graphical") {
        const keys = Object.keys(METRICS);
        handleMetric(keys[Number(e.key) - 1]);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [view, sheetOpen, sortTouched]);

  return (
    <section id="matrix" data-testid="matrix" className="shell py-16" aria-labelledby="matrix-title">
      <div className="mb-7 max-w-3xl">
        <div className="kicker" style={{ color: "var(--primary)" }}>
          Languages · {supportedFamilies} supported · {publicRepoRows} public-repo rows
        </div>
        <h2
          id="matrix-title"
          className="mt-3 text-balance font-semibold tracking-tight"
          style={{ fontSize: "clamp(22px,3vw,30px)", lineHeight: 1.1, letterSpacing: "-0.015em" }}
        >
          Every language, every artifact
        </h2>
        <p className="mt-3" style={{ fontSize: 15, lineHeight: 1.6, color: "var(--muted)" }}>
          One dataset, two honest lenses. The default lens is Atlas&rsquo;s public-repo coverage-parity story; token× and
          latency× are available but scoped &ldquo;vs graphify.&rdquo; Supported-only fixture rows fill the rest of Atlas&rsquo;s
          {supportedFamilies} parser-family surface and stay non-comparable where public-repo/native evidence is absent.
        </p>
        <p className="mono mt-2" style={{ fontSize: 11, color: "var(--faint)" }} aria-hidden>
          keys: <b>g</b>/<b>t</b> view · <b>/</b> search · <b>1–5</b> metric · <b>Esc</b> clear
        </p>
      </div>

      <div data-testid="lx-explorer" className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_352px]">
        <div className="min-w-0">
          <ExplorerControls
            query={query}
            onQuery={setQuery}
            searchRef={searchRef}
            filter={filter}
            onFilter={setFilter}
            counts={counts}
            sort={sort}
            onSort={(v) => {
              setSort(v);
              setSortTouched(true);
            }}
            dir={dir}
            onDir={() => {
              setDir((d) => (d === "asc" ? "desc" : "asc"));
              setSortTouched(true);
            }}
            view={view}
            onView={setView}
          />

          {/* inline inspect panel below controls when rail is collapsed (<xl) */}
          {isNarrow && !sheetOpen && inspectRow && (
            <div className="mt-4 xl:hidden">
              <RowDrawer row={inspectRow} onClose={() => setInspect(null)} asSheet={false} />
            </div>
          )}

          {view === "graphical" && (
            <div className="mt-4">
              <MetricSwitch metric={metric} onMetric={handleMetric} />
              <div className="panel mt-3 p-4 sm:p-5">
                {rows.length === 0 ? (
                  <EmptyState query={query} onReset={() => { setQuery(""); setFilter("all"); }} />
                ) : (
                  <MetricBars
                    rows={rows}
                    metric={metric}
                    onInspect={handleInspect}
                    inspect={inspectRow?.language}
                    data={data}
                    reduced={reduced}
                  />
                )}
              </div>
            </div>
          )}

          {view === "table" && (
            <div className="mt-4">
              {rows.length === 0 ? (
                <div className="panel p-5">
                  <EmptyState query={query} onReset={() => { setQuery(""); setFilter("all"); }} />
                </div>
              ) : (
                <MatrixTable
                  rows={rows}
                  sort={sort}
                  dir={dir}
                  onSort={handleSort}
                  onInspect={handleInspect}
                  inspect={inspectRow?.language}
                />
              )}
            </div>
          )}

          <div className="mono mt-3" style={{ fontSize: 11.5, color: "var(--faint)" }} aria-live="polite">
            {rows.length} of {allRows.length} languages
            {filter !== "all" && <span> · {CHIPS.find(([k]) => k === filter)?.[1]}</span>}
            {query && <span> · matching “{query}”</span>}
          </div>
        </div>

        {/* persistent right rail @xl */}
        <div className="hidden xl:block">
          <div className="xl:sticky xl:top-[72px]">
            <RowDrawer row={inspectRow} onClose={() => setInspect(null)} asSheet={false} />
          </div>
        </div>
      </div>

      {/* mobile/tablet slide-over sheet */}
      {isNarrow && sheetOpen && inspectRow && (
        <RowDrawer row={inspectRow} onClose={() => setSheetOpen(false)} asSheet />
      )}
    </section>
  );
}

function EmptyState({ query, onReset }) {
  return (
    <div data-testid="lx-empty" className="flex flex-col items-center gap-3 py-10 text-center">
      <p style={{ fontSize: 14, color: "var(--muted)" }}>
        No languages match {query ? <span className="mono" style={{ color: "var(--text)" }}>“{query}”</span> : "the current filters"}.
      </p>
      <button type="button" className="btn btn-ghost focusring" onClick={onReset}>
        Clear filters
      </button>
    </div>
  );
}
