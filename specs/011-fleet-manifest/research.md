# Research: Fleet Manifest — Deployed Version Tracking

**Feature**: `011-fleet-manifest`
**Date**: 2026-06-11

## R1 — Multi-profile `gh_aw_version` resolution

**Question**: When a repo participates in multiple profiles that each reference `github/gh-aw` with
potentially different pins, which version should the manifest record?

**Decision**: Record the `Ref` of the first `ResolvedWorkflow` where `Source == "github/gh-aw"`.

**Evidence from codebase**:
- `Config.ResolveRepoWorkflows(repo)` returns `[]ResolvedWorkflow` in profile-declaration order.
- `ResolvedWorkflow.Source` is the source slug (constant `sourceGitHubAW = "github/gh-aw"`).
- `ResolvedWorkflow.Ref` is the pin from `Profile.Sources[source].Ref`.

**Rationale**:
- Fleet operators are expected to keep all profiles at the same `github/gh-aw` pin — it is a compiler, and version skew across profiles within one repo is a misconfiguration, not a supported use case.
- A per-source map adds JSON complexity and makes version comparison more complex without practical benefit.
- Single string is human-readable and trivial to compare.

**Alternatives rejected**:
- Per-source map `{"github/gh-aw": "v0.79.2"}`: more precise, breaks simplicity contract.
- Per-profile map `{"default": "v0.79.2", "security-plus": "v0.79.2"}`: duplicative, grows with profile count.
- Max/min across profiles: string comparison on semver is undefined without a semver library (no new deps).

---

## R2 — Manifest read path in `status` (no clone)

**Question**: How to read the manifest from a managed repo without cloning?

**Decision**: Extend `statusFetcher` interface with `fetchManifestBody(ctx, repo) (string, error)`.
Production binding uses `ghAPIRaw("/repos/{repo}/contents/.github/aw/fleet-manifest.json")`.

**Evidence from codebase**:
- `ghAPIRaw(ctx, path)` returns raw file content (base64-decoded by GitHub API) for content endpoints.
- `fetchWorkflowBody` in `ghStatusFetcher` uses exactly this pattern.
- `isGitHubNotFound(err)` classifies 404 errors — use this for the "unmanaged" case.

**Implementation**:
```go
func (ghStatusFetcher) fetchManifestBody(ctx context.Context, repo string) (string, error) {
    path := fmt.Sprintf("/repos/%s/contents/.github/aw/fleet-manifest.json", repo)
    body, err := ghAPIRaw(ctx, path)
    if err != nil {
        if isGitHubNotFound(err) {
            return "", nil // unmanaged
        }
        zlog.Debug().Str(fieldRepo, repo).Err(err).Msg("manifest fetch skipped")
        return "", nil // fail-open: non-404 errors → treat as unmanaged
    }
    return body, nil
}
```

**Fail-open rationale**: A manifest read error (network flakiness, rate limit) must not fail the
`status` command. The drift state falls back to `unmanaged`, which is conservatively informative
rather than falsely negative (`current`). The debug log preserves diagnosability.

---

## R3 — Timestamp churn prevention

**Question**: How to avoid `deployed_at` causing a git diff on every redeploy?

**Decision**: Read the existing manifest from the clone before writing. If all fields except
`DeployedAt` are equal to what we would write, skip the write entirely (return without modifying
the file). The manifest is only written (and `DeployedAt` updated) when content actually changes.

**Comparison function**:
```go
func manifestEqualExceptTime(a, b *FleetManifest) bool {
    return a.Managed == b.Managed &&
        a.Fleet == b.Fleet &&
        a.GhAwVersion == b.GhAwVersion &&
        a.CLIVersion == b.CLIVersion &&
        slices.Equal(a.Profiles, b.Profiles)
}
```

**Why slices.Equal**: `Profiles` is a `[]string`. Two manifests with the same profiles in different
order would be considered different. Callers should sort profiles before building the manifest to
ensure stable ordering. `buildManifest` will sort `cfg.Repos[repo].Profiles` before assignment.

**Alternative rejected**: Update `DeployedAt` only when other fields change but always write the
file. This still produces a diff if the file didn't exist before. The read-and-compare approach
produces zero diff on true same-version redeployes.

---

## R4 — `ensureInit` version-aware replacement

**Question**: How to replace the `initMarkerPaths` file-check with a version comparison?

**Decision**: Change `ensureInit` signature to accept `fleetGhAwVersion string`. Use
`readManifestFromClone` to fetch the existing manifest. Compare `manifest.GhAwVersion` to
`fleetGhAwVersion`. If equal → skip init. If absent, unreadable, or version mismatch → run init.

**Evidence from codebase**:
```go
// Current (to be replaced):
var initMarkerPaths = []string{
    ".github/agents/agentic-workflows.agent.md",
    ".github/agents/agentic-workflows.md",
}
func ensureInit(ctx context.Context, dir string) (bool, error) {
    for _, marker := range initMarkerPaths {
        if fileExists(filepath.Join(dir, marker)) {
            return false, nil
        }
    }
    // run gh aw init
}
```

**New signature**:
```go
func ensureInit(ctx context.Context, dir, fleetGhAwVersion string) (bool, error)
```

**Risk analysis**:
- The original check existed because `gh aw init` in v0.79+ CLIs could fail on repos initialized
  with the v0.68 layout. Per AGENTS.md: "gh aw init in newer CLIs fails on repos predating the
  dispatcher-skill layout." This is now mitigated differently: the manifest-based check means we
  only re-run init when we know the version advanced, and at that point the operator has also
  updated their CLI. If init fails on a legacy layout, the error surfaces and the clone is
  preserved — same behavior as any other pipeline failure.
- For repos with no manifest (first deploy after this feature ships), `fleetGhAwVersion != ""` but
  `manifest == nil` → init runs. This is correct: we don't know what version initialized the repo.
- The `initMarkerPaths` global var is removed. The commit message should call out this supersession.

**Call site in Deploy**:
```go
fleetPin := resolvedGhAwPin(cfg, repo)
res.InitWasRun, err = ensureInit(ctx, res.CloneDir, fleetPin)
```

**Call site in Sync**:
```go
fleetPin := resolvedGhAwPin(cfg, repo)
if _, initErr := ensureInit(ctx, res.CloneDir, fleetPin); initErr != nil { ... }
```

---

## R5 — Manifest write placement in the deploy pipeline

**Question**: Where exactly in `Deploy` / `Sync` should the manifest write happen to ensure
it is included in the staged commit?

**Decision**: Write the manifest in `Deploy`'s apply path, AFTER `runCompileStrictIfNeeded`
and BEFORE `hasStagedOrUnstagedWorkflowChanges`. The file at `.github/aw/fleet-manifest.json`
will be detected by `hasStagedOrUnstagedWorkflowChanges` (which checks `git status -- .github/`)
and staged by `branchAndStageGithub` (which calls `git add .github/`).

**Deploy sequence after change**:
```
prepareClone → ensureInit (version-aware) → addResolvedWorkflows
→ fixMisplacedSkillImports → checkEngineSecret → checkActionsSettings
→ security.Run → [apply gate] → runCompileStrictIfNeeded
→ writeManifestIfNeeded    ← NEW
→ hasStagedOrUnstagedWorkflowChanges
→ createDeployPR (branchAndStageGithub → git add .github/ → manifest included)
```

**Edge case — prune-only sync**: When `Sync` prunes drift files but has no missing workflows,
it calls `commitAndPushPrune` directly (not `Deploy`). The manifest write must be added before
`commitAndPushPrune` in `applyDeployOrPrune` to ensure prune-only commits also carry an
up-to-date manifest.

**Resume paths** (`handleWorkDirResume`): Resume paths in `deploy.go` call `stageResumeGithub`
which uses `assertResumeStagedScopedToGithub` to check no non-.github/ paths are staged. A
manifest written before the resume is invoked would be under `.github/` → safe. For resume-at-
push-gate paths, the manifest was already written in the prior run → no re-write needed. The
`writeManifestIfNeeded` no-op path covers this.

---

## R6 — CLI version acquisition

**Question**: How to get the CLI version (`cli_version` field) without introducing subprocess
coupling in the manifest package?

**Decision**: Use the existing `ghAwVersion` injectable seam (already defined in `deploy.go`).
Call it once in `Deploy` and pass the string to `buildManifest`. If the call fails, record `""`.

```go
cliVer, _ := ghAwVersion(ctx) // fails gracefully; "" is a valid recorded value
m := buildManifest(cfg, repo, cliVer)
```

**Why not in `manifest.go`**: `ghAwVersion` is a `deploy.go` seam. Importing it from `manifest.go`
would require either moving the seam (needless churn) or creating a package-level parameter.
Passing `cliVer` as a parameter to `buildManifest` is simpler and keeps `manifest.go` free of
subprocess calls.

---

## R7 — `statusJob` extension for `expectedVersion`

**Question**: How to pass the fleet's resolved `github/gh-aw` pin to the status worker pool?

**Decision**: Add `expectedVersion string` to `statusJob`. Populate it in `buildStatusJobs`:
```go
expectedVersion: resolvedGhAwPin(cfg, repo),
```

`resolvedGhAwPin` is defined in `manifest.go` and takes `(*Config, repo string)`. `buildStatusJobs`
already has access to `cfg`.

**Why not compute in the worker**: `resolvedGhAwPin` calls `cfg.ResolveRepoWorkflows(repo)`, which
is also called in `buildStatusJobs` (for the `declared` field). Computing it separately in the
worker would double the work. Pre-computing in `buildStatusJobs` reuses the same `cfg` access.

---

## R8 — Text output for `status`

**Question**: How to surface `VersionDrift` in the text output of `status`?

**Decision**: Add a `VERSION_DRIFT` column to the tabwriter table in `cmd/status.go`. Values:
`current`, `behind (recorded: vX, expected: vY)`, `unmanaged`, `n/a` (when `VersionDrift` is nil).

This is a `cmd/` layer concern and is outside `internal/fleet/`. The plan records it here for
completeness. The `cmd/` layer reads `result.Repos[i].VersionDrift` and formats accordingly.

---

## R9 — Struct tag consistency

**Question**: Should `VersionDrift` in `RepoStatus` use `omitempty` or always be present?

**Decision**: Use `omitempty` (pointer type, `*VersionDrift`). The field is nil only when the repo
errored before the manifest fetch ran. For all non-errored repos, `VersionDrift` is always populated
(at minimum with state `unmanaged`). This matches the spec's FR-006 requirement: "present and
populated for every repo entry, including unmanaged repos."

`RepoStatus.VersionDrift` is a pointer so that JSON serialization emits `null` (omitted with
`omitempty`) for errored repos, and emits the full object for all healthy repos. Consumers can
reliably check for `version_drift.state == "unmanaged"` on non-errored repos.
