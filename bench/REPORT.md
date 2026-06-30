# Atlas vs graphify — per-language benchmark

Both tools build a code knowledge graph on the **same** real single-language repo per language; we measure build, call-graph coverage, edge precision, and query token cost. Tokens ≈ chars/4. Numbers are a one-machine snapshot (timings vary); the graphs themselves are deterministic.

- atlas: `atlas (dev)`
- graphify: graphifyy (PyPI), `graphify update` / `graphify explain` — both offline, no LLM

### Reading the metrics (the two tools model edges differently)

- **Atlas** emits *every* call expression as an edge (incl. calls out to stdlib/3rd-party), then resolves to in-graph targets on demand. *internal calls* = the resolvable subset (ToRef names a known symbol) — the fair coverage axis vs graphify.
- **graphify** keeps only node-to-node (already-internal) call links, each flagged **EXTRACTED** (AST-grounded) or **INFERRED** (guessed). EXTRACTED% is its precision signal.
- **method-receiver%** (Atlas) = call edges that are *method* calls with a resolved receiver type. It is meaningful only for OO method calls — correctly ~0 for procedural C and low for function-heavy JS/Python — and is where Atlas's type grounding (Go `go/types`, Java declared types) shows. It is NOT comparable to graphify's EXTRACTED%; they measure different things.

## Summary

| Lang | Repo | atlas symbols | gfy nodes | atlas calls (internal) | gfy calls (EXTRACTED%) | atlas method-recv% | build a/g |
|---|---|--:|--:|--:|--:|--:|--:|
| go | sirupsen/logrus | 679 | 711 | 2102 (1143) | 333 (59.5%) | 30.1% | 0.81/0.7s |
| python | psf/requests | 517 | 580 | 961 (371) | 229 (96.5%) | 7.6% | 0.18/0.66s |
| javascript | expressjs/express | 312 | 28 | 435 (231) | 3 (100.0%) | 0.0% | 0.09/0.16s |
| typescript | pmndrs/zustand | 226 | 111 | 197 (81) | 6 (100.0%) | 4.1% | 0.12/0.2s |
| java | google/gson | 1558 | 1016 | 3105 (2403) | 927 (64.6%) | 74.9% | 0.27/0.99s |
| c | DaveGamble/cJSON | 1790 | 971 | 4973 (1975) | 1018 (48.3%) | 0.4% | 0.28/0.95s |
| cpp | google/leveldb | 2088 | 2206 | 9481 (6217) | 1195 (85.9%) | 20.7% | 0.29/1.28s |

## Findings

- **Receiver-type precision leaders (Atlas):** java 74.9%, go 30.1%, cpp 20.7%. Java and Go lead because Atlas type-grounds receivers (Java declared types, Go `go/types`); this is the precision dimension graphify's name-level graph can't express.
- **Call-graph density:** Atlas extracts far more call expressions than graphify reports links (e.g. go 2102 vs 333, python 961 vs 229, javascript 435 vs 3) because Atlas keeps external/unresolved calls too; the *internal* count is the comparable figure.
- **Query token cost:** across all probed hub symbols, graphify totals 2899 tok vs Atlas terse 156 tok (Atlas terser by 18.58x). Atlas loses on overloaded names (Java create/write) where it returns every real definition; it wins on most single-definition symbols.

## go  —  https://github.com/sirupsen/logrus

target: `/tmp/atlas-graphify-vs-atlas-final/go`

**Build**

- atlas: 57 files, 679 symbols, 2948 edges (2102 calls, 1143 internal), 0.81s
- graphify: 711 nodes, 333 call links, 0.7s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 632/2102 = **30.1%**
  - extractor sources: go_ast:2102
- graphify EXTRACTED vs INFERRED: 198/333 = **59.5%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| log | 105 | 4 |
| newEntry | 62 | 5 |
| releaseEntry | 175 | 6 |
| Fire | 55 | 6 |

## python  —  https://github.com/psf/requests

target: `/tmp/atlas-graphify-vs-atlas-final/python/src`

**Build**

- atlas: 19 files, 517 symbols, 1371 edges (961 calls, 371 internal), 0.18s
- graphify: 580 nodes, 229 call links, 0.66s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 73/961 = **7.6%**
  - extractor sources: python_ts:961
- graphify EXTRACTED vs INFERRED: 221/229 = **96.5%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| get | 142 | 4 |
| request | 185 | 5 |
| __init__ | 62 | 7 |

## javascript  —  https://github.com/expressjs/express

target: `/tmp/atlas-graphify-vs-atlas-final/javascript/lib`

**Build**

- atlas: 6 files, 312 symbols, 486 edges (435 calls, 231 internal), 0.09s
- graphify: 28 nodes, 3 call links, 0.16s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 0/435 = **0.0%**
  - extractor sources: javascript_ts:435
- graphify EXTRACTED vs INFERRED: 3/3 = **100.0%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| get | 47 | 5 |
| sendFile | 46 | 6 |
| defineGetter | 47 | 6 |
| format | 8 | 6 |

## typescript  —  https://github.com/pmndrs/zustand

target: `/tmp/atlas-graphify-vs-atlas-final/typescript/src`

**Build**

- atlas: 16 files, 226 symbols, 224 edges (197 calls, 81 internal), 0.12s
- graphify: 111 nodes, 6 call links, 0.2s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 8/197 = **4.1%**
  - extractor sources: javascript_ts:197
- graphify EXTRACTED vs INFERRED: 6/6 = **100.0%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| DevtoolsImpl | 53 | 7 |
| hydrate | 8 | 5 |
| shallow | 75 | 5 |
| CreateStore | 89 | 6 |

## java  —  https://github.com/google/gson

target: `/tmp/atlas-graphify-vs-atlas-final/java/gson/src/main/java`

**Build**

- atlas: 86 files, 1558 symbols, 3782 edges (3105 calls, 2403 internal), 0.27s
- graphify: 1016 nodes, 927 call links, 0.99s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 2326/3105 = **74.9%**
  - extractor sources: java_ts:3105
- graphify EXTRACTED vs INFERRED: 599/927 = **64.6%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| write | 112 | 5 |
| read | 102 | 5 |

## c  —  https://github.com/DaveGamble/cJSON

target: `/tmp/atlas-graphify-vs-atlas-final/c`

**Build**

- atlas: 178 files, 1790 symbols, 5373 edges (4973 calls, 1975 internal), 0.28s
- graphify: 971 nodes, 1018 call links, 0.95s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 18/4973 = **0.4%**
  - extractor sources: cpp_ts:4275, call_pattern:556, python_ts:142
- graphify EXTRACTED vs INFERRED: 492/1018 = **48.3%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| cJSON_Delete | 284 | 6 |
| cjson_functions_should_not_crash_with_null_pointers | 288 | 17 |
| cJSON_CreateObject | 332 | 8 |
| UnityPrint | 302 | 5 |

## cpp  —  https://github.com/google/leveldb

target: `/tmp/atlas-graphify-vs-atlas-final/cpp`

**Build**

- atlas: 144 files, 2088 symbols, 10255 edges (9481 calls, 6217 internal), 0.29s
- graphify: 2206 nodes, 1195 call links, 1.28s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 1959/9481 = **20.7%**
  - extractor sources: cpp_ts:9481
- graphify EXTRACTED vs INFERRED: 1027/1195 = **85.9%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| RandomString | 76 | 7 |
| MemEnvTest | 100 | 8 |
| Size | 72 | 7 |
| size | 72 | 5 |


---
*Generated by `bench/graphify_vs_atlas.py`. Per-language raw logs in `bench/logs/`.*