# Implementation Plan: `add <owner/repo>` Subcommand for Fleet Onboarding

**Branch**: `001-add-subcommand` | **Date**: 2026-04-19 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/001-add-subcommand/spec.md`

## Summary

Implement the `add <owner/repo>` subcommand so operators can register a
new repo in `fleet.local.json` from the CLI instead of hand-editing
JSON. The command is dry-run by default (prints a preview of the
resolved workflow set) and writes the file only when both `--apply` and
confirmation (`--yes` or interactive prompt) are provided. Validation
runs the candidate through the existing `ResolveRepoWorkflows` resolver
before any write, so the preview shown to the operator is what `deploy`
will actually install. Shared state (profiles, defaults) stays
authoritative in `fleet.json`; `fleet.local.json` is written as a
minimal delta so that `mergeConfigs` at load time produces the correct
merged view.

## Technical Context

**Language/Version**: Go 1.25.8 (from `go.mod`)
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (CLI
framework — already used by every other subcommand), `encoding/json`
(stdlib — existing write path uses it via `writeJSON`)
**Storage**: `fleet.local.json` (the private source of truth — target
for all writes from this command) and `fleet.json` (public example,
read-only from this command's perspective). Both are JSON files at the
repo working dir, resolved by `LoadConfig(dir)`.
**Testing**: Go stdlib `testing` package (matches the existing style in
`internal/fleet/deploy_test.go`). Pure-logic unit tests for `Add()`,
`parseExtraWorkflowSpec()`, `validateSlug()`; no subprocess or network
mocks required — this command does no I/O beyond reading/writing local
files.
**Target Platform**: Linux / macOS developer shells (WSL2 tested by
the repo owner; macOS supported). No Windows build target. CLI-only.
**Project Type**: Single-project Go CLI. No frontend/backend split.
**Performance Goals**: ≤2 s for dry-run and `--apply` (SC-006). No
network calls. Load + resolve + write should be well under 100 ms on a
typical fleet (≤50 repos, ≤20 workflows per profile).
**Constraints**:
- No mutation of `fleet.json` under any circumstance (FR-014).
- No subprocess calls (no `gh`, no `git`, no `gh aw`) — writes stay
  local.
- Atomic writes via temp-file + rename (existing `writeJSON`
  pattern).
- `--apply` without `--yes` in a non-TTY MUST fail; never silently
  write (FR-003).

**Scale/Scope**: Single-operator, single-repo-per-invocation. Fleets
observed in this project top out at ~10 repos × ~20 workflows; 100×
that is still fine in memory.

All NEEDS CLARIFICATION from earlier drafts resolved via
`/speckit.clarify` sessions — see `## Clarifications` in `spec.md`
(FR-006, FR-008, FR-013, FR-015).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` v1.0.0
(2026-04-18). Each principle's applicability and compliance stance
for this feature:

### I. Thin-Orchestrator Code Quality — PASS

- `add` does not wrap any upstream tool; it is pure local config
  manipulation. This is an intentional carve-out: the principle says
  the tool orchestrates `gh aw` / `gh` / `git` rather than
  reimplementing them; it does NOT forbid local config logic. `add`
  sits at the same layer as `LoadConfig`, which is also pure Go.
- The command delegates workflow resolution to the existing
  `ResolveRepoWorkflows` function — no re-implementation of profile
  merging, source layout handling, or exclusion logic.
- `go build ./...` and `go vet ./...` clean is a shipping requirement
  (enforced by the existing CLAUDE.md hook into every task's
  definition-of-done).
- File size: the new `internal/fleet/add.go` and `cmd/add.go` will
  each stay well under the 300-line soft ceiling. Estimated ~150 LOC
  and ~80 LOC respectively.

### II. Testing Standards (Build-Green + Real-World Dry-Run) — PASS

- `go build ./...` and `go vet ./...` clean: enforced.
- The "real-world dry-run" clause applies to **mutating commands that
  touch external state** (`deploy`, `sync`, `upgrade`). `add` mutates
  only a local file; its equivalent of "real-world dry-run" is the
  dry-run preview it prints before any write, which exercises the
  exact `ResolveRepoWorkflows` code path `deploy` will use. This
  satisfies the spirit of the rule (dry-run exercises real logic
  against real state) without requiring a scratch clone.
- Unit tests are appropriate here because the logic is pure: config in
  → config out. Tests land in `internal/fleet/add_test.go` following
  the table-driven style already used in `deploy_test.go`.
- No skills need to be re-exercised in subagents beyond the
  `fleet-onboard-repo` update called out in FR-019.

### III. User Experience Consistency (Three-Turn Mutation Pattern) — PASS

- Dry-run by default (FR-002): Turn 1.
- `--apply` requires `--yes` or interactive approval (FR-003): Turn 2
  is the confirmation step; non-TTY enforces explicit `--yes`, which
  matches `deploy`/`sync`/`upgrade`'s convention.
- Mutation (write to `fleet.local.json`) happens only on Turn 3
  (FR-014, FR-016).
- Error messages route through a per-error-class remediation hint
  pattern consistent with `fleet.CollectHints` (we do NOT need new
  `CollectHints` entries — `add`'s errors are parse/validation errors,
  not network/auth errors that require tooling remediation).
- Commit signing / commit messages: N/A. `add` does not run `git`.

### IV. Performance Requirements — PASS

- No network, no `exec.Command`, no parallelism required. Single
  load, single resolve, single write.
- Well under the 5-minute ceiling and the 2-second spec target
  (SC-006).

**Constitution Check outcome**: All principles PASS. No violations;
Complexity Tracking section below remains empty.

## Project Structure

### Documentation (this feature)

```text
specs/001-add-subcommand/
├── plan.md              # This file
├── research.md          # Phase 0 output (resolves deferred clarify items)
├── data-model.md        # Phase 1 output (AddOptions, AddPreview)
├── quickstart.md        # Phase 1 output (operator-facing walkthrough)
├── contracts/
│   └── cli.md           # Phase 1 output (CLI surface contract)
├── checklists/
│   └── requirements.md  # From /speckit.specify
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created here)
```

### Source Code (repository root)

```text
cmd/
├── add.go               # NEW: cobra command wiring, flag parsing, TTY detection,
│                        #      calls into fleet.Add(), renders AddPreview/AddResult
├── stubs.go             # MODIFIED: remove newAddCmd stub (keep newStatusCmd)
├── deploy.go            # UNCHANGED
├── list.go              # UNCHANGED
├── root.go              # UNCHANGED (already wires newAddCmd into root)
├── sync.go              # UNCHANGED
├── template.go          # UNCHANGED
└── upgrade.go           # UNCHANGED

internal/fleet/
├── add.go               # NEW: Add(cfg, opts) → (*AddResult, error)
│                        #      parseExtraWorkflowSpec, validateSlug, buildRepoSpec
├── add_test.go          # NEW: table-driven unit tests for Add() + helpers
├── load.go              # MODIFIED: replace SaveConfig (dead code today) with
│                        #           SaveLocalConfig targeting fleet.local.json
├── schema.go            # UNCHANGED (RepoSpec/Config/ExtraWorkflow already fit)
├── deploy.go            # UNCHANGED (EngineSecrets map consumed as-is)
├── deploy_test.go       # UNCHANGED
├── diagnostics.go       # UNCHANGED (no new hint classes needed)
├── fetch.go             # UNCHANGED
├── frontmatter.go       # UNCHANGED
├── sync.go              # UNCHANGED
└── upgrade.go           # UNCHANGED

skills/fleet-onboard-repo/
└── SKILL.md             # MODIFIED: replace JSON-edit step with gh-aw-fleet add

README.md                # MODIFIED: add `add` to the CLI surface list;
                         #          update Quickstart to show onboarding via add
```

**Structure Decision**: Single-project Go CLI — matches the existing
`cmd/` + `internal/fleet/` split. No new packages. No new top-level
directories. The `cmd/add.go` / `internal/fleet/add.go` pairing
mirrors the existing `cmd/deploy.go` / `internal/fleet/deploy.go`
pairing exactly, preserving the one-concept-per-file organization the
rest of the codebase uses.

## Complexity Tracking

> No Constitution Check violations. Section intentionally empty.
