"use strict";

const state = {
  data: null,
  liveFilter: "all",
  liveSearch: "",
  liveSort: "language",
  detail: null,
};

const $ = (selector) => document.querySelector(selector);

function fmtNumber(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return "n/a";
  return Intl.NumberFormat("en-US").format(value);
}

function fmtSeconds(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return "n/a";
  return `${Number(value).toFixed(Number(value) < 1 ? 3 : 2)}s`;
}

function fmtRatio(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return "not comparable";
  return `${Number(value).toFixed(2)}x`;
}

function shortSha(value) {
  return value ? value.slice(0, 12) : "not recorded";
}

function languageLabel(value) {
  if (value === "cpp") return "C++";
  if (value === "objc") return "Objective-C";
  if (value === "csharp") return "C#";
  return String(value || "").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function sourceLink(label, href) {
  return `<a data-source-artifact href="${href}" target="_blank" rel="noreferrer">${label}</a>`;
}

function ratioBar(value, max = 42, cls = "") {
  const width = Math.max(0, Math.min(100, ((Number(value) || 0) / max) * 100));
  return `<div class="bar ${cls}"><span style="width:${width}%"></span></div>`;
}

function rowStatus(summary) {
  if (summary.equivalentRows === 0) return `<span class="chip warn">no comparable rows</span>`;
  if (summary.pass5x) return `<span class="chip ok">5x on comparable rows</span>`;
  return `<span class="chip warn">below 5x threshold</span>`;
}

function renderKPIs(data) {
  const { summary, provenance } = data;
  const cards = [
    ["Core matrix", summary.core.languages, `${summary.core.equivalentRows}/${summary.core.queryRows} comparable query rows`],
    ["Core latency", fmtRatio(summary.core.latencyRatio), "graphify ms / Atlas ms, comparable rows"],
    ["Core tokens", fmtRatio(summary.core.tokenRatio), "graphify tokens / Atlas tokens, comparable rows"],
    ["Live smokes", summary.live.artifacts, `${summary.live.withComparableRows} with comparable rows`],
    ["Graphify support", `${summary.coverage.deterministicRowsCovered}/${summary.coverage.graphifyRows}`, `${provenance.graphify.version}; ${summary.coverage.detectorOnlyRowsCovered} detector-only`],
  ];
  $("#kpis").innerHTML = cards.map(([label, value, note]) => `
    <article class="metric-card">
      <span class="label">${label}</span>
      <span class="value">${value}</span>
      <div class="note">${note}</div>
    </article>
  `).join("");
}

function renderProvenance(data) {
  const tools = data.provenance.tools.core
    .filter((tool) => ["atlas", "graphify", "scip-go", "scip-python", "scip-typescript", "scip-java", "gopls", "pyright", "tsc", "jdtls", "clangd"].includes(tool.tool))
    .map((tool) => `<tr><td>${tool.tool}</td><td>${tool.status}</td><td class="mono">${tool.version || "n/a"}</td></tr>`)
    .join("");

  $("#provenance-body").innerHTML = `
    <p data-testid="benchmark-source-root">Data source: ${data.sourceLabel}. The page loads <span class="mono">data/benchmark-data.json</span>, generated from copied raw benchmark JSON in <span class="mono">data/raw/</span>.</p>
    <p>Tool manifest generated: <span class="mono">${data.provenance.toolManifestGeneratedAt}</span>. Platform: ${data.provenance.platform.system} ${data.provenance.platform.release} ${data.provenance.platform.machine}; Python ${data.provenance.platform.python}.</p>
    <div class="table-wrap">
      <table>
        <thead><tr><th>Tool</th><th>Status</th><th>Version</th></tr></thead>
        <tbody>${tools}</tbody>
      </table>
    </div>
  `;
}

function renderCoreMatrix(data) {
  const rows = data.coreMatrix.map((row) => {
    const atlas = row.atlas.metrics || {};
    const graphify = row.graphify.metrics || {};
    return `
      <tr data-testid="matrix-row-${row.language}">
        <td>${languageLabel(row.language)}<br><span class="muted mono">${row.language}</span></td>
        <td><a href="${row.repo}" target="_blank" rel="noreferrer">${row.repo.replace("https://github.com/", "")}</a></td>
        <td>${row.atlas.status}<br><span class="muted">${fmtSeconds(atlas.cold_seconds ?? row.atlas.seconds)}</span></td>
        <td>${row.graphify.status}<br><span class="muted">${fmtSeconds(row.graphify.seconds)}</span></td>
        <td>${fmtNumber(atlas.files)} files<br>${fmtNumber(atlas.symbols)} symbols<br>${fmtNumber(atlas.edges)} edges</td>
        <td>${fmtNumber(graphify.nodes)} nodes<br>${fmtNumber(graphify.links)} links<br>${fmtNumber(graphify.calls)} calls</td>
        <td>${fmtNumber(atlas.calls)} calls<br>${fmtNumber(atlas.internal_calls)} internal</td>
        <td>${fmtNumber(graphify.extracted_calls)}/${fmtNumber(graphify.calls)}<br>${fmtNumber(graphify.extracted_pct)}% extracted</td>
        <td class="ratio">${fmtRatio(row.querySummary.latencyRatio)}${ratioBar(row.querySummary.latencyRatio, 10)}</td>
        <td class="ratio">${fmtRatio(row.querySummary.tokenRatio)}${ratioBar(row.querySummary.tokenRatio, 32, "blue")}</td>
        <td>${row.querySummary.equivalentRows}/${row.querySummary.rows}<br>${rowStatus(row.querySummary)}</td>
        <td>${sourceLink("JSON", row.artifact)}</td>
      </tr>
    `;
  }).join("");
  $("#core-matrix-body").innerHTML = rows;
}

function renderCoverageAudit(data) {
  const rows = data.coverageAudit.map((row) => `
    <tr>
      <td>${row.family}</td>
      <td class="mono">${row.extensions}</td>
      <td>${row.graphifyExtractor}</td>
      <td>${row.supportType === "detector-only" ? `<span class="chip warn">detector-only</span>` : `<span class="chip ok">deterministic</span>`}</td>
      <td>${row.atlasStatus}</td>
      <td>${sourceLink("source", row.artifact)}</td>
    </tr>
  `).join("");
  $("#coverage-body").innerHTML = rows;
}

function liveRows() {
  let rows = [...state.data.liveSmokes];
  const term = state.liveSearch.trim().toLowerCase();
  if (state.liveFilter === "comparable") {
    rows = rows.filter((row) => row.querySummary.equivalentRows > 0);
  } else if (state.liveFilter === "saturated") {
    rows = rows.filter((row) => row.querySummary.equivalentRows === 0);
  } else if (state.liveFilter === "detector") {
    rows = rows.filter((row) => row.detectorOnly);
  }
  if (term) {
    rows = rows.filter((row) => [row.language, row.repo, row.commit, row.native.tool, row.artifact].some((value) => String(value || "").toLowerCase().includes(term)));
  }
  rows.sort((a, b) => {
    if (state.liveSort === "latency") return (b.querySummary.latencyRatio || 0) - (a.querySummary.latencyRatio || 0);
    if (state.liveSort === "tokens") return (b.querySummary.tokenRatio || 0) - (a.querySummary.tokenRatio || 0);
    if (state.liveSort === "coverage") return (b.coverage.ratio || 0) - (a.coverage.ratio || 0);
    if (state.liveSort === "cycles") return (b.optimization.cyclesRun || 0) - (a.optimization.cyclesRun || 0);
    return a.language.localeCompare(b.language);
  });
  return rows;
}

function renderLiveTable() {
  const rows = liveRows().map((row) => `
    <tr data-live-language="${row.language}">
      <td>${languageLabel(row.language)}<br><span class="muted mono">${row.language}</span></td>
      <td><a href="${row.repo}" target="_blank" rel="noreferrer">${row.repo.replace("https://github.com/", "")}</a><br><span class="mono muted">${shortSha(row.commit)}</span></td>
      <td>${row.native.tool}<br><span class="chip ${row.native.ok ? "ok" : "warn"}">${row.native.status}</span></td>
      <td>${fmtRatio(row.coverage.ratio)}<br><span class="muted">${fmtNumber(row.coverage.atlasDefinitions)} / ${fmtNumber(row.coverage.nativeDefinitions)} defs</span></td>
      <td>${row.querySummary.equivalentRows}/${row.querySummary.rows}<br>${row.querySummary.graphifyMissing ? `<span class="muted">${row.querySummary.graphifyMissing} graphify missing</span>` : ""}</td>
      <td class="ratio">${fmtRatio(row.querySummary.latencyRatio)}${ratioBar(row.querySummary.latencyRatio, 10)}</td>
      <td class="ratio">${fmtRatio(row.querySummary.tokenRatio)}${ratioBar(row.querySummary.tokenRatio, 42, "blue")}</td>
      <td>${row.optimization.cyclesRun ?? "n/a"}</td>
      <td>${row.detectorOnly ? `<span class="chip warn">detector-only</span>` : rowStatus(row.querySummary)}</td>
      <td><button class="row-button" data-detail="${row.language}" type="button">Inspect</button><br>${sourceLink("raw", row.artifact)}</td>
    </tr>
  `).join("");
  $("#live-body").innerHTML = rows;
  $("#live-count").textContent = `${liveRows().length} rows`;
  $("#live-body").querySelectorAll("[data-detail]").forEach((button) => {
    button.addEventListener("click", () => {
      state.detail = state.data.liveSmokes.find((row) => row.language === button.dataset.detail);
      renderDetail();
    });
  });
}

function renderDetail() {
  const row = state.detail || state.data.liveSmokes.find((item) => item.language === "rust") || state.data.liveSmokes[0];
  state.detail = row;
  const queries = row.queries.map((query) => `
    <tr>
      <td>${query.symbol}</td>
      <td>${fmtSeconds((query.atlasMs || 0) / 1000)}</td>
      <td>${fmtSeconds((query.graphifyMs || 0) / 1000)}</td>
      <td>${fmtNumber(query.atlasTokens)}</td>
      <td>${fmtNumber(query.graphifyTokens)}</td>
      <td>${query.graphifyMissing ? `<span class="chip warn">graphify missing</span>` : `<span class="chip ok">comparable</span>`}</td>
    </tr>
  `).join("");
  $("#detail").innerHTML = `
    <h3>${languageLabel(row.language)} evidence</h3>
    <p class="break-anywhere"><a href="${row.repo}" target="_blank" rel="noreferrer">${row.repo}</a></p>
    <p class="mono break-anywhere">${row.commit}</p>
    <p>Native baseline: <strong>${row.native.tool}</strong> <span class="chip ${row.native.ok ? "ok" : "warn"}">${row.native.status}</span></p>
    <p>Coverage proxy: <strong>${fmtRatio(row.coverage.ratio)}</strong>. Comparable query rows: <strong>${row.querySummary.equivalentRows}/${row.querySummary.rows}</strong>.</p>
    <p>${sourceLink("Raw JSON artifact", row.artifact)}</p>
    <div class="table-wrap">
      <table>
        <thead><tr><th>Query</th><th>Atlas</th><th>graphify</th><th>Atlas tok</th><th>graphify tok</th><th>Status</th></tr></thead>
        <tbody>${queries}</tbody>
      </table>
    </div>
    <h3>Optimization note</h3>
    <p>${row.optimization.stopReason || "No optimization note recorded."}</p>
  `;
}

function renderSaturation(data) {
  const rows = data.saturation.map((row) => `
    <tr>
      <td>${languageLabel(row.language)}</td>
      <td>${row.status}</td>
      <td>${row.iterationsRun}/${row.iterationsRequested}</td>
      <td>${row.iterations.map((iteration) => iteration.equivalentRows).join(", ")}</td>
      <td>${row.iterations.map((iteration) => iteration.graphifyMissing).join(", ")}</td>
      <td>not comparable</td>
      <td>${sourceLink("saturation JSON", row.artifact)}</td>
    </tr>
  `).join("");
  $("#saturation-body").innerHTML = rows;
}

function renderArtifacts(data) {
  const important = ["MATRIX_REPORT.json", "MATRIX_TOOL_VERSIONS.json", "GRAPHIFY_LANGUAGE_DISCOVERY.json", "SATURATION_REPORT.json", "REPORT.json"];
  const artifacts = data.sourceArtifacts
    .filter((artifact) => important.includes(artifact.name) || /^LIVE_(RUST|BYOND|ETS|R|JAVA|PYTHON|APEX)_SMOKE/.test(artifact.name))
    .slice(0, 18)
    .map((artifact) => `
      <div class="artifact-item">
        <a href="${artifact.path}" data-source-artifact target="_blank" rel="noreferrer">${artifact.name}</a>
        <div class="muted">${fmtNumber(artifact.bytes)} bytes</div>
        <div class="mono muted">${artifact.sha256.slice(0, 16)}</div>
      </div>
    `).join("");
  $("#artifact-grid").innerHTML = artifacts;
}

function renderCaveats(data) {
  $("#caveats").innerHTML = data.caveats.map((item) => `<li>${item}</li>`).join("");
}

function bindControls() {
  $("#live-search").addEventListener("input", (event) => {
    state.liveSearch = event.target.value;
    renderLiveTable();
  });
  $("#live-filter").addEventListener("change", (event) => {
    state.liveFilter = event.target.value;
    renderLiveTable();
  });
  $("#live-sort").addEventListener("change", (event) => {
    state.liveSort = event.target.value;
    renderLiveTable();
  });
}

async function init() {
  const response = await fetch("data/benchmark-data.json", { cache: "no-store" });
  if (!response.ok) throw new Error(`Unable to load benchmark data: ${response.status}`);
  state.data = await response.json();
  renderKPIs(state.data);
  renderProvenance(state.data);
  renderCoreMatrix(state.data);
  renderCoverageAudit(state.data);
  renderLiveTable();
  renderDetail();
  renderSaturation(state.data);
  renderArtifacts(state.data);
  renderCaveats(state.data);
  bindControls();
  $("#generated-at").textContent = state.data.generatedAt;
}

init().catch((error) => {
  console.error(error);
  $("#app-error").hidden = false;
  $("#app-error").textContent = error.message;
});
