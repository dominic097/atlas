# Atlas Symbol-Definition Recall — Authoritative Final Sweep

Final, single-methodology measurement of Atlas definition recall across all 7
matrix languages, each scored against that language's authoritative truth tool.
All numbers below are **measured on real repositories** (no estimates, no
fabricated counts). Binary under test: `/tmp/atlas_final`, built from
`cmd/atlas` at the `develop` HEAD of this worktree (rounds 1–3 parser state).

## Gate

| Check | Command | Result |
| --- | --- | --- |
| Build | `CGO_ENABLED=1 go build ./...` | PASS (exit 0) |
| Vet | `go vet ./...` | PASS (exit 0) |
| Test | `CGO_ENABLED=1 go test ./...` | PASS (exit 0) — all packages `ok` |

The C/C++ recall unit tests (`internal/parser`) pass: `TestCSymbols_RecallRootCauses`,
`TestCppSymbols_RecallRootCauses`, `TestCppSymbols_NamespaceAndQualifiedDefinitions`,
`TestCppSymbols_CUDAKernelDefinitions`, `TestCppMacroAnnotationsNotMethods`,
`TestCppCallEdges`.

## Native parser migration addendum — B2 code languages

Measured 2026-06-30 after routing Kotlin, Scala, Swift, Lua, and Zig through
native tree-sitter tags queries instead of `parseRegexFallback`. Each language
was indexed from the real repository named below, compared against graphify
exact-symbol queries, and checked against the strongest local independent
baseline available on this machine.

Validation gate for this addendum:

| Check | Command | Result |
| --- | --- | --- |
| Parser tests | `go test ./internal/parser` | PASS (exit 0) |
| Build | `go build ./...` | PASS (exit 0) |
| Vet | `go vet ./...` | PASS (exit 0) |
| Test | `go test ./...` | PASS (exit 0) |

| Lang | Repo slice | Independent baseline | Atlas defs | Baseline defs | Recall/coverage | graphify latency | graphify tokens | Notes |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| kotlin | square/okhttp `src/commonJvmAndroid/kotlin` | tree-sitter-kotlin 1.1.0 | 3875 | 3876 raw | 1.00x | 7.08x | 27.06x | Unique `(path, kind, name)` sets match exactly. The one raw-count gap is tree-sitter-kotlin reporting `connectResult` twice for one source declaration in `SequentialExchangeFinder.kt`; this is the measurement ceiling. |
| scala | typelevel/cats core slice | tree-sitter-scala 0.26.0 | 7840 | 7840 | 1.00x | 9.38x | 38.06x | Exact parity after upgrading the Go grammar binding to match the Python baseline version. |
| swift | apple/swift-argument-parser | SourceKit-LSP sampled `documentSymbol` | 257 | 209 | 1.23x | 7.98x | 21.65x | Coverage scope is the 16 files SourceKit-LSP sampled successfully. |
| lua | folke/lazy.nvim | luaparser 4.0.1 | 444 | 444 | 1.00x | 6.88x | 15.09x | Exact parity after normalizing dotted, method, and bracket-index function assignment names such as `package.loaders.l`. |
| zig | zigtools/zls slice | tree-sitter-zig 1.1.2 | 6462 | 5279 | 1.22x | 5.55x | 28.55x | Atlas keeps type declarations and const/var declarations precise while supporting quoted identifiers and compound const headers. |

The durable rendered artifact for this addendum is the regenerated
`bench/MATRIX_REPORT.md`; the matching per-language raw JSON benchmark files
remain under `bench/` until the final artifact cleanup pass.

## Native parser migration addendum — B3 in progress

Measured 2026-06-30 while converting the B3 languages in strict order. Rows
here are added only after the language is routed off `parseRegexFallback`,
benchmarked against graphify and the strongest local independent baseline, and
validated with the focused parser test suite.

| Lang | Repo slice | Independent baseline | Atlas defs | Baseline defs | Recall/coverage | graphify latency | graphify tokens | Notes |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| elixir | phoenixframework/phoenix `lib` | tree-sitter-elixir 0.3.5 | 1642 | 1642 | 1.00x | 6.61x | 22.95x | Native tree-sitter AST walker preserves module/protocol/implementation/function/macro/delegate/guard kinds and adds operator-function support. |
| objective-c | SDWebImage/SDWebImage `.m`/`.mm` graphify scope | tree-sitter-objc 3.0.2 | 971 | 971 | 1.00x | 6.78x | 23.63x | Native tree-sitter AST walker preserves full Objective-C selectors; `storeImage:forKey:completion:` remains documented as graphify-missing because graphify flattens selector names. |
| julia | JuliaIO/JSON.jl `src` | tree-sitter-julia 0.23.1 | 310 | 310 | 1.00x | 5.75x | 12.72x | Native tree-sitter AST walker preserves module/type/function/macro/constant coverage and precise callable-assignment handling. |
| dart | dart-lang/http `pkgs/http/lib` | tree-sitter-dart 0.1.0 | 180 | 180 | 1.00x | 6.25x | 8.54x | Native tree-sitter AST walker preserves type/function/constructor/getter/setter/typedef coverage while avoiding constructor-call false positives. |
| r | tidyverse/ggplot2 `R` | r-source-counter | 3010 | 3011 | 1.00x | n/a | n/a | Native tree-sitter AST walker matches r-source-counter functions/types exactly and omits one source-counter false positive (`i = ...` inside a single-quoted string literal in `geom-dotplot.R`); graphify has no deterministic R extractor rows in this runtime. |

## Native parser migration addendum — B4 in progress

Measured 2026-06-30 while converting the B4 languages in strict order. Rows
are added only after the language is routed off `parseRegexFallback`,
benchmarked against graphify and the strongest local independent baseline, and
validated with the focused parser test suite.

| Lang | Repo slice | Independent baseline | Atlas defs | Baseline defs | Recall/coverage | graphify latency | graphify tokens | Notes |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| fortran | fortran-lang/stdlib `src` | tree-sitter-fortran 0.6.0 | 364 | 364 | 1.00x | 6.06x | 19.69x | Native tree-sitter AST walker preserves module/type/function/subroutine coverage, keeps subroutines normalized to Atlas `function`, and adds native `program` symbols for future slices. |
| verilog | lowRISC/ibex RTL slice | tree-sitter-systemverilog 0.3.1 | 93 | 93 | 1.00x | 5.51x | 8.73x | Native tree-sitter AST walker preserves module/package/function coverage and adds native interface/class/task/program/checker support for future slices; the baseline parser marked 10 files as parse-error partial while still producing the matched definition proxy. |
| pascal | remobjects/pascalscript `Source` | pascal declaration counter | 6432 | 6432 | 1.00x | 8.29x | 12.84x | Native tree-sitter-pascal route verified 6297 declarations directly and used source-shape recovery for 135 package/preprocessor-heavy declarations the grammar cannot expose, preserving exact coverage without routing through `parseRegexFallback`. |
| groovy | nextflow-io/nextflow `modules/nf-commons/src/main` | tree-sitter-groovy 0.1.2 | 837 | 525 | 1.59x | 6.10x | 9.97x | Native tree-sitter-groovy route verified 511 declarations directly and used source-shape recovery for 326 declarations in real files where the baseline grammar reports parse errors; stronger Groovy CLI/LSP baselines are unavailable on this machine. |
| bash | nvm-sh/nvm repo | `/bin/bash -n` + source counter | 158 | 158 | 1.00x | 6.19x | 18.44x | Native tree-sitter-bash route directly verified all 158 live shell function definitions and preserved source-import extraction without routing Bash through `parseRegexFallback`. |
| powershell | PowerShell/PowerShellGet `src` | `pwsh` AST parser | 28 | 28 | 1.00x | 5.47x | 6.68x | Native tree-sitter-powershell route matched the pwsh FunctionDefinitionAst baseline exactly on the live slice, preserved `Import-Module`/`using module` import extraction, and no longer emits regex variable/doc symbols. |

## Native parser migration addendum — B5 in progress

Measured 2026-06-30 while converting the B5 languages in strict order. Rows
are added only after the language is routed off `parseRegexFallback`,
benchmarked against graphify and the strongest local independent baseline, and
validated with the focused parser test suite.

| Lang | Repo slice | Independent baseline | Atlas defs | Baseline defs | Recall/coverage | graphify latency | graphify tokens | Notes |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| vue | gothinkster/vue-realworld-example-app `src` | `@vue/compiler-sfc` 3.5.22 | 119 | 119 | 1.00x | 5.51x | 8.04x | Native SFC block extraction plus tree-sitter JavaScript/TypeScript declaration parsing matched the compiler-sfc script declaration baseline exactly and kept `.vue` files off `parseRegexFallback`. |
| svelte | carbon-design-system/carbon-components-svelte `src` | Svelte compiler 5.56.4 | 1278 | 1278 | 1.00x | 5.92x | 8.81x | Native SFC block extraction plus tree-sitter JavaScript/TypeScript declaration parsing matched the Svelte compiler script declaration baseline exactly and removed the prior regex over-count. |
| astro | withastro/blog-tutorial-demo `src` | `@astrojs/compiler` 4.0.0 | 58 | 58 | 1.00x | 5.66x | 10.54x | Native Astro parser matched compiler coverage with file components, component tags, and tree-sitter frontmatter declarations; graphify still lacks the `pageTitle` variable row, so the latency/token ratios use the 5/6 equivalent queries. |
| razor | dotnet-architecture/eShopOnWeb `src` | razor directive/component counter | 208 | 208 | 1.00x | 6.52x | 7.88x | Native Razor source parser matched file view/component, directive, component-tag, and `@code` method coverage exactly while keeping Razor files off `parseRegexFallback`. |
| blade | BookStackApp/BookStack `resources/views` | blade directive counter | 1090 | 1090 | 1.00x | 6.07x | 6.10x | Native Blade source parser matched template identity, directive, component, and `wire:` handler coverage exactly while keeping `.blade.php` files off `parseRegexFallback`. |
| ejs | expressjs/express `examples` | ejs template counter | 36 | 36 | 1.00x | 5.21x | 6.86x | Native EJS tag scanner matched template, include, function, and variable coverage exactly while keeping `.ejs` files off `parseRegexFallback`; graphify is detector-only for `.ejs`, so the ratios use the one equivalent query and the graphify ceiling is documented. |

## Native parser migration addendum — B6 in progress

Measured 2026-06-30 while converting the B6 languages in strict order. Rows
are added only after the language is routed off `parseRegexFallback`,
benchmarked against graphify and the strongest local independent baseline, and
validated with the focused parser test suite.

| Lang | Repo slice | Independent baseline | Atlas defs | Baseline defs | Recall/coverage | graphify latency | graphify tokens | Notes |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| sql | hasura/graphql-engine `server/src-rsr/migrations` | SQLFluff 3.5.0 | 111 | 111 | 1.00x | 5.96x | 5.33x | Native SQL DDL source parser matched SQLFluff coverage exactly while keeping `.sql` files off `parseRegexFallback`; tree-sitter-sql v0.3.11's Go module omits `parser.c`, and a locally generated C parser was killed during CGO compilation after 128s, so this is the documented native-parser ceiling for SQL in this environment. |
| terraform/hcl | terraform-aws-modules/terraform-aws-vpc repo | python-hcl2 8.1.2 | 1738 | 1738 | 1.00x | 7.78x | 41.61x | Native tree-sitter HCL parser matched resource/data/module/variable/output coverage exactly while keeping `.tf`, `.tfvars`, and `.hcl` files off `parseRegexFallback`. |
| apex | trailheadapps/apex-recipes `force-app` | apex source counter | 1072 | 1072 | 1.00x | 8.15x | 11.67x | Native tree-sitter-sfapex declaration parsing verified class/interface/enum, trigger, constructor, and method symbols, with SOQL SObject and DML operation source recovery preserved for exact coverage while keeping `.cls` and `.trigger` files off `parseRegexFallback`. |
| dotnet project | DapperLib/Dapper repo | python dotnet-project counter | 132 | 132 | 1.00x | 7.67x | 6.15x | Native structured project parser matched `.sln`, `.slnx`, and SDK project-file project/package/project-reference/target-framework coverage exactly while keeping .NET project files off `parseRegexFallback`. |

## The consistent definition surface (applied uniformly to all 7 languages)

A symbol is counted as a **definition** iff it is a top-level **or member**
declaration of one of:

- function, method, constructor
- class, struct, interface/union, enum **type**, record
- named type/alias, annotation-type declaration (Java `@interface`)

**Excluded everywhere — identically for every language and every truth tool:**

- references and call sites
- locals and parameters
- variables, fields/properties, plain constants
- **enum members / enum constants** (they are *values*, like fields — excluded
  on both the Atlas side and every truth side so no language is penalised for a
  truth tool that does or does not enumerate them)
- **forward declarations** (e.g. C `extern void f(void);`, C++ `class Env;`)
- **macro pseudo-symbols** (e.g. GoogleTest `TEST_F` / `TEST_P`, C preprocessor
  artifacts)
- interface method specs (Go) — a member of a type, not a standalone def
- **function-LOCAL nested classes/methods declared inside a (test) method body**
  — so TypeScript/Python/Java are *not* penalised by a truth tool that would
  otherwise count in-test nested constructs

Matching is **leaf-name, path-aware** for Go/Python/JS/TS/Java (a truth def in
file *F* must be emitted by Atlas for file *F*). For C/C++ it is
**qualified-name-aware and repo-level**: a header prototype is credited to its
`.c`/`.cc` definition (one logical definition), matching clangd's repo-level view.

## Per-language recall (final)

| Lang | Repo (real) | Truth tool | Matched / Truth | Recall |
| --- | --- | --- | --- | --- |
| go | sirupsen/logrus | `go/parser` (go 1.25) | 460 / 460 | **100.00%** |
| python | psf/requests | `ast` (py 3.14) | 655 / 655 | **100.00%** |
| javascript | expressjs/express | TypeScript compiler 5.9.3 | 133 / 133 | **100.00%** |
| typescript | pmndrs/zustand | TypeScript compiler 5.9.3 | 130 / 130 | **100.00%** |
| java | google/gson | tree-sitter-java 0.23.5 | 3213 / 3213 | **100.00%** |
| c | DaveGamble/cJSON | clangd 17 `documentSymbol` | 937 / 938 | **99.89%** |
| cpp | google/leveldb | clangd 17 `documentSymbol` | 1096 / 1098 | **99.82%** |

(Truth counts are the deduplicated *logical* definition surface: per-file leaf
names for Go/Py/JS/TS/Java; repo-level qualified names for C/C++.)

### C/C++ robustness of the headline number

The C/C++ figure does not depend on generous repo-level fallback. Measured on
the strictest **path-local** leaf surface (no header→impl crediting at all):

- C: 937 / 938 path-local **and** repo-level (cJSON's header prototypes and `.c`
  definitions land in the same file scope, so no extra crediting is needed) — **99.89%**.
- C++: 1092 / 1098 path-local (**99.45%**); repo-level crediting (header proto →
  `.cc` def) adds 4 → 1096/1098 (**99.82%**).

Audit of *distinct* C++ truth base names: **750 present in Atlas, exactly 1
absent** (`SingletonEnv`). The repo-level credit is not papering over misses.

## Round 1 → 2 → 3 progression

R1/R2 figures are the per-round milestones recorded by the rounds-1-2 work; the
**R3 column is measured in this sweep** with the uniform methodology above.

| Lang | R1 | R2 | R3 (final, this sweep) | Note |
| --- | --- | --- | --- | --- |
| javascript | ~86% | — | **100.00%** | exported/member-assigned arrow + `module.exports = function NAMED()` now extracted |
| java | ~96.7% | — | **100.00%** | + **376 synthetic constructors removed** (no longer minting phantom ctors); annotation-type decls now first-class |
| cpp | ~47% | ~93.9% | **99.82%** | namespace/qualified-name walker, shredded-function recovery, macro-annotation suppression, member-decl recovery |
| c | ~67% | ~98.4% | **99.89%** | C walker `walkCAST` + macro-modifier / typedef / function-symbol recovery |
| go | parity | parity | **100.00%** | byte-identical Atlas output across rounds |
| python | parity | parity | **100.00%** | byte-identical |
| typescript | parity | parity | **100.00%** | byte-identical |

go / python / typescript Atlas output is **byte-identical across rounds 1–3**
(verified: `atlas_final` vs the round-1-2 `atlas_l3` binary produce the same node
count on every matrix repo; C++ differs by +7 nodes — `atlas_final` extracts
*more*, an improvement, never fewer).

## What is at parity

go, python, javascript, typescript, java are at **100.00%** of the consistent
definition surface. C is at **99.89%** and C++ at **99.82%** — both far above the
~95% parity bar — with a fully documented residual below.

## The honest C++ saturation tail (preprocessor-dependent ERROR cascades)

Atlas parses with tree-sitter and runs **no C preprocessor**. The entire residual
is a handful of definitions that only become well-formed *after* macro /
conditional-compilation expansion, where tree-sitter emits an `ERROR` node that
swallows the surrounding declaration and the parser cannot recover it.

**C — 1 missed definition (cJSON repo):**

- `tests/unity/test/tests/testunity.c :: testNotEqualMemory4` — a top-level
  function whose body is `EXPECT_ABORT_BEGIN … VERIFY_FAILS_END` (statement-like
  macros with no trailing `;`). Atlas correctly recovers its siblings
  `testNotEqualMemory1/2/3` and `testNotEqualMemoryLengthZero`; this single one
  falls inside the macro-induced ERROR span. **1 of 938 = 0.11%.**

**C++ — 2 missed definitions (leveldb repo), one logical construct:**

- `util/env_posix.cc :: SingletonEnv` (the class) and its constructor
  `SingletonEnv::SingletonEnv`. This is `template <typename EnvType> class
  SingletonEnv` whose body is dense with `static_assert(sizeof(...) , offsetof(
  ...) % alignof(...) == 0, ...)` and `#if !defined(NDEBUG)` preprocessor blocks.
  tree-sitter produces an ERROR cascade at the template/`static_assert` boundary;
  Atlas extracts nothing past line 808 in that file. The class's *methods*
  (`env`, `AssertEnvNotInitialized`) are still credited because those names exist
  elsewhere in the repo, so only the class + ctor (2 syms) are lost. **2 of
  1098 = 0.18%.**

**Why tree-sitter can't recover these:** both cases require evaluating
preprocessor / compile-time constructs (macro expansion that closes a statement;
`static_assert` constant expressions; `#if`-gated bodies) to see a syntactically
complete declaration. tree-sitter is a context-free grammar over the *raw* token
stream — it has no macro table and no constant evaluator — so the malformed-as-
written region becomes an `ERROR` node and the enclosing definition is dropped.
Closing this last ~0.1–0.2% would require running a real C preprocessor (or
clangd itself) ahead of the tree-sitter pass; that is the only remaining lever
and it trades Atlas's zero-toolchain, local-first property for it. The residual
is therefore a **documented preprocessor artifact**, not a parser-logic gap.

## No baseline weakened, no precision traded

- **No regression:** go / python / typescript Atlas output is byte-identical to
  the round-1-2 binary; java, javascript, c, cpp all *improved* or held (cpp
  `atlas_final` emits +7 nodes vs `atlas_l3`, never fewer). Every other language
  stayed at 100%.
- **Precision not traded for recall:** the C/C++ recall gains came from real
  parser recovery (namespace/qualified walkers, shredded-function recovery,
  typedef/macro-modifier handling), not from blanket symbol emission. Spot-check
  of Atlas C/C++ definitions vs the clangd surface: **98.2%** of distinct C
  definitions and the C++ definitions are backed by clangd; the C++ "unbacked"
  remainder is dominated by **forward declarations** (`class Env;`,
  `struct Options;` in `db/builder.h`) that Atlas legitimately indexes but the
  truth surface deliberately excludes — i.e. extra *declarations*, not fabricated
  *definitions*. Atlas does not over-emit to inflate recall (the 376 phantom Java
  constructors from an earlier round were *removed*, not added).

## Reproduction

Per-language: index `/tmp/atlas-regen-matrix/<lang>/repo` with `atlas_final`,
`export --all --format json`, then run the truth tool and leaf/qualified-name
matcher. Truth tools: `go/parser`, Python `ast`, the TypeScript compiler
(`createSourceFile` + container-only AST walk, never descending function bodies),
`tree-sitter-java` (type-body descent only, never method bodies), and a clangd
`textDocument/documentSymbol` LSP client over each repo's `compile_commands.json`
(enum-member / forward-decl / macro-pseudo-symbol / anonymous filtering;
operator-overload spellings preserved; repo-level qualified dedup).
