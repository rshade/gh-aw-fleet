# Phase 0 Research: Adopt ax-go as the AX Foundation — Phase 1

All "NEEDS CLARIFICATION" items from Technical Context are resolved here. Like
slice 015, this is a constrained adoption, so the research is less "evaluate
libraries" and more "verify the chosen mechanics against the actual ax-go and gh-aw-fleet
source." Every decision below was checked against `ax-go@v0.2.0` and the current
`internal/fleet` / `cmd` code, not just the spec.

---

## Decision 1 — Pin `ax-go@v0.2.0` and import only the isolated `config` + `schema` packages

**Decision**: Pin `github.com/rshade/ax-go` at **`v0.2.0`** (the issue proposed
`v0.1.0`) and import **only** the import-isolated thin-consumer packages
`github.com/rshade/ax-go/config` and `github.com/rshade/ax-go/schema` (which pull
in the stdlib-only `…/ax-go/contract`). **Never** import the root `package ax`.

**Rationale**: `v0.1.0`'s only public surface is the root `package ax`, and that
package is a single Go compilation unit whose files `execute.go`, `http.go`,
`trace.go`, and `telemetry.go` import **OpenTelemetry + gRPC + protobuf**. Because
Go compiles a package as a unit, importing `package ax` for *anything*
(`ParseConfig`, `NewSchemaCommand`) would have dragged that entire heavy stack
into the gh-aw-fleet binary and `go.mod` — absurd for a thin git/`gh`-orchestrator
that uses none of it. `v0.2.0` (#78/#79, "add import-isolated public contract
packages for thin consumers") factored the pure contracts into sibling packages.
Verified against the `v0.2.0` tag:

- `config/config.go` imports only stdlib + `ax-go/contract` + `ax-go/internal/config`.
- `schema/schema.go` imports only stdlib + `cobra` + `ax-go/contract` +
  `ax-go/internal/{mcp,schema}`.
- `contract/*` imports only stdlib (`context`, `encoding/json`, `errors`, `fmt`,
  `io`, `strings`).
- Grepping the entire `config` + `schema` + `contract` + `internal/{config,mcp,schema}`
  closure for `go.opentelemetry.io` / `google.golang.org/grpc` / `…/protobuf`
  returns **zero** hits.

So importing `ax-go/config` + `ax-go/schema` compiles **none** of OTel/gRPC/protobuf
into gh-aw-fleet (Go's pruned module graph also keeps those modules out of
gh-aw-fleet's `go.mod`). `cobra` is already a direct dep; `schema` adds no new
compiled module beyond ax-go's own packages. This is the spec's FR-003a and is
asserted by SC-003a (`go list -deps` reaches no OTel/gRPC package).

**Alternatives considered**:

- **Pin `v0.1.0` and import `package ax`** (the issue as literally written) —
  rejected: pulls OTel + gRPC + protobuf into a thin-orchestrator binary, in
  tension with the constitution's minimal-deps intent. The user surfaced `v0.2.0`
  specifically to avoid this.
- **Fork / vendor just the config + schema code** — rejected: forks the shared DNA
  (the exact anti-goal of standing on ax-go); `v0.2.0` already solves it upstream.
- **Wait for a further ax-go release** — unnecessary: `v0.2.0` already ships the
  isolation.

**Spec impact**: pin, import paths, and FR-003a/SC-003a were updated in spec.md to
reflect `v0.2.0` + the `config`/`schema` packages (helper names lose the redundant
`Config` suffix: `config.Parse`/`config.ParseFile`/`config.Patch`/`config.PatchFile`).

---

## Decision 2 — The existing load/save tests are the parity guard, run unchanged

**Decision**: Treat the existing `internal/fleet` load/save suite as the
regression guard and run it **with zero assertion edits** (FR-010). Add new parity
cases only if implementation surfaces a gap.

**Rationale**: Verified against `internal/fleet/load_test.go`: the only
error-string assertions are on the `"ambiguous"` message — and that text comes
from `probeConfigPath`, which is **not** being swapped (it stays in `load.go`,
FR-008). No test asserts on the `fmt.Errorf("parse %s: %w", …)` text that
`config.Parse` replaces with ax-go's typed `*contract.Error` (`config_invalid`),
so the internal error-shape change is invisible to the suite. The comment-
preservation test (`TestSaveTemplates_PreservesEvaluationsComments`) asserts on
**substrings** (`upstream/new` present, `upstream/old` absent, the sentinel
comment present), not byte-golden output — and `config.Patch` preserves comments
the same way the current `hujson.Format()`+`Pack()` does. `TestBillingMetadata_RoundTrip`
exercises marshal fidelity governed by struct tags, untouched by the read swap.
The probe/merge/version-check tests don't touch the swapped primitives at all.

**Risk + mitigation**: `config.Patch` normalizes whitespace to canonical hujson
formatting (documented: indentation/alignment/blank-lines not byte-preserved), the
same *class* of normalization the current writer already applies. The single
residual risk is that normalization perturbs text near the sentinel comment enough
to break the substring match. Mitigation: run `TestSaveTemplates_PreservesEvaluationsComments`
first during implementation; if it breaks, the comment is still present (it's a
formatting drift, not a loss) and the assertion can be confirmed against the actual
`config.Patch` output without weakening it.

**Alternatives considered**: Add byte-golden write tests — rejected as
over-specifying; the spec's parity guarantee is "comments preserved + existing
tests pass," not byte-identical templates output (which neither writer guarantees).

---

## Decision 3 — Write path: `config.Patch` (reader→bytes) + the project's `atomicWrite`

**Decision**: In `SaveTemplates`, replace the `writeHujson` →
`hujson.Parse`/`Value.Patch`/`Pack` path with `config.Patch(ctx, reader,
patchBytes)` and write the returned bytes through the **existing**
`atomicWrite` helper. Keep the existing fallback: on patch error, log
`event=hujson_fallback_to_rewrite` at `warn` and rewrite via `writeJSON`
(FR-009). Remove the now-dead `writeHujson` / `readHujsonOrScaffold` helpers.

**Rationale**: Two ax-go write entry points exist — `config.Patch` (returns patched
bytes; caller writes) and `config.PatchFile` (atomic write, **preserves the file's
existing mode**). The project's `atomicWrite` writes `0o600` and guarantees a
trailing newline (FR-007). Using the reader→bytes `config.Patch` + the existing
`atomicWrite` keeps that exact policy and the existing `.tmp`+rename atomicity,
rather than inheriting `PatchFile`'s mode-preservation (which would diverge from
the current 0600 behavior on a fresh file). The RFC 6902 patch document is built
exactly as today (`buildTemplatesPatch` — three `add` ops on `/version`,
`/fetched_at`, `/sources`), so `/evaluations` and its comments are untouched.

**Alternatives considered**:

- **`config.PatchFile`** — simpler (one call, atomic) but changes the write to
  mode-preserving instead of fixed-0600, and skips the project's trailing-newline
  guarantee. Rejected to hold FR-007 byte-policy parity. (Reconsider in a later
  phase if the write policy is itself unified under ax.)
- **Keep `writeHujson` and only swap the read path** — rejected: the spec's
  acceptance criteria put both read *and* write on the ax-go primitives, and
  leaving `writeHujson` keeps a second hujson write path the swap is meant to
  retire from `load.go`. (`hujson` stays a direct dep regardless, via `Add` —
  FR-011.)

---

## Decision 4 — `__schema` tool version via `runtime/debug.ReadBuildInfo`, `dev` fallback

**Decision**: `schema.WithSchemaVersion(v)` is fed by a small `toolVersion()`
helper that reads `runtime/debug.ReadBuildInfo().Main.Version`, falling back to
`"dev"` when build info is absent or reports `"(devel)"`.

**Name-vs-field caveat (verified against `ax-go@v0.2.0` source —
`schema/schema.go:62-99`)**: despite its name, `WithSchemaVersion` populates the
**`version`** field (`Schema.Version`, `json:"version"`) — the *tool* version —
**not** `schema_version`. The `schema_version` field is a fixed ax-owned constant
(`schema.SchemaVersion = contract.ErrorSchemaVersion`) that the option does not
touch. The option's own godoc confirms this: *"sets the tool version reported by
`__schema`."* So `schema.WithSchemaVersion(toolVersion())` correctly fills the
`version` field that SC-004 (and quickstart §3's `assert s["version"]`) check, and
`tool` comes from `root.Name()` — which is `"gh-aw-fleet"` (root `Use` is
`gh-aw-fleet`), satisfying the `tool == "gh-aw-fleet"` assertion. Do not be misled
by the option name into wiring it expecting it to set `schema_version`.

**Rationale**: gh-aw-fleet has **no** version constant anywhere — releases are
release-please git tags, not embedded in the binary (verified: no `var version` /
`Version =` in `main.go`, `cmd/`, or `internal/`). `ReadBuildInfo` yields the real
module version for binaries installed via `go install …@vX` and a stable `"dev"`
for local `go build`/`go run`, with zero new build wiring (no ldflags, no
goreleaser change in phase 1). The `__schema` `version` field is informational
machine-discoverability; perfect release-version reporting is not a phase-1 goal.

**Alternatives considered**:

- **`-ldflags -X cmd.version=…`** — rejected for phase 1: requires goreleaser/CI
  wiring that's out of scope; can be layered later by setting the same var.
- **Omit the version** (`WithSchemaVersion("")`) — rejected: leaves an empty
  `version` field, strictly less useful than `"dev"` / the installed tag.

---

## Decision 5 — `__schema` wiring: hidden, added last, both `ax` and `mcp` formats

**Decision**: Add a `cmd/schema.go` with `newSchemaCmd(root)` returning a hidden
command that **mirrors** `schema.NewSchemaCommand` — the same `--as ax|mcp`
switch, `schema.BuildSchema(root, schema.WithSchemaVersion(toolVersion()))` for
the ax tree, and `contract.NewError` on an invalid `--as` — but calls
`schema.BuildSchema`/`schema.BuildMCPSchema` **directly** so the `mcp` path can
augment each tool with its CLI positional arguments (the repo arg on
`add`/`deploy`/`sync`/`status`/`upgrade`, variadic repos on `consumption`),
which ax-go's flag-only reflection cannot derive. Call
`root.AddCommand(newSchemaCmd(root))` in `NewRootCmd` **after** the eight
subcommands are added. Keep ax-go's default `--as ax|mcp` (FR-014).

**Rationale**: `schema.BuildSchema` reflects the command tree **lazily** inside
`RunE`, so adding `__schema` after the other subcommands means it sees the full
tree (all eight subcommands + root flags) at invocation time — no init-order or
import-cycle issue (the command merely *references* `root`). `Hidden = true` keeps
the machine-facing `__schema` out of the human `--help` listing (spec Assumption)
while leaving it fully invokable by name and over MCP. The `mcp` format is free
and directly serves the agent/control-plane discoverability goal.

**One contained consequence — the only ax error envelope emitted in phase 1**:
`newSchemaCmd`'s `RunE` (mirroring `schema.NewSchemaCommand`) calls
`contract.NewError(...)` for an invalid `--as` value (e.g.
`__schema --as bogus`), emitting an ax-style error + exit code.
This is acceptable and in-scope: it is *within the ax-native `__schema` command's
own surface*, additive, and does not change any existing command's error handling.
It does not constitute adopting the ax error envelope tool-wide (that stays
deferred, FR-015 / Out of Scope).

**Alternatives considered**:

- **Non-hidden `__schema`** — rejected: clutters human `--help` with a
  machine-only command; the `__` prefix + `Hidden` is the ax convention.
- **`ax`-only (drop `mcp`)** — rejected: the MCP adapter is free and is exactly
  the control-plane/agent payoff; FR-014 requires both.
- **Custom schema builder to suppress the `error_envelope` block** — rejected per
  spec FR-015 (forking ax's output diverges from the shared contract; the block is
  a documented forward-declaration).

---

## Decision 6 — Raise the module `go` directive `1.25.8 → 1.26.4`

**Decision**: Set `go.mod`'s directive to `go 1.26.4` (matching `ax-go@v0.2.0`).

**Rationale**: `ax-go@v0.2.0`'s `go.mod` declares `go 1.26.4`; the Go toolchain
requires the main module's directive to be ≥ a dependency's. The local gate
already runs Go 1.26.4 (per AGENTS.md), so the tool already builds. Per the user's
spec clarification (FR-004), the bump happens in this phase; the accepted
consequence is that every `pkg/fleet` consumer (the external control plane, #152)
must also move to 1.26.x. CLAUDE.md / AGENTS.md notes that lean on the prior
`1.25.8` compatibility claim are updated to match (FR-020).

**Alternatives considered**: Gate the bump as a separate prerequisite change, or
hold adoption until ax-go lowers its directive — both rejected by the user's
FR-004 answer (bump now).

---

## Resolved unknowns summary

| Unknown (from Technical Context) | Resolution |
|----------------------------------|------------|
| Which ax-go version / import surface | `v0.2.0`, import-isolated `config` + `schema` only (Decision 1) |
| Does adopting ax-go pull OTel/gRPC into the binary | No — `config`/`schema` closure is OTel/gRPC-free; verified (Decision 1) |
| Will existing load/save tests still pass | Yes, unchanged — no test asserts swapped error text; comment test is substring-based (Decision 2) |
| Write-path mechanics (perms, newline, fallback) | `config.Patch` + existing `atomicWrite`; keep fallback; drop dead helpers (Decision 3) |
| Where does `__schema`'s tool version come from | `runtime/debug.ReadBuildInfo`, `dev` fallback (Decision 4) |
| `__schema` visibility / formats / wiring order | Hidden, added last, `ax`+`mcp` (Decision 5) |
| Is any ax error envelope emitted in phase 1 | Only `__schema`'s own `--as` validation error — contained & acceptable (Decision 5) |
| `go` directive | raise `1.25.8 → 1.26.4` (Decision 6) |
| New third-party deps beyond ax-go | none — `cobra` already direct; `hujson` stays direct (FR-011) |
