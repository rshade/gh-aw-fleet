# Feature Specification: Adopt ax-go as the AX Foundation — Phase 1

**Feature Branch**: `016-ax-go-foundation`
**Created**: 2026-06-21
**Status**: Draft
**Input**: GitHub issue #156 — "feat: adopt ax-go as the AX foundation — phase 1 (amend constitution + config primitives + `__schema` discoverability)"

## Overview

`github.com/rshade/ax-go` is the rshade portfolio's shared "Agentic Experience
(AX) foundation for Go CLI tools." gh-aw-fleet today hand-rolls every contract
ax-go standardizes: the JSON-on-stdout envelope (`cmd/output.go`,
`cmd.SchemaVersion`), structured diagnostics (`fleet.Diagnostic` +
`CollectHints`), stream separation, structured logging
(`internal/log.Configure` over zerolog), and comment-preserving config IO
(`internal/fleet/load.go` over `tailscale/hujson`). That duplication is exactly
what ax-go exists to remove, and it becomes a liability as the tool moves toward
headless / control-plane use (#145, #146): every consumer must special-case this
tool's bespoke AX surface.

This is **phase 1** of a multi-phase adoption. It deliberately takes on only the
two lowest-risk fronts and lands the dependency the constitutional way — the same
amendment process #73 used for hujson:

1. **Amend the constitution** to approve `github.com/rshade/ax-go` as a direct
   dependency (MINOR bump, v1.1.0 → v1.2.0) and pin it at `v0.2.0`.
2. **Swap the config IO primitives** in `internal/fleet/load.go` from direct
   `tailscale/hujson` calls to ax-go's import-isolated `config` package
   (`config.Parse` / `config.Patch`) — behavior-equivalent, proven against the
   existing load/save tests.
3. **Add net-new `__schema` discoverability** (via ax-go's import-isolated
   `schema` package) so agents and the future control plane can introspect the
   CLI surface — purely additive, zero wire-contract change.

**Pin advanced from the issue's `v0.1.0` to `v0.2.0`**: ax-go `v0.2.0` (#78/#79)
adds *import-isolated public contract packages for thin consumers* — `config`,
`schema`, and their stdlib-only `contract` base — whose import graphs never reach
ax-go's OpenTelemetry / gRPC / protobuf dependencies. Importing the root
`package ax` (the only public surface in `v0.1.0`) would have compiled that
entire heavy stack into gh-aw-fleet's binary for features that use none of it;
importing only `ax-go/config` + `ax-go/schema` from `v0.2.0` pulls **zero** of it.
This keeps the thin-orchestrator binary lean and is a strict improvement over the
issue's original `v0.1.0` plan.

The riskier convergences — error envelopes, the `--output json` payload
contract, the logger, and idempotency/mode/dry-run context — are explicitly
deferred to enumerated follow-up phases (see Out of Scope). Phase 1 must not
touch the frozen, golden-pinned `cmd.SchemaVersion` wire contract.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Land ax-go the constitutional way, behavior-equivalent (Priority: P1)

A maintainer of the rshade Go-CLI portfolio adds `ax-go` as the shared AX
foundation, governed by the constitution exactly as hujson was. They amend
§Third-Party Dependencies, pin `ax-go@v0.2.0`, and migrate the config
read/write primitives in `internal/fleet/load.go` from direct hujson calls to
`config.Parse` / `config.Patch`. Nothing an operator or a config file
observes changes — comments are still preserved, the `.hujson`/`.json` probe
still works, and a malformed config still fails — but the tool now stands on the
shared contract instead of its own.

**Why this priority**: This is the foundational, gating step. The dependency
cannot be adopted at all without the amendment, and the config-IO swap is the
first concrete proof that ax-go's primitives are a drop-in for the tool's own.
Delivering only this story already eliminates one slice of duplication and
unblocks every later phase. It carries the most risk (a config-IO regression
would corrupt operator-authored comments), so it must be proven first.

**Independent Test**: Run the existing `internal/fleet` load/save suite
(`TestLoadConfig_HuJsonSyntax`, `TestLoadConfig_HujsonExtensionWins`,
`TestLoadConfig_BothExtensionsError`, `TestLoadTemplates_HuJsonSyntax`,
`TestProbeConfigPath_*`, `TestSaveTemplates_PreservesEvaluationsComments`,
`TestBillingMetadata_RoundTrip`, `TestAdd_Apply_*`) against the swapped
implementation with **no edits to the test assertions**; all pass. The
`TestAdd_Apply_*` cases are included because `load.go` drops its `hujson` import
while `add.go` keeps mutating the hujson AST — they guard that the `Add` write
path still works after the swap. Confirm `go.mod` carries exactly one
new direct require (`ax-go v0.2.0`) and the constitution reads v1.2.0.

**Acceptance Scenarios**:

1. **Given** a `fleet.json` (or `fleet.hujson`) containing `//` line comments,
   `/* */` block comments, and trailing commas, **When** the loader reads it via
   `config.Parse`, **Then** it unmarshals successfully and produces the same
   `Config` value the current `hujson.Standardize` path produces.
2. **Given** a `templates.json` carrying operator comments on individual
   `/evaluations` entries, **When** `SaveTemplates` applies its RFC 6902 patch
   via `config.Patch`, **Then** `/version`, `/fetched_at`, and `/sources` are
   replaced while the `/evaluations` comments survive intact — identical to
   today's observable result.
3. **Given** both `<base>.json` and `<base>.hujson` present for the same config,
   **When** the loader probes, **Then** it still rejects the pair with an
   "ambiguous" error (the probe logic is unchanged and does not move into
   ax-go).
4. **Given** a `PatchConfig` failure on the write path, **When** `SaveTemplates`
   cannot apply the comment-preserving patch, **Then** it still logs
   `event=hujson_fallback_to_rewrite` (or an equivalent fallback marker) and
   falls back to a full rewrite, exactly as today.

---

### User Story 2 - Agents and the control plane introspect the CLI surface (Priority: P2)

An agent or the future control plane (#145/#146) needs to discover what
`gh-aw-fleet` can do without scraping `--help` text. It invokes a reserved
`__schema` command and receives a machine-readable JSON description of the full
command tree — every subcommand, its flags, and the tool version — emitted on
stdout. An MCP-oriented consumer can request the same surface adapted to the MCP
tools shape.

**Why this priority**: Discoverability is the strategic payoff of the adoption —
it is the contract that lets the portfolio (and eventually a control plane)
consume every tool uniformly. It is net-new and additive, so it carries near-zero
risk to existing behavior, which is why it ranks below the config-IO swap but
above the operator no-change guarantee.

**Independent Test**: Invoke `gh-aw-fleet __schema` and assert the output is
valid JSON whose command tree contains all eight current subcommands (`list`,
`status`, `add`, `template`, `deploy`, `sync`, `upgrade`, `consumption`) and the
tool version; invoke `gh-aw-fleet __schema --as mcp` and assert it returns the
MCP tools-list shape. Diff every other command's output against `main` and
confirm no change.

**Acceptance Scenarios**:

1. **Given** the wired root command, **When** `__schema` is invoked with no
   format flag, **Then** it writes a JSON document on stdout describing the root
   command, its persistent flags (`--dir`, `--log-level`, `--log-format`,
   `--output`), and each subcommand with its flags.
2. **Given** the wired root command, **When** `__schema --as mcp` is invoked,
   **Then** it writes an MCP-adapter JSON document (a `tools` list) on stdout.
3. **Given** any existing command (`list`, `status`, `deploy`, …), **When** it is
   run before and after `__schema` is added, **Then** its stdout, stderr, exit
   code, and JSON envelope are byte-identical, and `cmd.SchemaVersion` is
   unchanged.

---

### User Story 3 - The operator sees zero change (Priority: P3)

An operator who already uses `gh-aw-fleet` runs their normal commands (`list`,
`deploy --apply`, `sync`, `upgrade`, `status`, `consumption`) and sees no
difference in output, exit behavior, on-disk config format, log streams, or the
`--output json` envelope. The adoption is invisible to day-to-day use.

**Why this priority**: This is the safety constraint on the whole phase.
Standing on ax-go is only acceptable if it is regression-free; a changed
envelope, a corrupted comment, or a shifted log line would be a net negative
regardless of the architectural win. It is the lowest priority because it is a
guarantee about *absence of change* rather than new value — but it gates the
release.

**Independent Test**: `make ci` is green with no modified behavior-test
expectations and no new lint suppressions; a representative set of read-only CLI
invocations (`go run . list`, `go run . status`) produce output identical to
`main`.

**Acceptance Scenarios**:

1. **Given** the full existing test suite, **When** it runs against the adopted
   code, **Then** every test passes with no edits to assertions beyond
   references to relocated symbols (if any).
2. **Given** an existing `fleet.json` / `fleet.local.json` / `templates.json`,
   **When** the tool reads and (for templates) writes them, **Then** the on-disk
   format is unchanged and neither `fleet.SchemaVersion` nor `cmd.SchemaVersion`
   is bumped.
3. **Given** the stream-separation invariant, **When** any command runs, **Then**
   structured data stays on stdout and logs/warnings stay on stderr, exactly as
   today.

---

### Edge Cases

- **Import isolation keeps the heavy stack out**: ax-go's module requires
  OpenTelemetry / gRPC / protobuf for its root `package ax` (tracing, HTTP,
  execute helpers). gh-aw-fleet imports only `ax-go/config` + `ax-go/schema`,
  whose transitive closure is stdlib + cobra + the stdlib-only `contract`
  package — so none of the heavy modules are compiled into the binary or added to
  gh-aw-fleet's `go.mod` (FR-003a). A future phase that reaches for ax-go's
  tracing/execute helpers would change this and must be evaluated separately.
- **Read-path size cap is new**: `config.Parse` reads under a bounded cap
  (`config.DefaultMaxBytes`, 1 MiB) that the current `loadConfigFile` does not
  impose. Real fleet configs are kilobytes, so the default is harmless, but a
  pathologically large config that previously loaded would now fail with
  `config_too_large`. This is an accepted, benign new failure mode (see
  Assumptions); no `config.WithMaxBytes` override is needed in phase 1.
- **Parse-error shape differs internally**: a malformed config previously failed
  with `fmt.Errorf("parse %s: %w", path, err)`; via `config.Parse` it fails with
  ax-go's typed `*contract.Error` (`config_invalid`). No existing test asserts on
  that message text — the only error-string assertions cover the "ambiguous"
  probe path, which is untouched — so existing tests pass. Whether the
  user-facing surfacing of that error changes is governed by the (deferred)
  error-envelope phase.
- **Whitespace normalization on write**: `config.Patch` normalizes to canonical
  hujson formatting (it does not preserve byte-exact indentation, value
  alignment, or blank lines), the same class of normalization the current
  `hujson.Format()` + `Pack()` already performs. Comments survive — which is what
  `TestSaveTemplates_PreservesEvaluationsComments` asserts (substring, not
  byte-golden) — so the test passes, but the write path must be validated to not
  perturb comment-adjacent text enough to break the assertion.
- **`__schema` describes contracts not all of which are emitted yet**: ax's
  schema output always includes an `error_envelope` block describing the standard
  ax error shape. gh-aw-fleet has **not** adopted the ax error envelope in phase
  1 (it is out of scope), so the discoverability output advertises an error
  contract the tool does not yet produce. Per FR-015 this block is emitted as-is
  as a documented forward-declaration of the target contract, reconciled in the
  later error-envelope phase.
- **`hujson` stays in the graph**: removing `tailscale/hujson` is explicitly not
  part of this phase; the `Add` path still mutates the hujson AST directly
  (`add.go`) and the Renovate scanner parses with it (`security/renovate.go`), so
  the dependency remains and `go mod tidy` must keep it as a direct require.
- **Toolchain floor rises**: `ax-go@v0.2.0` declares `go 1.26.4`; adopting it
  raises this module's `go` directive from `1.25.8` to `1.26.x` (FR-004), which
  also raises the minimum Go for every `pkg/fleet` consumer (#152) — an accepted
  consequence of this phase.

## Requirements *(mandatory)*

### Functional Requirements

#### Constitution amendment (gate)

- **FR-001**: The constitution's §Third-Party Dependencies **Approved direct
  dependencies** list MUST add `github.com/rshade/ax-go` with the
  three-alternatives rationale: *stdlib* rejected (AX contracts — error
  envelopes, discoverability, idempotency, mode resolution — are bespoke, not in
  stdlib); *vendoring* rejected (the value is a shared, upstream-maintained
  contract with golden-pinned output shapes; vendoring forks the DNA); *delegate
  to `gh aw`/`gh`/`git`* N/A (this is the tool's own output/AX layer, not
  orchestratable work). The amendment SHOULD note that gh-aw-fleet consumes only
  ax-go's import-isolated thin-consumer packages (`config`, `schema`,
  `contract`), so the module's heavier OpenTelemetry / gRPC transitive deps stay
  out of this tool's build (keeping it consistent with the minimal-deps norm).
- **FR-002**: The constitution version MUST bump from v1.1.0 to v1.2.0, with the
  `Last Amended` footer date updated and a Sync Impact Report entry recording the
  amendment (mirroring the #73 hujson precedent).

#### Dependency adoption

- **FR-003**: `go.mod` MUST add `github.com/rshade/ax-go` pinned **exactly** at
  `v0.2.0`; `go.sum` MUST be updated; `go mod tidy` MUST produce no further diff;
  and the change MUST introduce exactly one new top-level (direct) require entry.
- **FR-003a**: gh-aw-fleet MUST import **only** ax-go's import-isolated
  thin-consumer packages (`github.com/rshade/ax-go/config`,
  `github.com/rshade/ax-go/schema`, and their transitive stdlib-only
  `…/contract`). It MUST NOT import the root `package ax`. As a result, ax-go's
  OpenTelemetry / gRPC / protobuf dependencies MUST NOT be compiled into the
  gh-aw-fleet binary and MUST NOT appear in gh-aw-fleet's `go.mod` require blocks.
- **FR-004**: The module's declared `go` directive MUST be raised to satisfy
  `ax-go@v0.2.0`'s `go 1.26.4` requirement (i.e. to `go 1.26.x`), as part of this
  phase. The local development gate already runs Go 1.26.4, so the tool already
  builds under the new floor. This bump raises the minimum Go version for every
  downstream `pkg/fleet` consumer (notably the external control plane, #152), and
  that consequence is accepted; any CLAUDE.md / AGENTS.md notes that lean on the
  prior `go 1.25.8` compatibility claim MUST be updated to match.

#### Config IO swap (`internal/fleet/load.go`)

- **FR-005**: The config **read** path (`loadConfigFile` and the `LoadTemplates`
  read) MUST parse via `config.Parse` / `config.ParseFile` (from
  `github.com/rshade/ax-go/config`) instead of `hujson.Standardize` +
  `json.Unmarshal`, preserving JWCC tolerance (`//` line comments, `/* */` block
  comments, and trailing commas) in all config files.
- **FR-006**: The comment-preserving **write** path (`SaveTemplates`'s RFC 6902
  patch) MUST apply via `config.Patch` / `config.PatchFile` (from
  `github.com/rshade/ax-go/config`) instead of `hujson.Parse` + `Value.Patch` +
  `Pack`, replacing only `/version`, `/fetched_at`, and `/sources` while
  preserving operator-authored comments on `/evaluations`.
- **FR-007**: The on-disk write policy MUST remain consistent with today's
  behavior: an atomic write (temp + rename), a guaranteed trailing newline, and
  no regression to any committed config file's byte shape beyond ax's documented
  canonical-whitespace normalization.
- **FR-008**: The file-probe behavior MUST remain unchanged and MUST stay in
  `probeConfigPath` (not move into ax-go): `<base>.hujson` is preferred over
  `<base>.json`; both present is rejected as ambiguous; neither present returns
  the `.json` path for synthesis.
- **FR-009**: The `SaveTemplates` fallback MUST be preserved: when the
  comment-preserving patch cannot be applied, the tool logs a fallback event at
  `warn` and rewrites the file from scratch via the standard-JSON writer.
- **FR-010**: All existing `internal/fleet` load/save tests MUST pass with **no
  edits to their assertions** (comment preservation, `.hujson`/`.json` probe
  order, both-present rejection, billing-metadata round-trip).
- **FR-011**: `github.com/tailscale/hujson` MUST remain a **direct** dependency.
  Multiple call sites still use it directly — the `Add` path mutates the hujson
  AST (`internal/fleet/add.go`) and the Renovate scanner parses JWCC configs with
  it (`internal/fleet/security/renovate.go`) — so dropping `load.go`'s import does
  not remove the module. It MUST NOT be removed in this phase; removing it later
  (once ax-go owns all config IO) is a MAJOR-bump amendment.

#### `__schema` discoverability (net-new, additive)

- **FR-012**: A reserved `__schema` command MUST be wired into the root command
  tree (built in `cmd/schema.go` on `github.com/rshade/ax-go/schema`'s
  `BuildSchema`/`BuildMCPSchema`, mirroring `schema.NewSchemaCommand` while
  augmenting the MCP output with CLI positional arguments) and MUST emit a
  machine-readable JSON description of the command tree on **stdout**.
- **FR-013**: The `__schema` output MUST reflect the full current command
  surface: the root command with its persistent flags (`--dir`, `--log-level`,
  `--log-format`, `--output`) and all eight subcommands (`list`, `status`,
  `add`, `template`, `deploy`, `sync`, `upgrade`, `consumption`) with their
  flags, plus the tool version.
- **FR-014**: `__schema` MUST support both the native ax schema and the
  MCP-adapter schema, selected via `--as ax|mcp`, defaulting to `ax`.
- **FR-015**: `__schema` MUST emit ax-go's standard `error_envelope` block
  **as-is** (unmodified from `schema.BuildSchema`). Because phase 1 does not yet
  adopt the ax error envelope, this block is a **forward-declaration** of the
  target error contract, not a description of what the tool emits today — the two
  reconcile when the error-envelope convergence phase lands (see Out of Scope).
  The tool MUST NOT fork ax's schema output to suppress the block (doing so would
  diverge from the shared contract this adoption exists to stand on). This known,
  temporary mismatch MUST be documented alongside the adoption notes (FR-020) so a
  consuming agent is not misled into parsing today's errors as ax envelopes.
- **FR-016**: `__schema` MUST be **additive only**: no existing command's stdout,
  stderr, exit code, or `--output json` envelope changes, and `cmd.SchemaVersion`
  MUST NOT bump.

#### Invariants & docs

- **FR-017**: The stream-separation invariant MUST hold: `__schema` writes its
  JSON to stdout only; logs/warnings stay on stderr.
- **FR-018**: `fleet.SchemaVersion` (on-disk config format) MUST NOT change.
- **FR-019**: Any newly-exported identifier introduced by the wiring MUST carry a
  godoc comment per the repo's self-documentation convention; `make lint`
  (revive/staticcheck) MUST report zero exported-symbol violations.
- **FR-020**: `CLAUDE.md` and `AGENTS.md` MUST record `ax-go` in the Active
  Technologies / dependency notes and reference this phase-1 adoption and its
  enumerated follow-up phases.

### Key Artifacts *(what this feature produces or changes)*

- **Constitution amendment**: `.specify/memory/constitution.md` at v1.2.0 with
  `ax-go` on the approved-direct-dependencies list and a Sync Impact Report
  entry.
- **`ax-go` dependency**: a single new direct `require` for
  `github.com/rshade/ax-go v0.2.0`, consumed only through its import-isolated
  `config` / `schema` / `contract` packages.
- **`__schema` discoverability output**: a JSON document describing the command
  tree (root + flags + subcommands + version), available in a native ax shape and
  an MCP-adapter shape — the one net-new data artifact, consumed by agents and the
  future control plane rather than by humans.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: `make ci` (fmt-check, vet, lint, full test suite) is green with no
  new failures and no new lint suppressions.
- **SC-002**: 100% of the existing `internal/fleet` load/save tests pass with
  zero edits to their assertions.
- **SC-003**: `go mod tidy` leaves no diff; `go.mod`'s direct-require block gains
  exactly one entry, `github.com/rshade/ax-go v0.2.0`, and `tailscale/hujson`
  remains a direct require.
- **SC-003a**: `go list -deps ./...` for the gh-aw-fleet build reaches **no**
  `go.opentelemetry.io/…`, `google.golang.org/grpc`, or
  `google.golang.org/protobuf` package, and none of those modules appear in
  gh-aw-fleet's `go.mod` (proving the import-isolation of FR-003a holds).
- **SC-004**: `gh-aw-fleet __schema` emits valid JSON enumerating all eight
  current subcommands and the tool version, verified by an automated test; a diff
  of every other command's stdout/stderr/exit-code against `main` shows no
  change.
- **SC-005**: The constitution reads **v1.2.0**, lists `ax-go` with the
  three-alternatives rationale, and carries a Sync Impact Report entry for the
  amendment.
- **SC-006**: Neither `cmd.SchemaVersion` nor `fleet.SchemaVersion` changes
  value.
- **SC-007**: A config file with `//` / `/* */` comments and trailing commas
  still loads, and a `SaveTemplates` run still preserves an operator comment on a
  `/evaluations` entry — config-IO behavior parity demonstrated end to end.

## Assumptions

- **Pin**: `ax-go@v0.2.0` is the adopted pin — the first release to ship the
  import-isolated thin-consumer packages (#78/#79), advanced from the issue's
  original `v0.1.0` precisely to get that isolation. A later ax-go MINOR bump is
  treated as potentially breaking and is not auto-adopted.
- **Primitive parity (verified against ax-go `v0.2.0` source)**: `config.Parse`
  standardizes hujson then unmarshals (read), and `config.Patch` applies an
  RFC 6902 patch while preserving comments (write) — the same semantics
  `internal/fleet/load.go` relies on today. The probe / merge / version-check
  logic stays in `internal/fleet`; the `config` package owns only the parse and
  patch primitives.
- **Read-path cap**: the `config` package's default 1 MiB read cap
  (`config.DefaultMaxBytes`) is accepted as-is; real fleet configs are far
  smaller, so no `config.WithMaxBytes` override is wired in phase 1. The new
  `config_too_large` failure mode is documented but not expected to trigger.
- **Write-path file mechanics**: the implementation will preserve the project's
  atomic-write + trailing-newline policy (using the reader→bytes `config.Patch`
  with the existing `atomicWrite`, or the mode-preserving `config.PatchFile`);
  the exact choice is a plan-time detail, and both preserve comments.
- **`__schema` visibility**: the `__schema` command is registered under its
  reserved double-underscore name (machine-facing, kept out of the human `--help`
  command listing) and exposes both `ax` and `mcp` formats, since the MCP variant
  is free and directly serves the agent/control-plane goal.
- **Scope of the swap**: phase 1 swaps the config read path (`loadConfigFile`,
  `LoadTemplates`) and the `SaveTemplates` RFC 6902 patch write path. The `Add`
  AST-append write, `SaveLocalConfig`, and `writeJSON` fallback paths are not in
  scope (the first keeps `hujson` direct; the latter two are already
  standard-JSON writers).
- **Toolchain**: the local development gate already runs Go 1.26.4 and
  golangci-lint v2.12.2, satisfying ax-go's `go 1.26.4` build requirement. Per
  FR-004 the module's declared `go` directive is raised to `1.26.x` in this
  phase; the consequence (every `pkg/fleet` consumer moves to 1.26.x) is
  accepted.
- **No envelope/logger/error convergence**: phase 1 leaves `cmd/output.go`,
  `internal/log`, and `fleet.Diagnostic` / `CollectHints` untouched; those
  convergences are deferred (see Out of Scope).

## Out of Scope *(→ follow-up phases)*

- **Error-envelope convergence**: mapping `fleet.Diagnostic` + `CollectHints`
  onto `ax.Error` / `NewError` (`WithActionableFix` / `WithSuggestions` /
  `WithErrorContext` / `WithErrorExitCode`), and the decision whether to preserve
  the existing envelope shape or bump `cmd.SchemaVersion`.
- **`--output json` envelope** onto ax's payload contract / `ax.Execute`.
- **`ax.NewLogger`** replacing zerolog (and the eventual MAJOR-bump removal of
  direct zerolog).
- **`ax.WithIdempotencyKey` / `ax.WithMode` / `ax.WithDryRun`** context plumbing
  for `deploy` / `sync` / `upgrade` (directly supports headless invocation,
  #145).
- **Removing direct `tailscale/hujson`** once ax-go owns all config IO
  (MAJOR-bump amendment).

## Dependencies & References

- **Issue**: #156. **Precedent**: #73 (hujson constitution-amendment).
- **Unblocks**: #145 (harden deploy/sync/upgrade for headless invocation).
  **Supports**: EPIC #146, public `pkg/` API #141 (and its first slice
  #148/#152).
- **Foundation dependency**: `github.com/rshade/ax-go` @ `v0.2.0` — the shared AX
  foundation across rshade Go CLIs; consumed via its import-isolated `config` /
  `schema` / `contract` packages (ax-go #78/#79).
- **Invariant preserved**: the wire contract (`cmd.SchemaVersion`) and on-disk
  format (`fleet.SchemaVersion`) are frozen for this phase.
