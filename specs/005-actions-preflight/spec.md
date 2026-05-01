# Feature Specification: Deploy Preflight for Actions Enabled and Workflow Write Permissions

**Feature Branch**: `005-actions-preflight`
**Created**: 2026-04-29
**Status**: Draft
**Input**: GitHub issue #11 — `feat(deploy): preflight check for Actions enabled and workflow write permissions`

## Clarifications

### Session 2026-04-29

- Q: Should this feature bump `cmd.SchemaVersion` (the JSON output envelope version from spec 003) when adding two new optional boolean fields to `DeployResult` and two new diagnostic codes? → A: No bump. The additions are strictly additive (boolean defaults to `false`, `warnings[].code` values are open-set), and no release has shipped since the schema was introduced — there are no real-world consumers to break.
- Q: When multiple preflight findings fire (Actions disabled, read-only token, missing secret) under `--apply`, how should they appear in the PR body? → A: One unified "Setup required" section with one sub-bullet (or sub-block) per finding in a fixed order — Actions enabled, workflow token, engine secret. Existing PR-body composer is extended to accept all findings; absent findings produce no output.
- Q: How should the preflight behave when the API returns 200 but the JSON is structurally valid yet lacks the expected field (e.g., API version drift, partial response, future field rename)? → A: Treat as indeterminate — skip silently with a debug log line identifying the endpoint and the missing field. Same fail-open path as 403/5xx; never assume defaults, never assume worst case.
- Q: What's the unit-test injection strategy for `checkActionsSettings`? → A: Reuse the existing `ghAPIJSON` package-level `var func(...)` swap pattern. Tests override the variable with a closure that returns canned `map[string]any` per endpoint path, mirroring `TestCheckEngineSecret`'s override of `ghAPIExists`. No new abstraction layer; no `GitHubClient` interface; no integration-only testing.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Catch a disabled-Actions repo before merge (Priority: P1)

A fleet operator onboards a new repository by running `gh-aw-fleet deploy <repo>` (dry-run by default). The repo's GitHub Actions setting is "Disable actions" (a common default for forks and freshly-imported repos). Today the deploy succeeds, the PR opens, the operator merges it, and every workflow file just sits there inert — discovered only when nobody can explain why the agentic workflows never run. With this feature, the dry-run emits a clear warning naming the repo and linking directly to `https://github.com/<repo>/settings/actions`, so the operator either fixes the setting before requesting review or, at minimum, knows the merge alone won't activate anything.

**Why this priority**: This is the failure mode that *silently* breaks a deploy. The PR looks healthy, CI on the deploy itself looks healthy, the workflow files land cleanly — and yet the entire deploy delivers zero value because the runtime is off. Every other deploy preflight (engine secret, source pin) catches a problem the operator can already see in failure logs; this one catches a problem the operator might never look for. P1 because the cost of missing it is "the whole deploy was wasted" and the check is read-only and cheap.

**Independent Test**: On a test repo with Actions disabled (`gh api -X PUT /repos/<owner>/<repo>/actions/permissions -f enabled=false`), run `gh-aw-fleet deploy <repo>` (no `--apply`) and assert the stderr output contains a warning naming the repo and linking to the settings URL, and that no other warnings or errors are emitted when the only configured-incorrectly thing is Actions being disabled.

**Acceptance Scenarios**:

1. **Given** a repo with GitHub Actions disabled at the repo level, **When** the operator runs `gh-aw-fleet deploy <repo>` (dry-run), **Then** stderr emits a warning identifying the repo as having Actions disabled and a direct link to `https://github.com/<repo>/settings/actions`, the dry-run still completes (does not error), and the warning is also surfaced as a structured diagnostic in `--output json` mode.
2. **Given** a repo with Actions enabled and write workflow permissions, **When** the operator runs `gh-aw-fleet deploy <repo>`, **Then** no Actions-settings warnings appear, only the existing deploy output.
3. **Given** the operator runs `gh-aw-fleet deploy <repo> --apply` against a repo with Actions disabled, **When** the deploy executes, **Then** the same warning is emitted (the check fires before commit/push, identical to the existing engine-secret check), the PR is still created, and the warning is included in the PR body's setup-required section so the human reviewer sees it without re-running the dry-run.

---

### User Story 2 - Catch a read-only `GITHUB_TOKEN` before merge (Priority: P1)

The same operator deploys to a repo whose "Workflow permissions" setting is "Read repository contents and packages permissions" (the default for new repos and many enterprise org defaults). Several agentic workflows in the default profile (e.g., `pr-fix`, `code-simplifier`) push commits, create reviews, or comment on PRs — all of which require *write* token permissions. With a read-only token, the workflows execute, then fail at the first write call with a `403`. With this feature, the operator sees a warning during dry-run that names the repo, names the consequence ("workflows that push commits or create reviews will fail"), and links to the settings page with the exact toggle to flip.

**Why this priority**: Same blast radius as User Story 1 — every write-using workflow in the deploy is broken — but the symptom (403 in workflow logs) is at least visible in the Actions tab if the operator thinks to look. Slightly less silent than fully-disabled Actions, but still routinely missed because the *workflow* succeeds at the trigger layer; it's the inner step that fails. P1 alongside US1 because a fleet operator deploying a write-using profile to a read-only-token repo is the *expected* fault-injection scenario this whole feature was filed for, and the same single API call answers it.

**Independent Test**: On a test repo with `gh api -X PUT /repos/<owner>/<repo>/actions/permissions/workflow -f default_workflow_permissions=read`, run `gh-aw-fleet deploy <repo>` and assert that stderr emits a warning naming the repo as having a read-only workflow token, including the consequence sentence and the settings URL, AND that the warning fires independently of US1's check (i.e., works even when Actions itself is enabled).

**Acceptance Scenarios**:

1. **Given** a repo with Actions enabled but `default_workflow_permissions: "read"`, **When** the operator runs `gh-aw-fleet deploy <repo>`, **Then** stderr emits a read-only-token warning with the consequence ("workflows that push commits or create reviews will fail"), the settings URL, and the explicit fix ("Workflow permissions → Read and write permissions"), AND a structured diagnostic appears in `--output json` mode.
2. **Given** a repo with `default_workflow_permissions: "write"` (regardless of `can_approve_pull_request_reviews`), **When** the operator runs `gh-aw-fleet deploy <repo>`, **Then** no read-only-token warning appears.
3. **Given** a repo where *both* Actions is disabled AND the workflow token is read-only, **When** the operator runs `gh-aw-fleet deploy <repo>`, **Then** both warnings fire (US1 and US2 are independent; one does not suppress the other), and both appear in the PR body's setup-required section under `--apply`.

---

### User Story 3 - Fail-open behavior under restricted tokens (Priority: P2)

A fleet operator runs deploy from a CI environment where the `GH_TOKEN` is intentionally narrow-scoped (no `admin:repo` or repo-admin scope). The Actions-settings API endpoints return `403`. The deploy MUST NOT fail — the operator's token might be the right one for *deploy* (push branch + open PR is just `repo`), even though it cannot read repo administrative settings. The preflight skips silently with at most a single debug-level log entry; the operator gets the deploy outcome they asked for.

**Why this priority**: Without this behavior, the feature is worse than nothing — it would break legitimate CI deploys that have always worked. P2 because it's a *non-regression* requirement rather than added value, but it's the gating condition that makes US1/US2 safe to ship at all.

**Independent Test**: With a token that has `repo` scope but lacks `admin:repo` (or any scope that returns `403` from `/repos/<owner>/<repo>/actions/permissions`), run `gh-aw-fleet deploy <repo>` and assert: (a) the dry-run completes successfully with no Actions-settings warnings, (b) no panic, (c) at most one debug-level log line acknowledging the skipped check is emitted (visible at `--log-level debug`, hidden at default `info`).

**Acceptance Scenarios**:

1. **Given** a token that returns `403` from `/repos/<owner>/<repo>/actions/permissions`, **When** the operator runs `gh-aw-fleet deploy <repo>`, **Then** no Actions-settings warning fires, the deploy proceeds normally, no panic occurs, and a debug log line records that the preflight was skipped due to insufficient scope.
2. **Given** a network failure or rate-limit response (HTTP 5xx, 429, or transport error) from the Actions-settings endpoint, **When** the preflight runs, **Then** behavior matches the 403 case (skip silently, debug log only) — no warning, no failure.
3. **Given** any non-200, non-403 error from the API (e.g., 401 token expired), **When** the preflight runs, **Then** the deploy still proceeds; the malformed-or-unexpected response does not crash the tool.

---

### Edge Cases

- **Repo doesn't exist (404 from `/repos/<owner>/<repo>/actions/permissions`)**: Preflight skips. The deploy itself will fail later with its own clearer 404 hint when `gh aw add` or the clone step runs; preflight does not duplicate that error.
- **Repo is archived**: The Actions-permissions endpoint returns the same shape as a live repo. If `enabled: false` because archiving disabled Actions, the warning fires — which is the correct outcome (the operator should know).
- **Actions allowed but restricted by an org-level allow-list**: `/repos/.../actions/permissions` returns `enabled: true` because the *repo* setting is enabled. The org-level restriction surfaces only in the workflow run, not at this layer. Out of scope for this preflight; document as a known limitation.
- **Workflow permissions inherited from org default**: The repo-level endpoint reflects the *effective* setting (org default cascades into the repo response unless explicitly overridden). The check is correct against effective state without needing org-level lookups.
- **`enabled: true` but `allowed_actions: "selected"` with a reusable-workflow allow-list that excludes `actions/checkout`**: Out of scope — the preflight covers the on/off and read/write axes only. A separate follow-up issue can extend to allow-list checks if it becomes a real operator pain point.
- **Repo migration / rename redirect**: `gh api` follows the redirect; the preflight reads the canonical repo's settings. No special handling needed.
- **Two repos in one deploy invocation (current code is single-repo, but design forward)**: Each repo's preflight runs independently; warnings are scoped per-repo. Today `Deploy()` takes a single repo, so this is forward-compat only.
- **Dry-run vs. apply parity**: Both modes emit the same warning. The PR body's setup-required section (already present for `MissingSecret`) gains the same Actions-settings warnings under `--apply` so a reviewer who didn't run the dry-run still sees them.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Deploy MUST query `/repos/<owner>/<repo>/actions/permissions` during preflight (alongside the existing engine-secret check) and detect whether `.enabled` is `false`.
- **FR-002**: Deploy MUST query `/repos/<owner>/<repo>/actions/permissions/workflow` during preflight and detect whether `.default_workflow_permissions` is `"read"`. The value of `.can_approve_pull_request_reviews` MUST NOT influence this determination.
- **FR-003**: When Actions is disabled, deploy MUST emit a single-line stderr warning naming the repo and including the URL `https://github.com/<repo>/settings/actions`. The warning MUST be structured (zerolog `warn` level), so `--log-format json` consumers receive a typed event.
- **FR-004**: When the workflow token is read-only, deploy MUST emit a stderr warning naming the repo, the operational consequence ("workflows that push commits or create reviews will fail"), the settings URL, and the explicit fix instruction ("Workflow permissions → Read and write permissions").
- **FR-005**: When both checks pass, deploy MUST NOT emit any Actions-settings warning.
- **FR-006**: Both warnings MUST fire identically in dry-run and `--apply` modes; the preflight MUST run before any commit/push activity, mirroring the existing engine-secret check call site.
- **FR-007**: When `gh api` returns `403` from either endpoint, the preflight MUST treat the check as "indeterminate" and emit no warning. A single debug-level log entry MUST record that the check was skipped, identifying which endpoint and which repo.
- **FR-008**: When `gh api` returns any non-success response (HTTP 5xx, network error, malformed JSON), OR returns 200 with a structurally valid JSON body that lacks the expected field (`enabled` for `/actions/permissions`, `default_workflow_permissions` for `/actions/permissions/workflow`), the preflight MUST behave identically to the 403 case (skip with a debug log line that identifies the endpoint and, where applicable, the missing field name). The preflight MUST NOT assume default values, MUST NOT assume worst-case values, and MUST NOT surface a generic "could not determine" warning. The deploy MUST proceed regardless.
- **FR-009**: Both findings MUST be exposed as fields on `DeployResult` (or equivalent contract type returned from `Deploy()`), enabling callers (the `cmd/deploy.go` printer, JSON envelope writer, and PR-body composer) to surface them without re-running the check.
- **FR-010**: When `--output json` is set (per spec 003), each warning MUST appear as a structured `Diagnostic` entry in the envelope's `warnings[]` array with stable `code` values (one per finding, distinct from the existing `DiagMissingSecret` code) and a `fields.url` value pointing at the relevant settings page. The affected repo is identified by the envelope's top-level `result.repo` field — per-`Diagnostic` `fields` carry feature-specific context only, mirroring the placement of the existing `missing_secret` diagnostic in `warnings[]`.
- **FR-011**: When `--apply` opens a PR, the PR body MUST emit a single unified "Setup required" section containing one sub-bullet (or sub-block) per active preflight finding. Findings MUST appear in this fixed order: (1) Actions disabled, (2) workflow token read-only, (3) missing engine secret. The section heading MUST be omitted entirely when no findings are active. Each sub-bullet MUST be self-contained (operator can act without re-running deploy) and include the relevant settings URL or copy-pastable command.
- **FR-012**: The preflight MUST NOT issue any state-changing API calls (no `PATCH`, `PUT`, `POST`, `DELETE` against the Actions-settings endpoints). It MUST be observably read-only.
- **FR-013**: Adding the preflight MUST NOT add new third-party dependencies (constitution Principle I); the existing `gh api` invocation pattern used by `checkEngineSecret` is the implementation precedent.
- **FR-014**: The two checks MUST be independent — the disabled-Actions warning and the read-only-token warning each fire based on their own endpoint result, regardless of whether the other check fired, errored, or skipped.

### Key Entities

- **Repo Actions Settings**: The pair `(actions_enabled: bool, workflow_permissions: "read" | "write")` derived from the two API endpoints. Only the `default_workflow_permissions` field of the second response is consumed; other fields (`can_approve_pull_request_reviews`, `allowed_actions`) are out of scope.
- **Preflight Finding**: A discriminated outcome `{none | actions_disabled | workflow_token_read_only | indeterminate}` per check. `indeterminate` (covering 403/5xx/network errors) is observable only via debug logs and does NOT surface as a warning.
- **Deploy Result Extension**: Two boolean fields added to the existing `DeployResult` contract — `ActionsDisabled` and `WorkflowTokenReadOnly` — populated by the preflight and consumed by the human-readable printer, JSON envelope, and PR-body composer.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a deploy targeting a repo with Actions disabled, the operator sees a warning in the dry-run output that names the repo and provides a clickable settings URL — verifiable by running deploy against a known-disabled test repo and asserting one stderr line contains both the repo and the URL.
- **SC-002**: For a deploy targeting a repo with a read-only workflow token, the operator sees a warning that includes both the consequence and the explicit fix instruction — verifiable by running deploy against a known-read-only test repo and asserting one stderr line contains all three: repo, consequence sentence, and "Read and write permissions" fix string.
- **SC-003**: For a deploy with restricted-scope tokens (403 on either endpoint), the deploy completes with the same exit code and same stdout/stderr as it does today (no regression) — verifiable by capturing baseline output before/after the change against a 403-returning token and asserting they match outside of new debug-level entries.
- **SC-004**: For a deploy with both checks healthy, the new feature adds zero output to stderr or stdout (no spurious "all good" notices) — verifiable by diffing dry-run output against a known-healthy repo before/after the change.
- **SC-005**: PR bodies created by `--apply` against a misconfigured repo include the warnings in the setup-required section without operator intervention — verifiable by running `--apply` against a test repo with both findings and inspecting the resulting PR body.
- **SC-006**: Unit tests cover all four preflight outcomes (healthy, disabled, read-only, indeterminate-via-403/5xx/missing-field) by overriding the existing `ghAPIJSON` package-level variable with fixture closures — no real network calls, no new test infrastructure, total runtime under one second. Verifiable from `make test` timing and from inspection of the new test file mirroring `TestCheckEngineSecret`'s shape.
- **SC-007**: The structured-JSON envelope (`--output json`) emits one stable `Diagnostic` entry per finding with stable `code` values, allowing CI consumers to gate on `warnings[].code == "actions_disabled" OR warnings[].code == "workflow_token_read_only"` — verifiable by `jq` extraction against a captured envelope from a misconfigured repo.

## Assumptions

- The existing `ghAPIJSON` package-level `var func(...)` (used by `fetch.go` and `status.go`) is the call site for both new endpoints; no new HTTP plumbing is needed. Tests inject fixtures by overriding this variable, matching the `TestCheckEngineSecret` pattern that overrides `ghAPIExists`.
- `default_workflow_permissions` is the only field needed to answer the read-only question. `can_approve_pull_request_reviews` is orthogonal (controls approval, not write) and is excluded by design.
- The `DeployResult` JSON wire contract (spec 003) grows with new optional boolean fields without a `cmd.SchemaVersion` bump (decided in clarifications): boolean defaults to `false`, `warnings[].code` is an open set, and no release has shipped since the schema was introduced, so additive growth is non-breaking by construction.
- "Fail open" on 403 is the operator-friendly default; this matches the precedent set by the engine-secret check, which silently skips when the org-level secret-list endpoint requires elevated scope.
- The PR body extension surface (`setup-required` section) already established for `MissingSecret` is the right place for the new warnings; format follows the same paste-friendly, copy-into-shell pattern (link plus one-line action).
- Two distinct diagnostic codes (`actions_disabled`, `workflow_token_read_only`) are preferable to a single combined code, because operators want to filter and gate on them independently.
- Out of scope for this feature: automated remediation (modifying the repo's settings via API), org-level allow-list checks, branch-protection-rule preflights, and per-workflow `permissions:` block analysis. Each is its own follow-up if demand emerges.
