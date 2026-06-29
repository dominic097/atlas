# Agent Edit-Loop Benchmark — graphify vs Atlas

Simulates an AI coding agent's real workflow: after each code change it
runs the graph-update command to refresh its knowledge graph. The edit is
**uncommitted** (working-tree only) — exactly the agent scenario. We measure
per-edit update time, peak RSS, and whether the refreshed graph actually
contains the just-added symbol (accuracy).

- atlas: `/tmp/atlas`
- graphify: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify`
- cycles per repo: 5  |  generated: 2026-06-29T21:59:34+0530
- platform: darwin

update s = median wall-time per edit · RSS = peak resident set · acc = markers_found / markers_added

| lang | atlas update s | atlas RSS MB | atlas acc | graphify update s | graphify RSS MB | graphify acc |
|------|---------------:|-------------:|----------:|------------------:|----------------:|-------------:|
| go | 0.31 | 80.1 | 1.0 (5/5) | 0.5715 | 55.5 | 1.0 (5/5) |
| python | 0.068 | 49.3 | 1.0 (5/5) | 0.6464 | 59.7 | 1.0 (5/5) |
| javascript | 0.06 | 43.7 | 1.0 (5/5) | 0.1501 | 46.0 | 1.0 (5/5) |
| java | 0.127 | 66.5 | 1.0 (5/5) | 0.848 | 77.9 | 1.0 (5/5) |
| c | 0.189 | 72.9 | 1.0 (5/5) | 0.9144 | 66.1 | 1.0 (5/5) |
| cpp | 0.292 | 90.4 | 1.0 (5/5) | 1.377 | 97.9 | 1.0 (5/5) |

## Honest summary

- **Speed**: median per-edit update — atlas 0.158s vs graphify 0.747s (across 6 repos).
- **Memory**: peak RSS — atlas 69.7MB vs graphify 62.9MB (median across repos). graphify rebuilds/reloads the whole graph into RAM each update; atlas is incremental.
- **Accuracy (THE catch)**: atlas mean 1.0 vs graphify mean 1.0. Atlas update modes observed: ['delta'].
- Atlas accuracy is 1.0: the incremental update now sees uncommitted edits.
