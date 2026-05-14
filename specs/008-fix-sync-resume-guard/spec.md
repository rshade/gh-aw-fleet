# Feature Specification: Sync Resume-Guard Regression Coverage (Issue #48)

**Feature Branch**: `008-fix-sync-resume-guard`
**Created**: 2026-05-12
**Status**: Draft
**Input**: User description: Issue #48 — `bug(sync): preflight + apply mis-trigger "refusing to resume" check on internally-prepared clones`. The operator observes that `gh-aw-fleet sync` aborts with `fatal: work-dir is on default branch "main"; refusing to resume on a protected branch` on any repo with `Missing > 0` workflows, blocking dry-run preflight, `--apply`, and `--apply --prune` paths.

## Clarifications

### Session 2026-05-12

- Q: How thick should the new apply-path regression test be (FR-008b, US2)? → A: Stop at the commit gate. Assert `res.Deploy.Added` is populated and the fake `gh` shim recorded one `aw add` per missing workflow; do not fake `gh pr create`. Push/PR coverage already lives in `deploy_test.go`; this test pins only the resume-guard bypass.
- Q: Which Conventional Commits type should the merge commit closing #48 use (`fix(sync):` vs `test(sync):`)? → A: `fix(sync):`. release-please surfaces `fix:` under "Fixed" in CHANGELOG; `test:` is hidden. Even though the diff is test-only, the user-facing delivery is the bug closure and belongs in release notes.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator dry-runs `sync` for a repo with missing workflows (Priority: P1)

A fleet operator is onboarding a repo (e.g., `rshade/finfocus`) into declared state. The repo's `.github/workflows/` directory lacks one or more workflows declared by its profile in `fleet.json`. The operator runs `gh-aw-fleet sync <repo>` (no `--apply`) to see what would change. They expect a clean dry-run summary listing the missing workflows and a successful exit code; they do **not** expect the tool to abort with a "refusing to resume" error message that refers to internal mechanics they did not invoke.

**Why this priority**: The bug originally manifested most visibly here — the dry-run still printed its summary, then a fatal log line caused a non-zero exit. This was the gateway error operators hit first; if dry-run is broken, they have no confidence to run `--apply`. The fix scaffolding (the `InternalClone` field on `DeployOpts` and its plumbing through `runPreflight`) already shipped in PR #66, and one regression test exists. This story confirms that path stays covered and the behavior is what the issue requires.

**Independent Test**: Construct a fleet config declaring one workflow that does not exist in the target's workflows directory. Invoke `Sync` with `SyncOpts{}` (Apply=false). Assert: returns nil error, `res.Missing` contains the workflow, `res.DeployPreflight` is non-nil, and `res.DeployPreflight.Added` contains the workflow (proving the preflight pipeline actually ran rather than aborting at the resume guard).

**Acceptance Scenarios**:

1. **Given** a fleet config where the target repo's profile declares workflows not present on disk, **When** the operator runs `gh-aw-fleet sync <repo>` without `--apply`, **Then** the command exits 0, prints the missing/expected/drift summary, and emits no `level=fatal` log line referencing "refusing to resume".
2. **Given** the same setup, **When** Sync hands its internally-prepared clone to `Deploy` for preflight compilation, **Then** `Deploy` does not invoke the `handleWorkDirResume` branch and continues into `ensureInit`/`addResolvedWorkflows`, producing a populated `DeployPreflight` result on `SyncResult`.

---

### User Story 2 - Operator applies `sync` for a repo with missing workflows (Priority: P1)

The operator confirmed the dry-run, then runs `gh-aw-fleet sync --apply <repo>` to open a PR adding the missing workflows. They expect the standard deploy flow — clone, switch to a new branch, `gh aw add` for each missing workflow, commit, push, open PR — to proceed normally. They do not expect the apply path to fail with the same protected-branch error that the dry-run preflight no longer triggers.

**Why this priority**: This is the operator's actual goal. The dry-run is reconnaissance; the `--apply` path is the value delivery. If P1 (dry-run) is covered but P2 (apply) regresses silently, the operator loses the ability to onboard repos through `sync` and is forced into the documented workaround (`deploy --apply`), which is itself only partially functional (no prune support).

**Independent Test**: Construct the same fleet config as US1 plus a fake `gh` shim that records every invocation. Invoke `Sync` with `SyncOpts{Apply: true}`. Assert: returns nil error, `res.Deploy` is non-nil, `res.Deploy.Added` contains the missing workflow, the fake `gh` shim recorded one `aw add` invocation per missing workflow, and no fatal error referencing "refusing to resume" surfaced. The test stops at the commit gate — it does not fake `gh pr create` and does not assert on `res.Deploy.PRURL`, because PR creation is already covered by `deploy_test.go` and is not under test for the resume-guard regression.

**Acceptance Scenarios**:

1. **Given** a fleet config with one missing workflow declared, **When** the operator runs `gh-aw-fleet sync --apply <repo>`, **Then** `Sync` calls `Deploy` with `InternalClone=true`, the resume guard at `internal/fleet/deploy.go:203` is bypassed, and the apply pipeline runs through `addResolvedWorkflows` and reaches the commit gate with `res.Deploy.Added` populated (the regression test stops at this point per the Clarifications session; downstream push/PR steps are out of scope for this test).
2. **Given** an operator runs `--apply` against a repo that is `Missing == 0`, **When** the command runs, **Then** behavior is unchanged from before this spec — no `Deploy` call, no resume guard interaction, no error — proving the fix does not regress repos already in declared state.

---

### User Story 3 - Operator runs `sync --apply --prune` in a single shot (Priority: P2)

The operator has a repo with both drift (workflow files present that fleet.json does not declare) and missing workflows. They want one PR that removes the drift files and adds the missing workflows. They run `gh-aw-fleet sync --apply --prune <repo>` and expect a single PR containing both the deletions and the additions.

**Why this priority**: This combination is the one path with no documented workaround in the issue. `deploy --apply` covers the additive case but does not prune, so a repo requiring both operations currently cannot be reconciled in a single PR. Covering it as an acceptance scenario both validates the fix and protects the orchestration in `applyDeployOrPrune` (prune first, then call `Deploy` with the prepared clone) against future regressions.

**Independent Test**: Construct a fleet config declaring workflow A; seed the on-disk workflows directory with workflow B (drift) but not A (missing). Invoke `Sync` with `SyncOpts{Apply: true, Prune: true}`. Assert: returns nil error, `res.Pruned` contains B, `res.Deploy.Added` contains A, and the fake `gh` shim records the staged deletion before the add — proving both operations occurred under one `Deploy` invocation that bypassed the resume guard.

**Acceptance Scenarios**:

1. **Given** a target repo with `Drift > 0` (workflow B on disk, not declared) and `Missing > 0` (workflow A declared, not on disk), **When** the operator runs `gh-aw-fleet sync --apply --prune <repo>`, **Then** `applyDeployOrPrune` removes B and stages the deletion, then calls `Deploy` with `InternalClone=true` to add A, and both changes land in the result without the resume guard firing.
2. **Given** the same drift+missing setup, **When** `--prune` is omitted (just `--apply`), **Then** only A is added and B remains untouched on disk, confirming prune is opt-in and the InternalClone propagation does not accidentally enable prune behavior.

---

### Edge Cases

- **Repo already at declared state (Missing == 0, Drift == 0).** Sync must not invoke `Deploy` at all (`sync.go` short-circuits before both `runPreflight` and `applyDeployOrPrune`'s `Deploy` call). The fix must not regress this path — no `InternalClone` propagation issues can arise if `Deploy` is never called.
- **Operator-supplied `--work-dir <existing-clone>`.** When `SyncOpts.WorkDir != ""`, `InternalClone` must remain `false` on the `DeployOpts` passed downstream so a genuine user-initiated resume still triggers `handleWorkDirResume`. The wiring in `sync.go:158` and `sync.go:206` (`InternalClone: opts.WorkDir == ""`) encodes this contract; the spec covers it as a negative test.
- **`Sync` called with `--prune` but without `--apply`.** Already errors with `"--prune requires --apply"` at `sync.go:66` — the spec must not relax this validation while adding coverage to the apply+prune path.
- **`Sync` with `Apply=true` but `Missing == 0` and `Drift > 0` (prune-only).** Goes through `commitAndPushPrune` (`sync.go:165–167`), never calling `Deploy`. The fix must not perturb this path; if a future refactor routes prune-only through `Deploy`, the InternalClone propagation must be re-evaluated.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `Sync` MUST hand its internally-prepared clone to `Deploy` in a way that distinguishes it from a user-supplied `--work-dir` resume request, so `Deploy`'s resume-on-protected-branch safety check does not fire on those calls. (Already implemented via `DeployOpts.InternalClone`; this spec pins the behavior with regression tests.)
- **FR-002**: When `Sync` is invoked without `--apply` and the target repo has at least one missing workflow, `Sync` MUST return successfully (nil error) after populating `SyncResult.DeployPreflight`, and MUST NOT surface any error message containing "refusing to resume".
- **FR-003**: When `Sync` is invoked with `--apply` and the target repo has at least one missing workflow, `Sync` MUST drive `Deploy` through its add → commit → push → PR pipeline using the internally-prepared clone, and MUST NOT surface any error message containing "refusing to resume".
- **FR-004**: When `Sync` is invoked with `--apply --prune` and the target repo has both drift and missing workflows, `Sync` MUST remove the drift files first, then call `Deploy` to add the missing workflows on the same clone, producing a single result containing both `Pruned` and `Deploy.Added` populated.
- **FR-005**: When `Sync` is invoked with `SyncOpts.WorkDir` explicitly set by the operator (`--work-dir <path>`), `Sync` MUST propagate `InternalClone=false` so `Deploy`'s resume detection still runs and the protected-branch guard remains in force for genuine user-initiated resume attempts.
- **FR-006**: The `--prune` flag without `--apply` MUST continue to error with the message `"--prune requires --apply"` — this validation predates the spec and is unchanged.
- **FR-007**: Repos already in declared state (`Missing == 0`, `Drift == 0`) MUST NOT trigger any `Deploy` invocation under any combination of `Sync` flags that this spec covers, preserving the existing short-circuit semantics in `sync.go`.
- **FR-008**: Test coverage in `internal/fleet/sync_test.go` MUST include at minimum: (a) the existing dry-run preflight case (P1), (b) a new apply case proving the resume guard is bypassed (P1) — depth: stop at the commit gate, assert `res.Deploy.Added` populated and fake `gh` recorded one `aw add` per missing workflow, do not fake `gh pr create`, (c) a new apply+prune case proving both operations execute on the same internal clone (P2) — same depth as (b), with the added assertion that the staged prune deletion is observable before the `aw add` calls. Coverage for the negative case (operator-supplied `--work-dir` preserves the guard, FR-005) is desirable but may live in `deploy_test.go` if more natural there.
- **FR-009**: Issue #48 on GitHub MUST be referenced from the merge commit, and the commit subject MUST use Conventional Commits type `fix(sync):` (e.g., `fix(sync): close resume-guard regression coverage gap (#48)`). `test(sync):` is rejected because release-please hides `test:` from `CHANGELOG.md`, which would invisibilize the bug closure to users reading release notes. Once tests merge and `make ci` is green, issue #48 must be closed with a comment linking to the merge commit.

### Key Entities

- **`SyncOpts.WorkDir`** — operator-facing flag value. Empty when omitted (the common case). Non-empty when the operator explicitly resumes an interrupted apply.
- **`DeployOpts.InternalClone`** — internal-only signal added in PR #66. `true` whenever `Sync` is the caller and the clone was prepared by `prepareClone` in this process. The disambiguator that makes FR-001 implementable without renaming the `WorkDir` field.
- **`SyncResult.DeployPreflight`** — populated when `Apply=false` and `Missing > 0`; the assertion target proving the preflight call site bypassed the resume guard.
- **`SyncResult.Deploy`** — populated when `Apply=true` and `Missing > 0`; the assertion target proving the apply call site bypassed the resume guard.
- **`SyncResult.Pruned`** — populated when `Apply=true && Prune=true && Drift > 0`; the assertion target proving prune ran before `Deploy` under the combined-flag path.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Fleet operator dry-run, apply, and apply+prune `sync` invocations against repos with `Missing > 0` complete without any `"refusing to resume"` error reaching the operator — measured by the existing test `TestSyncDryRunPreflightTreatsPreparedCloneAsInternal` continuing to pass *and* the new `TestSyncApplyBypassesResumeGuard` + `TestSyncApplyPruneBypassesResumeGuard` cases passing; each case asserts the absence of the substring on the returned error and on captured logs.
- **SC-002**: An operator can reconcile a repo requiring both drift removal and missing-workflow additions in **one** invocation of `gh-aw-fleet sync --apply --prune <repo>`, producing **one** PR — measured by a new test demonstrating that `applyDeployOrPrune` calls `Deploy` exactly once when both `Pruned` and `Missing` are non-empty, and by the operator's ability to run the command against a real repo without falling back to the two-PR workaround.
- **SC-003**: The new sync test cases are *cheap* — observable indirectly via `make test` wall-clock not regressing materially from the pre-slice baseline. This is **not CI-enforced** (no per-test timing gate exists); the goal is design discipline (reuse `newTestRepo` + `installFakeGhForSync`, stop at the commit gate, avoid network calls) rather than a measured ceiling. If a future test breaches this design, it should be split rather than retroactively wired into a CI timer.
- **SC-004**: Zero new external dependencies introduced by this change — measured by `go.mod` and `go.sum` being unchanged after the spec is implemented (per project Constitution Principle I; the work is test-coverage-only against an already-shipped fix).
- **SC-005**: Issue #48 is closed on GitHub within the same change set that lands the new tests, with a closing comment that links to the merge commit and identifies PR #66 as the original fix — measured by the issue state being `CLOSED` and the comment present.
- **SC-006**: The `DeployOpts.InternalClone` field's godoc remains accurate and self-explanatory after this work — measured by `make lint` (which runs `revive`/`staticcheck` with no `exported X should have comment` suppressions per `AGENTS.md`) staying clean.

## Assumptions

- The field-level fix described in the issue's "Suggested fix — Option 1" already landed in PR #66 (commit `0694ae9`, merged 2026-04-30). This spec's primary deliverable is therefore the regression-coverage gap, not a fresh implementation of `InternalClone`. The issue remained open because the apply and apply+prune call sites lacked dedicated tests proving the fix held.
- The user explicitly preferred Option 1 (single field addition) over Option 2 (split `CloneDir` vs. `WorkDir` params) in the issue body. This spec honors that preference and does not include a follow-on refactor to rename or split the field. If the codebase later outgrows the single-flag approach, that becomes a separate spec.
- Test infrastructure (`newTestRepo`, `installFakeGhForSync`) already exists in `internal/fleet/sync_test.go` and is sufficient for the new cases. The fake `gh` script may need a small extension to record `aw add` invocations more strictly (e.g., counting them) for the apply-path assertions; that's an in-test edit, not a new helper.
- The new tests for the apply and apply+prune paths stop at the commit gate. They do **not** fake `gh pr create`, do **not** drive a real `git push`, and do **not** assert on `res.Deploy.PRURL`. The success condition is that `Deploy` executes past `handleWorkDirResume` and reaches `addResolvedWorkflows` — observable through `res.Deploy.Added` being populated and the fake `gh` shim recording one `aw add` per missing workflow. PR-creation coverage already exists in `deploy_test.go` and is not under test for the resume-guard regression. This depth choice keeps the per-test wall-clock under ~50ms and avoids growing the fake `gh` shim into a PR-creation fixture.
- The Conventional Commits type for the merge commit is `fix(sync):` (clarified in Session 2026-05-12). `test(sync):` and `ci(workflows):` are both rejected: `test:` is hidden from `CHANGELOG.md` by release-please and would invisibilize the bug closure; `ci(workflows):` is reserved for changes that modify generated workflow files in target repos per `AGENTS.md`. release-please will categorize `fix(sync):` under "Fixed" in `CHANGELOG.md`, surfacing the issue #48 closure in the next release notes.
- The operator does not need any user-facing CLI documentation update for this change. `gh-aw-fleet sync` retains its existing flags and exit-code contract; the fix restores the documented behavior. No `--help` text, README, or skill file needs to change.
