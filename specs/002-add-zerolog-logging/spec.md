# Feature Specification: Structured Logging for Errors, Warnings, and Diagnostics

**Feature Branch**: `002-add-zerolog-logging`
**Created**: 2026-04-20
**Status**: Draft
**Input**: User description: "feat(logging): introduce zerolog for errors, warnings, and diagnostics — add a structured logging layer for errors, warnings, and diagnostic output; keep readable CLI status tables intact; issue #24"

## Clarifications

### Session 2026-04-20

- Q: Should the logger redact secrets (PATs, tokens embedded in URLs, env-injected credentials) that could leak through subprocess-summary fields? → A: Never log raw subprocess argv, env, or URL-bearing strings. Structured fields are restricted to an allowlist: tool name, subcommand name, repo, workflow, exit code, duration, clone dir.
- Q: At what log level should subprocess summary events be emitted? → A: `debug`. Default runs stay quiet; operators opt in with `--log-level=debug`.
- Q: How should Go error chains be represented in JSON log events? → A: Both — errors appear in a dedicated top-level `error` field AND the error text is also appended to `message`. Operators get clean jq-filterability on `.error` while console output stays readable from `.message` alone.
- Q: How should invalid `--log-level` / `--log-format` values be reported, given the logger is not yet configured at that point? → A: Plain stderr via the CLI framework's default flag-error handling; these errors bypass the logger entirely. JSON-parsing consumers must tolerate non-JSON flag-error lines (consistent with existing flag-error UX for unknown flags, missing args, etc.).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator configures log verbosity and format from the CLI (Priority: P1)

An operator invoking `gh-aw-fleet` locally or in CI selects a log verbosity level and an output format via persistent CLI flags. The command emits diagnostics (timestamps, levels, structured fields) on stderr while preserving the existing human-readable status output (tabwriter tables, PR URLs) on stdout byte-for-byte.

**Why this priority**: This is the foundation. Without a configured log pipeline, no other structured log event can exist. Delivering just P1 gives operators a working knob (`--log-level=debug`, `--log-format=json`) that they can pipe into CI log collectors, while all prior status output continues to work unchanged. It is the minimum viable diagnostic layer.

**Independent Test**: Run `gh-aw-fleet list --log-level=debug --log-format=json 2>/tmp/log.json`, confirm `/tmp/log.json` contains parseable line-delimited JSON with `level` and `time` fields, and confirm stdout output is byte-identical to `gh-aw-fleet list` without the flags.

**Acceptance Scenarios**:

1. **Given** no logging flags are passed, **When** any subcommand runs, **Then** stderr shows only messages at `info` level or higher in human-readable (console) format and stdout status output is unchanged from the pre-feature baseline.
2. **Given** `--log-level=debug` is passed, **When** a subcommand runs, **Then** debug-level messages appear on stderr that were hidden at the default level.
3. **Given** `--log-format=json` is passed, **When** any message is logged, **Then** each line on stderr is a self-contained JSON object with at minimum `level`, `time`, and `message` keys.
4. **Given** `--log-level=invalid-value` is passed, **When** the command starts up, **Then** it exits with a non-zero status and a clear error message identifying the invalid level, without attempting to run the subcommand.

---

### User Story 2 - Existing user-facing warnings carry structured context for filtering (Priority: P2)

Warnings that `deploy`, `sync`, and `upgrade` already emit today (secret drift, workflow drift, gpg signing failures) route through the new log pipeline at `warn` level and carry structured fields — at minimum the affected repository name — so an operator running against many repos can filter or group by repo in a log collector or with grep/jq.

**Why this priority**: This is the principal payoff of P1's infrastructure. It turns unstructured `⚠ WARNING:` lines — currently indistinguishable from each other when 10 repos fail — into queryable records. It depends on P1's pipeline being live but is independently demonstrable: pick one deploy warning path and verify its structured output.

**Independent Test**: Trigger a known warning path (e.g., run `deploy --apply` in a context that produces a secret-drift warning) with `--log-format=json`, and confirm the warning appears as a `level=warn` JSON record that includes the repo identifier as a distinct field.

**Acceptance Scenarios**:

1. **Given** `deploy` or `sync` detects a condition that previously produced a `⚠ WARNING:` line, **When** the warning is emitted with `--log-format=json`, **Then** stderr contains a `level=warn` JSON record with a `repo` field identifying the affected repository.
2. **Given** the same warning is emitted with `--log-format=console`, **When** it is displayed, **Then** it remains human-readable and includes the repo identifier.
3. **Given** `--log-level=error` is passed, **When** only warnings would fire, **Then** no warning output appears on stderr.

---

### User Story 3 - Subprocess outcomes and diagnostic hints are captured as queryable events (Priority: P3)

When the orchestrator invokes external tools (`git`, `gh`, `gh aw`), the live output remains teed to stderr (unchanged) so operators can watch progress. In addition, after each subprocess returns, a structured summary event records the command identity, exit code, and elapsed duration. Diagnostic hints produced by the existing hint-collection layer (e.g., unknown-property, HTTP 404, gpg failure) are also emitted as structured warn events with the hint text available as a field.

**Why this priority**: These are the debug-path improvements that accelerate root-causing a failed multi-repo deploy. They are nice-to-have rather than blocking: grep/pipe already works at P2. This slice can ship in a follow-up commit without invalidating earlier slices.

**Independent Test**: Run `deploy --apply` under `--log-level=debug --log-format=json` against a small test fleet, and confirm stderr contains (a) one summary JSON event per invoked subprocess with `exit_code` (integer) and `duration` (integer milliseconds) fields, and (b) any triggered diagnostic hints as `level=warn` JSON records with the hint text carried in a field (not only in the message string).

**Acceptance Scenarios**:

1. **Given** a subprocess runs to completion, **When** it returns, **Then** a structured summary event is emitted at debug or info level identifying the command and its exit code.
2. **Given** the diagnostic layer matches a known error pattern, **When** the hint is produced, **Then** it is emitted as a warn-level structured event with the hint text in a dedicated field.
3. **Given** live subprocess stdout/stderr is being teed to the operator's terminal, **When** logging is enabled, **Then** the tee is unaffected — operators still see `git` / `gh` output in real time.

---

### Edge Cases

- **Default invocation vs. baseline**: Running without any logging flag MUST preserve stdout output byte-for-byte relative to the pre-feature baseline; any drift there is a regression (this is explicit in the issue's acceptance criteria).
- **Non-TTY stderr**: When stderr is piped (CI, redirection), the `--log-format` flag wins — the tool does NOT auto-switch to JSON. CI that wants JSON passes `--log-format=json` explicitly. Rationale: flag predictability over environment sniffing.
- **Invalid flag values**: An invalid `--log-level` or `--log-format` must be rejected before the subcommand runs, not discovered mid-pipeline after side-effects have started (especially relevant for `--apply` runs). The rejection message is plain text on stderr (flag-error path), NOT routed through the logger — the logger cannot honor a format value it is still validating. JSON-parsing stderr consumers accept that flag-error lines are non-JSON, matching existing CLI UX for other flag errors.
- **Pre-feature stderr content (status lines, progress)**: Human-readable progress/status currently written to stderr by tabwriter or similar is UX, not logging; it remains untouched. Only the specific `⚠ WARNING:` and error-exit paths are rerouted.
- **Tests as consumers**: Test suites that call into the codebase need a way to silence the logger (quiet level + console format is acceptable) so that unrelated stderr content does not leak into test expectations.
- **Dependency hygiene**: Adding the logging library must not pull in further transitive dependencies or duplicate responsibilities already handled by upstream tools (thin-orchestrator principle).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The CLI MUST accept a persistent `--log-level` flag on the root command with values `trace`, `debug`, `info`, `warn`, and `error`, defaulting to `info`.
- **FR-002**: The CLI MUST accept a persistent `--log-format` flag on the root command with values `console` and `json`, defaulting to `console`.
- **FR-003**: Logging configuration MUST be applied exactly once, before any subcommand begins work, so that the very first log event honors the selected level and format.
- **FR-004**: An invalid value for `--log-level` or `--log-format` MUST cause the CLI to exit with a non-zero status and a clear message identifying the offending flag and value, WITHOUT executing the subcommand's main logic. The rejection message MUST be written to stderr as plain text via the CLI framework's default flag-error path; it MUST NOT be routed through the logger (the logger cannot be configured with a flag value that is itself invalid). Consumers parsing stderr as JSON are expected to tolerate non-JSON flag-error lines, consistent with the CLI's existing behavior for unknown flags and missing arguments.
- **FR-005**: When `--log-format=json` is selected, every log event written to stderr MUST be a single line of valid JSON containing at minimum `level`, `time`, and `message` fields, plus any structured fields supplied at the call site. For events that carry an error value, JSON output MUST also include a top-level `error` field containing the error's string form (see FR-017).
- **FR-006**: When `--log-format=console` is selected, log output MUST be human-readable on a terminal (level indicator, timestamp, message, structured fields rendered compactly).
- **FR-007**: All log output MUST be written to stderr. Stdout MUST NOT receive any log events under any flag combination.
- **FR-008**: Human-readable CLI status output (tabwriter status tables, `added: N` lines, `PR: <url>` lines, progress indicators, PR body and commit message templating) MUST remain byte-identical on stdout when the CLI is run with default flags (`--log-level=info --log-format=console`) relative to the pre-feature baseline, on invocations that do NOT trigger a migrated warning path (see FR-009). On invocations that do trigger a migrated warning path, stdout intentionally loses the `⚠ WARNING:` block (now on stderr as a structured event); the remainder of stdout remains byte-identical.
- **FR-009**: Every existing `⚠ WARNING:` output path in the deploy and sync flows MUST be re-emitted as a `warn`-level structured log event. Each such event MUST include at minimum a field identifying the affected repository.
- **FR-010**: Diagnostic hints produced by the existing hint-collection layer MUST be emitted as `warn`-level structured events with the hint text accessible as a distinct field (not only embedded in the message string), to support grep / jq filtering. The existing plaintext `hint: <text>` lines on stdout (tabwriter) are PRESERVED; the structured event is emitted *in addition*, on stderr. Rationale: hints were pre-existing operator-readable output on stdout; removing them would break SC-001 on any invocation that hits a hint pattern, and the structured stderr event is an additive debugging aid rather than a replacement.
- **FR-011**: For each external subprocess invocation, a structured summary event MUST be emitted after the process completes, carrying at minimum the tool name (e.g., `git`, `gh`, `gh aw`), the subcommand name (e.g., `push`, `pr create`), exit code, and elapsed duration. The summary event MUST be emitted at `debug` level so that the default verbosity (`info`) produces no subprocess summary output; operators see summaries only when they pass `--log-level=debug` or lower.
- **FR-012**: Live subprocess stdio teeing to stderr MUST be preserved; operators MUST still see `git` / `gh` / `gh aw` output in real time while subprocesses run.
- **FR-016**: Structured log events MUST NOT include raw subprocess argv, raw environment variables, or any URL-bearing strings that could carry credentials. Event fields are restricted to a fixed allowlist: `tool`, `subcommand`, `repo`, `workflow`, `exit_code`, `duration`, `clone_dir`, `hint`, `error`, `level`, `time`, `message`, `secret` (name of a missing Actions secret, e.g. `DEPLOY_TOKEN` — never the secret value), `drift` (array of drifted workflow base names). New fields MAY be added over time, but each addition MUST be explicit and reviewed against this no-secrets constraint; automatic capture of argv, env, or command strings is prohibited.
- **FR-017**: For log events that carry a Go error value, the error MUST be represented in two places: (a) as a dedicated top-level `error` field containing the error's string form (preserving the error chain's `.Error()` output), AND (b) appended to the `message` field so the human-readable message remains self-contained. The `error` field gives operators a clean `jq 'select(.error)'`-style filter target; the duplicated text in `message` keeps console output readable without cross-field inspection. Implementations MUST ensure the two representations are produced from the same underlying error value so they cannot drift.
- **FR-013**: Existing user-facing behavior for error exits (non-zero return code, error message visible to the operator) MUST be preserved. The error message MUST additionally be routed through the logger at `error` level so it inherits the selected format.
- **FR-014**: The logging library added to satisfy these requirements MUST NOT pull in further transitive dependencies beyond itself, and MUST NOT duplicate responsibilities already provided by upstream tools (thin-orchestrator check).
- **FR-015**: Test suites MUST have a supported way to silence the logger (e.g., setting the level to `error` with `console` format) so that logging output does not interfere with test assertions.

### Key Entities *(include if feature involves data)*

- **Log event**: A single structured record emitted to stderr. Carries `level`, `time`, `message`, and call-site-supplied fields drawn from a fixed allowlist: `tool`, `subcommand`, `repo`, `workflow`, `exit_code`, `duration`, `clone_dir`, `hint`, `error`, `secret`, `drift`. Raw subprocess argv, raw environment variables, and URL-bearing strings are explicitly excluded (see FR-016). When an underlying Go error is present, it populates both the top-level `error` field and is appended to `message` (see FR-017). Serialized as a human-readable line in console mode or a JSON object in JSON mode.
- **Logger configuration**: The once-per-invocation state established from `--log-level` and `--log-format`. Immutable for the lifetime of a command run.
- **Subprocess summary**: A log event emitted once per invoked external tool, describing the command, its exit code, and its duration, independent of the live teed output.
- **Diagnostic hint**: An existing item produced by the hint-collection layer when a subprocess error matches a known pattern; elevated in this feature from plain-text stderr writes to a structured warn-level event carrying the hint text in a field.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a default invocation of any subcommand (no logging flags), stdout output is byte-identical to the pre-feature baseline on at least three representative commands (`list`, `deploy` dry-run, `upgrade` dry-run).
- **SC-002**: With `--log-format=json`, 100% of emitted log lines on stderr parse as valid JSON; zero invalid lines appear in a sampled run across all subcommands.
- **SC-003**: An operator filtering for events about a single repository across a multi-repo deploy can isolate them with one structured filter (e.g., a single `jq` or grep predicate on the `repo` field) — no multi-line parsing or regex gymnastics required.
- **SC-004**: Existing tests pass unchanged; new unit tests cover all supported log levels, both formats, and invalid-input rejection for the configuration function.
- **SC-005**: `go vet ./...` and the project's full CI gate (`make ci`) pass with the feature merged — no lint regressions, no formatting drift.
- **SC-006**: Added dependency footprint is one direct dependency with zero additional transitive dependencies introduced (measured via `go mod graph` before-and-after).
- **SC-007**: Every pre-existing `⚠ WARNING:` output path in deploy and sync is covered by the new structured emission (zero regressions: no warning disappears silently and no warning remains as an unstructured `fmt.Fprintln(os.Stderr, ...)` call).

## Assumptions

- **Implementation library**: `github.com/rs/zerolog` is the logging library chosen (per the issue). The spec's behavioral requirements do not depend on that choice, but the thin-dependency assumption (FR-014, SC-006) was validated against zerolog specifically; a different choice would need re-validation.
- **Flag-wins over environment sniffing**: When stderr is not a TTY, the `--log-format` value is honored as provided — the tool does NOT auto-switch to JSON. CI operators that want JSON pass the flag explicitly. Rationale: predictable behavior over magic.
- **Stderr-only sink**: No log file, log rotation, or remote shipping is in scope. All events go to stderr; operators redirect as needed.
- **Tabwriter / status output stays as-is**: Readable progress and status output currently emitted to stdout (and any stderr that is part of UX, not errors) is outside the scope of this feature. Only the specific warning and error-exit paths are rerouted.
- **`fmt.Fprintf(&b, ...)` builders stay as-is**: PR body and commit-message content generation via `strings.Builder` is content, not logging, and is unaffected.
- **Subprocess teeing stays as-is**: The `io.MultiWriter(os.Stderr, &buf)` pattern used for live `git` / `gh` output is preserved; the feature adds a post-hoc summary event without interrupting the live tee.
- **Test environments already call into the CLI entry points**: Silencing the logger in tests is achievable through the configuration function, without needing to alter test invocation patterns broadly.
- **Operators and CI are the relevant audience**: End-users of workflows deployed by this tool are not affected; the audience for the diagnostic layer is the person (or CI job) running `gh-aw-fleet`.
