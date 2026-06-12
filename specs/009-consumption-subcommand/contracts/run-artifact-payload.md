# Contract: Run-artifact payload parsing

**Feature**: 009-consumption-subcommand
**Plan**: [../plan.md](../plan.md) — Decision 2 (REST artifact fetch) and Decision 6 (cost zero-vs-absent) in [../research.md](../research.md).
**Producer**: `fetchRunArtifacts(ctx, ref reportRef) (*ConsumptionReport, error)` in `internal/fleet/consumption.go`.

## Fetch sequence

Two `gh api` calls per `reportRef`, both via the injection seam.

### Step 1 — list artifacts

```bash
gh api "repos/{owner}/{name}/actions/runs/{run_id}/artifacts"
```

Response (subset consumed):

```json
{
  "total_count": 2,
  "artifacts": [
    {
      "id": 1234567890,
      "name": "activation",
      "archive_download_url": "https://api.github.com/repos/owner/name/actions/artifacts/1234567890/zip"
    },
    {
      "id": 1234567891,
      "name": "run_summary",
      "archive_download_url": "..."
    }
  ]
}
```

**Artifact names (verified 2026-06-09 against rshade/finfocus run 27241899611):** the artifact carrying `aw_info.json` is named **`activation`** (gh-aw v5+, multi-file: `aw_info.json` + `prompt.txt` + `github_rate_limits.jsonl`) or **`aw-info`** (legacy single-file, note the dash) — **never `aw_info`** (underscore). `selectArtifactIDs` matches the precedence list `["activation", "aw-info", "aw_info"]` (earliest wins), with `aw_info` retained only as a defensive fallback. `run_summary` (or `run-summary`) is rarely present — `run_summary.json` is a local `gh aw audit` cache, not normally uploaded; when absent we proceed with an empty per-workflow breakdown.

### Step 2 — download and unzip

For each selected artifact:

```bash
gh api "repos/{owner}/{name}/actions/artifacts/{artifact_id}/zip"
```

> Do **not** send `-H "Accept: application/octet-stream"` — GitHub now rejects it on the `/zip` endpoint with **HTTP 415** ("Must accept 'application/json'"). The endpoint replies with a 302 to blob storage and the default `gh api` Accept header follows it correctly.

The response body is a zip archive (bytes). Unzip in-memory:

```go
zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
// Files sit at the archive root (the "activation" zip carries aw_info.json
// directly). readZipFile matches on basename suffix, so any future nesting
// (e.g. aw-info/aw_info.json) still resolves.
```

If the archive cannot be read (truncated, not a zip) → wrap as a generic error and propagate; the caller emits `DiagHint` with the wrapped message.

## Payload shape consumed

### `aw_info.json` (subset)

```json
{
  "github_rate_limit_usage": {
    "core_consumed": 4827,
    "core_limit": 5000,
    "core_remaining": 173
  },
  "safe_outputs": {
    "total_calls": 31
  },
  "cost": 12.45
}
```

Field map onto `ConsumptionReport`:

| Source JSON path | Target Go field | Notes |
|---|---|---|
| `github_rate_limit_usage.core_consumed` | `ConsumptionReport.GitHubAPICalls` | `int` |
| `safe_outputs.total_calls` | `ConsumptionReport.SafeOutputCalls` | `int`; absent → 0 |
| `cost` | `ConsumptionReport.Cost` | `*float64`; absent OR ≤ 0 → `nil` (Decision 6) |

Decode struct:

```go
type awInfoPayload struct {
    GithubRateLimitUsage struct {
        CoreConsumed int `json:"core_consumed"`
    } `json:"github_rate_limit_usage"`
    SafeOutputs struct {
        TotalCalls int `json:"total_calls"`
    } `json:"safe_outputs"`
    // Cost as *float64 so we can distinguish absent (nil) from present-and-zero.
    Cost *float64 `json:"cost,omitempty"`
}
```

> **⚠ Known gap (discovered 2026-06-09, pending verification against a live `api-consumption-report` run).** A *regular* agentic run's `aw_info.json` (verified: finfocus run 27241899611) is engine/run metadata only — `engine_id`, `model`, `workflow_name`, `run_id`, `cli_version` — and carries **none** of `github_rate_limit_usage`, `safe_outputs`, or `cost`. Rate-limit data lives in a sibling `github_rate_limits.jsonl` (per-snapshot `used` counter); safe-outputs in `safe-output-items.jsonl`; the fleet-wide aggregate in the report's published **discussion body**. It is not yet confirmed whether the `api-consumption-report` run *itself* uploads an `aw_info.json`/`run_summary.json` carrying these fields. If it does not, the artifact path yields zeros and the data source should move to the discussion-body table. Resolve once a real report run exists.

Cost-field post-processing:

```go
func normalizeCost(raw *float64) *float64 {
    if raw == nil || *raw <= 0 {
        return nil
    }
    return raw
}
```

Applied immediately after JSON decode so the report's `Cost` always honors the nil-until-positive invariant.

### `run_summary.json` (subset)

```json
{
  "workflows": [
    {
      "name": "issue-triage",
      "runs": 156,
      "api_calls": 3210,
      "avg_duration_seconds": 42.7,
      "cost": 8.91
    },
    {
      "name": "ci-doctor",
      "runs": 84,
      "api_calls": 2104,
      "avg_duration_seconds": 31.2
    }
  ]
}
```

Field map onto `WorkflowConsumption`:

| Source JSON path | Target Go field | Notes |
|---|---|---|
| `workflows[].name` | `WorkflowConsumption.Workflow` | `string` |
| `workflows[].runs` | `WorkflowConsumption.Runs` | `int` |
| `workflows[].api_calls` | `WorkflowConsumption.APICalls` | `int` |
| `workflows[].avg_duration_seconds` | `WorkflowConsumption.AvgDurationS` | `float64` |
| `workflows[].cost` | `WorkflowConsumption.Cost` | `*float64`, post-processed via `normalizeCost` |

Decode struct:

```go
type runSummaryPayload struct {
    Workflows []struct {
        Name               string   `json:"name"`
        Runs               int      `json:"runs"`
        APICalls           int      `json:"api_calls"`
        AvgDurationSeconds float64  `json:"avg_duration_seconds"`
        Cost               *float64 `json:"cost,omitempty"`
    } `json:"workflows"`
}
```

## Assembly into `ConsumptionReport`

```go
report := &ConsumptionReport{
    Repo:            ref.Repo,
    Date:            ref.Date,
    RunID:           ref.RunID,
    GitHubAPICalls:  awInfo.GithubRateLimitUsage.CoreConsumed,
    SafeOutputCalls: awInfo.SafeOutputs.TotalCalls,
    Cost:            normalizeCost(awInfo.Cost),
    PerWorkflow:     toWorkflowConsumption(runSummary.Workflows),
    // Profile and CostCenter intentionally left empty — joined by AggregateConsumption.
}
```

## Error handling

| Condition | Behavior |
|---|---|
| Artifact list returns HTTP 404 | Caller treats run as garbage-collected per FR-009. Returns `nil, nil` + a `DiagHTTP404` warning naming the repo / run / date. |
| Artifact zip download returns HTTP 404 (race: listed but vanished) | Same as above. |
| `aw_info` artifact missing from the list | Returns `nil, nil` + a `DiagHint` warning. Soft failure — the discussion exists but the data is unfetchable. |
| `run_summary` artifact missing from the list | Returns a `ConsumptionReport` with `PerWorkflow = []`. No warning — some older runs may not have produced this artifact. |
| Zip parse failure (truncated, not a zip) | Returns `nil, err` wrapping the parse error. Caller emits `DiagHint` and skips the report. |
| JSON decode failure on either payload | Returns `nil, err`. Caller emits `DiagHint` `"Run #{id} on {repo} has malformed {file} JSON — skipping; possibly upstream format drift"`. |
| `cost` field is the literal zero | Stored as `nil` in `ConsumptionReport.Cost` (Decision 6). No warning. |
| `cost` field is the literal negative | Stored as `nil`. No warning. |

## Mockability for tests

Same package-level seam pattern as the discovery layer:

```go
//nolint:gochecknoglobals // test-injection seam for gh api run-artifact fetch
var ghRunArtifactAPI = func(ctx context.Context, repo string, runID int64) (artifactPayload, error) {
    // Production impl: two exec.CommandContext calls (list + zip), in-memory unzip,
    // JSON decode of the two files. Returns the structs already decoded.
}

type artifactPayload struct {
    AWInfo     awInfoPayload
    RunSummary runSummaryPayload
}
```

Tests substitute a closure that reads fixture JSON files directly, bypassing the zip step entirely:

```go
ghRunArtifactAPI = func(ctx context.Context, repo string, runID int64) (artifactPayload, error) {
    return artifactPayload{
        AWInfo:     mustReadAWInfo(t, "consumption/aw_info_cost_present.json"),
        RunSummary: mustReadRunSummary(t, "consumption/run_summary.json"),
    }, nil
}
```

Fixtures under `internal/fleet/testdata/consumption/`:

- `aw_info_cost_present.json` — full payload with `cost: 12.45`
- `aw_info_cost_absent.json` — same payload without the `cost` key
- `aw_info_cost_zero.json` — `cost: 0` (exercises Decision 6's normalization)
- `aw_info_cost_negative.json` — `cost: -1.5` (defensive)
- `run_summary.json` — three workflows, two with cost populated, one without
- `run_summary_empty.json` — `{ "workflows": [] }` (exercises the empty per-workflow case)
