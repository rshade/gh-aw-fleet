---
description: "Task list for 010-compile-strict-public — auto-on `gh aw compile --strict` on public repos with explicit per-repo override, CLI version probe, and typed envelope fields."
---

# Tasks: Compile Workflows with --strict on Public Repos by Default

**Input**: Design documents from `/specs/010-compile-strict-public/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/json-envelope.md, contracts/cli-semantics.md, quickstart.md

**Tests**: Included. SC-006, SC-007, SC-008, SC-009, SC-010 mandate offline unit tests via the four package-level `var func(...)` injection seams introduced by Foundational task T004. Per Constitution v1.1.0 §II all subprocess and `gh api` paths MUST be substitutable behind a seam — no live network in `make test`. SC-008 requires the full `make ci` gate (gofmt, vet, golangci-lint, full test suite) to pass before this slice is considered done.

**Organization**: Five user stories ship in a single bundled PR. US3 (fail-secure on lookup failure) and US4 (compile-failure diagnostic) are *entangled with US1's main path* — their behavior is implemented in the same Foundational + US1 code paths because the failure branches cannot be split from the happy path. Those story phases are therefore test-only verifications of branches that ship in US1. US2 (explicit override) is also test-and-docs-only — the resolver's branch logic lands in Foundational. US5 (onboarding info line) is the only story with genuinely independent new code in `cmd/add.go`. Tasks within a story that target *distinct* files are marked `[P]`; tasks targeting the *same* file with non-overlapping additions are also marked `[P]` (consistent with the precedent in `specs/009-consumption-subcommand/tasks.md`).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, or distinct non-overlapping additions to the same test file)
- **[Story]**: Which user story this task belongs to (US1 = public-auto MVP, US2 = explicit override, US3 = fail-secure, US4 = compile/probe failure, US5 = onboarding info)
- Include exact file paths in descriptions

## Path Conventions

Single-binary Go CLI. No separate `tests/` tree — `*_test.go` lives next to the code under test. Paths below are repo-root-relative.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No project-level initialization is required — this feature is an additive extension on an established Go module with all dependencies already present in `go.mod`. No fixture files are needed (all test fixtures are inline closures injected via the seams introduced by Foundational task T004).

**Checkpoint**: Setup is intentionally empty. Proceed directly to Phase 2.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The schema field, resolver, injection seams, result-struct fields, diagnostic codes, and hint patterns underpin every user story. Without them, no story phase can be wired or tested end-to-end. Note: implementation of the FR-007 warn-log emission and the FR-009 / FR-016 abort paths happens in Phase 3 (US1) because they live inside the deploy/upgrade wiring helper; this phase produces only the data-plumbing primitives the helper consumes.

**⚠️ CRITICAL**: No user-story phase may begin until T001 through T009 are complete.

- [X] T001 [P] Add three new diagnostic constants to `internal/fleet/fleetdiag/diag.go` (the canonical leaf-package home for stable diagnostic codes — `internal/fleet/diagnostics.go` is the alias/re-export surface, not the source-of-truth): `DiagCompileStrictFailed = "compile_strict_failed"`, `DiagGhAwTooOld = "gh_aw_too_old"`, `DiagGhAwMissing = "gh_aw_missing"`. Each gets a one-line godoc describing the trigger condition per data-model.md §E5. Then add the matching re-exports to `internal/fleet/diagnostics.go` next to the existing `DiagMissingSecret = fleetdiag.DiagMissingSecret` line.
- [X] T002 [P] Add the `CompileStrict *bool` field to `RepoSpec` in `internal/fleet/schema.go` (between `Engine` and `ExtraWorkflows` per the issue's struct sketch), JSON tag `compile_strict,omitempty`, godoc per data-model.md §E1: tri-state semantics (nil = auto-detect, true = force ON, false = force OFF), no validation at load time, round-trip-safe per the existing HuJson AST mutation contract.
- [X] T003 In `internal/fleet/schema.go`, add the resolver method: `func (c *Config) EffectiveCompileStrict(ctx context.Context, repo string) (effective bool, source string, reason string)` per FR-003 + data-model.md §E2. Resolution order: (1) `RepoSpec.CompileStrict != nil` → return its value with `source = "explicit"` and empty reason; (2) call `ghRepoVisibility(ctx, repo)` (the seam introduced by T004) — `"public"` → `(true, "auto-public", "")`, anything else → `(false, "auto-private", "")`; (3) on error → `(true, "auto-fallback", truncate(err.Error(), 200))`. The 200-char truncation prevents leaking large network errors into logs. Method depends on T002 (RepoSpec field) AND T004 (visibility seam).
- [X] T004 In `internal/fleet/deploy.go`, add four package-level injection seams next to the existing `ghAPIExists` / `ghAPIJSON` declarations (look for the `// ghAPIExists returns true...` block near line 613): (a) `var ghRepoVisibility = func(ctx context.Context, repo string) (string, error) { ... }` — production impl runs `gh api /repos/<repo> --jq .visibility` via the existing `exec.CommandContext` pattern, returns the trimmed string or wrapped error; (b) `var ghAwCompileHelp = func(ctx context.Context) (string, error) { ... }` — runs `gh aw compile --help` (no working-directory dependency — `--help` is process-local), captures combined stdout+stderr; (c) `var ghAwVersion = func(ctx context.Context) (string, error) { ... }` — runs `gh aw --version`, regex-extracts the first `v\d+\.\d+\.\d+` token per research.md R3; (d) `var runGhAwCompileStrict = func(ctx context.Context, dir string) (string, error) { ... }` — runs `gh aw compile --strict` in `dir`, tees combined output to stderr via the existing `runLoggedOutput` pattern from `runUpgrade`. All four seams marked `//nolint:gochecknoglobals // test-injection seam mirroring ghAPIExists/ghAPIJSON`.
- [X] T005 In `internal/fleet/deploy.go`, add two new fields to `DeployResult` next to the existing `ActionsDisabled` / `WorkflowTokenReadOnly` fields (around line 63-68): `CompileStrictApplied bool \`json:"compile_strict_applied"\`` and `CompileStrictSource string \`json:"compile_strict_source"\`` with godoc per data-model.md §E4 — including the explicit note that empty-string `Source` is distinct from the four valid values and consumers MUST treat it as "not applicable to this result." Depends on T004 file co-location only (different lines in same file).
- [X] T006 [P] In `internal/fleet/upgrade.go`, add the same two fields to `UpgradeResult` with identical godoc. Mirror of T005 in a separate file.
- [X] T007 In `internal/fleet/diagnostics.go`, extend `CollectHints(texts ...string) []Diagnostic` with three new substring-match branches (matching the existing `if strings.Contains(...)` pattern): (a) trigger on `"strict mode validation"` or `"strict mode requires"` → emit `DiagCompileStrictFailed` with hint text per data-model.md §E5 row 1; (b) trigger on `"unknown flag: --strict"` or `"unknown long flag '--strict'"` → emit `DiagGhAwTooOld` with hint text mentioning `v0.68.3` minimum and `gh extension upgrade aw`; (c) trigger on `"executable file not found"` or `"gh aw"` + `"not found"` → emit `DiagGhAwMissing` with hint text mentioning `gh extension install github/gh-aw`. Depends on T001 (the constants must exist first).
- [X] T008 [P] In `internal/fleet/schema_test.go`, add `TestEffectiveCompileStrict` — table-driven across the six branches covering SC-006 plus the `"internal"` edge case from spec.md: `(spec=*bool(true), seam=any)` → `("explicit", true)`; `(spec=*bool(false), seam=any)` → `("explicit", false)`; `(spec=nil, seam="public")` → `("auto-public", true)`; `(spec=nil, seam="private")` → `("auto-private", false)`; `(spec=nil, seam="internal")` → `("auto-private", false)` (locks in the spec's "treat `internal` as private" edge case); `(spec=nil, seam=err)` → `("auto-fallback", true)`. Add a local helper `func boolPtr(b bool) *bool { return &b }` at the top of the test file (not present elsewhere in the codebase). Overrides `ghRepoVisibility` via t.Cleanup-style closure swap. Asserts BOTH the boolean and the source string. For the explicit branches, asserts the seam was NOT invoked (counter increment check) — covers FR-008. Depends on T002+T003+T004.
- [X] T009 [P] In `internal/fleet/diagnostics_test.go`, add `TestCollectHints_CompileStrictPatterns` — table-driven across the three new patterns: stderr containing `"strict mode validation failed"` → exactly one diagnostic with code `compile_strict_failed`; stderr containing `"unknown flag: --strict"` → code `gh_aw_too_old`; stderr containing `"gh aw: executable file not found"` → code `gh_aw_missing`. Asserts no false-positive matches against unrelated stderr. Depends on T001+T007.

**Checkpoint**: Foundation ready — the schema field round-trips through the existing loader, the resolver is unit-tested across six rows (five SC-006 branches + the `internal` visibility edge case), the four seams have offline-substitutable defaults, and the diagnostic pattern matchers fire correctly. User-story phases can begin.

---

## Phase 3: User Story 1 — Public repo gets strict-compiled workflows automatically (Priority: P1) 🎯 MVP

**Goal**: A `gh-aw-fleet deploy <public-repo>` with no explicit `compile_strict` setting invokes `gh aw compile --strict` after `gh aw add` completes (and before `git add .github/`), producing strict-compiled `.lock.yml` files in the resulting PR. Symmetric for `upgrade`.

**Independent Test**: Stub `ghRepoVisibility` to return `"public"`, stub `ghAwCompileHelp` to return output containing `"--strict"`, stub `runGhAwCompileStrict` to return success. Run `Deploy()` against a fixture repo. Assert: `DeployResult.CompileStrictApplied == true`, `CompileStrictSource == "auto-public"`, the compile seam was invoked exactly once, and the FR-006 info log event fired with the correct fields.

**Note on entanglement**: T010 (the helper) also implements the FR-007 warn-log path, the FR-009 compile-failure abort, the FR-016 probe abort paths, and the FR-015 envelope-field population — but those branches are tested in US3 (T020-T021) and US4 (T022-T025) phases respectively. This phase tests only the happy-path branch.

### Implementation for User Story 1

- [X] T010 [US1] In `internal/fleet/deploy.go`, add a small unexported interface `compileStrictResult` with two setters (`SetCompileStrictSource(string)`, `SetCompileStrictApplied(bool)`) and a `CloneDir() string` accessor. Both `*DeployResult` and `*UpgradeResult` implement it via two-line methods next to their fields (added in T005/T006). Then add the helper function `runCompileStrictIfNeeded(ctx context.Context, res compileStrictResult, cfg *Config, repo string) error`. Algorithm per cli-semantics.md §"Apply mode": (1) call `cfg.EffectiveCompileStrict(ctx, repo)` and call `res.SetCompileStrictSource(source)`; (2) emit FR-006 zerolog info event `compile_strict_resolved` with fields `repo`, `effective`, `source` per research.md R6; (3) if `source == "auto-fallback"`, also emit FR-007 zerolog warn event `compile_strict_lookup_failed` with `repo` and `reason` fields; (4) if `effective == false`, return nil (compile skipped); (5) call `ghAwCompileHelp(ctx)` — on error, classify as probe-failed → wrap via `CollectHints` into a `DiagGhAwMissing`-bearing error, return; (6) if output lacks `"--strict"`, call `ghAwVersion(ctx)` for diagnostic enrichment → wrap into `DiagGhAwTooOld`-bearing error, return; (7) call `runGhAwCompileStrict(ctx, res.CloneDir())` — on non-zero exit, wrap stderr via `CollectHints` into `DiagCompileStrictFailed`-bearing error, AND preserve raw stderr in the wrapped error's chain so operators see the underlying violation per FR-009 (use `fmt.Errorf("gh aw compile --strict failed: %s: %w", hint.Message, exitErr)` or equivalent), return; (8) on success, call `res.SetCompileStrictApplied(true)`, return nil. The function does NOT delete the work-dir clone on failure (the caller's existing failure-preservation behavior handles that). Depends on T003-T007.
- [X] T011 [US1] In `internal/fleet/deploy.go`, wire `runCompileStrictIfNeeded` into `Deploy()`: invoke it inside `Deploy()` after `addResolvedWorkflows(...)` completes successfully and BEFORE the first `git add` call (find the insertion point by searching for `addResolvedWorkflows(` and stepping forward to just before `git add`; do NOT pin to line numbers — `deploy.go` drifts). On error from the helper, return the error without proceeding to `git add` / commit / push. Also wire it into the resume-from-work-dir path — find it by searching for the second occurrence of `res.MissingSecret, res.SecretKeyURL = checkEngineSecret(...)` (the resume branch sets these post-Init) and invoke the helper directly afterward, so resumed deploys re-run the resolver and re-attempt compile — compile is idempotent. Depends on T010.
- [X] T012 [P] [US1] In `internal/fleet/upgrade.go`, implement the `compileStrictResult` interface methods on `*UpgradeResult` (mirror of the implementations on `*DeployResult` from T010), then invoke the shared `runCompileStrictIfNeeded` helper after `runUpgrade` succeeds and (when applicable) after `runUpdate` succeeds, BEFORE the existing `git add` step. The interface-based design means no duplicated helper — the same function services both pipelines. Depends on T010.
- [X] T013 [P] [US1] In `cmd/deploy.go`, extend the existing human-readable printer (the function that prints `DeployResult` to stdout) to include the new fields when non-default: print `compile-strict: applied (source: <source>)` when `CompileStrictApplied == true`, print `compile-strict: skipped (source: <source>)` when `CompileStrictSource != ""` AND `CompileStrictApplied == false`, omit the line entirely when `Source == ""`. Keep the line at the same indent and grouping as the existing `actions-disabled` / `workflow-token-read-only` lines. Depends on T005.
- [X] T014 [P] [US1] In `cmd/upgrade.go`, mirror T013 for `UpgradeResult`. Depends on T006.

### Tests for User Story 1

- [X] T015 [US1] In `internal/fleet/deploy_test.go`, add `TestDeploy_AutoPublicPath_InvokesStrictCompile` — set up a minimal fake `DeployResult.CloneDir` (temp dir from `t.TempDir`), override `ghRepoVisibility` to return `"public"`, override `ghAwCompileHelp` to return a string containing `"--strict"`, override `runGhAwCompileStrict` with a closure that increments a counter and returns success. Construct a `Config` with a `RepoSpec` for the test repo and `CompileStrict = nil`. Call `runCompileStrictIfNeeded`. Assert: returned err is nil, `res.CompileStrictApplied == true`, `res.CompileStrictSource == "auto-public"`, the compile-strict seam counter == 1, the probe seam counter == 1, the visibility seam counter == 1. Use a `zerolog.New(&buf)` captured logger and assert exactly one `compile_strict_resolved` event with `effective=true source=auto-public` is present in the captured stream. Depends on T010-T011.
- [X] T016 [P] [US1] In `internal/fleet/upgrade_test.go`, add `TestUpgrade_AutoPublicPath_InvokesStrictCompile` — symmetric to T015 against the upgrade pipeline. Asserts the same four counters and the same log shape on the upgrade path. Depends on T012.

**Checkpoint**: US1 MVP works end-to-end. A public-repo deploy with default settings invokes `gh aw compile --strict` automatically; the typed envelope fields populate; the structured log line fires; the human-readable printer shows the resolution. The slice is releasable as the minimum value increment — every other story phase is verification or ergonomics atop this base.

---

## Phase 4: User Story 2 — Explicit operator override beats auto-detection (Priority: P1)

**Goal**: Setting `"compile_strict": true` in `fleet.local.json` forces strict ON (skipping the visibility lookup); setting `"compile_strict": false` forces strict OFF (also skipping the visibility lookup, per FR-008).

**Independent Test**: Two test cases — (a) `CompileStrict = *bool(false)` on a fixture-public repo → assert the compile seam is NEVER invoked, the visibility seam is NEVER invoked, `Source == "explicit"`, `Applied == false`; (b) `CompileStrict = *bool(true)` on a fixture-private repo → assert the compile seam IS invoked, the visibility seam is NEVER invoked, `Source == "explicit"`, `Applied == true`.

**Note on entanglement**: US2's implementation is entirely covered by the Foundational resolver (T003) plus the US1 helper (T010). This phase is test-and-docs-only.

### Tests for User Story 2

- [X] T017 [P] [US2] In `internal/fleet/deploy_test.go`, add `TestDeploy_ExplicitFalseOnPublic_SkipsAll` — `RepoSpec.CompileStrict = boolPtr(false)`, override `ghRepoVisibility` with a closure that fails the test if invoked (per FR-008: explicit override MUST skip the visibility lookup). Call `runCompileStrictIfNeeded`. Assert: returned err is nil, `Applied == false`, `Source == "explicit"`, probe seam never invoked, compile seam never invoked. Depends on T010.
- [X] T018 [P] [US2] In `internal/fleet/deploy_test.go`, add `TestDeploy_ExplicitTrueOnPrivate_InvokesCompile` — `RepoSpec.CompileStrict = boolPtr(true)`, `ghRepoVisibility` closure fails-test-if-invoked. Override `ghAwCompileHelp` to return `"--strict"`, `runGhAwCompileStrict` to return success. Assert: returned err is nil, `Applied == true`, `Source == "explicit"`, probe seam invoked once, compile seam invoked once, visibility seam NEVER invoked. Depends on T010.
- [X] T019 [P] [US2] In `internal/fleet/load_test.go` (or `internal/fleet/schema_test.go` if `load_test.go` doesn't exist), add `TestLoad_CompileStrictRoundtrip` covering SC-007: create a temporary `fleet.local.json` containing a repo with `"compile_strict": false`, load it via `LoadConfig`, save it back via `SaveLocalConfig` (the canonical `fleet.local.json` write path at `internal/fleet/load.go:205` — NOT `fleet.Add` which appends a NEW repo, and NOT `SaveTemplates` which writes `templates.json`). Assert the resulting bytes are identical to the input. Repeat with `"compile_strict": true`. Then test the absence case: load a file WITHOUT `compile_strict`, save via `SaveLocalConfig`, assert byte-identical (the field's `omitempty` JSON tag ensures absence is preserved). Depends on T002.

**Checkpoint**: US2 verified. The explicit-override branches of the resolver are tested; the loader round-trip is asserted byte-identical for present/absent and true/false values.

---

## Phase 5: User Story 3 — Fail-secure when visibility lookup fails (Priority: P2)

**Goal**: When `ghRepoVisibility` returns an error (HTTP 403, 404, 5xx, network failure, malformed JSON, missing field), the resolver returns `(true, "auto-fallback", reason)` and the deploy proceeds with strict ON. One FR-007 warn-log line emits naming the repo and the truncated reason.

**Independent Test**: Stub `ghRepoVisibility` to return an error. Run `runCompileStrictIfNeeded` against a `RepoSpec` with `CompileStrict = nil`. Assert: returned err is nil (deploy proceeds), `Applied == true`, `Source == "auto-fallback"`, the compile seam IS invoked, and exactly one warn-level structured log event `compile_strict_lookup_failed` fires with `repo` and `reason` fields.

**Note on entanglement**: US3's implementation is entirely covered by T003 (resolver returning fail-secure) plus T010 (warn-log emission). This phase is test-only.

### Tests for User Story 3

- [X] T020 [P] [US3] In `internal/fleet/deploy_test.go`, add `TestDeploy_VisibilityLookupFails_FailSecureStrictOn` — override `ghRepoVisibility` to return `errors.New("HTTP 403 Forbidden")`, `ghAwCompileHelp` to return `"--strict"`, `runGhAwCompileStrict` to return success. Capture zerolog output to `&buf`. Call `runCompileStrictIfNeeded`. Assert: returned err is nil, `Applied == true`, `Source == "auto-fallback"`, compile seam invoked once, AND captured log contains one `compile_strict_lookup_failed` warn event with `reason` field containing `"403"`. Depends on T010.
- [X] T021 [P] [US3] In `internal/fleet/upgrade_test.go`, add `TestUpgrade_VisibilityLookupFails_FailSecureStrictOn` — symmetric to T020 for the upgrade path. Depends on T012.

**Checkpoint**: US3 verified. Fail-secure semantics work on both deploy and upgrade; the warn-log surface is contract-tested.

---

## Phase 6: User Story 4 — Compile / probe failures surface actionable hints (Priority: P2)

**Goal**: When `gh aw compile --strict` fails (FR-009), or the probe detects `--strict` is missing (FR-016 flag-absent), or the probe itself fails (FR-016 probe-failed), the deploy/upgrade aborts cleanly: clone preserved, no PR opened, error chain contains an actionable `CollectHints` entry with the appropriate diagnostic code.

**Independent Test**: Three failure-mode fixtures — (a) `runGhAwCompileStrict` returns non-zero with stderr containing `"strict mode validation failed"`; (b) `ghAwCompileHelp` returns output WITHOUT `"--strict"`; (c) `ghAwCompileHelp` returns `errors.New("executable not found")`. For each: assert the wrapped error contains the expected diagnostic code, no PR is created, and the work-dir clone exists on disk after the error.

**Note on entanglement**: US4's implementation is in T010 (the helper's failure-branch wiring). This phase is test-only.

### Tests for User Story 4

- [X] T022 [P] [US4] In `internal/fleet/deploy_test.go`, add `TestDeploy_CompileFails_EmitsDiagCompileStrictFailed` — `ghRepoVisibility` returns `"public"`, `ghAwCompileHelp` returns `"--strict"`, `runGhAwCompileStrict` returns `("strict mode validation failed for workflow foo.md", errors.New("exit 1"))`. Call `runCompileStrictIfNeeded`. Assert: returned err is non-nil, the wrapped error's `CollectHints()` output (or equivalent error-message scan) contains the string `compile_strict_failed` and the `"compile_strict": false`-mentioning hint text, the wrapped error's `Error()` string ALSO contains the literal raw stderr fixture `"strict mode validation failed for workflow foo.md"` per FR-009 ("Raw compile stderr MUST still be included in the error"), AND `res.CompileStrictApplied == false`. Verify the temp clone dir from `t.TempDir` still exists (filesystem `os.Stat` returns success — `t.TempDir` cleans up after the test, but the helper itself MUST NOT delete it). Depends on T010.
- [X] T023 [P] [US4] In `internal/fleet/deploy_test.go`, add `TestDeploy_ProbeFlagAbsent_EmitsDiagGhAwTooOld` — `ghRepoVisibility` returns `"public"`, `ghAwCompileHelp` returns a string LACKING `"--strict"` (e.g., `"Usage: gh aw compile [flags]\n  --some-other-flag\n"`), `ghAwVersion` returns `"v0.50.0"`. Assert: returned err is non-nil, error message contains `gh_aw_too_old`, contains `v0.68.3` (the minimum), contains the detected `v0.50.0`, AND the compile seam was NEVER invoked. Depends on T010.
- [X] T024 [P] [US4] In `internal/fleet/deploy_test.go`, add `TestDeploy_ProbeFailed_EmitsDiagGhAwMissing` — `ghRepoVisibility` returns `"public"`, `ghAwCompileHelp` returns `("", errors.New("exec: \"gh\": executable file not found in $PATH"))`. Assert: returned err is non-nil, error message contains `gh_aw_missing`, contains `gh extension install`, AND the compile seam was NEVER invoked. Depends on T010.
- [X] T025 [P] [US4] In `internal/fleet/upgrade_test.go`, add `TestUpgrade_CompileFails_EmitsDiagCompileStrictFailed`, `TestUpgrade_ProbeFlagAbsent_EmitsDiagGhAwTooOld`, and `TestUpgrade_ProbeFailed_EmitsDiagGhAwMissing` as three separate test functions mirroring T022-T024 against the upgrade pipeline. The compile-fails test MUST also assert raw-stderr preservation in the wrapped error per FR-009 (symmetric with T022). Depends on T012.

**Checkpoint**: US4 verified. All three failure modes produce clean diagnostic output, the clone is preserved, no PR is opened. The operator's failure UX is contract-tested.

---

## Phase 7: User Story 5 — Onboarding feedback at `fleet add` time (Priority: P3)

**Goal**: `gh-aw-fleet add <owner/repo>` prints one stdout info line after the existing add output, describing the deploy-time strict policy that will apply on the next deploy. Suppressed when the visibility lookup fails.

**Independent Test**: Override `ghRepoVisibility` to return `"public"` and run the `add` command against a fixture repo. Assert: stdout contains the auto-on info line naming the override key. Repeat for `"private"` → auto-off line. Repeat for error → no info line, add still exits 0.

**Note on entanglement**: US5 is the only user story with genuinely independent new code.

### Implementation for User Story 5

- [X] T026 [US5] In `cmd/add.go`, after the existing add operation succeeds (look for the function that prints the add-confirmation line to stdout), call `fleet.GhRepoVisibility(ctx, repo)` (or the unexported `ghRepoVisibility` seam if accessible) and print one of three messages on stdout: (a) `"public repo: workflows will be compiled with --strict on next deploy (auto-on; override with \"compile_strict\": false in fleet.local.json)"` when visibility is `"public"`; (b) `"private repo: workflows will NOT be compiled with --strict on next deploy (auto-off; override with \"compile_strict\": true in fleet.local.json)"` for non-public; (c) NO output when the lookup returns an error (suppressed per FR-010). Use `fmt.Fprintln(cmd.OutOrStdout(), ...)` so tests can capture via `cmd.SetOut(&buf)`. The add command itself MUST still exit 0 on lookup error. Depends on T004 (the seam must exist and be accessible from `cmd/`).

### Tests for User Story 5

- [X] T027 [P] [US5] In `cmd/add_test.go` (create if not present, follow the pattern of any existing `cmd/*_test.go`), add `TestAdd_PublicRepo_PrintsAutoOnInfoLine` — override `ghRepoVisibility` to return `"public"`, capture cobra stdout via `cmd.SetOut(&buf)`, run the add command. Assert buf contains the literal substring `auto-on` AND `"compile_strict": false`. Depends on T026.
- [X] T028 [P] [US5] In `cmd/add_test.go`, add `TestAdd_PrivateRepo_PrintsAutoOffInfoLine` — `ghRepoVisibility` returns `"private"`. Assert stdout contains `auto-off` AND `"compile_strict": true`. Depends on T026.
- [X] T029 [P] [US5] In `cmd/add_test.go`, add `TestAdd_VisibilityLookupFails_SuppressesInfoLine` — `ghRepoVisibility` returns `errors.New("network error")`. Assert stdout does NOT contain `auto-on` or `auto-off`, AND the add command exited 0. Depends on T026.

**Checkpoint**: All five user stories are independently verified. The MVP slice (Phases 1-3) is releasable; Phases 4-7 add progressive confidence and the onboarding ergonomic.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Documentation updates and the final CI gate. These tasks are not story-specific and complete the feature.

- [X] T030 [P] Update `README.md`: add a new subsection (under the existing config-reference section, or create one if absent) titled "Compile-strict resolution" documenting (a) the three-state semantics of `compile_strict` (auto / true / false), (b) the resolution order from FR-003, (c) the minimum `gh aw` version (`v0.68.3`) and how to upgrade, (d) the four resolution-source values (`explicit`, `auto-public`, `auto-private`, `auto-fallback`) and what each means for CI consumers. Include a copy-pasteable `fleet.local.json` snippet showing both override directions. Cross-link to `specs/010-compile-strict-public/quickstart.md` for the full troubleshooting walk-through.
- [X] T031 [P] Update `skills/fleet-deploy/SKILL.md` to note the auto-strict behavior in the three-turn pattern: between the dry-run and `--apply` steps, the operator now sees a `compile_strict_resolved` info log line that names the policy that will apply on `--apply`. The skill's three-turn template stays unchanged in shape; only the dry-run "what to look for" section gains a bullet about the new log line.
- [X] T032 Run `make ci` locally (or wait for CI on push). Confirm gofmt, `go vet`, `golangci-lint`, and the full test suite all pass. No new `//nolint` suppressions allowed beyond the `gochecknoglobals` markers introduced by T004. This is SC-008's verification.
- [X] T033 Manual smoke test (REQUIRES OPERATOR): run `gh-aw-fleet deploy rshade/gh-aw-fleet` (no `--apply`) against the canonical public test target. Confirm the stderr output contains exactly one `compile_strict_resolved` info event with `source=auto-public effective=true`, no warnings or errors fire, and the dry-run completes successfully. Then run with `--apply` and confirm the compile step's stdout/stderr is visible in the operator's terminal during compile (FR-011 tee verification — the production `runGhAwCompileStrict` impl uses the `runLoggedOutput` pattern from `runUpgrade`, which cannot be exercised offline through the seam, so this is the contractual verification point). This is SC-001's manual verification — the integration tests cover the wire-level behavior; this confirms the live `gh api` + `gh aw compile --help` + `gh aw compile --strict` calls work in the operator's environment.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: Intentionally empty. No work blocks Foundational.
- **Foundational (Phase 2)**: Depends on no prior work. T001+T002 are leaves; T003 needs T002+T004; T005 needs T004; T007 needs T001. All Phase 2 tasks must complete before any Phase 3+ task begins.
- **User Stories (Phases 3-7)**:
  - US1 (Phase 3): Depends on Foundational. Must complete before US3 (Phase 5), US4 (Phase 6), and US5 (Phase 7) testing because those phases reference the helper introduced by T010.
  - US2 (Phase 4): Depends on Foundational + T010 (US1 helper). Does NOT depend on T011 (deploy-wiring) because the tests call `runCompileStrictIfNeeded` directly without going through `Deploy()`.
  - US3 (Phase 5): Depends on US1 (T010 implements the warn-log path that US3 tests).
  - US4 (Phase 6): Depends on US1 (T010 implements the failure-branch wiring that US4 tests).
  - US5 (Phase 7): Depends on Foundational T004 only — it adds new code in `cmd/add.go` independent of US1's helper.
- **Polish (Phase 8)**: T030+T031 depend on user-visible behavior being implemented (Phases 3-7). T032 depends on everything else. T033 is the last step.

### Within Each Phase

- T001 [P] || T002 [P] → T004 → T003 (needs T002 + T004's `ghRepoVisibility` seam) → T005 → T006 [P] (different file than T005) → T007 (after T001) → T008 [P] || T009 [P]
- T010 → T011 → T012 [P] || T013 [P] || T014 [P]
- T015 (after T011) → T016 [P]
- T017 [P] || T018 [P] || T019 [P] (all after T010 / T002)
- T020 [P] (after T010) || T021 [P] (after T012)
- T022 [P] || T023 [P] || T024 [P] (all after T010) || T025 [P] (after T012)
- T026 (after T004) → T027 [P] || T028 [P] || T029 [P]
- T030 [P] || T031 [P] → T032 → T033

### Parallel Opportunities

- T001 + T002 can run in parallel (different files, no shared deps).
- T006 + (T010 once T005 lands) — different files, parallel after their respective predecessors.
- T012 + T013 + T014 — three separate files (`upgrade.go`, `cmd/deploy.go`, `cmd/upgrade.go`), parallel after T010-T011.
- T015 + T016 — separate test files, parallel after T011/T012.
- All Phase 4-6 tests are append-only additions to `deploy_test.go` and `upgrade_test.go` with distinct test function names; they can be developed concurrently after T010-T012 land.
- T027-T029 — separate test functions in `cmd/add_test.go`, parallel after T026.
- T030 + T031 — different documentation files, parallel.

---

## Parallel Example: Foundational Phase

```bash
# Run in parallel (T001 and T002 are leaves):
Task: "Add diagnostic codes in internal/fleet/fleetdiag/diag.go + re-exports in internal/fleet/diagnostics.go"
Task: "Add RepoSpec.CompileStrict field in internal/fleet/schema.go"

# After T002 lands, write T004 FIRST (T003 needs the ghRepoVisibility seam):
Task: "Add four injection seams in internal/fleet/deploy.go"             # needs nothing
Task: "Add EffectiveCompileStrict resolver in internal/fleet/schema.go"  # needs T002 + T004

# After T005 lands, parallel with T006 (different file) and T007 (needs T001):
Task: "Add CompileStrict* fields to UpgradeResult in internal/fleet/upgrade.go"
Task: "Add hint patterns in internal/fleet/diagnostics.go"

# Tests in parallel (different files, no overlap):
Task: "TestEffectiveCompileStrict table-driven in internal/fleet/schema_test.go"
Task: "TestCollectHints_CompileStrictPatterns in internal/fleet/diagnostics_test.go"
```

---

## Parallel Example: US4 Tests

```bash
# All four are distinct test functions in two test files; parallel after T010+T012:
Task: "TestDeploy_CompileFails_EmitsDiagCompileStrictFailed in internal/fleet/deploy_test.go"
Task: "TestDeploy_ProbeFlagAbsent_EmitsDiagGhAwTooOld in internal/fleet/deploy_test.go"
Task: "TestDeploy_ProbeFailed_EmitsDiagGhAwMissing in internal/fleet/deploy_test.go"
Task: "Three Upgrade_* test functions in internal/fleet/upgrade_test.go"
```

---

## Implementation Strategy

### MVP First (Phases 1 → 2 → 3)

1. Complete Phase 1 (empty — no-op).
2. Complete Phase 2 (Foundational): the schema field, resolver, seams, result fields, hint patterns. Run Foundational tests (T008, T009) to validate primitives.
3. Complete Phase 3 (US1): the helper, the deploy-and-upgrade wiring, the cmd printers, the auto-public integration test.
4. **STOP and VALIDATE**: Run `make ci` to confirm the slice is green. Run a manual dry-run against `rshade/gh-aw-fleet` (per T033) and inspect the resulting info log line. This MVP is releasable.

### Incremental Delivery (Phases 4-7)

Each subsequent phase adds verification or ergonomic value atop the MVP. Order doesn't matter much — US2/US3/US4 are all test-only and US5 is independent code.

- US2 (Phase 4): adds three test functions + the loader round-trip test. Adds operator confidence in the override path.
- US3 (Phase 5): adds two test functions. Locks in the fail-secure contract for CI-environment regressions.
- US4 (Phase 6): adds six test functions (three deploy + three upgrade). Locks in the failure-mode UX contract.
- US5 (Phase 7): adds the `cmd/add.go` info line + three test functions. Improves onboarding ergonomics.

### Parallel Team Strategy

With multiple developers and a team budget:

1. **Day 1**: Pair on T001-T009 (Foundational). All primitives land in one PR-able batch.
2. **Day 2**:
   - Developer A: T010 → T011 → T015 (US1 deploy MVP + test).
   - Developer B: T012 → T016 (US1 upgrade MVP + test).
   - Developer C: T013 + T014 (cmd printers — depends on T005/T006 which is already done).
3. **Day 3**:
   - Developer A: T017-T019 (US2 tests).
   - Developer B: T020-T021 (US3 tests).
   - Developer C: T022-T025 (US4 tests).
4. **Day 4**: Pair on T026-T029 (US5 implementation + tests).
5. **Day 5**: T030-T033 (polish + final CI gate + manual smoke).

Three-developer pipeline completes the feature in ~5 working days. Solo developer can sequence everything in ~7-10 working days.

---

## Notes

- [P] tasks may target the same test file as long as they add distinct test functions with non-overlapping line ranges (consistent with the precedent in `specs/009-consumption-subcommand/tasks.md`).
- [Story] label maps task to specific user story for traceability against the spec's user stories.
- Every user story EXCEPT US1 is independently testable atop the MVP (Foundational + US1). US3 and US4 test behavior that ships in US1's helper; US2 tests the resolver branch logic; US5 tests its own new code in `cmd/add.go`.
- The work-dir clone preservation contract is owned by the existing Deploy/Upgrade failure-handling code — `runCompileStrictIfNeeded` only returns the error, it does NOT delete the clone (T010).
- Sync's compile coverage is **transitive** — `applyDeployOrPrune` in `sync.go` delegates to `Deploy`, which inherits the new helper. No sync-specific test is included in this slice; if a future refactor changes `applyDeployOrPrune`'s shape to bypass `Deploy`, the FR-004 chain breaks silently. Reviewers of any future sync refactor SHOULD re-verify `SyncResult.Deploy.CompileStrictApplied` populates correctly on a public-repo dry-run.
- All new `var func(...)` seams MUST have a `//nolint:gochecknoglobals` comment naming the test-injection rationale, matching the existing pattern in `internal/fleet/fetch.go:183`.
- No `cmd.SchemaVersion` bump and no `fleet.SchemaVersion` bump (both confirmed in plan.md). New fields are additive.
- The compile step is invoked once per Deploy/Upgrade on the entire `.github/workflows/` directory (no per-workflow arguments) — this is the natural `gh aw compile` invocation shape and is intentional per spec.md Edge Case "Multi-workflow runAdd loop."
- T033 (manual smoke) is the only task requiring a live operator + live `gh api`. Everything else runs offline via the four injection seams.
