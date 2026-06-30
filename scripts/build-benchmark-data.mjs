#!/usr/bin/env node
"use strict";

import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const benchInput = process.argv[2] || process.env.ATLAS_BENCH_DIR;
if (!benchInput) {
  throw new Error("Usage: node scripts/build-benchmark-data.mjs /path/to/bench");
}
const benchDir = path.resolve(benchInput);
const dataDir = path.join(repoRoot, "data");
const rawDir = path.join(dataDir, "raw");

const required = [
  "MATRIX_REPORT.json",
  "SATURATION_REPORT.json",
  "REPORT.json",
  "MATRIX_TOOL_VERSIONS.json",
  "GRAPHIFY_LANGUAGE_DISCOVERY.json",
];

const TEN_X_TARGET = 10;
const COVERAGE_EPSILON = 0.0001;

function readJSON(file) {
  return JSON.parse(fs.readFileSync(path.join(benchDir, file), "utf8"));
}

function round(value, digits = 2) {
  if (!Number.isFinite(value)) return null;
  return Number(value.toFixed(digits));
}

function sum(rows, key) {
  return rows.reduce((total, row) => total + (Number(row[key]) || 0), 0);
}

function sha256(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

function isoFromUnix(value) {
  return Number.isFinite(value) ? new Date(value * 1000).toISOString() : null;
}

function querySummary(queries = []) {
  const comparable = queries.filter((query) => {
    return !query.atlas_missing &&
      !query.graphify_missing &&
      Number(query.atlas_tokens) > 0 &&
      Number(query.graphify_tokens) > 0 &&
      Number(query.atlas_ms) > 0 &&
      Number(query.graphify_ms) > 0;
  });
  const atlasTokens = sum(comparable, "atlas_tokens");
  const graphifyTokens = sum(comparable, "graphify_tokens");
  const atlasMs = sum(comparable, "atlas_ms");
  const graphifyMs = sum(comparable, "graphify_ms");
  const tokenRatio = atlasTokens > 0 ? graphifyTokens / atlasTokens : null;
  const latencyRatio = atlasMs > 0 ? graphifyMs / atlasMs : null;
  const token10x = Number(tokenRatio) >= TEN_X_TARGET;
  const latency10x = Number(latencyRatio) >= TEN_X_TARGET;
  return {
    rows: queries.length,
    equivalentRows: comparable.length,
    atlasMissing: queries.filter((query) => query.atlas_missing).length,
    graphifyMissing: queries.filter((query) => query.graphify_missing).length,
    atlasTokens,
    graphifyTokens,
    atlasMs: round(atlasMs, 3),
    graphifyMs: round(graphifyMs, 3),
    tokenRatio: round(tokenRatio, 2),
    latencyRatio: round(latencyRatio, 2),
    targetRatio: TEN_X_TARGET,
    pass5x: Number(tokenRatio) >= 5 && Number(latencyRatio) >= 5,
    token10x,
    latency10x,
    pass10x: token10x && latency10x,
    tokenGapTo10x: token10x ? 1 : round(TEN_X_TARGET / tokenRatio, 2),
    latencyGapTo10x: latency10x ? 1 : round(TEN_X_TARGET / latencyRatio, 2),
  };
}

function coverageSummary(coverage = {}) {
  const ratioKey = Object.keys(coverage).find((key) => key.endsWith("_definition_ratio"));
  const atlasKey = Object.keys(coverage).find((key) => key.startsWith("atlas_") && key.endsWith("_definition_symbols"));
  return {
    ratio: ratioKey ? coverage[ratioKey] : null,
    ratioKey: ratioKey || null,
    atlasDefinitions: atlasKey ? coverage[atlasKey] : null,
    nativeDefinitions: coverage.native_definitions ?? null,
    scope: coverage.coverage_scope || null,
  };
}

function nativeSummary(nativeBaseline = {}) {
  return {
    tool: nativeBaseline.tool || nativeBaseline.name || "native baseline",
    status: nativeBaseline.status || (nativeBaseline.ok ? "ok" : "unknown"),
    ok: Boolean(nativeBaseline.ok),
    seconds: nativeBaseline.seconds ?? null,
    command: nativeBaseline.command || "",
    definitions: nativeBaseline.metrics?.definitions ?? null,
    diagnostics: nativeBaseline.metrics?.diagnostics ?? null,
  };
}

function artifactPath(file) {
  return `data/raw/${file}`;
}

function publicSource(file) {
  return `bench/${file}`;
}

function copyRawArtifacts() {
  const live = fs.readdirSync(benchDir).filter((file) => /^LIVE_.*_BENCHMARK\.json$/.test(file)).sort();
  const files = [...required, ...live];
  if (benchDir === rawDir) {
    return files.map((file) => {
      const source = path.join(benchDir, file);
      return {
        name: file,
        source: publicSource(file),
        path: artifactPath(file),
        bytes: fs.statSync(source).size,
        sha256: sha256(source),
      };
    });
  }
  fs.rmSync(rawDir, { recursive: true, force: true });
  fs.mkdirSync(rawDir, { recursive: true });
  for (const file of files) {
    fs.copyFileSync(path.join(benchDir, file), path.join(rawDir, file));
  }
  return files.map((file) => {
    const source = path.join(benchDir, file);
    const copied = path.join(rawDir, file);
    return {
      name: file,
      source: publicSource(file),
      path: artifactPath(file),
      bytes: fs.statSync(copied).size,
      sha256: sha256(source),
    };
  });
}

function buildCoreMatrix(matrix) {
  return matrix.map((row) => {
    const atlas = row.tools.atlas;
    const graphify = row.tools.graphify;
    const nativeTools = Object.entries(row.tools)
      .filter(([name]) => !["atlas", "graphify"].includes(name))
      .map(([name, tool]) => ({
        name,
        status: tool.status,
        ok: Boolean(tool.ok),
        seconds: tool.seconds ?? null,
        metrics: tool.metrics || {},
      }));
    return {
      language: row.language,
      repo: row.repo,
      subdir: row.subdir || "",
      artifact: artifactPath("MATRIX_REPORT.json"),
      source: publicSource("MATRIX_REPORT.json"),
      atlas: {
        status: atlas.status,
        ok: Boolean(atlas.ok),
        seconds: atlas.seconds,
        metrics: atlas.metrics,
      },
      graphify: {
        status: graphify.status,
        ok: Boolean(graphify.ok),
        seconds: graphify.seconds,
        metrics: graphify.metrics,
      },
      nativeTools,
      truth: row.truth || {},
      querySummary: querySummary(row.queries || []),
      queries: (row.queries || []).map((query) => ({
        symbol: query.symbol,
        atlasTokens: query.atlas_tokens,
        graphifyTokens: query.graphify_tokens,
        atlasMs: query.atlas_ms,
        graphifyMs: query.graphify_ms,
        atlasMissing: Boolean(query.atlas_missing),
        graphifyMissing: Boolean(query.graphify_missing),
      })),
    };
  });
}

function tenXStatus(row) {
  const qs = row.querySummary || {};
  const cov = row.coverage || {};
  const coverageRatio = typeof cov.ratio === "number" ? cov.ratio : null;
  const comparable = Number(qs.equivalentRows) > 0 && qs.tokenRatio != null && qs.latencyRatio != null;
  const coverageParity = coverageRatio != null && coverageRatio >= 1.0 - COVERAGE_EPSILON;
  const coverageExceedsNative = coverageRatio != null && coverageRatio > 1.0 + COVERAGE_EPSILON;
  const blockers = [];
  if (!coverageExceedsNative) blockers.push(coverageRatio == null ? "coverage_missing" : "coverage_at_parity");
  if (!comparable) blockers.push("no_comparable_query_rows");
  if (comparable && !qs.token10x) blockers.push("token_ratio_below_10x");
  if (comparable && !qs.latency10x) blockers.push("latency_ratio_below_10x");
  return {
    targetRatio: TEN_X_TARGET,
    comparable,
    coverageParity,
    coverageExceedsNative,
    token10x: Boolean(qs.token10x),
    latency10x: Boolean(qs.latency10x),
    performance10x: Boolean(qs.pass10x),
    strict10x: comparable && coverageExceedsNative && Boolean(qs.pass10x),
    blockers,
    note:
      "Coverage is Atlas definitions divided by the native baseline. It proves parity/exceed, not a 10x accuracy multiplier.",
  };
}

function buildLiveBenchmarks(artifactFiles) {
  return artifactFiles
    .filter((file) => /^LIVE_.*_BENCHMARK\.json$/.test(file))
    .map((file) => {
      const row = readJSON(file);
      const out = {
        language: row.language,
        repo: row.repo,
        commit: row.commit,
        artifact: artifactPath(file),
        source: publicSource(file),
        detectorOnly: Boolean(row.graphify_detector_only),
        atlas: {
          coldSeconds: row.atlas?.cold_wall_seconds ?? null,
          reindexSeconds: row.atlas?.reindex_wall_seconds ?? null,
          index: row.atlas?.index || {},
        },
        graphify: {
          status: row.graphify?.status || "unknown",
          ok: Boolean(row.graphify?.ok),
          seconds: row.graphify?.seconds ?? null,
        },
        native: nativeSummary(row.native_baseline || {}),
        richerNativeBaselines: row.richer_native_baselines || {},
        coverage: coverageSummary(row.coverage || {}),
        optimization: {
          cyclesRun: row.optimization?.cycles_run ?? null,
          stopReason: row.optimization?.stop_reason || "",
          cycleNotes: row.optimization?.cycle_notes || [],
        },
        querySummary: querySummary(row.queries || []),
        queries: (row.queries || []).map((query) => ({
          symbol: query.symbol,
          atlasTokens: query.atlas_tokens,
          graphifyTokens: query.graphify_tokens,
          atlasMs: query.atlas_ms,
          graphifyMs: query.graphify_ms,
          atlasMissing: Boolean(query.atlas_missing),
          graphifyMissing: Boolean(query.graphify_missing),
        })),
      };
      out.tenX = tenXStatus(out);
      return out;
    })
    .sort((a, b) => a.language.localeCompare(b.language));
}

function buildCoverageAudit(discovery, liveBenchmarks) {
  const detectorMap = new Map([
    [".ejs", "ejs"],
    [".ets", "ets"],
    [".r", "r"],
  ]);
  const rows = (discovery.rows || []).map(([family, extensions, extractor, atlasStatus]) => ({
    family,
    extensions,
    graphifyExtractor: extractor,
    atlasStatus,
    supportType: "deterministic extractor",
    artifact: artifactPath("GRAPHIFY_LANGUAGE_DISCOVERY.json"),
    source: publicSource("GRAPHIFY_LANGUAGE_DISCOVERY.json"),
  }));
  for (const extension of discovery.detector_only_code_extensions || []) {
    const language = detectorMap.get(extension) || extension.replace(/^\./, "");
    const live = liveBenchmarks.find((item) => item.language === language);
    rows.push({
      family: `detector-only ${extension}`,
      extensions: extension,
      graphifyExtractor: "detector only, no _DISPATCH extractor",
      atlasStatus: live ? "live Atlas benchmark evidence" : "no live benchmark artifact",
      supportType: "detector-only",
      artifact: live?.artifact || artifactPath("GRAPHIFY_LANGUAGE_DISCOVERY.json"),
      source: live?.source || publicSource("GRAPHIFY_LANGUAGE_DISCOVERY.json"),
    });
  }
  return rows;
}

function buildSaturation(saturation) {
  return (saturation.languages || []).map((row) => ({
    language: row.language,
    status: row.status,
    artifact: artifactPath("SATURATION_REPORT.json"),
    source: publicSource("SATURATION_REPORT.json"),
    iterationsRequested: saturation.iterations_requested,
    iterationsRun: row.iterations_run,
    nonImprovingIterations: row.non_improving_iterations,
    saturated: Boolean(row.saturated),
    note: row.note,
    iterations: (row.iterations || []).map((iteration) => ({
      iteration: iteration.iteration,
      seconds: iteration.seconds,
      commit: iteration.commit,
      coverageRatio: iteration.coverage_ratio,
      equivalentRows: iteration.queries?.equivalent_rows ?? null,
      graphifyMissing: iteration.queries?.graphify_missing ?? null,
      tokenRatio: iteration.queries?.token_ratio ?? null,
      latencyRatio: iteration.queries?.latency_ratio ?? null,
      artifact: iteration.artifact || null,
    })),
  }));
}

function aggregateCore(coreMatrix) {
  const summaries = coreMatrix.map((row) => row.querySummary);
  const atlasTokens = sum(summaries, "atlasTokens");
  const graphifyTokens = sum(summaries, "graphifyTokens");
  const atlasMs = sum(summaries, "atlasMs");
  const graphifyMs = sum(summaries, "graphifyMs");
  return {
    languages: coreMatrix.length,
    queryRows: sum(summaries, "rows"),
    equivalentRows: sum(summaries, "equivalentRows"),
    graphifyMissingRows: sum(summaries, "graphifyMissing"),
    tokenRatio: round(graphifyTokens / atlasTokens, 2),
    latencyRatio: round(graphifyMs / atlasMs, 2),
    targetRatio: TEN_X_TARGET,
    token10xRows: summaries.filter((row) => row.token10x).length,
    latency10xRows: summaries.filter((row) => row.latency10x).length,
    pass10xRows: summaries.filter((row) => row.pass10x).length,
  };
}

function aggregateLive(liveBenchmarks) {
  const comparable = liveBenchmarks.filter((row) => row.querySummary.equivalentRows > 0);
  const coverageRows = liveBenchmarks.filter((row) => row.coverage && typeof row.coverage.ratio === "number");
  const parity = coverageRows.filter((row) => row.coverage.ratio <= 1.0 + COVERAGE_EPSILON);
  const exceed = coverageRows.filter((row) => row.coverage.ratio > 1.0 + COVERAGE_EPSILON);
  const parityComparable = parity.filter((row) => row.querySummary.equivalentRows > 0);
  return {
    artifacts: liveBenchmarks.length,
    withComparableRows: comparable.length,
    saturatedNoComparable: liveBenchmarks.filter((row) => row.querySummary.equivalentRows === 0).length,
    fiveXComparable: comparable.filter((row) => row.querySummary.pass5x).length,
    targetRatio: TEN_X_TARGET,
    token10xComparable: comparable.filter((row) => row.querySummary.token10x).length,
    latency10xComparable: comparable.filter((row) => row.querySummary.latency10x).length,
    tenXComparable: comparable.filter((row) => row.querySummary.pass10x).length,
    strict10x: comparable.filter((row) => row.tenX?.strict10x).length,
    coverageParityLanguages: parity.length,
    coverageExceedLanguages: exceed.length,
    coverageBelowLanguages: coverageRows.filter((row) => row.coverage.ratio < 1.0 - COVERAGE_EPSILON).length,
    parityComparable: parityComparable.length,
    parityToken10x: parityComparable.filter((row) => row.querySummary.token10x).length,
    parityLatency10x: parityComparable.filter((row) => row.querySummary.latency10x).length,
    parityTenX: parityComparable.filter((row) => row.querySummary.pass10x).length,
    detectorOnlyArtifacts: liveBenchmarks.filter((row) => row.detectorOnly).length,
  };
}

function buildTenXGap(dataset) {
  const live = dataset.liveBenchmarks.map((row) => ({
    language: row.language,
    repo: row.repo,
    commit: row.commit,
    artifact: row.artifact,
    coverageRatio: row.coverage?.ratio ?? null,
    coverageExceedsNative: Boolean(row.tenX?.coverageExceedsNative),
    equivalentRows: row.querySummary?.equivalentRows ?? 0,
    tokenRatio: row.querySummary?.tokenRatio ?? null,
    latencyRatio: row.querySummary?.latencyRatio ?? null,
    tokenGapTo10x: row.querySummary?.tokenGapTo10x ?? null,
    latencyGapTo10x: row.querySummary?.latencyGapTo10x ?? null,
    blockers: row.tenX?.blockers ?? [],
  }));
  const parity = live.filter((row) => typeof row.coverageRatio === "number" && row.coverageRatio <= 1.0 + COVERAGE_EPSILON);
  const comparable = live.filter((row) => row.equivalentRows > 0 && row.tokenRatio != null && row.latencyRatio != null);
  return {
    schemaVersion: 1,
    generatedAt: dataset.generatedAt,
    source: "data/benchmark-data.json",
    targetRatio: TEN_X_TARGET,
    metricNote:
      "Token and latency are ratio targets. Coverage is a native-definition coverage ratio, so the honest accuracy target is >1.0 native coverage exceed, not a fabricated 10x accuracy multiplier.",
    summary: {
      liveLanguages: live.length,
      parityLanguages: parity.length,
      coverageExceedLanguages: live.filter((row) => row.coverageExceedsNative).length,
      comparableLanguages: comparable.length,
      token10xComparable: comparable.filter((row) => Number(row.tokenRatio) >= TEN_X_TARGET).length,
      latency10xComparable: comparable.filter((row) => Number(row.latencyRatio) >= TEN_X_TARGET).length,
      performance10xComparable: comparable.filter(
        (row) => Number(row.tokenRatio) >= TEN_X_TARGET && Number(row.latencyRatio) >= TEN_X_TARGET
      ).length,
    },
    parityLanguages: parity.map((row) => row.language),
    live,
    biggestLatencyGaps: comparable
      .filter((row) => row.latencyGapTo10x != null && row.latencyGapTo10x > 1)
      .sort((a, b) => b.latencyGapTo10x - a.latencyGapTo10x)
      .slice(0, 12),
    biggestTokenGaps: comparable
      .filter((row) => row.tokenGapTo10x != null && row.tokenGapTo10x > 1)
      .sort((a, b) => b.tokenGapTo10x - a.tokenGapTo10x)
      .slice(0, 12),
  };
}

function renderTenXGapMarkdown(report) {
  const lines = [];
  lines.push("# Atlas 10x Gap Report", "");
  lines.push(`Generated: ${report.generatedAt}`, "");
  lines.push(report.metricNote, "");
  lines.push("## Summary", "");
  lines.push(`- Live languages: ${report.summary.liveLanguages}`);
  lines.push(`- Coverage parity languages still to move into exceed: ${report.summary.parityLanguages}`);
  lines.push(`- Coverage exceed languages: ${report.summary.coverageExceedLanguages}`);
  lines.push(`- Comparable live languages: ${report.summary.comparableLanguages}`);
  lines.push(`- Token >=10x comparable: ${report.summary.token10xComparable}`);
  lines.push(`- Latency >=10x comparable: ${report.summary.latency10xComparable}`);
  lines.push(`- Token+latency >=10x comparable: ${report.summary.performance10xComparable}`);
  lines.push("", "## Biggest Latency Gaps", "");
  lines.push("| Language | latencyRatio | improvement to 10x | tokenRatio | blockers |");
  lines.push("|---|--:|--:|--:|---|");
  for (const row of report.biggestLatencyGaps) {
    lines.push(
      `| ${row.language} | ${row.latencyRatio ?? "n/a"} | ${row.latencyGapTo10x ?? "n/a"}x | ${row.tokenRatio ?? "n/a"} | ${row.blockers.join(", ")} |`
    );
  }
  lines.push("", "## Biggest Token Gaps", "");
  lines.push("| Language | tokenRatio | improvement to 10x | latencyRatio | blockers |");
  lines.push("|---|--:|--:|--:|---|");
  for (const row of report.biggestTokenGaps) {
    lines.push(
      `| ${row.language} | ${row.tokenRatio ?? "n/a"} | ${row.tokenGapTo10x ?? "n/a"}x | ${row.latencyRatio ?? "n/a"} | ${row.blockers.join(", ")} |`
    );
  }
  lines.push("");
  return `${lines.join("\n")}\n`;
}

function main() {
  for (const file of required) {
    if (!fs.existsSync(path.join(benchDir, file))) {
      throw new Error(`Missing required benchmark artifact: ${path.join(benchDir, file)}`);
    }
  }

  fs.mkdirSync(dataDir, { recursive: true });
  const artifacts = copyRawArtifacts();
  const matrix = readJSON("MATRIX_REPORT.json");
  const saturation = readJSON("SATURATION_REPORT.json");
  const toolVersions = readJSON("MATRIX_TOOL_VERSIONS.json");
  const graphifyDiscovery = readJSON("GRAPHIFY_LANGUAGE_DISCOVERY.json");
  const coreMatrix = buildCoreMatrix(matrix);
  const liveBenchmarks = buildLiveBenchmarks(artifacts.map((artifact) => artifact.name));
  const coverageAudit = buildCoverageAudit(graphifyDiscovery, liveBenchmarks);
  const saturationRows = buildSaturation(saturation);

  const dataset = {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    sourceLabel: "Atlas benchmark JSON artifacts from bench/",
    sourceArtifacts: artifacts,
    provenance: {
      toolManifestGeneratedAt: isoFromUnix(toolVersions.generated_at_unix),
      saturationGeneratedAt: isoFromUnix(saturation.generated_at_unix),
      platform: toolVersions.platform,
      graphify: {
        version: graphifyDiscovery.version,
        dispatchCount: graphifyDiscovery.dispatch_count,
        codeExtensionCount: graphifyDiscovery.code_extension_count,
        detectorOnlyCodeExtensions: graphifyDiscovery.detector_only_code_extensions || [],
        detectBenchmarkTotalFiles: graphifyDiscovery.detect_benchmark_total_files,
      },
      tools: {
        coreCount: (toolVersions.core_tools || []).length,
        liveBenchmarkCount: (toolVersions.live_benchmark_tools || []).length,
        core: toolVersions.core_tools || [],
        liveBenchmarkTools: toolVersions.live_benchmark_tools || [],
      },
    },
    summary: {
      core: aggregateCore(coreMatrix),
      live: aggregateLive(liveBenchmarks),
      coverage: {
        graphifyRows: (graphifyDiscovery.rows || []).length,
        deterministicRowsCovered: (graphifyDiscovery.rows || []).length,
        detectorOnlyRowsCovered: (graphifyDiscovery.detector_only_code_extensions || []).length,
      },
      saturation: {
        rows: saturationRows.length,
        iterationsRequested: saturation.iterations_requested,
        noComparableRows: saturationRows.filter((row) => row.saturated).length,
      },
    },
    coreMatrix,
    liveBenchmarks,
    coverageAudit,
    saturation: saturationRows,
    caveats: [
      "Ratios are computed only where both Atlas and graphify returned comparable query rows.",
      "BYOND, ETS, and R have saturation evidence with zero graphify-equivalent query rows; no latency/token ratio is claimed for those rows.",
      "Timings are one-machine benchmark snapshots, not production guarantees.",
      "Atlas and graphify expose different graph models, so coverage and precision fields are shown separately.",
      "10x target fields are strict for token and latency ratios; coverage is reported as native-definition parity/exceed and is not converted into a fabricated 10x accuracy multiplier.",
    ],
  };

  fs.writeFileSync(path.join(dataDir, "benchmark-data.json"), `${JSON.stringify(dataset, null, 2)}\n`);
  const tenXGap = buildTenXGap(dataset);
  fs.writeFileSync(path.join(dataDir, "tenx-gap-report.json"), `${JSON.stringify(tenXGap, null, 2)}\n`);
  fs.writeFileSync(path.join(dataDir, "tenx-gap-report.md"), renderTenXGapMarkdown(tenXGap));
  console.log(`Wrote data/benchmark-data.json and tenx gap reports from ${artifacts.length} JSON artifacts`);
}

main();
