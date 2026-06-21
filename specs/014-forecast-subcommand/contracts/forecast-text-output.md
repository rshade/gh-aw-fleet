# Contract: `gh-aw-fleet forecast` text output

Default (`--output text`) rendering. Uses `text/tabwriter` with the shared
`tabPadding`, matching `consumption`'s table style. Stderr carries the
`(loaded ...)` breadcrumb and structured warnings; stdout carries the table.

## Primary table

Header columns depend on the `--by` axis key column (`REPO` / `PROFILE` /
`COST_CENTER` / `TIER`), followed by the fixed projection columns:

```text
REPO              PROJECTED_AIC  PROJECTED_COST  P10     P50      P90       SAMPLED  WORKFLOWS
rshade/finfocus   137.49         $1.37           0.00    137.49   549.98    1        1
rshade/gh-aw-fleet 0.00          -               -       -        -         0        4
```

### Column semantics

| Column | Source | Empty rendering |
|---|---|---|
| key column | `ForecastGroup.Key` | n/a |
| `PROJECTED_AIC` | `ProjectedAIC` (point estimate, summed) | `0.00` |
| `PROJECTED_COST` | `ProjectedCostUSD` (`aicToUSD`) | `-` when nil |
| `P10` / `P50` / `P90` | `AICP10/50/90` (Monte Carlo band, summed) | `-` when nil (all-cold) |
| `SAMPLED` | `SampledRuns` (summed) | `0` |
| `WORKFLOWS` | `WorkflowCount` | `0` |

- AIC values render `%.2f` (no `$`). Cost renders `$%.2f` (reuse `formatCost`).
  Band cells reuse `formatAIC` (dash when nil).
- A `cold:true` group renders `0.00` AIC, `-` cost, `-` for all band columns —
  visually distinct from a low-but-nonzero projection.
- Rows sorted by Key ascending (deterministic; `materializeForecastGroups`).

## Period label

The table is preceded by a one-line stderr context note echoing the period and
axis, mirroring consumption's framing:

```text
  (loaded fleet.json + fleet.local.json)
  forecast: period=month by=repo
```

## Diagnostics (stderr)

Warnings route through zerolog `log.Warn().Str("code", w.Code).Msg(w.Message)`
(same `emitConsumptionWarnings` style). Expected warning classes:

- `No agentic workflows ... nothing to forecast` — per repo with no `.lock.yml`
  workflows (if forecast reports none).
- `Forecast for <repo> is partial ...` — FR-013 timeout/partial.
- `` `gh aw forecast` failed for <repo>: ... `` — FR-007 hard skip.
- `Monte Carlo band for <repo> is low-confidence ...` — reliability caveat.
- All-cold fleet note — when every group is cold (mirrors `nilAICDiag`).

## Exit codes

- `0` when at least one repo produced a forecast (even all-cold). "No history"
  is not a failure.
- Non-zero only when: version gate fails (FR-008), config load fails, an
  unknown positional repo is named, or every repo hard-fails its forecast call.
