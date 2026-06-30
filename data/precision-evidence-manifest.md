# Precision Evidence Manifest

Generated: 2026-06-30T18:43:24.740Z

This manifest checks the precision evidence already present in raw live benchmark artifacts. It does not fabricate full precision; rows are marked `sampled-name-location`, `kind-count-only`, or `count-only` from observable artifact fields.

## Summary

- Artifacts: 36
- Sampled name/location evidence: 32
- Kind-count-only evidence: 4
- Count-only artifacts: 0
- Equivalent query rows checked: 199
- Query rows with both name and location: 161
- Validation rows with native/Atlas kind maps: 81
- Artifacts with native metric kind maps: 27

| Language | Status | Query name+location | Validation kind rows | Native metric kind map | Gap |
|---|---|--:|--:|---|---|
| apex | sampled-name-location | 12/12 | 3/3 | yes | none |
| astro | sampled-name-location | 6/6 | 3/3 | no | none |
| bash | sampled-name-location | 4/4 | 0/3 | no | none |
| blade | kind-count-only | 0/6 | 3/3 | yes | No comparable query row currently proves both Atlas and Graphify returned the same symbol name with source locations; kind-count evidence is present. |
| byond | sampled-name-location | 5/5 | 3/3 | yes | none |
| csharp | sampled-name-location | 2/4 | 3/3 | yes | none |
| cuda | sampled-name-location | 4/4 | 3/3 | no | none |
| dart | kind-count-only | 0/6 | 0/3 | yes | No comparable query row currently proves both Atlas and Graphify returned the same symbol name with source locations; kind-count evidence is present. |
| delphi | sampled-name-location | 9/9 | 3/3 | yes | none |
| dotnet | sampled-name-location | 7/10 | 3/3 | yes | none |
| ejs | sampled-name-location | 1/1 | 3/3 | yes | none |
| elixir | sampled-name-location | 5/5 | 3/3 | yes | none |
| ets | kind-count-only | 0/5 | 3/3 | yes | No comparable query row currently proves both Atlas and Graphify returned the same symbol name with source locations; kind-count evidence is present. |
| fortran | sampled-name-location | 6/6 | 0/3 | yes | none |
| groovy | sampled-name-location | 5/5 | 3/3 | yes | none |
| json | sampled-name-location | 12/12 | 0/0 | yes | none |
| julia | sampled-name-location | 4/5 | 0/3 | yes | none |
| kotlin | sampled-name-location | 3/5 | 3/3 | yes | none |
| lua | sampled-name-location | 4/4 | 3/3 | yes | none |
| markdown | sampled-name-location | 6/6 | 0/3 | yes | none |
| objc | sampled-name-location | 3/4 | 3/3 | yes | none |
| pascal | sampled-name-location | 11/11 | 3/3 | yes | none |
| php | sampled-name-location | 3/3 | 0/3 | no | none |
| powershell | sampled-name-location | 4/4 | 0/3 | yes | none |
| razor | sampled-name-location | 6/6 | 3/3 | yes | none |
| ruby | sampled-name-location | 4/4 | 3/3 | no | none |
| rust | sampled-name-location | 4/4 | 3/3 | no | none |
| r | kind-count-only | 0/5 | 3/3 | yes | No comparable query row currently proves both Atlas and Graphify returned the same symbol name with source locations; kind-count evidence is present. |
| scala | sampled-name-location | 1/5 | 3/3 | yes | none |
| sql | sampled-name-location | 4/4 | 0/3 | yes | none |
| svelte | sampled-name-location | 5/5 | 3/3 | no | none |
| swift | sampled-name-location | 2/4 | 3/3 | no | none |
| terraform | sampled-name-location | 4/4 | 3/3 | yes | none |
| verilog | sampled-name-location | 6/6 | 3/3 | yes | none |
| vue | sampled-name-location | 5/5 | 3/3 | no | none |
| zig | sampled-name-location | 4/5 | 3/3 | yes | none |

