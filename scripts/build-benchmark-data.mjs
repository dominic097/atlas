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
    pass5x: Number(tokenRatio) >= 5 && Number(latencyRatio) >= 5,
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
  fs.rmSync(rawDir, { recursive: true, force: true });
  fs.mkdirSync(rawDir, { recursive: true });
  const live = fs.readdirSync(benchDir).filter((file) => /^LIVE_.*_BENCHMARK\.json$/.test(file)).sort();
  const files = [...required, ...live];
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

function buildLiveBenchmarks(artifactFiles) {
  return artifactFiles
    .filter((file) => /^LIVE_.*_BENCHMARK\.json$/.test(file))
    .map((file) => {
      const row = readJSON(file);
      return {
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
  };
}

function aggregateLive(liveBenchmarks) {
  const comparable = liveBenchmarks.filter((row) => row.querySummary.equivalentRows > 0);
  return {
    artifacts: liveBenchmarks.length,
    withComparableRows: comparable.length,
    saturatedNoComparable: liveBenchmarks.filter((row) => row.querySummary.equivalentRows === 0).length,
    fiveXComparable: comparable.filter((row) => row.querySummary.pass5x).length,
    detectorOnlyArtifacts: liveBenchmarks.filter((row) => row.detectorOnly).length,
  };
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
    ],
  };

  fs.writeFileSync(path.join(dataDir, "benchmark-data.json"), `${JSON.stringify(dataset, null, 2)}\n`);
  console.log(`Wrote data/benchmark-data.json from ${artifacts.length} JSON artifacts`);
}

main();
