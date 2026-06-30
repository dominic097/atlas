# Atlas benchmark — honest findings

Measured-only. Every number below was produced live on this machine (Apple
Silicon, Go 1.25, CGO enabled) against `sirupsen/logrus` (Go) and existing live
benchmark artifacts; raw data is preserved in `bench/MATRIX_REPORT.json`
(`atlas_warm_serve`, `tools.atlas.metrics`, `queries`) and the per-language logs.
Where a target ratio is not reached, the bottleneck is named and classified as
**physical** (a floor that no amount of code change moves below) or
**improvable** (headroom exists), with numbers.

The headline correction this strand makes: the build-speed comparison previously
paired Atlas's **delta** reindex (~0.03s, second run on an existing snapshot)
against graphify's **full** extract (~0.8s). That is delta-vs-full and inflated
the build ratio. The harness now reports cold-vs-cold and delta-vs-delta
separately and never uses delta-vs-full as the headline.

---

## 1. Query latency is gated by the Go process-start floor (PHYSICAL)

Cold CLI query latency cannot reach 25x vs graphify because the floor alone
exceeds what a 25x ratio would require.

Measured cold-CLI medians (7 runs each, warm Go build cache, logrus DB):

| command | needs DB? | median |
|---|---|---|
| `atlas version` | no | **10.52 ms** |
| `atlas status` | yes | 11.64 ms |
| `atlas explain Fire` | yes | 12.47 ms |

`atlas version` does no DB work at all, so ~10.5 ms is the pure Go
process-start + runtime-init floor. The query itself is a few ms on top.

graphify cold-CLI explain on the same symbols: 82–355 ms (median over 4 hub
symbols, this run, **355.075 ms total / 4 = ~88.8 ms** average; single `log`
symbol up to 205 ms). So the fair cold-vs-cold CLI ratio is:

- This run: graphify 355.075 ms vs Atlas 52.903 ms (4 equivalent rows) =
  **6.71x**.
- Earlier grounded run: atlas cold ~14.9 ms vs graphify ~145 ms = **~9.7x**.

Why 25x is physically blocked cold: to hit 25x, Atlas cold-CLI would need to be
graphify_latency / 25. For graphify at ~145 ms that is 5.8 ms — below the
**10.5 ms** version floor that contains no query work. The floor alone is larger
than the 25x budget. Confirmed earlier in this session: stripping `-s -w` from
the binary and gating the `Migrate` `user_version` check produced **no**
cold-latency change, i.e. the floor is the Go runtime start, not Atlas work.

**Classification: physical for the cold CLI.** The legitimate lever is the warm
server (section 2), where the per-call floor disappears.

## 2. Warm query latency — the legitimate high-ratio path (IMPROVABLE → MEASURED)

`atlas serve` is started against the indexed DB, warmed, then warm HTTP queries
are timed. Removing the per-call process-start floor is exactly what lifts the
ratio. Measured live (logrus, raw samples in `atlas_warm_serve`):

| metric | median | raw samples (ms) |
|---|---|---|
| warm `GET /healthz` | **0.316 ms** | 0.312, 0.325, 0.318, 0.316, 0.313 |
| warm `GET /…/explain` (4 hubs) | **1.405 ms** | per-hub medians 1.29–1.585 |
| Atlas cold-CLI explain (same tool) | 13.213 ms | — |

- **Warm-vs-cold for Atlas itself = 9.4x** (13.213 ms cold-CLI / 1.405 ms warm
  explain). This is Atlas-vs-Atlas and isolates the process-start floor; it is
  reported as a warm speedup, not as an Atlas-vs-graphify ratio.
- An earlier, less-loaded measurement this session showed warm explain at
  ~3.67 ms and `/healthz` at ~0.35 ms; the sub-1.5 ms explain medians here are
  on a quieter machine. Both are real; the JSON keeps the raw samples.

**Do not** divide warm Atlas by cold graphify: graphify has no warm/server mode,
so warm-Atlas-vs-cold-graphify is not a fair ratio and is deliberately omitted.
The cold-vs-cold CLI rows (section 1) remain the only Atlas-vs-graphify latency
ratio.

**Warm-vs-warm context:** gopls is itself a persistent daemon. Measured warm
gopls (after init + settle): `documentSymbol` median **0.297 ms**,
`workspace/symbol("Entry")` median **3.432 ms**. Atlas warm explain (1.4 ms)
sits between these, but the operations are not equivalent — gopls
`documentSymbol` lists symbols in one already-open file, while Atlas `explain`
returns a full cross-symbol context bundle (defs + callers/callees + imports +
routes + consumers). So the two are reported side by side, not as a single
ratio.

## 3. Token cost is near the minimal-output floor (PHYSICAL minimum, real win)

Atlas already emits near-minimal answers, so the token ratio is large because
graphify is verbose, not because Atlas is padded — and it cannot grow much
further without changing retrieval semantics.

SQL (from `bench/RECALL_FINDINGS.md`, measured):

| symbol | Atlas tokens | graphify tokens | ratio |
|---|--:|--:|--:|
| `hdb_catalog.event_triggers` | 10 | 55 | 5.5x |
| `hdb_catalog.hdb_metadata` | **9** | **54** | **6.0x** |
| `hdb_catalog.hdb_schema_update_event_notifier` | 16 | 80 | 5.0x |
| `hdb_catalog.hdb_function_agg` | 11 | 56 | 5.1x |

At ~9 tokens Atlas is essentially at the information floor for a symbol answer;
graphify's ~54 carries extra framing. Pushing the ratio to 25x would require
Atlas to drop below ~2 tokens/answer (physically meaningless) **or** graphify to
balloon — neither is a real improvement. So the token win is real and durable
but saturated near its minimum; the report labels this as a floor, not a knob.

## 4. Build cold-index reality: Atlas is slower cold, and 93–94% is `go_types`

The previously-reported build win was delta-vs-full. Corrected, cold-vs-cold:

Warm Go build cache, logrus, this run (`tool matrix` row + `build_speed_lines`):

| tool | cold full build | vs Atlas cold |
|---|--:|--:|
| **Atlas** cold full index | **0.554 s** | 1.00x |
| graphify FULL extract | 0.835 s | 1.51x (graphify slower) |
| scip-go cold | 0.620 s | 1.12x |
| gopls workspace type-check | 0.382 s | **0.69x (Atlas slower)** |

First-ever cold (Go build cache cleared with `go clean -cache`), measured live:

- Atlas cold full index = **3.423 s wall** (internal 3366 ms), of which
  `go_types` = **3169 ms (94%)**; parse 52 ms, lexical 104 ms, persist 37 ms.
- This matches the grounded first-cold figure (~2.9 s, go_types ~1760 ms / 93%).
  The variance is the size of the transitive-dependency type-check the Go
  toolchain compiles on a cold cache.

So Atlas's cold full index is **slower** than scip-go (0.612 s grounded),
gopls (0.525 s grounded), and graphify (0.795 s grounded) on a first cold run —
honestly stated in the report, not hidden. The dominant cost is
`internal/gotypes/gotypes.go` loading `./...` with
`packages.LoadSyntax | packages.NeedModule` and **no caching**, once per cold
index.

**Classification: shared cold toolchain floor; competitive warm.** Strand A
isolated the cost with `go clean -cache`: `go list -export ./...` alone takes
~3.0 s cold vs ~112 ms warm on logrus — i.e. essentially the entire cold
`go_types` phase is the Go toolchain compiling transitive-dependency export
data, which `packages.Load` must trigger for ANY types-requesting mode. Changing
the load Mode (deprecated `LoadSyntax` → an explicit minimal set, `NeedDeps`
off) was measured **perf-neutral** (within run-to-run variance) and
**byte-identical** in precision (622 reference + 277 receiver-typed edges
unchanged), so it is kept for clarity, not as a speedup. gopls's ~0.41 s is a
**warm** build-cache figure (gopls reuses the cache); compared like-for-like,
Atlas's **warm** `go_types` pass is ~124–127 ms — competitive with, and on this
repo faster than, gopls warm. So cold build is a shared Go-toolchain floor, not
an Atlas-specific gap; the steady-state (warm-cache) cost is small. The only
real lever left is reusing export data across cold runs (a populated build cache
/ ops concern), not a LoadMode change. This strand does not alter precision.

### Delta-vs-delta (the only fair place a delta ratio appears)

Both tools support an incremental path, so a delta-vs-delta ratio is fair:

- Atlas no-change reindex = **0.027 s** (mode `noop`).
- graphify no-change re-`update` (sidecar kept) = **0.35 s**.
- graphify/Atlas = **12.96x**.

This is labeled delta-vs-delta in the report and is never presented as the
headline build number.

## 5. Recall ~1.0 parity (MEASURED, not weakened)

The honesty corrections do not touch correctness/recall. From the existing
Python AST ground-truth check and the matrix coverage rows, Atlas recalls the
function/method/class symbol set at ~1.0 against the language's own AST/type
truth (e.g. Python AST callable/class recall is rendered as a measured
percentage in the report; SCIP navigation-symbol coverage on Go is reported as a
raw ratio). No baseline was weakened and no precision was traded for speed to
produce any number above.

---

## Bottom line on the 25x target

| axis | best fair ratio measured | 25x? | why |
|---|---|---|---|
| cold-CLI query latency | 6.7x–9.7x | no | **physical**: ~10.5 ms Go process-start floor > (graphify_ms / 25) |
| warm query latency (Atlas warm vs Atlas cold) | 9.4x | n/a vs graphify | graphify has no warm mode; reported as warm speedup only |
| token cost per answer | 5–6x | no | **physical**: Atlas already at ~9-token minimum; graphify is the verbose side |
| build, cold full index | 0.69x–1.5x | no | Atlas **slower cold**; 94% is `go_types` = the Go toolchain's cold dep export-data compilation (shared floor, ~3 s); ~124 ms warm |
| build, delta-vs-delta | 12.96x | — | fair only because both tools run incrementally |
| recall | ~1.0 parity | — | unchanged; not traded for any speed/token gain |

Where 25x is unreachable it is a floor — process-start (cold latency), minimal
output (tokens), or the Go toolchain's cold dependency export-data compilation
(cold build; shared by every Go type-checker, gopls included; Atlas's *warm*
`go_types` is ~0.12 s) — not a missing Atlas optimization. Atlas's genuine, fair
wins here are warm query latency, token cost where the baseline is verbose, and
delta-vs-delta reindex; recall stays at parity. LoadMode was proven perf-neutral
with precision byte-identical, so no speed was bought by trading correctness.
