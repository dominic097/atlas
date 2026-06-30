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
const outJson = path.resolve(repoRoot, argValue("--out-json", "data/graphify-support-manifest.json"));
const outMd = path.resolve(repoRoot, argValue("--out-md", "data/graphify-support-manifest.md"));

const FAMILY_TO_LANGUAGE = new Map([
  ["cpp/cuda", "cuda"],
  ["objective-c", "objc"],
  ["verilog/systemverilog", "verilog"],
  ["json config", "json"],
  ["terraform/hcl", "terraform"],
  ["byond dm", "byond"],
  ["dotnet project", "dotnet"],
  ["delphi/lazarus forms", "delphi"],
  ["shell", "bash"],
  ["groovy/gradle", "groovy"],
]);

const DETECTOR_EXTENSION_TO_LANGUAGE = new Map([
  [".ejs", "ejs"],
  [".ets", "ets"],
  [".r", "r"],
]);

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function languageForFamily(family) {
  return FAMILY_TO_LANGUAGE.get(family) || family;
}

function round(value, digits = 4) {
  const n = Number(value);
  if (!Number.isFinite(n)) return null;
  return Number(n.toFixed(digits));
}

function artifactPath(file) {
  return `data/raw/${file}`;
}

function liveArtifacts() {
  return fs
    .readdirSync(rawDir)
    .filter((file) => /^LIVE_.*_BENCHMARK\.json$/.test(file))
    .sort()
    .map((file) => {
      const raw = readJSON(path.join(rawDir, file));
      return {
        file,
        raw,
        language: raw.language || file.replace(/^LIVE_/, "").replace(/_BENCHMARK\.json$/, "").toLowerCase(),
      };
    });
}

function buildLiveRow(item, deterministicByLanguage, detectorByLanguage) {
  const queries = Array.isArray(item.raw.queries) ? item.raw.queries : [];
  const graphifyDetectorOnly = Boolean(item.raw.graphify_detector_only);
  const deterministic = deterministicByLanguage.get(item.language) || null;
  const detector = detectorByLanguage.get(item.language) || null;
  const graphifyEquivalentRows = queries.filter((query) => query.graphify_missing !== true).length;
  const atlasEquivalentRows = queries.filter((query) => query.atlas_missing !== true).length;
  const comparableRows = queries.filter((query) => query.graphify_missing !== true && query.atlas_missing !== true).length;
  const graphifyMissingRows = queries.filter((query) => query.graphify_missing === true).length;
  const supportType = graphifyDetectorOnly || detector ? "detector-only" : "deterministic-extractor";
  return {
    language: item.language,
    artifact: artifactPath(item.file),
    supportType,
    graphifyExtractor: deterministic?.extractor || detector?.extractor || null,
    graphifyExtensions: deterministic?.extensions || detector?.extension || null,
    graphifyDetectorOnly,
    totalQueryRows: queries.length,
    comparableRows,
    graphifyEquivalentRows,
    graphifyMissingRows,
    atlasEquivalentRows,
    graphifyMissingRatio: queries.length ? round(graphifyMissingRows / queries.length) : null,
    graphifyStatus: item.raw.graphify?.status || "unknown",
    graphifyOk: Boolean(item.raw.graphify?.ok),
    note:
      supportType === "detector-only"
        ? "Graphify detects this extension but discovery reports no deterministic _DISPATCH extractor."
        : graphifyMissingRows
          ? "Graphify has a deterministic extractor but some sampled query rows have no Graphify equivalent."
          : "Graphify has a deterministic extractor and all sampled query rows have a Graphify equivalent.",
  };
}

function renderMarkdown(report) {
  const lines = [];
  lines.push("# Graphify Support Manifest", "");
  lines.push(`Generated: ${report.generatedAt}`, "");
  lines.push(
    "This manifest separates Graphify deterministic extractor support from detector-only extension support and records sampled query rows with or without a Graphify equivalent."
  );
  lines.push("");
  lines.push("## Summary", "");
  lines.push(`- Graphify version: ${report.summary.graphifyVersion}`);
  lines.push(`- Dispatch count: ${report.summary.dispatchCount}`);
  lines.push(`- Deterministic discovery rows: ${report.summary.deterministicDiscoveryRows}`);
  lines.push(`- Detector-only extensions: ${report.summary.detectorOnlyExtensions}`);
  lines.push(`- Live artifacts: ${report.summary.liveArtifacts}`);
  lines.push(`- Live deterministic artifacts: ${report.summary.liveDeterministicArtifacts}`);
  lines.push(`- Live detector-only artifacts: ${report.summary.liveDetectorOnlyArtifacts}`);
  lines.push(`- Sampled query rows: ${report.summary.queryRows}`);
  lines.push(`- Sampled Graphify-equivalent rows: ${report.summary.graphifyEquivalentRows}`);
  lines.push(`- Sampled Graphify-missing rows: ${report.summary.graphifyMissingRows}`);
  lines.push("");
  lines.push("## Detector-Only Extensions", "");
  lines.push("| Extension | Language | Artifact |");
  lines.push("|---|---|---|");
  for (const row of report.detectorOnly) {
    lines.push(`| ${row.extension} | ${row.language} | \`${row.artifact || "n/a"}\` |`);
  }
  lines.push("");
  lines.push("## Live Support", "");
  lines.push("| Language | Support | Query rows | Graphify equivalent | Graphify missing | Detector-only | Note |");
  lines.push("|---|---|--:|--:|--:|---|---|");
  for (const row of report.live) {
    lines.push(
      `| ${row.language} | ${row.supportType} | ${row.totalQueryRows} | ${row.graphifyEquivalentRows} | ${row.graphifyMissingRows} | ${row.graphifyDetectorOnly ? "yes" : "no"} | ${row.note} |`
    );
  }
  lines.push("");
  return `${lines.join("\n")}\n`;
}

function main() {
  const discoveryPath = path.join(rawDir, "GRAPHIFY_LANGUAGE_DISCOVERY.json");
  if (!fs.existsSync(discoveryPath)) throw new Error(`Missing ${discoveryPath}`);
  const discovery = readJSON(discoveryPath);
  const deterministic = (discovery.rows || []).map(([family, extensions, extractor, atlasStatus]) => ({
    family,
    language: languageForFamily(family),
    extensions,
    extractor,
    atlasStatus,
    artifact: artifactPath("GRAPHIFY_LANGUAGE_DISCOVERY.json"),
  }));
  const deterministicByLanguage = new Map(deterministic.map((row) => [row.language, row]));
  const detectorOnly = (discovery.detector_only_code_extensions || []).map((extension) => {
    const language = DETECTOR_EXTENSION_TO_LANGUAGE.get(extension) || extension.replace(/^\./, "");
    return {
      extension,
      language,
      extractor: "detector only, no _DISPATCH extractor",
      artifact: artifactPath("GRAPHIFY_LANGUAGE_DISCOVERY.json"),
    };
  });
  const detectorByLanguage = new Map(detectorOnly.map((row) => [row.language, row]));
  const live = liveArtifacts().map((item) => buildLiveRow(item, deterministicByLanguage, detectorByLanguage));
  const summary = {
    graphifyVersion: discovery.version || "unknown",
    dispatchCount: Number(discovery.dispatch_count) || 0,
    deterministicDiscoveryRows: deterministic.length,
    detectorOnlyExtensions: detectorOnly.length,
    liveArtifacts: live.length,
    liveDeterministicArtifacts: live.filter((row) => row.supportType === "deterministic-extractor").length,
    liveDetectorOnlyArtifacts: live.filter((row) => row.supportType === "detector-only").length,
    queryRows: live.reduce((total, row) => total + row.totalQueryRows, 0),
    graphifyEquivalentRows: live.reduce((total, row) => total + row.graphifyEquivalentRows, 0),
    graphifyMissingRows: live.reduce((total, row) => total + row.graphifyMissingRows, 0),
    passed:
      deterministic.length > 0 &&
      detectorOnly.length === (discovery.detector_only_code_extensions || []).length &&
      live.every((row) => row.supportType === "deterministic-extractor" || row.supportType === "detector-only"),
  };
  const report = {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    source: "data/raw/GRAPHIFY_LANGUAGE_DISCOVERY.json and data/raw/LIVE_*_BENCHMARK.json",
    harness: "scripts/graphify-support-harness.mjs",
    summary,
    deterministic,
    detectorOnly,
    live,
  };

  fs.mkdirSync(path.dirname(outJson), { recursive: true });
  fs.writeFileSync(outJson, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(outMd, renderMarkdown(report));
  if (!summary.passed) {
    console.error("Graphify support manifest failed to classify deterministic and detector-only rows");
    process.exit(1);
  }
  console.log(
    `Graphify support: ${summary.deterministicDiscoveryRows} deterministic discovery rows, ${summary.detectorOnlyExtensions} detector-only extensions, ${summary.graphifyEquivalentRows}/${summary.queryRows} sampled equivalent rows`
  );
}

main();
