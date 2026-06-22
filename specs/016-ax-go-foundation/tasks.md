---

description: "Task list for: Adopt ax-go as the AX Foundation — Phase 1"
---

# Tasks: Adopt ax-go as the AX Foundation — Phase 1

**Input**: Design documents from `/specs/016-ax-go-foundation/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/schema-command.md, quickstart.md

**Tests**: INCLUDED — the spec mandates them. The `__schema` output test is the
verification surface for SC-004 (US2); the **existing** `internal/fleet` load/save
suite is the parity guard for the config swap (US1) and is run **unchanged**
(FR-010, SC-002). No new test rewrites an existing assertion.

**Organization**: Grouped by user story (US1 P1 → US2 P2 → US3 P3). US1 (config
swap) and US2 (`__schema`) are independent once the dependency is available
(different files: `internal/fleet/load.go` vs `cmd/`), so they can proceed in
parallel. US3 is the cross-cutting "zero observable change" safety gate that
depends on both. Strategy: **make ax-go available (Setup) → swap config IO and
prove parity (US1, MVP) → add `__schema` (US2) → verify nothing else moved (US3)**.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: US1 / US2 / US3 (Setup, Polish carry no story label)
- Exact file paths included in every task

## Path Conventions

Single Go module rooted at repository root. Touched code under
`internal/fleet/` and `cmd/`; governance under `.specify/memory/`; docs at root.
Paths below are repo-relative.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Authorize and make the ax-go dependency available — the shared
prerequisite for both US1 and US2.

- [X] T001 [P] Amend `.specify/memory/constitution.md`: add `github.com/rshade/ax-go` to the **Approved direct dependencies** list in §Third-Party Dependencies with the three-alternatives rationale (stdlib rejected — AX contracts are bespoke; vendoring rejected — forks the shared DNA; CLI-delegation N/A — the tool's own output layer) **plus** the import-isolation note (gh-aw-fleet consumes only the `config` / `schema` / `contract` packages, so ax-go's OTel/gRPC transitive deps stay out of the build); bump the footer **v1.1.0 → v1.2.0**, update `Last Amended` to 2026-06-21, and add a Sync Impact Report comment block at the top describing the amendment (mirror the #73 hujson entry) (FR-001, FR-002, SC-005).
- [X] T002 Make the dependency available in `go.mod`: run `go mod edit -go=1.26.4` (raise the directive from `1.25.8`, FR-004) and `go get github.com/rshade/ax-go@v0.2.0`. **Do NOT run `go mod tidy` yet** — no code imports ax-go until US1/US2, so tidy would prune the require; the tidy + isolation verification is T005 (after the first import lands). Confirm `go build ./...` still succeeds (FR-003, FR-004).

---

## Phase 2: Foundational (Blocking Prerequisites)

**No foundational tasks.** Beyond Setup (the available dependency), US1 and US2
share no blocking infrastructure. US3 depends on both stories being complete.

**Checkpoint**: ax-go is available and the constitution authorizes it — US1 and US2 can begin (in parallel if staffed).

---

## Phase 3: User Story 1 - Land ax-go the constitutional way, behavior-equivalent (Priority: P1) 🎯 MVP

**Goal**: `internal/fleet/load.go` reads and writes config through ax-go's
import-isolated `config` package (`config.Parse` / `config.Patch`) instead of
direct `tailscale/hujson` calls, with no observable change — comments preserved,
probe order intact, malformed config still fails — and the heavy ax-go transitive
stack kept out of the build.

**Independent Test**: The existing `internal/fleet` load/save suite passes with
**zero** assertion edits, and `go list -deps ./...` reaches no OpenTelemetry /
gRPC / protobuf package.

### Implementation for User Story 1

- [X] T003 [US1] In `internal/fleet/load.go`, swap the **read** path to `config.ParseFile` (from `github.com/rshade/ax-go/config`): rewrite `loadConfigFile` to call `config.ParseFile(context.Background(), path, &c)` (replacing `os.ReadFile` + `hujson.Standardize` + `json.Unmarshal`) while **keeping** the existing `c.Version != SchemaVersion` check; rewrite the read in `LoadTemplates` the same way (`config.ParseFile(context.Background(), path, &t)`), keeping the empty-catalog first-run branch. Use `context.Background()` to avoid changing the `LoadConfig`/`LoadTemplates` signatures (research.md Decision 2; FR-005). Depends on T002.
- [X] T004 [US1] In `internal/fleet/load.go`, swap the comment-preserving **write** path: rewrite `SaveTemplates`'s patch branch to `patched, err := config.Patch(context.Background(), bytes.NewReader(existing), opsBytes)` (where `existing` is `os.ReadFile(path)` and `opsBytes` is the existing `buildTemplatesPatch(t)` output) then write via the existing `atomicWrite(path, patched)` — preserving the 0600-perm + trailing-newline policy (FR-007); **keep** the `!exists → writeJSON` first-write branch and the `event=hujson_fallback_to_rewrite` warn + `writeJSON` fallback on patch error (FR-009); **relocate** the `writeHujson` and `readHujsonOrScaffold` helpers from `load.go` to `internal/fleet/add.go` — they are **not** dead; the `Add` path (`appendRepoMember`) is their live consumer and still mutates the hujson AST directly (spec edge case, FR-011) — and drop the `tailscale/hujson` import from `load.go` only (`add.go` **and** `security/renovate.go` still import it, so hujson stays a direct module dep regardless — FR-011). Same file as T003 → sequence after T003. (FR-006).
- [X] T005 [US1] Settle and verify the module graph now that `config` is imported: run `go mod tidy`, then confirm (a) `go.mod`'s direct-require block gained exactly one entry, `github.com/rshade/ax-go v0.2.0`, and `tailscale/hujson` is still direct (FR-003, FR-011, SC-003); (b) **import isolation** holds — `go list -deps ./... | grep -E 'go.opentelemetry.io|google.golang.org/grpc|google.golang.org/protobuf'` returns nothing, and none of those modules appear in `go.mod` (FR-003a, SC-003a). Depends on T003, T004.
- [X] T006 [US1] Run the existing parity suite **unchanged**: `go test ./internal/fleet/ -run 'TestLoadConfig|TestLoadTemplates|TestProbeConfigPath|TestSaveTemplates|TestBillingMetadata|TestAdd_Apply' -v`; confirm all green with no edits to any assertion — including `TestSaveTemplates_PreservesEvaluationsComments` (comments survive `config.Patch`) and `TestLoadConfig_BothExtensionsError` (the `"ambiguous"` probe error, unchanged because `probeConfigPath` is untouched — FR-008). If a comment-substring assertion breaks on whitespace drift, confirm the comment is still present and validate against `config.Patch`'s actual output without weakening the assertion (research.md Decision 2; FR-010, SC-002, SC-007). Depends on T003, T004.

**Checkpoint**: Config IO stands on ax-go; existing tests green; dependency isolated (no OTel/gRPC in the build). This is the MVP — the foundation is laid and proven behavior-equivalent.

---

## Phase 4: User Story 2 - Agents and the control plane introspect the CLI surface (Priority: P2)

**Goal**: A reserved, additive `__schema` command emits a machine-readable JSON
description of the full command tree (and an MCP-adapter form) on stdout, with no
change to any existing command.

**Independent Test**: `gh-aw-fleet __schema` emits valid JSON enumerating all eight
subcommands plus the tool version; `__schema --as mcp` returns the MCP tools list.

### Implementation for User Story 2

- [X] T007 [P] [US2] Create `cmd/schema.go`: add `newSchemaCmd(root *cobra.Command) *cobra.Command` that builds a hidden command **mirroring** `schema.NewSchemaCommand` (from `github.com/rshade/ax-go/schema`) on `schema.BuildSchema(root, schema.WithSchemaVersion(toolVersion()))` / `schema.BuildMCPSchema`, augmenting the `mcp` output with CLI positional arguments that flag-only reflection cannot derive, with `c.Hidden = true`; add the `toolVersion()` helper reading `runtime/debug.ReadBuildInfo().Main.Version`, returning `"dev"` when build info is absent or `"(devel)"`. NOTE: despite its name, `schema.WithSchemaVersion(v)` sets the output's `version` (tool) field, **not** `schema_version` (which is a fixed ax-owned const) — so feeding `toolVersion()` is correct (research.md Decision 4) (research.md Decisions 4–5; FR-012, FR-014, FR-015). Different file from US1 → may run in parallel with T003–T006 once T002 is done. Depends on T002.
- [X] T008 [US2] In `cmd/root.go`, after the existing eight `root.AddCommand(...)` subcommand registrations in `NewRootCmd`, add `root.AddCommand(newSchemaCmd(root))` so `__schema` reflects the full tree at invocation (lazy reflection — no init-order issue) (FR-012, FR-013). Depends on T007.
- [X] T009 [US2] Create `cmd/schema_test.go`: build the root via `NewRootCmd()`, execute `__schema` capturing stdout, and assert it is valid JSON whose `tool == "gh-aw-fleet"`, whose `version` is non-empty, and whose `command.commands[]` includes all eight subcommands (`list`, `status`, `add`, `template`, `deploy`, `sync`, `upgrade`, `consumption`) with the root persistent flags (`--dir`, `--log-level`, `--log-format`, `--output`); assert `__schema --as mcp` returns a `tools` list; assert `__schema --as bogus` exits non-zero (the one contained ax error). Mirror the quickstart §3 checks (FR-013, FR-014, FR-016, FR-017, SC-004). Depends on T008.

**Checkpoint**: `__schema` is invokable and tested (both `ax` and `mcp` forms), hidden from human `--help`, additive — no existing command touched.

---

## Phase 5: User Story 3 - The operator sees zero change (Priority: P3)

**Goal**: Prove the whole phase is regression-free — full gate green, no observable
change to any command, and the frozen version constants untouched.

**Independent Test**: `make ci` is green with no modified behavior-test
expectations and no new lint suppressions; `go run . list` / `status` output and
exit codes match `main`.

### Implementation for User Story 3

- [X] T010 [US3] Run the full gate and the no-observable-change checks: `make ci` (fmt-check, vet, lint, test) green with no new lint suppressions (SC-001); `go run . list` and `go run . status` produce output and exit codes identical to `main` (diff against a clean checkout if in doubt). SC-004's "**every other** command is byte-identical" is satisfied by construction — the only shared code path that changed is `internal/fleet/load.go` (exercised by `list`/`status` via `LoadConfig`) and `__schema` is purely additive — so the `list`/`status` spot-check plus the additivity argument covers the mutating/network commands (`deploy`/`sync`/`upgrade`/`consumption`/`template`) that can't be diffed read-only; `cmd.SchemaVersion` (`cmd/output.go`) and `fleet.SchemaVersion` (`internal/fleet/schema.go`) both still equal `1` and the `--output json` envelope is unchanged (FR-016, FR-018, SC-004, SC-006); re-confirm `go mod tidy` leaves no diff (SC-003). Depends on T006, T009.

**Checkpoint**: All three stories complete; the adoption is invisible to operators and the wire/on-disk contracts are frozen as promised.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T011 [P] Record the adoption in the agent docs: update `CLAUDE.md` and `AGENTS.md` (Active Technologies / dependency notes) to list `github.com/rshade/ax-go v0.2.0` (consumed via the import-isolated `config` / `schema` / `contract` packages — never root `package ax`), and reference this phase-1 plan plus the enumerated follow-up phases (error-envelope, `--output json` payload, logger, idempotency) (FR-020; Constitution §Development Workflow — new architectural invariant). Three specific edits are required, not just a single note: (a) **sweep every stale `go 1.25.8` compatibility claim** — both files repeat "module declares `go 1.25.8` compatibility" across their Active-Technologies blocks (~6 occurrences); each MUST be updated to `1.26.4` to reflect the FR-004 directive bump and its `pkg/fleet`-consumer impact (#152); (b) **document the FR-015 `error_envelope` forward-declaration caveat** — `__schema` advertises ax-go's standard error envelope, but phase 1 does **not** yet emit it, so the docs MUST warn that a consuming agent should not parse today's errors as ax envelopes (the two reconcile in the deferred error-envelope phase); (c) keep `tailscale/hujson` recorded as a direct dep retained by `add.go` **and** `security/renovate.go` (not removed in this phase — FR-011). The SPECKIT plan pointer was already updated during `/speckit-plan`.
- [X] T012 Run the `quickstart.md` acceptance checklist end-to-end (§1–§6) and confirm every check passes (SC-001…SC-007). Depends on T010, T011.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001–T002)**: T001 (constitution) is an independent doc edit; T002 (dependency) has no code prerequisite. Both must land in the same change (the dep requires the amendment).
- **Foundational**: none.
- **US1 (T003–T006)**: depends on T002. T003 → T004 (same file) → T005 + T006.
- **US2 (T007–T009)**: depends on T002. T007 → T008 → T009. Independent of US1 (different files).
- **US3 (T010)**: depends on US1 (T006) **and** US2 (T009).
- **Polish (T011–T012)**: T011 anytime after the work exists; T012 after T010 + T011.

### Story Dependencies

- US1 (P1) → needs only the available dependency; the MVP increment.
- US2 (P2) → needs only the available dependency; independent of US1.
- US3 (P3) → the safety gate; needs US1 + US2 complete.

### Within Each Story

- **US1**: read swap (T003) before write swap (T004) — same file; then tidy/isolation (T005) ‖ parity tests (T006).
- **US2**: `cmd/schema.go` (T007) before root wiring (T008) before the test (T009).

### Parallel Opportunities

- T001 ‖ T002 (constitution doc vs `go.mod` — different files).
- **US1 ‖ US2**: once T002 lands, T007 (`cmd/schema.go`) can proceed alongside T003–T006 (`internal/fleet/load.go`) — different files, no shared state.
- T005 ‖ T006 (module-graph verify vs running the parity suite, both after T003/T004).
- T011 ‖ everything after the code exists (docs only).

---

## Parallel Example: US1 and US2 in parallel

```bash
# After T002 (ax-go available), the config swap and the __schema wiring touch
# different files and proceed concurrently:
Task: "T003 swap the read path to config.ParseFile in internal/fleet/load.go"   # US1
Task: "T007 create cmd/schema.go with newSchemaCmd + toolVersion"               # US2

# US1 continues in load.go (T004 write swap → T005/T006 verify) while
# US2 continues in cmd/ (T008 root wiring → T009 schema_test).
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. T001–T002 Setup (constitution + dependency available).
2. T003–T006 US1: swap config IO, settle the module graph, prove parity.
3. **STOP & VALIDATE**: existing load/save tests green, no OTel/gRPC in the build,
   comments preserved. The tool now stands on ax-go's config primitives — the
   foundational proof the whole adoption rests on.

### Incremental Delivery

1. Setup + US1 → config IO on ax-go, isolated and behavior-equivalent (MVP).
2. US2 → `__schema` discoverability for agents / the control plane.
3. US3 → full gate green, zero observable change confirmed. Ship.

### Notes

- [P] = different files, no dependency on an incomplete task.
- Tests are included because the spec mandates them (the `__schema` test, SC-004)
  and because the existing load/save suite is the parity guard (FR-010, SC-002) —
  run it **unchanged**; do not edit any assertion.
- **Do NOT `go mod tidy` in Setup** — run it in T005 once `config` is imported,
  or tidy will prune the unused require (research.md Decision 1 sequencing).
- Import only `ax-go/config` + `ax-go/schema` (never root `package ax`) so the
  OTel/gRPC/protobuf stack never enters the build (FR-003a, SC-003a).
- Do not bump `cmd.SchemaVersion` or `fleet.SchemaVersion`; do not touch
  `cmd/output.go`, `internal/log`, or `fleet.Diagnostic`/`CollectHints` (deferred
  follow-up phases).
- `tailscale/hujson` stays a direct dep via `add.go` **and**
  `security/renovate.go`; only `load.go` drops its hujson import (FR-011).
