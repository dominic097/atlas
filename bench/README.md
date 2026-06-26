# Atlas vs graphify benchmark

A reproducible, per-language comparison of Atlas against
[graphify](https://github.com/safishamsi/graphify). For each of Atlas's seven
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
