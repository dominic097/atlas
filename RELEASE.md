# Cutting An Atlas Release

Releases are automated from semver tags:

```sh
git tag v0.1.0
git push origin v0.1.0
```

| Job | Produces |
|-----|----------|
| `goreleaser` | per-OS/arch binaries, `tar.gz` archives, Linux `.deb`/`.rpm`/`.apk` packages, `checksums.txt`, per-archive SBOMs, a keyless cosign signature over the checksums, the Homebrew cask, and the GitHub Release |
| `npm` | Configured to publish `atlas`, an npm wrapper that installs and runs the local native `atlas` binary from the GitHub Release, once package ownership and `NPM_TOKEN` are available |

## Release Repositories

- Release assets: `aziron-ai/atlas`
- Homebrew tap: `dominic097/homebrew-atlas`
- Homebrew install name: `dominic097/atlas/atlas`
- npm package name: `atlas`

The release-asset repository must exist before a tag is pushed. Homebrew and npm
both download native archives from that public release URL. Releases are pinned
to the public repository's `main` branch as `target_commitish`, because the
source build can run from a private repository while the public release assets
live in `aziron-ai/atlas`.

## Why A Cross-Compile Image

Atlas is a CGO binary because tree-sitter grammars and SQLite use C. The
`goreleaser` job runs inside `ghcr.io/goreleaser/goreleaser-cross:v1.25.0`,
which provides the linux gcc cross-compilers, osxcross for darwin CGO, cosign,
and syft. The per-target `CC` and `CXX` overrides in `.goreleaser.yaml` map each
`GOOS` and `GOARCH` to that image's toolchains.

Keep the image's Go-version tag in sync with the `go` directive in `go.mod`.

Atlas itself is local-first after install: the packaged `atlas` CLI defaults to
embedded SQLite at `sqlite://./.atlas/atlas.db`; no hosted server is required for
local indexing, search, impact, or MCP.

## Required Secrets

| Secret | Used by | Purpose |
|--------|---------|---------|
| `GITHUB_TOKEN` | `goreleaser` | default token when releases are published from the same repository |
| `ATLAS_RELEASE_TOKEN` | `goreleaser` | optional PAT with contents write access to `aziron-ai/atlas` when releasing from another repository |
| `HOMEBREW_TAP_TOKEN` | `goreleaser` | PAT with contents write access to `dominic097/homebrew-atlas` so GoReleaser can push `Casks/atlas.rb` |
| `NPM_TOKEN` | `npm` | npm automation token with publish rights for the `atlas` package |

Keyless cosign signing uses GitHub OIDC (`id-token: write` plus Fulcio/Rekor);
no signing key secret is needed.

## Current External Prerequisites

- `dominic097/homebrew-atlas` exists and is writable by the current GitHub user.
- `aziron-ai/atlas` must exist before the release workflow can publish
  clean public release assets.
- The exact npm package name `atlas` is already registered on npm. Publishing
  under that exact name requires owner access to the existing package plus an
  `NPM_TOKEN` automation token with publish rights.

## Validate Locally Before Tagging

```sh
go test ./...
make build
./bin/atlas version
go run github.com/goreleaser/goreleaser/v2@v2.11.2 check

docker run --rm -v "$PWD":/src -w /src \
  ghcr.io/goreleaser/goreleaser-cross:v1.25.0 \
  release --snapshot --clean --skip=publish,sign

cd npm/atlas
ATLAS_BINARY=../../bin/atlas npm run smoke
ATLAS_SKIP_DOWNLOAD=1 npm publish --dry-run --access public
```

## Verifying Signatures

```sh
cosign verify-blob \
  --certificate checksums.txt.pem --signature checksums.txt.sig \
  --certificate-identity-regexp '.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```
