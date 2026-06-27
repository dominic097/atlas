# Atlas code-intelligence matrix benchmark

This report benchmarks Atlas against the agreed per-language baselines. Raw metrics are kept separate by tool because Atlas, graphify, SCIP, and LSP servers expose different surfaces.

## graphify language discovery

- Installed graphify: graphifyy 0.8.49 (`/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify`).
- Runtime Python: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/python3`.
- Source inspected: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify`.
- Extract source: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify/extract.py`.
- Detect source: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify/detect.py`.
- Evidence: CLI help from `graphify --help` did not enumerate languages, but confirmed `update`, `extract`, and code-only AST update commands.
- Evidence: `graphify.detect.CODE_EXTENSIONS` plus a runtime `detect()` smoke listed code extensions.
- Evidence: `graphify.extract._DISPATCH` provided the deterministic extractor map used as the parser-parity target.
- Runtime help probe: `graphify --help` succeeded and listed 148 command/help lines.
- Runtime support probe: `_DISPATCH` plus filename-special extractors exposed 89 deterministic extractor entries; `CODE_EXTENSIONS` exposed 88 code extensions.
- Detector-only code extensions in this graphify build, not counted as deterministic parser support because `_DISPATCH` has no extractor for them: `.ejs`, `.ets`, `.r`.

| graphify family | extensions / special cases | graphify extractor | Atlas status |
|---|---|---|---|
| go | `.go` | `extract_go` | native go/parser + go/types |
| python | `.py` | `extract_python` | tree-sitter |
| javascript | `.js .jsx .mjs` | `extract_js` | tree-sitter |
| typescript | `.ts .tsx` | `extract_js` | tree-sitter |
| java | `.java` | `extract_java` | tree-sitter |
| groovy/gradle | `.groovy .gradle` | `extract_groovy` | lightweight regex |
| c | `.c .h` | `extract_c` | tree-sitter |
| cpp/cuda | `.cpp .cc .cxx .hpp .cu .cuh` | `extract_cpp` | tree-sitter |
| csharp | `.cs` | `extract_csharp` | lightweight regex |
| rust | `.rs` | `extract_rust` | lightweight regex |
| ruby | `.rb` | `extract_ruby` | lightweight regex |
| kotlin | `.kt .kts` | `extract_kotlin` | lightweight regex |
| scala | `.scala` | `extract_scala` | lightweight regex |
| php | `.php` | `extract_php` | lightweight regex |
| blade | `*.blade.php` | `extract_blade` | lightweight regex |
| swift | `.swift` | `extract_swift` | lightweight regex |
| lua | `.lua .luau .toc` | `extract_lua` | lightweight regex |
| zig | `.zig` | `extract_zig` | lightweight regex |
| powershell | `.ps1 .psm1 .psd1` | `extract_powershell/extract_powershell_manifest` | lightweight regex |
| elixir | `.ex .exs` | `extract_elixir` | lightweight regex |
| objective-c | `.m .mm` | `extract_objc` | lightweight regex |
| julia | `.jl` | `extract_julia` | lightweight regex |
| fortran | `.f .F .f90 .F90 .f95 .F95 .f03 .F03 .f08 .F08` | `extract_fortran` | lightweight regex |
| dart | `.dart` | `extract_dart` | lightweight regex |
| verilog/systemverilog | `.v .sv .svh` | `extract_verilog` | lightweight regex |
| sql | `.sql` | `extract_sql` | lightweight regex |
| markdown | `.md .mdx .qmd` | `extract_markdown` | document parser |
| pascal | `.pas .pp .dpr .dpk .lpr .inc` | `extract_pascal` | lightweight regex |
| delphi/lazarus forms | `.dfm .lfm .lpk` | `extract_delphi_form/extract_lazarus_form/extract_lazarus_package` | lightweight regex/document fallback |
| shell | `.sh .bash` | `extract_bash` | lightweight regex |
| json config | `.json` | `extract_json` | document parser |
| terraform/hcl | `.tf .tfvars .hcl` | `extract_terraform` | lightweight regex |
| byond dm | `.dm .dme .dmi .dmm .dmf` | `extract_dm/extract_dmf/extract_dmi/extract_dmm` | lightweight regex/document fallback |
| dotnet project | `.sln .slnx .csproj .fsproj .vbproj` | `extract_csproj/extract_sln/extract_slnx` | lightweight regex |
| razor | `.razor .cshtml` | `extract_razor` | lightweight regex |
| apex | `.cls .trigger` | `extract_apex` | lightweight regex |
| vue | `.vue` | `extract_js` | lightweight regex |
| svelte | `.svelte` | `extract_svelte` | lightweight regex |
| astro | `.astro` | `extract_astro` | lightweight regex |

## Live additional-language smokes

### Bash

Raw artifact: `bench/LIVE_BASH_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/nvm-sh/nvm` at commit `a6ec73943099a86fba98bde3b04a1c60944a4549`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-bash-nvm/atlas.db --json index /tmp/atlas-live-bash-nvm/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-bash-nvm/atlas.db --json index /tmp/atlas-live-bash-nvm/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/bin/bash -n <5 shell files>`

Results:

- Atlas indexed 64 files, 468 symbols, and 102 edges in 0.215s cold; no-change reindex was 0.021s (`mode=noop`).
- Atlas language counts were `text:18`, `yaml:18`, `markdown:12`, `json:6`, `bash:5`, `config:2`, `dockerfile:1`, `javascript:1`, `makefile:1`.
- graphify rebuilt 531 nodes and 807 links in 0.645s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `bash-n` status: ok (files:5, parsed_files:5, syntax_errors:0, functions:158, source_edges:0, definitions:158, bash_version:GNU bash, version 3.2.57(1)-release (arm64-apple-darwin25)).
- Richer native baselines not available on this machine: `shellcheck`, `shfmt`.
- Coverage proxy: atlas_vs_bash_n_definition_ratio: 1.0, atlas_bash_definition_symbols: 158, native_definitions: 158.
- Optimization cycles: 1 (Bash live smoke met the current 5x latency/token thresholds and matched the /bin/bash -n function-definition coverage proxy on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `nvm` | 13.881 | 97.326 | 7.01x | 4 | 269 | 67.25x |
| `nvm_install_binary` | 13.089 | 94.474 | 7.22x | 8 | 87 | 10.88x |
| `nvm_die_on_prefix` | 13.390 | 97.377 | 7.27x | 8 | 90 | 11.25x |
| `nvm_get_os` | 13.016 | 99.984 | 7.68x | 6 | 52 | 8.67x |

5x note: this Bash smoke meets the 5x threshold on equivalent query rows for latency (7.29x) and token output (19.15x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### C#

Raw artifact: `bench/LIVE_CSHARP_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/DapperLib/Dapper` at commit `72a54c475f75e18cb93cba0809d00a5e6e49efd9`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-csharp-dapper/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-csharp-dapper/atlas.db --json index /tmp/atlas-live-csharp-dapper/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-csharp-dapper/atlas.db --json index /tmp/atlas-live-csharp-dapper/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`

Results:

- Atlas indexed 207 files, 3619 symbols, and 8244 edges in 0.756s cold; no-change reindex was 0.029s (`mode=noop`).
- Atlas language counts were `csharp:157`, `text:15`, `dotnet:12`, `markdown:8`, `yaml:6`, `json:4`, `xml:2`, `batch:1`, `config:1`, `powershell:1`.
- graphify rebuilt 2463 nodes and 5135 links in 1.598s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `roslyn` status: missing.
- Richer native baselines not available on this machine: `dotnet`, `csc`, `omnisharp`, `csharp-ls`.
- Coverage proxy: atlas_vs_roslyn_definition_ratio: n/a, atlas_csharp_definition_symbols: 3419, native_definitions: 0.
- Optimization cycles: 2 (C# live smoke improves Atlas type/method recall on real Dapper code and records Atlas/graphify query metrics; native Roslyn/OmniSharp coverage remains pending because C# tooling is not installed on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `SqlMapper` | 17.785 | 132.377 | 7.44x | 10 | 296 | 29.6x |
| `CommandDefinition` | 20.273 | 131.104 | 6.47x | 13 | 293 | 22.54x |
| `DynamicParameters` | 18.260 | 130.765 | 7.16x | 18 | 69 | 3.83x |
| `TypeHandlerCache` | 15.133 | 128.136 | 8.47x | 13 | 106 | 8.15x |

5x note: this C# smoke meets the 5x threshold on equivalent query rows for latency (7.31x) and token output (14.15x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Dart

Raw artifact: `bench/LIVE_DART_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/dart-lang/http` at commit `5d94ef52582867e077bf41c3fa20fb8b1d1d834e`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-dart-http/repo/pkgs/http/lib`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-dart-http/atlas.db --json index /tmp/atlas-live-dart-http/repo/pkgs/http/lib`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-dart-http/atlas.db --json index /tmp/atlas-live-dart-http/repo/pkgs/http/lib`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-dart-http/tree-sitter-dart-venv/bin/python -c <tree-sitter-dart definition counter> /tmp/atlas-live-dart-http/repo/pkgs/http/lib`

Results:

- Atlas indexed 27 files, 184 symbols, and 565 edges in 0.155s cold; no-change reindex was 0.022s (`mode=noop`).
- Atlas language counts were `dart:27`.
- graphify rebuilt 314 nodes and 424 links in 0.566s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-dart` status: ok (files:27, parsed_files:27, parse_errors:0, definitions:180, tree_sitter_version:0.25.2, tree_sitter_dart_version:0.1.0).
- Richer native baselines not available on this machine: `dart`, `flutter`, `dart_language_server`.
- Coverage proxy: atlas_vs_tree_sitter_dart_definition_ratio: 1.0, atlas_dart_definition_symbols: 180, native_definitions: 180.
- Optimization cycles: 2 (Dart live smoke met the current 5x latency/token thresholds and matched the tree-sitter-dart definition coverage proxy after replacing generic regex parsing with a Dart-specific signature scanner.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Client` | 13.254 | 107.397 | 8.1x | 8 | 42 | 5.25x |
| `BaseClient` | 12.450 | 97.510 | 7.83x | 8 | 101 | 12.62x |
| `Request` | 12.854 | 88.300 | 6.87x | 8 | 51 | 6.38x |
| `Response` | 13.009 | 88.326 | 6.79x | 9 | 59 | 6.56x |
| `send` | 13.908 | 97.893 | 7.04x | 8 | 42 | 5.25x |
| `RetryClient` | 14.580 | 88.046 | 6.04x | 8 | 55 | 6.88x |

5x note: this Dart smoke meets the 5x threshold on equivalent query rows for latency (7.09x) and token output (7.14x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Elixir

Raw artifact: `bench/LIVE_ELIXIR_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/phoenixframework/phoenix` at commit `9d3f1f63e00c92aafc8f015073969a2632c6879a`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-elixir-phoenix/repo/lib`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-elixir-phoenix/atlas.db --json index /tmp/atlas-live-elixir-phoenix/repo/lib`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-elixir-phoenix/atlas.db --json index /tmp/atlas-live-elixir-phoenix/repo/lib`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-elixir-phoenix/tree-sitter-elixir-venv/bin/python -c <tree-sitter-elixir definition counter> /tmp/atlas-live-elixir-phoenix/repo/lib`

Results:

- Atlas indexed 74 files, 1642 symbols, and 4834 edges in 0.582s cold; no-change reindex was 0.037s (`mode=noop`).
- Atlas language counts were `elixir:74`.
- graphify rebuilt 1051 nodes and 1844 links in 1.093s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-elixir` status: ok (files:74, parsed_files:74, parse_errors:0, definitions:1642, tree_sitter_version:0.25.2, tree_sitter_elixir_version:0.3.5).
- Richer native baselines not available on this machine: `elixir`, `mix`, `lexical`.
- Coverage proxy: atlas_vs_tree_sitter_elixir_definition_ratio: 1.0, atlas_elixir_definition_symbols: 1642, native_definitions: 1642.
- Optimization cycles: 3 (Elixir live smoke matched the tree-sitter-elixir definition coverage proxy exactly and met the current 5x latency/token thresholds; elixir/mix/Lexical remain unavailable on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Phoenix.Router` | 87.826 | 212.843 | 2.42x | 7 | 260 | 37.14x |
| `Phoenix.Endpoint` | 24.936 | 191.014 | 7.66x | 9 | 186 | 20.67x |
| `Phoenix.Controller` | 20.626 | 195.087 | 9.46x | 9 | 275 | 30.56x |
| `path` | 23.902 | 160.175 | 6.7x | 8 | 70 | 8.75x |
| `socket` | 21.094 | 155.795 | 7.39x | 8 | 58 | 7.25x |

5x note: this Elixir smoke meets the 5x threshold on equivalent query rows for latency (5.13x) and token output (20.71x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Fortran

Raw artifact: `bench/LIVE_FORTRAN_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/fortran-lang/stdlib` at commit `4c8521d5658455a576946cca3bfe2bd8ede36e24`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-fortran-stdlib/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-fortran-stdlib/atlas.db --json index /tmp/atlas-live-fortran-stdlib/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-fortran-stdlib/atlas.db --json index /tmp/atlas-live-fortran-stdlib/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-fortran-stdlib/tree-sitter-fortran-venv/bin/python -c <tree-sitter-fortran definition counter> /tmp/atlas-live-fortran-stdlib/repo/src`

Results:

- Atlas indexed 53 files, 414 symbols, and 4543 edges in 0.152s cold; no-change reindex was 0.025s (`mode=noop`).
- Atlas language counts were `text:29`, `fortran:22`, `c:2`.
- graphify rebuilt 453 nodes and 815 links in 0.581s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-fortran` status: ok (files:22, parsed_files:22, parse_errors:0, definitions:364, tree_sitter_version:0.25.2, tree_sitter_fortran_version:0.6.0).
- Richer native baselines not available on this machine: `gfortran`, `fortls`, `fpm`.
- Coverage proxy: atlas_vs_tree_sitter_fortran_definition_ratio: 1.0, atlas_fortran_definition_symbols: 364, native_definitions: 364.
- Optimization cycles: 2 (Fortran live smoke matched the tree-sitter-fortran definition coverage proxy and met the current 5x latency/token thresholds after widening Atlas Fortran declaration handling.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `stdlib_array` | 12.124 | 78.484 | 6.47x | 8 | 83 | 10.38x |
| `stdlib_datetime` | 12.039 | 78.108 | 6.49x | 10 | 253 | 25.3x |
| `datetime_type` | 11.724 | 76.094 | 6.49x | 10 | 267 | 26.7x |
| `hashmap_type` | 11.971 | 75.471 | 6.3x | 9 | 292 | 32.44x |
| `loading` | 11.721 | 75.730 | 6.46x | 8 | 65 | 8.12x |
| `free_chaining_map` | 11.813 | 75.815 | 6.42x | 15 | 103 | 6.87x |

5x note: this Fortran smoke meets the 5x threshold on equivalent query rows for latency (6.44x) and token output (17.72x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Groovy/Gradle

Raw artifact: `bench/LIVE_GROOVY_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/nextflow-io/nextflow` at commit `83d452a51796aca34f136f796383185d703c349c`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-groovy-nextflow/repo/modules/nf-commons/src/main`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-groovy-nextflow/atlas.db --json index /tmp/atlas-live-groovy-nextflow/repo/modules/nf-commons/src/main`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-groovy-nextflow/atlas.db --json index /tmp/atlas-live-groovy-nextflow/repo/modules/nf-commons/src/main`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-groovy-nextflow/tree-sitter-groovy-venv/bin/python -c <tree-sitter-groovy definition counter> /tmp/atlas-live-groovy-nextflow/repo/modules/nf-commons/src/main`

Results:

- Atlas indexed 100 files, 1049 symbols, and 3712 edges in 0.347s cold; no-change reindex was 0.035s (`mode=noop`).
- Atlas language counts were `groovy:88`, `java:12`.
- graphify rebuilt 742 nodes and 1227 links in 1.174s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-groovy` status: partial (files:88, parsed_files:21, parse_errors:67, definitions:525, tree_sitter_version:0.25.2, tree_sitter_groovy_version:0.1.2).
- Richer native baselines not available on this machine: `groovy`, `gradle`, `groovy-language-server`.
- Coverage proxy: atlas_vs_tree_sitter_groovy_definition_ratio: 1.59, atlas_groovy_definition_symbols: 836, native_definitions: 525.
- Optimization cycles: 3 (Groovy/Gradle live smoke met the current 5x latency/token thresholds after widening Atlas Groovy declaration handling; definition coverage is saturated by tree-sitter-groovy parse errors on real Nextflow files, so the 1.59x native ratio is not claimed as exact recall.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `SysEnv` | 13.724 | 97.614 | 7.11x | 7 | 125 | 17.86x |
| `Const` | 13.759 | 96.585 | 7.02x | 6 | 77 | 12.83x |
| `Duration` | 13.798 | 101.710 | 7.37x | 10 | 36 | 3.6x |
| `getVersion` | 14.396 | 94.221 | 6.54x | 9 | 64 | 7.11x |
| `format` | 13.870 | 94.569 | 6.82x | 8 | 97 | 12.12x |

5x note: this Groovy/Gradle smoke meets the 5x threshold on equivalent query rows for latency (6.97x) and token output (9.97x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Julia

Raw artifact: `bench/LIVE_JULIA_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/JuliaIO/JSON.jl` at commit `e5ef310dece16746843753e4c3b44e868b917b64`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-julia-json/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-julia-json/atlas.db --json index /tmp/atlas-live-julia-json/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-julia-json/atlas.db --json index /tmp/atlas-live-julia-json/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-julia-json/tree-sitter-julia-venv/bin/python -c <tree-sitter-julia definition counter> /tmp/atlas-live-julia-json/repo/src`

Results:

- Atlas indexed 6 files, 310 symbols, and 1047 edges in 0.13s cold; no-change reindex was 0.022s (`mode=noop`).
- Atlas language counts were `julia:6`.
- graphify rebuilt 114 nodes and 179 links in 0.165s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-julia` status: partial (files:6, parsed_files:5, parse_errors:1, definitions:310, tree_sitter_version:0.25.2, tree_sitter_julia_version:0.23.1).
- Richer native baselines not available on this machine: `julia`.
- Coverage proxy: atlas_vs_tree_sitter_julia_definition_ratio: 1.0, atlas_julia_definition_symbols: 310, native_definitions: 310.
- Optimization cycles: 2 (Julia live smoke met the current 5x latency/token thresholds and matched the tree-sitter-julia definition coverage proxy after masking docstrings/strings and supporting macro-prefixed structs.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `JSON` | 12.086 | 70.196 | 5.81x | 4 | 38 | 9.5x |
| `JSONText` | 11.494 | 78.972 | 6.87x | 7 | 51 | 7.29x |
| `Object` | 11.927 | 73.096 | 6.13x | 8 | 75 | 9.38x |
| `parse` | 11.946 | 71.937 | 6.02x | 7 | 167 | 23.86x |
| `json` | 11.853 | 71.125 | 6.0x | 7 | 38 | 5.43x |
| `LazyValue` | 11.257 | 70.828 | 6.29x | 6 | 9 | 1.5x |

- Query caveat: `LazyValue` (graphify_missing); raw rows remain in the table.
5x note: this Julia smoke meets the 5x threshold on equivalent query rows for latency (6.16x) and token output (11.18x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Kotlin

Raw artifact: `bench/LIVE_KOTLIN_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/square/okhttp` at commit `62ce184cf11abf9a42c743ca32781dac875aa413`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-kotlin-okhttp/repo/okhttp/src/commonJvmAndroid/kotlin`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-kotlin-okhttp/atlas.db --json index /tmp/atlas-live-kotlin-okhttp/repo/okhttp/src/commonJvmAndroid/kotlin`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-kotlin-okhttp/atlas.db --json index /tmp/atlas-live-kotlin-okhttp/repo/okhttp/src/commonJvmAndroid/kotlin`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-kotlin-okhttp/tree-sitter-kotlin-venv/bin/python -c <tree-sitter-kotlin definition counter> /tmp/atlas-live-kotlin-okhttp/repo/okhttp/src/commonJvmAndroid/kotlin`

Results:

- Atlas indexed 138 files, 3912 symbols, and 7554 edges in 0.624s cold; no-change reindex was 0.029s (`mode=noop`).
- Atlas language counts were `kotlin:138`.
- graphify rebuilt 1844 nodes and 5202 links in 1.252s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-kotlin` status: partial (files:138, parsed_files:136, parse_errors:2, definitions:3876, tree_sitter_version:0.25.2, tree_sitter_kotlin_version:1.1.0).
- Richer native baselines not available on this machine: `kotlinc`, `kotlin-language-server`, `ktlint`.
- Coverage proxy: atlas_vs_tree_sitter_kotlin_definition_ratio: 1.01, atlas_kotlin_definition_symbols: 3912, native_definitions: 3876.
- Optimization cycles: 2 (Kotlin live smoke met the current 5x latency/token thresholds and exceeded the tree-sitter-kotlin definition coverage proxy after widening Atlas Kotlin modifier handling.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `OkHttpClient` | 16.752 | 120.261 | 7.18x | 9 | 255 | 28.33x |
| `Request` | 16.048 | 118.772 | 7.4x | 6 | 56 | 9.33x |
| `Response` | 16.572 | 123.273 | 7.44x | 7 | 97 | 13.86x |
| `HttpUrl` | 17.182 | 119.189 | 6.94x | 7 | 239 | 34.14x |
| `Headers` | 16.959 | 119.478 | 7.05x | 6 | 246 | 41.0x |

5x note: this Kotlin smoke meets the 5x threshold on equivalent query rows for latency (7.2x) and token output (25.51x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Lua

Raw artifact: `bench/LIVE_LUA_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/folke/lazy.nvim` at commit `306a05526ada86a7b30af95c5cc81ffba93fef97`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-lua-lazy/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-lua-lazy/atlas.db --json index /tmp/atlas-live-lua-lazy/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-lua-lazy/atlas.db --json index /tmp/atlas-live-lua-lazy/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-lua-lazy/luaparser-venv/bin/python -c <luaparser definition counter> /tmp/atlas-live-lua-lazy/repo`

Results:

- Atlas indexed 90 files, 5211 symbols, and 3929 edges in 0.907s cold; no-change reindex was 0.026s (`mode=noop`).
- Atlas language counts were `lua:65`, `yaml:14`, `markdown:4`, `json:3`, `toml:2`, `config:1`, `text:1`.
- graphify rebuilt 1011 nodes and 1288 links in 0.591s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `luaparser` status: ok (files:65, parsed_files:65, parse_errors:0, definitions:444, luaparser_version:4.0.1).
- Richer native baselines not available on this machine: `lua`, `luac`, `luacheck`, `stylua`.
- Coverage proxy: atlas_vs_luaparser_definition_ratio: 1.24, atlas_lua_definition_symbols: 551, native_definitions: 444.
- Optimization cycles: 1 (Lua live smoke exceeded 5x query latency plus token output on equivalent function-symbol queries and exceeded the luaparser named-definition coverage proxy on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Loader.load` | 19.175 | 102.155 | 5.33x | 7 | 146 | 20.86x |
| `Async.new` | 15.424 | 104.809 | 6.8x | 6 | 58 | 9.67x |
| `M.add` | 16.059 | 103.875 | 6.47x | 6 | 65 | 10.83x |
| `M.reload` | 16.280 | 101.722 | 6.25x | 8 | 78 | 9.75x |

5x note: this Lua smoke meets the 5x threshold on equivalent query rows for latency (6.16x) and token output (12.85x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Objective-C

Raw artifact: `bench/LIVE_OBJC_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/SDWebImage/SDWebImage` at commit `c3ad5e1a9bf55c9b76d4c362430b5fcded96c502`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-objc-sdwebimage/repo/SDWebImage`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-objc-sdwebimage/atlas.db --json index /tmp/atlas-live-objc-sdwebimage/repo/SDWebImage`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-objc-sdwebimage/atlas.db --json index /tmp/atlas-live-objc-sdwebimage/repo/SDWebImage`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-objc-sdwebimage/tree-sitter-objc-venv/bin/python -c <tree-sitter-objc definition counter> /tmp/atlas-live-objc-sdwebimage/repo/SDWebImage`

Results:

- Atlas indexed 147 files, 975 symbols, and 1742 edges in 0.255s cold; no-change reindex was 0.037s (`mode=noop`).
- Atlas language counts were `c:75`, `objc:72`.
- graphify rebuilt 1101 nodes and 1009 links in 0.735s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-objc` status: partial (files:72, parsed_files:67, parse_errors:5, definitions:971, tree_sitter_version:0.25.2, tree_sitter_objc_version:3.0.2).
- Coverage proxy: atlas_vs_tree_sitter_objc_definition_ratio: 1.0, atlas_objc_definition_symbols: 973, native_definitions: 971.
- Optimization cycles: 2 (Objective-C live smoke matched the graphify-scoped tree-sitter-objc definition coverage proxy and exceeded 5x latency/token thresholds on equivalent rows after preserving full multi-part selectors in Atlas.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `SDImageCache` | 12.308 | 99.109 | 8.05x | 9 | 300 | 33.33x |
| `SDWebImageManager` | 14.063 | 109.113 | 7.76x | 11 | 414 | 37.64x |
| `sharedImageCache` | 13.844 | 95.012 | 6.86x | 9 | 57 | 6.33x |
| `storeImage:forKey:completion:` | 11.604 | 86.381 | 7.44x | 12 | 14 | 1.17x |
| `objectForKey:` | 11.856 | 87.341 | 7.37x | 8 | 56 | 7.0x |

- Query caveat: `storeImage:forKey:completion:` (graphify_missing); raw rows remain in the table.
5x note: this Objective-C smoke meets the 5x threshold on equivalent query rows for latency (7.5x) and token output (22.35x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### PHP

Raw artifact: `bench/LIVE_PHP_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/slimphp/Slim` at commit `0da7dd2fc66956730b6633f6a056b35e59126583`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-php-slim/atlas.db --json index /tmp/atlas-live-php-slim/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-php-slim/atlas.db --json index /tmp/atlas-live-php-slim/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/php /tmp/atlas-live-php-slim/php_token_stats.php /tmp/atlas-live-php-slim/repo`

Results:

- Atlas indexed 140 files, 1080 symbols, and 6546 edges in 0.351s cold; no-change reindex was 0.024s (`mode=noop`).
- Atlas language counts were `php:125`, `markdown:8`, `yaml:4`, `config:1`, `json:1`, `xml:1`.
- graphify rebuilt 1152 nodes and 2115 links in 0.743s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `php-tokenizer` status: ok (files:125, parsed_files:125, parse_errors:0, classes:106, interfaces:18, traits:0, enums:0, functions:742, requires:1, namespaces:124, uses:881, use_functions:163, php_version:8.4.14, definitions:866).
- Richer native baselines not available on this machine: `intelephense`, `phpstan`, `psalm`.
- Coverage proxy: atlas_vs_php_tokenizer_definition_ratio: 1.0, atlas_php_definition_symbols: 866, native_definitions: 866.
- Optimization cycles: 2 (PHP live smoke met the current 5x latency/token thresholds and matched PHP tokenizer definition coverage after separating `use function` imports from real function definitions in the native baseline.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `handle` | 16.464 | 108.286 | 6.58x | 8 | 78 | 9.75x |
| `process` | 15.401 | 105.351 | 6.84x | 13 | 116 | 8.92x |
| `addRoute` | 13.873 | 105.176 | 7.58x | 2 | 8 | 4.0x |
| `getResponseFactory` | 15.801 | 105.729 | 6.69x | 13 | 77 | 5.92x |

- Query caveat: `addRoute` (graphify_missing); raw rows remain in the table.
5x note: this PHP smoke meets the 5x threshold on equivalent query rows for latency (6.7x) and token output (7.97x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### PowerShell

Raw artifact: `bench/LIVE_POWERSHELL_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/PowerShell/PowerShellGet` at commit `a2dac8e74603f7c9eec4a54c5e23459531751b0d`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-powershell-powershellget/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-powershell-powershellget/atlas.db --json index /tmp/atlas-live-powershell-powershellget/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-powershell-powershellget/atlas.db --json index /tmp/atlas-live-powershell-powershellget/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/usr/local/bin/pwsh -NoLogo -NoProfile -File /tmp/atlas-live-powershell-powershellget/pwsh_stats.ps1 /tmp/atlas-live-powershell-powershellget/repo/src`

Results:

- Atlas indexed 2 files, 159 symbols, and 805 edges in 0.123s cold; no-change reindex was 0.026s (`mode=noop`).
- Atlas language counts were `powershell:2`.
- graphify rebuilt 30 nodes and 43 links in 0.241s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `pwsh-parser` status: ok (files:2, parsed_files:2, parse_errors:0, functions:28, assignments:391, definitions:28, powershell_version:7.4.6).
- Richer native baselines not available on this machine: `powershell-editor-services`, `psscriptanalyzer`.
- Coverage proxy: atlas_vs_pwsh_parser_definition_ratio: 1.0, atlas_powershell_definition_symbols: 28, native_definitions: 28.
- Optimization cycles: 1 (PowerShell live smoke met the current 5x latency/token thresholds and matched pwsh AST function-definition coverage on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Find-Module` | 12.850 | 87.926 | 6.84x | 9 | 66 | 7.33x |
| `Install-Module` | 13.026 | 90.585 | 6.95x | 10 | 67 | 6.7x |
| `Register-PSRepository` | 13.272 | 86.944 | 6.55x | 12 | 57 | 4.75x |
| `Update-ModuleManifest` | 12.793 | 85.859 | 6.71x | 12 | 57 | 4.75x |

5x note: this PowerShell smoke meets the 5x threshold on equivalent query rows for latency (6.76x) and token output (5.74x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Ruby

Raw artifact: `bench/LIVE_RUBY_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/sinatra/sinatra` at commit `5236d3459b8b9015e5ce21ddd0c6beb0db4081d4`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-ruby-sinatra/atlas.db --json index /tmp/atlas-live-ruby-sinatra/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-ruby-sinatra/atlas.db --json index /tmp/atlas-live-ruby-sinatra/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/usr/bin/ruby /tmp/atlas-live-ruby-sinatra/ripper_stats.rb /tmp/atlas-live-ruby-sinatra/repo`

Results:

- Atlas indexed 178 files, 1506 symbols, and 3970 edges in 0.342s cold; no-change reindex was 0.02s (`mode=noop`).
- Atlas language counts were `ruby:147`, `markdown:12`, `yaml:10`, `css:4`, `config:3`, `text:2`.
- graphify rebuilt 1281 nodes and 1869 links in 0.866s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `ruby-ripper` status: ok (files:147, parsed_files:147, parse_errors:0, classes:131, modules:138, methods:895, requires:0, ruby_version:2.6.10, definitions:1164).
- Richer native baselines not available on this machine: `solargraph`, `ruby-lsp`.
- Coverage proxy: atlas_vs_ripper_definition_ratio: 1.01, atlas_ruby_definition_symbols: 1173, ripper_definitions: 1164.
- Optimization cycles: 2 (Ruby live smoke met the current 5x latency/token thresholds and the Ripper definition coverage proxy after adding operator, receiver-qualified, and ::-qualified module/class parsing.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `initialize` | 16.316 | 111.241 | 6.82x | 8 | 61 | 7.62x |
| `call` | 16.018 | 103.485 | 6.46x | 8 | 89 | 11.12x |
| `route` | 13.919 | 103.783 | 7.46x | 7 | 96 | 13.71x |
| `settings` | 13.857 | 102.544 | 7.4x | 7 | 58 | 8.29x |

5x note: this Ruby smoke meets the 5x threshold on equivalent query rows for latency (7.0x) and token output (10.13x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Rust

Raw artifact: `bench/LIVE_RUST_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/BurntSushi/ripgrep` at commit `dfe4a81d2591daca76d25ae4e052c34b26578155`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-rust-ripgrep/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-rust-ripgrep/atlas.db --json index /tmp/atlas-live-rust-ripgrep/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-rust-ripgrep/atlas.db --json index /tmp/atlas-live-rust-ripgrep/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`

Results:

- Atlas indexed 161 files, 3440 symbols, and 17263 edges in 0.827s cold; no-change reindex was 0.038s (`mode=noop`).
- Atlas language counts were `rust:100`, `markdown:22`, `toml:13`, `csv:12`, `yaml:5`, `config:4`, `bash:2`, `ruby:1`, `text:1`, `xml:1`.
- graphify rebuilt 3457 nodes and 9148 links in 2.04s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `rust-analyzer` status: missing.
- Richer native baselines not available on this machine: `rust-analyzer`, `cargo`, `rustc`.
- Coverage proxy: atlas_vs_rust_analyzer_definition_ratio: n/a, atlas_rust_definition_symbols: 3299, native_definitions: 0.
- Optimization cycles: 1 (Rust live smoke restores reproducible Atlas/graphify measurements and exceeds 5x query latency plus token output on comparable type-symbol queries; native rust-analyzer coverage remains pending because Rust tooling is not installed on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `HiArgs` | 15.372 | 140.173 | 9.12x | 5 | 249 | 49.8x |
| `LowArgs` | 14.529 | 138.169 | 9.51x | 6 | 258 | 43.0x |
| `PatternMatcher` | 15.477 | 139.596 | 9.02x | 8 | 119 | 14.88x |
| `WalkBuilder` | 14.344 | 139.598 | 9.73x | 7 | 259 | 37.0x |

5x note: this Rust smoke meets the 5x threshold on equivalent query rows for latency (9.34x) and token output (34.04x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Scala

Raw artifact: `bench/LIVE_SCALA_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/typelevel/cats` at commit `851965a582940d804f9a23179f58a53fc97f07dc`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-scala-cats/repo/core/src/main/scala`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-scala-cats/atlas.db --json index /tmp/atlas-live-scala-cats/repo/core/src/main/scala`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-scala-cats/atlas.db --json index /tmp/atlas-live-scala-cats/repo/core/src/main/scala`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-scala-cats/tree-sitter-scala-venv/bin/python -c <tree-sitter-scala definition counter> /tmp/atlas-live-scala-cats/repo/core/src/main/scala`

Results:

- Atlas indexed 206 files, 7859 symbols, and 9436 edges in 1.776s cold; no-change reindex was 0.028s (`mode=noop`).
- Atlas language counts were `scala:206`.
- graphify rebuilt 4796 nodes and 18787 links in 2.906s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-scala` status: partial (files:206, parsed_files:204, parse_errors:2, definitions:7840, tree_sitter_version:0.25.2, tree_sitter_scala_version:0.26.0).
- Richer native baselines not available on this machine: `metals`, `scalac`, `scala-cli`.
- Coverage proxy: atlas_vs_tree_sitter_scala_definition_ratio: 1.0, atlas_scala_definition_symbols: 7858, native_definitions: 7840.
- Optimization cycles: 2 (Scala live smoke met the current 5x latency/token thresholds and improved Cats definition coverage after widening Atlas modifier and type-alias handling; Metals/scalac remain unavailable on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Functor` | 19.980 | 184.560 | 9.24x | 7 | 308 | 44.0x |
| `Applicative` | 19.877 | 180.650 | 9.09x | 10 | 272 | 27.2x |
| `Monad` | 22.065 | 177.156 | 8.03x | 8 | 291 | 36.38x |
| `Traverse` | 18.986 | 176.082 | 9.27x | 9 | 91 | 10.11x |
| `Eval` | 20.377 | 177.834 | 8.73x | 6 | 256 | 42.67x |

5x note: this Scala smoke meets the 5x threshold on equivalent query rows for latency (8.85x) and token output (30.45x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### SQL

Raw artifact: `bench/LIVE_SQL_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/hasura/graphql-engine` at commit `417c174c0ac3c80dafe6e8f9e8ac39d868334724`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-sql-hasura/repo/server/src-rsr/migrations`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-sql-hasura/atlas.db --json index /tmp/atlas-live-sql-hasura/repo/server/src-rsr/migrations`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-sql-hasura/atlas.db --json index /tmp/atlas-live-sql-hasura/repo/server/src-rsr/migrations`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-sql-hasura/sqlfluff-venv/bin/sqlfluff parse --dialect postgres <82 SQL files>`

Results:

- Atlas indexed 82 files, 152 symbols, and 857 edges in 0.181s cold; no-change reindex was 0.025s (`mode=noop`).
- Atlas language counts were `sql:82`.
- graphify rebuilt 240 nodes and 194 links in 0.64s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `sqlfluff` status: ok (files:82, parsed_files:82, parse_errors:0, definitions:111, sqlfluff_version:sqlfluff, version 3.5.0).
- Coverage proxy: atlas_vs_sqlfluff_definition_ratio: 1.0, atlas_sql_definition_symbols: 111, native_definitions: 111.
- Optimization cycles: 2 (SQL live smoke matched SQLFluff DDL definition coverage after installing graphify's optional tree_sitter_sql parser and exceeded 5x query latency, but token ratio saturated just below 5x on already-terse exact-symbol output.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `hdb_catalog.event_triggers` | 22.837 | 91.033 | 3.99x | 11 | 55 | 5.0x |
| `hdb_catalog.hdb_metadata` | 12.510 | 88.501 | 7.07x | 10 | 54 | 5.4x |
| `hdb_catalog.hdb_schema_update_event_notifier` | 13.476 | 91.737 | 6.81x | 17 | 80 | 4.71x |
| `hdb_catalog.hdb_function_agg` | 18.628 | 92.777 | 4.98x | 12 | 56 | 4.67x |

Saturation note: this SQL smoke proves Atlas has lower latency than graphify on these live queries (5.4x overall), but it does not prove every 5x target (token ratio 4.9x overall). Pulse should use `atlas context`/MCP with a hard token budget rather than raw `search --json` when measuring review-context token cost.

### Svelte

Raw artifact: `bench/LIVE_SVELTE_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/carbon-design-system/carbon-components-svelte` at commit `db1dfd0344296142cffea0012591e8b6fd58a78b`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-svelte-carbon/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-svelte-carbon/atlas.db --json index /tmp/atlas-live-svelte-carbon/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-svelte-carbon/atlas.db --json index /tmp/atlas-live-svelte-carbon/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/node /tmp/atlas-live-svelte-carbon/svelte-compiler/svelte_stats.js /tmp/atlas-live-svelte-carbon/repo/src`

Results:

- Atlas indexed 405 files, 1830 symbols, and 3526 edges in 0.459s cold; no-change reindex was 0.026s (`mode=noop`).
- Atlas language counts were `svelte:250`, `javascript:115`, `typescript:40`.
- graphify rebuilt 818 nodes and 1453 links in 1.689s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `svelte-compiler` status: ok (files:250, parsed_files:250, parse_errors:0, script_blocks:226, functions:264, variables:933, definitions:1197, compiler_version:5.56.4).
- Richer native baselines not available on this machine: `svelte-check`, `svelte-language-server`.
- Coverage proxy: atlas_vs_svelte_compiler_definition_ratio: 1.04, atlas_svelte_definition_symbols: 1244, native_definitions: 1197.
- Optimization cycles: 1 (Svelte live smoke exceeded 5x query latency plus token output and exceeded the Svelte compiler script-declaration coverage proxy on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `setChar` | 14.142 | 102.648 | 7.26x | 9 | 78 | 8.67x |
| `focusInput` | 14.246 | 102.845 | 7.22x | 10 | 141 | 14.1x |
| `handleInput` | 14.476 | 111.323 | 7.69x | 11 | 88 | 8.0x |
| `handleKeydown` | 14.628 | 107.046 | 7.32x | 12 | 79 | 6.58x |
| `handleOutsideClick` | 14.727 | 107.412 | 7.29x | 12 | 77 | 6.42x |

5x note: this Svelte smoke meets the 5x threshold on equivalent query rows for latency (7.36x) and token output (8.57x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Swift

Raw artifact: `bench/LIVE_SWIFT_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/apple/swift-argument-parser` at commit `8122bc5941426c9494c78ff5ad01951e81c02f53`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-swift-argument-parser/atlas.db --json index /tmp/atlas-live-swift-argument-parser/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-swift-argument-parser/atlas.db --json index /tmp/atlas-live-swift-argument-parser/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/usr/bin/sourcekit-lsp --scratch-path /tmp/atlas-live-swift-argument-parser/sourcekit-scratch --default-workspace-type swiftPM`

Results:

- Atlas indexed 237 files, 4669 symbols, and 7588 edges in 0.68s cold; no-change reindex was 0.028s (`mode=noop`).
- Atlas language counts were `swift:165`, `markdown:38`, `text:13`, `json:11`, `bash:5`, `yaml:4`, `config:1`.
- graphify rebuilt 2838 nodes and 6444 links in 1.572s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `sourcekit-lsp` status: ok (sample_files:16, document_symbol_files:16, document_symbols:209, definitions:209, diagnostic_files:0, diagnostics:0, swift_version:Apple Swift version 6.2.4 (swiftlang-6.2.4.1.4 clang-1700.6.4.2)).
- Richer native baselines not available on this machine: `swift-syntax`.
- Coverage proxy: atlas_vs_sourcekit_lsp_definition_ratio: 1.19, atlas_swift_definition_symbols: 248, native_definitions: 209, coverage_scope: 16 SourceKit-LSP sampled files.
- Optimization cycles: 1 (Swift live smoke met the current 5x latency/token thresholds and exceeded the SourceKit-LSP sampled definition coverage proxy on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `ArgumentParser` | 13.997 | 127.255 | 9.09x | 11 | 332 | 30.18x |
| `parse` | 16.153 | 132.105 | 8.18x | 11 | 139 | 12.64x |
| `run` | 16.298 | 127.956 | 7.85x | 8 | 48 | 6.0x |
| `help` | 16.573 | 127.229 | 7.68x | 8 | 44 | 5.5x |

5x note: this Swift smoke meets the 5x threshold on equivalent query rows for latency (8.16x) and token output (14.82x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Terraform/HCL

Raw artifact: `bench/LIVE_TERRAFORM_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/terraform-aws-modules/terraform-aws-vpc` at commit `3ffbd46fb1c7733e1b34d8666893280454e27436`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-terraform-vpc/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-terraform-vpc/atlas.db --json index /tmp/atlas-live-terraform-vpc/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-terraform-vpc/atlas.db --json index /tmp/atlas-live-terraform-vpc/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-terraform-vpc/hcl2-venv/bin/python -c <hcl2 definition counter> /tmp/atlas-live-terraform-vpc/repo`

Results:

- Atlas indexed 109 files, 2305 symbols, and 1078 edges in 0.472s cold; no-change reindex was 0.026s (`mode=noop`).
- Atlas language counts were `terraform:77`, `markdown:24`, `yaml:6`, `config:1`, `json:1`.
- graphify rebuilt 2461 nodes and 4736 links in 1.108s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `python-hcl2` status: ok (files:77, parsed_files:77, parse_errors:0, definitions:1738, python_hcl2_version:8.1.2).
- Richer native baselines not available on this machine: `terraform`.
- Coverage proxy: atlas_vs_python_hcl2_definition_ratio: 1.0, atlas_terraform_definition_symbols: 1738, native_definitions: 1738.
- Optimization cycles: 2 (Terraform/HCL live smoke matched python-hcl2 definition coverage and exceeded 5x query latency plus token output after installing graphify's optional tree_sitter_hcl parser.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `aws_vpc.this` | 15.705 | 130.057 | 8.28x | 6 | 291 | 48.5x |
| `aws_subnet.public` | 14.038 | 130.228 | 9.28x | 8 | 308 | 38.5x |
| `aws_route_table.public` | 13.854 | 131.116 | 9.46x | 9 | 280 | 31.11x |
| `aws_nat_gateway.this` | 17.264 | 126.076 | 7.3x | 9 | 286 | 31.78x |

5x note: this Terraform/HCL smoke meets the 5x threshold on equivalent query rows for latency (8.5x) and token output (36.41x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Verilog

Raw artifact: `bench/LIVE_VERILOG_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/lowRISC/ibex` at commit `022f084096baed0a9b5ebdf697ed2965f13e8ed8`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-verilog-ibex/repo/rtl`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-verilog-ibex/atlas.db --json index /tmp/atlas-live-verilog-ibex/repo/rtl`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-verilog-ibex/atlas.db --json index /tmp/atlas-live-verilog-ibex/repo/rtl`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-verilog-ibex/tree-sitter-systemverilog-venv/bin/python -c <tree-sitter-systemverilog definition counter> /tmp/atlas-live-verilog-ibex/repo/rtl`

Results:

- Atlas indexed 31 files, 94 symbols, and 2666 edges in 0.11s cold; no-change reindex was 0.025s (`mode=noop`).
- Atlas language counts were `verilog:30`, `fortran:1`.
- graphify rebuilt 170 nodes and 139 links in 0.385s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-systemverilog` status: partial (files:30, parsed_files:20, parse_errors:10, definitions:93, tree_sitter_version:0.25.2, tree_sitter_systemverilog_version:0.3.1).
- Richer native baselines not available on this machine: `verilator`, `slang`, `svlint`.
- Coverage proxy: atlas_vs_tree_sitter_systemverilog_definition_ratio: 1.0, atlas_verilog_definition_symbols: 93, native_definitions: 93.
- Optimization cycles: 2 (Verilog/SystemVerilog live smoke matched the tree-sitter-systemverilog definition coverage proxy and exceeded 5x overall latency/token ratios after widening Atlas declaration handling; native tree-sitter parse errors and the one below-5x `ibex_core` query row are reported in raw metrics.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `ibex_core` | 13.234 | 72.976 | 5.51x | 7 | 32 | 4.57x |
| `ibex_top` | 12.904 | 71.169 | 5.52x | 6 | 105 | 17.5x |
| `ibex_pkg` | 11.807 | 70.885 | 6.0x | 6 | 46 | 7.67x |
| `ibex_alu` | 11.907 | 70.740 | 5.94x | 6 | 55 | 9.17x |
| `cm_stack_adj_base` | 11.249 | 71.126 | 6.32x | 13 | 67 | 5.15x |
| `decode_i_insn` | 11.668 | 71.031 | 6.09x | 9 | 53 | 5.89x |

5x note: this Verilog smoke meets the 5x threshold on equivalent query rows for latency (5.88x) and token output (7.62x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Vue

Raw artifact: `bench/LIVE_VUE_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/gothinkster/vue-realworld-example-app` at commit `f7e48c8178602ce25d43293bc6f8ca51d84ae222`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-vue-realworld/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-vue-realworld/atlas.db --json index /tmp/atlas-live-vue-realworld/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-vue-realworld/atlas.db --json index /tmp/atlas-live-vue-realworld/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/node /tmp/atlas-live-vue-realworld/vue-compiler/vue_sfc_stats.js /tmp/atlas-live-vue-realworld/repo/src`

Results:

- Atlas indexed 32 files, 165 symbols, and 390 edges in 0.126s cold; no-change reindex was 0.027s (`mode=noop`).
- Atlas language counts were `vue:20`, `javascript:12`.
- graphify rebuilt 111 nodes and 91 links in 0.439s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `vue-compiler-sfc` status: ok (files:20, parsed_files:20, parse_errors:0, script_blocks:21, functions:0, variables:119, definitions:119, compiler_version:3.5.22).
- Richer native baselines not available on this machine: `vue-tsc`, `volar`.
- Coverage proxy: atlas_vs_vue_compiler_sfc_definition_ratio: 1.0, atlas_vue_definition_symbols: 119, native_definitions: 119.
- Optimization cycles: 1 (Vue live smoke matched @vue/compiler-sfc script declaration coverage and exceeded 5x query latency plus token output on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `parseMarkdown` | 13.880 | 88.956 | 6.41x | 8 | 51 | 6.38x |
| `follow` | 13.161 | 88.325 | 6.71x | 7 | 48 | 6.86x |
| `goTo` | 13.236 | 86.649 | 6.55x | 5 | 45 | 9.0x |
| `onPageChange` | 13.380 | 89.522 | 6.69x | 8 | 49 | 6.12x |

5x note: this Vue smoke meets the 5x threshold on equivalent query rows for latency (6.59x) and token output (6.89x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Zig

Raw artifact: `bench/LIVE_ZIG_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/zigtools/zls` at commit `8da87d4f3305a550e7b739bad764e34bf1e46a08`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-zig-zls/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-zig-zls/atlas.db --json index /tmp/atlas-live-zig-zls/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-zig-zls/atlas.db --json index /tmp/atlas-live-zig-zls/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-zig-zls/tree-sitter-zig-venv/bin/python -c <tree-sitter-zig definition counter> /tmp/atlas-live-zig-zls/repo/src`

Results:

- Atlas indexed 45 files, 5297 symbols, and 11835 edges in 1.077s cold; no-change reindex was 0.025s (`mode=noop`).
- Atlas language counts were `zig:43`, `json:1`, `markdown:1`.
- graphify rebuilt 1096 nodes and 2536 links in 0.685s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-zig` status: partial (files:43, parsed_files:42, parse_errors:1, definitions:5279, tree_sitter_version:0.25.2, tree_sitter_zig_version:1.1.2).
- Richer native baselines not available on this machine: `zig`, `zls`.
- Coverage proxy: atlas_vs_tree_sitter_zig_definition_ratio: 1.0, atlas_zig_definition_symbols: 5294, native_definitions: 5279.
- Optimization cycles: 2 (Zig live smoke met the current 5x latency/token thresholds and exceeded the tree-sitter-zig definition coverage proxy after widening Atlas Zig declaration handling; zig/zls remain unavailable on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Server` | 16.710 | 103.246 | 6.18x | 7 | 268 | 38.29x |
| `DocumentStore` | 17.593 | 102.641 | 5.83x | 11 | 277 | 25.18x |
| `Analyser` | 17.349 | 102.959 | 5.93x | 8 | 259 | 32.38x |
| `Config` | 17.113 | 105.624 | 6.17x | 9 | 79 | 8.78x |
| `main` | 16.903 | 102.943 | 6.09x | 9 | 202 | 22.44x |

5x note: this Zig smoke meets the 5x threshold on equivalent query rows for latency (6.04x) and token output (24.66x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

## Tool matrix

| Language | Repo | Atlas | graphify | SCIP | LSP |
|---|---|---|---|---|---|
| go | sirupsen/logrus | 676 symbols, 2085 calls, 0.022s reindex (cold 0.397s) | 707 nodes, 330 calls, 0.678s | 2214 symbols, 11799 occ, 0.257s | 12 pkgs, 0 diag, 0.411s |
| python | psf/requests | 517 symbols, 961 calls, 0.021s reindex (cold 0.154s) | 580 nodes, 229 calls, 0.583s | 1518 symbols, 8224 occ, 2.102s | 19 files, 12 diag, 0.727s |
| javascript | expressjs/express | 300 symbols, 435 calls, 0.028s reindex (cold 0.114s) | 31 nodes, 3 calls, 0.142s | 398 symbols, 2649 occ, 0.985s | 57 files, 257 diag, 0.814s |
| typescript | pmndrs/zustand | 222 symbols, 197 calls, 0.02s reindex (cold 0.1s) | 112 nodes, 6 calls, 0.168s | 792 symbols, 2461 occ, 0.635s | 124 files, 1 diag, 0.724s |
| java | google/gson | 1401 symbols, 3105 calls, 0.034s reindex (cold 0.3s) | 1016 nodes, 927 calls, 0.862s | 3408 symbols, 21514 occ, 3.346s | 54 doc syms, 425 diag, 1.782s |
| c | DaveGamble/cJSON | 1387 symbols, 4417 calls, 0.02s reindex (cold 0.353s) | 971 nodes, 1018 calls, 0.87s | n/a | 258 doc syms, 2 diag, 0.11s |
| cpp | google/leveldb | 1372 symbols, 9537 calls, 0.03s reindex (cold 0.358s) | 2206 nodes, 1195 calls, 1.202s | n/a | 421 doc syms, 167 diag, 0.227s |

## Derived Go ratios

- Speed: Atlas reindex 0.022s (cold index 0.397s) vs graphify 0.678s, graphify/Atlas = 30.82x.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:2085, imports:224, references:622.
- Call coverage proxy: Atlas internal calls 1134 vs graphify calls 330, Atlas/graphify = 3.44x.
- Atlas receiver-typed calls: 624/2085 = 29.9%.
- graphify extracted calls: 196/330 = 59.4%.
- SCIP semantic index: 47 documents, 2214 symbols, 11799 occurrences, 9579 references.
- SCIP navigation symbols (excluding local variables/packages) = 634; Atlas symbols vs SCIP navigation symbols = 1.07x.
- SCIP local variables = 1562. Atlas currently keeps locals out of the first-class symbol table, which lowers token cost but limits fine-grained reference parity.
- gopls workspace truth: 12 workspace packages, 57 compiled Go files, 0 diagnostics, initial load 289.017ms.
- Query token cost (4/4 equivalent rows): graphify 389 tokens vs Atlas 32 tokens, graphify/Atlas = 12.16x.
- Query latency (4/4 equivalent rows): graphify 312.441ms vs Atlas 48.454ms, graphify/Atlas = 6.45x.

## Derived Python ratios

- Speed: Atlas reindex 0.021s (cold index 0.154s) vs graphify 0.583s, graphify/Atlas = 27.76x.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:961, imports:189.
- Call coverage proxy: Atlas internal calls 371 vs graphify calls 229, Atlas/graphify = 1.62x.
- graphify extracted calls: 221/229 = 96.5%.
- SCIP semantic index: 19 documents, 1518 symbols, 8224 occurrences, 6739 references, scope=repo-root.
- Atlas symbols vs SCIP symbols = 0.34x. scip-python 0.6.6 reports all Python symbols as UnspecifiedKind, so this is a raw coverage proxy, not navigation-kind parity.
- Python AST callable/class truth: Atlas 320/320 function/method/class symbols = 100.0% recall across 19 files.
- Python AST assignment truth: Atlas 197 assignment symbols vs 133 direct module/class assignment names; extra symbols can come from conditional class scopes.
- Pyright truth pass: 19 files analyzed, 12 diagnostics (error:12), version 1.1.411.
- Query token cost (3/3 equivalent rows): graphify 389 tokens vs Atlas 22 tokens, graphify/Atlas = 17.68x.
- Query latency (3/3 equivalent rows): graphify 240.392ms vs Atlas 35.721ms, graphify/Atlas = 6.73x.

## Derived JS/TS ratios

### javascript

- Speed: Atlas reindex 0.028s (cold index 0.114s) vs graphify 0.142s, graphify/Atlas = 5.07x.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:435.
- Call coverage proxy: Atlas internal calls 194 vs graphify calls 3, Atlas/graphify = 64.67x.
- Atlas receiver-typed calls: 0/435 = 0.0%.
- graphify extracted calls: 3/3 = 100.0%.
- SCIP semantic index: 6 documents, 398 symbols, 2649 occurrences, 2251 references, scope=lib.
- Atlas symbols vs SCIP symbols = 0.75x. scip-typescript reports symbols as UnspecifiedKind, so this is a raw coverage proxy.
- TypeScript semantic check proxy: 57 files, 257 diagnostics, total 0.19s, memory 82436KB.
- LSP caveat: tsc returned diagnostics/exit 2; used as scriptable tsserver proxy.
- Query token cost (3/4 equivalent rows): graphify 136 tokens vs Atlas 25 tokens, graphify/Atlas = 5.44x.
- Query latency (3/4 equivalent rows): graphify 209.17ms vs Atlas 34.493ms, graphify/Atlas = 6.06x.
- Query caveat: graphify missed 1 Atlas-selected hub symbols; raw rows remain in the table.
### typescript

- Speed: Atlas reindex 0.02s (cold index 0.1s) vs graphify 0.168s, graphify/Atlas = 8.4x.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:197, imports:17.
- Call coverage proxy: Atlas internal calls 79 vs graphify calls 6, Atlas/graphify = 13.17x.
- Atlas receiver-typed calls: 8/197 = 4.1%.
- graphify extracted calls: 6/6 = 100.0%.
- SCIP semantic index: 16 documents, 792 symbols, 2461 occurrences, 1669 references, scope=src.
- Atlas symbols vs SCIP symbols = 0.28x. scip-typescript reports symbols as UnspecifiedKind, so this is a raw coverage proxy.
- TypeScript semantic check proxy: 124 files, 1 diagnostics, total 0.14s, memory 72473KB.
- LSP caveat: tsc returned diagnostics/exit 2; used as scriptable tsserver proxy.
- Query token cost (3/4 equivalent rows): graphify 180 tokens vs Atlas 22 tokens, graphify/Atlas = 8.18x.
- Query latency (3/4 equivalent rows): graphify 210.632ms vs Atlas 34.077ms, graphify/Atlas = 6.18x.
- Query caveat: graphify missed 1 Atlas-selected hub symbols; raw rows remain in the table.

## Derived Java ratios

- Speed: Atlas reindex 0.034s (cold index 0.3s) vs graphify 0.862s, graphify/Atlas = 25.35x.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:3105, imports:677.
- Call coverage proxy: Atlas internal calls 2401 vs graphify calls 927, Atlas/graphify = 2.59x.
- Atlas receiver-typed calls: 2326/3105 = 74.9%.
- graphify extracted calls: 599/927 = 64.6%.
- SCIP semantic index: 85 documents, 3408 symbols, 21514 occurrences, 18109 references, scope=gson.
- SCIP navigation symbols (excluding local variables/packages) = 1549; Atlas symbols vs SCIP navigation symbols = 0.9x.
- JDTLS LSP smoke: initialized against build root gson, sampled 5/5 files, 54 document symbols, 11 workspace symbols for query `Gson`, 425 diagnostics.
- Query token cost (4/4 equivalent rows): graphify 517 tokens vs Atlas 37 tokens, graphify/Atlas = 13.97x.
- Query latency (4/4 equivalent rows): graphify 362.663ms vs Atlas 64.634ms, graphify/Atlas = 5.61x.

## Derived C/C++ ratios

### c

- Speed: Atlas reindex 0.02s (cold index 0.353s) vs graphify 0.87s, graphify/Atlas = 43.5x.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:4417, imports:371.
- Call coverage proxy: Atlas internal calls 1690 vs graphify calls 1018, Atlas/graphify = 1.66x.
- Atlas receiver-typed calls: 18/4417 = 0.4%.
- graphify extracted calls: 492/1018 = 48.3%.
- clangd LSP smoke: sampled 8/8 files, 258 document symbols, 2 diagnostics.
- Query token cost (4/4 equivalent rows): graphify 1206 tokens vs Atlas 42 tokens, graphify/Atlas = 28.71x.
- Query latency (4/4 equivalent rows): graphify 351.778ms vs Atlas 53.748ms, graphify/Atlas = 6.54x.
### cpp

- Speed: Atlas reindex 0.03s (cold index 0.358s) vs graphify 1.202s, graphify/Atlas = 40.07x.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:9537, imports:777.
- Call coverage proxy: Atlas internal calls 4388 vs graphify calls 1195, Atlas/graphify = 3.67x.
- Atlas receiver-typed calls: 1977/9537 = 20.7%.
- graphify extracted calls: 1027/1195 = 85.9%.
- clangd LSP smoke: sampled 8/8 files, 421 document symbols, 167 diagnostics.
- Query token cost (3/3 equivalent rows): graphify 397 tokens vs Atlas 27 tokens, graphify/Atlas = 14.7x.
- Query latency (3/3 equivalent rows): graphify 294.113ms vs Atlas 51.777ms, graphify/Atlas = 5.68x.

## Query token probes

### go

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| log | equivalent | 96 | 6 | 78.765 | 12.091 |
| newEntry | equivalent | 62 | 8 | 78.265 | 12.121 |
| releaseEntry | equivalent | 175 | 9 | 77.168 | 12.038 |
| Fire | equivalent | 56 | 9 | 78.243 | 12.204 |

### python

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| get | equivalent | 142 | 6 | 80.169 | 11.181 |
| request | equivalent | 185 | 7 | 80.555 | 12.249 |
| __init__ | equivalent | 62 | 9 | 79.668 | 12.291 |

### javascript

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| sendFile | equivalent | 46 | 8 | 70.374 | 10.805 |
| defineGetter | equivalent | 47 | 8 | 69.434 | 11.93 |
| render | equivalent | 43 | 9 | 69.362 | 11.758 |
| contentDisposition | graphify_missing | 11 | 9 | 68.543 | 11.743 |

### typescript

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| DevtoolsImpl | equivalent | 53 | 7 | 70.148 | 10.368 |
| hydrate | graphify_missing | 8 | 7 | 70.293 | 11.591 |
| persistImpl | equivalent | 52 | 8 | 70.058 | 12.22 |
| shallow | equivalent | 75 | 7 | 70.426 | 11.489 |

### java

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| create | equivalent | 64 | 10 | 89.14 | 14.988 |
| write | equivalent | 112 | 9 | 93.721 | 17.719 |
| peek | equivalent | 239 | 10 | 89.163 | 14.96 |
| read | equivalent | 102 | 8 | 90.639 | 16.967 |

### c

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| cJSON_Delete | equivalent | 284 | 8 | 88.342 | 14.093 |
| cjson_functions_should_not_crash_with_null_pointers | equivalent | 288 | 18 | 86.638 | 13.142 |
| cJSON_CreateObject | equivalent | 332 | 9 | 86.998 | 13.242 |
| UnityPrint | equivalent | 302 | 7 | 89.8 | 13.271 |

### cpp

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| TEST_F | equivalent | 93 | 11 | 98.738 | 25.458 |
| slice | equivalent | 232 | 6 | 98.496 | 13.139 |
| Size | equivalent | 72 | 10 | 96.879 | 13.18 |

## Missing or partial adapters


---
Generated by `bench/codeintel_matrix.py`. Raw JSON sits next to this report; logs are in `bench/logs/`.