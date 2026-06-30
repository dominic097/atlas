# Atlas Recall — UI prototypes

Three design directions for the **Atlas Recall** observability dashboard — the UI for Atlas's
self-building org automation layer (semantic answer cache → frequency-mined, distilled,
reusable automations). Open [`index.html`](./index.html) to choose a direction.

| Option | Direction | For |
|---|---|---|
| **A** | [`option-a-mission-control.html`](./option-a-mission-control.html) — dense dark ops-room: KPI strip, 30-day trend, served-by donut, hot-prompt leaderboard spine, right-rail detail, live feed | the operator watching the system |
| **B** | [`option-b-automation-library.html`](./option-b-automation-library.html) — card/catalog "skill store": searchable automation cards + detail drawer (Cowork three-pane, staleness, versions, provenance) | discovering & managing automations |
| **C** | [`option-c-cowork-canvas.html`](./option-c-cowork-canvas.html) — Cowork-native: Progress / Working-folder / Context three-pane, candidate→trusted lifecycle pipeline, approval-queue-first | the human-in-the-loop trust workflow |

## What they show

All three render the same surfaces from one shared mock dataset: token savings + % LLM
avoided + hit-rate + p50/p95 KPIs, served-by tier breakdown (exact-cache / semantic-cache /
automation-replay / live-LLM), the **hot-prompt leaderboard** ranked by HotScore
(`frequency × distinct_users^1.5 × avg_tokens` — one user asking = skip, whole org = automate),
per-cluster **automatability** verdict (`automatable` / `gated` / `no` / `unknown`), the
**automation registry** (Cowork-shaped: workflow steps + script artifact + mutable context),
the **staleness** change-metric signals (code/config/data/time/external; fingerprint vs probe),
and the **approval queue** for gated cases (the `deploy {service} to {env}` example).

## Constraints (why they're built this way)

- **Fully self-contained** — all CSS, vanilla JS, mock data, and icons are inline. **No CDN /
  network dependencies.** Each file renders offline via `file://`, so it can be `//go:embed`'d
  and served by `atlas serve` (e.g. at `/dashboard`) from any base path.
- **No frameworks / chart libs** — hand-rolled inline-SVG charts, system font stack.
- Dark default with a working light theme toggle; keyboard-accessible, WCAG-AA palette.

Verified headlessly (Playwright / chromium-1228): all three load with **zero console errors**
and zero external network requests.

## Status

Prototypes only — mock data, no backend. After a direction is chosen, the dashboard wires to
the Atlas Recall API (see the plan) and ships embedded in the binary.

Plan: [`../../docs/ATLAS_RECALL_PLAN.md`](../../docs/ATLAS_RECALL_PLAN.md).
