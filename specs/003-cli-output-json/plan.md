# Implementation Plan: CLI JSON Output Mode

**Branch**: `003-cli-output-json` | **Date**: 2026-04-21 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/003-cli-output-json/spec.md`

## Summary

Add a second serializer to four existing commands (`list`, `deploy`, `sync`, `upgrade`) that emits the commands' already-computed result structs as a versioned JSON envelope on stdout. A new persistent root flag `-o, --output {text|json}` (default `text`) selects the serializer. Text mode is byte-identical to current behavior. JSON mode emits one envelope on stdout (`{schema_version: 1, command, repo, apply, result, warnings, hints}`), routes all breadcrumbs and diagnostics to stderr via the existing zerolog layer (already landed in #34), and in bulk mode (`upgrade --all`) emits newline-delimited envelopes — one per repo, streamed as each completes. Pre-result failures still emit an envelope with `result: null`; exit codes mirror text mode (Q2/Q3 clarifications).

Implementation is concentrated in `cmd/output.go` (new, ~120 LOC — envelope writer, `Diagnostic` type, mode validator, ND-stream helper), a small `ListResult` type factored out of `cmd/list.go` into `internal/fleet`, `json:"..."` tags added to `DeployResult`/`WorkflowOutcome`/`SyncResult`/`UpgradeResult`, and one new `CollectHintDiagnostics` helper in `internal/fleet/diagnostics.go` that returns structured `Diagnostic` entries instead of raw hint strings. Each of the four subcommands gains a single branch: after computing its result, it either calls the existing text-mode printer or `writeEnvelope(...)`. Warnings come from the same sites that today log via `zlog.Warn()` — the commands collect a parallel `[]Diagnostic` slice at those sites for embedding in the envelope.

## Technical Context

**Language/Version**: Go 1.25.8 (from `go.mod`).
**Primary Dependencies**: `encoding/json` (stdlib, new usage site); `github.com/spf13/cobra` v1.10.2 (existing); `github.com/rs/zerolog` v1.35.1 (existing, landed in #34). No new third-party dependencies — constitution Principle I.
**Storage**: N/A (no persistent state; envelope writes are transient to stdout).
**Testing**: Go standard `testing` package. New tests live in `cmd/output_test.go` (envelope shape, empty-array-not-null, schema_version pin, flag validator, NDJSON per-line structure, pre-result failure envelope). Existing `cmd/root_logging_test.go` and `internal/fleet/*_test.go` continue to pass unchanged.
**Target Platform**: Cross-platform CLI (Linux, macOS, Windows) — same surface `gh-aw` extension already supports.
**Project Type**: Single-project CLI (thin orchestrator around `gh aw`, `gh`, `git`).
**Performance Goals**: JSON serialization overhead MUST be imperceptible (sub-millisecond per envelope; stdlib `json.Marshal` against small result structs is trivially fast relative to the network and subprocess work each command already does). No command regresses past constitution's 5-minute ceiling. NDJSON lines MUST flush as each repo completes (bufio flush after each `Encode` call) to enable true streaming consumption.
**Constraints**:
- **Text-mode byte-identity** (spec FR-014, SC-003): any run that omits `--output` or passes `--output text` MUST produce stdout byte-equal to pre-feature baseline. This is the invariant that makes the feature safe to land without a migration window.
- **Single JSON object per command invocation** (spec SC-002): `jq -e . < <(cmd -o json)` succeeds for every invocation, including non-zero exits (covered by FR-020 pre-result failure envelope).
- **Empty arrays, never null** (spec FR-009): enforced via `json:"...,omitempty"` NOT used on slice fields — slices stay un-tagged-omitempty so `nil` slices marshal as `null`; the fix is to initialize slice fields to non-nil empty slices before JSON marshal. A helper `ensureSlicesInitialized(result any) any` handles this at the envelope-writer boundary.
- **Snake_case JSON keys** (spec FR-008): applied via explicit `json:"snake_case_name"` tags; no reliance on auto-lowercasing.
- **No new third-party dependencies** (constitution I; spec FR-018): verified by `go mod graph` diff = 0 new edges after implementation.
- **gpg signing, three-turn mutation pattern, git-from-Bash deny** (constitution III + Declarative Reconcile Invariants): untouched — this feature only changes serialization, not mutation flow.
**Scale/Scope**: 1 new file (`cmd/output.go`). 1 new file (`internal/fleet/list_result.go` — houses `ListResult` / `ListRow`; a few dozen LOC moved from `cmd/list.go`). Small edits to 4 subcommand files (`cmd/list.go`, `cmd/deploy.go`, `cmd/sync.go`, `cmd/upgrade.go`). Small edits to 3 result-struct files (`internal/fleet/deploy.go`, `sync.go`, `upgrade.go`) to add `json:"..."` tags and ensure empty-slice semantics. Small edit to `internal/fleet/diagnostics.go` to add `Code` field to `Hint` and expose `CollectHintDiagnostics`. One edit to `cmd/root.go` to register `--output` with validator. One new test file (`cmd/output_test.go`). ~400 new LOC total; no file crosses 300 LOC.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md` version 1.0.0.

### I. Thin-Orchestrator Code Quality — **PASS**

- Zero new dependencies. All serialization uses stdlib `encoding/json`. Verified by spec FR-018, SC-008.
- The feature does not duplicate any upstream tool behavior: `gh aw`, `gh`, `git` remain the mutation surface; this change only re-serializes results that are already computed during dry-run / apply.
- `go build ./...` and `go vet ./...` MUST stay clean. The `make ci` gate (fmt-check, vet, lint, test) MUST pass locally before the change is reported done (user feedback memory `feedback_local_gate.md`).
- File-size guidance (~300 LOC): `cmd/output.go` is projected at ~120 LOC; `cmd/list.go` stays under its current 59 LOC after the extraction; no existing file crosses the threshold as a result of this change.
- Comments explain WHY only — specifically (a) why we initialize slices before marshal (to satisfy FR-009 `[]`-not-`null`), (b) why `upgrade --all` emits NDJSON and not an aggregate envelope (Q1 clarification; single-parser contract for consumers), (c) why we embed warnings inline in the envelope AND on stderr (FR-011 contract; satisfies both scripted and human consumers simultaneously). WHAT comments are avoided.

### II. Testing Standards (Build-Green + Real-World Dry-Run) — **PASS**

- `go build ./...` and `go vet ./...` stay green trivially — changes are additive.
- Dry-run surface (`deploy` / `sync` / `upgrade` without `--apply`) is unchanged; the JSON branch runs off the same `*DeployResult` / `*SyncResult` / `*UpgradeResult` the dry-run already produces. `fleet.CollectHints` continues to run inside dry-runs; the new `CollectHintDiagnostics` is a parallel call-site that produces structured versions of the same hints for envelope embedding — text-mode path still sees the same `[]string` return from `CollectHints`.
- Skills (`skills/fleet-deploy`, `skills/fleet-eval-templates`, `skills/fleet-upgrade-review`, `skills/fleet-onboard-repo`): Each gains a one-line note under its "Debugging" or "Extended usage" section that `-o json` is available for programmatic consumption. No skill-logic change; three-turn pattern untouched.
- New unit tests in `cmd/output_test.go` cover: (a) envelope top-level key set pinning (FR-004, FR-005, SC-005); (b) empty slices marshal as `[]` not `null` for every result type (FR-009, SC-004); (c) `--output` flag validator accepts `text`/`json`, rejects `yaml`/`JSON`/empty (FR-001, FR-002); (d) `audit_json` embeds as nested JSON object when the underlying bytes are valid JSON (FR-016); (e) pre-result failure emits an envelope with `result: null` (FR-020); (f) NDJSON mode produces exactly N lines for N repos, each a valid standalone envelope (FR-019, User Story 3 acceptance scenario 5).
- Integration check (manual, documented in `quickstart.md`): run `go run . list -o json | jq -e .schema_version` and confirm output is `1`; run `go run . deploy rshade/gh-aw-fleet -o json 2>/dev/null | jq -e .command` and confirm output is `"deploy"`.

### III. User Experience Consistency (Three-Turn Mutation Pattern) — **PASS**

- Three-turn pattern (dry-run → approval → apply) is untouched. The `--output` flag is orthogonal to `--apply`. `-o json` without `--apply` is a dry-run in JSON form; with `--apply` it is an actual apply in JSON form. The interactive go-ahead requirement from constitution III still applies to `--apply` in either mode.
- Conventional Commits (`ci(workflows)` scope; ≤72-char subject; no trailing period): unchanged; this feature's own commit lands on the tool's `main` branch, not in any workflow repo.
- Diagnostic hints via `fleet.CollectHints`: extended, not replaced. Text mode still prints `hint:` lines on failure; JSON mode additionally embeds structured equivalents in `hints[]` and emits them on stderr via zerolog. **UX shift (documented in CHANGELOG.md)**: In JSON mode, the tabwriter `hint:` lines do not appear on stdout — consumers parse `.hints[]` from the envelope. Operators reading human output (text mode, default) see no change.
- Scratch clones at `/tmp/gh-aw-fleet-*` after `--apply` failure: preserved identically. The envelope's `result.clone_dir` carries the path so agents can resume via `--work-dir`.

### IV. Performance Requirements — **PASS**

- Parallelism / catalog-cache surface untouched.
- `encoding/json.Marshal` on a result struct of ~10 fields with ~20–50 total nested entries is O(μs) — invisible against network and subprocess overhead.
- `upgrade --all` NDJSON emission requires per-repo flush. This doesn't change parallelism (the command remains serial today), but makes streaming consumption practical once parallelization lands. **Note**: If `upgrade --all` is parallelized in a future feature, the NDJSON emission helper MUST serialize writes behind a mutex to prevent interleaved partial lines. The Phase 1 contract (`contracts/ndjson.md`) documents this requirement explicitly so it's not forgotten.
- No command regresses past the 5-minute ceiling.

### Declarative Reconcile Invariants — **PASS**

- `fleet.json`/`fleet.local.json` source-of-truth direction untouched.
- `fleet.local.json` MUST NOT be committed — unchanged.
- gpg signing bypass forbidden — unchanged (this feature touches serialization, not git).
- `git add`/`git commit`/`git push` from Bash denied — unchanged (feature does not invoke git at all).
- `github/gh-aw` source pinning rule — unchanged.

### Gate verdict

All gates pass on initial check. **Complexity Tracking table is intentionally empty — no violations to justify.**

## Project Structure

### Documentation (this feature)

```text
specs/003-cli-output-json/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── cli-flags.md     # --output flag contract
│   ├── envelope.md      # JSON envelope shape and invariants
│   └── ndjson.md        # NDJSON stream contract for upgrade --all
├── checklists/
│   └── requirements.md  # Spec quality checklist (from /speckit.specify)
├── spec.md              # Feature specification
└── tasks.md             # (Phase 2 — created by /speckit.tasks, NOT by this command)
```

### Source Code (repository root)

```text
cmd/
├── root.go              # +PersistentFlag "output", +validateOutputMode()
├── output.go            # NEW: Diagnostic, Envelope, writeEnvelope, writeNDJSON, outputMode()
├── output_test.go       # NEW: envelope shape, flag validator, empty-[], NDJSON, pre-result failure
├── list.go              # branch on outputMode(); text-mode path unchanged; json-mode path calls writeEnvelope
├── deploy.go            # branch on outputMode(); collect []Diagnostic from existing warning sites
├── sync.go              # branch on outputMode(); collect []Diagnostic from existing drift-warn site
├── upgrade.go           # branch on outputMode(); NDJSON path for --all; audit_json nesting
├── hints.go             # unchanged (existing zerolog hint-emit helper)
├── add.go               # unchanged (not in scope)
├── template.go          # reject -o json with clear error (FR-013)
└── stubs.go             # unchanged

internal/fleet/
├── list_result.go       # NEW: ListResult, ListRow (moved from cmd/list.go inline), json tags
├── deploy.go            # +json:"..." tags on DeployResult, WorkflowOutcome; ensure-[] helpers
├── sync.go              # +json:"..." tags on SyncResult; ensure-[] helpers
├── upgrade.go           # +json:"..." tags on UpgradeResult
├── diagnostics.go       # +Code field on Hint; +CollectHintDiagnostics() []Diagnostic
├── fetch.go             # confirm AuditJSON json.RawMessage nests inline (no code change expected)
├── schema.go            # unchanged (reference style for json tags)
├── load.go              # unchanged
├── add.go               # unchanged
├── frontmatter.go       # unchanged
├── execlog.go           # unchanged
└── *_test.go            # unchanged (existing tests continue to pass)
```

**Structure Decision**: Single-project CLI layout (same as features 001 and 002). The `cmd/` package owns CLI wiring and serialization; `internal/fleet/` owns business types and their JSON contract. The `Diagnostic` type lives in `cmd/output.go` (not `internal/fleet/`) because it is a CLI-surface concern — it has no role in the core fleet business logic — and because importing `internal/fleet` from `cmd/` is the established direction (keeping it one-way avoids a cycle).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

_No violations to justify. All gates pass as designed. This section is intentionally left without entries._

## Phase 0: Research Summary

Full details in [research.md](research.md). Five research tasks:

- **R1**: Go `encoding/json` handling of `json.RawMessage` nested inside a parent struct (verify no double-encoding when `AuditJSON` is marshaled as a field of `UpgradeResult`). **Decision**: `json.RawMessage` is documented to embed verbatim bytes when present and valid; it marshals as `null` when the slice is nil and produces a marshal error when the bytes are malformed. Plan: ensure the bytes captured from `gh aw audit --json` are either valid JSON (the happy path) or the slice is nil (in which case the envelope field renders as `null`). Validation at capture time is already done implicitly by `gh aw audit` — if it emits invalid JSON, that's an upstream bug and we let the marshal error surface.
- **R2**: Empty-slice-vs-nil-slice JSON marshaling (how to guarantee `[]` not `null` without plumbing `omitempty` everywhere). **Decision**: Initialize slice fields to `[]T{}` (non-nil empty) before marshaling. Either in the business-logic layer (constructor-time) or via a small `ensureSlicesInitialized(*Result)` helper invoked from the envelope writer. Plan chooses the latter — keeps business logic untouched, confines the JSON concern to the JSON path.
- **R3**: Cobra persistent-flag validation pattern (how to reject `--output yaml` before subcommand RunE fires). **Decision**: Use a `PersistentPreRunE` on the root command that validates the `--output` value against a closed set and returns an error; cobra routes that error to its default stderr writer and exits non-zero without calling the subcommand. Matches the pattern already used by `--log-level` / `--log-format`.
- **R4**: NDJSON emission semantics (one JSON object per line, with flush semantics for streaming). **Decision**: Use `json.NewEncoder(stdout).Encode(envelope)` per repo — `Encoder.Encode` writes the JSON form followed by a newline and flushes the underlying writer if it's a bufio-wrapped stream. Per-repo flush is implicit in os.Stdout being unbuffered by default. Document the mutex requirement for future parallelization.
- **R5**: Zerolog stderr-duplication for warnings (how warnings land both in `warnings[]` and on stderr via the logger). **Decision**: At each warning site, the command builds a `Diagnostic{Code, Message, Fields}` struct, appends it to a local `[]Diagnostic` slice for later envelope embedding, and also calls `zlog.Warn().Fields(d.Fields).Msg(d.Message)` at the same site. The two emissions share a source (the `Diagnostic` struct); the zerolog call is unchanged in structure from pre-feature (warnings already emit to stderr via zerolog — #34 landed this). Net effect: one new line of code per warning site (`warnings = append(warnings, d)`).

## Phase 1: Design Artifacts

### data-model.md
Defines four entities: `Envelope` (the top-level JSON object), `Diagnostic` (shared shape for warnings and hints), `ListResult` / `ListRow` (new types), and the JSON-tag contract for `DeployResult`/`WorkflowOutcome`/`SyncResult`/`UpgradeResult`.

### contracts/cli-flags.md
The `--output` / `-o` flag surface: accepted values, default, rejection behavior, interaction with `--log-level` / `--log-format` / `--dir`, and the `template fetch` rejection (FR-013).

### contracts/envelope.md
The JSON envelope shape: top-level key set (pinned), per-key semantics (`schema_version`, `command`, `repo`, `apply`, `result`, `warnings`, `hints`), nullability rules (`result: null` on pre-result failure), and the snake_case key convention per result struct.

### contracts/ndjson.md
The NDJSON stream contract for `upgrade --all -o json`: one envelope per line, flush-per-repo, no trailing bare newline beyond per-line terminators, mutex-requirement note for future parallelization.

### quickstart.md
Operator-oriented walkthrough: install, invoke `-o json` on each of the four commands, pipe through `jq`, observe stderr routing, observe NDJSON stream for `upgrade --all`.

### Agent context update
Ran `.specify/scripts/bash/update-agent-context.sh claude` — appended "Active Technologies" entry covering the `--output` flag surface and the `internal/fleet.ListResult` / `Diagnostic` types to `CLAUDE.md`'s managed block.
