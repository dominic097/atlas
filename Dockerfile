# syntax=docker/dockerfile:1
#
# Atlas container image. Two notes drive the shape of this file:
#   1. Atlas needs CGO — the tree-sitter grammars are C, so the builder stage
#      keeps CGO_ENABLED=1 and relies on the gcc that ships in the golang image.
#   2. Atlas shells out to `git` at runtime (commit SHA resolution + `git diff`
#      for delta indexing), so the final image installs git + ca-certificates.
#
# Build metadata is injected via the same -X seams `atlas version` reads
# (main.Version / main.Commit / main.Date), passed in as --build-args. The
# release pipeline builds this image from source with buildx + QEMU (the
# `container` job in .github/workflows/release.yml) so the multi-arch image is
# a real CGO build, not a repackaged binary; goreleaser handles only the
# archives/checksums/SBOMs/signatures. This file is also a self-contained
# `docker build .` for local/dev images.

# ---- build stage --------------------------------------------------------------
FROM golang:1.25-bookworm AS build

ENV CGO_ENABLED=1 GOFLAGS=-trimpath
WORKDIR /src

# Cache module downloads independently of source churn.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN go build \
      -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE}" \
      -o /out/atlas ./cmd/atlas

# ---- runtime stage ------------------------------------------------------------
FROM debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/atlas /usr/local/bin/atlas

# Atlas selects its store from the --db DSN (default sqlite://./.atlas/atlas.db,
# relative to the working dir). /data is exposed as a volume for a durable index;
# point the index there to survive restarts, e.g.:
#   docker run -v $PWD:/work -v atlas-data:/data ghcr.io/.../atlas \
#       index --db sqlite:///data/atlas.db .
VOLUME /data
WORKDIR /work

ENTRYPOINT ["atlas"]
CMD ["--help"]
