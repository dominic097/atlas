# Atlas 10x Gap Report

Generated: 2026-06-30T07:35:21.464Z

Token and latency are ratio targets. Coverage is a native-definition coverage ratio, so the honest accuracy target is >1.0 native coverage exceed, not a fabricated 10x accuracy multiplier.

## Summary

- Live languages: 36
- Coverage parity languages still to move into exceed: 29
- Coverage exceed languages: 7
- Comparable live languages: 33
- Token >=10x comparable: 26
- Latency >=10x comparable: 16
- Token+latency >=10x comparable: 16

## Biggest Latency Gaps

| Language | latencyRatio | improvement to 10x | tokenRatio | blockers |
|---|--:|--:|--:|---|
| zig | 5.74 | 1.74x | 27.82 | coverage_at_parity, latency_ratio_below_10x |
| cuda | 5.96 | 1.68x | 6.67 | coverage_at_parity, token_ratio_below_10x, latency_ratio_below_10x |
| julia | 6.11 | 1.64x | 12.72 | coverage_at_parity, latency_ratio_below_10x |
| verilog | 6.09 | 1.64x | 8.73 | coverage_at_parity, token_ratio_below_10x, latency_ratio_below_10x |
| ejs | 6.19 | 1.61x | 8 | coverage_at_parity, token_ratio_below_10x, latency_ratio_below_10x |
| lua | 6.22 | 1.61x | 15.09 | latency_ratio_below_10x |
| vue | 6.22 | 1.61x | 8.04 | coverage_at_parity, token_ratio_below_10x, latency_ratio_below_10x |
| astro | 6.26 | 1.6x | 10.54 | coverage_at_parity, latency_ratio_below_10x |
| svelte | 6.3 | 1.59x | 8.81 | token_ratio_below_10x, latency_ratio_below_10x |
| markdown | 6.69 | 1.5x | 10.31 | coverage_at_parity, latency_ratio_below_10x |
| groovy | 6.79 | 1.47x | 9.97 | token_ratio_below_10x, latency_ratio_below_10x |
| kotlin | 6.82 | 1.47x | 27.06 | latency_ratio_below_10x |

## Biggest Token Gaps

| Language | tokenRatio | improvement to 10x | latencyRatio | blockers |
|---|--:|--:|--:|---|
| cuda | 6.67 | 1.5x | 5.96 | coverage_at_parity, token_ratio_below_10x, latency_ratio_below_10x |
| razor | 7.88 | 1.27x | 7.15 | coverage_at_parity, token_ratio_below_10x, latency_ratio_below_10x |
| ejs | 8 | 1.25x | 6.19 | coverage_at_parity, token_ratio_below_10x, latency_ratio_below_10x |
| vue | 8.04 | 1.24x | 6.22 | coverage_at_parity, token_ratio_below_10x, latency_ratio_below_10x |
| verilog | 8.73 | 1.15x | 6.09 | coverage_at_parity, token_ratio_below_10x, latency_ratio_below_10x |
| svelte | 8.81 | 1.13x | 6.3 | token_ratio_below_10x, latency_ratio_below_10x |

