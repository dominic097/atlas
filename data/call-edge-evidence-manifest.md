# Call Edge Evidence Manifest

Generated: 2026-06-30T19:52:50.142Z

This manifest audits call-edge and receiver-type evidence already present in committed raw benchmark artifacts. It separates receiver-typed core rows from live rows that currently expose only call counts.

## Summary

- Core matrix languages: 7
- Core languages with receiver-typed calls: 7
- Core Atlas calls: 21254
- Core Atlas receiver-typed calls: 5016
- Core receiver-typed ratio: 0.236
- Live artifacts: 36
- Live artifacts with Atlas call counts: 36
- Live artifacts with receiver-typed calls: 0
- Live Atlas calls: 258529
- Live Graphify calls: 23248

## Core Matrix

| Language | Status | Atlas calls | Receiver-typed | Receiver ratio | Internal calls | Graphify calls |
|---|---|--:|--:|--:|--:|--:|
| go | receiver-typed | 2102 | 632 | 0.3007 | 1143 | 333 |
| python | receiver-typed | 961 | 73 | 0.076 | 371 | 229 |
| javascript | receiver-typed | 435 | 0 | 0 | 231 | 3 |
| typescript | receiver-typed | 197 | 8 | 0.0406 | 81 | 6 |
| java | receiver-typed | 3105 | 2326 | 0.7491 | 2403 | 927 |
| c | receiver-typed | 4973 | 18 | 0.0036 | 1975 | 1018 |
| cpp | receiver-typed | 9481 | 1959 | 0.2066 | 6217 | 1195 |

## Live Artifacts

| Language | Status | Atlas calls | Graphify calls | Detector-only Graphify | Note |
|---|---|--:|--:|---|---|
| apex | calls-only | 5006 | 44 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| astro | calls-only | 50 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| bash | calls-only | 102 | 292 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| blade | calls-only | 4062 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| byond | calls-only | 39076 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| csharp | calls-only | 8113 | 690 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| cuda | calls-only | 52 | 2 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| dart | calls-only | 417 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| delphi | calls-only | 65729 | 10195 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| dotnet | calls-only | 8113 | 693 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| ejs | calls-only | 669 | 1 | yes | Raw live artifact exposes call counts but no receiver-type metric. |
| elixir | calls-only | 4783 | n/a | no | Raw live artifact exposes call counts but no receiver-type metric. |
| ets | calls-only | 704 | 0 | yes | Raw live artifact exposes call counts but no receiver-type metric. |
| fortran | calls-only | 4470 | 58 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| groovy | calls-only | 3131 | 358 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| json | calls-only | 359 | 22 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| julia | calls-only | 1038 | 70 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| kotlin | calls-only | 6285 | 1096 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| lua | calls-only | 3347 | 226 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| markdown | calls-only | 19 | 1 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| objc | calls-only | 1828 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| pascal | calls-only | 20943 | 2187 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| php | calls-only | 5713 | 169 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| powershell | calls-only | 804 | 14 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| razor | calls-only | 2327 | 103 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| ruby | calls-only | 3305 | 737 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| rust | calls-only | 15805 | 2657 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| r | calls-only | 16377 | 0 | yes | Raw live artifact exposes call counts but no receiver-type metric. |
| scala | calls-only | 9157 | 1161 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| sql | calls-only | 825 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| svelte | calls-only | 3118 | 114 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| swift | calls-only | 7224 | 994 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| terraform | calls-only | 1078 | 0 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| verilog | calls-only | 2656 | n/a | no | Raw live artifact exposes call counts but no receiver-type metric. |
| vue | calls-only | 280 | 2 | no | Raw live artifact exposes call counts but no receiver-type metric. |
| zig | calls-only | 11564 | 1362 | no | Raw live artifact exposes call counts but no receiver-type metric. |

