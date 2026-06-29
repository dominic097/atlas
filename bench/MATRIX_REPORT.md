# Atlas code-intelligence matrix benchmark

This report benchmarks Atlas against the agreed per-language baselines. Raw metrics are kept separate by tool because Atlas, graphify, SCIP, and LSP servers expose different surfaces.

## Tool version manifest

Raw artifact: `bench/MATRIX_TOOL_VERSIONS.json`.
- Platform: Darwin 25.5.0 arm64; Python 3.14.6.

| tool | status | version / first output line | command |
|---|---|---|---|
| atlas | ok | `atlas v0.1.24-dirty (6ea596d, 2026-06-29T21:49:19Z)` | `bin/atlas version` |
| graphify | ok | `graphifyy 0.8.49` | `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify --version` |
| go | ok | `go version go1.25.0 darwin/arm64` | `/usr/local/go/bin/go version` |
| python | ok | `Python 3.14.6` | `/opt/homebrew/opt/python@3.14/bin/python3.14 --version` |
| java | ok | `openjdk version "17.0.18" 2026-01-20` | `/usr/bin/java -version` |
| maven | ok | `Apache Maven 3.9.16 (2bdd9fddda4b155ebf8000e807eb73fd829a51d5)` | `/opt/homebrew/bin/mvn --version` |
| scip-go | ok | `0.2.7` | `/Users/damirdarasu/go/bin/scip-go --version` |
| scip-python | ok | `0.6.6` | `/opt/homebrew/bin/scip-python --version` |
| scip-typescript | ok | `0.4.0` | `/opt/homebrew/bin/npx --yes -p @sourcegraph/scip-typescript scip-typescript --version` |
| scip-java | missing | `` | `` |
| gopls | ok | `golang.org/x/tools/gopls v0.22.0` | `/Users/damirdarasu/go/bin/gopls version` |
| pyright | ok | `pyright 1.1.411` | `/opt/homebrew/bin/pyright --version` |
| tsc | ok | `Version 5.9.3` | `/opt/homebrew/bin/npx --yes -p typescript tsc --version` |
| jdtls | ok | `1.58.0` | `/opt/homebrew/bin/jdtls` |
| clangd | ok | `Apple clangd version 17.0.0 (clang-1700.6.4.2)` | `/usr/bin/clangd --version` |
| rust-analyzer | ok | `rust-analyzer 0.0.0 (69ccffdb5b 2026-06-21)` | `/opt/homebrew/bin/rust-analyzer --version` |
| dotnet | ok | `10.0.301` | `/opt/homebrew/bin/dotnet --version` |
| ruby | ok | `ruby 2.6.10p210 (2022-04-12 revision 67958) [universal.arm64e-darwin25]` | `/usr/bin/ruby --version` |
| php | ok | `PHP 8.4.14 (cli) (built: Oct 21 2025 19:23:55) (NTS)` | `/opt/homebrew/bin/php --version` |
| pwsh | ok | `PowerShell 7.4.6` | `/usr/local/bin/pwsh --version` |
| sourcekit-lsp | ok | `Apple Swift version 6.2.4 (swiftlang-6.2.4.1.4 clang-1700.6.4.2)` | `/usr/bin/sourcekit-lsp (no version flag); /usr/bin/swift --version` |

- Live smoke native-version details: 24/36 artifacts expose explicit native tool or library version fields in raw JSON; all artifacts include native command/status.

## graphify language discovery

- Installed graphify: graphifyy 0.8.49 (`/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify`).
- Runtime Python: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/python3`.
- Source inspected: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify`.
- Extract source: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify/extract.py`.
- Detect source: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify/detect.py`.
- Evidence: CLI help from `graphify --help` did not enumerate languages, but confirmed `update`, `extract`, and code-only AST update commands.
- Evidence: `graphify.detect.CODE_EXTENSIONS` plus a runtime `detect()` smoke listed code extensions.
- Evidence: `graphify.extract._DISPATCH` provided the deterministic extractor map used as the parser-parity target.
- Raw discovery artifact: `bench/GRAPHIFY_LANGUAGE_DISCOVERY.json`.
- Runtime help probe: `graphify --help` succeeded and listed 148 command/help lines.
- Runtime support probe: `_DISPATCH` plus filename-special extractors exposed 89 deterministic extractor entries; `CODE_EXTENSIONS` exposed 88 code extensions.
- Runtime detect smoke: generated one sample per `CODE_EXTENSIONS` entry; `detect()` returned 88 code files.
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
| csharp | `.cs` | `extract_csharp` | native tree-sitter tags |
| rust | `.rs` | `extract_rust` | native tree-sitter tags |
| ruby | `.rb` | `extract_ruby` | native tree-sitter tags |
| kotlin | `.kt .kts` | `extract_kotlin` | native tree-sitter tags |
| scala | `.scala` | `extract_scala` | native tree-sitter tags |
| php | `.php` | `extract_php` | native tree-sitter tags |
| blade | `*.blade.php` | `extract_blade` | lightweight regex |
| swift | `.swift` | `extract_swift` | native tree-sitter tags |
| lua | `.lua .luau .toc` | `extract_lua` | native tree-sitter tags |
| zig | `.zig` | `extract_zig` | native tree-sitter tags |
| powershell | `.ps1 .psm1 .psd1` | `extract_powershell/extract_powershell_manifest` | lightweight regex |
| elixir | `.ex .exs` | `extract_elixir` | native tree-sitter AST |
| objective-c | `.m .mm` | `extract_objc` | native tree-sitter AST |
| julia | `.jl` | `extract_julia` | native tree-sitter AST |
| fortran | `.f .F .f90 .F90 .f95 .F95 .f03 .F03 .f08 .F08` | `extract_fortran` | lightweight regex |
| dart | `.dart` | `extract_dart` | native tree-sitter AST |
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

## graphify coverage audit

- Deterministic graphify families covered by Atlas evidence: 39/39. Detector-only extensions covered by live Atlas smokes: 3/3.
- Unsupported graphify rows: none.
- Missing evidence: none.

| graphify support | status | Atlas evidence |
|---|---|---|
| go<br>`.go` via `extract_go` | ok | core matrix `go` ok; native scip-go:ok, gopls:ok; query eq 4/4, latency 6.07x, tokens 14.21x |
| python<br>`.py` via `extract_python` | ok | core matrix `python` ok; native scip-python:ok, pyright:ok; query eq 3/3, latency 6.01x, tokens 18.52x |
| javascript<br>`.js .jsx .mjs` via `extract_js` | ok | core matrix `javascript` ok; native scip-typescript:ok, tsserver:ok; query eq 3/4, latency 5.6x, tokens 7.0x |
| typescript<br>`.ts .tsx` via `extract_js` | ok | core matrix `typescript` ok; native scip-typescript:ok, tsserver:ok; query eq 3/4, latency 5.88x, tokens 11.42x |
| java<br>`.java` via `extract_java` | ok | core matrix `java` ok; native scip-java:missing, jdtls:ok; query eq 2/2, latency 3.91x, tokens 15.29x |
| groovy/gradle<br>`.groovy .gradle` via `extract_groovy` | ok | `bench/LIVE_GROOVY_SMOKE.json` native_limited=partial; repo `https://github.com/nextflow-io/nextflow` commit `83d452a51796aca34f136f796383185d703c349c`; native `tree-sitter-groovy`; atlas_vs_tree_sitter_groovy_definition_ratio=1.59, query eq 5/5, latency 6.79x, tokens 9.97x |
| c<br>`.c .h` via `extract_c` | ok | core matrix `c` ok; native clangd:ok; query eq 4/4, latency 6.09x, tokens 28.71x |
| cpp/cuda<br>`.cpp .cc .cxx .hpp .cu .cuh` via `extract_cpp` | ok | core matrix `cpp` ok; native clangd:ok; query eq 4/4, latency 5.75x, tokens 10.0x<br>`bench/LIVE_CUDA_SMOKE.json` ok; repo `https://github.com/NVIDIA/cuda-samples` commit `b7c5481c556c3fe98db060207ecaa41a4b9a9abc`; native `cuda-source-counter`; atlas_vs_cuda_source_counter_definition_ratio=1.0, query eq 3/3, latency 5.96x, tokens 6.67x |
| csharp<br>`.cs` via `extract_csharp` | ok | `bench/LIVE_CSHARP_SMOKE.json` ok; repo `https://github.com/DapperLib/Dapper` commit `72a54c475f75e18cb93cba0809d00a5e6e49efd9`; native `roslyn`; atlas_vs_roslyn_definition_ratio=1.84, query eq 4/4, latency 7.26x, tokens 14.15x |
| rust<br>`.rs` via `extract_rust` | ok | `bench/LIVE_RUST_SMOKE.json` ok; repo `https://github.com/BurntSushi/ripgrep` commit `dfe4a81d2591daca76d25ae4e052c34b26578155`; native `rust-analyzer`; atlas_vs_rust_analyzer_definition_ratio=1.0, query eq 4/4, latency 8.71x, tokens 40.23x |
| ruby<br>`.rb` via `extract_ruby` | ok | `bench/LIVE_RUBY_SMOKE.json` ok; repo `https://github.com/sinatra/sinatra` commit `5236d3459b8b9015e5ce21ddd0c6beb0db4081d4`; native `ruby-ripper`; atlas_vs_ruby_ripper_definition_ratio=1.01, query eq 4/4, latency 7.13x, tokens 10.86x |
| kotlin<br>`.kt .kts` via `extract_kotlin` | ok | `bench/LIVE_KOTLIN_SMOKE.json` native_limited=partial; repo `https://github.com/square/okhttp` commit `0cadfa2997513d20bf88ca530c963a1266f17af9`; native `tree-sitter-kotlin`; atlas_vs_tree_sitter_kotlin_definition_ratio=1.0, query eq 5/5, latency 7.08x, tokens 27.06x |
| scala<br>`.scala` via `extract_scala` | ok | `bench/LIVE_SCALA_SMOKE.json` native_limited=partial; repo `https://github.com/typelevel/cats` commit `851965a582940d804f9a23179f58a53fc97f07dc`; native `tree-sitter-scala`; atlas_vs_tree_sitter_scala_definition_ratio=1.0, query eq 5/5, latency 9.38x, tokens 38.06x |
| php<br>`.php` via `extract_php` | ok | `bench/LIVE_PHP_SMOKE.json` ok; repo `https://github.com/slimphp/Slim` commit `0da7dd2fc66956730b6633f6a056b35e59126583`; native `php-tokenizer`; atlas_vs_php_tokenizer_definition_ratio=1.0, query eq 3/4, latency 7.04x, tokens 8.74x |
| blade<br>`*.blade.php` via `extract_blade` | ok | `bench/LIVE_BLADE_SMOKE.json` ok; repo `https://github.com/BookStackApp/BookStack` commit `f7df78b91b12ff1d8e248ded7747e76203904b8e`; native `blade-directive-counter`; atlas_vs_blade_directive_counter_definition_ratio=1.0, query eq 6/6, latency 6.74x, tokens 6.1x |
| swift<br>`.swift` via `extract_swift` | ok | `bench/LIVE_SWIFT_SMOKE.json` ok; repo `https://github.com/apple/swift-argument-parser` commit `8122bc5941426c9494c78ff5ad01951e81c02f53`; native `sourcekit-lsp`; atlas_vs_sourcekit_lsp_definition_ratio=1.23, query eq 4/4, latency 7.98x, tokens 21.65x |
| lua<br>`.lua .luau .toc` via `extract_lua` | ok | `bench/LIVE_LUA_SMOKE.json` ok; repo `https://github.com/folke/lazy.nvim` commit `306a05526ada86a7b30af95c5cc81ffba93fef97`; native `luaparser`; atlas_vs_luaparser_definition_ratio=1.0, query eq 4/4, latency 6.88x, tokens 15.09x |
| zig<br>`.zig` via `extract_zig` | ok | `bench/LIVE_ZIG_SMOKE.json` native_limited=partial; repo `https://github.com/zigtools/zls` commit `8da87d4f3305a550e7b739bad764e34bf1e46a08`; native `tree-sitter-zig`; atlas_vs_tree_sitter_zig_definition_ratio=1.22, query eq 5/5, latency 5.55x, tokens 28.55x |
| powershell<br>`.ps1 .psm1 .psd1` via `extract_powershell/extract_powershell_manifest` | ok | `bench/LIVE_POWERSHELL_SMOKE.json` ok; repo `https://github.com/PowerShell/PowerShellGet` commit `a2dac8e74603f7c9eec4a54c5e23459531751b0d`; native `pwsh-parser`; atlas_vs_pwsh_parser_definition_ratio=1.0, query eq 4/4, latency 6.37x, tokens 6.68x |
| elixir<br>`.ex .exs` via `extract_elixir` | ok | `bench/LIVE_ELIXIR_SMOKE.json` ok; repo `https://github.com/phoenixframework/phoenix` commit `909725968776c2601bbea7827fd5b76e4992cc70`; native `tree-sitter-elixir`; atlas_vs_tree_sitter_elixir_definition_ratio=1.0, query eq 5/5, latency 6.61x, tokens 22.95x |
| objective-c<br>`.m .mm` via `extract_objc` | ok | `bench/LIVE_OBJC_SMOKE.json` native_limited=partial; repo `https://github.com/SDWebImage/SDWebImage` commit `c3ad5e1a9bf55c9b76d4c362430b5fcded96c502`; native `tree-sitter-objc`; atlas_vs_tree_sitter_objc_definition_ratio=1.0, query eq 4/5, latency 6.78x, tokens 23.63x |
| julia<br>`.jl` via `extract_julia` | ok | `bench/LIVE_JULIA_SMOKE.json` native_limited=partial; repo `https://github.com/JuliaIO/JSON.jl` commit `e5ef310dece16746843753e4c3b44e868b917b64`; native `tree-sitter-julia`; atlas_vs_tree_sitter_julia_definition_ratio=1.0, query eq 5/6, latency 5.75x, tokens 12.72x |
| fortran<br>`.f .F .f90 .F90 .f95 .F95 .f03 .F03 .f08 .F08` via `extract_fortran` | ok | `bench/LIVE_FORTRAN_SMOKE.json` ok; repo `https://github.com/fortran-lang/stdlib` commit `4c8521d5658455a576946cca3bfe2bd8ede36e24`; native `tree-sitter-fortran`; atlas_vs_tree_sitter_fortran_definition_ratio=1.0, query eq 6/6, latency 6.61x, tokens 19.69x |
| dart<br>`.dart` via `extract_dart` | ok | `bench/LIVE_DART_SMOKE.json` ok; repo `https://github.com/dart-lang/http` commit `5d94ef52582867e077bf41c3fa20fb8b1d1d834e`; native `tree-sitter-dart`; atlas_vs_tree_sitter_dart_definition_ratio=1.0, query eq 6/6, latency 6.25x, tokens 8.54x |
| verilog/systemverilog<br>`.v .sv .svh` via `extract_verilog` | ok | `bench/LIVE_VERILOG_SMOKE.json` native_limited=partial; repo `https://github.com/lowRISC/ibex` commit `022f084096baed0a9b5ebdf697ed2965f13e8ed8`; native `tree-sitter-systemverilog`; atlas_vs_tree_sitter_systemverilog_definition_ratio=1.0, query eq 6/6, latency 6.09x, tokens 8.73x |
| sql<br>`.sql` via `extract_sql` | ok | `bench/LIVE_SQL_SMOKE.json` ok; repo `https://github.com/hasura/graphql-engine` commit `417c174c0ac3c80dafe6e8f9e8ac39d868334724`; native `sqlfluff`; atlas_vs_sqlfluff_definition_ratio=1.0, query eq 4/4, latency 6.23x, tokens 5.33x |
| markdown<br>`.md .mdx .qmd` via `extract_markdown` | ok | `bench/LIVE_MARKDOWN_SMOKE.json` ok; repo `https://github.com/rust-lang/mdBook` commit `cb49cc5523e609a731f27dea1af4395a504815a5`; native `markdown-it-py`; atlas_vs_markdown_it_py_definition_ratio=1.0, query eq 6/6, latency 6.69x, tokens 10.31x |
| pascal<br>`.pas .pp .dpr .dpk .lpr .inc` via `extract_pascal` | ok | `bench/LIVE_PASCAL_SMOKE.json` ok; repo `https://github.com/remobjects/pascalscript` commit `2c826cd803a3c0417354fa37b2417f21993ce4ac`; native `pascal-regex-counter`; atlas_vs_pascal_regex_counter_definition_ratio=1.0, query eq 4/5, latency 8.33x, tokens 12.84x |
| delphi/lazarus forms<br>`.dfm .lfm .lpk` via `extract_delphi_form/extract_lazarus_form/extract_lazarus_package` | ok | `bench/LIVE_DELPHI_SMOKE.json` ok; repo `https://github.com/fpc/Lazarus` commit `a37c7c35c6a271b377cd799c48586ea2689a8e0f`; native `delphi-lazarus-source-counter`; atlas_vs_delphi_lazarus_source_counter_definition_ratio=1.0, query eq 7/7, latency 9.25x, tokens 12.35x |
| shell<br>`.sh .bash` via `extract_bash` | ok | `bench/LIVE_BASH_SMOKE.json` ok; repo `https://github.com/nvm-sh/nvm` commit `a6ec73943099a86fba98bde3b04a1c60944a4549`; native `bash-n`; atlas_vs_bash_n_definition_ratio=1.0, query eq 4/4, latency 6.69x, tokens 18.44x |
| json config<br>`.json` via `extract_json` | ok | `bench/LIVE_JSON_SMOKE.json` ok; repo `https://github.com/eslint/create-config` commit `58d77fc302b25976bb4cc7dc273377b421bc226b`; native `python-json`; atlas_vs_python_json_definition_ratio=1.0, query eq 4/6, latency 6.58x, tokens 13.28x |
| terraform/hcl<br>`.tf .tfvars .hcl` via `extract_terraform` | ok | `bench/LIVE_TERRAFORM_SMOKE.json` ok; repo `https://github.com/terraform-aws-modules/terraform-aws-vpc` commit `3ffbd46fb1c7733e1b34d8666893280454e27436`; native `python-hcl2`; atlas_vs_python_hcl2_definition_ratio=1.0, query eq 4/4, latency 6.27x, tokens 41.61x |
| byond dm<br>`.dm .dme .dmi .dmm .dmf` via `extract_dm/extract_dmf/extract_dmi/extract_dmm` | ok | `bench/LIVE_BYOND_SMOKE.json` ok; repo `https://github.com/tgstation/tgstation` commit `4c88909444709d715adfb8f96e9e96cccd986095`; native `byond-source-counter`; atlas_vs_byond_source_counter_definition_ratio=1.0, query eq 0/6, latency n/a, tokens n/a |
| dotnet project<br>`.sln .slnx .csproj .fsproj .vbproj` via `extract_csproj/extract_sln/extract_slnx` | ok | `bench/LIVE_DOTNET_SMOKE.json` ok; repo `https://github.com/DapperLib/Dapper` commit `72a54c475f75e18cb93cba0809d00a5e6e49efd9`; native `python-dotnet-project`; atlas_vs_python_dotnet_project_definition_ratio=1.0, query eq 6/6, latency 8.89x, tokens 6.15x |
| razor<br>`.razor .cshtml` via `extract_razor` | ok | `bench/LIVE_RAZOR_SMOKE.json` ok; repo `https://github.com/dotnet-architecture/eShopOnWeb` commit `4da8212117e87d808d4bbc7da6286fd2147ce606`; native `razor-directive-counter`; atlas_vs_razor_directive_counter_definition_ratio=1.0, query eq 6/6, latency 7.15x, tokens 7.88x |
| apex<br>`.cls .trigger` via `extract_apex` | ok | `bench/LIVE_APEX_SMOKE.json` ok; repo `https://github.com/trailheadapps/apex-recipes` commit `cb5cd9d3621985b816476e5a317b5a0016fc3576`; native `apex-source-counter`; atlas_vs_apex_source_counter_definition_ratio=1.0, query eq 5/6, latency 8.52x, tokens 11.67x |
| vue<br>`.vue` via `extract_js` | ok | `bench/LIVE_VUE_SMOKE.json` ok; repo `https://github.com/gothinkster/vue-realworld-example-app` commit `f7e48c8178602ce25d43293bc6f8ca51d84ae222`; native `vue-compiler-sfc`; atlas_vs_vue_compiler_sfc_definition_ratio=1.0, query eq 4/4, latency 6.22x, tokens 8.04x |
| svelte<br>`.svelte` via `extract_svelte` | ok | `bench/LIVE_SVELTE_SMOKE.json` ok; repo `https://github.com/carbon-design-system/carbon-components-svelte` commit `454a3881cd2b5ac3e47b92d7524a8faa5d99f03d`; native `svelte-compiler`; atlas_vs_svelte_compiler_definition_ratio=1.04, query eq 5/5, latency 6.3x, tokens 8.81x |
| astro<br>`.astro` via `extract_astro` | ok | `bench/LIVE_ASTRO_SMOKE.json` ok; repo `https://github.com/withastro/blog-tutorial-demo` commit `0859a3ea85f35325c0a292f9b3131824ef95ee38`; native `astro-compiler`; atlas_vs_astro_compiler_definition_ratio=1.0, query eq 5/6, latency 6.26x, tokens 10.54x |
| detector-only .ejs<br>`.ejs` in `CODE_EXTENSIONS`, no `_DISPATCH` extractor | ok | `bench/LIVE_EJS_SMOKE.json` ok; repo `https://github.com/expressjs/express` commit `18e5985b8a9d5e8423db0a9121f22bdaecd5b120`; native `ejs-template-counter`; atlas_vs_ejs_template_counter_definition_ratio=1.0, query eq 1/7, latency 6.19x, tokens 8.0x |
| detector-only .ets<br>`.ets` in `CODE_EXTENSIONS`, no `_DISPATCH` extractor | ok | `bench/LIVE_ETS_SMOKE.json` ok; repo `https://github.com/openharmony/applications_app_samples` commit `a826ab0e75fe51d028c1c5af58188e908736b53b`; native `ets-source-counter`; atlas_vs_ets_source_counter_definition_ratio=1.0, query eq 0/8, latency n/a, tokens n/a |
| detector-only .r<br>`.r` in `CODE_EXTENSIONS`, no `_DISPATCH` extractor | ok | `bench/LIVE_R_SMOKE.json` ok; repo `https://github.com/tidyverse/ggplot2` commit `6870419aa6e106c3580c45c81d5b688cb31758bd`; native `r-source-counter`; atlas_vs_r_source_counter_definition_ratio=1.0, query eq 0/8, latency n/a, tokens n/a |

## Saturation loop evidence

Raw artifacts: `bench/SATURATION_REPORT.json` and `bench/SATURATION_REPORT.md`. Iterations requested per language: 5.

| language | status | iterations | equivalent rows by pass | graphify missing rows by pass | coverage ratio by pass |
|---|---|---:|---|---|---|
| byond | saturated_no_equivalent_graphify_rows | 5 | 0/6, 0/6, 0/6, 0/6, 0/6 | 6, 6, 6, 6, 6 | 1.0, 1.0, 1.0, 1.0, 1.0 |
| ets | saturated_no_equivalent_graphify_rows | 5 | 0/8, 0/8, 0/8, 0/8, 0/8 | 8, 8, 8, 8, 8 | 1.0, 1.0, 1.0, 1.0, 1.0 |
| r | saturated_no_equivalent_graphify_rows | 5 | 0/8, 0/8, 0/8, 0/8, 0/8 | 8, 8, 8, 8, 8 | 1.0, 1.0, 1.0, 1.0, 1.0 |

Saturation note: these languages are marked saturated only for graphify-equivalent query-score improvement. Their native coverage proxies remain in the live smoke artifacts; no 5x query claim is made where graphify exposes no equivalent rows.

## Live additional-language smokes

### Apex

Raw artifact: `bench/LIVE_APEX_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/trailheadapps/apex-recipes` at commit `cb5cd9d3621985b816476e5a317b5a0016fc3576`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-apex-recipes/repo/force-app`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-apex-recipes/atlas.db --json index /tmp/atlas-live-apex-recipes/repo/force-app`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-apex-recipes/atlas.db --json index /tmp/atlas-live-apex-recipes/repo/force-app`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `python3 <apex source counter> /tmp/atlas-live-apex-recipes/repo/force-app`

Results:

- Atlas indexed 501 files, 5101 symbols, and 5045 edges in 1.365s cold; no-change reindex was 0.024s (`mode=noop`).
- Atlas language counts were `xml:254`, `apex:142`, `markdown:74`, `javascript:18`, `html:7`, `css:3`, `json:3`.
- graphify rebuilt 3992 nodes and 3824 links in 1.944s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `apex-source-counter` status: ok (files:142, parsed_files:142, parse_errors:0, definitions:1072).
- Richer native baselines not available on this machine: `sf`, `sfdx`, `apex-language-server`.
- Coverage proxy: atlas_vs_apex_source_counter_definition_ratio: 1.0, atlas_apex_definition_symbols: 1072, native_definitions: 1072.
- Optimization cycles: 2 (Apex live smoke matched the source-counter coverage proxy after adding an Apex-specific parser for Salesforce classes, triggers, constructors, SOQL SObjects, and DML operations.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `SOQLRecipes` | 16.125 | 145.285 | 9.01x | 8 | 262 | 32.75x |
| `DMLRecipes` | 16.394 | 145.145 | 8.85x | 7 | 60 | 8.57x |
| `AccountTrigger` | 16.624 | 145.117 | 8.73x | 11 | 97 | 8.82x |
| `Account` | 18.395 | 149.905 | 8.15x | 13 | 76 | 5.85x |
| `insert` | 18.075 | 143.748 | 7.95x | 9 | 65 | 7.22x |
| `listAccounts` | 17.206 | 145.855 | 8.48x | 3 | 9 | 3.0x |

- Query caveat: `listAccounts` (graphify_missing); raw rows remain in the table.
5x note: this Apex smoke meets the 5x threshold on equivalent query rows for latency (8.52x) and token output (11.67x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Astro

Raw artifact: `bench/LIVE_ASTRO_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/withastro/blog-tutorial-demo` at commit `0859a3ea85f35325c0a292f9b3131824ef95ee38`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-astro-blog-tutorial/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-astro-blog-tutorial/atlas.db --json index /tmp/atlas-live-astro-blog-tutorial/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-astro-blog-tutorial/atlas.db --json index /tmp/atlas-live-astro-blog-tutorial/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/node /tmp/atlas-live-astro-blog-tutorial/astro-compiler/astro_stats.js /tmp/atlas-live-astro-blog-tutorial/repo/src`

Results:

- Atlas indexed 22 files, 68 symbols, and 69 edges in 0.065s cold; no-change reindex was 0.034s (`mode=noop`).
- Atlas language counts were `astro:14`, `markdown:4`, `javascript:3`, `css:1`.
- graphify rebuilt 36 nodes and 30 links in 0.267s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `astro-compiler` status: ok (files:14, parsed_files:14, parse_errors:0, file_components:14, component_tags:17, functions:1, variables:26, definitions:58, compiler_version:4.0.0).
- Richer native baselines not available on this machine: `astro`, `astro-language-server`.
- Coverage proxy: atlas_vs_astro_compiler_definition_ratio: 1.0, atlas_astro_definition_symbols: 58, native_definitions: 58.
- Optimization cycles: 2 (Astro live smoke matched the @astrojs/compiler/frontmatter coverage proxy after adding an Astro-specific parser for component files, component tags, frontmatter functions, and variables.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `BaseLayout` | 12.712 | 77.182 | 6.07x | 9 | 171 | 19.0x |
| `BlogPost` | 12.384 | 77.095 | 6.23x | 8 | 67 | 8.38x |
| `pageTitle` | 11.686 | 75.617 | 6.47x | 8 | 9 | 1.12x |
| `getStaticPaths` | 11.897 | 75.751 | 6.37x | 7 | 51 | 7.29x |
| `ThemeIcon` | 12.253 | 77.217 | 6.3x | 8 | 68 | 8.5x |
| `Social` | 12.015 | 76.110 | 6.33x | 7 | 54 | 7.71x |

- Query caveat: `pageTitle` (graphify_missing); raw rows remain in the table.
5x note: this Astro smoke meets the 5x threshold on equivalent query rows for latency (6.26x) and token output (10.54x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Bash

Raw artifact: `bench/LIVE_BASH_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/nvm-sh/nvm` at commit `a6ec73943099a86fba98bde3b04a1c60944a4549`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-bash-nvm/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-bash-nvm/atlas.db --json index /tmp/atlas-live-bash-nvm/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-bash-nvm/atlas.db --json index /tmp/atlas-live-bash-nvm/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/bin/bash -n <5 shell files>`

Results:

- Atlas indexed 64 files, 479 symbols, and 102 edges in 0.163s cold; no-change reindex was 0.025s (`mode=noop`).
- Atlas language counts were `text:18`, `yaml:18`, `markdown:12`, `json:6`, `bash:5`, `config:2`, `dockerfile:1`, `javascript:1`, `makefile:1`.
- graphify rebuilt 452 nodes and 729 links in 0.551s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `bash-n` status: ok (files:5, parsed_files:5, syntax_errors:0, functions:158, source_edges:0, definitions:158, bash_version:GNU bash, version 3.2.57(1)-release (arm64-apple-darwin25)).
- Richer native baselines not available on this machine: `shellcheck`, `shfmt`.
- Coverage proxy: atlas_vs_bash_n_definition_ratio: 1.0, atlas_bash_definition_symbols: 158, native_definitions: 158.
- Optimization cycles: 1 (Bash live smoke met the current 5x latency/token thresholds and matched the /bin/bash -n function-definition coverage proxy on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `nvm` | 12.298 | 84.038 | 6.83x | 5 | 269 | 53.8x |
| `nvm_install_binary` | 12.470 | 83.141 | 6.67x | 8 | 87 | 10.88x |
| `nvm_die_on_prefix` | 12.192 | 81.912 | 6.72x | 8 | 90 | 11.25x |
| `nvm_get_os` | 12.337 | 80.538 | 6.53x | 6 | 52 | 8.67x |

5x note: this Bash smoke meets the 5x threshold on equivalent query rows for latency (6.69x) and token output (18.44x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Blade

Raw artifact: `bench/LIVE_BLADE_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/BookStackApp/BookStack` at commit `f7df78b91b12ff1d8e248ded7747e76203904b8e`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-blade-bookstack/repo/resources/views`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-blade-bookstack/atlas.db --json index /tmp/atlas-live-blade-bookstack/repo/resources/views`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-blade-bookstack/atlas.db --json index /tmp/atlas-live-blade-bookstack/repo/resources/views`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `python3 <blade directive counter> /tmp/atlas-live-blade-bookstack/repo/resources/views`

Results:

- Atlas indexed 304 files, 1093 symbols, and 4587 edges in 0.277s cold; no-change reindex was 0.019s (`mode=noop`).
- Atlas language counts were `blade:303`, `markdown:1`.
- graphify rebuilt 726 nodes and 421 links in 0.819s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `blade-directive-counter` status: ok (files:303, parsed_files:303, parse_errors:0, definitions:1090).
- Richer native baselines not available on this machine: `laravel`, `blade-language-server`.
- Coverage proxy: atlas_vs_blade_directive_counter_definition_ratio: 1.0, atlas_blade_definition_symbols: 1090, native_definitions: 1090.
- Optimization cycles: 3 (Blade live smoke matched the directive-counter coverage proxy and improved token score after compacting verbose `.blade.php` suffixes in terse plain locations while preserving full paths in JSON.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `settings.parts.navbar` | 14.024 | 103.236 | 7.36x | 9 | 60 | 6.67x |
| `home.parts.sidebar` | 13.748 | 90.541 | 6.59x | 8 | 57 | 7.12x |
| `entities.view-toggle` | 15.681 | 107.455 | 6.85x | 15 | 80 | 5.33x |
| `common.dark-mode-toggle` | 13.738 | 90.157 | 6.56x | 12 | 59 | 4.92x |
| `form.user-select` | 14.230 | 87.108 | 6.12x | 11 | 66 | 6.0x |
| `books.parts.list` | 12.180 | 84.813 | 6.96x | 7 | 56 | 8.0x |

5x note: this Blade smoke meets the 5x threshold on equivalent query rows for latency (6.74x) and token output (6.1x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### BYOND/DM

Raw artifact: `bench/LIVE_BYOND_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/tgstation/tgstation` at commit `4c88909444709d715adfb8f96e9e96cccd986095`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-byond-tgstation/repo/code/modules/mob`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-byond-tgstation/atlas.db --json index /tmp/atlas-live-byond-tgstation/repo/code/modules/mob`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-byond-tgstation/atlas.db --json index /tmp/atlas-live-byond-tgstation/repo/code/modules/mob`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `python3 <byond source counter> /tmp/atlas-live-byond-tgstation/repo/code/modules/mob`

Results:

- Atlas indexed 616 files, 8882 symbols, and 39073 edges in 2.088s cold; no-change reindex was 0.041s (`mode=noop`).
- Atlas language counts were `byond:614`, `markdown:1`, `text:1`.
- graphify rebuilt 6 nodes and 5 links in 3.654s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `byond-source-counter` status: ok (files:614, parsed_files:614, parse_errors:0, definitions:8874).
- Richer native baselines not available on this machine: `dreamchecker`, `dm-langserver`, `tree-sitter-dm`.
- Coverage proxy: atlas_vs_byond_source_counter_definition_ratio: 1.0, atlas_byond_definition_symbols: 8874, native_definitions: 8874.
- Optimization cycles: 5 (BYOND/DM live smoke replaces the generic regex fallback with a dedicated source parser and records saturation evidence; the five-pass saturation report records that graphify misses every path-like DM query on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `/mob/living` | 19.157 | 76.007 | 3.97x | 8 | 9 | 1.12x |
| `/mob/living/Initialize` | 15.875 | 76.856 | 4.84x | 8 | 12 | 1.5x |
| `/mob/living/prepare_data_huds` | 16.651 | 76.508 | 4.59x | 10 | 14 | 1.4x |
| `/mob/living/ZImpactDamage` | 16.472 | 76.601 | 4.65x | 9 | 13 | 1.44x |
| `/datum/movespeed_modifier/landed_on_feet` | 16.683 | 75.371 | 4.52x | 13 | 16 | 1.23x |
| `/mob/living/MobBump` | 16.260 | 76.457 | 4.7x | 8 | 11 | 1.38x |

- Query caveat: `/mob/living` (graphify_missing), `/mob/living/Initialize` (graphify_missing), `/mob/living/prepare_data_huds` (graphify_missing), `/mob/living/ZImpactDamage` (graphify_missing), `/datum/movespeed_modifier/landed_on_feet` (graphify_missing), `/mob/living/MobBump` (graphify_missing); raw rows remain in the table.
No-equivalent saturation note: this BYOND/DM smoke proves Atlas indexes the live language slice and matches the native coverage proxy, but graphify returned no equivalent query rows. Latency/token ratios from missing rows are not treated as 5x evidence; see the saturation loop artifact where applicable.

### C#

Raw artifact: `bench/LIVE_CSHARP_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/DapperLib/Dapper` at commit `72a54c475f75e18cb93cba0809d00a5e6e49efd9`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-csharp-dapper/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-csharp-dapper/atlas.db --json index /tmp/atlas-live-csharp-dapper/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-csharp-dapper/atlas.db --json index /tmp/atlas-live-csharp-dapper/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/dotnet run --no-build -- /tmp/atlas-live-csharp-dapper/repo (Roslyn syntax tree counter)`

Results:

- Atlas indexed 207 files, 3673 symbols, and 8243 edges in 0.644s cold; no-change reindex was 0.037s (`mode=noop`).
- Atlas language counts were `csharp:157`, `text:15`, `dotnet:12`, `markdown:8`, `yaml:6`, `json:4`, `xml:2`, `batch:1`, `config:1`, `powershell:1`.
- graphify rebuilt 2463 nodes and 5135 links in 1.447s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `roslyn` status: ok (files:157, parsed_files:157, parse_errors:0, definitions:1855, roslyn_version:5.6.0.0).
- Richer native baselines not available on this machine: `csc`, `omnisharp`, `csharp-ls`.
- Coverage proxy: atlas_vs_roslyn_definition_ratio: 1.84, atlas_csharp_definition_symbols: 3419, native_definitions: 1855.
- Optimization cycles: 3 (C# live smoke improves Atlas type/method recall on real Dapper code, records Atlas/graphify query metrics, and measures coverage against a Roslyn syntax-tree baseline when dotnet is installed.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `SqlMapper` | 14.840 | 120.399 | 8.11x | 10 | 296 | 29.6x |
| `CommandDefinition` | 18.041 | 114.687 | 6.36x | 13 | 293 | 22.54x |
| `DynamicParameters` | 17.532 | 115.266 | 6.57x | 18 | 69 | 3.83x |
| `TypeHandlerCache` | 13.714 | 115.341 | 8.41x | 13 | 106 | 8.15x |

5x note: this C# smoke meets the 5x threshold on equivalent query rows for latency (7.26x) and token output (14.15x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### CUDA C++

Raw artifact: `bench/LIVE_CUDA_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/NVIDIA/cuda-samples` at commit `b7c5481c556c3fe98db060207ecaa41a4b9a9abc`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-cuda-samples/repo/cpp/0_Introduction/simpleAtomicIntrinsics`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-cuda-samples/atlas.db --json index /tmp/atlas-live-cuda-samples/repo/cpp/0_Introduction/simpleAtomicIntrinsics`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-cuda-samples/atlas.db --json index /tmp/atlas-live-cuda-samples/repo/cpp/0_Introduction/simpleAtomicIntrinsics`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `python3 <cuda source counter> /tmp/atlas-live-cuda-samples/repo/cpp/0_Introduction/simpleAtomicIntrinsics`

Results:

- Atlas indexed 7 files, 26 symbols, and 63 edges in 0.073s cold; no-change reindex was 0.023s (`mode=noop`).
- Atlas language counts were `cpp:3`, `json:2`, `markdown:1`, `text:1`.
- graphify rebuilt 19 nodes and 17 links in 0.129s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `cuda-source-counter` status: ok (files:2, parsed_files:2, parse_errors:0, functions:3, definitions:3).
- Richer native baselines not available on this machine: `nvcc`.
- Coverage proxy: atlas_vs_cuda_source_counter_definition_ratio: 1.0, atlas_cuda_definition_symbols: 3, native_definitions: 3.
- Optimization cycles: 2 (CUDA live smoke covers graphify's .cu/.cuh support inside the cpp/cuda family and improved token score after compacting CUDA source suffixes in terse plain locations while preserving full paths in JSON.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `testKernel` | 12.634 | 78.659 | 6.23x | 11 | 73 | 6.64x |
| `runTest` | 13.256 | 76.893 | 5.8x | 10 | 64 | 6.4x |
| `main` | 12.683 | 74.508 | 5.87x | 9 | 63 | 7.0x |

5x note: this CUDA C++ smoke meets the 5x threshold on equivalent query rows for latency (5.96x) and token output (6.67x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Dart

Raw artifact: `bench/LIVE_DART_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/dart-lang/http` at commit `5d94ef52582867e077bf41c3fa20fb8b1d1d834e`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-dart-http/repo/pkgs/http/lib`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-dart-http/atlas.db --json index /tmp/atlas-live-dart-http/repo/pkgs/http/lib`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-dart-http/atlas.db --json index /tmp/atlas-live-dart-http/repo/pkgs/http/lib`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-dart-http/tree-sitter-dart-venv/bin/python -c <tree-sitter-dart definition counter> /tmp/atlas-live-dart-http/repo/pkgs/http/lib`

Results:

- Atlas indexed 27 files, 180 symbols, and 531 edges in 0.098s cold; no-change reindex was 0.027s (`mode=noop`).
- Atlas language counts were `dart:27`.
- graphify rebuilt 314 nodes and 424 links in 0.481s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-dart` status: ok (files:27, parsed_files:27, parse_errors:0, definitions:180, tree_sitter_version:0.25.2, tree_sitter_dart_version:0.1.0).
- Richer native baselines not available on this machine: `dart`, `flutter`, `dart_language_server`.
- Coverage proxy: atlas_vs_tree_sitter_dart_definition_ratio: 1.0, atlas_dart_definition_symbols: 180, native_definitions: 180.
- Optimization cycles: 3 (Dart native tree-sitter AST parsing met the current 5x latency/token thresholds and matched the tree-sitter-dart definition coverage proxy.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Client` | 13.484 | 87.202 | 6.47x | 7 | 42 | 6.0x |
| `BaseClient` | 13.896 | 85.253 | 6.14x | 7 | 101 | 14.43x |
| `Request` | 13.843 | 86.957 | 6.28x | 7 | 51 | 7.29x |
| `Response` | 14.155 | 86.693 | 6.12x | 7 | 59 | 8.43x |
| `send` | 14.192 | 87.197 | 6.14x | 7 | 42 | 6.0x |
| `RetryClient` | 13.433 | 85.696 | 6.38x | 6 | 55 | 9.17x |

5x note: this Dart smoke meets the 5x threshold on equivalent query rows for latency (6.25x) and token output (8.54x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Delphi/Lazarus

Raw artifact: `bench/LIVE_DELPHI_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/fpc/Lazarus` at commit `a37c7c35c6a271b377cd799c48586ea2689a8e0f`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-delphi-lazarus/repo/ide`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-delphi-lazarus/atlas.db --json index /tmp/atlas-live-delphi-lazarus/repo/ide`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-delphi-lazarus/atlas.db --json index /tmp/atlas-live-delphi-lazarus/repo/ide`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `python3 <delphi/lazarus source counter> /tmp/atlas-live-delphi-lazarus/repo/ide`

Results:

- Atlas indexed 594 files, 39171 symbols, and 73200 edges in 7.385s cold; no-change reindex was 0.038s (`mode=noop`).
- Atlas language counts were `pascal:376`, `delphi:206`, `makefile:6`, `xml:5`, `text:1`.
- graphify rebuilt 19383 nodes and 29702 links in 14.155s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `delphi-lazarus-source-counter` status: ok (files:206, parsed_files:206, parse_errors:0, definitions:11308).
- Richer native baselines not available on this machine: `fpc`, `lazbuild`, `pasls`.
- Coverage proxy: atlas_vs_delphi_lazarus_source_counter_definition_ratio: 1.0, atlas_delphi_definition_symbols: 11308, native_definitions: 11308.
- Optimization cycles: 2 (Delphi/Lazarus live smoke replaces the generic one-regex form fallback with a dedicated parser for form component instances, component classes, event handlers, package names, package dependencies, and units.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `AboutForm` | 36.756 | 357.847 | 9.74x | 6 | 50 | 8.33x |
| `FormClose` | 38.859 | 357.931 | 9.21x | 9 | 47 | 5.22x |
| `Notebook` | 37.102 | 351.196 | 9.47x | 7 | 53 | 7.57x |
| `TPageControl` | 45.704 | 355.692 | 7.78x | 9 | 71 | 7.89x |
| `IdeProject` | 36.465 | 355.333 | 9.74x | 9 | 50 | 5.56x |
| `BuildManager` | 36.586 | 359.216 | 9.82x | 8 | 268 | 33.5x |
| `IdePackager` | 37.451 | 350.304 | 9.35x | 9 | 165 | 18.33x |

5x note: this Delphi/Lazarus smoke meets the 5x threshold on equivalent query rows for latency (9.25x) and token output (12.35x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### .NET Project

Raw artifact: `bench/LIVE_DOTNET_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/DapperLib/Dapper` at commit `72a54c475f75e18cb93cba0809d00a5e6e49efd9`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-dotnet-dapper/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-dotnet-dapper/atlas.db --json index /tmp/atlas-live-dotnet-dapper/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-dotnet-dapper/atlas.db --json index /tmp/atlas-live-dotnet-dapper/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/python3 -c <dotnet project counter> /tmp/atlas-live-dotnet-dapper/repo`

Results:

- Atlas indexed 207 files, 3673 symbols, and 8243 edges in 0.645s cold; no-change reindex was 0.024s (`mode=noop`).
- Atlas language counts were `csharp:157`, `text:15`, `dotnet:12`, `markdown:8`, `yaml:6`, `json:4`, `xml:2`, `batch:1`, `config:1`, `powershell:1`.
- graphify rebuilt 2463 nodes and 5135 links in 1.442s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `python-dotnet-project` status: ok (files:12, parsed_files:12, parse_errors:0, definitions:132).
- Richer native baselines not available on this machine: `msbuild`.
- Coverage proxy: atlas_vs_python_dotnet_project_definition_ratio: 1.0, atlas_dotnet_definition_symbols: 132, native_definitions: 132.
- Optimization cycles: 3 (.NET project live smoke matched the Python XML/solution coverage proxy and improved token score after compacting project-file suffixes in terse plain locations while preserving full paths in JSON.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Dapper` | 14.003 | 122.175 | 8.72x | 10 | 56 | 5.6x |
| `Dapper.Tests` | 13.230 | 117.522 | 8.88x | 7 | 57 | 8.14x |
| `Microsoft.NET.Sdk` | 13.610 | 116.902 | 8.59x | 15 | 90 | 6.0x |
| `Microsoft.Bcl.AsyncInterfaces` | 13.182 | 119.961 | 9.1x | 14 | 75 | 5.36x |
| `Dapper.ProviderTools` | 13.238 | 119.179 | 9.0x | 12 | 62 | 5.17x |
| `net8.0` | 13.411 | 121.641 | 9.07x | 9 | 72 | 8.0x |

5x note: this .NET Project smoke meets the 5x threshold on equivalent query rows for latency (8.89x) and token output (6.15x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### EJS

Raw artifact: `bench/LIVE_EJS_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/expressjs/express` at commit `18e5985b8a9d5e8423db0a9121f22bdaecd5b120`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-ejs-express/repo/examples`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-ejs-express/atlas.db --json index /tmp/atlas-live-ejs-express/repo/examples`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-ejs-express/atlas.db --json index /tmp/atlas-live-ejs-express/repo/examples`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `python3 <ejs template counter> /tmp/atlas-live-ejs-express/repo/examples`

Results:

- Atlas indexed 77 files, 251 symbols, and 767 edges in 0.097s cold; no-change reindex was 0.021s (`mode=noop`).
- Atlas language counts were `javascript:43`, `ejs:20`, `css:4`, `html:4`, `text:4`, `markdown:2`.
- graphify rebuilt 86 nodes and 52 links in 0.317s.
- graphify detector-only caveat: `.ejs` is present in `CODE_EXTENSIONS`, but this installed graphify runtime has no `_DISPATCH` extractor for it; graphify query rows are kept as missing-baseline evidence rather than 5x proof.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `ejs-template-counter` status: ok (files:20, parsed_files:20, parse_errors:0, definitions:36).
- Richer native baselines not available on this machine: `ejs`, `ejs-language-server`.
- Coverage proxy: atlas_vs_ejs_template_counter_definition_ratio: 1.0, atlas_ejs_definition_symbols: 36, native_definitions: 36.
- Optimization cycles: 2 (EJS live smoke covers a graphify detector-only extension by adding Atlas template-file, include, function, and variable symbols; graphify has no deterministic EJS extractor in this installed runtime.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `login` | 12.050 | 76.097 | 6.32x | 5 | 8 | 1.6x |
| `index` | 12.325 | 76.349 | 6.19x | 6 | 48 | 8.0x |
| `header` | 12.215 | 75.490 | 6.18x | 6 | 8 | 1.33x |
| `footer` | 12.131 | 75.008 | 6.18x | 5 | 8 | 1.6x |
| `../header` | 11.775 | 75.232 | 6.39x | 7 | 9 | 1.29x |
| `../footer` | 12.129 | 74.627 | 6.15x | 7 | 9 | 1.29x |
| `error_header` | 11.802 | 77.753 | 6.59x | 7 | 9 | 1.29x |

- Query caveat: `login` (graphify_missing), `header` (graphify_missing), `footer` (graphify_missing), `../header` (graphify_missing), `../footer` (graphify_missing), `error_header` (graphify_missing); raw rows remain in the table.
Detector-only saturation note: this EJS smoke proves Atlas indexes the live language slice and matches the native coverage proxy, but it does not prove graphify/native 5x query superiority for this extension because the installed graphify runtime has no deterministic extractor for it.

### Elixir

Raw artifact: `bench/LIVE_ELIXIR_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/phoenixframework/phoenix` at commit `909725968776c2601bbea7827fd5b76e4992cc70`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-elixir-phoenix/repo/lib`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-elixir-phoenix/atlas.db --json index /tmp/atlas-live-elixir-phoenix/repo/lib`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-elixir-phoenix/atlas.db --json index /tmp/atlas-live-elixir-phoenix/repo/lib`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-elixir-phoenix/tree-sitter-elixir-venv/bin/python -c <tree-sitter-elixir definition counter> /tmp/atlas-live-elixir-phoenix/repo/lib`

Results:

- Atlas indexed 74 files, 1642 symbols, and 4897 edges in 0.341s cold; no-change reindex was 0.028s (`mode=noop`).
- Atlas language counts were `elixir:74`.
- graphify rebuilt 1051 nodes and 1844 links in 0.579s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-elixir` status: ok (files:74, parsed_files:74, parse_errors:0, definitions:1642, tree_sitter_version:0.25.2, tree_sitter_elixir_version:0.3.5).
- Richer native baselines not available on this machine: `elixir`, `mix`, `lexical`.
- Coverage proxy: atlas_vs_tree_sitter_elixir_definition_ratio: 1.0, atlas_elixir_definition_symbols: 1642, native_definitions: 1642.
- Optimization cycles: 4 (Elixir native tree-sitter AST parsing matched the tree-sitter-elixir definition coverage proxy exactly and met the current 5x latency/token thresholds; elixir/mix/Lexical remain unavailable on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Phoenix.Router` | 14.423 | 102.743 | 7.12x | 6 | 260 | 43.33x |
| `Phoenix.Endpoint` | 15.261 | 101.131 | 6.63x | 8 | 186 | 23.25x |
| `Phoenix.Controller` | 15.175 | 98.935 | 6.52x | 9 | 275 | 30.56x |
| `path` | 15.774 | 100.239 | 6.35x | 7 | 70 | 10.0x |
| `socket` | 15.585 | 101.003 | 6.48x | 7 | 58 | 8.29x |

5x note: this Elixir smoke meets the 5x threshold on equivalent query rows for latency (6.61x) and token output (22.95x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### ETS/ArkTS

Raw artifact: `bench/LIVE_ETS_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/openharmony/applications_app_samples` at commit `a826ab0e75fe51d028c1c5af58188e908736b53b`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-ets-openharmony-tabs/repo/code/ArkTS1.2/TabsSample/entry/src/main/ets`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-ets-openharmony-tabs/atlas.db --json index /tmp/atlas-live-ets-openharmony-tabs/repo/code/ArkTS1.2/TabsSample/entry/src/main/ets`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-ets-openharmony-tabs/atlas.db --json index /tmp/atlas-live-ets-openharmony-tabs/repo/code/ArkTS1.2/TabsSample/entry/src/main/ets`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `python3 <ets source counter> /tmp/atlas-live-ets-openharmony-tabs/repo/code/ArkTS1.2/TabsSample/entry/src/main/ets`

Results:

- Atlas indexed 18 files, 157 symbols, and 757 edges in 0.117s cold; no-change reindex was 0.021s (`mode=noop`).
- Atlas language counts were `ets:18`.
- graphify rebuilt 0 nodes and 0 links in 0.123s.
- graphify detector-only caveat: `.ets` is present in `CODE_EXTENSIONS`, but this installed graphify runtime has no `_DISPATCH` extractor for it; graphify query rows are kept as missing-baseline evidence rather than 5x proof.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `ets-source-counter` status: ok (files:18, parsed_files:18, parse_errors:0, definitions:153).
- Richer native baselines not available on this machine: `arkts`, `hvigor`, `ohpm`.
- Coverage proxy: atlas_vs_ets_source_counter_definition_ratio: 1.0, atlas_ets_definition_symbols: 153, native_definitions: 153.
- Optimization cycles: 5 (ETS live smoke covers a graphify detector-only extension with ArkTS/ETS declarations while avoiding control-flow false positives; the five-pass saturation report records that graphify exposes no equivalent query rows in this runtime.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `MyStateSample` | 12.235 | 74.375 | 6.08x | 8 | 10 | 1.25x |
| `ComExampleTrivialApplication` | 12.306 | 76.301 | 6.2x | 11 | 13 | 1.18x |
| `WaterFlowDataSource` | 12.173 | 122.841 | 10.09x | 12 | 11 | 0.92x |
| `notifyDataReload` | 13.973 | 81.888 | 5.86x | 12 | 10 | 0.83x |
| `ArticleNode` | 11.965 | 79.414 | 6.64x | 8 | 9 | 1.12x |
| `TabViewComponent` | 13.155 | 79.140 | 6.02x | 8 | 10 | 1.25x |
| `CollapseMenuSection` | 13.363 | 77.271 | 5.78x | 12 | 11 | 0.92x |
| `articleItemBuilder` | 12.108 | 79.298 | 6.55x | 14 | 11 | 0.79x |

- Query caveat: `MyStateSample` (graphify_missing), `ComExampleTrivialApplication` (graphify_missing), `WaterFlowDataSource` (graphify_missing), `notifyDataReload` (graphify_missing), `ArticleNode` (graphify_missing), `TabViewComponent` (graphify_missing), `CollapseMenuSection` (graphify_missing), `articleItemBuilder` (graphify_missing); raw rows remain in the table.
No-equivalent saturation note: this ETS/ArkTS smoke proves Atlas indexes the live language slice and matches the native coverage proxy, but graphify returned no equivalent query rows. Latency/token ratios from missing rows are not treated as 5x evidence; see the saturation loop artifact where applicable.

### Fortran

Raw artifact: `bench/LIVE_FORTRAN_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/fortran-lang/stdlib` at commit `4c8521d5658455a576946cca3bfe2bd8ede36e24`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-fortran-stdlib/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-fortran-stdlib/atlas.db --json index /tmp/atlas-live-fortran-stdlib/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-fortran-stdlib/atlas.db --json index /tmp/atlas-live-fortran-stdlib/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-fortran-stdlib/tree-sitter-fortran-venv/bin/python -c <tree-sitter-fortran definition counter> /tmp/atlas-live-fortran-stdlib/repo/src`

Results:

- Atlas indexed 53 files, 414 symbols, and 4543 edges in 0.16s cold; no-change reindex was 0.02s (`mode=noop`).
- Atlas language counts were `text:29`, `fortran:22`, `c:2`.
- graphify rebuilt 453 nodes and 815 links in 0.718s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-fortran` status: ok (files:22, parsed_files:22, parse_errors:0, definitions:364, tree_sitter_version:0.25.2, tree_sitter_fortran_version:0.6.0).
- Richer native baselines not available on this machine: `gfortran`, `fortls`, `fpm`.
- Coverage proxy: atlas_vs_tree_sitter_fortran_definition_ratio: 1.0, atlas_fortran_definition_symbols: 364, native_definitions: 364.
- Optimization cycles: 2 (Fortran live smoke matched the tree-sitter-fortran definition coverage proxy and met the current 5x latency/token thresholds after widening Atlas Fortran declaration handling.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `stdlib_array` | 11.999 | 79.767 | 6.65x | 7 | 83 | 11.86x |
| `stdlib_datetime` | 12.466 | 80.609 | 6.47x | 9 | 253 | 28.11x |
| `datetime_type` | 12.047 | 79.543 | 6.6x | 9 | 267 | 29.67x |
| `hashmap_type` | 11.816 | 78.883 | 6.68x | 8 | 292 | 36.5x |
| `loading` | 12.066 | 80.689 | 6.69x | 7 | 65 | 9.29x |
| `free_chaining_map` | 12.172 | 79.943 | 6.57x | 14 | 103 | 7.36x |

5x note: this Fortran smoke meets the 5x threshold on equivalent query rows for latency (6.61x) and token output (19.69x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Groovy/Gradle

Raw artifact: `bench/LIVE_GROOVY_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/nextflow-io/nextflow` at commit `83d452a51796aca34f136f796383185d703c349c`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-groovy-nextflow/repo/modules/nf-commons/src/main`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-groovy-nextflow/atlas.db --json index /tmp/atlas-live-groovy-nextflow/repo/modules/nf-commons/src/main`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-groovy-nextflow/atlas.db --json index /tmp/atlas-live-groovy-nextflow/repo/modules/nf-commons/src/main`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-groovy-nextflow/tree-sitter-groovy-venv/bin/python -c <tree-sitter-groovy definition counter> /tmp/atlas-live-groovy-nextflow/repo/modules/nf-commons/src/main`

Results:

- Atlas indexed 100 files, 1049 symbols, and 3712 edges in 0.287s cold; no-change reindex was 0.023s (`mode=noop`).
- Atlas language counts were `groovy:88`, `java:12`.
- graphify rebuilt 742 nodes and 1227 links in 0.937s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-groovy` status: partial (files:88, parsed_files:21, parse_errors:67, definitions:525, tree_sitter_version:0.25.2, tree_sitter_groovy_version:0.1.2).
- Richer native baselines not available on this machine: `groovy`, `gradle`, `groovy-language-server`.
- Coverage proxy: atlas_vs_tree_sitter_groovy_definition_ratio: 1.59, atlas_groovy_definition_symbols: 836, native_definitions: 525.
- Optimization cycles: 3 (Groovy/Gradle live smoke met the current 5x latency/token thresholds after widening Atlas Groovy declaration handling; definition coverage is saturated by tree-sitter-groovy parse errors on real Nextflow files, so the 1.59x native ratio is not claimed as exact recall.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `SysEnv` | 12.605 | 86.908 | 6.89x | 7 | 125 | 17.86x |
| `Const` | 12.385 | 88.607 | 7.15x | 6 | 77 | 12.83x |
| `Duration` | 12.422 | 86.620 | 6.97x | 10 | 36 | 3.6x |
| `getVersion` | 12.340 | 83.422 | 6.76x | 9 | 64 | 7.11x |
| `format` | 13.739 | 85.268 | 6.21x | 8 | 97 | 12.12x |

5x note: this Groovy/Gradle smoke meets the 5x threshold on equivalent query rows for latency (6.79x) and token output (9.97x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### JSON Config

Raw artifact: `bench/LIVE_JSON_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/eslint/create-config` at commit `58d77fc302b25976bb4cc7dc273377b421bc226b`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-json-eslint-create-config/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-json-eslint-create-config/atlas.db --json index /tmp/atlas-live-json-eslint-create-config/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-json-eslint-create-config/atlas.db --json index /tmp/atlas-live-json-eslint-create-config/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/python3 -c <python-json key counter> /tmp/atlas-live-json-eslint-create-config/repo`

Results:

- Atlas indexed 27 files, 252 symbols, and 409 edges in 0.099s cold; no-change reindex was 0.02s (`mode=noop`).
- Atlas language counts were `javascript:13`, `yaml:6`, `json:5`, `markdown:2`, `config:1`.
- graphify rebuilt 184 nodes and 220 links in 0.298s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `python-json` status: ok (files:5, parsed_files:5, parse_errors:0, definitions:60).
- Richer native baselines not available on this machine: `ajv`.
- Coverage proxy: atlas_vs_python_json_definition_ratio: 1.0, atlas_json_definition_symbols: 60, native_definitions: 60.
- Optimization cycles: 2 (JSON config live smoke matched Python stdlib object-key coverage after replacing opaque file-level JSON documents with structured key-path symbols.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `scripts` | 11.970 | 79.198 | 6.62x | 6 | 113 | 18.83x |
| `scripts.test` | 11.486 | 75.439 | 6.57x | 7 | 9 | 1.29x |
| `dependencies` | 11.395 | 74.984 | 6.58x | 7 | 86 | 12.29x |
| `devDependencies` | 11.579 | 75.906 | 6.56x | 8 | 129 | 16.12x |
| `publishConfig` | 11.464 | 75.482 | 6.58x | 8 | 57 | 7.12x |
| `publishConfig.access` | 11.713 | 74.802 | 6.39x | 9 | 11 | 1.22x |

- Query caveat: `scripts.test` (graphify_missing), `publishConfig.access` (graphify_missing); raw rows remain in the table.
5x note: this JSON Config smoke meets the 5x threshold on equivalent query rows for latency (6.58x) and token output (13.28x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Julia

Raw artifact: `bench/LIVE_JULIA_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/JuliaIO/JSON.jl` at commit `e5ef310dece16746843753e4c3b44e868b917b64`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-julia-json/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-julia-json/atlas.db --json index /tmp/atlas-live-julia-json/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-julia-json/atlas.db --json index /tmp/atlas-live-julia-json/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-julia-json/tree-sitter-julia-venv/bin/python -c <tree-sitter-julia definition counter> /tmp/atlas-live-julia-json/repo/src`

Results:

- Atlas indexed 6 files, 310 symbols, and 1049 edges in 0.126s cold; no-change reindex was 0.025s (`mode=noop`).
- Atlas language counts were `julia:6`.
- graphify rebuilt 114 nodes and 179 links in 0.159s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-julia` status: partial (files:6, parsed_files:5, parse_errors:1, definitions:310, tree_sitter_version:0.25.2, tree_sitter_julia_version:0.23.1).
- Richer native baselines not available on this machine: `julia`.
- Coverage proxy: atlas_vs_tree_sitter_julia_definition_ratio: 1.0, atlas_julia_definition_symbols: 310, native_definitions: 310.
- Optimization cycles: 3 (Julia native tree-sitter AST parsing met the current 5x latency/token thresholds and matched the tree-sitter-julia definition coverage proxy.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `JSON` | 12.029 | 70.308 | 5.84x | 3 | 38 | 12.67x |
| `JSONText` | 12.125 | 70.489 | 5.81x | 6 | 51 | 8.5x |
| `Object` | 12.420 | 71.455 | 5.75x | 7 | 75 | 10.71x |
| `parse` | 12.396 | 70.050 | 5.65x | 7 | 167 | 23.86x |
| `json` | 12.339 | 70.521 | 5.72x | 6 | 38 | 6.33x |
| `LazyValue` | 12.144 | 69.943 | 5.76x | 6 | 9 | 1.5x |

- Query caveat: `LazyValue` (graphify_missing); raw rows remain in the table.
5x note: this Julia smoke meets the 5x threshold on equivalent query rows for latency (5.75x) and token output (12.72x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Kotlin

Raw artifact: `bench/LIVE_KOTLIN_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/square/okhttp` at commit `0cadfa2997513d20bf88ca530c963a1266f17af9`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-kotlin-okhttp/repo/okhttp/src/commonJvmAndroid/kotlin`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-kotlin-okhttp/atlas.db --json index /tmp/atlas-live-kotlin-okhttp/repo/okhttp/src/commonJvmAndroid/kotlin`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-kotlin-okhttp/atlas.db --json index /tmp/atlas-live-kotlin-okhttp/repo/okhttp/src/commonJvmAndroid/kotlin`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-kotlin-okhttp/tree-sitter-kotlin-venv/bin/python -c <tree-sitter-kotlin definition counter> /tmp/atlas-live-kotlin-okhttp/repo/okhttp/src/commonJvmAndroid/kotlin`

Results:

- Atlas indexed 138 files, 3901 symbols, and 7504 edges in 0.7s cold; no-change reindex was 0.043s (`mode=noop`).
- Atlas language counts were `kotlin:138`.
- graphify rebuilt 1844 nodes and 5202 links in 1.257s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-kotlin` status: partial (files:138, parsed_files:136, parse_errors:2, definitions:3876, tree_sitter_version:0.25.2, tree_sitter_kotlin_version:1.1.0).
- Richer native baselines not available on this machine: `kotlinc`, `kotlin-language-server`, `ktlint`.
- Coverage proxy: atlas_vs_tree_sitter_kotlin_definition_ratio: 1.0, atlas_kotlin_definition_symbols: 3875, native_definitions: 3876.
- Optimization cycles: 3 (Kotlin live smoke met the current 5x latency/token thresholds and matched the unique tree-sitter-kotlin definition set exactly. The remaining one-count raw gap is a duplicated native counter entry for `connectResult` in `SequentialExchangeFinder.kt`, so this is recorded as a measurement ceiling rather than an Atlas recall miss.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `OkHttpClient` | 17.729 | 129.630 | 7.31x | 8 | 255 | 31.88x |
| `Request` | 16.936 | 120.456 | 7.11x | 6 | 56 | 9.33x |
| `Response` | 17.327 | 117.375 | 6.77x | 6 | 97 | 16.17x |
| `HttpUrl` | 17.174 | 118.643 | 6.91x | 7 | 239 | 34.14x |
| `Headers` | 16.049 | 116.876 | 7.28x | 6 | 246 | 41.0x |

5x note: this Kotlin smoke meets the 5x threshold on equivalent query rows for latency (7.08x) and token output (27.06x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Lua

Raw artifact: `bench/LIVE_LUA_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/folke/lazy.nvim` at commit `306a05526ada86a7b30af95c5cc81ffba93fef97`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-lua-lazy/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-lua-lazy/atlas.db --json index /tmp/atlas-live-lua-lazy/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-lua-lazy/atlas.db --json index /tmp/atlas-live-lua-lazy/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-lua-lazy/luaparser-venv/bin/python -c <luaparser definition counter> /tmp/atlas-live-lua-lazy/repo`

Results:

- Atlas indexed 90 files, 1001 symbols, and 3543 edges in 0.979s cold; no-change reindex was 0.037s (`mode=noop`).
- Atlas language counts were `lua:65`, `yaml:14`, `markdown:4`, `json:3`, `toml:2`, `config:1`, `text:1`.
- graphify rebuilt 1011 nodes and 1288 links in 0.635s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `luaparser` status: ok (files:65, parsed_files:65, parse_errors:0, definitions:444, luaparser_version:4.0.1).
- Richer native baselines not available on this machine: `lua`, `luac`, `luacheck`, `stylua`.
- Coverage proxy: atlas_vs_luaparser_definition_ratio: 1.0, atlas_lua_definition_symbols: 444, native_definitions: 444.
- Optimization cycles: 1 (Lua live smoke exceeded 5x query latency plus token output on equivalent function-symbol queries and exceeded the luaparser named-definition coverage proxy on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Loader.load` | 14.792 | 102.143 | 6.91x | 6 | 146 | 24.33x |
| `Async.new` | 14.810 | 101.411 | 6.85x | 5 | 58 | 11.6x |
| `M.add` | 14.664 | 103.973 | 7.09x | 5 | 65 | 13.0x |
| `M.reload` | 15.532 | 103.940 | 6.69x | 7 | 78 | 11.14x |

5x note: this Lua smoke meets the 5x threshold on equivalent query rows for latency (6.88x) and token output (15.09x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Markdown

Raw artifact: `bench/LIVE_MARKDOWN_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/rust-lang/mdBook` at commit `cb49cc5523e609a731f27dea1af4395a504815a5`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-markdown-mdbook/repo/guide/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-markdown-mdbook/atlas.db --json index /tmp/atlas-live-markdown-mdbook/repo/guide/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-markdown-mdbook/atlas.db --json index /tmp/atlas-live-markdown-mdbook/repo/guide/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-markdown-mdbook/markdown-it-venv/bin/python -c <markdown-it-py heading counter> /tmp/atlas-live-markdown-mdbook/repo/guide/src`

Results:

- Atlas indexed 38 files, 161 symbols, and 27 edges in 0.079s cold; no-change reindex was 0.034s (`mode=noop`).
- Atlas language counts were `markdown:35`, `rust:2`, `toml:1`.
- graphify rebuilt 205 nodes and 249 links in 0.318s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `markdown-it-py` status: ok (files:35, parsed_files:35, parse_errors:0, definitions:156, markdown_it_py_version:4.0.0).
- Richer native baselines not available on this machine: `markdownlint`, `remark`.
- Coverage proxy: atlas_vs_markdown_it_py_definition_ratio: 1.0, atlas_markdown_definition_symbols: 156, native_definitions: 156.
- Optimization cycles: 2 (Markdown live smoke matched the markdown-it-py CommonMark heading coverage proxy after making Atlas section extraction fence-aware; query latency/token ratios are reported against graphify's document parser output.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Installation` | 11.915 | 78.396 | 6.58x | 8 | 95 | 11.88x |
| `Creating a book` | 11.739 | 76.825 | 6.54x | 8 | 88 | 11.0x |
| `The build command` | 12.123 | 77.995 | 6.43x | 7 | 83 | 11.86x |
| `Running `mdbook` in continuous integration` | 11.824 | 75.329 | 6.37x | 18 | 105 | 5.83x |
| `mdBook-specific features` | 11.798 | 75.959 | 6.44x | 9 | 140 | 15.56x |
| `Configuring Renderers` | 12.256 | 94.648 | 7.72x | 9 | 97 | 10.78x |

5x note: this Markdown smoke meets the 5x threshold on equivalent query rows for latency (6.69x) and token output (10.31x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Objective-C

Raw artifact: `bench/LIVE_OBJC_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/SDWebImage/SDWebImage` at commit `c3ad5e1a9bf55c9b76d4c362430b5fcded96c502`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-objc-sdwebimage/repo/SDWebImage`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-objc-sdwebimage/atlas.db --json index /tmp/atlas-live-objc-sdwebimage/repo/SDWebImage`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-objc-sdwebimage/atlas.db --json index /tmp/atlas-live-objc-sdwebimage/repo/SDWebImage`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-objc-sdwebimage/tree-sitter-objc-venv/bin/python -c <tree-sitter-objc definition counter> /tmp/atlas-live-objc-sdwebimage/repo/SDWebImage`

Results:

- Atlas indexed 147 files, 979 symbols, and 2118 edges in 0.238s cold; no-change reindex was 0.04s (`mode=noop`).
- Atlas language counts were `c:75`, `objc:72`.
- graphify rebuilt 1101 nodes and 1009 links in 0.663s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-objc` status: partial (files:72, parsed_files:67, parse_errors:5, definitions:971, tree_sitter_version:0.25.2, tree_sitter_objc_version:3.0.2).
- Coverage proxy: atlas_vs_tree_sitter_objc_definition_ratio: 1.0, atlas_objc_definition_symbols: 971, native_definitions: 971.
- Optimization cycles: 3 (Objective-C native tree-sitter AST parsing matched the graphify-scoped tree-sitter-objc definition coverage proxy and exceeded 5x latency/token thresholds on equivalent rows after preserving full multi-part selectors in Atlas.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `SDImageCache` | 13.762 | 93.471 | 6.79x | 8 | 300 | 37.5x |
| `SDWebImageManager` | 13.876 | 92.040 | 6.63x | 11 | 414 | 37.64x |
| `sharedImageCache` | 13.484 | 92.003 | 6.82x | 8 | 57 | 7.12x |
| `storeImage:forKey:completion:` | 13.547 | 90.015 | 6.64x | 12 | 14 | 1.17x |
| `objectForKey:` | 13.261 | 90.933 | 6.86x | 8 | 56 | 7.0x |

- Query caveat: `storeImage:forKey:completion:` (graphify_missing); raw rows remain in the table.
5x note: this Objective-C smoke meets the 5x threshold on equivalent query rows for latency (6.78x) and token output (23.63x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Pascal

Raw artifact: `bench/LIVE_PASCAL_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/remobjects/pascalscript` at commit `2c826cd803a3c0417354fa37b2417f21993ce4ac`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-pascal-pascalscript/repo/Source`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-pascal-pascalscript/atlas.db --json index /tmp/atlas-live-pascal-pascalscript/repo/Source`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-pascal-pascalscript/atlas.db --json index /tmp/atlas-live-pascal-pascalscript/repo/Source`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/python3 -c <pascal declaration counter> /tmp/atlas-live-pascal-pascalscript/repo/Source`

Results:

- Atlas indexed 105 files, 6487 symbols, and 21350 edges in 1.365s cold; no-change reindex was 0.037s (`mode=noop`).
- Atlas language counts were `pascal:93`, `config:8`, `delphi:2`, `batch:1`, `text:1`.
- graphify rebuilt 5275 nodes and 7437 links in 1.92s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `pascal-regex-counter` status: ok (files:93, parsed_files:93, parse_errors:0, definitions:6431).
- Richer native baselines not available on this machine: `fpc`, `pasls`.
- Coverage proxy: atlas_vs_pascal_regex_counter_definition_ratio: 1.0, atlas_pascal_definition_symbols: 6431, native_definitions: 6431.
- Optimization cycles: 2 (Pascal live smoke matched the declaration-counter coverage proxy after widening Atlas parsing for constructors, destructors, and class methods; stronger Pascal LSP/tree-sitter baselines are recorded as unavailable or unusable on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `TPSPascalCompiler` | 16.064 | 136.972 | 8.53x | 9 | 255 | 28.33x |
| `TPSExec.InnerfuseCall` | 16.454 | 132.532 | 8.05x | 11 | 12 | 1.09x |
| `TPSRuntimeClassImporter` | 15.639 | 135.358 | 8.66x | 11 | 120 | 10.91x |
| `TPSInternalProcedure` | 16.195 | 135.320 | 8.36x | 10 | 109 | 10.9x |
| `RegisterClassLibraryRuntime` | 17.420 | 136.635 | 7.84x | 14 | 81 | 5.79x |

- Query caveat: `TPSExec.InnerfuseCall` (graphify_missing); raw rows remain in the table.
5x note: this Pascal smoke meets the 5x threshold on equivalent query rows for latency (8.33x) and token output (12.84x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### PHP

Raw artifact: `bench/LIVE_PHP_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/slimphp/Slim` at commit `0da7dd2fc66956730b6633f6a056b35e59126583`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-php-slim/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-php-slim/atlas.db --json index /tmp/atlas-live-php-slim/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-php-slim/atlas.db --json index /tmp/atlas-live-php-slim/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/php /tmp/atlas-live-php-slim/php_token_stats.php /tmp/atlas-live-php-slim/repo`

Results:

- Atlas indexed 140 files, 1144 symbols, and 6546 edges in 0.288s cold; no-change reindex was 0.021s (`mode=noop`).
- Atlas language counts were `php:125`, `markdown:8`, `yaml:4`, `config:1`, `json:1`, `xml:1`.
- graphify rebuilt 1152 nodes and 2115 links in 0.582s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `php-tokenizer` status: ok (files:125, parsed_files:125, parse_errors:0, classes:106, interfaces:18, traits:0, enums:0, functions:742, requires:1, namespaces:124, uses:881, use_functions:163, php_version:8.4.14, definitions:866).
- Richer native baselines not available on this machine: `intelephense`, `phpstan`, `psalm`.
- Coverage proxy: atlas_vs_php_tokenizer_definition_ratio: 1.0, atlas_php_definition_symbols: 866, native_definitions: 866.
- Optimization cycles: 2 (PHP live smoke met the current 5x latency/token thresholds and matched PHP tokenizer definition coverage after separating `use function` imports from real function definitions in the native baseline.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `handle` | 14.811 | 108.190 | 7.3x | 7 | 78 | 11.14x |
| `process` | 14.003 | 96.875 | 6.92x | 12 | 116 | 9.67x |
| `addRoute` | 12.723 | 93.636 | 7.36x | 2 | 8 | 4.0x |
| `getResponseFactory` | 14.040 | 96.832 | 6.9x | 12 | 77 | 6.42x |

- Query caveat: `addRoute` (graphify_missing); raw rows remain in the table.
5x note: this PHP smoke meets the 5x threshold on equivalent query rows for latency (7.04x) and token output (8.74x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### PowerShell

Raw artifact: `bench/LIVE_POWERSHELL_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/PowerShell/PowerShellGet` at commit `a2dac8e74603f7c9eec4a54c5e23459531751b0d`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-powershell-powershellget/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-powershell-powershellget/atlas.db --json index /tmp/atlas-live-powershell-powershellget/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-powershell-powershellget/atlas.db --json index /tmp/atlas-live-powershell-powershellget/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/usr/local/bin/pwsh -NoLogo -NoProfile -File /tmp/atlas-live-powershell-powershellget/pwsh_stats.ps1 /tmp/atlas-live-powershell-powershellget/repo/src`

Results:

- Atlas indexed 2 files, 159 symbols, and 805 edges in 0.12s cold; no-change reindex was 0.021s (`mode=noop`).
- Atlas language counts were `powershell:2`.
- graphify rebuilt 30 nodes and 43 links in 0.14s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `pwsh-parser` status: ok (files:2, parsed_files:2, parse_errors:0, functions:28, assignments:391, definitions:28, powershell_version:7.4.6).
- Richer native baselines not available on this machine: `powershell-editor-services`, `psscriptanalyzer`.
- Coverage proxy: atlas_vs_pwsh_parser_definition_ratio: 1.0, atlas_powershell_definition_symbols: 28, native_definitions: 28.
- Optimization cycles: 2 (PowerShell live smoke matched pwsh AST function-definition coverage and improved token score after compacting PowerShell script suffixes in terse plain locations while preserving full paths in JSON.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Find-Module` | 11.894 | 86.563 | 7.28x | 8 | 66 | 8.25x |
| `Install-Module` | 14.181 | 81.918 | 5.78x | 9 | 67 | 7.44x |
| `Register-PSRepository` | 11.968 | 76.330 | 6.38x | 10 | 57 | 5.7x |
| `Update-ModuleManifest` | 12.137 | 75.029 | 6.18x | 10 | 57 | 5.7x |

5x note: this PowerShell smoke meets the 5x threshold on equivalent query rows for latency (6.37x) and token output (6.68x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Razor

Raw artifact: `bench/LIVE_RAZOR_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/dotnet-architecture/eShopOnWeb` at commit `4da8212117e87d808d4bbc7da6286fd2147ce606`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-razor-eshoponweb/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-razor-eshoponweb/atlas.db --json index /tmp/atlas-live-razor-eshoponweb/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-razor-eshoponweb/atlas.db --json index /tmp/atlas-live-razor-eshoponweb/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `python3 <razor directive counter> /tmp/atlas-live-razor-eshoponweb/repo/src`

Results:

- Atlas indexed 333 files, 1283 symbols, and 3113 edges in 0.277s cold; no-change reindex was 0.022s (`mode=noop`).
- Atlas language counts were `csharp:209`, `razor:61`, `css:34`, `json:16`, `dotnet:6`, `dockerfile:2`, `javascript:2`, `markdown:2`, `xml:1`.
- graphify rebuilt 1086 nodes and 1504 links in 2.925s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `razor-directive-counter` status: ok (files:61, parsed_files:61, parse_errors:0, definitions:208).
- Richer native baselines not available on this machine: `razor-language-server`.
- Coverage proxy: atlas_vs_razor_directive_counter_definition_ratio: 1.0, atlas_razor_definition_symbols: 208, native_definitions: 208.
- Optimization cycles: 3 (Razor live smoke matched the directive/component coverage proxy and improved token score after compacting Razor view suffixes in terse plain locations while preserving full paths in JSON.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `EditForm` | 12.185 | 91.115 | 7.48x | 6 | 63 | 10.5x |
| `IJSRuntime` | 13.605 | 90.218 | 6.63x | 6 | 57 | 9.5x |
| `ICatalogItemService` | 12.434 | 90.932 | 7.31x | 8 | 69 | 8.62x |
| `BlazorAdmin.Helpers.BlazorComponent` | 12.479 | 89.684 | 7.19x | 12 | 77 | 6.42x |
| `CreateClick` | 12.565 | 89.172 | 7.1x | 8 | 60 | 7.5x |
| `ConfirmEmailModel` | 12.354 | 89.845 | 7.27x | 9 | 60 | 6.67x |

5x note: this Razor smoke meets the 5x threshold on equivalent query rows for latency (7.15x) and token output (7.88x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Ruby

Raw artifact: `bench/LIVE_RUBY_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/sinatra/sinatra` at commit `5236d3459b8b9015e5ce21ddd0c6beb0db4081d4`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-ruby-sinatra/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-ruby-sinatra/atlas.db --json index /tmp/atlas-live-ruby-sinatra/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-ruby-sinatra/atlas.db --json index /tmp/atlas-live-ruby-sinatra/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/usr/bin/ruby /tmp/atlas-live-ruby-sinatra/ripper_stats.rb /tmp/atlas-live-ruby-sinatra/repo`

Results:

- Atlas indexed 178 files, 1450 symbols, and 3970 edges in 0.272s cold; no-change reindex was 0.025s (`mode=noop`).
- Atlas language counts were `ruby:147`, `markdown:12`, `yaml:10`, `css:4`, `config:3`, `text:2`.
- graphify rebuilt 1281 nodes and 1869 links in 0.721s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `ruby-ripper` status: ok (files:147, parsed_files:147, parse_errors:0, classes:131, modules:138, methods:895, requires:0, ruby_version:2.6.10, definitions:1164).
- Richer native baselines not available on this machine: `solargraph`, `ruby-lsp`.
- Coverage proxy: atlas_vs_ruby_ripper_definition_ratio: 1.01, atlas_ruby_definition_symbols: 1173, native_definitions: 1164.
- Optimization cycles: 2 (Ruby live smoke met the current 5x latency/token thresholds and the Ripper definition coverage proxy after adding operator, receiver-qualified, and ::-qualified module/class parsing.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `initialize` | 13.700 | 90.373 | 6.6x | 8 | 61 | 7.62x |
| `call` | 14.037 | 108.646 | 7.74x | 7 | 89 | 12.71x |
| `route` | 13.280 | 94.267 | 7.1x | 7 | 96 | 13.71x |
| `settings` | 13.291 | 93.757 | 7.05x | 6 | 58 | 9.67x |

5x note: this Ruby smoke meets the 5x threshold on equivalent query rows for latency (7.13x) and token output (10.86x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Rust

Raw artifact: `bench/LIVE_RUST_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/BurntSushi/ripgrep` at commit `dfe4a81d2591daca76d25ae4e052c34b26578155`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-rust-ripgrep/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-rust-ripgrep/atlas.db --json index /tmp/atlas-live-rust-ripgrep/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-rust-ripgrep/atlas.db --json index /tmp/atlas-live-rust-ripgrep/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/rust-analyzer documentSymbol <16 Rust files>`

Results:

- Atlas indexed 161 files, 3515 symbols, and 17253 edges in 1.637s cold; no-change reindex was 0.042s (`mode=noop`).
- Atlas language counts were `rust:100`, `markdown:22`, `toml:13`, `csv:12`, `yaml:5`, `config:4`, `bash:2`, `ruby:1`, `text:1`, `xml:1`.
- graphify rebuilt 3457 nodes and 9148 links in 2.067s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `rust-analyzer` status: ok (sample_files:16, document_symbol_files:16, document_symbols:1310, comparable_document_symbols:1151, definitions:1151, definition_kind_scope:LSP module/class/method/enum/interface/function/constant/struct kinds; fields, variables, enum members, and type parameters excluded, diagnostic_files:0, diagnostics:0).
- Coverage proxy: atlas_vs_rust_analyzer_definition_ratio: 1.0, atlas_rust_definition_symbols: 1153, native_definitions: 1151, coverage_scope: 16 sampled files from rust-analyzer.
- Optimization cycles: 2 (Rust live smoke records reproducible Atlas/graphify measurements and measures sampled source coverage against rust-analyzer documentSymbol when available, with deterministic source-counter fallback.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `HiArgs` | 15.561 | 134.509 | 8.64x | 4 | 249 | 62.25x |
| `LowArgs` | 15.000 | 132.317 | 8.82x | 5 | 258 | 51.6x |
| `PatternMatcher` | 14.436 | 132.168 | 9.16x | 7 | 119 | 17.0x |
| `WalkBuilder` | 17.170 | 142.743 | 8.31x | 6 | 259 | 43.17x |

5x note: this Rust smoke meets the 5x threshold on equivalent query rows for latency (8.71x) and token output (40.23x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### R

Raw artifact: `bench/LIVE_R_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/tidyverse/ggplot2` at commit `6870419aa6e106c3580c45c81d5b688cb31758bd`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-r-ggplot2/repo/R`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-r-ggplot2/atlas.db --json index /tmp/atlas-live-r-ggplot2/repo/R`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-r-ggplot2/atlas.db --json index /tmp/atlas-live-r-ggplot2/repo/R`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `python3 <R source counter> /tmp/atlas-live-r-ggplot2/repo/R`

Results:

- Atlas indexed 201 files, 3010 symbols, and 16377 edges in 1.456s cold; no-change reindex was 0.039s (`mode=noop`).
- Atlas language counts were `r:201`.
- graphify rebuilt 0 nodes and 0 links in 0.278s.
- graphify detector-only caveat: `.r` is present in `CODE_EXTENSIONS`, but this installed graphify runtime has no `_DISPATCH` extractor for it; graphify query rows are kept as missing-baseline evidence rather than 5x proof.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `r-source-counter` status: ok (files:201, parsed_files:201, parse_errors:0, definitions:3011).
- Richer native baselines not available on this machine: `Rscript`.
- Coverage proxy: atlas_vs_r_source_counter_definition_ratio: 1.0, atlas_r_definition_symbols: 3010, native_definitions: 3011.
- Optimization cycles: 6 (R native tree-sitter AST parsing covers graphify's detector-only .r extension and matches r-source-counter functions/types exactly; the single raw-count gap is a source-counter false positive inside a single-quoted string literal, so adding it would reduce Atlas precision.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `ggplot` | 17.443 | 85.977 | 4.93x | 6 | 8 | 1.33x |
| `ggplot.default` | 14.898 | 76.719 | 5.15x | 7 | 10 | 1.43x |
| `GeomPoint` | 15.383 | 77.530 | 5.04x | 7 | 9 | 1.29x |
| `geom_point` | 17.594 | 86.258 | 4.9x | 2 | 9 | 4.5x |
| `StatSummary` | 14.176 | 75.311 | 5.31x | 9 | 9 | 1.0x |
| `theme` | 16.821 | 77.025 | 4.58x | 6 | 8 | 1.33x |
| `aes` | 18.056 | 72.710 | 4.03x | 6 | 7 | 1.17x |
| `coord_cartesian` | 13.903 | 71.674 | 5.16x | 11 | 10 | 0.91x |

- Query caveat: `ggplot` (graphify_missing), `ggplot.default` (graphify_missing), `GeomPoint` (graphify_missing), `geom_point` (graphify_missing), `StatSummary` (graphify_missing), `theme` (graphify_missing), `aes` (graphify_missing), `coord_cartesian` (graphify_missing); raw rows remain in the table.
No-equivalent saturation note: this R smoke proves Atlas indexes the live language slice and matches the native coverage proxy, but graphify returned no equivalent query rows. Latency/token ratios from missing rows are not treated as 5x evidence; see the saturation loop artifact where applicable.

### Scala

Raw artifact: `bench/LIVE_SCALA_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/typelevel/cats` at commit `851965a582940d804f9a23179f58a53fc97f07dc`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-scala-cats/repo/core/src/main/scala`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-scala-cats/atlas.db --json index /tmp/atlas-live-scala-cats/repo/core/src/main/scala`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-scala-cats/atlas.db --json index /tmp/atlas-live-scala-cats/repo/core/src/main/scala`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-scala-cats/tree-sitter-scala-venv/bin/python -c <tree-sitter-scala definition counter> /tmp/atlas-live-scala-cats/repo/core/src/main/scala`

Results:

- Atlas indexed 206 files, 7840 symbols, and 9385 edges in 2.017s cold; no-change reindex was 0.028s (`mode=noop`).
- Atlas language counts were `scala:206`.
- graphify rebuilt 4796 nodes and 18787 links in 2.872s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-scala` status: partial (files:206, parsed_files:204, parse_errors:2, definitions:7840, tree_sitter_version:0.25.2, tree_sitter_scala_version:0.26.0).
- Richer native baselines not available on this machine: `metals`, `scalac`, `scala-cli`.
- Coverage proxy: atlas_vs_tree_sitter_scala_definition_ratio: 1.0, atlas_scala_definition_symbols: 7840, native_definitions: 7840.
- Optimization cycles: 2 (Scala live smoke met the current 5x latency/token thresholds and improved Cats definition coverage after widening Atlas modifier and type-alias handling; Metals/scalac remain unavailable on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Functor` | 18.173 | 176.474 | 9.71x | 6 | 308 | 51.33x |
| `Applicative` | 18.612 | 179.561 | 9.65x | 9 | 272 | 30.22x |
| `Monad` | 19.279 | 181.431 | 9.41x | 6 | 291 | 48.5x |
| `Traverse` | 18.045 | 176.085 | 9.76x | 7 | 91 | 13.0x |
| `Eval` | 20.505 | 174.025 | 8.49x | 4 | 256 | 64.0x |

5x note: this Scala smoke meets the 5x threshold on equivalent query rows for latency (9.38x) and token output (38.06x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### SQL

Raw artifact: `bench/LIVE_SQL_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/hasura/graphql-engine` at commit `417c174c0ac3c80dafe6e8f9e8ac39d868334724`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-sql-hasura/repo/server/src-rsr/migrations`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-sql-hasura/atlas.db --json index /tmp/atlas-live-sql-hasura/repo/server/src-rsr/migrations`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-sql-hasura/atlas.db --json index /tmp/atlas-live-sql-hasura/repo/server/src-rsr/migrations`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-sql-hasura/sqlfluff-venv/bin/sqlfluff parse --dialect postgres <82 SQL files>`

Results:

- Atlas indexed 82 files, 152 symbols, and 857 edges in 0.094s cold; no-change reindex was 0.021s (`mode=noop`).
- Atlas language counts were `sql:82`.
- graphify rebuilt 240 nodes and 194 links in 0.484s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `sqlfluff` status: ok (files:82, parsed_files:82, parse_errors:0, definitions:111, sqlfluff_version:sqlfluff, version 3.5.0).
- Coverage proxy: atlas_vs_sqlfluff_definition_ratio: 1.0, atlas_sql_definition_symbols: 111, native_definitions: 111.
- Optimization cycles: 4 (SQL live smoke matches SQLFluff DDL definition coverage and now exceeds 5x latency/token thresholds after compacting SQL terse locations and warming both tools before measured query rows.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `hdb_catalog.event_triggers` | 12.253 | 77.696 | 6.34x | 10 | 55 | 5.5x |
| `hdb_catalog.hdb_metadata` | 11.626 | 76.589 | 6.59x | 9 | 54 | 6.0x |
| `hdb_catalog.hdb_schema_update_event_notifier` | 12.489 | 76.536 | 6.13x | 16 | 80 | 5.0x |
| `hdb_catalog.hdb_function_agg` | 12.950 | 76.437 | 5.9x | 11 | 56 | 5.09x |

5x note: this SQL smoke meets the 5x threshold on equivalent query rows for latency (6.23x) and token output (5.33x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Svelte

Raw artifact: `bench/LIVE_SVELTE_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/carbon-design-system/carbon-components-svelte` at commit `454a3881cd2b5ac3e47b92d7524a8faa5d99f03d`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-svelte-carbon/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-svelte-carbon/atlas.db --json index /tmp/atlas-live-svelte-carbon/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-svelte-carbon/atlas.db --json index /tmp/atlas-live-svelte-carbon/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/node /tmp/atlas-live-svelte-carbon/svelte-compiler/svelte_stats.js /tmp/atlas-live-svelte-carbon/repo/src`

Results:

- Atlas indexed 424 files, 1923 symbols, and 4184 edges in 0.343s cold; no-change reindex was 0.022s (`mode=noop`).
- Atlas language counts were `svelte:262`, `javascript:120`, `typescript:42`.
- graphify rebuilt 853 nodes and 1521 links in 1.516s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `svelte-compiler` status: ok (files:262, parsed_files:262, parse_errors:0, script_blocks:236, functions:293, variables:985, definitions:1278, compiler_version:5.56.4).
- Richer native baselines not available on this machine: `svelte-check`, `svelte-language-server`.
- Coverage proxy: atlas_vs_svelte_compiler_definition_ratio: 1.04, atlas_svelte_definition_symbols: 1323, native_definitions: 1278.
- Optimization cycles: 1 (Svelte live smoke exceeded 5x query latency plus token output and exceeded the Svelte compiler script-declaration coverage proxy on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `setChar` | 12.954 | 84.556 | 6.53x | 9 | 78 | 8.67x |
| `focusInput` | 12.742 | 84.822 | 6.66x | 10 | 141 | 14.1x |
| `handleInput` | 13.390 | 84.546 | 6.31x | 11 | 88 | 8.0x |
| `handleKeydown` | 14.273 | 84.848 | 5.94x | 12 | 79 | 6.58x |
| `handleOutsideClick` | 13.963 | 85.432 | 6.12x | 12 | 90 | 7.5x |

5x note: this Svelte smoke meets the 5x threshold on equivalent query rows for latency (6.3x) and token output (8.81x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Swift

Raw artifact: `bench/LIVE_SWIFT_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/apple/swift-argument-parser` at commit `8122bc5941426c9494c78ff5ad01951e81c02f53`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-swift-argument-parser/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-swift-argument-parser/atlas.db --json index /tmp/atlas-live-swift-argument-parser/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-swift-argument-parser/atlas.db --json index /tmp/atlas-live-swift-argument-parser/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/usr/bin/sourcekit-lsp --scratch-path /tmp/atlas-live-swift-argument-parser/sourcekit-scratch --default-workspace-type swiftPM`

Results:

- Atlas indexed 238 files, 5926 symbols, and 7430 edges in 1.247s cold; no-change reindex was 0.034s (`mode=noop`).
- Atlas language counts were `swift:165`, `markdown:38`, `text:13`, `json:11`, `bash:5`, `yaml:4`, `config:1`, `image:1`.
- graphify rebuilt 2838 nodes and 6444 links in 1.701s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `sourcekit-lsp` status: ok (sample_files:16, document_symbol_files:16, document_symbols:209, definitions:209, diagnostic_files:0, diagnostics:0, swift_version:Apple Swift version 6.2.4 (swiftlang-6.2.4.1.4 clang-1700.6.4.2)).
- Richer native baselines not available on this machine: `swift-syntax`.
- Coverage proxy: atlas_vs_sourcekit_lsp_definition_ratio: 1.23, atlas_swift_definition_symbols: 257, native_definitions: 209, coverage_scope: 16 sampled files from sourcekit-lsp.
- Optimization cycles: 1 (Swift live smoke met the current 5x latency/token thresholds and exceeded the SourceKit-LSP sampled definition coverage proxy on cycle 1.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `ArgumentParser` | 17.000 | 147.749 | 8.69x | 3 | 332 | 110.67x |
| `parse` | 18.646 | 143.501 | 7.7x | 10 | 139 | 13.9x |
| `run` | 18.878 | 144.785 | 7.67x | 6 | 48 | 8.0x |
| `help` | 18.602 | 147.211 | 7.91x | 7 | 44 | 6.29x |

5x note: this Swift smoke meets the 5x threshold on equivalent query rows for latency (7.98x) and token output (21.65x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Terraform/HCL

Raw artifact: `bench/LIVE_TERRAFORM_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/terraform-aws-modules/terraform-aws-vpc` at commit `3ffbd46fb1c7733e1b34d8666893280454e27436`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-terraform-vpc/repo`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-terraform-vpc/atlas.db --json index /tmp/atlas-live-terraform-vpc/repo`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-terraform-vpc/atlas.db --json index /tmp/atlas-live-terraform-vpc/repo`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-terraform-vpc/hcl2-venv/bin/python -c <hcl2 definition counter> /tmp/atlas-live-terraform-vpc/repo`

Results:

- Atlas indexed 109 files, 2276 symbols, and 1078 edges in 0.329s cold; no-change reindex was 0.022s (`mode=noop`).
- Atlas language counts were `terraform:77`, `markdown:24`, `yaml:6`, `config:1`, `json:1`.
- graphify rebuilt 2461 nodes and 4736 links in 0.921s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `python-hcl2` status: ok (files:77, parsed_files:77, parse_errors:0, definitions:1738, python_hcl2_version:8.1.2).
- Richer native baselines not available on this machine: `terraform`.
- Coverage proxy: atlas_vs_python_hcl2_definition_ratio: 1.0, atlas_terraform_definition_symbols: 1738, native_definitions: 1738.
- Optimization cycles: 2 (Terraform/HCL live smoke matched python-hcl2 definition coverage and exceeded 5x query latency plus token output after installing graphify's optional tree_sitter_hcl parser.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `aws_vpc.this` | 13.376 | 115.136 | 8.61x | 5 | 291 | 58.2x |
| `aws_subnet.public` | 14.151 | 117.631 | 8.31x | 7 | 308 | 44.0x |
| `aws_route_table.public` | 13.102 | 118.337 | 9.03x | 8 | 280 | 35.0x |
| `aws_nat_gateway.this` | 35.727 | 127.937 | 3.58x | 8 | 286 | 35.75x |

5x note: this Terraform/HCL smoke meets the 5x threshold on equivalent query rows for latency (6.27x) and token output (41.61x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Verilog

Raw artifact: `bench/LIVE_VERILOG_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/lowRISC/ibex` at commit `022f084096baed0a9b5ebdf697ed2965f13e8ed8`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-verilog-ibex/repo/rtl`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-verilog-ibex/atlas.db --json index /tmp/atlas-live-verilog-ibex/repo/rtl`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-verilog-ibex/atlas.db --json index /tmp/atlas-live-verilog-ibex/repo/rtl`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-verilog-ibex/tree-sitter-systemverilog-venv/bin/python -c <tree-sitter-systemverilog definition counter> /tmp/atlas-live-verilog-ibex/repo/rtl`

Results:

- Atlas indexed 31 files, 94 symbols, and 2666 edges in 0.117s cold; no-change reindex was 0.028s (`mode=noop`).
- Atlas language counts were `verilog:30`, `fortran:1`.
- graphify rebuilt 170 nodes and 139 links in 0.326s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-systemverilog` status: partial (files:30, parsed_files:20, parse_errors:10, definitions:93, tree_sitter_version:0.25.2, tree_sitter_systemverilog_version:0.3.1).
- Richer native baselines not available on this machine: `verilator`, `slang`, `svlint`.
- Coverage proxy: atlas_vs_tree_sitter_systemverilog_definition_ratio: 1.0, atlas_verilog_definition_symbols: 93, native_definitions: 93.
- Optimization cycles: 3 (Verilog/SystemVerilog live smoke matched the tree-sitter-systemverilog definition coverage proxy and improved token score after compacting HDL source suffixes in terse plain locations while preserving full paths in JSON.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `ibex_core` | 14.042 | 77.328 | 5.51x | 6 | 32 | 5.33x |
| `ibex_top` | 13.436 | 74.677 | 5.56x | 5 | 105 | 21.0x |
| `ibex_pkg` | 11.998 | 75.145 | 6.26x | 5 | 46 | 9.2x |
| `ibex_alu` | 11.944 | 78.387 | 6.56x | 5 | 55 | 11.0x |
| `cm_stack_adj_base` | 11.734 | 76.675 | 6.53x | 12 | 67 | 5.58x |
| `decode_i_insn` | 12.058 | 75.524 | 6.26x | 8 | 53 | 6.62x |

5x note: this Verilog smoke meets the 5x threshold on equivalent query rows for latency (6.09x) and token output (8.73x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Vue

Raw artifact: `bench/LIVE_VUE_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/gothinkster/vue-realworld-example-app` at commit `f7e48c8178602ce25d43293bc6f8ca51d84ae222`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-vue-realworld/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-vue-realworld/atlas.db --json index /tmp/atlas-live-vue-realworld/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-vue-realworld/atlas.db --json index /tmp/atlas-live-vue-realworld/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/opt/homebrew/bin/node /tmp/atlas-live-vue-realworld/vue-compiler/vue_sfc_stats.js /tmp/atlas-live-vue-realworld/repo/src`

Results:

- Atlas indexed 32 files, 165 symbols, and 391 edges in 0.097s cold; no-change reindex was 0.021s (`mode=noop`).
- Atlas language counts were `vue:20`, `javascript:12`.
- graphify rebuilt 111 nodes and 91 links in 0.276s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `vue-compiler-sfc` status: ok (files:20, parsed_files:20, parse_errors:0, script_blocks:21, functions:0, variables:119, definitions:119, compiler_version:3.5.22).
- Richer native baselines not available on this machine: `vue-tsc`, `volar`.
- Coverage proxy: atlas_vs_vue_compiler_sfc_definition_ratio: 1.0, atlas_vue_definition_symbols: 119, native_definitions: 119.
- Optimization cycles: 2 (Vue live smoke matched @vue/compiler-sfc script declaration coverage and improved token score after compacting `.vue` suffixes in terse plain locations while preserving full paths in JSON.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `parseMarkdown` | 12.269 | 76.959 | 6.27x | 7 | 51 | 7.29x |
| `follow` | 12.117 | 74.343 | 6.14x | 6 | 48 | 8.0x |
| `goTo` | 12.177 | 75.946 | 6.24x | 4 | 45 | 11.25x |
| `onPageChange` | 12.068 | 75.255 | 6.24x | 7 | 49 | 7.0x |

5x note: this Vue smoke meets the 5x threshold on equivalent query rows for latency (6.22x) and token output (8.04x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

### Zig

Raw artifact: `bench/LIVE_ZIG_SMOKE.json`. Smoke used a fresh shallow clone of `https://github.com/zigtools/zls` at commit `8da87d4f3305a550e7b739bad764e34bf1e46a08`. `graphify-out/` was removed before Atlas indexed the repo, then graphify was run afterward for the comparison.
Benchmark target: `/tmp/atlas-live-zig-zls/repo/src`.

Commands:

- Atlas index: `bin/atlas --db sqlite:///tmp/atlas-live-zig-zls/atlas.db --json index /tmp/atlas-live-zig-zls/repo/src`
- Atlas no-change reindex: `bin/atlas --db sqlite:///tmp/atlas-live-zig-zls/atlas.db --json index /tmp/atlas-live-zig-zls/repo/src`
- graphify update: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify update .`
- Native baseline: `/tmp/atlas-live-zig-zls/tree-sitter-zig-venv/bin/python -c <tree-sitter-zig definition counter> /tmp/atlas-live-zig-zls/repo/src`

Results:

- Atlas indexed 45 files, 6469 symbols, and 11846 edges in 1.279s cold; no-change reindex was 0.06s (`mode=noop`).
- Atlas language counts were `zig:43`, `json:1`, `markdown:1`.
- graphify rebuilt 1096 nodes and 2536 links in 0.954s.
- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.
- Native baseline `tree-sitter-zig` status: partial (files:43, parsed_files:42, parse_errors:1, definitions:5279, tree_sitter_version:0.25.2, tree_sitter_zig_version:1.1.2).
- Richer native baselines not available on this machine: `zig`, `zls`.
- Coverage proxy: atlas_vs_tree_sitter_zig_definition_ratio: 1.22, atlas_zig_definition_symbols: 6462, native_definitions: 5279.
- Optimization cycles: 2 (Zig live smoke met the current 5x latency/token thresholds and exceeded the tree-sitter-zig definition coverage proxy after widening Atlas Zig declaration handling; zig/zls remain unavailable on this machine.).

| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |
|---|---:|---:|---:|---:|---:|---:|
| `Server` | 20.369 | 105.420 | 5.18x | 6 | 268 | 44.67x |
| `DocumentStore` | 22.171 | 102.605 | 4.63x | 10 | 277 | 27.7x |
| `Analyser` | 17.471 | 106.933 | 6.12x | 7 | 259 | 37.0x |
| `Config` | 17.543 | 106.124 | 6.05x | 8 | 79 | 9.88x |
| `main` | 17.303 | 105.760 | 6.11x | 7 | 202 | 28.86x |

5x note: this Zig smoke meets the 5x threshold on equivalent query rows for latency (5.55x) and token output (28.55x). Accuracy still uses the native/graphify coverage proxies above; this is not a blanket quality claim.

## Tool matrix

| Language | Repo | Atlas | graphify | SCIP | LSP |
|---|---|---|---|---|---|
| go | sirupsen/logrus | 679 symbols, 2102 calls, 0.379s cold full (0.021s delta) | 711 nodes, 333 calls, 0.618s full (0.285s delta) | 2225 symbols, 11887 occ, 0.243s | 12 pkgs, 0 diag, 0.388s |
| python | psf/requests | 517 symbols, 961 calls, 0.17s cold full (0.037s delta) | 580 nodes, 229 calls, 0.591s full (0.27s delta) | 1518 symbols, 8224 occ, 1.995s | 19 files, 12 diag, 0.717s |
| javascript | expressjs/express | 314 symbols, 435 calls, 0.157s cold full (0.022s delta) | 31 nodes, 3 calls, 0.139s full (0.128s delta) | 398 symbols, 2649 occ, 0.914s | 57 files, 257 diag, 0.525s |
| typescript | pmndrs/zustand | 227 symbols, 197 calls, 0.086s cold full (0.025s delta) | 112 nodes, 6 calls, 0.167s full (0.148s delta) | 792 symbols, 2461 occ, 1.059s | 124 files, 1 diag, 0.715s |
| java | google/gson | 1558 symbols, 3105 calls, 0.279s cold full (0.03s delta) | 1016 nodes, 927 calls, 0.909s full (0.567s delta) | missing | 54 doc syms, 425 diag, 1.872s |
| c | DaveGamble/cJSON | 1790 symbols, 4973 calls, 0.354s cold full (0.036s delta) | 971 nodes, 1018 calls, 0.876s full (0.483s delta) | n/a | 258 doc syms, 2 diag, 0.07s |
| cpp | google/leveldb | 2088 symbols, 9481 calls, 0.347s cold full (0.033s delta) | 2206 nodes, 1195 calls, 1.241s full (0.773s delta) | n/a | 421 doc syms, 167 diag, 0.212s |

## Derived Go ratios

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.379s vs graphify FULL extract 0.618s (graphify/Atlas = 1.63x); scip-go cold 0.243s (scip-go/Atlas = 0.64x); gopls (workspace type-check via `gopls stats`) cold 0.388s (gopls/Atlas = 1.02x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.021s vs graphify 0.285s, graphify/Atlas = 13.57x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:6ms, resolve_head:0ms.
- Atlas edge kinds: calls:2102, imports:224, references:622.
- Call coverage proxy: Atlas internal calls 1143 vs graphify calls 333, Atlas/graphify = 3.43x.
- Atlas receiver-typed calls: 632/2102 = 30.1%.
- graphify extracted calls: 198/333 = 59.5%.
- SCIP semantic index: 47 documents, 2225 symbols, 11887 occurrences, 9656 references.
- SCIP navigation symbols (excluding local variables/packages) = 637; Atlas symbols vs SCIP navigation symbols = 1.07x.
- SCIP local variables = 1570. Atlas currently keeps locals out of the first-class symbol table, which lowers token cost but limits fine-grained reference parity.
- gopls workspace truth: 12 workspace packages, 57 compiled Go files, 0 diagnostics, initial load 273.775ms.
- Query token cost (4/4 equivalent rows): graphify 398 tokens vs Atlas 28 tokens, graphify/Atlas = 14.21x.
- Query latency (4/4 equivalent rows): graphify 314.702ms vs Atlas 51.82ms, graphify/Atlas = 6.07x.
- Go cold-build saturation: cold-vs-cold full-index ratio is 1.63x (graphify FULL 0.618s / Atlas cold 0.379s), below 5x; Atlas's largest cold phases are build_symbols_edges:210ms, go_types:209ms, lexical:86ms.

## Derived Python ratios

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.17s vs graphify FULL extract 0.591s (graphify/Atlas = 3.48x); scip-python cold 1.995s (scip-python/Atlas = 11.74x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.037s vs graphify 0.27s, graphify/Atlas = 7.3x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:8ms, resolve_head:0ms.
- Atlas edge kinds: calls:961, imports:410.
- Call coverage proxy: Atlas internal calls 371 vs graphify calls 229, Atlas/graphify = 1.62x.
- graphify extracted calls: 221/229 = 96.5%.
- SCIP semantic index: 19 documents, 1518 symbols, 8224 occurrences, 6739 references, scope=repo-root.
- Atlas symbols vs SCIP symbols = 0.34x. scip-python 0.6.6 reports all Python symbols as UnspecifiedKind, so this is a raw coverage proxy, not navigation-kind parity.
- Python AST callable/class truth: Atlas 320/320 function/method/class symbols = 100.0% recall across 19 files.
- Python AST assignment truth: Atlas 197 assignment symbols vs 133 direct module/class assignment names; extra symbols can come from conditional class scopes.
- Pyright truth pass: 19 files analyzed, 12 diagnostics (error:12), version 1.1.411.
- Query token cost (3/3 equivalent rows): graphify 389 tokens vs Atlas 21 tokens, graphify/Atlas = 18.52x.
- Query latency (3/3 equivalent rows): graphify 249.025ms vs Atlas 41.462ms, graphify/Atlas = 6.01x.
- Python cold-build saturation: cold-vs-cold full-index ratio is 3.48x (graphify FULL 0.591s / Atlas cold 0.17s), below 5x; Atlas's largest cold phases are lexical:103ms, persist:103ms, write_sqlite:103ms.

## Derived JS/TS ratios

### javascript

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.157s vs graphify FULL extract 0.139s (graphify/Atlas = 0.89x); scip-typescript cold 0.914s (scip-typescript/Atlas = 5.82x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.022s vs graphify 0.128s, graphify/Atlas = 5.82x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:0ms, resolve_head:0ms.
- Atlas edge kinds: calls:435, imports:51.
- Call coverage proxy: Atlas internal calls 231 vs graphify calls 3, Atlas/graphify = 77.0x.
- Atlas receiver-typed calls: 0/435 = 0.0%.
- graphify extracted calls: 3/3 = 100.0%.
- SCIP semantic index: 6 documents, 398 symbols, 2649 occurrences, 2251 references, scope=lib.
- Atlas symbols vs SCIP symbols = 0.79x. scip-typescript reports symbols as UnspecifiedKind, so this is a raw coverage proxy.
- TypeScript semantic check proxy: 57 files, 257 diagnostics, total 0.19s, memory 82567KB.
- LSP caveat: tsc returned diagnostics/exit 2; used as scriptable tsserver proxy.
- Query token cost (3/4 equivalent rows): graphify 140 tokens vs Atlas 20 tokens, graphify/Atlas = 7.0x.
- Query latency (3/4 equivalent rows): graphify 213.378ms vs Atlas 38.096ms, graphify/Atlas = 5.6x.
- Query caveat: graphify missed 1 Atlas-selected hub symbols; raw rows remain in the table.
- javascript cold-build saturation: cold-vs-cold full-index ratio is 0.89x (graphify FULL 0.139s / Atlas cold 0.157s), below 5x; Atlas's largest cold phases are lexical:80ms, persist:80ms, write_sqlite:80ms.
### typescript

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.086s vs graphify FULL extract 0.167s (graphify/Atlas = 1.94x); scip-typescript cold 1.059s (scip-typescript/Atlas = 12.31x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.025s vs graphify 0.148s, graphify/Atlas = 5.92x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:4ms, resolve_head:0ms.
- Atlas edge kinds: calls:197, imports:27.
- Call coverage proxy: Atlas internal calls 81 vs graphify calls 6, Atlas/graphify = 13.5x.
- Atlas receiver-typed calls: 8/197 = 4.1%.
- graphify extracted calls: 6/6 = 100.0%.
- SCIP semantic index: 16 documents, 792 symbols, 2461 occurrences, 1669 references, scope=src.
- Atlas symbols vs SCIP symbols = 0.29x. scip-typescript reports symbols as UnspecifiedKind, so this is a raw coverage proxy.
- TypeScript semantic check proxy: 124 files, 1 diagnostics, total 0.14s, memory 72400KB.
- LSP caveat: tsc returned diagnostics/exit 2; used as scriptable tsserver proxy.
- Query token cost (3/4 equivalent rows): graphify 217 tokens vs Atlas 19 tokens, graphify/Atlas = 11.42x.
- Query latency (3/4 equivalent rows): graphify 211.966ms vs Atlas 36.025ms, graphify/Atlas = 5.88x.
- Query caveat: graphify missed 1 Atlas-selected hub symbols; raw rows remain in the table.
- typescript cold-build saturation: cold-vs-cold full-index ratio is 1.94x (graphify FULL 0.167s / Atlas cold 0.086s), below 5x; Atlas's largest cold phases are lexical:24ms, persist:24ms, write_sqlite:24ms.

## Derived Java ratios

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.279s vs graphify FULL extract 0.909s (graphify/Atlas = 3.26x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.03s vs graphify 0.567s, graphify/Atlas = 18.9x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:5ms, resolve_head:0ms.
- Atlas edge kinds: calls:3105, imports:677.
- Call coverage proxy: Atlas internal calls 2403 vs graphify calls 927, Atlas/graphify = 2.59x.
- Atlas receiver-typed calls: 2326/3105 = 74.9%.
- graphify extracted calls: 599/927 = 64.6%.
- JDTLS LSP smoke: initialized against build root gson, sampled 5/5 files, 54 document symbols, 11 workspace symbols for query `Gson`, 425 diagnostics.
- Query token cost (2/2 equivalent rows): graphify 214 tokens vs Atlas 14 tokens, graphify/Atlas = 15.29x.
- Query latency (2/2 equivalent rows): graphify 182.181ms vs Atlas 46.551ms, graphify/Atlas = 3.91x.
- Java cold-build saturation: cold-vs-cold full-index ratio is 3.26x (graphify FULL 0.909s / Atlas cold 0.279s), below 5x; Atlas's largest cold phases are lexical:185ms, persist:185ms, write_sqlite:185ms.

## Derived C/C++ ratios

### c

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.354s vs graphify FULL extract 0.876s (graphify/Atlas = 2.47x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.036s vs graphify 0.483s, graphify/Atlas = 13.42x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:9ms, resolve_head:0ms.
- Atlas edge kinds: calls:4973, imports:400.
- Call coverage proxy: Atlas internal calls 1975 vs graphify calls 1018, Atlas/graphify = 1.94x.
- Atlas receiver-typed calls: 18/4973 = 0.4%.
- graphify extracted calls: 492/1018 = 48.3%.
- clangd LSP smoke: sampled 8/8 files, 258 document symbols, 2 diagnostics.
- Query token cost (4/4 equivalent rows): graphify 1206 tokens vs Atlas 42 tokens, graphify/Atlas = 28.71x.
- Query latency (4/4 equivalent rows): graphify 356.316ms vs Atlas 58.528ms, graphify/Atlas = 6.09x.
- c cold-build saturation: cold-vs-cold full-index ratio is 2.47x (graphify FULL 0.876s / Atlas cold 0.354s), below 5x; Atlas's largest cold phases are lexical:176ms, persist:176ms, write_sqlite:176ms.
### cpp

- Build speed (cold-vs-cold, full index): Atlas COLD full index 0.347s vs graphify FULL extract 1.241s (graphify/Atlas = 3.58x). A ratio < 1.0x means Atlas is slower cold; this is the honest headline.
- Build speed (delta-vs-delta, no-change reindex): Atlas 0.033s vs graphify 0.773s, graphify/Atlas = 23.42x. Both tools re-run against an existing snapshot/sidecar here.
- Atlas index phase timings: delta_check:7ms, resolve_head:0ms.
- Atlas edge kinds: calls:9481, imports:774.
- Call coverage proxy: Atlas internal calls 6217 vs graphify calls 1195, Atlas/graphify = 5.2x.
- Atlas receiver-typed calls: 1959/9481 = 20.7%.
- graphify extracted calls: 1027/1195 = 85.9%.
- clangd LSP smoke: sampled 8/8 files, 421 document symbols, 167 diagnostics.
- Query token cost (4/4 equivalent rows): graphify 320 tokens vs Atlas 32 tokens, graphify/Atlas = 10.0x.
- Query latency (4/4 equivalent rows): graphify 401.87ms vs Atlas 69.942ms, graphify/Atlas = 5.75x.
- cpp cold-build saturation: cold-vs-cold full-index ratio is 3.58x (graphify FULL 1.241s / Atlas cold 0.347s), below 5x; Atlas's largest cold phases are lexical:217ms, persist:217ms, write_sqlite:217ms.

## Warm query latency (persistent server)

Atlas `serve` is started against the already-indexed DB, warmed, then warm HTTP queries are timed. Raw per-call samples are preserved in the JSON (`atlas_warm_serve`). graphify has no warm/server mode, so warm Atlas is NOT divided by any graphify time; the cold-vs-cold CLI latency rows above remain the only Atlas-vs-graphify latency ratio.

| Language | Atlas warm /healthz (median ms) | Atlas warm explain (median ms) | Atlas cold-CLI explain (median ms) | warm speedup (cold/warm) |
|---|--:|--:|--:|--:|
| go | 0.485 | 1.328 | 12.936 | 9.74x |
| python | 0.398 | 1.557 | 13.699 | 8.8x |
| javascript | 0.509 | 1.094 | 12.445 | 11.38x |
| typescript | 0.396 | 0.942 | 12.117 | 12.86x |
| java | 0.495 | 11.168 | 23.276 | 2.08x |
| c | 0.427 | 2.372 | 14.634 | 6.17x |
| cpp | 0.425 | 2.382 | 14.751 | 6.19x |

- go warm-vs-warm context: both Atlas `serve` and gopls run as persistent daemons. Atlas warm explain median is 1.328ms and warm /healthz is 0.485ms. gopls's steady-state per-request latency is measured separately in its LSP smoke (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.
- python warm-vs-warm context: both Atlas `serve` and pyright run as persistent daemons. Atlas warm explain median is 1.557ms and warm /healthz is 0.398ms. pyright's steady-state per-request latency is measured separately in its LSP smoke (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.
- java warm-vs-warm context: both Atlas `serve` and jdtls run as persistent daemons. Atlas warm explain median is 11.168ms and warm /healthz is 0.495ms. jdtls's steady-state per-request latency is measured separately in its LSP smoke (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.
- c warm-vs-warm context: both Atlas `serve` and clangd run as persistent daemons. Atlas warm explain median is 2.372ms and warm /healthz is 0.427ms. clangd's steady-state per-request latency is measured separately in its LSP smoke (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.
- cpp warm-vs-warm context: both Atlas `serve` and clangd run as persistent daemons. Atlas warm explain median is 2.382ms and warm /healthz is 0.425ms. clangd's steady-state per-request latency is measured separately in its LSP smoke (different query semantics: a full Atlas context bundle vs a single LSP method), so the two are reported side by side, not as a single ratio.


## Query token probes

### go

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| log | equivalent | 105 | 5 | 79.33 | 13.152 |
| newEntry | equivalent | 62 | 7 | 79.318 | 12.845 |
| releaseEntry | equivalent | 175 | 8 | 77.82 | 13.027 |
| Fire | equivalent | 56 | 8 | 78.234 | 12.796 |

### python

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| get | equivalent | 142 | 6 | 83.092 | 13.108 |
| request | equivalent | 185 | 6 | 84.61 | 14.655 |
| __init__ | equivalent | 62 | 9 | 81.323 | 13.699 |

### javascript

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| get | equivalent | 47 | 6 | 71.471 | 13.206 |
| sendFile | equivalent | 46 | 7 | 69.931 | 12.463 |
| defineGetter | equivalent | 47 | 7 | 71.976 | 12.427 |
| format | graphify_missing | 8 | 7 | 75.071 | 12.412 |

### typescript

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| DevtoolsImpl | equivalent | 53 | 7 | 70.52 | 11.935 |
| hydrate | graphify_missing | 8 | 7 | 71.132 | 12.903 |
| shallow | equivalent | 75 | 6 | 70.94 | 12.298 |
| CreateStore | equivalent | 89 | 6 | 70.506 | 11.792 |

### java

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| write | equivalent | 112 | 7 | 91.061 | 25.315 |
| read | equivalent | 102 | 7 | 91.12 | 21.236 |

### c

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| cJSON_Delete | equivalent | 284 | 8 | 88.237 | 14.939 |
| cjson_functions_should_not_crash_with_null_pointers | equivalent | 288 | 18 | 90.135 | 15.106 |
| cJSON_CreateObject | equivalent | 332 | 9 | 88.753 | 14.154 |
| UnityPrint | equivalent | 302 | 7 | 89.191 | 14.329 |

### cpp

| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |
|---|---|--:|--:|--:|--:|
| RandomString | equivalent | 76 | 9 | 100.273 | 14.854 |
| MemEnvTest | equivalent | 100 | 8 | 99.906 | 13.401 |
| Size | equivalent | 72 | 9 | 101.36 | 14.648 |
| size | equivalent | 72 | 6 | 100.331 | 27.039 |

## Missing or partial adapters

- java scip-java: missing - command not found: scip-java

---
Generated by `bench/codeintel_matrix.py`. Raw JSON sits next to this report; logs are in `bench/logs/`.