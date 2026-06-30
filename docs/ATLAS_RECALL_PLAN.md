# Atlas Recall — Self-Building Org Automation Layer (semantic cache → distilled Cowork automations)

## Context

Atlas (`github.com/dominic097/atlas`, repo `aziron-atlas/`) is a **deterministic, LLM-free** code-intelligence engine (24 MCP tools + HTTP API + CLI over embedded SQLite / hosted Postgres). It is being **embedded into the Aziron server** and exposed org-wide, connected to Aziron's Claude and Codex (and Cowork) integrations.

The waste we're attacking: across an org, the **same prompts are issued over and over**. Atlas itself spends no tokens — the cost is the **agent** (Claude/Codex/Cowork) reasoning, planning a tool sequence, and synthesizing an answer. That reasoning is paid again every time, even when nothing relevant changed. One person asking "deploy the service" is fine; the **same query across the whole org** is pure repeated spend.

**The product (reframed to the user's intent):** Atlas should act like a self-building **Cowork automation layer**. It watches the org's prompt/outcome stream, finds the **frequently-repeated** queries, asks for **every outcome whether it can be automated**, and for the automatable hot ones **distills a workflow + script + mutable context** (a Cowork-shaped automation) with an explicit **staleness contract** — *on what change metric it goes stale so it can be recomputed*. Future matching queries **run the automation instead of re-reasoning** → tokens saved in proportion to org-wide repetition. The savings rule is built into ranking: **1 person → skip; many people → automate.**

**Division of responsibility** (per the user's decisions): Atlas owns everything **deterministic** — event ingest, frequency mining, the automation registry, replay of code-only automations, staleness tracking, savings accounting, and dynamic tool advertising. It **never calls a completion API and never executes side-effectful scripts**. The **Aziron connectors (Claude/Codex/Cowork) own the LLM key + tool orchestration + execution**: they emit events, judge automatability, distill automations, extract params, and execute side-effectful scripts (via Pulse's warm-pool runner) behind an approval gate. The future "Atlas Ask gateway" is just another internal caller of these primitives — nothing to rip out. Atlas's embeddings go through `ATLAS_EMBED_URL` → Aziron's endpoint, so Atlas holds no model key.

Working name: **Atlas Recall** (rename freely). New code lives in `aziron-atlas/internal/recall/`.

---

## IMMEDIATE DELIVERABLE — UI prototypes (3 options), then decide implementation

Before any backend build, produce a **UI for Atlas Recall** as **3 distinct prototype options**, built with a **multi-agent workflow** (one builder agent per direction, plus a shared design-spec agent so they use one token set / icon set / mock dataset and stay comparable). Each is a **self-contained single-file HTML prototype** (inline CSS/JS, faithful Lucide-icon stub, realistic mock data — no backend), matching the prototype-first convention used across Aziron.

Output location: write all three + a short `README` + this plan doc to the **`aziron-atlas` repo on its dev branch** (resolve the actual branch — `develop` if it exists, else branch `dev` off `main`), under `aziron-atlas/prototypes/atlas-recall-ui/` and `aziron-atlas/docs/ATLAS_RECALL_PLAN.md`. Do **not** open a PR or merge; just push the branch. (Per repo convention, omit the Claude co-author trailer on commits.)

The three directions (all cover the same surfaces — hit-rate/savings dashboard, hot-prompt leaderboard, automation registry, staleness monitor, approval queue — but with different information architecture):

1. **Option A — "Mission Control" command center.** Dense, dark, ops-room aesthetic (Linear/Vercel-grade), following the existing aziron-ui / Pulse mission-control theme. Top KPI strip (tokens saved, % LLM avoided, hit-rate, p95), a live event feed, the hot-prompt leaderboard as the spine, right rail for the selected cluster → its automation + staleness + savings. For operators who watch the system.

2. **Option B — "Automation Library / Skill Store."** Card/catalog-centric (echoes the skill-marketplace "Lifecycle Atlas" redesign): browse/search/filter automations as cards (state badge: candidate/validating/trusted/stale, savings, hits, kind), pin/evict, and an automation **detail** view showing the Cowork three-pane (Workflow steps / Script artifact / mutable Context) + staleness contract + version history. For people discovering and managing automations.

3. **Option C — "Cowork Flow Canvas."** Cowork-native: per-automation the literal three-pane (Progress timeline / Working-folder script / Context-memory), a pipeline/graph view of the candidate→trusted lifecycle, and an **approval-queue-first** layout for the `automatable_gated` (e.g. deploy) cases — review the plan, see the diff/blast-radius, approve/run. For the human-in-the-loop trust workflow.

**Multi-agent build shape:** a `Workflow` with (1) one design-spec agent emitting a shared `spec.json` (tokens, type scale, icon list, mock dataset of clusters/automations/events/savings), then (2) three builder agents in parallel (worktree isolation to avoid write conflicts), each consuming the spec to build one option, then (3) a verify agent that headlessly loads each HTML (Playwright + faithful Lucide stub) and confirms zero console errors + key views render. Then commit all three to the dev branch.

After review of the prototypes, we pick a direction and resume the backend implementation below.

---

## The self-building loop

```
   ┌──────────────────────────────────────────────────────────────────────────┐
   │ 1 OBSERVE   connectors emit OutcomeEvent per resolved turn                  │
   │             {prompt, prompt_norm, tool_trace, outcome, token cost,          │
   │              snapshot_id, user_id}  →  Atlas recall_events                   │
   │ 2 MINE      cluster repeated prompts (exact bucket + cosine merge);          │
   │             rank by HotScore (cross-user spread is super-linear)            │
   │ 3 DECIDE    per hot cluster ask "automatable?" (determinism × reversibility │
   │             × recurrence) — LLM-judged on connector, recorded in Atlas       │
   │ 4 DISTILL   connector LLM → Cowork automation {workflow, script, params,    │
   │             mutable context, staleness contract}                            │
   │ 5 VALIDATE  shadow-replay vs LLM (agreement ≥ τ) + invariants → trusted      │
   │ 6 SERVE     trusted automation advertised as MCP tool recall__<slug>;        │
   │             Claude/Codex call it directly (≈0 LLM). Freshness-gated.         │
   │ 7 STALE     change-metric contract trips → de-advertise → re-distill         │
   └──────────────────────────────────────────────────────────────────────────┘
```

Two layered token-savers share this loop:
- **Recall Cache (Tiers 0–1)** — exact + semantic *answer* cache for repeated Q&A: identical/near-identical prompt → return stored answer, snapshot/fingerprint-gated. Immediate savings, and it produces the event stream that mining needs.
- **Recall Automations (Tier 2)** — for hot **and automatable** clusters, a distilled callable automation that **replaces reasoning with a function**. Deeper savings; the headline feature.

---

## 1. OBSERVE — ingest the org prompt/outcome stream

The connector fires one `OutcomeEvent` per resolved turn (after its tool loop, so the outcome is terminal). Atlas only receives facts — no LLM dependency.

`OutcomeEvent`: `event_id` (idempotency), `org_id`/`scope`, `user_id` (hashed — the cross-user spread signal), `session_id`, `prompt_text`, `prompt_norm` (connector-normalized: lowercased, literals masked to params), `prompt_embedding?` (optional — if the connector already embedded it, Atlas skips the round-trip), `connector`, `repo_id`, `snapshot_id` (freshness anchor), `tool_trace[]` (the workflow skeleton seed), `outcome{status, result_kind, side_effects[], artifacts[]}`, `cost{input/output/cache tokens, tool_calls, wall_ms}`.

- **Endpoint:** `POST /api/v1/recall/events` on `aziron-atlas/internal/api/server.go routes()`, behind the existing `withAuth` bearer middleware. Idempotent on `event_id`.
- **Storage:** `recall_events` table (the missing `query_events` the audit flagged). Embeddings via `encodeVector`/`decodeVector`; brute-force cosine is fine at org-prompt cardinality.
- **Tier-gated:** ingest + mining gate to the hosted Postgres tier via a new `Capabilities().RecallRegistry` flag (SQLite local can still record its own events + replay manually-registered automations).
- **`prompt_norm` param-masking** is what makes "deploy *staging*" and "deploy *prod*" cluster together and later become the automation's typed params.

---

## 2. MINE — cluster + rank (the "many people" rule)

Runs as a periodic batch in the background loop (sibling of `startBackgroundWatch`), per scope.

- **Cluster:** (1) exact bucket on `prompt_norm` hash (zero-cost, catches the bulk); (2) greedy single-pass cosine merge of bucket centroids via the existing `rankEmbeddings`/`dotProduct` (merge when cosine ≥ τ, default 0.92). No k-means, no new lib — reuse the cosine substrate.
- **Rank — HotScore** encodes "1 person skip, whole org automate":

  ```
  HotScore = log1p(frequency)                  // saturating: 50 vs 500 shouldn't dominate
           × distinct_user_count ^ α            // α≈1.5 — SUPER-LINEAR cross-user spread
           × avg_tokens_per_run                 // = tokens saved per future replay ($ signal)
           × recency_decay                       // exp(-age/half_life): retire dead patterns
           × success_ratio                       // don't automate what people keep failing at
  ```

  One SRE running `deploy` alone → `spread=1` → low score → **skip**. Twenty engineers → `20^1.5 ≈ 89×` lift → **candidate**. Exactly the user's rule, falling out of the exponent.
- Clusters above `ATLAS_RECALL_HOT_THRESHOLD` → `recall_candidates` (status `open`) for the decide step. All weights `envInt`/`envTrue`-tunable (no redeploy).

---

## 3. DECIDE — "can this outcome be automated?" (asked for every hot cluster)

Frequency makes a cluster *worth* examining; automatability decides whether it *can* become a deterministic replay. **LLM-judged on the connector; recorded in Atlas.** Atlas pre-computes deterministic priors (intra-cluster `tool_trace` variance, side-effect histogram from `outcome.side_effects`, `success_ratio`) and ships them into the judgment prompt so the LLM confirms a structured signal rather than guessing.

Three-axis taxonomy (verdict = AND of the three):

| Axis | Automatable (green) | Not (red) |
|---|---|---|
| **Determinism** | deterministic / parameterizable (variation = finite param set) | judgment-heavy, "it depends" per run |
| **Reversibility** | read-only / idempotent / safely reversible | irreversible side-effect (prod deploy, push, money, external write) |
| **Recurrence** | stable structure across cluster members | one-off / structurally divergent |

Verdicts recorded as an append-only `AutomatabilityVerdict` (snapshot-pinned, with `confidence`, `confidence_band`, `requires_approval`, `reason`, `judge_model`):
- **automatable** — build it.
- **automatable_gated** — deterministic + org-wide but irreversible (the **deploy** case): build the *workflow*, but `requires_approval=true` → replay produces a **plan**, a human confirms execution.
- **not_automatable** — stays a normal LLM turn; parked until its structure shifts.

Confidence gate: `confidence ≥ ATLAS_RECALL_AUTOJUDGE_CONF` (e.g. 0.8) to proceed; below → human review. Re-judgment is event-driven (on staleness or material structure change), not periodic — don't burn LLM calls re-deciding settled clusters.

---

## 4. DISTILL — the Cowork-shaped Automation artifact

For a promoted candidate the connector's LLM turns the recorded `tool_trace` + outcome into an `Automation` (registered via `POST /api/v1/recall/automations`). The artifact mirrors the Cowork three-pane shape:

- **Progress pane → `Workflow`**: ordered/DAG `Steps` (`atlas_tool` | `script` | `decision` | `connector_tool`), each with `Inputs`/`Outputs` and a `Reversible` flag; `Edges` with conditions (reusing Pulse's edge-condition model).
- **Working-folder pane → `Artifacts[]`**: the script body(ies) (e.g. `cleanup-caches.sh`), content-addressed by sha256 + `Checksum` re-verified before any execution.
- **Params → `ParamSchema`**: typed fields extracted from the masked literals — how an incoming query binds to this automation.
- **Context pane → `recall_context`** (the **mutable memory**, §7): `variables`/`findings`/`referenced` (mutable, overwrite) + `citations`/`audit_ledger` (append-only). Accumulates org knowledge across runs ("the 3 cache dirs that matter", "last cleanup freed 4.2 GB") — the self-building property.
- **`StalenessContract`** (§6) — the recompute trigger.
- **`Provenance`** (mined-from cluster, connector, author hash, distill model, `ApprovedBy`) and **`SavingsAccount`** (`baseline_tokens_per_run` from the connector's usage tracking; Atlas accumulates `Hits × (baseline − replay)`).
- **`Version`** (monotonic; re-distillation = `version+1`, prior kept → rollback) and **`State`** (§5).

---

## 5. VALIDATE — candidate → validating → trusted → revoked/stale

State machine in `internal/recall/lifecycle.go`. LLM-touching transitions happen on the connector; **Atlas enforces transitions, runs deterministic checks, persists.**

- **candidate → validating:** connector POSTs the distilled artifact. Atlas validates schema + artifact checksums; **rejects a side-effectful script step lacking `Provenance.ApprovedBy`**.
- **validating → trusted:** **shadow mode** (`recall_runs.mode="shadow"`). For `code_query` automations Atlas runs the steps itself (pure engine ops, 0 LLM) and the connector returns the LLM-reasoned answer for the same events; Atlas compares (set-equality for structured outputs; cosine over answer embeddings for prose). Promote at `agreement_rate ≥ 0.95` over N ≥ 5 **and** all `Invariants` pass. For `script` automations, run the connector/Pulse sandbox and check invariants only.
- **trusted → stale:** staleness loop sets `Drifted=true` → **immediately de-advertised** from `tools/list`, kept for cheap re-distillation (reuse workflow/params; only drifted anchors recompute).
- **any → revoked:** manual (RBAC) or auto on invariant regression. Rows retained for audit.

---

## 6. STALENESS — the generalized change-metric contract (the core ask)

> "For every outcome, how can it go stale, on **what change metric**, so it can be recomputed."

A uniform `StalenessContract` holding a **set of `Signal`s**, AND-combined and **fail-closed** (any stale/errored/unknown signal ⇒ stale; "cannot prove fresh" ⇒ recompute). Every signal is one of two strategies:

- **Fingerprint** (cheap, local, ~0 cost, checked synchronously on every serve): recompute a comparable value and compare to the distill-time baseline. Cannot fail.
- **Probe** (bounded I/O, checked on the periodic sweep + opportunistically on serve, with a verdict TTL): ask the outside world; timeout/non-2xx ⇒ fail-closed stale.

A pluggable `SignalEvaluator` interface (`Type()`, `Capture()`, `Evaluate()`) with a mutable `Registry` — new change metrics = new evaluator + `Register()`, no edits to the serving path. Five built-ins:

| Signal | Strategy | Metric / capture → evaluate (reuses) |
|---|---|---|
| **code** | fingerprint | dependency file-hash subset + `snapshot_id`; `resolveSnapshot` + `File.Hash` (worktree.go) + `SnapshotDiff`. Unrelated edits bump snapshot but **don't** invalidate — only the touched files do. |
| **config/deploy** | fingerprint | hash of declared manifest globs (`deploy/**`, compose, image tags); reuse `hashContent` restricted to globs. |
| **data** | probe (+schema fingerprint) | schema-version hash (exact) and/or row-count/freshness query with `DriftPct`/`MaxAgeSec` threshold. |
| **time** | fingerprint | TTL / schedule — the cheapest signal and the universal backstop. |
| **external** | probe | health/version endpoint; mismatch or timeout ⇒ stale (reuse `embed/http.go` client shape). |

**Soundness:** false-fresh is structurally impossible for fingerprints and bounded to one sweep interval (with TTL backstop) for probes; the worst case is a safe, self-correcting false-stale (an unnecessary recompute). Distill-time and serve-time use the **same** `Capture`/`Evaluate` code, so baseline and observation can't drift apart.

**Triggers:** lazy-on-use for fingerprints (in `callTool` before serving); a `startBackgroundStalenessSweep` ticker (modeled on `watch.go`'s loop) for probes; plus an event-driven shortcut — hook the post-`refresh()`/`Index()` point so code-bound contracts whose files are in the just-indexed `changedSet()` get marked for re-eval for free.

---

## 7. SERVE — trusted automation → callable MCP tool

The payoff: Claude/Codex **invoke the automation instead of re-reasoning**. Two minimal changes at the single choke point `aziron-atlas/internal/mcp/server.go`:

- **Dynamic `tools/list`** (today returns the static `s.tools`): compose static core tools **+** trusted, non-drifted automations as tools named `recall__<slug>`, `InputSchema` generated from `ParamSchema`, `Description` = the gating prose. Cache the dynamic list keyed on registry `updated_at` (reuse the `contextcache.go` deep-copy LRU). When the registry is absent (`recall==nil`), behavior is byte-identical to today.
- **`callTool` default branch**: a `recall__` prefix → `serveAutomation(slug, args)`:
  1. resolve latest trusted version; if drifted/absent → return a **degrade** result (`{"status":"stale"}`, `isError:false`) so the agent falls back to normal reasoning (preserves the existing "degrade, don't abort" contract);
  2. **freshness gate** via the staleness contract;
  3. bind params from `args`;
  4. execute by `Kind`; write the mutable context; increment `Savings.Hits`/`TokensSavedTotal`; record a `recall_runs` row.

**Who executes what** (honors "Atlas won't own tool-calls" + reversibility):

| `Kind` | Executor | Why |
|---|---|---|
| `code_query` / code-only `workflow` | **Atlas, in-process** | pure engine ops (search/impact/explain/…), deterministic, side-effect-free, ≈0 LLM |
| `script` (deploy/cleanup/…) | **Connector / Pulse warm-pool runner executes; Atlas only returns a verified execution descriptor (artifact body + checksum + resolved params) and records the outcome** | side-effectful — Atlas never shells out |

**Approval gate:** any `Reversible=false` step (or side-effectful script) is served as `status:"awaiting_approval"` + descriptor + `run_id`; the connector surfaces the Cowork approval UI; on approval it executes and POSTs the outcome back. Destructive automations stay human-gated; Atlas stays out of the execution path entirely.

---

## Data model (new tables — `internal/recall/`, both `sqlite_schema.go` + `postgres_schema.go`)

Additive `CREATE TABLE IF NOT EXISTS` (idempotent `Migrate` picks them up); TEXT PK + JSON-as-TEXT on SQLite / JSONB on Postgres, matching the six existing tables. Postgres (the shared org tier) is additive-only.

- `recall_events` — the prompt/outcome stream (mining substrate). Indexed `(scope, query_hash)`, `(scope, created_at DESC)`. Optional `query_vec` BLOB.
- `recall_candidates` — hot clusters awaiting distillation (`frequency`, `user_count`, `avg_tokens`, `automatable`, `status`).
- `recall_automations` — the registry, versioned (`UNIQUE(scope, slug, version)`); JSON columns for `workflow`/`artifacts`/`params`/`staleness`/`validation`/`provenance`/`savings`.
- `recall_context` — the mutable memory blob; `checkpoint` for optimistic CAS on concurrent replays; mutable `variables`/`findings`/`referenced` + append-only `citations`/`audit_ledger`.
- `recall_runs` — run history / Progress timeline; `mode` (shadow|replay|approved), `snapshot_id` (freshness proof), `tokens_saved`.
- `staleness_signals` — lightweight index of each automation's probe signals so the sweep pulls just those (the full contract lives as JSON on `recall_automations`).

**`StorageDriver`** (extend interface in `aziron-atlas/internal/store/store.go`; implement in `sqlite.go` + `postgres.go`, mirroring `SaveEmbeddings`/`NearestSymbols`): `RecordRecallEvent`, `RecentRecallEvents`, `CountByQueryHash`, `UpsertRecallCandidate`, `ListRecallCandidates`, `SaveAutomation`, `GetAutomation(scope,slug,version=0→latest-trusted)`, `ListAutomations(scope,state)`, `SetAutomationState`, `LoadContext`, `CommitContext(expectedCheckpoint)` (CAS), `SaveRun`, `ListRuns`. Add `Capabilities.RecallRegistry` (hosted-only, gated like `PushReindex`). Add value types to `internal/graph/model.go` (or `internal/recall/`).

---

## Reuse map (grounded — do not reinvent)

| Need | Reuse | Location |
|---|---|---|
| Embed prompts (no LLM) | `embed.Provider.Embed`, `ATLAS_EMBED_URL` | `internal/embed` |
| Cluster + answer-equality | `encodeVector`/`decodeVector`/`rankEmbeddings`/`dotProduct` | `internal/store/embeddings.go` |
| Context-read + dynamic-tool-list cache | deep-copy snapshot-keyed LRU | `internal/engine/contextcache.go` |
| Staleness fingerprints | `resolveSnapshot`, `File.Hash`/`scanWorkTree`/`changedSet`, `SnapshotDiff`, `route_contracts` | `internal/engine/engine.go`, `internal/index/worktree.go` |
| Tenant isolation | `Config.Scope` / `WithScope` | `internal/engine/engine.go` |
| Dynamic tools + replay choke point | `tools/list` + `callTool` switch | `internal/mcp/server.go` |
| Ingest/register endpoints | `routes()` + `withAuth` | `internal/api/server.go` |
| Staleness sweep loop | `startBackgroundWatch` + `Watcher.loop` timer | `internal/cli/serve.go`, `internal/watch/watch.go` |
| Feature/tier gating | `WithVectors`/`ATLAS_ENABLE_VECTORS`, `envTrue`/`envInt`, `store.Capabilities` | engine + store |
| **Execution + LLM orchestration (connector side)** | Pulse `FlowOrchestrator`/`FlowGraph` (DAG), `flow_memory` (bounded mutable memory), cron scheduler, warm-pool `runner.js`, `aziron_llm_client` (per-request model/provider + token usage) | `aziron-pulse/internal/service/*`, `docker/.../runner.js` |
| Provenance / append-only ledger shape | RCA citation/evidence-ledger model | `aziron-pulse` RCA models |

---

## Phasing (each independently shippable; deterministic core untouched when flag off)

- **P0 — Observe + Cache.** `recall_events` ingest endpoint + tier gate; the exact + semantic **answer cache** (Tiers 0–1) with snapshot + dependency-fingerprint freshness; ship in **shadow mode** (log would-be hits, always serve live) to tune the similarity threshold and prove **zero stale served**, then flip to serve-cached per scope with a kill-switch. Immediate token savings + builds the mining data.
- **P1 — Mine + Decide.** Background miner: clustering + HotScore (cross-user spread); `recall_candidates`; record `AutomatabilityVerdict` (connector-judged); admin hot-prompt leaderboard.
- **P2 — Distill + Serve (code automations).** `recall_automations`/`recall_context`/`recall_runs`; `POST /recall/automations` register; shadow validation → trusted; dynamic `recall__<slug>` MCP tools + in-process replay; savings ledger. Fingerprint staleness signals (code/config/time) + lazy-on-use gate + event-driven invalidation.
- **P3 — General automations + probes + approval.** `script`/`workflow` automations executed by connector/Pulse; reversibility/approval gate (the deploy case); probe staleness signals (data/external) + periodic sweep; mutable cross-run learning.

---

## Decisions / risks (flagged, defaults chosen)

- **Execution boundary:** Atlas never executes side effects or calls an LLM — connector/Pulse does, approval-gated for irreversible automations. Keeps the deterministic core and matches "Atlas won't own the LLM/tool-calls for now"; the future Ask gateway slots in as another caller.
- **Correctness over hit-rate:** high similarity threshold, fail-closed staleness, shadow-validation before trust, and a discriminator guard against negation/direction false matches. A miss costs one LLM call; a wrong cached/automated answer poisons the whole org.
- **Privacy:** prompts are sensitive — hash `user_id`, isolate by `scope`, share answers only within a scope where all users could already run the underlying tools; retention/GC on `recall_events`.
- **LLM-authored scripts:** reconciled by **validation-gating** — the connector's LLM may *generate* the automation, but Atlas only trusts it after shadow-replay agreement + invariants; replay itself never invents (executes recorded/declared steps).

---

## Verification

- **Unit (Go):** `recall` package — clustering (exact + cosine merge), HotScore (assert 1-user cluster ranks below a 20-user one at equal frequency), staleness evaluators (fingerprint fresh on unrelated change, stale on dep change/delete; probe fail-closed on timeout; AND semantics), context CAS under concurrent commits, savings arithmetic. Store round-trips on **both** drivers, mirroring `store_embeddings_test.go`.
- **Lifecycle:** register → shadow-replay agreement promotes to trusted; invariant break blocks; drift de-advertises; `version+1` rollback resolves prior trusted.
- **E2E:** `atlas serve` with the flag + `ATLAS_EMBED_URL` → POST `recall/events` (miss) → register a `code_query` automation → `tools/list` shows `recall__<slug>` → `tools/call` replays it with **no model call** → edit a dependency file → tool de-advertised/degrades → re-register. Confirm a `script` automation returns `awaiting_approval` (never executed by Atlas).
- **Metrics/ROI:** `bench/` harness reporting cache+automation hit-rate, % LLM calls avoided, tokens-$ saved (from `SavingsAccount`), p50/p95 replay latency vs the live LLM path; assert **0** stale-served in shadow logs before enabling serve.

## Critical files

- `aziron-atlas/internal/mcp/server.go` — `tools/list` compose + `callTool` default branch (dynamic tools + replay; trace capture).
- `aziron-atlas/internal/store/store.go`, `sqlite_schema.go`, `postgres_schema.go`, `sqlite.go`, `postgres.go` — new tables + driver methods + `Capabilities.RecallRegistry`.
- `aziron-atlas/internal/store/embeddings.go` — reuse cosine helpers for clustering + answer-equality.
- `aziron-atlas/internal/engine/engine.go` + `internal/index/worktree.go` — `resolveSnapshot`/`SnapshotDiff`/`File.Hash` staleness primitives; `Config`/option gating; expose recall ops on the `Engine` interface.
- `aziron-atlas/internal/engine/contextcache.go` — template for context-read + dynamic-tool-list caching.
- `aziron-atlas/internal/api/server.go` — `POST /recall/events`, `POST/GET /recall/automations`, `POST /recall/automations/{id}/runs` behind `withAuth`.
- `aziron-atlas/internal/cli/serve.go` + `internal/watch/watch.go` — background miner + staleness sweep goroutines.
- **New:** `aziron-atlas/internal/recall/` — `events.go`, `mine.go`, `automation.go`, `lifecycle.go`, `staleness.go`, `evaluator.go`, `context.go`, `config.go`.
- **Connector side (Aziron/Pulse, separate):** OutcomeEvent emit + token attach; automatability judgment; distillation + param extraction; approval UI; side-effectful execution via the warm-pool runner (reuse `FlowOrchestrator`/`runner.js`/`aziron_llm_client`).
