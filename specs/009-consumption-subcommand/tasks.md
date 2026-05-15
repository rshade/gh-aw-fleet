---

description: "Task list for 009-consumption-subcommand — read-only `gh-aw-fleet consumption` subcommand that aggregates per-repo api-consumption-report output across the fleet"
---

# Tasks: Consumption Subcommand

**Input**: Design documents from `/specs/009-consumption-subcommand/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/consumption-output.json, contracts/consumption-text-output.md, contracts/discussion-discovery.md, contracts/run-artifact-payload.md, quickstart.md

**Tests**: Included. SC-007 mandates the full `make ci` gate passes with the new subcommand and all fixtures included, and the spec's "Testing Strategy" section enumerates five distinct test classes (filter matrix, body parsing, artifact parsing, multi-profile aggregation, envelope shape). Per Constitution v1.1.0 §II.2 the offline-only constraint applies: all `gh api` paths must be substitutable via the test-injection seam (research.md Decision 7) — no live network in CI.

**Organization**: Four user stories ship in a single bundled PR (research.md Decision 10) but are organized as independent phases below — US1 (repo rollup snapshot, MVP), US2 (temporal modes), US3 (group-by axes), US4 (top-burners footer). Stories share files (`internal/fleet/consumption.go`, `internal/fleet/consumption_test.go`, `cmd/consumption.go`); tasks within a story that target *distinct* files are marked `[P]`; cross-story tasks targeting the *same* file are sequenced.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1 = repo snapshot, US2 = temporal modes, US3 = group-by axes, US4 = top-burners)
- Include exact file paths in descriptions

## Path Conventions

Single-binary Go CLI. No separate `tests/` tree — `*_test.go` lives next to the code under test. Paths below are repo-root-relative.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Provision the fixture tree that every test class in the foundational and story phases consumes. No project-level initialization needed — this feature is an additive extension on an established Go module.

- [X] T001 Create the fixture directory at `internal/fleet/testdata/consumption/` (a new subtree under the existing `internal/fleet/testdata/` root used by other internal tests). This single directory holds all twelve fixture files added by T002 and T003.
- [X] T002 [P] Author the six discussion-body JSON fixtures under `internal/fleet/testdata/consumption/`: `discussion_valid.json` (full body with run-ID link + expires marker + non-in-progress), `discussion_in_progress.json` (body contains the `🔄 in-progress` substring), `discussion_expired.json` (expires marker set to a past timestamp like `2020-01-01T00:00:00Z`), `discussion_malformed.json` (body lacks the `actions/runs/{id}/agentic_workflow` link, exercises soft-failure diagnostic path), `discussion_wrong_category.json` (category.slug = `"general"`, must be filtered out by the discoverReports filter), `discussion_no_tracker.json` (body lacks the `<!-- gh-aw-tracker-id: api-consumption-report-daily -->` marker, must be filtered out). Each fixture mirrors the shape consumed by `discussionJSON` in `contracts/discussion-discovery.md` §"Response shape consumed".
- [X] T003 [P] Author the six artifact JSON fixtures under `internal/fleet/testdata/consumption/`: `aw_info_cost_present.json` (cost = 12.45), `aw_info_cost_absent.json` (no `cost` key), `aw_info_cost_zero.json` (cost = 0 — exercises Decision 6's nil-on-non-positive rule), `aw_info_cost_negative.json` (cost = -1.5 — defensive), `run_summary.json` (three workflows, two with cost populated), `run_summary_empty.json` (`{"workflows": []}`). Each fixture mirrors the shape consumed by `awInfoPayload` / `runSummaryPayload` in `contracts/run-artifact-payload.md` §"Payload shape consumed".

**Checkpoint**: Fixture tree exists. All foundational and story-phase tests will read from these files.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Source skeleton plus the discovery + artifact-fetch layers underpin every user story — the default `fetchLatest` mode in US1 already exercises both. Without `discoverReports`, `fetchRunArtifacts`, and the package-level injection seams, no story can be wired end-to-end.

**⚠️ CRITICAL**: No user-story phase may begin until T004 through T012 are complete.

- [X] T004 Create `internal/fleet/consumption.go` with the package skeleton: `// Package fleet consumption-rollup layer ...` package-doc paragraph (one comment to satisfy Constitution §I "every package gets one"; existing `diagnostics.go:1-7` is the local model), exported types (`ConsumptionReport`, `WorkflowConsumption`, `ConsumptionGroup`, `ConsumptionResult`, `FetchMode`) with full godoc per data-model.md §"Layer 2" and §"Layer 3", unexported types (`reportRef`, `fetchKind`, `discussionJSON`, `awInfoPayload`, `runSummaryPayload`, `artifactPayload`), `fetchKind` consts (`fetchLatest`, `fetchTrailing`, `fetchSince`). No function bodies yet. Verify `go build ./...` and `go vet ./...` clean before moving on.
- [X] T005 In `internal/fleet/consumption.go`, add the small helpers: `parseTrailing(s string) (int, error)` accepting the `^(\d+)d$` shape per research.md Decision 4 (returns an explicit error naming the accepted form on miss), `normalizeCost(*float64) *float64` per research.md Decision 6 (nil-on-non-positive), and the package-level `regexp.MustCompile` vars `runIDRe` (`/actions/runs/(\d+)/agentic_workflow`) and `expiresRe` (`<!-- gh-aw-expires:\s*([^\s]+)\s*-->`) per contracts/discussion-discovery.md §"Body-marker extraction". Mark the regex vars `//nolint:gochecknoglobals // anchored marker regexps compiled once at init`.
- [X] T006 In `internal/fleet/consumption.go`, add the discovery seam and implementation: package-level `var ghDiscussionsAPI = func(ctx context.Context, repo string) ([]discussionJSON, error) { ... }` (the production impl shells `gh api --paginate repos/{repo}/discussions` via the existing `exec.CommandContext` + `runLoggedOutput` pattern mirrored from `internal/fleet/fetch.go:184`), marked `//nolint:gochecknoglobals // test-injection seam ...`. Then implement `discoverReports(ctx context.Context, repo string) ([]reportRef, []Diagnostic, error)`: applies the two-predicate filter (`category.slug == "audits"` AND body contains the tracker marker), extracts the four body markers per contracts/discussion-discovery.md §"Body-marker extraction" (run-ID, date from title, expires, in-progress), emits a soft-failure `DiagHint` diagnostic per malformed/missing-marker record without aborting, sorts the returned slice by `Date` descending so `fetchLatest` can take element 0 without re-sorting.
- [X] T007 In `internal/fleet/consumption.go`, add the artifact-fetch seam and implementation: package-level `var ghRunArtifactAPI = func(ctx context.Context, repo string, runID int64) (artifactPayload, error) { ... }` (production impl: two `gh api` calls per contracts/run-artifact-payload.md §"Fetch sequence" — list artifacts at `repos/{repo}/actions/runs/{runID}/artifacts`, then download zip at `repos/{repo}/actions/artifacts/{id}/zip`, unzip in-memory via stdlib `archive/zip` + `bytes.Reader`, JSON-decode `aw_info.json` and `run_summary.json`), marked `//nolint:gochecknoglobals`. Then implement `fetchRunArtifacts(ctx context.Context, ref reportRef) (*ConsumptionReport, *Diagnostic, error)`: invokes the seam, applies `normalizeCost` to the top-level `Cost` and every `PerWorkflow[i].Cost`, returns a fully-populated `ConsumptionReport` minus the `Profile` and `CostCenter` joined fields (those are populated by aggregation).
- [X] T008 [P] Create `internal/fleet/consumption_test.go` and add table-driven tests for `parseTrailing` (cases: `"7d"` → 7 valid, `""` → error, `"7"` → error, `"7h"` → error, `"7d "` → error, `"0d"` → error, `"-1d"` → error) and `normalizeCost` (cases: nil → nil, &0.0 → nil, &-1.5 → nil, &12.45 → &12.45). Both target the helpers landed in T005.
- [X] T009 [P] In `internal/fleet/consumption_test.go`, add `TestDiscoverReports` table-driven against the six discussion fixtures from T002. Sets `ghDiscussionsAPI` to a closure that returns the fixture payloads (per the t.Cleanup teardown pattern in contracts/discussion-discovery.md §"Mockability for tests"). Verifies: valid discussion → one reportRef with correct fields, wrong-category / no-tracker → filtered out, in-progress → reportRef with `InProgress=true`, expired → reportRef with the past Expires timestamp, malformed → soft-failure diagnostic emitted, no panic. Asserts the returned slice is sorted descending by Date.
- [X] T010 [P] In `internal/fleet/consumption_test.go`, add `TestFetchRunArtifacts` table-driven against the six artifact fixtures from T003. Substitutes `ghRunArtifactAPI` and `ghDiscussionsAPI` similarly. Verifies: cost-present → `*Cost != nil && *Cost == 12.45`, cost-absent → `Cost == nil`, cost-zero → `Cost == nil` (Decision 6), cost-negative → `Cost == nil`, run_summary with three workflows → `len(PerWorkflow) == 3`, run_summary_empty → `len(PerWorkflow) == 0`.
- [X] T011 Create `cmd/consumption.go` with the cobra skeleton: `newConsumptionCmd(flagDir *string) *cobra.Command` declaring `Use: "consumption"`, `Short: "Aggregate api-consumption-report output across the fleet"`. Define the four flags exactly as written in the originating issue body and in plan.md (`--latest` bool default true, `--trailing` string, `--since` string, `--by` string default `"repo"`). Call `cmd.MarkFlagsMutuallyExclusive("latest", "trailing", "since")` per FR-004. Leave `RunE` as a stub returning `nil` (filled in by US1).
- [X] T012 In `cmd/root.go`, add the single-line `AddCommand(newConsumptionCmd(&flagDir))` to `NewRootCmd` next to the existing `AddCommand` calls for `list`, `status`, etc. Verify `go build ./...` clean and `go run . consumption --help` prints the four flags.

**Checkpoint**: Foundation ready — discovery + artifact fetch are unit-tested against offline fixtures, the cobra subcommand is wired into root, `--help` works, but `RunE` is still a stub. User-story phases can now begin.

---

## Phase 3: User Story 1 — Fleet-wide consumption rollup at a glance (Priority: P1) 🎯 MVP

**Goal**: Operator runs `gh-aw-fleet consumption` with no flags and sees a tabwriter table grouped by repo summarizing each repo's most-recent valid consumption report. JSON envelope mode works in symmetry with `list`. Diagnostic warnings surface for repos with no reports / in-progress-only / expired-only candidates.

**Independent Test**: After Phase 2 + this phase, `go run . consumption` against a fleet with a real consumption report on at least one repo prints the breadcrumb on stderr + a repo-keyed table on stdout. `go run . consumption --output json | jq '.result.groups | length'` returns the count of repos with reports. With the four mutual-exclusion flags not yet wired into a full `FetchMode`, only the default snapshot mode is functional — but US1 is complete on its own slice.

### Tests for User Story 1 ⚠️

> **Write these tests FIRST, ensure they FAIL before the implementation tasks (T024 onward), then watch them pass.**

- [X] T013 [P] [US1] In `internal/fleet/consumption_test.go`, add `TestShouldIncludeReport_FetchLatest` table-driven matrix covering: (valid, non-expired, non-in-progress) → include=true, no warning; (in-progress, non-expired) → include=false, warning suggesting `--trailing 7d`; (expired) → include=false, no per-report warning; (every candidate in-progress or expired) → caller-level warning that all candidates were filtered. Mocks `time.Now()` via the `now time.Time` parameter on `shouldIncludeReport`.
- [X] T014 [P] [US1] In `internal/fleet/consumption_test.go`, add `TestAggregateConsumption_GroupByRepo` covering: two reports for one repo with summed totals, three repos each with one report, sums of `GitHubAPICalls` / `SafeOutputCalls` correct, `Cost` summed when present + nil when absent, `ReportCount` accurate, output sorted alphabetically by `Key` per data-model.md §"Sort order".
- [X] T015 [P] [US1] In `internal/fleet/consumption_test.go`, add `TestConsumptionResult_JSONEnvelope` mirroring `list_result_test.go`'s pattern: build a `*ConsumptionResult` via `AggregateConsumption`, marshal through `writeEnvelopeTo`, assert the JSON shape matches `contracts/consumption-output.json` (top-level keys present, `result.groups[]` and `result.top_burners[]` always non-null even when empty per FR-019, `cost` omitted when nil per Decision 6, `schema_version` = 1).

### Implementation for User Story 1

- [X] T016 [US1] In `internal/fleet/consumption.go`, implement `shouldIncludeReport(ref reportRef, mode FetchMode, now time.Time) (bool, *Diagnostic)` for the `fetchLatest` arm only (the other two arms in US2 extend this function). Decision logic per spec.md §"Filter semantics": skip in-progress and expired; emit a diagnostic naming the repo and suggesting `--trailing 7d` only when *every* candidate for a repo is filtered. The per-repo "every candidate filtered" check happens at the caller (T018) because `shouldIncludeReport` works one-ref-at-a-time.
- [X] T017 [US1] In `internal/fleet/consumption.go`, implement `AggregateConsumption(cfg *Config, mode FetchMode, by string) (*ConsumptionResult, []Diagnostic, error)`. For US1, support `by == "repo"` only (US3 extends to the other three axes). Loop over `cfg.Repos` in sorted order, call `discoverReports`, filter via `shouldIncludeReport` for the `fetchLatest` arm taking the first non-filtered ref per repo, call `fetchRunArtifacts`, accumulate one `ConsumptionGroup` per repo (sum `GitHubAPICalls` / `SafeOutputCalls`, sum `Cost` when present on all contributing reports — nil if any is nil; alternative is to sum the present ones and emit a diagnostic, but for v1 keep the conservative rule "all-or-nothing for cost"), increment `ReportCount`. Surface diagnostics for: zero discovered reports (FR-010), every-candidate-filtered (FR-011), retention-expired artifact fetch (FR-009 via DiagHTTP404 already in the existing hint table).
- [X] T018 [US1] In `cmd/consumption.go`, fill in `RunE`: load config via `fleet.LoadConfig(*flagDir)` mirroring `cmd/list.go:23-29`, emit the `(loaded {cfg.LoadedFrom})` stderr breadcrumb mirroring `cmd/list.go:32`, branch on `outputMode(cmd)` per `cmd/list.go:34`. JSON path: call `AggregateConsumption(cfg, FetchMode{Kind: fetchLatest}, "repo")` and write via `writeEnvelope(cmd, commandConsumption, "", false, res, warnings, hints)`. Text path: render a tabwriter table with header `REPO\tAPI_CALLS\tSAFE_WRITES\tCOST\tREPORTS` per `contracts/consumption-text-output.md` §"--by repo (default)", one row per group (cost rendered as `$%.2f` when populated else `"-"` via the existing `orDash` helper at `cmd/list.go:70`).
- [X] T019 [US1] In `cmd/output.go`, add the `commandConsumption = "consumption"` const next to the existing five command-name consts (`commandDeploy`, `commandList`, etc.). Verify `consumption` is NOT in `rejectJSONMode`'s deny list — that function rejects `--output json` only for commands explicitly listed (template, add); the default is "supported," so consumption joins the JSON-supporting set with no edit needed beyond the const.

**Checkpoint**: `go run . consumption` produces a fleet-wide rollup. `go run . consumption --output json | jq .result.groups` returns valid JSON. US1 is independently demoable. Run `make ci`; T013/T014/T015 should now pass.

---

## Phase 4: User Story 2 — Cost-aware audit across a trailing window or since a fixed date (Priority: P2)

**Goal**: `--trailing <Nd>` and `--since <YYYY-MM-DD>` flags extend the rollup beyond the default snapshot. In-progress reports surface a per-row warning but are included in the sum (FR-012); expired reports remain excluded (FR-013). Mutual-exclusion errors are clear.

**Independent Test**: After US2 lands, `go run . consumption --trailing 7d` sums seven days of reports per repo (`ReportCount` per row jumps from ~1 to ~7); `go run . consumption --since 2026-04-01` sums everything from the cutoff date; `go run . consumption --latest --trailing 7d` exits non-zero with the mutual-exclusion message.

### Tests for User Story 2 ⚠️

> **Write these tests FIRST**, then watch them pass after T024/T025.

- [X] T020 [P] [US2] In `internal/fleet/consumption_test.go`, add `TestShouldIncludeReport_FetchTrailing` table-driven matrix covering: (ref date within window, non-expired, non-in-progress) → include=true, no warning; (ref date within window, in-progress) → include=true *with* warning per FR-012; (ref date within window, expired) → include=false; (ref date outside window) → include=false. Tests use the `now` parameter to pin the reference time.
- [X] T021 [P] [US2] In `internal/fleet/consumption_test.go`, add `TestShouldIncludeReport_FetchSince` with parallel structure to T020: (date >= since, non-expired, non-in-progress) → include=true; (date >= since, in-progress) → include=true with warning; (date >= since, expired) → include=false; (date < since) → include=false. Edge case: a since-date older than the platform's run-log retention window should still produce a valid `shouldIncludeReport` result — the retention-expired warning surfaces at the artifact-fetch layer (FR-009), not here.
- [X] T022 [P] [US2] In `cmd/consumption_test.go` (new file), add `TestConsumption_MutualExclusion`: invokes the cobra command with `--latest --trailing 7d`, asserts non-zero exit + error message contains `"mutually exclusive"` per cobra's `MarkFlagsMutuallyExclusive` default. Add a parallel case for `--trailing 7d --since 2026-04-01` and `--latest --since 2026-04-01`.

### Implementation for User Story 2

- [X] T023 [US2] In `internal/fleet/consumption.go`, extend `shouldIncludeReport` to handle the `fetchTrailing` and `fetchSince` arms. For `fetchTrailing`: `ref.Date >= now.Add(-mode.Days * 24 * time.Hour)` AND `!ref.InProgress && !now.After(ref.Expires)` for non-warning case; include with warning when in-progress; exclude when expired. For `fetchSince`: identical semantics with `ref.Date >= mode.Since` as the window predicate. Same `*Diagnostic` return signature.
- [X] T024 [US2] In `cmd/consumption.go`, replace the hardcoded `FetchMode{Kind: fetchLatest}` in `RunE` with a flag-driven constructor: when `--trailing` is set, call `parseTrailing(flagTrailing)` and build `FetchMode{Kind: fetchTrailing, Days: n}`; when `--since` is set, call `time.Parse("2006-01-02", flagSince)` (returning a clear error message on parse failure) and build `FetchMode{Kind: fetchSince, Since: t.UTC()}`. Default (and `--latest`): `FetchMode{Kind: fetchLatest}`.
- [X] T025 [US2] In `internal/fleet/consumption.go`, when `AggregateConsumption` aggregates multiple reports per repo (which now happens under trailing/since modes), update the per-repo aggregation loop to sum across all included reports (was: take the first only). Update `ReportCount` to reflect the actual contributing-report count per group. The function's signature does not change.
- [X] T026 [US2] In `internal/fleet/consumption.go`, populate `ConsumptionResult.FetchMode` with a human-readable string: `"latest"` for `fetchLatest`, `"trailing-{N}d"` for `fetchTrailing` (e.g., `"trailing-7d"`), `"since-{YYYY-MM-DD}"` for `fetchSince` (e.g., `"since-2026-04-01"`). This is the value rendered into `contracts/consumption-output.json` `result.fetch_mode`.

**Checkpoint**: All three temporal modes work end-to-end. `go run . consumption --trailing 7d` and `go run . consumption --since 2026-04-01` produce correct sums; mutual-exclusion violations exit non-zero with clear messages. Run `make ci`; T020/T021/T022 should now pass.

---

## Phase 5: User Story 3 — Group-by-axis to expose cost concentration (Priority: P2)

**Goal**: `--by profile`, `--by cost-center`, and `--by workflow` pivot the rollup along the axes operators budget against. Multi-profile additive double-counting is the documented semantic (FR-014); the `<unset>` bucket holds repos with no `cost_center` declared (FR-015); the workflow pivot keys on workflow name not repo (FR-016).

**Independent Test**: After US3 lands, `go run . consumption --by profile` displays one row per profile across the fleet (with repos belonging to multiple profiles contributing additively to each); `--by cost-center` displays one row per declared cost-center plus an `<unset>` bucket; `--by workflow` displays one row per distinct workflow name; `--by tier` exits non-zero with a clear error listing the four valid values.

### Tests for User Story 3 ⚠️

> **Write these tests FIRST**, then watch them pass after T030–T033.

- [X] T027 [P] [US3] In `internal/fleet/consumption_test.go`, add `TestAggregateConsumption_GroupByProfile`: a fleet of four repos split across two profiles `standard` and `premium`, one repo participating in *both* (multi-profile). Assert exactly two rows keyed by profile name, the multi-profile repo's consumption summed into both rows (additive double-count per FR-014, research.md Decision 5), `ReportCount` per row reflects per-profile contributing-report count.
- [X] T028 [P] [US3] In `internal/fleet/consumption_test.go`, add `TestAggregateConsumption_GroupByCostCenter`: a fleet of three repos where two declare `cost_center: "platform-eng"`, one declares `cost_center: "data-platform"`, and the fourth declares no cost-center. Assert three rows: `platform-eng` summing two repos, `data-platform` summing one, `<unset>` summing the fourth. The `<unset>` literal must be the exact key value.
- [X] T029 [P] [US3] In `internal/fleet/consumption_test.go`, add `TestAggregateConsumption_GroupByWorkflow`: a fleet of three repos collectively running ten distinct workflows. Assert ten rows keyed by workflow name, each row's `GitHubAPICalls` / `Cost` reflect that workflow's totals summed across every contributing repo, ordering is alphabetical per `Key`.
- [X] T030 [P] [US3] In `cmd/consumption_test.go`, add `TestConsumption_InvalidByFlag`: invokes the cobra command with `--by tier`, asserts non-zero exit + error message contains all four valid values (`repo`, `profile`, `cost-center`, `workflow`).

### Implementation for User Story 3

- [X] T031 [US3] In `internal/fleet/consumption.go`, extend `AggregateConsumption` to handle `by == "profile"`. For each repo's contributing reports, look up `cfg.Repos[report.Repo].Profiles` and emit one report-contribution per profile name (so a repo with two profiles contributes its consumption twice — once per profile group). Sum into the corresponding `ConsumptionGroup` keyed by profile name. The additive double-count is intentional and documented in spec.md §"User Story 3" acceptance #5.
- [X] T032 [US3] In `internal/fleet/consumption.go`, extend `AggregateConsumption` to handle `by == "cost-center"`. Read `cfg.Repos[report.Repo].CostCenter`; empty string maps to the literal bucket key `"<unset>"` per FR-015. One contribution per repo (no double-count semantic here — each repo has exactly one cost-center declaration).
- [X] T033 [US3] In `internal/fleet/consumption.go`, extend `AggregateConsumption` to handle `by == "workflow"`. Pivot away from per-repo aggregation: iterate every `PerWorkflow[i]` of every report, sum into `ConsumptionGroup` keyed by `Workflow` name. `ReportCount` per row counts the number of *reports* (not repos, not runs) that mention this workflow — operators reading this column should be told what it counts in the textual rendering header.
- [X] T034 [US3] In `cmd/consumption.go`, add the `--by` value validation before invoking `AggregateConsumption`: `switch flagBy { case "repo", "profile", "cost-center", "workflow": ... default: return fmt.Errorf("--by value %q invalid: expected one of repo, profile, cost-center, workflow", flagBy) }` per FR-005.
- [X] T035 [US3] In `cmd/consumption.go`, update the text-mode rendering to use the correct uppercase key-column header name per `contracts/consumption-text-output.md` §"Primary table" — `REPO` / `PROFILE` / `COST_CENTER` / `WORKFLOW` depending on `--by`. Populate `ConsumptionResult.GroupBy` with the lowercase axis string (`"repo"` / `"profile"` / `"cost-center"` / `"workflow"`) for JSON consumers.

**Checkpoint**: All four `--by` axes work. Run `make ci`; T027/T028/T029/T030 should now pass.

---

## Phase 6: User Story 4 — Top-burner spotlight for hotspot triage (Priority: P3)

**Goal**: A clearly-labeled secondary footer block lists the highest-consuming individual workflows ranked descending by API-call volume (or by cost when populated, per spec.md Story 4 #3), capped at ten rows, gracefully presenting fewer rows when fewer distinct workflows exist (FR-017).

**Independent Test**: After US4 lands, `go run . consumption` output ends with a `TOP 10 BURNERS:` header and a per-workflow table; for a fleet with fewer than ten distinct workflows, the table shows as many rows as exist (not ten padded). The JSON envelope's `result.top_burners[]` mirrors this.

### Tests for User Story 4 ⚠️

> **Write these tests FIRST**, then watch them pass after T038/T039.

- [X] T036 [P] [US4] In `internal/fleet/consumption_test.go`, add `TestAggregateConsumption_TopBurnersFullTen`: a fleet collectively running fifteen distinct workflows of varying consumption levels. Assert `len(result.TopBurners) == 10`, the slice is sorted descending by `APICalls`, the highest-consuming workflow is at index 0.
- [X] T037 [P] [US4] In `internal/fleet/consumption_test.go`, add `TestAggregateConsumption_TopBurnersFewer`: a fleet running only three distinct workflows total. Assert `len(result.TopBurners) == 3` (no padding to 10) — graceful fewer-rows behavior per FR-017.

### Implementation for User Story 4

- [X] T038 [US4] In `internal/fleet/consumption.go`, populate `ConsumptionResult.TopBurners` at the end of `AggregateConsumption` regardless of the `by` value: build a `map[string]*WorkflowConsumption` keyed by workflow name across all reports' `PerWorkflow` entries, sum `Runs` / `APICalls` / `Cost` per workflow, average `AvgDurationS` weighted by `Runs` (a simple average of the per-report averages biases toward sparsely-running workflows; weighted-by-runs is honest), sort the materialized slice descending by `APICalls` (or by `Cost` when *every* workflow has a populated cost — checked via a first-pass loop), cap at 10 with `min(10, len(slice))`.
- [X] T039 [US4] In `cmd/consumption.go`, render the top-burners footer in text mode after the primary table: emit one blank line, then the header `TOP 10 BURNERS:`, then a fresh tabwriter with header `WORKFLOW\tRUNS\tAPI_CALLS\tAVG_DURATION\tCOST` per contracts/consumption-text-output.md §"Top-burners footer". `AvgDurationS` renders as `%.1fs`. Cost renders identically to the primary table.

**Checkpoint**: Full feature complete. Run `make ci`; T036/T037 should now pass. End-to-end `go run . consumption` against the project's own fleet (which has no consumption-report deployments today) should produce empty groups + a top-level diagnostic warning per FR-010.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, diagnostic-copy verification, and final validation.

- [X] T040 Verify `internal/fleet/diagnostics.go:64` `billingQuotaHint` copy reads correctly after this subcommand ships (FR-025). The existing copy reads: `"Cross-repo cost attribution will be available via `+"`gh-aw-fleet consumption`"+` once that subcommand ships."` — after this PR merges, that sentence becomes literally true (no longer aspirational). No code change expected; this task is a verification pass to confirm the wording survives review.
- [X] T041 [P] Update `AGENTS.md` with a `consumption` subsection under the architectural framing — describe the two-layer fetch (discovery via discussion category + tracker marker, data via run artifacts), the four group-by axes (repo / profile / cost-center / workflow), the additive multi-profile double-count semantic, the cost-field nil-until-populated rule (Decision 6), the no-caching invariant (FR-022), and the read-only constraint (FR-002). Two paragraphs is the right length; mirror the existing `*-plus` and billing-metadata-fields paragraph density. Closes FR-027.
- [X] T042 [P] Update `CLAUDE.md` "Common commands" block: add a single line `go run . consumption    # Aggregate api-consumption-report output across the fleet` between the existing `go run . list` and `go run . template fetch` lines.
- [X] T043 Update the spec dir's `agent-file-template`-driven block: per `.specify/templates/agent-file-template.md`, the speckit framework expects an `## Active Technologies` and `## Recent Changes` bullet list at the bottom of `AGENTS.md` to be updated each feature. Add: `- 009-consumption-subcommand: new read-only `+"`gh-aw-fleet consumption`"+` subcommand; no new third-party deps; reuses existing diagnostic codes.`
- [X] T044 Run `make ci` end-to-end. Address any `gofmt`, `golangci-lint`, or test failures. Per CLAUDE.md "Local gate" rule, `make ci` must be green before reporting the slice done.
- [X] T045 Run `go run . consumption` and `go run . consumption --output json` against the project's own fleet config (no consumption reports deployed today). Capture the expected "no reports discovered" diagnostics for the PR description. The command must exit 0 — no-data is not failure per FR-010.
- [X] T046 Validate the `specs/009-consumption-subcommand/quickstart.md` examples are accurate against the shipped surface: the four flag forms, the JSON envelope shape, the diagnostic glossary. If any example's wording drifted during implementation (different error string, different column header), update the quickstart to match.

**Checkpoint**: PR-ready. `make ci` green. Documentation in sync. The forward-reference hint at `internal/fleet/diagnostics.go:64` is now literally true. Phase 8 below is OPTIONAL and deferred to a follow-up issue per plan.md §"Source Code → skills".

---

## Phase 8: OPTIONAL — fleet-budget-review skill (deferred follow-up)

**Purpose**: A new SKILL.md following the established `skills/` pattern, so operators have a documented "budget review" flow that pairs with the new subcommand. Plan.md flagged this as deferred-to-tasks scope; this phase captures the placeholder so the work is not lost.

- [X] T047 [OPTIONAL] Author `skills/fleet-budget-review/SKILL.md` following the structure of the existing skill SKILL.md files. Three-turn pattern is *not* needed here since `consumption` is read-only — adapt the skill template to a single-turn read-only flow that demonstrates the four `--by` modes and the diagnostic warnings glossary. This task SHOULD be split into its own follow-up issue if the operator-skill surface needs more design discussion than fits in this PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: T001 must precede T002 / T003 (directory must exist before files). T002 and T003 are `[P]` once T001 lands.
- **Foundational (Phase 2)**: T004 must precede everything else in the phase (it provides the type declarations that T005–T012 reference). T005 must precede T006/T007 (they consume the helpers). T006/T007 must precede T008/T009/T010 (the tests substitute the seams those tasks define). T011 must precede T012 (root wires it). T012 unblocks all user-story phases.
- **User Story 1 (Phase 3)**: T013/T014/T015 are `[P]` test tasks that can be authored in parallel (all in `consumption_test.go` — they target distinct test functions and can be appended without conflict, but if a single editor opens the file, sequence them T013 → T014 → T015). T016/T017 add functions; T018/T019 wire them into the cobra command. T013 must logically *predate* T016 in execution order (TDD), but file-level parallelism is fine.
- **User Story 2 (Phase 4)**: Tests T020/T021/T022 are `[P]`. T023 extends a function landed in US1 (`shouldIncludeReport`) — same file as T016, so sequential after US1. T024/T025/T026 each touch the same file as well — sequential.
- **User Story 3 (Phase 5)**: Tests T027/T028/T029/T030 are `[P]`. T031/T032/T033 extend `AggregateConsumption` — same file, sequential. T034 and T035 each touch the cobra command file — sequential after T024/T026.
- **User Story 4 (Phase 6)**: Tests T036/T037 are `[P]`. T038 extends `AggregateConsumption` — sequential after T033. T039 touches the cobra command file — sequential after T035.
- **Polish (Phase 7)**: T041 (AGENTS.md) and T042 (CLAUDE.md) are `[P]` (different files). T043 also touches AGENTS.md so it sequences after T041. T040 is verify-only (no code change). T044/T045/T046 are validation, must come last.

### User Story Dependencies

- **US1**: Can start as soon as Phase 2 is complete. No dependencies on other stories. Independently demoable as "default snapshot rollup grouped by repo."
- **US2**: Can start as soon as US1's `shouldIncludeReport` skeleton (T016) and `AggregateConsumption` (T017) exist. Independently demoable as "the same rollup over a trailing window or since a fixed date."
- **US3**: Can start as soon as US1's `AggregateConsumption` exists. Pull request can land US1 + US2 + US3 separately if reviewers want smaller diffs, but research.md Decision 10 keeps them in one PR. Independently demoable as "the same rollup grouped by profile / cost-center / workflow."
- **US4**: Can start as soon as US1's `AggregateConsumption` exists. Independently demoable as "the rollup with a TOP 10 BURNERS footer."

### Within Each User Story

- Tests are written first per the TDD-ish practice in this repo (every `*.go` has a `*_test.go` neighbor). Make sure tests FAIL before the implementation tasks satisfy them.
- Foundational types before logic.
- Logic before wiring into the cobra command.
- Wiring before documentation.

### Parallel Opportunities

- **Within Phase 1**: T002 ‖ T003 (different fixture files).
- **Within Phase 2**: T008 ‖ T009 ‖ T010 (different test functions in the same file — author in distinct commits or as separate functions in one commit; trivially conflict-free).
- **Within US1 tests**: T013 ‖ T014 ‖ T015.
- **Within US2 tests**: T020 ‖ T021 ‖ T022.
- **Within US3 tests**: T027 ‖ T028 ‖ T029 ‖ T030.
- **Within US4 tests**: T036 ‖ T037.
- **Within Polish**: T041 ‖ T042 (different files).
- **Cross-story**: Once Foundational is done, US1 + US2 + US3 + US4 could in principle be staffed in parallel by four developers; in practice US2/US3/US4 each extend functions that US1 lands, so US1 lands first and US2/US3/US4 are concurrent after.

---

## Parallel Example: User Story 1 tests

```bash
# Once T012 (cobra root wiring) is complete, US1 tests can be authored together:
Task: "T013 [US1] TestShouldIncludeReport_FetchLatest in internal/fleet/consumption_test.go"
Task: "T014 [US1] TestAggregateConsumption_GroupByRepo in internal/fleet/consumption_test.go"
Task: "T015 [US1] TestConsumptionResult_JSONEnvelope in internal/fleet/consumption_test.go"
```

All three target the same file, but distinct top-level test functions — multiple authors can write them without conflict if they each append their function to the file. If a single agent runs them sequentially, the file ordering does not matter — Go tests are independent.

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (fixture tree)
2. Complete Phase 2: Foundational (skeleton + discovery + fetch + cobra wiring)
3. Complete Phase 3: US1 (default snapshot rollup by repo)
4. **STOP and VALIDATE**: `go run . consumption` works end-to-end. Demo if standalone US1 ship is desired.

### Incremental Delivery (single PR per research.md Decision 10)

1. Complete Setup + Foundational → foundation ready.
2. Add US1 → test independently → first usable surface.
3. Add US2 → test independently → temporal modes wired.
4. Add US3 → test independently → all four `--by` axes wired.
5. Add US4 → test independently → top-burners footer in place.
6. Complete Phase 7: Polish → docs in sync, diagnostic hint verified, `make ci` green.
7. PR ready.

### Parallel Team Strategy

If staffed with multiple developers:

1. Together: complete Phase 1 + Phase 2.
2. Developer A: US1 (T013–T019).
3. Developer B (after T016/T017 land): US2 (T020–T026).
4. Developer C (after T017 lands): US3 (T027–T035).
5. Developer D (after T017 lands): US4 (T036–T039).
6. Together: Polish (Phase 7).

Stories USx ≥ 2 all extend the same `consumption.go` file, so a single-developer slice is the more honest model. Multi-developer parallelism here is principle, not practice — the actual PR will be one author's commits in priority order.

---

## Notes

- **Tests are TDD-ish but not strict**. The repo's practice is `*_test.go` next to the code; the test tasks are listed before the implementation tasks within each phase to honor the spirit of "tests fail first" without imposing a strict TDD ritual.
- **`[P]` semantics**: distinct files OR distinct top-level functions in one file with no shared state. The fixture-tree tasks (T002, T003) are the cleanest `[P]` examples; the test-function-pair `[P]` markers (T013/T014/T015 etc.) acknowledge that multi-author parallelism is practical even within one file.
- **Cross-story file conflicts**: `consumption.go` is touched by every story phase. The dependency arrows in §"Phase Dependencies" make the sequencing explicit; do not skip them.
- **Commit cadence**: One commit per task or per logical group (e.g., the four `--by` axis additions in T031/T032/T033 could land in one commit). Follow the project's `ci(workflows)` Conventional-Commits scope per CLAUDE.md and the constitution.
- **Stop at any checkpoint** if reviewers want to inspect the partial slice. The phase-by-phase checkpoints in §"Checkpoint" lines are real stop points, not just narrative beats.
- **`make ci` is the gate** at T044 and at the end of every meaningful checkpoint. Run it locally before considering a phase done.
