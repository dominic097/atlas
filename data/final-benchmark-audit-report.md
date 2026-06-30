# Atlas Final Benchmark Audit

Generated: 2026-06-30T18:09:49.196Z

Final pass over Atlas benchmark artifacts: core matrix against Graphify plus native SCIP/LSP tools, live language artifacts against Graphify plus language-specific native/proxy baselines, and public three-repo validation metadata.

## Summary

- Core matrix languages: 7
- Live code/parser languages: 34
- Total code language surfaces: 41
- Live artifacts: 36
- Strict 10x live artifacts: 36/36
- Three-repo validated live artifacts: 35
- Pending code languages: none
- Precision evidence harness: present
- Precision sampled name/location artifacts: 32
- Precision kind-count-only artifacts: 4
- Precision count-only artifacts: 0
- Precision sampled query rows with name+location: 161/199
- Precision validation rows with kind maps: 81
- Precision artifacts with native metric kind maps: 27
- Call-edge evidence harness: present
- Core receiver-typed call languages: 7/7
- Core receiver-typed calls: 5016/21254 (0.236)
- Live artifacts with Atlas call counts: 36/36
- Live artifacts with receiver-typed calls: 0
- Graphify support harness: present
- Graphify deterministic discovery rows: 39
- Graphify detector-only extensions: 3
- Graphify live deterministic artifacts: 33
- Graphify live detector-only artifacts: 3
- Graphify sampled equivalent rows: 199/224
- Graphify: graphifyy 0.8.49, dispatch count 89

## Ground Truth Closeness

Coverage ratios prove Atlas produced at least as many definitions as the selected independent denominator for that scoped benchmark. They do not by themselves prove precision, complete call-edge recall, or semantic equivalence across all repos.

Low-risk live languages: astro, bash, csharp, elixir, fortran, groovy, julia, kotlin, lua, objc, php, powershell, ruby, rust, scala, sql, svelte, swift, terraform, verilog, vue, zig.

### Precision Evidence

The precision manifest is an artifact-level audit of what the raw benchmark JSON can prove today: sampled query rows with matching symbol names and locations, native-vs-Atlas kind-count maps, or count-only gaps. It is not a full 99% precision oracle.

Manifest: data/precision-evidence-manifest.md, generated 2026-06-30T18:09:48.810Z.
Sampled name/location artifacts: 32; kind-count-only artifacts: 4; count-only artifacts: 0.
Matched sampled query rows: 161/199; validation kind-map rows: 81.
Artifacts with native metric kind maps: 27.

| Language | Status | Query name+location | Validation kind rows | Native metric kind map | Gap |
|---|---|--:|--:|---|---|
| blade | kind-count-only | 0/6 | 3/3 | yes | No comparable query row currently proves both Atlas and Graphify returned the same symbol name with source locations; kind-count evidence is present. |
| dart | kind-count-only | 0/6 | 0/3 | yes | No comparable query row currently proves both Atlas and Graphify returned the same symbol name with source locations; kind-count evidence is present. |
| ets | kind-count-only | 0/5 | 3/3 | yes | No comparable query row currently proves both Atlas and Graphify returned the same symbol name with source locations; kind-count evidence is present. |
| r | kind-count-only | 0/5 | 3/3 | yes | No comparable query row currently proves both Atlas and Graphify returned the same symbol name with source locations; kind-count evidence is present. |

### Call Edge Evidence

The call-edge manifest audits what raw artifacts can prove today: receiver-typed call counts for the core matrix and call-count evidence for live artifacts. It does not prove receiver-type precision for live converted languages.

Manifest: data/call-edge-evidence-manifest.md, generated 2026-06-30T18:09:48.967Z.
Core receiver-typed calls: 5016/21254; live Atlas calls: 258529.
Live receiver-typed artifacts: 0/36; live artifacts with call counts: 36/36.

| Language | Status | Atlas calls | Graphify calls | Detector-only Graphify | Note |
|---|---|--:|--:|---|---|
| apex | calls-only | 5006 | 44 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| astro | calls-only | 50 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| bash | calls-only | 102 | 292 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| blade | calls-only | 4062 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| byond | calls-only | 39076 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| csharp | calls-only | 8113 | 690 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| cuda | calls-only | 52 | 2 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| dart | calls-only | 417 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| delphi | calls-only | 65729 | 10195 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| dotnet | calls-only | 8113 | 693 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| ejs | calls-only | 669 | 1 | yes | Raw live artifact exposes call counts but no receiver-type metric. |
| elixir | calls-only | 4783 | n/a | no | Raw live artifact exposes call counts but no receiver-type metric. |
| ets | calls-only | 704 | 0 | yes | Raw live artifact exposes call counts but no receiver-type metric. |
| fortran | calls-only | 4470 | 58 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| groovy | calls-only | 3131 | 358 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| json | calls-only | 359 | 22 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| julia | calls-only | 1038 | 70 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| kotlin | calls-only | 6285 | 1096 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| lua | calls-only | 3347 | 226 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| markdown | calls-only | 19 | 1 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| objc | calls-only | 1828 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| pascal | calls-only | 20943 | 2187 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| php | calls-only | 5713 | 169 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| powershell | calls-only | 804 | 14 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| razor | calls-only | 2327 | 103 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| ruby | calls-only | 3305 | 737 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| rust | calls-only | 15805 | 2657 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| r | calls-only | 16377 | 0 | yes | Raw live artifact exposes call counts but no receiver-type metric. |
| scala | calls-only | 9157 | 1161 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| sql | calls-only | 825 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| svelte | calls-only | 3118 | 114 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| swift | calls-only | 7224 | 994 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| terraform | calls-only | 1078 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| verilog | calls-only | 2656 | n/a | no | Raw live artifact exposes call counts but no receiver-type metric. |
| vue | calls-only | 280 | 2 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| zig | calls-only | 11564 | 1362 | no | Raw live artifact exposes call counts but no receiver-type metric. |

### Graphify Support

The Graphify support manifest separates deterministic extractor support from detector-only extension detection and sampled query rows with no Graphify equivalent.

Manifest: data/graphify-support-manifest.md, generated 2026-06-30T18:09:49.126Z.
Deterministic discovery rows: 39; detector-only extensions: 3.
Live deterministic artifacts: 33; live detector-only artifacts: 3; sampled Graphify-equivalent rows: 199/224.

Detector-only extensions:
- .ejs: ejs
- .ets: ets
- .r: r

| Language | Support | Query rows | Graphify equivalent | Graphify missing | Note |
|---|---|--:|--:|--:|---|
| ejs | detector-only | 7 | 1 | 6 | Graphify detects this extension but discovery reports no deterministic _DISPATCH extractor. |
| ets | detector-only | 13 | 5 | 8 | Graphify detects this extension but discovery reports no deterministic _DISPATCH extractor. |
| julia | deterministic-extractor | 6 | 5 | 1 | Graphify has a deterministic extractor but some sampled query rows have no Graphify equivalent. |
| objc | deterministic-extractor | 5 | 4 | 1 | Graphify has a deterministic extractor but some sampled query rows have no Graphify equivalent. |
| php | deterministic-extractor | 4 | 3 | 1 | Graphify has a deterministic extractor but some sampled query rows have no Graphify equivalent. |
| r | detector-only | 13 | 5 | 8 | Graphify detects this extension but discovery reports no deterministic _DISPATCH extractor. |

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
- The committed precision-evidence harness regenerates data/precision-evidence-manifest.* from raw live artifacts and separates sampled symbol/location evidence from weaker kind-count-only rows.
- The committed call-edge evidence harness regenerates data/call-edge-evidence-manifest.* from raw artifacts and separates core receiver-typed call evidence from live call-count-only rows.
- The committed Graphify support harness regenerates data/graphify-support-manifest.* and separates deterministic extractor rows from detector-only extension support.
- Objective-C validation can be inflated by vendored Pods if Atlas and the native counter use different dependency filters; the final validation excludes dependency folders for the validation count.
- CUDA host-function counters overcount the denominator for a CUDA-specific benchmark; the final validation labels and uses a CUDA-qualified __global__/__device__/__host__ function denominator.

No hidden synthetic-row finding: No published benchmark row in the final dataset is intentionally synthetic or sample-only. The weakest rows are labelled as detector-only or source-counter proxy rows rather than hidden.

### Missing Adapters

- none

## Improvement Todos

- P1: Promote the committed public-repo validation manifest harness from artifact verification to full remeasurement for every native/proxy counter.
- P1: Replace source-counter proxies for Apex, CUDA, Razor, BYOND, Blade, EJS, ETS, R, and structured/project surfaces with fuller compiler, LSP, tree-sitter, or parser-library denominators where available.
- P1: Close precision gaps by persisting full native and Atlas symbol name/kind/location sets for every validation repo; the current harness proves only sampled query name/location rows plus kind-count maps where raw artifacts expose them.
- P1: Extend receiver-type measurement into live converted tree-sitter artifacts; the current call-edge harness proves live call counts but receiver typing is only present in the core matrix artifacts.
- P2: Increase public-repo validation from 3 repos per language to a larger fixed sample for high-variance languages such as Objective-C, Razor, Apex, CUDA, and Swift.

