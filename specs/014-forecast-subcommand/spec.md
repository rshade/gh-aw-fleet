# Feature Specification: Fleet-Wide Pre-Spend Cost Forecast

**Feature Branch**: `014-forecast-subcommand`
**Created**: 2026-06-17
**Status**: Draft
**Issue**: #102

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Fleet-Wide Forecast Rollup (Priority: P1)

A fleet operator wants to see projected AI-credit spend across **all** repos
before committing to a rollout. They run `gh-aw-fleet forecast` and get a
table grouped by repo showing P10/P50/P90 projected spend for the next period.

**Why this priority**: This is the core value proposition — turning a
per-repo experimental CLI call into a single fleet-level forward view. Without
it the feature does not exist.

**Independent Test**: Run `gh-aw-fleet forecast` against a fleet with ≥2 repos
and verify a non-empty table is printed with one row per repo and numeric
AIC/COST columns. Demonstrates value before any group-by axis is implemented.

**Acceptance Scenarios**:

1. **Given** a configured fleet with ≥1 repo and `gh aw forecast` available,
   **When** the operator runs `gh-aw-fleet forecast`,
   **Then** a table is printed to stdout with one row per repo showing
   projected AIC (P50) and equivalent USD cost for the default period (week).

2. **Given** a repo that has no historical run data,
   **When** `gh aw forecast --json --repo <r>` returns an empty or zero
   projection,
   **Then** the row appears in the table with a `—` or `0` in the AIC column
   and no error is raised.

3. **Given** `gh aw forecast` returns a non-zero exit code for one repo,
   **When** the rollup runs across the fleet,
   **Then** that repo emits a diagnostic warning and is skipped; the remaining
   repos still appear in the output.

---

### User Story 2 — Period Selection (Priority: P2)

The operator wants to toggle between weekly and monthly projections to match
their billing cycle.

**Why this priority**: `gh aw forecast` already supports `--period week|month`;
surfacing this as a flag gives operators the cross-period view without
per-repo manual calls.

**Independent Test**: Run `gh-aw-fleet forecast --period month` and confirm the
projected numbers differ from the `--period week` run (or are clearly labeled
as monthly).

**Acceptance Scenarios**:

1. **Given** a fleet with historical run data,
   **When** `--period week` is passed (or defaulted),
   **Then** the output table header labels the projection as "Week" and the
   numbers reflect a 7-day horizon.

2. **Given** `--period month` is passed,
   **Then** the output table header labels the projection as "Month" and the
   numbers reflect a 30-day horizon.

3. **Given** an unrecognised `--period` value is passed,
   **Then** an actionable error message is returned and no API calls are made.

---

### User Story 3 — Group-By Axis (Priority: P2)

The operator wants to slice the forecast by profile, cost-center, or tier to
understand which workflow bundles or business units will drive the most spend.

**Why this priority**: Mirrors `consumption --by`, making the two commands
directly comparable. Required for the `fleet-budget-review` skill integration.

**Independent Test**: Run `gh-aw-fleet forecast --by profile` and confirm rows
are keyed by profile name, with repos contributing additively to every profile
they belong to (same multi-profile additive semantics as `consumption`).

**Acceptance Scenarios**:

1. **Given** `--by repo` (default), **Then** one row per repo is emitted.
2. **Given** `--by profile`, **Then** one row per profile is emitted; a repo
   belonging to multiple profiles contributes to each.
3. **Given** `--by cost-center`, **Then** repos with no `cost_center` field
   land in a `<unset>` bucket.
4. **Given** `--by tier`, **Then** rows are keyed by the profile's `tier`
   annotation (`minimal|standard|premium`); repos on profiles with no tier
   land in `<unset>`.

---

### User Story 4 — JSON Envelope Output (Priority: P3)

The operator pipes `gh-aw-fleet forecast --output json` into a dashboard or
budget-guard script and expects an envelope identical in shape to `consumption
--output json`.

**Why this priority**: Enables downstream tooling to treat both commands
uniformly. Not required for the initial CLI-only use case.

**Independent Test**: Run with `--output json` and validate with `jq` that the
standard envelope keys (`schema_version`, `command`, `result`) are present and
that each group carries a `projected_aic` point estimate and a
`projected_cost_usd` field.

**Acceptance Scenarios**:

1. **Given** `--output json` is passed,
   **Then** stdout is valid JSON matching the consumption envelope shape plus
   forecast-specific fields per group: a point estimate (`projected_aic`,
   `projected_cost_usd`) and an advisory Monte Carlo band (`aic_p10`,
   `aic_p50`, `aic_p90`).

2. **Given** a group rolls up only cold-start (zero-sampled) workflows,
   **Then** the group's `projected_aic` is `0` and its Monte Carlo band fields
   are `null` (not absent), consistent with the nil-until-positive convention
   in `consumption`.

---

### User Story 5 — Scoped Forecast for a Subset of Repos (Priority: P3)

The operator is rolling out a new profile to three repos and wants to forecast
only those repos' incremental cost before using `--apply`.

**Why this priority**: Mirrors `consumption [repo...]` positional-argument
scoping. Useful but not the core loop.

**Independent Test**: Pass two repo names as positional arguments and confirm
only those two rows appear in the output.

**Acceptance Scenarios**:

1. **Given** two repos are passed as positional arguments,
   **When** `gh-aw-fleet forecast owner/a owner/b` is run,
   **Then** only those repos appear in the output regardless of fleet size.

2. **Given** a repo name is passed that is not in the fleet config,
   **Then** an error is returned naming the unknown repo.

---

### Edge Cases

- What happens when `gh aw forecast` is not installed or is below v0.79.2?
  The command must emit a clear "minimum gh-aw version required" error and
  exit non-zero before attempting any repo calls.
- What happens when the fleet config loads zero repos?
  The command exits with an informational message and exit code 0 (no repos to
  forecast, not an error).
- What happens when all repos fail their `gh aw forecast` call?
  The command exits non-zero and all failures are listed in the diagnostic
  output.
- What happens when `--by` and `--output json` are combined with `--period`?
  All three flags must compose orthogonally; no flag combination is illegal.
- What happens with a very large fleet (50+ repos) where fan-out is slow?
  The fan-out must be sequential (mirrors `consumption` FR-022 / no caching);
  progress output to stderr keeps the operator informed.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST add a `forecast` subcommand to the
  `gh-aw-fleet` CLI that executes `gh aw forecast --json --repo <r>
  --period <p>` for each repo in the fleet (or the specified subset) and
  aggregates the results.

- **FR-002**: The system MUST support `--period week` (default) and
  `--period month`, passing the value through to each `gh aw forecast`
  invocation.

- **FR-003**: The system MUST expose a `--by repo|profile|cost-center|tier`
  flag (default `repo`) using the same additive multi-profile semantics as the
  `consumption` command.

- **FR-004**: The system MUST emit text-table output by default, showing at
  minimum the group key, projected AIC at P50, and equivalent USD at P50.

- **FR-005**: The system MUST support `--output json` emitting an envelope
  structurally compatible with `consumption --output json`, extended with a
  per-group projected-AIC point estimate (`projected_aic`) and its derived USD
  (`projected_cost_usd`), plus an advisory Monte Carlo confidence band
  (`aic_p10`, `aic_p50`, `aic_p90`) sourced from the upstream
  `<period>_monte_carlo` object. The band fields MUST be `null` when the
  upstream projection carries no Monte Carlo data (cold-start workflows with
  zero sampled runs), distinct from a present-and-zero value.

- **FR-006**: The system MUST expose a package-level `ghForecastAPI`
  injection seam (a replaceable function variable) so that tests run fully
  offline without shelling to `gh`.

- **FR-007**: Individual repo failures MUST emit a diagnostic warning and be
  skipped; they MUST NOT abort the entire fleet rollup.

- **FR-008**: The system MUST check that the installed `gh aw` CLI is at or
  above the documented fleet floor (`v0.79.2`, already encoded as
  `CompileStrictMinVersion`) before attempting any forecast calls, and emit a
  clear minimum-version error if not, reusing the existing version-probe
  machinery.

- **FR-009**: Workflows or repos without any historical run data (a
  cold-start projection where `sampled_runs == 0`, yielding an all-zero record
  with no Monte Carlo band) MUST appear in the output as a `0` / `—` row, not
  be silently omitted, and MUST be distinguishable from a genuinely cheap
  workflow.

- **FR-010**: The subcommand MUST accept zero or more positional `owner/repo`
  arguments to scope the rollup; with no arguments the full fleet is used.

- **FR-011**: The `fleet-budget-review` skill MUST be updated with a
  forecast section describing the single-turn `gh-aw-fleet forecast` flow.

- **FR-012**: README.md MUST be updated to document the `forecast` subcommand,
  its flags, and the minimum gh-aw CLI version requirement.

- **FR-013**: When `gh aw forecast` exits with a partial-result code (e.g.
  timeout exit 124) and still emits a decodable JSON document, the system MUST
  aggregate the partial workflows present and emit a diagnostic noting the
  projection for that repo is incomplete.

### Key Entities

- **ForecastGroup**: One row in the aggregated output. Contains: group key
  (repo name, profile name, cost-center label, or tier label), projected AIC
  at P10/P50/P90, equivalent USD at P10/P50/P90, and source-repo count. Nil
  percentile fields indicate no historical data (nil-until-positive rule).

- **ForecastResult**: The output of one `gh aw forecast --json` call for a
  single repo. Contains: capture timestamp (`as_of`), period, and a list of
  per-workflow projections. Each per-workflow projection carries the point
  estimate (`<period>_projected_aic`), per-run percentiles
  (`p50_aic_per_run`, `p95_aic_per_run`), the sampled-run count, and an
  optional nested Monte Carlo confidence band
  (`<period>_monte_carlo.{p10,p50,p90}_projected_aic` plus an `is_reliable`
  flag). Structure follows the captured `gh aw forecast --json` fixtures under
  `internal/fleet/testdata/forecast/` (gh-aw v0.79.2).

- **ForecastOpts**: The options bundle passed to the fleet-layer fan-out
  function, mirroring `ConsumptionOpts`. Contains: fleet config, repos filter,
  period, group-by axis, and the injectable `ghForecastAPI` seam.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator running `gh-aw-fleet forecast` against a fleet of
  N repos sees a complete result table in under N × 10 seconds (sequential
  fan-out baseline; actual time is dominated by upstream `gh aw forecast`
  latency).

- **SC-002**: The JSON envelope produced by `--output json` passes a structural
  validation check (required top-level keys present, each group has non-null
  group key and numeric or null percentile fields).

- **SC-003**: `make ci` passes with no new lint warnings or failing tests after
  the feature is introduced.

- **SC-004**: The offline test suite (using the `ghForecastAPI` injection seam
  with fixture data) achieves branch coverage of the fan-out, aggregation,
  group-by, and error-skip paths.

- **SC-005**: An operator receives a clear, actionable error message within 1
  second when running `forecast` with an installed `gh aw` CLI below v0.79.2,
  without any repo API calls being made.

- **SC-006**: The `fleet-budget-review` skill's forecast section allows an
  operator to complete the budget-review workflow (dry-run forecast → interpret
  results → no further steps needed) in a single conversation turn.

## Assumptions

- **gh aw forecast schema is grounded, not assumed**: The `--json` output
  schema was captured empirically by the FinOps baseline spike (#108) and
  committed as fixtures under `internal/fleet/testdata/forecast/`. The original
  issue's assumption of flat `P10/P50/P90` keys was **corrected**: the flat
  per-run percentiles are `p50_aic_per_run` / `p95_aic_per_run` (no P10/P90),
  while the P10/P50/P90 confidence band lives in the nested
  `<period>_monte_carlo` object and is absent on cold-start workflows. The
  point-estimate metric a fleet rollup sums is `<period>_projected_aic`. If the
  schema changes in a later release the `ghForecastAPI` seam isolates the
  impact to one parsing function.

- **Minimum version floor already shipped**: The fleet's documented and CI
  gh-aw floor was already raised to `v0.79.2` by spike #108
  (`CompileStrictMinVersion = "v0.79.2"`). This feature reuses that constant
  and the existing version-probe helpers (`ghAwVersion`,
  `compareVersionTokens`) rather than introducing a new floor. The issue's
  "installed CLI is v0.77.5" note is stale.

- **AIC-to-USD rate**: The same `aicToUSDRate` constant (0.01 USD/AIC) used
  by `consumption` applies to forecast output. If the rate changes, both
  commands share the constant and update together.

- **Sequential fan-out**: No concurrency is introduced (mirrors `consumption`
  FR-022). Fleet sizes are expected to be small enough (< 50 repos) that
  sequential calls are acceptable.

- **No caching**: Results are recomputed on every invocation (mirrors
  `consumption` FR-022). A future caching layer is out of scope.

- **gh aw forecast is experimental**: The feature is gated on gh-aw ≥ v0.79.2.
  Documentation must note the experimental status. The fleet's documented
  minimum CLI version is bumped from v0.77.5 to v0.79.2 as part of this work.

- **Text output uses tabwriter**: Consistent with all other `gh-aw-fleet`
  subcommands, text table output uses `text/tabwriter` aligned columns.

- **`tier` group-by reads from Profile.Tier**: The `tier` axis reuses the
  existing `Profile.Tier` field introduced in slice 007-billing-metadata-fields;
  no schema changes are required.
