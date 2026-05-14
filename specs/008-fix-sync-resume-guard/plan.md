# Implementation Plan: Sync Resume-Guard Regression Coverage (Issue #48)

**Branch**: `008-fix-sync-resume-guard` | **Date**: 2026-05-12 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/008-fix-sync-resume-guard/spec.md`

## Summary

The field-level fix for issue #48 already shipped in PR #66: `DeployOpts.InternalClone` (a bool) disambiguates an internally-prepared clone (Sync handing its scratch clone to Deploy) from a user-initiated resume (`--work-dir <path>`). `internal/fleet/deploy.go:203` gates `handleWorkDirResume` on `opts.WorkDir != "" && !opts.InternalClone`, so internally-prepared clones now bypass the protected-branch refusal that was blocking dry-run, `--apply`, and `--apply --prune` paths for repos with `Missing > 0`.

What this slice delivers is **regression coverage**: two new tests in `internal/fleet/sync_test.go` (apply, apply+prune) plus a small extension to `installFakeGhForSync` so the fake `gh` shim records `aw add` invocations and order-of-operations relative to staged prune deletions. Tests stop at the commit gate per Clarifications Session 2026-05-12 — no fake `gh pr create`, no `res.Deploy.PRURL` assertions; push/PR coverage already lives in `deploy_test.go`. The merge commit uses `fix(sync):` so release-please surfaces the closure under "Fixed" in CHANGELOG.

## Technical Context

**Language/Version**: Go 1.25.8 (per `go.mod`).
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (existing, not exercised by this slice), `github.com/rs/zerolog` v1.35.1 (existing, not exercised), `testing` (stdlib), `os/exec` for the fake-gh shim install (already used by existing tests). **No new direct dependencies** (per SC-004 and Constitution §Third-Party Dependencies).
**Storage**: N/A — tests construct an in-process `*Config` literal; the fake `gh` shim writes inside `t.TempDir()` directories that the test runtime garbage-collects.
**Testing**: Go's `testing` package via `make test`. The existing test `TestSyncDryRunPreflightTreatsPreparedCloneAsInternal` (sync_test.go:10) is the template — same helpers (`newTestRepo` from `deploy_test.go:530`, `installFakeGhForSync` from `sync_test.go:47`).
**Target Platform**: Linux/macOS developer workstations and CI; identical to existing test surface. No platform-specific behavior added.
**Project Type**: Go single-project CLI (`cmd/` + `internal/`). Tests live next to the package under test.
**Performance Goals**: New tests collectively add ≤ 200ms wall-clock to `make test` (SC-003). Each new case targets ≤ 50ms by reusing `newTestRepo` + `installFakeGhForSync` and stopping at the commit gate (no push, no PR-creation simulation).
**Constraints**: `make ci` (= `fmt-check vet lint test`) MUST pass. The merge commit subject MUST start with `fix(sync):` (Conventional Commits + release-please mapping). `go.mod` and `go.sum` MUST be unchanged (SC-004). Issue #48 MUST be closed in the same change set (SC-005).
**Scale/Scope**: Test surface expansion only — two new `Test*` functions (~30–50 lines each) and ~10 lines of incremental fake-gh shell. Zero non-test files modified.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The repo constitution is at `.specify/memory/constitution.md` (v1.1.0, ratified 2026-04-18, last amended 2026-05-10).

| Principle / Section | Outcome | Justification |
|---|---|---|
| I. Thin-Orchestrator Code Quality | ✅ PASS | No production code changes. Tests exercise the existing orchestration path. The fake-gh shim continues to *stub*, not *re-implement*, `gh aw add`. |
| II. Testing Standards (Build-Green + Real-World Dry-Run) | ✅ PASS | This slice IS adding tests; `make ci` must stay green per FR-008 + SC-001. Constitution Principle II also notes "Unit tests are not currently required" — i.e., adding them is allowed and welcome; no policy conflict. |
| III. UX Consistency (Three-Turn Mutation Pattern) | ✅ PASS | N/A — no CLI surface change, no new flag, no new error message, no help text edit. The fix being pinned is already in the shipped binary's behavior. |
| IV. Performance (5-Minute Ceiling, Parallelism, Cache) | ✅ PASS | New tests cap at ~200ms total (SC-003). No new commands, no new network calls, no new fetched state. |
| Declarative Reconcile Invariants | ✅ PASS | No change to `fleet.json` semantics, no change to commit-signing posture, no `git add`/`git commit`/`git push` invoked from anywhere new — the existing `Deploy` path (the one legitimate `exec.Command("git", ...)` site) is unchanged; the fake `gh` shim doesn't drive git directly because tests stop at the commit gate. |
| Third-Party Dependencies | ✅ PASS | Zero new direct deps. `go.mod` / `go.sum` unchanged (SC-004). |
| Development Workflow | ✅ PASS | One PR, `fix(sync):` Conventional Commits subject, release-please will categorize under "Fixed". Issue closure noted in `applyComplete` step (per FR-009). |

**Result**: No violations. Complexity Tracking table below is empty.

## Project Structure

### Documentation (this feature)

```text
specs/008-fix-sync-resume-guard/
├── plan.md              # This file (/speckit-plan command output)
├── spec.md              # Feature specification (already authored)
├── research.md          # Phase 0 output — fake-gh shim extension decision
├── data-model.md        # Phase 1 output — existing entity touchpoints (no new entities)
├── quickstart.md        # Phase 1 output — how to run the new tests locally
└── checklists/          # /speckit-checklist artifacts (pre-existing)
```

`contracts/` is **intentionally omitted**. The feature changes no external interface — no new CLI flag, no new JSON envelope field, no new exported Go API. Per the speckit plan template guidance ("Skip if project is purely internal"), this slice qualifies because the *feature scope* is internal even though the *project* has external CLI contracts that remain unchanged.

### Source Code (repository root)

```text
internal/fleet/
├── deploy.go              # UNCHANGED — InternalClone field & resume guard already shipped (PR #66)
├── deploy_test.go         # UNCHANGED — provides newTestRepo helper consumed by sync_test.go
├── sync.go                # UNCHANGED — applyDeployOrPrune and runPreflight already pass InternalClone=true
└── sync_test.go           # MODIFIED — extend installFakeGhForSync to record aw-add calls;
                            # add TestSyncApplyBypassesResumeGuard and
                            # TestSyncApplyPruneBypassesResumeGuard
```

No files added; one file modified. No `go.mod` / `go.sum` touched.

**Structure Decision**: Single Go project, tests co-located with the package under test (`internal/fleet/`). The only file modified is `internal/fleet/sync_test.go`; the rest of the repo is untouched. This is the smallest possible footprint that satisfies FR-008 (a, b, c) and SC-001/SC-002.

## Phase 0 — Research

See [research.md](./research.md) for the full record. Single decision captured:

- **How to extend `installFakeGhForSync` to support the new test assertions** — chosen approach: append-only invocation log written by the fake `gh` shell script to a file inside `binDir` (`$FAKE_GH_LOG`), read back from Go via `os.ReadFile`. Rejected alternatives: replacing the shim with a Go-level harness (over-engineering for two tests), exposing a counter via env-var arithmetic in the shim (fragile under `set -eu`), upgrading to a per-call JSON record (test depth doesn't need structured data — order + count of `aw add` invocations is enough).

No `NEEDS CLARIFICATION` markers remain. The spec's Session 2026-05-12 already resolved the only two open questions (test depth → "stop at commit gate"; commit type → `fix(sync):`).

## Phase 1 — Design Artifacts

### data-model.md

See [data-model.md](./data-model.md). Summary: the spec's Key Entities are all pre-existing fields. No new types, no new fields, no JSON envelope changes. The document catalogs which existing identifiers each new test asserts against and notes the unchanged godoc on `DeployOpts.InternalClone` per SC-006.

### contracts/

Omitted (rationale above).

### quickstart.md

See [quickstart.md](./quickstart.md). Reproduction recipe is a single `go test -run TestSync ./internal/fleet/...` invocation plus the full `make ci` gate. Local-loop time target: < 5 seconds for the targeted run, < 60 seconds for the full suite (SC-003).

### Agent context update

CLAUDE.md's `<!-- SPECKIT START -->`/`<!-- SPECKIT END -->` block is updated to point to this plan file (`specs/008-fix-sync-resume-guard/plan.md`).

## Re-evaluation of Constitution Check (post-design)

Re-checked after Phase 1: no gates flipped. The Phase 0/1 outputs do not introduce any production code, any dependency, any schema field, or any CLI surface. All seven principle/section rows above remain ✅ PASS. **Result**: cleared for `/speckit-tasks`.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

(empty — no violations)
