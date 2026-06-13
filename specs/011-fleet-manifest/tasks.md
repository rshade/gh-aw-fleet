# Tasks: Fleet Manifest — Deployed Version Tracking

**Input**: Design documents from `specs/011-fleet-manifest/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅, quickstart.md ✅

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel with other [P] tasks at the same phase level (different files, no
  shared write conflict)
- **[Story]**: Maps to user story from spec.md (US1=P1, US2=P2a, US3=P2b)
- Exact file paths required for every task

---

## Phase 1: Setup

No new project structure needed — all additions are in the existing `internal/fleet/` package.
The `specs/011-fleet-manifest/` directory is already created by `/speckit-specify`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The `FleetManifest` type, helpers, and `VersionDrift` type used by all three user
stories. Must be complete before any user story work can begin.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T001 Create `internal/fleet/manifest.go` with: `FleetManifest` struct (fields: `Managed bool`, `Fleet string`, `GhAwVersion string`, `CLIVersion string`, `Profiles []string`, `DeployedAt time.Time`; all JSON-tagged per data-model.md); `FleetManifestPath = ".github/aw/fleet-manifest.json"` constant; `VersionDrift` struct (fields: `State string`, `RecordedVersion string`, `ExpectedVersion string`; all JSON-tagged); `VersionDriftBehind`, `VersionDriftCurrent`, `VersionDriftUnmanaged` constants; `readManifestFromClone(dir string) (*FleetManifest, error)` (returns nil,nil on ENOENT); `writeManifestToClone(dir string, m *FleetManifest) error` (MkdirAll + WriteFile); `manifestEqualExceptTime(a, b *FleetManifest) bool` (compares all fields except DeployedAt, uses slices.Equal for Profiles); `writeManifestIfNeeded(dir string, m *FleetManifest) (bool, error)` (read→compare→write-only-on-change); `resolvedGhAwPin(cfg *Config, repo string) string` (first ResolvedWorkflow where Source == sourceGitHubAW); `buildManifest(cfg *Config, repo, cliVersion string) *FleetManifest` (populated from cfg + resolvedGhAwPin; Profiles sorted; DeployedAt = time.Now().UTC(); Fleet = hardcoded "rshade/gh-aw-fleet" or sourced from config); `parseManifestJSON(body string) (*FleetManifest, error)` (returns nil,nil for empty body; json.Unmarshal; rejects Managed==false); `computeVersionDrift(m *FleetManifest, expectedVersion string) *VersionDrift` (nil manifest → unmanaged; version match → current; mismatch → behind). Every exported identifier must have a godoc comment.

- [X] T002 [P] Create `internal/fleet/manifest_test.go` with offline unit tests (no subprocess, no network) covering: `buildManifest` fields match expected from a hand-crafted `*Config`; `manifestEqualExceptTime` returns true when only DeployedAt differs, false when any other field differs; `writeManifestIfNeeded` writes on first call, skips on same-content call, writes on version-change call (use `t.TempDir()`); `parseManifestJSON` handles valid JSON, empty string, malformed JSON, and `managed:false`; `computeVersionDrift` covers all three state values and nil manifest input; `resolvedGhAwPin` returns correct pin from multi-profile config and empty string when no gh-aw source.

**Checkpoint**: `go build ./internal/fleet/...` and `go test ./internal/fleet/...` pass. Foundation ready — user story implementation can now proceed.

---

## Phase 3: User Story 1 — Deploy Records Version Provenance (Priority: P1) 🎯 MVP

**Goal**: Every `deploy --apply` run writes `.github/aw/fleet-manifest.json` into the target repo
clone, committed with the workflow changes. Same-version redeployes produce zero manifest diff.

**Independent Test**: Run `deploy --apply` against a fresh test repo clone (using `--work-dir` with a
temp dir). Verify `.github/aw/fleet-manifest.json` exists in the clone with correct fields. Run again
at the same version and verify the file is not modified (git diff shows no changes to the manifest).

- [X] T003 [US1] Modify `internal/fleet/deploy.go` apply path in `Deploy` function (after `runCompileStrictIfNeeded`, before `hasStagedOrUnstagedWorkflowChanges`): call `cliVer, _ := ghAwVersion(ctx)` to get the CLI version string; call `m := buildManifest(cfg, repo, cliVer)` to build the manifest; call `if _, err := writeManifestIfNeeded(res.CloneDir, m); err != nil { return res, fmt.Errorf("write fleet manifest: %w", err) }`. Import `time` if not already present. The manifest write must happen before `hasStagedOrUnstagedWorkflowChanges` so the new/changed manifest is detected as a pending change and staged by `branchAndStageGithub`.

- [X] T004 [US1] Extend `internal/fleet/deploy_test.go`: add test that when `Deploy` is called with `Apply=true` in a temp clone dir, `internal/fleet/manifest.go`'s `FleetManifestPath` file exists after the call with correct `fleet`, `gh_aw_version`, `profiles`, and `managed=true` fields; add test that a second `Deploy` call with the same version produces no change to the manifest file (compare file mtime or content); add test that a `Deploy` call with `Apply=false` (dry-run) does NOT write the manifest.

**Checkpoint**: User Story 1 is fully functional. `go test ./internal/fleet/... -run TestDeploy` passes.

---

## Phase 4: User Story 2 — Status Reports Version Drift Per Repo (Priority: P2)

**Goal**: `gh-aw-fleet status` shows per-repo version-drift state (`behind` / `current` /
`unmanaged`) in both text and JSON output.

**Independent Test**: Construct a fleet config with three repos and a fake `statusFetcher` that
returns: one manifest at current version, one manifest at old version, one missing manifest (404).
Run `Status()` and verify each `RepoStatus.VersionDrift.State` is `current`, `behind`, and
`unmanaged` respectively.

- [X] T005 [P] [US2] Modify `internal/fleet/status.go`: (a) Add `fetchManifestBody(ctx context.Context, repo string) (string, error)` to the `statusFetcher` interface; (b) Implement `ghStatusFetcher.fetchManifestBody` — call `ghAPIRaw(ctx, fmt.Sprintf("/repos/%s/contents/.github/aw/fleet-manifest.json", repo))`, return `("", nil)` on `isGitHubNotFound`, return `("", nil)` on other errors with a `zlog.Debug` entry (fail-open per constitution III); (c) Add `VersionDrift *VersionDrift \`json:"version_drift,omitempty"\`` field to `RepoStatus` with a godoc comment; (d) Add `expectedVersion string` field to `statusJob` (unexported); (e) In `buildStatusJobs`, populate `expectedVersion: resolvedGhAwPin(cfg, repo)` on each job; (f) In `processRepo`, after the `computeDrift` call, fetch the manifest body via `fetcher.fetchManifestBody`, parse it with `parseManifestJSON`, and set `rs.VersionDrift = computeVersionDrift(manifest, job.expectedVersion)`.

- [X] T006 [P] [US2] Create `internal/fleet/testdata/manifest/` directory with fixture files: `current.json` (valid manifest at version matching expected), `behind.json` (valid manifest at older version), `malformed.json` (invalid JSON), `managed_false.json` (valid JSON but managed:false); extend the fake `statusFetcher` in `internal/fleet/status_test.go` to implement `fetchManifestBody` (returning fixture content or `""` based on repo name); add test cases for each drift state (current, behind, unmanaged-no-file, unmanaged-malformed, unmanaged-managed-false) asserting correct `VersionDrift.State`, `RecordedVersion`, and `ExpectedVersion` values.

**Checkpoint**: User Story 2 is fully functional. `go test ./internal/fleet/... -run TestStatus` passes.

---

## Phase 5: User Story 3 — Stale Init Artifacts Refreshed on Deploy/Sync (Priority: P2)

**Goal**: `deploy`/`sync` re-runs `gh aw init` when the manifest version is behind (or absent),
replacing the crude `initMarkerPaths` file-check that left legacy repos stale.

**Independent Test**: Call `ensureInit` with a temp dir containing: (a) a manifest at the current
version → verify init subprocess is NOT called; (b) no manifest → verify init subprocess IS called;
(c) a manifest at an older version → verify init subprocess IS called.

- [X] T007 [US3] Modify `internal/fleet/deploy.go`: (a) Change `ensureInit` signature from `func ensureInit(ctx context.Context, dir string) (bool, error)` to `func ensureInit(ctx context.Context, dir, fleetGhAwVersion string) (bool, error)`; (b) Replace the `initMarkerPaths` loop body with: `if fleetGhAwVersion != "" { m, _ := readManifestFromClone(dir); if m != nil && m.GhAwVersion == fleetGhAwVersion { return false, nil } }` before the `exec.CommandContext` call; (c) Remove the `initMarkerPaths` package-level var and its `//nolint` comment; (d) Update the call site in `Deploy`: add `fleetPin := resolvedGhAwPin(cfg, repo)` before the `ensureInit` call and pass it: `res.InitWasRun, err = ensureInit(ctx, res.CloneDir, fleetPin)`; (e) Update the godoc comment on `ensureInit` to describe the manifest-based skip logic and note that `initMarkerPaths` has been superseded.

- [X] T008 [US3] Modify `internal/fleet/sync.go`: (a) Update the `ensureInit` call in `Sync` to pass the fleet pin: add `fleetPin := resolvedGhAwPin(cfg, repo)` before the `ensureInit` call and pass it: `if _, initErr := ensureInit(ctx, res.CloneDir, fleetPin); initErr != nil { ... }`; (b) In `applyDeployOrPrune`, add manifest write before the `commitAndPushPrune` call in the prune-only branch (when `len(res.Missing) == 0 && opts.Prune && len(res.Pruned) > 0`): call `ghAwVer, _ := ghAwVersion(ctx)`, build manifest, call `writeManifestIfNeeded(res.CloneDir, m)`, then `git add .github/` to stage it (use existing `gitCmd` helper). Note: when `Deploy` is called (missing workflows path), the manifest write inside `Deploy` covers that path — this task only covers the prune-only path.

- [X] T009 [P] [US3] Extend `internal/fleet/deploy_test.go` with `ensureInit` behavior tests: inject a no-op `ghAwVersion` seam and a no-op `gh aw init` seam; test skip when manifest exists at current version; test run when manifest is absent; test run when manifest records older version; test that `initMarkerPaths` is no longer referenced (grep test confirming removal); verify `InitWasRun=true` on the `DeployResult` when init ran.

- [X] T010 [P] [US3] Extend `internal/fleet/sync_test.go` with tests for: ensureInit call passes correct fleet pin; prune-only path writes manifest before committing (verify `FleetManifestPath` file exists in clone after prune-only sync); verify manifest is not written in dry-run mode.

**Checkpoint**: All three user stories are independently functional. `GOTOOLCHAIN=go1.25.8 go test ./internal/fleet/...` passes.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Text output surface, documentation, and skills updates. All [P] tasks touch different
files and can run in parallel after Phase 5 completes.

- [X] T011 [P] Modify `cmd/status.go` (locate the tabwriter table that renders `RepoStatus` rows): add a `VERSION_DRIFT` column header; in each row, emit `rs.VersionDrift.State` when `rs.VersionDrift != nil`, or `"n/a"` otherwise; for `"behind"` state, emit `behind (recorded: <RecordedVersion>, expected: <ExpectedVersion>)` to give operators the full context in text mode; verify the column aligns correctly for fleets with mixed states.

- [X] T012 [P] Update `AGENTS.md` "Deploy absorbs gh-aw CLI quirks" section: add a new bullet explaining that `ensureInit` now uses a manifest version comparison instead of `initMarkerPaths`; document `FleetManifestPath` as the fleet's footprint marker; add to "Hard invariants": `.github/aw/fleet-manifest.json` is written by the fleet tool only and must not be hand-edited.

- [X] T013 [P] Update `skills/fleet-deploy/SKILL.md` and `skills/fleet-onboard-repo/SKILL.md`: in fleet-deploy, replace any reference to the marker-file init skip with the manifest-version check; note that the first deploy after a `github/gh-aw` pin advance will re-run init and produce a larger PR diff; in fleet-onboard-repo, add a note that newly onboarded repos will get a manifest written on their first fleet deploy.

- [X] T014 Run `GOTOOLCHAIN=go1.25.8 make ci` from the repo root (`make ci` = `fmt-check vet lint test`); fix any `gofmt`, `go vet`, `golangci-lint`, or `go test` failures before marking this task done. This is the final gate.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Foundational (Phase 2)**: No dependencies — start immediately.
- **US1, US2, US3 (Phases 3–5)**: All depend on T001 completion (manifest.go).
  - T002 (manifest tests) can run in parallel with T003, T005 — different file.
  - T003 (deploy manifest write) and T007 (ensureInit change) both modify `deploy.go` → sequence T003 → T007.
  - T005 (status extension) and T008 (sync extension) modify different files → parallel with US1 after T001.
- **Polish (Phase 6)**: Depends on T005 (for T011), T007 (for T012/T013), T008 (for T013). All [P] within phase.
- **T014 (make ci)**: Final gate — depends on all preceding tasks.

### User Story Dependencies

| Story | Depends On | Can Parallelize With |
|-------|-----------|---------------------|
| US1 (T003–T004) | T001 | T002, T005, T006 |
| US2 (T005–T006) | T001 | T002, T003, T004 |
| US3 (T007–T010) | T001, T003 | T005, T006 after T003 |
| Polish (T011–T013) | T005, T007 | Each other [P] |

### Within Each User Story

- Types and helpers (T001) before any story work
- Implementation before tests (spec does not require TDD)
- deploy.go changes sequential (T003 → T007, same file)
- status.go changes sequential within US2 (T005 then T006)
- sync.go (T008) independent of deploy.go changes

---

## Parallel Opportunities

### Fastest solo path

```
T001 → T002
         ↘
T001 → T003 → T004
              ↘
T001 → T005 → T006
T003 → T007 → T008 → T009 [P]
                      T010 [P]
                         ↘
                      T011 [P]
                      T012 [P]
                      T013 [P]
                             ↘
                           T014
```

### Two-developer split

```
Developer A: T001 → T003 → T004 → T007 → T009 → T012 → T013 → T014
Developer B:         T002 → T005 → T006 → T008 → T010 → T011
```

---

## Implementation Strategy

### MVP (User Story 1 only — 4 tasks)

1. Complete T001 (foundational manifest.go)
2. Complete T002 (manifest unit tests)
3. Complete T003 (deploy manifest write)
4. Complete T004 (deploy tests)
5. **STOP and VALIDATE**: `deploy --apply` on a test repo writes `.github/aw/fleet-manifest.json`
6. This satisfies all P1 acceptance criteria and sets up the foundation for US2/US3

### Incremental Delivery

1. T001–T004 (Foundation + US1) → First PR: manifest write on deploy
2. T005–T006 (US2) → Second PR: status drift reporting
3. T007–T010 (US3) → Third PR: version-aware ensureInit (largest impact, removes legacy skip)
4. T011–T014 (Polish + CI gate) → Can be in any of the above PRs or a separate cleanup PR

### Acceptance Criteria Coverage

| AC | Task(s) |
|----|---------|
| `deploy --apply` writes manifest | T003, T004 |
| Manifest records required fields | T001, T003 |
| `status` reports drift state (text + JSON) | T005, T006, T011 |
| `sync`/`deploy` refreshes init on stale manifest | T007, T008, T009, T010 |
| No noisy manifest diff on same-version redeploy | T001 (writeManifestIfNeeded), T004 |
| Offline tests (manifest write, drift detection, init refresh) | T002, T004, T006, T009, T010 |
| `make ci` passes | T014 |

---

## Notes

- [P] tasks = different files, no shared write conflict at that phase level
- All tasks in `internal/fleet/` — no new packages, no new `cmd/` commands
- `cmd/status.go` (T011) is the only `cmd/` change — additive column only
- `initMarkerPaths` global var in `deploy.go` is deliberately removed by T007 — this is not a refactor, it is a principled supersession documented in AGENTS.md by T012
- T014 (`make ci`) requires `GOTOOLCHAIN=go1.25.8` prefix to avoid golangci-lint panic under Go 1.26
- No new third-party dependencies in any task — constitution §Third-Party Dependencies is satisfied
