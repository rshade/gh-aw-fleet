---

description: "Task list for implementing the add <owner/repo> subcommand"
---

# Tasks: `add <owner/repo>` Subcommand for Fleet Onboarding

**Input**: Design documents from `specs/001-add-subcommand/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli.md, quickstart.md

**Tests**: INCLUDED. spec.md §"Testing Strategy" explicitly requests
unit tests for `Add()`, `SaveConfig` round-trip, duplicate-repo error
path, and unknown-profile error path. A manual integration test is
documented in quickstart.md §"Manual integration test."

**Organization**: Tasks are grouped by user story so each can be
implemented, tested, and delivered independently. US1 = happy-path
onboarding (MVP), US2 = actionable validation errors, US3 = per-repo
customization flags.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on
  incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- All file paths are relative to the repository root:
  `/mnt/c/GitHub/go/src/github.com/rshade/gh-aw/`

## Path Conventions

Single-project Go CLI. Key locations:

- `cmd/` — cobra command definitions (one file per subcommand)
- `internal/fleet/` — core logic, no cobra dependencies
- `skills/` — operator workflow documentation
- Tests live alongside source in the same package (`_test.go`)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the build environment is clean and verify the
one optional dependency called for in research.md §4.

- [X] T001 Verify baseline build by running `go build ./...` and `go vet ./...` on the current branch; both must exit 0 before any implementation begins. If `golang.org/x/term` is not already a transitive dependency (`go mod graph | grep 'golang.org/x/term'`), record that T011 will need to add it to `go.mod`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Establish the pure helpers and types that every user
story depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is
complete.

- [X] T002 Create `internal/fleet/add.go` with package docs, `AddOptions` struct, and `AddResult` struct per data-model.md §"New types"
- [X] T003 Implement `validateSlug(s string) (string, error)` in `internal/fleet/add.go` per research.md §6 (trim, reject empty, split on `/`, require exactly 2 non-empty halves, lowercase, enforce `[a-z0-9._-]+` character class)
- [X] T004 Implement `BuildMinimalLocalConfig(repo string, spec RepoSpec) *Config` in `internal/fleet/add.go` per data-model.md §"Supporting helper: BuildMinimalLocalConfig" (returns Config with only `Version` and `Repos` set)
- [X] T005 [P] Rename `SaveConfig` → `SaveLocalConfig` in `internal/fleet/load.go:95-99`; change target from `ConfigFile` to `LocalConfigFile`; delete the old `SaveConfig` declaration entirely so no function in the package can write to `fleet.json` (satisfies FR-021)
- [X] T006 Create `internal/fleet/add_test.go` with `TestValidateSlug` table-driven test covering: valid slugs (incl. case normalization, allowed chars `.`, `_`, `-`), empty string, missing slash, empty halves, whitespace, too many slashes, invalid characters
- [X] T007 Add `TestBuildMinimalLocalConfig` to `internal/fleet/add_test.go` asserting the produced `*Config` JSON-marshals to a blob with exactly top-level keys `version` and `repos` (no `defaults`, no `profiles`)
- [X] T008 Add `TestSaveLocalConfig_WritesLocalFile` to `internal/fleet/add_test.go` using `t.TempDir()`: call `SaveLocalConfig`, assert `fleet.local.json` exists with expected content, assert `fleet.json` was NOT created

**Checkpoint**: Foundation ready — all helpers are implemented and
unit-tested. User story phases can now proceed.

---

## Phase 3: User Story 1 — Onboard a new repo declaratively with a profile (Priority: P1) 🎯 MVP

**Goal**: Operator runs `gh-aw-fleet add rshade/new-repo --profile default [--apply --yes]` and gets either (a) a dry-run preview or (b) a written `fleet.local.json` and a "next: gh-aw-fleet deploy" hint. Default-profile case only; customization flags come in US3.

**Independent Test**: On a fresh checkout with only `fleet.json`
present (no `fleet.local.json`), run the command in dry-run mode,
confirm preview on stderr + workflow list on stdout, no file
changes. Re-run with `--apply --yes`, confirm `fleet.local.json` is
written as a minimal file and `gh-aw-fleet list` now surfaces the
new repo.

### Tests for User Story 1

> **NOTE**: Write each test and confirm it FAILS before implementing the corresponding behavior.

- [X] T009 [US1] Add `TestAdd_DryRun_HappyPath` to `internal/fleet/add_test.go` — given a Config with a `default` profile and no repo entry, call `Add()` with `Apply=false`; assert returned `AddResult` has correct `Repo`, `Profiles`, non-empty `Resolved`, `WroteLocal == false`, and no file is written to the temp dir
- [X] T010 [US1] Add `TestAdd_Apply_HappyPath` to `internal/fleet/add_test.go` — same Config as T009, but `Apply=true, Confirmed=true`; assert `fleet.local.json` is written to `t.TempDir()`, its content matches `BuildMinimalLocalConfig` output exactly, and `fleet.json` in the temp dir is byte-identical before and after
- [X] T011 [US1] Add `TestAdd_Apply_SynthesizesLocalFromJSON` to `internal/fleet/add_test.go` — temp dir has only `fleet.json` (no `fleet.local.json`); after `Add()` with `Apply=true, Confirmed=true`, assert `fleet.local.json` exists and is minimal, `fleet.json` unchanged, and the returned `AddResult.SynthesizedLocal == true`

### Implementation for User Story 1

- [X] T012 [US1] Implement `Add(cfg *Config, opts AddOptions) (*AddResult, error)` skeleton in `internal/fleet/add.go`: (1) validate `opts.Repo` (already normalized by caller), (2) reject if `cfg.Repos[opts.Repo]` already exists (basic error — US2 upgrades messaging), (3) reject if any `opts.Profiles` entry is not in `cfg.Profiles` (basic error — US2 upgrades messaging), (4) build candidate `RepoSpec{Profiles: opts.Profiles}` (engine/exclude/extra stay zero — US3 wires those), (5) transiently set `cfg.Repos[opts.Repo] = candidate`, (6) call `cfg.ResolveRepoWorkflows(opts.Repo)`, (7) populate and return `AddResult`
- [X] T013 [US1] Extend `Add()` with the `--apply` branch: detect whether `fleet.local.json` exists (via `os.Stat` on `filepath.Join(opts.Dir, LocalConfigFile)`), call `BuildMinimalLocalConfig(opts.Repo, candidate)`, call `SaveLocalConfig(opts.Dir, minimal)`, set `WroteLocal=true` and `SynthesizedLocal` accordingly, populate `LocalPath`
- [X] T014 [P] [US1] Create `cmd/add.go` with `newAddCmd(flagDir *string)` function: cobra command with `Use: "add <owner/repo>"`, `Args: cobra.ExactArgs(1)`, flags `--profile` (`StringSliceVar`, required, repeatable or comma-separated per research.md §1), `--apply` (bool), `--yes` (bool); `RunE` calls `validateSlug`, detects TTY via `term.IsTerminal(int(os.Stdin.Fd()))` from `golang.org/x/term` (add to `go.mod` if absent), prompts with `Write fleet.local.json? [y/N]` in TTY mode when `--apply` without `--yes`, errors otherwise, calls `fleet.LoadConfig` then `fleet.Add`, calls a new `printAdd` helper to render output
- [X] T015 [US1] Implement `printAdd(cmd *cobra.Command, res *fleet.AddResult, apply bool)` in `cmd/add.go` rendering per contracts/cli.md "Stdout" and "Stderr" sections: header line on stderr (tense varies by apply flag), workflow list on stdout (one `- <name>` per line, resolution order), next-step hint on stderr
- [X] T016 [US1] Remove `newAddCmd` stub from `cmd/stubs.go` (keep `newStatusCmd`). Root command in `cmd/root.go` already wires `newAddCmd` — verify no changes needed there by reading `cmd/root.go`
- [X] T017 [US1] Run `go build ./...` and `go vet ./...`; fix any errors. Run `go test ./internal/fleet/ -run TestAdd -v`; confirm T009, T010, T011 all pass.

**Checkpoint**: MVP complete. `gh-aw-fleet add <owner/repo> --profile default [--apply --yes]` is fully functional for the default-profile happy path. Error messages are minimal but errors exit non-zero. Duplicate-repo, unknown-profile, slug-validation paths work but lack actionable detail.

---

## Phase 4: User Story 2 — Validate before writing (Priority: P1)

**Goal**: Every error path produces an actionable, operator-readable
message: duplicate-repo names the source file, unknown-profile lists
available profiles, malformed-slug shows a valid example. This story
upgrades US1's error messaging; the happy path is unchanged.

**Independent Test**: Invoke the command with each failure condition
(duplicate, unknown profile, malformed slug); verify each exits
non-zero AND the stderr message names the offending input along with
a remediation hint (source file, profile list, example slug).

### Tests for User Story 2

- [X] T018 [US2] Add `TestAdd_DuplicateRepo_NamesSourceFile` to `internal/fleet/add_test.go` — three subtests: repo exists in `fleet.json` only, `fleet.local.json` only, both; assert error string contains the correct file name(s) in each case
- [X] T019 [US2] Add `TestAdd_UnknownProfile_ListsAvailable` to `internal/fleet/add_test.go` — Config has profiles `[default, experimental]`; call `Add()` with `Profiles: ["nonexistent"]`; assert error contains both available profile names alphabetically
- [X] T020 [US2] Extend `TestValidateSlug` (from T006) with malformed-slug subtests asserting error messages contain an example of valid form (e.g., `"owner/repo"`)

### Implementation for User Story 2

- [X] T021 [US2] Upgrade duplicate-repo error in `Add()` (from T012): determine source by checking `base.Repos` vs `local.Repos` — requires exposing which file contains the entry. Use the `Config.LoadedFrom` field (already populated by `LoadConfig`) to derive the phrasing. Error format: `repo %q already exists in %s` where `%s` is the relevant file(s).
- [X] T022 [US2] Upgrade unknown-profile error in `Add()` (from T012): on mismatch, collect sorted keys of `cfg.Profiles` and include in error. Format: `profile %q not defined; available profiles: [%s]`
- [X] T023 [US2] Upgrade `validateSlug` error in `internal/fleet/add.go` (from T003) to include a valid example in every failure message. Format: `invalid repo slug %q: %s; expected form: owner/repo`
- [X] T024 [US2] Run `go build ./...`, `go vet ./...`, and `go test ./internal/fleet/ -run TestAdd -v`; confirm T018, T019, T020 pass along with US1 tests.

**Checkpoint**: US1 + US2 complete. All error paths produce actionable
messages. Happy path unchanged. The MVP is now also "safe to use
without consulting docs."

---

## Phase 5: User Story 3 — Per-repo customization at onboarding time (Priority: P2)

**Goal**: Add `--engine`, `--exclude`, `--extra-workflow` flags and
their validation / warning behavior. `--extra-workflow` uses the
`gh aw`-style spec syntax per FR-008.

**Independent Test**: Run `add rshade/foo --profile default --engine claude --exclude ci-doctor --extra-workflow githubnext/agentics/security-guardian@v0.4.1 --apply --yes`; inspect `fleet.local.json` and confirm all four fields (`profiles`, `engine`, `exclude`, `extra`) are populated in the single new repo entry. Test each flag in isolation too.

### Tests for User Story 3

- [X] T025 [US3] Add `TestParseExtraWorkflowSpec` to `internal/fleet/add_test.go` — table-driven; subtests for: bare `name` → local; `owner/repo/name@ref` (3-part agentics) → remote with source/ref; `owner/repo/.github/workflows/name.md@ref` (4-part gh-aw) → remote with source/ref/path; error cases (no `@ref`, `owner/repo` alone, malformed path prefix)
- [X] T026 [US3] Add `TestAdd_UnknownEngine` to `internal/fleet/add_test.go` — call `Add()` with `Engine: "fictional-engine"`; assert error names rejected value and lists accepted engines (from `EngineSecrets` keys, sorted)
- [X] T027 [US3] Add `TestAdd_CustomizationFlags_WriteCorrectSpec` to `internal/fleet/add_test.go` — `Apply=true, Confirmed=true` with all three flags populated; assert written `fleet.local.json` contains a `RepoSpec` with correct `Engine`, `ExcludeFromProfiles`, and `ExtraWorkflows` (parsed) entries
- [X] T028 [US3] Add `TestAdd_Warnings` to `internal/fleet/add_test.go` — three subtests per research.md §3: (a) `--exclude` name not in any selected profile → warning includes name, exit 0; (b) `--extra-workflow` with a name that's also in a selected profile → warning about shadowing, exit 0; (c) profile resolves to zero workflows (e.g., all excluded) → warning about zero-resolved, exit 0

### Implementation for User Story 3

- [X] T029 [US3] Implement `parseExtraWorkflowSpec(s string) (ExtraWorkflow, error)` in `internal/fleet/add.go` per research.md §7: no slash → local; with `@`, split lhs on `/` → 3-part (agentics) or 4+ part starting with `.github/workflows/` (gh-aw 4-part); every other shape errors with an example
- [X] T030 [US3] Extend `Add()` in `internal/fleet/add.go` to call `parseExtraWorkflowSpec` for each `opts.ExtraWorkflows` entry, propagate parse errors early (before any mutation), and populate `candidate.ExtraWorkflows`
- [X] T031 [US3] Extend `Add()` with engine validation: if `opts.Engine != ""`, check `_, ok := EngineSecrets[opts.Engine]`; on miss, error with sorted list of accepted keys. Then set `candidate.Engine = opts.Engine`.
- [X] T032 [US3] Extend `Add()` to populate `candidate.ExcludeFromProfiles = opts.Excludes`
- [X] T033 [US3] Implement warning collection in `Add()` per research.md §3: after resolution, (a) for each `opts.Excludes` entry that didn't match any workflow in the selected profiles, append warning; (b) for each parsed extra whose Name also appears in a selected profile's workflow list, append warning; (c) if `len(result.Resolved) == 0`, append a loud warning. All warnings go into `AddResult.Warnings`.
- [X] T034 [US3] Add `--engine` (`StringVar`), `--exclude` (`StringArrayVar`, repeatable), `--extra-workflow` (`StringArrayVar`, repeatable) flags to `cmd/add.go` (from T014). Wire them into `AddOptions.Engine`, `.Excludes`, `.ExtraWorkflows` respectively.
- [X] T035 [US3] Update `printAdd` in `cmd/add.go` (from T015) to render the engine-override line (`engine override: <name>` on stderr, only when set) between the header and any warnings, and render warnings as `warning: <message>` on stderr after the header/engine line and before the next-step hint
- [X] T036 [US3] Run `go build ./...`, `go vet ./...`, and `go test ./internal/fleet/ -run TestAdd -v` and `-run TestParseExtraWorkflowSpec -v`; confirm T025, T026, T027, T028 pass along with all US1 + US2 tests.

**Checkpoint**: All three user stories complete. Full flag surface
matches contracts/cli.md. All unit tests pass.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Update operator-facing documentation and the skill that
advertises the now-obsolete JSON-editing workflow. Perform the manual
integration test from quickstart.md.

- [X] T037 [P] Update `skills/fleet-onboard-repo/SKILL.md` (FR-019): replace the "edit `fleet.local.json` by hand" step with a `gh-aw-fleet add <owner/repo> --profile <name>` invocation (dry-run then `--apply --yes`). Preserve the three-turn pattern structure. Verify the final skill instructions fit in ≤5 lines and contain no JSON editing guidance (SC-004).
- [X] T038 [P] Update `README.md` (FR-020): add `add` to the CLI surface list (wherever `deploy`, `sync`, `upgrade` are already enumerated); add a Quickstart section showing `gh-aw-fleet add <owner/repo> --profile default --apply --yes` before `gh-aw-fleet deploy <owner/repo>`
- [X] T039 Run full build + vet + test sweep: `go build ./... && go vet ./... && go test ./...`; all must exit 0
- [X] T040 Run `markdownlint specs/001-add-subcommand/*.md skills/fleet-onboard-repo/SKILL.md README.md`; fix any issues surfaced
- [X] T041 Execute the manual integration test documented in `specs/001-add-subcommand/quickstart.md` §"Manual integration test (for reviewers of PR #9)" — fresh working dir, run `add` dry-run then `--apply --yes`, verify file contents exactly (`{"version": 1, "repos": {...}}`), confirm `fleet.json` byte-identical, confirm `list` surfaces the new entry, confirm re-run produces duplicate error. Record pass/fail in the PR description.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies. T001 runs first.
- **Phase 2 (Foundational)**: Depends on T001 passing. BLOCKS all user stories.
- **Phase 3 (US1)**: Depends on Phase 2 complete. Independently shippable as MVP.
- **Phase 4 (US2)**: Depends on Phase 3 complete (it upgrades US1's error messaging in-place).
- **Phase 5 (US3)**: Depends on Phase 2 complete. Can technically run in parallel with Phase 3 if staffed, but shares `cmd/add.go` and `internal/fleet/add.go` with US1 — prefer sequential.
- **Phase 6 (Polish)**: Depends on desired user stories complete. Run before opening the PR.

### User Story Dependencies

- **US1 (P1)**: Can start after Phase 2. Self-contained.
- **US2 (P1)**: Depends on US1 — T021/T022/T023 modify code introduced in US1.
- **US3 (P2)**: Can start after Phase 2 (the helpers); T025/T029 are independent of US1. T030–T035 modify code introduced in US1; in practice, run US3 after US2.

### Within Each User Story

- Tests written BEFORE the implementation that makes them pass (TDD per spec.md's "Testing Strategy").
- Within `internal/fleet/add.go`: types → helpers → `Add()` body. Within `cmd/add.go`: command scaffold → flag wiring → print rendering.
- Story's final task is a verification run (`go build`, `go vet`, `go test ./internal/fleet/ -run TestAdd -v`).

### Parallel Opportunities

- **Phase 2**: T005 (`internal/fleet/load.go`) runs in parallel with T002–T004 (`internal/fleet/add.go`) — different files.
- **Phase 3**: T014 (`cmd/add.go` scaffold, new file) can run in parallel with T009–T011 (`internal/fleet/add_test.go`, different file) once the `Add` signature exists (after T012).
- **Phase 6**: T037 and T038 modify different files (SKILL.md and README.md) — can run in parallel.
- **Within a single `_test.go` file**: test additions are SEQUENTIAL (same-file edits conflict). Do not parallelize test-case additions within `internal/fleet/add_test.go`.

---

## Parallel Example: Phase 2 Foundational

```bash
# After T001 passes, these three can run in parallel:
Task: "T002 Create internal/fleet/add.go with AddOptions and AddResult types"
Task: "T005 Rename SaveConfig → SaveLocalConfig in internal/fleet/load.go"
# (T003/T004 must wait for T002, since they add functions to the same new file.)
```

## Parallel Example: Phase 3 User Story 1

```bash
# After T012 + T013 complete:
Task: "T014 Create cmd/add.go with cobra command (different file)"
Task: "T009/T010/T011 Add tests to internal/fleet/add_test.go (different file)"
# Note: T009, T010, T011 themselves are SEQUENTIAL because they share add_test.go.
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (T001)
2. Complete Phase 2 (T002–T008)
3. Complete Phase 3 / US1 (T009–T017)
4. **STOP and VALIDATE**: run quickstart.md §"Step 1" through §"Step 4" manually. Confirm: dry-run preview readable, `--apply --yes` writes minimal file, `fleet.json` untouched, `gh-aw-fleet list` surfaces new entry.
5. If the MVP holds, open a draft PR with the US1 scope for early review.

### Incremental Delivery

1. Complete Phase 1 + 2 → foundation ready
2. Add US1 → MVP → demo
3. Add US2 → error messaging is now production-quality
4. Add US3 → full flag surface
5. Phase 6 → documentation + skill update + manual integration test → merge-ready PR

### Parallel Team Strategy (if staffed)

1. Developer A: Phase 1 + 2 (T001–T008)
2. After Phase 2 completes:
   - Developer A: US1 (T009–T017)
   - Developer B: US3 helpers ONLY (T025 + T029) — they don't touch US1 code
3. After US1 completes:
   - Developer A: US2 (T018–T024)
   - Developer B: rest of US3 (T026–T036)
4. Either developer: Phase 6 (T037–T041)

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- Setup (Phase 1) and Foundational (Phase 2) tasks have NO [Story] label
- Polish (Phase 6) tasks have NO [Story] label
- Commit after each user-story phase is recommended but not required (spec.md Out of Scope: "Pushing the change to a remote git repo (this is local config only)" applies to the `add` command — this note is about the implementing PR's git hygiene)
- All commit messages for this work follow the project convention: `ci(workflows)` scope, Conventional Commits format, subject ≤72 chars (per CLAUDE.md and plan.md Constitution Check §III)
- Verify every test FAILS before implementing the behavior it asserts (TDD per spec.md §"Testing Strategy")
- Stop at any checkpoint to validate the current story independently against quickstart.md
- Avoid: modifying files outside the "Files Likely Affected" list in spec.md without a written rationale; same-file parallelization; cross-story coupling that breaks US1's independent shippability
