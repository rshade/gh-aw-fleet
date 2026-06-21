# Phase 1 Data Model: Fleet-Wide Pre-Spend Cost Forecast

All types live in `internal/fleet/forecast.go` unless noted. JSON tags shown
are the wire contract carried inside the standard `cmd.Envelope.Result`.
Nil-until-positive convention (`*float64` distinguishing absent from
present-and-zero) follows `consumption`.

---

## Upstream-decode types (unexported)

These mirror the captured `gh aw forecast --json` fixtures
(`internal/fleet/testdata/forecast/`). Only the consumed subset is declared;
unlisted fields decode-and-ignore.

### `forecastPayload`

One `gh aw forecast --json` document for a single repo.

| Field | Go type | JSON | Notes |
|---|---|---|---|
| AsOf | `time.Time` | `as_of` | RFC3339 capture time |
| Period | `string` | `period` | `"week"` \| `"month"` |
| Workflows | `[]forecastWorkflow` | `workflows` | per-workflow projections |

### `forecastWorkflow`

| Field | Go type | JSON | Notes |
|---|---|---|---|
| WorkflowID | `string` | `workflow_id` | display name (diagnostics only) |
| SampledRuns | `int` | `sampled_runs` | `0` ⇒ cold start |
| WeeklyProjectedAIC | `float64` | `weekly_projected_aic` | point estimate (week) |
| MonthlyProjectedAIC | `float64` | `monthly_projected_aic` | point estimate (month) |
| WeeklyMonteCarlo | `*monteCarlo` | `weekly_monte_carlo` | nil when cold start |
| MonthlyMonteCarlo | `*monteCarlo` | `monthly_monte_carlo` | nil when cold start |

### `monteCarlo`

The nested confidence band. Absent (`nil`) when `sampled_runs == 0`.

| Field | Go type | JSON | Notes |
|---|---|---|---|
| P10 | `float64` | `p10_projected_aic` | low band |
| P50 | `float64` | `p50_projected_aic` | median band |
| P90 | `float64` | `p90_projected_aic` | high band |
| IsReliable | `bool` | `is_reliable` | quality caveat |

**Selection helper**: `(forecastWorkflow).pick(period) (point float64, band *monteCarlo)`
returns `(WeeklyProjectedAIC, WeeklyMonteCarlo)` or the monthly pair based on
the requested period — keeps the period branch in one place.

---

## Aggregation types (exported)

### `ForecastGroupBy` (enum)

Forecast-local axis vocabulary (Decision 6). Closed set; parsed via
`ParseForecastGroupBy(string) (ForecastGroupBy, error)`.

| Const | CLI value | Key meaning |
|---|---|---|
| `ForecastByRepo` (iota, default) | `repo` | `"owner/name"` |
| `ForecastByProfile` | `profile` | profile name; multi-profile repos additive |
| `ForecastByCostCenter` | `cost-center` | `cost_center` value or `<unset>` |
| `ForecastByTier` | `tier` | `Profile.Tier` value or `<unset>`; additive |

`String()` returns the CLI value (table-driven, single source of truth, same
pattern as `GroupByKind`). Invalid input error names the four axes.

### `Period` (enum)

| Const | CLI value | `--days` |
|---|---|---|
| `PeriodWeek` (default) | `week` | `7` |
| `PeriodMonth` | `month` | `30` |

`ParsePeriod(string) (Period, error)`; `(Period).Days() int`;
`(Period).String() string`.

### `ForecastGroup`

One aggregated row. `*float64` band fields are `nil` for an all-cold group.

| Field | Go type | JSON | Notes |
|---|---|---|---|
| Key | `string` | `key` | axis-dependent (see `ForecastGroupBy`) |
| ProjectedAIC | `float64` | `projected_aic` | summed point estimate |
| ProjectedCostUSD | `*float64` | `projected_cost_usd` | `aicToUSD` of point; nil when 0 |
| AICP10 | `*float64` | `aic_p10` | summed band low; nil if all-cold |
| AICP50 | `*float64` | `aic_p50` | summed band median; nil if all-cold |
| AICP90 | `*float64` | `aic_p90` | summed band high; nil if all-cold |
| SampledRuns | `int` | `sampled_runs` | summed across contributing workflows |
| Cold | `bool` | `cold` | true when every contributing workflow cold-started |
| WorkflowCount | `int` | `workflow_count` | contributing workflow rows |

> **Band-summation caveat** (documented, not a bug): summing per-workflow
> Monte Carlo percentiles is statistically approximate (P90 of a sum ≠ sum of
> P90s). The band is advisory; `is_reliable: false` on any contributor sets a
> group-level reliability diagnostic. The authoritative number is
> `ProjectedAIC` (the point estimate).

### `ForecastResult`

The `Envelope.Result` payload. Slice fields normalized to `[]` by `initSlices`.

| Field | Go type | JSON | Notes |
|---|---|---|---|
| LoadedFrom | `string` | `loaded_from` | echoes `cfg.LoadedFrom` |
| Period | `string` | `period` | requested period |
| GroupBy | `string` | `group_by` | axis name |
| Groups | `[]ForecastGroup` | `groups` | sorted by Key ascending |

### `ForecastOpts` (optional bundle, internal)

Mirrors the call shape; may be passed positionally instead. Documented for
parity with the issue's "Reuse the `ConsumptionGroup` aggregation shape."

| Field | Go type | Notes |
|---|---|---|
| Cfg | `*Config` | already `ScopeToRepos`-filtered |
| Period | `Period` | week/month |
| By | `ForecastGroupBy` | axis |

---

## Entry point

```go
func AggregateForecast(
    ctx context.Context,
    cfg *Config,
    period Period,
    by ForecastGroupBy,
) (*ForecastResult, []Diagnostic, error)
```

**Flow** (mirrors `AggregateConsumption`):
1. `ensureForecastGhAwVersion(ctx)` — hard error if `gh aw` < v0.79.2.
2. Sort repo names; for each repo (honoring `ctx.Err()`):
   a. `ghForecastAPI(ctx, repo, period)` → payload (or per-repo skip diag on
      hard failure; partial diag on decodable-but-nonzero-exit).
   b. For each workflow: `pick(period)`, fold into the groups the repo's
      profiles/cost-center/tier map to (`addForecastToGroups`).
3. `materializeForecastGroups` — map → sorted slice.
4. Build `ForecastResult`; if every group is cold, append the all-cold
   fleet diagnostic.

**Shared helpers reused verbatim**: `ScopeToRepos`, `unsetCostCenter`,
`newSoftDiagnostic`, `aicToUSD`, `aicToUSDRate`, `ghAwVersion`,
`compareVersionTokens`, `CompileStrictMinVersion`, `Diagnostic` / `DiagHint`,
`cmd.Envelope` / `writeEnvelope` / `preResultFailureEnvelope`, `initSlices`.

---

## State & validation rules

- **No persistent state.** Every invocation re-fans-out (FR-022 parity).
- **Additive multi-axis membership.** A repo on profiles `[a, b]` with tiers
  `[standard, premium]` contributes its full projection to profile groups `a`
  and `b` *and* tier groups `standard` and `premium`. Sums across rows exceed
  the fleet total by design (documented, same as consumption).
- **`<unset>` buckets.** Empty `cost_center` → `<unset>` cost-center group;
  empty/absent tier (or repo with no profiles) → `<unset>` tier group.
- **Band nil rule.** A `ForecastGroup`'s `AICP{10,50,90}` stay `nil` until a
  non-cold workflow contributes a Monte Carlo block; once any contributor is
  cold the point estimate still sums but the band reflects only the workflows
  that had a band (additive over present bands).
- **Period field consistency.** If `payload.Period` disagrees with the
  requested period, trust the requested period for field selection and emit a
  debug-level note (defensive; should not happen since `--days` drives it).
