---
description: "Task list for Sync Resume-Guard Regression Coverage (Issue #48)"
---

# Tasks: Sync Resume-Guard Regression Coverage (Issue #48)

**Input**: Design documents from `/specs/008-fix-sync-resume-guard/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, quickstart.md

**Tests**: This slice **IS** the tests. The production fix (`DeployOpts.InternalClone`) already shipped in PR #66 (commit `0694ae9`, merged 2026-04-30). All implementation tasks below are test-only edits to `internal/fleet/sync_test.go` (plus one optional case in `internal/fleet/deploy_test.go`). No production source file is modified.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing. US1 is "verify the pre-existing test still passes" (no new code); US2 and US3 are the two new tests that close the regression coverage gap.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

Single Go project. All source under repository root. Tests co-located with the package under test in `internal/fleet/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Verify the working environment can run the new tests at all.

- [X] T001 Run `go test -run TestSyncDryRunPreflightTreatsPreparedCloneAsInternal ./internal/fleet/...` from repo root to confirm the existing sync test passes on the current branch before any edits. Captures the green baseline so any subsequent failure is attributable to this slice's edits.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Extend the fake-`gh` shim so both new tests can assert on `aw add` invocation count and ordering. Per [research.md Decision 1](./research.md), the shim writes an append-only log to `$FAKE_GH_LOG`.

**⚠️ CRITICAL**: This shim extension blocks both US2 and US3. Complete this phase before starting either user story.

- [X] T002 In `internal/fleet/sync_test.go` extend `installFakeGhForSync` (currently lines 47–83) to: (a) accept the log path implicitly via the existing `t.TempDir()` `binDir` — create `logPath := filepath.Join(binDir, "fake-gh.log")` and call `t.Setenv("FAKE_GH_LOG", logPath)`; (b) in the shell script, in each existing branch (`aw init`, `aw add`), append a canonical event line to `${FAKE_GH_LOG:?}` *before* `exit 0` — format: `init\n` for the init branch and `add <spec>\n` for the add branch (where `<spec>` is the unmodified `$3`); (c) keep `set -eu` and use `${FAKE_GH_LOG:?}` so a missing setenv fails the test loudly. Do **not** alter the existing `repo clone` branch — it has nothing to log for the cases under test. Return the log path from the helper (change signature to `installFakeGhForSync(t *testing.T, remote string) (logPath string)`) so each caller can `os.ReadFile(logPath)` without re-deriving the path.

- [X] T003 [P] In `internal/fleet/sync_test.go` update the existing `TestSyncDryRunPreflightTreatsPreparedCloneAsInternal` (line 10) to capture the new return value from `installFakeGhForSync` with `_ = installFakeGhForSync(t, remote)` — the dry-run test does not need to assert on the log, but the call site must compile after T002 changes the signature. This is a pure mechanical edit; it must not change the existing assertions.

**Checkpoint**: Foundation ready — `go test -run TestSyncDryRunPreflightTreatsPreparedCloneAsInternal ./internal/fleet/...` still passes, and US2/US3 can now consume `$FAKE_GH_LOG`.

---

## Phase 3: User Story 1 — Operator dry-runs `sync` for a repo with missing workflows (Priority: P1)

**Goal**: Pin the dry-run preflight path — `Sync` with `Apply=false` and `Missing > 0` returns nil, populates `DeployPreflight`, and never triggers the resume guard.

**Independent Test**: `go test -run TestSyncDryRunPreflightTreatsPreparedCloneAsInternal ./internal/fleet/...` returns PASS.

This story is **already covered** by the pre-existing `TestSyncDryRunPreflightTreatsPreparedCloneAsInternal` (sync_test.go:10). The only US1 task is verifying that coverage survives the Phase 2 shim signature change.

### Implementation for User Story 1

- [X] T004 [US1] After T002/T003 land, run `go test -run TestSyncDryRunPreflightTreatsPreparedCloneAsInternal ./internal/fleet/...` and confirm PASS. If it fails, the Phase 2 edits regressed the dry-run path — fix the shim change before proceeding to US2.

**Checkpoint**: US1 is green and the dry-run regression coverage from PR #66 remains intact.

---

## Phase 4: User Story 2 — Operator applies `sync` for a repo with missing workflows (Priority: P1) 🎯 MVP

**Goal**: Prove `Sync` with `Apply=true` and `Missing > 0` drives `Deploy` past the resume guard (`internal/fleet/deploy.go:203`) and into `addResolvedWorkflows`. This is the load-bearing deliverable per spec FR-008(b) and the gate for closing issue #48.

**Independent Test**: `go test -run TestSyncApplyBypassesResumeGuard ./internal/fleet/...` returns PASS, demonstrating `res.Deploy.Added` is populated and the fake-`gh` log recorded one `add` line per missing workflow.

### Implementation for User Story 2

- [X] T005 [US2] In `internal/fleet/sync_test.go` add a new test function `TestSyncApplyBypassesResumeGuard`. Construct the same fixture shape as `TestSyncDryRunPreflightTreatsPreparedCloneAsInternal` (one repo, profile `default`, source `githubnext/agentics` pinned to `v1.0.0`, one workflow `ci-doctor`). Set `remote := newTestRepo(t, nil)` (helper at `internal/fleet/deploy_test.go:530`). Capture `logPath := installFakeGhForSync(t, remote)`. Invoke `Sync(context.Background(), cfg, repo, SyncOpts{Apply: true})`. The test stops at the commit gate per Clarifications Session 2026-05-12 — accept that `Sync` may return a non-nil error from the downstream `git push` / `gh pr create` steps that the fake shim does not simulate. **Assert on observable state regardless of return error**: (a) `res.Missing` contains `ci-doctor` (sanity), (b) `res.Deploy != nil` and `res.Deploy.Added` contains a workflow whose `Name == "ci-doctor"` — proving `addResolvedWorkflows` ran past the resume guard, (c) read `logPath` and assert it contains exactly one line starting with the literal prefix `"add "` whose suffix is `githubnext/agentics/ci-doctor@v1.0.0` — proving the fake `gh aw add` shim was invoked once with the fleet-pinned spec, (d) the log MUST NOT be missing or empty (would mean Deploy aborted before `addResolvedWorkflows`). Count occurrences by splitting the log on `"\n"` and counting lines whose `strings.HasPrefix(line, "add ")` returns true — do **not** use `strings.Count(string(log), "\nadd ")`, which misses the first line when it is the very first record written (no preceding newline). Use `strings.Contains` for the per-spec substring assertion. Do **not** assert on `res.Deploy.PRURL`, `res.Deploy.MissingSecret`, `res.Deploy.ActionsDisabled`, or `res.Deploy.SecurityFindings` — those are downstream of the guard and out of scope per spec FR-008(b).

- [X] T006 [US2] Run `go test -run TestSyncApplyBypassesResumeGuard ./internal/fleet/...` and confirm PASS. If it fails with a `"refusing to resume"` fatal in the test output, the resume guard regressed — re-read `internal/fleet/deploy.go:203` and `internal/fleet/sync.go:158` to verify the `InternalClone: opts.WorkDir == ""` wiring is unbroken on the current branch.

**Checkpoint**: US2 is green. This single test is sufficient to close issue #48 from the operator's perspective — `sync --apply` against a `Missing > 0` repo is provably back to working.

---

## Phase 5: User Story 3 — Operator runs `sync --apply --prune` in a single shot (Priority: P2)

**Goal**: Prove `Sync` with `Apply=true && Prune=true` against a repo that has both drift and missing workflows runs prune first and then `Deploy` on the same clone, producing a single result with both `Pruned` and `Deploy.Added` populated. Protects `applyDeployOrPrune`'s prune-then-deploy orchestration against future regressions.

**Independent Test**: `go test -run TestSyncApplyPruneBypassesResumeGuard ./internal/fleet/...` returns PASS, demonstrating `res.Pruned` contains the seeded drift workflow, `res.Deploy.Added` contains the declared-but-missing workflow, and the order of events in the fake-`gh` log is consistent with prune-before-add.

### Implementation for User Story 3

- [X] T007 [US3] In `internal/fleet/sync_test.go` add a new test function `TestSyncApplyPruneBypassesResumeGuard`. Construct the same config shape as T005 but with the additional drift fixture. `newTestRepo` at `internal/fleet/deploy_test.go:530` has signature `func(t *testing.T, setup func(dir string)) string` — pass a setup callback (NOT a map) that creates `.github/workflows/drifted.md` inside the test repo with placeholder frontmatter (e.g., `"---\nsource: legacy/drifted@v0\n---\n"`) and commits it via `exec.Command("git", "add", "...")` + `exec.Command("git", "commit", "-m", "seed drift")` (mirror the file-write + `git add` + `git commit` pattern used elsewhere in `deploy_test.go`). The test repo itself acts as the remote that the fake `gh repo clone` consumes, so the seeded file must be present in HEAD before `Sync` runs. Declared workflows: only `ci-doctor` (so `drifted.md` is drift; `ci-doctor.md` is missing). Capture `logPath := installFakeGhForSync(t, remote)`. Invoke `Sync(context.Background(), cfg, repo, SyncOpts{Apply: true, Prune: true})`. Stop at the commit gate per Clarifications Session 2026-05-12. **Assertions** (made regardless of return error from downstream push/PR): (a) `res.Drift` contains `drifted` and `res.Missing` contains `ci-doctor` (sanity), (b) `res.Pruned` contains `drifted` — proving `pruneDriftFiles` ran, (c) `res.Deploy != nil` and `res.Deploy.Added` contains a workflow whose `Name == "ci-doctor"` — proving `Deploy` ran past the resume guard on the same clone after prune, (d) read `logPath` and assert exactly one line with the literal `"add "` prefix and exactly one line with the literal `"init"` prefix if init fires before add (otherwise just one `add`-prefixed line) — the count assertion is the load-bearing one; the order-of-operations assertion for prune-before-add is observable via `res.Pruned` being non-empty when `res.Deploy.Added` is non-empty in the same `*SyncResult` (the data structure encodes the order in code). Do **not** add a separate observation channel for git-add-after-prune; the existing assertions on `res.Pruned` plus `res.Deploy.Added` are sufficient per spec FR-008(c).

- [X] T008 [US3] In `internal/fleet/sync_test.go` add a second test function `TestSyncApplyWithoutPruneLeavesDriftUntouched` covering the negative case from spec Acceptance Scenario US3.2: same fixture as T007 but invoke `Sync` with `SyncOpts{Apply: true}` (no `Prune`). Assert in this order: (a) `res.CloneDir != ""` (sanity — if empty, `Deploy` aborted before `prepareClone` returned, and the subsequent file-existence check below would `Stat` an invalid path with a misleading error), (b) `res.Pruned` is empty (drift remained untouched), (c) `res.Deploy.Added` still contains `ci-doctor` (missing got added), (d) `drifted.md` still exists on disk at `filepath.Join(res.CloneDir, ".github", "workflows", "drifted.md")`. This pins the `--prune` opt-in contract from the spec's US3 acceptance scenario; FR-007's broader Missing==0/Drift==0 short-circuit is covered separately in T016.

- [X] T009 [US3] Run `go test -run TestSyncApplyPrune ./internal/fleet/...` and confirm both new cases pass.

**Checkpoint**: US3 is green. The only path with no documented workaround (`--apply --prune` against a drift+missing repo) is now regression-tested.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Pin the two negative-case FRs (FR-006, FR-007), run the full CI gate, optionally cover FR-005 in `deploy_test.go`, and close issue #48 per FR-009 / SC-005.

- [X] T015 [P] [Polish] **FR-006 negative-case coverage.** In `internal/fleet/sync_test.go` add `TestSyncPruneWithoutApplyErrors`: invoke `Sync(context.Background(), cfg, repo, SyncOpts{Prune: true, Apply: false})` with any valid `cfg`/`repo` shape (the existing US1 fixture works — no fake `gh` needed because validation fires at `sync.go:66` before any `gh` invocation). Assert the returned `err` is non-nil and its `Error()` contains the substring `"--prune requires --apply"`. ~5 lines of test code. Pins the validation gate against silent removal by future refactors.

- [X] T016 [P] [Polish] **FR-007 short-circuit coverage.** In `internal/fleet/sync_test.go` add `TestSyncNoMissingNoDriftShortCircuits`: construct a fleet config whose declared workflow already exists on disk in the test repo's `.github/workflows/` (seed it via `newTestRepo`'s `setup func(dir string)` callback — same shape as T007, but the seeded file's frontmatter `source:` matches the declared `githubnext/agentics/ci-doctor@v1.0.0` exactly so it counts as in-state, not drift). Capture `logPath := installFakeGhForSync(t, remote)`. Run the matrix `SyncOpts{Apply: false}`, `SyncOpts{Apply: true}`, and `SyncOpts{Apply: true, Prune: true}` in a table-driven loop. For each case assert: (a) `err == nil`, (b) `res.Deploy == nil` AND `res.DeployPreflight == nil` (no `Deploy` invocation under any flag combo), (c) `res.Pruned` is empty, (d) the fake-`gh` log at `logPath` contains zero `"add "`-prefixed lines. Pins FR-007 against any future refactor that accidentally routes a repo-at-declared-state through `Deploy`.

- [X] T010 [P] Investigate whether `internal/fleet/deploy_test.go` already covers FR-005 (operator-supplied `--work-dir` keeps `InternalClone=false` and the resume guard active). Grep for `InternalClone` in `internal/fleet/deploy_test.go`; if any existing case constructs `DeployOpts{WorkDir: "...", InternalClone: false}` against a clone on a default branch and asserts the `"refusing to resume"` error, mark FR-005 as already covered and skip T011. If not, proceed to T011. Per [research.md Decision 2](./research.md), this coverage belongs in `deploy_test.go`, not `sync_test.go`.

- [X] T011 [P] (Conditional on T010 finding no existing coverage.) In `internal/fleet/deploy_test.go` add `TestDeployUserSuppliedWorkDirTriggersResumeGuard`: prepare a temp directory that looks like a fresh clone of a repo on its default branch (no feature branch checked out), call `Deploy(ctx, cfg, repo, DeployOpts{Apply: false, WorkDir: tempDir, InternalClone: false})`, assert the returned error contains the substring `"refusing to resume"` and is a non-nil error. This pins the negative case for the field's contract.

- [X] T012 Run `make ci` from repo root. All four steps (`fmt-check vet lint test`) MUST pass. Per AGENTS.md, `make lint` can run over 5 minutes locally — do not skip and do not bypass with `--no-verify`. `revive`/`staticcheck` MUST stay clean on `DeployOpts.InternalClone`'s godoc per SC-006 (the field's godoc was not edited; if a lint complains, re-read PR #66's diff).

- [ ] T013 Run the manual smoke (optional, per [quickstart.md](./quickstart.md#manual-smoke-optional-against-a-real-repo)): `go run . sync rshade/finfocus` should exit 0 with no `"refusing to resume"` line on stderr. Not required for the slice to ship — included for reviewer confidence.

- [ ] T014 After the PR merges (separate PR-merge turn, not part of this slice's commit), close issue #48 with `gh issue close 48 --repo rshade/gh-aw-fleet --comment "Fixed by PR #66 (field-level fix, merged 2026-04-30); regression coverage added in <merge-commit-sha> via PR #<this-pr>."` per FR-009 and SC-005. The commit subject MUST start with `fix(sync):` so release-please surfaces the closure under "Fixed" in `CHANGELOG.md`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1, T001)**: No dependencies — run first to confirm green baseline.
- **Foundational (Phase 2, T002–T003)**: Depends on Setup. T002 is the load-bearing shim extension; T003 is the mechanical signature-fix follow-up. **Blocks US2 and US3.**
- **US1 (Phase 3, T004)**: Depends on Phase 2 (because the existing test now consumes the new `installFakeGhForSync` return).
- **US2 (Phase 4, T005–T006)**: Depends on Phase 2. Independent of US1 verification (US1 just confirms no regression).
- **US3 (Phase 5, T007–T009)**: Depends on Phase 2. Independent of US2 — but if doing serial MVP-first, US2 is the priority gate for closing #48.
- **Polish (Phase 6, T010–T016)**: T015 (FR-006) and T016 (FR-007) are independent negative-case coverage tasks; they share `sync_test.go` with US1/US2/US3 so they cannot literally run `[P]` against T002/T005/T007 but can land in either order relative to each other. Both depend on Phase 2's signature change so the `installFakeGhForSync` return value is consumable. T010/T011 are optional FR-005 coverage and can run any time after Phase 2. T012 (`make ci`) depends on all US tasks AND on T015/T016. T013 is optional. T014 happens post-merge.

### User Story Dependencies

- **US1 (P1)**: Already covered by `TestSyncDryRunPreflightTreatsPreparedCloneAsInternal` — Phase 3 only re-verifies.
- **US2 (P1)**: Independent; this is the MVP for closing #48.
- **US3 (P2)**: Independent; protects `applyDeployOrPrune` but not strictly required to close #48.

### Within Each User Story

- US2 (T005, T006): T005 writes the test, T006 runs it. Sequential.
- US3 (T007, T008, T009): T007 and T008 write different test functions in the same file (cannot mark `[P]` because they share `sync_test.go`); T009 runs both.

### Parallel Opportunities

- **T002 and T005 cannot run in parallel** (same file: `sync_test.go`). T005 depends on T002's signature change.
- **T003 must follow T002** (also `sync_test.go`).
- **US2 and US3 are independent in design** but the test functions share `sync_test.go`, so coding them in parallel is impractical — sequential is faster than rebasing twice.
- **T010 and T011** can run in parallel with T012 only if the developer is comfortable splitting `deploy_test.go` (T011) and the `make ci` run (T012); typical workflow runs T012 last.
- **The pre-existing US1 test (T004) and the new US2 test (T006) can be run in parallel** as separate `go test -run` invocations — both consume `installFakeGhForSync` but at different test-function granularity.

---

## Parallel Example

```bash
# Phase 6 (after US tasks land), confirm individual test functions before make ci:
go test -run TestSyncDryRunPreflightTreatsPreparedCloneAsInternal ./internal/fleet/... &
go test -run TestSyncApplyBypassesResumeGuard ./internal/fleet/... &
go test -run TestSyncApplyPruneBypassesResumeGuard ./internal/fleet/... &
wait
```

(`make test` runs everything serially via `go test ./...` and is the real CI gate.)

---

## Implementation Strategy

### MVP First (US1 verification + US2)

1. T001: confirm green baseline.
2. T002, T003: extend the fake-`gh` shim and fix the pre-existing test's call site.
3. T004: confirm US1 still green.
4. T005, T006: add and run `TestSyncApplyBypassesResumeGuard`.
5. **STOP and VALIDATE**: at this point, issue #48 is technically closable — the apply path (operator's actual workflow) is regression-tested. T007–T009 (US3) are nice-to-have polish but not required for the bug closure.

### Incremental Delivery

1. Setup + Foundational (T001–T003) → fake-gh shim emits a log.
2. US1 verification (T004) → MVP baseline holds.
3. US2 (T005–T006) → apply-path regression test green → **issue #48 closable**.
4. US3 (T007–T009) → apply+prune path locked in.
5. Polish (T010–T014) → optional FR-005 negative case, full `make ci`, issue close.

### Single-Developer Strategy

Recommended order: T001 → T002 → T003 → T004 → T005 → T006 → T007 → T008 → T009 → T015 → T016 → T010 → T011 → T012 → T013 → T014. All edits land in one PR with subject `fix(sync): close resume-guard regression coverage gap (#48)`. T015 and T016 are inserted *before* T010/T011 because they target sync_test.go (already the open editor for US1/US2/US3) — finishing all sync_test.go edits before pivoting to deploy_test.go keeps file-context churn minimal.

---

## Notes

- **No production source files are modified.** All edits target `internal/fleet/sync_test.go` (load-bearing) and optionally `internal/fleet/deploy_test.go` (FR-005 negative case). `go.mod` and `go.sum` MUST be unchanged after this slice (SC-004).
- **Commit subject**: `fix(sync): close resume-guard regression coverage gap (#48)` per FR-009. `test(sync):` is rejected because release-please hides `test:` from `CHANGELOG.md`.
- **The tests stop at the commit gate** per Clarifications Session 2026-05-12 — do not fake `gh pr create`, do not assert on `res.Deploy.PRURL`, do not extend the fake-`gh` shim to handle `pr` invocations.
- **Wall-clock budget**: each new test ≤ 50ms, cumulative ≤ 200ms (SC-003). Reuse `newTestRepo` and `installFakeGhForSync` to stay under budget.
- **`gpg` signing must not be bypassed** anywhere — irrelevant to this slice (no commits made by the tool path under test), but a standing invariant per AGENTS.md.
- **Issue closure (T014) is a post-merge action**, not part of the implementation commit. Sequence: merge PR → release-please picks up `fix(sync):` → close #48 with comment linking merge commit and PR #66.
