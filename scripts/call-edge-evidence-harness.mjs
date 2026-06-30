#!/usr/bin/env node
"use strict";

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

function argValue(name, fallback) {
  const idx = process.argv.indexOf(name);
  if (idx >= 0 && process.argv[idx + 1]) return process.argv[idx + 1];
  return fallback;
}

const rawDir = path.resolve(repoRoot, argValue("--raw-dir", "data/raw"));
const outJson = path.resolve(repoRoot, argValue("--out-json", "data/call-edge-evidence-manifest.json"));
const outMd = path.resolve(repoRoot, argValue("--out-md", "data/call-edge-evidence-manifest.md"));

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function round(value, digits = 4) {
  const n = Number(value);
  if (!Number.isFinite(n)) return null;
  return Number(n.toFixed(digits));
}

function numberOrNull(value) {
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

function edgeBreakdownCount(rows, kind) {
  if (!Array.isArray(rows)) return null;
  const row = rows.find((item) => item?.kind === kind);
  return numberOrNull(row?.count);
}

function liveAtlasCallCount(raw) {
  return (
    numberOrNull(raw.atlas?.index?.edge_kinds?.calls) ??
    edgeBreakdownCount(raw.atlas?.edge_breakdown, "calls") ??
    numberOrNull(raw.atlas?.index?.calls) ??
    numberOrNull(raw.atlas?.calls) ??
    null
  );
}

function liveAtlasEdgeCount(raw) {
  return numberOrNull(raw.atlas?.index?.edges) ?? numberOrNull(raw.atlas?.edges) ?? null;
}

function liveGraphifyCallCount(raw) {
  return numberOrNull(raw.graphify?.metrics?.calls) ?? numberOrNull(raw.graphify?.calls) ?? null;
}

function liveGraphifyEdgeCount(raw) {
  return numberOrNull(raw.graphify?.metrics?.links) ?? numberOrNull(raw.graphify?.metrics?.edges) ?? null;
}

function coreRows(matrixPath) {
  const matrix = readJSON(matrixPath);
  return matrix.map((row) => {
    const atlas = row.tools?.atlas?.metrics || {};
    const graphify = row.tools?.graphify?.metrics || {};
    const calls = numberOrNull(atlas.calls);
    const recvTyped = numberOrNull(atlas.recv_typed);
    const internalCalls = numberOrNull(atlas.internal_calls);
    return {
      language: row.language,
      artifact: "data/raw/MATRIX_REPORT.json",
      status: calls != null && recvTyped != null ? "receiver-typed" : calls != null ? "calls-only" : "missing",
      atlasCalls: calls,
      atlasInternalCalls: internalCalls,
      atlasReceiverTypedCalls: recvTyped,
      atlasReceiverTypedRatio: calls && recvTyped != null ? round(recvTyped / calls) : null,
      atlasCallSources: atlas.sources || null,
      atlasEdges: numberOrNull(atlas.edges),
      graphifyCalls: numberOrNull(graphify.calls),
      graphifyExtractedCalls: numberOrNull(graphify.extracted_calls),
      graphifyExtractedPct: numberOrNull(graphify.extracted_pct),
    };
  });
}

function liveRow(file) {
  const raw = readJSON(file);
  const language = raw.language || path.basename(file).replace(/^LIVE_/, "").replace(/_BENCHMARK\.json$/, "").toLowerCase();
  const atlasCalls = liveAtlasCallCount(raw);
  const graphifyCalls = liveGraphifyCallCount(raw);
  const graphifyDetectorOnly = Boolean(raw.graphify_detector_only);
  const receiverTyped = numberOrNull(raw.atlas?.index?.recv_typed ?? raw.atlas?.recv_typed ?? raw.atlas?.receiver_typed_calls);
  const status =
    atlasCalls == null
      ? "missing"
      : receiverTyped != null
        ? "receiver-typed"
        : "calls-only";
  return {
    language,
    artifact: `data/raw/${path.basename(file)}`,
    status,
    graphifyDetectorOnly,
    atlasCalls,
    atlasEdges: liveAtlasEdgeCount(raw),
    atlasReceiverTypedCalls: receiverTyped,
    atlasReceiverTypedRatio: atlasCalls && receiverTyped != null ? round(receiverTyped / atlasCalls) : null,
    graphifyCalls,
    graphifyEdges: liveGraphifyEdgeCount(raw),
    note:
      status === "receiver-typed"
        ? "Raw live artifact exposes receiver-typed calls."
        : status === "calls-only"
          ? "Raw live artifact exposes call counts but no receiver-type metric."
          : "Raw live artifact does not expose call-count evidence.",
  };
}

function renderMarkdown(report) {
  const lines = [];
  lines.push("# Call Edge Evidence Manifest", "");
  lines.push(`Generated: ${report.generatedAt}`, "");
  lines.push(
    "This manifest audits call-edge and receiver-type evidence already present in committed raw benchmark artifacts. It separates receiver-typed core rows from live rows that currently expose only call counts."
  );
  lines.push("");
  lines.push("## Summary", "");
  lines.push(`- Core matrix languages: ${report.summary.coreLanguages}`);
  lines.push(`- Core languages with receiver-typed calls: ${report.summary.coreReceiverTypedLanguages}`);
  lines.push(`- Core Atlas calls: ${report.summary.coreAtlasCalls}`);
  lines.push(`- Core Atlas receiver-typed calls: ${report.summary.coreAtlasReceiverTypedCalls}`);
  lines.push(`- Core receiver-typed ratio: ${report.summary.coreAtlasReceiverTypedRatio ?? "n/a"}`);
  lines.push(`- Live artifacts: ${report.summary.liveArtifacts}`);
  lines.push(`- Live artifacts with Atlas call counts: ${report.summary.liveArtifactsWithAtlasCalls}`);
  lines.push(`- Live artifacts with receiver-typed calls: ${report.summary.liveReceiverTypedArtifacts}`);
  lines.push(`- Live Atlas calls: ${report.summary.liveAtlasCalls}`);
  lines.push(`- Live Graphify calls: ${report.summary.liveGraphifyCalls}`);
  lines.push("");
  lines.push("## Core Matrix", "");
  lines.push("| Language | Status | Atlas calls | Receiver-typed | Receiver ratio | Internal calls | Graphify calls |");
  lines.push("|---|---|--:|--:|--:|--:|--:|");
  for (const row of report.core) {
    lines.push(
      `| ${row.language} | ${row.status} | ${row.atlasCalls ?? "n/a"} | ${row.atlasReceiverTypedCalls ?? "n/a"} | ${row.atlasReceiverTypedRatio ?? "n/a"} | ${row.atlasInternalCalls ?? "n/a"} | ${row.graphifyCalls ?? "n/a"} |`
    );
  }
  lines.push("", "## Live Artifacts", "");
  lines.push("| Language | Status | Atlas calls | Graphify calls | Detector-only Graphify | Note |");
  lines.push("|---|---|--:|--:|---|---|");
  for (const row of report.live) {
    lines.push(
      `| ${row.language} | ${row.status} | ${row.atlasCalls ?? "n/a"} | ${row.graphifyCalls ?? "n/a"} | ${row.graphifyDetectorOnly ? "yes" : "no"} | ${row.note} |`
    );
  }
  lines.push("");
  return `${lines.join("\n")}\n`;
}

function main() {
  const matrixPath = path.join(rawDir, "MATRIX_REPORT.json");
  if (!fs.existsSync(matrixPath)) throw new Error(`Missing ${matrixPath}`);
  const liveFiles = fs
    .readdirSync(rawDir)
    .filter((file) => /^LIVE_.*_BENCHMARK\.json$/.test(file))
    .sort()
    .map((file) => path.join(rawDir, file));
  if (!liveFiles.length) throw new Error(`No LIVE_*_BENCHMARK.json files found in ${rawDir}`);

  const core = coreRows(matrixPath);
  const live = liveFiles.map(liveRow);
  const coreAtlasCalls = core.reduce((total, row) => total + (row.atlasCalls || 0), 0);
  const coreAtlasReceiverTypedCalls = core.reduce((total, row) => total + (row.atlasReceiverTypedCalls || 0), 0);
  const liveAtlasCalls = live.reduce((total, row) => total + (row.atlasCalls || 0), 0);
  const liveGraphifyCalls = live.reduce((total, row) => total + (row.graphifyCalls || 0), 0);
  const summary = {
    coreLanguages: core.length,
    coreReceiverTypedLanguages: core.filter((row) => row.status === "receiver-typed").length,
    coreAtlasCalls,
    coreAtlasReceiverTypedCalls,
    coreAtlasReceiverTypedRatio: coreAtlasCalls ? round(coreAtlasReceiverTypedCalls / coreAtlasCalls) : null,
    liveArtifacts: live.length,
    liveArtifactsWithAtlasCalls: live.filter((row) => row.atlasCalls != null).length,
    liveReceiverTypedArtifacts: live.filter((row) => row.status === "receiver-typed").length,
    liveAtlasCalls,
    liveGraphifyCalls,
    passed: core.every((row) => row.status === "receiver-typed") && live.every((row) => row.atlasCalls != null),
  };
  const report = {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    source: "data/raw/MATRIX_REPORT.json and data/raw/LIVE_*_BENCHMARK.json",
    harness: "scripts/call-edge-evidence-harness.mjs",
    summary,
    core,
    live,
  };

  fs.mkdirSync(path.dirname(outJson), { recursive: true });
  fs.writeFileSync(outJson, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(outMd, renderMarkdown(report));
  if (!summary.passed) {
    console.error("Call-edge evidence harness found missing core receiver data or live call counts");
    process.exit(1);
  }
  console.log(
    `Call-edge evidence: ${summary.coreReceiverTypedLanguages}/${summary.coreLanguages} core receiver-typed, ${summary.liveArtifactsWithAtlasCalls}/${summary.liveArtifacts} live call-count artifacts`
  );
}

main();
