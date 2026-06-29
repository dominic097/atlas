# atlas releases

This repository publishes prebuilt `atlas` CLI release assets only.

Install surfaces:

- GitHub Releases: macOS and Linux archives plus Linux `.deb`, `.rpm`, and `.apk` packages
- Homebrew tap: `dominic097/homebrew-atlas`
- npm wrapper package: `@dominic097/atlas`

The command installed by every package surface is:

```sh
atlas
```

Smoke check:

```sh
atlas version
```

No source tree is maintained in this repository.

## Benchmark site

The Atlas benchmark dashboard is published with GitHub Pages:

https://dominic097.github.io/atlas/

The site is generated from benchmark JSON artifacts committed under `data/raw/`.
The browser loads `data/benchmark-data.json` at runtime and keeps raw artifact
links visible for auditability.
