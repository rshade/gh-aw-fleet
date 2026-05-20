# Implementation Plan: Compile Workflows with --strict on Public Repos by Default

**Branch**: `010-compile-strict-public` | **Date**: 2026-05-17 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/010-compile-strict-public/spec.md`

## Summary

Add a tri-state `compile_strict` field to `RepoSpec` and a `Config.EffectiveCompileStrict(ctx, repo)` resolver. After every `gh aw add` (Deploy) or `gh aw upgrade`/`update` (Upgrade) and before `git add .github/`, when the resolver returns `true`, probe the local `gh aw compile --help` for the `--strict` flag and then invoke `gh aw compile --strict` in the work-dir clone. Surface the outcome via:

1. **Structured logs**: one `info` line naming repo / effective value / resolution source; one `warn` line on visibility-lookup fallback.
2. **Typed envelope fields**: `CompileStrictApplied bool` + `CompileStrictSource string` on `DeployResult` / `UpgradeResult` (clarification Q1 → Option A).
3. **Diagnostic hints**: new entries in `internal/fleet/diagnostics.go` for "strict mode validation failed" (compile-time abort) and "gh aw too old, minimum v0.68.3" (probe failure — clarification Q2 → Option A).
4. **`gh-aw-fleet add` info line** announcing the deploy-time policy at onboarding.

No new third-party dependencies. No `cmd.SchemaVersion` bump. No `fleet.SchemaVersion` bump. Sync inherits the compile step transitively via its existing `applyDeployOrPrune` delegation to Deploy. The probe is conditional on `EffectiveCompileStrict == true`, so private-repo-only operators incur zero added latency.

## Technical Context

**Language/Version**: Go 1.25.8 (per `go.mod`).
**Primary Dependencies**: All existing — `github.com/spf13/cobra` v1.10.2, `github.com/rs/zerolog` v1.35.1, `github.com/tailscale/hujson` (for round-trip-safe `fleet.local.json` writes per #73), Go stdlib (`encoding/json`, `os/exec`, `context`, `strings`, `fmt`). **No new direct dependencies.**
**Storage**: N/A — pure read calls (`gh api /repos/<owner>/<repo>`) plus subprocess invocations (`gh aw compile --help`, `gh aw compile --strict`, `gh aw --version`). The new `compile_strict` field is persisted via the existing HuJson round-trip on `fleet.local.json`; no separate state file, no cache.
**Testing**: Go stdlib `testing` with table-driven cases. Network-touching and subprocess-touching code paths injected behind package-level `var func(...)` seams (matching `ghAPIExists` / `ghAPIJSON` / `ghDiscussionsAPI` / `ghRunArtifactAPI` precedents) so unit tests run offline. `make test` SHOULD remain under the existing baseline; new tests budget under 200ms growth per SC-006 / SC-010.
**Target Platform**: Linux + macOS developer shells; CI runs on Linux per `.github/workflows/`. WSL2 also supported (the active development environment).
**Project Type**: CLI tool — single Go module rooted at the repo root. `cmd/` for cobra subcommands, `internal/fleet/` for orchestration logic, `internal/security/`, `internal/log/`, `internal/fleet/fleetdiag/` for cross-cutting helpers (the diagnostics leaf package — `internal/fleet/diagnostics.go` is the alias/re-export surface).
**Performance Goals**:

- Visibility lookup adds at most one `gh api` call per Deploy/Upgrade invocation (≤500ms typical) — and is skipped entirely on explicit-override paths (FR-008).
- CLI capability probe adds at most one `gh aw compile --help` exec per invocation when strict is effective (≤200ms typical).
- Unit-test runtime growth ≤200ms total (SC-006, SC-010).
- Compile step itself is bounded by `gh aw compile` runtime (typically 1–10s per repo for a normal profile size); no new bottleneck introduced by this feature.

**Constraints**:

- No new third-party dependencies (Constitution §Third-Party Dependencies + Principle I).
- No `cmd.SchemaVersion` bump on the JSON envelope; no `fleet.SchemaVersion` bump on the on-disk fleet config (both additions are observably additive).
- Workflow markdown MUST NOT be rewritten by this feature; only `.lock.yml` files change (CLAUDE.md hard invariant).
- Work-dir clones at `/tmp/gh-aw-fleet-*` MUST be preserved on `--apply` failure (Constitution Principle III).
- No commit-signing bypass; this feature touches the `git add` / commit phase only by adding a precondition gate, not by re-routing signing logic.

**Scale/Scope**: Typical fleet is ~10–50 repos in `fleet.local.json`. Deploy/Upgrade today is single-repo per CLI invocation; this feature inherits that scope. Future multi-repo support (anticipated forward-compat) would invoke this feature once per repo, with no shared cache (Assumption #1 in spec).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle / Section | Status | Justification |
|--------------------|--------|---------------|
| **I. Thin-Orchestrator Code Quality** | **PASS** | The feature is a textbook orchestration: one `gh api` call (visibility), one `gh aw compile --help` exec (probe), one `gh aw compile --strict` exec (compile), one `gh aw --version` exec (version parse). Zero re-implementation of upstream behavior. The probe simply scans help output for a token rather than parsing CLI flag grammars. Workflow markdown is never rewritten. `go build` / `go vet` will stay clean — no exotic Go used. Files affected stay focused: `deploy.go` and `upgrade.go` each gain ~50 lines of compile + probe wiring; no file approaches the 300-line caution threshold. |
| **II. Testing Standards** | **PASS (exceeds floor)** | Constitutional floor is `go build` + `go vet` clean. This spec voluntarily adds unit-test coverage via SC-006 (resolver) and SC-010 (probe). Dry-run path is preserved (US1 scenario 2 — dry-run shows the would-be policy without invoking compile). Skills (`skills/fleet-deploy/SKILL.md`) updated per FR-006 visibility into the three-turn flow. |
| **III. User Experience Consistency** | **PASS** | `--apply` gate respected on both Deploy and Upgrade. Compile failure preserves `/tmp/gh-aw-fleet-*` clone per FR-009. Probe failure (FR-016) preserves clone identically — same precondition class as the existing `MissingSecret` / `ActionsDisabled` checks. All new error surfaces route through `diagnostics.CollectHints` per Principle III. Conventional Commits scope (`ci(workflows)`) untouched (this feature doesn't change commit-message generation). Commit-signing remains intact. |
| **IV. Performance Requirements** | **PASS** | Two new sub-second `exec.Command` calls (visibility + probe) per Deploy/Upgrade, and only when applicable. The probe is conditional on `EffectiveCompileStrict == true`, so private-repo-only operators see zero added latency. The actual `gh aw compile --strict` invocation runs `gh aw`'s own compiler — that workload is required by the feature's purpose and not avoidable. The 5-minute ceiling is comfortably preserved. No new caching is needed (Assumption: visibility is fetched once per invocation, fresh each time, by design). |
| **Declarative Reconcile Invariants** | **PASS** | `fleet.json` / `fleet.local.json` remain the source of truth — the new `compile_strict` field lives there. `.gitignore` already covers `fleet.local.json`. Commit-signing bypass forbidden — feature does not touch signing. `git add` / `git commit` / `git push` from Claude Code's Bash tool remain denied; the tool's own `exec.Command` calls inside Deploy/Upgrade are unchanged. Source-pin discipline (tagged refs for `github/gh-aw`) is unrelated to this feature. |
| **Third-Party Dependencies** | **PASS** | No new entries in `go.mod`'s direct `require()` block. All new code uses stdlib (`encoding/json`, `os/exec`, `context`, `strings`, `fmt`) plus the existing approved deps (`cobra`, `zerolog`, `hujson`). |
| **Development Workflow** | **PASS** | PR will include `go build` / `go vet` clean, a deploy dry-run against a real public repo (e.g., `rshade/gh-aw-fleet`) demonstrating US1, and the fleet-deploy skill update per FR documentation. The `CollectHints` patterns gain entries per FR-009 and FR-016. `CLAUDE.md` will be updated to point at this plan via the SPECKIT block (no new architectural invariant established — the probe pattern is scoped to this feature per Assumption #11). |

**Gate verdict**: **PASS, no violations**. No Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/010-compile-strict-public/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
│   ├── json-envelope.md      # DeployResult / UpgradeResult typed-field contract
│   └── cli-semantics.md      # User-facing CLI behavior for deploy/upgrade/add
├── checklists/
│   └── requirements.md  # Spec quality checklist (already generated)
├── spec.md
└── tasks.md             # Phase 2 output (/speckit-tasks command — NOT created here)
```

### Source Code (repository root)

```text
cmd/
├── add.go              # FR-010: print info line on visibility detection at add time
├── deploy.go           # Print typed CompileStrict fields in human-readable output
├── upgrade.go          # Same as deploy.go for the upgrade pipeline
└── (other cobra commands — untouched)

internal/fleet/
├── schema.go           # FR-001: add CompileStrict *bool to RepoSpec; FR-003: EffectiveCompileStrict resolver
├── deploy.go           # FR-004, FR-006, FR-007, FR-008, FR-015, FR-016: probe + compile invocation; populate DeployResult fields
├── upgrade.go          # FR-005: same compile invocation + probe + UpgradeResult fields
├── sync.go             # No changes (transitive via applyDeployOrPrune per spec Assumption #3)
├── diagnostics.go      # FR-009: strict-mode-failed hint; FR-016: gh-aw-too-old hint; gh-aw-missing hint
├── load.go             # No changes — HuJson round-trip already handles additive optional fields (spec Assumption #8)
└── (test files mirror source)

internal/fleet/
├── schema_test.go      # SC-006: table-driven EffectiveCompileStrict across 6 rows (5 SC-006 branches + the "internal" visibility edge case)
├── deploy_test.go      # SC-005, SC-009, SC-010: probe outcomes + compile invocation + JSON envelope assertions
└── upgrade_test.go     # Symmetric to deploy_test.go for upgrade pipeline

internal/fleet/fleetdiag/
└── diag.go             # New DiagCompileStrictFailed, DiagGhAwTooOld, DiagGhAwMissing codes
                        # (re-exported through internal/fleet/diagnostics.go per the existing alias pattern)

README.md               # FR documentation: compile_strict resolution order; minimum gh aw version

skills/fleet-deploy/SKILL.md   # Note the auto-strict behavior in the three-turn pattern
```

**Structure Decision**: Single-project Go CLI layout, unchanged from the existing repo organization. All new code lands in existing files (`schema.go`, `deploy.go`, `upgrade.go`, `diagnostics.go`, `cmd/add.go`, `cmd/deploy.go`, `cmd/upgrade.go`) or in new test functions in existing test files. The only directory addition is `specs/010-compile-strict-public/contracts/` for Phase 1 contract docs. No new packages, no new top-level directories.

## Complexity Tracking

> No Constitution Check violations — this section is intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|--------------------------------------|
| _(none)_  | _(n/a)_    | _(n/a)_                              |
