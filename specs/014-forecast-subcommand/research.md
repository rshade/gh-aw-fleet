# Phase 0 Research: Fleet-Wide Pre-Spend Cost Forecast

All `NEEDS CLARIFICATION` items from the Technical Context are resolved below.
The dominant input is the FinOps baseline spike (#108,
`specs/finops-108-baseline/spike-findings.md`), which captured the real
`gh aw forecast --json` schema as committed fixtures.

---

## Decision 1 — `gh aw forecast --json` schema shape

**Decision**: Parse the captured v0.79.2 schema directly. Top level:
`{ as_of, period, workflows[] }`. Each `workflows[]` object carries flat
point/percentile fields plus optional nested Monte Carlo objects.

**Fields consumed** (subset; ignore the rest):

| Field | Use |
|---|---|
| `as_of` (string, RFC3339) | echoed into result envelope for provenance |
| `period` (string) | `"week"` \| `"month"` — sanity-checks the requested period |
| `workflows[].workflow_id` (string) | display name; only used for diagnostics |
| `workflows[].sampled_runs` (int) | `0` ⇒ cold-start, all-zero, no Monte Carlo block |
| `workflows[].weekly_projected_aic` (number) | point estimate when period=week |
| `workflows[].monthly_projected_aic` (number) | point estimate when period=month |
| `workflows[].weekly_monte_carlo.{p10,p50,p90}_projected_aic` | week band |
| `workflows[].monthly_monte_carlo.{p10,p50,p90}_projected_aic` | month band |
| `workflows[].<period>_monte_carlo.is_reliable` (bool) | reliability caveat |

**Rationale**: The spike corrected the issue's assumption. Flat
`p50_aic_per_run` / `p95_aic_per_run` are *per-run* percentiles, not the
period band; the P10/P50/P90 *confidence band* lives in the nested
`<period>_monte_carlo` object and is **absent when `sampled_runs == 0`**. The
point estimate a fleet rollup sums is `<period>_projected_aic`.

**Alternatives considered**:
- *Sum `p50_aic_per_run` across workflows* — rejected: that is per-run, not
  per-period; multiplying by `observed_runs_per_period` re-derives the point
  estimate the CLI already computes as `<period>_projected_aic`. Use the
  upstream value.
- *Depend on the SCHEMA.md table alone* — rejected: the committed fixtures are
  richer than SCHEMA.md (they include the `monte_carlo` blocks); the fixtures
  are ground truth.

---

## Decision 2 — Period flag: `--period week|month` → upstream `--days 7|30`

**Decision**: Expose `--period week|month` (default `week`). Map to the
upstream `gh aw forecast --days` flag: `week → --days 7`, `month → --days 30`.
Select the matching point/band fields (`weekly_*` vs `monthly_*`) at parse
time.

**Rationale**: SCHEMA.md states `--days` accepts **only `7` or `30`**. A
`--period` enum is the safer operator surface than a free-form day count that
upstream would reject. The output object always carries *both* `weekly_*` and
`monthly_*` projections regardless of `--days`, so the fleet layer simply picks
the requested pair.

**Alternatives considered**:
- *Pass `--period` straight through* — `gh aw forecast` uses `--days`, not
  `--period`, for the sampling horizon; the per-object `period` field reflects
  the `--period` display choice. Mapping to `--days` is the reliable knob.
- *Free-form `--days N`* — rejected: upstream rejects anything but 7/30; an
  enum prevents a confusing downstream error.

---

## Decision 3 — Injection seam `ghForecastAPI`

**Decision**: A package-level `var ghForecastAPI = func(ctx, repo, period) (forecastPayload, error)`
mirroring `ghLogsAPI` / `ghDiscussionsAPI`. Production impl shells
`gh aw forecast --json --repo <repo> --days <7|30>`; tests reassign it to a
fixture loader and restore via `t.Cleanup`.

**Rationale**: This is the established offline-test pattern (see
`consumption_test.go:1081` — `ghWorkflowsAPI, ghLogsAPI, ghAwVersion` are
swapped and restored). Zero new dependency; `go test` runs with no `gh`
present.

**Implementation notes**:
- Decode with `json.Unmarshal` into a `forecastPayload` struct.
- Partial output (exit 124 with decodable body): capture stdout even on
  non-zero exit, attempt decode, and surface a partial diagnostic (FR-013).
  `runLoggedOutput` returns the error; the seam should still try to decode the
  buffered stdout before giving up.

---

## Decision 4 — AIC→USD reuse

**Decision**: Reuse `aicToUSDRate` (0.01) and the `aicToUSD(*float64) *float64`
helper from `consumption_logs.go` for the derived `projected_cost_usd` and the
band's USD equivalents (if surfaced).

**Rationale**: One rate constant shared across both FinOps commands means they
update together (spec Assumption). No reason to fork the conversion.

---

## Decision 5 — Group-by axes: `repo|profile|cost-center|tier`

**Decision**: Reuse the `consumption` group-by machinery for `repo`,
`profile`, `cost-center`; **replace `workflow` with `tier`**. Multi-profile
repos contribute **additively** to every profile group AND every tier group
(same semantic as consumption's profile additivity). Repos whose profiles
declare no `tier` (or repos with no profiles) land in a `<unset>` tier bucket,
reusing the `unsetCostCenter`-style sentinel pattern.

**Rationale**: `tier` (`Profile.Tier`, shipped in slice 007) is the natural
pre-spend axis — "what will each billing tier cost." `workflow` is redundant
pre-spend because the upstream forecast is already per-workflow; a fleet
operator wanting per-workflow projection scopes to one repo and reads the raw
rows. The issue explicitly lists `repo|profile|cost-center|tier`.

**Tier resolution**: for each repo, for each profile in `RepoSpec.Profiles`,
look up `cfg.Profiles[p].Tier`; empty tier → `<unset>`. A repo on two profiles
with different tiers contributes to both tier groups (additive, documented).

**Alternatives considered**:
- *Keep all four consumption axes plus tier (five axes)* — rejected:
  `workflow` adds maintenance and a column that duplicates upstream output for
  marginal value; the spec scoped exactly four axes.
- *A new `GroupByKind` enum for forecast* — rejected initially in favor of
  reuse, but see Decision 6.

---

## Decision 6 — Whether to reuse `GroupByKind` or define a forecast-local enum

**Decision**: Define a **forecast-local** axis enum
(`ForecastGroupBy` with `repo|profile|cost-center|tier`) rather than extend the
shared `GroupByKind` (which carries `workflow`, not `tier`).

**Rationale**: `GroupByKind` is tightly bound to consumption's
`groupByNames` lookup table and its `workflow` axis. Adding `tier` to the
shared enum and excluding `workflow` only for forecast would leak forecast
concerns into the consumption type. A small dedicated enum + parser
(`ParseForecastGroupBy`) keeps each command's axis vocabulary honest and the
error message accurate (`expected one of repo, profile, cost-center, tier`).
The `ScopeToRepos`, `unsetCostCenter`, materialize-and-sort, and
`newSoftDiagnostic` helpers are still shared verbatim — only the axis
vocabulary forks.

**Alternatives considered**:
- *Extend `GroupByKind` with `GroupByTier`* — viable, but then the
  consumption `--by` help string and forecast `--by` help string diverge from
  the shared `groupByNames` table, producing a misleading "valid axes" error in
  one of the two commands. Cleaner to fork the tiny enum.

---

## Decision 7 — Version gate

**Decision**: Reuse `CompileStrictMinVersion` (already `v0.79.2`) and an
`ensureForecastGhAwVersion(ctx)` helper modeled on
`ensureLogsSourceGhAwVersion`, calling the existing `ghAwVersion` seam +
`compareVersionTokens`. Gate runs once at the top of `AggregateForecast` before
any repo call.

**Rationale**: The floor already shipped with spike #108; no new constant. The
gate prevents a silent all-zero rollup on an old CLI whose forecast schema
differs. Reusing the helper keeps the error copy and the probe seam consistent.

---

## Decision 8 — Cold-start vs cheap distinction (FR-009)

**Decision**: Carry `sampled_runs` through to the group level as a summed
`SampledRuns` count and a per-group `Cold bool` (true when every contributing
workflow had `sampled_runs == 0`). Render cold-start groups with `—` in the
band columns and `0` in the point column; emit a fleet-wide diagnostic when
**every** group is cold (mirrors consumption's `allAICNil` → `nilAICDiag`).

**Rationale**: A `0` projection from no history is operationally different from
a genuinely cheap workflow; conflating them would mislead a rollout decision.
The `sampled_runs` field is the upstream signal for exactly this.

---

## Decision 9 — Partial / interrupted output (FR-013)

**Decision**: When the `gh aw forecast` subprocess exits non-zero (notably 124
on `--timeout`) but its stdout still decodes as valid JSON, aggregate the
workflows present and attach a per-repo `DiagHint` warning ("projection for
<repo> is partial — forecast timed out; widen --timeout or retry"). When stdout
does not decode, treat as a hard per-repo skip with a diagnostic (FR-007).

**Rationale**: SCHEMA.md documents that forecast emits partial results on
timeout. A fleet aggregator that discards partial data is strictly worse than
one that sums what it got and flags the gap.

---

## Decision 10 — JSON envelope parity

**Decision**: Reuse `cmd.Envelope` / `writeEnvelope` / `preResultFailureEnvelope`
unchanged. `schema_version` stays `1` (additive new subcommand, additive new
result struct — no breaking change to existing structs). Add
`commandForecast = "forecast"` to the command-name const block.

**Rationale**: Matches the consumption precedent
(`contracts/consumption-output.json` notes "Envelope schema_version remains 1;
additive new subcommand"). Downstream tooling treats both commands uniformly.

---

## Resolved unknowns summary

| Unknown (from Technical Context / issue) | Resolution |
|---|---|
| `--json` field names / percentile keys | Decision 1 — captured fixtures; P10/P50/P90 in nested `<period>_monte_carlo`, point = `<period>_projected_aic` |
| Period flag mapping | Decision 2 — `--period week\|month` → `--days 7\|30` |
| Offline test mechanism | Decision 3 — `ghForecastAPI` seam + committed fixtures |
| Group-by axes | Decisions 5–6 — `repo\|profile\|cost-center\|tier`, forecast-local enum |
| Min version handling | Decision 7 — reuse `CompileStrictMinVersion` v0.79.2 + existing probe |
| Cold-start handling | Decision 8 — `sampled_runs`-driven cold flag |
| Partial output | Decision 9 — decode-what-you-got + diagnostic |
| Envelope shape | Decision 10 — reuse envelope, schema_version stays 1 |
