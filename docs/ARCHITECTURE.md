# Atlas Architecture

This document is the short map. The full implementation plan (operation catalog,
port manifest, per-surface specs) is produced separately.

## Status

This repo is a **compiling scaffold**. Every operation stub returns
`ErrNotImplemented`. `atlas status` is the one stub that returns real data
(tier + storage driver) so the surfaces have something honest to render. The
scaffold depends only on stdlib + cobra so `go build ./...` is clean and fast.

## Layered dependency direction (strict)

```
graph        (pure data model; depends on nothing internal)
  ^
store        (StorageDriver interface + sqlite/postgres impls)  -> graph
  ^
engine       (Engine interface + stub impl)                     -> store, graph
  ^
pkg/atlas    (public SDK facade, S4a)                           -> engine
  ^
cli (S1)  api (S2)  mcp (S3)  -> pkg/atlas / engine
cmd/atlas    (binary, S5)      -> cli
```

The core query/impact path never imports a vector store — that structural
boundary keeps semantic search optional and off the LLM path.

## The keystone: StorageDriver

`internal/store/store.go` defines one interface. Two impls:

- `internal/store/sqlite.go` — local tier, zero infra (real build: CGO
  mattn/go-sqlite3, WAL, single-writer).
- `internal/store/postgres.go` — hosted tier (real build: `-tags hosted`, sqlx +
  lib/pq, durable SKIP-LOCKED queue, org-wide scope).

Tier selection is `store.Open()` keyed on the DSN scheme — a one-line swap.

## The five surfaces

| Surface | Package | Entry |
|---------|---------|-------|
| S1 CLI | `internal/cli` | `cmd/atlas` |
| S2 HTTP | `internal/api` | `atlas serve` |
| S3 MCP | `internal/mcp` | `atlas mcp` |
| S4 SDK | `pkg/atlas` (library) + generated HTTP clients | `import` / `npm` / `pip` |
| S5 Runtime | the `atlas` binary + container image | `docker run ... atlas serve` |

All four request/response surfaces are thin adapters over the single `Engine`
interface in `internal/engine`, so behavior is shared, not re-implemented.

## What the real build adds (not in the scaffold)

- `internal/parser` — tree-sitter (CGO), symbol + **call-edge** extraction.
- `internal/lexical` — bleve BM25 + code-aware camel/snake tokenizer + trigram.
- `internal/index` — full/delta/reuse indexing, git clone/diff, snapshot GC.
- `internal/query` — graph algorithms (callers/refs/neighbors/path/impact/explain).
- `internal/analysis` — route-contract + cross-service analyzers.
- `internal/vectorstore` — optional, off by default, behind a separate interface.
- `internal/cloud` — BUSL-1.1 hosted-only RCA/fix/review/webhook.
