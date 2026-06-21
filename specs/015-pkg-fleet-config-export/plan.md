# Implementation Plan: Export Fleet Config Contract into Public `pkg/fleet`

**Branch**: `015-pkg-fleet-config-export` | **Date**: 2026-06-20 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/015-pkg-fleet-config-export/spec.md` (GitHub issue #148)

## Summary

Publish the `fleet.json` *wire contract* — the seven config-contract types
(`Config`, `Defaults`, `Profile`, `SourcePin`, `ProfileWorkflow`, `RepoSpec`,
`ExtraWorkflow`), the `SchemaVersion` constant (`1`), the pure `EffectiveEngine`
method, and their exact JSON struct tags — in a new public package
`github.com/rshade/gh-aw-fleet/pkg/fleet`, so the separate `agentic-fleet.ai`
control-plane module can import one canonical type instead of re-declaring the
schema by hand (the silent-drift failure mode behind #142). All load / merge /
validate / analysis logic stays behind in `internal/fleet`; `internal/fleet`
re-exposes the moved types via **type aliases** so existing CLI behavior and the
on-disk format are byte-for-byte unchanged.

**Technical approach** (decided in [research.md](./research.md)): a pure
relocation within the existing module. Move the contract types, `SchemaVersion`,
and `EffectiveEngine` into `pkg/fleet`; in `internal/fleet` add
`type X = pkgfleet.X` aliases and a `const SchemaVersion = pkgfleet.SchemaVersion`
re-export. The
type-alias approach has one consequence the spec under-counted: **methods cannot
be attached to an aliased type**, so *both* impure methods on `Config` —
`EffectiveCompileStrict` (already named in the spec) **and** `ResolveRepoWorkflows`
(not named, ~7 production call sites) — must convert from methods to standalone
functions in `internal/fleet`. Catalog types (`Templates`, …) and load/merge stay
put per FR-015/FR-016. Byte-fidelity is locked by a golden round-trip test against
a newly-committed *canonical* baseline (the hand-aligned `fleet.example.json`
cannot be the byte target — see research.md Decision 4).

## Technical Context

**Language/Version**: Go 1.26.4 local development gate; `go.mod` declares module
compatibility at `go 1.25.8` (the `omitzero` tag in use requires Go 1.24+, which
both satisfy — no encoding behavior change from the toolchain).
**Primary Dependencies**: stdlib only for the new package (`encoding/json` via
struct tags; no imports needed in the contract file itself beyond what the types
require — none). Existing module deps (`cobra`, `zerolog`, `yaml.v3`, `hujson`,
`gitleaks/v8`) are untouched. **No new third-party dependency** (FR-014; Constitution
§Third-Party Dependencies).
**Storage**: N/A — pure code relocation; no on-disk format change, no new state,
no schema-version bump (FR-011). `fleet.json` / `fleet.local.json` bytes unchanged.
**Testing**: `go test ./...` (existing suite, run via `make test`); a new
black-box test in `package fleet_test` under `pkg/fleet/` for SC-001 (external
consumer compiles & uses all seven types + `SchemaVersion`) and SC-002 (golden
round-trip vs. canonical baseline). Full gate: `make ci` (fmt-check, vet, lint, test).
**Target Platform**: Linux / macOS developer toolchain; the package is consumed by
another Go module (the control plane) at build time.
**Project Type**: Single Go module — CLI + library. The feature adds the module's
first public (`pkg/`) surface; everything else stays under `internal/`.
**Performance Goals**: N/A — no runtime path changes; compile-time-only contract.
**Constraints**: One-way dependency — the engine MUST NOT depend on the control
plane (FR-013). JSON encoding MUST stay byte-identical (FR-005). 100% godoc on
new exported identifiers (FR-010, SC-005).
**Scale/Scope**: ~230 lines split out of `internal/fleet/schema.go`; ~9 call-site
edits for the two method→function conversions; 1 new package file + 2 new test
files (`config_test.go`, `roundtrip_test.go`) + 1 new golden testdata file. No
behavior-test expectations change (SC-004).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against Constitution v1.1.0:

- **I. Thin-Orchestrator Code Quality** — ✅ PASS. No upstream-tool logic is
  re-implemented; this is a pure intra-module relocation. `go build`/`go vet` stay
  clean. Splitting the impure logic (`EffectiveCompileStrict`) out of the contract
  file and naming the relocated functions expressively (no comments needed for the
  *what*) aligns with the WHY-only comment rule. `schema.go` shrinks; no file
  crosses the 300-line guidance as a result.
- **II. Testing Standards (Build-Green + Real-World Dry-Run)** — ✅ PASS. No
  mutating-command behavior changes, so the dry-run substrate is untouched; the
  existing dry-run/integration tests are the regression guard. New unit-style tests
  (golden round-trip + external-consumer compile) are additive and justified — they
  verify a wire contract, exactly the kind of pure, stateless transformation the
  project's unit-test guidance endorses.
- **III. UX Consistency (Three-Turn Mutation Pattern)** — ✅ PASS / N/A. No command
  surface, output, exit code, commit message, or PR title changes (FR-012). The
  three-turn pattern and `CollectHints` paths are untouched.
- **IV. Performance (Parallelism, Cache, 5-Min Ceiling)** — ✅ PASS / N/A. No new
  I/O, no network calls in the contract package (the network helper `ghRepoVisibility`
  stays in `internal/fleet`, FR-008/FR-013). No caching surface affected.
- **Third-Party Dependencies** — ✅ PASS. No new `require` entry (FR-014, SC-006).
  `go.mod` is unchanged except possibly nothing at all.
- **Declarative Reconcile Invariants** — ✅ PASS. `fleet.json` remains source of
  truth; no signing/git-path changes; `fleet.local.json` handling unchanged.

**Result**: No violations. Complexity Tracking table is empty (nothing to justify).

## Project Structure

### Documentation (this feature)

```text
specs/015-pkg-fleet-config-export/
├── plan.md              # This file (/speckit-plan output)
├── research.md          # Phase 0 — decisions (alias vs embed, method conversions, golden baseline)
├── data-model.md        # Phase 1 — the seven contract entities + field/tag fidelity table
├── quickstart.md        # Phase 1 — how to verify SC-001..SC-006 locally
├── contracts/
│   └── pkg-fleet.md      # Phase 1 — the public Go API surface (exported identifiers + godoc contract)
└── tasks.md             # Phase 2 — created by /speckit-tasks, NOT here
```

### Source Code (repository root)

```text
pkg/                                  # NEW — module's first public surface
└── fleet/
    ├── config.go                     # NEW — Config, Defaults, Profile, SourcePin,
    │                                 #       ProfileWorkflow, RepoSpec, ExtraWorkflow,
    │                                 #       SchemaVersion, (c *Config) EffectiveEngine,
    │                                 #       package doc. stdlib-only, no internal imports.
    ├── config_test.go                # NEW — package fleet_test (black-box): SC-001 compile
    │                                 #       + SchemaVersion==1 + EffectiveEngine behavior
    ├── roundtrip_test.go             # NEW — package fleet_test: SC-002 golden round-trip vs canonical baseline
    └── testdata/
        └── config.canonical.json     # NEW — committed canonical baseline (json.MarshalIndent form of the example)

internal/fleet/
├── schema.go                         # CHANGED — contract types removed; now holds:
│                                     #   type aliases (Config, Defaults, Profile, SourcePin,
│                                     #     ProfileWorkflow, RepoSpec, ExtraWorkflow = pkgfleet.X),
│                                     #   const SchemaVersion = pkgfleet.SchemaVersion (re-export),
│                                     #   CompileStrictSource* + VisibilityPublic + effectiveCompileStrictReasonMax,
│                                     #   truncateReason, EffectiveCompileStrict (now a standalone func),
│                                     #   Templates/TemplateSource/TemplateWorkflow/Evaluation (catalog — stay, FR-015)
├── load.go                           # CHANGED — ResolveRepoWorkflows: method → standalone func
├── deploy.go                         # CHANGED — 1 call site: cfg.EffectiveCompileStrict(ctx,repo) → EffectiveCompileStrict(ctx,cfg,repo); ghRepoVisibility stays here
├── add.go, sync.go, manifest.go,     # CHANGED — ResolveRepoWorkflows call sites (cfg.R(repo) → R(cfg,repo))
│   status.go, list_result.go
└── *_test.go                         # CHANGED — mechanical: schema_test.go references the relocated EffectiveCompileStrict func (sync_test.go mentions ResolveRepoWorkflows only in a comment — no change)

cmd/
├── list.go                           # CHANGED — fleet.ResolveRepoWorkflows(cfg,r) + fleet.EffectiveEngine unchanged (method stays)
```

**Structure Decision**: Single Go module. The feature introduces the conventional
`pkg/<name>` directory for the module's first publicly-importable package, keeping
the rest of the codebase under `internal/`. The public package is `package fleet`
at import path `…/pkg/fleet`; the internal package keeps `package fleet` at
`…/internal/fleet` and aliases the moved types — legal because the two are
distinguished by import path (spec Edge Case "Same package name, two import paths").

## Complexity Tracking

> No Constitution violations — table intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *(none)*  | —          | —                                    |
