# Agent Edit-Loop Benchmark — graphify vs Atlas

Simulates an AI coding agent's real workflow: after each code change it
runs the graph-update command to refresh its knowledge graph. The edit is
**uncommitted** (working-tree only) — exactly the agent scenario. We measure
per-edit update time, peak RSS, and whether the refreshed graph actually
contains the just-added symbol (accuracy).

- atlas: `/tmp/atlas`
- graphify: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify`
- cycles per repo: 5  |  generated: 2026-06-29T23:38:51+0530
- platform: darwin

update s = median wall-time per edit · RSS = peak resident set · acc = markers_found / markers_added

| lang | atlas update s | atlas RSS MB | atlas acc | graphify update s | graphify RSS MB | graphify acc |
|------|---------------:|-------------:|----------:|------------------:|----------------:|-------------:|
| go | 0.291 | 80.2 | 1.0 (5/5) | 0.548 | 55.3 | 1.0 (5/5) |
| python | 0.064 | 48.7 | 1.0 (5/5) | 0.6225 | 60.0 | 1.0 (5/5) |
| java | 0.121 | 65.2 | 1.0 (5/5) | 0.8377 | 78.6 | 1.0 (5/5) |
| cpp | 0.302 | 90.4 | 1.0 (5/5) | 1.3478 | 98.0 | 1.0 (5/5) |

## Honest summary

- **Speed**: median per-edit update — atlas 0.206s vs graphify 0.73s (across 4 repos).
- **Memory**: peak RSS — atlas 72.7MB vs graphify 69.3MB (median across repos). graphify rebuilds/reloads the whole graph into RAM each update; atlas is incremental.
- **Accuracy (THE catch)**: atlas mean 1.0 vs graphify mean 1.0. Atlas update modes observed: ['delta'].
- Atlas accuracy is 1.0: the incremental update now sees uncommitted edits.
