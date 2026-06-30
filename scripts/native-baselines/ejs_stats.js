"use strict";

const fs = require("fs");
const path = require("path");
const ejs = require("ejs");

const root = process.argv[2];
if (!root) {
  throw new Error("Usage: node scripts/native-baselines/ejs_stats.js <root>");
}

const stats = {
  files: 0,
  parsed_files: 0,
  parse_errors: 0,
  definition_counts: {
    template: 0,
    include: 0,
    function: 0,
    variable: 0,
  },
  definitions: 0,
  ejs_version: require("ejs/package.json").version,
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
    if (!entry.name.endsWith(".ejs")) {
      continue;
    }
    stats.files++;
    stats.definition_counts.template++;
    const source = fs.readFileSync(full, "utf8");
    try {
      ejs.compile(source, { filename: full });
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
    stats.definition_counts.include += [...source.matchAll(/include\s*\(\s*['"]([^'"]+)['"]/g)].length;
    stats.definition_counts.function += [...source.matchAll(/\bfunction\s+[A-Za-z_$][\w$]*\s*\(/g)].length;
    stats.definition_counts.variable += [...source.matchAll(/\b(?:const|let|var)\s+[A-Za-z_$][\w$]*/g)].length;
  }
}

walk(root);
stats.definitions = Object.values(stats.definition_counts).reduce((total, value) => total + value, 0);
console.log(JSON.stringify(stats, null, 2));
