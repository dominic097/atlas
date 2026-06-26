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
| go | sirupsen/logrus | 633 | 696 | 2081 (1118) | 325 (58.5%) | 29.8% | 0.85/1.03s |
| python | psf/requests | 192 | 580 | 961 (212) | 229 (96.5%) | 7.6% | 0.21/0.73s |
| javascript | expressjs/express | 92 | 28 | 435 (108) | 3 (100.0%) | 0.0% | 0.09/0.2s |
| typescript | pmndrs/zustand | 60 | 111 | 197 (58) | 6 (100.0%) | 4.1% | 0.1/0.21s |
| java | google/gson | 771 | 1016 | 3105 (2134) | 927 (64.6%) | 74.9% | 0.41/0.94s |
| c | DaveGamble/cJSON | 1449 | 971 | 4417 (1617) | 1018 (48.3%) | 0.4% | 0.61/1.04s |
| cpp | google/leveldb | 370 | 2206 | 9537 (302) | 1195 (85.9%) | 20.7% | 0.63/1.29s |

## Findings

- **Receiver-type precision leaders (Atlas):** java 74.9%, go 29.8%, cpp 20.7%. Java and Go lead because Atlas type-grounds receivers (Java declared types, Go `go/types`); this is the precision dimension graphify's name-level graph can't express.
- **Atlas symbol under-extraction (a real gap this benchmark exposed):** python 192 vs graphify 580 nodes; typescript 60 vs graphify 111 nodes; cpp 370 vs graphify 2206 nodes. Atlas's tree-sitter symbol extraction for these langs is shallower than graphify's — a precision/coverage gap to close (esp. C++).
- **Call-graph density:** Atlas extracts far more call expressions than graphify reports links (e.g. go 2081 vs 325, python 961 vs 229, javascript 435 vs 3) because Atlas keeps external/unresolved calls too; the *internal* count is the comparable figure.
- **Query token cost:** across all probed hub symbols, graphify totals 3733 tok vs Atlas terse 3257 tok (Atlas terser by 1.15x). Atlas loses on overloaded names (Java create/write) where it returns every real definition; it wins on most single-definition symbols.

## go  —  https://github.com/sirupsen/logrus

target: `/tmp/langbench/go`

**Build**

- atlas: 115 files, 633 symbols, 2989 edges (2081 calls, 1118 internal), 0.85s
- graphify: 696 nodes, 325 call links, 1.03s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 621/2081 = **29.8%**
  - extractor sources: go_ast:2081
- graphify EXTRACTED vs INFERRED: 190/325 = **58.5%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| log | 96 | 83 |
| newEntry | 62 | 75 |
| releaseEntry | 175 | 76 |
| Log | 96 | 96 |

## python  —  https://github.com/psf/requests

target: `/tmp/langbench/python/src`

**Build**

- atlas: 44 files, 192 symbols, 1111 edges (961 calls, 212 internal), 0.21s
- graphify: 580 nodes, 229 call links, 0.73s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 73/961 = **7.6%**
  - extractor sources: python_ts:961
- graphify EXTRACTED vs INFERRED: 221/229 = **96.5%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| Session | 246 | 122 |
| PreparedRequest | 247 | 141 |
| HTTPAdapter | 243 | 120 |
| Response | 242 | 121 |

## javascript  —  https://github.com/expressjs/express

target: `/tmp/langbench/javascript/lib`

**Build**

- atlas: 11 files, 92 symbols, 435 edges (435 calls, 108 internal), 0.09s
- graphify: 28 nodes, 3 call links, 0.2s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 0/435 = **0.0%**
  - extractor sources: javascript_ts:435
- graphify EXTRACTED vs INFERRED: 3/3 = **100.0%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| defineGetter | 47 | 52 |
| resolve | 61 | 38 |
| sendFile | 46 | 43 |
| acceptParams | 56 | 28 |

## typescript  —  https://github.com/pmndrs/zustand

target: `/tmp/langbench/typescript/src`

**Build**

- atlas: 21 files, 60 symbols, 214 edges (197 calls, 58 internal), 0.1s
- graphify: 111 nodes, 6 call links, 0.21s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 8/197 = **4.1%**
  - extractor sources: javascript_ts:197
- graphify EXTRACTED vs INFERRED: 6/6 = **100.0%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| devtoolsImpl | 53 | 86 |
| shallow | 75 | 54 |
| hydrate | 8 | 43 |
| persistImpl | 52 | 55 |

## java  —  https://github.com/google/gson

target: `/tmp/langbench/java/gson/src/main/java`

**Build**

- atlas: 176 files, 771 symbols, 3782 edges (3105 calls, 2134 internal), 0.41s
- graphify: 1016 nodes, 927 call links, 0.94s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 2326/3105 = **74.9%**
  - extractor sources: java_ts:3105
- graphify EXTRACTED vs INFERRED: 599/927 = **64.6%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| TypeAdapters | 173 | 199 |
| TypeAdapter | 84 | 167 |
| create | 64 | 320 |
| write | 112 | 450 |

## c  —  https://github.com/DaveGamble/cJSON

target: `/tmp/langbench/c`

**Build**

- atlas: 254 files, 1449 symbols, 4757 edges (4417 calls, 1617 internal), 0.61s
- graphify: 971 nodes, 1018 call links, 1.04s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 18/4417 = **0.4%**
  - extractor sources: cpp_ts:4275, python_ts:142
- graphify EXTRACTED vs INFERRED: 492/1018 = **48.3%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| cJSON_Delete | 284 | 131 |
| cjson_functions_should_not_crash_with_null_pointers | 288 | 146 |
| cJSON_CreateObject | 332 | 165 |
| UnityPrint | 302 | 120 |

## cpp  —  https://github.com/google/leveldb

target: `/tmp/langbench/cpp`

**Build**

- atlas: 288 files, 370 symbols, 10119 edges (9537 calls, 302 internal), 0.63s
- graphify: 2206 nodes, 1195 call links, 1.29s

**Edge precision**

- atlas method-receiver resolution (OO method calls): 1977/9537 = **20.7%**
  - extractor sources: cpp_ts:9537
- graphify EXTRACTED vs INFERRED: 1027/1195 = **85.9%** high-confidence

**Query token cost** (explain, response tokens)

| symbol | graphify | atlas terse |
|---|--:|--:|
| ExecErrorCheck | 78 | 65 |
| CheckEqual | 84 | 61 |
| Free | 61 | 43 |
| main | 66 | 157 |


---
*Generated by `bench/graphify_vs_atlas.py`. Per-language raw logs in `bench/logs/`.*