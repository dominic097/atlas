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
const outJson = path.resolve(repoRoot, argValue("--out-json", "data/precision-evidence-manifest.json"));
const outMd = path.resolve(repoRoot, argValue("--out-md", "data/precision-evidence-manifest.md"));

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function round(value, digits = 4) {
  const n = Number(value);
  if (!Number.isFinite(n)) return null;
  return Number(n.toFixed(digits));
}

function textLines(value) {
  return Array.isArray(value) ? value.map(String) : [];
}

function hasAtlasLocation(text) {
  return /[@ ][A-Za-z0-9_./-]+:\d+\b/.test(text);
}

function hasGraphifyLocation(text) {
  return /\bSource:\s+\S.*\bL\d+\b/.test(text) || /\bloc=L\d+\b/.test(text);
}

function parseJSONMap(match) {
  if (!match) return null;
  try {
    return JSON.parse(match[1]);
  } catch {
    return null;
  }
}

function parseFirstMap(note, patterns) {
  for (const pattern of patterns) {
    const parsed = parseJSONMap(pattern.exec(note || ""));
    if (parsed) return parsed;
  }
  return null;
}

function parseKindMaps(note) {
  return {
    atlasCounts: parseFirstMap(note, [
      /\bAtlas(?:\s+declaration)?\s+counts\s+(\{[^}]+\})/i,
      /\bAtlas\s+also\s+reports\b.*?\bin\s+(\{[^}]+\})/i,
    ]),
    nativeCounts: parseFirstMap(note, [
      /\bnative(?:\s+counter|\s+declaration)?\s+counts\s+(\{[^}]+\})/i,
      /\bnative\s+counts\s+(\{[^}]+\})/i,
      /\bvalidation\s+denominator\s+counts\s+functions\/types\s+(\{[^}]+\})/i,
    ]),
  };
}

function hasKindMap(value) {
  return Boolean(value && typeof value === "object" && !Array.isArray(value) && Object.keys(value).length);
}

function sampleEvidence(metrics = {}) {
  const keys = [
    "sample_definitions",
    "sample_names",
    "sample_headings",
    "sample_keys",
    "sample_paths",
    "parse_error_samples",
  ];
  return keys
    .filter((key) => Array.isArray(metrics[key]))
    .map((key) => ({ key, count: metrics[key].length }));
}

function analyzeArtifact(file) {
  const raw = readJSON(file);
  const language = raw.language || path.basename(file).replace(/^LIVE_/, "").replace(/_BENCHMARK\.json$/, "").toLowerCase();
  const queryRows = [];
  for (const query of raw.queries || []) {
    if (query.atlas_missing || query.graphify_missing) continue;
    const symbol = String(query.symbol || "");
    const atlasText = textLines(query.atlas_stdout_head).join("\n");
    const graphifyText = textLines(query.graphify_stdout_head).join("\n");
    const atlasHasName = Boolean(symbol && atlasText.includes(symbol));
    const graphifyHasName = Boolean(symbol && graphifyText.includes(symbol));
    const atlasHasLocation = hasAtlasLocation(atlasText);
    const graphifyHasLocation = hasGraphifyLocation(graphifyText);
    queryRows.push({
      symbol,
      atlasHasName,
      graphifyHasName,
      atlasHasLocation,
      graphifyHasLocation,
      matchedNameAndLocation: atlasHasName && graphifyHasName && atlasHasLocation && graphifyHasLocation,
      atlasHead: textLines(query.atlas_stdout_head).slice(0, 3),
      graphifyHead: textLines(query.graphify_stdout_head).slice(0, 4),
    });
  }

  const validationRows = [];
  for (const repo of raw.multi_repo_validation?.repos || []) {
    const note = repo.note || "";
    const { atlasCounts, nativeCounts } = parseKindMaps(note);
    validationRows.push({
      repo: repo.repo,
      commit: repo.commit || null,
      hasKindCountEvidence: Boolean(nativeCounts && atlasCounts),
      nativeCounts,
      atlasCounts,
    });
  }

  const equivalentRows = queryRows.length;
  const matchedNameLocationRows = queryRows.filter((row) => row.matchedNameAndLocation).length;
  const validationKindRows = validationRows.filter((row) => row.hasKindCountEvidence).length;
  const nativeMetrics = raw.native_baseline?.metrics || {};
  const nativeMetricKindEvidence = {
    hasDefinitionCounts: hasKindMap(nativeMetrics.definition_counts),
    hasExpandedDefinitionCounts: hasKindMap(nativeMetrics.expanded_definition_counts),
    definitionCounts: nativeMetrics.definition_counts || null,
    expandedDefinitionCounts: nativeMetrics.expanded_definition_counts || null,
  };
  const hasNativeMetricKindEvidence =
    nativeMetricKindEvidence.hasDefinitionCounts || nativeMetricKindEvidence.hasExpandedDefinitionCounts;
  const status =
    matchedNameLocationRows > 0
      ? "sampled-name-location"
      : validationKindRows > 0 || hasNativeMetricKindEvidence
        ? "kind-count-only"
        : "count-only";

  return {
    language,
    artifact: `data/raw/${path.basename(file)}`,
    status,
    equivalentRows,
    matchedNameLocationRows,
    matchedNameLocationRatio: round(matchedNameLocationRows / equivalentRows),
    validationRows: validationRows.length,
    validationKindRows,
    nativeMetricKindEvidence,
    nativeSampleEvidence: sampleEvidence(nativeMetrics),
    gap:
      status === "sampled-name-location"
        ? ""
        : status === "kind-count-only"
          ? "No comparable query row currently proves both Atlas and Graphify returned the same symbol name with source locations; kind-count evidence is present."
          : "Raw artifact has coverage counts but no sampled name/location or native-vs-Atlas kind-count evidence.",
    queryRows,
    validationKindEvidence: validationRows,
  };
}

function renderMarkdown(report) {
  const lines = [];
  lines.push("# Precision Evidence Manifest", "");
  lines.push(`Generated: ${report.generatedAt}`, "");
  lines.push(
    "This manifest checks the precision evidence already present in raw live benchmark artifacts. It does not fabricate full precision; rows are marked `sampled-name-location`, `kind-count-only`, or `count-only` from observable artifact fields."
  );
  lines.push("");
  lines.push("## Summary", "");
  lines.push(`- Artifacts: ${report.summary.artifacts}`);
  lines.push(`- Sampled name/location evidence: ${report.summary.sampledNameLocationArtifacts}`);
  lines.push(`- Kind-count-only evidence: ${report.summary.kindCountOnlyArtifacts}`);
  lines.push(`- Count-only artifacts: ${report.summary.countOnlyArtifacts}`);
  lines.push(`- Equivalent query rows checked: ${report.summary.equivalentRows}`);
  lines.push(`- Query rows with both name and location: ${report.summary.matchedNameLocationRows}`);
  lines.push(`- Validation rows with native/Atlas kind maps: ${report.summary.validationKindRows}`);
  lines.push(`- Artifacts with native metric kind maps: ${report.summary.nativeMetricKindArtifacts}`);
  lines.push("");
  lines.push("| Language | Status | Query name+location | Validation kind rows | Native metric kind map | Gap |");
  lines.push("|---|---|--:|--:|---|---|");
  for (const item of report.languages) {
    lines.push(
      `| ${item.language} | ${item.status} | ${item.matchedNameLocationRows}/${item.equivalentRows} | ${item.validationKindRows}/${item.validationRows} | ${item.nativeMetricKindEvidence?.hasDefinitionCounts || item.nativeMetricKindEvidence?.hasExpandedDefinitionCounts ? "yes" : "no"} | ${item.gap || "none"} |`
    );
  }
  lines.push("");
  return `${lines.join("\n")}\n`;
}

function main() {
  const files = fs
    .readdirSync(rawDir)
    .filter((file) => /^LIVE_.*_BENCHMARK\.json$/.test(file))
    .sort()
    .map((file) => path.join(rawDir, file));
  if (!files.length) throw new Error(`No LIVE_*_BENCHMARK.json files found in ${rawDir}`);

  const languages = files.map(analyzeArtifact);
  const summary = {
    artifacts: languages.length,
    sampledNameLocationArtifacts: languages.filter((item) => item.status === "sampled-name-location").length,
    kindCountOnlyArtifacts: languages.filter((item) => item.status === "kind-count-only").length,
    countOnlyArtifacts: languages.filter((item) => item.status === "count-only").length,
    equivalentRows: languages.reduce((total, item) => total + item.equivalentRows, 0),
    matchedNameLocationRows: languages.reduce((total, item) => total + item.matchedNameLocationRows, 0),
    validationRows: languages.reduce((total, item) => total + item.validationRows, 0),
    validationKindRows: languages.reduce((total, item) => total + item.validationKindRows, 0),
    nativeMetricKindArtifacts: languages.filter(
      (item) => item.nativeMetricKindEvidence?.hasDefinitionCounts || item.nativeMetricKindEvidence?.hasExpandedDefinitionCounts
    ).length,
    passed: languages.some((item) => item.status === "sampled-name-location") && languages.some((item) => item.validationKindRows > 0),
  };
  const report = {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    source: "data/raw/LIVE_*_BENCHMARK.json",
    harness: "scripts/precision-evidence-harness.mjs",
    summary,
    languages,
  };

  fs.mkdirSync(path.dirname(outJson), { recursive: true });
  fs.writeFileSync(outJson, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(outMd, renderMarkdown(report));
  if (!summary.passed) {
    console.error("Precision evidence harness found no sampled name/location or kind-count evidence");
    process.exit(1);
  }
  console.log(
    `Precision evidence: ${summary.sampledNameLocationArtifacts} sampled-name/location artifacts, ${summary.validationKindRows} kind-count validation rows`
  );
}

main();
