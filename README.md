# Aziron Atlas

**A live, org-wide code knowledge graph that acts.**

Graphify gives you a static map of one folder you read. Atlas is a live,
org-wide code *brain* that answers impact/search/cross-repo/test questions and
acts on them — root-cause analysis, autonomous fix, PR review.

- Module: `github.com/MsysTechnologiesllc/aziron-atlas`
- Binary: `atlas`
- Go 1.23+

> This repository is currently a **compiling scaffold**: every operation stub
> returns `ErrNotImplemented`. It exists to lock the architecture across all
> five consumption surfaces. See `docs/ARCHITECTURE.md`.

## Two tiers, one core

| Tier | Storage | Infra | Use |
|------|---------|-------|-----|
| **Local** (default) | embedded SQLite | none | OSS adoption wedge; code never leaves the machine |
| **Hosted** | Postgres + queue + webhooks | org-wide | cross-repo moat + monetization |

The keystone is one `StorageDriver` interface with two implementations. Tier
selection is a one-line swap (`--db sqlite://...` vs `--db postgres://...`).

## The four moats over graphify

1. **Cross-repo** blast radius via HTTP route-contract matching.
2. **Temporal** history via per-commit snapshots + delta indexing.
3. **Test intelligence**: symbol→test coverage + predictive selection + CI gate.
4. **Agentic action**: graph-driven RCA / autonomous fix / PR review.

## The five consumption surfaces

| | Surface | Entry point |
|---|---------|-------------|
| **S1** | CLI | `atlas <verb>` |
| **S2** | HTTP API | `atlas serve` → `/api/v1/atlas/*` |
| **S3** | MCP | `atlas mcp` (stdio/http) → tools for LLM agents |
| **S4** | SDK | `import github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas` (library) + generated HTTP clients |
| **S5** | Runtime | one static binary + container image hosting all of the above |

## Install

```sh
# from source
go install github.com/MsysTechnologiesllc/aziron-atlas/cmd/atlas@latest

# (release) homebrew / npm wrapper
brew install msystechnologiesllc/atlas/atlas
npx @aziron/atlas
```

## Quickstart per surface

### S1 — CLI
```sh
atlas index .                       # parse + persist the graph (local SQLite)
atlas search "checkout cart"         # code-aware lexical search
atlas impact --paths svc/cart.go     # single-repo blast radius
atlas status                         # tier, storage driver, freshness
```

### S2 — HTTP API
```sh
atlas serve --addr :8083
curl localhost:8083/api/v1/status
curl "localhost:8083/api/v1/search?q=Checkout"
```

### S3 — MCP (LLM agents)
```sh
atlas mcp --transport stdio          # what editors spawn
atlas install skill --agent claude    # register Atlas as an MCP server/skill
```

### S4 — SDK (embed in-process)
```go
import "github.com/MsysTechnologiesllc/aziron-atlas/pkg/atlas"

eng, _ := atlas.New(ctx, atlas.WithSQLite("./.atlas/atlas.db"))
defer eng.Close()
res, _ := eng.Index(ctx, atlas.IndexInput{ProjectPath: "."})
```

### S5 — Runtime
```sh
docker run ghcr.io/msystechnologiesllc/aziron-atlas:latest serve --addr :8083
```

## Build

```sh
make build      # local binary
make test       # unit tests
go build ./...  # the whole scaffold compiles with stdlib + cobra only
```

## License

Apache-2.0 for the core (`LICENSE`). The hosted-only agentic moat under
`internal/cloud/**` is BUSL-1.1 in the full build. See `docs/ARCHITECTURE.md`.
