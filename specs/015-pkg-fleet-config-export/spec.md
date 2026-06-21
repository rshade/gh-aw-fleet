# Feature Specification: Export Fleet Config Contract into Public `pkg/fleet`

**Feature Branch**: `015-pkg-fleet-config-export`
**Created**: 2026-06-20
**Status**: Draft
**Input**: GitHub issue #148 — "Export fleet config contract into public pkg/fleet (types + SchemaVersion + JSON) — minimal first slice of #141"

## Overview

The `agentic-fleet.ai` control plane (a separate, private Go module) must serve
the exact `fleet.json` wire contract that this engine's CLI will later pull
(`RemoteSource`, issue #142). For that to be correct, the control plane and this
engine must share **one canonical definition** of the fleet-config shape.

Today that shape lives in `internal/fleet/schema.go`. Go forbids importing an
`internal/` package across a module boundary, so the control plane cannot reuse
the type — its only alternative is to re-declare the schema by hand, producing
**two hand-maintained definitions of one contract that drift silently** and
break the remote pull. This feature removes that risk by publishing the config
*contract* (data shapes + version + JSON serialization) in a public
`pkg/fleet` package, while leaving all load / merge / validate / analysis logic
behind for later slices (issues #141 and #144).

This is the **minimal first slice of epic #146** via parent #141: export only the
contract, change no on-disk format, and keep the CLI's observable behavior
identical.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - External module consumes one canonical contract type (Priority: P1)

A maintainer of the separate control-plane module needs to serve `fleet.json`
from a Go type that is guaranteed to match what the engine produces and parses.
They add a dependency on this engine module and import the public package, then
use `Config` and `SchemaVersion` directly — with no copy-pasted struct
definitions and no second source of truth to keep in sync.

**Why this priority**: This is the entire reason the issue exists. Without an
importable canonical type, the control plane must re-declare the schema by hand,
which is precisely the silent-drift failure mode the feature is meant to
prevent. Delivering only this story already eliminates the drift risk and
unblocks #142/#143.

**Independent Test**: From a throwaway Go program *outside* this module's
`internal/` tree (e.g., a separate test module or the package's own external
example test), `import "github.com/rshade/gh-aw-fleet/pkg/fleet"`, declare a
`fleet.Config`, and read `fleet.SchemaVersion`. The program compiles and the
constant equals `1`.

**Acceptance Scenarios**:

1. **Given** an external Go module that depends on this engine module, **When**
   it imports `github.com/rshade/gh-aw-fleet/pkg/fleet` and references
   `fleet.Config`, `fleet.RepoSpec`, `fleet.Profile`, `fleet.SourcePin`,
   `fleet.ProfileWorkflow`, `fleet.ExtraWorkflow`, `fleet.Defaults`, and
   `fleet.SchemaVersion`, **Then** the module compiles with no errors.
2. **Given** the imported package, **When** the consumer reads
   `fleet.SchemaVersion`, **Then** the value is `1`.
3. **Given** a populated `fleet.Config` value, **When** the consumer calls
   `cfg.EffectiveEngine(repo)`, **Then** it returns the per-repo engine override
   if present, otherwise the fleet-level default — identical to today's
   behavior.

---

### User Story 2 - Wire bytes stay byte-identical to today's `fleet.json` (Priority: P2)

The control plane will serve, and the engine's future `RemoteSource` will parse,
the same bytes. A consumer marshals a `fleet.Config` and gets output that is
byte-for-byte identical to what the engine produces today for the same data, so
nothing downstream of the wire contract has to change.

**Why this priority**: Byte-fidelity is the correctness guarantee that protects
the remote-pull path (#142). Moving the structs is only safe if the JSON
encoding is provably unchanged. This story is the explicit verification layer on
top of Story 1.

**Independent Test**: A golden round-trip test reads the committed
`fleet.example.json`, unmarshals it into `pkg/fleet.Config`, re-marshals it, and
asserts the result matches the committed baseline byte-for-byte (modulo the
project's canonical formatting). The test lives so it exercises the public
package's tags only.

**Acceptance Scenarios**:

1. **Given** the committed `fleet.example.json`, **When** it is unmarshaled into
   `pkg/fleet.Config` and re-marshaled, **Then** the output is byte-identical to
   the canonical baseline.
2. **Given** a `Config` with an absent `Defaults` block and an empty `Profiles`
   map, **When** it is marshaled, **Then** the `omitzero`/`omitempty` behavior of
   each field is preserved exactly as it is today (no field appears or
   disappears relative to the current encoding).
3. **Given** a `Config` whose `LoadedFrom` field is set, **When** it is
   marshaled, **Then** `LoadedFrom` is excluded from the JSON (its `json:"-"`
   tag is preserved).

---

### User Story 3 - The CLI and internal callers behave identically (Priority: P3)

An operator who already uses `gh-aw-fleet` (`list`, `deploy`, `sync`, `upgrade`,
`status`, `consumption`, …) sees no change in any command's output, exit
behavior, or on-disk file format after the refactor. Internal code that
previously referenced the config types and methods continues to work with
minimal, mechanical edits.

**Why this priority**: This is the safety constraint on the refactor. Publishing
the contract is only acceptable if it is regression-free; a broken CLI or a
changed on-disk format would be a net negative regardless of the new export.

**Independent Test**: Run the full existing test suite and a representative set
of read-only CLI invocations (e.g., `go run . list`) before and after the
change; output and exit codes match, and `make ci` is green.

**Acceptance Scenarios**:

1. **Given** the existing internal codebase and tests, **When** the structs move
   to `pkg/fleet` and `internal/fleet` re-exposes them via type aliases, **Then**
   `internal/fleet` compiles and the full test suite passes.
2. **Given** the deploy path's previous `cfg.EffectiveCompileStrict(ctx, repo)`
   call, **When** the impure logic is relocated to a standalone function in
   `internal/fleet`, **Then** the single call site is updated and the
   compile-strict resolution (explicit / auto-public / auto-private /
   auto-fallback) behaves exactly as before.
3. **Given** the change set, **When** the loader reads any existing
   `fleet.json` / `fleet.local.json`, **Then** no on-disk format change occurs
   and neither `fleet.SchemaVersion` nor `cmd.SchemaVersion` is bumped.

---

### Edge Cases

- **Same package name, two import paths**: `pkg/fleet` and `internal/fleet` both
  declare `package fleet`. This is legal Go (distinct import paths); the internal
  package's aliases reference the public one. The build must remain unambiguous.
- **Methods cannot follow an aliased type**: any method that previously hung off
  the internal `Config`/`RepoSpec` must either move *with* the type into
  `pkg/fleet` (if pure) or be re-expressed as a standalone function in
  `internal/fleet` (if impure). The one pure method, `EffectiveEngine`, moves
  *with* the type; the two impure methods, `EffectiveCompileStrict` **and**
  `ResolveRepoWorkflows`, become standalone functions in `internal/fleet`. No
  method may be silently dropped in the move.
- **Network dependency must not leak into the contract package**:
  `EffectiveCompileStrict` calls `ghRepoVisibility` (network) — it and its
  helpers (`truncateReason`, the `CompileStrictSource*` constants,
  `VisibilityPublic`) must stay in `internal/fleet`, not the contract package.
- **`omitzero` vs `omitempty` fidelity**: `Defaults` uses `omitzero`, `Profiles`
  uses `omitempty`, and `Repos` has *no* omit tag (so it encodes as `null` when
  nil). All three behaviors must survive the move byte-for-byte.
- **Excluded-from-JSON field**: `Config.LoadedFrom` (`json:"-"`, set only by the
  loader) must continue to be omitted from serialized output.
- **Catalog types stay put**: `Templates`, `TemplateSource`, `TemplateWorkflow`,
  and `Evaluation` are the upstream-catalog cache, not the fleet config
  contract, and must remain in `internal/fleet`.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A public Go package MUST exist at import path
  `github.com/rshade/gh-aw-fleet/pkg/fleet` and be importable from a Go module
  outside this repository.
- **FR-002**: The public package MUST export the config-contract types `Config`,
  `Defaults`, `Profile`, `SourcePin`, `ProfileWorkflow`, `RepoSpec`, and
  `ExtraWorkflow` with the same field names, types, and JSON struct tags they
  have today.
- **FR-003**: The public package MUST export the `SchemaVersion` constant with
  the current value `1`.
- **FR-004**: The public package MUST export the pure helper `EffectiveEngine`
  (as a method on `Config`) with behavior identical to today's.
- **FR-005**: JSON marshaling and unmarshaling of the public types MUST be
  byte-identical to the current `internal/fleet` encoding for equivalent data;
  every struct tag (including `omitzero`, `omitempty`, and `json:"-"`) MUST be
  preserved verbatim.
- **FR-006**: A golden round-trip test MUST verify that `fleet.example.json`
  unmarshals into `pkg/fleet.Config` and re-marshals to the canonical baseline
  byte-for-byte.
- **FR-007**: The `internal/fleet` package MUST continue to expose the same type
  names (via type aliases to the `pkg/fleet` types) so existing internal code
  and tests compile with only mechanical changes.
- **FR-008**: Impure, deploy-path logic MUST remain in `internal/fleet`:
  `EffectiveCompileStrict` (and its dependencies `ghRepoVisibility`,
  `truncateReason`, the `CompileStrictSource*` constants, and `VisibilityPublic`)
  MUST NOT move into the contract package; where it was a method on `Config`, it
  MUST be relocated as a standalone function taking `*fleet.Config`.
- **FR-009**: All call sites of any relocated method MUST be updated to the new
  function form. Two methods are relocated to standalone functions:
  `EffectiveCompileStrict` (one call site — the deploy path) and
  `ResolveRepoWorkflows` (seven production call sites — `cmd/list.go`,
  `internal/fleet/add.go`, `deploy.go`, `list_result.go`, `manifest.go`,
  `sync.go`, and `status.go`). The `ResolveRepoWorkflows` conversion was
  discovered during planning (research.md Decision 2) and was not enumerated in
  the original draft of this requirement.
- **FR-010**: Every newly-exported identifier in `pkg/fleet` (package, types,
  exported fields, constant, method) MUST carry a godoc comment per the repo's
  self-documentation convention.
- **FR-011**: The on-disk fleet config format MUST NOT change; neither
  `fleet.SchemaVersion` nor `cmd.SchemaVersion` may be bumped.
- **FR-012**: The CLI MUST behave identically before and after the change — no
  observable change to any command's output, exit codes, or written files.
- **FR-013**: The engine module MUST NOT take on any dependency on the control
  plane or other external module (the one-way dependency rule: engine never
  depends on the control plane).
- **FR-014**: No new third-party dependency may be introduced (Constitution
  Principle I); the move is a pure relocation within the existing module.
- **FR-015**: The catalog types `Templates`, `TemplateSource`,
  `TemplateWorkflow`, and `Evaluation` MUST remain in `internal/fleet` (they are
  out of scope for this slice).
- **FR-016**: The load/merge/validate logic (`LoadConfig`, `mergeConfigs`, the
  HuJson read path, `ResolveRepoWorkflows`) MUST remain in `internal/fleet`
  (deferred to #141).

### Key Entities *(the config contract being exported)*

- **Config**: The declarative desired state for the fleet — `Version`,
  `Defaults`, `Profiles`, `Repos`, and the loader-only `LoadedFrom`. The root of
  the wire contract.
- **Defaults**: Fleet-wide defaults applied to every repo unless overridden
  (currently `Engine`).
- **Profile**: A named, atomically-advancing bundle of workflows with an
  advisory cost `Tier`, a `Sources` pin map, and a `Workflows` list.
- **SourcePin**: The pinned `Ref` (tag/branch/sha) for a source repo within a
  profile.
- **ProfileWorkflow**: A workflow entry naming its `Source` repo and optional
  `Path`.
- **RepoSpec**: Per-repo desired state — `Profiles`, advisory `CostCenter`,
  optional `Engine`, tri-state `CompileStrict`, `ExtraWorkflows`,
  `ExcludeFromProfiles`, and `Overrides`.
- **ExtraWorkflow**: A per-repo workflow not sourced from any profile.
- **SchemaVersion**: The on-disk fleet-config format version (`1`), distinct
  from the CLI output-envelope version.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A consumer outside this module's `internal/` tree can import
  `pkg/fleet`, reference all seven contract types plus `SchemaVersion`, and
  compile with zero errors (demonstrated by an external/black-box test).
- **SC-002**: The golden round-trip test passes — re-marshaled
  `fleet.example.json` is byte-identical to the committed baseline.
- **SC-003**: `make ci` (fmt-check, vet, lint, full test suite) is green with no
  new failures and no new lint suppressions.
- **SC-004**: Across the existing test suite and a representative set of
  read-only CLI commands, observable output and exit codes are unchanged
  relative to `main` (no behavior-test expectations are modified — only
  references to relocated symbols).
- **SC-005**: 100% of newly-exported identifiers in `pkg/fleet` carry godoc
  comments (revive/staticcheck report zero exported-symbol violations).
- **SC-006**: No `SchemaVersion` constant value changes and `go.mod` gains no new
  `require` entries.

## Assumptions

- **Package naming**: The public package is `package fleet` at `pkg/fleet`,
  matching the existing internal package name; the two are distinguished by
  import path. The internal package's type aliases reference the public types so
  there is exactly one underlying definition.
- **Preferred refactor approach**: Type aliases in `internal/fleet` plus
  standalone functions for relocated impure logic (the issue's recommended
  approach), rather than anonymous embedding (the issue's stated fallback). The
  alias path keeps internal and external sharing one type.
- **Constant placement for impure logic**: The `CompileStrictSource*` constants
  and `VisibilityPublic` stay in `internal/fleet` alongside the relocated
  `EffectiveCompileStrict`, since they are deploy-path concerns rather than part
  of the serialized contract. (Open to revisiting during planning if a cleaner
  split emerges, but not required by this slice.)
- **Golden baseline**: `fleet.example.json` is the byte-fidelity baseline for the
  round-trip test.
- **Toolchain**: The local development gate runs Go 1.26.4 / golangci-lint
  v2.12.2; the module's declared compatibility (`go 1.25.8`) is unaffected. The
  `omitzero` tag in use requires Go 1.24+, which both satisfy — no encoding
  behavior change from the toolchain.
- **Scope discipline**: This slice exports the *contract only*. Load/merge,
  remote `ConfigSource`/`RemoteSource` (#142), the wire-contract spec/doc (#143),
  consumption/FinOps and security analysis exports (#144), and the catalog types
  are explicitly deferred.

## Dependencies & References

- **Parent**: #141 (full `pkg/fleet` model + load/merge). **Epic**: #146.
- **Downstream consumers of this type + `SchemaVersion`**: #142 (`RemoteSource`),
  #143 (wire-contract spec/doc).
- **Consumer rationale** ("engine owns the `fleet.json` body, Goa owns the
  endpoint"): `agentic-fleet/control-plane/docs/architecture.md` (external,
  private repo).
- **Invariant**: One-way dependency — the engine must never depend on the
  control plane.
