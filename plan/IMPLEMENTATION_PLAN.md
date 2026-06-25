# Aziron Atlas — Implementation Plan

**Module:** `github.com/MsysTechnologiesllc/aziron-atlas` · **CLI:** `atlas` · **Go:** 1.25 (CGO-required core) · **Status:** principal-level build plan, ready for engineering

---

## 1. Executive Summary, Thesis & What Is Locked

### 1.1 Thesis

> **graphify is a static map of one folder you read; Atlas is a live, org-wide code brain that ACTS.**

Atlas is a code knowledge-graph engine spun out of the Aziron Pulse code-intelligence subsystem. It must **strictly dominate** [graphify](https://github.com/safishamsi/graphify) (the local-first, single-folder code knowledge graph distributed as an AI-assistant skill + MCP server, built on tree-sitter, emitting a portable `graph.json` and exposing MCP tools like `query_graph` / `get_neighbors` / `shortest_path` / `get_pr_impact`).

graphify's structural ceilings — the gaps Atlas exists to close:

| graphify ceiling | Atlas answer |
|---|---|
| Single graph per folder (no cross-repo) | Org-wide multi-repo graph + HTTP route-contract matching |
| Point-in-time (no history) | Per-commit immutable snapshots + delta indexing + structural diff |
| Test-blind | Symbol→test coverage edges + predictive test selection + CI gate |
| Read-only (never acts) | Graph-driven RCA, autonomous fix, PR review hooks |

### 1.2 Two tiers off one shared Go core

| | **LOCAL tier** | **HOSTED tier** |
|---|---|---|
| Distribution | Single static-ish Go binary, zero infra | Org-wide service + container image |
| Graph store | Embedded SQLite | Postgres |
| Index queue | In-process (synchronous default) | Durable Postgres queue + git webhooks |
| Cross-repo / agentic | Single-DB only (degrades honestly) | Full moat (RCA/fix/review/webhooks) |
| Privacy | Code never leaves the machine | Org-managed |
| Strategic role | OSS adoption wedge | The moat + monetization |

### 1.3 What is locked (do not relitigate)

1. **Two tiers off one shared Go core**, selected by a one-line driver swap.
2. **Retrieval core = GRAPH + SYMBOL INDEX + CODE-AWARE LEXICAL SEARCH** (BM25 via bleve + trigram, camelCase/snake_case tokenization). This is the default path.
3. **Vector/semantic search is OPTIONAL, OFF BY DEFAULT, PLUGGABLE.** Never on the core MCP-to-LLM path. pgvector (hosted) / sqlite-vec / chromem-go (local) behind a `VectorStore` interface. Never a mandatory Qdrant dependency.
4. **Storage abstraction = `StorageDriver` interface, two impls (SQLite, Postgres).** This is the keystone of the spin-out. `VectorStore` is a separate optional interface.
5. **The four moats** graphify structurally lacks, all ported from Pulse: (a) cross-repo blast radius via HTTP route contracts; (b) temporal history via per-commit snapshots + delta indexing; (c) test intelligence (coverage map + predictive selection + CI gate); (d) agentic action (RCA / fix / review).
6. **Parsing = tree-sitter** (7 langs today: Go, Python, JS, TS, Java, C, C++; roadmap to graphify-parity 36). CGO is mandatory.

### 1.4 What this plan ADDS to the locked decisions (resolving the feasibility critique)

The port-effort table in the source manifest was wrong on three load-bearing items. This plan reclassifies them and budgets the real work explicitly (see §8 and §9):

- **Symbol-granular call edges DO NOT EXIST in Pulse.** `DependencyEdge` has `FromFile/ToRef/EdgeType` only; the tree-sitter parser emits no call edges. Every callers/path/impact/cross-repo claim depends on symbol-granular edges → **net-new build, sized L/XL, not "S/port".**
- **Stable `NodeID` identity DOES NOT EXIST.** Pulse keys delta carry-forward on `path`; symbol rows get fresh server-assigned IDs every snapshot. Temporal diff (`history`, `snapshot_diff`) and incremental search delta require a content-stable `node_id` column + UNIQUE key → **net-new.**
- **Code-aware lexical tokenizer DOES NOT EXIST.** Pulse uses bleve's default mapping + `NewQueryStringQuery`. The custom analyzer + trigram side-index → **net-new.**

These are the three highest-risk items and they gate the product's core value. The roadmap front-loads them.

---

## 2. Positioning vs graphify (parity + four moats)

| Capability | graphify | Atlas | Atlas surface |
|---|---|---|---|
| Single-folder graph: neighbors, shortest-path, `graph.json` | ✅ | ✅ (drop-in compatible) | `neighbors` `path` `graph_export` |
| Local-first zero-infra binary + MCP + skill installer | ✅ | ✅ (parity) | `atlas mcp`, `atlas install` |
| **Cross-repo blast radius (route contracts)** | ❌ | ✅ | `cross_repo_impact` `consumers` `route_contracts` |
| **Temporal history / structural snapshot diff** | ❌ | ✅ | `history` `snapshot_diff` |
| **Test selection + coverage map + CI gate** | ❌ | ✅ | `tests_for_change` `coverage` `impact --gate` |
| **Agentic RCA / fix / PR review** | ❌ | ✅ | `rca` `fix` `review` |

**Parity is table stakes; the four moats are the wedge.** Atlas must be a graphify drop-in (`atlas graph_export --format graphjson` produces a graphify-compatible `graph.json`; the same MCP tool surface plus `neighbors`/`path`) so a graphify user loses nothing by switching — then gains four capability classes graphify cannot structurally provide.

---

## 3. Locked Feature Set

### 3.1 Core (both tiers)

- **Index** — tree-sitter parse, extract symbols/imports/**call-edges**/doc-comments; persist graph + symbol index + lexical (BM25/trigram) index. Full first run, git-diff delta thereafter.
- **Code-aware lexical search** — BM25 (bleve) + trigram, identifier-splitting. The default retrieval path. No vectors required.
- **Symbol context, callers, refs, neighbors, path, impact, explain, graph_export** — the graph-query primitives.
- **History, snapshot_diff** — temporal moat.
- **tests_for_change, coverage** — test-intel moat (single-repo on local).
- **status, repos** — admin.

### 3.2 Hosted-only

- **cross_repo_impact, consumers, route_contracts** — cross-repo moat.
- **rca, fix, review** — agentic moat (write-capable; `internal/cloud`, BUSL-licensed).
- **link** — repo + webhook onboarding (local = path-add no-op, see §4).

### 3.3 Vector / semantic search — OPTIONAL, OFF BY DEFAULT

> **Locked decision #3, restated as an engineering constraint:** the packages `query`, `impact`, `analysis`, `lexical` **must not import `vectorstore`**. This is enforced by package boundaries, not convention. The default `VectorStore` is `NoopVectorStore` (`IsAvailable() == false`). Vectors earn a place only for: human NL search in the hosted console, cross-vocabulary discovery, and find-similar-code.

Surface exposure of semantic search is reconciled here (resolving the critique's "search mode" divergence):

- **`semantic_search` is a first-class canonical op** (added to the catalog, §6), distinct from `search`.
- **CLI / HTTP** expose it via `search --mode {lexical|semantic|hybrid}` (mode defaults to `lexical`; `semantic`/`hybrid` require vectors enabled, else a typed `vectors_disabled` error).
- **MCP** exposes it as a **separate gated tool** `semantic_search` that is only advertised when `VectorStore.IsAvailable()`. The `search` MCP tool has no `mode` arg.
- This mode↔tool mapping is **intentional, documented surface-shaping**, not drift: same canonical op, two ergonomic projections.

---

## 4. THE FIVE SURFACES (the heart)

All five surfaces are **thin adapters over one `Engine` facade** (`pkg/atlas`). No surface touches storage directly. This guarantees the catalog operations behave identically everywhere. Cross-surface naming and behavior are reconciled below per the critique punch list.

### 4.0 Cross-surface contracts (resolving the critique's naming/consistency issues)

These rules bind every surface. They exist once here and are referenced, never re-specified divergently.

#### 4.0.1 Repo addressing (one contract)

| Concept | Canonical field | Format | Notes |
|---|---|---|---|
| Machine id | `repo_id` | `repo_<slug>` string, **stable across snapshots and tiers** | NOT a bare UUID on the wire. The UUID is an internal storage detail; `repo_id` is the public, stable, human-stable-ish slug derived from `repo_full_name`. |
| Human name | `repo_full_name` | `org/name` (hosted) or basename (local) | The only human alias field name used in **every** output. |
| Role label | `role` | `origin` \| `consumer` \| `producer` | Replaces ad-hoc `origin_repo`/`consumer_repo` synonyms. Outputs carry `repo_id` + `role`, never a bespoke synonym. |

Resolution rules (documented once, in `pkg/atlas/config.go` + `docs/operations.md`): a CLI `--repo` accepting a path / basename / `org/name` / `repo_id` resolves to a single `repo_id`. Sentinels: `all` (every linked/indexed repo), `.` (CWD git root). Every surface uses the same resolver.

#### 4.0.2 Canonical error-code vocabulary (one enum, all surfaces)

Defined once in `pkg/apitypes` as `ErrorCode string`. Every surface maps to/from it.

| `ErrorCode` | HTTP status (problem+json `code`) | SDK sentinel | MCP `degraded.status` | CLI exit |
|---|---|---|---|---|
| `invalid_request` | 400 | `ErrInvalidRequest` | — (validation rejected pre-dispatch) | 2 |
| `unauthorized` | 401 | `ErrUnauthorized` | — | 7 |
| `forbidden` | 403 (`insufficient_scope`) | `ErrForbidden` | — | 7 |
| `not_found` | 404 | `ErrNotFound` | `not_found` | 3 |
| `tier_unavailable` | 404 | `ErrTierUnsupported` | `unsupported_tier` | 6 |
| `not_indexed` | 409 | `ErrNotIndexed` | `not_indexed` | 4 |
| `ambiguous_symbol` | 409 | `ErrAmbiguousSymbol` | `ambiguous_symbol` | 3 |
| `job_in_progress` | 409 | `ErrJobInProgress` | — | 5 |
| `vectors_disabled` | 422 | `ErrVectorsDisabled` | `vectors_disabled` | 6 |
| `unprocessable` | 422 | `ErrUnprocessable` | `unprocessable` | 2 |
| `invalid_cursor` | 400 | `ErrInvalidCursor` | `cursor_stale` | 2 |
| `payload_too_large` | 413 | `ErrPayloadTooLarge` | — | 2 |
| `rate_limited` | 429 | `ErrRateLimited` | — | 7 |
| `degraded` | 200 + `meta.degraded` | `ErrDegraded` (only when `--strict`) | `degraded` (in `_meta`) | 8 |
| `storage` | 503 / 500 | `ErrStorage` | `storage` | 5 |
| `internal` | 500 | `ErrInternal` | — | 1 |
| `timeout` | 504 | `context.DeadlineExceeded` | — | 124 |

**`degraded`/`unlinked` is represented on every surface** (the critique flagged HTTP/SDK gaps): HTTP returns `200` with `meta.degraded` + an `unlinked_dependencies[]` body block; SDK exposes `Result.Degraded` + (under `WithStrict`) `ErrDegraded`; MCP sets `_meta.degraded`; CLI exits `8` (and `1` under `--strict`).

#### 4.0.3 Async lifecycle (one model, mapped to the catalog)

The critique flagged that jobs/runs/webhooks were unmapped to the catalog. Resolution: add three **canonical sub-resource ops** to the catalog (§6), explicitly transport-level:

- `job` — `get` / `events` (SSE) / `cancel`. Index async lifecycle.
- `run` — `get` / `events` (SSE) / `cancel`. Agentic (rca/fix/review/link) lifecycle.
- `webhook` — git webhook ingestion (hosted).

`status` is extended with an optional `run_id` poll that returns the run/job payload. SDK `Job[T]` (`Status/Await/Events/Cancel`) maps 1:1 onto `run`/`job`. CLI `--run <id>` polls via `status`.

#### 4.0.4 Pagination & truncation (one policy per op-category)

- **List ops** (`search`, `callers`, `refs`, `neighbors`, `consumers`, `route_contracts`, `history`, `coverage`): cursor pagination. Cursor = HMAC-signed `{snapshot_id, last_sort_key, last_id}`, pinned to one snapshot. Stale cursor → `invalid_cursor`/`cursor_stale`. Per-op `limit` defaults: search 15–20, refs 200, neighbors 100, callers 100; hard max 500.
- **Fan-out ops** (`impact`, `cross_repo_impact`, `tests_for_change`): bounded by `depth`/`max_depth` + per-level cap, **not** cursor. Each emits an explicit `{total, returned, truncated}` trio on **every** array it returns (`impacted_symbols`, `impacted_files`, `impacted_tests`, `impacted` consumers, `selected_tests`). MCP additionally hard-caps every list server-side before marshal (default 200 items) and sets `_meta.truncated`.

#### 4.0.5 `graph_export` transport (reconciled across surfaces)

- `format` enum: `graphjson` (single `application/json` document) | `jsonl` (== `application/x-ndjson`, one node/edge per line). The examples use `jsonl`; the content-type is `application/x-ndjson`. These are the same thing, named consistently.
- **HTTP / SDK**: stream the body. `jsonl` → chunked `application/x-ndjson`. `graphjson` → buffered-but-whole `application/json` (a single document cannot be "chunked-as-objects"; it is streamed as bytes).
- **CLI**: writes to `--out` / stdout.
- **MCP**: `scope:subgraph` returns inline `{nodes,edges,meta}`; **`scope:full` returns a resource link** (`atlas://graph/{repo_id}/{snapshot_id}.json`) — never inline (a monorepo graph would blow the context window). The MCP handler **rejects** `scope:full` inline and **requires** `root` when `scope:subgraph` (cross-field validation enforced in the handler, since JSON Schema cannot express it).

#### 4.0.6 Multi-tenancy (one enforcement boundary)

The critique flagged that HTTP claims structural tenant enforcement but the `StorageDriver` interface only threads `scope` into some methods. **Resolution: tenancy is enforced at the snapshot-resolution boundary, and this is made structural.**

- Every `StorageDriver` graph-read method keys on `snapshot_id`. **A `snapshot_id` is tenant-bound by construction** — it is only ever produced by `LatestSnapshot`/`FindSnapshotByCommit`/`ListSnapshots`, all of which **do** take `scope`. Once you hold a tenant-scoped `snapshot_id`, every downstream read is implicitly tenant-correct.
- The HTTP `TenantScope` middleware wraps the request's driver so repo/snapshot resolution **cannot** be called without the bound tenant. A cross-tenant `repo_id` resolves to `not_found` (never `forbidden` — don't leak existence).
- This is documented as the contract in `docs/storage-driver.md`; the storage contract test asserts it (a snapshot from tenant A is invisible to a tenant-B-scoped resolve).
- **Local tier**: `scope` is the nil sentinel; all filters are no-ops.

#### 4.0.7 Local-tier behavior of cross-repo / link (reconciled)

| Op | Local behavior | Rationale |
|---|---|---|
| `cross_repo_impact`, `consumers`, `route_contracts` | **Honest-empty, not error.** Matching runs across all repos in the single local DB; if only one repo is indexed, results are empty (never fabricated). `_meta.degraded` notes single-repo scope. | A local user who indexes sibling folders into one `.atlas` DB *can* get cross-repo matches; capability is `CrossScope` (org webhooks/scale), not a hard 404. |
| `rca`, `fix`, `review` | `tier_unavailable` (these require the BUSL `internal/cloud` runners, not compiled into the local binary). | Genuine tier boundary. |
| `link` | **Path-add no-op**, returns `{repo_id, linked:true, webhook_registered:false}`. **NOT** a `tier_unavailable` 404. | Carve-out from the hosted-only-404 rule, explicitly. |

> This corrects the SDK-vs-Core contradiction: cross-repo/consumers/route_contracts on local are **honest-empty**, not `ErrTierUnsupported`. Only `rca`/`fix`/`review` are tier errors.

---

### 4.1 Surface S1 — CLI (`internal/cli`)

cobra root, one command per canonical op, uniform output/exit contract.

**Output modes** (one `render(T, format)` dispatcher; no per-command printers):

| `--format` | Default when | Shape |
|---|---|---|
| `table` | stdout is a TTY | aligned columns, ANSI color, summary footer |
| `plain` | stdout is NOT a TTY | tab-separated, no headers/color (pipe-friendly) |
| `json` | explicit / `--json` | one envelope object (catalog `outputs` verbatim) |
| `ndjson` | explicit / `--ndjson` | one row per line + trailing `{"_meta":...}` |

**stdout discipline:** payload to `cmd.OutOrStdout()`, all prose/progress/prompts to `cmd.ErrOrStderr()`. So `atlas search foo --json > out.json` yields a clean file.

**Result envelope (json):**
```jsonc
{ "op":"impact", "tier":"local", "repo_id":"repo_aziron-ui",
  "result": { /* catalog outputs */ },
  "meta": { "duration_ms":142, "snapshot_id":"snap_…", "commit_sha":"…",
            "schema_version":"atlas-1", "degraded":null, "request_id":"req_…",
            "warnings":[] } }
```

**Exit codes** — per the canonical table (§4.0.2). Gate semantics: `0` = pass, `1` = **block** (gate verdict), `8` = degraded, `--strict` promotes `8`→`1`.

**Command tree (1:1 with catalog):**
```
atlas index | search | symbol | callers | refs | neighbors | path | impact
            | explain | export | history | snapshot {list,diff}
            | tests-for-change (tfc) | coverage | status | repos
            | cross-repo {impact,consumers,contracts}        # hosted/honest-empty local
            | rca | fix | review                            # hosted
            | link | serve | mcp | install {hook,skill,completion,mcp-config}
            | config {init,get,set,view,path} | version
```

**Persistent flags:** `--db <dsn>` (`sqlite://`|`postgres://`, scheme selects tier), `--tier`, `--repo`, `--config`, `--format`/`--json`/`--ndjson`/`--compact`, `--color`, `--vectors`, `--vector-backend`, `--token`, `--server <url>` (thin HTTP client mode), `--timeout`, `-q/-v/-vv`, `--strict`, `--no-progress`.

**Config precedence:** flag → env (`ATLAS_*`) → config file (`.atlas/config.yaml`, viper-discovered) → default. **Zero-config = working local engine** (SQLite at `./.atlas/atlas.db`, lexical on, vectors off, sync indexing, no auth).

**Headline CI commands:**
```bash
# blast-radius gate
git diff origin/main... | atlas impact --diff - --gate --max-files 25 --json
# predictive test selection → runner
git diff origin/main... | atlas tfc --diff - --print runner | xargs go test -run
# local git-hook gate (brings the hosted PR-gate to local, zero server)
atlas install hook --type pre-push --max-files 40
```

---

### 4.2 Surface S2 — HTTP API (`internal/api`)

REST for the hosted tier and remote clients. `gorilla/mux`. Handlers thin; logic in `Engine`. Base path `/api/v1`. All JSON `snake_case`.

**Route table (B=both, H=hosted; H routes return `tier_unavailable` 404 on a local server — except `link`, §4.0.7):**

| Op | Method | Path | Tier |
|---|---|---|---|
| index | POST | `/repos/{repo_id}/index` | B |
| job (poll) | GET | `/jobs/{job_id}` | B |
| job (events) | GET | `/jobs/{job_id}/events` (SSE) | B |
| search | GET | `/search` | B |
| symbol | GET | `/symbols/{symbol_id}` ; `/repos/{repo_id}/symbols?name=` | B |
| callers | GET | `/symbols/{symbol_id}/callers` | B |
| refs | GET | `/symbols/{symbol_id}/references` | B |
| neighbors | GET | `/graph/neighbors` | B |
| path | GET | `/graph/path` | B |
| impact | POST | `/repos/{repo_id}/impact` | B |
| explain | POST | `/explain` | B |
| graph_export | GET | `/repos/{repo_id}/graph` | B |
| cross_repo_impact | POST | `/repos/{repo_id}/cross-repo-impact` | H* |
| consumers | GET | `/repos/{repo_id}/consumers` | H* |
| route_contracts | GET | `/route-contracts` | H* |
| history | GET | `/history` | B |
| snapshot_diff | GET | `/repos/{repo_id}/snapshots/diff` | B |
| tests_for_change | POST | `/repos/{repo_id}/tests-for-change` | B |
| coverage | GET | `/coverage` | B |
| rca | POST | `/repos/{repo_id}/rca` | H |
| fix | POST | `/repos/{repo_id}/fix` | H |
| review | POST | `/repos/{repo_id}/review` | H |
| run (poll/events/cancel) | GET/GET/DELETE | `/runs/{run_id}` `/runs/{run_id}/events` `/runs/{run_id}` | H |
| status | GET | `/status` | B |
| repos | GET | `/repos` ; `/repos/{repo_id}` | B |
| link | POST | `/repos` | B (path-add on local) |
| unlink | DELETE | `/repos/{repo_id}` | H |
| webhook | POST | `/webhooks/github` | H (HMAC, no bearer) |
| infra | GET | `/healthz` `/readyz` `/metrics` `/openapi.json` `/docs` | B |

\* honest-empty on local single-repo, not a 404 (§4.0.7).

**Action-route rationale:** `impact`/`cross_repo_impact`/`tests_for_change`/`explain`/`review` accept diffs/symbol arrays too large for a query string → `POST` to a sub-resource, declared **idempotent & safe to retry**.

**Envelope:**
```jsonc
{ "data": <op-specific>,
  "meta": { "request_id":"req_…", "tier":"hosted", "snapshot_id":"…",
            "commit_sha":"…", "schema_version":"atlas-1", "mode_used":"lexical",
            "degraded": null,
            "page": { "next_cursor":"…", "limit":20, "has_more":true } } }
```

**Auth** (pluggable `Authenticator`): API keys (`atlas_sk_live_…`, argon2id-hashed, scoped `tenant_id`+`scopes`+`repos[]`+`expires_at`); JWT/SSO (HS256 shared-secret or RS256/JWKS OIDC, claims `sub`/`tenant_id`/`exp`); service tokens (M2M, `kind:service`); webhook HMAC (`X-Hub-Signature-256`, constant-time). Local default = `NoopAuthenticator` (loopback) or `--api-token-file`.

**RBAC scopes:** `read` ⊂ `impact` ⊂ … ; `write`; `agent` (rca/fix/review, separately gated — opens PRs); `admin`. Middleware `RequireScope` → `403 insufficient_scope` + `WWW-Authenticate`.

**Error model:** RFC 9457 `application/problem+json` with the canonical `code` (§4.0.2) + `request_id`.

**Streaming (SSE):** `/jobs/{id}/events` (index progress) and `/runs/{id}/events` (agentic). Named events `progress`/`stage`/`done`/`error`; `: ping` heartbeat 15s; `Last-Event-ID` resume from a bounded ring buffer.

**Idempotency:** `Idempotency-Key` on `index`/`link`/`rca`/`fix`/`review` (24h store, verbatim replay + `Idempotency-Replayed: true`). Natural dedupe: `index` by `(repo_id, commit_sha)`; webhooks by `X-GitHub-Delivery`.

**Rate limiting:** token-bucket per `(tenant, principal, route-class)`. Classes `read`/`impact`/`agent`/`write`; `RateLimit-*` headers + `Retry-After`; `agent` adds a concurrency cap.

**Middleware order:** `RequestID → Recover → Logger(zap) → CORS → Authenticate → TenantScope → RequireScope → RateLimit → BodyLimit → Idempotency → Handler`. `TenantScope` is the security keystone (§4.0.6).

**OpenAPI:** source of truth = Go DTOs in `pkg/apitypes`; spec generated via `kin-openapi` builders (typed control over problem+json + SSE media types). `GET /openapi.json` (3.1) + `/docs`. The spec drives the S4 client SDKs (one spec → 3 clients). CI `atlas openapi --check` fails on drift; `oasdiff` gates breaking changes.

---

### 4.3 Surface S3 — MCP (`internal/mcp`)

Exposes graph/search/impact/temporal/test/agentic as MCP tools to Claude Code / Cursor / Codex / Gemini / Copilot. Ported from Pulse `handlers/mcp.go`, stripped of Pulse-specific tools (`datasource_introspect`, canvas). Protocol `2025-06-18` (advertise `2024-11-05` fallback).

**Architecture: one handler, two backends, three transports.**

```
stdio | streamable-http | legacy-SSE  →  mcp.Server  →  ToolBackend
                                                          ├─ LocalBackend  (in-proc *atlas.Engine)
                                                          └─ HostedBackend (HTTP → atlas serve, JWT)
```

**`ToolBackend` seam — the upgrade funnel keystone.** The Server never knows which backend it holds; tool names/schemas/results are identical. `LocalBackend.Has(op)` returns `false` for the hosted moats (`rca`/`fix`/`review`) and for `semantic_search` unless vectors are enabled; cross-repo ops are **present but honest-empty** locally (§4.0.7). The capability handshake at `initialize` advertises only supported tools — so a local user does not *see* `rca`/`fix`/`review`, and the moment they upgrade, the tools appear. **That visible appearance is the funnel.**

**Design principles (LLM-optimized):** stable `symbol_id`/`repo_id` everywhere; token-aware defaults (small `limit`, snippets ≤3 lines / 240 chars unless `include_source`); **server-side hard cap before marshal** (default 200 items) + `_meta.truncated`; compact `json.Marshal` (not Indent); cursor pagination; graceful degrade (`isError:false` + `_meta.degraded` + `hint`); MCP `annotations` (`readOnlyHint` etc.) so agents auto-run vs confirm.

**`_meta` envelope on every result:** `{tier, snapshot_id, repo_id, returned, total, cursor, truncated, duration_ms, degraded}`.

**Transports:** `atlas mcp` (stdio, default — **stderr-only logging**, stdout is the protocol channel); `atlas mcp --http` (Streamable HTTP, `POST /mcp` + `Mcp-Session-Id`); `atlas mcp --sse` (legacy, ported handshake + 25s keepalive + the **long-lived `conn.ctx` tool-call fix** from Pulse).

**Tool catalog** (bare verbs, no `code_` prefix; 1:1 with canonical ops):
- Core/both: `search`, `symbol`, `callers`, `refs`, `neighbors`, `path`, `impact`, `explain`, `graph_export`, `history`, `snapshot_diff`, `tests_for_change`, `coverage`, `status`, `repos`, `index`.
- Cross-repo (advertised on hosted; honest-empty local): `cross_repo_impact`, `consumers`, `route_contracts`.
- Agentic (hosted + `AllowWrite`; `readOnlyHint:false`, `fix`/`review` `post:true` → `destructiveHint:true`): `rca`, `fix`, `review`.
- Optional, gated: `semantic_search` (advertised only when `VectorStore.IsAvailable()`).
- Admin: `link` (local path-add no-op).

**Resources** (`atlas://repo/...`, `atlas://graph/{repo}/{snapshot}.json`, `atlas://symbol/{id}/source`, subscribable `atlas://snapshot/{repo}/latest` → `notifications/resources/updated` on webhook reindex — the temporal moat surfaced to MCP).

**Prompts** (recipes that teach the moat call-order): `triage_failure`, `safe_to_change`, `pr_gate`, `understand_symbol`.

**`graph_export` in MCP:** `scope:full` → resource link (rejected inline by the handler); `scope:subgraph` requires `root` (handler cross-field check).

---

### 4.4 Surface S4 — SDK (two halves)

#### S4(a) — Embeddable Go library (`pkg/atlas`)

The engine itself, linked in-process. Owns the `StorageDriver`, parser (CGO), bleve. Go only. Code never leaves the box.

```go
package atlas

type Engine interface {
    // core (both)
    Index(ctx, IndexInput) (*IndexResult, error)
    Search(ctx, SearchInput) (*SearchResult, error)
    Symbol(ctx, SymbolInput) (*SymbolResult, error)
    Callers(ctx, CallersInput) (*CallersResult, error)
    Refs(ctx, RefsInput) (*RefsResult, error)
    Neighbors(ctx, NeighborsInput) (*NeighborsResult, error)
    Path(ctx, PathInput) (*PathResult, error)
    Impact(ctx, ImpactInput) (*ImpactResult, error)
    Explain(ctx, ExplainInput) (*ExplainResult, error)
    GraphExport(ctx, GraphExportInput, io.Writer) (*GraphExportMeta, error) // streams
    SemanticSearch(ctx, SearchInput) (*SearchResult, error)                 // ErrVectorsDisabled if off
    // cross-repo (honest-empty on local)
    CrossRepoImpact(ctx, CrossRepoImpactInput) (*CrossRepoImpactResult, error)
    Consumers(ctx, ConsumersInput) (*ConsumersResult, error)
    RouteContracts(ctx, RouteContractsInput) (*RouteContractsResult, error)
    // temporal / test-intel (both)
    History(ctx, HistoryInput) (*HistoryResult, error)
    SnapshotDiff(ctx, SnapshotDiffInput) (*SnapshotDiffResult, error)
    TestsForChange(ctx, TestsForChangeInput) (*TestsForChangeResult, error)
    Coverage(ctx, CoverageInput) (*CoverageResult, error)
    // agentic (hosted; ErrTierUnsupported on local) — return job handles
    RCA(ctx, RCAInput) (*Job[RCAResult], error)
    Fix(ctx, FixInput) (*Job[FixResult], error)
    Review(ctx, ReviewInput) (*Job[ReviewResult], error)
    // admin
    Status(ctx, StatusInput) (*StatusResult, error)
    Repos(ctx, ReposInput) (*ReposResult, error)
    Link(ctx, LinkInput) (*LinkResult, error)
    Close() error
}

func New(ctx, ...Option) (Engine, error) // zero opts = local SQLite, no vectors, sync
```

**Critical layering fix (resolving the SDK/BUSL critique):** all DTO result types (`RCAResult`, `FixResult`, `ReviewResult`, …) live in `pkg/apitypes` (Apache, **always** compiled). Only the *runners* live in `internal/cloud` (BUSL, `//go:build hosted`). The `Engine` interface compiles in the Apache-only build; local impls of `RCA`/`Fix`/`Review` return `ErrTierUnsupported`. **CI builds `pkg/atlas` with no `hosted` tag** to prove this.

**Options:** `WithSQLite(path)` / `WithPostgres(dsn)` (the keystone swap) / `WithStorageDriver(d)`; `WithVectors(v)` / `WithEmbeddingClient(c)`; `WithLanguages` / `WithParserConcurrency`; `WithIndexQueue` / `WithRetention`; `WithLogger` / `WithMetrics` / `WithStrict`.

**`Job[T]`** (generic): `ID()`, `Status(ctx)`, `Await(ctx)`, `Events(ctx) <-chan JobEvent`, `Cancel(ctx)`. Core ops are synchronous in-library; only the three agentic ops return jobs.

**Sentinel errors:** the canonical set (§4.0.2). `SemanticSearch`/`Search(mode:semantic)` with no vectors → `ErrVectorsDisabled` (explicit); `mode:hybrid` degrades silently to lexical with `Result.Degraded`.

#### S4(b) — Client SDKs (Go / TS / Python)

Thin HTTP wrappers over `atlas serve`. No tree-sitter, no bleve, no SQLite, no CGO. Generated from the **one OpenAPI spec**:

```
internal/api/openapi.yaml
   ├─ oapi-codegen        → sdk/go
   ├─ openapi-typescript  → sdk/ts   (@aziron/atlas-sdk)
   └─ openapi-python-client → sdk/python (aziron-atlas)
```

Each = generated core + a hand-written facade (auth, retry/backoff, pagination iterators, `Job` await/events). **Method-isomorphic with the library**: `engine.Impact(...)` ⇄ `POST /impact` ⇄ `client.Impact(...)` in all three languages, so prototyping in-process and shipping remote is a type-compatible swap. Idioms: Go (`ctx`, `io.Writer` streams, `*Job`); TS (camelCase via typed transform, `Promise`, async-iterator, `EventEmitter` `Job`); Python (snake_case native, pydantic v2, sync `AtlasClient` + `AsyncAtlasClient`, context-manager streams).

**Decision guide:** *if your process must own the SQLite/Postgres file and tree-sitter CGO → library. If you just want answers from a running Atlas → client SDK.* The HTTP API is the seam; the OpenAPI doc is the contract keeping them isomorphic.

---

### 4.5 Surface S5 — Runtime (the binary)

One static-ish binary + container image hosting all of the above.

- `atlas serve` — boots REST (`/api/v1/*`); `--mcp` also mounts MCP-over-HTTP on the same listener; `--webhook` mounts `/webhooks/github`; `--workers N` runs durable-queue workers (hosted).
- `atlas mcp` — the LLM-agent surface (stdio default; `--http`/`--sse`).
- `atlas daemon` — local in-process worker over the in-memory queue (single-writer, lock-file enforced — see §5.6).
- **Distribution:** goreleaser matrix (darwin/linux × amd64/arm64), Homebrew tap, **npm wrapper** (`npx @aziron/atlas` downloads the signed release asset + installs the skill — graphify's exact adoption surface), GHCR container (`:X.Y.Z` local + `:X.Y.Z-hosted`), `go install`, signed raw archives + SBOM, and `atlas install` skill/MCP-config writers for all five agent clients.

---

## 5. Architecture

### 5.1 Package layout & dependency direction

```
pkg/atlas      → public Engine facade (Apache, SemVer)
pkg/graphjson  → graph.json schema (graphify-compatible + Atlas ext)
pkg/apitypes   → shared DTOs (source of truth for OpenAPI + all result types incl. RCA/Fix/Review)

internal/graph        → pure data model (no I/O); JSONBMap; NodeID derivation
internal/storage      → StorageDriver iface + sqlite/ + postgres/ + migrate/
internal/vectorstore  → VectorStore iface + noop + sqlitevec/ + pgvector/ + qdrant/ (build-tagged)
internal/parser       → tree-sitter (7 langs) + comment extraction + CALL-EDGE extraction (net-new)
internal/lexical      → bleve BM25 + code-aware tokenizer + trigram (net-new analyzer)
internal/index        → IndexService (full/delta/reuse), git ops, snapshots, retention GC
internal/search       → graph queries: callers/refs/neighbors/path/impact + lexical fusion
internal/analysis     → route-contract + cross-service analyzers + matcher
internal/testintel    → tests_for_change + coverage
internal/queue        → Queue iface + memory + postgres (hosted)
internal/engine       → composition root
internal/cli|api|mcp  → surfaces
internal/cloud        → BUSL, //go:build hosted: rca/fix/review/webhook/tenancy runners
internal/auth|observability
```

**Strict dependency rule:** `graph` imports nothing internal. `storage`/`vectorstore`/`parser`/`lexical` import only `graph`. `query`/`analysis`/`index` import `graph`+`storage` (+optional `lexical`). **`query`/`impact`/`analysis`/`lexical` never import `vectorstore`** — the structural guarantee that vectors stay off the LLM path.

### 5.2 Data model (`internal/graph`) — with the net-new fields called out

Ported from `models/devops.go` (SQL `db:` tags stripped; `pq.StringArray` fields **replaced** with a portable `StringList` type implementing `Scan` for both `[]byte` and `string` — see §5.4; `JSONBMap.Scan` fixed to handle both types — §5.4).

```go
type NodeKind string // repo|file|symbol|route|test
type EdgeKind string // calls|imports|references|covers|serves|consumes

// NodeID: content-stable identity, INDEPENDENT of volatile line numbers (critique fix).
type NodeID string
func SymbolNodeID(repoFullName, path, kind, name, sigHash string, overloadIdx int) NodeID {
    // path + kind + name + signature-hash (+ overload idx). NOT start_line.
    return NodeID(fmt.Sprintf("sym:%s:%s:%s:%s:%s#%d",
        repoFullName, path, kind, name, sigHash, overloadIdx))
}
func FileNodeID(repoFullName, path string) NodeID
func RouteNodeID(repoFullName, method, path string) NodeID

type Symbol struct {
    ID         uuid.UUID
    SnapshotID uuid.UUID
    NodeID     NodeID    // NET-NEW: indexed, content-stable, UNIQUE(snapshot_id, node_id)
    Path, Language, Kind, Name, Signature, Doc string
    StartLine, EndLine int
    Metadata   JSONBMap
}

type Edge struct {
    ID         uuid.UUID
    SnapshotID uuid.UUID
    FromFile   string
    FromSymbol string   // NET-NEW column: caller symbol (Pulse had none)
    FromNodeID NodeID   // NET-NEW: resolved caller node
    ToRef      string   // callee identifier / import path / route key
    ToNodeID   NodeID   // NET-NEW: resolved callee node (when resolvable)
    Kind       EdgeKind
    Language   string
    Line       int      // NET-NEW: call-site line
    Metadata   JSONBMap
}

// Repo, Snapshot, File, Route(=CrossServiceContract), Coverage(=CodeSymbolTestCoverage),
// CrossDep(=CrossServiceDep), RepoLink — ported, scope-aware.
```

**`CrossRepoImpact` result struct — reconciled to ONE shape** (resolving the four-divergent-shapes critique). Defined once in `pkg/apitypes`, serialized verbatim by all surfaces. We **extend** the ported `service.CrossRepoImpact` to carry the per-symbol fields the catalog promises (the alternative — dropping `via`/`contract`/`confidence`/`symbol`/`path`/`line` — loses agent value), and update the engine to populate them:

```go
type CrossImpact struct {
    RepoID      string          `json:"repo_id"`
    Role        string          `json:"role"`          // "consumer"
    Symbol      string          `json:"symbol"`        // NET-NEW vs ported struct
    Path        string          `json:"path"`          // NET-NEW
    Line        int             `json:"line"`          // NET-NEW
    Via         string          `json:"via"`           // route_contract|interface|import
    Contract    RouteContract   `json:"contract"`      // {method,path}
    Confidence  string          `json:"confidence"`    // high|medium|low
    Linked      bool            `json:"linked"`
    Indexed     bool            `json:"indexed"`
    RiskLevel   string          `json:"risk_level"`
    Tests       []CoverageRef   `json:"tests"`
    TestsTotal  int             `json:"tests_total"`
    TestsTruncated bool         `json:"tests_truncated"`
}
type CrossRepoImpactResult struct {
    OriginRepoID string                   `json:"repo_id"`   // role implied = origin
    Impacted     []CrossImpact            `json:"impacted"`
    ImpactedTestsByRepo map[string]int    `json:"impacted_tests_by_repo"`
    UnlinkedDependencies []UnlinkedDep    `json:"unlinked_dependencies"`
    Truncation   Truncation               `json:"truncation"` // {total,returned,truncated}
}
```
The ported `service.CrossRepoImpactForChangedPaths` returns `map[string]any` today; converting it to this typed, capped struct is **explicit net-new mapping work**, budgeted in P1.

### 5.3 StorageDriver (`internal/storage`) — the keystone

One interface, two impls. Postgres lifts `code_intelligence_repository.go` (schema `pulse`→`atlas`, tenant `*uuid.UUID` → `scope`). SQLite re-expresses every query. Tier = one-line swap. Snapshot-bound tenancy (§4.0.6): every read keys on `snapshot_id`; only resolution methods take `scope`.

```go
type StorageDriver interface {
    Migrate(ctx) error; Close() error; Dialect() string; Capabilities() Capabilities
    // repos (scope-aware resolution)
    EnsureRepo / FindRepo(scope) / GetRepo / ListRepos(scope) / MarkRepo
    // snapshots (temporal; resolution carries scope → snapshot_id is tenant-bound)
    SaveSnapshot / SaveSnapshotDelta(base, head, changedPaths, …)
    FindSnapshotByCommit(scope) / LatestSnapshot(scope) / ListSnapshots(scope) / PruneSnapshots
    // graph reads (snapshot-keyed; tenant-correct by construction)
    ListFiles / ListSymbols / ListSymbolsForPaths / GetSymbolsByIDs
    GetSymbolsByNodeIDs / ListEdges / ListEdgesForPaths / ListRoutes
    // cross-repo
    SaveRoutes / SaveCrossDeps / ListCrossDeps / ListProducerRoutes(scope, method, path)
    UpsertRepoLink / ListRepoLinks / UpsertServiceAlias / ListServiceAliases(scope)
    // test-intel
    SaveCoverage / ListCoverageForSymbols / ListCoverageForTest
    // job queue
    EnqueueJob / ClaimNextJob / CompleteJob / ReapStaleJobs / ListJobs / QueueDepth
    // portable multi-value predicate (resolves the =ANY()/pq.Array critique)
    InClause(col string, vals []string) (sqlFrag string, args []any)
}
type Capabilities struct { DurableQueue, CrossScope, ConcurrentWrite, Webhooks bool }
```

**Delta carry-forward keyed on `node_id`, not `path`** (critique fix): `SaveSnapshotDelta` inserts the new snapshot, carries forward unchanged rows from base via `node_id NOT IN (changed)`, inserts re-parsed changed rows with their stable `node_id`, recomputes counts. `UNIQUE(snapshot_id, node_id)` enables symbol-level diff (a moved/renamed symbol with a stable `node_id` is "modified", not "removed+added").

**Backend differences:**

| Concern | Postgres | SQLite |
|---|---|---|
| Multi-value predicate | `node_id = ANY($n)` via `pq.Array` | `InClause` → temp-table join (`node_id IN (SELECT … FROM _changed)`); **no `pq.Array` anywhere in non-hosted files** |
| Txn | `BeginTxx`, serializable on snapshot write | `BEGIN IMMEDIATE` (WAL) |
| Concurrency | many writers (per-repo) | single global write mutex + OS flock on the DB file (§5.6) |
| IDs | `gen_random_uuid()` | deterministic v5 UUID from `NodeID` (stable re-index) |
| JSONB Scan | `[]byte` | **handles both `[]byte` and `string`** (mattn returns TEXT as `string`) — §5.4 |

### 5.4 Portability fixes (mandatory, from the critique)

1. **`JSONBMap.Scan` handles `string` AND `[]byte`** and **errors** on unknown types (never silent-nil). A storage-contract test round-trips `JSONBMap` through SQLite and asserts non-nil. *(Pulse's `if !ok { return nil }` silently nulls `Metadata`/`Imports` on SQLite — would quietly degrade the entire local graph.)*
2. **Replace `pq.StringArray` struct fields with `graph.StringList`** (JSON/TEXT-CSV custom type, `Scan` for `string`+`[]byte`). No `lib/pq` type may appear in any file compiled without the `hosted` tag — preserves build-tag isolation.
3. **Every `=ANY()` / `pq.Array` call-site routed through `StorageDriver.InClause`** (temp-table on SQLite, `ANY()` on PG).
4. **Audit gate:** CI greps for `pq.` in non-`hosted` files; any hit fails the build.

### 5.5 Retrieval

- **Graph queries** (`internal/search`/`query`): `GraphView` materialized from `StorageDriver` lists; `callers`/`refs`/`neighbors` (depth-bounded BFS with the ported **ambiguity guard** so generic-named symbols don't cartesian-explode); `path` (bidirectional BFS + cross-repo route-contract bridge edges); `impact` (identity-seeded blast radius from changed-file *definitions* by `node_id`, directness-ranked, `seenFile` memoization). **All pure; never import `vectorstore`.**
- **Lexical** (`internal/lexical`, **net-new**): custom bleve analyzer `atlas_code` (camelCase/snake_case/dotted split → emits original + sub-tokens + lowercased + edge-ngram; per-field boosts name×3/sig×1.5/doc×1) + a trigram side-index (roaring bitmaps) for substring/fuzzy identifier match. Query construction uses **constructed MatchQuery disjunctions, NOT `NewQueryStringQuery`** (so `foo:bar`/unbalanced parens can't trip the parser). RRF fusion of BM25+trigram(+semantic only in hybrid). Hydrate hits from `GetSymbolsByIDs` so each result carries signature/doc/snippet inline.
- **`ApplyDelta`** is gated on stable `node_id` as the bleve doc id (you cannot delete-old-docs-by-id when ids churn). Until `node_id` lands, search index is **full rebuild per snapshot** (documented honestly; the incremental claim is not made before then).
- **Vectors** (`internal/vectorstore`, optional): `NoopVectorStore` default; pgvector (hosted) / sqlite-vec→chromem (local) behind build tags. Semantic only runs when `IsAvailable()` AND the caller asked (`mode:semantic|hybrid` / `semantic_search`).

### 5.6 Concurrency model

- **Parsing**: worker pool (≤GOMAXPROCS). **One `tree_sitter.Parser` per goroutine, `defer Close()` on Parser AND Tree** (tree-sitter parsers/trees are NOT goroutine-safe; never pooled/shared; sharing leaks C memory). Results funnel through a channel to a single accumulator.
- **SQLite writes**: single global driver mutex + `BEGIN IMMEDIATE` + **OS flock on `atlas.db`** so a second `atlas index` *process* blocks rather than races. `busy_timeout` raised + retry-on-`SQLITE_BUSY`. We do **not** claim WAL makes concurrent *writers* safe — WAL gives concurrent readers + one writer.
- **Local job queue**: single-writer-process only; `atlas daemon` holds a lock file and is the sole indexer. `ClaimNextJob` UPDATE checks `RowsAffected` and loops on 0 (TOCTOU guard; SQLite has no `SKIP LOCKED`).
- **PG queue**: `SELECT … FOR UPDATE SKIP LOCKED`.
- **Reads** lock-free (WAL / MVCC). `GraphView` immutable once loaded.
- **Cancellation**: every long walk checks `ctx.Done()` every ~1000 edges.

### 5.7 Two-tier deployment

| | Local | Hosted |
|---|---|---|
| Process | one binary, `atlas index`/`mcp`/`serve` | `atlas serve --workers N` + Postgres(+pgvector) + GHCR image |
| Store | `.atlas/atlas.db` (WAL, flock) | Postgres schema `atlas` |
| Queue | in-process (sync default) | durable `SKIP LOCKED` + webhooks |
| Auth | none / file token | API key + JWT/SSO + service token |
| Vectors | off (opt-in sqlite-vec/chromem) | off (opt-in pgvector) |

---

## 6. Canonical Operation Catalog

| Op | Category | Tier | One-line |
|---|---|---|---|
| `index` | core | both | Parse + persist graph/symbol/lexical index; full or git-diff delta. |
| `search` | core | both | Code-aware lexical (BM25+trigram); `mode` lexical/semantic/hybrid (latter two need vectors). |
| `semantic_search` | core | both* | Embedding NL / find-similar. *Only when vectors enabled; MCP-separate tool. |
| `symbol` | core | both | Full bundle: def + callers + callees + tests. |
| `callers` | core | both | Inbound call edges, transitive to depth. |
| `refs` | core | both | All usages (call/read/write/type/import). |
| `neighbors` | core | both | Adjacent nodes by edge kind/direction (multi-repo aware). |
| `path` | core | both | Shortest path; cross-repo via route-contract bridge. |
| `impact` | core | both | Single-repo blast radius (symbols/files/tests), directness-ranked. |
| `explain` | core | both | Cited, LLM-ready context bundle. |
| `graph_export` | core | both | Portable graph.json (graphify-compatible + ext). |
| `cross_repo_impact` | cross-repo | hosted† | Org-wide blast radius via route contracts. †honest-empty local. |
| `consumers` | cross-repo | hosted† | Who consumes a route/symbol/interface. |
| `route_contracts` | cross-repo | hosted† | List resolved producer/consumer contracts. |
| `history` | temporal | both | Per-commit snapshot chain for a target. |
| `snapshot_diff` | temporal | both | Structural (graph-level) diff between two snapshots. |
| `tests_for_change` | test-intel | both | Predictive minimal test set + recall estimate. |
| `coverage` | test-intel | both | Symbol↔test map; coverage gaps. |
| `rca` | agentic | hosted | Reverse graph walk → cited causal hypotheses (job). |
| `fix` | agentic | hosted | Graph-grounded patch + validate + PR (job). |
| `review` | agentic | hosted | Impact + tests + cited findings + merge verdict (job). |
| `status` | admin | both | Health, freshness, queue depth; `run_id` poll. |
| `repos` | admin | both | List repos + index/snapshot metadata. |
| `link` | admin | both‡ | Register repo + webhook. ‡local = path-add no-op. |
| `job` | lifecycle | both | get / events(SSE) / cancel — index async. |
| `run` | lifecycle | hosted | get / events(SSE) / cancel — agentic async. |
| `webhook` | lifecycle | hosted | Git webhook ingestion (HMAC). |

---

## 7. Repo Layout & Package Map

```
aziron-atlas/                                  # github.com/MsysTechnologiesllc/aziron-atlas (go 1.25)
├── go.mod  go.sum
├── LICENSE (Apache-2.0)  LICENSE-BUSL  NOTICE
├── README.md  CHANGELOG.md  CONTRIBUTING.md  SECURITY.md  CODEOWNERS
├── Makefile  .goreleaser.yaml  .golangci.yaml  Dockerfile  docker-compose.yml
│
├── cmd/atlas/{main.go,version.go}             # only main; -ldflags Version/Commit/Date
│
├── pkg/                                        # PUBLIC, SemVer, Apache
│   ├── atlas/{atlas.go,config.go,options.go,results.go}   # Engine facade (S4a)
│   ├── graphjson/schema.go                    # SchemaVersion = "atlas-1"
│   └── apitypes/types.go                      # DTOs incl. RCA/Fix/Review results (always compiled)
│
├── internal/
│   ├── graph/{model.go,ids.go,jsonb.go,stringlist.go}     # net-new: NodeID, StringList; fixed JSONBMap
│   ├── storage/{driver.go,migrate/, sqlite/, postgres/(//go:build hosted)}
│   ├── vectorstore/{store.go,noop.go, sqlitevec/(tag), pgvector/(hosted), qdrant/(tag), embed/}
│   ├── parser/{parser.go,walk_*.go,comments.go,edges.go(NET-NEW),langs.go,testdata/}
│   ├── lexical/{index.go,tokenizer.go(NET-NEW),trigram.go(NET-NEW),rank.go}
│   ├── index/{service.go,full.go,delta.go,reuse.go,git.go,scan.go,snapshot.go,source.go}
│   ├── search/{service.go,graphwalk.go,impact.go,fusion.go}
│   ├── analysis/{routes.go,crossservice.go,match.go,hosts.go}
│   ├── testintel/{selection.go,coverage.go}
│   ├── queue/{queue.go,memory.go,postgres.go(//go:build hosted)}
│   ├── engine/engine.go                       # composition root
│   ├── cli/  api/(openapi.yaml,handlers,middleware)  mcp/(server,transports,catalog,skills/)
│   ├── auth/  observability/{log.go,metrics.go,trace.go}
│   └── cloud/(//go:build hosted, BUSL: rca/ fix/ review/ webhook/ tenancy/)
│
├── sdk/{go,ts,python}/                         # S4b generated clients
├── npm/atlas/{package.json,install.js,skill/SKILL.md}  # S5 npm wedge
├── examples/{embed-go,ci-impact-gate,mcp-claude,hosted-compose}/
├── docs/{architecture,storage-driver,operations,tiers,licensing,mcp,cross-repo, adr/, benchmarks/}
├── test/{golden/,integration/(tags),bench/(graphify),testutil/}
├── tools/{gen-openapi,gen-sdks.sh,tools.go}
└── .github/workflows/{ci,integration,benchmark,release,codeql,govulncheck,sdk-publish}.yml
```

---

## 8. Port / Reuse Plan from aziron-pulse

Engine at `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal`. **Effort reclassified per the feasibility critique** — the manifest under-budgeted the core by ~10×.

| Concern | Source | Target | Real effort | Notes |
|---|---|---|---|---|
| Data models | `models/devops.go` | `internal/graph` | **M** (not S) | Strip `db:` tags; replace `pq.StringArray`→`StringList`; fix `JSONBMap.Scan`; **add `NodeID`, `Edge.FromSymbol/FromNodeID/ToNodeID/Line`**. |
| Tree-sitter parser | `service/tree_sitter_parser.go` | `internal/parser` | **S** (port) + **L** (call edges) | Parser walks port as-is; **call-edge extraction is NET-NEW** (second AST pass per function body, callee resolution, overload disambiguation). This is the single largest core item. |
| StorageDriver | `repository/code_intelligence_repository.go` | `internal/storage` | **XL** (not L) | PG impl ≈ lift; **SQLite impl = from-scratch** (temp-table predicates, flock, deterministic UUIDs, node_id-keyed delta, JSON Scan). Shared contract test gates equivalence. |
| Index engine | `service/code_intelligence_service.go` | `internal/index` | **M** | Driver-interface DI; `Source` abstraction (local working-tree vs git clone); `updateSearchIndex` nil-optional; retention GC. |
| Lexical/graph search | `service/code_search_service.go` | `internal/search`+`internal/lexical` | **M** (graph) + **L** (lexical NET-NEW) | Graph traversals + impact + ambiguity guard port. **Code-aware analyzer + trigram + MatchQuery-disjunction = net-new.** `ApplyDelta` gated on `node_id`. |
| Cross-service + route contracts | `service/cross_service_analyzer.go` + `route_contract_analyzer.go` | `internal/analysis` | **S** | Port; known-hosts map → config. Typed `CrossRepoImpactResult` (map→struct) = **M** net-new. |
| Vector store + embedder | `service/qdrant_code_store.go` + `embedding_client.go` | `internal/vectorstore` | **M** | Extract `VectorStore` iface; sqlite-vec/chromem local; nil-safe; build-tagged. |
| MCP surface | `handlers/mcp.go` | `internal/mcp` | **M** | Drop datasource/canvas; collapse keyword/semantic→`search`+gated `semantic_search`; add 11 net-new tools; `ToolBackend` seam; compact `_meta` envelope; keep JWT `authSession`, SSE `conn.ctx` fix, keepalive. |
| Job queue | service + repository | `internal/queue` | **M** | memory (local, single-writer+flock) / PG (`SKIP LOCKED`). |
| RCA/fix/review | `service/session_runner.go` (`StartRCA`/`StartFix`/`BuildReviewContext`) | `internal/cloud` | **L** | BUSL; compose `impact`+`cross_repo_impact`+`explain`+`tests_for_change`. |

---

## 9. Phased Roadmap (P0→P3)

### P0 — Foundation & the three net-new keystones (the riskiest work, front-loaded)

**Goal:** a local engine that indexes, searches code-aware-lexically, and answers symbol-granular callers/impact — on SQLite, with stable identity.

| Workstream | Deliverable | Sizing |
|---|---|---|
| Scaffold | `go.mod`, `cmd/atlas`, `pkg/atlas` facade skeleton, Makefile, ci.yml; `atlas version` runs | S |
| Graph model | `internal/graph` with `NodeID`, `StringList`, **fixed `JSONBMap.Scan`**, `Edge` symbol fields | M |
| **StorageDriver (SQLite first)** | iface + SQLite impl: repos/snapshots/graph-reads + **node_id-keyed delta** + temp-table `InClause` + flock; `storagecontract_test.go` written FIRST | XL |
| Parser + **call edges** | 7-lang port + **net-new call/ref edge extraction** + comment/doc | L |
| Index engine | full/delta/reuse, git ops, `Source` abstraction, retention GC | M |
| **Lexical (net-new)** | `atlas_code` analyzer + trigram + MatchQuery-disjunction; full-rebuild delta (honest) | L |
| Graph queries | callers/refs/neighbors/path/impact + ambiguity guard | M |
| CLI core | `index search symbol callers refs impact status repos config version` | M |

**Exit criteria:**
- `atlas index .` then `atlas impact --diff -` returns symbol-granular blast radius on a real repo (gin v1.10.0 integration test green).
- `storagecontract_test.go` passes against SQLite (Postgres impl stubbed/skipped); JSONBMap round-trips non-nil on SQLite.
- Golden-graph fixtures frozen for all 7 langs; `schema_version == "atlas-1"`.
- `node_id` stable across a re-index (determinism + a "rename keeps node_id" test).
- No `pq.` symbols in any non-`hosted` file (CI grep gate green).

### P1 — Moats on the local tier + remaining surfaces

**Goal:** temporal + test-intel + (single-DB) cross-repo working locally; MCP + HTTP + SDK shipped.

| Workstream | Deliverable | Sizing |
|---|---|---|
| Temporal | `history` + `snapshot_diff` via node_id set-diff; `ApplyDelta` incremental lexical (now that node_id is the doc id) | M |
| Cross-repo analysis | route/cross-service analyzers + matcher; typed `CrossRepoImpactResult` (map→struct); honest-empty on single-DB | L |
| Test-intel | coverage edges + `tests_for_change` (+ recall estimate, truncation trio) | M |
| MCP | full tool catalog + `ToolBackend(Local)` + stdio/http/sse + resources + prompts + `atlas install` | L |
| HTTP API | REST routes + envelope + problem+json + cursor pagination + SSE + OpenAPI gen | L |
| SDK | `pkg/atlas` complete (incl. `Job[T]`, agentic methods returning `ErrTierUnsupported` locally); Apache-only build CI; generated Go/TS/Python clients | M |
| `explain`, `graph_export` | cited bundle + graphify-compatible export (+ MCP resource-link for full) | M |

**Exit criteria:**
- Two sibling folders indexed into one local DB → `cross_repo_impact` finds a real route-contract match (not fabricated).
- `history`/`snapshot_diff` show symbol-level added/modified/removed across 3 commits.
- `tests_for_change` selects a known covering test for an injected change; recall estimate emitted.
- Claude Code picks up Atlas via `atlas install skill --agent claude`; an agent completes the find→symbol→impact trace over stdio.
- `pkg/atlas` compiles with **no `hosted` tag** (proves DTO layering).
- OpenAPI spec generates all three clients; `atlas openapi --check` green.

### P2 — Hosted tier + the agentic moat

**Goal:** org-wide Postgres, durable queue, webhooks, RCA/fix/review.

| Workstream | Deliverable | Sizing |
|---|---|---|
| StorageDriver (Postgres) | lift `code_intelligence_repository.go`; **same contract test** runs green against PG (testcontainers/pgvector) | L |
| Durable queue + webhooks | PG `SKIP LOCKED` queue + `/webhooks/github` (HMAC) + delta reindex workers | M |
| Tenancy + auth | API keys + JWT/SSO + `TenantScope` (snapshot-bound enforcement) + RBAC scopes + rate limiting | L |
| `internal/cloud` (BUSL) | `rca` (reverse walk), `fix` (patch+validate+PR), `review` (cited findings + verdict) — compose existing facade ops | L |
| Cross-repo at org scale | `ListProducerRoutes` across linked repos; `link` + service-alias resolution; `unlinked_dependencies` honesty | M |
| HostedBackend (MCP) + `run` lifecycle | hosted MCP funnel; `run`/`job` SSE; SDK `Job[T]` over HTTP | M |

**Exit criteria:**
- Identical `storagecontract_test.go` green on **both** drivers (the spin-out claim proven).
- A change in repo A → `cross_repo_impact` reaches a test in repo B via route contract, org-wide.
- `review --pr N --gate` returns a `block` verdict on an unsafe API change and posts cited findings; `rca`→`fix` opens a validated draft PR.
- Upgrade funnel demonstrated: a tool absent locally appears after `atlas link`.
- Tenant isolation test: tenant-A `repo_id` → `not_found` for tenant-B principal.

### P3 — Distribution, vectors, parity polish

| Workstream | Deliverable | Sizing |
|---|---|---|
| VectorStore | sqlite-vec/chromem local + pgvector hosted; `semantic_search` op + `search --mode` + gated MCP tool | M |
| Release eng | goreleaser (CGO cross via zig/osxcross), Homebrew tap, npm wrapper + skill, GHCR local+hosted, cosign + SBOM | L |
| graphify head-to-head | benchmark harness + published `docs/benchmarks/graphify-head-to-head.md` (the 10× demo) | M |
| Lang roadmap | additive grammars toward 36; observability (trace-id propagation, metrics) | M (ongoing) |

**Exit criteria:**
- `npx @aziron/atlas` installs the binary (cosign-verified) + skill on mac/linux.
- Benchmark doc shows graphify `N/A (structural)` on all four moats, parity on single-repo.
- Vectors prove off-by-default: default build links no vector code; `--vectors` enables `semantic_search`.

---

## 10. Cross-Cutting Concerns

### 10.1 Auth
`Authenticator` interface (local noop/file-token; hosted API-key/JWT/SSO/service). RBAC scopes `read⊂impact⊂…`, `write`, `agent`, `admin`. Same pluggable-impl pattern as `StorageDriver`/`VectorStore`.

### 10.2 Config
Functional options (library) ⇄ `.atlas/config.yaml` + `ATLAS_*` env + flags (CLI), one precedence chain. Zero-value = working local engine.

### 10.3 Versioning (one literal per axis, reconciled)
- `graph.json` `schema_version = "atlas-1"` (single literal everywhere; golden tests assert it).
- HTTP `/api/v1` (path); breaking → `/v2`; additive never bumps.
- `pkg/**` SemVer; `internal/**` exempt.
- MCP `protocolVersion = "2025-06-18"`.
- The four axes are independent; documented in `docs/architecture.md`.

### 10.4 Error model
One canonical `ErrorCode` enum (§4.0.2) in `pkg/apitypes`, mapped to HTTP problem+json `code`, SDK sentinels, MCP `degraded.status`, CLI exit codes. `degraded`/`unlinked` represented on every surface.

### 10.5 Observability (one cross-cutting contract)
- **One request/trace-id** (`X-Request-Id`) propagated CLI→SDK→HTTP→engine→job; surfaced in CLI `--json` `meta.request_id` and MCP `_meta`.
- **Named engine metrics** emitted regardless of surface: `index_duration_ms`, `search_latency_ms`, `impact_depth_reached`, `queue_depth`, `snapshot_count`, `cross_repo_match_count`. zap logger (stderr for CLI/MCP; structured for HTTP); Prometheus `/metrics` (hosted), no-op (local).
- **OTel tracing**: scoped to hosted, behind a flag (documented as optional in `docs/observability.md`).

### 10.6 Security
Webhook HMAC; API-key argon2id hashing + revocation; tenant isolation (existence not leaked); `govulncheck` + CodeQL in CI; cosign-signed releases + SBOM; SPDX headers; secret handling via env-var references in skill installers (`chmod 600` fallback warning).

### 10.7 Performance targets
| Op | Local target | Notes |
|---|---|---|
| Full index (10k-symbol repo) | < 10 s | parser pool |
| Delta index (1 file) | < `full` and < 1 s | node_id carry-forward |
| `search` (lexical) | < 50 ms p50 | bleve + trigram |
| `impact` (depth 3) | < 200 ms p50 | bounded BFS |
| MCP tool result | ≤ 200 items, ≤ ~8 KB | server-side cap before marshal |

### 10.8 Multi-tenancy
Snapshot-bound enforcement (§4.0.6): tenant correctness is structural because every graph read keys on a `snapshot_id` that only a scope-aware resolver can produce. Contract test asserts cross-tenant invisibility. Local = nil sentinel.

---

## 11. Testing, CI, Release & Licensing

### 11.1 Testing (four layers)
| Layer | Where | Tag | Gate | Proves |
|---|---|---|---|---|
| Unit | `*_test.go` | none | every PR, `-race` | walks, tokenizer split, impact ranking, driver CRUD (in-mem sqlite), MCP dispatch |
| Golden-graph | `test/golden/` | none | every PR | parser+indexer determinism; `-update` regenerates (visible in PR diff); asserts `schema_version` |
| Real-repo integration | `test/integration/` | `integration` | nightly/label | end-to-end on pinned OSS repos; delta is incremental+correct; hosted PG path (testcontainers) |
| graphify head-to-head | `test/bench/` | `integration` | nightly/`perf` | the competitive claim (skips gracefully if graphify absent) |

**Storage contract test (`storagecontract_test.go`)** runs the *same* suite against any `StorageDriver`, parametrized over SQLite + Postgres — the central proof of the spin-out. Includes the JSONBMap-non-nil-on-SQLite assertion and the cross-tenant-invisibility assertion.

### 11.2 CI (GitHub Actions)
- `ci.yml`: lint (golangci, CGO on) + **build/test matrix** over OS × flavor `{local(CGO), hosted(CGO), local-static}`. **`local-static` swaps only SQLite→modernc; it is NOT CGO-free** (tree-sitter is CGO-mandatory) — the matrix labels this honestly and does NOT set `CGO_ENABLED=0` for the whole module (that would hard-fail at build). A genuinely CGO-free build requires a regex-only parser fallback → tracked as ADR-0004, not a v0 gate. + `govulncheck` + the `pq.`-in-non-hosted grep gate.
- `integration.yml`: PG service (pgvector/pg16), nightly + `run-integration` label.
- `benchmark.yml`: graphify head-to-head, nightly + `perf` label, posts the table as a PR comment.
- `release.yml`: goreleaser on tag `v*` + cosign + SBOM + Homebrew + npm.

### 11.3 Release & distribution
goreleaser (darwin/linux × amd64/arm64, CGO via zig-cc/osxcross cross toolchain — the biggest release-eng cost, budgeted in P3); Homebrew tap; **npm wrapper** (the graphify-parity wedge); GHCR `:X.Y.Z` (local) + `:X.Y.Z-hosted`; `go install`; signed archives + SBOM; `atlas install` skill/MCP writers for all five agent clients.

### 11.4 OSS / cloud licensing boundary (open-core, single repo)
| Scope | License | Mechanism |
|---|---|---|
| Everything except `internal/cloud/**` | **Apache-2.0** | default; parser, storage, search, cross-repo *analysis*, MCP, CLI, REST core, SDKs, local tier |
| `internal/cloud/**` (rca/fix/review/webhook/tenancy) | **BUSL-1.1** (→Apache after 4y) | per-package `LICENSE` + SPDX header; compiled only under `//go:build hosted` |

**The OSS local binary cannot include moat code** — `internal/cloud/**` is `//go:build hosted`, so default `go build` neither compiles nor links it (CI `local` flavor proves self-containment). Reading the graph (incl. cross-repo analysis) is open; **acting** on it at org scale (RCA/fix/review + durable webhook graph) is the paid moat. SPDX headers checked in `make lint`.

---

## 12. Risks & Open Decisions

| Risk | Severity | Mitigation / decision |
|---|---|---|
| Call-edge extraction (net-new, P0) under-delivers symbol resolution | **High** | Front-loaded in P0; overload-ambiguity guard; name→node_id resolution with explicit "unresolved" edges rather than fabrication. The whole product gates on this — budget XL across parser+model+storage. |
| SQLite StorageDriver equivalence to PG | **High** | `storagecontract_test.go` written FIRST, run against both in CI from day one; temp-table predicates; JSON Scan fix; flock. |
| `node_id` stability under rename/move | Medium | `NodeID` excludes line numbers; uses signature-hash; "rename keeps node_id" test. **Open decision:** how aggressively to fuzzy-match signature changes (a sig change = "modified", not new — confirm the hash policy in ADR-0002). |
| Full-static binary impossible (tree-sitter CGO) | Medium | Honest labeling; ADR-0004 for an optional regex-only parser fallback if a true static build is later required. Don't advertise "pure-Go static" in v0. |
| CGO cross-compile pain (release) | Medium | zig-cc/osxcross in goreleaser; budgeted in P3 as the largest release item. |
| Cross-repo precision (route-contract false matches) | Medium | Confidence scoring + `unlinked_dependencies` honesty + `matched_only` introspection via `route_contracts`. |
| MCP context blowups (full graph / hub impact) | Medium | Server-side hard cap before marshal; `scope:full`→resource link; truncation trio on every fan-out array. |
| **Open:** does local single-DB multi-repo cross-matching ship in P1 or is it hosted-only? | — | **Decided:** ships P1 as honest-empty/honest-match; `CrossScope` capability is about webhooks/org-scale, not a hard gate. (Resolves the licensing-vs-SDK contradiction.) |
| **Open:** semantic search exposure shape | — | **Decided:** `semantic_search` is a first-class catalog op; CLI/HTTP via `--mode`, MCP via separate gated tool. |

---

## 13. The 10× Acceptance Demo vs graphify

**Scenario:** an engineer edits `src/api/order.ts` in `aziron-ui` (an HTTP call to `POST /v1/orders`) and wants to know what breaks before pushing.

**graphify** (single-folder, point-in-time, test-blind, read-only):
```
$ graphify query get_pr_impact --files src/api/order.ts
impacted (aziron-ui only): src/api/order.ts, src/components/Cart.tsx, src/store/order.ts
# stops at the repo boundary. no tests. no history. cannot act.
```

**Atlas** — same change, strictly more, in one command + three follow-ups:
```
$ git diff | atlas cross-repo impact --repo aziron-ui --diff - --tests --gate --json | jq .

ORIGIN  repo_aziron-ui @ a1b9c3d
within-repo impact   : 6 symbols, 3 files                          # ≡ graphify's answer
CROSS-REPO impact    : orders-svc CreateOrder   (route POST /v1/orders,  conf 0.94)   # graphify CANNOT see this
                       payments-svc ChargeHandler(route POST /v1/charge,  conf 0.88)
selected tests       : aziron-ui:2 ut · orders-svc:4 · payments-svc:2 e2e             # graphify is test-blind
history              : order.ts last changed in c4d2e1f (auth refactor, 3d ago)       # graphify is point-in-time
unlinked             : notifications-svc reached but not_indexed (impact unknown)      # honest blind-spot
verdict (merge-gate) : BLOCK — touches a producer route 2 consumers depend on          # graphify is read-only

# And Atlas ACTS — none of which graphify can do:
$ atlas rca    --repo orders-svc --symptom 'orders-svc:TestCreateOrder' --cross-repo
$ atlas review --repo aziron-ui --pr 686 --post --gate
$ atlas fix    --rca run_8f1c --mode draft_pr --validate
```

**Acceptance:** the demo passes when each Atlas claim is backed by a real ground-truth run (cross-repo match resolved via a route contract, a covering test in another repo selected, a structural history entry, a posted cited review with a `block` verdict), the `unlinked_dependencies` honesty block is present, and the four moat columns read `N/A (structural)` for graphify in the published benchmark.

---

## 14. Appendices

### 14.A Tool catalog (MCP) outline
`search`, `semantic_search`(gated), `symbol`, `callers`, `refs`, `neighbors`, `path`, `impact`, `explain`, `graph_export`, `cross_repo_impact`, `consumers`, `route_contracts`, `history`, `snapshot_diff`, `tests_for_change`, `coverage`, `rca`(write), `fix`(write), `review`(write), `status`, `repos`, `index`, `link`. Each carries JSON Schema `inputSchema`, `annotations` (`readOnlyHint`/`destructiveHint`/`idempotentHint`/`openWorldHint`), and the `_meta` envelope. Resources: `atlas://repo/...`, `atlas://graph/...`, `atlas://symbol/.../source`, subscribable `atlas://snapshot/.../latest`. Prompts: `triage_failure`, `safe_to_change`, `pr_gate`, `understand_symbol`.

### 14.B HTTP API reference outline
Base `/api/v1`. Routes per §4.2 table. Envelope `{data, meta}`. Auth: Bearer (API key / JWT / service) + webhook HMAC. Errors: RFC 9457 + canonical `code`. Pagination: cursor (`meta.page`). Streaming: `/jobs/{id}/events`, `/runs/{id}/events` (SSE). Idempotency: `Idempotency-Key`. Rate limit: `RateLimit-*`. Spec: `GET /openapi.json` (3.1) + `/docs`.

### 14.C CLI reference outline
`atlas <verb>` per §4.1 tree. Global flags `--db/--tier/--repo/--config/--format/--json/--ndjson/--vectors/--vector-backend/--token/--server/--timeout/--strict/-q/-v`. Output: table(TTY)/plain(pipe)/json/ndjson. Exit codes per §4.0.2. `atlas config {init,get,set,view,path}`. `atlas install {hook,skill,completion,mcp-config}`.

### 14.D SDK reference outline
- **S4a library** (`pkg/atlas`): `Engine` interface (§4.4), `New(...Option)`, `Job[T]`, sentinel errors. DTO results in `pkg/apitypes` (Apache, always compiled).
- **S4b clients** (`sdk/{go,ts,python}`): generated from one OpenAPI spec, method-isomorphic with the library. Hand facade adds auth/retry/pagination/`Job`. Go (`ctx`+`io.Writer`+`*Job`); TS (`Promise`+async-iterator+`EventEmitter Job`); Python (pydantic + sync/async + context-manager streams).

### 14.E ADR index
- ADR-0001 — two-tier StorageDriver (keystone).
- ADR-0002 — NodeID derivation policy (signature-hash, no line numbers).
- ADR-0003 — open-core boundary (`internal/cloud` BUSL).
- ADR-0004 — CGO-free build via regex parser fallback (deferred; not v0).
- ADR-0005 — vectors optional / off-core-path package boundary.
- ADR-0006 — semantic_search as first-class op with two surface projections.

---

**Source anchors (absolute, for implementers):**
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/service/code_intelligence_service.go` — `IndexRepository`, `CrossRepoImpactForChangedPaths` (~:1145, returns `map[string]any` → typed in P1), `cloneFetchCheckout`/`gitDiffNameStatus`/`lockRepoPath`, `BuildReviewContext`, `RunRetentionGC`.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/repository/code_intelligence_repository.go` — `SaveSnapshotDelta` carry-forward (`=ANY()`/`pq.Array` → `InClause`), `EnqueueIndexJob`/`ClaimNextJob`/`ReapStaleRunningJobs`, list methods.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/service/code_search_service.go` — `Search` (default bleve mapping + `NewQueryStringQuery` → net-new analyzer + MatchQuery), `GetCallers`/`GetReferences`/`GetImpactForChangedSymbols`, `blastRadiusGuardedCtx` ambiguity guard, `SearchResult`/`FileImpact`/`CrossRepoImpact`/`UnlinkedDependency` DTOs.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/service/tree_sitter_parser.go` — parser port (CGO `import "C"`/`unsafe`, official `github.com/tree-sitter/*`); **emits no call edges → net-new**.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/service/{cross_service_analyzer,route_contract_analyzer}.go` — cross-repo analyzers.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/handlers/mcp.go` — MCP transport/dispatch/auth port.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/models/devops.go` — model port (`DependencyEdge` has `FromFile/ToRef/EdgeType` only → add symbol/node_id fields; `pq.StringArray`→`StringList`; `JSONBMap.Scan` fix).