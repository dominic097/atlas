"use strict";

const fs = require("fs");
const path = require("path");
const { parse, VERSION } = require("svelte/compiler");

const root = process.argv[2];
if (!root) {
  throw new Error("Usage: node scripts/native-baselines/svelte_stats.js <root>");
}

const stats = {
  files: 0,
  parsed_files: 0,
  parse_errors: 0,
  script_blocks: 0,
  functions: 0,
  variables: 0,
  definitions: 0,
  compiler_version: VERSION,
  parse_error_samples: [],
};

function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name !== "graphify-out" && entry.name !== "node_modules") {
        walk(full);
      }
      continue;
    }
    if (!entry.name.endsWith(".svelte")) {
      continue;
    }
    stats.files++;
    const source = fs.readFileSync(full, "utf8");
    try {
      parse(source, { filename: full });
      stats.parsed_files++;
    } catch (error) {
      stats.parse_errors++;
      if (stats.parse_error_samples.length < 8) {
        stats.parse_error_samples.push({
          path: path.relative(root, full),
          error: String(error.message || error).slice(0, 500),
        });
      }
    }
    const scripts = [...source.matchAll(/<script(?:\s[^>]*)?>([\s\S]*?)<\/script>/gi)].map(
      (match) => match[1] || ""
    );
    stats.script_blocks += scripts.length;
    for (const content of scripts) {
      stats.functions += (content.match(/^\s*function\s+[A-Za-z_$][\w$]*/gm) || []).length;
      stats.variables += (content.match(/^\s*(?:const|let)\s+[A-Za-z_$][\w$]*/gm) || []).length;
    }
  }
}

walk(root);
stats.definitions = stats.functions + stats.variables;
console.log(JSON.stringify(stats, null, 2));
