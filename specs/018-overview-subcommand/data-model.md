# Phase 1 Data Model: Fleet Overview Subcommand

All types live in `internal/fleet/overview.go` unless noted. Field tags shown are the
intended JSON wire names (the envelope `result` payload). Pointer-to-number fields follow
the project's **nil-until-positive** convention: `nil` renders as a blank `-` placeholder,
never `0`.

## OverviewOpts (input)

Mirrors `StatusOpts` + `consumption`'s flag surface.

| Field | Type | Meaning |
|---|---|---|
| `Mode` | `FetchMode` | Reused from `consumption.go`. Default constructed as `{Kind: FetchTrailing, Days: 7}` when no temporal flag is set. |
| `fetcher` | `statusFetcher` | Test seam passed through to `Status()` (nil → real). |

Scoping is applied **before** `Overview` is called: `cmd/overview.go` runs
`fleet.ScopeToRepos(cfg, args)` and passes the restricted `cfg`. Unknown repo →
`ErrRepoNotTracked` (FR-017), surfaced by the command before any fetch.

## OverviewResult (top-level payload)

The envelope `result`. Honors the existing `Envelope` shape (`cmd.SchemaVersion = 1`).

| Field | Type | JSON | Meaning |
|---|---|---|---|
| `LoadedFrom` | `string` | `loaded_from` | Which config file(s) the fleet loaded from (mirrors `ConsumptionResult.LoadedFrom`). |
| `Window` | `string` | `window` | Human label for the active temporal window: `latest`, `trailing-7d`, `since-2026-06-01`. |
| `Repos` | `[]RepoOverview` | `repos` | One joined row per in-scope repo, sorted alphabetically by `Repo`. |
| `Total` | `OverviewTotal` | `total` | Pooled fleet aggregate (the `TOTAL` row). |

Diagnostics are NOT a field of `OverviewResult` — they flow through the envelope's
`warnings[]` / `hints[]` slots (as in `consumption`/`status`), via the `[]Diagnostic`
return value.

## RepoOverview (one joined row)

The core join: drift × health × cost+run-rate for one repo.

| Field | Type | JSON | Meaning |
|---|---|---|---|
| `Repo` | `string` | `repo` | `owner/name`. |
| `DriftState` | `string` | `drift_state` | `aligned` / `drifted` / `errored` (from `RepoStatus.DriftState`). |
| `DriftDetail` | `*RepoStatus` | `drift_detail,omitempty` | The full `RepoStatus` (Missing/Extra/Drifted/Unpinned/ErrorMessage) for the text detail block; omitted in JSON when `aligned` with nothing to show. |
| `Runs` | `int` | `runs` | Health-counting runs in the window = successes + failures (FR-008). Excludes cancelled/skipped/non-terminal. |
| `Failures` | `int` | `failures` | `failure`/`timed_out`/`startup_failure` count. |
| `NoOps` | `int` | `noops` | No-op count (aggregate `safeoutputs/noop` call_count, clamped to `[0, successes]`). |
| `HealthRate` | `*float64` | `health_rate,omitempty` | `successes ÷ Runs` in `[0,1]`. **Nil when `Runs == 0`** (undefined — renders the single `-` placeholder, distinct from `0%` and `100%`). |
| `AIC` | `*float64` | `aic,omitempty` | Summed AI credits over the window (nil-until-positive). |
| `Cost` | `*float64` | `cost,omitempty` | `AIC × aicToUSDRate` via `aicToUSD` (nil-until-positive). |
| `RunsAvailable` | `bool` | `runs_available` | False when the health/cost fan-out failed for this repo; RUNS/FAIL/NOOP/HEALTH/AIC/COST then render `-`. |
| `RunsError` | `string` | `runs_error,omitempty` | Fan-out error message when `RunsAvailable == false`. |

**Derived/invariants**:

- `Failures ≤ Runs`; `NoOps ≤ (Runs − Failures)` (≤ successes) by the clamp.
- `successes = Runs − Failures`; `HealthRate = successes / Runs` when `Runs > 0`, else nil.
- A repo with only failed runs → `Runs > 0`, `Failures == Runs`, `HealthRate = 0`, `AIC/Cost = nil` (FR-010).
- A repo with zero health-counting runs → `Runs = 0`, `HealthRate = nil`, `AIC/Cost = nil` (FR-011).
- Drift availability and run availability are **independent**: any of the four combinations is representable (FR-020).

## OverviewTotal (the TOTAL row)

Pooled aggregate across the in-scope rows.

| Field | Type | JSON | Meaning |
|---|---|---|---|
| `Runs` | `int` | `runs` | Σ `Runs`. |
| `Failures` | `int` | `failures` | Σ `Failures`. |
| `NoOps` | `int` | `noops` | Σ `NoOps`. |
| `HealthRate` | `*float64` | `health_rate,omitempty` | Pooled: `(Runs − Failures) / Runs` from pooled totals, **not** an average of per-repo rates (FR-012). Nil when pooled `Runs == 0`. |
| `AIC` | `*float64` | `aic,omitempty` | Σ `AIC`; nil when no repo had positive AIC (all-or-nothing nil-until-positive, as in `consumption`). |
| `Cost` | `*float64` | `cost,omitempty` | `aicToUSD(AIC)`. |
| `Aligned` | `int` | `aligned` | Count of `aligned` repos. |
| `Drifted` | `int` | `drifted` | Count of `drifted` repos. |
| `Errored` | `int` | `errored` | Count of `errored` repos. |

The exit-code disposition is `Drifted + Errored > 0 → exit 1`, computed in `cmd/overview.go`
from the pooled `OverviewResult.Total` counts (semantically identical to `statusExitCode`'s
per-repo `DriftState != DriftStateAligned` scan, since `Total.Aligned/Drifted/Errored` are the
tallies of those same per-repo states). `Overview()` populates these counts (see T014).

## Shared/extended types (in `consumption_logs.go`)

Additive change to the shared fan-out — `consumption` ignores the new field.

```go
// mcpToolUsage decodes the .mcp_tool_usage block of `gh aw logs --json`.
// Only the noop summary entry is consumed (run-rate signal); the rest is ignored.
type mcpToolUsage struct {
    Summary []mcpToolSummary `json:"summary"`
}
type mcpToolSummary struct {
    ServerName string `json:"server_name"` // e.g. "safeoutputs"
    ToolName   string `json:"tool_name"`   // e.g. "noop"
    CallCount  int    `json:"call_count"`
}

// logsPayload gains:
//   MCPToolUsage mcpToolUsage `json:"mcp_tool_usage"`
```

`noopCount(payload) int` returns the `call_count` for `{safeoutputs, noop}` (0 if absent).

The shared `collectRepoRuns` helper (factored out of `logSourceToReports`, FR-004) returns a
concrete per-workflow slice so both `consumption` and `overview` reduce the same shape:

```go
// repoRunData is one workflow's window-filtered runs plus its decoded tool-usage
// aggregate, as returned per workflow by collectRepoRuns.
type repoRunData struct {
    Workflow     string        // GitHub Actions display name (not the fleet slug)
    Runs         []logsRun     // window-filtered via filterRunsByWindow
    MCPToolUsage mcpToolUsage  // for noopCount; ignored by consumption
}

// collectRepoRuns(ctx, repo, mode, now) ([]repoRunData, []Diagnostic)
```

`overview`'s reducer consumes `[]repoRunData`; `logSourceToReports` consumes the same slice and
summarizes into `ConsumptionReport` exactly as before (behavior-preserving, guarded by T007).

## Reused types (no change)

- `FetchMode` / `FetchKind` (`consumption.go`) — temporal window; overview's default builder yields `{FetchTrailing, Days: 7}`.
- `RepoStatus` / `WorkflowDrift` / `DriftState*` consts (`status.go`) — embedded as `DriftDetail`.
- `Diagnostic` (`fleetdiag`) — per-repo + top-level diagnostics.
- `logsRun` / `filterRunsByWindow` / `aicToUSD` / `aicToUSDRate` (`consumption_logs.go`) — health/cost math.

## State transitions

None. `overview` is a stateless read: load config → scope → (drift ∥ health/cost) → join →
render. No persistence, no baseline, no caching (FR-026).
