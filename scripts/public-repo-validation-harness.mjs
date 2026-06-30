#!/usr/bin/env node
"use strict";

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const CODE_MISSING_ALLOWED = new Set(["json"]);
const EPSILON = 0.0001;

function argValue(name, fallback) {
  const idx = process.argv.indexOf(name);
  if (idx >= 0 && process.argv[idx + 1]) return process.argv[idx + 1];
  return fallback;
}

const rawDir = path.resolve(repoRoot, argValue("--raw-dir", "data/raw"));
const outJson = path.resolve(repoRoot, argValue("--out-json", "data/public-repo-validation-manifest.json"));
const outMd = path.resolve(repoRoot, argValue("--out-md", "data/public-repo-validation-manifest.md"));

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

function replayCommands(row, repo, index) {
  const slug = repoSlug(repo.repo);
  const localDir = `work/${row.language}/${String(index + 1).padStart(2, "0")}-${slug.replace("/", "__")}`;
  const target = targetRelPath(repo.target_path);
  const targetPath = target === "." ? localDir : `${localDir}/${target}`;
  return [
    `git clone ${repo.repo} ${localDir}`,
    `git -C ${localDir} checkout ${repo.commit}`,
    `rm -rf ${localDir}/graphify-out`,
    `atlas index ${targetPath} --db sqlite://${localDir}/.atlas/atlas.db --reindex --json`,
  ];
}

function validateArtifact(file) {
  const raw = readJSON(file);
  const language = raw.language || path.basename(file).replace(/^LIVE_/, "").replace(/_BENCHMARK\.json$/, "").toLowerCase();
  const validation = raw.multi_repo_validation || null;
  const minimumRepos = Number(validation?.minimum_repos) || 3;
  const repos = Array.isArray(validation?.repos) ? validation.repos : [];
  const passedRepos = repos.filter((repo) => {
    return repo.ok !== false && Number(repo.coverage_ratio) >= 1 - EPSILON;
  });
  const ratios = repos.map((repo) => Number(repo.coverage_ratio)).filter(Number.isFinite);
  const warnings = [];
  const errors = [];

  if (!validation) {
    if (!CODE_MISSING_ALLOWED.has(language)) {
      errors.push("missing multi_repo_validation block");
    }
  } else {
    if (validation.status && validation.status !== "passed") errors.push(`validation status is ${validation.status}`);
    if (repos.length < minimumRepos) errors.push(`only ${repos.length}/${minimumRepos} validation repos recorded`);
    if (passedRepos.length < minimumRepos) errors.push(`only ${passedRepos.length}/${minimumRepos} repos pass coverage`);
    for (const [idx, repo] of repos.entries()) {
      if (!repo.repo || !repo.commit) errors.push(`repo row ${idx + 1} missing repo or commit`);
      if (!Number.isFinite(Number(repo.atlas_definitions))) errors.push(`repo row ${idx + 1} missing atlas_definitions`);
      if (!Number.isFinite(Number(repo.native_definitions))) errors.push(`repo row ${idx + 1} missing native_definitions`);
      if (!Number.isFinite(Number(repo.coverage_ratio))) errors.push(`repo row ${idx + 1} missing coverage_ratio`);
      if (repo.ok === false) errors.push(`repo row ${idx + 1} marked ok=false`);
    }
    if (!validation.selection_method) warnings.push("selection_method missing; fixed repo set is preserved but random sampling cannot be replayed exactly");
    if (!validation.measurement_order) warnings.push("measurement_order missing; native/Graphify replay order inferred from artifact notes");
  }

  const passed = errors.length === 0 && Boolean(validation) && repos.length >= minimumRepos && passedRepos.length >= minimumRepos;
  return {
    language,
    artifact: `data/raw/${path.basename(file)}`,
    status: passed ? "passed" : CODE_MISSING_ALLOWED.has(language) && !validation ? "structured-pending" : "failed",
    isCodeLanguage: !CODE_MISSING_ALLOWED.has(language),
    minimumRepos,
    repoCount: repos.length,
    passedRepoCount: passedRepos.length,
    minCoverageRatio: ratios.length ? round(Math.min(...ratios)) : null,
    generatedAt: validation?.generated_at || null,
    scope: validation?.scope || "",
    selectionMethod: validation?.selection_method || "",
    measurementOrder: validation?.measurement_order || "",
    warnings,
    errors,
    rejectedCandidates: validation?.rejected_candidates || [],
    repos: repos.map((repo, idx) => ({
      repo: repo.repo,
      slug: repoSlug(repo.repo),
      commit: repo.commit || null,
      targetRelPath: targetRelPath(repo.target_path),
      atlasDefinitions: repo.atlas_definitions ?? null,
      nativeDefinitions: repo.native_definitions ?? null,
      coverageRatio: repo.coverage_ratio ?? null,
      ok: repo.ok !== false,
      note: repo.note || "",
      replayCommands: replayCommands({ language }, repo, idx),
    })),
  };
}

function renderMarkdown(report) {
  const lines = [];
  lines.push("# Public Repo Validation Manifest", "");
  lines.push(`Generated: ${report.generatedAt}`, "");
  lines.push(
    "This manifest is regenerated from committed `data/raw/LIVE_*_BENCHMARK.json` artifacts by `scripts/public-repo-validation-harness.mjs`."
  );
  lines.push(
    "It validates the recorded public-repo evidence and preserves clone/checkout replay commands for every pinned validation repo."
  );
  lines.push("");
  lines.push("## Summary", "");
  lines.push(`- Live artifacts: ${report.summary.artifacts}`);
  lines.push(`- Code artifacts passed: ${report.summary.codePassed}/${report.summary.codeArtifacts}`);
  lines.push(`- Structured pending: ${report.summary.structuredPending}`);
  lines.push(`- Repo rows checked: ${report.summary.repoRows}`);
  lines.push(`- Warnings: ${report.summary.warnings}`);
  lines.push(`- Errors: ${report.summary.errors}`);
  lines.push("");
  lines.push("| Language | Status | Repos | Min coverage | Warnings | Artifact |");
  lines.push("|---|---|--:|--:|--:|---|");
  for (const item of report.languages) {
    lines.push(
      `| ${item.language} | ${item.status} | ${item.passedRepoCount}/${item.minimumRepos} | ${item.minCoverageRatio ?? "n/a"} | ${item.warnings.length} | \`${item.artifact}\` |`
    );
  }
  lines.push("");
  lines.push("## Replay Command Example", "");
  const first = report.languages.flatMap((item) => item.repos.map((repo) => ({ language: item.language, repo }))).find(Boolean);
  if (first) {
    lines.push(`Language: ${first.language}; repo: ${first.repo.slug}`, "");
    lines.push("```sh");
    for (const command of first.repo.replayCommands) lines.push(command);
    lines.push("```", "");
  }
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
    harness: "scripts/public-repo-validation-harness.mjs",
    summary: {
      artifacts: languages.length,
      codeArtifacts: code.length,
      codePassed: code.filter((item) => item.status === "passed").length,
      structuredPending: languages.filter((item) => item.status === "structured-pending").length,
      repoRows: languages.reduce((total, item) => total + item.repoCount, 0),
      warnings: warnings.length,
      errors: errors.length,
      passed: errors.length === 0 && code.every((item) => item.status === "passed"),
    },
    errors,
    warnings,
    languages,
  };

  fs.mkdirSync(path.dirname(outJson), { recursive: true });
  fs.writeFileSync(outJson, `${JSON.stringify(report, null, 2)}\n`);
  fs.writeFileSync(outMd, renderMarkdown(report));
  if (!report.summary.passed) {
    console.error(`Public repo validation harness failed with ${errors.length} errors`);
    process.exit(1);
  }
  console.log(`Validated ${report.summary.codePassed}/${report.summary.codeArtifacts} code artifacts; wrote ${path.relative(repoRoot, outJson)}`);
}

main();
