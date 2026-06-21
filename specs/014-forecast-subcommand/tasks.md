# Tasks: Fleet-Wide Pre-Spend Cost Forecast

**Input**: Design documents from `/specs/014-forecast-subcommand/`
**Prerequisites**: plan.md ✅ · spec.md ✅ · research.md ✅ · data-model.md ✅ · contracts/ ✅ · quickstart.md ✅

**Tests**: Included — the offline fixture-based test suite is standard practice for this
codebase (Constitution §II requires `make ci` to pass; `ghForecastAPI` injection seam
enables fully offline coverage without a live `gh` binary).

**Organization**: Tasks are grouped by user story to enable independent implementation
and testing of each story. See `spec.md` for full acceptance criteria.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no shared state)
- **[Story]**: Which user story this task belongs to (US1–US5)
- All file paths are relative to the repository root

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the skeleton wiring so the cobra tree recognizes the subcommand
and output constants are in place before any logic is written. This unblocks parallel
development of the `internal/fleet/` and `cmd/` layers.

- [X] T001 Add `commandForecast = "forecast"` to the existing `const` block in `cmd/output.go`
- [X] T002 Create `cmd/forecast.go` with `forecastFlags` struct + `newForecastCmd` stub (flags declared, `RunE` returns `runForecast`; body is `return nil` placeholder)
- [X] T003 Register `newForecastCmd(&flagDir)` in `cmd/root.go` alongside `newConsumptionCmd`

**Checkpoint**: `go build ./...` succeeds; `go run . --help` lists `forecast` in the subcommand list.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: All shared types, the `ghForecastAPI` seam, and the version gate must
exist before any user story can call `AggregateForecast`. These are the inputs and
outputs of the package-level contract.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 Create `internal/fleet/forecast.go`; add unexported decode types `forecastPayload`, `forecastWorkflow`, `monteCarlo` (fields from `data-model.md §Upstream-decode types`) plus the `(forecastWorkflow).pick(period Period) (point float64, band *monteCarlo)` selection helper
- [X] T005 Add `ForecastGroupBy` iota enum (`ForecastByRepo`, `ForecastByProfile`, `ForecastByCostCenter`, `ForecastByTier`) with table-driven `String()` and `ParseForecastGroupBy(string) (ForecastGroupBy, error)` in `internal/fleet/forecast.go`
- [X] T006 Add `Period` iota enum (`PeriodWeek`, `PeriodMonth`) with `String()`, `Days() int` (7/30), and `ParsePeriod(string) (Period, error)` in `internal/fleet/forecast.go`
- [X] T007 Add `ghForecastAPI` package-level var seam (shells `gh aw forecast --json --repo <r> --days <7|30>`; captures stdout even on non-zero exit for FR-013 partial handling) in `internal/fleet/forecast.go`
- [X] T008 Add `ensureForecastGhAwVersion(ctx context.Context) error` (reuses `ghAwVersion` seam + `compareVersionTokens` + `CompileStrictMinVersion`) in `internal/fleet/forecast.go`
- [X] T009 Add exported result types `ForecastGroup` and `ForecastResult` (all fields from `data-model.md §Aggregation types`) in `internal/fleet/forecast.go`

**Checkpoint**: `go vet ./internal/fleet/` passes on the new file with all six additions in place.

---

## Phase 3: User Story 1 — Fleet-Wide Forecast Rollup (Priority: P1) 🎯 MVP

**Goal**: `gh-aw-fleet forecast` fans out over the fleet, aggregates per-workflow
projections by repo, and renders a text table to stdout with one row per repo.

**Independent Test**: Run `go test ./internal/fleet/ -run TestAggregateForecast` and
`go test ./cmd/ -run TestForecast`; confirm a non-empty table is printed for the
happy-path fixture and a `cold=true` row for the cold-start fixture.

### Implementation for User Story 1

- [X] T010 [US1] Implement `addForecastToGroups` (by-repo axis only for now: each workflow folds `point` into `ProjectedAIC`, band pointers into `AICP10/50/90`, `SampledRuns`, `WorkflowCount`, `Cold`) as an unexported helper in `internal/fleet/forecast.go`
- [X] T011 [US1] Implement `materializeForecastGroups(map → sorted []ForecastGroup)` in `internal/fleet/forecast.go`
- [X] T012 [US1] Implement `AggregateForecast(ctx, cfg, period, by)` body: version gate → sorted repo loop (`ctx.Err()` check) → `ghForecastAPI` call → hard-fail skip diagnostic (FR-007) → partial diagnostic (FR-013) → `addForecastToGroups` → `materializeForecastGroups` → all-cold fleet diagnostic → return `*ForecastResult, []Diagnostic, error` in `internal/fleet/forecast.go`
- [X] T013 [US1] Implement `renderForecastText(cmd, by, period, res)` (tabwriter, `REPO` header, cold group dashes, `formatBandAIC` helper for nil-aware `%.2f`) + stderr context note (`forecast: period=X by=Y`) in `cmd/forecast.go`
- [X] T014 [US1] Implement `runForecast` body in `cmd/forecast.go`: parse flags → `fleet.LoadConfig` → `fleet.ScopeToRepos` (no positional repos yet; pass nil) → `fleet.AggregateForecast` → text path calls `emitConsumptionWarnings` + `renderForecastText`
- [X] T015 [P] [US1] Write `internal/fleet/forecast_test.go`: happy path (`forecast_single_workflow.json`, period=week, by=repo → 1 group, non-nil P50), cold-start path (`forecast_cold_start.json` → cold=true, nil bands, all-cold fleet diag), hard-fail-skip path (seam returns error → diagnostic emitted, zero groups)
- [X] T016 [P] [US1] Write `cmd/forecast_test.go`: golden text-output test using fixture seam (one warm repo + one cold repo → correct column values, `-` dashes in band columns for cold group)

**Checkpoint**: `go test ./internal/fleet/ -run TestAggregateForecast` and `go test ./cmd/ -run TestForecast` both pass. `go run . forecast` (with seam wired to fixtures in a test build) renders a multi-row table.

---

## Phase 4: User Story 2 — Period Selection (Priority: P2)

**Goal**: `--period week|month` flag selects the projection horizon and passes
`--days 7|30` to the upstream `gh aw forecast` call.

**Independent Test**: Run `go test ./internal/fleet/ -run TestForecastPeriod`; confirm
`period=week` picks `weekly_projected_aic` / `weekly_monte_carlo` from the fixture and
`period=month` picks the monthly counterparts. Confirm invalid `--period xyz` returns
an error before any repo call.

### Implementation for User Story 2

- [X] T017 [US2] Wire `--period week|month` flag (default `week`) into `newForecastCmd` in `cmd/forecast.go`; parse via `fleet.ParsePeriod` in `runForecast` with json-mode envelope for parse errors
- [X] T018 [P] [US2] Add period-selection tests in `internal/fleet/forecast_test.go`: `period=week` sums `weekly_projected_aic` + picks `weekly_monte_carlo`, `period=month` sums `monthly_projected_aic` + picks `monthly_monte_carlo`, invalid period string returns actionable error
- [X] T019 [P] [US2] Add `--period month` cmd-layer test in `cmd/forecast_test.go` (projected values differ from `--period week` run or header labels correctly)

**Checkpoint**: `go test ./internal/fleet/ -run TestForecastPeriod` passes. `--period xyz` returns a clear error message immediately.

---

## Phase 5: User Story 3 — Group-By Axis (Priority: P2)

**Goal**: `--by repo|profile|cost-center|tier` slices the rollup. Multi-profile repos
contribute additively to each profile group and each tier group (same additive semantics
as `consumption --by profile`).

**Independent Test**: Run `go test ./internal/fleet/ -run TestForecastGroupBy`; confirm
a repo with two profiles contributes to both profile groups, `cost_center: ""` lands in
the `<unset>` bucket, and `Profile.Tier` drives the tier bucket.

### Implementation for User Story 3

- [X] T020 [US3] Extend `addForecastToGroups` in `internal/fleet/forecast.go` for the three remaining axes: `ForecastByProfile` (fan out over `cfg.Repos[repo].Profiles`, additive), `ForecastByCostCenter` (`cfg.Repos[repo].CostCenter` or `unsetCostCenter`), `ForecastByTier` (fan out over `cfg.Profiles[p].Tier` for each of the repo's profiles; empty tier → `unsetCostCenter`)
- [X] T021 [US3] Add `forecastByColumnHeader(by fleet.ForecastGroupBy) string` helper in `cmd/forecast.go` (returns `PROFILE`, `COST_CENTER`, `TIER`, or `REPO`) and update `renderForecastText` to use it for the dynamic key column header
- [X] T022 [P] [US3] Add group-by axis tests in `internal/fleet/forecast_test.go`: by-profile additive (two profiles → two rows summing the same projection), by-cost-center `<unset>` bucket (repo with no CostCenter), by-tier (repo on profile with Tier="standard" → `standard` row)
- [X] T023 [P] [US3] Add `--by profile` and `--by tier` cmd-layer tests in `cmd/forecast_test.go` (correct key column header, rows keyed as expected)

**Checkpoint**: `go test ./internal/fleet/ -run TestForecastGroupBy` passes. `go run . forecast --by tier` shows a tier-keyed table with a `<unset>` row for profiles without a tier.

---

## Phase 6: User Story 4 — JSON Envelope Output (Priority: P3)

**Goal**: `--output json` emits a standard fleet envelope (`schema_version: 1`,
`command: "forecast"`) with `result.groups[]` matching the shape in
`contracts/forecast-output.json`.

**Independent Test**: Run `go test ./cmd/ -run TestForecastJSON`; confirm the
envelope has all required top-level keys, each group carries `projected_aic` (numeric),
`cold` (bool), and band fields are `null` (not absent) for the cold-start group.

### Implementation for User Story 4

- [X] T024 [US4] Wire JSON envelope path in `runForecast` in `cmd/forecast.go`: on `--output json` call `writeEnvelope(cmd, commandForecast, "", false, res, warnings, nil)`; on pre-result errors call `preResultFailureEnvelope`; verify `ForecastGroup` JSON tags match `contracts/forecast-output.json` (see `data-model.md §ForecastGroup`)
- [X] T025 [P] [US4] Add `--output json` test in `cmd/forecast_test.go`: decode envelope, assert `schema_version==1`, `command=="forecast"`, `result.groups` non-empty, warm group has non-nil `aic_p50`, cold group has `null` `aic_p10`/`aic_p50`/`aic_p90` and `cold==true`; run `initSlices` behavior validation (groups is `[]` not `null`)

**Checkpoint**: `go test ./cmd/ -run TestForecastJSON` passes; output validates against `contracts/forecast-output.json` shape.

---

## Phase 7: User Story 5 — Scoped Forecast for Subset of Repos (Priority: P3)

**Goal**: Positional `owner/repo` arguments scope the rollup to only the named repos.
Unknown repo names return an error before any API call.

**Independent Test**: Pass two repo names as positional args and confirm only those two
appear in the output; pass an unknown name and confirm an error names the offender.

### Implementation for User Story 5

- [X] T026 [US5] Enable positional repo args in `runForecast` in `cmd/forecast.go`: pass `args` (cobra positional) to `fleet.ScopeToRepos(cfg, args)` (already called but passing `nil` — change to `args`); `ScopeToRepos` already validates unknown names, so error handling is automatic
- [X] T027 [P] [US5] Add positional-scope tests in `cmd/forecast_test.go`: two named repos → only two rows in output; unknown repo name → error returned before any `ghForecastAPI` call

**Checkpoint**: `go run . forecast owner/a owner/b` (with seam active) shows exactly two rows; `go run . forecast owner/unknown` exits non-zero with a clear error.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Documentation updates and CI gate. These tasks do not add new behaviors —
they satisfy FR-011, FR-012, and the Constitution's documentation invariants.

- [X] T028 [P] Add `## Pre-spend forecast` section to `skills/fleet-budget-review/SKILL.md` (FR-011): describe single-turn `gh-aw-fleet forecast` flow, how it pairs with `consumption` for a complete budget conversation, flag reference (`--period`, `--by`, `[repo...]`), and note about the advisory Monte Carlo band vs. authoritative `projected_aic`
- [X] T029 [P] Update `README.md` (FR-012): add `forecast` row to the subcommand table, add a paragraph covering `--period week|month`, `--by repo|profile|cost-center|tier`, positional scoping, minimum gh-aw CLI version (`v0.79.2`), and reference `quickstart.md` for the column semantics
- [X] T030 [P] Add `014-forecast-subcommand` bullet to `## Recent Changes` in `AGENTS.md` (the file included by CLAUDE.md via `@AGENTS.md`): one-sentence description of the new subcommand, flags, and gh-aw floor
- [X] T031 Run `make ci` (fmt-check → vet → lint → test) and fix any issues; confirm `make ci` exits 0 before marking the feature complete

**Checkpoint**: `make ci` exits 0; `go run . forecast --help` shows complete flag documentation; `skills/fleet-budget-review/SKILL.md` and `README.md` both describe the new subcommand.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 — **BLOCKS all user stories**
- **US1 (Phase 3)**: Depends on Foundational — first user story, required for all later stories to have a working base
- **US2 (Phase 4)**: Depends on Foundational — period flag can start as soon as types exist; US1 must be shippable first
- **US3 (Phase 5)**: Depends on Foundational + US1 (extends `addForecastToGroups`)
- **US4 (Phase 6)**: Depends on US1 (needs `AggregateForecast` result to wrap)
- **US5 (Phase 7)**: Depends on US1 (needs `runForecast` to pass `args` to `ScopeToRepos`)
- **Polish (Phase 8)**: Depends on all desired user stories being complete

### User Story Dependencies

- **US1 (P1)**: Can start after Foundational — no dependencies on other stories
- **US2 (P2)**: Can start after US1 — period flag is a thin layer over types already in Foundational; best delivered on a working US1 base
- **US3 (P2)**: Can start after US1 — extends `addForecastToGroups` which US1 introduced
- **US4 (P3)**: Can start after US1 — wraps existing `runForecast`
- **US5 (P3)**: Can start after US1 — single-line change to `runForecast`; US2/US3/US4 independent

### Within Each Phase

- All same-file tasks must be sequential (Go files can't be edited by two tasks simultaneously)
- Tests marked [P] can start once the implementation task they cover is complete
- T015 and T016 (US1 tests) can run in parallel since they target different packages

### Parallel Opportunities

```text
Phase 1:        T001, T002, T003 are independent (different files)
Phase 2:        T004 → T005 → T006 → T007 → T008 → T009 (all same file, sequential)
Phase 3:        T010 → T011 → T012 → T013 → T014  (same file chain)
                T015 [P] and T016 [P] can start after T014 completes (different files)
Phase 4:        T017 → T018 [P] + T019 [P]  (T018/T019 after T017, parallel with each other)
Phase 5:        T020 → T021 → T022 [P] + T023 [P]
Phase 6:        T024 → T025 [P]
Phase 7:        T026 → T027 [P]
Phase 8:        T028 [P], T029 [P], T030 [P] all in parallel; T031 after all three
```

---

## Parallel Example: User Story 1

```bash
# After T014 completes (runForecast wired), launch test authoring in parallel:
Task: "Write internal/fleet/forecast_test.go for US1 (T015)"   # tests fleet-layer logic
Task: "Write cmd/forecast_test.go for US1 golden output (T016)" # tests cmd rendering
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup (T001–T003) — 3 tasks
2. Phase 2: Foundational (T004–T009) — 6 tasks
3. Phase 3: User Story 1 (T010–T016) — 7 tasks
4. **STOP and VALIDATE**: `go test ./internal/fleet/ -run Forecast` and `go test ./cmd/ -run Forecast` pass
5. Optional manual smoke-test: `go run . forecast` against real fleet (read-only, no `--apply`)

**MVP total**: 16 tasks; delivers fleet-wide repo-keyed forecast table.

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. US1 (T010–T016) → Fleet rollup table, by=repo, period=week ← **shippable MVP**
3. US2 (T017–T019) → `--period week|month` ← period parity with upstream
4. US3 (T020–T023) → `--by profile|cost-center|tier` ← budget-review skill integration
5. US4 (T024–T025) → `--output json` ← downstream tooling support
6. US5 (T026–T027) → positional repo scoping ← rollout-preview use case
7. Polish (T028–T031) → docs, skill, `make ci` ← merge-ready

Each increment is `make ci`-clean and independently useful.

### Parallel Team Strategy

With two developers after Foundational phase:

- **Dev A**: US1 → US3 (aggregation logic, group-by axes)
- **Dev B**: US2 flag wiring → US4 JSON envelope → US5 scoping → Polish

Stories are independently testable. Merge order: US1 first (both depend on it), then US2–US5 in any order.

---

## Notes

- **[P] means different files**: Tasks in the same file (e.g., all additions to `internal/fleet/forecast.go`) must be sequential to avoid conflicts
- **[Story] maps to spec.md user stories**: US1=Story 1, US2=Story 2, ... US5=Story 5
- **No new direct dependencies**: Every package used is stdlib or already in `go.mod` (Constitution §Third-Party Dependencies — verified in plan.md Constitution Check)
- **`make ci` is the gate**: `go build` alone is not sufficient — prior commits have landed lint failures that build+vet didn't catch (see AGENTS.md "Local gate")
- **Fixtures are already committed**: `internal/fleet/testdata/forecast/forecast_single_workflow.json` and `forecast_cold_start.json` exist; no fixture-creation tasks needed
- **Cold groups**: `AICP10/50/90` are `*float64`; a `nil` pointer marshals as JSON `null`. The `omitempty` tag must NOT be on these fields (FR-005 requires `null`, not absent)
- **Band summation is approximate**: documented in `data-model.md §ForecastGroup`; no test should assert statistical accuracy of the summed percentiles
- **`make lint` may exceed 5 minutes**: use extended timeout when running locally (`golangci-lint` note in AGENTS.md)
