# Validation Remeasurement Readiness Manifest

Generated: 2026-06-30T19:52:49.769Z

This manifest is regenerated from committed `data/raw/LIVE_*_BENCHMARK.json` artifacts by `scripts/validation-remeasurement-harness.mjs`.
It is a readiness audit, not a full remeasurement run: Atlas and Graphify replay commands are generated from pinned repo/commit/target rows, while native/proxy counters are marked ready only when the validation row stores an executable native replay command.

## Summary

- Live artifacts: 36
- Live code artifacts: 34
- Structured artifacts: 2
- Validation artifacts: 35
- Validation repo rows: 105
- Pinned repo rows: 105
- Atlas replay-ready rows: 105
- Graphify replay-ready rows: 105
- Native/proxy command candidate rows: 105
- Native/proxy candidate executable rows: 21
- Native/proxy remeasurement command-ready rows: 0
- Full remeasurement-ready artifacts: 0
- Native candidates with placeholders: 69
- Native candidates with ephemeral helper paths: 51
- Proxy or detector-only code artifacts: 14
- Warnings: 35
- Errors: 0

## Language Readiness

| Language | Tool class | Risk | Validation | Repos | Atlas replay | Graphify replay | Native candidates | Candidate executable | Native ready | Blockers |
|---|---|---|---|--:|--:|--:|--:|--:|--:|---|
| apex | source-counter-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| astro | parser-library-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| bash | syntax-checker-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_unexpanded_file_list, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| blade | source-counter-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| byond | source-counter-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| csharp | compiler-or-lsp-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_inline_note, native_or_proxy_remeasurement_command_not_recorded |
| cuda | source-counter-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| dart | tree-sitter-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| delphi | source-counter-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| dotnet | scope-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| ejs | graphify-detector-only-proxy | high | passed | 3/3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, graphify_detector_only_or_weak_proxy_truth, native_or_proxy_remeasurement_command_not_recorded |
| elixir | scope-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| ets | graphify-detector-only-proxy | high | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, graphify_detector_only_or_weak_proxy_truth, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded |
| fortran | tree-sitter-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| groovy | tree-sitter-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| json | structured-format | structured | structured-no-validation | 0/3 | 0 | 0 | 0 | 0 | 0 | no_public_repo_validation_rows, structured_format_outside_code_parser_gate |
| julia | tree-sitter-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| kotlin | tree-sitter-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| lua | parser-library-baseline | low | passed | 3/3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded |
| markdown | structured-format | structured | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded, structured_format_outside_code_parser_gate |
| objc | tree-sitter-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| pascal | source-counter-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| php | parser-library-baseline | low | passed | 3/3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded |
| powershell | parser-library-baseline | low | passed | 3/3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded |
| razor | source-counter-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| ruby | parser-library-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| rust | compiler-or-lsp-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_unexpanded_file_list, native_or_proxy_remeasurement_command_not_recorded |
| r | graphify-detector-only-proxy | high | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, graphify_detector_only_or_weak_proxy_truth, native_command_contains_placeholder, native_or_proxy_remeasurement_command_not_recorded |
| scala | tree-sitter-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| sql | parser-library-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| svelte | parser-library-baseline | low | passed | 3/3 | 3 | 3 | 3 | 3 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_or_proxy_remeasurement_command_not_recorded |
| swift | compiler-or-lsp-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| terraform | parser-library-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| verilog | scope-proxy | medium | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded, proxy_denominator_not_full_semantic_truth |
| vue | parser-library-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |
| zig | tree-sitter-baseline | low | passed | 3/3 | 3 | 3 | 3 | 0 | 0 | full_symbol_name_kind_location_sets_not_persisted, native_command_contains_placeholder, native_command_uses_ephemeral_helper_path, native_or_proxy_remeasurement_command_not_recorded |

## Replay Command Example

Language: apex; repo: aquivalabs/my-org-butler

```sh
git clone https://github.com/aquivalabs/my-org-butler work/apex/01-aquivalabs__my-org-butler
git -C work/apex/01-aquivalabs__my-org-butler checkout f1225426aa62d207f8efa9a434fbda9082e48b9a
atlas index work/apex/01-aquivalabs__my-org-butler --db sqlite://work/apex/01-aquivalabs__my-org-butler/.atlas/atlas.db --reindex --json
rm -rf work/apex/01-aquivalabs__my-org-butler/graphify-out
(cd work/apex/01-aquivalabs__my-org-butler && uv tool run --from graphifyy graphify update .)
```

Native command candidate from language-level template:

```sh
python3 <apex source counter> work/apex/01-aquivalabs__my-org-butler
```
Candidate blockers: native_command_contains_placeholder.

## Remaining Gap

A full remeasurement pass still needs executable native/proxy counter commands per validation repo, committed helper scripts or tool invocations for placeholder templates, plus persisted Atlas and native symbol name/kind/location sets. Until those are present, the benchmark proves pinned artifact evidence, replay command candidates, and replay readiness, not a complete 99% precision oracle.

