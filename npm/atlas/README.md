# atlas

Installs the native `atlas` CLI for local code intelligence.

Atlas runs locally and uses embedded SQLite by default:

```sh
npm install -g @dominic097/atlas
atlas version
atlas index .
atlas search "auth middleware"
```

For one-off use without a global install:

```sh
npx @dominic097/atlas version
```

The package downloads the matching GitHub Release archive for macOS/Linux
`amd64` or `arm64` during `postinstall` and exposes it as the `atlas` binary.
If the release repository is private, export `GITHUB_TOKEN` or `GH_TOKEN` with
release-asset read access before installing.

Set `ATLAS_BINARY=/path/to/atlas` to force the wrapper to use an existing binary.
Set `ATLAS_SKIP_DOWNLOAD=1` when packaging offline and placing the binary yourself.
