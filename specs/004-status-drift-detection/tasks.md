---
description: "Task list for status subcommand drift detection (feature 004-status-drift-detection)"
---

# Tasks: `status` Subcommand for Drift Detection

**Input**: Design documents from `/specs/004-status-drift-detection/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-surface.md, contracts/json-envelope.md

**Tests**: Tests are INCLUDED. The spec's Testing Strategy and research R10 mandate table-driven unit tests for the pure diff function (≥7 fixture cases) plus an orchestrator test using a stub fetcher. Manual integration validation is documented in the final phase.

**Organization**: Tasks are grouped by user story (US1 = fleet-wide drift, US2 = single-repo drill-down, US3 = JSON output). Each story is independently testable and deliverable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

This is a single-module Go project. All paths are relative to the repository root `/mnt/c/GitHub/go/src/github.com/rshade/gh-aw-fleet/`.

- Cobra wrappers live in `cmd/`
- Feature logic lives in `internal/fleet/`
- Tests sit beside their subject (`*_test.go`)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Prepare the diagnostic layer and confirm the spec-003 envelope helpers status will consume.

- [X] T001 [P] Add new diagnostic code constants to `internal/fleet/diagnostics.go`: `DiagRateLimited = "rate_limited"`, `DiagRepoInaccessible = "repo_inaccessible"`, and (optional, see research R5) `DiagNetworkUnreachable = "network_unreachable"`. Append matching `Hint` entries to the `hints` slice for substring patterns `"API rate limit exceeded"` (→ `DiagRateLimited`) and optionally `"Could not resolve host"` (→ `DiagNetworkUnreachable`). Do NOT widen the existing `"HTTP 404"` hint message — status emits a structural per-repo `repo_inaccessible` diagnostic instead (constructed at the call site, not via `CollectHintDiagnostics`).
- [X] T002 [P] Add a compile-time helper-binding test `cmd/status_envelope_binding_test.go` (new file) that imports each spec-003 envelope helper status will call (`writeEnvelope`, `preResultFailureEnvelope`, `outputMode`, `validateOutputMode`, `ensureFailureHint`) by referencing them from a no-op test (`var _ = writeEnvelope` — or call with zero values inside `t.Skip()`). If any helper is missing or its signature drifted from `specs/004-status-drift-detection/contracts/json-envelope.md`, the test fails to compile and the PR cannot land — making the verification a hard gate rather than a documentation step. Includes a one-line comment pointing at this file from the contracts doc.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Land the types, the fetcher seam, and the pure drift-computation function. Every user story depends on these.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T003 Create `internal/fleet/status.go` with the package comment line and the exported type definitions per data-model.md (§Type 1 through §Type 5). Define: `StatusOpts`, `StatusResult` (one field: `Repos []RepoStatus`), `RepoStatus` (fields `Repo`, `DriftState`, `Missing`, `Extra`, `Drifted`, `Unpinned`, `ErrorMessage` — JSON tags exactly as listed in data-model.md §Type 3), and `WorkflowDrift` (fields `Name`, `DesiredRef`, `ActualRef`). Add the unexported `statusJob` struct, the `statusFetcher` interface with two methods `listWorkflowsDir(ctx, repo)` and `fetchWorkflowBody(ctx, repo, file)`, and `const statusWorkerPoolSize = 6`. Each exported identifier MUST have a godoc comment per the repo's CLAUDE.md self-documentation rule. No logic yet — types only.
- [X] T004 Implement the production `statusFetcher` binding in `internal/fleet/status.go` as an unexported struct (e.g., `ghStatusFetcher`) whose `listWorkflowsDir(ctx, repo)` calls `ghAPIJSON(ctx, "/repos/<repo>/contents/.github/workflows")` (no `?ref=` per research R2), filters the result to `.md` files, and returns the names; whose `fetchWorkflowBody(ctx, repo, file)` calls `ghAPIRaw(ctx, "/repos/<repo>/contents/.github/workflows/<file>")` and returns the body string. On error, wrap with `fmt.Errorf("...: %w", err)`. The fetcher methods MUST be safe for concurrent use (they are by virtue of `exec.Command`'s isolation).
- [X] T005 Implement the pure `computeDrift` function in `internal/fleet/status.go` with signature `func computeDrift(repo string, declared []ResolvedWorkflow, listing []string, fetchedBodies map[string]string, fetchErr error) RepoStatus`. Apply the mutual-exclusivity table from data-model.md §Type 3:
  - If `fetchErr != nil` → `RepoStatus{Repo: repo, DriftState: "errored", ErrorMessage: fetchErr.Error()}` (drift slices stay empty).
  - For each declared workflow `d`: if its `<name>.md` is NOT in `listing` → append to `Missing`. If present, fetch body from `fetchedBodies`; run `SplitFrontmatter` + `ParseFrontmatter`; read `source:` value; extract ref segment after `@`. If parse fails or `source:` missing/non-string/no `@` → append to `Unpinned`. If ref differs from `d.Ref` (strict string equality per FR-004) → append `WorkflowDrift{Name, DesiredRef: d.Ref, ActualRef}` to `Drifted`. Otherwise aligned (no slice).
  - For each `.md` in `listing` whose name (minus `.md`) is NOT in the declared set: parse its `source:` frontmatter; if parseable → append name to `Extra`; if missing/malformed → ignore (not gh-aw managed, per spec Edge Cases).
  - Sort each slice deterministically (`sort.Strings` for `Missing` / `Extra` / `Unpinned`; `sort.Slice` by `.Name` for `Drifted`).
  - Compute `DriftState` from the rule: any non-empty drift slice → `"drifted"`, else `"aligned"`. `ErrorMessage = ""` in both cases.
  - Return the populated `RepoStatus`. No goroutines, no `gh api`, no I/O — this function is pure and table-testable.

**Checkpoint**: Foundation ready — types compile, the fetcher seam exists, the pure diff function is callable. User-story phases can now begin.

---

## Phase 3: User Story 1 - Fleet-wide drift summary without cloning (Priority: P1) 🎯 MVP

**Goal**: An operator runs `gh-aw-fleet status` and gets a per-repo drift summary across the entire loaded fleet, with no clones, with exit code `0` only if every repo is `aligned`.

**Independent Test**: Run `gh-aw-fleet status` against a fleet whose state is partially drifted (one repo missing a declared workflow, one repo pinned to an old ref, one repo aligned). Output identifies all three repos correctly, exits `1`, and creates no `/tmp/gh-aw-fleet-*` directories during the run.

### Tests for User Story 1

- [X] T006 [US1] Create `internal/fleet/status_test.go` with table-driven tests for `computeDrift`. Per research R10, cover at least these 7 cases as `t.Run` subtests with a struct fixture `{name string, declared []ResolvedWorkflow, listing []string, fetchedBodies map[string]string, fetchErr error, want RepoStatus}`:
  1. **aligned**: one declared workflow, present in listing, frontmatter `source:` ref equals desired → `DriftState: "aligned"`, all slices empty.
  2. **missing**: one declared workflow, NOT in listing → `Missing: [name]`, `DriftState: "drifted"`.
  3. **drifted**: one declared workflow, present, `source:` ref differs → `Drifted: [{name, desired, actual}]`, `DriftState: "drifted"`.
  4. **extra**: zero declared, one `.md` in listing with parseable `source:` → `Extra: [name]`, `DriftState: "drifted"`.
  5. **unpinned (missing source)**: one declared workflow, present, frontmatter has no `source:` field → `Unpinned: [name]`, `DriftState: "drifted"`.
  6. **unpinned (malformed yaml)**: one declared workflow, present, frontmatter is invalid YAML → `Unpinned: [name]`, `DriftState: "drifted"`.
  7. **errored**: `fetchErr != nil` → `DriftState: "errored"`, `ErrorMessage` set, drift slices empty.
   Tests MUST run without network (no `gh api` invocations, no goroutines spawned).
- [X] T007 [US1] In `internal/fleet/status_test.go`, add an orchestrator test for `Status()` (T008) using an injected fake `statusFetcher`. The fake returns deterministic listings/bodies/errors per repo. The test asserts on the three return values of `Status()` (`*StatusResult`, `[]Diagnostic`, `error`). Assert: (a) all repos in `cfg.Repos` produce a `RepoStatus` in `result.Repos`; (b) result is sorted alphabetically by repo; (c) per-repo errors do NOT abort siblings (FR-009); (d) the worker pool runs at most `statusWorkerPoolSize` repo fetches concurrently (use a counting semaphore in the fake to assert this); (e) workflow fetches WITHIN one repo are serial (assert call ordering on the fake); (f) a config with a broken profile reference (one repo whose `cfg.ResolveRepoWorkflows(repo)` errors) produces a `RepoStatus{DriftState: "errored", ErrorMessage: ...}` for that repo AND a corresponding `Diagnostic` in the second return value, while sibling repos still produce normal drift reports; (g) errored repos produce a per-repo `Diagnostic` in the second return value with `Code: DiagRepoInaccessible` and `Fields: {"repo": <name>}`; **(h) (C3 — FR-017 no-mutation enforcement):** the fake `statusFetcher` is implemented as a recorder that logs every method invocation; after `Status()` returns, assert the recorded call set contains ONLY `listWorkflowsDir` and `fetchWorkflowBody` invocations (no mutating method names). Additionally, assert that the working directory's `os.Getwd()` content listing is unchanged before/after the call (no file writes — the in-package `internal/fleet` test sandbox doubles as a write-detector). This is the FR-017 enforcement at the unit-test layer; T021 supplements with the no-clone integration check. Suggest naming the injection seam: a package-level var `statusFetcherFactory` defaulting to the production binding from T004, overridable in tests.

### Implementation for User Story 1

- [X] T008 [US1] Implement `Status(ctx context.Context, cfg *Config, opts StatusOpts) (*StatusResult, []Diagnostic, error)` in `internal/fleet/status.go`. The three returns: (1) `*StatusResult` carries the per-repo wire payload; (2) `[]Diagnostic` carries per-repo `errored` diagnostics (one per failed repo, `Code: DiagRepoInaccessible` or `DiagRateLimited`, `Fields: {"repo": <name>}`) plus fleet-wide warnings (e.g., a `Diagnostic{Code: "empty_fleet"}` when `len(cfg.Repos) == 0`) — `cmd/status.go` splits these into the envelope's `warnings[]` vs `hints[]` by `Code`; (3) `error` is reserved for setup-time failures (config load, single-repo arg validation per FR-008) that prevent constructing a `StatusResult`. Build the list of `statusJob`s from `cfg.Repos` (alphabetically sorted keys → deterministic worker dispatch). Spawn `statusWorkerPoolSize` worker goroutines reading from a buffered jobs channel; each worker runs the per-repo logic (T009) serially for its dequeued repo. Collect both results AND per-repo diagnostics on parallel channels; after all workers exit (`sync.WaitGroup.Wait()`), drain into `StatusResult.Repos` (sorted alphabetically by `Repo`) and `[]Diagnostic` (sorted by `Fields.repo` for determinism). `Status()` itself NEVER returns a non-nil `error` for per-repo failures — those become `RepoStatus.ErrorMessage` AND a `Diagnostic` in the second return.
- [X] T009 [US1] Implement the per-repo worker logic in `internal/fleet/status.go` as an unexported free function `processRepo(ctx context.Context, fetcher statusFetcher, repo string, declared []ResolvedWorkflow) RepoStatus`. Sequence: (1) call `fetcher.listWorkflowsDir(ctx, repo)`; if error → return `computeDrift(repo, declared, nil, nil, err)` (errored). (2) For each declared `<name>` whose `<name>.md` IS in the listing, call `fetcher.fetchWorkflowBody(ctx, repo, "<name>.md")`; collect bodies by name. (3) For each `.md` in listing NOT in the declared set, fetch body too (needed by `computeDrift` to filter "extra" by parseable `source:`). (4) Call `computeDrift(repo, declared, listing, bodies, nil)`. Per FR-018: workflow fetches within one repo are serial. Per FR-009: a single workflow-fetch error should still produce a per-repo errored RepoStatus (not partial drift); short-circuit on the first per-workflow fetch error and surface it as `ErrorMessage`. (Free function rather than a method receiver — no orchestrator state is shared across calls; the fetcher is the only dependency and is passed in.)
- [X] T010 [US1] Replace the stub in `cmd/stubs.go` with a real `cmd/status.go`: delete `newStatusCmd()` from `cmd/stubs.go`, then create `cmd/status.go` with a new `newStatusCmd(flagDir *string)` that mirrors `cmd/sync.go`'s wiring shape. Cobra command: `Use: "status [repo]"`, `Args: cobra.MaximumNArgs(1)`, `Short` from contracts/cli-surface.md. RunE flow: (1) load cfg via `fleet.LoadConfig(*flagDir)`; (2) print breadcrumb `(loaded ...)` to stderr; (3) build `StatusOpts{Repo: <arg or "">}`; (4) call `result, diags, err := fleet.Status(ctx, cfg, opts)` — on non-nil `err` (setup-time failure: bad cfg, repo-not-in-fleet) return it directly so Cobra exits non-zero; (5) text mode: render the tabwriter table per contracts/cli-surface.md (columns `REPO STATE MISSING EXTRA DRIFTED UNPINNED`), then a per-drifted-repo detail block (drifted: `name desired_ref → actual_ref`; missing/extra/unpinned: list); errored repos get a single `error: <message>` line under the table; emit any fleet-wide warnings from `diags` (e.g., `empty_fleet`) on stderr via zerolog at `warn` level — the per-repo `error_message` rendering already covers errored-repo visibility on stdout, so the `diags` slice is consumed only for non-per-repo warnings in text mode; (6) compute exit code: return `nil` when every `RepoStatus.DriftState == "aligned"`, otherwise return a non-nil error so Cobra exits `1` (mirror the pattern in `cmd/sync.go`). Update `cmd/root.go` if needed so the registration line points at the new constructor (the existing `newStatusCmd()` call site stays — only its definition moves).
- [X] T011 [US1] Wire per-repo diagnostic emission into the worker (`internal/fleet/status.go`): when a per-repo fetch fails, build a `Diagnostic` directly at the call site with `Code: DiagRepoInaccessible` (or `DiagRateLimited` if the error message matches `"API rate limit exceeded"`), `Message` describing the failure, and `Fields: map[string]any{"repo": repo}`. Each worker emits a `RepoStatus` AND (when errored) a `Diagnostic` on parallel results channels; the orchestrator (T008) collects both and surfaces the diagnostic slice as `Status()`'s second return value (see data-model.md Type 2 lifecycle). Do NOT route through `CollectHintDiagnostics` for repo-level structural errors — the substring-match model can't disambiguate workflow-404 from repo-404 (research R5), and `CollectHintDiagnostics` always emits `Fields: {"hint": <message>}` and does not accept the `Fields.repo` augmentation. For non-repo-level error strings (e.g., gh-aw-style messages embedded in `gh api` stderr that match a hint pattern like `Unknown property:`), it's fine to call `CollectHintDiagnostics` as a fallback alongside the structured per-repo diagnostic.

**Checkpoint**: At this point, US1 is fully functional. `gh-aw-fleet status` against the project's own fleet returns a real drift report in text mode. MVP shippable.

---

## Phase 4: User Story 2 - Targeted single-repo drift check (Priority: P2)

**Goal**: `gh-aw-fleet status owner/repo` returns drift for one repo only, with the same categories and exit-code semantics as US1, validating the repo is in the fleet config BEFORE any GitHub API call.

**Independent Test**: Run `gh-aw-fleet status rshade/gh-aw-fleet` against a known drifted repo. Exactly one repo's drift report is emitted; unrelated repos in fleet.local.json are not queried (verifiable by `gh api` call count via `--log-level debug`). A second invocation `gh-aw-fleet status some/unknown` exits non-zero with the message `repo "some/unknown" is not declared in fleet config`, and zero `gh api` calls are issued.

### Tests for User Story 2

- [X] T012 [US2] In `internal/fleet/status_test.go`, add subtests covering the single-repo path of `Status()`: (a) `StatusOpts{Repo: "owner/repo"}` with the repo present in cfg → only one entry in `result.Repos`, fake fetcher invoked exactly once for that repo; (b) `StatusOpts{Repo: "owner/notinfleet"}` → `Status()` returns a non-nil error before invoking the fake fetcher (assert zero fetcher calls).

### Implementation for User Story 2

- [X] T013 [US2] Extend `Status()` in `internal/fleet/status.go` to honor `StatusOpts.Repo`: when non-empty, validate it appears in `cfg.Repos` first; if not, return `(nil, nil, fmt.Errorf("repo %q is not declared in fleet config", opts.Repo))` BEFORE any fetcher calls (FR-008) — this is a setup-time failure routed through the third (`error`) return per data-model.md Type 2 lifecycle. The error string MUST match the contract example in `contracts/cli-surface.md` and `contracts/json-envelope.md` (no `fleet:` prefix). When the repo IS in cfg, build a single-element `statusJob` list rather than iterating every repo. The worker pool can still be used (with one in-flight job); no separate code path needed.
- [X] T014 [US2] In `cmd/status.go`, when a positional arg is supplied: pass `args[0]` as `StatusOpts.Repo`. On the validation error from T013, in text mode return the error (Cobra prints to stderr and exits non-zero); JSON-mode handling lands in US3 (T016). The Cobra-level arg validation (`cobra.MaximumNArgs(1)`) already rejects 2+ args at T010.

**Checkpoint**: US1 + US2 both work. Operators can run fleet-wide checks AND drill into a single repo, with proper "repo not in fleet" guard.

---

## Phase 5: User Story 3 - Machine-readable JSON output for CI and dashboards (Priority: P3)

**Goal**: `gh-aw-fleet status -o json` emits a single envelope conforming to spec 003's shape with `command: "status"`, `result: {repos: [...]}`, empty arrays as `[]` (never `null`), and per-repo errors surfaced both in `result.repos[].error_message` and in `hints[]` with stable codes.

**Independent Test**: Run `gh-aw-fleet status -o json` against a known-drifted fleet; pipe to `jq -e .schema_version` (succeeds), `jq -e '.result.repos | length > 0'` (succeeds), `jq -e '.result.repos[0].drift_state'` (returns `aligned` / `drifted` / `errored`), and `jq -e '.result.repos[] | .missing | type == "array"'` (always `array`, never `null`). Run against an unreachable repo and assert `hints[]` contains a `repo_inaccessible` diagnostic with `fields.repo` set.

### Tests for User Story 3

- [X] T015 [P] [US3] In `cmd/status_test.go` (new file), table-driven JSON-envelope tests calling `emitStatusEnvelope` (the helper T017 defines) directly. Each fixture supplies a `*fleet.StatusResult` AND a `[]fleet.Diagnostic` (the second return from `Status()`); the helper splits diagnostics into `warnings` / `hints` by `Code`. Cover: (a) success envelope with mixed-state `StatusResult` and zero diagnostics — assert `schema_version: 1`, `command: "status"`, `apply: false`, all four drift slices serialize as `[]` when empty (FR-015); (b) errored repo with a corresponding `Diagnostic{Code: DiagRepoInaccessible, Fields: {"repo": ...}}` in `diags` → `result.repos[].error_message` populated AND a `hints[]` entry with `code: "repo_inaccessible"` and `fields.repo` set; (c) empty fleet with a `Diagnostic{Code: "empty_fleet"}` in `diags` → `result.repos: []` AND `warnings[]` contains the `empty_fleet` warning; (d) pre-result failure (LoadConfig error or unknown-repo arg) → `result: null`, hint via `preResultFailureEnvelope`. Use `bytes.Buffer` as the writer; assert exact JSON keys via `json.Unmarshal` round-trip rather than string comparison. **(C5 — FR-019 byte-identical text-mode coverage):** Add one further test `TestStatusTextOutputUnaffectedByJSONHelpers` that captures stdout from the text-mode renderer with the same fixture twice — once after `emitStatusEnvelope` has been invoked (and discarded) and once without — and asserts the two captures are byte-identical. This is the FR-019 enforcement: text mode and JSON mode are disjoint.

### Implementation for User Story 3

- [X] T016 [US3] In `cmd/status.go`, add the JSON dispatch at the top of RunE (mirroring `cmd/sync.go:28`): `jsonMode := outputMode(cmd) == outputJSON`. Route `LoadConfig` errors and the "repo not in fleet" error from T013/T014 through `preResultFailureEnvelope(cmd, "status", repoArg, false, err)` when `jsonMode` is true. Otherwise fall through to text mode as before.
- [X] T017 [US3] In `cmd/status.go`, implement `emitStatusEnvelope(cmd *cobra.Command, repoArg string, res *fleet.StatusResult, diags []fleet.Diagnostic, statusErr error) error` mirroring `emitSyncEnvelope` in `cmd/sync.go`. Split the `diags` slice (the second return from `Status()`) into the envelope's `warnings []fleet.Diagnostic` and `hints []fleet.Diagnostic` by `Code`: per-repo error codes (`DiagRepoInaccessible`, `DiagRateLimited`) go to `hints`; warning-class codes (`empty_fleet`) go to `warnings`. Call `writeEnvelope(cmd, "status", repoArg, false, res, warnings, hints)`. Compute exit code from `res` (any non-aligned → return non-nil error so Cobra exits `1`). When `jsonMode` is true, dispatch to `emitStatusEnvelope` after `Status()` returns; the text-mode path is unchanged (FR-019: byte-identical regardless of JSON path existence).
- [X] T018 [US3] Confirm `initSlices` (in `cmd/output.go`) walks into `StatusResult.Repos[].Missing`/`Extra`/`Drifted`/`Unpinned` and replaces nil slices with empty (FR-015). The reflective walker already handles slice-of-struct + slice-of-string via the kind switch, but exercise it once with a fixture in `cmd/status_test.go` (expand T015) to confirm — if a `RepoStatus` is constructed with `Missing: nil` programmatically, the JSON output must still be `"missing": []`.

**Checkpoint**: All three user stories functional. Text mode unaffected by JSON code paths; JSON mode parses cleanly with `jq -e`; CI gate pattern `status && deploy --apply` works end-to-end.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, gate-passing, and the manual integration test required by the spec's Testing Strategy.

- [X] T019 [P] Update `README.md` to mention `status` in the CLI surface examples — at minimum, replace any "status (stub)" or omission with a one-line example like `gh-aw-fleet status` and a one-line example `gh-aw-fleet status -o json | jq ...` matching `specs/004-status-drift-detection/quickstart.md`.
- [X] T020 [P] Run `make ci` (`fmt-check`, `vet`, `lint`, `test`) from the repo root. Resolve every fmt/lint warning. Confirm `go.sum` is unchanged from `main` (SC-006: zero new third-party dependencies). Tests must pass on first run; flaky-test investigation is in scope.
- [X] T021 Manual integration test: run `gh-aw-fleet status` against the project's own fleet (`rshade/gh-aw-fleet` and any other repos in `fleet.local.json`). Compare the drift report against `gh-aw-fleet deploy --dry-run rshade/gh-aw-fleet` to confirm parity for that repo (spec Testing Strategy item 3). Confirm via `lsof` / process tracing OR by absence of `/tmp/gh-aw-fleet-*` directories that no clones occurred during the run (SC-003). Additionally verify: (i) the CI-gate chain works — run `gh-aw-fleet status; echo $?` in two states (a known-aligned fleet → exit `0`; a known-drifted fleet → exit `1`), per SC-004; (ii) status does NOT require `gh aw` — temporarily move the `gh-aw` extension out of `PATH` (e.g., `gh extension remove aw` in a scratch shell) and confirm `gh-aw-fleet status` still produces a drift report, per spec edge case "`gh aw` not installed"; **(iii) (C1 — SC-001/SC-002 wall-clock measurement):** time the fleet-wide run with `time gh-aw-fleet status` and record the result for the project's fleet (≥3 repos). Spec targets are <20s for a 10-repo fleet (SC-001) and <2s/repo when M ≤ 5 (SC-002). For fleets smaller than 10 repos, extrapolate per-repo timing via `--log-level debug` subprocess summaries and record both numbers in the PR description. If targets are missed, file a follow-up performance issue rather than blocking the merge — the targets are operator ergonomics, not hard correctness gates; **(iv) (C4 — SC-007 no-cache enforcement):** verify status writes no on-disk state by listing `~/.cache/gh-aw-fleet*`, `~/.local/state/gh-aw-fleet*`, `~/.config/gh-aw-fleet/state*`, and the working directory before vs. after a run that includes at least one errored repo. None of these paths should be created or modified. Document the run outputs in the PR description.
- [X] T022 [P] Walk through `specs/004-status-drift-detection/quickstart.md` end-to-end: each example command in §"Five-minute walkthrough" must run as documented against the project's own fleet (or fail in a way that matches §"Troubleshooting"). File any discrepancy as a docs fix in the same PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies. T001 and T002 are independent files (`internal/fleet/diagnostics.go` and `cmd/output.go` review-only) — can run in parallel.
- **Phase 2 (Foundational)**: Depends on Phase 1. Within the phase, T003 → T004 → T005 are sequential (each builds on the prior in the same file `internal/fleet/status.go`).
- **Phase 3 (US1)**: Depends on Phase 2 completion. T006 (computeDrift tests) can run as soon as T005 lands. T007 depends on T008's interface seam being defined; pragmatically write T008's signature first then iterate. T010/T011 depend on T008/T009.
- **Phase 4 (US2)**: Depends on Phase 3 (US2 reuses `Status()` and `cmd/status.go` from US1). T012 → T013 → T014 sequential.
- **Phase 5 (US3)**: Depends on Phase 3 (the text-mode path must exist before the JSON dispatch wraps it). T015 can begin once T017's helper signature is fixed; T016/T017/T018 sequential.
- **Phase 6 (Polish)**: Depends on US1 + US2 + US3 complete. T019 and T020 / T022 parallel (different files); T021 is the manual integration step run after all code is in.

### User Story Dependencies

- **US1 (P1)**: Self-contained MVP. Depends only on Phase 2.
- **US2 (P2)**: Builds on US1's `Status()` and `cmd/status.go`. Cannot start before US1's foundation is in place but does not modify US1's text rendering.
- **US3 (P3)**: Builds on US1 + US2's command shell. Adds the JSON dispatch; does NOT alter the text-mode path (FR-019).

### Within Each User Story

- Tests written first (or alongside) per the spec's call for diff-logic verification.
- Foundational types (Phase 2) before any orchestration (US1).
- Pure functions (`computeDrift`) before goroutine wiring (`Status()`).
- Logic in `internal/fleet/` before CLI wiring in `cmd/`.

### Parallel Opportunities

- T001 [P] and T002 [P] (different files).
- T015 [P] (cmd/status_test.go), T019 [P] (README.md), T020 [P] (make ci is idempotent), T022 [P] (quickstart walkthrough) once US3 lands.
- Within Phase 2, T003/T004/T005 cannot parallelize (same file, sequential build-up).
- Tests within a single story file (`internal/fleet/status_test.go`) are NOT [P] — they share a file. Order: T006 then T007 then T012.

---

## Parallel Example: Setup Phase

```bash
# Two independent file edits — run together.
Task: "T001 — Add diagnostic codes to internal/fleet/diagnostics.go"
Task: "T002 — Verify cmd/output.go envelope helpers stable"
```

## Parallel Example: Polish Phase

```bash
# Three independent finalization tasks — run together once US3 lands.
Task: "T019 — Update README.md status examples"
Task: "T020 — Run make ci (fmt-check, vet, lint, test)"
Task: "T022 — Walk quickstart.md commands end-to-end"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 (Setup) — diagnostic codes + envelope helper verification.
2. Phase 2 (Foundational) — types, fetcher, pure `computeDrift`.
3. Phase 3 (US1) — orchestrator, worker pool, text-mode `cmd/status.go`.
4. **STOP and VALIDATE**: `gh-aw-fleet status` against the project's own fleet returns a real drift report; exit code matches alignment; no `/tmp/gh-aw-fleet-*` directories appear.
5. Demo / merge MVP. Operators get fleet-wide drift detection in text mode immediately.

### Incremental Delivery

1. Setup + Foundational complete → core ready (no user-visible feature yet).
2. Add US1 → text-mode fleet-wide status — **MVP shippable**, addresses the issue's primary motivation.
3. Add US2 → single-repo drill-down + arg validation — operator UX improvement.
4. Add US3 → JSON envelope — unlocks CI/dashboard integration.
5. Polish — docs + integration test + gate.

### Parallel Team Strategy

With one developer (likely): work the phases sequentially.

With two developers post-foundational:

- Dev A: US1 + US2 (text path; sequential within itself).
- Dev B: T015 (write JSON test fixtures against the type contract) — no dependency on US1's text rendering, just on the types from Phase 2.

US3's implementation (T016/T017/T018) cannot start until US1's text path lands — they share `cmd/status.go`.

---

## Notes

- **[P] tasks**: different files, no dependencies on incomplete tasks.
- **[Story] label**: maps task to user story for traceability and selective demo.
- Tests (T006, T007, T012, T015) verify behavior empirically; the spec mandates them via Testing Strategy and research R10. Prefer table-driven `t.Run` subtests over per-case top-level functions.
- Commit after each task or logical group. Follow the repo's Conventional Commits convention with the `ci(workflows)` scope ONLY when the change touches workflow files; for the Go code in this feature, use `feat(cmd)` / `feat(fleet)` / `fix(...)` scopes — these become CHANGELOG entries via release-please.
- **Hard invariants** (per CLAUDE.md): never bypass gpg, never invoke `git add` / `git commit` / `git push` from the Bash tool, never hand-edit `CHANGELOG.md`. Status is read-only and never invokes git, so most of these don't apply mechanically — but the commit-message rule applies to every commit you make for this feature.
- Stop at any checkpoint to demo / validate / cut a partial release.
