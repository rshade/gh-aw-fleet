# Phase 0 Research: Export Fleet Config Contract into Public `pkg/fleet`

All "NEEDS CLARIFICATION" items from Technical Context are resolved here. The
feature is a constrained refactor, so the research is less "evaluate libraries"
and more "verify the chosen Go mechanics against the actual code." Each decision
below was checked against the current `internal/fleet` source, not just the spec.

---

## Decision 1 — Refactor mechanism: type aliases (not embedding)

**Decision**: In `internal/fleet`, re-expose every moved contract type with a Go
**type alias** (`type Config = pkgfleet.Config`), and re-export the constant with
`const SchemaVersion = pkgfleet.SchemaVersion`. The seven contract types and
`SchemaVersion` physically live in `pkg/fleet`.

**Rationale**: A type alias makes `internal/fleet.Config` and `pkg/fleet.Config`
the *same* type — there is exactly one underlying definition, so the control plane
(importing `pkg/fleet`) and the engine (using the alias) can never drift, and JSON
encoding is provably identical because it is literally the same struct + tags.
This is the issue's recommended approach and what the spec selected (FR-007,
Assumptions "Preferred refactor approach").

**Alternatives considered**:

- **Anonymous embedding** (`type Config struct { pkgfleet.Config }`) — the issue's
  stated *fallback*. Rejected: it creates a *distinct* internal type, reintroducing
  a second (thin) definition and undermining "one canonical type." Field promotion
  preserves most JSON behavior but is a subtle, easy-to-break surface; and anywhere
  a literal `pkg/fleet.Config` is required (future `RemoteSource` parse target,
  control-plane interop) the embedder is the wrong type. Embedding's only advantage —
  methods can stay attached — is outweighed by Decision 2 making the method
  conversions cheap and mechanical anyway.
- **Re-declare structs in both packages** — the status quo the feature exists to
  kill. Rejected by definition (silent drift is the bug).

---

## Decision 2 — Methods on the aliased type must become standalone functions

**Decision**: Convert **both** impure methods currently hanging off `*Config` into
standalone functions in `internal/fleet`:

- `(c *Config) EffectiveCompileStrict(ctx, repo)` → `func EffectiveCompileStrict(c *Config, ctx context.Context, repo string)` (FR-008/FR-009, already in spec)
- `(c *Config) ResolveRepoWorkflows(repo)` → `func ResolveRepoWorkflows(c *Config, repo string)` (**NOT in the spec — discovered during code verification**)

The one *pure* method, `(c *Config) EffectiveEngine(repo)`, moves **with** the
type into `pkg/fleet` and stays a method (FR-004).

**Rationale**: Go forbids declaring methods on a non-local (aliased) type — once
`Config` aliases `pkg/fleet.Config`, any `func (c *Config) …` in `internal/fleet`
fails to compile ("cannot define new methods on non-local type"). The spec's Edge
Case "Methods cannot follow an aliased type" states the rule but FR-009 only
enumerated the single `EffectiveCompileStrict` call site. Verification against the
code found a second method, `ResolveRepoWorkflows` in `internal/fleet/load.go:371`,
with **7 production call sites** the spec did not budget for:

| Call site | Current | After |
|-----------|---------|-------|
| `cmd/list.go:51` | `cfg.ResolveRepoWorkflows(r)` | `fleet.ResolveRepoWorkflows(cfg, r)` |
| `internal/fleet/add.go:135` | `cfg.ResolveRepoWorkflows(opts.Repo)` | `ResolveRepoWorkflows(cfg, opts.Repo)` |
| `internal/fleet/deploy.go:218` | `cfg.ResolveRepoWorkflows(repo)` | `ResolveRepoWorkflows(cfg, repo)` |
| `internal/fleet/list_result.go:53` | `cfg.ResolveRepoWorkflows(repo)` | `ResolveRepoWorkflows(cfg, repo)` |
| `internal/fleet/manifest.go:92` | `cfg.ResolveRepoWorkflows(repo)` | `ResolveRepoWorkflows(cfg, repo)` |
| `internal/fleet/sync.go:43` | `cfg.ResolveRepoWorkflows(repo)` | `ResolveRepoWorkflows(cfg, repo)` |
| `internal/fleet/status.go:263` | `cfg.ResolveRepoWorkflows(repo)` | `ResolveRepoWorkflows(cfg, repo)` |
| `internal/fleet/sync_test.go` | (comment only, line 271 — not a call) | no change needed |

Plus the single `EffectiveCompileStrict` site at `internal/fleet/deploy.go:1060`
and its test in `internal/fleet/schema_test.go`. `ResolveRepoWorkflows` cannot
instead move to `pkg/fleet` (the spec defers load/resolve logic — FR-016 — and it
returns the internal `ResolvedWorkflow` type, which stays internal), so the
standalone-function-in-`internal/fleet` form is the only option consistent with
the slice's scope.

**Signature ordering note**: Go style puts `context.Context` first. The relocated
`EffectiveCompileStrict` should read `func EffectiveCompileStrict(ctx context.Context, c *Config, repo string)`
(context first, then the former receiver) — confirm during implementation; the
exact parameter order is the implementer's mechanical choice as long as the single
call site matches.

**Alternatives considered**: Keep methods via embedding (Decision 1 rejects
embedding wholesale). Move `ResolveRepoWorkflows` + `ResolvedWorkflow` to
`pkg/fleet` (rejected: out of scope for this slice per FR-016; pulls load logic
forward into #141's territory).

---

## Decision 3 — Package layout and what stays behind

**Decision**: New file `pkg/fleet/config.go` holds the seven contract types,
`SchemaVersion`, the `EffectiveEngine` method, and the package doc comment, with
**stdlib-only** content (no imports needed). Everything else stays in
`internal/fleet`:

- **Impure / deploy-path** (FR-008): `EffectiveCompileStrict` (now a func),
  `ghRepoVisibility` (network — `deploy.go:943`), `truncateReason`,
  `effectiveCompileStrictReasonMax`, `CompileStrictSource*` constants,
  `VisibilityPublic`.
- **Catalog cache** (FR-015): `Templates`, `TemplateSource`, `TemplateWorkflow`,
  `Evaluation` — these are the upstream-catalog cache, not the wire contract.
- **Load / merge / resolve** (FR-016): `LoadConfig`, `mergeConfigs`, the HuJson
  read path, `ResolveRepoWorkflows` (now a func).

**Rationale**: The contract package must stay free of network dependencies and
internal-only types so an external module can import it cleanly with no transitive
surprises (FR-013 one-way dependency). Keeping the catalog and load logic internal
honors the "contract only" scope discipline (spec Assumptions / Scope discipline).

**Verification**: The seven contract types form a closed set — `Config` → `Defaults`,
`Profile`, `RepoSpec`; `Profile` → `SourcePin`, `ProfileWorkflow`; `RepoSpec` →
`ExtraWorkflow`. No field references `Templates`, `ResolvedWorkflow`, or any
internal type, so they relocate without dragging internal symbols across the
boundary. (Confirmed by reading every field type in `internal/fleet/schema.go`.)

---

## Decision 4 — Golden round-trip baseline is a NEW canonical file, not `fleet.example.json`

**Decision**: Commit a new `pkg/fleet/testdata/config.canonical.json` produced by
`json.MarshalIndent(cfg, "", "  ")` (the repo's canonical `writeJSON` format —
2-space indent, trailing newline) of the data unmarshaled from `fleet.example.json`.
The golden round-trip test reads `fleet.example.json` (the human-facing example,
single source of input data), unmarshals into `pkg/fleet.Config`, re-marshals, and
asserts byte-equality against `config.canonical.json`.

**Rationale**: `fleet.example.json` **cannot** be the byte-for-byte target, for two
independent reasons found by inspecting the file:

1. **Hand alignment**: the example column-aligns workflow entries
   (`"audit-workflows",          "source": …`). `json.MarshalIndent` produces
   single-space-after-colon output and never reproduces that padding.
2. **`omitempty` drops fields**: the example's `acme/widgets` repo carries
   `"extra": []` and `"exclude": []`, but `RepoSpec.ExtraWorkflows` and
   `.ExcludeFromProfiles` are tagged `omitempty`. An empty slice (len 0) is omitted
   on marshal, so a re-marshal *deletes* both keys.

Therefore "re-marshal == `fleet.example.json`" is structurally impossible. The
spec's intent (SC-002: "byte-identical to the committed baseline"; FR-006:
"re-marshals to the canonical baseline") is satisfied by committing the canonical
form as the baseline — `fleet.example.json` supplies the *input data*, the new
golden supplies the *expected wire bytes*. This also makes the test a genuine
regression guard for any future tag change.

**Alternatives considered**:

- **Compare re-marshal to `fleet.example.json` directly** — rejected: impossible
  (above); the test would never pass.
- **`reflect.DeepEqual` on structs instead of bytes** — rejected: proves semantic
  round-trip but not *byte*-fidelity, which is the actual correctness guarantee for
  the remote-pull wire path (Story 2).
- **No committed golden; assert marshal idempotence** (`m(u(m(u(x)))) == m(u(x))`) —
  rejected: proves stability, not that the right fields are present/absent. A
  committed golden pins the exact expected bytes.

---

## Decision 5 — Demonstrating the "external consumer" (SC-001) without a second module

**Decision**: Place the new tests in `package fleet_test` (a black-box external
test package) inside `pkg/fleet/`. The test imports
`github.com/rshade/gh-aw-fleet/pkg/fleet`, declares values of all seven types,
reads `fleet.SchemaVersion` (asserts `== 1`), and calls `cfg.EffectiveEngine(repo)`.

**Rationale**: A `_test` external package can reference **only exported**
identifiers — the compiler enforces exactly the constraint a real outside consumer
faces, so a passing black-box test *is* the SC-001 proof that the public surface is
complete and importable. The spec's Independent Test for Story 1 explicitly allows
"the package's own external example test" as an acceptable form, so no nested
throwaway module (separate `go.mod` + `replace`) is required for this slice. An
`Example` function additionally renders in godoc and doubles as living
documentation of the import path.

**Alternatives considered**: A separate nested module under `pkg/fleet/internal_e2e/`
with its own `go.mod` — rejected as heavier than the slice needs; the external test
package gives the same compile-time guarantee within the existing module and CI run.

---

## Decision 6 — `SchemaVersion` re-export keeps internal churn mechanical

**Decision**: `internal/fleet` keeps the identifier `SchemaVersion` via
`const SchemaVersion = pkgfleet.SchemaVersion`. No call site that references the
bare `SchemaVersion` (verified: `add.go:413`, `load.go:164/222`, `fetch.go:40`,
plus ~20 test sites, and `Templates{Version: SchemaVersion}`) needs to change.

**Rationale**: FR-007 requires existing internal code to compile "with only
mechanical changes." A re-export const preserves the bare name everywhere it is
used, so the move touches *zero* `SchemaVersion` call sites. The value (`1`) is
unchanged (FR-011, SC-006). `cmd.SchemaVersion` (the JSON-envelope version in
`cmd/output.go`) is a separate, unrelated constant and is not touched — the
two-`SchemaVersion` invariant in CLAUDE.md still holds.

---

## Resolved unknowns summary

| Unknown (from Technical Context) | Resolution |
|----------------------------------|------------|
| Alias vs. embedding | Type aliases (Decision 1) |
| Which methods move vs. become functions | `EffectiveEngine`→moves; `EffectiveCompileStrict` + `ResolveRepoWorkflows`→funcs (Decision 2) |
| What stays in `internal/fleet` | impure/catalog/load logic (Decision 3) |
| Golden baseline definition | new committed canonical file (Decision 4) |
| How to prove external importability | `package fleet_test` black-box test (Decision 5) |
| Keeping internal `SchemaVersion` references compiling | re-export const (Decision 6) |
| New dependencies | none (stdlib only; FR-014/SC-006) |
