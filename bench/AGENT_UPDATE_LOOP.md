# Agent Edit-Loop Benchmark — graphify vs Atlas

Simulates an AI coding agent's real workflow: after each code change it
runs the graph-update command to refresh its knowledge graph. The edit is
**uncommitted** (working-tree only) — exactly the agent scenario. We measure
per-edit update time, peak RSS, and whether the refreshed graph actually
contains the just-added symbol (accuracy).

- atlas: `/tmp/atlas`
- graphify: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify`
- cycles per repo: 5  |  generated: 2026-06-29T19:44:30+0530
- platform: darwin

update s = median wall-time per edit · RSS = peak resident set · acc = markers_found / markers_added

| lang | atlas update s | atlas RSS MB | atlas acc | graphify update s | graphify RSS MB | graphify acc |
|------|---------------:|-------------:|----------:|------------------:|----------------:|-------------:|
| go | 0.353 | 80.0 | 1.0 (5/5) | 0.5241 | 55.0 | 1.0 (5/5) |
| python | 0.112 | 50.4 | 1.0 (5/5) | 0.5985 | 57.8 | 1.0 (5/5) |
| javascript | 0.069 | 46.8 | 1.0 (5/5) | 0.1412 | 46.1 | 1.0 (5/5) |
| java | 0.301 | 82.6 | 1.0 (5/5) | 0.8082 | 77.9 | 1.0 (5/5) |
| c | 0.349 | 89.5 | 1.0 (5/5) | 0.8114 | 64.8 | 1.0 (5/5) |
| cpp | 0.438 | 108.2 | 1.0 (5/5) | 1.2027 | 99.1 | 1.0 (5/5) |

## Honest summary

- **Speed**: median per-edit update — atlas 0.325s vs graphify 0.703s (across 6 repos).
- **Memory**: peak RSS — atlas 81.3MB vs graphify 61.3MB (median across repos). graphify rebuilds/reloads the whole graph into RAM each update; atlas is incremental.
- **Accuracy (THE catch)**: atlas mean 1.0 vs graphify mean 1.0. Atlas update modes observed: ['delta'].
- Atlas accuracy is 1.0: the incremental update now sees uncommitted edits.
