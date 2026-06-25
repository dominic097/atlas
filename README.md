# Aziron Atlas

**The deterministic code-intelligence layer — a live, org-wide code knowledge graph.**

Graphify gives you a static map of one folder you read. Atlas is a live,
org-wide, **deterministic** code-intelligence engine: it answers
impact / search / cross-repo / temporal / coverage questions over a real code
graph. It does **no LLM reasoning** — the agentic layer (root-cause analysis,
autonomous fix, PR review, risk-scored test selection) lives in **Aziron Pulse**,
which consumes Atlas via its SDK / API / MCP.

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
| **Hosted** | Postgres + durable queue | org-wide | cross-repo + temporal moat; consumed by Pulse |

The keystone is one `StorageDriver` interface with two implementations. Tier
selection is a one-line swap (`--db sqlite://...` vs `--db postgres://...`).

## The moats over graphify (intelligence axis)

1. **Cross-repo** blast radius via HTTP route-contract matching.
2. **Temporal** history via per-commit snapshots + delta indexing.
3. **Deeper graph + code-aware lexical**: symbol-granular call edges, the
   symbol↔test coverage map (facts), and BM25 + trigram search with
   camel/snake tokenization.

All deterministic and LLM-free. The **agentic moat** — graph-driven RCA,
autonomous fix, PR review, and risk-scored test *selection* — is layered on top
by **Pulse**, which calls Atlas for the underlying intelligence.

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

Apache-2.0 for the core (`LICENSE`). Hosted-only org features (multi-tenant
cross-repo, durable queue) under `internal/cloud/**` are BUSL-1.1 in the full
build. The agentic layer (RCA/fix/review) is **not** part of Atlas — it lives in
Pulse. See `docs/ARCHITECTURE.md`.
