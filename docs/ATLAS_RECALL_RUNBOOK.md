# Atlas Recall — Implementation Runbook

> **Audience:** a capable coding agent (e.g. GPT‑5.5) implementing Atlas Recall end‑to‑end **without** access to the conversation that produced it. Everything you need is in this file plus the two referenced documents:
> - **Plan (backend design):** `aziron-atlas/docs/ATLAS_RECALL_PLAN.md`
> - **Data contract (mock dataset + tokens):** `…/scratchpad/atlas-recall-spec.json` (the same JSON is reproduced inline in §3.3 below — treat §3.3 as authoritative for the embedded dashboard).
>
> All file paths are absolute-from-repo-root under `aziron-atlas/`. The Go module is `github.com/dominic097/atlas`. Work on branch `develop` (it exists). Per repo convention, **omit the Claude co‑author trailer on commits.**

---

## 1. Context & goal

**The waste.** Atlas is a deterministic, LLM‑free code‑intelligence engine (24 MCP tools + REST API + CLI over embedded SQLite / hosted Postgres), being embedded into the Aziron server and exposed org‑wide behind Aziron's Claude, Codex, and Cowork connectors. Atlas itself spends **no** tokens. The cost is the **agent** (Claude/Codex/Cowork) reasoning, planning a tool sequence, and synthesizing an answer — and across an org the **same prompts are issued over and over**. That reasoning is paid again every time, even when nothing relevant changed. One SRE running "deploy the service" is fine; the same query across twenty engineers is pure repeated spend.

**The product.** Atlas Recall makes Atlas a self‑building **automation layer**. It watches the org's prompt/outcome stream, finds the frequently‑repeated queries, asks for **every hot outcome whether it can be automated**, and for the automatable ones **distills a Cowork‑shaped automation** (workflow + script + mutable context) with an explicit **staleness contract**. Future matching queries **run the automation instead of re‑reasoning** → tokens saved in proportion to org‑wide repetition. The savings rule is built into ranking: **1 person → skip; many people → automate.**

**Two layered token‑savers** share one loop:
- **Recall Cache (Tiers 0–1)** — exact + semantic *answer* cache for repeated Q&A. Immediate savings, and it produces the event stream mining needs.
- **Recall Automations (Tier 2)** — for hot **and automatable** clusters, a distilled callable automation that replaces reasoning with a function. The headline feature.

### Division of responsibility — Atlas vs the Aziron connector

This boundary is **load‑bearing**; every design decision flows from it.

| Concern | Owner | Notes |
|---|---|---|
| Event ingest, frequency mining, automation registry, replay of **code‑only** automations, staleness tracking, savings accounting, dynamic tool advertising | **Atlas** | All deterministic. |
| **Calling a completion API** (judging automatability, distilling automations, extracting params, summarizing) | **Connector (Claude/Codex/Cowork)** | Holds the LLM key + tool orchestration. |
| **Executing side‑effectful scripts** (deploy/cleanup/migrate) | **Connector / Pulse warm‑pool runner** | Behind an approval gate. |
| Embeddings | **Aziron endpoint** via `ATLAS_EMBED_URL` | Atlas holds **no** model key. |

> **Two hard invariants, enforced in code, that you must never violate:**
> 1. **Atlas never calls an LLM.** Embeddings go out over `ATLAS_EMBED_URL` (already the case). Any judgment/distillation/summarization is performed by the connector and POSTed to Atlas as recorded facts.
> 2. **Atlas never executes a side effect.** For `script`/side‑effectful `workflow` automations Atlas returns a *verified execution descriptor* (artifact body + checksum + resolved params + `run_id`) and records the outcome the connector POSTs back. It never shells out.
>
> The future "Atlas Ask gateway" is just another internal caller of these primitives — nothing here needs to be ripped out for it.

---

## 2. Architecture recap

### 2.1 The 7‑step self‑building loop

```
1 OBSERVE   connector emits one OutcomeEvent per resolved turn
            {prompt, prompt_norm, prompt_embedding?, tool_trace, outcome,
             token cost, snapshot_id, user_id(hashed)} → recall_events
2 MINE      cluster repeated prompts (exact bucket on prompt_norm hash +
            cosine merge of centroids); rank by HotScore (cross-user spread
            is SUPER-linear) → recall_candidates
3 DECIDE    per hot cluster ask "automatable?" (determinism × reversibility
            × recurrence). LLM-judged ON THE CONNECTOR, recorded in Atlas as
            an append-only AutomatabilityVerdict
4 DISTILL   connector LLM → Cowork automation {workflow, artifacts[], params,
            mutable context, staleness contract} → recall_automations
5 VALIDATE  shadow-replay vs LLM (agreement ≥ τ) + invariants → trusted
6 SERVE     trusted automation advertised as MCP tool recall__<slug>;
            agents call it directly (≈0 LLM). Freshness-gated, fail-closed.
7 STALE     change-metric contract trips → de-advertise → re-distill
```

### 2.2 The three serving tiers (how a turn is answered)

Every served turn is tagged with one `servedBy` value (a **closed enum** the UI badges depend on):

| Tier | `servedBy` | What it is | Cost |
|---|---|---|---|
| 0 | `exact` | identical `prompt_norm` hash hit on a fresh cached answer | ~0 |
| 1 | `semantic` | cosine ≥ τ near‑match to a cached answer, snapshot/fingerprint‑gated | ~0 (one embed) |
| 2 | `automation` | a trusted `recall__<slug>` automation replayed deterministically | ~0 LLM (replay tokens only) |
| — | `live` | cache+automation miss → normal LLM turn (also the event that feeds mining) | full |

### 2.3 Freshness model (the core correctness mechanism)

A uniform **`StalenessContract`** holds a **set of `Signal`s**, **AND‑combined and fail‑closed**: *any* stale, errored, or unknown signal ⇒ the whole contract is stale ("cannot prove fresh" ⇒ recompute). Every signal uses one of two strategies:

- **Fingerprint** (cheap, local, ~0 cost, checked **synchronously on every serve**): recompute a comparable value and compare to the distill‑time baseline. Cannot fail (no I/O).
- **Probe** (bounded I/O, checked on the periodic sweep + opportunistically on serve, with a verdict TTL): ask the outside world; **timeout / non‑2xx ⇒ fail‑closed stale.**

Two freshness anchors back the fingerprints, both already in Atlas:
- **`snapshot_id`** — the per‑commit immutable graph state (`graph.Snapshot.ID`). An automation records the snapshot it was distilled against.
- **per‑file content SHA** — `graph.File.Hash`, computed by `scanWorkTree`/`hashContent` in `internal/index/worktree.go`. Unrelated edits bump the snapshot but **do not** invalidate a code‑bound contract — only the *touched dependency files* (intersection with `changedSet()`) do.

A pluggable **`SignalEvaluator`** interface (`Type()`, `Capture()`, `Evaluate()`) with a mutable **`Registry`** means a new change metric = a new evaluator + `Register()`, with **no edits to the serving path**. Five built‑ins:

| Signal | Strategy | Metric / capture → evaluate (reuse) |
|---|---|---|
| **code** | fingerprint | dependency file‑hash subset + `snapshot_id`; `resolveSnapshot` + `File.Hash` + `SnapshotDiff`. Only touched dep files invalidate. |
| **config/deploy** | fingerprint | hash of declared manifest globs (`deploy/**`, compose, image tags); reuse the content hasher restricted to globs. |
| **data** | probe (+ schema fingerprint) | schema‑version hash (exact) and/or row‑count/freshness query with `DriftPct`/`MaxAgeSec` threshold. |
| **time** | fingerprint | TTL / schedule — cheapest signal and the universal backstop. |
| **external** | probe | health/version endpoint; mismatch or timeout ⇒ stale (reuse `internal/embed/http.go` client shape). |

**Soundness:** false‑fresh is structurally impossible for fingerprints and bounded to one sweep interval (TTL backstop) for probes; the worst case is a safe, self‑correcting false‑stale (an unnecessary recompute). Distill‑time and serve‑time use the **same** `Capture`/`Evaluate` code, so baseline and observation cannot drift apart.

**Triggers:** lazy‑on‑use for fingerprints (in `callTool`, before serving); a `startBackgroundStalenessSweep` ticker (modeled on `watch.go`'s debounced loop) for probes; plus an event‑driven shortcut — hook the post‑`Index()` point so code‑bound contracts whose files are in the just‑indexed `changedSet()` get marked for re‑eval for free.

---

## 3. The UI — the merged "Atlas Recall" dashboard

### 3.1 What it is, and how it ships

One merged single‑page dashboard fusing **Option A "Mission Control"** (observability + token‑savings command center) with **Option C "Cowork Flow"** (automation lifecycle, three‑pane Cowork detail, human‑approval workflow). Every view answers either *"how much are we saving?"* (A) or *"can I trust/approve this automation?"* (C).

- **Prototype source files** (single‑file HTML, inline CSS/JS, faithful Lucide stub, mock data, no backend):
  - A: `aziron-atlas/prototypes/atlas-recall-ui/option-a-mission-control.html`
  - C: `aziron-atlas/prototypes/atlas-recall-ui/option-c-cowork-canvas.html`
- **Build target (the merged file you produce):** `aziron-atlas/prototypes/atlas-recall-ui/atlas-recall-dashboard.html`
- **Shipping:** the merged HTML is embedded into the Atlas Go binary via `//go:embed` and served by `atlas serve` at **`GET /dashboard`**, added to `internal/api/server.go`'s `routes()` on `s.mux` (no auth — it's a discovery surface like `/openapi.json`; or behind `withAuth` if `ATLAS_API_TOKEN` is set and you choose to gate it). It is a self‑contained file: it boots from inlined mock `DATA` and, when the real endpoints (§5) exist, swaps `DATA` for `fetch()` calls against the same shapes. The dashboard never calls an LLM and reads only the recall REST surface.

Embed wiring (new, minimal — put the embed directive in a small Go file in `internal/api/`, e.g. `internal/api/dashboard.go`):

```go
package api

import (
	_ "embed"
	"net/http"
)

//go:embed assets/atlas-recall-dashboard.html
var dashboardHTML []byte

func (s *Server) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(dashboardHTML)
}
```

Copy the built `atlas-recall-dashboard.html` to `internal/api/assets/atlas-recall-dashboard.html` at build time (or commit it there directly — `//go:embed` needs the file inside the package directory tree). Register in `routes()`:

```go
m.HandleFunc("GET /dashboard", s.handleDashboard)
```

> **Note on `--mcp` mounting:** when `serve --mcp` wraps the REST handler behind a parent mux (`internal/cli/serve.go` mounts `POST /mcp` then delegates `/` to `srv.Handler()`), `GET /dashboard` continues to work because it falls through to the REST handler. No change needed there.

### 3.2 Views, layout, and components

**Global shell — keep Option A's Mission Control chrome** (left‑nav, not C's top‑tabs):

- App grid: `grid-template-columns: 228px 1fr; grid-template-rows: auto 1fr; grid-template-areas: "head head" / "nav main"`.
- Header (54px): brand (layers glyph) + "Atlas **Recall**" wordmark; a `served by atlas serve` pill (`.served-hint` + pulsing dot, title="embedded in the Atlas Go binary, served by atlas serve"); global search (`/` kbd); a **Live toggle** (pause/resume the event feed); a **theme toggle** (sun/moon).
- Left nav: section labels + rows with active rail + data‑derived `.badge` counts. Two groups:
  - **OPERATE:** Overview · Hot Prompts `[clusters.length]` · Automations `[automations.length]`
  - **MAINTAIN:** Staleness `[danger: # automations with staleness.drifted===true]` · Approvals `[warn: # approvals with status==='awaiting']` · Events
- Theme tokens: adopt the `palette` block from §3.3 verbatim (dark default via `data-theme="dark"`, `html[data-theme="light"]` overrides). They are byte‑identical across A, C, and the spec.

**Six views** (routing = A's `state.view` + `VIEWS` map + `setView(v)` + single `render()` dispatcher):

1. **Overview** *(source A)* — the savings command center. Top‑to‑bottom: KPI strip (5–6 cards from `kpis`, with `monthlyUsdSaved` as the $ proof and `tokensSavedTotal` as the feature card); 30‑day trend (hand‑rolled SVG, Tokens‑saved | Hit‑rate toggle, from `series`); served‑by donut (over `tierBreakdown` — proves replay displacing live LLM); hot‑prompt leaderboard (sortable/filterable, the spine); right rail = selected‑cluster detail (verdict + linked automation + staleness + savings + mini feed). Prefer grid `1fr 340px` so leaderboard + rail read as one master/detail ops surface.
2. **Hot Prompts** *(source A, verdict copy from C)* — full‑width sortable leaderboard (Prompt cluster / Freq / Users / Avg tok / HotScore bar / Automatable verdict / State) + filter box + verdict chips (`all/yes/gated/no/unknown`); right rail = cluster detail. Fold in C's richer verdict block: the "1 person → skip; many people → automate" banner and the 3‑axis (determinism × reversibility × recurrence) `verdict-grid`.
3. **Automations** *(MERGE — the keystone view)* — layout = C's `268px 1fr`: (a) **lifecycle header** (C `renderLifecycle`): `candidate → validating → trusted → stale` pipeline strip with live counts, each stage clickable to filter; (b) **left registry list** (C's compact searchable/filterable list, keyboard ↑/↓ nav, primary; A's richer card "gallery" as an optional density toggle); (c) **detail = C's Cowork three‑pane** (`renderAutoDetail`): **Pane 1 Progress/Workflow** (timeline of `workflow.steps`, irreversible step tagged), **Pane 2 Working‑folder/Artifact** (line‑numbered `artifact.bodyExcerpt` + baseline‑vs‑replay token stat‑minis), **Pane 3 Context/mutable memory** (`context.variables`/`findings`/`referenced`); plus a **staleness sub‑panel** (per‑signal rows: fresh/drift, type, strategy, detail).
4. **Staleness** *(source A)* — org‑wide change‑metric monitor: one `.stale-row` per automation, drifted‑first, each with its `staleness.signals[]` chips (fresh/stale dot, type, strategy badge, detail) + actions (recompute, open automation).
5. **Approvals** *(source C — approval‑queue‑first)* — banner (human‑in‑the‑loop framing + awaiting count) → `1.3fr 1fr`: left = one card per awaiting approval (id, requester, param rows, **blast‑radius** block, Approve / Run / Reject actions; resolved cards dim); right = the gated `deploy-service-to-env` workflow timeline highlighting the irreversible step + a recent‑approval activity log. Keep the nav `warn` badge synced to `status==='awaiting'`.
6. **Events** *(source A)* — full live feed table over `events` (time, user hash, connector chip, prompt, repo, **served‑by** badge `automation/exact/semantic/live`, tokens, latency); searchable; driven by the header Live toggle's `pushEvent` simulator.

**Single dataset + the TDZ trap (must‑follow source ordering).** Inline **one** `const DATA = {…}` mirroring §3.3 exactly. Only mutable UI state lives in `state` (selections, filters, sort, theme, live on/off, and `approvals` as a deep clone: `JSON.parse(JSON.stringify(DATA.approvals))`). Option C hit a **temporal‑dead‑zone bug**: `setTheme()` reads `state`, but the initial theme apply ran while `const state` was still in its TDZ (C worked around it with an inline attribute set instead of calling `setTheme`). Avoid it by **strict source ordering** in the single `<script>` at end of `<body>`:

1. Constants & pure helpers first (`DATA`, icons, formatters, color/verdict maps — **no** `state` refs, **no** DOM reads).
2. **Declare `state` next** — before any function or top‑level statement that references it.
3. Then all `render*`/`view*` functions and `setTheme` (they may *reference* `state` because they only *run* later).
4. Then `addEventListener` wiring.
5. **Initial render strictly last** — one bottom block: `applyThemeIcon(); render();`.

**Rule:** no top‑level call (theme apply, render, feed start, keyboard binding) may execute above the `state` declaration; the single initial‑render call is the last line of the script. This removes the need for C's inline‑apply workaround entirely.

### 3.3 The exact mock‑data JSON shape (authoritative `DATA`)

The dashboard's inlined `DATA` and `palette` are exactly the `data` and `palette` objects of `…/scratchpad/atlas-recall-spec.json`. The structure (all keys closed sets where noted) is:

```jsonc
{
  "palette": { "dark": {…12 tokens…}, "light": {…12 tokens…}, "chart": ["#6366f1","#a78bfa","#34d399","#60a5fa","#fbbf24","#fb7185"] },
  "data": {
    "kpis": {
      "tokensSavedTotal": 62770896, "tokensSavedTrendPct": 23.7,
      "pctLlmAvoided": 0.7324, "hitRate": 0.7324,
      "p50ms": 41, "p95ms": 138, "trustedAutomations": 3,
      "eventsToday": 1287, "monthlyUsdSaved": 194.59
    },
    "series": { "days": ["2026-06-01", … 30 ISO dates …],
                "tokensSaved": [ … 30 ints, ~715k→1.78M … ],
                "hitRate":     [ … 30 floats, 0.38→0.74 … ] },
    "tierBreakdown": { "exactCache": 5120, "semanticCache": 3380,
                       "automationReplay": 4203, "liveLlm": 4642 },
    "clusters": [
      { "id": "clu_01", "promptNorm": "who calls {symbol}",
        "exemplar": "who calls FlowOrchestrator.dispatch?",
        "frequency": 1842, "distinctUsers": 23, "avgTokens": 14200,
        "automatable": "yes",          // enum: yes | gated | no | unknown
        "state": "automated",          // enum: automated | validating | candidate | rejected | stale | not_promoted
        "repo": "aziron-pulse", "lastSeen": "2026-06-30T09:41:00Z",
        "hotScore": 2885.2 },
      … 10 clusters total (clu_01…clu_10); clu_10 is the 1-user/low-score "skip" case …
    ],
    "automations": [
      { "id": "auto_001", "slug": "callers-of-symbol", "title": "Who calls {symbol}?",
        "kind": "code_query",          // enum: code_query | script | workflow
        "state": "trusted",            // enum: candidate | validating | trusted | stale | rejected | not_promoted
        "version": 7, "hits": 1842,
        "baselineTokensPerRun": 14200, "replayTokensPerRun": 38, "agreementRate": 0.992,
        "params": [ { "name": "symbol", "type": "string", "required": true },
                    { "name": "repo", "type": "string", "required": false } ],
        "workflow": { "steps": [ { "id":"s1","name":"…","type":"lookup","reversible":true }, … ],
                      "edges": [ { "from":"s1","to":"s2","condition":"symbol_found" }, … ] },
        "artifact": { "name": "callers.atlas.js", "lang": "javascript",
                      "bodyExcerpt": [ "…lines of the script…" ] },
        "context": { "variables": {…kv…}, "findings": {…kv…}, "referenced": ["path", …] },
        "staleness": { "signals": [
                         { "type":"code","strategy":"fingerprint","fresh":true,"detail":"…" },
                         { "type":"time","strategy":"probe","fresh":true,"detail":"…" } ],
                       "drifted": false },
        "provenance": { "minedFromCluster":"clu_01","connector":"claude",
                        "author":"atlas-distiller","approvedBy":"auto-trust (…)" },
        "requiresApproval": false, "tokensSaved": 26086404 },
      … 6 automations total: auto_001/002/003 trusted code_query; auto_004 deploy (kind:workflow,
        state:validating, requiresApproval:true); auto_005 find-dead-config (kind:script, candidate);
        auto_006 migration-status (kind:script, state:stale, staleness.drifted:true) …
    ],
    "approvals": [
      { "id": "apr_001", "automationSlug": "deploy-service-to-env", "requestedBy": "u_7af3",
        "params": { "service": "pulse", "env": "prod" },
        "blastRadius": "prod namespace …; rolling restart of 4 pods; ~64s; reversible via rollback",
        "requestedAt": "2026-06-30T09:12:00Z",
        "status": "awaiting" },           // enum: awaiting | approved | rejected | ran
      … 3 awaiting approvals (apr_001 pulse/prod, apr_002 atlas/stg, apr_003 canvas/prod) …
    ],
    "events": [
      { "ts": "2026-06-30T09:41:12Z", "userHash": "u_7af3", "connector": "claude", // enum: claude | codex | cowork
        "prompt": "who calls FlowOrchestrator.dispatch?", "repo": "aziron-pulse",
        "servedBy": "automation",         // enum: automation | exact | semantic | live
        "tokens": 38, "latencyMs": 44 },
      … 14 events total, mix of automation/exact/semantic/live …
    ]
  }
}
```

> **Invariants the mock preserves (preserve them in live aggregates too, or the Overview math lies):** `kpis.hitRate === kpis.pctLlmAvoided`; `automations[i].tokensSaved ≈ hits × (baselineTokensPerRun − replayTokensPerRun)`; nav badge counts are *derived* from the arrays, never hardcoded.

### 3.4 Field‑by‑field mapping: mock section → real Atlas Recall API

When the backend lands, the dashboard replaces inlined `DATA.<section>` with a `fetch` against the matching endpoint. Shapes are the contract of record; the response `data` envelope is `{ "data": … }` (matching the existing API's `writeResult`).

| `DATA` section | Backing endpoint (method, path) | Source tables / compute | Plan ref |
|---|---|---|---|
| `kpis` | `GET /api/v1/recall/metrics` | aggregates over `recall_runs` + per‑automation `savings` (the `SavingsAccount`) | §SavingsAccount |
| `series` | `GET /api/v1/recall/metrics/series?window=30d` | time‑bucketed `recall_runs.tokens_saved` + hit‑rate | bench harness |
| `tierBreakdown` | `GET /api/v1/recall/served-by` | counts of `recall_events`/`recall_runs` by serve tier (`exact`/`semantic`/`automation`/`live`) | §SERVE, §2.2 |
| `clusters` | `GET /api/v1/recall/candidates` | `recall_candidates` joined with latest `AutomatabilityVerdict`; `hotScore` from the MINE formula | §2 MINE, §3 DECIDE |
| `automations` (list) | `GET /api/v1/recall/automations?state=<opt>` | `recall_automations` (latest version per slug) | §4 DISTILL, §5 |
| `automations[i]` (detail: workflow/artifact/context/staleness) | `GET /api/v1/recall/automations/{slug}` | `recall_automations` + `recall_context` + recent `recall_runs` | §4, §6, data model |
| `automations[i].staleness.signals[]` | included in the `{slug}` detail; sweep status from `staleness_signals` | `SignalEvaluator.Evaluate()` over the contract | §6 STALENESS |
| `approvals` | `GET /api/v1/recall/approvals` (list) | `recall_runs` rows in `awaiting_approval`/`approved` mode | §7 approval gate |
| approval actions | `POST /api/v1/recall/approvals/{id}/approve` · `…/reject` · `…/run` | mutate the `recall_runs` row; `run` returns the verified execution descriptor for the connector to execute | §7 |
| `events` | `GET /api/v1/recall/events?recent=N` | `recall_events` (the live feed) | §1 OBSERVE |
| (write path, not displayed) | `POST /api/v1/recall/events` | idempotent insert into `recall_events` | §1 OBSERVE |
| (write path) | `POST /api/v1/recall/automations` | register a distilled artifact (`candidate`→`validating`) | §4 DISTILL |
| (write path) | `POST /api/v1/recall/automations/{slug}/runs` | record a shadow/replay/approved run outcome | §5 VALIDATE, §7 |

Contract notes for the UI: all reads are **scope‑isolated** (`Config.Scope`); `userHash` is pre‑hashed (never a raw user id); the `state`/`automatable`/`servedBy`/`status`/`signal.strategy` enums are **closed sets** — adding a value requires a matching badge CSS class.

---

## 4. Backend data model

New code lives in **`aziron-atlas/internal/recall/`**. New tables are **additive `CREATE TABLE IF NOT EXISTS`** in both `internal/store/sqlite_schema.go` (TEXT PK, JSON‑as‑TEXT, RFC3339 time) and `internal/store/postgres_schema.go` (JSONB, additive‑only — the shared org tier). Value types go in `internal/graph/model.go` (or a `model.go` under `internal/recall/`).

> **⚠ Sharp edge — SQLite `Migrate` drops all Atlas tables on a schema‑version bump.** `internal/store/sqlite.go` keys DDL replay off `PRAGMA user_version` vs `sqliteSchemaVersion`, and on *any* mismatch it runs `dropAllSQLite` (which lists only `repos/snapshots/files/symbols/edges/routes/coverage/embeddings`) then re‑applies `schemaSQLite`. **Recall tables are NOT in `dropAllSQLite`, so recall rows survive a code/graph schema bump** (good — they are persistent automation state, not rebuildable from a reindex). To keep that property:
> - Append the recall `CREATE TABLE IF NOT EXISTS` DDL to `schemaSQLite` so it is (re)created on every migrate, **and**
> - Do **not** add recall tables to `dropAllSQLite`. (If you ever need to evolve a recall table's shape, write a real additive `ALTER`/migration; never let the drop‑and‑recreate path touch automation state.)

### 4.1 Tables (columns)

**`recall_events`** — the prompt/outcome stream (mining substrate; the missing "query_events" an audit flagged).

| column | type (sqlite / pg) | notes |
|---|---|---|
| `event_id` | TEXT PK | idempotency key |
| `scope` | TEXT | tenant/org isolation |
| `user_hash` | TEXT | pre‑hashed; never raw user id |
| `session_id` | TEXT | |
| `connector` | TEXT | `claude`/`codex`/`cowork` |
| `repo_id` | TEXT | |
| `snapshot_id` | TEXT | freshness anchor |
| `prompt_text` | TEXT | |
| `prompt_norm` | TEXT | normalized, literals masked → params |
| `query_hash` | TEXT | hash of `prompt_norm` (exact‑bucket key) |
| `query_vec` | BLOB / BYTEA, nullable | `encodeVector` output; optional |
| `tool_trace` | TEXT(JSON) / JSONB | the workflow‑skeleton seed |
| `outcome` | TEXT(JSON) / JSONB | `{status, result_kind, side_effects[], artifacts[]}` |
| `cost` | TEXT(JSON) / JSONB | `{input,output,cache tokens, tool_calls, wall_ms}` |
| `served_by` | TEXT | tier the turn was served at (§2.2 enum) |
| `created_at` | TEXT(RFC3339) / TIMESTAMPTZ | |

Indexes: `(scope, query_hash)`, `(scope, created_at DESC)`.

**`recall_candidates`** — hot clusters awaiting distillation.

`cluster_id` TEXT PK · `scope` · `prompt_norm` · `exemplar` · `frequency` INT · `user_count` INT · `avg_tokens` INT · `hot_score` REAL · `automatable` TEXT (`yes/gated/no/unknown`) · `verdict` TEXT(JSON) (the append‑only `AutomatabilityVerdict`: `confidence`, `confidence_band`, `requires_approval`, `reason`, `judge_model`, `snapshot_id`) · `status` TEXT (`open`/`promoted`/`rejected`) · `repo` · `last_seen` · `created_at`/`updated_at`. Index `(scope, hot_score DESC)`.

**`recall_automations`** — the registry, versioned.

`id` TEXT PK · `scope` · `slug` · `version` INT · `title` · `kind` TEXT (`code_query/script/workflow`) · `state` TEXT (lifecycle, §4.2) · `params` TEXT(JSON) (the `ParamSchema`) · `workflow` TEXT(JSON) (`{steps[],edges[]}`) · `artifacts` TEXT(JSON) (`[{name,lang,body,sha256,checksum}]`) · `staleness` TEXT(JSON) (the full `StalenessContract`) · `validation` TEXT(JSON) (`{agreement_rate, n, invariants[]}`) · `provenance` TEXT(JSON) (`{minedFromCluster,connector,author,distill_model,approvedBy}`) · `savings` TEXT(JSON) (`{baseline_tokens_per_run, replay_tokens_per_run, hits, tokens_saved_total}`) · `requires_approval` INT/BOOL · `created_at`/`updated_at`. **`UNIQUE(scope, slug, version)`**; re‑distillation = `version+1`, prior versions retained → rollback. Index `(scope, state)`.

**`recall_context`** — the mutable memory blob (the self‑building property).

`automation_id` TEXT (FK→`recall_automations.id`, latest version) · `scope` · `checkpoint` INT (optimistic CAS token, bumped on each commit) · `variables` TEXT(JSON, mutable, overwrite) · `findings` TEXT(JSON, mutable) · `referenced` TEXT(JSON array, mutable) · `citations` TEXT(JSON, **append‑only**) · `audit_ledger` TEXT(JSON, **append‑only**) · `updated_at`. PK `(scope, automation_id)`.

**`recall_runs`** — run history / Progress timeline / approval queue backing.

`run_id` TEXT PK · `scope` · `automation_id` (or `slug`+`version`) · `mode` TEXT (`shadow`/`replay`/`approved`/`awaiting_approval`) · `params` TEXT(JSON) · `snapshot_id` (freshness proof at run time) · `served_by` · `requested_by` (user hash) · `blast_radius` TEXT · `status` TEXT (`awaiting`/`approved`/`rejected`/`ran`/`ok`/`error`) · `agreement` REAL (shadow runs) · `tokens_saved` INT · `result` TEXT(JSON) · `created_at`/`resolved_at`. Indexes `(scope, automation_id, created_at DESC)`, `(scope, mode, status)` (drives the approvals queue).

**`staleness_signals`** — lightweight index of each automation's **probe** signals, so the background sweep pulls just those (the full contract lives as JSON on `recall_automations`).

`id` TEXT PK · `scope` · `automation_id` · `type` TEXT (`code/config/data/time/external`) · `strategy` TEXT (`fingerprint/probe`) · `baseline` TEXT(JSON) (the captured fingerprint / probe target) · `last_verdict` TEXT (`fresh/stale/unknown`) · `verdict_ttl_sec` INT · `last_checked_at` · `detail` TEXT. Index `(scope, strategy)` (sweep selects `strategy='probe'`).

### 4.2 Automation lifecycle states

`candidate → validating → trusted → stale`, plus `rejected`/`not_promoted`/`revoked`. Enforced in `internal/recall/lifecycle.go` (Atlas enforces transitions + runs deterministic checks + persists; LLM‑touching transitions happen on the connector):

- **candidate → validating:** connector POSTs the distilled artifact; Atlas validates schema + artifact checksums; **rejects any side‑effectful script step lacking `provenance.approvedBy`.**
- **validating → trusted:** shadow mode (`recall_runs.mode="shadow"`). For `code_query`, Atlas runs the steps itself (pure engine ops, 0 LLM) and compares to the connector's LLM answer (set‑equality for structured; cosine over answer embeddings for prose). Promote at `agreement_rate ≥ 0.95` over `N ≥ 5` **and** all invariants pass. For `script`, run the connector/Pulse sandbox and check invariants only.
- **trusted → stale:** staleness loop sets `drifted=true` → **immediately de‑advertised** from `tools/list`; kept for cheap re‑distillation.
- **any → revoked:** manual (RBAC) or auto on invariant regression. Rows retained for audit.

### 4.3 StorageDriver methods to add

Extend the `StorageDriver` interface in `internal/store/store.go`; implement in both `internal/store/sqlite.go` and `internal/store/postgres.go`, mirroring the existing `SaveEmbeddings`/`NearestSymbols` pattern (chunked IN‑lists, JSON encode/decode helpers, scope‑scoped WHERE):

```go
// events
RecordRecallEvent(ctx, ev *graph.RecallEvent) error            // idempotent on event_id
RecentRecallEvents(ctx, scope string, limit int) ([]graph.RecallEvent, error)
CountByQueryHash(ctx, scope, queryHash string) (int, error)
// candidates
UpsertRecallCandidate(ctx, c *graph.RecallCandidate) error
ListRecallCandidates(ctx, scope, status string) ([]graph.RecallCandidate, error)
// automations
SaveAutomation(ctx, a *graph.Automation) error                 // version+1 on re-distill
GetAutomation(ctx, scope, slug string, version int) (*graph.Automation, error) // version=0 → latest TRUSTED
ListAutomations(ctx, scope, state string) ([]graph.Automation, error)          // state="" → all
SetAutomationState(ctx, scope, slug string, version int, state string) error
// context (mutable memory, CAS)
LoadContext(ctx, scope, automationID string) (*graph.RecallContext, error)
CommitContext(ctx, c *graph.RecallContext, expectedCheckpoint int) error       // optimistic CAS
// runs
SaveRun(ctx, r *graph.RecallRun) error
ListRuns(ctx, scope, automationID string, limit int) ([]graph.RecallRun, error)
ListAwaitingApprovals(ctx, scope string) ([]graph.RecallRun, error)
// staleness sweep index
ListProbeSignals(ctx, scope string) ([]graph.StalenessSignal, error)
UpsertStalenessSignal(ctx, s *graph.StalenessSignal) error
```

Add a capability flag to `store.Capabilities`:

```go
type Capabilities struct {
	DurableQueue    bool
	CrossScope      bool
	ConcurrentWrite bool
	PushReindex     bool
	RecallRegistry  bool // NEW: ingest+mining+registry (hosted Postgres tier; gated like PushReindex)
}
```

SQLite reports `RecallRegistry: false` for ingest/mining gating **but can still record its own events and replay manually‑registered automations** (so local dev works). Postgres (hosted) reports `true`.

### 4.4 Graph value types (`internal/graph/model.go`)

Add structs mirroring the JSON shapes in §3.3 and the table columns above: `RecallEvent`, `RecallCandidate`, `AutomatabilityVerdict`, `Automation` (with `Workflow{Steps[],Edges[]}`, `Artifact{Name,Lang,Body,SHA256,Checksum}`, `ParamField`, `StalenessContract{Signals[],Drifted}`, `Signal{Type,Strategy,Baseline,Fresh,Detail,VerdictTTLSec}`, `Provenance`, `SavingsAccount`), `RecallContext`, `RecallRun`, `StalenessSignal`. Reuse `graph.JSONBMap` for the kv blobs (`variables`/`findings`). Keep them persistence‑agnostic (no `db:` tags) like the existing model.

### 4.5 Reuse map (anchored — do not reinvent)

| Need | Reuse | Location |
|---|---|---|
| Embed prompts (no LLM) | `embed.NewProvider()` → `Provider.Embed`; selects HTTP when `ATLAS_EMBED_URL` set, else offline Hashing | `internal/embed/embed.go`, `internal/embed/http.go` |
| Cluster + answer‑equality cosine | `encodeVector`/`decodeVector`/`rankEmbeddings`/`dotProduct` (L2‑normalized; cosine == dot) | `internal/store/embeddings.go` |
| Context‑read + dynamic‑tool‑list cache | deep‑copy snapshot‑keyed LRU (`contextCache.get/put`) | `internal/engine/contextcache.go` |
| Staleness fingerprints | `resolveSnapshot`, `File.Hash`/`scanWorkTree`/`changedSet`, `SnapshotDiff`, `route_contracts` | `internal/engine/engine.go`, `internal/index/worktree.go` |
| Tenant isolation | `Config.Scope` / `WithScope(scope)` | `internal/engine/engine.go` |
| Dynamic tools + replay choke point | `tools/list` (currently `resp.Result = {"tools": s.tools}`) + `callTool` switch default | `internal/mcp/server.go` |
| Ingest/register/read endpoints | `routes()` on `s.mux` + `withAuth` bearer middleware | `internal/api/server.go` |
| Staleness sweep + miner loop | `startBackgroundWatch` + `Watcher.loop` debounced ticker | `internal/cli/watch.go`, `internal/watch/watch.go` |
| Feature/tier gating | `WithVectors`/`ATLAS_ENABLE_VECTORS`, `envTrue`/`envInt`, `store.Capabilities` | `internal/engine/engine.go`, `internal/store/store.go` |
| Schema/migrate pattern | `schemaSQLite` + idempotent `Migrate` (mind §4 sharp edge) | `internal/store/sqlite_schema.go`, `internal/store/sqlite.go`, `postgres_schema.go` |
| **Execution + LLM orchestration (connector side)** | Pulse `FlowOrchestrator`/`FlowGraph` (DAG), `flow_memory`, cron scheduler, warm‑pool `runner.js`, `aziron_llm_client` (per‑request model/provider + token usage) | `aziron-pulse/internal/service/*`, `docker/.../runner.js` |

### 4.6 How recall attaches to the engine

The `localEngine` (`internal/engine/engine.go`) already holds `store store.StorageDriver` and a `Config{Scope,…}`. Add a `recall *recall.Service` field, constructed in `New()` when `EnableRecall` (a new `Option`/env gate) is set, wired with the same `store`, `cfg.Scope`, and an `embed.Provider`. Expose the recall ops on the `Engine` interface only as far as the MCP/API layers need (e.g. `ServeAutomation`, `ListRecallAutomations`, `RecordRecallEvent`, …) — keep the deterministic core methods untouched. When `recall == nil` (flag off), `tools/list` and `callTool` behave **byte‑identically to today.**

---

## 5. API surface (every new endpoint)

All under `/api/v1/recall/`, registered in `internal/api/server.go routes()` on `s.mux`, behind `withAuth` (bearer required iff `ATLAS_API_TOKEN`/`cfg.Token` set). All responses use the existing `{ "data": … }` envelope via `writeResult`; errors are RFC 9457 problem+json via `writeProblem`. All are **scope‑isolated** by `Config.Scope`.

| Method | Path | Auth | Request body | Response `data` |
|---|---|---|---|---|
| `POST` | `/api/v1/recall/events` | bearer | `OutcomeEvent` (see below) | `{ "event_id": "...", "deduped": false }` (idempotent on `event_id`) |
| `GET` | `/api/v1/recall/events?recent=N` | bearer | — | `RecallEvent[]` (newest first; default N=50) |
| `GET` | `/api/v1/recall/metrics` | bearer | — | `kpis` object (§3.3) |
| `GET` | `/api/v1/recall/metrics/series?window=30d` | bearer | — | `{ days[], tokensSaved[], hitRate[] }` |
| `GET` | `/api/v1/recall/served-by` | bearer | — | `{ exactCache, semanticCache, automationReplay, liveLlm }` |
| `GET` | `/api/v1/recall/candidates?status=open` | bearer | — | `cluster[]` (§3.3 `clusters` shape; includes verdict) |
| `POST` | `/api/v1/recall/automations` | bearer | distilled `Automation` artifact | `{ "slug", "version", "state": "validating" }` |
| `GET` | `/api/v1/recall/automations?state=<opt>` | bearer | — | `automation[]` (latest version per slug) |
| `GET` | `/api/v1/recall/automations/{slug}` | bearer | — | full `automation` (workflow/artifact/context/staleness) |
| `POST` | `/api/v1/recall/automations/{slug}/runs` | bearer | `{ mode, params, snapshot_id, outcome }` | `RecallRun` (records shadow/replay/approved outcome) |
| `GET` | `/api/v1/recall/approvals` | bearer | — | `approval[]` (the `awaiting`/resolved queue) |
| `POST` | `/api/v1/recall/approvals/{id}/approve` | bearer | `{ approver }` | updated `RecallRun` (status `approved`) |
| `POST` | `/api/v1/recall/approvals/{id}/reject` | bearer | `{ approver, reason }` | updated `RecallRun` (status `rejected`) |
| `POST` | `/api/v1/recall/approvals/{id}/run` | bearer | — | **verified execution descriptor** `{ artifact_body, checksum, params, run_id }` for the connector to execute (Atlas does NOT execute) |
| `GET` | `/dashboard` | open (or bearer if gated) | — | embedded merged dashboard HTML (§3.1) |

**`OutcomeEvent` request body** (`POST /recall/events`):

```jsonc
{
  "event_id": "evt_…",            // idempotency key (required)
  "scope": "org_…",                // optional; server may derive from auth/scope
  "user_id": "u_7af3",             // PRE-HASHED by the connector (never raw)
  "session_id": "sess_…",
  "connector": "claude",           // claude | codex | cowork
  "repo_id": "aziron-pulse",
  "snapshot_id": "0723d89",        // freshness anchor
  "prompt_text": "who calls FlowOrchestrator.dispatch?",
  "prompt_norm": "who calls {symbol}",   // normalized, literals masked
  "prompt_embedding": [/* optional float32[]; if present Atlas skips the embed round-trip */],
  "tool_trace": [ { "tool": "callers", "args": {…} }, … ],
  "outcome": { "status": "ok", "result_kind": "structured", "side_effects": [], "artifacts": [] },
  "cost": { "input": 9200, "output": 5000, "cache": 0, "tool_calls": 3, "wall_ms": 2210 }
}
```

> **Gating:** `POST /recall/events`, mining, and `POST /recall/automations` require `store.Capabilities().RecallRegistry` (hosted Postgres tier) **OR** an explicit local‑dev override env (`ATLAS_RECALL_LOCAL=1`). When disabled, return `501 not_implemented` with a clear hint. Read endpoints + replay work locally so the dashboard renders against a local engine.

---

## 6. Phased task list (P0 → P3)

Each phase is independently shippable; the deterministic core is untouched when the recall flag is off. Every phase ends with concrete acceptance criteria and how to verify.

### Phase P0 — Observe + Cache

**Goal:** ingest the event stream and ship the exact + semantic *answer cache* (Tiers 0–1) in **shadow mode** (log would‑be hits, always serve live), then flip to serve‑cached per scope with a kill‑switch.

Tasks:
- [ ] `internal/graph/model.go`: add `RecallEvent` (+ minimal cache types).
- [ ] `internal/store/store.go`: add `RecordRecallEvent`, `RecentRecallEvents`, `CountByQueryHash`; add `Capabilities.RecallRegistry`.
- [ ] `internal/store/sqlite_schema.go` + `postgres_schema.go`: add `recall_events` DDL (additive; see §4 sharp edge — append to `schemaSQLite`, do **not** add to `dropAllSQLite`).
- [ ] `internal/store/sqlite.go` + `postgres.go`: implement the three methods (reuse `encodeVector` for `query_vec`).
- [ ] `internal/recall/events.go` + `internal/recall/cache.go` + `internal/recall/config.go`: ingest + exact‑hash cache + cosine semantic cache (reuse `rankEmbeddings`/`dotProduct`, `embed.NewProvider()`); snapshot + dependency‑fingerprint freshness gate; `ATLAS_RECALL_SIM_THRESHOLD` (default 0.92), `ATLAS_RECALL_SHADOW` (default on), `ATLAS_RECALL_LOCAL` knobs via `envTrue`/`envInt`.
- [ ] `internal/api/server.go`: add `POST /recall/events`, `GET /recall/events`, behind `withAuth`; gate on `RecallRegistry || ATLAS_RECALL_LOCAL`.
- [ ] `internal/engine/engine.go`: add `EnableRecall` option + `recall *recall.Service` field, constructed in `New()`; keep `recall==nil` behavior byte‑identical.

**Acceptance criteria:**
- `go build ./...` and `go test ./internal/store/... ./internal/recall/...` pass.
- `POST /recall/events` is idempotent (second POST of the same `event_id` returns `deduped:true`, no duplicate row).
- In shadow mode, a repeated identical prompt **logs a would‑be exact hit but still serves live** (no behavior change).
- Store round‑trip tests on **both** drivers (mirror `internal/store/store_embeddings_test.go`).

**Verify:**
```bash
go build ./... && go test ./internal/store/... ./internal/recall/...
ATLAS_RECALL_LOCAL=1 ATLAS_API_TOKEN=dev go run ./cmd/atlas serve --addr :8083 &
curl -s -XPOST localhost:8083/api/v1/recall/events -H 'Authorization: Bearer dev' \
  -d '{"event_id":"e1","connector":"claude","prompt_norm":"who calls {symbol}","prompt_text":"who calls X?","cost":{"input":100,"output":50}}'
curl -s localhost:8083/api/v1/recall/events?recent=10 -H 'Authorization: Bearer dev' | jq .
```

### Phase P1 — Mine + Decide

**Goal:** background miner (clustering + HotScore) → `recall_candidates`; record connector‑judged `AutomatabilityVerdict`; the hot‑prompt leaderboard reads live.

Tasks:
- [ ] `internal/graph/model.go`: `RecallCandidate`, `AutomatabilityVerdict`.
- [ ] `internal/store/*`: `UpsertRecallCandidate`, `ListRecallCandidates` + `recall_candidates` DDL.
- [ ] `internal/recall/mine.go`: exact bucket on `query_hash`, then greedy single‑pass cosine merge of bucket centroids (`rankEmbeddings`, merge when cosine ≥ τ); HotScore = `log1p(frequency) × distinct_user_count^α × avg_tokens × recency_decay × success_ratio` (α≈1.5, all weights `envInt`/`envTrue`‑tunable). Clusters above `ATLAS_RECALL_HOT_THRESHOLD` → `recall_candidates(status=open)`.
- [ ] `internal/cli/serve.go`: add a `startBackgroundMiner` goroutine (sibling of `startBackgroundWatch`, ticker‑driven, per scope; opt‑in via flag/env).
- [ ] `internal/api/server.go`: `GET /recall/candidates`; accept the connector's verdict (either a field on the candidate or a small `POST` to attach it).

**Acceptance criteria:**
- Unit test asserts **a 1‑user cluster ranks below a 20‑user cluster at equal frequency** (the "many people" rule falls out of α).
- Cosine merge groups "deploy staging"/"deploy prod" into one candidate (param‑masked `prompt_norm`).
- `GET /recall/candidates` returns the `clusters` shape the dashboard expects.

**Verify:** `go test ./internal/recall/...` (mining + HotScore tests); seed events via the P0 endpoint, run the miner once, `curl …/recall/candidates | jq`.

### Phase P2 — Distill + Serve (code automations)

**Goal:** the keystone — register distilled `code_query` automations, shadow‑validate to `trusted`, advertise as dynamic `recall__<slug>` MCP tools, replay **in‑process (0 LLM)**, account savings; fingerprint staleness (code/config/time) + lazy‑on‑use gate + event‑driven invalidation.

Tasks:
- [ ] `internal/graph/model.go`: `Automation`, `RecallContext`, `RecallRun`, `StalenessContract`/`Signal`.
- [ ] `internal/store/*`: `SaveAutomation`, `GetAutomation` (version=0→latest‑trusted), `ListAutomations`, `SetAutomationState`, `LoadContext`, `CommitContext` (CAS), `SaveRun`, `ListRuns` + `recall_automations`/`recall_context`/`recall_runs` DDL.
- [ ] `internal/recall/automation.go`, `lifecycle.go`, `context.go`: register (schema + checksum validation; reject side‑effectful script without `approvedBy`), the state machine, mutable‑context CAS.
- [ ] `internal/recall/staleness.go` + `evaluator.go`: the `SignalEvaluator` interface + `Registry` + fingerprint evaluators (code/config/time); `Capture` at distill time, `Evaluate` at serve time (same code path).
- [ ] `internal/mcp/server.go`: **dynamic `tools/list`** — compose `s.tools` + trusted, non‑drifted automations as `recall__<slug>` tools (`InputSchema` from `ParamSchema`, `Description` = gating prose), cached keyed on registry `updated_at` (reuse the deep‑copy LRU). **`callTool` default branch** — `recall__` prefix → `serveAutomation(slug,args)`: resolve latest trusted; if drifted/absent → **degrade** result `{"status":"stale"}` `isError:false`; freshness‑gate; bind params; execute by `Kind` (code_query in‑process); write mutable context; increment savings; record `recall_runs`. When `recall==nil`, behavior is byte‑identical to today.
- [ ] `internal/engine/engine.go`: hook the post‑`Index()` point so contracts whose dep files are in `changedSet()` are marked for re‑eval.
- [ ] `internal/api/server.go`: `POST /recall/automations`, `GET /recall/automations`, `GET /recall/automations/{slug}`, `POST /recall/automations/{slug}/runs`, `GET /recall/metrics`, `GET /recall/metrics/series`, `GET /recall/served-by`.
- [ ] **Embed the merged dashboard** (`internal/api/dashboard.go` + `GET /dashboard`, §3.1).

**Acceptance criteria:**
- Register a `code_query` automation → after N≥5 shadow runs at agreement ≥0.95 it becomes `trusted` → `tools/list` shows `recall__<slug>`.
- `tools/call recall__<slug>` replays it with **no model call** (assert no `embed`/LLM path hit; replay tokens only).
- Edit a dependency file → the tool is **de‑advertised / degrades** (`{"status":"stale"}`); re‑register restores it.
- `version+1` rollback: registering a new version keeps the prior; `GetAutomation(…,0)` resolves the latest **trusted** one.
- `GET /dashboard` returns the HTML; headless load shows **zero console errors** and the Overview + Automations views render.

**Verify:**
```bash
go build ./... && go test ./internal/mcp/... ./internal/recall/... ./internal/store/...
ATLAS_RECALL_LOCAL=1 ATLAS_API_TOKEN=dev ATLAS_EMBED_URL=… go run ./cmd/atlas serve --mcp --addr :8083 &
# register + list tools + call:
curl -s -XPOST localhost:8083/api/v1/recall/automations -H 'Authorization: Bearer dev' -d @automation.json
curl -s -XPOST localhost:8083/mcp -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | jq '.result.tools[].name' | grep recall__
curl -s -XPOST localhost:8083/mcp -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"recall__callers-of-symbol","arguments":{"symbol":"X"}}}' | jq .
# dashboard:
curl -s localhost:8083/dashboard | head -c 200      # expect <!doctype html>
```
Headless dashboard check: load the file with Playwright + a faithful Lucide stub (copy all attrs), assert `console.error` count == 0 and the Overview KPI strip + Automations three‑pane are present in the DOM (per the prototype‑runtime‑verification convention).

### Phase P3 — General automations + probes + approval

**Goal:** `script`/`workflow` automations executed by the **connector/Pulse** (never Atlas); reversibility/approval gate (the deploy case); probe staleness signals (data/external) + periodic sweep; mutable cross‑run learning.

Tasks:
- [ ] `internal/recall/evaluator.go`: probe evaluators (data: schema‑hash + row‑count/freshness threshold; external: health/version endpoint via the `embed/http.go` client shape) — **timeout/non‑2xx ⇒ fail‑closed stale**, with verdict TTL.
- [ ] `internal/store/*`: `ListProbeSignals`, `UpsertStalenessSignal`, `ListAwaitingApprovals` + `staleness_signals` DDL.
- [ ] `internal/cli/serve.go`: `startBackgroundStalenessSweep` ticker (modeled on `watch.go`) — selects `strategy='probe'`, re‑evaluates, updates `last_verdict`/`last_checked_at`, marks contracts stale.
- [ ] `internal/recall/serve.go`: for any `Reversible=false` step / side‑effectful script → serve `status:"awaiting_approval"` + verified descriptor + `run_id` (Atlas stays out of execution).
- [ ] `internal/api/server.go`: `GET /recall/approvals`, `POST …/{id}/approve|reject|run` (the `run` action returns the verified execution descriptor; the connector executes via the Pulse warm‑pool runner and POSTs the outcome back to `…/{slug}/runs`).

**Acceptance criteria:**
- A `script` automation (e.g. `deploy-service-to-env`) returns **`awaiting_approval`** and is **never executed by Atlas** (assert no shell/exec call in Atlas).
- A probe that times out marks the contract **stale** (fail‑closed); the automation de‑advertises.
- Approve → `run` returns the descriptor `{artifact_body, checksum, params, run_id}`; reject → status `rejected`; both reflected in `GET /recall/approvals` and the dashboard Approvals view.
- The sweep updates probe verdicts on its interval; a TTL‑expired probe re‑checks on next serve.

**Verify:** `go test ./internal/recall/...` (probe fail‑closed, AND semantics, approval state machine); E2E with `atlas serve` + curl the approvals flow; confirm the `deploy` automation's `run` action returns a descriptor and Atlas logs no execution.

---

## 7. Correctness & risks

- **Fail‑closed staleness (non‑negotiable).** Any stale/errored/unknown signal ⇒ the whole `StalenessContract` is stale. A probe timeout or non‑2xx ⇒ stale, not "assume fresh". The worst case is a safe, self‑correcting false‑stale (one unnecessary recompute). Distill‑time and serve‑time MUST run the **same** `Capture`/`Evaluate` code so baseline and observation cannot diverge.
- **Correctness over hit‑rate.** A miss costs one LLM call; a wrong cached/automated answer poisons the whole org. Keep the similarity threshold **high** (≥0.92) and add a **discriminator guard** against negation/direction false matches (e.g. "who calls X" vs "who does X call", "deploy" vs "rollback") — never let a near‑match flip semantics. Ship the cache in **shadow mode first** and assert **0 stale‑served** in the shadow logs before flipping to serve‑cached per scope; keep a per‑scope kill‑switch.
- **Validation‑gating LLM‑authored scripts.** The connector's LLM may *generate* an automation, but Atlas only trusts it after shadow‑replay agreement (≥0.95, N≥5) **and** invariants pass. Replay itself never invents — it executes recorded/declared steps. Reject side‑effectful script steps lacking `provenance.approvedBy` at registration.
- **Privacy / scope.** Prompts are sensitive: `user_id` is **pre‑hashed** by the connector (Atlas never stores a raw id), every read/write is isolated by `Config.Scope`, and answers are shared only within a scope where all users could already run the underlying tools. Add retention/GC on `recall_events`.
- **Approval gate for irreversible.** Any `Reversible=false` step / side‑effectful script is served as `awaiting_approval` + descriptor + `run_id`; the connector surfaces the Cowork approval UI; on approval it executes (via Pulse warm‑pool) and POSTs the outcome back. **Atlas stays entirely out of the execution path.**
- **Schema‑drop sharp edge (§4).** SQLite `Migrate` runs `dropAllSQLite` on a `user_version` mismatch. Recall tables are persistent automation state, not rebuildable from a reindex — keep them out of `dropAllSQLite` and create them idempotently via `schemaSQLite`. Evolving a recall table's shape requires a real additive migration, never the drop path.
- **Flag‑off = byte‑identical.** When `recall==nil` / the flag is off, `tools/list`, `callTool`, and every existing route behave exactly as today. This is an acceptance criterion, not an aspiration — assert it in the MCP tests.

---

## 8. Concrete first‑PR scope

Keep PR #1 small, end‑to‑end‑provable, and behind a flag. Deliver **P0 ingest + the embedded dashboard skeleton** — the spine everything else hangs off:

**In scope for PR #1:**
1. `internal/graph/model.go`: `RecallEvent` struct.
2. `internal/store/store.go`: `RecordRecallEvent`, `RecentRecallEvents`, `CountByQueryHash`; `Capabilities.RecallRegistry` field.
3. `recall_events` DDL appended to `internal/store/sqlite_schema.go` and `internal/store/postgres_schema.go` (additive; **not** added to `dropAllSQLite`).
4. Implementations in `internal/store/sqlite.go` + `postgres.go`, with a round‑trip test mirroring `store_embeddings_test.go` on both drivers.
5. `internal/recall/{events.go,config.go}`: ingest + idempotency; env knobs via `envTrue`/`envInt`; `ATLAS_RECALL_LOCAL` gate.
6. `internal/engine/engine.go`: `EnableRecall` option + `recall` field, `recall==nil` ⇒ unchanged.
7. `internal/api/server.go`: `POST /recall/events`, `GET /recall/events` behind `withAuth`, gated.
8. `internal/api/dashboard.go` + `internal/api/assets/atlas-recall-dashboard.html` (the merged file from §3) + `GET /dashboard` route.
9. Build the merged `prototypes/atlas-recall-ui/atlas-recall-dashboard.html` (fusing A's shell + C's lifecycle/three‑pane/approvals per §3.2), booting from inlined `DATA` (§3.3), strict source ordering (§3.2 TDZ rule). Copy it to the embed path.

**Acceptance for PR #1:**
- `go build ./...` + `go test ./internal/store/... ./internal/recall/...` pass.
- `atlas serve` → `GET /dashboard` returns the HTML; headless load = **zero console errors**, Overview + Automations + Approvals views render from mock `DATA`.
- `POST /recall/events` idempotent; `GET /recall/events?recent=N` returns them newest‑first.
- With the flag off, `tools/list` / `callTool` / existing routes are unchanged (existing tests stay green).

**Explicitly out of scope for PR #1** (follow‑on PRs, one per phase): mining + HotScore (P1), distillation/registry/dynamic MCP tools/replay/fingerprint staleness (P2), probes/approval/script execution descriptors (P3). Wiring the dashboard's `fetch` calls to the real endpoints lands incrementally as each section's endpoint ships (§3.4).

---

*End of runbook. The merged dashboard prototype (`prototypes/atlas-recall-ui/atlas-recall-dashboard.html`) and this document are the two artifacts a downstream agent needs to build Atlas Recall without further context.*
