# Implementation Plan: Interactive security-finding prompt before commit

**Branch**: `019-security-finding-prompt` | **Date**: 2026-06-28 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/019-security-finding-prompt/spec.md`

## Summary

Add an interactive `[y/N]` confirmation that fires after the Layer 1 scanner
runs and the `--strict` gate passes, but before any commit/push/PR, whenever an
apply (`deploy`/`sync`/`upgrade --apply`) produced one or more security
findings and is running at an interactive terminal. Add a `--yes` flag to
`deploy`, `sync`, and `upgrade` that skips only this prompt — it does not
suppress the findings already emitted on stderr or in the PR body. In
non-interactive contexts (piped/redirected stdout, CI, or `--output json`) the
prompt is suppressed and the apply proceeds, so nothing ever hangs; operators
who want a non-interactive *block* use the existing `--strict` gate.

The implementation is a thin operator-confirmation layer over the existing
scanner output, mirroring how slice 017 added the strict gate: extend the
existing `SecurityOpts` with a `Yes bool`, add the Cobra `--yes` flag to the
three commands, add a shared `internal/fleet/security_prompt.go` helper
(`PromptUser` plus a stdout-TTY seam built on the stdlib `os.Stat` +
`ModeCharDevice` pattern already used by `cmd/add.go`), and call the helper at
each apply-mode commit/push/PR boundary after the strict gate. No scanner
rules, finding ordering, stderr/PR rendering, output-schema versions, or
compile-strict behavior change. **No new third-party dependency** — the
`golang.org/x/term` package the issue suggested is rejected in favor of the
in-repo stdlib TTY pattern (see research.md), avoiding a Constitution
§Third-Party Dependencies amendment.

## Technical Context

**Language/Version**: Go 1.26.4 local toolchain; `go.mod` records the same floor.  
**Primary Dependencies**: Existing `github.com/spf13/cobra` v1.10.2 (flag wiring), existing `github.com/rs/zerolog` v1.35.1 (stderr warnings), existing `internal/fleet/security`; stdlib `bufio`, `errors`, `fmt`, `io`, `os`, `strings`. **No new third-party dependencies** (Constitution Principle I / §Third-Party Dependencies). `golang.org/x/term` remains an indirect-only entry in `go.sum` and is NOT promoted to a direct require.  
**Storage**: N/A — no persistent state, cache, or new on-disk artifact. On decline the existing work-dir clone is left in place as a breadcrumb (no new file written; `--strict`'s `findings.json` is unchanged and unrelated).  
**Testing**: Table-driven unit tests for `PromptUser` (the six issue cases), the severity-summary line, the stdout-TTY seam, the typed decline error, and the command flag propagation; placement tests asserting the prompt fires only on apply, only after the strict gate, and only when a commit is pending, across `deploy`/`sync`/`upgrade` using existing seams + temp dirs and an injected non-TTY stub. Full gate: `make ci` (fmt-check, vet, lint, test).  
**Target Platform**: Linux / macOS developer and CI environments running the `gh-aw-fleet` CLI.  
**Project Type**: Single Go module CLI orchestrator (`cmd/` plus `internal/fleet/...`).  
**Performance Goals**: Negligible — one O(findings) severity tally and, only in the interactive-with-findings case, one line read from stdin. No network, no extra scanner pass, no parallelism impact.  
**Constraints**: Prompt is per-invocation and per-repo; never persisted to fleet config; fires only on `--apply`, only after the `--strict` gate, and only when a commit is actually pending; suppressed when stdout is not a terminal and when `--yes` is set; declining aborts before any commit/push/PR, exits non-zero with re-run guidance, and preserves the clone; `cmd.SchemaVersion` and `fleet.SchemaVersion` do not change; `--yes` does not alter stderr or PR-body findings output.  
**Scale/Scope**: Per-repo confirmation over typical fleets of ≤10 repos and ≤20 workflows each. Multi-repo `upgrade --all` confirms per repo as each repo reaches its commit; batch continuation after a decline follows the command's existing fail-fast behavior.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against Constitution v1.3.0.

| Principle | Verdict | Notes |
|-----------|---------|-------|
| **I. Thin-Orchestrator Code Quality** | PASS | No workflow-markdown rewriting and no re-implemented detection. Adds one small operator-confirmation helper over the existing `security.Finding` slice plus a stdlib TTY check. New exported identifiers (`SecurityOpts.Yes`, `PromptUser`, the decline error, `security.SeveritySummary`) get godoc. Files stay well under the 300-line guidance. |
| **II. Testing Standards** | PASS | `PromptUser` is pure enough for table-driven unit tests via injected `io.Reader`/`io.Writer` and a TTY stub. Command placement is covered with existing seams and dry-run mode; no live PR creation needed. `make ci` required before done; any live `--apply` validation remains user-approved only. |
| **III. User Experience Consistency** | PASS | Strengthens the three-turn mutation pattern: dry-run stays the default and unprompted; the new prompt is an *additional* explicit gate before mutation, not a bypass. Decline routes an actionable "re-run with --yes" message; the clone is preserved on decline per the breadcrumb convention. No commit-message/PR-title format changes. |
| **IV. Performance Requirements** | PASS | One in-memory tally over findings and, only in the interactive case, one stdin read. No network, no cache change, no new scanner pass. |
| **Declarative Reconcile Invariants** | PASS | `--yes`/prompt are invocation state, never written to `fleet.json`/`fleet.local.json`. No signing bypass; git is still only invoked by the Go tool inside Deploy/Sync/Upgrade. |
| **Third-Party Dependencies** | PASS | **No new direct dependency.** The issue's suggested `golang.org/x/term` is rejected for the stdlib `os.Stat`+`ModeCharDevice` pattern already in `cmd/add.go`, so no amendment is required. |
| **Documentation Impact** | PASS | User-facing `--yes` flag added and apply-time behavior changes when findings are present, so README, the Starlight reconcile/security docs, and the relevant operator skills must be updated (see below). |

**Result**: All gates pass. No Complexity Tracking entries required.

## Documentation Impact

*GATE: Record which user-facing documentation surfaces this feature touches.*

- **Surfaces touched**: Cobra help for `deploy`, `sync`, and `upgrade` (new `--yes` flag); `README.md` reconcile/quickstart usage; `docs/src/content/docs/reconcile.md` (and the security/scanner doc page if one exists) to describe the prompt + `--yes` + the three-surface model; `skills/fleet-deploy/SKILL.md` and `skills/fleet-upgrade-review/SKILL.md` (the three-turn flow now includes an in-tool confirmation, and the `--yes` bypass must be shown alongside `--apply`).
- **Update planned**: yes. The feature adds a visible CLI flag and changes apply-time behavior when findings are present; per the constitution's Development Workflow rule the human-facing docs and the affected skills must be updated in the same change.
- **Hidden surfaces**: none. The hidden `__schema` command will expose `--yes` automatically through the existing schema generator; it remains deliberately undocumented.

## Project Structure

### Documentation (this feature)

```text
specs/019-security-finding-prompt/
├── plan.md                       # This file (/speckit-plan output)
├── spec.md                       # Feature spec (/speckit-specify output)
├── research.md                   # Phase 0 output
├── data-model.md                 # Phase 1 output
├── quickstart.md                 # Phase 1 output
├── contracts/
│   └── security-prompt-contract.md   # Phase 1 output
├── checklists/
│   └── requirements.md           # /speckit-specify output
└── tasks.md                      # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
cmd/
├── deploy.go               # add --yes flag; pass into fleet.SecurityOpts{Strict, Yes}
├── deploy_test.go          # --yes flag/help registration + decline-exit surface coverage
├── sync.go                 # add --yes flag; thread into SecurityOpts
├── sync_test.go            # sync --yes propagation coverage
├── upgrade.go              # add --yes flag; thread into SecurityOpts (incl. --all)
├── upgrade_test.go         # single-repo and --all --yes coverage
├── findings.go             # unchanged; stderr warning emission stays shared (surface 1)
└── output.go               # map the typed decline error to a clean non-zero failure (no hint spam)

internal/fleet/
├── security_prompt.go      # NEW: PromptUser, stdoutIsTerminal seam, OperatorDeclinedError,
│                           #      confirmSecurityFindings wrapper (preserves clone on decline)
├── security_prompt_test.go # NEW: six PromptUser cases, TTY-stub, summary line, decline error
├── security_gate.go        # SecurityOpts gains `Yes bool` (sits beside `Strict bool`)
├── deploy.go               # call confirm after strict gate / before createDeployPR (fresh);
│                           #   before resume commit-gate + push-gate PR calls (handleWorkDirResume)
├── deploy_test.go          # prompt fires only on apply, after strict, before commit; decline aborts
├── sync.go                 # prune-only path: confirm before commitAndPushPrune (add path inherits Deploy's)
├── sync_test.go            # delegated-add prompts once (via Deploy); prune-only prompts; clean path silent
├── upgrade.go              # confirm before createUpgradePR (main + no-change manifest backfill)
└── upgrade_test.go         # dry-run no prompt; apply prompts; --all decline halts that repo

internal/fleet/security/
└── render.go               # export SeveritySummary(findings) wrapping the existing severityTally

README.md                            # --yes flag + three-surface UX
docs/src/content/docs/reconcile.md   # prompt + --yes + non-interactive behavior
skills/fleet-deploy/SKILL.md         # in-tool confirmation step + --yes bypass
skills/fleet-upgrade-review/SKILL.md # same for upgrade
CLAUDE.md / AGENTS.md                # managed Spec Kit plan reference (SPECKIT markers)
```

**Structure Decision**: Keep the confirmation in `internal/fleet`
(`security_prompt.go`), not in `internal/fleet/security`, exactly as slice 017
kept the strict gate in `internal/fleet/security_gate.go`. The `security`
package means "produce findings"; the `fleet` package decides what a command
invocation *does* with them — render, block (`--strict`), or confirm (this
feature). The only `security`-package change is exporting the existing
severity-tally helper for the prompt's one-line summary.

## Phase 0: Outline & Research

The spec contains no unresolved `NEEDS CLARIFICATION` markers. Phase 0 resolves
the placement and compatibility decisions the issue's sketch left open or got
wrong against the current code — most notably rejecting the suggested
`golang.org/x/term` dependency in favor of the in-repo stdlib TTY pattern. See
[research.md](./research.md).

## Phase 1: Design & Contracts

- [data-model.md](./data-model.md) defines the `SecurityOpts.Yes` field,
  `PromptUser`, the `stdoutIsTerminal` seam, `OperatorDeclinedError`, the
  `confirmSecurityFindings` wrapper, and the exported `SeveritySummary` helper —
  all consuming the existing `security.Finding` shape unchanged.
- [contracts/security-prompt-contract.md](./contracts/security-prompt-contract.md)
  defines the `--yes` flag, the six-row `PromptUser` decision table, the
  interactivity gate, the decline/abort behavior, surface coexistence, and the
  per-command placement (after the strict gate, apply-only, commit-pending).
- [quickstart.md](./quickstart.md) lists the offline unit/placement scenarios and
  the user-approved live `--apply` validation.
- Agent context: the managed Spec Kit block in `CLAUDE.md` and `AGENTS.md` is
  retargeted to this plan.

## Post-Design Constitution Check

All Phase 1 artifacts preserve the pre-design decisions:

| Principle | Verdict | Notes |
|-----------|---------|-------|
| **I. Thin-Orchestrator Code Quality** | PASS | The contract keeps detection in `security` and confirmation policy in `fleet`; the only new abstractions are one pure function, one seam, one typed error, and one exported tally wrapper. |
| **II. Testing Standards** | PASS | Quickstart specifies table-driven unit tests plus offline placement tests and `make ci`; live `--apply` stays user-approved. |
| **III. User Experience Consistency** | PASS | The prompt adds an explicit pre-mutation gate; decline preserves the clone and emits an actionable re-run message; dry-run stays unprompted. |
| **IV. Performance Requirements** | PASS | One tally + at most one stdin read; no network or extra scanner pass. |
| **Third-Party Dependencies** | PASS | No new direct dependency — stdlib TTY check confirmed in research.md. |
| **Documentation Impact** | PASS | README, Starlight reconcile/security docs, and the deploy/upgrade skills are listed for update. |

## Complexity Tracking

> No constitution violations or justified complexity exceptions.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| (none) | — | — |
