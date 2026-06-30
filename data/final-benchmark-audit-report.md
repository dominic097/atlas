# Atlas Final Benchmark Audit

Generated: 2026-06-30T17:37:40.737Z

Final pass over Atlas benchmark artifacts: core matrix against Graphify plus native SCIP/LSP tools, live language artifacts against Graphify plus language-specific native/proxy baselines, and public three-repo validation metadata.

## Summary

- Core matrix languages: 7
- Live code/parser languages: 34
- Total code language surfaces: 41
- Live artifacts: 36
- Strict 10x live artifacts: 36/36
- Three-repo validated live artifacts: 35
- Pending code languages: none
- Graphify: graphifyy 0.8.49, dispatch count 89

## Ground Truth Closeness

Coverage ratios prove Atlas produced at least as many definitions as the selected independent denominator for that scoped benchmark. They do not by themselves prove precision, complete call-edge recall, or semantic equivalence across all repos.

Low-risk live languages: astro, bash, csharp, dart, elixir, fortran, groovy, julia, kotlin, lua, objc, php, powershell, ruby, rust, scala, sql, svelte, swift, terraform, verilog, vue, zig.

### Weak Or Proxy Truth Rows

| Language | Native tool | Risk | Coverage | Min validation coverage | Reason |
|---|---|---|--:|--:|---|
| apex | apex-source-counter | medium | 1.215 | 1.0607 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| blade | blade-directive-counter | medium | 1.0028 | 1.0121 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| byond | byond-source-counter | medium | 1.2311 | 1 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| cuda | cuda-source-counter | medium | 1.3333 | 2.5753 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| delphi | delphi-lazarus-source-counter | medium | 3.4626 | 1 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| dotnet | python-dotnet-project | medium | 17.235 | 1 | Atlas counts a wider code-target surface than the native denominator; inspect scope before treating the ratio as precision. |
| ejs | ejs-template-counter | high | 2.278 | 1 | Graphify detector-only/source-counter proxy; weaker than a deterministic Graphify extractor and native compiler/LSP truth. |
| ets | ets-source-counter | high | 1.0523 | 1.0229 | Graphify detector-only/source-counter proxy; weaker than a deterministic Graphify extractor and native compiler/LSP truth. |
| json | python-json | structured | 1.483 | n/a | Structured artifact, not a code-parser ground-truth row; keep separate from regex-language completion claims. |
| markdown | markdown-it-py | structured | 1.026 | 1 | Structured artifact, not a code-parser ground-truth row; keep separate from regex-language completion claims. |
| pascal | pascal-regex-counter | medium | 1.0067 | 1.21 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| r | r-source-counter | high | 1.7531 | 1.6067 | Graphify detector-only/source-counter proxy; weaker than a deterministic Graphify extractor and native compiler/LSP truth. |
| razor | razor-directive-counter | medium | 5.101 | 1 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |

### Core Matrix

| Language | Native tools | Graphify | Equivalent rows | Graphify missing | Token ratio | Latency ratio |
|---|---|---|--:|--:|--:|--:|
| go | scip-go:ok, gopls:ok | ok | 4 | 0 | 18.95 | 6.44 |
| python | scip-python:ok, pyright:ok | ok | 3 | 0 | 24.31 | 6.63 |
| javascript | scip-typescript:ok, tsserver:ok | ok | 3 | 1 | 8.24 | 5.74 |
| typescript | scip-typescript:ok, tsserver:ok | ok | 3 | 1 | 12.06 | 5.95 |
| java | scip-java:ok, jdtls:ok | ok | 2 | 0 | 21.4 | 7.37 |
| c | clangd:ok | ok | 4 | 0 | 33.5 | 7.33 |
| cpp | clangd:ok | ok | 4 | 0 | 11.85 | 8.21 |

## Stubs And Hallucination Audit

### Found During Final Pass

- The UI previously carried a hard-coded native tool manifest; this final pass renders tool status/version from provenance data so missing tools are no longer shown as healthy.
- scip-java now resolves through the pinned bench/tools/scip-java-coursier launcher; Java is reported with both SCIP and JDTLS baselines present.
- The committed public-repo validation harness regenerates data/public-repo-validation-manifest.* from raw live artifacts and fails when a code language lacks passing three-repo evidence.
- Objective-C validation can be inflated by vendored Pods if Atlas and the native counter use different dependency filters; the final validation excludes dependency folders for the validation count.
- CUDA host-function counters overcount the denominator for a CUDA-specific benchmark; the final validation labels and uses a CUDA-qualified __global__/__device__/__host__ function denominator.

No hidden synthetic-row finding: No published benchmark row in the final dataset is intentionally synthetic or sample-only. The weakest rows are labelled as detector-only or source-counter proxy rows rather than hidden.

### Missing Adapters

- none

## Improvement Todos

- P1: Promote the committed public-repo validation manifest harness from artifact verification to full remeasurement for every native/proxy counter.
- P1: Replace source-counter proxies for Apex, CUDA, Razor, BYOND, Blade, EJS, ETS, R, and structured/project surfaces with fuller compiler, LSP, tree-sitter, or parser-library denominators where available.
- P1: Add precision checks that compare symbol names/kinds/locations, not only Atlas/native definition-count coverage ratios.
- P1: Extend call-edge and receiver-type measurement for converted tree-sitter languages beyond definition coverage.
- P2: Increase public-repo validation from 3 repos per language to a larger fixed sample for high-variance languages such as Objective-C, Razor, Apex, CUDA, and Swift.
- P2: Keep Graphify no-equivalent rows as saturation evidence, but separate detector-only language support from deterministic Graphify extractor support in all headlines.

