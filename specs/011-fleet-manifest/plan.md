# Implementation Plan: Fleet Manifest — Deployed Version Tracking

**Branch**: `011-fleet-manifest` | **Date**: 2026-06-11 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/011-fleet-manifest/spec.md`

## Summary

Add a fleet-owned manifest file (`.github/aw/fleet-manifest.json`) written into each managed repo on `deploy --apply` / `sync --apply`, recording the `github/gh-aw` source pin, CLI version, profiles, and deployment timestamp. Extend `status` to read the manifest via the GitHub API and report per-repo version-drift state (`behind` / `current` / `unmanaged`) in both text and JSON output. Replace the crude `ensureInit` marker-file skip with a manifest-version comparison so stale init artifacts are refreshed automatically when the fleet's `github/gh-aw` pin advances.

## Technical Context

**Language/Version**: Go 1.25.8 (from `go.mod`)
**Primary Dependencies**: stdlib (`encoding/json`, `os`, `path/filepath`, `time`). **No new direct dependencies** — constitution §Third-Party Dependencies.
**Storage**: JSON files written into target-repo clones (`os.WriteFile`); read in `status` via `ghAPIRaw` (existing seam).
**Testing**: `go test ./...` (existing suite) + new offline unit tests for manifest read/write and version-drift computation, using the existing seam-injection patterns.
**Target Platform**: Linux/macOS CLI tool.
**Project Type**: CLI tool (thin orchestrator).
**Performance Goals**: Status fan-out adds one API call per repo (manifest read) — absorbed by the existing `statusWorkerPoolSize = 6` pool. No clone required for status drift checks.
**Constraints**: No new third-party dependencies; no new commands; no `cmd.SchemaVersion` or `fleet.SchemaVersion` bumps (additive fields only).
**Scale/Scope**: ≤10 repos, ≤20 workflows each (constitution §Performance ceiling).

## FR-003 Resolution

The spec left open how to record the `gh_aw_version` when a repo has multiple profiles with potentially different `github/gh-aw` source pins. **Decision**: record a single `gh_aw_version` string = the `Ref` of the first resolved workflow whose `Source == "github/gh-aw"`. In the common case (all profiles share the same `github/gh-aw` pin), this is unambiguous. For the rare case where profiles disagree, the manifest reflects the version of the first gh-aw-sourced workflow; the operator is expected to converge profiles to a single pin (per AGENTS.md: always pin `gh-aw` to a tagged release). Documented in the `FleetManifest` godoc.

**Rationale**: A per-source map (`{"github/gh-aw": "v0.79.2"}`) was the alternative. It is structurally cleaner but adds JSON complexity and makes the drift comparison more complex. Since fleet operators are expected to keep all profiles at the same `github/gh-aw` pin (it's a compiler, not a workflow), a single string is sufficient and keeps the manifest human-readable.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Thin Orchestrator | ✅ | Manifest write: `os.WriteFile` into clone (stdlib). Init refresh: delegates to `exec.Command("gh", "aw", "init")` — no re-implementation. Status read: via `ghAPIRaw` (existing seam). |
| II. Testing Standards | ✅ | New pure functions (`parseManifest`, `computeVersionDrift`, `buildManifest`) are unit-testable without subprocess. Manifest write in deploy clone covered by the existing dry-run integration contract (no `--apply` in tests). |
| III. Three-Turn Mutation Pattern | ✅ | Manifest write is inside the existing `--apply` gate. No new mutation surface added. No new commands. |
| IV. Performance | ✅ | Status: one additional API call per repo, absorbed by existing worker pool. Deploy: `os.ReadFile` + `os.WriteFile` into a local clone — microseconds. |
| No new deps | ✅ | `encoding/json`, `os`, `path/filepath`, `time` are all stdlib. |

## Project Structure

### Documentation (this feature)

```text
specs/011-fleet-manifest/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
└── checklists/
    └── requirements.md  # Spec quality checklist
```

### Source Code Changes

```text
internal/fleet/
├── manifest.go          # NEW — FleetManifest type + read/write + version-drift helpers
├── manifest_test.go     # NEW — offline unit tests for manifest logic
├── deploy.go            # MODIFY — writeManifestIfNeeded before createDeployPR; version-aware ensureInit
├── sync.go              # MODIFY — version-aware ensureInit (same change as deploy.go)
├── status.go            # MODIFY — VersionDrift field on RepoStatus; manifest fetch in processRepo
└── result_json.go       # MODIFY — serialize VersionDrift in deployResultJSON (additive)

internal/fleet/testdata/
└── manifest/            # NEW — fixture manifest JSONs for offline status tests
```

**Structure Decision**: single-package additions. All new code lives in `internal/fleet/`; no new packages, no new CLI commands. The `cmd/` layer needs no changes — `status`, `deploy`, and `sync` already surface whatever their result structs carry.

## Complexity Tracking

> No Constitution violations.

## Phase 0: Research

Research output is captured in [research.md](./research.md). Summary of key resolved unknowns:

### R1 — `gh_aw_version` source for multi-profile repos

**Decision**: First resolved `ResolvedWorkflow` where `Source == "github/gh-aw"` provides the `Ref`. `cfg.ResolveRepoWorkflows(repo)` returns `[]ResolvedWorkflow`; `ResolvedWorkflow.Source` is the source slug constant (`sourceGitHubAW = "github/gh-aw"`).

**Helper**:
```go
// resolvedGhAwPin returns the github/gh-aw source pin for repo, or "" when no
// gh-aw-sourced workflows are declared.
func resolvedGhAwPin(cfg *Config, repo string) string {
    workflows, _ := cfg.ResolveRepoWorkflows(repo)
    for _, w := range workflows {
        if w.Source == sourceGitHubAW {
            return w.Ref
        }
    }
    return ""
}
```

**Alternatives considered**: per-source map (more precise, more complex); minimum/maximum ref across profiles (fragile — string comparison on semver is undefined). Single-pin wins on simplicity.

### R2 — Reading the manifest in `status` (no clone)

**Decision**: Extend `statusFetcher` interface with `fetchManifestBody(ctx, repo) (string, error)`. Production binding uses `ghAPIRaw("/repos/{repo}/contents/.github/aw/fleet-manifest.json")`. On GitHub 404 → return `("", nil)` (unmanaged). On other errors → return `("", err)` (treated as unmanaged with a debug log, not an error that aborts the repo status).

**Rationale**: `ghAPIRaw` already handles file-content fetching (used by `fetchWorkflowBody`). The existing test seam pattern (`statusFetcher` interface + fake in tests) already supports extension. Adding one method is far cheaper than adding a clone step.

### R3 — Timestamp churn prevention

**Decision**: Before writing the manifest, read the existing manifest from the clone (`readManifestFromClone`). If one exists and all fields except `DeployedAt` are byte-equal to what we would write, skip the write entirely. This means a same-version redeploy produces zero git diff.

**Implementation**:
```go
// writeManifestIfNeeded writes m to dir/.github/aw/fleet-manifest.json only when
// the content would differ from any existing manifest (ignoring DeployedAt).
// Returns (changed, error).
func writeManifestIfNeeded(dir string, m *FleetManifest) (bool, error) {
    existing, err := readManifestFromClone(dir)
    if err == nil && existing != nil && manifestEqualExceptTime(existing, m) {
        return false, nil
    }
    return true, writeManifestToClone(dir, m)
}
```

### R4 — `ensureInit` replacement logic

**Decision**: Replace the `initMarkerPaths` file-check with a manifest version comparison. The new `ensureInit` reads the manifest from the clone, compares `manifest.GhAwVersion` to `fleetGhAwVersion` (the fleet's resolved `github/gh-aw` pin for this repo). If they match → skip. Otherwise (no manifest, unreadable manifest, or version mismatch) → run `gh aw init`.

**Key change**: `ensureInit` signature gains a `fleetGhAwVersion string` parameter. Callers (`Deploy` and `Sync`) pass `resolvedGhAwPin(cfg, repo)`.

```go
func ensureInit(ctx context.Context, dir, fleetGhAwVersion string) (bool, error) {
    if fleetGhAwVersion != "" {
        m, _ := readManifestFromClone(dir)
        if m != nil && m.GhAwVersion == fleetGhAwVersion {
            return false, nil
        }
    }
    // Run gh aw init (covers: no manifest, stale version, or fleetGhAwVersion unknown)
    ...
}
```

**Risk note**: The original `ensureInit` avoided re-running init on repos initialized by older CLIs because newer CLIs could fail on the old layout. The manifest check changes the behavior: if a repo has a stale manifest (or no manifest), we WILL re-run init. This is the intended behavior for v0.79.2+ where init is idempotent on previously-initialized repos. If a pre-v0.79 layout causes `gh aw init` to fail, the error surfaces in the existing error path and the clone is preserved for manual inspection.

### R5 — Manifest file location in the commit

`branchAndStageGithub` calls `git add .github/` which stages all of `.github/` recursively. Writing `.github/aw/fleet-manifest.json` to the clone before `branchAndStageGithub` is called means the manifest is staged and committed automatically — no changes needed to the staging logic.

**Placement in Deploy**:
```go
// After Apply-gate; before createDeployPR
if opts.Apply {
    ...
    m := buildManifest(cfg, repo, cliVersion)
    if _, writeErr := writeManifestIfNeeded(res.CloneDir, m); writeErr != nil {
        return res, writeErr
    }
    staged, err := hasStagedOrUnstagedWorkflowChanges(ctx, res.CloneDir)
    // staged is now true if the manifest was written
    ...
}
```

### R6 — CLI version acquisition

`ghAwVersion` is already a package-level injectable seam:
```go
var ghAwVersion = func(ctx context.Context) (string, error) { ... }
```
Call once per deploy and pass the result to `buildManifest`. If the call fails, record `""` with a debug log — non-fatal per the fail-open pattern used elsewhere.

## Phase 1: Design & Contracts

### Data Model — `data-model.md`

See [data-model.md](./data-model.md) for full entity definitions. Summary:

#### `FleetManifest` (new type in `manifest.go`)

```go
// FleetManifest is the fleet-owned deployment record written into each managed
// repo at .github/aw/fleet-manifest.json. It records the provenance of the
// most recent fleet-driven deploy so that status, sync, and deploy can detect
// and remediate version drift without cloning.
type FleetManifest struct {
    // Managed is always true; its presence distinguishes fleet-written manifests
    // from any accidental file at the same path.
    Managed bool `json:"managed"`
    // Fleet is the fleet repo slug (e.g. "rshade/gh-aw-fleet").
    Fleet string `json:"fleet"`
    // GhAwVersion is the github/gh-aw SOURCE pin from fleet.json used for this
    // deploy. It is the single version from the first resolved github/gh-aw
    // workflow (see R1 in research.md). Empty when the repo has no gh-aw-sourced
    // workflows declared.
    GhAwVersion string `json:"gh_aw_version"`
    // CLIVersion is the output of `gh aw --version` at deploy time — the runtime
    // artifact provenance, distinct from the source pin. Recorded for diagnostics;
    // not used for drift comparison.
    CLIVersion string `json:"cli_version"`
    // Profiles is the list of profile names active for this repo at deploy time.
    Profiles []string `json:"profiles"`
    // DeployedAt is the RFC3339 UTC timestamp of the deploy that last changed
    // manifest content. It is NOT updated on same-version redeployes (see R3).
    DeployedAt time.Time `json:"deployed_at"`
}
```

**Manifest file path**: `.github/aw/fleet-manifest.json` in the target repo.

#### `VersionDrift` (new type in `status.go`)

```go
// VersionDrift describes the version-drift state of a managed repo's init
// artifacts relative to the fleet's current github/gh-aw pin.
type VersionDrift struct {
    // State is one of VersionDriftBehind, VersionDriftCurrent, VersionDriftUnmanaged.
    State string `json:"state"`
    // RecordedVersion is the GhAwVersion from the repo's manifest. Empty when
    // State == VersionDriftUnmanaged.
    RecordedVersion string `json:"recorded_version"`
    // ExpectedVersion is the fleet's current github/gh-aw pin for this repo.
    // Empty when the repo has no gh-aw-sourced workflows.
    ExpectedVersion string `json:"expected_version"`
}
```

**Version-drift state constants**:
```go
const (
    VersionDriftBehind    = "behind"
    VersionDriftCurrent   = "current"
    VersionDriftUnmanaged = "unmanaged"
)
```

`RepoStatus` gains a new field:
```go
// VersionDrift reports the manifest-based version-drift state. Nil only when
// status encountered an error reading the manifest AND the repo itself errored.
VersionDrift *VersionDrift `json:"version_drift,omitempty"`
```

### Interface Contracts — `contracts/`

See `contracts/` for the full JSON envelope contract delta. Summary:

#### `status --output json` delta (additive, no version bump)

Each object in `result.repos[]` gains:
```json
"version_drift": {
  "state": "behind | current | unmanaged",
  "recorded_version": "v0.79.2 | \"\"",
  "expected_version": "v0.79.2 | \"\""
}
```

`version_drift` is omitted (not `null`) only when the repo itself errored before the manifest fetch could run.

#### `deploy --output json` delta (no change)

No new fields on `DeployResult`. The manifest write is a side effect (file in the clone); it does not add a field to the JSON envelope. `init_was_run` continues to reflect whether `gh aw init` ran.

### Key Implementation Steps

Below is the ordered sequence of implementation tasks. Each task is independently testable and mergeable.

#### Task 1 — `internal/fleet/manifest.go` (new file)

Implement:
- `FleetManifest` type + godoc
- `FleetManifestPath = ".github/aw/fleet-manifest.json"` constant
- `readManifestFromClone(dir string) (*FleetManifest, error)` — reads and parses the manifest from a local clone. Returns `(nil, nil)` when the file doesn't exist.
- `writeManifestToClone(dir string, m *FleetManifest) error` — marshals and writes with `os.MkdirAll` + `os.WriteFile`.
- `buildManifest(cfg *Config, repo, cliVersion string) *FleetManifest` — constructs the manifest using `resolvedGhAwPin(cfg, repo)` and the repo's profile list.
- `writeManifestIfNeeded(dir string, m *FleetManifest) (bool, error)` — reads existing, compares excluding `DeployedAt`, writes only on change.
- `manifestEqualExceptTime(a, b *FleetManifest) bool` — pure comparison helper.
- `resolvedGhAwPin(cfg *Config, repo string) string` — see R1.
- `parseManifestJSON(body string) (*FleetManifest, error)` — used by `status` path (body is raw JSON string from `ghAPIRaw`).
- `computeVersionDrift(manifest *FleetManifest, expectedVersion string) *VersionDrift` — pure function; nil manifest → `unmanaged`; match → `current`; mismatch → `behind`.

Test coverage in `manifest_test.go`:
- `buildManifest` produces correct fields from a test `Config`
- `manifestEqualExceptTime` correctly ignores timestamp
- `writeManifestIfNeeded` no-ops when nothing changed
- `parseManifestJSON` handles valid, malformed, and empty input
- `computeVersionDrift` covers all three states

#### Task 2 — `deploy.go`: version-aware `ensureInit` + manifest write

**`ensureInit` signature change**:
```go
// Before: func ensureInit(ctx context.Context, dir string) (bool, error)
// After:
func ensureInit(ctx context.Context, dir, fleetGhAwVersion string) (bool, error)
```

Change the body to use `readManifestFromClone` + version comparison (see R4). Remove `initMarkerPaths` var and file-check loop (this is the principled supersession of the legacy skip — document in the commit message).

**Manifest write in `Deploy`** (apply path only, before `createDeployPR`):
```go
cliVer, _ := ghAwVersion(ctx)
m := buildManifest(cfg, repo, cliVer)
if _, err := writeManifestIfNeeded(res.CloneDir, m); err != nil {
    return res, fmt.Errorf("write fleet manifest: %w", err)
}
```

This happens after `runCompileStrictIfNeeded` and before `hasStagedOrUnstagedWorkflowChanges` so the manifest is included in the staged change detection.

**Call-site change in `Deploy`**:
```go
fleetPin := resolvedGhAwPin(cfg, repo)
res.InitWasRun, err = ensureInit(ctx, res.CloneDir, fleetPin)
```

#### Task 3 — `sync.go`: version-aware `ensureInit`

Same `ensureInit` call-site change as deploy.go:
```go
fleetPin := resolvedGhAwPin(cfg, repo)
if _, initErr := ensureInit(ctx, res.CloneDir, fleetPin); initErr != nil { ... }
```

The manifest write does not need to be added here separately — when `Sync` calls `Deploy` (in `applyDeployOrPrune`), the manifest write in `Deploy` covers the sync path. For prune-only paths (`commitAndPushPrune`), a separate manifest write may be needed — see note below.

**Prune-only manifest write note**: `commitAndPushPrune` runs when there are no missing workflows but there are drift files to prune. In this path, `Deploy` is not called. If we want the manifest updated on prune-only deploys, we'd add a manifest write before `commitAndPushPrune`. Given the spec's AC-1 says "committed with the deploy" and prune is part of `sync`, this should be added. Implementation: write manifest before `commitAndPushPrune`, use same `writeManifestIfNeeded` pattern.

#### Task 4 — `status.go`: version-drift reporting

**`statusFetcher` interface extension**:
```go
type statusFetcher interface {
    listWorkflowsDir(ctx context.Context, repo string) ([]string, error)
    fetchWorkflowBody(ctx context.Context, repo, file string) (string, error)
    fetchManifestBody(ctx context.Context, repo string) (string, error) // NEW
}
```

**`ghStatusFetcher.fetchManifestBody` production binding**:
```go
func (ghStatusFetcher) fetchManifestBody(ctx context.Context, repo string) (string, error) {
    path := fmt.Sprintf("/repos/%s/contents/.github/aw/fleet-manifest.json", repo)
    body, err := ghAPIRaw(ctx, path)
    if err != nil {
        if isGitHubNotFound(err) {
            return "", nil // unmanaged — not an error
        }
        zlog.Debug().Str(fieldRepo, repo).Err(err).Msg("manifest fetch skipped")
        return "", nil // other errors → treat as unmanaged, don't fail status
    }
    return body, nil
}
```

**`RepoStatus` field addition**:
```go
// VersionDrift is the manifest-based version-drift state. Nil only when
// DriftState == DriftStateErrored and the manifest fetch was not attempted.
VersionDrift *VersionDrift `json:"version_drift,omitempty"`
```

**`processRepo` change** — after fetching workflow bodies, fetch and parse manifest:
```go
func processRepo(ctx context.Context, fetcher statusFetcher, job statusJob) RepoStatus {
    // existing workflow fetch logic...
    rs := computeDrift(...)

    // manifest version drift
    manifestBody, _ := fetcher.fetchManifestBody(ctx, job.repo)
    manifest, _ := parseManifestJSON(manifestBody)
    rs.VersionDrift = computeVersionDrift(manifest, job.expectedVersion)
    return rs
}
```

`job.expectedVersion` is populated in `buildStatusJobs`:
```go
type statusJob struct {
    repo            string
    declared        []ResolvedWorkflow
    resolveErr      error
    expectedVersion string // gh-aw source pin from fleet config
}
```

And in `buildStatusJobs`:
```go
jobs = append(jobs, statusJob{
    repo:            repo,
    declared:        declared,
    resolveErr:      err,
    expectedVersion: resolvedGhAwPin(cfg, repo),
})
```

#### Task 5 — Tests

New file `manifest_test.go`:
- Pure unit tests (no subprocess, no network)
- Test `buildManifest`, `manifestEqualExceptTime`, `writeManifestIfNeeded`, `parseManifestJSON`, `computeVersionDrift`
- Use a temp dir for write tests (`t.TempDir()`)

Extension of `status_test.go`:
- Extend the existing fake `statusFetcher` to implement `fetchManifestBody`
- Add test cases for `behind`, `current`, `unmanaged` drift states
- Add `testdata/manifest/` fixture files

Extension of `deploy_test.go`:
- Add test cases for manifest write in deploy clone
- Add test case for `ensureInit` skipping when manifest version matches
- Add test case for `ensureInit` running when manifest is absent
- Add test case for `ensureInit` running when manifest version is behind

#### Task 6 — `result_json.go`: additive JSON marshaling

Add `VersionDrift` to `deployResultJSON` shape (to expose it on the deploy result for informational purposes):

Actually, re-reading the spec, `deploy` JSON output doesn't need a `VersionDrift` field — that's a `status` concept. The deploy result does need `init_was_run` which already exists. No changes needed to `result_json.go`.

For `StatusResult` / `RepoStatus` — these already marshal via standard `json.Marshal` (no custom `MarshalJSON`), so adding `VersionDrift *VersionDrift json:"version_drift,omitempty"` to `RepoStatus` is sufficient.

#### Task 7 — `CLAUDE.md` / `AGENTS.md` update

Per constitution §Development Workflow: "CLAUDE.md MUST be updated when a new architectural invariant is established." The manifest introduces two new invariants:
1. `.github/aw/fleet-manifest.json` is the fleet's footprint marker in managed repos
2. `ensureInit` is now version-aware; the `initMarkerPaths` skip is gone

Add a short paragraph to the `deploy.go` section of AGENTS.md under "Deploy absorbs gh-aw CLI quirks" describing the new manifest-based init behavior.

#### Task 8 — Skills update

Per constitution §Development Workflow: "The four skills in `skills/` MUST be updated when a command they reference gains or loses a flag or when a new failure class surfaces." The `fleet-deploy` and `fleet-onboard-repo` skills reference the `ensureInit` behavior. Update them to reflect that init refresh is now version-gated via the manifest.

### Quickstart

See [quickstart.md](./quickstart.md) for the operator-facing change summary. Key points:

**Manifest location**: `.github/aw/fleet-manifest.json` in each managed repo's `.github/aw/` directory (alongside `actions-lock.json`).

**Status new output column**: `gh-aw-fleet status` (text output) gains a `VERSION_DRIFT` column alongside the existing drift state. `--output json` result `repos[].version_drift` is the machine-readable form.

**No operator action required**: The manifest is created on the next `deploy --apply` or `sync --apply` against each repo. Repos without a manifest report as `unmanaged` in `status` until their first post-feature deploy.

**Init refresh behavior**: After this change, `deploy --apply` / `sync --apply` will re-run `gh aw init` on repos whose manifest is absent or stale, producing updated init artifacts in the PR. This is the intended behavior — operators should expect a slightly larger PR diff the first time they deploy after upgrading the fleet's `github/gh-aw` pin.

## Post-Phase-1 Constitution Re-check

After Phase 1 design:

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Thin Orchestrator | ✅ | No new subprocess logic; all writes use stdlib |
| II. Testing Standards | ✅ | Pure functions unit-testable offline; subprocess paths covered by seam injection |
| III. Three-Turn Mutation | ✅ | Manifest write inside existing `--apply` gate |
| IV. Performance | ✅ | One additional API call per repo in status; no cloning |
| No new deps | ✅ | All stdlib |
| Self-documenting code | ✅ | All new exported identifiers have godoc; unexported helpers use expressive names |
