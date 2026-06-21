# Contract: upstream `gh aw forecast --json` input consumed

This documents the subset of the `gh aw forecast --json` document that
`internal/fleet/forecast.go` decodes. Ground truth is the committed fixtures
(`internal/fleet/testdata/forecast/`) captured on gh-aw **v0.79.2** by spike
issue #108; this contract is the parsing target and the fixtures are the test
substrate.

## Invocation

```bash
gh aw forecast --json --repo <owner/name> --days <7|30>
```

- `--days` accepts **only** `7` or `30` (SCHEMA.md). The fleet `--period`
  flag maps `week → 7`, `month → 30`.
- An optional positional workflow basename narrows to one workflow; the fleet
  rollup omits it to forecast every workflow.
- Monte Carlo; slow. `--timeout` is in **minutes**; on expiry the CLI exits
  `124` with a **partial** but decodable JSON body (FR-013).

## Document shape (consumed subset)

```jsonc
{
  "as_of": "2026-06-10T11:53:18Z",   // RFC3339 — provenance
  "period": "month",                  // "week" | "month"
  "workflows": [
    {
      "workflow_id": "Code Simplifier",   // display name (diagnostics only)
      "sampled_runs": 1,                  // 0 => cold start, all-zero, no monte_carlo
      "weekly_projected_aic": 32.082,     // point estimate when period=week
      "monthly_projected_aic": 137.494,   // point estimate when period=month
      "weekly_monte_carlo": {             // ABSENT when sampled_runs == 0
        "p10_projected_aic": 0,
        "p50_projected_aic": 0,
        "p90_projected_aic": 137.494,
        "is_reliable": false
      },
      "monthly_monte_carlo": {            // ABSENT when sampled_runs == 0
        "p10_projected_aic": 0,
        "p50_projected_aic": 137.494,
        "p90_projected_aic": 549.976,
        "is_reliable": false
      }
      // ignored: avg_aic, avg_duration_seconds, p50_aic_per_run,
      // p95_aic_per_run, projected_aic, observed_runs_per_period,
      // success_rate, history_days, active_triggers, concurrency_limit,
      // run_samples[], monte_carlo (the non-period-prefixed block)
    }
  ]
}
```

## Parsing rules

1. **Point estimate** = `weekly_projected_aic` or `monthly_projected_aic` per
   the requested period (NOT the bare `projected_aic`, which tracks the CLI's
   own `--period` display choice and may differ from the requested `--days`
   mapping).
2. **Confidence band** = `<period>_monte_carlo.{p10,p50,p90}_projected_aic`.
   The whole nested object is `nil` for cold-start workflows — decode into a
   `*monteCarlo` pointer so absence is `nil`, not zero.
3. **Cold start** = `sampled_runs == 0`. Such a workflow contributes `0` to the
   group point estimate, `nil` to the band, and increments `Cold`-eligibility.
4. **Reliability** = `<period>_monte_carlo.is_reliable`. Any `false` among a
   group's contributors raises a group-level low-confidence diagnostic.
5. **Per-variant (A/B) split** appears as additional `workflows[]` entries when
   present; the parser treats each as an independent workflow row (no special
   casing needed). None present in the captured fixtures.

## Fixtures

| Fixture | Shape exercised |
|---|---|
| `forecast_single_workflow.json` | one workflow, `sampled_runs:1`, full Monte Carlo bands (week + month), non-zero projection |
| `forecast_cold_start.json` | four workflows, all `sampled_runs:0`, no Monte Carlo blocks, all-zero |
| *(add)* `forecast_partial.json` | optional — a decodable document representing a timeout-truncated `workflows[]` for the FR-013 partial path, if a dedicated fixture is preferred over reusing single_workflow with an injected non-zero exit |

## Failure modes the parser must tolerate

| Upstream condition | Parser behavior |
|---|---|
| exit 0, valid JSON | normal aggregation |
| exit 124, decodable partial JSON | aggregate present workflows + partial diagnostic (FR-013) |
| exit ≠ 0, undecodable / empty stdout | per-repo hard skip diagnostic (FR-007) |
| `workflows: []` or null | repo contributes nothing; optional per-repo "no workflows" note |
| gh-aw < v0.79.2 | blocked earlier by `ensureForecastGhAwVersion` (FR-008) |
