# Feature Specification: CLI JSON Output Mode

**Feature Branch**: `003-cli-output-json`
**Created**: 2026-04-21
**Status**: Draft
**Input**: GitHub issue #25 — `feat(cli): add --output json to list/deploy/sync/upgrade for LLM consumption`

## Clarifications

### Session 2026-04-21

- Q: How should `upgrade --all -o json` emit its output across multiple repos? → A: NDJSON stream — one complete JSON envelope per line, one per repo, preserving the single-repo envelope shape per line.
- Q: What envelope shape is emitted on stdout when a command fails before its internal result struct is built (e.g., config parse error, missing tool, repo not found)? → A: `result: null` with populated `warnings[]`/`hints[]`; envelope still carries `schema_version`, `command`, `repo`, `apply`; command exits non-zero.
- Q: What are the exit code semantics in JSON mode when warnings are present but the command structurally succeeded? → A: Text-mode-parity — JSON mode exit codes mirror text mode exactly for the same conditions; no new exit-code contract is introduced. Agents that want warning-level gating parse `warnings[]` from the envelope.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Structured `list` output for agentic consumption (Priority: P1)

An operator running `gh-aw-fleet list -o json` receives a single, valid JSON object on stdout describing every repo in the fleet (repo name, profiles, engine, workflow slugs, exclusions, extras) along with which config file was loaded. The human-readable tabwriter table is fully suppressed in JSON mode; any diagnostics go to stderr.

**Why this priority**: `list` is read-only, side-effect-free, and the most frequent entry point for any automation that wants a snapshot of fleet state. It has the highest value-to-risk ratio, unblocks LLM agents and shell scripts immediately, and validates the envelope contract before any write-path command adopts it.

**Independent Test**: Run `gh-aw-fleet list -o json | jq -e .schema_version` and `... | jq '.result.repos[0]'`; both must succeed and return expected shape without any regex scraping.

**Acceptance Scenarios**:

1. **Given** a valid `fleet.local.json` with multiple repos, **When** the operator runs `gh-aw-fleet list -o json`, **Then** stdout contains exactly one JSON object with `schema_version: 1`, `command: "list"`, `result.loaded_from`, and `result.repos[]` populated with one entry per repo.
2. **Given** the JSON mode is active, **When** the command would normally print the `(loaded fleet.local.json)` breadcrumb, **Then** that breadcrumb is emitted on stderr (never stdout), leaving stdout as pure JSON consumable by `jq -e`.
3. **Given** the `list` command is invoked without `--output`, **When** the operator runs `gh-aw-fleet list`, **Then** the tabwriter output is byte-identical to the behavior before this feature landed.

---

### User Story 2 - Structured `deploy` output with embedded warnings (Priority: P2)

An operator pipes `gh-aw-fleet deploy rshade/gh-aw-fleet -o json` (dry-run) into their agentic pipeline. The envelope carries the full `DeployResult` (added, skipped, failed, PR URL if applicable, missing-secret state) plus a `warnings[]` array populated with any `⚠ WARNING: Actions secret ...` diagnostics. The same warnings are also emitted on stderr via the structured logger, so humans tailing the run still see them.

**Why this priority**: `deploy` is the highest-value write-path command. Agents need to programmatically inspect which workflows were added vs. skipped vs. failed, pick up PR URLs for downstream tracking, and surface missing-secret conditions without scraping text. Embedding warnings in the envelope makes the JSON output self-sufficient (no need to also parse stderr).

**Independent Test**: Run a dry-run `deploy` that would trigger a missing-secret warning; assert the envelope contains a matching entry in `warnings[]` with a `code` (e.g., `missing_secret`) and a populated `fields` object, and that the same warning appears on stderr.

**Acceptance Scenarios**:

1. **Given** a repo that would have workflows added, skipped, and failed in a dry-run, **When** the operator runs `... deploy <repo> -o json`, **Then** `result.added`, `result.skipped`, and `result.failed` are present as JSON arrays (never `null`, never omitted), each containing structured entries with `name`, `spec`, `reason`, and optional `error`.
2. **Given** a deploy that detects a missing Actions secret, **When** the command completes, **Then** `warnings[]` includes an entry with a stable `code` identifying the class of warning and enough structured context (e.g., secret name) to drive downstream automation without regex.
3. **Given** JSON mode is active and a failure occurs mid-pipeline, **When** hints from the diagnostics layer fire (e.g., unknown-property, HTTP 404), **Then** they appear in `hints[]` inside the envelope and also on stderr, and the command still exits with a non-zero status.
4. **Given** `deploy` is run without `--output` or with `--output text`, **When** it executes, **Then** the existing tabwriter output and ordering is preserved exactly (no drift, no extra blank lines, no moved text).

---

### User Story 3 - Structured `sync` and `upgrade` output (Priority: P3)

An operator running `gh-aw-fleet sync <repo> -o json` or `gh-aw-fleet upgrade <repo> -o json` receives envelopes carrying the full `SyncResult` / `UpgradeResult`. For `sync`: missing/drift/expected workflow lists, pruned entries, and an optional nested deploy preview. For `upgrade`: changed files, merge conflict list, PR URL, and the raw `gh aw` audit JSON embedded inline as a JSON sub-object (not a stringified blob).

**Why this priority**: These commands are valuable for complete fleet coverage, but lower-frequency than `list`/`deploy` and more complex to serialize (sync can carry a nested dry-run deploy result; upgrade embeds a `json.RawMessage` audit blob). Deferring them to P3 lets the envelope contract settle on the simpler two commands first.

**Independent Test**: Run `sync` against a repo with known drift; assert `result.drift[]` is a structured array. Run `upgrade` that produces a PR; assert `result.pr_url` is a string and `result.audit_json` parses as a nested object under `jq '.result.audit_json'` (no escape-encoded string).

**Acceptance Scenarios**:

1. **Given** a `sync` run where the repo is fully in sync, **When** the operator runs `... sync <repo> -o json`, **Then** `result.missing`, `result.drift`, `result.expected`, and `result.pruned` are all `[]` (not `null`, not absent), and `result.deploy` is `null`.
2. **Given** a `sync` with drift that triggers a dry-run deploy preview, **When** the envelope is emitted, **Then** `result.deploy` is a nested object carrying the full embedded `DeployResult`, using the same JSON key conventions.
3. **Given** an `upgrade` run that produces merge conflicts, **When** the envelope is emitted, **Then** `result.conflicts[]` is populated, `result.upgrade_ok` is `false`, and the raw `gh aw audit` JSON is embedded inline under `result.audit_json` as a nested object, preserving every field that `gh aw audit --json` produced.
4. **Given** an `upgrade` with no changes, **When** the envelope is emitted, **Then** `result.no_changes` is `true`, `result.changed_files` is `[]`, and `result.pr_url` is an empty string or omitted consistently.
5. **Given** `upgrade --all -o json` across N repos, **When** the command runs, **Then** stdout contains exactly N lines, each line a complete, single-repo JSON envelope (NDJSON) with its own `repo`, `result`, `warnings`, and `hints`; `jq -c` consumers can process each line independently as it streams.

---

### Edge Cases

- **Invalid flag value**: `gh-aw-fleet list -o yaml` must error with a clear message (e.g., `unsupported output mode "yaml"; expected "text" or "json"`) and exit non-zero, without emitting a partial envelope.
- **Unsupported subcommand**: `gh-aw-fleet template fetch -o json` must error (not silently fall back to text), since `template fetch` is explicitly out of scope and would confuse downstream consumers expecting the envelope.
- **Stderr contamination**: Any diagnostic, breadcrumb, or warning printed to stdout in text mode must be routed to stderr in JSON mode so that `stdout` is pure, single-object JSON parseable by `jq -e`.
- **Empty collections**: Every slice field in every result struct must marshal as `[]` when empty, never as `null`, so downstream consumers can iterate without nil-guards.
- **Nested JSON fidelity**: The `upgrade` command's `audit_json` field (a `json.RawMessage`) must nest as a native JSON object inside the envelope, not as a string containing escaped JSON.
- **Interrupted runs**: If a command panics or is killed mid-output, downstream consumers must either get a complete envelope or nothing on stdout — not a truncated JSON fragment. (The command should compute the full result before beginning stdout emission.)
- **Pre-result failure**: If the command fails before it can build a result struct (config parse error, missing tool, repo not in fleet, profile resolution failure), JSON mode still emits one complete envelope on stdout with `result: null` and the cause recorded in `warnings[]` or `hints[]` — never an empty stdout. Exit code is non-zero.
- **Dependency ordering**: If issue #24 (zerolog) has not landed yet, the stderr warning emission path must still work (via a stub helper), so this feature is not blocked on #24's merge order.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The root command MUST accept a persistent `-o` / `--output` flag whose values are restricted to `text` (default) and `json`.
- **FR-002**: The flag validator MUST reject any other value (including `yaml`, `JSON`, empty string) with a clear error message naming the accepted values, and MUST cause the process to exit non-zero before any subcommand logic runs.
- **FR-003**: When `--output json` is set, the `list`, `deploy`, `sync`, and `upgrade` commands MUST emit a single JSON object to stdout and suppress all existing tabwriter output for that command.
- **FR-004**: The JSON envelope MUST contain, at minimum, the top-level keys `schema_version`, `command`, `repo`, `apply`, `result`, `warnings`, and `hints`, in any order.
- **FR-005**: `schema_version` MUST be the integer `1` for the initial release and MUST be bumped (monotonically) on any breaking change to envelope or result field shapes.
- **FR-006**: `warnings` and `hints` MUST always be JSON arrays (never `null`, never absent), each element of which MUST include at minimum a `code` (string), `message` (string), and optional `fields` (object) for structured context.
- **FR-007**: The `result` field MUST be the typed struct already produced internally by the command (`DeployResult`, `SyncResult`, `UpgradeResult`, or a new `ListResult`), serialized with explicit, stable JSON tags.
- **FR-008**: JSON keys in every result struct MUST use `snake_case` (e.g., `clone_dir`, `pr_url`, `missing_secret`, `audit_json`, `loaded_from`) for consistency with the rest of the project's JSON surface.
- **FR-009**: All slice/array fields in result structs MUST marshal as empty arrays `[]` (never `null`) when empty.
- **FR-010**: Any diagnostic, breadcrumb, or warning that appears on stdout in text mode MUST be routed to stderr in JSON mode, so stdout remains a single valid JSON object parseable by `jq -e`.
- **FR-011**: In JSON mode, every warning embedded in the envelope's `warnings[]` MUST also be emitted on stderr via the structured logger (zerolog from issue #24, or a stub helper if #24 has not landed), so humans monitoring a run still see them live.
- **FR-012**: In JSON mode, every diagnostic hint from the existing `CollectHints` layer MUST be embedded in `hints[]` in the envelope and MUST also appear on stderr via the structured logger.
- **FR-013**: Subcommands outside the four primary commands (notably `template fetch`) MUST reject `--output json` with a clear error rather than silently falling back to text mode.
- **FR-014**: Text-mode output (when `--output` is omitted or set to `text`) MUST remain byte-identical to the pre-feature behavior for all existing commands, including ordering, spacing, and blank lines.
- **FR-015**: The envelope MUST be emitted as compact single-line JSON (no pretty-printing); consumers wanting formatted output can pipe through `jq`.
- **FR-016**: The `upgrade` command's `audit_json` field MUST nest as a native JSON object inside the envelope when the underlying `gh aw audit --json` output is present, not as a string containing escape-encoded JSON.
- **FR-017**: The new `ListResult` type MUST be defined as part of this feature with fields `loaded_from` (string) and `repos` (array of rows containing `repo`, `profiles`, `engine`, `workflows`, `excluded`, `extra`), matching the data the current tabwriter `list` path already computes.
- **FR-018**: No new dependencies MAY be introduced; JSON serialization MUST use only the standard library `encoding/json` package.
- **FR-019**: When `upgrade --all` is invoked with `--output json`, the command MUST emit newline-delimited JSON (NDJSON) on stdout — one complete single-repo envelope per line, one per repo — preserving the single-repo envelope shape (`schema_version`, `command`, `repo`, `apply`, `result`, `warnings`, `hints`) on each line. Envelopes MUST be flushed as each repo completes to enable streaming consumption.
- **FR-020**: When a command fails before its internal result struct can be built (e.g., config parse error, missing `gh aw` binary, target repo not found in fleet, profile resolution failure), JSON mode MUST still emit exactly one complete envelope on stdout with `result: null`, the failure described in `warnings[]` or `hints[]` as appropriate, `schema_version`, `command`, `repo` (may be empty if not yet determined), and `apply` populated; the process MUST exit non-zero.
- **FR-021**: Exit codes in JSON mode MUST mirror text-mode exit codes for every condition (warnings, structural success, pre-result failure, `--apply` mid-pipeline failure, flag validation). JSON mode MUST NOT introduce a new exit-code contract. Agents requiring warning-level gating MUST parse `warnings[]` from the envelope rather than gate on exit code alone.

### Key Entities *(data)*

- **JSON Envelope**: The top-level stdout object. Fields: `schema_version` (int), `command` (string — one of `list`, `deploy`, `sync`, `upgrade`), `repo` (string — may be empty for `list`), `apply` (bool — false for dry-runs and for `list`), `result` (command-specific struct), `warnings` (Diagnostic array), `hints` (Diagnostic array).
- **Diagnostic**: Shared shape for warnings and hints. Fields: `code` (string — stable machine-parseable identifier like `missing_secret`, `unknown_property`), `message` (string — human-readable text), `fields` (object — optional structured context such as `{secret: "ANTHROPIC_API_KEY"}`).
- **ListResult**: New struct introduced by this feature. Fields: `loaded_from` (string — which config file was loaded, e.g., `fleet.local.json`), `repos` (array of ListRow).
- **ListRow**: A single repo entry in `ListResult.repos`. Fields: `repo` (string), `profiles` (string array), `engine` (string), `workflows` (string array), `excluded` (string array), `extra` (string array — workflows present on the remote but not in the profile).
- **DeployResult**: Existing struct, newly JSON-tagged. Fields include `repo`, `clone_dir`, `added`, `skipped`, `failed`, `init_was_run`, `branch_pushed`, `pr_url`, `missing_secret`, `secret_key_url`.
- **WorkflowOutcome**: Existing struct used inside deploy/sync arrays. Fields: `name`, `spec`, `reason`, `error`.
- **SyncResult**: Existing struct, newly JSON-tagged. Fields include `repo`, `clone_dir`, `missing`, `drift`, `expected`, `deploy` (nullable nested DeployResult), `pruned`, `deploy_preflight`.
- **UpgradeResult**: Existing struct, newly JSON-tagged. Fields include `repo`, `clone_dir`, `upgrade_ok`, `update_ok`, `changed_files`, `conflicts`, `no_changes`, `branch_pushed`, `pr_url`, `audit_json` (nested JSON object), `output_log`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A downstream consumer can extract any data point previously visible in the `list` / `deploy` / `sync` / `upgrade` tabwriter output using a single `jq` expression, with zero regex parsing of text — verifiable by round-tripping a known fleet state through `jq` and comparing against the text-mode output for equivalence.
- **SC-002**: `stdout` in JSON mode is a single valid JSON object for 100% of runs (including failure paths); `jq -e . < <(gh-aw-fleet <cmd> -o json)` succeeds even when the underlying command exits non-zero.
- **SC-003**: Text-mode output is byte-identical to pre-feature behavior across the full regression test suite (zero diffs when `--output` is omitted or set to `text`).
- **SC-004**: Every slice field in every result struct round-trips as `[]` (not `null`) when empty, verified by a unit test fixture for each command.
- **SC-005**: The envelope's top-level key set (`schema_version`, `command`, `repo`, `apply`, `result`, `warnings`, `hints`) is pinned by a regression test that fails on accidental key addition, removal, or rename.
- **SC-006**: A user running `gh-aw-fleet deploy <repo> -o json 2>/dev/null | jq .result.added[].name` gets the list of added workflow names with no stderr noise leaking into stdout.
- **SC-007**: Agent pipelines consuming fleet state reduce their parsing code complexity measurably (qualitative: single `jq` expression replaces multi-step text scraping), enabling cleaner integration with LLM context windows.
- **SC-008**: The feature adds zero new third-party dependencies to `go.mod` — confirmable by a `go.sum` diff.
- **SC-009**: The full local gate (`make ci` — `fmt-check`, `vet`, `lint`, `test`) passes with the feature in place.

## Assumptions

- **JSON is the only new output format in scope.** The flag is designed to be extensible to `yaml` or other formats later, but only `text` and `json` ship in this iteration.
- **`template fetch` is deferred.** It already writes its own `templates.json` as its primary artifact and has its own `--json` manifest flag; reconciling its output model with the envelope is out of scope here.
- **Schema versioning uses a monotonic integer, not a published JSON Schema file.** The `schema_version` field is the contract. Publishing a separate schema document (e.g., `docs/json-schema/v1.json`) is a future task.
- **Stderr is the accepted channel for human-readable diagnostics in JSON mode.** Consumers who want purely machine-readable output silence stderr (`2>/dev/null`). Humans running interactively get both structured JSON on stdout and live warnings/hints on stderr.
- **Warnings and hints are populated from existing sources.** Warnings come from code paths that already print `⚠ WARNING: ...` lines; hints come from the existing `internal/fleet/diagnostics.go` `CollectHints` layer. No new diagnostic sources are introduced.
- **The feature depends on issue #24 (zerolog) for stderr structured logging**, but can land alongside or before #24 via a thin `stderrWarn()` stub that #24 later replaces — the feature is not strictly blocked on #24's merge order.
- **No internal fleet package result struct shapes change.** Only JSON tags are added; no fields are added, removed, or renamed (other than introducing the new `ListResult` which did not previously exist).
- **Compact JSON output is acceptable.** No `--json-pretty` variant is provided; consumers pipe through `jq` if they want formatting.
- **Flag is persistent on root.** All subcommands inherit it through cobra, even those that don't honor it (which error on `-o json` rather than silently ignore).
