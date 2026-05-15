# Data Model: Consumption Subcommand

**Feature**: 009-consumption-subcommand
**Plan**: [plan.md](./plan.md)
**Status**: Phase 1 — fully derived from spec.md's Key Entities + the typed APIs declared in the originating issue body.

## Scope

This document declares the Go types added under `internal/fleet/consumption.go`. Every exported identifier here will carry godoc-comment-as-one-complete-sentence per CLAUDE.md "Code self-documentation" rules. JSON tag choices honor the project's existing `omitempty`-for-truly-optional vs. always-emit-for-known-shape convention (compare `ListRow.CostCenter` always-emitted vs. `ProfileTiers` always-emitted-as-empty-map at `internal/fleet/list_result.go:19-35`).

---

## Layer 1 — Discovery (internal only)

These types are package-private; they exist only within the discovery → filter → aggregate pipeline and are not surfaced on the JSON envelope.

### `reportRef` *(unexported)*

One row of the discovery layer's index. Identifies a consumption discussion + its referenced workflow run + the temporal-filter metadata needed by `shouldIncludeReport`.

```go
type reportRef struct {
    Repo       string    // "owner/name" tracked in fleet config
    RunID      int64     // captured from the actions/runs/{id}/agentic_workflow link
    Date       time.Time // parsed from the discussion title (YYYY-MM-DD)
    Expires    time.Time // parsed from <!-- gh-aw-expires: ISO --> marker
    InProgress bool      // true when body contains "🔄 in-progress"
    URL        string    // discussion html_url, for diagnostic-warning copy
}
```

Construction: built by `discoverReports(ctx, repo)` from the discussion JSON array. One per matching discussion. No method surface.

### `fetchKind` *(unexported)*

Sum-type discriminant for the temporal-mode selector.

```go
type fetchKind int

const (
    fetchLatest fetchKind = iota
    fetchTrailing
    fetchSince
)
```

### `FetchMode` *(exported because the CLI layer in `cmd/consumption.go` builds it)*

The one-of selector populated from the mutually-exclusive `--latest` / `--trailing` / `--since` flags. The CLI layer validates exactly one is set and constructs this.

```go
type FetchMode struct {
    Kind  fetchKind  // which arm is active
    Days  int        // populated when Kind == fetchTrailing; > 0
    Since time.Time  // populated when Kind == fetchSince; UTC midnight of the input date
}
```

**Invariants**:

- Exactly one of `Days > 0` and `!Since.IsZero()` is non-zero, gated by `Kind`. The constructor at the CLI layer enforces this; helpers downstream assume it.
- `Kind == fetchLatest` ⇒ both `Days == 0` and `Since.IsZero()`.

---

## Layer 2 — Reports (parsed; consumed by aggregation)

### `ConsumptionReport`

One per-repo per-day attribution record. Built by `fetchRunArtifacts(ctx, ref)` from the run's artifact payload. The repo-level join (profile, cost_center) happens at aggregation time, not at fetch time, so a freshly-parsed report carries empty `Profile` / `CostCenter` until `AggregateConsumption` joins it against `*fleet.Config`.

```go
// ConsumptionReport is one repo-day's worth of attribution data.
// Built from a discovered reportRef plus the workflow run artifacts it points to.
type ConsumptionReport struct {
    Repo            string                `json:"repo"`
    Date            time.Time             `json:"date"`
    RunID           int64                 `json:"run_id"`
    GitHubAPICalls  int                   `json:"github_api_calls"`
    SafeOutputCalls int                   `json:"safe_output_calls"`
    Cost            *float64              `json:"cost,omitempty"`        // nil-until-populated (Decision 6)
    PerWorkflow     []WorkflowConsumption `json:"per_workflow"`
    Profile         string                `json:"profile,omitempty"`     // joined from fleet config at aggregation
    CostCenter      string                `json:"cost_center,omitempty"` // joined from fleet config at aggregation
}
```

**JSON-tag policy**:

- `cost`: `*float64` + `omitempty` per Decision 6 (zero-or-absent both render as absent).
- `profile`, `cost_center`: `omitempty` because they're joined data — absent on a freshly-parsed report, populated on an aggregated report. A `ConsumptionReport` value with neither still serializes cleanly.
- `per_workflow`: always-emitted; `initSlices` (cmd/output.go) normalizes nil to `[]`.

### `WorkflowConsumption`

One row in the per-workflow breakdown table. Used both as the unit inside `ConsumptionReport.PerWorkflow` and as the standalone row type emitted into `ConsumptionResult.TopBurners`.

```go
// WorkflowConsumption is one row in the per-workflow breakdown table.
type WorkflowConsumption struct {
    Workflow     string   `json:"workflow"`
    Runs         int      `json:"runs"`
    APICalls     int      `json:"api_calls"`
    AvgDurationS float64  `json:"avg_duration_s"`
    Cost         *float64 `json:"cost,omitempty"`
}
```

**JSON-tag policy**:

- `cost`: same nil-until-populated semantic as `ConsumptionReport.Cost`.
- All other fields always-emitted including zero values; downstream consumers gate on `APICalls > 0` etc.

---

## Layer 3 — Result envelope (consumed by output writer)

### `ConsumptionResult`

The top-level result struct embedded in the JSON envelope under `result:`. Mirrors `ListResult`'s shape and field-naming conventions at `internal/fleet/list_result.go:9`.

```go
// ConsumptionResult is the JSON envelope payload for `gh-aw-fleet consumption`.
// Slice fields are normalized to non-nil empty slices by initSlices (cmd/output.go)
// so JSON marshaling renders them as [] (FR-019 envelope contract).
type ConsumptionResult struct {
    LoadedFrom string             `json:"loaded_from"`  // mirrors ListResult.LoadedFrom
    FetchMode  string             `json:"fetch_mode"`   // human-readable: "latest" | "trailing-7d" | "since-2026-04-01"
    GroupBy    string             `json:"group_by"`     // "repo" | "profile" | "cost-center" | "workflow"
    Groups     []ConsumptionGroup `json:"groups"`
    TopBurners []WorkflowConsumption `json:"top_burners"`
}
```

### `ConsumptionGroup`

One row in `ConsumptionResult.Groups`. Keyed on whichever axis the `--by` selector chose; sums consumption across every contributing report.

```go
// ConsumptionGroup is one aggregated row in the consumption rollup.
// The Key field's meaning depends on the GroupBy axis on the parent result:
//   - GroupBy == "repo":         Key is "owner/name"
//   - GroupBy == "profile":      Key is the profile name (e.g., "standard")
//   - GroupBy == "cost-center":  Key is the cost-center value, or "<unset>" for repos without one
//   - GroupBy == "workflow":     Key is the workflow name
type ConsumptionGroup struct {
    Key             string   `json:"key"`
    GitHubAPICalls  int      `json:"github_api_calls"`
    SafeOutputCalls int      `json:"safe_output_calls"`
    Cost            *float64 `json:"cost,omitempty"`
    ReportCount     int      `json:"report_count"`  // # of ConsumptionReport rows that summed into this group
}
```

**Sort order**: `Groups` is sorted by `Key` ascending in the result, mirroring `ListResult.Repos` which is alphabetic. (`TopBurners` is sorted descending by ranking metric — see Decision 9.)

---

## Validation rules and invariants

| Invariant | Enforced by | Behavior on violation |
|---|---|---|
| Exactly one of `--latest`, `--trailing`, `--since` is set | cobra `MarkFlagsMutuallyExclusive` at CLI layer (FR-004) | Cobra returns non-zero with the three-mode error message |
| `--by` value is in `{repo, profile, cost-center, workflow}` | switch statement in `cmd/consumption.go`'s RunE (FR-005) | Returns `fmt.Errorf("--by value %q invalid: expected one of %s", v, validList)`; cobra propagates |
| `--trailing` value matches `^(\d+)d$` | small helper `parseTrailing(s string) (int, error)` (Decision 4) | Returns explicit error naming the accepted form |
| `--since` value parses as `2006-01-02` | `time.Parse` in CLI layer | Cobra returns non-zero with the date-format error message |
| `Cost ≤ 0` is treated as nil | `parseCost(raw any) *float64` helper inside the artifact-decoder (Decision 6) | Stored value becomes `nil`, serialized via `omitempty` |
| `Groups` and `TopBurners` are always non-nil | `initSlices` walker (existing, `cmd/output.go:179`) | nil → empty slice automatically; no field-level annotation needed |
| `ConsumptionResult` populated only when discovery + fetch succeed for at least one repo | `AggregateConsumption(cfg, mode, by)` returns `(*ConsumptionResult, []Diagnostic, error)` | Returns a zero-`Groups` result with diagnostics surfaced — exit code stays 0, FR-010 |
| In-progress reports omitted in `fetchLatest`, included with warning in `fetchTrailing`/`fetchSince` | `shouldIncludeReport(ref, mode, now) (bool, *Diagnostic)` (FR-011, FR-012) | Filter returns include=false + warning for the latest case; include=true + warning for the trailing/since cases |
| Expired reports always excluded | `shouldIncludeReport` (FR-013) | Filter returns include=false; in `fetchLatest`, surfaces "try --trailing 7d" hint when every candidate is excluded |

## State transitions

No state transitions in the persistence sense — the subcommand is stateless. The conceptual transitions inside one invocation:

```text
                                 ┌─────────────┐
       LoadConfig(flagDir) ─────►│ *fleet.Config│
                                 └──────┬──────┘
                                        │
                          per repo:     ▼
                ┌──────────────────────────────────┐
                │  discoverReports(ctx, repo)      │
                │  → []reportRef                   │
                └──────────┬───────────────────────┘
                           │
            FetchMode ────►│ shouldIncludeReport(ref, mode, now)
                           │ → (include bool, warn *Diagnostic)
                           ▼
                ┌──────────────────────────────────┐
                │  fetchRunArtifacts(ctx, ref)     │
                │  → *ConsumptionReport            │
                └──────────┬───────────────────────┘
                           │
                           ▼
                ┌──────────────────────────────────┐
                │  AggregateConsumption(cfg,       │
                │    reports, by)                  │
                │  → *ConsumptionResult            │
                └──────────┬───────────────────────┘
                           │
                           ▼
                  text tabwriter
                  OR JSON envelope
```

## Relationships to existing types

| New type | Existing type | Relationship |
|---|---|---|
| `ConsumptionReport.Profile` | `fleet.RepoSpec.Profiles []string` | Aggregation reads `cfg.Repos[report.Repo].Profiles` and emits one report per profile per repo (additive double-count, Decision 5) |
| `ConsumptionReport.CostCenter` | `fleet.RepoSpec.CostCenter string` (007-billing-metadata-fields) | Aggregation reads `cfg.Repos[report.Repo].CostCenter` and uses it as the group key when `--by cost-center` |
| `ConsumptionGroup.Key` (when grouping by tier) | `fleet.Profile.Tier string` (007-billing-metadata-fields) | Not directly grouped on in v1 (issue's spec did not list `--by tier`), but the aggregator joins `Profile.Tier` for forward-compatibility when an operator later wants to display it. Initial implementation omits this until a follow-up issue requests it. |
| `ConsumptionResult` | `cmd.Envelope.Result any` (cmd/output.go:50) | Embedded directly; `cmd.SchemaVersion = 1` does not bump |
| Diagnostics emitted during discovery / fetch | `[]fleet.Diagnostic` consumed by `writeEnvelope`'s `warnings` arg | Each warning carries one of the existing codes (Decision 8) |

## What this model deliberately does not include

- **No persisted state**. The model has no Save/Load methods; no on-disk cache; no baseline file. Each invocation re-discovers from scratch (FR-022, Constitution Principle IV exception).
- **No tier grouping in v1**. The spec scoped `--by` to four axes (repo, profile, cost-center, workflow). Tier grouping is a one-line addition when the operator requests it; the `fleet.Profile.Tier` field is already available.
- **No historical-attribution tracking**. The model joins reports against *current* fleet config, not the config-as-of-when-the-report-ran (Assumptions §5 in spec.md).
- **No parallel fetch primitives**. Decision 7's seam is a function variable — concurrency happens (if added later) inside the aggregator's repo loop, not in the type system.
