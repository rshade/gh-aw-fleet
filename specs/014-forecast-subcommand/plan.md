# Implementation Plan: Fleet-Wide Pre-Spend Cost Forecast

**Branch**: `014-forecast-subcommand` | **Date**: 2026-06-21 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/014-forecast-subcommand/spec.md`

## Summary

Add a `gh-aw-fleet forecast` subcommand that fans out `gh aw forecast --json --repo <r>
--days <7|30>` across the fleet (or a named subset), aggregates per-workflow projections
into `ForecastGroup` rows keyed on four axes (`repo|profile|cost-center|tier`), and
renders both a text table and a JSON envelope structurally compatible with `consumption
--output json`. The implementation is a thin orchestration layer over the existing
`gh aw forecast` CLI — no Monte Carlo math lives here; we sum the upstream point
estimates and advisory bands and carry a `cold` flag for zero-history repos.

All design decisions are resolved in `research.md`. Schema details are in
`data-model.md`. CLI contract is in `contracts/`. Fixtures are in
`internal/fleet/testdata/forecast/`.

## Technical Context

**Language/Version**: Go 1.26.4 (local gate); `go.mod` declares `go 1.25.8` compat
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog`
v1.35.1 (stderr warnings), stdlib `encoding/json`, `context`, `os/exec`,
`text/tabwriter`, `sort` — **no new direct dependencies** (Constitution
§Third-Party Dependencies)
**Storage**: N/A — pure fan-out to `gh aw forecast --json` subprocess; no on-disk
state, no cache, no baseline. Results are transient in memory and written to stdout.
**Testing**: `go test ./...` via `make ci`; offline test suite driven by the
`ghForecastAPI` injection seam and committed fixtures under
`internal/fleet/testdata/forecast/`
**Target Platform**: Linux/macOS CLI tool (same gate as `consumption`)
**Project Type**: CLI subcommand addition to an existing cobra tree
**Performance Goals**: Sequential per-repo fan-out baseline (SC-001: N repos ×
upstream `gh aw forecast` latency); version-gate check < 1 s before any repo call
(SC-005)
**Constraints**: No parallelism (mirrors consumption FR-022); no caching (mirrors
FR-022); no new third-party dependencies; no `SchemaVersion` bump (additive
subcommand, additive result struct)
**Scale/Scope**: Typical fleet ≤ 50 repos; sequential fan-out acceptable at that scale

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| **I — Thin-Orchestrator Code Quality** | ✅ PASS | Shells to `gh aw forecast --json` via `exec.Command`; zero workflow logic reimplemented. `go build`/`go vet` must exit clean. |
| **II — Testing Standards** | ✅ PASS | `ghForecastAPI` seam enables fully offline tests against committed fixtures. `make ci` is the gate. No `--apply` (read-only command; three-turn dry-run not required). |
| **III — UX Consistency** | ✅ PASS | `forecast` is read-only — mirrors `consumption` single-turn pattern (no three-turn mutation pattern required). Structured warnings route through zerolog. |
| **IV — Performance** | ✅ PASS | Sequential fan-out < N × 10 s baseline. No re-fetch of data we already have. No caching needed for a point-in-time command. |
| **§Third-Party Dependencies** | ✅ PASS | No new `go.mod` direct entries. All packages used (`cobra`, `zerolog`, `json`, `exec`) are approved or stdlib. |
| **Declarative Reconcile Invariants** | ✅ PASS | Fleet.json/local.json is still the source of truth. `forecast` is read-only; nothing mutates. GPG bypass never issued (N/A — no git ops). |

**Post-design re-check**: All checks still pass after Phase 1 design. The
`ForecastGroupBy` fork (Decision 6 in research.md) avoids polluting `GroupByKind`
while adding zero complexity — it is the same four-line enum pattern. Band summation
caveat is documented, not hidden.

## Project Structure

### Documentation (this feature)

```text
specs/014-forecast-subcommand/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — all 10 design decisions
├── data-model.md        # Phase 1 output — Go types, entry point, state rules
├── quickstart.md        # Phase 1 output — operator guide and flag reference
├── contracts/
│   ├── forecast-json-input.md   # upstream gh aw forecast --json consumed subset
│   ├── forecast-output.json     # example JSON envelope for --output json
│   └── forecast-text-output.md  # text-mode rendering contract
├── checklists/          # existing
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

**New files**:

```text
internal/fleet/
└── forecast.go            # types + seam + AggregateForecast
    # Exports: ForecastGroupBy, Period, ForecastGroup, ForecastResult,
    #          AggregateForecast, ParseForecastGroupBy, ParsePeriod
    # Unexported: forecastPayload, forecastWorkflow, monteCarlo,
    #             ghForecastAPI, ensureForecastGhAwVersion,
    #             addForecastToGroups, materializeForecastGroups

internal/fleet/
└── forecast_test.go       # offline tests — all paths covered by fixtures

cmd/
├── forecast.go            # newForecastCmd, runForecast, renderForecastText,
│                          #   forecastByColumnHeader
└── forecast_test.go       # cmd-layer golden-output tests
```

**Modified files**:

```text
cmd/output.go              # add: commandForecast = "forecast" to const block
cmd/root.go                # add: rootCmd.AddCommand(newForecastCmd(&flagDir))
skills/fleet-budget-review/SKILL.md
                           # add: §"Pre-spend forecast" section (FR-011)
README.md                  # add: `forecast` subcommand docs + min gh-aw version
                           #   (FR-012)
CLAUDE.md / AGENTS.md      # add note in ## Recent Changes for 014-forecast-subcommand
```

**Test fixtures** (already committed by spike #108):

```text
internal/fleet/testdata/forecast/
├── forecast_single_workflow.json   # sampled_runs:1, full Monte Carlo bands
├── forecast_cold_start.json        # 4 workflows, all sampled_runs:0, no bands
└── SCHEMA.md                       # field-by-field schema notes
```

**Structure Decision**: Single-project layout, same package split as `consumption`
(`internal/fleet/` for aggregation logic, `cmd/` for CLI wiring). Keeps the
`internal/fleet` package boundary consistent: exported types only, no cobra imports.

## Implementation Strategy

> This section bridges research.md decisions to concrete code. Read alongside
> data-model.md and the existing `consumption_logs.go` / `cmd/consumption.go`.

### Phase A — `internal/fleet/forecast.go`

**File layout** (mirrors `consumption_logs.go` style):

1. **Unexported decode types** (`forecastPayload`, `forecastWorkflow`, `monteCarlo`):
   - Map exactly to the fields listed in `data-model.md §"Upstream-decode types"`.
   - `forecastWorkflow.pick(period Period) (point float64, band *monteCarlo)` — the
     single place the `weekly_*/monthly_*` branch lives (Decision 1 + 2).

2. **Exported enums** (`ForecastGroupBy`, `Period`):
   - Same table-driven `String()` / `Parse*` pattern as `GroupByKind` / `SourceKind`.
   - `Period.Days() int` returns 7 or 30 for the `--days` upstream arg (Decision 2).
   - `ForecastGroupBy` has four consts: `ForecastByRepo`, `ForecastByProfile`,
     `ForecastByCostCenter`, `ForecastByTier` (Decision 6).

3. **Exported result types** (`ForecastGroup`, `ForecastResult`) — see data-model.md.
   - `ForecastGroup.AICP10/50/90` are `*float64` (nil for all-cold groups).
   - `ForecastGroup.Cold bool` and `ForecastGroup.SampledRuns int` carry the
     cold-start signal through to the table renderer (Decision 8).

4. **`ghForecastAPI` seam** (package-level `var`):
   ```go
   //nolint:gochecknoglobals // test-injection seam for `gh aw forecast --json`
   var ghForecastAPI = func(ctx context.Context, repo string, period Period) (forecastPayload, error)
   ```
   - Production body: `exec.CommandContext(ctx, "gh", "aw", "forecast", "--json", "--repo", repo, "--days", strconv.Itoa(period.Days()))`.
   - Capture stdout even on non-zero exit (FR-013): use `cmd.Output()` but also read
     `Stdout` buffer before propagating the error. If stdout decodes, return partial
     payload + a wrapped error; the caller distinguishes partial from hard-fail.
   - Tests swap this to a fixture-loading closure and restore via `t.Cleanup`.

5. **`ensureForecastGhAwVersion`** — mirrors `ensureLogsSourceGhAwVersion` from
   `consumption_logs.go`. Uses existing `ghAwVersion` seam and `compareVersionTokens`.
   Error message mirrors the logs pattern (names `CompileStrictMinVersion`).

6. **`AggregateForecast(ctx, cfg, period, by)`** — the entry point:
   - Gate: `ensureForecastGhAwVersion(ctx)` — hard error if `gh aw` < v0.79.2.
   - Repo loop (sorted, `ctx.Err()` checked each iteration):
     - Call `ghForecastAPI(ctx, repo, period)` → payload / error.
     - On hard-fail (no decodable stdout): `newSoftDiagnostic(repo, ...)` + skip.
     - On partial (decodable stdout + non-zero exit): aggregate what's there +
       partial diagnostic (Decision 9 / FR-013).
     - On success: `addForecastToGroups(groups, cfg, repo, by, payload, period)`.
   - `materializeForecastGroups(groups)` → `[]ForecastGroup` (sorted by Key asc).
   - All-cold check: if every `ForecastGroup.Cold`, append the fleet-wide cold
     diagnostic (mirrors `allAICNil` + `nilAICDiag` pattern).
   - Return `*ForecastResult`, `[]Diagnostic`, `error`.

7. **`addForecastToGroups`** (unexported helper):
   - For each `forecastWorkflow` in payload, call `wf.pick(period)` → `point, band`.
   - Determine which group keys this repo maps to given the `by` axis:
     - `ForecastByRepo`: one key, `"owner/name"`.
     - `ForecastByProfile`: one key per profile in `cfg.Repos[repo].Profiles` (additive).
     - `ForecastByCostCenter`: one key = `cfg.Repos[repo].CostCenter` or `unsetCostCenter`.
     - `ForecastByTier`: one key per distinct `cfg.Profiles[p].Tier` across the
       repo's profiles; empty tier → `unsetCostCenter` sentinel (Decision 5).
   - For each key: lookup or create `ForecastGroup`; add `point` to `ProjectedAIC`;
     fold `band` into `AICP10/50/90` (nil until first non-nil band); update
     `SampledRuns`, `WorkflowCount`, `Cold`.

### Phase B — `cmd/forecast.go`

Follows the exact same shape as `cmd/consumption.go`:

1. **`forecastFlags` struct** (period, by — no temporal modes, no source flag).

2. **`newForecastCmd(flagDir *string)`**:
   - `--period week|month` (default `week`).
   - `--by repo|profile|cost-center|tier` (default `repo`).
   - Positional `[repo...]` for `ScopeToRepos`.
   - `--output text|json` is inherited from root's persistent flag.

3. **`runForecast(cmd, flagDir, flags, repos)`**:
   - Parse `period` via `fleet.ParsePeriod`; handle json-mode envelope for errors.
   - Parse `by` via `fleet.ParseForecastGroupBy`.
   - `fleet.LoadConfig(*flagDir)`.
   - `fleet.ScopeToRepos(cfg, repos)`.
   - `fleet.AggregateForecast(ctx, cfg, period, by)`.
   - JSON mode: `writeEnvelope(cmd, commandForecast, "", false, res, warnings, nil)`.
   - Text mode: `emitConsumptionWarnings(warnings)` (reuse existing helper) +
     `renderForecastText(cmd, by, period, res)`.
   - Stderr context note: `fmt.Fprintf(cmd.ErrOrStderr(), "  (loaded %s)\n  forecast: period=%s by=%s\n", ...)`.

4. **`renderForecastText(cmd, by, period, res)`**:
   - `tabwriter.NewWriter` with `tabPadding = 2` (imported from `cmd/list.go`).
   - Header: `<AXIS_KEY>\tPROJECTED_AIC\tPROJECTED_COST\tP10\tP50\tP90\tSAMPLED\tWORKFLOWS`.
   - Per-group row: key, `formatAIC(&g.ProjectedAIC)` (always present — use non-nil
     `float64`; cold groups print `0.00`), `formatCost(g.ProjectedCostUSD)`,
     `formatBandAIC(g.AICP10)`, `formatBandAIC(g.AICP50)`, `formatBandAIC(g.AICP90)`,
     `g.SampledRuns`, `g.WorkflowCount`.
   - `formatBandAIC(*float64) string` — `-` when nil, `%.2f` when not (reuse
     `formatAIC` or add a thin alias).

5. **`forecastByColumnHeader(by fleet.ForecastGroupBy) string`** — mirrors
   `byColumnHeader` for the four forecast axes.

### Phase C — Output constant

In `cmd/output.go`, add `commandForecast = "forecast"` to the existing `const` block.

### Phase D — Root registration

In `cmd/root.go`, add `rootCmd.AddCommand(newForecastCmd(&flagDir))` alongside
the existing `newConsumptionCmd` line.

### Phase E — Skill + README updates (FR-011, FR-012)

- **`skills/fleet-budget-review/SKILL.md`**: add a `## Pre-spend forecast` section
  describing the single-turn `gh-aw-fleet forecast` flow (how it pairs with
  consumption, flags, and how to frame the output in a budget conversation).
- **`README.md`**: add the `forecast` row to the command table and a short
  paragraph covering `--period`, `--by`, positional repos, and the minimum
  gh-aw CLI version (v0.79.2).

### Phase F — CLAUDE.md / AGENTS.md update

Add a `## Recent Changes` bullet for `014-forecast-subcommand`.

## Test Plan

All tests run offline — `go test ./internal/fleet/ -run Forecast` and
`go test ./cmd/ -run Forecast`. No `gh` binary required.

| Test | File | Fixture | Path exercised |
|---|---|---|---|
| Non-zero repo, week, by-repo | `forecast_test.go` | `forecast_single_workflow.json` | happy path: one group, non-nil bands |
| Cold-start repo, by-repo | `forecast_test.go` | `forecast_cold_start.json` | cold=true, nil bands, all-cold fleet diag |
| Non-zero repo, month, by-repo | `forecast_test.go` | `forecast_single_workflow.json` | period=month: picks `monthly_*` fields |
| By-profile (multi-profile additive) | `forecast_test.go` | `forecast_single_workflow.json` | two profile keys, both get the projection |
| By-cost-center (`<unset>` bucket) | `forecast_test.go` | any | repos with no CostCenter → `<unset>` |
| By-tier | `forecast_test.go` | any | `Profile.Tier` resolution; empty → `<unset>` |
| Per-repo hard-fail skip | `forecast_test.go` | seam returning error | diagnostic emitted, other repos continue |
| Partial output (FR-013) | `forecast_test.go` | seam returning (payload, nonzeroErr) | partial diag + aggregated workflows present |
| Version gate blocks old CLI | `forecast_test.go` | ghAwVersion seam returning old ver | `ensureForecastGhAwVersion` error before any repo call |
| JSON envelope structure | `cmd/forecast_test.go` | fixtures | `schema_version:1`, `command:"forecast"`, groups present |
| Text rendering: cold group `-` dash | `cmd/forecast_test.go` | `forecast_cold_start.json` | band columns render `-` |
| Unknown positional repo | `cmd/forecast_test.go` | n/a | `ScopeToRepos` error before any network call |

## Complexity Tracking

> No Constitution violations. No new abstractions beyond what `consumption` already
> establishes. No new direct dependencies. No SchemaVersion bump. No `CommandForecast`
> constant conflict (additive). Complexity is proportional to the feature's value.

## Decision Log Cross-Reference

All numbered decisions from `research.md` are referenced below so plan readers can
trace each code choice back to its rationale:

| Code area | Decision(s) |
|---|---|
| `forecastWorkflow.pick(period)` | D1 (field names), D2 (period → days mapping) |
| `ghForecastAPI` seam | D3 (offline test seam + partial output handling) |
| `aicToUSD` reuse | D4 |
| `addForecastToGroups` tier axis | D5 (group-by axes, additive multi-profile) |
| `ForecastGroupBy` enum (separate from `GroupByKind`) | D6 |
| `ensureForecastGhAwVersion` | D7 |
| `ForecastGroup.Cold` + `SampledRuns` | D8 |
| partial-output handling in `ghForecastAPI` body | D9 |
| `commandForecast`, `SchemaVersion` unchanged | D10 |
