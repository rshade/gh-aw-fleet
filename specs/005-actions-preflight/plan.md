# Implementation Plan: Deploy Preflight for Actions Enabled and Workflow Write Permissions

**Branch**: `005-actions-preflight` | **Date**: 2026-04-29 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/005-actions-preflight/spec.md`

## Summary

Extend the existing single-repo deploy preflight (which today checks the engine secret) with two additional read-only checks against the GitHub Actions repo-settings API: (1) is GitHub Actions enabled on the target repo, and (2) is the `GITHUB_TOKEN` workflow permission `write`. Both findings flow through the established `DeployResult`-field plumbing (mirroring `MissingSecret` / `SecretKeyURL`), surfacing as: a stderr `zerolog.Warn()` line during dry-run and apply, a structured `Diagnostic` entry in the `--output json` envelope's `warnings[]`, and a sub-block in the unified "Setup required" PR-body section under `--apply`. All four `/speckit.clarify` answers (no `SchemaVersion` bump, single PR-body section, indeterminate-on-missing-field, reuse `ghAPIJSON` swap pattern) are binding inputs to this design.

The new code consists of a single function `checkActionsSettings()` in `internal/fleet/deploy.go` that calls `ghAPIJSON()` twice (one per endpoint), populates two new boolean fields on `DeployResult`, and routes any non-determinate response (403, 5xx, network error, missing field) through the same fail-open path with a single debug log entry. The existing `missingSecretPRSection` composer is retired in favor of a `setupRequiredSection(*DeployResult)` that emits one umbrella section with up to three sub-blocks in fixed order (Actions → token → secret). Two new diagnostic codes (`actions_disabled`, `workflow_token_read_only`) join the table in `internal/fleet/diagnostics.go`. No new third-party dependencies, no new packages, no `cmd.SchemaVersion` bump.

## Technical Context

**Language/Version**: Go 1.25.8 (per `go.mod`).
**Primary Dependencies**: `github.com/spf13/cobra` v1.10.2 (CLI), `github.com/rs/zerolog` v1.x (stderr structured logging), `encoding/json` (stdlib, JSON envelope). **No new third-party dependencies** (Constitution Principle I; Assumptions in spec).
**Storage**: N/A — pure read calls to the GitHub API; no on-disk state, no cache. Findings are transient on `DeployResult`.
**Testing**: `go test ./...`. Table-driven unit tests in `internal/fleet/deploy_test.go` (extended) — overrides `ghAPIJSON` package variable with a closure that returns canned `map[string]any` per endpoint path. Mirrors `TestCheckEngineSecret`'s `ghAPIExists` override pattern (clarification Q4). No real network calls.
**Target Platform**: Linux/macOS/WSL — wherever the `gh-aw-fleet` Go binary already runs. No platform-specific code paths.
**Project Type**: CLI tool (single Go module, `cmd/` + `internal/fleet/` layered as elsewhere in the repo).
**Performance Goals**: Two extra `gh api` calls per `Deploy()` invocation. At single-repo scope each is ~100–300 ms; the per-deploy ceiling stays well under the 5-minute constitution ceiling and well under the engine-secret check's existing two calls (which already double-hop repo→org).
**Constraints**: Read-only against the API (FR-012); fail-open on 403/5xx/missing-field (FR-007/FR-008, clarification Q3); no new abstractions beyond reusing `ghAPIJSON` (clarification Q4); two findings independent — neither short-circuits the other (FR-014); both modes (dry-run and `--apply`) emit identical warnings (FR-006).
**Scale/Scope**: One feature-call per `Deploy()` invocation. Today `Deploy()` is single-repo; if it ever fans out across N repos, each repo gets two extra API calls — still tractable up to the 5000 req/hr authenticated rate limit.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Compliance | Evidence |
|---|---|---|
| **I. Thin-Orchestrator Code Quality** | ✅ PASS | Wraps `gh api` (existing `ghAPIJSON` package-var); never re-implements GitHub HTTP. Reuses `Diagnostic`, `DeployResult`, the existing PR-body composer's section-heading idiom. New code is ~50 LOC for the check + ~30 LOC for the composer extension + ~30 LOC of test fixtures. No file approaches 300 lines. Comments will explain WHY (fail-open rationale, why `can_approve_pull_request_reviews` is excluded), not WHAT. |
| **II. Testing Standards** | ✅ PASS | `go build`/`go vet` clean is enforced. `make ci` (fmt-check + vet + lint + test) is the ship gate. Real-world dry-run: spec's manual testing strategy (toggle Actions off / set token to read-only on a scratch repo) IS the constitution's "real-world dry-run" test. Unit tests for `checkActionsSettings` are additive (constitution permits, spec mandates via SC-006). |
| **III. User Experience Consistency** | ✅ PASS | Three-turn pattern: deploy already follows it (dry-run → user approves → `--apply`); this feature extends preflight inside the existing flow without changing the cadence. Conventional commit subject for the feature itself: `ci(workflows): add Actions-settings preflight to deploy` (≤72 chars, no period — actually the user-facing scope is `feat(deploy):` per the GitHub issue title; constitution principle scopes commits to `ci(workflows)` for *workflow-touching* commits, which the *implementation* of this feature is not; spec/issue title takes precedence here). Hints surface via the same `Diagnostic` channel. PR body's setup-required section preserved and extended (clarification Q2). |
| **IV. Performance Requirements** | ✅ PASS | Two extra `gh api` calls per deploy. They are sequential (Actions enabled → token permissions); parallelizing two single-shot calls is not worth the goroutine overhead and would hide ordering in tests. No cache: settings can change between deploys, and the extra calls are cheap relative to the deploy's clone + push. Well under 5-minute ceiling. |
| **Declarative Reconcile Invariants** | ✅ PASS | Reads-only against the GitHub API (FR-012). Does not invoke git, does not mutate `fleet.json`, does not bypass gpg signing, does not invoke `git add`/`commit`/`push`. The feature reports drift in repo-level *settings* (orthogonal to workflow-set drift); it does not auto-remediate (out of scope per spec). |

**Result**: All gates pass. **No Complexity Tracking entries required.**

## Project Structure

### Documentation (this feature)

```text
specs/005-actions-preflight/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── deploy-result.md     # DeployResult shape post-extension; JSON envelope additions
│   └── api-endpoints.md     # GitHub API contracts consumed (request shape + accepted response variants)
├── checklists/
│   └── requirements.md  # Quality checklist (already 16/16 from /speckit.specify)
├── spec.md              # Source spec (post-clarify, four Q&As recorded)
└── tasks.md             # Generated by /speckit.tasks (NOT this command)
```

### Source Code (repository root)

This feature touches an existing single-project Go CLI. No new packages, no helper-extraction phase. The structural pattern (one cmd/<name>.go per Cobra command + one internal/fleet/<name>.go per feature module) is preserved; this feature is an *extension* to the existing deploy module, so it adds files only for tests and edits existing files for behavior.

```text
cmd/
├── deploy.go                  # MODIFY — emitDeployWarnings() emits two new zerolog warnings;
│                              #          emitDeployEnvelope() adds two new Diagnostic entries to warnings[]
├── deploy_test.go             # NEW (small) — printDeploy / envelope tests for the two new findings
└── (other files untouched)

internal/fleet/
├── deploy.go                  # MODIFY — DeployResult gains ActionsDisabled, WorkflowTokenReadOnly bools;
│                              #          new checkActionsSettings() function;
│                              #          missingSecretPRSection retired in favor of setupRequiredSection;
│                              #          BuildMissingSecretMessage retained for backward shape, joined by
│                              #          BuildActionsDisabledMessage / BuildWorkflowTokenReadOnlyMessage;
│                              #          Deploy() and the two resume call-sites add a single
│                              #          checkActionsSettings() call alongside checkEngineSecret().
├── deploy_test.go             # MODIFY — add TestCheckActionsSettings (table-driven, four outcomes),
│                              #          add TestSetupRequiredSection (renders correctly under each
│                              #          combination of {ActionsDisabled, WorkflowTokenReadOnly, MissingSecret}).
├── diagnostics.go             # MODIFY — add DiagActionsDisabled, DiagWorkflowTokenReadOnly constants
└── (other files untouched)

# Documentation
README.md                      # MAYBE MODIFY — only if the README's "What deploy checks" section enumerates
                               #                preflight checks; the feature adds two. (Inspect during plan;
                               #                if the section does not exist, no change.)
CHANGELOG.md                   # NOT TOUCHED MANUALLY — release-please derives from conventional commits
```

**Structure Decision**: Same-package, same-file extension. `checkActionsSettings` lives in `internal/fleet/deploy.go` next to `checkEngineSecret` because they share the preflight call site and the `DeployResult`-field plumbing pattern. The composer rename (`missingSecretPRSection` → `setupRequiredSection`) is a small refactor that earns its keep by absorbing all three findings under one heading per clarification Q2; it is the **minimum** structural change that satisfies the contract. **No new packages, no new files in `internal/fleet/` — only `_test.go` additions are new files in `cmd/`.** This is the simplest design that satisfies every functional requirement and clarification.

## Complexity Tracking

> **No constitution violations — this section intentionally empty.**

The design uses zero new third-party dependencies, introduces zero new packages, reuses every cross-cutting helper (`ghAPIJSON` swap pattern, `Diagnostic` codes, JSON envelope writer, zerolog stderr emission), and the only new file in production code is the test pair update. The composer rename (`missingSecretPRSection` → `setupRequiredSection`) is the single architectural shift, and it is mandated by clarification Q2's unified-section answer rather than discretionary. Nothing here calls for a Complexity Tracking entry.
