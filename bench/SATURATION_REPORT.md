# Atlas benchmark saturation report

This report records repeated live smokes for languages where graphify did not expose equivalent query rows.

- Iterations requested per language: 5
- Atlas binary: `bin/atlas`
- graphify binary: `/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify`

| language | status | iterations | equivalent rows | graphify missing rows | coverage ratio | note |
|---|---|---:|---|---|---|---|
| byond | saturated_no_equivalent_graphify_rows | 5 | 0/6, 0/6, 0/6, 0/6, 0/6 | 6, 6, 6, 6, 6 | 1.0, 1.0, 1.0, 1.0, 1.0 | Five repeated live benchmark attempts produced zero graphify-equivalent query rows while Atlas maintained native coverage >= 1.0; no honest 5x latency/token ratio can be computed. |
| ets | saturated_no_equivalent_graphify_rows | 5 | 0/8, 0/8, 0/8, 0/8, 0/8 | 8, 8, 8, 8, 8 | 1.0, 1.0, 1.0, 1.0, 1.0 | Five repeated live benchmark attempts produced zero graphify-equivalent query rows while Atlas maintained native coverage >= 1.0; no honest 5x latency/token ratio can be computed. |
| r | saturated_no_equivalent_graphify_rows | 5 | 0/8, 0/8, 0/8, 0/8, 0/8 | 8, 8, 8, 8, 8 | 1.0, 1.0, 1.0, 1.0, 1.0 | Five repeated live benchmark attempts produced zero graphify-equivalent query rows while Atlas maintained native coverage >= 1.0; no honest 5x latency/token ratio can be computed. |

Raw JSON sits next to this report at `bench/SATURATION_REPORT.json`.
