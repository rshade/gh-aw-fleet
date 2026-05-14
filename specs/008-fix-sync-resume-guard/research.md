# Phase 0 — Research

**Feature**: Sync Resume-Guard Regression Coverage (Issue #48)
**Branch**: `008-fix-sync-resume-guard`
**Date**: 2026-05-12

## Scope

The implementation already exists. The only open design question is how the new tests (apply and apply+prune) observe `aw add` invocations and their ordering relative to staged prune deletions. The spec's Session 2026-05-12 clarification fixed test depth ("stop at commit gate") and commit type (`fix(sync):`), so no `NEEDS CLARIFICATION` markers were carried into this phase.

## Decision 1 — How to record `aw add` invocations in the fake `gh` shim

### Decision

Extend `installFakeGhForSync` so the shell shim **appends** one line per significant invocation (`aw init`, `aw add <spec>`) to a log file whose path is exported via `$FAKE_GH_LOG`. The Go test creates the log path under `t.TempDir()`, sets the env var alongside `FLEET_TEST_REMOTE` and `PATH`, and reads the file back with `os.ReadFile` after `Sync` returns.

Log format (one event per line, space-separated):

```text
init
add githubnext/agentics/ci-doctor@v1.0.0
```

This gives both **count** (line count where the first field is `add`) and **order** (line index relative to other events the test cares about — specifically the `git add` Sync.go performs at `pruneDriftFiles` time, which the prune test will observe via a sibling `git` shim entry or by inspecting the workflows dir state at known points).

### Rationale

- **`set -eu` safe**: the shim already runs under `set -eu`; appending to a file (`printf >> "$FAKE_GH_LOG"`) cannot fail in a way that breaks the strict mode contract (and if the var is unset, `${FAKE_GH_LOG:?}` errors cleanly with a useful message).
- **Test depth match**: spec FR-008(b) requires "the fake `gh` shim recorded one `aw add` invocation per missing workflow." A line-per-call log is the minimum data shape that supports both the count assertion and the prune+add ordering assertion in FR-008(c). Anything richer (per-call JSON, separate stdout/stderr capture) would exceed the documented test depth.
- **Reuses existing infrastructure**: the shim already lives in `installFakeGhForSync` (sync_test.go:47); adding a `printf >>` line is a one-token edit. No new helper functions, no new Go-level harness.
- **Single read site**: `os.ReadFile` + `strings.Split(..., "\n")` is the same pattern `deploy_test.go` uses for inspecting captured output; reviewers will recognize it.

### Alternatives considered

| Alternative | Why rejected |
|---|---|
| Replace the shim with a Go-level test harness that intercepts `exec.Command` | Over-engineered for two tests. Would require either an injectable command runner in `Deploy` (production-code surface change for test convenience — Principle I says no) or a vendored `os/exec` wrapper. Cost vastly exceeds value at this scope. |
| Counter env var (`COUNT=$((COUNT+1)); export COUNT`) | Fragile under `set -eu` if the var isn't pre-seeded; export-to-parent doesn't survive subshell boundaries in some shells; and the prune-ordering assertion (FR-008c) wants more than a count. |
| One-file-per-call (`touch $TMP/aw-add-$N`) | Awkward to enumerate (need `ls | sort -n`) and adds filesystem noise; no benefit over a single append-only log. |
| Per-call JSON record (`printf '{"cmd":"add",...}\n'`) | Test depth does not need structured data; line-based parsing is shorter and faster to write/read for the two cases we add. |

### Implementation notes (deferred to `/speckit-tasks`)

- Pre-create the log file path in Go via `filepath.Join(binDir, "fake-gh.log")` and `t.Setenv("FAKE_GH_LOG", logPath)`.
- In the shim, after the existing branch decision but before `exit 0`, append the canonical event line. Keep `set -eu` and `${FAKE_GH_LOG:?}` so a missing setenv fails the test loudly instead of producing a silent-pass false positive.
- Tests assert via `strings.Count(string(log), "\nadd ")` for counts and `strings.Index` for ordering.

## Decision 2 — Where the negative-case coverage for `--work-dir` (FR-005) lives

### Decision

Cover FR-005 (operator-supplied `--work-dir` keeps `InternalClone=false` and the resume guard active) in `internal/fleet/deploy_test.go`, not `sync_test.go`. Spec FR-008 explicitly authorizes this placement: "Coverage for the negative case (operator-supplied `--work-dir` preserves the guard, FR-005) is desirable but may live in `deploy_test.go` if more natural there."

### Rationale

- The behavior under test is a `Deploy`-side branch (`opts.WorkDir != "" && !opts.InternalClone` at deploy.go:203). The closest existing test neighborhood is `deploy_test.go`'s resume-related cases, not `sync_test.go`'s orchestration cases.
- Sync-side wiring for FR-005 is mechanical (`InternalClone: opts.WorkDir == ""` at sync.go:158 and sync.go:206) — a unit test of `Sync` would be testing two-line plumbing rather than the actual guard contract.

### Alternatives considered

- Put it in `sync_test.go` with an operator-supplied `WorkDir` — possible but couples a Sync test to a Deploy-side branch, increasing setup cost (need a clone on a non-default branch with staged changes) for no extra signal beyond a deploy_test.go case would already provide.

### Scope note

Whether to actually add a new `deploy_test.go` case is a *task-time* decision (see `/speckit-tasks`). If equivalent coverage already exists in `deploy_test.go` (around the `handleWorkDirResume` cases), the new test may be marked redundant. If it doesn't exist, `/speckit-tasks` will emit a task for it. Either way, the resume-guard regression closure (the load-bearing deliverable) is the apply and apply+prune cases in `sync_test.go`.

## Open Questions

None. All `NEEDS CLARIFICATION` are resolved.
