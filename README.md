# Atlas

Deterministic local code intelligence for code review agents and developer
tooling.

Atlas builds a local code graph and answers search, impact, route, symbol,
coverage, and MCP context questions without sending source code to a hosted
service. The default storage engine is embedded SQLite at
`sqlite://./.atlas/atlas.db`.

- Binary: `atlas`
- Default mode: local static binary
- Storage: embedded SQLite by default, Postgres optional for hosted deployments
- Surfaces: CLI, HTTP API, MCP, SDK

## Install

```sh
# from the checked-out source
make build
./bin/atlas version

# after a release is published
brew install --cask dominic097/atlas/atlas
npx atlas version
```

Homebrew installs from the public `dominic097/homebrew-atlas` tap. The npm
package installs and runs the same native `atlas` binary.

## Quickstart

```sh
atlas index .                       # parse and persist the graph
atlas search "checkout cart"         # code-aware lexical search
atlas impact --paths svc/cart.go     # single-repo blast radius
atlas status                         # tier, storage driver, freshness
```

## HTTP API

```sh
atlas serve --addr :8083
curl localhost:8083/api/v1/status
curl "localhost:8083/api/v1/search?q=Checkout"
```

## MCP

```sh
atlas mcp --transport stdio
atlas install skill --agent claude
```

Editors and local agents can spawn `atlas mcp` and retrieve bounded context for
code review without first loading the whole repository into the model.

## SDK

The SDK exposes the same engine used by the CLI and API. It is intended for
in-process integrations that need deterministic code graph context without an
extra service hop.

## Build And Test

```sh
make build
make test
go test ./...
```

## Release Outputs

Tagged releases produce:

- macOS and Linux `atlas` archives for `amd64` and `arm64`
- Linux `.deb`, `.rpm`, and `.apk` packages
- checksums, SBOMs, and keyless cosign signatures
- a Homebrew cask in `dominic097/homebrew-atlas`
- an npm wrapper package named `atlas`

## License

Apache-2.0. See `LICENSE`.
