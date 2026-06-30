# atlas releases

This repository publishes prebuilt `atlas` CLI release assets and the public
benchmark/product site.

Install surfaces:

- GitHub Releases: macOS and Linux archives plus Linux `.deb`, `.rpm`, and `.apk` packages
- Homebrew tap: `dominic097/homebrew-atlas`
- npm wrapper package: `@dominic097/atlas`

The command installed by every package surface is:

```sh
atlas
```

Benchmark check:

```sh
atlas version
```

Homebrew install:

```sh
brew install --cask dominic097/atlas/atlas
atlas version
```

npm install:

```sh
npm install -g @dominic097/atlas
atlas version
```

Linux archive install example:

```sh
curl -LO https://github.com/aziron-ai/atlas/releases/download/v0.1.21/atlas_0.1.21_linux_amd64.tar.gz
tar -xzf atlas_0.1.21_linux_amd64.tar.gz
sudo install -m 0755 atlas /usr/local/bin/atlas
atlas version
```

Basic local workflow:

```sh
atlas index . --reindex
atlas context --paths path/to/changed-file.go --query "review risk" --format json
atlas search "symbol or concept" --limit 10
atlas mcp --transport http --http 127.0.0.1:8765
```

Atlas uses embedded SQLite by default at `sqlite://./.atlas/atlas.db`; no server
is required for local indexing, context retrieval, or MCP usage.

No Atlas CLI source tree is maintained in this repository.

## Benchmark site

The Atlas benchmark dashboard is published with GitHub Pages:

https://aziron-ai.github.io/atlas/

The site is generated from benchmark JSON artifacts committed under `data/raw/`.
The browser loads `data/benchmark-data.json` at runtime and keeps raw artifact
links visible for auditability.

The site itself is a static React/Tailwind build. Source lives under `src/`, and
the generated GitHub Pages assets are committed under `assets/`.

```sh
npm install
npm run build
npm run test:site
```
