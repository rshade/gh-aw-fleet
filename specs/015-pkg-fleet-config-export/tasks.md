---

description: "Task list for: Export Fleet Config Contract into Public pkg/fleet"
---

# Tasks: Export Fleet Config Contract into Public `pkg/fleet`

**Input**: Design documents from `/specs/015-pkg-fleet-config-export/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/pkg-fleet.md, quickstart.md

**Tests**: INCLUDED — the spec explicitly requires them (FR-006 golden round-trip;
SC-001 external black-box compile test). These are the slice's primary
verification surface, not optional extras.

**Organization**: Grouped by user story (US1 P1 → US2 P2 → US3 P3). This is a
refactor, so the stories are sequentially dependent (US2 and US3 build on the
public types delivered in US1), but each leaves `go build ./...` green and is
independently testable. Strategy: **build the new public package first (US1),
prove byte-fidelity (US2), then cut the internal package over to it (US3)** — the
internal duplicate exists only transiently inside this one PR and is deleted in
US3 (T007), at which point "one canonical definition" is fully realized.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1 / US2 / US3 (Setup, Polish carry no story label)
- Exact file paths included in every task

## Path Conventions

Single Go module rooted at repository root. New public surface under `pkg/fleet/`;
existing code under `internal/fleet/` and `cmd/`. Paths below are repo-relative.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the new public package path.

- [X] T001 Create the `pkg/fleet/` package directory (and `pkg/fleet/testdata/`) and add `pkg/fleet/config.go` containing only the `package fleet` clause and the package-level godoc comment from `specs/015-pkg-fleet-config-export/contracts/pkg-fleet.md` — establishes import path `github.com/rshade/gh-aw-fleet/pkg/fleet` (FR-001).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Cross-story prerequisites.

**No foundational tasks.** This refactor has no blocking infrastructure beyond
Setup; the shared dependency for US2 and US3 is the public type set delivered by
US1 (Phase 3). US2/US3 therefore begin once US1's types compile.

**Checkpoint**: Setup done — User Story 1 can begin.

---

## Phase 3: User Story 1 - External module consumes one canonical contract type (Priority: P1) 🎯 MVP

**Goal**: A public `pkg/fleet` package exports the seven config-contract types,
the `SchemaVersion` constant (`1`), and the pure `EffectiveEngine` method — all
importable and usable from outside this module's `internal/` tree.

**Independent Test**: From `package fleet_test` (black-box, exported-only access),
import `github.com/rshade/gh-aw-fleet/pkg/fleet`, declare all seven types, read
`fleet.SchemaVersion`, and call `EffectiveEngine`; it compiles and `SchemaVersion == 1`.

### Tests for User Story 1 ⚠️ (write first; it fails until T002–T003 land)

- [X] T002 [US1] Write the black-box compile/use test `TestExternalConsumer` (plus an `Example` that renders in godoc) in `pkg/fleet/config_test.go` (`package fleet_test`): import the public package; declare values of `fleet.Config`, `fleet.Defaults`, `fleet.Profile`, `fleet.SourcePin`, `fleet.ProfileWorkflow`, `fleet.RepoSpec`, `fleet.ExtraWorkflow`; assert `fleet.SchemaVersion == 1`; assert `(&fleet.Config{...}).EffectiveEngine(repo)` returns the per-repo override then the fleet default (SC-001; US1 acceptance 1–3). The function names match the quickstart command `go test ./pkg/fleet/ -run 'TestExternalConsumer|Example'`.

### Implementation for User Story 1

- [X] T003 [US1] In `pkg/fleet/config.go` define the seven contract types — `Config`, `Defaults`, `Profile`, `SourcePin`, `ProfileWorkflow`, `RepoSpec`, `ExtraWorkflow` — copying field names, types, and JSON struct tags **verbatim** from `internal/fleet/schema.go` (preserve `omitzero`, `omitempty`, and `json:"-"` exactly), plus `const SchemaVersion = 1`; add a godoc comment to every exported identifier and field (FR-002, FR-003, FR-005, FR-010; data-model.md fidelity tables).
- [X] T004 [US1] In `pkg/fleet/config.go` add the pure method `func (c *Config) EffectiveEngine(repo string) string` (per-repo `Engine` override else `Defaults.Engine`), copied from the internal method, with godoc (FR-004). Depends on T003 (same file).
- [X] T005 [US1] Run `go test ./pkg/fleet/` and `go vet ./pkg/fleet/`; confirm T002 now passes and the package is stdlib-only (no internal imports, no new `go.mod` require) (FR-013, FR-014, SC-001). Depends on T002–T004.

**Checkpoint**: `pkg/fleet` is importable and exercised by an external test; `internal/fleet` is untouched and still builds. MVP for the control plane (#142) is unblocked.

---

## Phase 4: User Story 2 - Wire bytes stay byte-identical to today's `fleet.json` (Priority: P2) 🔒

**Goal**: Prove that marshaling a `pkg/fleet.Config` reproduces the canonical wire
bytes for `fleet.example.json` data, so the remote-pull path is encoding-safe.

**Independent Test**: The golden round-trip test reads `fleet.example.json`,
unmarshals into `pkg/fleet.Config`, re-marshals, and matches the committed
canonical baseline byte-for-byte.

### Tests for User Story 2 ⚠️

- [X] T006 [US2] Write the golden round-trip test in `pkg/fleet/roundtrip_test.go` (`package fleet_test`) with a `-update` flag: read `../../fleet.example.json`, `json.Unmarshal` into `fleet.Config`, `json.MarshalIndent(cfg, "", "  ")` + trailing newline, and assert **byte-equality** against `pkg/fleet/testdata/config.canonical.json`; additionally assert `LoadedFrom` is absent from the output and that the `omitzero` (Defaults) / `omitempty` (Profiles, `extra`, `exclude`) / no-omit (`repos`→`null` when nil) cases each behave as documented (FR-005, FR-006, SC-002; US2 acceptance 1–3; contracts C-4/C-5/C-6). Depends on T003.

### Implementation for User Story 2

- [X] T007 [US2] Materialize the baseline: run `go test ./pkg/fleet/ -run TestGoldenRoundTrip -update` to generate `pkg/fleet/testdata/config.canonical.json` from `fleet.example.json`, then re-run **without** `-update` to confirm it passes; commit the golden file. Note the baseline is NOT `fleet.example.json` verbatim — the example's hand alignment and `omitempty` empty `extra`/`exclude` arrays do not survive re-marshal (research.md Decision 4). Depends on T006.

**Checkpoint**: Byte-fidelity is locked by a committed golden; US1 + US2 both pass with `internal/fleet` still unchanged.

---

## Phase 5: User Story 3 - The CLI and internal callers behave identically (Priority: P3)

**Goal**: Cut `internal/fleet` over to the public types via type aliases, relocate
the two impure `Config` methods to standalone functions, update every call site,
and prove zero observable change — completing the "one canonical definition" goal
by deleting the internal duplicate.

**Independent Test**: `make ci` is green and `go run . list` (plus the full suite)
produces identical output and exit codes to `main`; only relocated-symbol
references changed.

### Implementation for User Story 3

- [X] T008 [US3] In `internal/fleet/schema.go` delete the seven contract type definitions and replace them with type aliases to the public package — `type Config = pkgfleet.Config`, `Defaults`, `Profile`, `SourcePin`, `ProfileWorkflow`, `RepoSpec`, `ExtraWorkflow` — add `const SchemaVersion = pkgfleet.SchemaVersion` (re-export), add the `pkgfleet "github.com/rshade/gh-aw-fleet/pkg/fleet"` import, and **keep** `Templates`/`TemplateSource`/`TemplateWorkflow`/`Evaluation` defined here (FR-007, FR-011, FR-015; research.md Decisions 1 & 6). Depends on T003.
- [X] T009 [P] [US3] In `internal/fleet/schema.go` convert `EffectiveCompileStrict` from a method on `*Config` to a standalone function (e.g. `func EffectiveCompileStrict(ctx context.Context, c *Config, repo string) (bool, string, string)`); keep `ghRepoVisibility`, `truncateReason`, `effectiveCompileStrictReasonMax`, the `CompileStrictSource*` constants, and `VisibilityPublic` in `internal/fleet` (FR-008). Depends on T008.
- [X] T010 [P] [US3] In `internal/fleet/load.go` convert `ResolveRepoWorkflows` from a method on `*Config` to a standalone function `func ResolveRepoWorkflows(c *Config, repo string) ([]ResolvedWorkflow, error)`; `ResolvedWorkflow` stays in `internal/fleet` (research.md Decision 2; FR-016). Depends on T008.
- [X] T011 [US3] Update the single `EffectiveCompileStrict` call site at `internal/fleet/deploy.go:1060` from `cfg.EffectiveCompileStrict(ctx, repo)` to the function form (FR-009). Depends on T009.
- [X] T012 [US3] Update all seven `ResolveRepoWorkflows` call sites from `cfg.ResolveRepoWorkflows(repo)` to `ResolveRepoWorkflows(cfg, repo)` (or `fleet.ResolveRepoWorkflows(cfg, r)` from `cmd`): `internal/fleet/add.go:135`, `internal/fleet/deploy.go:218`, `internal/fleet/list_result.go:53`, `internal/fleet/manifest.go:92`, `internal/fleet/sync.go:43`, `internal/fleet/status.go:263`, and `cmd/list.go:51` (FR-009; research.md Decision 2). Sequenced after T011 to avoid a same-file edit conflict in `deploy.go`. Depends on T010.
- [X] T013 [P] [US3] Update the one internal test that calls a relocated symbol — `internal/fleet/schema_test.go` (`EffectiveCompileStrict` function form, two call sites at lines 79 and 214) — mechanical reference changes only; do **not** alter any behavioral expectation (SC-004). No test invokes `ResolveRepoWorkflows` as a method (`sync_test.go` only mentions it in a comment at line 271), so that conversion needs no test edit. Depends on T009, T010.
- [X] T014 [US3] Run `make ci` (fmt-check, vet, lint, full suite) and capture `go run . list` output/exit; confirm green with no new lint suppressions, output/exit identical to `main`, `go.mod` gains no `require` entry, and both `SchemaVersion` values still equal `1` (SC-003, SC-004, SC-006; FR-011, FR-012). Depends on T011, T012, T013.

**Checkpoint**: Internal duplicate deleted — exactly one definition of the contract; CLI and on-disk format unchanged; full gate green.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T015 [P] Add a short note to `CLAUDE.md` (Architecture section) recording the new architectural invariant: `pkg/fleet` is the module's first public surface and the single canonical home of the `fleet.json` wire contract; `internal/fleet` aliases it (Constitution §Development Workflow — new architectural invariant). The SPECKIT plan pointer was already updated during `/speckit-plan`.
- [x] T016 [P] Reconcile the spec with the implementation: update `specs/015-pkg-fleet-config-export/spec.md` FR-009 (and the "Methods cannot follow an aliased type" Edge Case) to enumerate `ResolveRepoWorkflows` alongside `EffectiveCompileStrict` as a method that must be relocated, since the original draft under-counted the conversions (research.md Decision 2). ✅ Completed during `/speckit-analyze` remediation — spec.md FR-009 and the Edge Case now enumerate both impure methods.
- [X] T017 Run the `quickstart.md` acceptance checklist end-to-end (SC-001…SC-006) and confirm every box passes. Depends on T014.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)**: no dependencies — start immediately.
- **Foundational**: none.
- **US1 (T002–T005)**: depends on Setup. Delivers the public package (MVP).
- **US2 (T006–T007)**: depends on US1's types (T003).
- **US3 (T008–T014)**: depends on US1's types (T003); T008 deletes the internal
  duplicate and aliases to the public package.
- **Polish (T015–T017)**: T015/T016 anytime after their targets exist; T017 after T014.

### Story Dependencies (refactor — sequential, unlike a typical greenfield feature)

- US1 (P1) → independent of US2/US3; build the new package.
- US2 (P2) → needs US1 types; pure additive test + golden.
- US3 (P3) → needs US1 types; rewires `internal/fleet` to consume them.

### Within US3

- T008 (aliases) before T009/T010 (method→function conversions).
- T009 before T011; T010 before T012; T011 before T012 (shared `deploy.go`).
- T009/T010 before T013 (tests reference the new function forms).
- All before T014 (verification).

### Parallel Opportunities

- T009 ‖ T010 (different files: `schema.go` vs `load.go`, both after T008).
- T013 ‖ T011/T012 (test files vs non-test files; after T009/T010).
- T015 ‖ T016 (docs vs spec, independent files).
- US1 and US2 are mostly serial (shared `config.go` / dependent on its types); the
  real parallelism lives inside US3's conversion step.

---

## Parallel Example: User Story 3 conversions

```bash
# After T008 (aliases in place), the two method→function conversions touch
# different files and can proceed in parallel:
Task: "T009 convert EffectiveCompileStrict to a func in internal/fleet/schema.go"
Task: "T010 convert ResolveRepoWorkflows to a func in internal/fleet/load.go"

# After both, test updates run alongside the production call-site updates:
Task: "T013 update internal/fleet/schema_test.go + sync_test.go references"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. T001 Setup → 2. T002–T005 US1. **STOP & VALIDATE**: external `package fleet_test`
   compiles and uses all seven types; `SchemaVersion == 1`. The control plane can
   now import `pkg/fleet` — the entire reason the issue exists is satisfied.

### Incremental Delivery

1. Setup + US1 → public package importable (MVP, unblocks #142).
2. US2 → byte-fidelity locked by golden round-trip.
3. US3 → internal cut over to aliases, duplicate deleted, full gate green, CLI
   unchanged. Ship.

### Notes

- [P] = different files, no dependency on an incomplete task.
- Tests are included because the spec mandates them (FR-006, SC-001) — not the
  usual optional case.
- Keep `go build ./...` green at each checkpoint; the internal duplicate is
  transient (US1→US3) and removed by T008.
- Do not bump `fleet.SchemaVersion` or `cmd.SchemaVersion`; do not add a `go.mod`
  require entry; do not modify any behavioral test expectation.
- Golden baseline is the canonical re-marshal, not `fleet.example.json` verbatim
  (research.md Decision 4).
