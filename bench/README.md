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
# scip-java can be a downloaded release binary or a coursier launch command.
# The harness accepts commands with args, e.g.
#   --scip-java "./coursier launch com.sourcegraph:scip-java_2.13:0.12.3 --"
# JDTLS is a real LSP smoke adapter; eclipse.jdt.ls currently requires Java 21+.
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

## Additional graphify-language smokes

`additional_language_smoke.py` adds live checks for graphify-supported languages
outside the core matrix. Each smoke uses a fresh open-source clone, Atlas with
SQLite, graphify, and the best scriptable native parser baseline available on
this machine. Benchmark-only dependencies are installed under `/tmp/...` workdirs
and do not enter the Atlas runtime binary.

Current live artifacts include Apex, Astro, Bash, Blade, BYOND/DM, C#, CUDA,
Dart, Delphi/Lazarus, .NET project files, EJS, Elixir, ETS, Fortran,
Groovy/Gradle, JSON config, Julia, Kotlin, Lua, Markdown, Objective-C, Pascal,
PHP, PowerShell, R, Razor, Ruby, Rust, Scala, Svelte, SQL, Swift,
Terraform/HCL, Verilog/SystemVerilog, Vue, and Zig. Example:

```sh
CGO_ENABLED=1 go build -o bin/atlas ./cmd/atlas
python3 bench/additional_language_smoke.py \
  --language terraform \
  --atlas ./bin/atlas \
  --graphify "$HOME/.local/share/uv/tools/graphifyy/bin/graphify" \
  --out bench/LIVE_TERRAFORM_SMOKE.json
```

For languages where graphify exposes no equivalent query rows, run the
five-pass saturation loop and keep the raw artifact:

```sh
python3 bench/saturation_check.py \
  --languages byond,ets,r \
  --iterations 5 \
  --atlas ./bin/atlas \
  --graphify "$HOME/.local/share/uv/tools/graphifyy/bin/graphify" \
  --out bench/SATURATION_REPORT.json
```
