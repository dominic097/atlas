# Aziron Atlas — Implementation Plan

**Module:** `github.com/MsysTechnologiesllc/aziron-atlas` · **CLI:** `atlas` · **Go:** 1.25 (CGO-required core) · **Status:** principal-level build plan, ready for engineering

---

## 1. Executive Summary, Thesis & What Is Locked

### 1.1 Thesis

> **graphify is a static map of one folder you read; Atlas is a live, org-wide, DETERMINISTIC code brain.**

Atlas is the **deterministic code-intelligence layer** (a library + service) spun out of the Aziron Pulse code-intelligence subsystem. It does **ZERO LLM reasoning** and is fully reproducible: the same repo at the same commit always yields the same graph, the same impact set, the same citations. It must **strictly dominate** [graphify](https://github.com/safishamsi/graphify) (the local-first, single-folder code knowledge graph distributed as an AI-assistant skill + MCP server, built on tree-sitter, emitting a portable `graph.json` and exposing MCP tools like `query_graph` / `get_neighbors` / `shortest_path` / `get_pr_impact`) **on the code-intelligence axis**.

**Atlas is the intelligence; Pulse is the agent.** The LLM / agentic layer (RCA, autonomous fix, PR review, predictive/risk-scored test selection) lives in **Pulse**, a separate existing product that **consumes Atlas** via its SDK / API / MCP and then acts. Atlas hands Pulse deterministic facts (impacted symbols, cross-repo blast radius, coverage edges, cited context); Pulse layers LLM reasoning on top. The agentic moat is Pulse's, built on Atlas's intelligence moat.

graphify's structural ceilings — the gaps Atlas exists to close (all on the deterministic intelligence axis):

| graphify ceiling | Atlas answer |
|---|---|
| Single graph per folder (no cross-repo) | Org-wide multi-repo graph + HTTP route-contract matching |
| Point-in-time (no history) | Per-commit immutable snapshots + delta indexing + structural diff |
| Test-blind | Symbol→test coverage edges (deterministic coverage map) + CI gate |
| Shallow call graph | Deeper symbol-granular call edges, cited and reproducible |

*(Agentic action — RCA / fix / PR review — is not an Atlas capability; it is handled in Pulse, which consumes the facts above. See §1.3.)*

### 1.2 Two tiers off one shared Go core

| | **LOCAL tier** | **HOSTED tier** |
|---|---|---|
| Distribution | Single static-ish Go binary, zero infra | Org-wide service + container image |
| Graph store | Embedded SQLite | Postgres |
| Index queue | In-process (synchronous default) | Durable Postgres queue |
| Cross-repo | Single-DB only (degrades honestly) | Org-wide cross-repo at scale (durable indexing) |
| Privacy | Code never leaves the machine | Org-managed |
| Strategic role | OSS adoption wedge | The intelligence moat + monetization (org-scale cross-repo) |

*(Indexing lifecycle is Atlas's; **when** to (re)index is decided by Pulse / CI / cron calling Atlas — there is no GitHub/webhook coupling inside Atlas. The agentic moat — RCA / fix / review — is Pulse, layered on top of the hosted tier, not a tier of Atlas itself.)*

### 1.3 What is locked (do not relitigate)

0. **Atlas is LLM-FREE and deterministic.** No model call, no NL generation, no risk scoring, no hypothesis or patch generation lives in Atlas. Every op is reproducible from the indexed graph alone. Any semantic / natural-language / risk / hypothesis / patch generation is **Pulse** (the LLM layer), which consumes Atlas. This is the boundary the whole plan enforces.
1. **Two tiers off one shared Go core**, selected by a one-line driver swap.
2. **Retrieval core = GRAPH + SYMBOL INDEX + CODE-AWARE LEXICAL SEARCH** (BM25 via bleve + trigram, camelCase/snake_case tokenization). This is the default path.
3. **Vector/semantic search is OPTIONAL, OFF BY DEFAULT, PLUGGABLE.** Never on the core retrieval path. pgvector (hosted) / sqlite-vec / chromem-go (local) behind a `VectorStore` interface. Never a mandatory Qdrant dependency. Because **Pulse is the LLM layer, it is the primary semantic consumer** — Atlas offers vectors as an optional capability, but does no reasoning over them itself.
4. **Storage abstraction = `StorageDriver` interface, two impls (SQLite, Postgres).** This is the keystone of the spin-out. `VectorStore` is a separate optional interface.
5. **The three intelligence moats** graphify structurally lacks, all deterministic and all ported from Pulse: (a) cross-repo blast radius via HTTP route contracts; (b) temporal history via per-commit snapshots + delta indexing; (c) test intelligence as deterministic FACTS (symbol↔test coverage map + CI gate via `impact` joining coverage edges). **The agentic moat — RCA / fix / review, and predictive/risk-scored test selection — is Pulse, layered on top of these facts; it is not part of Atlas.**
6. **Atlas owns its indexing LIFECYCLE** (index / reindex + status), but **Pulse / CI / cron decide WHEN** to run it. No GitHub / webhook coupling lives inside Atlas; Pulse already owns the GitHub integration.
7. **Parsing = tree-sitter** (7 langs today: Go, Python, JS, TS, Java, C, C++; roadmap to graphify-parity 36). CGO is mandatory.

### 1.4 What this plan ADDS to the locked decisions (resolving the feasibility critique)

The port-effort table in the source manifest was wrong on three load-bearing items. This plan reclassifies them and budgets the real work explicitly (see §8 and §9):

- **Symbol-granular call edges DO NOT EXIST in Pulse.** `DependencyEdge` has `FromFile/ToRef/EdgeType` only; the tree-sitter parser emits no call edges. Every callers/path/impact/cross-repo claim depends on symbol-granular edges → **net-new build, sized L/XL, not "S/port".**
- **Stable `NodeID` identity DOES NOT EXIST.** Pulse keys delta carry-forward on `path`; symbol rows get fresh server-assigned IDs every snapshot. Temporal diff (`history`, `snapshot_diff`) and incremental search delta require a content-stable `node_id` column + UNIQUE key → **net-new.**
- **Code-aware lexical tokenizer DOES NOT EXIST.** Pulse uses bleve's default mapping + `NewQueryStringQuery`. The custom analyzer + trigram side-index → **net-new.**

These are the three highest-risk items and they gate the product's core value. The roadmap front-loads them.

---

## 2. Positioning vs graphify (parity + three deterministic intelligence moats)

Atlas wins on the **deterministic code-intelligence axis** — it beats graphify purely on what can be computed reproducibly from the graph, over CLI / API / MCP / SDK / binary. The agentic layer (RCA / fix / review / predictive selection) is **Pulse, layered on top of Atlas**, and is therefore not an Atlas-vs-graphify row.

| Capability | graphify | Atlas | Atlas surface |
|---|---|---|---|
| Single-folder graph: neighbors, shortest-path, `graph.json` | ✅ | ✅ (drop-in compatible) | `neighbors` `path` `graph_export` |
| Local-first zero-infra binary + MCP + skill installer | ✅ | ✅ (parity) | `atlas mcp`, `atlas install` |
| Deterministic, reproducible (no LLM in the path) | ✅ | ✅ | every op |
| **Cross-repo blast radius (route contracts)** | ❌ | ✅ | `cross_repo_impact` `consumers` `route_contracts` |
| **Temporal history / structural snapshot diff** | ❌ | ✅ | `history` `snapshot_diff` |
| **Deterministic impacted-tests + coverage map + CI gate** | ❌ | ✅ | `impact` (joins coverage edges) `coverage` `impact --gate` |
| **Deeper symbol-granular call graph** | partial | ✅ | `callers` `refs` `impact` |
| Agentic RCA / fix / PR review, predictive/risk test selection | ❌ | **handled in Pulse** (consumes Atlas) | — (Pulse, not an Atlas surface) |

**Parity is table stakes; the three deterministic moats are the wedge.** Atlas must be a graphify drop-in (`atlas graph_export --format graphjson` produces a graphify-compatible `graph.json`; the same MCP tool surface plus `neighbors`/`path`) so a graphify user loses nothing by switching — then gains three reproducible intelligence classes graphify cannot structurally provide. The agentic differentiation is Pulse's, built on these Atlas facts.

---

## 3. Locked Feature Set

### 3.1 Core (both tiers)

- **Index** — tree-sitter parse, extract symbols/imports/**call-edges**/doc-comments; persist graph + symbol index + lexical (BM25/trigram) index. Full first run, git-diff delta thereafter. Deterministic.
- **Code-aware lexical search** — BM25 (bleve) + trigram, identifier-splitting. The default retrieval path. No vectors required.
- **Symbol context, callers, refs, neighbors, path, impact, explain, graph_export** — the graph-query primitives. `explain` returns a **deterministic structured context bundle** (symbol + edges + provenance/citations); it contains **NO LLM narrative** — narration is Pulse's job.
- **impact** — single-repo blast radius; **still returns deterministic `impacted_tests` by joining coverage edges** (a fact derived from the graph, not a prediction).
- **History, snapshot_diff** — temporal moat.
- **coverage** — the symbol↔test coverage MAP as graph facts (single-repo on local). *(Predictive / risk-scored test selection — P(fail|change), budgets, recall estimates — is **not** an Atlas op; it is Pulse SignalChain consuming `coverage` + `impact`. See §3.4.)*
- **status, repos** — admin.

### 3.2 Hosted-only

- **cross_repo_impact, consumers, route_contracts** — cross-repo moat (deterministic, org-scale).
- **link** — register a repo for indexing (local = path-add no-op, see §4). **No webhook** — Atlas does not ingest GitHub events; Pulse/CI/cron trigger reindex.

### 3.3 Handled in Pulse (Atlas is consumed, not extended)

The following are **NOT Atlas features** — they belong to **Pulse**, the LLM / agentic layer, which calls Atlas (`impact`, `cross_repo_impact`, `coverage`, `explain`) over SDK / API / MCP and then reasons + acts:

- **rca, fix, review** — agentic, LLM-driven, write-capable actions (open PRs, post findings). Pulse already owns the GitHub integration that triggers and posts these.
- **Predictive / risk-scored test selection** — `P(fail|change)`, risk budgets, ML, recall estimates → Pulse SignalChain (see §3.4).
- **Async job machinery for the above** (poll / events / cancel `run`s) — Pulse-side; Atlas ops are synchronous.

### 3.4 Vector / semantic search — OPTIONAL, OFF BY DEFAULT

> **Locked decision #3, restated as an engineering constraint:** the packages `query`, `impact`, `analysis`, `lexical` **must not import `vectorstore`**. This is enforced by package boundaries, not convention. The default `VectorStore` is `NoopVectorStore` (`IsAvailable() == false`). Vectors earn a place only for: human NL search in the hosted console, cross-vocabulary discovery, and find-similar-code. **Because Pulse is the LLM layer, Pulse is the primary semantic consumer** — Atlas exposes optional embeddings as a deterministic retrieval aid and does no reasoning over them.

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

#### 4.0.3 Async lifecycle (indexing only — every other Atlas op is synchronous)

**Atlas ops are synchronous.** The only asynchronous Atlas operation is **indexing** (a repo parse can be long). Resolution: add exactly one **canonical sub-resource op** to the catalog (§6), explicitly transport-level:

- `job` — `get` / `events` (SSE) / `cancel`. **Index** async lifecycle only.

`status` is extended with an optional `job_id` poll that returns the job payload. CLI `--job <id>` polls via `status`.

*(There is no `run` op and no agentic async machinery in Atlas. The async `run` lifecycle — poll/events/cancel for rca/fix/review — existed only to serve agentic ops and now lives in **Pulse**. Likewise there is no `webhook` op: Pulse owns the GitHub integration and triggers Atlas indexing itself.)*

#### 4.0.4 Pagination & truncation (one policy per op-category)

- **List ops** (`search`, `callers`, `refs`, `neighbors`, `consumers`, `route_contracts`, `history`, `coverage`): cursor pagination. Cursor = HMAC-signed `{snapshot_id, last_sort_key, last_id}`, pinned to one snapshot. Stale cursor → `invalid_cursor`/`cursor_stale`. Per-op `limit` defaults: search 15–20, refs 200, neighbors 100, callers 100; hard max 500.
- **Fan-out ops** (`impact`, `cross_repo_impact`): bounded by `depth`/`max_depth` + per-level cap, **not** cursor. Each emits an explicit `{total, returned, truncated}` trio on **every** array it returns (`impacted_symbols`, `impacted_files`, `impacted_tests` — the deterministic coverage-edge join — and `impacted` consumers). MCP additionally hard-caps every list server-side before marshal (default 200 items) and sets `_meta.truncated`.

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
| `cross_repo_impact`, `consumers`, `route_contracts` | **Honest-empty, not error.** Matching runs across all repos in the single local DB; if only one repo is indexed, results are empty (never fabricated). `_meta.degraded` notes single-repo scope. | A local user who indexes sibling folders into one `.atlas` DB *can* get cross-repo matches; capability is `CrossScope` (org scale + durable indexing), not a hard 404. |
| `link` | **Path-add no-op**, returns `{repo_id, linked:true}`. **NOT** a `tier_unavailable` 404. | Carve-out from the hosted-only-404 rule, explicitly. No webhook registration — Atlas registers a repo for indexing only. |

> This corrects the SDK-vs-Core contradiction: cross-repo/consumers/route_contracts on local are **honest-empty**, not `ErrTierUnsupported`. (Agentic ops — rca/fix/review — are not Atlas ops at all; they are handled in Pulse, which consumes Atlas.)

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
            | coverage | status | repos
            | cross-repo {impact,consumers,contracts}        # hosted/honest-empty local
            | link | serve | mcp | install {hook,skill,completion,mcp-config}
            | config {init,get,set,view,path} | version
# (no rca/fix/review/tests-for-change verbs — those are Pulse, which consumes Atlas)
```

**Persistent flags:** `--db <dsn>` (`sqlite://`|`postgres://`, scheme selects tier), `--tier`, `--repo`, `--config`, `--format`/`--json`/`--ndjson`/`--compact`, `--color`, `--vectors`, `--vector-backend`, `--token`, `--server <url>` (thin HTTP client mode), `--timeout`, `-q/-v/-vv`, `--strict`, `--no-progress`.

**Config precedence:** flag → env (`ATLAS_*`) → config file (`.atlas/config.yaml`, viper-discovered) → default. **Zero-config = working local engine** (SQLite at `./.atlas/atlas.db`, lexical on, vectors off, sync indexing, no auth).

**Headline CI commands:**
```bash
# deterministic blast-radius gate
git diff origin/main... | atlas impact --diff - --gate --max-files 25 --json
# deterministic impacted-tests (coverage-edge join, NOT a prediction) → runner
git diff origin/main... | atlas impact --diff - --print tests | xargs go test -run
# local git-hook gate (brings the CI impact-gate to local, zero server)
atlas install hook --type pre-push --max-files 40
```
*(Predictive / risk-scored selection — which subset to actually run under a budget — is Pulse SignalChain, which calls `atlas impact`/`coverage` and then scores. Atlas only emits the deterministic coverage-edge join.)*

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
| coverage | GET | `/coverage` | B |
| status | GET | `/status` | B |
| repos | GET | `/repos` ; `/repos/{repo_id}` | B |
| link | POST | `/repos` | B (path-add on local; registers for indexing, no webhook) |
| unlink | DELETE | `/repos/{repo_id}` | H |
| infra | GET | `/healthz` `/readyz` `/metrics` `/openapi.json` `/docs` | B |

\* honest-empty on local single-repo, not a 404 (§4.0.7).

*(No `rca`/`fix`/`review`/`tests_for_change` routes, no `/runs/...` agentic run lifecycle, no `/webhooks/github` route — all of that is **Pulse**, which calls the routes above and acts. Atlas ops are synchronous; the only async lifecycle is `job` for indexing.)*

**Action-route rationale:** `impact`/`cross_repo_impact`/`explain` accept diffs/symbol arrays too large for a query string → `POST` to a sub-resource, declared **idempotent & safe to retry** (and, being deterministic, byte-stable across retries).

**Envelope:**
```jsonc
{ "data": <op-specific>,
  "meta": { "request_id":"req_…", "tier":"hosted", "snapshot_id":"…",
            "commit_sha":"…", "schema_version":"atlas-1", "mode_used":"lexical",
            "degraded": null,
            "page": { "next_cursor":"…", "limit":20, "has_more":true } } }
```

**Auth** (pluggable `Authenticator`): API keys (`atlas_sk_live_…`, argon2id-hashed, scoped `tenant_id`+`scopes`+`repos[]`+`expires_at`); JWT/SSO (HS256 shared-secret or RS256/JWKS OIDC, claims `sub`/`tenant_id`/`exp`); service tokens (M2M, `kind:service`). Local default = `NoopAuthenticator` (loopback) or `--api-token-file`. *(No webhook HMAC — Atlas has no webhook ingestion endpoint. Pulse is a typical service-token caller.)*

**RBAC scopes:** `read` ⊂ `impact` ⊂ … ; `index`/`write` (index, link); `admin`. Middleware `RequireScope` → `403 insufficient_scope` + `WWW-Authenticate`. *(No `agent` scope — Atlas never opens PRs or posts; the actor that does is Pulse, governed by Pulse's own RBAC.)*

**Error model:** RFC 9457 `application/problem+json` with the canonical `code` (§4.0.2) + `request_id`.

**Streaming (SSE):** `/jobs/{id}/events` (**index progress only**). Named events `progress`/`stage`/`done`/`error`; `: ping` heartbeat 15s; `Last-Event-ID` resume from a bounded ring buffer. *(No agentic `/runs/{id}/events` stream — that was Pulse's.)*

**Idempotency:** `Idempotency-Key` on `index`/`link` (24h store, verbatim replay + `Idempotency-Replayed: true`). Natural dedupe: `index` by `(repo_id, commit_sha)`. Read ops are deterministic and intrinsically idempotent.

**Rate limiting:** token-bucket per `(tenant, principal, route-class)`. Classes `read`/`impact`/`index`; `RateLimit-*` headers + `Retry-After`.

**Middleware order:** `RequestID → Recover → Logger(zap) → CORS → Authenticate → TenantScope → RequireScope → RateLimit → BodyLimit → Idempotency → Handler`. `TenantScope` is the security keystone (§4.0.6).

**OpenAPI:** source of truth = Go DTOs in `pkg/apitypes`; spec generated via `kin-openapi` builders (typed control over problem+json + SSE media types). `GET /openapi.json` (3.1) + `/docs`. The spec drives the S4 client SDKs (one spec → 3 clients). CI `atlas openapi --check` fails on drift; `oasdiff` gates breaking changes.

#### 4.2.1 Pulse integration (Atlas is the deterministic substrate Pulse calls)

Pulse (the LLM / agentic layer) is a first-class **consumer** of this API. The integration is one-directional: **Pulse calls Atlas, Atlas never calls Pulse.** Pulse authenticates with a service token (`kind:service`) and calls the deterministic read ops — `impact`, `cross_repo_impact`, `coverage`, `explain` (plus `search`/`symbol`/`callers`/`history` as needed) — over the HTTP API or MCP (or links Atlas in-process via the Go SDK). It receives reproducible facts (impacted symbols, cross-repo blast radius, coverage edges, cited context) and then **does the agentic work itself**: RCA hypotheses, fix patches, PR review verdicts, and predictive/risk-scored test selection (SignalChain). All LLM calls, GitHub writes, and async run lifecycle live on the Pulse side; Atlas stays synchronous, deterministic, and write-free.

---

### 4.3 Surface S3 — MCP (`internal/mcp`)

Exposes graph/search/impact/temporal/coverage as deterministic MCP tools to Claude Code / Cursor / Codex / Gemini / Copilot (and to Pulse). Ported from Pulse `handlers/mcp.go`, stripped of Pulse-specific tools (`datasource_introspect`, canvas). Protocol `2025-06-18` (advertise `2024-11-05` fallback). **No tool in this surface invokes an LLM, writes code, or opens a PR** — those are Pulse's, layered on top.

**Architecture: one handler, two backends, three transports.**

```
stdio | streamable-http | legacy-SSE  →  mcp.Server  →  ToolBackend
                                                          ├─ LocalBackend  (in-proc *atlas.Engine)
                                                          └─ HostedBackend (HTTP → atlas serve, JWT)
```

**`ToolBackend` seam — the upgrade funnel keystone.** The Server never knows which backend it holds; tool names/schemas/results are identical. `LocalBackend.Has(op)` returns `false` for `semantic_search` unless vectors are enabled; cross-repo ops are **present but honest-empty** locally (§4.0.7). The capability handshake at `initialize` advertises only supported tools — so on upgrade to the hosted tier the org-scale cross-repo tools light up against real org data. **That visible enrichment is the funnel.** *(There are no agentic tools to funnel — rca/fix/review are Pulse's, not Atlas MCP tools.)*

**Design principles (LLM-optimized):** stable `symbol_id`/`repo_id` everywhere; token-aware defaults (small `limit`, snippets ≤3 lines / 240 chars unless `include_source`); **server-side hard cap before marshal** (default 200 items) + `_meta.truncated`; compact `json.Marshal` (not Indent); cursor pagination; graceful degrade (`isError:false` + `_meta.degraded` + `hint`); MCP `annotations` (`readOnlyHint` etc.) so agents auto-run vs confirm.

**`_meta` envelope on every result:** `{tier, snapshot_id, repo_id, returned, total, cursor, truncated, duration_ms, degraded}`.

**Transports:** `atlas mcp` (stdio, default — **stderr-only logging**, stdout is the protocol channel); `atlas mcp --http` (Streamable HTTP, `POST /mcp` + `Mcp-Session-Id`); `atlas mcp --sse` (legacy, ported handshake + 25s keepalive + the **long-lived `conn.ctx` tool-call fix** from Pulse).

**Tool catalog** (bare verbs, no `code_` prefix; 1:1 with canonical ops; **every tool is read-only/deterministic — `readOnlyHint:true`, no `destructiveHint`**):
- Core/both: `search`, `symbol`, `callers`, `refs`, `neighbors`, `path`, `impact`, `explain`, `graph_export`, `history`, `snapshot_diff`, `coverage`, `status`, `repos`, `index`.
- Cross-repo (advertised on hosted; honest-empty local): `cross_repo_impact`, `consumers`, `route_contracts`.
- Optional, gated: `semantic_search` (advertised only when `VectorStore.IsAvailable()`).
- Admin: `link` (local path-add no-op).

*(No `rca`/`fix`/`review` MCP tools and no predictive `tests_for_change` tool — those are Pulse, which consumes these deterministic tools. `impact`/`coverage` give Pulse the coverage-edge facts it scores; `explain` gives Pulse the cited context bundle it narrates.)*

**Resources** (`atlas://repo/...`, `atlas://graph/{repo}/{snapshot}.json`, `atlas://symbol/{id}/source`, subscribable `atlas://snapshot/{repo}/latest` → `notifications/resources/updated` on each reindex — the temporal moat surfaced to MCP; reindex is triggered by Pulse/CI/cron, not a webhook inside Atlas).

**Prompts** (recipes that teach the deterministic call-order): `triage_context`, `safe_to_change`, `impact_gate`, `understand_symbol`. *(These compose Atlas read ops to assemble context; the LLM consuming them — e.g. Pulse — does the reasoning.)*

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
    // temporal (both)
    History(ctx, HistoryInput) (*HistoryResult, error)
    SnapshotDiff(ctx, SnapshotDiffInput) (*SnapshotDiffResult, error)
    // coverage facts (both) — symbol↔test map; NOT predictive selection
    Coverage(ctx, CoverageInput) (*CoverageResult, error)
    // admin
    Status(ctx, StatusInput) (*StatusResult, error)
    Repos(ctx, ReposInput) (*ReposResult, error)
    Link(ctx, LinkInput) (*LinkResult, error)
    Close() error
}

func New(ctx, ...Option) (Engine, error) // zero opts = local SQLite, no vectors, sync
```

**Every method is synchronous** — Atlas does deterministic computation, not long agentic runs, so there are no `Job[T]` handles on the facade (the only long op, `Index`, reports progress via the `job` lifecycle on the service surfaces, but the library call returns when indexing completes). **There are no `RCA`/`Fix`/`Review` methods and no predictive `TestsForChange` method on the Engine** — those are Pulse, which embeds this `Engine` (or calls `atlas serve`) to get the deterministic facts and then reasons + acts itself.

**Layering note (BUSL boundary still holds, now smaller):** all DTO result types live in `pkg/apitypes` (Apache, **always** compiled). The hosted-only code in `internal/cloud` (BUSL, `//go:build hosted`) is the org-scale cross-repo / durable-queue machinery — **not** agentic runners (there are none in Atlas). The `Engine` interface compiles in the Apache-only build; **CI builds `pkg/atlas` with no `hosted` tag** to prove self-containment.

**Options:** `WithSQLite(path)` / `WithPostgres(dsn)` (the keystone swap) / `WithStorageDriver(d)`; `WithVectors(v)` / `WithEmbeddingClient(c)`; `WithLanguages` / `WithParserConcurrency`; `WithIndexQueue` / `WithRetention`; `WithLogger` / `WithMetrics` / `WithStrict`.

**Sentinel errors:** the canonical set (§4.0.2). `SemanticSearch`/`Search(mode:semantic)` with no vectors → `ErrVectorsDisabled` (explicit); `mode:hybrid` degrades silently to lexical with `Result.Degraded`. *(No `Job[T]` type — every Engine call is synchronous. `Index` progress, when surfaced over HTTP/MCP, uses the transport-level `job` lifecycle, not a library handle.)*

#### S4(b) — Client SDKs (Go / TS / Python)

Thin HTTP wrappers over `atlas serve`. No tree-sitter, no bleve, no SQLite, no CGO. Generated from the **one OpenAPI spec**:

```
internal/api/openapi.yaml
   ├─ oapi-codegen        → sdk/go
   ├─ openapi-typescript  → sdk/ts   (@aziron/atlas-sdk)
   └─ openapi-python-client → sdk/python (aziron-atlas)
```

Each = generated core + a hand-written facade (auth, retry/backoff, pagination iterators, index-`job` poll/events for the one async op). **Method-isomorphic with the library**: `engine.Impact(...)` ⇄ `POST /impact` ⇄ `client.Impact(...)` in all three languages, so prototyping in-process and shipping remote is a type-compatible swap. Idioms: Go (`ctx`, `io.Writer` streams); TS (camelCase via typed transform, `Promise`, async-iterator for index progress); Python (snake_case native, pydantic v2, sync `AtlasClient` + `AsyncAtlasClient`, context-manager streams). These clients are exactly how an external consumer like **Pulse** calls Atlas remotely.

**Decision guide:** *if your process must own the SQLite/Postgres file and tree-sitter CGO → library. If you just want answers from a running Atlas → client SDK.* The HTTP API is the seam; the OpenAPI doc is the contract keeping them isomorphic.

---

### 4.5 Surface S5 — Runtime (the binary)

One static-ish binary + container image hosting all of the above.

- `atlas serve` — boots REST (`/api/v1/*`); `--mcp` also mounts MCP-over-HTTP on the same listener; `--workers N` runs durable-queue indexing workers (hosted). *(No `--webhook` mount — Atlas has no GitHub ingestion; reindex is triggered by callers like Pulse/CI/cron hitting `POST /repos/{repo_id}/index`.)*
- `atlas mcp` — the surface for LLM-agent *clients* (Claude Code / Cursor / Pulse) to consume Atlas's deterministic tools (stdio default; `--http`/`--sse`). Atlas itself runs no LLM.
- `atlas daemon` — local in-process worker over the in-memory queue (single-writer, lock-file enforced — see §5.6).
- **Distribution:** goreleaser matrix (darwin/linux × amd64/arm64), Homebrew tap, **npm wrapper** (`npx @aziron/atlas` downloads the signed release asset + installs the skill — graphify's exact adoption surface), GHCR container (`:X.Y.Z` local + `:X.Y.Z-hosted`), `go install`, signed raw archives + SBOM, and `atlas install` skill/MCP-config writers for all five agent clients.

---

## 5. Architecture

### 5.1 Package layout & dependency direction

```
pkg/atlas      → public Engine facade (Apache, SemVer)
pkg/graphjson  → graph.json schema (graphify-compatible + Atlas ext)
pkg/apitypes   → shared DTOs (source of truth for OpenAPI + all deterministic result types)

internal/graph        → pure data model (no I/O); JSONBMap; NodeID derivation
internal/storage      → StorageDriver iface + sqlite/ + postgres/ + migrate/
internal/vectorstore  → VectorStore iface + noop + sqlitevec/ + pgvector/ + qdrant/ (build-tagged)
internal/parser       → tree-sitter (7 langs) + comment extraction + CALL-EDGE extraction (net-new)
internal/lexical      → bleve BM25 + code-aware tokenizer + trigram (net-new analyzer)
internal/index        → IndexService (full/delta/reuse), git ops, snapshots, retention GC
internal/search       → graph queries: callers/refs/neighbors/path/impact + lexical fusion
internal/analysis     → route-contract + cross-service analyzers + matcher
internal/coverage     → symbol↔test coverage map (deterministic facts; no prediction)
internal/queue        → Queue iface + memory + postgres (hosted) — indexing jobs only
internal/engine       → composition root
internal/cli|api|mcp  → surfaces
internal/cloud        → BUSL, //go:build hosted: hosted org features (multi-tenant cross-repo at scale, durable queue, tenancy). NO LLM/agentic runners — those live in Pulse.
internal/auth|observability
```

**Strict dependency rule:** `graph` imports nothing internal. `storage`/`vectorstore`/`parser`/`lexical` import only `graph`. `query`/`analysis`/`index` import `graph`+`storage` (+optional `lexical`). **`query`/`impact`/`analysis`/`lexical` never import `vectorstore`** — the structural guarantee that vectors stay off the core deterministic path. **No package imports an LLM client** — Atlas is LLM-free; any model call is Pulse's.

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
    // coverage facts (symbol↔test map; deterministic, no prediction)
    SaveCoverage / ListCoverageForSymbols / ListCoverageForTest
    // index job queue (Atlas's only async work)
    EnqueueJob / ClaimNextJob / CompleteJob / ReapStaleJobs / ListJobs / QueueDepth
    // portable multi-value predicate (resolves the =ANY()/pq.Array critique)
    InClause(col string, vals []string) (sqlFrag string, args []any)
}
type Capabilities struct { DurableQueue, CrossScope, ConcurrentWrite bool }
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
- **Vectors** (`internal/vectorstore`, optional): `NoopVectorStore` default; pgvector (hosted) / sqlite-vec→chromem (local) behind build tags. Semantic only runs when `IsAvailable()` AND the caller asked (`mode:semantic|hybrid` / `semantic_search`). Embeddings are a deterministic retrieval aid only — Atlas runs no LLM reasoning over them; the LLM consumer is Pulse.

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
| Queue | in-process (sync default) | durable `SKIP LOCKED` indexing queue (triggered by callers, not webhooks) |
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
| `impact` | core | both | Single-repo blast radius (symbols/files + **impacted_tests via coverage-edge join — a deterministic fact**), directness-ranked. |
| `explain` | core | both | **Deterministic** structured context bundle: symbol + edges + provenance/citations. NO LLM narrative. |
| `graph_export` | core | both | Portable graph.json (graphify-compatible + ext). |
| `cross_repo_impact` | cross-repo | hosted† | Org-wide blast radius via route contracts. †honest-empty local. |
| `consumers` | cross-repo | hosted† | Who consumes a route/symbol/interface. |
| `route_contracts` | cross-repo | hosted† | List resolved producer/consumer contracts. |
| `history` | temporal | both | Per-commit snapshot chain for a target. |
| `snapshot_diff` | temporal | both | Structural (graph-level) diff between two snapshots. |
| `coverage` | coverage | both | Symbol↔test map as graph facts; coverage gaps. |
| `status` | admin | both | Health, freshness, queue depth; `job_id` poll. |
| `repos` | admin | both | List repos + index/snapshot metadata. |
| `link` | admin | both‡ | Register repo for indexing (no webhook). ‡local = path-add no-op. |
| `job` | lifecycle | both | get / events(SSE) / cancel — **index** async (the only async Atlas op). |

**Op count: 21 canonical ops** (16 core/temporal/coverage/admin + 3 cross-repo + `semantic_search` gated + `job` lifecycle).

> **Not Atlas ops — handled in Pulse** (which consumes the table above): `rca`, `fix`, `review` (agentic, LLM-driven, write-capable) and predictive/risk-scored `tests_for_change` (P(fail|change), budgets, recall — Pulse SignalChain). There is also no `run` lifecycle (agentic async) and no `webhook` op inside Atlas; Pulse owns the GitHub integration and triggers Atlas indexing.

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
│   └── apitypes/types.go                      # deterministic DTOs (always compiled); no agentic result types
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
│   ├── coverage/coverage.go                    # symbol↔test map (deterministic facts; no prediction)
│   ├── queue/{queue.go,memory.go,postgres.go(//go:build hosted)}   # index jobs only
│   ├── engine/engine.go                       # composition root
│   ├── cli/  api/(openapi.yaml,handlers,middleware)  mcp/(server,transports,catalog,skills/)
│   ├── auth/  observability/{log.go,metrics.go,trace.go}
│   └── cloud/(//go:build hosted, BUSL: hosted org features — multi-tenant cross-repo at scale, durable queue, tenancy/. NO rca/fix/review/webhook — those are Pulse.)
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
| MCP surface | `handlers/mcp.go` | `internal/mcp` | **M** | Drop datasource/canvas **and the agentic rca/fix/review tools (those move to Pulse)**; collapse keyword/semantic→`search`+gated `semantic_search`; add the deterministic net-new tools; `ToolBackend` seam; compact `_meta` envelope; keep JWT `authSession`, SSE `conn.ctx` fix, keepalive. |
| Job queue | service + repository | `internal/queue` | **M** | **Indexing jobs only.** memory (local, single-writer+flock) / PG (`SKIP LOCKED`). |
| ~~RCA/fix/review~~ | — | **Pulse** (not Atlas) | — | **Removed from Atlas scope.** `StartRCA`/`StartFix`/`BuildReviewContext` stay in Pulse, which composes Atlas's `impact`+`cross_repo_impact`+`explain`+`coverage` over the SDK/API/MCP. |

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

**Goal:** temporal + coverage facts + (single-DB) cross-repo working locally; MCP + HTTP + SDK shipped.

| Workstream | Deliverable | Sizing |
|---|---|---|
| Temporal | `history` + `snapshot_diff` via node_id set-diff; `ApplyDelta` incremental lexical (now that node_id is the doc id) | M |
| Cross-repo analysis | route/cross-service analyzers + matcher; typed `CrossRepoImpactResult` (map→struct); honest-empty on single-DB | L |
| Coverage facts | coverage edges + `coverage` op + `impact` joins coverage → deterministic `impacted_tests` (truncation trio). **No predictive selection** (that's Pulse SignalChain). | M |
| MCP | full deterministic tool catalog + `ToolBackend(Local)` + stdio/http/sse + resources + prompts + `atlas install` | L |
| HTTP API | REST routes + envelope + problem+json + cursor pagination + index-`job` SSE + OpenAPI gen | L |
| SDK | `pkg/atlas` complete (synchronous; no `Job[T]`, no agentic methods); Apache-only build CI; generated Go/TS/Python clients | M |
| `explain`, `graph_export` | deterministic cited context bundle (no LLM narrative) + graphify-compatible export (+ MCP resource-link for full) | M |

**Exit criteria:**
- Two sibling folders indexed into one local DB → `cross_repo_impact` finds a real route-contract match (not fabricated).
- `history`/`snapshot_diff` show symbol-level added/modified/removed across 3 commits.
- `impact --diff` returns the known covering test(s) for an injected change via the coverage-edge join (deterministic; reproducible run-to-run).
- Claude Code picks up Atlas via `atlas install skill --agent claude`; an agent completes the find→symbol→impact trace over stdio.
- `pkg/atlas` compiles with **no `hosted` tag** (proves DTO layering).
- OpenAPI spec generates all three clients; `atlas openapi --check` green.

### P2 — Hosted tier: org-wide Postgres + durable queue + cross-repo at scale + Pulse integration

**Goal:** org-wide Postgres, durable indexing queue, deterministic cross-repo at org scale, multi-tenancy — and a proven integration path for **Pulse** (the LLM/agentic layer) to consume Atlas. (Agentic ops — rca/fix/review, predictive selection — are built in Pulse, not here.)

| Workstream | Deliverable | Sizing |
|---|---|---|
| StorageDriver (Postgres) | lift `code_intelligence_repository.go`; **same contract test** runs green against PG (testcontainers/pgvector) | L |
| Durable indexing queue | PG `SKIP LOCKED` queue + delta reindex workers (triggered by callers — Pulse/CI/cron — not webhooks) | M |
| Tenancy + auth | API keys + JWT/SSO + service tokens + `TenantScope` (snapshot-bound enforcement) + RBAC scopes + rate limiting | L |
| Cross-repo at org scale | `ListProducerRoutes` across linked repos; `link` + service-alias resolution; `unlinked_dependencies` honesty | M |
| HostedBackend (MCP) | hosted MCP funnel (org-scale cross-repo tools light up); index-`job` SSE over HTTP | M |
| Pulse integration | service-token auth path proven; Pulse consumes `impact`/`cross_repo_impact`/`coverage`/`explain` over API + MCP; documented in `docs/pulse-integration.md` | M |

**Exit criteria:**
- Identical `storagecontract_test.go` green on **both** drivers (the spin-out claim proven).
- A change in repo A → `cross_repo_impact` reaches a test in repo B via route contract, org-wide — deterministically reproducible.
- Pulse calls `cross_repo_impact` + `coverage` + `explain` with a service token and assembles its own RCA/review context from the returned facts (integration test against a running `atlas serve`).
- Upgrade funnel demonstrated: org-scale cross-repo enriches after `atlas link`.
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
- Benchmark doc shows graphify `N/A (structural)` on all three intelligence moats, parity on single-repo.
- Vectors prove off-by-default: default build links no vector code; `--vectors` enables `semantic_search`.

---

## 10. Cross-Cutting Concerns

### 10.1 Auth
`Authenticator` interface (local noop/file-token; hosted API-key/JWT/SSO/service). RBAC scopes `read⊂impact⊂…`, `index`/`write` (index, link), `admin`. Same pluggable-impl pattern as `StorageDriver`/`VectorStore`. *(No `agent` scope and no webhook HMAC — Atlas neither acts nor ingests GitHub events; the acting principal is Pulse, calling Atlas with a service token.)*

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
- **One request/trace-id** (`X-Request-Id`) propagated CLI→SDK→HTTP→engine→index-job; surfaced in CLI `--json` `meta.request_id` and MCP `_meta`.
- **Named engine metrics** emitted regardless of surface: `index_duration_ms`, `search_latency_ms`, `impact_depth_reached`, `queue_depth`, `snapshot_count`, `cross_repo_match_count`. zap logger (stderr for CLI/MCP; structured for HTTP); Prometheus `/metrics` (hosted), no-op (local).
- **OTel tracing**: scoped to hosted, behind a flag (documented as optional in `docs/observability.md`).

### 10.6 Security
API-key argon2id hashing + revocation; service-token (M2M) auth for callers like Pulse; tenant isolation (existence not leaked); `govulncheck` + CodeQL in CI; cosign-signed releases + SBOM; SPDX headers; secret handling via env-var references in skill installers (`chmod 600` fallback warning). *(No webhook HMAC surface — Atlas exposes no GitHub ingestion endpoint.)*

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
| Everything except `internal/cloud/**` | **Apache-2.0** | default; parser, storage, search, cross-repo *analysis*, coverage facts, MCP, CLI, REST core, SDKs, local tier |
| `internal/cloud/**` (hosted org features: multi-tenant cross-repo at scale, durable queue, tenancy) | **BUSL-1.1** (→Apache after 4y) | per-package `LICENSE` + SPDX header; compiled only under `//go:build hosted` |

**The OSS local binary cannot include the hosted-org moat code** — `internal/cloud/**` is `//go:build hosted`, so default `go build` neither compiles nor links it (CI `local` flavor proves self-containment). Reading the graph (incl. single-DB cross-repo analysis) is open; the paid moat is **org-scale cross-repo + durable indexing** (multi-tenant, at scale) — **not** RCA/fix/review, which are not part of Atlas at all (they live in Pulse, layered on top). SPDX headers checked in `make lint`.

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
| **Open:** does local single-DB multi-repo cross-matching ship in P1 or is it hosted-only? | — | **Decided:** ships P1 as honest-empty/honest-match; `CrossScope` capability is about org-scale + durable indexing, not a hard gate. |
| **Open:** boundary with Pulse (which layer owns agentic/LLM work)? | — | **Decided:** Atlas is the LLM-free deterministic intelligence layer; **Pulse owns all LLM/agentic work (rca/fix/review) and predictive/risk-scored test selection**, consuming Atlas via SDK/API/MCP. No model call, GitHub write, or async agentic run lives in Atlas. |
| **Open:** semantic search exposure shape | — | **Decided:** `semantic_search` is a first-class catalog op; CLI/HTTP via `--mode`, MCP via separate gated tool. |

---

## 13. The 10× Acceptance Demo vs graphify (pure cross-repo INTELLIGENCE)

This is a **deterministic intelligence** demo: it answers what graphify *structurally cannot* — cross-repo impact, temporal history, the coverage-edge join — all reproducibly, with no LLM in the path. (Agentic action on these facts is Pulse's job, demonstrated separately; it is not part of this Atlas demo.)

**Scenario:** an engineer edits `src/api/order.ts` in `aziron-ui` (an HTTP call to `POST /v1/orders`) and wants to know what breaks before pushing.

**graphify** (single-folder, point-in-time, test-blind):
```
$ graphify query get_pr_impact --files src/api/order.ts
impacted (aziron-ui only): src/api/order.ts, src/components/Cart.tsx, src/store/order.ts
# stops at the repo boundary. no tests. no history.
```

**Atlas** — same change, strictly more, deterministic, in one command + two follow-ups:
```
$ git diff | atlas cross-repo impact --repo aziron-ui --diff - --tests --gate --json | jq .

ORIGIN  repo_aziron-ui @ a1b9c3d
within-repo impact   : 6 symbols, 3 files                          # ≡ graphify's answer
CROSS-REPO impact    : orders-svc CreateOrder   (route POST /v1/orders,  conf 0.94)   # graphify CANNOT see this
                       payments-svc ChargeHandler(route POST /v1/charge,  conf 0.88)
impacted tests       : aziron-ui:2 ut · orders-svc:4 · payments-svc:2 e2e   # coverage-edge join — graphify is test-blind
unlinked             : notifications-svc reached but not_indexed (impact unknown)      # honest blind-spot
verdict (merge-gate) : BLOCK — touches a producer route 2 consumers depend on          # deterministic gate

# Atlas answers what graphify structurally cannot — temporal + cross-repo facts:
$ atlas history    --repo aziron-ui --path src/api/order.ts        # point-in-time graphify can't do
$ atlas cross-repo consumers --route 'POST /v1/orders'            # repo-boundary graphify can't cross
```

*(What comes next — RCA, a cited PR review verdict, an autonomous fix — is **Pulse** consuming exactly these facts. That is a separate Pulse demo, not an Atlas command.)*

**Acceptance:** the demo passes when each Atlas claim is backed by a real, reproducible ground-truth run (cross-repo match resolved via a route contract, a covering test in another repo surfaced via the coverage-edge join, a structural history entry), the `unlinked_dependencies` honesty block is present, the same inputs yield byte-identical outputs run-to-run, and the three intelligence-moat columns read `N/A (structural)` for graphify in the published benchmark.

---

## 14. Appendices

### 14.A Tool catalog (MCP) outline
`search`, `semantic_search`(gated), `symbol`, `callers`, `refs`, `neighbors`, `path`, `impact`, `explain`, `graph_export`, `cross_repo_impact`, `consumers`, `route_contracts`, `history`, `snapshot_diff`, `coverage`, `status`, `repos`, `index`, `link`. **Every tool is read-only/deterministic** (`readOnlyHint:true`); each carries JSON Schema `inputSchema`, `annotations` (`readOnlyHint`/`idempotentHint`/`openWorldHint`), and the `_meta` envelope. Resources: `atlas://repo/...`, `atlas://graph/...`, `atlas://symbol/.../source`, subscribable `atlas://snapshot/.../latest`. Prompts: `triage_context`, `safe_to_change`, `impact_gate`, `understand_symbol`. *(No `rca`/`fix`/`review` tools and no predictive `tests_for_change` tool — those are Pulse, which consumes these deterministic tools.)*

### 14.B HTTP API reference outline
Base `/api/v1`. Routes per §4.2 table. Envelope `{data, meta}`. Auth: Bearer (API key / JWT / service token — Pulse is a service-token caller). Errors: RFC 9457 + canonical `code`. Pagination: cursor (`meta.page`). Streaming: `/jobs/{id}/events` (index progress only — SSE). Idempotency: `Idempotency-Key`. Rate limit: `RateLimit-*`. Spec: `GET /openapi.json` (3.1) + `/docs`. *(No `/runs/...` agentic stream, no `/webhooks/github` — those are not Atlas.)*

### 14.C CLI reference outline
`atlas <verb>` per §4.1 tree. Global flags `--db/--tier/--repo/--config/--format/--json/--ndjson/--vectors/--vector-backend/--token/--server/--timeout/--strict/-q/-v`. Output: table(TTY)/plain(pipe)/json/ndjson. Exit codes per §4.0.2. `atlas config {init,get,set,view,path}`. `atlas install {hook,skill,completion,mcp-config}`.

### 14.D SDK reference outline
- **S4a library** (`pkg/atlas`): `Engine` interface (§4.4), `New(...Option)`, sentinel errors. **All methods synchronous (no `Job[T]`, no agentic methods).** DTO results in `pkg/apitypes` (Apache, always compiled). This is the in-process embedding path (e.g. for Pulse).
- **S4b clients** (`sdk/{go,ts,python}`): generated from one OpenAPI spec, method-isomorphic with the library. Hand facade adds auth/retry/pagination/index-`job` poll. Go (`ctx`+`io.Writer`); TS (`Promise`+async-iterator for index progress); Python (pydantic + sync/async + context-manager streams). These are how an external consumer like Pulse calls Atlas remotely.

### 14.E ADR index
- ADR-0001 — two-tier StorageDriver (keystone).
- ADR-0002 — NodeID derivation policy (signature-hash, no line numbers).
- ADR-0003 — open-core boundary (`internal/cloud` BUSL).
- ADR-0004 — CGO-free build via regex parser fallback (deferred; not v0).
- ADR-0005 — vectors optional / off-core-path package boundary.
- ADR-0006 — semantic_search as first-class op with two surface projections.

---

**Source anchors (absolute, for implementers):**
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/service/code_intelligence_service.go` — `IndexRepository`, `CrossRepoImpactForChangedPaths` (~:1145, returns `map[string]any` → typed in P1), `cloneFetchCheckout`/`gitDiffNameStatus`/`lockRepoPath`, `RunRetentionGC`. *(`BuildReviewContext` stays in Pulse — it is agentic review context assembly, not an Atlas port target.)*
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/repository/code_intelligence_repository.go` — `SaveSnapshotDelta` carry-forward (`=ANY()`/`pq.Array` → `InClause`), `EnqueueIndexJob`/`ClaimNextJob`/`ReapStaleRunningJobs`, list methods.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/service/code_search_service.go` — `Search` (default bleve mapping + `NewQueryStringQuery` → net-new analyzer + MatchQuery), `GetCallers`/`GetReferences`/`GetImpactForChangedSymbols`, `blastRadiusGuardedCtx` ambiguity guard, `SearchResult`/`FileImpact`/`CrossRepoImpact`/`UnlinkedDependency` DTOs.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/service/tree_sitter_parser.go` — parser port (CGO `import "C"`/`unsafe`, official `github.com/tree-sitter/*`); **emits no call edges → net-new**.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/service/{cross_service_analyzer,route_contract_analyzer}.go` — cross-repo analyzers.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/handlers/mcp.go` — MCP transport/dispatch/auth port.
- `/Users/damirdarasu/workspace/Aziron/aziron-pulse/internal/models/devops.go` — model port (`DependencyEdge` has `FromFile/ToRef/EdgeType` only → add symbol/node_id fields; `pq.StringArray`→`StringList`; `JSONBMap.Scan` fix).