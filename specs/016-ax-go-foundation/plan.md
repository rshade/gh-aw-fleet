# Implementation Plan: Adopt ax-go as the AX Foundation — Phase 1

**Branch**: `016-ax-go-foundation` | **Date**: 2026-06-21 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/016-ax-go-foundation/spec.md` (GitHub issue #156)

## Summary

Land `github.com/rshade/ax-go` as gh-aw-fleet's shared AX foundation the
constitutional way (amendment + MINOR bump, the #73 hujson precedent), then prove
it on two low-risk fronts: swap the config-IO primitives in
`internal/fleet/load.go` from direct `tailscale/hujson` calls to ax-go's
`config.Parse` / `config.Patch`, and wire a net-new, additive `__schema`
discoverability command (mirroring `schema.NewSchemaCommand` on
`schema.BuildSchema`/`schema.BuildMCPSchema`, with MCP positional-argument
augmentation). The frozen, golden-pinned
`cmd.SchemaVersion` wire contract and the on-disk `fleet.SchemaVersion` format are
not touched.

**Technical approach** (decided in [research.md](./research.md)): pin
**`ax-go@v0.2.0`** — not the issue's original `v0.1.0` — because `v0.2.0` (#78/#79)
adds *import-isolated public contract packages for thin consumers*. gh-aw-fleet
imports **only** `…/ax-go/config` and `…/ax-go/schema` (transitively the
stdlib-only `…/ax-go/contract`), whose import graphs never reach ax-go's
OpenTelemetry / gRPC / protobuf dependencies — so none of that heavy stack
compiles into the gh-aw-fleet binary or lands in its `go.mod` (Decision 1). The
config swap is behavior-equivalent and is locked by the *existing* load/save
tests run unchanged (Decision 2); the only nuance is reusing the project's
`atomicWrite` around `config.Patch` to keep the 0600-perm + trailing-newline
write policy (Decision 3). `__schema` is wired into the root tree, marked hidden
from human `--help`, and reports the tool version via `runtime/debug.ReadBuildInfo`
with a `dev` fallback (Decisions 4–5). Adopting ax-go forces the module's `go`
directive from `1.25.8` to `1.26.4` (Decision 6), an accepted consequence for
`pkg/fleet` consumers.

## Technical Context

**Language/Version**: Go 1.26.4 local development gate. `go.mod`'s `go` directive
is **raised** `1.25.8 → 1.26.4` (FR-004) to satisfy `ax-go@v0.2.0`'s `go 1.26.4`
requirement; the gate already runs 1.26.4, so the tool already builds under the
new floor.
**Primary Dependencies**: NEW direct dependency `github.com/rshade/ax-go v0.2.0`,
consumed **only** through its import-isolated `config` and `schema` packages
(transitively the stdlib-only `contract`). Existing deps (`cobra`, `zerolog`,
`yaml.v3`, `hujson`, `gitleaks/v8`) untouched; `tailscale/hujson` **stays direct**
(both the `Add` AST-append path in `add.go` and the Renovate scanner in
`security/renovate.go` still use it, FR-011). **No OpenTelemetry / gRPC /
protobuf may enter the build** (FR-003a) — the whole reason for the `v0.2.0` pin.
**Storage**: N/A — config read/write *semantics* are unchanged; no new on-disk
state, no `fleet.SchemaVersion` bump. The read path gains a bounded 1 MiB cap
(`config.DefaultMaxBytes`), benign for kilobyte fleet configs.
**Testing**: `go test ./...` (the existing `internal/fleet` load/save parity
suite, run **unchanged** — the regression guard); a new `cmd`-level test asserting
`__schema` emits valid JSON over the eight subcommands; a dependency-isolation
assertion via `go list -deps`. Full gate: `make ci` (fmt-check, vet, lint, test).
**Target Platform**: Linux / macOS developer toolchain; a single CLI binary.
**Project Type**: Single Go module — CLI. The change touches
`internal/fleet/load.go`, `cmd/root.go` (+ one small new `cmd` file), the
constitution, `go.mod`/`go.sum`, and the agent docs.
**Performance Goals**: N/A — no hot-path change; the bounded config read is
negligible; no network, no caching surface affected.
**Constraints**: `cmd.SchemaVersion` (wire envelope) and `fleet.SchemaVersion`
(on-disk format) are frozen (FR-016/FR-018, SC-006). Stream separation holds —
`__schema` writes JSON to stdout only (FR-017). 100% godoc on any new exported
identifier (FR-019). Import-isolation invariant (FR-003a, SC-003a).
**Scale/Scope**: ~3 call-site swaps in `load.go` (`loadConfigFile`,
`LoadTemplates`, `SaveTemplates`), relocation of the hujson write helpers from `load.go` to `add.go` (still live — they back the `Add` path),
one new `cmd/schema.go` (~20 lines) + one `root.go` `AddCommand`, the constitution
amendment (v1.1.0 → v1.2.0), `go.mod`/`go.sum`, and `CLAUDE.md`/`AGENTS.md`. No
behavior-test expectation changes.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against Constitution **v1.1.0** (this feature amends it to v1.2.0 as
part of the deliverable — FR-001/FR-002):

- **I. Thin-Orchestrator Code Quality** — ✅ PASS. No upstream-tool logic is
  re-implemented. The config-IO swap *replaces one maintained config-parsing
  library's primitives with another's* — the same delegate-don't-reimplement
  philosophy already applied to `hujson`. `__schema` is thin wiring over
  `schema.BuildSchema`/`schema.BuildMCPSchema` (mirroring
  `schema.NewSchemaCommand`, adding only MCP positional-argument augmentation).
  `go build`/`go vet` stay clean; `load.go` stays
  focused (the hujson write helpers relocate to `add.go` where they're used, net-shrinking `load.go`).
- **II. Testing Standards (Build-Green + Real-World Dry-Run)** — ✅ PASS. The
  mutating-command dry-run substrate is untouched; the existing load/save +
  dry-run tests are the regression guard, run unchanged (FR-010). New tests are
  additive: a `__schema` output-shape test and a dependency-isolation check — both
  verify pure, stateless surfaces, exactly the unit-test guidance's sweet spot.
- **III. UX Consistency (Three-Turn Mutation Pattern)** — ✅ PASS / N/A. No
  mutating-command surface, output, exit code, commit message, or PR title
  changes (FR-016). `__schema` is read-only. `CollectHints` paths are untouched.
- **IV. Performance (Parallelism, Cache, 5-Min Ceiling)** — ✅ PASS / N/A. No new
  I/O on any hot path; the bounded config read is negligible; no network calls;
  no caching surface affected.
- **Third-Party Dependencies** — ✅ PASS **via the in-scope amendment**. Adding a
  new direct dep requires a constitution amendment (MINOR bump) — which this
  feature *delivers* (FR-001/FR-002). The three-alternatives evaluation: *stdlib*
  rejected (AX contracts are bespoke), *vendoring* rejected (forks the shared
  DNA), *CLI-delegation* N/A (the tool's own output layer). The `v0.2.0`
  import-isolation choice keeps the transitive footprint minimal (no OTel/gRPC —
  FR-003a), honoring the section's "direct dependencies stay minimal" intent in
  spirit as well as letter. This satisfies the Development-Workflow rule that new
  deps need an amendment, not a one-line PR rationale.
- **Declarative Reconcile Invariants** — ✅ PASS. `fleet.json` stays the source of
  truth; no gpg/git-path changes; `fleet.local.json` handling unchanged; config
  read/write semantics preserved byte-for-byte (modulo documented canonical
  whitespace).

**Result**: No violations. The one new direct dependency is admitted through the
constitution's own amendment process, bundled into this feature. Complexity
Tracking table is empty.

**Post-design re-check** (after Phase 1): unchanged — PASS. The design decisions
*strengthen* the dependency gate rather than weaken it: Decision 1's `v0.2.0`
import-isolation keeps OTel/gRPC/protobuf out of the build (FR-003a/SC-003a), so
the "direct dependencies stay minimal" intent holds in spirit. No new abstraction,
network path, command-surface change, or on-disk-format change was introduced by
the contracts/data-model. `__schema` is read-only and additive; the config swap is
behavior-preserving. No new violations surfaced.

## Project Structure

### Documentation (this feature)

```text
specs/016-ax-go-foundation/
├── plan.md              # This file (/speckit-plan output)
├── research.md          # Phase 0 — decisions (v0.2.0 pin & import isolation, test parity, write mechanics, __schema version + visibility, go directive)
├── data-model.md        # Phase 1 — the __schema output shape; "no new persistent entities"
├── quickstart.md        # Phase 1 — how to verify SC-001..SC-007 locally
├── contracts/
│   └── schema-command.md # Phase 1 — the __schema CLI contract (usage, flags, output, streams, exit codes)
├── checklists/
│   └── requirements.md   # Spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 — created by /speckit-tasks, NOT here
```

### Source Code (repository root)

```text
.specify/memory/
└── constitution.md                  # CHANGED — amend §Third-Party Dependencies (add ax-go,
                                      #           3-alternatives rationale + import-isolation note);
                                      #           bump v1.1.0 → v1.2.0; footer + Sync Impact Report

go.mod                               # CHANGED — `go 1.25.8` → `go 1.26.4`; add direct require
                                      #           github.com/rshade/ax-go v0.2.0 (one new entry)
go.sum                               # CHANGED — ax-go + its config/schema/contract closure checksums

internal/fleet/
└── load.go                          # CHANGED — read path: loadConfigFile + LoadTemplates use
                                      #   config.Parse/config.ParseFile (was hujson.Standardize+Unmarshal);
                                      #   write path: SaveTemplates uses config.Patch + existing atomicWrite
                                      #   (was hujson.Parse/Value.Patch/Pack via writeHujson);
                                      #   relocate writeHujson/readHujsonOrScaffold helpers to add.go (live — back the Add path);
                                      #   probeConfigPath / mergeConfigs / version check UNCHANGED
internal/fleet/
└── load_test.go                     # UNCHANGED assertions (FR-010) — runs as the parity guard;
                                      #   add (optional) parity cases only if a gap is found
internal/fleet/
└── add.go                           # CHANGED — receives the relocated writeHujson/readHujsonOrScaffold
                                      #   helpers (moved out of load.go when it dropped its hujson import);
                                      #   add.go remains hujson's live consumer via appendRepoMember (FR-011)

cmd/
├── root.go                          # CHANGED — root.AddCommand(newSchemaCmd(root)) after the 8 subcommands
├── schema.go                        # NEW — newSchemaCmd: mirrors schema.NewSchemaCommand on
│                                     #   schema.BuildSchema(root, WithSchemaVersion(toolVersion()))/BuildMCPSchema,
│                                     #   adds MCP positional-arg augmentation; Hidden=true;
│                                     #   toolVersion() via runtime/debug.ReadBuildInfo, "dev" fallback
└── schema_test.go                   # NEW — __schema emits valid JSON over all 8 subcommands (SC-004);
                                      #   --as mcp returns the tools list; other commands' output unchanged

CLAUDE.md / AGENTS.md                 # CHANGED — record ax-go (v0.2.0, import-isolated config/schema)
                                      #   in Active Technologies / dependency notes; link the phase plan;
                                      #   update the SPECKIT plan pointer; note the go-directive bump

cmd/template.go, cmd/output_test.go   # CHANGED (incidental) — extract a `templateCommandName` const to
                                      #   satisfy goconst lint after the surrounding edits; no behavior change
```

**Structure Decision**: Single Go module, unchanged layout. The feature is a
dependency adoption plus two surgical edits — a primitive swap inside
`internal/fleet/load.go` and an additive `cmd` subcommand — so no new top-level
directories are introduced. The only structural addition is `cmd/schema.go`
(+ its test), keeping the `__schema` wiring isolated from `root.go`'s flag setup.

## Complexity Tracking

> No Constitution violations — the new dependency is admitted via the in-scope
> amendment, which is the constitution's prescribed mechanism (not a violation).
> Table intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *(none)*  | —          | —                                    |
