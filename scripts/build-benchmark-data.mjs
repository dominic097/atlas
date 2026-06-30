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

function readDataJSON(file) {
  const target = path.join(dataDir, file);
  if (!fs.existsSync(target)) return null;
  return JSON.parse(fs.readFileSync(target, "utf8"));
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

function validationSummary(validation = {}) {
  const repos = Array.isArray(validation.repos) ? validation.repos : [];
  const minimumRepos = Number(validation.minimum_repos) || 3;
  const coverageRatios = repos
    .map((repo) => Number(repo.coverage_ratio))
    .filter((ratio) => Number.isFinite(ratio));
  const minCoverageRatio = coverageRatios.length ? Math.min(...coverageRatios) : null;
  const passedRepos = repos.filter((repo) => repo.ok !== false && Number(repo.coverage_ratio) >= 1.0 - COVERAGE_EPSILON);
  const passed = repos.length >= minimumRepos && passedRepos.length >= minimumRepos;
  return {
    status: validation.status || (passed ? "passed" : repos.length ? "partial" : "missing"),
    minimumRepos,
    repoCount: repos.length,
    passedRepoCount: passedRepos.length,
    passed,
    minCoverageRatio: round(minCoverageRatio, 4),
    generatedAt: validation.generated_at || null,
    scope: validation.scope || "",
    repos: repos.map((repo) => ({
      repo: repo.repo,
      commit: repo.commit || null,
      targetPath: repo.target_path || "",
      atlasDefinitions: repo.atlas_definitions ?? null,
      nativeDefinitions: repo.native_definitions ?? null,
      coverageRatio: repo.coverage_ratio ?? null,
      ok: repo.ok !== false,
      note: repo.note || "",
    })),
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
        multiRepoValidation: validationSummary(row.multi_repo_validation || {}),
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
  const validationPassed = liveBenchmarks.filter((row) => row.multiRepoValidation?.passed);
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
    threeRepoValidated: validationPassed.length,
    threeRepoValidatedStrict10x: validationPassed.filter((row) => row.tenX?.strict10x).length,
    threeRepoValidationPending: liveBenchmarks.length - validationPassed.length,
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
    validation: {
      passed: Boolean(row.multiRepoValidation?.passed),
      repoCount: row.multiRepoValidation?.repoCount ?? 0,
      minimumRepos: row.multiRepoValidation?.minimumRepos ?? 3,
      minCoverageRatio: row.multiRepoValidation?.minCoverageRatio ?? null,
    },
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
      threeRepoValidated: live.filter((row) => row.validation.passed).length,
      threeRepoValidatedStrict10x: live.filter((row) => row.validation.passed && row.coverageExceedsNative).length,
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
  lines.push(`- Minimum 3-repo validated: ${report.summary.threeRepoValidated}`);
  lines.push(`- Minimum 3-repo validated and coverage-exceed: ${report.summary.threeRepoValidatedStrict10x}`);
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

function collectLiveBenchmarkTools(liveBenchmarks, manifestTools = []) {
  const tools = new Map();
  for (const tool of manifestTools) {
    if (!tool?.tool && !tool?.name) continue;
    tools.set(tool.tool || tool.name, tool);
  }
  for (const row of liveBenchmarks) {
    const native = row.native || {};
    if (native.tool && !tools.has(native.tool)) {
      tools.set(native.tool, {
        tool: native.tool,
        status: native.status || (native.ok ? "ok" : "unknown"),
        ok: Boolean(native.ok),
        version: native.command || native.status || "",
        note: native.diagnostics || "",
      });
    }
  }
  return [...tools.values()].sort((a, b) => String(a.tool || "").localeCompare(String(b.tool || "")));
}

function classifyTruthRisk(row) {
  const tool = String(row.native?.tool || "").toLowerCase();
  if (["json", "markdown"].includes(row.language)) return "structured";
  if (row.detectorOnly || ["ejs", "ets", "r"].includes(row.language)) return "high";
  if (tool.includes("source-counter") || tool.includes("directive-counter") || tool.includes("counter")) return "medium";
  if (!row.multiRepoValidation?.passed) return "medium";
  if (Number(row.coverage?.ratio) > 5) return "medium";
  return "low";
}

function buildFinalAuditReport(dataset) {
  const structuredLive = new Set(["json", "markdown"]);
  const liveCodeRows = dataset.liveBenchmarks.filter((row) => !structuredLive.has(row.language));
  const pendingCode = liveCodeRows.filter((row) => !row.multiRepoValidation?.passed).map((row) => row.language);
  const publicValidation = dataset.publicRepoValidation || null;
  const publicValidationPassed = Boolean(publicValidation?.summary?.passed);
  const precisionEvidence = dataset.precisionEvidence || null;
  const precisionEvidencePassed = Boolean(precisionEvidence?.summary?.passed);
  const precisionSummary = precisionEvidence?.summary || {};
  const callEdgeEvidence = dataset.callEdgeEvidence || null;
  const callEdgeEvidencePassed = Boolean(callEdgeEvidence?.summary?.passed);
  const callEdgeSummary = callEdgeEvidence?.summary || {};
  const precisionRows = Array.isArray(precisionEvidence?.languages) ? precisionEvidence.languages : [];
  const precisionGaps = precisionRows
    .filter((row) => row.status !== "sampled-name-location")
    .map((row) => ({
      language: row.language,
      status: row.status,
      matchedNameLocationRows: row.matchedNameLocationRows,
      equivalentRows: row.equivalentRows,
      validationKindRows: row.validationKindRows,
      validationRows: row.validationRows,
      nativeMetricKindEvidence: row.nativeMetricKindEvidence || null,
      gap: row.gap || "",
    }))
    .sort((a, b) => String(a.language).localeCompare(String(b.language)));
  const precisionGapLanguages = new Set(precisionGaps.map((row) => row.language));
  const missingCoreTools = (dataset.provenance.tools.core || [])
    .filter((tool) => tool.ok === false || tool.status === "missing")
    .map((tool) => ({
      tool: tool.tool,
      status: tool.status,
      note: tool.note || tool.version || "",
    }));
  const scipJavaMissing = missingCoreTools.some((tool) => tool.tool === "scip-java");
  const weakTruthRows = dataset.liveBenchmarks
    .filter((row) => classifyTruthRisk(row) !== "low")
    .map((row) => ({
      language: row.language,
      nativeTool: row.native?.tool || "",
      risk: classifyTruthRisk(row),
      detectorOnly: Boolean(row.detectorOnly),
      coverageRatio: row.coverage?.ratio ?? null,
      validationPassed: Boolean(row.multiRepoValidation?.passed),
      minValidationCoverage: row.multiRepoValidation?.minCoverageRatio ?? null,
      reason:
        ["json", "markdown"].includes(row.language)
          ? "Structured artifact, not a code-parser ground-truth row; keep separate from regex-language completion claims."
          : row.detectorOnly
          ? "Graphify detector-only/source-counter proxy; weaker than a deterministic Graphify extractor and native compiler/LSP truth."
          : String(row.native?.tool || "").includes("counter")
            ? "Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source."
            : !row.multiRepoValidation?.passed
              ? "Missing minimum three-repo public validation."
              : "Atlas counts a wider code-target surface than the native denominator; inspect scope before treating the ratio as precision.",
    }));
  const rejectedCandidates = dataset.liveBenchmarks
    .flatMap((row) => (row.multiRepoValidation?.repos || []).length ? [] : [{ language: row.language, rejected: [] }]);
  const nearMisses = dataset.liveBenchmarks
    .flatMap((row) => (row.multiRepoValidation?.repos || []).map((repo) => ({
      language: row.language,
      repo: repo.repo,
      coverageRatio: repo.coverageRatio,
      atlasDefinitions: repo.atlasDefinitions,
      nativeDefinitions: repo.nativeDefinitions,
    })))
    .filter((row) => Number(row.coverageRatio) <= 1.01)
    .sort((a, b) => Number(a.coverageRatio) - Number(b.coverageRatio))
    .slice(0, 20);

  const foundDuringFinalPass = [
    "The UI previously carried a hard-coded native tool manifest; this final pass renders tool status/version from provenance data so missing tools are no longer shown as healthy.",
    scipJavaMissing
      ? "scip-java is missing in this environment; Java still has a JDTLS baseline, and the missing SCIP adapter is reported instead of implied."
      : "scip-java now resolves through the pinned bench/tools/scip-java-coursier launcher; Java is reported with both SCIP and JDTLS baselines present.",
    publicValidationPassed
      ? "The committed public-repo validation harness regenerates data/public-repo-validation-manifest.* from raw live artifacts and fails when a code language lacks passing three-repo evidence."
      : "The public-repo validation harness is still missing or failing; multi-repo validation remains raw-artifact evidence only.",
    precisionEvidencePassed
      ? "The committed precision-evidence harness regenerates data/precision-evidence-manifest.* from raw live artifacts and separates sampled symbol/location evidence from weaker kind-count-only rows."
      : "The precision-evidence harness is still missing or failing; precision remains described only by coverage and validation notes.",
    callEdgeEvidencePassed
      ? "The committed call-edge evidence harness regenerates data/call-edge-evidence-manifest.* from raw artifacts and separates core receiver-typed call evidence from live call-count-only rows."
      : "The call-edge evidence harness is still missing or failing; call-edge coverage remains embedded in raw artifacts only.",
    "Objective-C validation can be inflated by vendored Pods if Atlas and the native counter use different dependency filters; the final validation excludes dependency folders for the validation count.",
    "CUDA host-function counters overcount the denominator for a CUDA-specific benchmark; the final validation labels and uses a CUDA-qualified __global__/__device__/__host__ function denominator.",
  ];
  const improvementTodos = [
    ...(scipJavaMissing
      ? [
          {
            priority: "P0",
            item: "Install or vendor a reproducible scip-java command and rerun the Java matrix with both SCIP and JDTLS present.",
          },
        ]
      : []),
    ...(!publicValidationPassed
      ? [
          {
            priority: "P0",
            item: "Move the public-repo random validation harness into the repository so live multi-repo artifacts are reproducible from committed code, not only from raw JSON evidence.",
          },
        ]
      : []),
    {
      priority: "P1",
      item: "Promote the committed public-repo validation manifest harness from artifact verification to full remeasurement for every native/proxy counter.",
    },
    {
      priority: "P1",
      item: "Replace source-counter proxies for Apex, CUDA, Razor, BYOND, Blade, EJS, ETS, R, and structured/project surfaces with fuller compiler, LSP, tree-sitter, or parser-library denominators where available.",
    },
    {
      priority: "P1",
      item: precisionEvidencePassed
        ? "Close precision gaps by persisting full native and Atlas symbol name/kind/location sets for every validation repo; the current harness proves only sampled query name/location rows plus kind-count maps where raw artifacts expose them."
        : "Add precision checks that compare symbol names/kinds/locations, not only Atlas/native definition-count coverage ratios.",
    },
    {
      priority: "P1",
      item: callEdgeEvidencePassed
        ? "Extend receiver-type measurement into live converted tree-sitter artifacts; the current call-edge harness proves live call counts but receiver typing is only present in the core matrix artifacts."
        : "Extend call-edge and receiver-type measurement for converted tree-sitter languages beyond definition coverage.",
    },
    {
      priority: "P2",
      item: "Increase public-repo validation from 3 repos per language to a larger fixed sample for high-variance languages such as Objective-C, Razor, Apex, CUDA, and Swift.",
    },
    {
      priority: "P2",
      item: "Keep Graphify no-equivalent rows as saturation evidence, but separate detector-only language support from deterministic Graphify extractor support in all headlines.",
    },
  ];

  return {
    generatedAt: dataset.generatedAt,
    benchmarkGeneratedAt: dataset.generatedAt,
    scope:
      "Final pass over Atlas benchmark artifacts: core matrix against Graphify plus native SCIP/LSP tools, live language artifacts against Graphify plus language-specific native/proxy baselines, and public three-repo validation metadata.",
    summary: {
      coreLanguages: dataset.summary.core.languages,
      liveArtifacts: dataset.summary.live.artifacts,
      liveCodeLanguages: liveCodeRows.length,
      structuredLiveArtifacts: dataset.liveBenchmarks.length - liveCodeRows.length,
      totalCodeLanguageSurfaces: dataset.summary.core.languages + liveCodeRows.length,
      strict10x: dataset.summary.live.strict10x,
      strict10xArtifacts: dataset.summary.live.artifacts,
      threeRepoValidated: dataset.summary.live.threeRepoValidated,
      threeRepoValidationPending: dataset.summary.live.threeRepoValidationPending,
      pendingCodeLanguages: pendingCode,
      publicRepoValidationHarness: publicValidationPassed,
      publicRepoValidationWarnings: publicValidation?.summary?.warnings ?? null,
      precisionEvidenceHarness: precisionEvidencePassed,
      precisionNameLocationArtifacts: precisionSummary.sampledNameLocationArtifacts ?? null,
      precisionKindCountOnlyArtifacts: precisionSummary.kindCountOnlyArtifacts ?? null,
      precisionCountOnlyArtifacts: precisionSummary.countOnlyArtifacts ?? null,
      precisionMatchedNameLocationRows: precisionSummary.matchedNameLocationRows ?? null,
      precisionEquivalentRows: precisionSummary.equivalentRows ?? null,
      precisionValidationKindRows: precisionSummary.validationKindRows ?? null,
      precisionNativeMetricKindArtifacts: precisionSummary.nativeMetricKindArtifacts ?? null,
      callEdgeEvidenceHarness: callEdgeEvidencePassed,
      callEdgeCoreReceiverTypedLanguages: callEdgeSummary.coreReceiverTypedLanguages ?? null,
      callEdgeCoreLanguages: callEdgeSummary.coreLanguages ?? null,
      callEdgeCoreAtlasCalls: callEdgeSummary.coreAtlasCalls ?? null,
      callEdgeCoreReceiverTypedCalls: callEdgeSummary.coreAtlasReceiverTypedCalls ?? null,
      callEdgeCoreReceiverTypedRatio: callEdgeSummary.coreAtlasReceiverTypedRatio ?? null,
      callEdgeLiveArtifactsWithAtlasCalls: callEdgeSummary.liveArtifactsWithAtlasCalls ?? null,
      callEdgeLiveArtifacts: callEdgeSummary.liveArtifacts ?? null,
      callEdgeLiveReceiverTypedArtifacts: callEdgeSummary.liveReceiverTypedArtifacts ?? null,
      callEdgeLiveAtlasCalls: callEdgeSummary.liveAtlasCalls ?? null,
      graphifyVersion: dataset.provenance.graphify.version,
      graphifyDispatchCount: dataset.provenance.graphify.dispatchCount,
      detectorOnlyLanguages: dataset.provenance.graphify.detectorOnlyCodeExtensions,
    },
    groundTruthCloseness: {
      statement:
        "Coverage ratios prove Atlas produced at least as many definitions as the selected independent denominator for that scoped benchmark. They do not by themselves prove precision, complete call-edge recall, or semantic equivalence across all repos.",
      lowRiskLiveLanguages: dataset.liveBenchmarks
        .filter((row) => classifyTruthRisk(row) === "low" && !precisionGapLanguages.has(row.language))
        .map((row) => row.language)
        .sort(),
      weakOrProxyTruthRows: weakTruthRows,
      coreMatrix: dataset.coreMatrix.map((row) => ({
        language: row.language,
        nativeTools: row.nativeTools.map((tool) => ({ name: tool.name, status: tool.status, ok: tool.ok })),
        graphifyOk: row.graphify.ok,
        equivalentRows: row.querySummary.equivalentRows,
        graphifyMissingRows: row.querySummary.graphifyMissing,
        tokenRatio: row.querySummary.tokenRatio,
        latencyRatio: row.querySummary.latencyRatio,
      })),
      nearParityValidationRows: nearMisses,
      precisionEvidence: {
        statement:
          "The precision manifest is an artifact-level audit of what the raw benchmark JSON can prove today: sampled query rows with matching symbol names and locations, native-vs-Atlas kind-count maps, or count-only gaps. It is not a full 99% precision oracle.",
        manifest: precisionEvidence
          ? {
              generatedAt: precisionEvidence.generatedAt,
              summary: precisionSummary,
              gaps: precisionGaps,
            }
          : null,
      },
      callEdgeEvidence: {
        statement:
          "The call-edge manifest audits what raw artifacts can prove today: receiver-typed call counts for the core matrix and call-count evidence for live artifacts. It does not prove receiver-type precision for live converted languages.",
        manifest: callEdgeEvidence
          ? {
              generatedAt: callEdgeEvidence.generatedAt,
              summary: callEdgeSummary,
              liveGaps: (callEdgeEvidence.live || [])
                .filter((row) => row.status !== "receiver-typed")
                .map((row) => ({
                  language: row.language,
                  status: row.status,
                  atlasCalls: row.atlasCalls,
                  graphifyCalls: row.graphifyCalls,
                  graphifyDetectorOnly: row.graphifyDetectorOnly,
                  note: row.note,
                })),
            }
          : null,
      },
    },
    stubsAndHallucinationAudit: {
      foundDuringFinalPass,
      notFound:
        "No published benchmark row in the final dataset is intentionally synthetic or sample-only. The weakest rows are labelled as detector-only or source-counter proxy rows rather than hidden.",
      missingAdapters: missingCoreTools,
      generatedButNotSourceOfTruth: rejectedCandidates,
    },
    improvementTodos,
  };
}

function renderFinalAuditMarkdown(report) {
  const lines = [];
  lines.push("# Atlas Final Benchmark Audit", "");
  lines.push(`Generated: ${report.generatedAt}`, "");
  lines.push(report.scope, "");
  lines.push("## Summary", "");
  lines.push(`- Core matrix languages: ${report.summary.coreLanguages}`);
  lines.push(`- Live code/parser languages: ${report.summary.liveCodeLanguages}`);
  lines.push(`- Total code language surfaces: ${report.summary.totalCodeLanguageSurfaces}`);
  lines.push(`- Live artifacts: ${report.summary.liveArtifacts}`);
  lines.push(`- Strict 10x live artifacts: ${report.summary.strict10x}/${report.summary.strict10xArtifacts}`);
  lines.push(`- Three-repo validated live artifacts: ${report.summary.threeRepoValidated}`);
  lines.push(`- Pending code languages: ${report.summary.pendingCodeLanguages.length ? report.summary.pendingCodeLanguages.join(", ") : "none"}`);
  lines.push(`- Precision evidence harness: ${report.summary.precisionEvidenceHarness ? "present" : "missing"}`);
  lines.push(`- Precision sampled name/location artifacts: ${report.summary.precisionNameLocationArtifacts ?? "n/a"}`);
  lines.push(`- Precision kind-count-only artifacts: ${report.summary.precisionKindCountOnlyArtifacts ?? "n/a"}`);
  lines.push(`- Precision count-only artifacts: ${report.summary.precisionCountOnlyArtifacts ?? "n/a"}`);
  lines.push(`- Precision sampled query rows with name+location: ${report.summary.precisionMatchedNameLocationRows ?? "n/a"}/${report.summary.precisionEquivalentRows ?? "n/a"}`);
  lines.push(`- Precision validation rows with kind maps: ${report.summary.precisionValidationKindRows ?? "n/a"}`);
  lines.push(`- Precision artifacts with native metric kind maps: ${report.summary.precisionNativeMetricKindArtifacts ?? "n/a"}`);
  lines.push(`- Call-edge evidence harness: ${report.summary.callEdgeEvidenceHarness ? "present" : "missing"}`);
  lines.push(`- Core receiver-typed call languages: ${report.summary.callEdgeCoreReceiverTypedLanguages ?? "n/a"}/${report.summary.callEdgeCoreLanguages ?? "n/a"}`);
  lines.push(`- Core receiver-typed calls: ${report.summary.callEdgeCoreReceiverTypedCalls ?? "n/a"}/${report.summary.callEdgeCoreAtlasCalls ?? "n/a"} (${report.summary.callEdgeCoreReceiverTypedRatio ?? "n/a"})`);
  lines.push(`- Live artifacts with Atlas call counts: ${report.summary.callEdgeLiveArtifactsWithAtlasCalls ?? "n/a"}/${report.summary.callEdgeLiveArtifacts ?? "n/a"}`);
  lines.push(`- Live artifacts with receiver-typed calls: ${report.summary.callEdgeLiveReceiverTypedArtifacts ?? "n/a"}`);
  lines.push(`- Graphify: ${report.summary.graphifyVersion}, dispatch count ${report.summary.graphifyDispatchCount}`);
  lines.push("", "## Ground Truth Closeness", "");
  lines.push(report.groundTruthCloseness.statement, "");
  lines.push(`Low-risk live languages: ${report.groundTruthCloseness.lowRiskLiveLanguages.join(", ") || "none"}.`, "");
  lines.push("### Precision Evidence", "");
  lines.push(report.groundTruthCloseness.precisionEvidence.statement, "");
  if (report.groundTruthCloseness.precisionEvidence.manifest) {
    const manifest = report.groundTruthCloseness.precisionEvidence.manifest;
    lines.push(
      `Manifest: data/precision-evidence-manifest.md, generated ${manifest.generatedAt}.`
    );
    lines.push(
      `Sampled name/location artifacts: ${manifest.summary.sampledNameLocationArtifacts}; kind-count-only artifacts: ${manifest.summary.kindCountOnlyArtifacts}; count-only artifacts: ${manifest.summary.countOnlyArtifacts}.`
    );
    lines.push(
      `Matched sampled query rows: ${manifest.summary.matchedNameLocationRows}/${manifest.summary.equivalentRows}; validation kind-map rows: ${manifest.summary.validationKindRows}.`
    );
    lines.push(`Artifacts with native metric kind maps: ${manifest.summary.nativeMetricKindArtifacts}.`);
    lines.push("");
    lines.push("| Language | Status | Query name+location | Validation kind rows | Native metric kind map | Gap |");
    lines.push("|---|---|--:|--:|---|---|");
    for (const row of manifest.gaps) {
      const hasNativeMetricKindMap =
        row.nativeMetricKindEvidence?.hasDefinitionCounts || row.nativeMetricKindEvidence?.hasExpandedDefinitionCounts;
      lines.push(
        `| ${row.language} | ${row.status} | ${row.matchedNameLocationRows}/${row.equivalentRows} | ${row.validationKindRows}/${row.validationRows} | ${hasNativeMetricKindMap ? "yes" : "no"} | ${row.gap || "none"} |`
      );
    }
  } else {
    lines.push("Manifest: missing.");
  }
  lines.push("");
  lines.push("### Call Edge Evidence", "");
  lines.push(report.groundTruthCloseness.callEdgeEvidence.statement, "");
  if (report.groundTruthCloseness.callEdgeEvidence.manifest) {
    const manifest = report.groundTruthCloseness.callEdgeEvidence.manifest;
    lines.push(`Manifest: data/call-edge-evidence-manifest.md, generated ${manifest.generatedAt}.`);
    lines.push(
      `Core receiver-typed calls: ${manifest.summary.coreAtlasReceiverTypedCalls}/${manifest.summary.coreAtlasCalls}; live Atlas calls: ${manifest.summary.liveAtlasCalls}.`
    );
    lines.push(
      `Live receiver-typed artifacts: ${manifest.summary.liveReceiverTypedArtifacts}/${manifest.summary.liveArtifacts}; live artifacts with call counts: ${manifest.summary.liveArtifactsWithAtlasCalls}/${manifest.summary.liveArtifacts}.`
    );
    lines.push("");
    lines.push("| Language | Status | Atlas calls | Graphify calls | Detector-only Graphify | Note |");
    lines.push("|---|---|--:|--:|---|---|");
    for (const row of manifest.liveGaps) {
      lines.push(
        `| ${row.language} | ${row.status} | ${row.atlasCalls ?? "n/a"} | ${row.graphifyCalls ?? "n/a"} | ${row.graphifyDetectorOnly ? "yes" : "no"} | ${row.note} |`
      );
    }
  } else {
    lines.push("Manifest: missing.");
  }
  lines.push("");
  lines.push("### Weak Or Proxy Truth Rows", "");
  lines.push("| Language | Native tool | Risk | Coverage | Min validation coverage | Reason |");
  lines.push("|---|---|---|--:|--:|---|");
  for (const row of report.groundTruthCloseness.weakOrProxyTruthRows) {
    lines.push(
      `| ${row.language} | ${row.nativeTool} | ${row.risk} | ${row.coverageRatio ?? "n/a"} | ${row.minValidationCoverage ?? "n/a"} | ${row.reason} |`
    );
  }
  lines.push("", "### Core Matrix", "");
  lines.push("| Language | Native tools | Graphify | Equivalent rows | Graphify missing | Token ratio | Latency ratio |");
  lines.push("|---|---|---|--:|--:|--:|--:|");
  for (const row of report.groundTruthCloseness.coreMatrix) {
    lines.push(
      `| ${row.language} | ${row.nativeTools.map((tool) => `${tool.name}:${tool.status}`).join(", ")} | ${row.graphifyOk ? "ok" : "not ok"} | ${row.equivalentRows} | ${row.graphifyMissingRows} | ${row.tokenRatio ?? "n/a"} | ${row.latencyRatio ?? "n/a"} |`
    );
  }
  lines.push("", "## Stubs And Hallucination Audit", "");
  lines.push("### Found During Final Pass", "");
  for (const item of report.stubsAndHallucinationAudit.foundDuringFinalPass) {
    lines.push(`- ${item}`);
  }
  lines.push("", `No hidden synthetic-row finding: ${report.stubsAndHallucinationAudit.notFound}`, "");
  lines.push("### Missing Adapters", "");
  if (report.stubsAndHallucinationAudit.missingAdapters.length) {
    for (const item of report.stubsAndHallucinationAudit.missingAdapters) {
      lines.push(`- ${item.tool}: ${item.status}${item.note ? ` - ${item.note}` : ""}`);
    }
  } else {
    lines.push("- none");
  }
  lines.push("", "## Improvement Todos", "");
  for (const todo of report.improvementTodos) {
    lines.push(`- ${todo.priority}: ${todo.item}`);
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
  const liveBenchmarkTools = collectLiveBenchmarkTools(liveBenchmarks, toolVersions.live_benchmark_tools || []);
  const coverageAudit = buildCoverageAudit(graphifyDiscovery, liveBenchmarks);
  const saturationRows = buildSaturation(saturation);

  const dataset = {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    sourceLabel: "Atlas benchmark JSON artifacts from bench/",
    sourceArtifacts: artifacts,
    derivedArtifacts: [
      { name: "benchmark-data.json", path: "data/benchmark-data.json" },
      { name: "tenx-gap-report.json", path: "data/tenx-gap-report.json" },
      { name: "tenx-gap-report.md", path: "data/tenx-gap-report.md" },
      { name: "public-repo-validation-manifest.json", path: "data/public-repo-validation-manifest.json" },
      { name: "public-repo-validation-manifest.md", path: "data/public-repo-validation-manifest.md" },
      { name: "precision-evidence-manifest.json", path: "data/precision-evidence-manifest.json" },
      { name: "precision-evidence-manifest.md", path: "data/precision-evidence-manifest.md" },
      { name: "call-edge-evidence-manifest.json", path: "data/call-edge-evidence-manifest.json" },
      { name: "call-edge-evidence-manifest.md", path: "data/call-edge-evidence-manifest.md" },
      { name: "final-benchmark-audit-report.json", path: "data/final-benchmark-audit-report.json" },
      { name: "final-benchmark-audit-report.md", path: "data/final-benchmark-audit-report.md" },
    ],
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
        liveBenchmarkCount: liveBenchmarkTools.length,
        core: toolVersions.core_tools || [],
        liveBenchmarkTools,
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
    publicRepoValidation: readDataJSON("public-repo-validation-manifest.json"),
    precisionEvidence: readDataJSON("precision-evidence-manifest.json"),
    callEdgeEvidence: readDataJSON("call-edge-evidence-manifest.json"),
    saturation: saturationRows,
    caveats: [
      "Ratios are computed only where both Atlas and graphify returned comparable query rows.",
      saturationRows.length
        ? `${saturationRows.map((row) => row.language).join(", ")} have saturation evidence with zero graphify-equivalent query rows; no latency/token ratio is claimed for those rows.`
        : "The final pass has no live language with zero graphify-equivalent query rows; historical saturation rows are superseded by the refreshed live artifacts.",
      "Timings are one-machine benchmark snapshots, not production guarantees.",
      "Atlas and graphify expose different graph models, so coverage and precision fields are shown separately.",
      "The precision evidence manifest records sampled symbol/location matches and validation kind-count maps when raw artifacts expose them; it does not claim full precision for rows that remain count-only or proxy-denominator based.",
      "The call-edge evidence manifest records core receiver-typed calls and live call counts when raw artifacts expose them; it does not claim live receiver-type coverage where no receiver metric exists.",
      "10x target fields are strict for token and latency ratios; coverage is reported as native-definition parity/exceed and is not converted into a fabricated 10x accuracy multiplier.",
    ],
  };
  const finalAudit = buildFinalAuditReport(dataset);
  dataset.finalAudit = {
    markdownPath: "data/final-benchmark-audit-report.md",
    jsonPath: "data/final-benchmark-audit-report.json",
    summary: finalAudit.summary,
  };

  fs.writeFileSync(path.join(dataDir, "benchmark-data.json"), `${JSON.stringify(dataset, null, 2)}\n`);
  const tenXGap = buildTenXGap(dataset);
  fs.writeFileSync(path.join(dataDir, "tenx-gap-report.json"), `${JSON.stringify(tenXGap, null, 2)}\n`);
  fs.writeFileSync(path.join(dataDir, "tenx-gap-report.md"), renderTenXGapMarkdown(tenXGap));
  fs.writeFileSync(path.join(dataDir, "final-benchmark-audit-report.json"), `${JSON.stringify(finalAudit, null, 2)}\n`);
  fs.writeFileSync(path.join(dataDir, "final-benchmark-audit-report.md"), renderFinalAuditMarkdown(finalAudit));
  console.log(`Wrote data/benchmark-data.json and tenx gap reports from ${artifacts.length} JSON artifacts`);
}

main();
