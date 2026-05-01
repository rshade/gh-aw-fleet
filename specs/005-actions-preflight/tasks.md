---
description: "Task list for 005-actions-preflight"
---

# Tasks: Deploy Preflight for Actions Enabled and Workflow Write Permissions

**Input**: Design documents from `/specs/005-actions-preflight/`
**Prerequisites**: plan.md (✅), spec.md with US1/US2/US3 priorities (✅), research.md (✅), data-model.md (✅), contracts/{deploy-result.md, api-endpoints.md} (✅), quickstart.md (✅)

**Tests**: Mandatory per spec **SC-006** ("Unit tests cover all four preflight outcomes…"). Test tasks are NOT optional for this feature.

**Organization**: Tasks are grouped by user story. US1 and US2 are both P1 and share the same `checkActionsSettings` function and the same `setupRequiredSection` composer; story independence lives in per-finding **consumer wiring** (stderr warn line, JSON envelope `Diagnostic`, PR-body sub-block). US3 is primarily test coverage for the indeterminate paths the foundational phase implements.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Maps task to user story for traceability (US1, US2, US3)
- File paths are absolute relative to repo root unless otherwise stated

## Path Conventions

Single Go module at the repo root: `cmd/`, `internal/fleet/`. No new packages — this feature is a same-package extension to `internal/fleet/deploy.go` plus consumers in `cmd/deploy.go`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish a clean baseline before adding any code, so regressions are easy to attribute.

- [X] T001 Run `make ci` against `005-actions-preflight` HEAD and capture pass-state — establishes the green baseline. If anything fails here, fix it before any feature work begins (the failure isn't from this feature).
- [X] T002 [P] Confirm Go toolchain matches `go.mod` (`go version` ≥ 1.25.8) and that `gh auth status` is clean — manual checks needed before any test that might need to fall back to a local network call.

**Checkpoint**: Local environment matches CI; `make ci` is green; baseline established for diff comparison.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add the data carriers (`DeployResult` fields, `Diagnostic` codes), the check function (`checkActionsSettings`), and the renamed composer (`setupRequiredSection` shell). After this phase, US1 and US2 can each independently wire their finding through the consumer surfaces.

**⚠️ CRITICAL**: No US1/US2/US3 work can begin until this phase is complete. The check function returns both booleans together (per data-model.md), so splitting it across user-story phases is not possible.

- [X] T003 Add two boolean fields `ActionsDisabled bool \`json:"actions_disabled"\`` and `WorkflowTokenReadOnly bool \`json:"workflow_token_read_only"\`` to `DeployResult` (append after `SecretKeyURL` at `internal/fleet/deploy.go:52`). Include godoc per CLAUDE.md "Code self-documentation" rule explaining that `false` is also the indeterminate value (no warning unless we have positive evidence).
- [X] T004 [P] Add `DiagActionsDisabled = "actions_disabled"` and `DiagWorkflowTokenReadOnly = "workflow_token_read_only"` constants to the existing `const (...)` block in `internal/fleet/diagnostics.go` (positioned adjacent to `DiagMissingSecret` per data-model.md §Type 2). Snake-case, no namespace prefix, consistent with the other ten codes.
- [X] T005 Implement `checkActionsSettings(ctx context.Context, repo string) (actionsDisabled, workflowTokenReadOnly bool)` in `internal/fleet/deploy.go` (next to `checkEngineSecret` at line 535). Body: two sequential `ghAPIJSON()` calls — one to `/repos/<repo>/actions/permissions` (read `enabled` bool), one to `/repos/<repo>/actions/permissions/workflow` (read `default_workflow_permissions` string, return `value == "read"`). Each endpoint is independent — neither short-circuits the other (FR-014). On any error / non-`map[string]any` body / missing field / wrong type / unknown enum value, return the corresponding boolean `false` AND emit one `zlog.Debug().Str("repo", repo).Str("endpoint", path).Str("reason", reason).Msg("actions-settings preflight skipped")` line per the contract in `contracts/api-endpoints.md` §"Observability contract". No `error` return — every failure is treated as indeterminate per clarification Q3 and FR-008. Include a godoc comment naming WHY: fail-open semantics, why `can_approve_pull_request_reviews` is excluded.
- [X] T006 Wire `checkActionsSettings()` into `Deploy()` at all three existing `checkEngineSecret()` call sites in `internal/fleet/deploy.go` — lines 137 (main flow), 199 (`handleWorkDirResume` commit-gate), and 209 (`handleWorkDirResume` push-gate). Each call site adds one new line: `res.ActionsDisabled, res.WorkflowTokenReadOnly = checkActionsSettings(ctx, repo)` immediately after the existing `res.MissingSecret, res.SecretKeyURL = checkEngineSecret(...)` line. The triplet placement (resume parity) is mandated by research §R6.
- [X] T007 Rename `missingSecretPRSection` to `setupRequiredSection` in `internal/fleet/deploy.go` (function at line 80, call site at line 614). Change the signature from `func missingSecretPRSection(res *DeployResult) string` to `func setupRequiredSection(res *DeployResult) string`. At this stage, the function body must still produce the *exact same output* for the missing-secret case (so existing snapshot/integration tests stay green); the per-finding sub-blocks land in T013 (US1) and T020 (US2). Update the call site at line 614 to: `if section := setupRequiredSection(res); section != "" { b.WriteString(section); ... }` so the heading is suppressed when no findings are active. Update godoc per CLAUDE.md to describe the new umbrella-section contract and fixed Actions→token→secret order.
- [X] T008 [P] Run `make fmt vet` after T003–T007 to confirm the foundational changes still compile and the new exported symbols carry godoc. `make lint` may fail until consumers are wired (deferred to per-story phases) — that's acceptable here because the new fields don't have consumers yet, but log any unexpected lint output for follow-up.
- [X] T009 [P] Smoke-test the foundational layer: `go run . deploy <healthy-repo>` (no `--apply`) against a repo with Actions enabled and write token. Expected: zero new stderr output (the new check populates `false`/`false`, the renamed composer renders identically to the old one). Compare stdout/stderr byte-for-byte against the T001 baseline; any diff means foundational scaffolding leaked output (FR-005, SC-004 violation).

**Checkpoint**: `DeployResult` carries the two new fields, `checkActionsSettings()` populates them at every preflight site, the composer is renamed, and a healthy-repo deploy is byte-identical to baseline. Ready for per-story consumer wiring.

---

## Phase 3: User Story 1 — Catch a disabled-Actions repo before merge (Priority: P1) 🎯 MVP

**Goal**: When `gh-aw-fleet deploy <repo>` runs against a repo whose GitHub Actions setting is "Disable actions," the operator sees a stderr warning naming the repo and linking to `https://github.com/<repo>/settings/actions`. Same warning fires under `--apply` and is included in the PR body's "Setup required" section. JSON envelope contains a `Diagnostic{Code: "actions_disabled"}` entry.

**Independent Test** (per spec): On a test repo with Actions disabled (`gh api -X PUT /repos/<repo>/actions/permissions -f enabled=false`), run `gh-aw-fleet deploy <repo>` (dry-run) and assert the stderr output contains a warning naming the repo and linking to the settings URL, and that no other warnings or errors are emitted when the only configured-incorrectly thing is Actions being disabled.

### Tests for User Story 1 ⚠️ (write FIRST, ensure they FAIL before T013–T015)

- [X] T010 [P] [US1] Add `TestCheckActionsSettings` (new test function) to `internal/fleet/deploy_test.go` mirroring the `TestCheckEngineSecret` shape. Override `ghAPIJSON` per research R7 with a closure that pattern-matches on path. Add table cases for the Actions-endpoint half: (a) `enabled: true` → returns `(false, _)`, (b) `enabled: false` → returns `(true, _)`. Token-endpoint cases land in T018 (US2). Test must compile but fail until `checkActionsSettings` is wired (it is — T005 — so this test should *pass* even before US1 implementation; that's expected because the check function lives in the foundational phase).
- [X] T011 [P] [US1] Create `cmd/deploy_test.go` (NEW file) with three tests: (1) `TestEmitDeployWarningsActionsDisabled` — construct `DeployResult{Repo: "alice/widgets", ActionsDisabled: true}` and assert `emitDeployWarnings(res)` writes one stderr line containing both `"alice/widgets"` and `"https://github.com/alice/widgets/settings/actions"`. (2) `TestEmitDeployEnvelopeActionsDisabled` — assert `emitDeployEnvelope(res, ...)` produces an envelope with `warnings[]` containing one `Diagnostic` whose `Code == "actions_disabled"` and `Fields["url"]` matches the settings URL. (3) `TestEmitDeployWarnings_HealthyEmitsNothing` (closes **SC-004**) — construct `DeployResult{Repo: "alice/widgets", ActionsDisabled: false, WorkflowTokenReadOnly: false, MissingSecret: ""}`, capture stderr via a buffer, assert the captured output is empty (zero bytes). Tests (1)–(2) MUST FAIL after T010 and before T013–T015; test (3) passes immediately because the existing `emitDeployWarnings` is already a no-op for the healthy `MissingSecret == ""` case — it pins the contract so a regression in future edits would surface immediately.
- [X] T012 [P] [US1] Add `TestSetupRequiredSection_ActionsOnly` to `internal/fleet/deploy_test.go`. Construct `DeployResult{Repo: "alice/widgets", ActionsDisabled: true}` and assert the rendered markdown: contains the `## ⚠ Setup required` heading exactly once, contains an Actions-disabled sub-block whose body names the repo and the settings URL, contains NO token sub-block, contains NO secret sub-block. Will FAIL after T010 and before T013.

### Implementation for User Story 1

- [X] T013 [US1] Add `BuildActionsDisabledMessage(repo string) string` exported helper in `internal/fleet/deploy.go` (positioned next to `BuildMissingSecretMessage` at line 69). Returns the single-line warning per the wire-shape in `contracts/deploy-result.md` §"warnings[] — additions" example: `GitHub Actions is disabled on <repo> — enable at https://github.com/<repo>/settings/actions`. Add godoc explaining the URL embed-and-link pattern.
- [X] T014 [US1] Extend `setupRequiredSection` in `internal/fleet/deploy.go` to render the Actions-disabled sub-block (first in fixed order, before any existing missing-secret block). Sub-block format follows `data-model.md` §Function 2 worked example: `**GitHub Actions is disabled on \`<repo>\`.** Workflows added in this PR will not run until Actions is enabled.\n\nEnable at: https://github.com/<repo>/settings/actions\n\n`. Conditional on `res.ActionsDisabled`. The umbrella `## ⚠ Setup required` heading must still appear exactly once (composed at the top of the function only when at least one finding is active).
- [X] T015 [US1] Extend `emitDeployWarnings` in `cmd/deploy.go` (line 121–131) to emit a `zlog.Warn().Str("repo", res.Repo).Str("url", "...").Msg(fleet.BuildActionsDisabledMessage(res.Repo))` line when `res.ActionsDisabled` is `true`. Place this branch **before** the existing `res.MissingSecret` branch so the stderr ordering matches the PR-body fixed order (Actions → token → secret).
- [X] T016 [US1] Extend `emitDeployEnvelope` in `cmd/deploy.go` (line 140) to append `Diagnostic{Code: fleet.DiagActionsDisabled, Message: fleet.BuildActionsDisabledMessage(res.Repo), Fields: map[string]any{"url": "https://github.com/" + res.Repo + "/settings/actions"}}` to `warnings[]` when `res.ActionsDisabled` is `true`. Append before the existing `MissingSecret` diagnostic so envelope ordering matches the contract.
- [X] T017 [US1] Run `make ci` end-to-end. T010/T011/T012 must now pass. Manual verification per quickstart §"Verify 'Actions disabled' warning": disable Actions on a test repo via `gh api -X PUT /repos/<test-repo>/actions/permissions -f enabled=false`, run `go run . deploy <test-repo>` (NO `--apply`), confirm warning fires with repo name and settings URL on a single stderr line. Re-enable.

**Checkpoint**: User Story 1 is fully functional. A repo with Actions disabled produces exactly one stderr warning, one `Diagnostic` in the JSON envelope, and one PR-body sub-block under `--apply`. Healthy repos remain byte-identical to baseline (SC-004).

---

## Phase 4: User Story 2 — Catch a read-only `GITHUB_TOKEN` before merge (Priority: P1)

**Goal**: When `gh-aw-fleet deploy <repo>` runs against a repo whose "Workflow permissions" setting is "Read repository contents and packages permissions" (i.e., `default_workflow_permissions: "read"`), the operator sees a stderr warning that names the repo, the operational consequence ("workflows that push commits or create reviews will fail"), the settings URL, and the explicit fix ("Workflow permissions → Read and write permissions"). Same warning fires under `--apply` and appears in the PR body. JSON envelope contains a `Diagnostic{Code: "workflow_token_read_only"}` entry.

**Independent Test** (per spec): On a test repo with `gh api -X PUT /repos/<repo>/actions/permissions/workflow -f default_workflow_permissions=read`, run `gh-aw-fleet deploy <repo>` and assert that stderr emits a warning naming the repo as having a read-only workflow token, including the consequence sentence and the settings URL, AND that the warning fires independently of US1's check.

### Tests for User Story 2 ⚠️ (write FIRST, ensure they FAIL before T021–T023)

- [X] T018 [P] [US2] Extend `TestCheckActionsSettings` in `internal/fleet/deploy_test.go` (added in T010) with token-endpoint cases: (a) `default_workflow_permissions: "write"` → returns `(_, false)`, (b) `default_workflow_permissions: "read"` → returns `(_, true)`, (c) cross-product cases with the Actions endpoint to prove independence (FR-014): healthy/write, disabled/write, healthy/read, disabled/read, (d) `{default_workflow_permissions: "write", can_approve_pull_request_reviews: true}` → returns `(_, false)` — proves **FR-002**'s "MUST NOT influence" clause from the write side, (e) `{default_workflow_permissions: "read", can_approve_pull_request_reviews: false}` → returns `(_, true)` — proves the same clause from the read side. Will fail-pass-by-design (foundational already implements this; the test merely codifies the behavior).
- [X] T019 [P] [US2] Add `TestEmitDeployWarningsTokenReadOnly` and `TestEmitDeployEnvelopeTokenReadOnly` to `cmd/deploy_test.go`. Construct `DeployResult{Repo: "alice/widgets", WorkflowTokenReadOnly: true}` and assert: warn line contains repo, the consequence string `"workflows that push commits or create reviews will fail"`, the settings URL, and the fix string `"Read and write permissions"`. Envelope contains one `Diagnostic` with `Code == "workflow_token_read_only"`. Add a third test `TestEmitDeployWarningsBothFindings` asserting **fixed order**: Actions warning emits BEFORE token warning, both before missing-secret. Tests MUST FAIL until T021–T023.
- [X] T020 [P] [US2] Add `TestSetupRequiredSection_TokenOnly`, `TestSetupRequiredSection_ActionsAndToken`, `TestSetupRequiredSection_AllThree`, and `TestPRBodyContainsSetupRequiredSection` to `internal/fleet/deploy_test.go`. Assert: token-only test contains exactly one heading + one token sub-block; actions+token test contains exactly one heading + Actions sub-block before token sub-block; all-three test contains all three sub-blocks in Actions→token→secret order with exactly one heading; empty-state assertion (no fields → empty string return). The new `TestPRBodyContainsSetupRequiredSection` (closes **SC-005**) calls `prBody(res)` with `res.ActionsDisabled = true; res.WorkflowTokenReadOnly = true` and asserts the returned PR body string contains the `## ⚠ Setup required` heading exactly once and the two sub-blocks in fixed order — proving the composer is wired into `prBody` correctly (the unit-level composer tests above only cover `setupRequiredSection` in isolation). All four MUST FAIL until T022.

### Implementation for User Story 2

- [X] T021 [US2] Add `BuildWorkflowTokenReadOnlyMessage(repo string) string` exported helper in `internal/fleet/deploy.go` next to `BuildActionsDisabledMessage`. Returns: `GITHUB_TOKEN is read-only on <repo> — workflows that push commits or create reviews will fail; set "Workflow permissions" → "Read and write permissions" at https://github.com/<repo>/settings/actions` per `contracts/deploy-result.md` §"warnings[] — additions". Godoc explains the consequence sentence and exact setting control are part of the contract surface (FR-004).
- [X] T022 [US2] Extend `setupRequiredSection` in `internal/fleet/deploy.go` (modified in T014) to render the token sub-block immediately after the Actions sub-block and before the missing-secret sub-block. Format per `data-model.md` §Function 2: `**Workflow token is read-only on \`<repo>\`.** Agentic workflows that push commits or create reviews will fail.\n\nFix at: https://github.com/<repo>/settings/actions\n\nSet "Workflow permissions" → "Read and write permissions"\n\n`. Conditional on `res.WorkflowTokenReadOnly`.
- [X] T023 [US2] Extend `emitDeployWarnings` in `cmd/deploy.go` (modified in T015) to emit a `zlog.Warn()` line for `res.WorkflowTokenReadOnly`, positioned after the Actions-disabled branch and before the missing-secret branch (fixed-order invariant from FR-011). Extend `emitDeployEnvelope` (modified in T016) to append a `Diagnostic{Code: fleet.DiagWorkflowTokenReadOnly, ...}` in the same fixed-order position.
- [X] T024 [US2] Run `make ci`. T018/T019/T020 must now pass. Manual verification per quickstart §"Verify 'Workflow token read-only' warning": `gh api -X PUT /repos/<test-repo>/actions/permissions/workflow -f default_workflow_permissions=read`, run `go run . deploy <test-repo>`, confirm the warning fires with all four substrings (repo, consequence, URL, fix). Restore. Then verify both warnings together: misconfigure both, run dry-run, confirm both stderr lines appear in fixed order Actions → token; run `--apply` against a clean branch and inspect the PR body for the unified `## ⚠ Setup required` section with exactly one heading and two sub-blocks.

**Checkpoint**: Both P1 user stories ship. The fixed-order invariant (Actions → token → secret) is enforced and tested across all three surfaces (stderr, envelope, PR body). The MVP is deliverable here — US3 below is non-regression coverage, not added value.

---

## Phase 5: User Story 3 — Fail-open behavior under restricted tokens (Priority: P2)

**Goal**: Deploys from CI environments with narrow-scoped tokens (where `gh api` returns 403 / 5xx / network error / malformed JSON / missing field on the Actions-settings endpoints) MUST complete with no Actions-settings warnings, no panic, and at most one `Debug`-level log line per skipped endpoint. Default `--log-level info` produces output byte-identical to a healthy-repo deploy.

**Independent Test** (per spec): With a token that has `repo` scope but lacks `admin:repo`, run `gh-aw-fleet deploy <repo>` and assert: (a) the dry-run completes successfully with no Actions-settings warnings, (b) no panic, (c) at most one debug-level log line per endpoint at `--log-level debug`, (d) zero output at `--log-level info`.

**Note**: The fail-open *behavior* lives in the foundational phase (T005). US3 is the **test coverage** that proves the contract holds across every documented indeterminate path in `contracts/api-endpoints.md`.

### Tests for User Story 3 ⚠️ (extend the test surface from US1/US2)

- [X] T025 [P] [US3] Extend `TestCheckActionsSettings` in `internal/fleet/deploy_test.go` with indeterminate-path cases for **Endpoint 1** (`/actions/permissions`): (a) `ghAPIJSON` returns `(nil, errors.New("http_403"))` → returns `(false, _)`, (b) `ghAPIJSON` returns `(nil, http_5xx-error)` → `(false, _)`, (c) `ghAPIJSON` returns `(map[string]any{}, nil)` (missing `enabled` field) → `(false, _)`, (d) returns `map[string]any{"enabled": "yes"}` (wrong type) → `(false, _)`, (e) returns `("not an object", nil)` → `(false, _)`, (f) network error / `errors.New("transport_error")` → `(false, _)`. Each case asserts the `false` return AND that NO warning was emitted (use a captured stderr writer or zerolog test sink).
- [X] T026 [P] [US3] Mirror T025 for **Endpoint 2** (`/actions/permissions/workflow`): same six failure shapes plus (g) `default_workflow_permissions: "none"` (unknown enum value, future GitHub addition per `contracts/api-endpoints.md` §"Field value space") → `(_, false)` with debug log `reason=unknown_value:none`. Each case asserts `false` return AND no warning emission.
- [X] T027 [US3] Add `TestCheckActionsSettings_DebugLogShape` to `internal/fleet/deploy_test.go`. Use a zerolog test writer (e.g., bytes.Buffer with `zerolog.New(buf)`) configured to capture the package logger at `Debug` level. For one indeterminate case (e.g., 403 on Endpoint 1), assert exactly one log line is emitted, parse it as JSON, and verify it contains `repo`, `endpoint`, and `reason` fields per `contracts/api-endpoints.md` §"Observability contract".
- [X] T028 [US3] Manual verification per quickstart §"Restricted-token CI — silent fall-through": Verified live against `rshade/finfocus` with `GH_TOKEN=ghp_invalid_garbage_for_test` + pre-cloned `--work-dir`. Result: deploy completed `exit=0`, info-level emitted zero settings noise, debug-level emitted exactly two `DBG actions-settings preflight skipped` lines (one per endpoint, both classified `reason=http_401`).

**Checkpoint**: All three preflight outcomes (healthy / positive finding / indeterminate) are tested across both endpoints. The fail-open contract is provable from the test suite alone, no live network needed.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final integration, documentation, and the local-CI gate.

- [X] T029 [P] Inspect `README.md` for any "What deploy checks" / "Preflight" enumeration. If present, add Actions-enabled and workflow-token-write to that list with the same link/depth as the existing engine-secret entry. If no such section exists, skip per plan.md §Project Structure ("MAYBE MODIFY").
- [X] T030 [P] Inspect `CLAUDE.md` for the "Active Technologies" block listing endpoints/dependencies per spec. Confirm the existing 005-actions-preflight bullet (already present) accurately names the two endpoints; no edit needed unless drift is found.
- [X] T031 Confirm **FR-012** (read-only invariant): grep the diff for `PUT|POST|PATCH|DELETE` against `actions/permissions` paths; expected zero matches in production code (test fixtures may use `PUT` to *describe* the manual reproduction steps in comments, but they must not execute against the live API in tests).
- [X] T032 Confirm **FR-013** (no new third-party deps): `git diff origin/main -- go.mod go.sum` must be empty. If anything appears, revert — the implementation must use only `encoding/json`, `context`, `fmt`, the existing zerolog, and the existing `ghAPIJSON` swap point.
- [X] T033 Confirm **clarification Q1** (no schema bump): `cmd.SchemaVersion` constant must remain `1`. `git diff origin/main -- cmd/output.go` (or wherever `SchemaVersion` is declared) shows no change to the constant. The two new `result.*` fields and two new `warnings[].code` values are strictly additive per `contracts/deploy-result.md` §"Stability guarantees".
- [X] T034 Run final `make ci` — fmt-check + vet + lint + test must all pass. This is the ship gate per CLAUDE.md §"Local gate." If lint catches a missing godoc on any new exported symbol, fix in place; do not suppress. Capture `make test` wall-clock timing and confirm the new `TestCheckActionsSettings` and `cmd/deploy_test.go` tests collectively complete in well under one second (closes **SC-006** timing constraint); flag for follow-up if the in-memory closure pattern unexpectedly slows the suite.
- [X] T035 Run quickstart.md end-to-end against a real test repo. Verified live against `rshade/finfocus` with Actions disabled (`gh api -X PUT … -F enabled=false`) + token already read-only (`default_workflow_permissions: "read"`). Result: `go run . deploy rshade/finfocus --apply --force` ran preflight, fired both stderr warnings in fixed order (Actions → token), added all 11 workflows, and staged 12 files for commit; gpg-non-interactive halted at `git commit` per the documented manual-finish path (CLAUDE.md), so the live `gh pr create` step did not execute. PR-body content verified by composition: `TestPRBodyContainsSetupRequiredSection` pins the exact `prBody(res)` output for the same `DeployResult{ActionsDisabled:true, WorkflowTokenReadOnly:true}` shape that the live deploy populated. Settings restored: `enabled: true` reconfirmed via `gh api`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Phase 1. **BLOCKS** all user stories — `checkActionsSettings`, the two `DeployResult` fields, and the `setupRequiredSection` rename are all required before any per-story consumer wiring can land.
- **User Story 1 (Phase 3)**: Depends on Phase 2.
- **User Story 2 (Phase 4)**: Depends on Phase 2. *Also touches the same files as Phase 3* (`emitDeployWarnings`, `emitDeployEnvelope`, `setupRequiredSection`). Within those functions, US1 and US2 add **independent branches** at distinct ordered positions — no shared lines, but conflicting if attempted in parallel by two developers without coordination. Recommended order: US1 first (it's the MVP), US2 second.
- **User Story 3 (Phase 5)**: Depends on Phase 2 (the function under test exists there). Independent of US1/US2 implementation — US3's tests live in different test functions and do not need US1/US2 production code to pass.
- **Polish (Phase 6)**: Depends on US1+US2 (and ideally US3) being complete.

### User Story Independence

- **US1**: Independently testable end-to-end after T017 (healthy + Actions-disabled paths covered; warning, envelope, PR body all carry the finding). The MVP can ship here — US2 is a separate value increment.
- **US2**: Independently testable after T024. Reuses US1's wiring at adjacent positions in the same functions; the dependency is structural (fixed-order Actions→token), not behavioral (US2 doesn't *need* US1 to fire to fire itself).
- **US3**: Test-only. Can be written and verified concurrently with US1/US2 because `checkActionsSettings` already implements fail-open in the foundational phase — US3's tests merely document and pin the behavior.

### Within Each User Story

- Tests (T010–T012, T018–T020, T025–T027) MUST be written and FAIL before implementation tasks land. (Exception: T010/T018/T025 may pass immediately because they exercise `checkActionsSettings`, implemented in foundational T005 — that is by design; the function logic is shared, only consumer wiring is per-story.)
- Helpers (`Build*Message`) before composer extension (`setupRequiredSection`) before consumer wiring (`emitDeployWarnings`, `emitDeployEnvelope`).
- Each story complete (with `make ci` green) before moving to the next priority.

### Parallel Opportunities

- **Within Phase 2**: T003 (DeployResult fields) and T004 (Diagnostic codes) touch different files — can run in parallel. T005 (`checkActionsSettings`) depends on T003 (uses the field types implicitly). T006 (call sites) depends on T005. T007 (composer rename) is independent of T003–T006 — can run in parallel.
- **Within US1**: T010 (deploy_test.go), T011 (cmd/deploy_test.go), T012 (setupRequiredSection test) are different files — all three [P]. Implementation tasks T013–T016 each touch different functions but in two shared files (`internal/fleet/deploy.go` and `cmd/deploy.go`); can be sequenced top-to-bottom in one developer's stream.
- **Within US2**: T018, T019, T020 are independent test files — all three [P]. T021–T023 are sequential extensions to functions modified in US1.
- **US1 ↔ US2 in parallel**: NOT recommended — shared function bodies (`setupRequiredSection`, `emitDeployWarnings`, `emitDeployEnvelope`) require fixed-order coordination. Sequence them.
- **US3 in parallel with US1+US2 implementation**: YES — US3 is test-only and does not modify production code.
- **Within Phase 6**: T029 (README) and T030 (CLAUDE.md) are independent — [P]. T031–T033 are independent grep/diff checks — [P]. T034 (`make ci`) and T035 (manual verification) must run after all polish tasks land.

---

## Parallel Example: User Story 1 Tests

```bash
# All three US1 test files can be drafted and run in parallel:
Task: "Add TestCheckActionsSettings (Actions-endpoint cases) in internal/fleet/deploy_test.go"
Task: "Create cmd/deploy_test.go with TestEmitDeployWarningsActionsDisabled and TestEmitDeployEnvelopeActionsDisabled"
Task: "Add TestSetupRequiredSection_ActionsOnly in internal/fleet/deploy_test.go"
```

Each test asserts a different surface (check function / command-layer warn+envelope / composer); none of them depend on the others passing first.

---

## Parallel Example: Foundational Phase

```bash
# Two parallel tasks while T005 is in flight:
Task: "Add DeployResult.ActionsDisabled and DeployResult.WorkflowTokenReadOnly fields in internal/fleet/deploy.go"  # T003
Task: "Add DiagActionsDisabled and DiagWorkflowTokenReadOnly constants in internal/fleet/diagnostics.go"  # T004 [P]
Task: "Rename missingSecretPRSection to setupRequiredSection in internal/fleet/deploy.go"  # T007 (independent of T003/T004)
# Then sequentially:
Task: "Implement checkActionsSettings function in internal/fleet/deploy.go"  # T005 (depends on T003)
Task: "Wire checkActionsSettings into Deploy() at three call sites"  # T006 (depends on T005)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup) — T001, T002.
2. Complete Phase 2 (Foundational) — T003 → T009. **BLOCKS US1**.
3. Complete Phase 3 (US1) — T010 → T017.
4. **STOP and VALIDATE**: dry-run against a known Actions-disabled test repo; assert the warning fires with the expected substrings. PR-body block renders correctly under `--apply` against a scratch repo.
5. MVP shippable here: a fleet operator deploying to a freshly-imported repo (Actions-off-by-default) gets warned before merge — the silent-failure mode the GitHub issue was filed for.

### Incremental Delivery

1. Setup + Foundational → green baseline + plumbing.
2. US1 → Actions-disabled finding shipped → MVP demo.
3. US2 → token-read-only finding shipped, fixed-order verified across all three findings.
4. US3 → fail-open coverage pinned with tests.
5. Polish → README/CLAUDE.md/`make ci` final pass + quickstart manual verification.

### Parallel Team Strategy

Single-developer feature; parallelism is task-level, not story-level. If staffed across two developers:

- Dev A: Phase 1 + Phase 2 + Phase 3 (US1 — MVP).
- Dev B (concurrent with Dev A's Phase 3): Phase 5 (US3 — test-only), once Phase 2 is merged.
- Dev A: Phase 4 (US2) once Dev A's US1 lands.
- Either: Phase 6 (Polish).

### Suggested MVP Scope

**Phase 1 + Phase 2 + Phase 3 (User Story 1)**. Ships the disabled-Actions warning end-to-end. US2 (token read-only) and US3 (fail-open coverage) are independent value increments that follow.

---

## Format Validation Note

Every task in this file follows the strict checklist format `- [ ] [TaskID] [P?] [Story?] Description with file path`:

- ✅ All 35 tasks open with `- [ ]`.
- ✅ Sequential `T001`…`T035` IDs with no gaps.
- ✅ `[P]` is present only where the task is genuinely parallelizable (different file, no dependency on incomplete tasks).
- ✅ `[US1]`, `[US2]`, `[US3]` story labels appear ONLY on tasks in Phases 3, 4, and 5 respectively. Setup (Phase 1), Foundational (Phase 2), and Polish (Phase 6) tasks have no story label.
- ✅ Every task names the exact file path or functional surface it modifies.
