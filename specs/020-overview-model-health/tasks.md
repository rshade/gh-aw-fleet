---
description: "Task list for feature 020-overview-model-health"
---

# Tasks: Flag Repos Failing on Unsupported/Deprecated Copilot Models in `overview`

**Input**: Design documents from `/specs/020-overview-model-health/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/overview-model-support.md, contracts/upstream-signals.md, quickstart.md

**Tests**: INCLUDED — the spec explicitly requires them. SC-006 mandates `make ci` green "with fixtures covering at least: one model-failure repo, one unrelated-failure repo, one healthy repo, and one undetermined/signal-unavailable repo"; SC-002 requires 100% fixture classification. Constitution §Testing Standards gates on the full suite. All seams are stubbed offline via the existing `withLogsStubs` pattern.

**Organization**: Tasks are grouped by user story. The detection engine is shared by all three stories, so it lives in **Foundational** (Phase 2); each story (Phase 3-5) adds one independent surfacing channel over that engine.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 / US3 (setup, foundational, and polish tasks carry no story label)
- File paths are repository-relative and exact.

## Architecture map (grounding for every task)

- `internal/fleet/overview.go` (415 ln) — `RepoOverview`, `overviewHealth`, `Overview` (two `wg.Go` collectors + `wg.Wait`), `joinOverviewRow`, `overviewRepoNames`, `addOverviewTotal`. **Result type + collector wiring live here** (plan Structure Decision).
- `internal/fleet/model_signal.go` — **NEW**. The two `gh` seams, marker parser, effective-model resolver, baked-default constant (keeps `overview.go` under the 300-line refactor threshold).
- `cmd/overview.go` (294 ln) — `renderOverviewText` → `printOverviewDetail` → `printOverviewRunsDetail`; `needsOverviewDetail`; `splitOverviewDiags` (routes `DiagHint` → hints) → `emitOverviewDiagnostics` → `emitHints`; `overviewExitCode` (drift-only, **unchanged**).
- Diagnostics: `DiagHint` (`internal/fleet/diagnostics.go`); `newSoftDiagnostic(repo, msg)` (`internal/fleet/consumption.go:436`) builds a `DiagHint`+`{hint, repo}` diag that routes to the **hints** channel — it fits the **affected-repo remediation hint** (T014), **not** the undetermined path; `withDiagnosticSignal(d, repo, signal)` (`internal/fleet/overview.go:260`) + `buildRepoErrorDiag(repo, msg)` (`internal/fleet/status.go:479`) build a **warnings-channel** `{repo, signal}` diag that fits the **undetermined** path (T008), matching the sibling runs-unavailable degradation at `overview.go:257`; field constants `fieldRepo`/`fieldHint`/`fieldSignal` (`internal/fleet/constants.go`).
- Test seam pattern: `withLogsStubs` (`internal/fleet/consumption_test.go:1074`) swaps package-level `var` seams and returns a restore func — mirror it for the two new seams.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the new source file scaffold and offline fixtures every later phase consumes.

- [X] T001 Create `internal/fleet/model_signal.go` with the package-`fleet` file header, imports (`context`, `encoding/json`, `os/exec`, `sort`, `strings`), and the constant block from data-model.md §Constants: marker tokens (`failureIssueMarker = "gh-aw-failure-issue: true"`, `failureCategoriesKey = "failure_categories:"`, `modelNotSupportedToken = "model_not_supported"`), variable names (`varAgentModelCopilot = "GH_AW_MODEL_AGENT_COPILOT"`, `varDefaultModelCopilot = "GH_AW_DEFAULT_MODEL_COPILOT"`), and `ghAwBakedDefaultCopilotModel = "claude-sonnet-4.6"`. The baked-default godoc MUST state it tracks the fleet's `github/gh-aw` source pin, cite that gh-aw drops defaults across releases (dropped `gpt-5-mini` in v0.81.5), and flag it as a maintenance touch-point on pin bumps (research Decision 2).
- [X] T002 [P] Create failure-issue body fixtures under `internal/fleet/testdata/failures/` matching data-model.md §Fixture matrix: a `model_not_supported.txt` body containing `<!-- gh-aw-failure-issue: true, workflow_id: code-simplifier, branch: main, failure_categories: agent_failure|model_not_supported_error -->` (the verified `rshade/finfocus#1349` marker), and an `unrelated.txt` body whose `failure_categories:` value is `agent_failure` only. Keep them realistic; each file ends with a trailing newline.

---

## Phase 2: Foundational (Blocking Prerequisites — the shared detection engine)

**Purpose**: The read-issue → parse-marker → resolve-model engine and its wiring into `Overview`. Every user story (text / hint / JSON) surfaces the status this engine produces.

**⚠️ CRITICAL**: No user story (Phase 3+) can begin until this phase is complete. Both the struct field and the collector are compile-time prerequisites for US1's `joinOverviewRow` wiring.

- [X] T003 Add the exported `ModelSupport` type (`Status string json:"status"`, `OffendingModels []string json:"offending_models,omitempty"`), the unexported status constants (`modelSupportAffected = "affected"`, `modelSupportUndetermined = "undetermined"`), and the additive field `ModelSupport *ModelSupport json:"model_support,omitempty"` on `RepoOverview` in `internal/fleet/overview.go`. Godoc every exported identifier per data-model.md §Entity; mirror the existing `DriftDetail *RepoStatus json:"drift_detail,omitempty"` pointer-with-omitempty convention. `OverviewTotal` is unchanged (no pooled counter — FR-007).
- [X] T004 Add the unexported reduce type `overviewModelHealth{ status string; offendingModels []string }` in `internal/fleet/overview.go` (mirrors `overviewHealth`). `status == ""` ⇒ clean ⇒ row's `ModelSupport` stays nil (data-model.md §Internal reduce type).
- [X] T005 [P] In `internal/fleet/model_signal.go` add the `failureIssue{Number, Title, Body}` type, the `ghFailureIssuesAPI(ctx, repo) ([]failureIssue, error)` package-level seam running `gh issue list --repo <repo> --label agentic-workflows --state open --json number,title,body --limit 100` (wrap errors with `ghErr`/`%w`), and the pure `parseModelFailure(body string) bool` implementing upstream-signals.md §Seam A rules: true iff body contains `failureIssueMarker` AND its `failure_categories:` value (up to the next `,` or `-->`) contains `modelNotSupportedToken`.
- [X] T006 In `internal/fleet/model_signal.go` add the `ghRepoVariablesAPI(ctx, repo) (map[string]string, error)` seam running `gh variable list --repo <repo> --json name,value` (decode to name→value), and the pure `resolveEffectiveModel(vars map[string]string) string` implementing the fallback chain `varAgentModelCopilot || varDefaultModelCopilot || ghAwBakedDefaultCopilotModel` (non-empty wins; upstream-signals.md §Seam B). (Same file as T005 — sequential.)
- [X] T007 [P] Add table-driven unit tests in `internal/fleet/model_signal_test.go` for `parseModelFailure` (marker present/absent, `model_not_supported_error` vs `model_not_supported`, `agent_failure`-only → false, `[aw] No-Op Runs` body lacking the marker → false) and `resolveEffectiveModel` (agent var set, default var set, both unset → baked default, empty-string values skipped). Load the Phase-1 fixtures where useful.
- [X] T008 Implement `collectOverviewModelHealth(ctx context.Context, repos []string) (map[string]overviewModelHealth, []Diagnostic)` in `internal/fleet/overview.go` per data-model.md §State transitions: iterate `repos` (already sorted `overviewRepoNames` order — FR-012); call `ghFailureIssuesAPI`; on error set `status = modelSupportUndetermined`, empty models, and append a **warnings-channel** diagnostic built as `withDiagnosticSignal(buildRepoErrorDiag(repo, msg), repo, overviewSignalModel)` — add `overviewSignalModel = "model"` beside `overviewSignalRuns`/`overviewSignalDrift` in `overview.go`; it carries `fieldRepo`+`fieldSignal` and routes through `splitOverviewDiags` exactly like the sibling runs-unavailable diagnostic (warnings for a generic read error; a matching actionable hint only if the error classifies as rate-limit/inaccessible). **Do not** use `newSoftDiagnostic` here: it is *unconditionally* `DiagHint`-coded, so it would always misroute the undetermined status to the hints channel (contract §3 forbids a plain hint for undetermined) with the wrong `{hint, repo}` fields. If any issue is `parseModelFailure`-true, set `status = modelSupportAffected` and — **only then** — call `ghRepoVariablesAPI` (gated to flagged repos), resolving the effective model into a sorted+deduped single-element `offendingModels` (on variable-read error leave models empty ⇒ unknown, FR-002); no matching issue ⇒ leave `status == ""` (clean). Depends on T003-T006.
- [X] T009 Wire the engine into `Overview` in `internal/fleet/overview.go`: add a **third** `wg.Go` calling `collectOverviewModelHealth` alongside the drift/health collectors, capture its rows + diags, merge the model diags into the returned `diags` slice (they flow through `sortOverviewDiagnostics`), and extend `joinOverviewRow` to accept the `overviewModelHealth` and set `row.ModelSupport = &ModelSupport{Status: h.status, OffendingModels: h.offendingModels}` when `h.status != ""` (nil otherwise). Depends on T008.
- [X] T010 Add collector/reduce tests in `internal/fleet/overview_test.go`: a `withModelStubs`-style helper (mirroring `withLogsStubs`) that swaps `ghFailureIssuesAPI`/`ghRepoVariablesAPI` and returns a restore func; assert the full data-model.md fixture matrix — model-failure+unset-var → `{affected, [claude-sonnet-4.6]}`, model-failure+override → `{affected, [gpt-5-mini]}`, unrelated → nil (FR-009), healthy → nil (SC-003), issues-read-error → `{undetermined, []}` + a **warnings-channel** diag carrying `fieldRepo`+`fieldSignal` (assert its `Code` is **not** `DiagHint`, so `splitOverviewDiags` routes it to warnings, not hints), var-read-error → `{affected, []}` (unknown). Depends on T008/T009.

**Checkpoint**: The engine produces a per-repo `ModelSupport` and the additive JSON field is present. `make test` for `internal/fleet` is green. No user-facing text/hint yet.

---

## Phase 3: User Story 1 - See which repos are failing on an unsupported model (Priority: P1) 🎯 MVP

**Goal**: The `overview` stdout dashboard names each affected repo and its offending Copilot model inside that repo's per-repo detail block — zero per-run logs needed (SC-001).

**Independent Test**: Run `overview` against the model-failure fixture; the affected repo's detail block shows `model-support: FAILING — offending model(s): claude-sonnet-4.6`; the unrelated-failure and healthy repos show no model line. Fully offline.

### Tests for User Story 1 ⚠️ (write first, watch them fail)

- [X] T011 [P] [US1] Add text-render assertions in `cmd/overview_test.go` per contract §2 (lines are 2-space indented under the repo header, printed after the runs line): affected+named → `model-support: FAILING — offending model(s): claude-sonnet-4.6`; affected+unknown (empty `OffendingModels`) → `model-support: FAILING — offending model(s): unknown`; undetermined → `model-support: undetermined — model-failure signal unavailable` **only** when the detail block is already open for another reason; clean, drift-aligned repo → no model line and no detail block at all (SC-003). Multi-model → comma-joined sorted set (a **renderer-only** test: hand-construct a `RepoOverview` whose `ModelSupport.OffendingModels` holds several models — the v1 collector resolves a single effective model per repo, so this exercises `printOverviewModelSupport`'s comma-join defensively, not collector output; see plan Research Decision 2 and spec "Multiple offending models").

### Implementation for User Story 1

- [X] T012 [US1] Add `printOverviewModelSupport(cmd *cobra.Command, row fleet.RepoOverview)` in `cmd/overview.go` and call it from `printOverviewDetail` after `printOverviewRunsDetail`. Render per contract §2 rules (FAILING+models / FAILING+unknown / undetermined), keyed on `row.ModelSupport`. Because it is only reached through `printOverviewDetail` (gated by the unchanged `needsOverviewDetail`), the "undetermined text only when a block already renders" rule (FR-011) holds with **no** change to `needsOverviewDetail` (research Decision 3). Make T011 pass.

**Checkpoint**: US1 is independently functional — the MVP delivers repo + offending model on stdout. `make test ./cmd/...` green.

---

## Phase 4: User Story 2 - Get an actionable remediation without leaving the dashboard (Priority: P2)

**Goal**: Every flagged repo gets exactly one stderr hint naming `GH_AW_DEFAULT_MODEL_COPILOT` as the fix (FR-004, SC-005); a healthy fleet emits none (FR-008).

**Independent Test**: With the model-failure fixture, stderr carries one `[hint] <repo>: … Set the GH_AW_DEFAULT_MODEL_COPILOT Actions variable …`; with the healthy fixture, no model hint appears.

### Tests for User Story 2 ⚠️

- [X] T013 [P] [US2] Add assertions in `internal/fleet/overview_test.go` (collector returns exactly one `DiagHint` diagnostic per **affected** repo, Fields carry `fieldRepo`+`fieldHint`, message names `GH_AW_DEFAULT_MODEL_COPILOT`; none for undetermined/clean) and in `cmd/overview_test.go` (the hint routes through `splitOverviewDiags` → hints and is emitted by `emitOverviewDiagnostics`; healthy fleet emits zero model hints — FR-008).

### Implementation for User Story 2

- [X] T014 [US2] In `collectOverviewModelHealth` (`internal/fleet/overview.go`) append one remediation `Diagnostic{Code: DiagHint, Message: <names GH_AW_DEFAULT_MODEL_COPILOT>, Fields: {fieldRepo, fieldHint}}` per affected repo (message per quickstart.md/contract §3). This is exactly the shape `newSoftDiagnostic(repo, msg)` (`consumption.go:436`) returns, so you MAY use that helper here (it sets `Code: DiagHint`, `Fields: {fieldHint: msg, fieldRepo: repo}`) — unlike the undetermined path in T008, the hints channel is correct for the affected-repo remediation. Confirm `splitOverviewDiags` already routes `DiagHint` → hints (cmd/overview.go:130) so no cmd-side routing change is needed. Make T013 pass. Depends on Phase 2.

**Checkpoint**: US1 + US2 both work independently — diagnosis on stdout, remediation on stderr.

---

## Phase 5: User Story 3 - Consume the signal programmatically (Priority: P3)

**Goal**: `overview --output json` exposes `model_support` as an additive, `omitempty` pointer field; existing consumers are byte-unaffected (FR-005/006, SC-004).

**Independent Test**: `overview --output json` on the fixtures: affected → `model_support.offending_models` present; unknown → `offending_models` omitted; undetermined → `status` only; clean → `model_support` absent; `schema_version` still `1`.

### Tests for User Story 3 ⚠️

- [X] T015 [P] [US3] Add JSON-envelope assertions in `cmd/overview_test.go` per contract §4: affected repo row carries `model_support: {status:"affected", offending_models:[…]}`; affected-unknown omits `offending_models`; undetermined → `{status:"undetermined"}`; a clean repo row omits `model_support` entirely and is byte-identical to today's keys; envelope `schema_version` unchanged; `result.total` unchanged (SC-004).

### Implementation for User Story 3

- [X] T016 [US3] Confirm the additive JSON contract holds: the pointer-`omitempty` tag from T003 already omits `model_support` on clean rows, and `OffendingModels` uses `omitempty`. Assert `cmd.SchemaVersion` is **not** bumped and no existing `RepoOverview` field was renamed/removed/repurposed (FR-006). Add any missing `omitempty` only if T015 reveals a gap. Make T015 pass. Depends on Phase 2.

**Checkpoint**: All three surfacing channels (text, hint, JSON) are independently functional over the shared engine.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: User-facing documentation (constitution Development Workflow requires it in-change) and the full acceptance gate.

- [X] T017 [P] Update `README.md` §Commands table: the `overview` row's one-line description gains "flags repos failing on unsupported/deprecated Copilot models" (plan Documentation Impact).
- [X] T018 [P] Update `docs/src/content/docs/overview.md`: new short section documenting the model-support annotation (contract §2), the `GH_AW_DEFAULT_MODEL_COPILOT` remediation hint (§3), and the additive `model_support` JSON field (§4); the JSON Output section notes the additive field explicitly. Run the `markdownlint` skill.
- [X] T019 [P] Add one sentence to the "Overview dashboard" architecture paragraph in both `AGENTS.md` and `CLAUDE.md`: the failure-issue-marker model-support signal, the effective-model resolution seam, and the version-coupled `ghAwBakedDefaultCopilotModel` constant as a gh-aw-pin maintenance touch-point.
- [X] T020 Run `make ci` (fmt-check, vet, lint, full suite) to green (SC-006), then walk quickstart.md: verify a healthy/drift-aligned fleet's `overview` text + JSON is byte-identical to pre-feature output (SC-003) and the exit code stays drift-gated with a model failure present (FR-007). Fix any fmt/lint/test fallout.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately.
- **Foundational (Phase 2)**: depends on Setup — **BLOCKS all user stories** (struct field + collector are compile-time prerequisites).
- **User Stories (Phase 3-5)**: each depends only on Foundational, not on each other. Order by priority (P1 → P2 → P3) or run in parallel once Foundational is done.
- **Polish (Phase 6)**: depends on the user stories you intend to ship.

### User Story Dependencies

- **US1 (P1)**: after Phase 2. No dependency on US2/US3. (MVP.)
- **US2 (P2)**: after Phase 2. Independent of US1/US3 (adds a hint diagnostic in the collector; text render is untouched).
- **US3 (P3)**: after Phase 2. Independent of US1/US2 (the JSON tag ships in Phase 2; US3 is the additive-contract guarantee + docs).

### Within-phase ordering

- T005 → T006 (same file `model_signal.go`).
- T003 → T004 (same file `overview.go`); T005/T006 (model_signal.go) run parallel to them.
- T007 needs T005/T006. T008 needs T003-T006. T009 needs T008. T010 needs T008/T009.
- Each story: test task before implementation task (write it, watch it fail, then implement).

### Parallel Opportunities

- **Setup**: T002 [P] runs alongside T001.
- **Foundational**: T005 [P] (model_signal.go) runs alongside T003/T004 (overview.go); T007 [P] once its subjects exist.
- **Across stories**: once Phase 2 lands, US1 (cmd/overview.go), US2 (overview.go collector), and US3 (cmd JSON tests) touch largely different surfaces — US1 and US3 test files can be written in parallel; US2's impl edits overview.go so serialize it against other overview.go edits.
- **Polish**: T017 / T018 / T019 [P] (three different doc trees).

---

## Parallel Example: Foundational engine

```bash
# overview.go type work and model_signal.go seam work touch different files:
Task: "T003 Add ModelSupport type + status constants + RepoOverview field in internal/fleet/overview.go"
Task: "T005 Add failureIssue type + ghFailureIssuesAPI seam + parseModelFailure in internal/fleet/model_signal.go"

# Once parsers/resolver exist, their unit tests run in parallel with collector wiring:
Task: "T007 Table-driven parseModelFailure + resolveEffectiveModel tests in internal/fleet/model_signal_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup, then Phase 2 Foundational (the engine), then Phase 3 US1 (text line).
2. **STOP and VALIDATE**: `go run . overview` against the model-failure fixture names the repo + offending model on stdout; healthy fleet unchanged.
3. Ship — this alone delivers SC-001.

### Incremental Delivery

1. Setup + Foundational → engine ready (JSON field present, no surfacing).
2. Add US1 → text detail line (MVP, SC-001/SC-002).
3. Add US2 → remediation hint (SC-005).
4. Add US3 → additive-JSON guarantee (SC-004).
5. Add Polish → docs + full `make ci` gate (SC-003/SC-006).

Each story adds one channel without touching the others' surfaces.

---

## Notes

- **Read-only command** — no `--apply`, no three-turn mutation flow; the build-green + `make ci` gate is the acceptance bar (Constitution §Testing Standards).
- **No new third-party dependency, no `cmd.SchemaVersion` bump** — verify both in T020 (FR-006).
- **Exit code stays drift-only** (`overviewExitCode`, cmd/overview.go:286) — do not touch it (FR-007).
- **`ghAwBakedDefaultCopilotModel` is version-coupled** — its godoc must warn future maintainers to re-verify on every `github/gh-aw` pin bump (research Decision 2).
- **Determinism**: sorted `overviewRepoNames` iteration + sorted/deduped `OffendingModels` + `DiagHint` sort by `fieldRepo` — all fixture-testable offline (FR-012, SC-002).
- Commit after each task or logical group; conventional-commit `ci(workflows)` scope per repo invariants.
