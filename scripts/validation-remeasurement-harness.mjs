#!/usr/bin/env node
"use strict";

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const STRUCTURED_LANGUAGES = new Set(["json", "markdown"]);
const EPSILON = 0.0001;

function argValue(name, fallback) {
  const idx = process.argv.indexOf(name);
  if (idx >= 0 && process.argv[idx + 1]) return process.argv[idx + 1];
  return fallback;
}

const rawDir = path.resolve(repoRoot, argValue("--raw-dir", "data/raw"));
const outJson = path.resolve(repoRoot, argValue("--out-json", "data/validation-remeasurement-manifest.json"));
const outMd = path.resolve(repoRoot, argValue("--out-md", "data/validation-remeasurement-manifest.md"));

function readJSON(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function round(value, digits = 4) {
  const n = Number(value);
  if (!Number.isFinite(n)) return null;
  return Number(n.toFixed(digits));
}

function repoSlug(url = "") {
  return String(url)
    .replace(/^https:\/\/github\.com\//, "")
    .replace(/\.git$/, "");
}

function targetRelPath(targetPath = "") {
  const marker = "/repo/";
  const idx = String(targetPath).indexOf(marker);
  if (idx >= 0) return String(targetPath).slice(idx + marker.length) || ".";
  return ".";
}

function localDir(language, repo, index) {
  const slug = repoSlug(repo.repo).replace("/", "__");
  return `work/${language}/${String(index + 1).padStart(2, "0")}-${slug}`;
}

function localTargetPath(language, repo, index) {
  const dir = localDir(language, repo, index);
  const target = targetRelPath(repo.target_path);
  return target === "." ? dir : `${dir}/${target}`;
}

function atlasReplayCommands(language, repo, index) {
  const dir = localDir(language, repo, index);
  const target = localTargetPath(language, repo, index);
  return [
    `git clone ${repo.repo} ${dir}`,
    `git -C ${dir} checkout ${repo.commit}`,
    `atlas index ${target} --db sqlite://${dir}/.atlas/atlas.db --reindex --json`,
  ];
}

function graphifyReplayCommands(language, repo, index) {
  const target = localTargetPath(language, repo, index);
  return [
    `rm -rf ${target}/graphify-out`,
    `(cd ${target} && uv tool run --from graphifyy graphify update .)`,
  ];
}

function classifyNativeTool(language, nativeTool, detectorOnly) {
  const tool = String(nativeTool || "").toLowerCase();
  if (STRUCTURED_LANGUAGES.has(language)) return "structured-format";
  if (detectorOnly) return "graphify-detector-only-proxy";
  if (tool.includes("source-counter") || tool.includes("directive-counter") || tool.includes("template-counter") || tool.includes("regex-counter")) {
    return "source-counter-proxy";
  }
  if (tool.includes("proxy") || tool.includes("project")) return "scope-proxy";
  if (tool.includes("tree-sitter")) return "tree-sitter-baseline";
  if (tool.includes("lsp") || tool.includes("analyzer") || tool.includes("roslyn") || tool.includes("clangd") || tool.includes("jdtls")) {
    return "compiler-or-lsp-baseline";
  }
  if (
    tool.includes("compiler") ||
    tool.includes("parser") ||
    tool.includes("tokenizer") ||
    tool.includes("ripper") ||
    tool.includes("sqlfluff") ||
    tool.includes("hcl")
  ) {
    return "parser-library-baseline";
  }
  if (tool === "bash-n") return "syntax-checker-proxy";
  return "other-baseline";
}

function toolRisk(language, toolClass, detectorOnly) {
  if (STRUCTURED_LANGUAGES.has(language)) return "structured";
  if (detectorOnly || toolClass === "graphify-detector-only-proxy") return "high";
  if (["source-counter-proxy", "scope-proxy", "syntax-checker-proxy"].includes(toolClass)) return "medium";
  return "low";
}

function hasRepoIdentity(repo) {
  return Boolean(repo?.repo && repo?.commit && repo?.target_path);
}

function hasNativeRemeasurementCommand(repo) {
  return Boolean(repo?.native_command || repo?.native_replay_command || repo?.native_replay_commands);
}

function validationStatus(raw, language) {
  const validation = raw.multi_repo_validation || null;
  const repos = Array.isArray(validation?.repos) ? validation.repos : [];
  const minimumRepos = Number(validation?.minimum_repos) || 3;
  const passedRows = repos.filter((repo) => repo.ok !== false && Number(repo.coverage_ratio) >= 1 - EPSILON);
  return {
    validation,
    repos,
    minimumRepos,
    passedRows,
    hasValidationBlock: Boolean(validation),
    validationPassed: Boolean(validation) && validation.status !== "failed" && repos.length >= minimumRepos && passedRows.length >= minimumRepos,
    isStructured: STRUCTURED_LANGUAGES.has(language),
  };
}

function validateArtifact(file) {
  const raw = readJSON(file);
  const language = raw.language || path.basename(file).replace(/^LIVE_/, "").replace(/_BENCHMARK\.json$/, "").toLowerCase();
  const nativeTool = raw.native_baseline?.tool || "";
  const toolClass = classifyNativeTool(language, nativeTool, Boolean(raw.graphify_detector_only));
  const risk = toolRisk(language, toolClass, Boolean(raw.graphify_detector_only));
  const status = validationStatus(raw, language);
  const warnings = [];
  const errors = [];

  if (!status.hasValidationBlock && !status.isStructured) {
    errors.push("missing multi_repo_validation block for code language");
  }
  if (status.hasValidationBlock && !status.validationPassed) {
    errors.push("multi_repo_validation does not meet minimum passing repo count");
  }

  const rows = status.repos.map((repo, index) => {
    const pinned = hasRepoIdentity(repo);
    const atlasReady = pinned;
    const graphifyReady = pinned;
    const nativeReady = hasNativeRemeasurementCommand(repo);
    const blockers = [];
    if (!pinned) blockers.push("missing repo/commit/target_path");
    if (!nativeReady) blockers.push("native_or_proxy_remeasurement_command_not_recorded");
    if (risk === "medium") blockers.push("proxy_denominator_not_full_semantic_truth");
    if (risk === "high") blockers.push("graphify_detector_only_or_weak_proxy_truth");
    blockers.push("full_symbol_name_kind_location_sets_not_persisted");
    return {
      repo: repo.repo || "",
      slug: repoSlug(repo.repo),
      commit: repo.commit || null,
      targetRelPath: targetRelPath(repo.target_path),
      atlasDefinitions: repo.atlas_definitions ?? null,
      nativeDefinitions: repo.native_definitions ?? null,
      coverageRatio: repo.coverage_ratio ?? null,
      ok: repo.ok !== false,
      note: repo.note || "",
      pinned,
      atlasReplayReady: atlasReady,
      graphifyReplayReady: graphifyReady,
      nativeRemeasurementReady: nativeReady,
      fullRemeasurementReady: atlasReady && graphifyReady && nativeReady,
      blockers,
      atlasReplayCommands: atlasReady ? atlasReplayCommands(language, repo, index) : [],
      graphifyReplayCommands: graphifyReady ? graphifyReplayCommands(language, repo, index) : [],
      nativeRemeasurementCommands: Array.isArray(repo.native_replay_commands)
        ? repo.native_replay_commands
        : repo.native_command || repo.native_replay_command
          ? [repo.native_command || repo.native_replay_command]
          : [],
    };
  });

  const ratios = rows.map((repo) => Number(repo.coverageRatio)).filter(Number.isFinite);
  const pinnedRows = rows.filter((repo) => repo.pinned).length;
  const atlasReplayReadyRows = rows.filter((repo) => repo.atlasReplayReady).length;
  const graphifyReplayReadyRows = rows.filter((repo) => repo.graphifyReplayReady).length;
  const nativeRemeasurementReadyRows = rows.filter((repo) => repo.nativeRemeasurementReady).length;
  const fullRemeasurementReadyRows = rows.filter((repo) => repo.fullRemeasurementReady).length;
  const languageBlockers = new Set(rows.flatMap((repo) => repo.blockers));
  if (status.isStructured) {
    languageBlockers.add("structured_format_outside_code_parser_gate");
  }
  if (!status.hasValidationBlock) {
    languageBlockers.add("no_public_repo_validation_rows");
  }
  if (status.hasValidationBlock && nativeRemeasurementReadyRows < rows.length) {
    warnings.push("Atlas and Graphify replay commands can be generated, but per-repo native/proxy commands are not persisted");
  }

  return {
    language,
    artifact: `data/raw/${path.basename(file)}`,
    isCodeLanguage: !status.isStructured,
    isStructured: status.isStructured,
    nativeTool,
    nativeCommandTemplate: raw.commands?.native_baseline || raw.native_baseline?.command || "",
    graphifyDetectorOnly: Boolean(raw.graphify_detector_only),
    toolClass,
    risk,
    validationStatus: status.hasValidationBlock
      ? status.validationPassed
        ? "passed"
        : "failed"
      : status.isStructured
        ? "structured-no-validation"
        : "missing",
    minimumRepos: status.minimumRepos,
    repoCount: rows.length,
    pinnedRows,
    atlasReplayReadyRows,
    graphifyReplayReadyRows,
    nativeRemeasurementReadyRows,
    fullRemeasurementReadyRows,
    fullRemeasurementReady: rows.length > 0 && fullRemeasurementReadyRows === rows.length,
    minCoverageRatio: ratios.length ? round(Math.min(...ratios)) : null,
    warnings,
    errors,
    blockers: [...languageBlockers].sort(),
    repos: rows,
  };
}

function renderMarkdown(report) {
  const lines = [];
  lines.push("# Validation Remeasurement Readiness Manifest", "");
  lines.push(`Generated: ${report.generatedAt}`, "");
  lines.push(
    "This manifest is regenerated from committed `data/raw/LIVE_*_BENCHMARK.json` artifacts by `scripts/validation-remeasurement-harness.mjs`."
  );
  lines.push(
    "It is a readiness audit, not a full remeasurement run: Atlas and Graphify replay commands are generated from pinned repo/commit/target rows, while native/proxy counters are marked ready only when the validation row stores an executable native replay command."
  );
  lines.push("");
  lines.push("## Summary", "");
  lines.push(`- Live artifacts: ${report.summary.artifacts}`);
  lines.push(`- Live code artifacts: ${report.summary.codeArtifacts}`);
  lines.push(`- Structured artifacts: ${report.summary.structuredArtifacts}`);
  lines.push(`- Validation artifacts: ${report.summary.validationArtifacts}`);
  lines.push(`- Validation repo rows: ${report.summary.repoRows}`);
  lines.push(`- Pinned repo rows: ${report.summary.pinnedRepoRows}`);
  lines.push(`- Atlas replay-ready rows: ${report.summary.atlasReplayReadyRows}`);
  lines.push(`- Graphify replay-ready rows: ${report.summary.graphifyReplayReadyRows}`);
  lines.push(`- Native/proxy remeasurement command-ready rows: ${report.summary.nativeRemeasurementReadyRows}`);
  lines.push(`- Full remeasurement-ready artifacts: ${report.summary.fullRemeasurementReadyArtifacts}`);
  lines.push(`- Proxy or detector-only code artifacts: ${report.summary.proxyOrDetectorCodeArtifacts}`);
  lines.push(`- Warnings: ${report.summary.warnings}`);
  lines.push(`- Errors: ${report.summary.errors}`);
  lines.push("");
  lines.push("## Language Readiness", "");
  lines.push("| Language | Tool class | Risk | Validation | Repos | Atlas replay | Graphify replay | Native ready | Blockers |");
  lines.push("|---|---|---|---|--:|--:|--:|--:|---|");
  for (const item of report.languages) {
    lines.push(
      `| ${item.language} | ${item.toolClass} | ${item.risk} | ${item.validationStatus} | ${item.repoCount}/${item.minimumRepos} | ${item.atlasReplayReadyRows} | ${item.graphifyReplayReadyRows} | ${item.nativeRemeasurementReadyRows} | ${item.blockers.join(", ") || "none"} |`
    );
  }
  lines.push("");
  lines.push("## Replay Command Example", "");
  const first = report.languages.flatMap((item) => item.repos.map((repo) => ({ language: item.language, repo }))).find(Boolean);
  if (first) {
    lines.push(`Language: ${first.language}; repo: ${first.repo.slug}`, "");
    lines.push("```sh");
    for (const command of first.repo.atlasReplayCommands) lines.push(command);
    for (const command of first.repo.graphifyReplayCommands) lines.push(command);
    lines.push("```", "");
  }
  lines.push("## Remaining Gap", "");
  lines.push(
    "A full remeasurement pass still needs executable native/proxy counter commands per validation repo, plus persisted Atlas and native symbol name/kind/location sets. Until those are present, the benchmark proves pinned artifact evidence and replay readiness, not a complete 99% precision oracle."
  );
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

  const languages = files.map(validateArtifact);
  const code = languages.filter((item) => item.isCodeLanguage);
  const errors = languages.flatMap((item) => item.errors.map((error) => `${item.language}: ${error}`));
  const warnings = languages.flatMap((item) => item.warnings.map((warning) => `${item.language}: ${warning}`));
  const report = {
    schemaVersion: 1,
    generatedAt: new Date().toISOString(),
    source: "data/raw/LIVE_*_BENCHMARK.json",
    harness: "scripts/validation-remeasurement-harness.mjs",
    summary: {
      artifacts: languages.length,
      codeArtifacts: code.length,
      structuredArtifacts: languages.filter((item) => item.isStructured).length,
      validationArtifacts: languages.filter((item) => item.repoCount > 0).length,
      repoRows: languages.reduce((total, item) => total + item.repoCount, 0),
      codeRepoRows: code.reduce((total, item) => total + item.repoCount, 0),
      pinnedRepoRows: languages.reduce((total, item) => total + item.pinnedRows, 0),
      atlasReplayReadyRows: languages.reduce((total, item) => total + item.atlasReplayReadyRows, 0),
      graphifyReplayReadyRows: languages.reduce((total, item) => total + item.graphifyReplayReadyRows, 0),
      nativeRemeasurementReadyRows: languages.reduce((total, item) => total + item.nativeRemeasurementReadyRows, 0),
      fullRemeasurementReadyRows: languages.reduce((total, item) => total + item.fullRemeasurementReadyRows, 0),
      fullRemeasurementReadyArtifacts: languages.filter((item) => item.fullRemeasurementReady).length,
      proxyOrDetectorCodeArtifacts: code.filter((item) => ["medium", "high"].includes(item.risk)).length,
      warnings: warnings.length,
      errors: errors.length,
      passed: errors.length === 0,
    },
    errors,
    warnings,
    languages,
  };

  fs.mkdirSync(path.dirname(outJson), { recursive: true });
  fs.writeFileSync(outJson, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(outMd, renderMarkdown(report));
  if (!report.summary.passed) {
    console.error(`Validation remeasurement readiness harness failed with ${errors.length} errors`);
    process.exit(1);
  }
  console.log(
    `Audited ${report.summary.repoRows} validation repo rows; Atlas replay-ready ${report.summary.atlasReplayReadyRows}, native command-ready ${report.summary.nativeRemeasurementReadyRows}`
  );
}

main();
