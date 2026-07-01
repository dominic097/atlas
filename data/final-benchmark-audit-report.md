# Atlas Final Benchmark Audit

Generated: 2026-07-01T05:39:57.345Z

Final pass over Atlas benchmark artifacts: supported-language fixture sweep for every parser.Supported family, public-repo matrix artifacts against Graphify plus native SCIP/LSP tools, live language artifacts against Graphify plus language-specific native/proxy baselines, and public three-repo validation metadata.

## Summary

- Core matrix languages: 7
- Live code/parser languages: 34
- Total code language surfaces: 41
- Live artifacts: 36
- Strict 10x live artifacts: 36/36
- Three-repo validated live artifacts: 35
- Pending code languages: none
- Validation remeasurement harness: present
- Validation repo rows: 105
- Atlas replay-ready validation rows: 105
- Graphify replay-ready validation rows: 105
- Native/proxy command candidate validation rows: 105
- Native/proxy candidate executable validation rows: 21
- Native/proxy command-ready validation rows: 0
- Full remeasurement-ready artifacts: 0
- Native command candidates with placeholders: 69
- Native command candidates with ephemeral helper paths: 51
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
- Atlas supported parser families: 64
- Supported-family Atlas ok: 64/64
- Supported fixture 100% recall: 43/62
- Supported fixture 100% precision: 39/62
- Supported fixture exact rows: 36/62
- Supported Graphify support: {"detector_only":2,"deterministic":40,"unsupported":22}
- Supported native/tool status: {"missing":42,"ok":22}

## Ground Truth Closeness

Coverage ratios prove Atlas produced at least as many definitions as the selected independent denominator for that scoped benchmark. They do not by themselves prove precision, complete call-edge recall, or semantic equivalence across all repos.

Low-risk live languages: astro, csharp, fortran, groovy, julia, kotlin, lua, objc, php, powershell, ruby, rust, scala, sql, svelte, swift, terraform, vue, zig.

### Validation Remeasurement Readiness

The validation remeasurement manifest audits replay readiness for public validation rows. It proves pinned Atlas and Graphify replay commands can be reconstructed, but it does not execute native/proxy counters unless executable per-repo native commands are present in the artifacts.

Manifest: data/validation-remeasurement-manifest.md, generated 2026-06-30T19:52:49.769Z.
Repo rows: 105; Atlas replay-ready: 105; Graphify replay-ready: 105; native/proxy command candidates: 105; candidate-executable: 21; native/proxy command-ready: 0.
Full remeasurement-ready artifacts: 0; candidates with placeholders: 69; candidates with ephemeral helper paths: 51; proxy or detector-only code artifacts: 14.

| Language | Tool class | Risk | Repos | Atlas replay | Graphify replay | Native candidates | Candidate executable | Native ready | Blockers |
|---|---|---|--:|--:|--:|--:|--:|--:|---|
| apex | source-counter-proxy | medium | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| astro | parser-library-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| bash | syntax-checker-proxy | medium | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_unexpanded_file_list, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| blade | source-counter-proxy | medium | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| byond | source-counter-proxy | medium | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| csharp | compiler-or-lsp-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_inline_note, native_or_proxy_remeasurement_command_not_recorded |
| cuda | source-counter-proxy | medium | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| dart | tree-sitter-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| delphi | source-counter-proxy | medium | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| dotnet | scope-proxy | medium | 3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| ejs | graphify-detector-only-proxy | high | 3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, graphify_detector_only_or_weak_proxy_truth, native_or_proxy_remeasurement_command_not_recorded |
| elixir | scope-proxy | medium | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| ets | graphify-detector-only-proxy | high | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, graphify_detector_only_or_weak_proxy_truth, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded |
| fortran | tree-sitter-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| groovy | tree-sitter-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| json | structured-format | structured | 0 | 0 | 0 | 0 | 0 | 0 | no_public_repo_validation_rows, structured_format_outside_code_parser_gate |
| julia | tree-sitter-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| kotlin | tree-sitter-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| lua | parser-library-baseline | low | 3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded |
| markdown | structured-format | structured | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded, structured_format_outside_code_parser_gate |
| objc | tree-sitter-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| pascal | source-counter-proxy | medium | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| php | parser-library-baseline | low | 3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded |
| powershell | parser-library-baseline | low | 3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded |
| razor | source-counter-proxy | medium | 3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| ruby | parser-library-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| rust | compiler-or-lsp-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_unexpanded_file_list, native_or_proxy_remeasurement_command_not_recorded |
| r | graphify-detector-only-proxy | high | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, graphify_detector_only_or_weak_proxy_truth, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded |
| scala | tree-sitter-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| sql | parser-library-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| svelte | parser-library-baseline | low | 3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded |
| swift | compiler-or-lsp-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| terraform | parser-library-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| verilog | scope-proxy | medium | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| vue | parser-library-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| zig | tree-sitter-baseline | low | 3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |

### Precision Evidence

The precision manifest is an artifact-level audit of what the raw benchmark JSON can prove today: sampled query rows with matching symbol names and locations, native-vs-Atlas kind-count maps, or count-only gaps. It is not a full 99% precision oracle.

Manifest: data/precision-evidence-manifest.md, generated 2026-06-30T19:52:49.947Z.
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

Manifest: data/call-edge-evidence-manifest.md, generated 2026-06-30T19:52:50.142Z.
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

Manifest: data/graphify-support-manifest.md, generated 2026-06-30T19:52:50.322Z.
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

### Supported-Language Fixture Sweep

The supported-language fixture sweep proves every Atlas-supported parser family can be indexed and compared against Graphify/runtime support plus local native/tool availability. It is fixture evidence, not a public-repo semantic oracle.

Artifact: data/raw/SUPPORTED_LANGUAGE_BENCHMARK.json, generated 2026-07-01T05:34:56Z.
Families: 64; Atlas ok: 64/64; Graphify support: {"detector_only":2,"deterministic":40,"unsupported":22}; native/tool status: {"missing":42,"ok":22}.
Fixture oracle: recall 100% 43/62; precision 100% 39/62; exact 36/62.

| Language | Category | Recall | Precision | Graphify | Native/tool | Missing | Extra |
|---|---|--:|--:|---|---|---|---|
| typescript | code | 1 | 0.8 | deterministic/ok | ok | none | constructor |
| kotlin | code | 1 | 1 | deterministic/ok | missing | none | none |
| scala | code | 1 | 1 | deterministic/ok | missing | none | none |
| swift | code | 1 | 1 | deterministic/ok | missing | none | none |
| lua | code | 0.5 | 0.5 | deterministic/ok | missing | run | M.run |
| zig | code | 1 | 0.6 | deterministic/ok | missing | none | _, std |
| elixir | code | 1 | 1 | deterministic/ok | missing | none | none |
| objc | code | 0.6667 | 1 | deterministic/ok | missing | helper | none |
| julia | code | 1 | 1 | deterministic/ok | missing | none | none |
| fortran | code | 1 | 1 | deterministic/ok | missing | none | none |
| dart | code | 1 | 1 | deterministic/ok | missing | none | none |
| verilog | code | 1 | 1 | deterministic/ok | missing | none | none |
| pascal | code | 0.6667 | 0.6667 | deterministic/ok | missing | TWorker | TWorker.Run |
| delphi | code | 1 | 0.5 | deterministic/ok | missing | none | TButton, TMainForm |
| terraform | code | 1 | 1 | deterministic/ok | missing | none | none |
| byond | code | 0.25 | 0.3333 | deterministic/ok | missing | health, helper, run | /proc, /proc/helper |
| dotnet | project | 1 | 0.5 | deterministic/ok | missing | none | Microsoft.NET.Sdk, Sample |
| razor | template | 0.5 | 0.25 | deterministic/ok | missing | counter | /counter, Counter, Microsoft.AspNetCore.Components |
| apex | code | 1 | 0.6667 | deterministic/ok | missing | none | Account |
| blade | template | 0.5 | 0.3333 | deterministic/ok | missing | foreach | layouts.app, view |
| vue | template | 1 | 1 | deterministic/ok | missing | none | none |
| svelte | template | 0.5 | 1 | deterministic/ok | missing | name | none |
| astro | template | 1 | 0.5 | deterministic/ok | missing | none | App, BaseLayout |
| ejs | template | 0.5 | 0.5 | detector_only/ok | missing | partials/header | view |
| ets | code | 1 | 1 | detector_only/ok | missing | none | none |
| r | code | 1 | 1 | unsupported/unsupported | missing | none | none |
| p4 | code | 1 | 0.75 | unsupported/unsupported | missing | none | start |
| csharp | code | 1 | 1 | deterministic/ok | missing | none | none |
| groovy | code | 1 | 1 | deterministic/ok | missing | none | none |
| html | markup | n/a | n/a | unsupported/unsupported | missing | none | none |
| css | style | n/a | n/a | unsupported/unsupported | missing | none | none |
| markdown | structured | 1 | 1 | deterministic/ok | missing | none | none |
| mdx | structured | 1 | 1 | deterministic/ok | missing | none | none |
| yaml | structured | 0 | 0 | unsupported/unsupported | missing | name, port, service | config.yaml |
| proto | structured | 0.6667 | 1 | unsupported/unsupported | ok | Run | none |
| toml | structured | 0 | 0 | unsupported/unsupported | ok | database, name, service, url | config.toml |
| xml | structured | 0 | 0 | unsupported/unsupported | ok | port, project, service | config.xml |
| plist | structured | 0 | 0 | unsupported/unsupported | missing | CFBundleName | Info.plist |
| gomod | structured | 0 | 0 | unsupported/unsupported | missing | example.com/atlas-fixture, github.com/google/uuid | go.mod |
| gosum | structured | 0 | 0 | unsupported/unsupported | missing | github.com/google/uuid | go.sum |
| config | structured | 0 | 0 | unsupported/unsupported | missing | ATLAS_DB, ATLAS_LOG_LEVEL | .env.example |
| makefile | structured | 1 | 1 | unsupported/unsupported | missing | none | none |
| batch | structured | 0 | 0 | unsupported/unsupported | missing | ATLAS_ENV | build.cmd |
| sql | structured | 1 | 1 | deterministic/ok | missing | none | none |
| csv | structured | 0 | 0 | unsupported/unsupported | ok | name, role | data.csv |
| text | structured | 1 | 1 | unsupported/unsupported | missing | none | none |
| dockerfile | structured | 0 | 0 | unsupported/unsupported | missing | ARG, CMD, FROM, RUN | Dockerfile |
| pptx | binary | 1 | 1 | unsupported/unsupported | ok | none | none |
| docx | binary | 1 | 1 | unsupported/unsupported | ok | none | none |
| xlsx | binary | 1 | 1 | unsupported/unsupported | ok | none | none |
| image | binary | 1 | 1 | unsupported/unsupported | ok | none | none |
| pdf | binary | 1 | 1 | unsupported/unsupported | ok | none | none |

### Weak Or Proxy Truth Rows

| Language | Native tool | Risk | Coverage | Min validation coverage | Reason |
|---|---|---|--:|--:|---|
| apex | apex-source-counter | medium | 1.215 | 1.0607 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| bash | bash-n | medium | 1.044 | 1 | Atlas counts a wider code-target surface than the native denominator; inspect scope before treating the ratio as precision. |
| blade | blade-directive-counter | medium | 1.0028 | 1.0121 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| byond | byond-source-counter | medium | 1.2311 | 1 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| cuda | cuda-source-counter | medium | 1.3333 | 2.5753 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| delphi | delphi-lazarus-source-counter | medium | 3.4626 | 1 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| dotnet | python-dotnet-project | medium | 17.235 | 1 | Atlas counts a wider code-target surface than the native denominator; inspect scope before treating the ratio as precision. |
| ejs | ejs-template-counter | high | 2.278 | 1 | Graphify detector-only/source-counter proxy; weaker than a deterministic Graphify extractor and native compiler/LSP truth. |
| elixir | tree-sitter-elixir module/function proxy | medium | 1.0116 | 1 | Atlas counts a wider code-target surface than the native denominator; inspect scope before treating the ratio as precision. |
| ets | ets-source-counter | high | 1.0523 | 1.0229 | Graphify detector-only/source-counter proxy; weaker than a deterministic Graphify extractor and native compiler/LSP truth. |
| json | python-json | structured | 1.483 | n/a | Structured artifact, not a code-parser ground-truth row; keep separate from regex-language completion claims. |
| markdown | markdown-it-py | structured | 1.026 | 1 | Structured artifact, not a code-parser ground-truth row; keep separate from regex-language completion claims. |
| pascal | pascal-regex-counter | medium | 1.0067 | 1.21 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| r | r-source-counter | high | 1.7531 | 1.6067 | Graphify detector-only/source-counter proxy; weaker than a deterministic Graphify extractor and native compiler/LSP truth. |
| razor | razor-directive-counter | medium | 5.101 | 1 | Scriptable source-counter proxy rather than a full compiler/LSP/SCIP truth source. |
| verilog | tree-sitter-systemverilog module/package/function proxy | medium | 1.5484 | 1 | Atlas counts a wider code-target surface than the native denominator; inspect scope before treating the ratio as precision. |

### Core Matrix

| Language | Native tools | Graphify | Equivalent rows | Graphify missing | Token ratio | Latency ratio |
|---|---|---|--:|--:|--:|--:|
| go | scip-go:ok, gopls:ok | ok | 4 | 0 | 18.95 | 7.18 |
| python | scip-python:ok, pyright:ok | ok | 3 | 0 | 24.31 | 7.51 |
| javascript | scip-typescript:ok, tsserver:ok | ok | 3 | 1 | 8.24 | 6.54 |
| typescript | scip-typescript:ok, tsserver:ok | ok | 3 | 1 | 12.06 | 6.89 |
| java | scip-java:ok, jdtls:ok | ok | 2 | 0 | 21.4 | 7.99 |
| c | clangd:ok | ok | 4 | 0 | 33.5 | 7.53 |
| cpp | clangd:ok | ok | 4 | 0 | 11.85 | 8.73 |

## Stubs And Hallucination Audit

### Found During Final Pass

- The supported-language fixture sweep now records 64 parser.Supported families, with Atlas indexing 64/64; Graphify is {"detector_only":2,"deterministic":40,"unsupported":22} and native/tool status is {"missing":42,"ok":22}.
- The UI previously carried a hard-coded native tool manifest; this final pass renders tool status/version from provenance data so missing tools are no longer shown as healthy.
- scip-java now resolves through the pinned bench/tools/scip-java-coursier launcher; Java is reported with both SCIP and JDTLS baselines present.
- The committed public-repo validation harness regenerates data/public-repo-validation-manifest.* from raw live artifacts and fails when a code language lacks passing three-repo evidence.
- The committed validation remeasurement readiness harness regenerates data/validation-remeasurement-manifest.* and separates Atlas/Graphify replay-ready validation rows, generated native/proxy command candidates, candidate-executable rows, and native/proxy rows that still lack executable per-repo commands.
- The committed precision-evidence harness regenerates data/precision-evidence-manifest.* from raw live artifacts and separates sampled symbol/location evidence from weaker kind-count-only rows.
- The committed call-edge evidence harness regenerates data/call-edge-evidence-manifest.* from raw artifacts and separates core receiver-typed call evidence from live call-count-only rows.
- The committed Graphify support harness regenerates data/graphify-support-manifest.* and separates deterministic extractor rows from detector-only extension support.
- Objective-C validation can be inflated by vendored Pods if Atlas and the native counter use different dependency filters; the final validation excludes dependency folders for the validation count.
- CUDA host-function counters overcount the denominator for a CUDA-specific benchmark; the final validation labels and uses a CUDA-qualified __global__/__device__/__host__ function denominator.

No hidden synthetic-row finding: No published benchmark row in the final dataset is intentionally synthetic or sample-only. The weakest rows are labelled as detector-only or source-counter proxy rows rather than hidden.

### Missing Adapters

- none

## Improvement Todos

- P1: Implement execution mode for validation remeasurement: the readiness manifest proves pinned Atlas/Graphify replay rows, native/proxy command candidates, and candidate-executable rows for committed helpers, but placeholder templates, remaining ephemeral helper paths, and persisted output sets still need to be replaced with executable committed commands.
- P1: Replace the source-counter, proxy, detector-only, and structured/project denominators listed in the weak-truth table with fuller compiler, LSP, tree-sitter, or parser-library denominators where available.
- P1: Close supported-family fixture gaps: 43/62 rows have 100% fixture recall, 39/62 have 100% fixture precision, and 42 rows still lack an executable local native/tool baseline.
- P1: Close precision gaps by persisting full native and Atlas symbol name/kind/location sets for every validation repo; the current harness proves only sampled query name/location rows plus kind-count maps where raw artifacts expose them.
- P1: Extend receiver-type measurement into live converted tree-sitter artifacts; the current call-edge harness proves live call counts but receiver typing is only present in the core matrix artifacts.
- P2: Increase public-repo validation from 3 repos per language to a larger fixed sample for high-variance languages such as Objective-C, Razor, Apex, CUDA, and Swift.

