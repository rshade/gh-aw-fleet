# Implementation Plan: Read-Only Over-Budget Highlighting in the Consumption Rollup

**Branch**: `017-consumption-budget` | **Date**: 2026-06-22 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/017-consumption-budget/spec.md`

## Summary

Add an optional, read-only `--budget <AIC>` flag to `gh-aw-fleet consumption` that highlights which rollup rows exceed a per-row AI-credit ceiling. The determination is a pure post-aggregation pass over the `*ConsumptionResult` that `AggregateConsumption` already builds: a new `ApplyBudget` function sets a per-row over-budget flag (when the row's `AIC` is non-nil and strictly greater than the ceiling) on every `ConsumptionGroup` and every `TopBurners` row, and echoes the ceiling on the result envelope. The text renderer gains an `OVER` column (only when a budget is supplied); the JSON envelope gains additive `over_budget` (per group / per top-burner) and `budget` (envelope-level) fields only when a budget is supplied, preserving no-budget JSON compatibility and requiring no `cmd.SchemaVersion` bump. The feature stays strictly on the highlight side of the 009 spec's FR-023 ("no alarm"): no enforcement, no external signal, exit code unaffected by breaches (only malformed input — a negative or non-numeric ceiling — exits non-zero). A decision record under `specs/009-consumption-subcommand/decisions/` reconciles "highlight" with FR-023.

## Technical Context

**Language/Version**: Go 1.26.4 (local development gate; `go.mod` directive `go 1.26.4`)
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (CLI flag), `github.com/rs/zerolog` v1.35.1 (stderr warnings) — both pre-existing. Standard library only for new logic (`strconv` already imported in `internal/fleet/consumption.go`; `text/tabwriter` already imported in `cmd/consumption.go`). **No new third-party dependency.**
**Storage**: N/A — pure read/post-process of the in-memory rollup result. No on-disk state, no cache, no network beyond what the rollup already performs (FR-013).
**Testing**: `go test ./...` via `make test`; table-driven unit tests over the existing `internal/fleet/testdata/{consumption,logs}/` fixtures plus the `cmd/consumption_test.go` flag-validation pattern.
**Target Platform**: Linux/macOS CLI (same as the rest of the tool).
**Project Type**: Single-project Go CLI (thin orchestrator).
**Performance Goals**: No added latency — the budget pass is O(groups + top-burners) over an already-materialized slice, no I/O. Well within the constitution's 5-minute ceiling (Principle IV).
**Constraints**: Read-only / no enforcement (FR-010); exit code invariant under breaches (FR-011); `cmd.SchemaVersion` MUST NOT bump (FR-008); native AIC unit, not derived USD (spec Assumptions); strictly-greater-than semantics (FR-002); nil-AIC groups are not over budget (FR-009).
**Scale/Scope**: Typical fleet ≤10 repos × ≤20 workflows. Net new code is small: one flag, one pure exported function (`ApplyBudget`), two additive struct fields, one render column, one decision record, and tests.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against the gh-aw-fleet Constitution v1.2.0:

- **I. Thin-Orchestrator Code Quality** — PASS. No upstream-tool logic is re-implemented; the feature is local presentation/annotation over data the rollup already computes. No new `exec.Command` calls. `internal/fleet/consumption.go` is already beyond the 300-line refactor trigger, so only the unavoidable struct-field additions land there; the new budget pass lives in a focused sibling file (`internal/fleet/consumption_budget.go`) with focused tests. Errors wrapped with `%w`; exported identifiers get godoc (`ApplyBudget`, `ConsumptionResult.Budget`, `ConsumptionGroup.OverBudget`, `WorkflowConsumption.OverBudget`).
- **II. Testing Standards** — PASS. The command is read-only, so the mutating-command dry-run requirement does not apply. `go build` / `go vet` clean is mandatory. Unit tests are not constitution-required but are an explicit acceptance criterion here and are a natural fit (stateless pure function); they run fully offline against existing fixtures (no live `gh`). Because this feature updates `skills/fleet-budget-review/SKILL.md`, the affected skill must also receive one realistic subagent test before shipping, with hard stops before destructive actions.
- **III. User Experience Consistency** — PASS (with note). The three-turn mutation pattern governs commands that mutate external state; `consumption` mutates nothing, so it is out of that pattern's scope. The feature reinforces UX consistency by mirroring the existing flag/validation idioms (`ParseGroupBy`, `buildFetchMode`) and the `--output json` envelope contract.
- **IV. Performance Requirements** — PASS. No new network or disk I/O; a single linear pass over already-materialized slices. No caching concerns (nothing fetched).
- **Third-Party Dependencies** — PASS. No new direct dependency; no `go.mod` `require()` change; no amendment needed.

**Result: PASS — no violations. Complexity Tracking table intentionally omitted.**

## Project Structure

### Documentation (this feature)

```text
specs/017-consumption-budget/
├── plan.md              # This file (/speckit-plan command output)
├── spec.md              # Feature spec (/speckit-specify output)
├── research.md          # Phase 0 output (/speckit-plan)
├── data-model.md        # Phase 1 output (/speckit-plan)
├── quickstart.md        # Phase 1 output (/speckit-plan)
├── contracts/           # Phase 1 output (/speckit-plan)
│   ├── budget-flag.md            # CLI flag contract: --budget
│   ├── consumption-output.json   # Updated JSON envelope (additive budget/over_budget)
│   └── consumption-text-output.md # Updated text contract: OVER column
├── checklists/
│   └── requirements.md  # Spec quality checklist (/speckit-specify output)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/fleet/
├── consumption.go             # ADD: unavoidable struct fields only:
│                              #      ConsumptionResult.Budget,
│                              #      ConsumptionGroup.OverBudget,
│                              #      WorkflowConsumption.OverBudget.
├── consumption_budget.go      # NEW: ApplyBudget(res, *float64) pure post-aggregation
│                              #      pass. (AggregateConsumption unchanged.)
└── consumption_budget_test.go # ADD: table-driven ApplyBudget tests (strictly-greater,
                               #      equal, nil-AIC, zero-ceiling, top-burner annotation,
                               #      every-axis, no-budget JSON compatibility).

cmd/
├── consumption.go        # ADD: --budget flag (Float64Var + Changed() supplied-detection),
│                         #      negative/parse validation, ApplyBudget wiring, OVER column
│                         #      in renderConsumptionText + top-burners footer.
└── consumption_test.go   # ADD: flag-validation tests (negative rejected non-zero exit;
                          #      zero accepted; absent → no OVER column / identical output;
                          #      exit code unaffected by breach count).

specs/009-consumption-subcommand/
└── decisions/
    └── 0001-highlight-not-alarm.md  # NEW: reconciles --budget highlight with FR-023 (FR-014).

skills/fleet-budget-review/
└── SKILL.md              # UPDATE: document --budget, unit, strictly-greater semantics,
                          #         nil-AIC exclusion, and the highlight≠enforce statement (FR-015).
```

**Structure Decision**: Single-project Go CLI; the change keeps the existing split between `internal/fleet` aggregation/logic and `cmd` flag/rendering, but avoids adding new logic to the already-large `internal/fleet/consumption.go`. That file receives only the struct-field additions that must live with the existing types; the new budget pass and its focused tests live in `internal/fleet/consumption_budget.go` and `internal/fleet/consumption_budget_test.go`. The command layer remains in `cmd/consumption.go` plus command tests; `cmd/output.go` is a validation surface for the unchanged envelope schema, not a primary implementation surface. The decision record lands under the existing 009 spec directory (new `decisions/` subfolder); the skill update layers onto the current `skills/fleet-budget-review/SKILL.md` and is verified by a subagent skill test before shipping.

## Complexity Tracking

> No Constitution Check violations — table intentionally omitted.
