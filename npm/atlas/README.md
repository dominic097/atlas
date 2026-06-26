# atlas

Installs the native `atlas` CLI for local code intelligence.

Atlas runs locally and uses embedded SQLite by default:

```sh
npx atlas version
npx atlas index .
npx atlas search "auth middleware"
```

The package downloads the matching GitHub Release archive for macOS/Linux
`amd64` or `arm64` during `postinstall` and exposes it as the `atlas` binary.
If the release repository is private, export `GITHUB_TOKEN` or `GH_TOKEN` with
release-asset read access before installing.

Set `ATLAS_BINARY=/path/to/atlas` to force the wrapper to use an existing binary.
Set `ATLAS_SKIP_DOWNLOAD=1` when packaging offline and placing the binary yourself.
