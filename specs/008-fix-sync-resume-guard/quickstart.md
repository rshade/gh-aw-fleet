# Quickstart — Sync Resume-Guard Regression Coverage (Issue #48)

**Branch**: `008-fix-sync-resume-guard`
**Date**: 2026-05-12

## What this slice does

Pins the existing `DeployOpts.InternalClone` fix (PR #66) with two new regression tests in `internal/fleet/sync_test.go`. No production code change.

## Prereqs

- Go 1.25.8+ (matches `go.mod`).
- `git` on `$PATH` (the test creates real temp git repos via `newTestRepo`).
- The repo cloned at `008-fix-sync-resume-guard`.

No `gh` CLI install required for the tests — `installFakeGhForSync` writes a POSIX shell script to a temp dir and prepends it to `$PATH`.

## Run just the new tests

After `/speckit-tasks` lands the test bodies:

```bash
go test -run TestSync ./internal/fleet/...
```

Expected: three tests pass.

- `TestSyncDryRunPreflightTreatsPreparedCloneAsInternal` (pre-existing — must keep passing)
- `TestSyncApplyBypassesResumeGuard` (new — US2 / FR-008b)
- `TestSyncApplyPruneBypassesResumeGuard` (new — US3 / FR-008c)

Wall-clock target: under 5 seconds for the targeted run (SC-003 budget for the new cases is +200ms cumulative on top of the existing suite).

## Run the full CI gate

```bash
make ci
```

This runs `fmt-check vet lint test` per AGENTS.md. **All four must pass before claiming done.**

- `make fmt-check` — gofmt-aligned (no `\t`/space mismatch).
- `make vet` — `go vet ./...` clean.
- `make lint` — `golangci-lint` clean. Note from AGENTS.md: can run > 5 min locally; use extended timeouts; do **not** skip. `revive`/`staticcheck` will reject any missing exported-comment regression on `DeployOpts.InternalClone` per SC-006.
- `make test` — full suite passes, including the new cases.

## Manual smoke (optional, against a real repo)

Once `make ci` is green, the historical reproduction recipe from issue #48 should now produce a clean dry-run instead of the "refusing to resume" fatal. Example against any repo with `Missing > 0`:

```bash
go run . sync rshade/finfocus           # dry-run; expect 0 exit, no "refusing to resume"
go run . sync --apply rshade/finfocus   # apply; expect normal deploy pipeline (clone → branch → aw add → commit → push → PR)
```

This is **not** required for the slice to ship — the regression tests are the durable proof. The manual smoke is here only if a reviewer wants extra confidence before merging.

## Closing the issue

Per FR-009 and SC-005:

1. Merge the PR with a `fix(sync):` Conventional Commits subject (e.g., `fix(sync): close resume-guard regression coverage gap (#48)`).
2. release-please will pick up `fix:` and put the entry under "Fixed" in `CHANGELOG.md` on the next release PR.
3. Close issue #48 with a comment linking to the merge commit and identifying PR #66 as the original field-level fix.

## Troubleshooting

- **`FAKE_GH_LOG` unset / log file missing**: the shim uses `${FAKE_GH_LOG:?}` so this fails loudly. Check `t.Setenv("FAKE_GH_LOG", ...)` was called before `Sync`.
- **`aw add` count mismatch**: confirm the fake `gh` script's `add` branch appends to `$FAKE_GH_LOG` before `exit 0`, not after (after-exit code is unreachable).
- **`make lint` complaining about a comment on `DeployOpts.InternalClone`**: nothing in this slice should touch that field. If the lint regression appears, re-read PR #66's diff — the godoc must remain on the existing field definition at `internal/fleet/deploy.go:42`.
