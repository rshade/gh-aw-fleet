# `gh aw forecast --json` schema (captured on v0.79.2)

Real `--json` output captured for issue #108 to ground the
[#102 `gh-aw-fleet forecast`](https://github.com/rshade/gh-aw-fleet/issues/102)
fleet-wide projection feature. Captured 2026-06-10 with `gh aw` **v0.79.2**.

## Fixtures

| File | Source | Shape |
| --- | --- | --- |
| `forecast_cold_start.json` | `gh aw forecast --json` in `rshade/gh-aw-fleet` | 4 workflows, **all zero** — no successful runs in the sample window (cold start) |
| `forecast_single_workflow.json` | `gh aw forecast code-simplifier --json --repo rshade/finfocus --days 30` | one workflow with `sampled_runs: 1` — **non-zero** AIC projection |

## Top-level shape

```jsonc
{
  "as_of":   "2026-06-10T11:44:11Z", // RFC 3339 capture time
  "period":  "month",                // "month" | "week"
  "workflows": [ /* per-workflow objects, side-by-side */ ]
}
```

## Per-workflow object

| Field | Type | Meaning |
| --- | --- | --- |
| `workflow_id` | string | **Display name** (e.g. `"Code Simplifier"`) — note the CLI *argument* matches on the `.md` basename (`code-simplifier`), but the output key is the display name. |
| `period` | string | `"month"` \| `"week"` |
| `sampled_runs` | int | Completed runs drawn from history. `0` ⇒ every projection field is `0`. |
| `history_days` | int | Sampling window. **Only `7` or `30` are accepted** by `--days`. |
| `observed_runs_per_period` | number | Run frequency projected onto the period. |
| `success_rate` | number | 0..1. |
| `avg_aic` | number | Mean **AI Credits** per run. |
| `avg_duration_seconds` | number | Mean wall-clock per run. |
| `p50_aic_per_run` | number | Median AIC/run. |
| `p95_aic_per_run` | number | 95th-percentile AIC/run. |
| `projected_aic` | number | AIC projected over `period`. |
| `weekly_projected_aic` | number | AIC projected over a week. |
| `monthly_projected_aic` | number | AIC projected over a month. |
| `active_triggers` | array \| **null** | Trigger list; `null` when not derivable. |
| `concurrency_limit` | int | `0` when unset. |

## Grounding notes for #102 (assumptions corrected)

- **Percentiles are P50 + P95 — *not* P10/P50/P90.** The roadmap entry for #102
  assumed a P10/P50/P90 confidence band; the real v0.79.2 schema exposes only
  `p50_aic_per_run` and `p95_aic_per_run`. There is **no P10 or P90 field.**
- **AIC-denominated, not tokens.** v0.79.2 reframed forecast around **AI Credits**
  (`avg_aic`, `*_projected_aic`); v0.77.5 spoke of "effective token usage". A
  fleet rollup should sum `monthly_projected_aic` (or `weekly_*`) across repos.
- **Zero ⇒ no successful run history**, not zero cost. `sampled_runs: 0` produces
  an all-zero record (see `forecast_cold_start.json`). #102 must distinguish
  "no history" from "cheap workflow".
- **Forecast is a Monte Carlo sample and is slow.** `--timeout` is in *minutes*;
  on timeout it emits **partial** results for the workflows processed so far
  (exit 124). A fleet aggregator must tolerate partial/interrupted output.
- **Per-variant split** (A/B experiments) appears as additional `workflows[]`
  entries when present; none were present in these captures.
