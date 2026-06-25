# Cutting an Atlas release

Releases are fully automated. Pushing a semver tag fans out into two GitHub
Actions jobs in [`.github/workflows/release.yml`](.github/workflows/release.yml):

```sh
git tag v0.1.0
git push origin v0.1.0
```

| Job | Produces |
|-----|----------|
| `goreleaser` | per-OS/arch binaries, `tar.gz` archives, `checksums.txt`, per-archive SBOMs (syft), a keyless cosign signature over the checksums, the Homebrew formula, and the GitHub Release |
| `container`  | multi-arch (`linux/amd64,linux/arm64`) image pushed to `ghcr.io/msystechnologiesllc/aziron-atlas`, with provenance + SBOM attestations and a keyless cosign image signature |

## Why a cross-compile image

Atlas is a **CGO** binary — the tree-sitter grammars are C and sqlite is C — so
cross-compilation needs real cross toolchains, not just `GOOS`/`GOARCH`. The
`goreleaser` job runs inside `ghcr.io/goreleaser/goreleaser-cross:v1.25.0`
(tagged by Go version, here Go 1.25.0), which bundles the linux gcc
cross-compilers, osxcross for darwin CGO, and cosign + syft. The per-target
`CC`/`CXX` overrides in [`.goreleaser.yaml`](.goreleaser.yaml) map each
`GOOS`/`GOARCH` onto that image's toolchains. Keep the image's Go-version tag in
sync with the `go` directive in `go.mod`.

The container image is built **separately from source** (buildx + QEMU against
the [`Dockerfile`](Dockerfile)) so it is a genuine CGO build per architecture,
not a repackaged linux binary.

## Required secrets

| Secret | Used by | Purpose |
|--------|---------|---------|
| `GITHUB_TOKEN` (auto) | both jobs | create the Release, push the GHCR image |
| `HOMEBREW_TAP_TOKEN`  | `goreleaser` | PAT with write access to `MsysTechnologiesllc/homebrew-atlas` (pushes `Formula/atlas.rb`) |

Keyless cosign signing uses GitHub OIDC (`id-token: write` + Fulcio/Rekor) — no
key secret is needed.

## Validate locally before tagging

```sh
# config schema (matches the version: 2 goreleaser in the cross image)
go run github.com/goreleaser/goreleaser/v2@latest check

# a full dry-run release into ./dist with no publish, in the same image CI uses
docker run --rm -v "$PWD":/src -w /src \
  ghcr.io/goreleaser/goreleaser-cross:v1.25.0 release --snapshot --clean

# the container image (build metadata flows through the -X seams atlas version reads)
docker build -t atlas:dev \
  --build-arg VERSION=v0.0.0-dev --build-arg COMMIT="$(git rev-parse --short HEAD)" .
docker run --rm atlas:dev version
```

## Verifying signatures (consumers)

```sh
# checksums signature (covers every archive via checksums.txt)
cosign verify-blob \
  --certificate checksums.txt.pem --signature checksums.txt.sig \
  --certificate-identity-regexp '.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# container image signature
cosign verify ghcr.io/msystechnologiesllc/aziron-atlas:<tag> \
  --certificate-identity-regexp '.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```
