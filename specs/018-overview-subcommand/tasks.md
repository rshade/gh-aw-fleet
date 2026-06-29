# Tasks: Fleet Overview Subcommand

**Input**: Design documents from `/specs/018-overview-subcommand/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Tests ARE in scope for this feature. The plan's Constitution Check §II ("Testing Standards") enumerates the required unit tests as deliverables, the Project Structure lists `overview_test.go` files as NEW files, and the project gate (`make ci`) runs the full suite. Test tasks are therefore included per story, but the project uses **co-located `*_test.go`** files (no separate `tests/` tree) and does not mandate strict test-first ordering — write the test alongside or just before its implementation in the same story.

**Test-function naming (required so the quickstart/T033 filters catch every test)**: every test function in `internal/fleet/overview_test.go` MUST start with `TestOverview` and every test function in `cmd/overview_test.go` MUST start with `TestOverviewCmd`. The quickstart validation (T033) runs `go test ./internal/fleet/ -run TestOverview` and `go test ./cmd/ -run TestOverviewCmd`; a test named off-prefix (e.g. `TestClassifyConclusion`) would be silently skipped by those filters and the step would false-green. Each test task below names the exact function to use.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story the task belongs to (US1–US5); Setup/Foundational/Polish carry no story label
- Exact file paths are included in every task

## Path Conventions

Single-binary Go CLI. Business logic in `internal/fleet/`, cobra wiring in `cmd/`, tests co-located as `*_test.go`. No new packages, no new third-party dependencies (FR-025).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Wire the empty command surface so later phases have anchors to fill in.

- [X] T001 [P] Add `commandOverview = "overview"` to the command-name const block in `cmd/output.go` (keep the block alphabetical: insert after `commandList` and before `commandStatus`); godoc not required (unexported const in existing block).
- [X] T002 [P] Create `internal/fleet/overview.go` with the package-scoped type skeletons from `data-model.md` — `OverviewOpts`, `OverviewResult`, `RepoOverview`, `OverviewTotal` — each field carrying its JSON tag and a full godoc sentence per CLAUDE.md "Code self-documentation"; add an `Overview(ctx context.Context, cfg *Config, opts OverviewOpts) (*OverviewResult, []Diagnostic, error)` stub returning a not-implemented error so the package compiles.
- [X] T003 Create `cmd/overview.go` with a `newOverviewCmd(flagDir *string) *cobra.Command` stub (mirrors `newConsumptionCmd` shape) and register it via a one-line `root.AddCommand(newOverviewCmd(&flagDir))` in `cmd/root.go` (alongside `newStatusCmd`/`newConsumptionCmd`). Depends on T002 (references `fleet.Overview`/`fleet.OverviewOpts`).

**Checkpoint**: `go build ./...` and `go run . overview` succeed (command exists, returns the not-implemented stub).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Factor the shared `gh aw logs` fan-out so `overview` reuses it verbatim (FR-004) and gains the `mcp_tool_usage` decode the NOOP column needs (FR-031). All changes are in `internal/fleet/consumption_logs.go` and MUST be behavior-preserving for `consumption`.

**⚠️ CRITICAL**: US1 (health/cost) and US5 (no-op) both consume `collectRepoRuns` — no story work can begin until this phase is complete and the consumption suite is still green.

- [X] T004 Extend `logsPayload` in `internal/fleet/consumption_logs.go` with `MCPToolUsage mcpToolUsage` (`json:"mcp_tool_usage"`) and define the additive `mcpToolUsage` / `mcpToolSummary` types per `data-model.md` (decoding `summary[].{server_name,tool_name,call_count}`); godoc each exported-or-not type and field. Additive — `consumption` ignores the new field.
- [X] T005 Add `noopCount(payload logsPayload) int` to `internal/fleet/consumption_logs.go` returning the `call_count` for the `{server_name:"safeoutputs", tool_name:"noop"}` summary entry (0 when absent). Depends on T004.
- [X] T006 Factor a behavior-preserving `collectRepoRuns(ctx context.Context, repo string, mode FetchMode, now time.Time) ([]repoRunData, []Diagnostic)` out of `logSourceToReports` in `internal/fleet/consumption_logs.go`, defining the `repoRunData` struct (`Workflow`, `Runs []logsRun`, `MCPToolUsage mcpToolUsage`) per `data-model.md` — it runs `ghWorkflowsAPI` → `ghLogsAPI` → `filterRunsByWindow` once per repo and yields, per workflow, the window-filtered `[]logsRun` plus the decoded `mcpToolUsage`; refactor `logSourceToReports` to call it and summarize as today. Keep the documented quirks (display-name-vs-slug, throwaway temp dir, version gate) in this one place. Depends on T004/T005.
- [X] T007 Regression guard: run `go test ./internal/fleet/ -run TestConsumption` (and any `*Logs*` tests) to prove the T006 refactor is byte-for-byte behavior-preserving for `consumption` (research.md §3, SC-006). Depends on T006.

**Checkpoint**: Consumption tests pass unchanged; `collectRepoRuns` + `noopCount` are available to both consumers.

---

## Phase 3: User Story 1 - One-command fleet health, cost, and drift dashboard (Priority: P1) 🎯 MVP

**Goal**: A single read-only command that fans out across the fleet and joins drift × run-health × cost into one row per repo, closed by a pooled `TOTAL` row, plus per-repo detail blocks for drifted/errored/unhealthy repos.

**Independent Test**: Point the tool at a 2–3 repo fixture fleet, run `overview` with no flags, and confirm one table appears with a row per repo (drift state, RUNS, FAIL, HEALTH, AIC, COST) and an aggregating TOTAL row — assembled from a single invocation, no second command or browser tab.

### Tests for User Story 1

- [X] T008 [US1] Table test `TestOverviewClassifyConclusion` for `classifyConclusion` in `internal/fleet/overview_test.go`: `success`→healthy; `failure`/`timed_out`/`startup_failure`→failed; `cancelled`/`skipped`/`""`→not-counted; unknown-terminal→failed (fail-safe) per `contracts/run-health-extraction.md` (FR-007).
- [X] T009 [US1] Reducer test `TestOverviewReducer` in `internal/fleet/overview_test.go` (same file as T008): RUNS = successes+failures, HEALTH = successes/RUNS; only-failed repo → HEALTH 0% + nil AIC/Cost (FR-010); zero health-counting runs → RUNS 0, nil HEALTH, nil AIC/Cost (FR-011).
- [X] T010 [US1] Join + per-repo isolation test `TestOverviewJoin` in `internal/fleet/overview_test.go` (same file): row set built from the declared scoped fleet; drift errored but health fetched (and vice versa) both representable; per-signal availability flags set; empty-fleet path emits `empty_fleet` diagnostic (FR-020/FR-024). Include a **partial rate-limit** subtest (spec Edge Cases): when the run-log fan-out fails mid-fleet with a rate-limit error for one repo, the already-fetched repos still render full rows and the rate-limited repo gets `RunsAvailable=false` + a `rate_limited` diagnostic with `Fields{"repo","signal":"runs"}` — the command does not abort (FR-020/SC-003).
- [X] T011 [P] [US1] Text-render test `TestOverviewCmdText` in `cmd/overview_test.go`: table columns `REPO DRIFT RUNS FAIL NOOP HEALTH AIC COST` + separator + TOTAL row; nil cells render `-` (distinct from `0`); detail block appears for drifted/errored/unhealthy repos; uses the `Status` fetcher + logs seams against fixtures (offline).

### Implementation for User Story 1

- [X] T012 [US1] Implement `classifyConclusion(c string) (healthy, failed, counted bool)` in `internal/fleet/overview.go` (failures-only taxonomy; unknown terminal → failed) per `contracts/run-health-extraction.md`.
- [X] T013 [US1] Implement the per-repo health/cost reducer in `internal/fleet/overview.go` producing `{Runs, Failures, HealthRate, AIC, Cost}` from `collectRepoRuns` output (sum AIC nil-until-positive via existing `aicToUSD`; HealthRate nil when RUNS==0). Leave `NoOps` at 0 here — US5 fills it. Depends on T012.
- [X] T014 [US1] Implement `Overview()` in `internal/fleet/overview.go`: build the row set from the scoped `cfg.Repos`; run the drift batch (`Status(ctx, cfg, StatusOpts{fetcher: opts.fetcher})`, threading the `OverviewOpts.fetcher` test seam through so drift stays offline in tests) and the health/cost batch (the `collectRepoRuns` fan-out) **concurrently in two goroutines** (research.md §5); join into `[]RepoOverview` sorted by repo + the pooled `OverviewTotal`. Populate `OverviewTotal.Aligned/Drifted/Errored` by tallying the per-repo `DriftState`s (these drive the exit code in T022 — data-model.md) and the pooled HEALTH from pooled run/failure totals, not an average (FR-012). Set `RunsAvailable`/`RunsError` and emit per-repo `repo_inaccessible`/`rate_limited` diagnostics with `Fields{"repo","signal"}` on isolated failure (FR-020/FR-024). Depends on T013.
- [X] T015 [US1] Implement the text renderer in `cmd/overview.go`: build `OverviewOpts` with the default `Mode = {FetchTrailing, Days: 7}`, call `fleet.Overview`, render the tabwriter table (`REPO DRIFT RUNS FAIL NOOP HEALTH AIC COST`) + separator + `TOTAL`, with `-` for nil cells and `aicToUSD`-derived `$` cost; write the `(loaded …)` breadcrumb and the `overview · window: …` line to **stderr** per `contracts/overview-text-output.md`. Depends on T014.
- [X] T016 [US1] Add per-repo detail blocks in `cmd/overview.go` for any repo that is `drifted`, `errored`, or unhealthy (`HEALTH < 100%` with `Runs > 0`), mirroring `printRepoDetail` in `cmd/status.go` (FR-023). Same file as T015.

**Checkpoint**: `go run . overview` renders the joined dashboard from fixtures; US1 is independently testable as the MVP.

---

## Phase 4: User Story 2 - Recency window and repo scoping (Priority: P2)

**Goal**: Default the dashboard to the trailing 7 days, expose the three mutually-exclusive temporal selectors `consumption` already has, and scope to named repos with fail-fast on unknown repos.

**Independent Test**: Run `overview` with no flag (trailing-7d, window visible), with `--trailing 7d`, and with `--since`; confirm figures cover the expected span. Name one repo → only that row + total. Pass two temporal selectors → non-zero exit with a mutual-exclusion message. Name an undeclared repo → fail fast.

### Tests for User Story 2

- [X] T017 [P] [US2] Flag test `TestOverviewCmdFlags` in `cmd/overview_test.go`: no temporal flag → `{FetchTrailing, Days:7}` and window label `trailing-7d` (FR-014); `--trailing 7d`/`--since YYYY-MM-DD` parse correctly; combining ≥2 selectors → non-zero exit + clear message (FR-015).
- [X] T018 [US2] Scoping test `TestOverviewCmdScope` in `cmd/overview_test.go` (same file as T017): named repos → only those rows + a scoped TOTAL (FR-016); undeclared repo → fail fast with `ErrRepoNotTracked` before any fetch (FR-017).

### Implementation for User Story 2

- [X] T019 [US2] In `cmd/overview.go`, add the `--latest` / `--trailing` / `--since` flags (note: `--latest` default **false**, unlike `consumption`) and a `buildFetchMode`-style helper that defaults to `{FetchTrailing, Days: 7}` when none is set, rejects combinations (mutual exclusion), and produces the `window` label string for `OverviewResult.Window`. Mirror `cmd/consumption.go`'s `buildFetchMode` + `MarkFlagsMutuallyExclusive`.
- [X] T020 [US2] In `cmd/overview.go`, accept positional `[repo...]`, call `fleet.ScopeToRepos(cfg, args)` before `fleet.Overview`, and surface `ErrRepoNotTracked` (which names the unknown repo) before any fetch (FR-017). Same file as T019.

**Checkpoint**: Temporal windowing + repo scoping work on top of the US1 dashboard; both stories independently testable.

---

## Phase 5: User Story 3 - CI-safe drift gate with advisory run health (Priority: P2)

**Goal**: Exit non-zero iff any in-scope repo is drifted or errored; zero otherwise — even with failing runs (run failures advisory).

**Independent Test**: Aligned fleet with failing runs → exit 0. Fleet with a drifted repo → exit 1. Fleet with an errored repo → exit 1. Fully aligned + all-passing → exit 0.

### Tests for User Story 3

- [X] T021 [P] [US3] Exit-code matrix test `TestOverviewCmdExitCode` in `cmd/overview_test.go`: aligned+failing-runs → 0; any drifted → 1; any errored → 1; aligned+all-pass → 0 (FR-018/FR-019).

### Implementation for User Story 3

- [X] T022 [US3] In `cmd/overview.go`, compute the exit disposition from drift only — same drift-only semantics as `statusExitCode` in `cmd/status.go`, but read the pooled `OverviewTotal` counts T014 populates: when `Total.Drifted + Total.Errored > 0`, return `newCommandExitError(<errOverviewDrift>, 1, true)` (silent); otherwise nil. Run failures never change the exit code (FR-019).

**Checkpoint**: `overview` doubles as a CI drift gate that does not flap on flaky runs.

---

## Phase 6: User Story 4 - Machine-readable output for automation (Priority: P3)

**Goal**: Emit the standard envelope (`cmd.SchemaVersion = 1`, unchanged) carrying the per-repo rows, the total, and diagnostics — JSON supported by default.

**Independent Test**: `overview --output json` against a fixture fleet validates against `contracts/overview-output.json`, contains one entry per repo + a total + diagnostics, marks an inaccessible repo as errored (present, not aborted), and honors nil-until-positive (absent AIC/Cost) — all with `schema_version` unchanged.

### Tests for User Story 4

- [X] T023 [P] [US4] JSON envelope test `TestOverviewCmdJSON` in `cmd/overview_test.go`: `schema_version == 1` and `command == "overview"`; `result` has `repos[]` + `total` + the diagnostics route through envelope `warnings[]`/`hints[]`; errored repo entry present with its diagnostic (FR-021/FR-022); only-failed repo has `aic`/`cost` absent (FR-009/SC-004).

### Implementation for User Story 4

- [X] T024 [US4] In `cmd/overview.go`, emit via `writeEnvelope(cmd, commandOverview, "", false, res, warnings, hints)` in JSON mode and do **not** call `rejectJSONMode` (opt-in-by-omission, research.md §7); ensure errored rows still serialize. Depends on T015.

**Checkpoint**: Downstream tooling can consume the joined signal as structured data with no schema bump.

---

## Phase 7: User Story 5 - Spot run-rate waste from no-op runs (Priority: P2)

**Goal**: Make the NOOP column real — a per-repo no-op count (healthy successes that took no action but still cost credits), summed from the aggregate `safeoutputs/noop` tool-usage and clamped to `[0, successes]`, surfaced alongside RUNS/FAIL/HEALTH and in the TOTAL.

**Independent Test**: A repo whose window is mostly no-ops shows a high NOOP, a high HEALTH (no-ops are successes), and non-blank cost (no-ops cost credits) — distinguishing "paying for nothing" from a genuinely idle repo (zero runs, blank cost).

### Tests for User Story 5

- [X] T025 [US5] No-op reducer test `TestOverviewNoOp` in `internal/fleet/overview_test.go`: 4 productive + 36 no-op successes + 2 failures → RUNS 42, FAIL 2, NOOP 36, HEALTH 95%, non-blank cost; all-no-op repo → RUNS==NOOP, FAIL 0, HEALTH 100%, non-blank cost; `noopRaw > successes` clamps to `successes` (FR-028/FR-029/FR-030/FR-031).
- [X] T026 [P] [US5] Create fixture `internal/fleet/testdata/logs/logs_mixed_noop.json`: mixed `success`/`failure`/`cancelled` runs (with per-success AIC) plus an `mcp_tool_usage.summary` `{safeoutputs, noop, call_count}` entry matching the no-op success count.

### Implementation for User Story 5

- [X] T027 [US5] In the `internal/fleet/overview.go` reducer (extends T013), sum `noopCount(payload)` across the repo's workflows, clamp to `[0, successes]`, and populate `RepoOverview.NoOps` + `OverviewTotal.NoOps` (FR-031 clamp).
- [X] T028 [US5] In `cmd/overview.go`, render the NOOP value in the table + TOTAL + the detail block (`(N no-op)` per `contracts/overview-text-output.md`), and emit a best-effort `DiagHint` for the `--latest` NOOP windowing caveat (research.md §2, FR-031). Same file as T015/T016.

**Checkpoint**: The run-rate-efficiency lens is visible; all five stories independently functional.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Documentation currency (FR-027 + Constitution v1.3.0 docs gate) and the full quality gate.

- [X] T029 [P] Update `README.md`: add `overview` to the command list + a short usage block (columns, trailing-7d default, drift-only exit code, advisory run failures, NOOP meaning, wall-clock note) (FR-027).
- [X] T030 [P] Create `docs/src/content/docs/overview.md` (NEW, mirroring `consumption.md`): columns, the three temporal flags + the trailing-7d default and its rationale, the drift-only exit contract, advisory run failures, the NOOP/run-rate column, and the `gh aw logs` fan-out wall-clock cost (FR-027).
- [X] T031 [P] Add the overview page to the Starlight site: link from `docs/src/content/docs/index.mdx` and add the sidebar entry in `docs/astro.config.mjs`.
- [X] T032 [P] Update `AGENTS.md` and `CLAUDE.md`: a `go run . overview` line under "Common commands" and an architecture paragraph describing the drift×health×cost join, the reused fan-out, the trailing-7d default, the drift-only exit code, and the NOOP aggregate/clamp (FR-027).
- [X] T033 Run quickstart validation: `go test ./internal/fleet/ -run TestOverview` and `go test ./cmd/ -run TestOverviewCmd` (quickstart.md) — all green, no network. The `TestOverview*` / `TestOverviewCmd*` naming required by the Tests note guarantees these filters exercise every overview test rather than silently matching none; sanity-check the run counts are non-zero (a `-run` pattern that matches no test still exits 0).
- [X] T034 Run the full local gate `make ci` (`fmt-check vet lint test`); fix any gofmt/golangci-lint findings until clean (SC-006). Markdownlint the new/edited `.md` files.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately. T003 depends on T002.
- **Foundational (Phase 2)**: Depends on Setup. BLOCKS US1 and US5 (the shared fan-out). T004→T005→T006→T007 are sequential (same file + regression guard).
- **User Stories (Phase 3–7)**: All depend on Foundational completion.
  - US1 (P1) is the MVP and the base the others extend.
  - US2 / US3 / US4 each extend `cmd/overview.go` on top of US1's renderer + `Overview()`.
  - US5 extends US1's reducer (T013) and renderer; it needs Foundational's `noopCount`.
- **Polish (Phase 8)**: Depends on all targeted stories being complete.

### User Story Dependencies

- **US1 (P1)**: Depends only on Foundational. The MVP.
- **US2 (P2)**: Depends on Foundational + US1's `cmd/overview.go` renderer and `Overview()`.
- **US3 (P2)**: Depends on Foundational + US1's `OverviewTotal` (drift counts). Independent of US2/US4/US5.
- **US4 (P3)**: Depends on Foundational + US1's `OverviewResult` (T015 path). Independent of US2/US3/US5.
- **US5 (P2)**: Depends on Foundational (`noopCount`) + US1's reducer (T013) and renderer. Independent of US2/US3/US4.

### Within Each User Story

- Reducer/classifier (`overview.go`) before `Overview()` join; `Overview()` before the `cmd` renderer.
- Tests are co-located and may be written alongside or just before their implementation (no strict TDD gate, but they MUST exist and pass — Constitution §II).
- Tasks editing the **same file** are sequential (not `[P]`).

### Parallel Opportunities

- **Setup**: T001 and T002 are `[P]` (different files); T003 follows T002.
- **Foundational**: sequential (single file + guard).
- **After Foundational, the four extension stories (US2, US3, US4, US5) can be developed in parallel by different people** — but US2/US3/US4/US5 all edit `cmd/overview.go`, so within a single working copy, serialize the `cmd/overview.go` edits (T019/T020 → T022 → T024 → T028) and parallelize their **tests** (T017, T021, T023 are `[P]` only against tasks in *other* files) and US5's reducer/fixture work (T026 `[P]`).
- **Polish**: T029–T032 are all `[P]` (distinct doc files); T033/T034 run last, sequentially.

---

## Parallel Example: Setup Phase

```bash
# T001 and T002 touch different files — run together:
Task: "Add commandOverview const in cmd/output.go"
Task: "Create internal/fleet/overview.go type skeletons + Overview() stub"
# then T003 (depends on T002's types):
Task: "Create cmd/overview.go stub + register in cmd/root.go"
```

## Parallel Example: Documentation Polish

```bash
# All distinct files — run together:
Task: "Update README.md command list + usage block"
Task: "Create docs/src/content/docs/overview.md"
Task: "Add overview to docs/src/content/docs/index.mdx + docs/astro.config.mjs sidebar"
Task: "Update AGENTS.md + CLAUDE.md common-commands line + architecture paragraph"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1: Setup (T001–T003).
2. Phase 2: Foundational (T004–T007) — **CRITICAL**, factor the shared fan-out and keep `consumption` green.
3. Phase 3: User Story 1 (T008–T016).
4. **STOP and VALIDATE**: `go run . overview` renders the joined drift×health×cost dashboard with a TOTAL row, exit code keys on drift. This is a shippable MVP (NOOP shows 0; trailing-7d is the hardcoded default until US2 wires the flag).

### Incremental Delivery

1. Setup + Foundational → shared fan-out ready.
2. US1 → the joined dashboard (MVP).
3. US2 → temporal windowing + repo scoping.
4. US3 → CI-safe drift exit gate.
5. US4 → `--output json` envelope.
6. US5 → the NOOP run-rate column (the additive cost-efficiency lens).
7. Polish → docs + `make ci`.

Each story adds value without breaking the previous ones; US2–US5 are independently mergeable on top of US1.

---

## Notes

- `[P]` = different files, no incomplete-task dependency. Same-file tasks are intentionally **not** `[P]`.
- No new third-party dependencies (FR-025); `cmd.SchemaVersion` stays `1` (SC-004) — never bump it.
- Read-only command (FR-002): no `--apply`, no git, no gpg, no writes — the three-turn mutation pattern does not apply.
- The Foundational refactor (T006) is the highest-risk task: it MUST be behavior-preserving for `consumption`. T007 is its regression guard.
- Every external call goes through the existing seams (`ghWorkflowsAPI`, `ghLogsAPI`, the `Status` fetcher) so all tests run offline (SC-006). Reuse the existing `internal/fleet/testdata/logs/` + status fixtures; only `logs_mixed_noop.json` is new.
- Do not claim done until `make ci` (T034) passes locally.
