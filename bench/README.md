# Atlas vs graphify benchmark

A reproducible, per-language comparison of Atlas against
[graphify](https://github.com/safishamsi/graphify). For the core matrix
languages it builds **both** code knowledge graphs on the same real
single-language repo and measures build, call-graph coverage, edge precision, and
query token cost — both tools run **offline, no LLM**.

## Run it

```sh
pip install graphifyy          # provides the `graphify` CLI
CGO_ENABLED=1 go build -o /tmp/atlas ./cmd/atlas

python3 bench/graphify_vs_atlas.py \
  --atlas /tmp/atlas \
  --graphify "$(command -v graphify)" \
  --workdir /tmp/langbench \
  --out bench/REPORT.md
# optional: --langs go,java,cpp
```

It clones the pinned per-language repos (shallow) into `--workdir`, runs both
tools, and writes:

- `bench/REPORT.md` — the human report (summary table + findings + per-language detail)
- `bench/REPORT.json` — the raw metrics
- `bench/logs/<lang>.log` — the raw `graphify update` / `atlas index` / `explain` output

## What the metrics mean

The two tools model edges differently, so read them with care (the report
repeats this):

- **Atlas** emits *every* call expression as an edge (including calls out to
  stdlib/3rd-party) and resolves to in-graph targets on demand; *internal calls*
  is the resolvable subset — the fair coverage axis vs graphify.
- **graphify** keeps only node-to-node call links, each flagged `EXTRACTED`
  (AST-grounded) or `INFERRED` (guessed).
- **method-receiver%** (Atlas) is OO-method receiver resolution — meaningful only
  for method calls (≈0 for procedural C by design), and where Atlas's type
  grounding (Go `go/types`, Java declared types) shows. It is **not** comparable
  to graphify's `EXTRACTED%`.

Numbers in the committed `REPORT.md` are a one-machine snapshot; timings vary, but
the graphs are deterministic.

## Code-intelligence matrix benchmark

`codeintel_matrix.py` is the broader benchmark harness for the agreed baseline
matrix:

- Go: Atlas vs graphify vs `scip-go` vs `gopls`
- Python: Atlas vs graphify vs `scip-python`/Pyright
- JS/TS: Atlas vs graphify vs `scip-typescript`/TS server
- Java: Atlas vs graphify vs `scip-java`/JDTLS
- C/C++: Atlas vs graphify vs `clangd`, with CodeQL/Kythe optional later

The live implementation currently covers the Go, Python, JavaScript,
TypeScript, Java, C, and C++ slices end to end.

```sh
CGO_ENABLED=1 go build -o /tmp/atlas ./cmd/atlas
go install github.com/scip-code/scip-go/cmd/scip-go@latest
npm install -g @sourcegraph/scip-python pyright
npm install -g @sourcegraph/scip-typescript typescript
# scip-java is launched through the pinned Coursier wrapper in bench/tools by
# default. Override SCIP_JAVA_BIN or --scip-java to use a local release binary.
#   bench/tools/scip-java-coursier --version
# JDTLS is a real LSP benchmark adapter; eclipse.jdt.ls currently requires Java 21+.
# clangd is used as the C/C++ LSP baseline.

# graphify installed by uv is often not on PATH; this path is auto-detected too.
uv tool install --upgrade graphifyy

python3 bench/codeintel_matrix.py \
  --atlas /tmp/atlas \
  --graphify "$HOME/.local/share/uv/tools/graphifyy/bin/graphify" \
  --workdir /tmp/atlas-codeintel-matrix \
  --out bench/MATRIX_REPORT.md \
  --langs go,python,javascript,typescript,java,c,cpp
```

The Go, Python, JavaScript, TypeScript, and Java SCIP metrics are parsed by
`bench/scipstats`, a small benchmark-owned Go helper. Its dependencies stay in a
nested module so SCIP protobuf dependencies do not enter the Atlas production
module.

### Build-speed fairness (cold-vs-cold and delta-vs-delta)

Build-speed is reported on two clearly-labeled axes, and a delta-vs-full pairing
is never used as the headline:

- **Cold-vs-cold full index (headline):** Atlas's *cold* full index vs the *cold*
  full build of every baseline that also builds from scratch — graphify FULL
  extract, `scip-go`/`scip-python`/`scip-typescript`/`scip-java`, and `gopls`
  workspace type-check. A ratio < 1.0x means Atlas is slower cold and the report
  says so. On a warm Go build cache, Atlas's cold full index of logrus is ~0.55s
  vs graphify FULL ~0.84s / scip-go ~0.62s / gopls ~0.38s. On a *first-ever*
  cold cache the Go `go/types` load dominates (the `cold_timings_ms.go_types`
  phase), pushing Atlas's first cold index materially higher; both are real and
  preserved in `cold_seconds` / `cold_timings_ms`.
- **Delta-vs-delta no-change reindex:** Atlas's no-change reindex vs graphify's
  incremental re-`update` (its `graphify-out/` sidecar is kept between runs).
  Both tools re-run against an existing snapshot here, so the ratio is fair. This
  is the only place a delta number appears as a ratio.

Raw seconds for every build are kept in the JSON: Atlas `full_seconds` /
`delta_seconds` (+ `cold_timings_ms`), graphify `full_seconds` / `delta_seconds`.

### Warm query latency (persistent server)

The matrix also starts `atlas serve` against the indexed DB, warms it, and times
warm HTTP queries (`GET /healthz`, `GET /api/v1/symbols/<name>/explain`). This is
the legitimate path to higher latency ratios because the warm server skips the
per-call Go process-start floor that gates the cold CLI. Warm numbers live in
their own report section and in `atlas_warm_serve` in the JSON (raw per-call
samples preserved). graphify has no warm/server mode, so warm Atlas is never
divided by a graphify time — the only Atlas-vs-graphify latency ratio stays
cold-vs-cold CLI. The server is stopped cleanly (SIGTERM, then kill as a last
resort) after measurement.

See `bench/HONEST_FINDINGS.md` for the measured saturation analysis (where a 25x
target is or is not physically reachable, with numbers).

## Additional graphify-language benchmarks

Per-language native-parser evidence is consolidated in
`bench/RECALL_FINDINGS.md`, with matrix-level summaries in
`bench/MATRIX_REPORT.md` and `bench/MATRIX_REPORT.json`. The old per-language
live JSON artifacts and one-off helper scripts were removed after consolidation,
so committed evidence should be updated in those consolidated reports rather
than by adding fresh per-language raw files.

For languages where graphify exposes no equivalent query rows, keep the ceiling
documented in `bench/RECALL_FINDINGS.md` and `bench/SATURATION_REPORT.md` with
the exact counts and reason the ratio cannot be computed.
