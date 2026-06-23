# Phase 1 Data Model: Over-Budget Highlighting

This feature adds three additive fields and one pure function. No entity is created from
scratch; the change extends existing types in `internal/fleet/consumption.go` and keeps
the new budget logic in `internal/fleet/consumption_budget.go`.

## Modified entities

### `ConsumptionResult` (envelope payload)

| Field | Type | JSON | Change | Meaning |
|-------|------|------|--------|---------|
| `Budget` | `*float64` | `budget,omitempty` | **NEW** | The AIC ceiling the operator supplied via `--budget`. `nil`/omitted when no ceiling was supplied. Echoed so consumers see the basis of the determinations. |

All existing fields (`LoadedFrom`, `FetchMode`, `GroupBy`, `Source`, `Groups`, `TopBurners`)
are unchanged.

### `ConsumptionGroup` (one rollup row)

| Field | Type | JSON | Change | Meaning |
|-------|------|------|--------|---------|
| `OverBudget` | `*bool` | `over_budget,omitempty` | **NEW** | Present only when a ceiling was supplied. `true` iff this group's `AIC` is non-nil **and** `*AIC > Budget`; `false` when a budget was supplied and the row is nil-AIC, equal-to-ceiling, or below-ceiling. Omitted in no-budget output to preserve pre-feature JSON compatibility. |

Existing fields (`Key`, `GitHubAPICalls`, `SafeOutputCalls`, `AIC`, `Cost`, `ReportCount`)
unchanged. The comparison reads the existing `AIC` field — no new aggregation.

### `WorkflowConsumption` (per-workflow row; used in `PerWorkflow` and `TopBurners`)

| Field | Type | JSON | Change | Meaning |
|-------|------|------|--------|---------|
| `OverBudget` | `*bool` | `over_budget,omitempty` | **NEW** | Same rule as `ConsumptionGroup.OverBudget`, applied to the row's `AIC`. Annotated on `TopBurners` when a budget is supplied so the footer never disagrees with the body for a workflow appearing in both (spec edge case). Omitted in no-budget output. |

Existing fields (`Workflow`, `Runs`, `APICalls`, `AvgDurationS`, `AIC`, `Cost`) unchanged.

## New behavior

### `ApplyBudget(res *ConsumptionResult, budget *float64)`

Exported, pure, no I/O. Idempotent. Signature and contract:

```go
// ApplyBudget annotates res in place with over-budget markers when budget is
// non-nil. A row is over budget iff its AIC is non-nil and strictly greater
// than *budget; nil-AIC rows and rows equal to the ceiling are never marked.
// When budget is nil the function is a no-op (no field is touched), preserving
// byte-identical output for invocations without --budget. Sets res.Budget to
// the supplied ceiling for the JSON echo.
func ApplyBudget(res *ConsumptionResult, budget *float64)
```

Logic:

1. If `budget == nil` → return immediately (no-op).
2. Set `res.Budget = budget`.
3. For each `g` in `res.Groups`: set `g.OverBudget` to a pointer whose value is `g.AIC != nil && *g.AIC > *budget`.
4. For each `w` in `res.TopBurners`: set `w.OverBudget` to a pointer whose value is `w.AIC != nil && *w.AIC > *budget`.

## Validation rules

| Input | Outcome | Exit |
|-------|---------|------|
| `--budget` absent | No annotation; `res.Budget` nil; output identical to pre-feature | 0 |
| `--budget 0` | Every row with strictly-positive AIC marked over budget | 0 (regardless of how many breach) |
| `--budget 12.5` (any ≥ 0) | Rows with `AIC > 12.5` marked | 0 (regardless of how many breach) |
| `--budget -1` (negative) | Rejected with actionable error (and `preResultFailureEnvelope` in JSON mode) | **non-zero** (input validation, FR-011/FR-012) |
| `--budget abc` (non-numeric) | Rejected by cobra `Float64Var` parse | **non-zero** (flag parse) |

Breach count never affects the exit code (FR-011): a fleet where every row is over budget
still exits 0.

## Relationships & invariants

- `OverBudget` is derived solely from the existing `AIC` field and the supplied `Budget`;
  it introduces no new data source and no network call (FR-013).
- The all-or-nothing nil-merge semantics for `AIC` (`mergeFloat`) are inherited unchanged;
  this feature only reads the resulting `*float64` (FR-009).
- `cmd.SchemaVersion` is **not** bumped — all three fields are additive and omitted when
  no budget is supplied (FR-008 / SC-003).
