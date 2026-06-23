# Tasks: Read-Only Over-Budget Highlighting in the Consumption Rollup

**Input**: Design documents from `/specs/017-consumption-budget/`
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Required by the feature specification success criteria. Test tasks are listed before implementation tasks in each user-story phase.

**Organization**: Tasks are grouped by user story so each increment can be implemented and validated independently.

## Phase 1: Setup

**Purpose**: Establish the exact implementation surface before editing.

- [X] T001 Review the budget feature scope in `specs/017-consumption-budget/spec.md`, `specs/017-consumption-budget/plan.md`, and `specs/017-consumption-budget/research.md`
- [X] T002 [P] Review existing consumption data structures and aggregation flow in `internal/fleet/consumption.go`
- [X] T003 [P] Review existing consumption CLI flag, rendering, and validation patterns in `cmd/consumption.go` and `cmd/consumption_test.go`
- [X] T004 [P] Review the operator budget-review guidance baseline in `skills/fleet-budget-review/SKILL.md`

---

## Phase 2: Foundational

**Purpose**: Shared test scaffolding needed by all story phases.

**Critical**: No user-story implementation should begin until these helpers exist.

- [X] T005 Add shared float-pointer, bool-pointer, and minimal `ConsumptionResult` fixture helpers for budget tests in `internal/fleet/consumption_budget_test.go`
- [X] T006 [P] Add a shared command/output-buffer helper for `renderConsumptionText` tests in `cmd/consumption_test.go`

**Checkpoint**: Budget tests can be added without repeating fixture setup.

---

## Phase 3: User Story 1 - Flag Over-Threshold Rows at a Glance (Priority: P1) MVP

**Goal**: `gh-aw-fleet consumption --budget <AIC>` marks primary table rows whose AIC strictly exceeds the supplied ceiling, while no-budget output remains unchanged.

**Independent Test**: Run budget tests with rows above, below, equal to, and missing the ceiling value; confirm only rows with `AIC > budget` are marked and no `OVER` column appears when no budget was supplied.

### Tests for User Story 1

- [X] T007 [P] [US1] Add `TestApplyBudget_StrictThreshold` covering above-ceiling, below-ceiling, equal-to-ceiling, nil-AIC, and zero-ceiling group rows in `internal/fleet/consumption_budget_test.go`
- [X] T008 [P] [US1] Add `TestRenderConsumptionText_BudgetColumn` covering the primary table `OVER` column, `!` marker, empty compliant cells, and absent-budget output in `cmd/consumption_test.go`
- [X] T009 [US1] Add `TestConsumption_BudgetFlagValidation` covering negative budget rejection, zero budget acceptance, and non-numeric cobra parse rejection in `cmd/consumption_test.go`
- [X] T010 [US1] Add `TestConsumption_BudgetBreachesExitZero` covering zero, some, and all rows over budget with successful command exit in `cmd/consumption_test.go`

### Implementation for User Story 1

- [X] T011 [US1] Add `Budget *float64`, `ConsumptionGroup.OverBudget *bool`, and `WorkflowConsumption.OverBudget *bool` fields with godoc-compatible comments and `omitempty` JSON tags in `internal/fleet/consumption.go`
- [X] T012 [US1] Implement `ApplyBudget(res *ConsumptionResult, budget *float64)` for primary `ConsumptionResult.Groups` rows using strict `AIC > budget` semantics in new `internal/fleet/consumption_budget.go`
- [X] T013 [US1] Add the `--budget` cobra flag using `Float64Var` and `cmd.Flags().Changed("budget")` supplied-detection in `cmd/consumption.go`
- [X] T014 [US1] Reject negative `--budget` values before config loading and pass a non-nil budget pointer to `fleet.ApplyBudget` after aggregation in `cmd/consumption.go`
- [X] T015 [US1] Append the primary-table `OVER` header and per-group marker cells only when `res.Budget != nil` in `cmd/consumption.go`
- [X] T016 [US1] Run targeted US1 validation for `internal/fleet/consumption_budget_test.go` and `cmd/consumption_test.go`

**Checkpoint**: `--budget` works for the default repo table, no-budget text output stays unchanged, and malformed budget input fails as usage validation rather than breach enforcement.

---

## Phase 4: User Story 2 - Honor the Active Grouping Axis (Priority: P1)

**Goal**: The budget threshold applies to whichever row set the active `--by` axis produced, including profile double-counting semantics and the top-burners footer.

**Independent Test**: Run the rollup budget tests for repo, profile, cost-center, and workflow-shaped result rows plus top-burner rows; confirm the over-budget set matches each row's already-aggregated AIC.

### Tests for User Story 2

- [X] T017 [P] [US2] Add `TestApplyBudget_AllGroupingAxes` with repo, profile, cost-center, and workflow-shaped result fixtures in `internal/fleet/consumption_budget_test.go`
- [X] T018 [US2] Add `TestApplyBudget_TopBurners` covering over, equal, below, and nil-AIC top-burner rows in `internal/fleet/consumption_budget_test.go`
- [X] T019 [P] [US2] Add `TestRenderConsumptionText_TopBurnersBudgetColumn` covering the footer `OVER` header and marker cells in `cmd/consumption_test.go`

### Implementation for User Story 2

- [X] T020 [US2] Extend `ApplyBudget` to annotate `ConsumptionResult.TopBurners` using the same strict AIC comparison in `internal/fleet/consumption_budget.go`
- [X] T021 [US2] Append the top-burners footer `OVER` header and per-workflow marker cells only when `res.Budget != nil` in `cmd/consumption.go`
- [X] T022 [US2] Run targeted US2 validation for `internal/fleet/consumption_budget_test.go` and `cmd/consumption_test.go`

**Checkpoint**: The threshold is a pure post-aggregation pass over the active axis and the footer cannot disagree with workflow rows.

---

## Phase 5: User Story 3 - Machine-Readable Over-Budget Signal (Priority: P2)

**Goal**: JSON output exposes the same over-budget determination as data, echoes the supplied ceiling, and keeps the schema version unchanged.

**Independent Test**: Marshal a budget-applied result and run JSON-mode validation; confirm `budget`, `over_budget`, and `schema_version` match the additive contract.

### Tests for User Story 3

- [X] T023 [P] [US3] Add `TestConsumptionResult_JSONBudgetFields` covering `budget,omitempty`, no-budget omission of `over_budget`, and present true/false group/top-burner `over_budget` fields when a budget is supplied in `internal/fleet/consumption_budget_test.go`
- [X] T024 [P] [US3] Add `TestConsumption_JSONNegativeBudgetEnvelope` confirming JSON mode returns the standard pre-result failure envelope for negative `--budget` in `cmd/consumption_test.go`
- [X] T025 [US3] Add `TestConsumption_SchemaVersionUnchangedForBudget` confirming `cmd.SchemaVersion` remains unchanged when budget fields are present in `cmd/consumption_test.go`

### Implementation for User Story 3

- [X] T026 [US3] Ensure `ConsumptionResult.Budget` uses `json:"budget,omitempty"` and both over-budget fields use `json:"over_budget,omitempty"` pointer tags in `internal/fleet/consumption.go`
- [X] T027 [US3] Ensure `ApplyBudget(nil budget)` is a no-op and `ApplyBudget(non-nil budget)` sets `ConsumptionResult.Budget` plus per-row `OverBudget` pointers for envelope echoing in `internal/fleet/consumption_budget.go`
- [X] T028 [US3] Keep `SchemaVersion` unchanged while writing annotated results through `writeEnvelope` in `cmd/output.go` and `cmd/consumption.go`
- [X] T029 [US3] Run targeted US3 validation for `internal/fleet/consumption_budget_test.go`, `cmd/consumption_test.go`, and `cmd/output.go`

**Checkpoint**: JSON consumers can read budget state additively, and consumers that ignore new fields keep working.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, governance, and local quality gate.

- [X] T030 [P] Add the highlight-versus-alarm decision record in `specs/009-consumption-subcommand/decisions/0001-highlight-not-alarm.md`
- [X] T031 [P] Update budget-review operator guidance for `--budget`, AIC units, strict comparison, nil-AIC exclusion, and no enforcement in `skills/fleet-budget-review/SKILL.md`
- [X] T032 Run one realistic subagent test of `skills/fleet-budget-review/SKILL.md` with hard stops before destructive actions, and record the evidence in the implementation notes or PR body
- [X] T033 [P] Validate quickstart examples and acceptance checks against the final CLI behavior in `specs/017-consumption-budget/quickstart.md`
- [X] T034 Run `make fmt` and fix formatting in `internal/fleet/consumption.go`, `internal/fleet/consumption_budget.go`, `internal/fleet/consumption_budget_test.go`, `cmd/consumption.go`, and `cmd/consumption_test.go`
- [X] T035 Run `make ci` and fix any gate failures in `internal/fleet/consumption.go`, `internal/fleet/consumption_budget.go`, `internal/fleet/consumption_budget_test.go`, `cmd/consumption.go`, `cmd/consumption_test.go`, `cmd/output.go`, and `skills/fleet-budget-review/SKILL.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup completion; blocks story test work.
- **US1 (Phase 3)**: Depends on Foundational; provides the MVP table marker and CLI flag.
- **US2 (Phase 4)**: Depends on US1's `ApplyBudget` and render plumbing; extends coverage to every axis and the top-burners footer.
- **US3 (Phase 5)**: Depends on US1's fields and `ApplyBudget`; can run after US1, but should validate after US2 so top-burner JSON is covered too.
- **Polish (Phase 6)**: Depends on the implemented story scope relevant to the docs and final gate.

### User Story Dependencies

- **US1 (P1)**: Required MVP. No dependency on other stories after Foundational.
- **US2 (P1)**: Depends on US1's core budget annotation but remains independently testable with prebuilt result fixtures for each axis.
- **US3 (P2)**: Depends on US1's additive fields and budget echo; should be delivered after P1 behavior is stable.

### Within Each User Story

- Add tests first and confirm they fail for the missing behavior.
- Add shared data fields before the pure `ApplyBudget` behavior that uses them.
- Keep `ApplyBudget` in `internal/fleet/consumption_budget.go`; `internal/fleet/consumption.go` receives only the struct-field additions required by existing type definitions.
- Wire CLI flag parsing before renderer changes that depend on `res.Budget`.
- Keep breach detection independent from exit-code decisions; malformed input is the only non-zero path.

---

## Parallel Opportunities

- T002, T003, and T004 can run in parallel during setup.
- T005 and T006 touch different test files and can run in parallel.
- T007 and T008 can run in parallel; T009 and T010 share `cmd/consumption_test.go` with T008 and should be sequenced by a single editor.
- T017 and T019 can run in parallel; T018 shares `internal/fleet/consumption_budget_test.go` with T017 and should be sequenced by a single editor.
- T023 and T024 can run in parallel; T025 shares `cmd/consumption_test.go` with T024 and should be sequenced by a single editor.
- T030, T031, and T033 are documentation tasks in different files and can run in parallel; T032 must run after T031.

---

## Parallel Example: User Story 1

```text
Task: "T007 [US1] Add TestApplyBudget_StrictThreshold in internal/fleet/consumption_budget_test.go"
Task: "T008 [US1] Add TestRenderConsumptionText_BudgetColumn in cmd/consumption_test.go"
```

After those tests exist, sequence T009 and T010 with the same `cmd/consumption_test.go` editor, then implement T011 through T015 and run T016.

---

## Parallel Example: User Story 2

```text
Task: "T017 [US2] Add TestApplyBudget_AllGroupingAxes in internal/fleet/consumption_budget_test.go"
Task: "T019 [US2] Add TestRenderConsumptionText_TopBurnersBudgetColumn in cmd/consumption_test.go"
```

Sequence T018 after T017 because both edit `internal/fleet/consumption_budget_test.go`.

---

## Parallel Example: User Story 3

```text
Task: "T023 [US3] Add TestConsumptionResult_JSONBudgetFields in internal/fleet/consumption_budget_test.go"
Task: "T024 [US3] Add TestConsumption_JSONNegativeBudgetEnvelope in cmd/consumption_test.go"
```

Sequence T025 after T024 because both edit `cmd/consumption_test.go`.

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 and Phase 2.
2. Complete Phase 3 (US1) with tests first.
3. Validate strict greater-than behavior, nil-AIC exclusion, zero ceiling, negative input rejection, and no-budget table compatibility.
4. Stop if only the MVP table highlight is needed.

### Incremental Delivery

1. Deliver US1 to make `--budget` useful for the default repo view.
2. Add US2 so the same ceiling is correct for every `--by` axis and top-burner rows.
3. Add US3 so downstream JSON consumers see the same determination and the budget echo.
4. Finish Phase 6 docs, run the affected skill subagent test, and run the full local gate.

### Single-Developer Sequence

This feature touches `internal/fleet/consumption.go`, `internal/fleet/consumption_budget.go`, `internal/fleet/consumption_budget_test.go`, `cmd/consumption.go`, and `cmd/consumption_test.go` across all stories. A single developer should work in task order to avoid file conflicts, using only the marked documentation tasks for true parallel work.
