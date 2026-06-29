# Real-LLM Code-Q&A Benchmark — Atlas vs graphify vs raw files

**Question:** does a real LLM, given each tool's output as its only context, answer
a code question *correctly*? Not a token proxy — actual Claude inferences answer,
and the answers are scored deterministically against a neutral ground truth.

## Setup (no gaming, neutral truth)

- **Repo:** `sirupsen/logrus` (real-world Go).
- **Question type:** "List the functions that directly call `X`." (`callers`) — the
  canonical code-intel relational question.
- **Ground truth:** `gopls call_hierarchy` (the official Go LSP, go/types-based) —
  neutral to *both* Atlas and graphify. For overloaded names (e.g. the package
  func + `Logger.WithField` + `Entry.WithField` all named `WithField`) the truth is
  the **union** of every real definition's callers, matching a by-name `callers` op.
- **Three contexts, each fed to a real Claude subagent:**
  - `atlas` — Atlas's `callers` op output (full caller list, as an agent gets it via MCP/JSON).
  - `graphify` — `graphify explain X`.
  - `baseline` — the raw definition file (what a tool-less agent would open), capped.
- **Scoring:** the subagent's answer set vs the gopls truth set →
  recall / precision / F1, computed in code (not by an LLM).

Harness: [`bench/llm_qa.py`](llm_qa.py) → [`bench/LLM_QA_SET.json`](LLM_QA_SET.json).
Answering + scoring workflow: `atlas-llm-qa-v2` (16 real subagents per run).

## The fix this measured (callers under-recall)

The first run exposed a real Atlas bug, not a token artifact: Atlas **extracted** 82
call edges to `New` but its `callers` op **resolved only 16** (gopls finds 55).
Qualified calls like `logrus.New()` from `_test`/example/cross-package files were
dropped because `resolveTargets` matched `pkgBase(path) == qualifier`, and
`pkgBase("logger.go") == ""` never equals `"logrus"` (a ROOT package's name is not
recoverable from its path).

**Fix (`29f0bba`):** the Go parser records each symbol's package
(`Metadata["package"]`), and `resolveTargets` matches a qualified call against the
candidate's recorded package. `logrus.New()` now resolves while `errors.New()` stays
external (no in-repo candidate has package `errors`) — recall up, **precision held**.

Resolved-caller counts, before → after (vs gopls truth):

| symbol | before | after | gopls | precision |
|---|--:|--:|--:|--:|
| `New` | 16 | **50** | 55 | 100% (no `errors.New` leak) |
| `WithField` | ~12 | **45** | 46 | 100% |
| `WithFields` | small | **15** | 16 | 100% |

## Result

16 real Claude (`claude-opus-4-8`) subagents answered; 5 symbols × 3 contexts,
scored vs gopls union truth. Averages over the 5 questions:

| context | avg recall | avg precision | avg F1 | avg ctx tokens |
|---|--:|--:|--:|--:|
| **atlas** | **0.952** | **1.000** | **0.975** | **169** |
| graphify | 0.053 | 0.322 | 0.084 | 98 |
| baseline (raw file) | 0.009 | 0.200 | 0.017 | 2227 |

Atlas answers the relational "who calls X" question **~12× more accurately than
graphify and ~57× more accurately than dumping the raw file** — while using **13×
fewer context tokens than the raw file**. Every Atlas answer was precision 1.000
(no hallucinated callers); the only misses are a handful of callers Atlas doesn't
yet resolve (recall 0.95, not 1.0).

Per question (Atlas):

| symbol | recall | precision | F1 | atlas ctx tok | gopls truth |
|---|--:|--:|--:|--:|--:|
| `New` | 0.91 | 1.00 | 0.95 | 308 | 55 |
| `NewEntry` | 0.94 | 1.00 | 0.97 | 93 | 16 |
| `WithField` | 0.98 | 1.00 | 0.99 | 321 | 46 |
| `WithFields` | 0.94 | 1.00 | 0.97 | 103 | 16 |
| `UnmarshalText` | 1.00 | 1.00 | 1.00 | 20 | 2 |

### Before vs after the callers-recall fix (`29f0bba`)

The same benchmark, run **before** the fix, scored Atlas **avgRecall 0.48 /
avgF1 0.387** — Atlas resolved only 16 of 55 `New` callers. The fix (resolve
qualified in-repo package calls) took Atlas to **avgRecall 0.95 / avgF1 0.975**
with precision held at 1.000. That is the "test → diagnose → improve → re-test
with real LLM calls" loop closing on a measured, reproducible accuracy gain:

| | avg recall | avg F1 |
|---|--:|--:|
| atlas (before fix) | 0.48 | 0.387 |
| **atlas (after fix)** | **0.952** | **0.975** |

### Why graphify / raw files lose

- **graphify** `explain` does not surface a complete caller list, so the LLM can
  only guess from prose (recall 0.05, and its few guesses are often wrong →
  precision 0.32).
- **raw file** contains the *definition* of `X`, but callers live in *other*
  files — so the answer is simply not in the context (recall ≈ 0), at the highest
  token cost of the three.

## Reproduce

```sh
# 1. build the Q&A set (gopls truth + the three contexts)
python3 bench/llm_qa.py --atlas ./atlas --graphify graphify \
  --repo /path/to/logrus --db sqlite:///tmp/llmqa.db \
  --symbols New,NewEntry,WithField,WithFields,UnmarshalText --out bench/LLM_QA_SET.json
# 2. answer + score with real LLM subagents via the atlas-llm-qa-v2 workflow
```
