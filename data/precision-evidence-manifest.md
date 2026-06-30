# Precision Evidence Manifest

Generated: 2026-06-30T17:49:06.841Z

This manifest checks the precision evidence already present in raw live benchmark artifacts. It does not fabricate full precision; rows are marked `sampled-name-location`, `kind-count-only`, or `count-only` from observable artifact fields.

## Summary

- Artifacts: 36
- Sampled name/location evidence: 31
- Kind-count-only evidence: 0
- Count-only artifacts: 5
- Equivalent query rows checked: 199
- Query rows with both name and location: 156
- Validation rows with native/Atlas kind maps: 63

| Language | Status | Query name+location | Validation kind rows | Gap |
|---|---|--:|--:|---|
| apex | sampled-name-location | 12/12 | 3/3 | none |
| astro | sampled-name-location | 6/6 | 3/3 | none |
| bash | sampled-name-location | 4/4 | 0/3 | none |
| blade | count-only | 0/6 | 0/3 | Raw artifact has coverage counts but no sampled name/location or native-vs-Atlas kind-count evidence. |
| byond | count-only | 0/5 | 0/3 | Raw artifact has coverage counts but no sampled name/location or native-vs-Atlas kind-count evidence. |
| csharp | sampled-name-location | 2/4 | 3/3 | none |
| cuda | sampled-name-location | 4/4 | 3/3 | none |
| dart | count-only | 0/6 | 0/3 | Raw artifact has coverage counts but no sampled name/location or native-vs-Atlas kind-count evidence. |
| delphi | sampled-name-location | 9/9 | 3/3 | none |
| dotnet | sampled-name-location | 7/10 | 3/3 | none |
| ejs | sampled-name-location | 1/1 | 3/3 | none |
| elixir | sampled-name-location | 5/5 | 3/3 | none |
| ets | count-only | 0/5 | 0/3 | Raw artifact has coverage counts but no sampled name/location or native-vs-Atlas kind-count evidence. |
| fortran | sampled-name-location | 6/6 | 0/3 | none |
| groovy | sampled-name-location | 5/5 | 3/3 | none |
| json | sampled-name-location | 12/12 | 0/0 | none |
| julia | sampled-name-location | 4/5 | 0/3 | none |
| kotlin | sampled-name-location | 3/5 | 3/3 | none |
| lua | sampled-name-location | 4/4 | 3/3 | none |
| markdown | sampled-name-location | 6/6 | 0/3 | none |
| objc | sampled-name-location | 3/4 | 3/3 | none |
| pascal | sampled-name-location | 11/11 | 0/3 | none |
| php | sampled-name-location | 3/3 | 0/3 | none |
| powershell | sampled-name-location | 4/4 | 0/3 | none |
| razor | sampled-name-location | 6/6 | 3/3 | none |
| ruby | sampled-name-location | 4/4 | 3/3 | none |
| rust | sampled-name-location | 4/4 | 3/3 | none |
| r | count-only | 0/5 | 0/3 | Raw artifact has coverage counts but no sampled name/location or native-vs-Atlas kind-count evidence. |
| scala | sampled-name-location | 1/5 | 3/3 | none |
| sql | sampled-name-location | 4/4 | 0/3 | none |
| svelte | sampled-name-location | 5/5 | 3/3 | none |
| swift | sampled-name-location | 2/4 | 3/3 | none |
| terraform | sampled-name-location | 4/4 | 0/3 | none |
| verilog | sampled-name-location | 6/6 | 3/3 | none |
| vue | sampled-name-location | 5/5 | 3/3 | none |
| zig | sampled-name-location | 4/5 | 3/3 | none |

