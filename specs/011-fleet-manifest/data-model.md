# Data Model: Fleet Manifest — Deployed Version Tracking

**Feature**: `011-fleet-manifest`
**Date**: 2026-06-11

## New Types

### `FleetManifest` (`internal/fleet/manifest.go`)

Represents the on-disk deployment record written into each managed repo.

| Field | Go Type | JSON Key | Description |
|-------|---------|----------|-------------|
| `Managed` | `bool` | `managed` | Always `true`. Distinguishes fleet-written files from accidentals. |
| `Fleet` | `string` | `fleet` | Fleet repo slug, e.g. `"rshade/gh-aw-fleet"`. |
| `GhAwVersion` | `string` | `gh_aw_version` | `github/gh-aw` source pin from fleet.json (first resolved gh-aw workflow). Empty if no gh-aw-sourced workflows declared. |
| `CLIVersion` | `string` | `cli_version` | Output of `gh aw --version` at deploy time. Provenance only; not used for drift comparison. |
| `Profiles` | `[]string` | `profiles` | Sorted list of profile names active for this repo at deploy time. |
| `DeployedAt` | `time.Time` | `deployed_at` | RFC3339 UTC timestamp of last content-changing deploy. Not updated on same-version redeployes. |

**File path in managed repo**: `.github/aw/fleet-manifest.json`

**Example**:
```json
{
  "managed": true,
  "fleet": "rshade/gh-aw-fleet",
  "gh_aw_version": "v0.79.2",
  "cli_version": "v0.79.2",
  "profiles": ["default", "observability-plus"],
  "deployed_at": "2026-06-11T01:39:27Z"
}
```

**Validation rules**:
- `Managed` must be `true` for the manifest to be recognized as fleet-owned.
- Malformed JSON or `managed: false` → treated as `unmanaged` by drift detection.
- `GhAwVersion` empty string is a valid value (repo has no gh-aw-sourced workflows).

### `VersionDrift` (`internal/fleet/status.go`)

Computed per-repo result for the version-drift dimension in `status`.

| Field | Go Type | JSON Key | Description |
|-------|---------|----------|-------------|
| `State` | `string` | `state` | One of `"behind"`, `"current"`, `"unmanaged"`. |
| `RecordedVersion` | `string` | `recorded_version` | `GhAwVersion` from the manifest. Empty when `State == "unmanaged"`. |
| `ExpectedVersion` | `string` | `expected_version` | Fleet's current `github/gh-aw` pin for this repo. Empty when no gh-aw-sourced workflows. |

**State transitions**:

```
manifest == nil OR malformed → unmanaged
manifest.GhAwVersion == expectedVersion → current
manifest.GhAwVersion != expectedVersion → behind
```

Note: There is no "ahead" state. Fleet pins only advance; a manifest version ahead of the fleet
pin indicates a misconfiguration (manual manifest edit or stale fleet.json). This is treated as
`behind` (not equal → refresh) because the correct response is the same: re-run init on next
deploy.

## Modified Types

### `RepoStatus` (`internal/fleet/status.go`) — additive field

```go
// VersionDrift reports the manifest-based version-drift state for this repo.
// Nil only when DriftState == DriftStateErrored and the manifest fetch was
// not attempted (early error path). Non-nil for all successfully-queried repos,
// including those where the manifest is absent (State == VersionDriftUnmanaged).
VersionDrift *VersionDrift `json:"version_drift,omitempty"`
```

**No other changes to `RepoStatus`.**

### `statusJob` (`internal/fleet/status.go`) — internal only, additive field

```go
type statusJob struct {
    repo            string
    declared        []ResolvedWorkflow
    resolveErr      error
    expectedVersion string // NEW: github/gh-aw pin for this repo from fleet config
}
```

## New Constants

In `internal/fleet/manifest.go`:

```go
// FleetManifestPath is the repo-relative path of the fleet manifest file
// written into each managed repository.
const FleetManifestPath = ".github/aw/fleet-manifest.json"
```

In `internal/fleet/status.go`:

```go
// Version-drift state constants emitted on VersionDrift.State.
const (
    VersionDriftBehind    = "behind"
    VersionDriftCurrent   = "current"
    VersionDriftUnmanaged = "unmanaged"
)
```

## New Functions

In `internal/fleet/manifest.go`:

| Function | Signature | Description |
|----------|-----------|-------------|
| `resolvedGhAwPin` | `(cfg *Config, repo string) string` | Returns first gh-aw source pin for repo. |
| `buildManifest` | `(cfg *Config, repo, cliVersion string) *FleetManifest` | Constructs a manifest from fleet config. |
| `readManifestFromClone` | `(dir string) (*FleetManifest, error)` | Reads and parses `.github/aw/fleet-manifest.json` from a local clone. Returns `(nil, nil)` when absent. |
| `writeManifestToClone` | `(dir string, m *FleetManifest) error` | Marshals and writes the manifest; creates parent dirs. |
| `writeManifestIfNeeded` | `(dir string, m *FleetManifest) (bool, error)` | Writes only when content differs (excluding timestamp). Returns `(changed, err)`. |
| `manifestEqualExceptTime` | `(a, b *FleetManifest) bool` | Pure comparison ignoring `DeployedAt`. |
| `parseManifestJSON` | `(body string) (*FleetManifest, error)` | Parses raw JSON; returns `(nil, nil)` for empty input. |
| `computeVersionDrift` | `(m *FleetManifest, expectedVersion string) *VersionDrift` | Pure function; computes drift state from manifest + expected version. |

## Modified Functions

| Function | File | Change |
|----------|------|--------|
| `ensureInit` | `deploy.go` | Signature: add `fleetGhAwVersion string` param. Body: replace `initMarkerPaths` check with manifest version comparison. |
| `Deploy` | `deploy.go` | Apply path: add `cliVer, _ := ghAwVersion(ctx)` + `buildManifest` + `writeManifestIfNeeded` before `hasStagedOrUnstagedWorkflowChanges`. |
| `Sync` | `sync.go` | Update `ensureInit` call site to pass `resolvedGhAwPin(cfg, repo)`. |
| `applyDeployOrPrune` | `sync.go` | Add manifest write before `commitAndPushPrune` for prune-only paths. |
| `buildStatusJobs` | `status.go` | Populate `statusJob.expectedVersion = resolvedGhAwPin(cfg, repo)`. |
| `processRepo` | `status.go` | After `computeDrift`, fetch manifest and call `computeVersionDrift`; set `rs.VersionDrift`. |
| `statusFetcher` (interface) | `status.go` | Add `fetchManifestBody(ctx context.Context, repo string) (string, error)`. |
| `ghStatusFetcher` | `status.go` | Implement `fetchManifestBody`. |

## JSON Envelope Contract

### `status --output json` (`result.repos[]`)

**Before**:
```json
{
  "repo": "owner/name",
  "drift_state": "aligned",
  "missing": [],
  "extra": [],
  "drifted": [],
  "unpinned": [],
  "error_message": ""
}
```

**After** (additive — no version bump):
```json
{
  "repo": "owner/name",
  "drift_state": "aligned",
  "missing": [],
  "extra": [],
  "drifted": [],
  "unpinned": [],
  "error_message": "",
  "version_drift": {
    "state": "current",
    "recorded_version": "v0.79.2",
    "expected_version": "v0.79.2"
  }
}
```

For unmanaged repos:
```json
"version_drift": {
  "state": "unmanaged",
  "recorded_version": "",
  "expected_version": "v0.79.2"
}
```

For errored repos (`drift_state == "errored"`): `version_drift` is omitted (null with omitempty).

### `deploy --output json`

No changes. The manifest write is a side effect; it does not add a field to `DeployResult`.
`init_was_run` continues to report whether `gh aw init` executed.
