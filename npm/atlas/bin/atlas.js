#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

function platformTriple() {
  const platformMap = { darwin: "darwin", linux: "linux" };
  const archMap = { x64: "amd64", arm64: "arm64" };
  const os = platformMap[process.platform];
  const arch = archMap[process.arch];
  if (!os || !arch) {
    return null;
  }
  return `${os}-${arch}`;
}

const triple = platformTriple();
const packaged = triple
  ? path.join(__dirname, "..", "vendor", triple, process.platform === "win32" ? "atlas.exe" : "atlas")
  : "";
const binary = process.env.ATLAS_BINARY || (packaged && fs.existsSync(packaged) ? packaged : "atlas");
const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  const hint = process.env.ATLAS_BINARY
    ? `ATLAS_BINARY=${process.env.ATLAS_BINARY}`
    : "the packaged Atlas binary or atlas on PATH";
  console.error(`atlas npm wrapper could not execute ${hint}: ${result.error.message}`);
  process.exit(127);
}

if (result.signal) {
  console.error(`atlas terminated by signal ${result.signal}`);
  process.exit(1);
}

process.exit(result.status ?? 0);
