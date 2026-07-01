# Atlas Supported-Language Benchmark

Generated: 2026-07-01T05:45:14Z

This is the language-complete fixture sweep for every `parser.Supported` Atlas family. It is not the old 7-language matrix and it does not use a core-language subset.

## Summary

- Atlas-supported parser families measured: 64
- Atlas index succeeded: 64/64
- Fixture-oracle rows: 62
- Fixture-oracle 100% recall rows: 43/62
- Fixture-oracle 100% precision rows: 39/62
- Exact fixture-oracle rows: 36/62
- Graphify support: {'deterministic': 40, 'detector_only': 3, 'unsupported': 21}
- Graphify run status: {'ok': 43, 'unsupported': 21}
- Native/tool baseline status: {'ok': 22, 'missing': 42}
- Graphify runtime: graphifyy 0.8.49 (`_DISPATCH` entries: 89)

## Rows

| Language | Category | Atlas files/symbols/edges/calls | Oracle recall | Oracle precision | Graphify | Native/tool baseline | Notes |
|---|---|---:|---:|---:|---|---|---|
| go | code | 1/6/4/3 | 1.0 | 1.0 | deterministic/ok nodes=6 calls=2 | go/parser:ok | ok |
| python | code | 1/3/3/2 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=1 | python ast:ok | ok |
| javascript | code | 1/3/1/1 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=1 | acorn:ok | ok |
| typescript | code | 1/6/0/0 | 1.0 | 0.8 | deterministic/ok nodes=6 calls=1 | typescript compiler api:ok | extra: constructor |
| java | code | 1/3/4/3 | 1.0 | 1.0 | deterministic/ok nodes=5 calls=1 | javac:ok | ok |
| c | code | 1/3/1/0 | 1.0 | 1.0 | deterministic/ok nodes=3 calls=1 | clang:ok | ok |
| cpp | code | 1/4/4/3 | 1.0 | 1.0 | deterministic/ok nodes=5 calls=3 | clang++:ok | ok |
| rust | code | 1/4/1/1 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=1 | rustc:ok | ok |
| ruby | code | 1/3/0/0 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=0 | ripper:ok | ok |
| kotlin | code | 1/3/1/1 | 1.0 | 1.0 | deterministic/ok nodes=5 calls=1 | kotlin parser candidate:missing | ok |
| scala | code | 1/3/1/1 | 1.0 | 1.0 | deterministic/ok nodes=5 calls=1 | scala parser candidate:missing | ok |
| php | code | 1/3/1/1 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=1 | php tokenizer:ok | ok |
| swift | code | 1/3/2/1 | 1.0 | 1.0 | deterministic/ok nodes=6 calls=1 | swift parser:missing | ok |
| lua | code | 1/2/2/2 | 0.5 | 0.5 | deterministic/ok nodes=3 calls=1 | lua parser candidate:missing | missing: run; extra: M.run |
| zig | code | 1/5/4/3 | 1.0 | 0.6 | deterministic/ok nodes=4 calls=1 | zig parser candidate:missing | extra: _, std |
| elixir | code | 1/3/2/1 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=1 | elixir parser candidate:missing | ok |
| objc | code | 1/4/3/2 | 0.6667 | 1.0 | deterministic/ok nodes=4 calls=0 | clang objc parse:missing | missing: helper |
| julia | code | 1/4/1/1 | 1.0 | 1.0 | deterministic/ok nodes=6 calls=1 | julia parser candidate:missing | ok |
| fortran | code | 1/3/2/2 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=0 | fortran parser candidate:missing | ok |
| dart | code | 1/3/2/1 | 1.0 | 1.0 | deterministic/ok nodes=5 calls=0 | dart analyzer candidate:missing | ok |
| verilog | code | 1/2/0/0 | 1.0 | 1.0 | deterministic/ok nodes=3 calls=0 | verilog parser candidate:missing | ok |
| pascal | code | 1/3/0/0 | 0.6667 | 0.6667 | deterministic/ok nodes=4 calls=0 | pascal parser candidate:missing | missing: TWorker; extra: TWorker.Run |
| delphi | code | 1/4/0/0 | 1.0 | 0.5 | deterministic/ok nodes=3 calls=0 | delphi form parser proxy:missing | extra: TButton, TMainForm |
| terraform | code | 1/3/0/0 | 1.0 | 1.0 | deterministic/ok nodes=0 calls=0 | hcl parser candidate:missing | ok |
| byond | code | 1/3/3/3 | 0.25 | 0.3333 | deterministic/ok nodes=0 calls=0 | byond source parser proxy:missing | missing: health, helper, run; extra: /proc, /proc/helper |
| dotnet | project | 1/4/2/0 | 1.0 | 0.5 | deterministic/ok nodes=4 calls=0 | dotnet project xml:missing | extra: Microsoft.NET.Sdk, Sample |
| razor | template | 1/4/1/0 | 0.5 | 0.25 | deterministic/ok nodes=4 calls=0 | razor parser proxy:missing | missing: counter; extra: /counter, Counter, Microsoft.AspNetCore.Components |
| apex | code | 1/3/0/0 | 1.0 | 0.6667 | deterministic/ok nodes=3 calls=0 | apex parser candidate:missing | extra: Account |
| blade | template | 1/3/4/3 | 0.5 | 0.3333 | deterministic/ok nodes=1 calls=0 | blade directive parser proxy:missing | missing: foreach; extra: layouts.app, view |
| vue | template | 1/2/2/1 | 1.0 | 1.0 | deterministic/ok nodes=3 calls=0 | vue sfc parser:missing | ok |
| svelte | template | 1/1/0/0 | 0.5 | 1.0 | deterministic/ok nodes=1 calls=0 | svelte compiler:missing | missing: name |
| astro | template | 1/4/1/0 | 1.0 | 0.5 | deterministic/ok nodes=3 calls=0 | astro parser proxy:missing | extra: App, BaseLayout |
| ejs | template | 1/2/0/0 | 0.5 | 0.5 | detector_only/ok nodes=0 calls=0 | ejs compiler:missing | missing: partials/header; extra: view |
| ets | code | 1/4/3/2 | 1.0 | 1.0 | detector_only/ok nodes=0 calls=0 | arkts parser candidate:missing | ok |
| r | code | 1/2/3/1 | 1.0 | 1.0 | detector_only/ok nodes=0 calls=0 | R parser candidate:missing | ok |
| p4 | code | 1/4/0/0 | 1.0 | 0.75 | unsupported/unsupported | p4 parser candidate:missing | extra: start |
| csharp | code | 1/3/1/1 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=1 | csharp compiler/lsp candidate:missing | ok |
| groovy | code | 1/3/2/1 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=1 | groovy parser candidate:missing | ok |
| bash | code | 1/2/1/0 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=1 | bash -n:ok | ok |
| html | markup | 1/0/0/0 | n/a | n/a | unsupported/unsupported | html document parser:missing | ok |
| css | style | 1/0/0/0 | n/a | n/a | unsupported/unsupported | css parser candidate:missing | ok |
| markdown | structured | 1/3/0/0 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=0 | markdown heading parser:missing | ok |
| mdx | structured | 1/2/0/0 | 1.0 | 1.0 | deterministic/ok nodes=3 calls=0 | mdx document parser:missing | ok |
| yaml | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | yaml key parser:missing | missing: name, port, service; extra: config.yaml |
| json | structured | 1/3/0/0 | 1.0 | 1.0 | deterministic/ok nodes=4 calls=0 | python json:ok | ok |
| proto | structured | 1/2/1/0 | 0.6667 | 1.0 | unsupported/unsupported | protoc:ok | missing: Run |
| toml | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | python tomllib:ok | missing: database, name, service, url; extra: config.toml |
| xml | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | python xml:ok | missing: port, project, service; extra: config.xml |
| plist | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | plist parser:missing | missing: CFBundleName; extra: Info.plist |
| gomod | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | go mod parser:missing | missing: example.com/atlas-fixture, github.com/google/uuid; extra: go.mod |
| gosum | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | go sum parser:missing | missing: github.com/google/uuid; extra: go.sum |
| config | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | config key parser:missing | missing: ATLAS_DB, ATLAS_LOG_LEVEL; extra: .env.example |
| makefile | structured | 1/2/0/0 | 1.0 | 1.0 | unsupported/unsupported | make target parser:missing | ok |
| batch | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | batch parser proxy:missing | missing: ATLAS_ENV; extra: build.cmd |
| powershell | code | 1/2/0/0 | 1.0 | 1.0 | deterministic/ok nodes=3 calls=1 | powershell parser:ok | ok |
| sql | structured | 1/3/1/1 | 1.0 | 1.0 | deterministic/ok nodes=3 calls=0 | sqlite parser proxy:missing | ok |
| csv | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | python csv:ok | missing: name, role; extra: data.csv |
| text | structured | 1/1/0/0 | 1.0 | 1.0 | unsupported/unsupported | plain text document parser:missing | ok |
| dockerfile | structured | 1/1/0/0 | 0.0 | 0.0 | unsupported/unsupported | dockerfile parser proxy:missing | missing: ARG, CMD, FROM, RUN; extra: Dockerfile |
| pptx | binary | 1/3/0/0 | 1.0 | 1.0 | unsupported/unsupported | zip office parser:ok | ok |
| docx | binary | 1/1/0/0 | 1.0 | 1.0 | unsupported/unsupported | zip office parser:ok | ok |
| xlsx | binary | 1/1/0/0 | 1.0 | 1.0 | unsupported/unsupported | zip office parser:ok | ok |
| image | binary | 1/1/0/0 | 1.0 | 1.0 | unsupported/unsupported | image decoder:ok | ok |
| pdf | binary | 1/1/0/0 | 1.0 | 1.0 | unsupported/unsupported | pdf parser:ok | ok |

## Ground Truth Closeness

- The fixture oracle is a deterministic name-set oracle for generated fixtures. It is useful for all-family coverage and regression detection, but weaker than independent public-repo semantic ground truth.
- Native/tool baselines are executed when installed locally. Missing rows are explicit and are not counted as wins.
- Graphify comparison is executed for deterministic or detector-only runtime support. Unsupported extensions stay visible as unsupported rows rather than being converted into ratios.
- Precision here is name-set precision for the fixture. It does not prove complete semantic precision, call-edge receiver precision, or all public-repo recall.

## Improvement Areas

| Priority | Language | Area | Detail |
|---|---|---|---|
| P2 | typescript | fixture_oracle_precision_review | constructor |
| P1 | kotlin | native_baseline_missing | No executable local native/tool baseline was found for kotlin; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | scala | native_baseline_missing | No executable local native/tool baseline was found for scala; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | swift | native_baseline_missing | No executable local native/tool baseline was found for swift; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | lua | fixture_oracle_recall_gap | run |
| P2 | lua | fixture_oracle_precision_review | M.run |
| P1 | lua | native_baseline_missing | No executable local native/tool baseline was found for lua; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | zig | fixture_oracle_precision_review | _, std |
| P1 | zig | native_baseline_missing | No executable local native/tool baseline was found for zig; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | elixir | native_baseline_missing | No executable local native/tool baseline was found for elixir; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | objc | fixture_oracle_recall_gap | helper |
| P1 | objc | native_baseline_missing | No executable local native/tool baseline was found for objc; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | julia | native_baseline_missing | No executable local native/tool baseline was found for julia; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | fortran | native_baseline_missing | No executable local native/tool baseline was found for fortran; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | dart | native_baseline_missing | No executable local native/tool baseline was found for dart; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | verilog | native_baseline_missing | No executable local native/tool baseline was found for verilog; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | pascal | fixture_oracle_recall_gap | TWorker |
| P2 | pascal | fixture_oracle_precision_review | TWorker.Run |
| P1 | pascal | native_baseline_missing | No executable local native/tool baseline was found for pascal; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | delphi | fixture_oracle_precision_review | TButton, TMainForm |
| P1 | delphi | native_baseline_missing | No executable local native/tool baseline was found for delphi; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | terraform | native_baseline_missing | No executable local native/tool baseline was found for terraform; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | byond | fixture_oracle_recall_gap | health, helper, run |
| P2 | byond | fixture_oracle_precision_review | /proc, /proc/helper |
| P1 | byond | native_baseline_missing | No executable local native/tool baseline was found for byond; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | dotnet | fixture_oracle_precision_review | Microsoft.NET.Sdk, Sample |
| P1 | dotnet | native_baseline_missing | No executable local native/tool baseline was found for dotnet; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | razor | fixture_oracle_recall_gap | counter |
| P2 | razor | fixture_oracle_precision_review | /counter, Counter, Microsoft.AspNetCore.Components |
| P1 | razor | native_baseline_missing | No executable local native/tool baseline was found for razor; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | apex | fixture_oracle_precision_review | Account |
| P1 | apex | native_baseline_missing | No executable local native/tool baseline was found for apex; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | blade | fixture_oracle_recall_gap | foreach |
| P2 | blade | fixture_oracle_precision_review | layouts.app, view |
| P1 | blade | native_baseline_missing | No executable local native/tool baseline was found for blade; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | vue | native_baseline_missing | No executable local native/tool baseline was found for vue; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | svelte | fixture_oracle_recall_gap | name |
| P1 | svelte | native_baseline_missing | No executable local native/tool baseline was found for svelte; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | astro | fixture_oracle_precision_review | App, BaseLayout |
| P1 | astro | native_baseline_missing | No executable local native/tool baseline was found for astro; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | ejs | fixture_oracle_recall_gap | partials/header |
| P2 | ejs | fixture_oracle_precision_review | view |
| P1 | ejs | native_baseline_missing | No executable local native/tool baseline was found for ejs; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | ets | native_baseline_missing | No executable local native/tool baseline was found for ets; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | r | native_baseline_missing | No executable local native/tool baseline was found for r; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | p4 | fixture_oracle_precision_review | start |
| P1 | p4 | native_baseline_missing | No executable local native/tool baseline was found for p4; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | p4 | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | csharp | native_baseline_missing | No executable local native/tool baseline was found for csharp; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | groovy | native_baseline_missing | No executable local native/tool baseline was found for groovy; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | html | native_baseline_missing | No executable local native/tool baseline was found for html; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | html | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | css | native_baseline_missing | No executable local native/tool baseline was found for css; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | css | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | markdown | native_baseline_missing | No executable local native/tool baseline was found for markdown; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | mdx | native_baseline_missing | No executable local native/tool baseline was found for mdx; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | yaml | fixture_oracle_recall_gap | name, port, service |
| P2 | yaml | fixture_oracle_precision_review | config.yaml |
| P1 | yaml | native_baseline_missing | No executable local native/tool baseline was found for yaml; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | yaml | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | proto | fixture_oracle_recall_gap | Run |
| P2 | proto | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | toml | fixture_oracle_recall_gap | database, name, service, url |
| P2 | toml | fixture_oracle_precision_review | config.toml |
| P2 | toml | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | xml | fixture_oracle_recall_gap | port, project, service |
| P2 | xml | fixture_oracle_precision_review | config.xml |
| P2 | xml | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | plist | fixture_oracle_recall_gap | CFBundleName |
| P2 | plist | fixture_oracle_precision_review | Info.plist |
| P1 | plist | native_baseline_missing | No executable local native/tool baseline was found for plist; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | plist | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | gomod | fixture_oracle_recall_gap | example.com/atlas-fixture, github.com/google/uuid |
| P2 | gomod | fixture_oracle_precision_review | go.mod |
| P1 | gomod | native_baseline_missing | No executable local native/tool baseline was found for gomod; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | gomod | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | gosum | fixture_oracle_recall_gap | github.com/google/uuid |
| P2 | gosum | fixture_oracle_precision_review | go.sum |
| P1 | gosum | native_baseline_missing | No executable local native/tool baseline was found for gosum; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | gosum | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | config | fixture_oracle_recall_gap | ATLAS_DB, ATLAS_LOG_LEVEL |
| P2 | config | fixture_oracle_precision_review | .env.example |
| P1 | config | native_baseline_missing | No executable local native/tool baseline was found for config; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | config | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | makefile | native_baseline_missing | No executable local native/tool baseline was found for makefile; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | makefile | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | batch | fixture_oracle_recall_gap | ATLAS_ENV |
| P2 | batch | fixture_oracle_precision_review | build.cmd |
| P1 | batch | native_baseline_missing | No executable local native/tool baseline was found for batch; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | batch | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | sql | native_baseline_missing | No executable local native/tool baseline was found for sql; fixture oracle and Atlas/Graphify slots are still recorded. |
| P1 | csv | fixture_oracle_recall_gap | name, role |
| P2 | csv | fixture_oracle_precision_review | data.csv |
| P2 | csv | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | text | native_baseline_missing | No executable local native/tool baseline was found for text; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | text | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P1 | dockerfile | fixture_oracle_recall_gap | ARG, CMD, FROM, RUN |
| P2 | dockerfile | fixture_oracle_precision_review | Dockerfile |
| P1 | dockerfile | native_baseline_missing | No executable local native/tool baseline was found for dockerfile; fixture oracle and Atlas/Graphify slots are still recorded. |
| P2 | dockerfile | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P2 | pptx | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P2 | docx | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P2 | xlsx | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P2 | image | graphify_unsupported | not present in Graphify detector/dispatch runtime |
| P2 | pdf | graphify_unsupported | not present in Graphify detector/dispatch runtime |

## Stubs And Hallucination Audit

- No benchmark row is fabricated from prose; every row comes from a generated fixture, an Atlas index run, and recorded Graphify/native-tool status.
- The harness does contain generated fixtures and proxy baselines. Those are labelled as fixture/proxy evidence and should not be described as public-repo 99% ground truth.
- Missing native AST/LSP/compiler tools remain missing in JSON and Markdown. The report does not silently promote fixture-oracle rows into native-tool rows.
- Existing public-repo live artifacts remain the stronger evidence for the 7-language matrix and converted code-language families; this artifact fills the all-supported-family completeness gap.

