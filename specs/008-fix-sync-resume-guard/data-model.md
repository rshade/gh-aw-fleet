# Phase 1 — Data Model

**Feature**: Sync Resume-Guard Regression Coverage (Issue #48)
**Branch**: `008-fix-sync-resume-guard`
**Date**: 2026-05-12

## Summary

**No new entities, no new fields, no schema changes.** This is a test-coverage slice; every identifier under test is pre-existing. This document catalogs the existing entities the new tests assert against and pins the godoc that SC-006 promises stays accurate.

## Entities under test (all pre-existing)

### `DeployOpts.InternalClone` *(internal-only signal)*

- **Location**: `internal/fleet/deploy.go:42`
- **Type**: `bool`
- **Origin**: PR #66 (commit `0694ae9`, merged 2026-04-30).
- **Contract**: `true` when the caller (i.e., `Sync`) prepared the `WorkDir` clone in this process and the resume guard should not fire; `false` (zero value) when the operator supplied `--work-dir <path>` and a genuine resume attempt should be detected. The guard at `deploy.go:203` is `opts.WorkDir != "" && !opts.InternalClone`.
- **Godoc invariant (SC-006)**: The existing comment — `// WorkDir was prepared by this process; not a user resume request.` — accurately captures the disambiguator. `make lint` (revive + staticcheck under the no-suppression `.golangci.yml` policy per AGENTS.md) MUST stay clean post-change. No edit planned.

### `SyncOpts.WorkDir` *(operator-facing flag value)*

- **Location**: `internal/fleet/sync.go:19`
- **Type**: `string`
- **Contract**: Empty when the operator omits `--work-dir` (common case → Sync prepares a `/tmp/gh-aw-fleet-*` clone and sets `InternalClone=true` on the downstream `DeployOpts`). Non-empty when the operator passes `--work-dir <path>` (Sync re-uses the directory and sets `InternalClone=false` so a genuine resume is detectable).
- **Wiring sites** (the two-line propagation contract): `sync.go:158` (`applyDeployOrPrune`) and `sync.go:206` (`runPreflight`), both `InternalClone: opts.WorkDir == ""`.

### `SyncResult` fields the tests assert against

| Field | Source line | When populated | Assertion used by |
|---|---|---|---|
| `Missing []string` | `sync.go:28` | After `computeDriftAndMissing` (`sync.go:63`) when the on-disk dir lacks a desired workflow. | All three tests (US1/US2/US3) — sanity check that the test fixture set up the missing condition correctly. |
| `Drift []string` | `sync.go:29` | After `computeDriftAndMissing` when an on-disk `.md` is not in the desired set and not extra/`copilot-setup-steps`. | US3 only — confirms the drift seed produced the expected drift entry. |
| `Pruned []string` | `sync.go:31` | Each entry appended in `pruneDriftFiles` (`sync.go:172`) after `os.Remove`. | US3 only — proves prune ran before Deploy under combined-flag path. |
| `Deploy *DeployResult` | `sync.go:30` | Set by `applyDeployOrPrune` (`sync.go:161`) only when `Apply=true && len(Missing)>0`. Stays nil for prune-only or already-in-state repos. | US2 + US3 — `res.Deploy.Added` is the proof-of-life that `Deploy` ran past `handleWorkDirResume`. |
| `DeployPreflight *DeployResult` | `sync.go:32` | Set by `runPreflight` (`sync.go:209`) only when `Apply=false && len(Missing)>0`. | US1 — already covered by `TestSyncDryRunPreflightTreatsPreparedCloneAsInternal`. |

### `DeployResult.Added` *(populated by `addResolvedWorkflows`)*

- **Why it's the load-bearing assertion target**: Spec FR-008(b) defines "the fix held" as `addResolvedWorkflows` (`deploy.go:218`) executing past the resume guard. `Added` being populated is the observable signal — the test does not need to assert on `MissingSecret`, `ActionsDisabled`, `SecurityFindings`, or any other field downstream of the guard. (Clarifications Session 2026-05-12.)

## State transitions

No new state. The behavior under test is a *guard bypass*, which from the caller's perspective is a no-op (the guard correctly does nothing) and from the test's perspective is observable only via "the next branch ran" assertions. There is no transition graph to draw.

## JSON envelope

Unchanged. `cmd.SchemaVersion` does not bump (no new fields exposed to stdout — this is a test slice, not a feature add per the cmd-envelope contract documented in AGENTS.md).

## `fleet.SchemaVersion` (on-disk config schema)

Unchanged. No new fields in `Config`, `Profile`, `RepoSpec`, or `SourcePin`.

## Test-harness data (transient, not "model" data — recorded here for completeness)

| Identifier | Type | Lifetime | Purpose |
|---|---|---|---|
| `$FAKE_GH_LOG` (env var, new) | `string` (filepath) | Single test invocation; cleaned by `t.TempDir()` | Append-only log of fake-gh events; read back by tests for count + order assertions. Decision recorded in [research.md](./research.md#decision-1--how-to-record-aw-add-invocations-in-the-fake-gh-shim). |

This is local test scaffolding, not a tool-visible identifier — it never appears in production code, in JSON output, or in any config file.
