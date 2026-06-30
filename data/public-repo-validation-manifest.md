# Public Repo Validation Manifest

Generated: 2026-06-30T19:12:26.082Z

This manifest is regenerated from committed `data/raw/LIVE_*_BENCHMARK.json` artifacts by `scripts/public-repo-validation-harness.mjs`.
It validates the recorded public-repo evidence and preserves clone/checkout replay commands for every pinned validation repo.

## Summary

- Live artifacts: 36
- Code artifacts passed: 35/35
- Structured pending: 1
- Repo rows checked: 105
- Warnings: 34
- Errors: 0

| Language | Status | Repos | Min coverage | Warnings | Artifact |
|---|---|--:|--:|--:|---|
| apex | passed | 3/3 | 1.0607 | 0 | `data/raw/LIVE_APEX_BENCHMARK.json` |
| astro | passed | 3/3 | 1 | 0 | `data/raw/LIVE_ASTRO_BENCHMARK.json` |
| bash | passed | 3/3 | 1 | 2 | `data/raw/LIVE_BASH_BENCHMARK.json` |
| blade | passed | 3/3 | 1.0121 | 2 | `data/raw/LIVE_BLADE_BENCHMARK.json` |
| byond | passed | 3/3 | 1 | 2 | `data/raw/LIVE_BYOND_BENCHMARK.json` |
| csharp | passed | 3/3 | 1.2411 | 0 | `data/raw/LIVE_CSHARP_BENCHMARK.json` |
| cuda | passed | 3/3 | 2.5753 | 0 | `data/raw/LIVE_CUDA_BENCHMARK.json` |
| dart | passed | 3/3 | 1.3222 | 2 | `data/raw/LIVE_DART_BENCHMARK.json` |
| delphi | passed | 3/3 | 1 | 2 | `data/raw/LIVE_DELPHI_BENCHMARK.json` |
| dotnet | passed | 3/3 | 1 | 0 | `data/raw/LIVE_DOTNET_BENCHMARK.json` |
| ejs | passed | 3/3 | 1 | 0 | `data/raw/LIVE_EJS_BENCHMARK.json` |
| elixir | passed | 3/3 | 1 | 2 | `data/raw/LIVE_ELIXIR_BENCHMARK.json` |
| ets | passed | 3/3 | 1.0229 | 2 | `data/raw/LIVE_ETS_BENCHMARK.json` |
| fortran | passed | 3/3 | 1 | 2 | `data/raw/LIVE_FORTRAN_BENCHMARK.json` |
| groovy | passed | 3/3 | 1.0857 | 0 | `data/raw/LIVE_GROOVY_BENCHMARK.json` |
| json | structured-pending | 0/3 | n/a | 0 | `data/raw/LIVE_JSON_BENCHMARK.json` |
| julia | passed | 3/3 | 1.0037 | 2 | `data/raw/LIVE_JULIA_BENCHMARK.json` |
| kotlin | passed | 3/3 | 1.3429 | 0 | `data/raw/LIVE_KOTLIN_BENCHMARK.json` |
| lua | passed | 3/3 | 1 | 0 | `data/raw/LIVE_LUA_BENCHMARK.json` |
| markdown | passed | 3/3 | 1 | 2 | `data/raw/LIVE_MARKDOWN_BENCHMARK.json` |
| objc | passed | 3/3 | 1 | 0 | `data/raw/LIVE_OBJC_BENCHMARK.json` |
| pascal | passed | 3/3 | 1.21 | 2 | `data/raw/LIVE_PASCAL_BENCHMARK.json` |
| php | passed | 3/3 | 1 | 2 | `data/raw/LIVE_PHP_BENCHMARK.json` |
| powershell | passed | 3/3 | 1 | 2 | `data/raw/LIVE_POWERSHELL_BENCHMARK.json` |
| razor | passed | 3/3 | 1 | 0 | `data/raw/LIVE_RAZOR_BENCHMARK.json` |
| ruby | passed | 3/3 | 1 | 0 | `data/raw/LIVE_RUBY_BENCHMARK.json` |
| rust | passed | 3/3 | 1.4076 | 0 | `data/raw/LIVE_RUST_BENCHMARK.json` |
| r | passed | 3/3 | 1.6067 | 2 | `data/raw/LIVE_R_BENCHMARK.json` |
| scala | passed | 3/3 | 1.4184 | 0 | `data/raw/LIVE_SCALA_BENCHMARK.json` |
| sql | passed | 3/3 | 1.1802 | 2 | `data/raw/LIVE_SQL_BENCHMARK.json` |
| svelte | passed | 3/3 | 1.0476 | 0 | `data/raw/LIVE_SVELTE_BENCHMARK.json` |
| swift | passed | 3/3 | 1.0118 | 0 | `data/raw/LIVE_SWIFT_BENCHMARK.json` |
| terraform | passed | 3/3 | 1 | 2 | `data/raw/LIVE_TERRAFORM_BENCHMARK.json` |
| verilog | passed | 3/3 | 1 | 2 | `data/raw/LIVE_VERILOG_BENCHMARK.json` |
| vue | passed | 3/3 | 3.4167 | 0 | `data/raw/LIVE_VUE_BENCHMARK.json` |
| zig | passed | 3/3 | 2.1118 | 0 | `data/raw/LIVE_ZIG_BENCHMARK.json` |

## Replay Command Example

Language: apex; repo: aquivalabs/my-org-butler

```sh
git clone https://github.com/aquivalabs/my-org-butler work/apex/01-aquivalabs__my-org-butler
git -C work/apex/01-aquivalabs__my-org-butler checkout f1225426aa62d207f8efa9a434fbda9082e48b9a
rm -rf work/apex/01-aquivalabs__my-org-butler/graphify-out
atlas index work/apex/01-aquivalabs__my-org-butler --db sqlite://work/apex/01-aquivalabs__my-org-butler/.atlas/atlas.db --reindex --json
```

