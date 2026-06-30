# Phase 0 Research: Interactive security-finding prompt before commit

The spec carries no unresolved `[NEEDS CLARIFICATION]` markers. Phase 0 records
the implementation-placement and compatibility decisions that the issue's
implementation sketch glossed over or got wrong against the current codebase.

## Decision 1: TTY detection — stdlib `os.Stat`, not `golang.org/x/term`

**Decision**: Detect whether stdout is an interactive terminal with the stdlib
pattern `fi, _ := os.Stdout.Stat(); fi.Mode()&os.ModeCharDevice != 0`. Do **not**
add `golang.org/x/term` as a direct dependency.

**Rationale**: The issue suggests `term.IsTerminal(int(os.Stdout.Fd()))`, but a
new direct dependency in `go.mod`'s `require()` block requires a Constitution
§Third-Party Dependencies amendment (MINOR bump) and must first be evaluated
against the standard library. The standard library already suffices: `cmd/add.go`
already ships `isStdinTerminal()` using exactly the `os.Stat` + `ModeCharDevice`
approach. Reusing that pattern (applied to stdout) is constitutionally required
("evaluate the Go standard library first"), avoids the amendment, and keeps the
build's dependency surface unchanged. `golang.org/x/term` stays an indirect-only
entry in `go.sum`.

**Alternatives considered**:

- *Promote `golang.org/x/term` to a direct dep* — rejected: constitution
  amendment for zero functional gain over stdlib; `IsTerminal` is a two-line
  `ModeCharDevice` check the repo already implements.
- *Probe `os.Getenv("CI")` / `TERM`* — rejected: brittle and incomplete;
  `ModeCharDevice` is the canonical, env-independent signal.

## Decision 2: Gate on stdout, write the prompt to stdout, read from stdin

**Decision**: Key the interactivity gate on **stdout** being a character device
(per FR-014), write the one-line prompt to the `out` writer wired to `os.Stdout`,
and read the answer from `in` wired to `os.Stdin`. Suppress the prompt entirely
when the command is in `--output json` mode (treated as non-interactive).

**Rationale**: FR-014 requires detecting on the surface the question is written
to, so that `tool | tee` (stdout redirected, stdin still a TTY) does **not**
prompt — the operator cannot see a question written to a redirected stdout. This
deliberately diverges from `cmd/add.go`, which gates on stdin; the divergence is
already recorded in the spec's Assumptions. JSON mode is suppressed because a
prompt written to stdout would corrupt the JSON envelope, and a machine
consuming JSON cannot answer anyway — consistent with FR-009's "non-interactive
contexts proceed automatically" and codified as **FR-018** (JSON mode is
non-interactive even when stdout is a TTY). Operators wanting a JSON-mode block
use `--strict`.

**Alternatives considered**:

- *Gate on stdin (mirror `add`)* — rejected: FR-014 + the `tool | tee` edge case
  in the spec require the stdout surface; prompting into a redirected stdout that
  the human never sees is the exact footgun the spec calls out.
- *Write the prompt to stderr while gating on stdout* — viable and JSON-safe, but
  contradicts FR-014's "the surface the question is written to" and the issue's
  `out`-is-stdout contract; rejected to keep the gate and the write on one
  surface. JSON-mode suppression (above) closes the only corruption gap.

## Decision 3: Placement — after the strict gate, at each apply commit/push/PR boundary

**Decision**: Fire the confirmation inside the `internal/fleet` functions,
immediately after `security.Run` + the `--strict` gate, on the apply path only,
at each boundary that is about to commit/push/PR — and only when a commit is
actually pending:

- **Deploy fresh path** — after the `if !opts.Apply { return }` dry-run return
  and the `len(res.Added)==0 && !staged` no-op guard, before `createDeployPR`.
- **Deploy `--work-dir` resume** (`handleWorkDirResume`) — before the
  commit-gate `createDeployPR` call and before the push-gate `pushAndCreatePR`
  call (both already re-run the scanner + strict gate).
- **Sync prune-only path** — before `commitAndPushPrune`. Sync's *add* path
  delegates to `Deploy` (`applyDeployOrPrune` calls `Deploy` with
  `Security: opts.Security`), so it inherits Deploy's single prompt; sync must
  **not** add a second prompt there.
- **Upgrade** — before `createUpgradePR` (both the main changed-files path and
  the no-change manifest-backfill path that can still open a PR).

**Rationale**: FR-011 requires the prompt to run after `--strict` (no point
asking "proceed?" about a decision strict already made). FR-001 limits it to
apply. Asking "proceed with commit?" when nothing will be committed is noise, so
the prompt sits after the no-op guards. Placing it at the *fleet* layer (not the
`cmd` layer like `add`) is mandatory because findings are unknown until after the
clone is made and the scanner runs deep inside `Deploy`/`Sync`/`Upgrade`.

**The "prompt exactly once" guarantee** (spec edge case) is structural: sync's
add path runs through `Deploy`, which owns the single prompt; the only sync path
that commits *outside* `Deploy` is prune-only, which gets its own prompt.

**Alternatives considered**:

- *Prompt in the `cmd` layer (mirror `add`)* — impossible: the cmd layer has no
  findings until `Deploy`/`Sync`/`Upgrade` returns, which is after the commit.
- *Prompt inside `createDeployPR`* — rejected: the clone-cleanup flag
  (`cleanupClone`) needed to preserve the clone on decline is in the
  `Deploy`-function scope, not inside `createDeployPR`; and `createDeployPR` is
  also the resume commit-gate, which would double-count against the push-gate.

## Decision 4: Decline semantics — typed error, non-zero exit, preserve clone

**Decision**: On decline (including empty input and EOF/read error), return a
typed `OperatorDeclinedError` from the fleet function. The fleet layer preserves
the work-dir clone (set `cleanupClone=false`, mirroring
`preserveCloneForStrictError`; on resume paths cleanup is already disabled). The
`cmd` layer recognizes the typed error and prints a clean, actionable
"aborted by operator — re-run with --yes to skip the prompt" message, exiting
non-zero, without routing it through the hint engine as if it were a crash.

**Rationale**: `cmd/add.go` already establishes the in-repo precedent: declining
its confirmation returns `errors.New("aborted: re-run with --apply --yes to
confirm")` → non-zero exit. Mirroring that keeps the fleet's confirmation UX
consistent (spec Assumptions). A *typed* error (rather than `errors.New`) lets
`output.go` distinguish a deliberate operator decline from a real failure and
present it cleanly. Preserving the clone matches the Constitution's
failure-breadcrumb invariant and lets the operator resume with `--work-dir
<clone> --yes` instead of re-cloning.

**Alternatives considered**:

- *Exit 0 on decline* — rejected: the operator requested `--apply` and it did not
  complete; non-zero is the correct signal and matches `add`.
- *Untyped `errors.New`* — rejected: the cmd layer needs to tell a decline apart
  from a genuine error to avoid emitting misleading remediation hints.

## Decision 5: `--yes` lives on `SecurityOpts`; prompt fires on any severity

**Decision**: Add `Yes bool` to the existing `SecurityOpts` struct (beside
`Strict bool`), wire `--yes` on all three commands via
`fleet.SecurityOpts{Strict: flagStrict, Yes: flagYes}`. The prompt fires on the
presence of **any** finding regardless of severity (FR-015).

**Rationale**: `SecurityOpts` is already the grouping threaded into
`DeployOpts`/`SyncOpts`/`UpgradeOpts` and into sync's delegated `Deploy` call, so
`Yes` rides the same path that already carries `Strict` — including the
delegation that makes "prompt once" work. Any-severity triggering matches the
binding acceptance criteria ("findings present"), not the user story's
illustrative "HIGH" (spec Assumptions). The lowest-severity informational
findings therefore prompt too; a severity floor is explicitly out of scope.

**Alternatives considered**:

- *New top-level `Yes` on each Opts struct* — rejected: duplicates wiring and
  misses the sync→Deploy delegation that `SecurityOpts` already rides.
- *Prompt only on HIGH/MEDIUM* — rejected: contradicts the binding acceptance
  criteria; `--strict` already owns severity-thresholded behavior.

## Decision 6: Reuse the existing severity tally for the summary line

**Decision**: Export the existing unexported `severityTally` from
`internal/fleet/security/render.go` as `SeveritySummary(findings) string` and use
it verbatim for the prompt's one-line summary (e.g. `2 HIGH, 1 MEDIUM`).

**Rationale**: The PR-body summary already renders exactly this tally
(`**Summary**: 2 HIGH, 1 MEDIUM`). Reusing it keeps the prompt's count
consistent with the PR body and avoids a second severity-counting code path
(DRY; FR-002).

**Alternatives considered**:

- *Inline a new tally in `security_prompt.go`* — rejected: duplicates logic that
  already exists and must stay in sync with the PR-body summary.

## No-amendment / no-schema-bump confirmation

- **No new direct dependency** (Decision 1) → no Constitution amendment.
- `cmd.SchemaVersion` (JSON envelope) and `fleet.SchemaVersion` (on-disk config)
  are **unchanged**: `--yes` is invocation state, not config; the decline path is
  a failure, not a new envelope field.
- No change to scanner rules, finding ordering, stderr rendering, PR-body
  rendering, or `gh aw compile --strict` resolution.
