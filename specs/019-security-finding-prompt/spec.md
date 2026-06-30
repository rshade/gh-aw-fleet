# Feature Specification: Interactive security-finding prompt before commit

**Feature Branch**: `019-security-finding-prompt`  
**Created**: 2026-06-28  
**Status**: Draft  
**Input**: GitHub issue #40 — "Defense-in-depth UX: stderr + interactive prompt + PR body Security Findings section" (part of epic #36)

## Overview

When a fleet operator runs `deploy`, `sync`, or `upgrade` with `--apply` in an
interactive terminal and the security scanner reports findings, the tool must
**pause and ask the operator to confirm** before any commit, branch push, or
pull request is created. This is the second of three independent "defense in
depth" surfaces that put scanner findings in front of a human:

1. **Terminal output** — findings already appear on the operator's standard
   error before the run finishes (delivered by the Layer 1 scanner).
2. **Interactive confirmation** — *this feature*: a one-line, severity-summary
   `[y/N]` prompt that gives the operator a last chance to abort.
3. **Pull-request body** — findings already appear as a `## Security Findings`
   section in the generated PR description (delivered by the Layer 1 scanner),
   so a reviewer sees them too.

The premise is that a single output surface is a single chance to miss a
problem; three independent surfaces (the operator's screen, the operator's
active attention, the reviewer's attention) make a bad change far harder to let
through by accident.

This feature also adds a `--yes` skip flag to `deploy`, `sync`, and `upgrade`
that bypasses **only** the interactive prompt — it must not suppress the
terminal output or the PR-body section.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Last-chance abort before a commit lands (Priority: P1)

A fleet operator runs `gh-aw-fleet deploy <repo> --apply` from their terminal.
The scanner reports security findings. Before anything is committed or pushed,
the tool stops and asks: a short severity summary followed by `Proceed with
commit? [y/N]`. The operator can type `n` to abort with nothing changed on the
remote, or `y` to continue. When they continue, the resulting pull request
contains the `## Security Findings` section so a teammate reviewing the PR sees
the same findings.

**Why this priority**: This is the entire point of the feature — the
interactive confirmation is the only one of the three surfaces that can *stop*
a change rather than merely report it after the fact. Without it there is no
"last chance to abort." Everything else in this spec exists to support or
constrain this interaction.

**Independent Test**: In an interactive terminal, deploy a repo whose clone
contains a scanner-triggering artifact with `--apply`. Confirm the prompt
appears with a severity summary, that answering `n` produces no commit, no
branch push, and no PR, and that answering `y` produces a PR whose body
contains `## Security Findings`.

**Acceptance Scenarios**:

1. **Given** an interactive terminal and a repo with at least one finding,
   **When** the operator runs `deploy <repo> --apply` and answers `n` at the
   prompt, **Then** no commit, branch push, or PR is created and the command
   reports that it was aborted by the operator.
2. **Given** the same setup, **When** the operator answers `y`, **Then** the
   apply proceeds and the resulting PR body contains a `## Security Findings`
   section.
3. **Given** an interactive terminal and a repo with **zero** findings,
   **When** the operator runs `deploy <repo> --apply`, **Then** no
   confirmation prompt appears and the PR body contains no `## Security
   Findings` section.
4. **Given** an interactive terminal and a repo with findings, **When** the
   operator presses Enter without typing anything, **Then** the empty response
   is treated as "No" and the apply is aborted.

---

### User Story 2 - Skip the prompt without hiding the findings (Priority: P2)

An operator who has already reviewed the findings (or who is driving an apply
they have decided to trust) runs the command with `--yes`. The interactive
prompt is skipped and the apply proceeds, but the findings still appear on the
terminal and the generated PR body still contains the `## Security Findings`
section. `--yes` buys speed, not silence.

**Why this priority**: Without a documented bypass, operators who legitimately
want to skip the prompt would have no supported path and might resort to
piping output to defeat the terminal check — which would also be confusing. The
explicit `--yes` flag makes the intent clear and guarantees the other two
surfaces stay intact.

**Independent Test**: In an interactive terminal, deploy a repo with findings
using `--apply --yes`. Confirm no prompt appears, the apply proceeds, the
findings appear on standard error, and the PR body contains `## Security
Findings`.

**Acceptance Scenarios**:

1. **Given** an interactive terminal and a repo with findings, **When** the
   operator runs `deploy <repo> --apply --yes`, **Then** no prompt appears,
   the apply proceeds, and the PR body still contains `## Security Findings`.
2. **Given** the same setup, **When** the apply runs with `--yes`, **Then**
   the findings are still surfaced on standard error.
3. **Given** `deploy`, `sync`, and `upgrade`, **When** an operator inspects
   their flags, **Then** all three expose a `--yes` flag with the same
   meaning.

---

### User Story 3 - Non-interactive runs never hang (Priority: P3)

An operator (or an automation script) runs the command with output piped or
redirected, or in CI where there is no terminal. Even when findings are
present, the command must **not** stop to wait for a keypress that will never
come. It proceeds automatically, and the findings still appear on standard
error and in the PR body. Operators who want findings to *block* a
non-interactive run use the separate `--strict` gate, not this prompt.

**Why this priority**: A prompt that hangs in CI or under a pipe would be a
worse outcome than no prompt at all — it would silently stall pipelines. This
story is lower priority than P1/P2 because it largely preserves today's
behavior (deploy/sync/upgrade have no interactive prompt today), but it is a
hard correctness constraint on the P1 prompt.

**Independent Test**: Run `deploy <repo> --apply` with standard output piped to
another command, against a repo with findings. Confirm the command does not
wait for input, completes the apply, and still emits the findings on standard
error and in the PR body.

**Acceptance Scenarios**:

1. **Given** a repo with findings and standard output redirected (not a
   terminal), **When** the operator runs `deploy <repo> --apply`, **Then** no
   prompt appears, the command does not wait for input, and the apply
   proceeds.
2. **Given** the same non-interactive run, **When** it proceeds, **Then** the
   findings still appear on standard error and the PR body still contains
   `## Security Findings`.

---

### Edge Cases

- **Strict gate already blocked the run**: When `--strict` is active and the
  findings are severe enough to block, the run is aborted by the strict gate
  *before* the confirmation prompt is reached — the operator is never asked
  "proceed?" about a decision that has already been made. The prompt only
  appears for runs the strict gate let through (no `--strict`, or findings
  below the strict threshold).
- **Output redirected but input still a terminal** (`tool | tee`): Because the
  question is written to standard output and the operator cannot see it when
  output is redirected, no prompt is shown and the apply proceeds
  automatically.
- **Machine-readable output** (`--output json`): The prompt is suppressed even
  when standard output is an interactive terminal, because writing the question
  to standard output would corrupt the JSON envelope and an automated consumer
  cannot answer. The apply proceeds (findings still on standard error and in the
  PR body); a machine-readable run that must block on findings uses `--strict`.
- **Input closed / end-of-file at the prompt**: Treated as a decline (fail
  safe) — the apply is aborted rather than proceeding on an unanswered
  question.
- **Informational-only findings**: A run whose only findings are the lowest,
  purely informational severity (e.g. an unparseable advisory config) still
  triggers the prompt in an interactive terminal, because the confirmation
  fires on the presence of *any* finding. A severity floor for the prompt is
  explicitly out of scope for this version (see Out of Scope).
- **`--yes` without `--apply`**: `--yes` only governs the apply-time
  confirmation, so passing it on a dry run has no effect; the tool surfaces
  this consistently with how `--yes` already behaves on the existing `add`
  command (an informational note, not an error).
- **Multi-repo runs** (e.g. `upgrade` across several repos): Each repo's
  findings gate that repo's own commit, so the confirmation is asked per repo
  as each repo is about to be committed. Declining one repo aborts that repo;
  whether the remaining repos continue follows the command's existing
  multi-repo failure behavior.
- **`sync` delegating to `deploy`**: `sync` reuses the deploy pipeline on its
  apply path. The operator must be prompted exactly once for a given repo, not
  twice.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When an apply (`deploy`, `sync`, or `upgrade` with `--apply`)
  produces one or more security findings, is running in an interactive
  terminal, and `--yes` was not supplied, the system MUST ask the operator to
  confirm before creating any commit, branch push, or pull request.
- **FR-002**: The confirmation prompt MUST present a one-line summary of the
  findings by severity so the operator can decide without scrolling back through
  earlier output. The summary MUST list only the severities actually present,
  using the labels `HIGH` / `MEDIUM` / `LOW` / `INFO` (e.g. `2 HIGH, 1 INFO`);
  severities with a zero count are omitted.
- **FR-003**: An affirmative response (`y`, `Y`, or `yes`, case-insensitive)
  MUST be treated as "proceed." Any other non-empty response MUST be treated
  as "decline."
- **FR-004**: An empty response (the operator presses Enter with no input)
  MUST default to "decline" — the safe default is No.
- **FR-005**: When the operator declines, the system MUST NOT create any
  commit, branch push, or pull request, MUST report that the run was aborted
  by the operator with guidance on how to re-run non-interactively (re-run with
  `--yes`), and MUST exit with a non-zero status.
- **FR-006**: When the operator confirms, the apply MUST proceed and complete
  as it would have without the prompt.
- **FR-007**: When `--yes` is supplied, the system MUST skip the interactive
  prompt and proceed, without otherwise changing the apply.
- **FR-008**: `--yes` MUST NOT suppress the findings shown on standard error,
  and MUST NOT suppress the `## Security Findings` section of the generated PR
  body. It governs the interactive prompt only.
- **FR-009**: When standard output is not an interactive terminal (CI, piped,
  or redirected output), the system MUST NOT display the prompt and MUST NOT
  wait for input; it MUST proceed automatically. The findings MUST still be
  surfaced on standard error and in the PR body.
- **FR-010**: When there are zero findings, the system MUST NOT display the
  prompt and MUST NOT add a `## Security Findings` section to the PR body,
  regardless of terminal state or `--yes`.
- **FR-011**: The interactive prompt MUST run only *after* the `--strict`
  security gate. If the strict gate aborts the run, the prompt MUST NOT appear.
- **FR-012**: If reading the operator's response fails or reaches end-of-file
  before an answer arrives, the system MUST treat it as a decline (abort the
  apply rather than proceed).
- **FR-013**: A `--yes` flag with the semantics above MUST be available on
  `deploy`, `sync`, and `upgrade`, with consistent behavior across all three.
- **FR-014**: Terminal detection MUST key on the surface the question is
  written to (standard output), so that a run whose output is redirected does
  not prompt even if input happens to be a terminal. Machine-readable output
  (`--output json`) is an explicit exception that suppresses the prompt even on
  a terminal (FR-018).
- **FR-015**: The confirmation MUST fire on the presence of *any* finding,
  irrespective of severity (the prompt is not limited to high-severity
  findings).
- **FR-016**: The operator MUST be able to see the findings (at minimum the
  severity summary in the prompt) *before* the apply commits — the findings
  must not be visible only after the change has already landed. This is the
  pre-commit visibility guarantee underlying the FR-001 confirmation.
- **FR-017**: The flag's help text MUST state that `--yes` skips interactive
  confirmation and does **not** suppress findings from terminal output or the
  PR body.
- **FR-018**: When the command is producing machine-readable output (`--output
  json`), the system MUST treat the run as non-interactive and MUST NOT display
  the prompt, regardless of whether standard output is a terminal — a prompt
  written to standard output would corrupt the JSON envelope and an automated
  consumer cannot answer it. The findings MUST still be surfaced on standard
  error and in the PR body; operators who want a machine-readable run to *block*
  on findings use `--strict`.

### Key Entities

- **Security finding**: A single scanner result with a severity (HIGH /
  MEDIUM / LOW / informational), an identifying rule, an optional file and
  line location, a human-readable message, and a suggested remedy. Findings
  are produced by the existing Layer 1 scanner; this feature consumes them but
  does not define or modify them.
- **Severity summary**: An aggregate count of findings grouped by severity,
  used as the single human-readable line the confirmation prompt shows.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In an interactive terminal, an apply that produces at least one
  finding pauses for confirmation before any remote change, in 100% of runs.
- **SC-002**: When the operator declines, zero remote changes occur — no
  commit, no branch push, and no PR — in 100% of declined runs.
- **SC-003**: In non-interactive contexts (piped, redirected, CI, or `--output
  json`), an apply with findings never waits for input; it either completes or
  fails without blocking on a prompt, in 100% of runs.
- **SC-004**: When findings exist and the apply proceeds (confirmed or
  `--yes`), the resulting PR body contains the `## Security Findings` section
  in 100% of such runs.
- **SC-005**: When there are zero findings, the operator sees no new prompt and
  no empty findings section — there is no added friction relative to today's
  behavior.
- **SC-006**: `deploy`, `sync`, and `upgrade` expose identical prompt and
  `--yes` behavior, verifiable by inspecting each command's behavior under the
  same finding and terminal conditions.

## Out of Scope

- **Per-finding accept/reject**: The confirmation is whole-batch. There is no
  "skip this one finding, keep the rest" interaction in this version.
- **Rich terminal UI**: No scrollable or expandable finding list. A single
  terse severity-summary line plus the `[y/N]` prompt is sufficient.
- **A severity floor for the prompt**: The prompt fires on any finding; there
  is no option to only prompt on high-severity findings. Operators who want
  severity-gated *blocking* in non-interactive contexts use `--strict`.
- **A flag that forces the prompt in non-interactive contexts**: There is no
  "always prompt even without a terminal" mode. Non-interactive gating is the
  job of `--strict`, not this prompt.
- **Changing what the scanner detects or how findings render**: The set of
  findings, their severities, the standard-error rendering, and the PR-body
  rendering are all delivered by the Layer 1 scanner and are not modified here.
- **Changing the `--strict` gate**: This feature orders itself after the strict
  gate but does not alter strict's behavior.

## Assumptions

- **`--yes` is newly introduced on `deploy`, `sync`, and `upgrade`.** Today the
  flag exists only on the `add` command; `deploy`/`sync`/`upgrade` gate solely
  on `--apply` and have no interactive confirmation. This feature adds the flag
  and the prompt to those three commands. The issue's statement that "`--yes`
  already exists on deploy" does not match the current code and is treated as
  intent to add it.
- **Non-interactive `--apply` proceeds without requiring `--yes`.** This
  deliberately diverges from the existing `add` command, which *requires*
  `--yes` for `--apply` in a non-interactive shell. For `deploy`/`sync`/
  `upgrade`, a non-interactive apply with findings auto-proceeds (findings
  still surfaced), because CI gating is the responsibility of the separate
  `--strict` gate, not this prompt.
- **Terminal detection keys on standard output, not standard input.** This
  diverges from `add` (which checks standard input) and is chosen so that a run
  whose output is redirected — where the operator cannot see the question —
  does not prompt.
- **"Findings present" means any severity.** The user story emphasizes
  high-severity findings, but the binding acceptance criteria say "findings
  present," and the lowest-severity informational findings still trigger the
  prompt in an interactive terminal.
- **Declining exits non-zero with re-run guidance.** This mirrors the existing
  `add` command's decline behavior (which returns an error advising the
  operator to re-run with `--yes`), keeping the fleet's confirmation UX
  consistent across commands.
- **The local clone is preserved on decline.** Consistent with the existing
  invariant that apply-time clones are breadcrumbs, declining the prompt leaves
  the work-dir clone in place so the operator can re-run against it (e.g. with
  `--work-dir` and `--yes`) without re-cloning.
- **Surface 1 (standard error) and surface 3 (PR body) already exist.** The
  Layer 1 scanner already surfaces findings on standard error and in the PR
  body. This feature adds surface 2 (the prompt) and must ensure the three
  coexist — in particular it must not duplicate or contradict the existing
  standard-error output, and the operator must be able to see findings before
  the commit, not only after it.
- **Confirmation is per repo for multi-repo runs.** Findings are computed per
  repo and gate that repo's own commit, so each repo is confirmed
  independently; batch-continuation after a decline follows each command's
  existing multi-repo failure behavior.

## Dependencies

- **Layer 1 security scanner** (slice 006): supplies the findings, the
  standard-error rendering, and the PR-body `## Security Findings` section that
  this feature's prompt and `--yes` semantics wrap around.
- **Strict security gate** (slice 017): already implemented; this feature
  orders its prompt to run after the strict gate so a strict block is never
  followed by a redundant "proceed?" question.
- **Parent epic #36**: establishes the three-surface defense-in-depth model;
  this feature delivers the middle surface.
