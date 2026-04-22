# Contract: CLI Flag Surface

**Feature**: 002-add-zerolog-logging
**Spec FRs**: FR-001, FR-002, FR-003, FR-004

This contract defines the persistent CLI-flag surface this feature adds to the root `gh-aw-fleet` command. All flags are persistent (inherited by every subcommand). Default values preserve pre-feature behavior.

---

## Flags

### `--log-level`

| Property | Value |
|---|---|
| Type | string (enum) |
| Default | `info` |
| Valid values | `trace`, `debug`, `info`, `warn`, `error` |
| Scope | persistent on root |
| Case | lowercase only (zerolog's `ParseLevel` accepts these exactly) |

**Semantics**: Events at this level and higher are emitted; lower-severity events are dropped. Ordering (lowest to highest severity): `trace < debug < info < warn < error`. At `info` (default), `trace` and `debug` events are dropped — this is what makes subprocess summary events (at `debug`) invisible unless the operator opts in.

**Rejection behavior**: Any value not in the valid-values list causes cobra to print `Error: invalid --log-level "<value>": ...` to stderr via its default error writer and exit with status 1, WITHOUT running the requested subcommand (FR-004, Q4 clarification). The rejection message is plain text, never JSON — consumers parsing stderr as JSON must tolerate non-JSON flag-error lines.

---

### `--log-format`

| Property | Value |
|---|---|
| Type | string (enum) |
| Default | `console` |
| Valid values | `console`, `json` |
| Scope | persistent on root |
| Case | lowercase only |

**Semantics**:
- `console`: Human-readable rendering via `zerolog.ConsoleWriter`. Compact multi-column layout, RFC3339 timestamp, level abbreviation (`WRN`/`ERR`/`INF`/`DBG`/`TRC`), message, then `key=value` fields.
- `json`: Line-delimited JSON. Each log event is exactly one `{…}\n` line with `level`, `time`, `message`, and any call-site-supplied fields.

**Rejection behavior**: Same as `--log-level` — cobra's default error path, plain-text stderr, exit 1.

---

### Non-TTY behavior

The `--log-format` value is honored regardless of whether stderr is a terminal. The tool does NOT auto-switch to JSON when stderr is piped (spec Assumptions, Q4 of original issue). Operators and CI that want JSON output pass `--log-format=json` explicitly. This is predictable and deterministic; environment-sniffing magic is rejected.

---

## Flag registration (implementation-level)

```go
// cmd/root.go (fragment)
root.PersistentFlags().String("log-level", "info", "Log verbosity: trace|debug|info|warn|error")
root.PersistentFlags().String("log-format", "console", "Log format: console|json")

root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
    level, _ := cmd.Flags().GetString("log-level")
    format, _ := cmd.Flags().GetString("log-format")
    return logpkg.Configure(level, format)
}
```

Flag name choices follow the spec's literal wording (FR-001, FR-002). Short aliases (`-l`, `-f`) are intentionally NOT added — they'd collide with future command-specific flags and they provide negligible ergonomic value for infrequent tuning.

---

## Exit-status behavior

| Scenario | Exit status | Output path |
|---|---|---|
| Valid flags, subcommand succeeds | 0 | stdout (subcommand output) + stderr (any logs at configured level) |
| Valid flags, subcommand fails | 1 | stdout (partial subcommand output) + stderr (final error via `log.Error().Err(err).Msg(...)` in `main.go`, in the chosen format) |
| Invalid `--log-level` value | 1 | stderr plain text: `Error: invalid --log-level "<v>": ...` via cobra; no subcommand runs |
| Invalid `--log-format` value | 1 | stderr plain text: `Error: invalid --log-format "<v>": ...` via cobra; no subcommand runs |
| Unknown flag (pflag's own rejection) | 1 | stderr plain text via cobra (pre-existing behavior, unchanged) |

---

## Backward-compatibility contract

A pre-feature invocation `gh-aw-fleet <subcommand> [args…]` and a post-feature invocation with no new flags MUST produce:

- **stdout**: byte-identical output on every code path that does not trigger a `⚠ WARNING:` emission (spec SC-001).
- **stderr**: the pre-feature content plus, at `info` level by default, any `warn`/`error` events that were previously `fmt.Fprintln(os.Stderr, ...)` unstructured lines. The warning lines that previously appeared on **stdout** (via tabwriter) move to **stderr** as structured events — this is the one intentional UX shift and is noted in CHANGELOG.md.

---

## Test contract

Unit tests in `internal/log/log_test.go` MUST verify:
- All five valid `--log-level` values produce a logger that emits at/above that level.
- Both valid `--log-format` values produce parseable output in the expected shape.
- Invalid values for either flag return a non-nil error from `Configure`.
- The error message from `Configure` identifies the offending flag name and value.

Integration tests in `cmd/root_logging_test.go` MUST verify:
- Both flags register as persistent flags on the root command.
- An invalid `--log-level` exits the process with non-zero status WITHOUT executing the subcommand (use a subcommand with an observable side effect — e.g., a stub — and assert the side effect did NOT occur).
- A `warn` event emitted during subcommand execution reaches stderr as valid JSON when `--log-format=json` is passed, carrying the expected `repo` field.
