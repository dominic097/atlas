# Atlas code-intelligence matrix benchmark

This report benchmarks Atlas against the agreed per-language baselines. Raw metrics are kept separate by tool because Atlas, graphify, SCIP, and LSP servers expose different surfaces.

## Tool version manifest

Raw artifact: `bench/MATRIX_TOOL_VERSIONS.json`.
- Platform: Darwin 25.5.0 arm64; Python 3.14.6.

| tool | status | version / first output line | command |
|---|---|---|---|
| atlas | ok | `atlas v0.1.26-20-g06c4d91-dirty (06c4d91, 2026-06-30T17:30:33Z)` | `/Users/damirdarasu/workspace/Aziron/aziron-atlas/bin/atlas version` |
| graphify | ok | `graphifyy 0.8.49` | `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify --version` |
| go | ok | `go version go1.25.0 darwin/arm64` | `/usr/local/go/bin/go version` |
| python | ok | `Python 3.14.6` | `/opt/homebrew/opt/python@3.14/bin/python3.14 --version` |
| java | ok | `openjdk version "17.0.18" 2026-01-20` | `/usr/bin/java -version` |
| maven | ok | `Apache Maven 3.9.16 (2bdd9fddda4b155ebf8000e807eb73fd829a51d5)` | `/opt/homebrew/bin/mvn --version` |
| scip-go | ok | `0.2.7` | `/Users/damirdarasu/go/bin/scip-go --version` |
| scip-python | ok | `0.6.6` | `/opt/homebrew/bin/scip-python --version` |
| scip-typescript | ok | `0.4.0` | `/tmp/atlas-bench-tools/node_modules/.bin/scip-typescript --version` |
| scip-java | ok | `0.12.3` | `/Users/damirdarasu/workspace/Aziron/aziron-atlas/bench/tools/scip-java-coursier --version` |
| gopls | ok | `golang.org/x/tools/gopls v0.22.0` | `/Users/damirdarasu/go/bin/gopls version` |
| pyright | ok | `pyright 1.1.411` | `/opt/homebrew/bin/pyright --version` |
| tsc | ok | `Version 6.0.3` | `/tmp/atlas-bench-tools/node_modules/.bin/tsc --version` |
| jdtls | ok | `1.58.0` | `/opt/homebrew/bin/jdtls` |
| clangd | ok | `Apple clangd version 17.0.0 (clang-1700.6.4.2)` | `/usr/bin/clangd --version` |
| rust-analyzer | ok | `rust-analyzer 0.0.0 (69ccffdb5b 2026-06-21)` | `/opt/homebrew/bin/rust-analyzer --version` |
| dotnet | ok | `10.0.301` | `/opt/homebrew/bin/dotnet --version` |
| ruby | ok | `ruby 2.6.10p210 (2022-04-12 revision 67958) [universal.arm64e-darwin25]` | `/usr/bin/ruby --version` |
| php | ok | `PHP 8.4.14 (cli) (built: Oct 21 2025 19:23:55) (NTS)` | `/opt/homebrew/bin/php --version` |
| pwsh | ok | `PowerShell 7.4.6` | `/usr/local/bin/pwsh --version` |
| sourcekit-lsp | ok | `Apple Swift version 6.2.4 (swiftlang-6.2.4.1.4 clang-1700.6.4.2)` | `/usr/bin/sourcekit-lsp (no version flag); /usr/bin/swift --version` |

- Live benchmark native-version details: 0/0 artifacts expose explicit native tool or library version fields in raw JSON; all artifacts include native command/status.

## graphify language discovery

- Installed graphify: graphifyy 0.8.49 (`/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify`).
- Runtime Python: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/python`.
- Source inspected: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify`.
- Extract source: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify/extract.py`.
- Detect source: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify/detect.py`.
- Evidence: CLI help from `graphify --help` did not enumerate languages, but confirmed `update`, `extract`, and code-only AST update commands.
- Evidence: `graphify.detect.CODE_EXTENSIONS` plus a runtime `detect()` benchmark listed code extensions.
- Evidence: `graphify.extract._DISPATCH` provided the deterministic extractor map used as the parser-parity target.
- Raw discovery artifact: `bench/GRAPHIFY_LANGUAGE_DISCOVERY.json`.
- Runtime help probe: `graphify --help` succeeded and listed 148 command/help lines.
- Runtime support probe: `_DISPATCH` plus filename-special extractors exposed 89 deterministic extractor entries; `CODE_EXTENSIONS` exposed 88 code extensions.
- Runtime detect benchmark: generated one sample per `CODE_EXTENSIONS` entry; `detect()` returned 88 code files.
- Detector-only code extensions in this graphify build, not counted as deterministic parser support because `_DISPATCH` has no extractor for them: `.ejs`, `.ets`, `.r`.

| graphify family | extensions / special cases | graphify extractor | Atlas status |
|---|---|---|---|
| go | `.go` | `extract_go` | native go/parser + go/types |
| python | `.py` | `extract_python` | tree-sitter |
| javascript | `.js .jsx .mjs` | `extract_js` | tree-sitter |
| typescript | `.ts .tsx` | `extract_js` | tree-sitter |
| java | `.java` | `extract_java` | tree-sitter |
| groovy/gradle | `.groovy .gradle` | `extract_groovy` | native tree-sitter AST |
| c | `.c .h` | `extract_c` | tree-sitter |
| cpp/cuda | `.cpp .cc .cxx .hpp .cu .cuh` | `extract_cpp` | tree-sitter |
| csharp | `.cs` | `extract_csharp` | native tree-sitter tags |
| rust | `.rs` | `extract_rust` | native tree-sitter tags |
| ruby | `.rb` | `extract_ruby` | native tree-sitter tags |
| kotlin | `.kt .kts` | `extract_kotlin` | native tree-sitter tags |
| scala | `.scala` | `extract_scala` | native tree-sitter tags |
| php | `.php` | `extract_php` | native tree-sitter tags |
| blade | `*.blade.php` | `extract_blade` | native Blade source parser |
| swift | `.swift` | `extract_swift` | native tree-sitter tags |
| lua | `.lua .luau .toc` | `extract_lua` | native tree-sitter tags |
| zig | `.zig` | `extract_zig` | native tree-sitter tags |
| powershell | `.ps1 .psm1 .psd1` | `extract_powershell/extract_powershell_manifest` | native tree-sitter AST |
| elixir | `.ex .exs` | `extract_elixir` | native tree-sitter AST |
| objective-c | `.m .mm` | `extract_objc` | native tree-sitter AST |
| julia | `.jl` | `extract_julia` | native tree-sitter AST |
| fortran | `.f .F .f90 .F90 .f95 .F95 .f03 .F03 .f08 .F08` | `extract_fortran` | native tree-sitter AST |
| dart | `.dart` | `extract_dart` | native tree-sitter AST |
| verilog/systemverilog | `.v .sv .svh` | `extract_verilog` | native tree-sitter AST |
| sql | `.sql` | `extract_sql` | native SQL source parser |
| markdown | `.md .mdx .qmd` | `extract_markdown` | document parser |
| pascal | `.pas .pp .dpr .dpk .lpr .inc` | `extract_pascal` | native tree-sitter AST |
| delphi/lazarus forms | `.dfm .lfm .lpk` | `extract_delphi_form/extract_lazarus_form/extract_lazarus_package` | native Delphi/Lazarus source parser |
| shell | `.sh .bash` | `extract_bash` | native tree-sitter AST |
| json config | `.json` | `extract_json` | document parser |
| terraform/hcl | `.tf .tfvars .hcl` | `extract_terraform` | native tree-sitter HCL |
| byond dm | `.dm .dme .dmi .dmm .dmf` | `extract_dm/extract_dmf/extract_dmi/extract_dmm` | native BYOND source parser |
| dotnet project | `.sln .slnx .csproj .fsproj .vbproj` | `extract_csproj/extract_sln/extract_slnx` | native structured project parser |
| razor | `.razor .cshtml` | `extract_razor` | native Razor source parser |
| apex | `.cls .trigger` | `extract_apex` | native tree-sitter Apex |
| vue | `.vue` | `extract_js` | native SFC/tree-sitter AST |
| svelte | `.svelte` | `extract_svelte` | native SFC/tree-sitter AST |
| astro | `.astro` | `extract_astro` | native Astro/tree-sitter AST |

## graphify coverage audit

- Deterministic graphify families covered by Atlas evidence: 6/39. Detector-only extensions covered by live Atlas benchmarks: 0/3.
- Unsupported graphify rows: none.
- Missing or partial evidence: `groovy/gradle`, `cpp/cuda`, `csharp`, `rust`, `ruby`, `kotlin`, `scala`, `php`, `blade`, `swift`, `lua`, `zig`, `powershell`, `elixir`, `objective-c`, `julia`, `fortran`, `dart`, `verilog/systemverilog`, `sql`, `markdown`, `pascal`, `delphi/lazarus forms`, `shell`, `json config`, `terraform/hcl`, `byond dm`, `dotnet project`, `razor`, `apex`, `vue`, `svelte`, `astro`, `detector-only .ejs`, `detector-only .ets`, `detector-only .r`.

| graphify support | status | Atlas evidence |
|---|---|---|
| go<br>`.go` via `extract_go` | ok | core matrix `go` ok; native scip-go:ok, gopls:ok; query eq 4/4, latency 6.44x, tokens 18.95x |
| python<br>`.py` via `extract_python` | ok | core matrix `python` ok; native scip-python:ok, pyright:ok; query eq 3/3, latency 6.63x, tokens 24.31x |
| javascript<br>`.js .jsx .mjs` via `extract_js` | ok | core matrix `javascript` ok; native scip-typescript:ok, tsserver:ok; query eq 3/4, latency 5.74x, tokens 8.24x |
| typescript<br>`.ts .tsx` via `extract_js` | ok | core matrix `typescript` ok; native scip-typescript:ok, tsserver:ok; query eq 3/4, latency 5.95x, tokens 12.06x |
| java<br>`.java` via `extract_java` | ok | core matrix `java` ok; native scip-java:ok, jdtls:ok; query eq 2/2, latency 7.37x, tokens 21.4x |
| groovy/gradle<br>`.groovy .gradle` via `extract_groovy` | partial | `bench/LIVE_GROOVY_BENCHMARK.json` missing |
| c<br>`.c .h` via `extract_c` | ok | core matrix `c` ok; native clangd:ok; query eq 4/4, latency 7.33x, tokens 33.5x |
| cpp/cuda<br>`.cpp .cc .cxx .hpp .cu .cuh` via `extract_cpp` | partial | core matrix `cpp` ok; native clangd:ok; query eq 4/4, latency 8.21x, tokens 11.85x<br>`bench/LIVE_CUDA_BENCHMARK.json` missing |
| csharp<br>`.cs` via `extract_csharp` | partial | `bench/LIVE_CSHARP_BENCHMARK.json` missing |
| rust<br>`.rs` via `extract_rust` | partial | `bench/LIVE_RUST_BENCHMARK.json` missing |
| ruby<br>`.rb` via `extract_ruby` | partial | `bench/LIVE_RUBY_BENCHMARK.json` missing |
| kotlin<br>`.kt .kts` via `extract_kotlin` | partial | `bench/LIVE_KOTLIN_BENCHMARK.json` missing |
| scala<br>`.scala` via `extract_scala` | partial | `bench/LIVE_SCALA_BENCHMARK.json` missing |
| php<br>`.php` via `extract_php` | partial | `bench/LIVE_PHP_BENCHMARK.json` missing |
| blade<br>`*.blade.php` via `extract_blade` | partial | `bench/LIVE_BLADE_BENCHMARK.json` missing |
| swift<br>`.swift` via `extract_swift` | partial | `bench/LIVE_SWIFT_BENCHMARK.json` missing |
| lua<br>`.lua .luau .toc` via `extract_lua` | partial | `bench/LIVE_LUA_BENCHMARK.json` missing |
| zig<br>`.zig` via `extract_zig` | partial | `bench/LIVE_ZIG_BENCHMARK.json` missing |
| powershell<br>`.ps1 .psm1 .psd1` via `extract_powershell/extract_powershell_manifest` | partial | `bench/LIVE_POWERSHELL_BENCHMARK.json` missing |
| elixir<br>`.ex .exs` via `extract_elixir` | partial | `bench/LIVE_ELIXIR_BENCHMARK.json` missing |
| objective-c<br>`.m .mm` via `extract_objc` | partial | `bench/LIVE_OBJC_BENCHMARK.json` missing |
| julia<br>`.jl` via `extract_julia` | partial | `bench/LIVE_JULIA_BENCHMARK.json` missing |
| fortran<br>`.f .F .f90 .F90 .f95 .F95 .f03 .F03 .f08 .F08` via `extract_fortran` | partial | `bench/LIVE_FORTRAN_BENCHMARK.json` missing |
| dart<br>`.dart` via `extract_dart` | partial | `bench/LIVE_DART_BENCHMARK.json` missing |
| verilog/systemverilog<br>`.v .sv .svh` via `extract_verilog` | partial | `bench/LIVE_VERILOG_BENCHMARK.json` missing |
| sql<br>`.sql` via `extract_sql` | partial | `bench/LIVE_SQL_BENCHMARK.json` missing |
| markdown<br>`.md .mdx .qmd` via `extract_markdown` | partial | `bench/LIVE_MARKDOWN_BENCHMARK.json` missing |
| pascal<br>`.pas .pp .dpr .dpk .lpr .inc` via `extract_pascal` | partial | `bench/LIVE_PASCAL_BENCHMARK.json` missing |
| delphi/lazarus forms<br>`.dfm .lfm .lpk` via `extract_delphi_form/extract_lazarus_form/extract_lazarus_package` | partial | `bench/LIVE_DELPHI_BENCHMARK.json` missing |
| shell<br>`.sh .bash` via `extract_bash` | partial | `bench/LIVE_BASH_BENCHMARK.json` missing |
| json config<br>`.json` via `extract_json` | partial | `bench/LIVE_JSON_BENCHMARK.json` missing |
| terraform/hcl<br>`.tf .tfvars .hcl` via `extract_terraform` | partial | `bench/LIVE_TERRAFORM_BENCHMARK.json` missing |
| byond dm<br>`.dm .dme .dmi .dmm .dmf` via `extract_dm/extract_dmf/extract_dmi/extract_dmm` | partial | `bench/LIVE_BYOND_BENCHMARK.json` missing |
| dotnet project<br>`.sln .slnx .csproj .fsproj .vbproj` via `extract_csproj/extract_sln/extract_slnx` | partial | `bench/LIVE_DOTNET_BENCHMARK.json` missing |
| razor<br>`.razor .cshtml` via `extract_razor` | partial | `bench/LIVE_RAZOR_BENCHMARK.json` missing |
| apex<br>`.cls .trigger` via `extract_apex` | partial | `bench/LIVE_APEX_BENCHMARK.json` missing |
| vue<br>`.vue` via `extract_js` | partial | `bench/LIVE_VUE_BENCHMARK.json` missing |
| svelte<br>`.svelte` via `extract_svelte` | partial | `bench/LIVE_SVELTE_BENCHMARK.json` missing |
| astro<br>`.astro` via `extract_astro` | partial | `bench/LIVE_ASTRO_BENCHMARK.json` missing |
| detector-only .ejs<br>`.ejs` in `CODE_EXTENSIONS`, no `_DISPATCH` extractor | partial | `bench/LIVE_EJS_BENCHMARK.json` missing |
| detector-only .ets<br>`.ets` in `CODE_EXTENSIONS`, no `_DISPATCH` extractor | partial | `bench/LIVE_ETS_BENCHMARK.json` missing |
| detector-only .r<br>`.r` in `CODE_EXTENSIONS`, no `_DISPATCH` extractor | partial | `bench/LIVE_R_BENCHMARK.json` missing |

## Saturation loop evidence

Raw artifacts: `bench/SATURATION_REPORT.json` and `bench/SATURATION_REPORT.md`. Iterations requested per language: 0.

| language | status | iterations | equivalent rows by pass | graphify missing rows by pass | coverage ratio by pass |
|---|---|---:|---|---|---|

## Tool matrix

| Language | Repo | Atlas | graphify | SCIP | LSP |
|---|---|---|---|---|---|
| go | sirupsen/logrus | 679 symbols, 2102 calls, 0.5s cold full (0.025s delta) | 711 nodes, 333 calls, 0.636s full (0.287s delta) | 2225 symbols, 11887 occ, 0.41s | 12 pkgs, 0 diag, 0.608s |
| python | psf/requests | 517 symbols, 961 calls, 0.13s cold full (0.027s delta) | 580 nodes, 229 calls, 0.611s full (0.281s delta) | 1518 symbols, 8224 occ, 2.191s | 19 files, 12 diag, 0.776s |
| javascript | expressjs/express | 312 symbols, 435 calls, 0.103s cold full (0.032s delta) | 31 nodes, 3 calls, 0.153s full (0.134s delta) | 398 symbols, 2649 occ, 0.359s | 57 files, 257 diag, 0.274s |
| typescript | pmndrs/zustand | 226 symbols, 197 calls, 0.109s cold full (0.022s delta) | 112 nodes, 6 calls, 0.182s full (0.155s delta) | 792 symbols, 2461 occ, 0.387s | 124 files, 1 diag, 0.229s |
| java | google/gson | 1558 symbols, 3105 calls, 0.259s cold full (0.026s delta) | 1016 nodes, 927 calls, 0.988s full (0.621s delta) | 3408 symbols, 21514 occ, 7.313s | 54 doc syms, 0 diag, 5.153s |
| c | DaveGamble/cJSON | 1790 symbols, 4973 calls, 0.359s cold full (0.027s delta) | 971 nodes, 1018 calls, 0.942s full (0.514s delta) | n/a | 258 doc syms, 2 diag, 0.135s |
| cpp | google/leveldb | 2088 symbols, 9481 calls, 0.314s cold full (0.042s delta) | 2206 nodes, 1195 calls, 1.309s full (0.82s delta) | n/a | 421 doc syms, 167 diag, 0.242s |

## Derived Go ratios

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.5s vs graphify FULL extract 0.636s (graphify/Atlas = 1.27x); scip-go cold 0.41s (scip-go/Atlas = 0.82x); gopls (workspace type-check via `gopls stats`) cold 0.608s (gopls/Atlas = 1.22x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.025s vs graphify 0.287s, graphify/Atlas = 11.48x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:2ms, resolve_head:0ms.
- Atlas edge kinds: calls:2102, imports:224, references:622.
- Call coverage proxy: Atlas internal calls 1143 vs graphify calls 333, Atlas/graphify = 3.43x.
- Atlas receiver-typed calls: 632/2102 = 30.1%.
- graphify extracted calls: 198/333 = 59.5%.
- SCIP semantic index: 47 documents, 2225 symbols, 11887 occurrences, 9656 references.
- SCIP navigation symbols (excluding local variables/packages) = 637; Atlas symbols vs SCIP navigation symbols = 1.07x.
- SCIP local variables = 1570. Atlas currently keeps locals out of the first-class symbol table, which lowers token cost but limits fine-grained reference parity.
- gopls workspace truth: 12 workspace packages, 57 compiled Go files, 0 diagnostics, initial load 293.084ms.
- Query token cost (4/4 equivalent rows): graphify 398 tokens vs Atlas 21 tokens, graphify/Atlas = 18.95x.
- Query latency (4/4 equivalent rows): graphify 331.001ms vs Atlas 51.396ms, graphify/Atlas = 6.44x.
- Go cold-build saturation: cold-vs-cold full-index ratio is 1.27x (graphify FULL 0.636s / Atlas cold 0.5s), below 5x; Atlas's largest cold phases are build_symbols_edges:331ms, go_types:330ms, lexical:45ms.

## Derived Python ratios

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.13s vs graphify FULL extract 0.611s (graphify/Atlas = 4.7x); scip-python cold 2.191s (scip-python/Atlas = 16.85x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.027s vs graphify 0.281s, graphify/Atlas = 10.41x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:961, imports:410.
- Call coverage proxy: Atlas internal calls 371 vs graphify calls 229, Atlas/graphify = 1.62x.
- graphify extracted calls: 221/229 = 96.5%.
- SCIP semantic index: 19 documents, 1518 symbols, 8224 occurrences, 6739 references, scope=repo-root.
- Atlas symbols vs SCIP symbols = 0.34x. scip-python 0.6.6 reports all Python symbols as UnspecifiedKind, so this is a raw coverage proxy, not navigation-kind parity.
- Python AST callable/class truth: Atlas 320/320 function/method/class symbols = 100.0% recall across 19 files.
- Python AST assignment truth: Atlas 197 assignment symbols vs 133 direct module/class assignment names; extra symbols can come from conditional class scopes.
- Pyright truth pass: 19 files analyzed, 12 diagnostics (error:12), version 1.1.411.
- Query token cost (3/3 equivalent rows): graphify 389 tokens vs Atlas 16 tokens, graphify/Atlas = 24.31x.
- Query latency (3/3 equivalent rows): graphify 257.002ms vs Atlas 38.775ms, graphify/Atlas = 6.63x.
- Python cold-build saturation: cold-vs-cold full-index ratio is 4.7x (graphify FULL 0.611s / Atlas cold 0.13s), below 5x; Atlas's largest cold phases are lexical:48ms, persist:48ms, write_sqlite:48ms.

## Derived JS/TS ratios

### javascript

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.103s vs graphify FULL extract 0.153s (graphify/Atlas = 1.49x); scip-typescript cold 0.359s (scip-typescript/Atlas = 3.49x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.032s vs graphify 0.134s, graphify/Atlas = 4.19x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:435, imports:51.
- Call coverage proxy: Atlas internal calls 231 vs graphify calls 3, Atlas/graphify = 77.0x.
- Atlas receiver-typed calls: 0/435 = 0.0%.
- graphify extracted calls: 3/3 = 100.0%.
- SCIP semantic index: 6 documents, 398 symbols, 2649 occurrences, 2251 references, scope=lib.
- Atlas symbols vs SCIP symbols = 0.78x. scip-typescript reports symbols as UnspecifiedKind, so this is a raw coverage proxy.
- TypeScript semantic check proxy: 57 files, 257 diagnostics, total 0.2s, memory 82203KB.
- LSP caveat: tsc returned diagnostics/exit 2; used as scriptable tsserver proxy.
- Query token cost (3/4 equivalent rows): graphify 140 tokens vs Atlas 17 tokens, graphify/Atlas = 8.24x.
- Query latency (3/4 equivalent rows): graphify 222.357ms vs Atlas 38.759ms, graphify/Atlas = 5.74x.
- Query caveat: graphify missed 1 Atlas-selected hub symbols; raw rows remain in the table.
- javascript cold-build saturation: cold-vs-cold full-index ratio is 1.49x (graphify FULL 0.153s / Atlas cold 0.103s), below 5x; Atlas's largest cold phases are lexical:23ms, persist:23ms, write_sqlite:23ms.
### typescript

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.109s vs graphify FULL extract 0.182s (graphify/Atlas = 1.67x); scip-typescript cold 0.387s (scip-typescript/Atlas = 3.55x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.022s vs graphify 0.155s, graphify/Atlas = 7.05x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:197, imports:27.
- Call coverage proxy: Atlas internal calls 81 vs graphify calls 6, Atlas/graphify = 13.5x.
- Atlas receiver-typed calls: 8/197 = 4.1%.
- graphify extracted calls: 6/6 = 100.0%.
- SCIP semantic index: 16 documents, 792 symbols, 2461 occurrences, 1669 references, scope=src.
- Atlas symbols vs SCIP symbols = 0.29x. scip-typescript reports symbols as UnspecifiedKind, so this is a raw coverage proxy.
- TypeScript semantic check proxy: 124 files, 1 diagnostics, total 0.15s, memory 72310KB.
- LSP caveat: tsc returned diagnostics/exit 2; used as scriptable tsserver proxy.
- Query token cost (3/4 equivalent rows): graphify 217 tokens vs Atlas 18 tokens, graphify/Atlas = 12.06x.
- Query latency (3/4 equivalent rows): graphify 230.445ms vs Atlas 38.7ms, graphify/Atlas = 5.95x.
- Query caveat: graphify missed 1 Atlas-selected hub symbols; raw rows remain in the table.
- typescript cold-build saturation: cold-vs-cold full-index ratio is 1.67x (graphify FULL 0.182s / Atlas cold 0.109s), below 5x; Atlas's largest cold phases are lexical:17ms, persist:17ms, write_sqlite:17ms.

## Derived Java ratios

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.259s vs graphify FULL extract 0.988s (graphify/Atlas = 3.81x); scip-java cold 7.313s (scip-java/Atlas = 28.24x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.026s vs graphify 0.621s, graphify/Atlas = 23.88x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:4ms, resolve_head:0ms.
- Atlas edge kinds: calls:3105, imports:677.
- Call coverage proxy: Atlas internal calls 2403 vs graphify calls 927, Atlas/graphify = 2.59x.
- Atlas receiver-typed calls: 2326/3105 = 74.9%.
- graphify extracted calls: 599/927 = 64.6%.
- SCIP semantic index: 85 documents, 3408 symbols, 21514 occurrences, 18109 references, scope=gson.
- SCIP navigation symbols (excluding local variables/packages) = 1549; Atlas symbols vs SCIP navigation symbols = 1.01x.
- JDTLS LSP benchmark: initialized against build root gson, sampled 5/5 files, 54 document symbols, 11 workspace symbols for query `Gson`, 0 diagnostics.
- Query token cost (2/2 equivalent rows): graphify 214 tokens vs Atlas 10 tokens, graphify/Atlas = 21.4x.
- Query latency (2/2 equivalent rows): graphify 210.069ms vs Atlas 28.511ms, graphify/Atlas = 7.37x.
- Java cold-build saturation: cold-vs-cold full-index ratio is 3.81x (graphify FULL 0.988s / Atlas cold 0.259s), below 5x; Atlas's largest cold phases are lexical:123ms, persist:123ms, write_sqlite:123ms.

## Derived C/C++ ratios

### c

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.359s vs graphify FULL extract 0.942s (graphify/Atlas = 2.62x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.027s vs graphify 0.514s, graphify/Atlas = 19.04x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:10ms, resolve_head:0ms.
- Atlas edge kinds: calls:4973, imports:400.
- Call coverage proxy: Atlas internal calls 1975 vs graphify calls 1018, Atlas/graphify = 1.94x.
- Atlas receiver-typed calls: 18/4973 = 0.4%.
- graphify extracted calls: 492/1018 = 48.3%.
- clangd LSP benchmark: sampled 8/8 files, 258 document symbols, 2 diagnostics.
- Query token cost (4/4 equivalent rows): graphify 1206 tokens vs Atlas 36 tokens, graphify/Atlas = 33.5x.
- Query latency (4/4 equivalent rows): graphify 421.505ms vs Atlas 57.491ms, graphify/Atlas = 7.33x.
- c cold-build saturation: cold-vs-cold full-index ratio is 2.62x (graphify FULL 0.942s / Atlas cold 0.359s), below 5x; Atlas's largest cold phases are lexical:177ms, persist:177ms, write_sqlite:177ms.
### cpp

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.314s vs graphify FULL extract 1.309s (graphify/Atlas = 4.17x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.042s vs graphify 0.82s, graphify/Atlas = 19.52x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:21ms, resolve_head:0ms.
- Atlas edge kinds: calls:9481, imports:774.
- Call coverage proxy: Atlas internal calls 6217 vs graphify calls 1195, Atlas/graphify = 5.2x.
- Atlas receiver-typed calls: 1959/9481 = 20.7%.
- graphify extracted calls: 1027/1195 = 85.9%.
- clangd LSP benchmark: sampled 8/8 files, 421 document symbols, 167 diagnostics.
- Query token cost (4/4 equivalent rows): graphify 320 tokens vs Atlas 27 tokens, graphify/Atlas = 11.85x.
- Query latency (4/4 equivalent rows): graphify 442.3ms vs Atlas 53.847ms, graphify/Atlas = 8.21x.
- cpp cold-build saturation: cold-vs-cold full-index ratio is 4.17x (graphify FULL 1.309s / Atlas cold 0.314s), below 5x; Atlas's largest cold phases are lexical:179ms, persist:179ms, write_sqlite:179ms.

## Warm query latency (persistent server)

Atlas `serve` is started against the already-indexed DB, warmed, then warm HTTP queries are timed. Raw per-call samples are preserved in the JSON (`atlas_warm_serve`). graphify has no warm/server mode, so warm Atlas is NOT divided by any graphify time; the cold-vs-cold CLI latency rows above remain the only Atlas-vs-graphify latency ratio.

| Language | Atlas warm /healthz (median ms) | Atlas warm explain (median ms) | Atlas cold-CLI explain (median ms) | warm speedup (cold/warm) |
|---|--:|--:|--:|--:|
| go | 0.441 | 0.925 | 12.844 | 13.89x |
| python | 0.385 | 1.215 | 12.979 | 10.68x |
| javascript | 0.426 | 0.845 | 13.045 | 15.44x |
| typescript | 0.435 | 0.643 | 12.962 | 20.16x |
| java | 0.292 | 5.39 | 14.256 | 2.64x |
| c | 0.345 | 1.272 | 14.394 | 11.32x |
| cpp | 0.298 | 1.581 | 13.484 | 8.53x |

- go warm-vs-warm context: both Atlas `serve` and gopls run as persistent daemons. Atlas warm explain median is 0.925ms and warm /healthz is 0.441ms. gopls's steady-state per-request latency is measured separately in its LSP benchmark (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.
- python warm-vs-warm context: both Atlas `serve` and pyright run as persistent daemons. Atlas warm explain median is 1.215ms and warm /healthz is 0.385ms. pyright's steady-state per-request latency is measured separately in its LSP benchmark (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.
- java warm-vs-warm context: both Atlas `serve` and jdtls run as persistent daemons. Atlas warm explain median is 5.39ms and warm /healthz is 0.292ms. jdtls's steady-state per-request latency is measured separately in its LSP benchmark (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.
- c warm-vs-warm context: both Atlas `serve` and clangd run as persistent daemons. Atlas warm explain median is 1.272ms and warm /healthz is 0.345ms. clangd's steady-state per-request latency is measured separately in its LSP benchmark (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.
- cpp warm-vs-warm context: both Atlas `serve` and clangd run as persistent daemons. Atlas warm explain median is 1.581ms and warm /healthz is 0.298ms. clangd's steady-state per-request latency is measured separately in its LSP benchmark (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.


## Query token probes

### go

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| log | equivalent | 105 | 4 | 83.104 | 12.978 |
| newEntry | equivalent | 62 | 5 | 82.474 | 12.73 |
| releaseEntry | equivalent | 175 | 6 | 82.663 | 12.871 |
| Fire | equivalent | 56 | 6 | 82.76 | 12.817 |

### python

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| get | equivalent | 142 | 4 | 85.456 | 12.78 |
| request | equivalent | 185 | 5 | 86.179 | 13.016 |
| __init__ | equivalent | 62 | 7 | 85.367 | 12.979 |

### javascript

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| get | equivalent | 47 | 5 | 74.479 | 13.018 |
| sendFile | equivalent | 46 | 6 | 73.924 | 13.072 |
| defineGetter | equivalent | 47 | 6 | 73.954 | 12.669 |
| format | graphify_missing | 8 | 6 | 73.713 | 13.125 |

### typescript

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| DevtoolsImpl | equivalent | 53 | 7 | 75.73 | 12.884 |
| hydrate | graphify_missing | 8 | 5 | 75.148 | 13.112 |
| shallow | equivalent | 75 | 5 | 78.56 | 12.776 |
| CreateStore | equivalent | 89 | 6 | 76.155 | 13.04 |

### java

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| write | equivalent | 112 | 5 | 104.499 | 14.07 |
| read | equivalent | 102 | 5 | 105.57 | 14.441 |

### c

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| cJSON_Delete | equivalent | 284 | 6 | 104.936 | 14.386 |
| cjson_functions_should_not_crash_with_null_pointers | equivalent | 288 | 17 | 105.44 | 14.419 |
| cJSON_CreateObject | equivalent | 332 | 8 | 105.83 | 14.402 |
| UnityPrint | equivalent | 302 | 5 | 105.299 | 14.284 |

### cpp

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| RandomString | equivalent | 76 | 7 | 108.102 | 13.113 |
| MemEnvTest | equivalent | 100 | 8 | 109.15 | 13.555 |
| Size | equivalent | 72 | 7 | 112.401 | 13.413 |
| size | equivalent | 72 | 5 | 112.647 | 13.766 |

## Missing or partial adapters


---
Generated by `bench/codeintel_matrix.py`. Raw JSON sits next to this report; logs are in `bench/logs/`.