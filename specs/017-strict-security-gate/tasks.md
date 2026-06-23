# Tasks: Strict Security Gate

**Input**: Design documents from `/specs/017-strict-security-gate/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/strict-gate-contract.md](./contracts/strict-gate-contract.md), [quickstart.md](./quickstart.md)

**Tests**: Required. The feature specification defines independent tests for all three user stories, and quickstart.md requires unit and command-path coverage.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested independently after the shared strict-gate foundation is in place.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label for story phases only (`[US1]`, `[US2]`, `[US3]`)
- Every task names exact repository file paths to edit or validate

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the active feature context and current implementation seams before editing.

- [X] T001 Review strict-gate design artifacts in specs/017-strict-security-gate/plan.md, specs/017-strict-security-gate/data-model.md, and specs/017-strict-security-gate/contracts/strict-gate-contract.md
- [X] T002 Review existing scanner and mutation ordering seams in internal/fleet/deploy.go, internal/fleet/sync.go, internal/fleet/upgrade.go, cmd/deploy.go, cmd/sync.go, and cmd/upgrade.go
- [X] T003 [P] Review existing security finding output tests in cmd/output_test.go, cmd/deploy_test.go, cmd/upgrade_test.go, and internal/fleet/deploy_test.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add the shared option, predicate, error, and breadcrumb primitives required by every user story.

**CRITICAL**: No user story implementation should begin until this phase compiles.

- [X] T004 [P] Add failing unit tests for SecurityOpts default behavior, HIGH blocking count, lower-severity non-blocking behavior, breadcrumb JSON contents, and strict error text in internal/fleet/security_gate_test.go
- [X] T005 Implement SecurityOpts, StrictSecurityError, BlockingSecurityFindings, EvaluateStrictSecurityGate, and findings.json breadcrumb writing in internal/fleet/security_gate.go
- [X] T006 Add Security SecurityOpts fields with godoc-compatible comments to DeployOpts in internal/fleet/deploy.go, SyncOpts in internal/fleet/sync.go, and UpgradeOpts in internal/fleet/upgrade.go
- [X] T007 Add assertions that findings.json preserves the existing security.Finding JSON field names and numeric severity representation in internal/fleet/security_gate_test.go
- [X] T008 [P] Add strict gate preservation assertions proving strict evaluation preserves finding count, order, severity, IDs/codes, fields, and rendered breadcrumb JSON content in internal/fleet/security_gate_test.go
- [X] T009 Run focused foundational tests for the new gate helper from /github/go/src/github.com/rshade/gh-aw-fleet using internal/fleet/security_gate_test.go

**Checkpoint**: Shared gate primitives compile and focused helper tests fail/pass as expected.

---

## Phase 3: User Story 1 - Hard-gate a deploy on HIGH findings (Priority: P1)

**Goal**: `deploy --strict` and `sync --strict` block HIGH Layer 1 findings before commit, push, or PR creation, while non-strict behavior remains advisory.

**Independent Test**: Run the deploy/sync strict tests against a fixture clone with a fake HIGH finding; strict apply returns non-zero and creates no PR, while the equivalent non-strict path proceeds through existing advisory behavior.

### Tests for User Story 1

> Write these tests first and ensure they fail before implementation.

- [X] T010 [P] [US1] Add deploy strict apply tests that assert StrictSecurityError, findings.json, preserved clone, and no compile/manifest/PR step in internal/fleet/deploy_test.go
- [X] T011 [US1] Add deploy non-strict regression tests proving identical HIGH findings remain advisory in internal/fleet/deploy_test.go
- [X] T012 [P] [US1] Add clean strict no-op command tests verifying --strict with zero blocking findings follows the same success path, exit status, and mutation behavior as non-strict mode in cmd/deploy_test.go or cmd/sync_test.go
- [X] T013 [P] [US1] Add sync strict tests for missing-workflow delegation and prune-only apply blocking before commit/push in internal/fleet/sync_test.go
- [X] T014 [P] [US1] Add deploy and sync Cobra flag/help/propagation tests for --strict in cmd/deploy_test.go and cmd/sync_test.go

### Implementation for User Story 1

- [X] T015 [US1] Add --strict flag wiring to newDeployCmd and pass fleet.SecurityOpts into fleet.Deploy in cmd/deploy.go
- [X] T016 [US1] Add --strict flag wiring to newSyncCmd and pass fleet.SecurityOpts into fleet.Sync in cmd/sync.go
- [X] T017 [US1] Evaluate the strict gate in Deploy immediately after security.Run and before dry-run/apply branching in internal/fleet/deploy.go
- [X] T018 [US1] Preserve deploy dry-run temp clones when StrictSecurityError is returned by adjusting cleanup logic in internal/fleet/deploy.go
- [X] T019 [US1] Propagate SyncOpts.Security into delegated Deploy calls and evaluate strict gate before direct sync prune/return paths in internal/fleet/sync.go
- [X] T020 [US1] Ensure strict deploy/sync errors still surface existing security warnings and JSON warning diagnostics in cmd/deploy.go, cmd/sync.go, and cmd/output_test.go

**Checkpoint**: User Story 1 is independently functional for single-repo deploy/sync apply paths.

---

## Phase 4: User Story 2 - Gate without applying, for pre-merge CI (Priority: P2)

**Goal**: `upgrade --strict` blocks in dry-run mode and `upgrade --all --strict` fails fast at the first blocked repo, including JSON mode.

**Independent Test**: Run `upgrade --strict` without `--apply` against a fixture with a HIGH finding and assert non-zero exit, rendered findings, findings.json, preserved clone, and no branch/commit/PR.

### Tests for User Story 2

> Write these tests first and ensure they fail before implementation.

- [X] T021 [P] [US2] Add upgrade dry-run strict abort tests for non-zero error, findings.json, preserved clone, and unchanged compile-strict fields in internal/fleet/upgrade_test.go
- [X] T022 [US2] Add upgrade apply strict abort tests proving branch/stage/commit/push/PR steps do not run in internal/fleet/upgrade_test.go
- [X] T023 [US2] Add UpgradeAll text-mode fail-fast strict tests in internal/fleet/upgrade_test.go
- [X] T024 [P] [US2] Add upgrade --all --strict --output json tests proving NDJSON stops after the blocked repo but emits that repo envelope in cmd/output_test.go
- [X] T025 [P] [US2] Add upgrade strict abort rendering tests asserting security findings are emitted before StrictSecurityError is returned, including JSON/NDJSON output mode, in cmd/upgrade_test.go and cmd/output_test.go
- [X] T026 [P] [US2] Add upgrade --audit --strict no-op behavior coverage in cmd/upgrade_test.go

### Implementation for User Story 2

- [X] T027 [US2] Add --strict flag wiring to newUpgradeCmd and pass fleet.SecurityOpts into fleet.Upgrade and fleet.UpgradeAll in cmd/upgrade.go
- [X] T028 [US2] Evaluate the strict gate in Upgrade immediately after security.Run and before no-change, dry-run, compile-strict, manifest, and PR branches in internal/fleet/upgrade.go
- [X] T029 [US2] Preserve upgrade dry-run temp clones when StrictSecurityError is returned by adjusting cleanup logic in internal/fleet/upgrade.go
- [X] T030 [US2] Update runUpgradeAllJSON to stop after emitting the blocked repo envelope when errors.As detects StrictSecurityError in cmd/upgrade.go
- [X] T031 [US2] Keep upgrade --audit --strict behavior unchanged while ensuring the flag combination is accepted in cmd/upgrade.go and internal/fleet/upgrade.go

**Checkpoint**: User Story 2 is independently functional for dry-run upgrade and fleet-wide strict CI use.

---

## Phase 5: User Story 3 - Prompt-injection findings stay advisory (Priority: P3)

**Goal**: HIGH findings whose rule ID starts with `promptinj:` never block under `--strict`, while a HIGH Layer 1 finding in the same set still blocks.

**Independent Test**: Construct finding sets directly in unit tests: HIGH `promptinj:*` only proceeds, and HIGH `promptinj:*` plus HIGH `fleet.*` blocks with count 1.

### Tests for User Story 3

> Write these tests first and ensure they fail before implementation.

- [X] T032 [US3] Add promptinj-only HIGH non-blocking tests to internal/fleet/security_gate_test.go
- [X] T033 [US3] Add mixed promptinj HIGH plus Layer 1 HIGH blocking-count tests to internal/fleet/security_gate_test.go

### Implementation for User Story 3

- [X] T034 [US3] Implement the promptinj: prefix carve-out in BlockingSecurityFindings in internal/fleet/security_gate.go
- [X] T035 [US3] Document the promptinj: strict-gate carve-out in the SecurityOpts or BlockingSecurityFindings godoc in internal/fleet/security_gate.go

**Checkpoint**: User Story 3 is independently verified by pure unit tests and protects future Layer 3 findings from strict blocking.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, schema/help verification, and full project gate.

- [X] T036 [P] Update README.md to document deploy/sync/upgrade --strict, advisory default behavior, and findings.json strict-abort breadcrumb
- [X] T037 [P] Update docs/src/content/docs/reconcile.md to distinguish the security strict gate from gh aw compile --strict and describe dry-run/apply behavior
- [X] T038 [P] Update skills/fleet-deploy/SKILL.md with strict dry-run/apply handling, findings rendering, and breadcrumb inspection guidance
- [X] T039 [P] Update skills/fleet-upgrade-review/SKILL.md with upgrade --strict and upgrade --all --strict fail-fast guidance
- [X] T040 [P] Verify __schema includes the new --strict flags without documenting the hidden command by updating expectations in cmd/schema_test.go if needed
- [X] T041 Run quickstart validation commands from specs/017-strict-security-gate/quickstart.md against the completed implementation
- [X] T042 Run make ci from /github/go/src/github.com/rshade/gh-aw-fleet/Makefile and fix any fmt, vet, lint, or test failures

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup; blocks all user stories.
- **User Story 1 (Phase 3)**: Depends on Foundational; MVP.
- **User Story 2 (Phase 4)**: Depends on Foundational; can start after shared gate primitives exist, but easiest after US1 establishes command wiring patterns.
- **User Story 3 (Phase 5)**: Depends on Foundational; pure predicate work can proceed in parallel with US1/US2 after T005.
- **Polish (Phase 6)**: Depends on completed user stories selected for delivery.

### User Story Dependencies

- **US1 (P1)**: No dependency on US2 or US3 after Phase 2. Provides MVP value.
- **US2 (P2)**: No functional dependency on US1, but reuses the same `--strict` flag wiring and strict gate helper.
- **US3 (P3)**: No command dependency; validates and documents the shared blocking predicate.

### Within Each User Story

- Tests should be written first and fail before implementation.
- Command flag wiring before command-level propagation assertions can pass.
- Gate evaluation before cleanup-preservation assertions can pass.
- Documentation and full `make ci` run only after story implementation is complete.

### Parallel Opportunities

- T003 can run alongside T001/T002.
- T004 can be drafted alongside T005 because it targets internal/fleet/security_gate_test.go while T005 targets internal/fleet/security_gate.go.
- T010, T013, and T014 can be drafted in parallel because they touch separate test areas; T011 and T012 should coordinate with T010 because they may share deploy command or deploy internals fixtures.
- T021, T024, and T026 can be drafted in parallel because they target upgrade internals, output JSON, and audit behavior separately; T022 and T023 should coordinate with T021 because all three edit internal/fleet/upgrade_test.go; T025 should coordinate with T024 and T026 if it touches shared command-output fixtures.
- T032 and T033 should be coordinated because both edit internal/fleet/security_gate_test.go.
- T036, T037, T038, T039, and T040 can run in parallel after behavior is implemented.

---

## Parallel Example: User Story 1

```text
Task: "Add deploy strict apply tests that assert StrictSecurityError, findings.json, preserved clone, and no compile/manifest/PR step in internal/fleet/deploy_test.go"
Task: "Add sync strict tests for missing-workflow delegation and prune-only apply blocking before commit/push in internal/fleet/sync_test.go"
Task: "Add deploy and sync Cobra flag/help/propagation tests for --strict in cmd/deploy_test.go and cmd/sync_test.go"
```

## Parallel Example: User Story 2

```text
Task: "Add upgrade dry-run strict abort tests for non-zero error, findings.json, preserved clone, and unchanged compile-strict fields in internal/fleet/upgrade_test.go"
Task: "Add upgrade --all --strict --output json tests proving NDJSON stops after the blocked repo but emits that repo envelope in cmd/output_test.go"
Task: "Add upgrade --audit --strict no-op behavior coverage in cmd/upgrade_test.go"
```

## Parallel Example: User Story 3

```text
Task: "Add promptinj-only HIGH non-blocking tests to internal/fleet/security_gate_test.go"
Task: "Add mixed promptinj HIGH plus Layer 1 HIGH blocking-count tests to internal/fleet/security_gate_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 and Phase 2.
2. Complete Phase 3 for deploy/sync strict apply gating.
3. Stop and validate US1 independently: strict blocks HIGH findings before mutation; non-strict remains advisory.
4. Update minimal docs if shipping MVP alone.

### Incremental Delivery

1. Deliver US1: single-repo deploy/sync hard gate.
2. Deliver US2: dry-run and fleet-wide upgrade strict behavior for CI.
3. Deliver US3: prompt-injection carve-out for future Layer 3 rules.
4. Complete polish and run `make ci`.

### Validation Gate

Before reporting implementation complete:

1. Run focused tests for internal/fleet/security_gate_test.go, internal/fleet/deploy_test.go, internal/fleet/sync_test.go, internal/fleet/upgrade_test.go, and cmd/output_test.go.
2. Run `make ci`.
3. Confirm `cmd.SchemaVersion` and `fleet.SchemaVersion` were not changed.
4. Confirm no `--strict` value is written to fleet.json or fleet.local.json.

## Notes

- `--strict` in this feature is the security gate only; it must not change compile-strict behavior.
- `findings.json` is a failure breadcrumb and should be written only on strict abort.
- Do not run live `--apply` validation without explicit user approval in the immediately preceding turn.
- Do not hand-edit CHANGELOG.md; release-please owns it.
