---

description: "Task list for 007-billing-metadata-fields — Profile.Tier + RepoSpec.CostCenter additive schema fields surfaced in `gh-aw-fleet list`"
---

# Tasks: Billing Metadata Fields

**Input**: Design documents from `/specs/007-billing-metadata-fields/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/list-output.json, contracts/list-text-output.md, quickstart.md

**Tests**: Included. Plan §Constitution Check (II. Testing Standards) explicitly requires extending `list_result_test.go` and adding a `tiers` empty-map JSON assertion (FR-007 edge case). The existing `internal/fleet/profiles_parity_test.go` must remain green after fixtures gain tier annotations (FR-012).

**Organization**: Two P1 user stories ship in a single bundled PR (research.md Decision 7) but are organized as independent phases below — Story 1 (Tier) and Story 2 (CostCenter) are each independently testable on their own slice of the schema. They share files; tasks within a story are parallelizable across distinct files; cross-story tasks targeting the same file are sequenced.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1 = Tier, US2 = CostCenter)
- Include exact file paths in descriptions

## Path Conventions

Single-binary Go CLI. No separate `tests/` tree — `*_test.go` lives next to the code under test. Paths below are repo-root-relative.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No project initialization is needed — this feature is an additive extension on an established Go module. This phase is intentionally minimal.

- [ ] T001 Confirm working tree is on branch `007-billing-metadata-fields` and `make ci` passes baseline (no schema fields added yet) — captures green-before-change evidence for the PR description per AGENTS.md "Local gate" rule.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema struct fields underpin both user stories' rendering work. Without the Go fields on `Profile` and `RepoSpec`, neither `BuildListResult` nor `cmd/list.go` has anything to read.

**⚠️ CRITICAL**: No user-story implementation can begin until T002 + T003 are merged into the working tree.

- [ ] T002 [P] Add the `Tier string \`json:"tier,omitempty"\`` field to `Profile` in `internal/fleet/schema.go` (per data-model.md §Profile). Update the godoc comment on `Profile` to mention the new advisory tier label, and add a one-sentence godoc on the field itself describing the recommended `minimal | standard | premium` vocabulary, that the value is advisory (no enforcement, FR-010), and that empty is equivalent to unset.
- [ ] T003 [P] Add the `CostCenter string \`json:"cost_center,omitempty"\`` field to `RepoSpec` in `internal/fleet/schema.go` (per data-model.md §RepoSpec). Update the godoc on `RepoSpec`, and add a one-sentence field godoc describing the free-form per-repo budget-attribution label, no validation (FR-011), no per-file special handling (FR-016).
- [ ] T004 Verify `fleet.SchemaVersion` constant in `internal/fleet/schema.go:9` remains `1` (no bump — FR-004). Add a one-line code comment on that constant only if reviewing the godoc reveals the additive-rule rationale is unclear; otherwise leave untouched.
- [ ] T005 Run `go build ./...` and `go vet ./...` from repo root to confirm schema changes compile cleanly before any rendering code touches the new fields.

**Checkpoint**: Foundation ready — the two schema fields exist, the on-disk format silently accepts both, and the schema-version contract is preserved. User-story phases can now begin.

---

## Phase 3: User Story 1 — Cost-aware profile selection via Tier (Priority: P1) 🎯 MVP

**Goal**: A fleet operator can annotate any profile in `fleet.json` / `fleet.local.json` / `profiles/default.json` with an advisory `tier` label and immediately read it back in both text and JSON output of `gh-aw-fleet list`. Existing configs with no tier annotations continue to load and render correctly.

**Independent Test**: With only T002 + T004 + T005 from Phase 2 merged and only the tasks under Phase 3 applied (no T003 / T010-T012), set `tier: "standard"` on the `default` profile in `fleet.json` and run `go run . list` — the `TIERS` column shows `[standard]` paired 1:1 with `[default]` in `PROFILES`. `go run . list --output json | jq '.result.repos[0].profile_tiers'` returns `{"default":"standard"}`. Removing the annotation makes the column render `[]` in text and `{}` in JSON. The `cost_center` work is not required for this story to demonstrate value.

### Tests for User Story 1 ⚠️

> **NOTE**: Write tests FIRST, ensure they FAIL before implementation. Then implement and confirm they pass.

- [ ] T006 [P] [US1] Extend `internal/fleet/list_result_test.go` with a table-driven test asserting that `BuildListResult` populates `ListRow.ProfileTiers` correctly: (a) a row with one profile carrying `tier: "standard"` yields `{"default":"standard"}`; (b) a row with two profiles, only one tiered, includes only that key in the map; (c) a row whose every profile is untiered yields an **empty map**, not nil — `require.NotNil(t, row.ProfileTiers)` plus `require.Empty(t, row.ProfileTiers)`. The non-nil contract directly enforces FR-007.
- [ ] T007 [P] [US1] In a new file `cmd/list_test.go`, unit-test the new `tiersForRow` helper (introduced by T009) by table-driven cases mirroring data-model.md §Rendering examples: one tiered profile, all-untiered (returns `[]string{}`), mixed (returns `["-", "standard", "-"]`-style slice with `-` placeholders). The helper is a pure function — no tabwriter dependency in the test.
- [ ] T008 [P] [US1] Add a JSON-marshalling assertion to `internal/fleet/list_result_test.go` (or extend an existing JSON test there): marshal a `ListRow` whose `ProfileTiers` is an empty initialized map and assert the output substring contains `"profile_tiers":{}` — never `"profile_tiers":null`. This is the FR-007 edge case the plan explicitly calls out.

### Implementation for User Story 1

- [ ] T009 [US1] Add the `ProfileTiers map[string]string \`json:"profile_tiers"\`` field to `ListRow` in `internal/fleet/list_result.go` (per data-model.md §ListRow). Field tag is `"profile_tiers"` with **no** `omitempty` — the contract is always-present. Update `ListRow`'s godoc to describe the per-profile tier mapping; add a field-level godoc noting the empty-map (not nil) invariant.
- [ ] T010 [US1] In `BuildListResult` in `internal/fleet/list_result.go`, populate `ProfileTiers` for each row: initialize as `map[string]string{}` (NEVER nil), iterate `spec.Profiles`, and for each name where `cfg.Profiles[name].Tier != ""` insert the entry. Profiles with empty tier are omitted from the map (per data-model.md §Population). Make this change in the same iteration that already builds `Profiles []string` so order and source are visibly consistent.
- [ ] T011 [US1] Add the `tiersForRow(profiles []string, profileDefs map[string]Profile) []string` private helper to `cmd/list.go`. Behavior per data-model.md §tabwriter (text-mode) output: walk `profiles` in order; for each, append the tier value if non-empty otherwise append `"-"`. **Special case**: if every position would be `"-"`, return `[]string{}` so `%v` formats as `[]` matching the existing slice-empty convention. Add a one-sentence godoc explaining the special case (this is the non-obvious behavior reviewers must understand). Not marked `[P]` because T011 and T012 both touch `cmd/list.go` and land in the same commit per the Notes section below.
- [ ] T012 [US1] Update the tabwriter header and row format in `cmd/list.go` to insert the `TIERS` column between `PROFILES` and `ENGINE` (per contracts/list-text-output.md §Header row). The new `fmt.Fprintf` format string becomes `"%s\t%v\t%v\t%s\t%d\t%v\t%d\n"` (one extra `%v` after `PROFILES`), passing `tiersForRow(spec.Profiles, cfg.Profiles)` as the new argument. Keep all other columns and their data sources unchanged.
- [ ] T013 [US1] Add `tier` annotations to every profile in the public example `fleet.json` per research.md Decision 6 honest cost framing: `default` → `standard`, `quality-plus` → `premium`, `security-plus` → `premium`, `docs-plus` → `premium`, `community-plus` → `standard`, `observability-plus` → `premium`. Verify all six profiles ship with non-empty tier (FR-013, SC-004).
- [ ] T014 [US1] Mirror the `tier: "standard"` annotation onto the canonical default profile in `profiles/default.json` to preserve the byte-identical-mirror invariant for the `default` profile (FR-012). Confirm by running `internal/fleet/profiles_parity_test.go` — it must stay green.
- [ ] T015 [US1] Run `go test ./internal/fleet/... ./cmd/...` and verify T006, T007, T008 now pass; verify `profiles_parity_test.go` stays green after T013 + T014. Run `go run . list` against the modified `fleet.json` and visually confirm the `TIERS` column renders per the example rows in contracts/list-text-output.md (paste the output into the PR description for the before/after evidence required by AGENTS.md §Development Workflow).

**Checkpoint**: User Story 1 fully functional and independently testable. Operators can read profile tier in both text and JSON modes. Existing configs without tier render `[]` in text and `{}` in JSON. The bundled PR may stop here and demo MVP value, even though Story 2 ships in the same PR per Decision 7.

---

## Phase 4: User Story 2 — Budget attribution per repo via CostCenter (Priority: P1)

**Goal**: A fleet operator can annotate any repo entry in `fleet.local.json` (or `fleet.json` — the tool does not discriminate per FR-016) with a free-form `cost_center` label and read it back in both text and JSON output of `gh-aw-fleet list`. The field is always emitted in JSON (empty string when unset) and uses the existing `-` placeholder in text mode.

**Independent Test (value-prop independence)**: US2 demonstrates value without US1's tier visibility — the `COST_CENTER` column is meaningful on its own. **File-edit independence does not hold**: T020 extends the same tabwriter format string T012 introduces, so T012 must land first within `cmd/list.go`. With T003 + T004 + T005 from Phase 2 merged AND T012 from Phase 3 merged (tabwriter format-string baseline), apply only T016–T020 from Phase 4: set `cost_center: "platform-eng"` on a repo entry in `fleet.local.json` and run `go run . list` — the `COST_CENTER` column shows `platform-eng`. `go run . list --output json | jq '.result.repos[0].cost_center'` returns `"platform-eng"`. A repo without the annotation renders `-` in text and `""` in JSON. The data-side US1 work (fixtures T013/T014, tests T006–T008) is not required for this story to demonstrate value.

### Tests for User Story 2 ⚠️

> **NOTE**: Write tests FIRST, ensure they FAIL before implementation. Then implement and confirm they pass.

- [ ] T016 [P] [US2] Extend `internal/fleet/list_result_test.go` with a test asserting `BuildListResult` copies `spec.CostCenter` to `ListRow.CostCenter` verbatim: (a) repo with `cost_center: "platform-eng"` → `row.CostCenter == "platform-eng"`; (b) repo with no annotation → `row.CostCenter == ""`. The field must always be present in the struct (no `*string`, no `omitempty` on the envelope tag).
- [ ] T017 [P] [US2] Add a JSON-marshalling assertion in `internal/fleet/list_result_test.go`: a `ListRow` with `CostCenter: ""` marshals to a JSON object containing `"cost_center":""` exactly (the field is always present even when unset, per FR-008 and contracts/list-output.json row 2 + row 3 examples).

### Implementation for User Story 2

- [ ] T018 [US2] Add the `CostCenter string \`json:"cost_center"\`` field to `ListRow` in `internal/fleet/list_result.go` (per data-model.md §ListRow). Tag is `"cost_center"` with **no** `omitempty` — always-present contract (FR-008). Update `ListRow`'s godoc; add a field godoc noting the always-emitted, empty-when-unset behavior.
- [ ] T019 [US2] In `BuildListResult` in `internal/fleet/list_result.go`, populate `row.CostCenter = spec.CostCenter` (direct copy from the `RepoSpec`). Place this assignment alongside other simple field copies like `row.Engine`.
- [ ] T020 [US2] Update the tabwriter header and row format in `cmd/list.go` to append a `COST_CENTER` column at the end of the row (per contracts/list-text-output.md §Header row). The format string becomes `"%s\t%v\t%v\t%s\t%d\t%v\t%d\t%s\n"` after T012's TIERS insertion (one extra `%s` at the end), passing `orDash(spec.CostCenter)` as the new argument. The existing `orDash` helper renders `-` when the value is the empty string — exactly the contract.

**Checkpoint**: User Stories 1 AND 2 both work independently and together. The bundled PR ships both columns and both schema fields in one review surface.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Documentation updates (FR-015) that describe both fields' semantics together, the local-gate evidence required by AGENTS.md, and round-trip verification of SC-006.

- [ ] T021 [P] Update `AGENTS.md` with a one-paragraph description of both fields under the fleet-schema subsection: advisory `tier` label on profiles, free-form `cost_center` on repos, neither enforced, both prerequisite group-by keys for the planned `gh-aw-fleet consumption` subcommand. Per FR-015 + plan.md §Project Structure. Do **not** restate per-field details that godoc on the struct fields already carries — link by mentioning the relevant types.
- [ ] T022 [P] Update `skills/fleet-onboard-repo/SKILL.md` to mention `cost_center` as an optional onboarding field operators may set on the `RepoSpec` in `fleet.local.json` during repo onboarding (per plan.md §Project Structure and FR-015). Keep the addition short — one sentence in the step that already discusses repo entries.
- [ ] T023 [P] Update `skills/fleet-build-profile/SKILL.md` to mention `tier` in the Step 2 (sources/pins) flow as an optional advisory label operators set when materializing a new profile (per plan.md §Project Structure and FR-015). One sentence is sufficient.
- [ ] T024 Run `make ci` from repo root and confirm the full gate (`fmt-check vet lint test`) passes. Per AGENTS.md "Local gate" rule, no task is complete until `make ci` is green. Capture the output for the PR description.
- [ ] T025 Round-trip verification (SC-006): load `fleet.json` via `LoadConfig`, marshal back with `json.MarshalIndent` (4-space indent matching the existing file), and `diff` against the on-disk file. Result must be byte-identical (or trivially-different only by trailing-newline rules the existing tests already encode). If there is no existing round-trip test that covers `Profile.Tier` and `RepoSpec.CostCenter`, add a one-shot test in `internal/fleet/load_test.go` that materializes a minimal config with both fields, round-trips it through `json.Marshal` → `json.Unmarshal`, and asserts deep equality (per SC-006). Skip task only if existing round-trip coverage in `load_test.go` already exercises both fields.
- [ ] T026 Run the three quickstart.md scenarios end-to-end against `go run . list` and `go run . list --output json | jq ...` and confirm output matches the documented examples (per AGENTS.md before/after `go run . list` paste requirement). Paste both outputs into the PR description.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: T001 has no code dependencies — runs first to capture green-baseline.
- **Phase 2 (Foundational)**: T002 + T003 + T004 + T005 must complete before any Phase 3 / Phase 4 work touches `list_result.go` or `cmd/list.go`. T002 and T003 are independent file edits to disjoint struct types and can run in parallel.
- **Phase 3 (US1 — Tier)**: depends on Phase 2 completing. Stories 1 and 2 share files (`list_result.go`, `cmd/list.go`) so Phase 3 and Phase 4 are sequenced within those files even though the user-story value-prop is independent.
- **Phase 4 (US2 — CostCenter)**: depends on Phase 2 completing AND on T012 (US1's tabwriter format-string change) landing first — T020 extends the same format string T012 introduced. The `list_result.go` edits (T018, T019) can land independently of T009/T010 but the test file (`list_result_test.go`) is touched by both stories so sequencing the test edits avoids merge churn.
- **Phase 5 (Polish)**: T021/T022/T023 (docs) are independent of code and of each other; T024 (make ci) depends on **all** prior code tasks; T025 (round-trip) depends on T002+T003 only; T026 (end-to-end) depends on all code in Phases 2-4.

### User Story Dependencies

- **US1 (Tier, P1)**: depends on T002 (schema field) from Phase 2.
- **US2 (CostCenter, P1)**: depends on T003 (schema field) from Phase 2 AND on T012 from US1 because the tabwriter format-string change is incremental — T020 modifies the format string T012 introduces. If the two stories were genuinely independent at the file level the [P] markers would extend across them; here they don't.

### Within Each User Story

- Tests (T006/T007/T008 for US1, T016/T017 for US2) MUST be written and FAIL before implementation per AGENTS.md "Test Writing" / TDD guidance.
- Schema-side (`list_result.go`) before render-side (`cmd/list.go`).
- Fixture data (`fleet.json` + `profiles/default.json`) lands LAST in US1 (T013 + T014) so the parity test sees both files updated atomically.

### Parallel Opportunities

- T002 and T003 (different struct types in `schema.go` — Go file edits with no overlap region) can run in parallel.
- T006, T007, T008 (test additions in two different test files: `internal/fleet/list_result_test.go` and a new `cmd/list_test.go`) can run in parallel.
- T011 (helper in `cmd/list.go`) overlaps the same file as T012 and lands in the same commit — not marked `[P]`.
- T021, T022, T023 (three docs files: AGENTS.md, two SKILL.md files) can run in parallel.
- T013 and T014 must land in the same commit (parity invariant) — not parallel.

---

## Parallel Example: User Story 1

```bash
# Launch all US1 tests together (different files, no dependencies):
Task: "Extend internal/fleet/list_result_test.go with ProfileTiers population assertions (T006)"
Task: "Unit-test cmd/list.go tiersForRow helper in new cmd/list_test.go (T007)"
Task: "Add JSON-marshalling empty-map assertion in internal/fleet/list_result_test.go (T008)"

# Phase 2 schema fields (different struct types, same file but disjoint regions):
Task: "Add Profile.Tier field in internal/fleet/schema.go (T002)"
Task: "Add RepoSpec.CostCenter field in internal/fleet/schema.go (T003)"
```

## Parallel Example: Polish phase

```bash
# Launch all docs updates together (three independent files):
Task: "Update AGENTS.md fleet-schema section (T021)"
Task: "Update skills/fleet-onboard-repo/SKILL.md (T022)"
Task: "Update skills/fleet-build-profile/SKILL.md (T023)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

Per research.md Decision 7, the PR bundles both stories. But if a reviewer wants to validate MVP value before reviewing Story 2:

1. Complete Phase 1: Setup (T001)
2. Complete Phase 2: Foundational (T002, T003, T004, T005)
3. Complete Phase 3: User Story 1 (T006-T015)
4. **STOP and VALIDATE**: run quickstart.md Task 1 (annotate a profile with tier) and confirm output matches. Story 1 alone delivers cost-tier visibility — the highest-priority outcome per spec.md.

### Incremental Delivery (single bundled PR)

1. Setup + Foundational → Schema fields land, no behavior change visible yet.
2. Add User Story 1 → `TIERS` column visible in text + JSON; MVP demo possible.
3. Add User Story 2 → `COST_CENTER` column visible. Both columns now ship.
4. Polish → docs + round-trip verification + `make ci` green-light.

The PR description should walk a reviewer through (1) → (4) with before/after `go run . list` paste at the (2) and (3) boundaries so the reviewer can see each column's value addition.

### Sequencing Constraints

- T013 + T014 in the same commit (parity invariant; the test enforces it).
- T012 before T020 (T020 extends the format string T012 changes).
- All tests (T006, T007, T008, T016, T017) authored before their corresponding implementation (TDD per AGENTS.md).
- T024 (`make ci`) is the final gate — nothing ships until it passes.

---

## Notes

- [P] tasks = different files OR disjoint regions of the same file, no dependencies on incomplete tasks.
- [Story] label maps each task to its user story (US1 = Tier, US2 = CostCenter) for traceability into spec.md acceptance scenarios.
- Both user stories ship in a single bundled PR per research.md Decision 7; the per-story phase organization is for review traceability and independent-test framing, not for separate PRs.
- Verify all new tests fail before implementing.
- Commit boundaries: schema → render → fixtures → docs is a defensible four-commit sequence inside the PR if reviewers prefer commit-level granularity; one squashed commit is equally acceptable per CLAUDE.md commit-conventions guidance.
- `make ci` is the local gate — do not claim any task done until `make ci` is green per AGENTS.md.
- Never bypass gpg signing on the PR commit per AGENTS.md hard invariants.
- The `CHANGELOG.md` entry is generated by release-please from the conventional commit subject — write the subject as `feat(billing): add Profile.Tier and RepoSpec.CostCenter metadata fields` so it surfaces under "Added" in the next release section. Do not hand-edit `CHANGELOG.md`.
