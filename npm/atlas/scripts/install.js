"use strict";

const fs = require("fs");
const https = require("https");
const os = require("os");
const path = require("path");
const { execFileSync } = require("child_process");

const pkg = require("../package.json");

const platformMap = { darwin: "darwin", linux: "linux" };
const archMap = { x64: "amd64", arm64: "arm64" };

function fail(message) {
  console.error(`atlas postinstall: ${message}`);
  process.exit(1);
}

function requestOptions(url) {
  const token = process.env.GITHUB_TOKEN || process.env.GH_TOKEN;
  const headers = token
    ? {
        Authorization: `Bearer ${token}`,
        Accept: "application/octet-stream",
        "User-Agent": "atlas npm installer",
      }
    : {
        "User-Agent": "atlas npm installer",
      };
  return { headers };
}

function download(url, destination, redirects = 0) {
  if (redirects > 5) {
    fail(`too many redirects while downloading ${url}`);
  }
  https
    .get(url, requestOptions(url), (response) => {
      if ([301, 302, 303, 307, 308].includes(response.statusCode)) {
        response.resume();
        download(response.headers.location, destination, redirects + 1);
        return;
      }
      if (response.statusCode !== 200) {
        response.resume();
        fail(`download failed ${response.statusCode} for ${url}`);
      }
      const file = fs.createWriteStream(destination);
      response.pipe(file);
      file.on("finish", () => {
        file.close();
        extract(destination);
      });
    })
    .on("error", (error) => fail(error.message));
}

function extract(archive) {
  fs.mkdirSync(destinationDir, { recursive: true });
  execFileSync("tar", ["-xzf", archive, "-C", destinationDir, "atlas"], { stdio: "inherit" });
  fs.chmodSync(path.join(destinationDir, "atlas"), 0o755);
  fs.rmSync(archive, { force: true });
}

if (process.env.ATLAS_SKIP_DOWNLOAD === "1" || process.env.ATLAS_SKIP_DOWNLOAD === "true") {
  process.exit(0);
}

const goos = platformMap[process.platform];
const goarch = archMap[process.arch];
if (!goos || !goarch) {
  fail(`unsupported platform ${process.platform}/${process.arch}`);
}

const destinationDir = path.join(__dirname, "..", "vendor", `${goos}-${goarch}`);
const existing = path.join(destinationDir, "atlas");
if (fs.existsSync(existing)) {
  process.exit(0);
}

const version = pkg.version;
const asset = `atlas_${version}_${goos}_${goarch}.tar.gz`;
const baseURL =
  process.env.ATLAS_DOWNLOAD_BASE_URL ||
  `https://github.com/dominic097/atlas/releases/download/v${version}`;
const archive = path.join(os.tmpdir(), asset);

download(`${baseURL}/${asset}`, archive);
