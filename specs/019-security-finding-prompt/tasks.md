# Tasks: Interactive security-finding prompt before commit

**Input**: Design documents from `/specs/019-security-finding-prompt/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/security-prompt-contract.md](./contracts/security-prompt-contract.md), [quickstart.md](./quickstart.md)

**Tests**: Required. The spec's Testing Strategy defines table-driven unit tests for the six `PromptUser` cases plus per-command placement coverage; quickstart.md enumerates them.

**Organization**: Tasks are grouped by user story so each story is independently implementable and testable on top of the shared confirmation foundation.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label for story phases only (`[US1]`, `[US2]`, `[US3]`)
- Every task names exact repository file paths to edit or validate

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the feature context and the existing seams before editing.

- [x] T001 Review the design artifacts in specs/019-security-finding-prompt/plan.md, specs/019-security-finding-prompt/data-model.md, and specs/019-security-finding-prompt/contracts/security-prompt-contract.md
- [x] T002 Review the apply-boundary and option seams in internal/fleet/deploy.go (createDeployPR, handleWorkDirResume push/commit gates), internal/fleet/sync.go (applyDeployOrPrune, commitAndPushPrune), internal/fleet/upgrade.go (createUpgradePR, finishNoChangeUpgrade), internal/fleet/security_gate.go (SecurityOpts), internal/fleet/security/render.go (severityTally), cmd/add.go (isStdinTerminal precedent), and cmd/output.go (error mapping)
- [x] T003 [P] Review existing test patterns in internal/fleet/deploy_test.go, internal/fleet/sync_test.go, internal/fleet/upgrade_test.go, internal/fleet/strict_gate_flow_test.go, and cmd/output_test.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add the shared confirmation primitives — the prompt function, options field, typed decline error, summary export, and cmd error mapping — that every user story depends on.

**⚠️ CRITICAL**: No user story implementation should begin until this phase compiles and its focused tests pass.

- [x] T004 Export `SeveritySummary(findings []Finding) string` wrapping the existing unexported `severityTally`, with a godoc sentence, in internal/fleet/security/render.go
- [x] T005 [P] Add unit tests for `SeveritySummary` (empty → "", tally order HIGH→MEDIUM→LOW→INFO, zero-count omission) in internal/fleet/security/render_test.go
- [x] T006 Add a `Yes bool` field (godoc: skips the interactive confirmation; does not suppress stderr/PR-body findings) beside `Strict bool` on `SecurityOpts` in internal/fleet/security_gate.go
- [x] T007 Add failing table-driven unit tests for `PromptUser` covering the six contract rows plus empty-line default-No, `"Y\n"`/`"YES\n"` accept, an **INFO-only** finding set that still fires the prompt (FR-015 / the informational-only edge case — the prompt triggers on any severity, not just HIGH), and the written summary containing `SeveritySummary`, using bytes.Buffer in/out and an overridden `stdoutIsTerminal` seam, in internal/fleet/security_prompt_test.go
- [x] T008 Implement `PromptUser`, the stdlib `stdoutIsTerminal` seam (os.Stdout.Stat + ModeCharDevice), `OperatorDeclinedError` (Error/Unwrap), `IsOperatorDeclinedError`, and the `confirmSecurityFindings` wrapper (preserves the clone on decline) in internal/fleet/security_prompt.go
- [x] T009 Map `OperatorDeclinedError` to a clean non-zero failure (recognized via `IsOperatorDeclinedError`, not routed through the hint engine) in cmd/output.go, with coverage in cmd/output_test.go
- [x] T010 Run focused foundational tests for internal/fleet/security_prompt_test.go and internal/fleet/security/render_test.go from /github/go/src/github.com/rshade/gh-aw-fleet

**Checkpoint**: The shared prompt primitives compile; `PromptUser` and `SeveritySummary` tests pass; no command wiring exists yet.

---

## Phase 3: User Story 1 - Last-chance abort before a commit lands (Priority: P1) 🎯 MVP

**Goal**: An interactive `deploy`/`sync`/`upgrade --apply` with findings pauses for `[y/N]` before any commit/push/PR; `n` aborts cleanly (no remote change, clone preserved, non-zero exit); `y` proceeds and the PR body carries `## Security Findings`.

**Independent Test**: With the `stdoutIsTerminal` stub true and injected stdin, run the deploy/sync/upgrade apply paths against a fixture clone with a forced finding; `"n"` returns `*OperatorDeclinedError` and skips the PR step while preserving the clone, `"y"` proceeds, and dry-run never prompts.

### Tests for User Story 1

> Write these tests first and ensure they fail before implementation.

- [x] T011 [P] [US1] Add Deploy fresh-path placement tests (prompt only on apply, only after the strict gate, only when a commit is pending; TTY+"n" → OperatorDeclinedError, no createDeployPR, clone preserved; TTY+"y" → proceeds; zero findings → no prompt and no `## Security Findings` PR section per FR-010/SC-005; prompt reached before `createDeployPR` so findings are visible pre-commit per FR-016) in internal/fleet/deploy_test.go
- [x] T012 [P] [US1] Add Deploy `--work-dir` resume placement tests (prompt before the commit-gate createDeployPR and before the push-gate pushAndCreatePR; decline aborts before PR) in internal/fleet/deploy_test.go
- [x] T013 [P] [US1] Add sync placement tests (add path prompts exactly once via the delegated Deploy; prune-only path prompts before commitAndPushPrune; clean path prompts zero times) in internal/fleet/sync_test.go
- [x] T014 [P] [US1] Add Upgrade placement tests (apply prompts before createUpgradePR; dry-run never prompts; decline aborts before the PR; `--all` decline on one repo halts that repo and the batch continues per the existing fail-fast behavior) in internal/fleet/upgrade_test.go

### Implementation for User Story 1

- [x] T015 [US1] Call `confirmSecurityFindings` in Deploy after the strict gate and the no-op guard, before `createDeployPR` (so the findings summary is shown pre-commit per FR-016), setting `cleanupClone=false` on decline, in internal/fleet/deploy.go
- [x] T016 [US1] Call `confirmSecurityFindings` in `handleWorkDirResume` before the commit-gate `createDeployPR` and before the push-gate `pushAndCreatePR` (cleanup already disabled on resume) in internal/fleet/deploy.go
- [x] T017 [US1] Call `confirmSecurityFindings` in the sync prune-only path before `commitAndPushPrune`, and confirm the add path inherits Deploy's single prompt (no second prompt), in internal/fleet/sync.go
- [x] T018 [US1] Call `confirmSecurityFindings` in Upgrade before `createUpgradePR` on both the changed-files path and the no-change manifest-backfill path in internal/fleet/upgrade.go

**Checkpoint**: The interactive prompt gates every apply commit/push/PR boundary across deploy/sync/upgrade; decline preserves the clone and exits non-zero; the MVP is independently testable.

---

## Phase 4: User Story 2 - Skip the prompt without hiding the findings (Priority: P2)

**Goal**: `--yes` on `deploy`/`sync`/`upgrade` bypasses only the interactive prompt; the stderr findings and the PR-body `## Security Findings` section are still produced.

**Independent Test**: With `SecurityOpts.Yes=true`, findings present, and the TTY stub true, the apply proceeds with no stdin read; the stderr warnings still emit and `RenderPRSection` still returns the section.

### Tests for User Story 2

> Write these tests first and ensure they fail before implementation.

- [x] T019 [P] [US2] Add Cobra flag/help/propagation tests for `--yes` on all three commands (registered, documented help text, threaded into `fleet.SecurityOpts.Yes`) in cmd/deploy_test.go, cmd/sync_test.go, and cmd/upgrade_test.go
- [x] T020 [P] [US2] Add bypass tests proving `Yes=true` + findings + TTY stub proceeds without reading stdin (no OperatorDeclinedError) in internal/fleet/deploy_test.go
- [x] T021 [P] [US2] Add surface-coexistence tests proving `--yes` still emits stderr findings warnings (cmd) and still includes the `## Security Findings` PR section (`security.RenderPRSection`) in cmd/deploy_test.go and internal/fleet/deploy_test.go

### Implementation for User Story 2

- [x] T022 [US2] Add the `--yes` flag to newDeployCmd and set `fleet.SecurityOpts{Strict: flagStrict, Yes: flagYes}` in cmd/deploy.go
- [x] T023 [US2] Add the `--yes` flag to newSyncCmd and thread it into the SyncOpts SecurityOpts in cmd/sync.go
- [x] T024 [US2] Add the `--yes` flag to newUpgradeCmd (covering the single-repo and `--all` paths) and thread it into the UpgradeOpts SecurityOpts in cmd/upgrade.go

**Checkpoint**: `--yes` skips the prompt on all three commands while the other two surfaces remain intact; US1 and US2 both work independently.

---

## Phase 5: User Story 3 - Non-interactive runs never hang (Priority: P3)

**Goal**: A non-interactive apply with findings (piped/redirected stdout, CI, or `--output json`) auto-proceeds without waiting for input; the stderr and PR-body surfaces still appear.

**Independent Test**: With the `stdoutIsTerminal` stub false, an apply with findings proceeds with no stdin read and no decline error; in `--output json` mode the prompt is suppressed and the envelope is uncorrupted.

### Tests for User Story 3

> Write these tests first and ensure they fail before implementation.

- [x] T025 [P] [US3] Add command-level non-TTY placement tests proving an apply with findings and the stdoutIsTerminal stub false proceeds without reading stdin and without an OperatorDeclinedError in internal/fleet/deploy_test.go and internal/fleet/upgrade_test.go
- [x] T026 [P] [US3] Add `--output json` suppression tests (FR-018) proving an apply with findings (TTY stub true) emits no prompt into the JSON envelope and the envelope stays valid in cmd/output_test.go and cmd/deploy_test.go

### Implementation for User Story 3

- [x] T027 [US3] Suppress the prompt in `--output json` mode (FR-018 — treat JSON output as non-interactive even when stdout is a TTY) by passing the skip through the SecurityOpts from cmd/deploy.go, cmd/sync.go, and cmd/upgrade.go where the resolved output mode is JSON

**Checkpoint**: Non-interactive contexts never block; JSON output is never corrupted by a prompt; all three stories are independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: User-facing docs, skills, schema verification, and the full project gate.

- [x] T028 [P] Document the `--yes` flag, the interactive prompt, and the non-interactive/CI behavior in README.md
- [x] T029 [P] Document the three-surface confirmation UX (prompt + `--yes` + non-interactive) and how it differs from `--strict` in docs/src/content/docs/reconcile.md
- [x] T030 [P] Update skills/fleet-deploy/SKILL.md so the three-turn flow includes the in-tool confirmation and the `--yes` bypass alongside `--apply`
- [x] T031 [P] Update skills/fleet-upgrade-review/SKILL.md with the same confirmation/`--yes` guidance for upgrade
- [x] T032 [P] Verify `__schema` lists `--yes` for deploy/sync/upgrade without documenting the hidden command, updating expectations in cmd/schema_test.go if needed
- [x] T033 Run the offline quickstart scenarios from specs/019-security-finding-prompt/quickstart.md against the completed implementation
- [x] T034 Run `make ci` from /github/go/src/github.com/rshade/gh-aw-fleet/Makefile and fix any fmt, vet, lint, or test failures
- [x] T035 Confirm `cmd.SchemaVersion` and `fleet.SchemaVersion` are unchanged, `golang.org/x/term` is NOT promoted to a direct require in go.mod, and no `--yes` value is written to fleet.json/fleet.local.json

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup; blocks all user stories. `PromptUser` + `confirmSecurityFindings` + `SecurityOpts.Yes` must exist first.
- **User Story 1 (Phase 3)**: Depends on Foundational; MVP.
- **User Story 2 (Phase 4)**: Depends on Foundational; reuses the same boundaries US1 wired but adds the `--yes` flag. Can proceed after Phase 2, easiest after US1 establishes the wiring.
- **User Story 3 (Phase 5)**: Depends on Foundational; the non-TTY branch already exists in `PromptUser`, so this phase mostly adds command-level assertions plus JSON suppression.
- **Polish (Phase 6)**: Depends on the user stories selected for delivery.

### User Story Dependencies

- **US1 (P1)**: No dependency on US2/US3 after Phase 2. Delivers the core value.
- **US2 (P2)**: No functional dependency on US1, but the `--yes` flag governs the prompt US1 wired; their command-file edits (cmd/deploy.go, internal/fleet/deploy.go) overlap and should be coordinated.
- **US3 (P3)**: No command dependency; hardens and documents the non-interactive path of the shared `PromptUser`.

### Within Each User Story

- Tests are written first and must fail before implementation.
- Foundational `confirmSecurityFindings` exists before any boundary wiring (T015–T018) can pass placement tests.
- Command flag wiring (T022–T024) before flag/propagation tests (T019) can pass.
- Documentation and full `make ci` run only after the selected stories are complete.

### Parallel Opportunities

- T003 can run alongside T001/T002.
- T005 (render_test.go) can be drafted alongside T004 (render.go); T007 (security_prompt_test.go) alongside T008 (security_prompt.go).
- T011, T012 both edit internal/fleet/deploy_test.go — coordinate; T013 (sync_test.go) and T014 (upgrade_test.go) are independent and parallel.
- T015 and T016 both edit internal/fleet/deploy.go — sequential; T017 (sync.go) and T018 (upgrade.go) are independent and parallel with each other.
- T019 spans three cmd test files and can be split per command; T020/T021 touch deploy tests and should coordinate.
- T028, T029, T030, T031, T032 can all run in parallel after behavior is implemented.

---

## Parallel Example: User Story 1

```text
Task: "Add Deploy fresh-path placement tests ... in internal/fleet/deploy_test.go"
Task: "Add sync placement tests ... in internal/fleet/sync_test.go"
Task: "Add Upgrade placement tests ... in internal/fleet/upgrade_test.go"
```

## Parallel Example: User Story 2

```text
Task: "Add Cobra flag/help/propagation tests for --yes ... in cmd/deploy_test.go, cmd/sync_test.go, cmd/upgrade_test.go"
Task: "Add bypass tests proving Yes=true proceeds without reading stdin ... in internal/fleet/deploy_test.go"
Task: "Add surface-coexistence tests for --yes ... in cmd/deploy_test.go and internal/fleet/deploy_test.go"
```

## Parallel Example: Polish

```text
Task: "Document the --yes flag and the interactive prompt in README.md"
Task: "Document the confirmation UX in docs/src/content/docs/reconcile.md"
Task: "Update skills/fleet-deploy/SKILL.md with the confirmation step and --yes bypass"
Task: "Update skills/fleet-upgrade-review/SKILL.md with the same guidance"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup) and Phase 2 (Foundational).
2. Complete Phase 3 (US1): the interactive prompt gating every apply boundary.
3. **STOP and VALIDATE**: interactive decline blocks before any commit/push/PR and preserves the clone; accept proceeds with the PR-body section; dry-run never prompts.
4. Ship MVP with minimal docs if delivering US1 alone.

### Incremental Delivery

1. Deliver US1: the interactive last-chance abort (MVP).
2. Deliver US2: the `--yes` bypass that keeps stderr + PR-body surfaces intact.
3. Deliver US3: non-interactive robustness and JSON-mode suppression.
4. Complete Polish and run `make ci`.

### Validation Gate

Before reporting implementation complete:

1. Run focused tests for internal/fleet/security_prompt_test.go, internal/fleet/security/render_test.go, internal/fleet/deploy_test.go, internal/fleet/sync_test.go, internal/fleet/upgrade_test.go, and cmd/output_test.go.
2. Run `make ci`.
3. Confirm `cmd.SchemaVersion` / `fleet.SchemaVersion` are unchanged and no new direct dependency was added.

## Notes

- The prompt is the security confirmation only; it fires after the `--strict` gate and only on `--apply` when a commit is pending.
- "Prompt exactly once" is structural: sync's add path inherits Deploy's prompt; only sync's standalone prune commit gets its own.
- TTY detection is stdlib (os.Stdout.Stat + ModeCharDevice) — do NOT add `golang.org/x/term` as a direct dependency.
- Decline preserves the work-dir clone as a breadcrumb; resume with `--work-dir <clone> --yes`.
- Do not run live `--apply` validation (the spec's fake-secret end-to-end flow) without explicit user approval in the immediately preceding turn; dry-runs and injected-seam tests are the offline surface.
- Do not hand-edit CHANGELOG.md; release-please owns it.
