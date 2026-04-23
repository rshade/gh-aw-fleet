# Contract: CLI Flag Surface

**Feature**: 003-cli-output-json
**Spec FRs**: FR-001, FR-002, FR-003, FR-013

This contract defines the persistent CLI-flag surface this feature adds to the root `gh-aw-fleet` command. The flag is persistent (inherited by every subcommand). Default preserves pre-feature behavior.

---

## Flag

### `-o` / `--output`

| Property | Value |
|---|---|
| Type | string (enum) |
| Default | `text` |
| Valid values | `text`, `json` |
| Short form | `-o` |
| Long form | `--output` |
| Scope | persistent on root (`rootCmd.PersistentFlags()`) |
| Case | lowercase only; `JSON`, `Text`, `TEXT`, etc. are rejected |

**Semantics**:

- `text` (default) — every subcommand uses its existing text-mode printer. Stdout is byte-identical to pre-feature behavior (FR-014).
- `json` — supported subcommands (`list`, `deploy`, `sync`, `upgrade`) emit a single JSON envelope on stdout per invocation (NDJSON per repo for `upgrade --all`). All human-oriented breadcrumbs (e.g., `(loaded fleet.local.json)`) are routed to stderr via zerolog. Unsupported subcommands (`template fetch`, `add`, `status` if present) reject `-o json` with a clear error.

**Rejection behavior — invalid value** (FR-002):

Cobra's `PersistentPreRunE` on the root command validates the flag value against the closed set. Any other value (including `yaml`, `YAML`, `Json`, empty string `""`) causes:

```text
Error: unsupported output mode "yaml": expected one of: text, json
```

...written to stderr as plain text (not JSON — flag errors are pre-serialization and never emit an envelope). Exit code: 1. Subcommand RunE is NOT invoked.

**Rejection behavior — unsupported subcommand** (FR-013):

When `-o json` is passed to `template fetch` (or any other subcommand that doesn't support JSON mode), the subcommand's own RunE rejects it with:

```text
Error: command "template fetch" does not support --output json; use --output text or omit the flag
```

...written to stderr as plain text. Exit code: 1. The subcommand does not execute.

Rationale: a silent fallback to text mode when an operator expected JSON would cause `jq` parse failures and confusing debugging. Explicit rejection is kinder.

---

## Interaction with other persistent flags

| Flag | Interaction |
|---|---|
| `--dir` | Orthogonal. `--dir` selects where to load `fleet.json` / `fleet.local.json`; `--output` selects how to render results. Both are persistent on root. Any combination is valid. |
| `--log-level` | Orthogonal. `--log-level` filters stderr zerolog events by severity. In JSON mode, warnings and hints emit on stderr via zerolog AND in the envelope on stdout; `--log-level=error` suppresses the stderr emission but does NOT affect envelope contents. This is intentional — machine consumers get the full structured record regardless of log-level. |
| `--log-format` | Orthogonal. `--log-format=json` makes stderr also line-structured JSON; stdout envelope is unaffected. A pipeline wanting pure machine-parseable output on both streams uses `-o json --log-format=json`. |
| `--apply` | Orthogonal, subcommand-scoped (present on `deploy`, `sync`, `upgrade`). `-o json --apply` runs the actual apply and emits the envelope for the apply result. The three-turn pattern (constitution III) still applies; the `-o json` flag has no bearing on whether approval is required. |
| `--all` | Scoped to `upgrade`. `upgrade --all -o json` triggers NDJSON emission per `contracts/ndjson.md`. |
| `--work-dir` | Scoped to `deploy`. Orthogonal to `-o json`. The resume flow works identically in JSON mode. |

---

## Subcommand support matrix

| Subcommand | `-o json` supported | Notes |
|---|---|---|
| `list` | ✓ | Emits `ListResult` envelope |
| `deploy <repo>` | ✓ | Emits `DeployResult` envelope |
| `sync <repo>` | ✓ | Emits `SyncResult` envelope |
| `upgrade <repo>` | ✓ | Emits `UpgradeResult` envelope (single) |
| `upgrade --all` | ✓ | Emits NDJSON stream (per contracts/ndjson.md) |
| `add <owner/repo>` | ✗ | Out of scope. Rejects `-o json`. |
| `template fetch` | ✗ | Explicitly out of scope per spec Out of Scope; has its own `--json` flag for a different purpose. Rejects `-o json`. |
| `status [repo]` | ✗ | Stub command today; out of scope. Rejects `-o json`. |

---

## Implementation hooks

### Root command (`cmd/root.go`)

```go
// Registration inside newRoot():
root.PersistentFlags().StringP("output", "o", "text", "Output format: text|json")

// Validation inside PersistentPreRunE (chained after existing log-flag validation):
out, _ := cmd.Flags().GetString("output")
switch out {
case "text", "json":
    // OK
default:
    return fmt.Errorf("unsupported output mode %q: expected one of: text, json", out)
}
```

### Helper (`cmd/output.go`)

```go
// outputMode reads the resolved --output value from the given cobra command.
// Safe to call from any subcommand's RunE; returns "text" if the flag is unset
// (though PersistentPreRunE has already validated).
func outputMode(cmd *cobra.Command) string {
    v, _ := cmd.Flags().GetString("output")
    if v == "" {
        return "text"
    }
    return v
}
```

### Subcommand-level rejection (example: `cmd/template.go`)

```go
RunE: func(cmd *cobra.Command, args []string) error {
    if outputMode(cmd) == "json" {
        return fmt.Errorf("command %q does not support --output json; use --output text or omit the flag", "template fetch")
    }
    // ... existing RunE body unchanged
},
```

---

## Testing (`cmd/output_test.go`)

| Test name | Covers |
|---|---|
| `TestOutputFlag_AcceptsText` | `-o text` passes validation (FR-001) |
| `TestOutputFlag_AcceptsJSON` | `-o json` passes validation (FR-001) |
| `TestOutputFlag_DefaultText` | omitting `--output` resolves to `text` (FR-001) |
| `TestOutputFlag_RejectsYAML` | `-o yaml` returns non-nil error with expected message (FR-002) |
| `TestOutputFlag_RejectsUpperCase` | `-o JSON` returns non-nil error (FR-002, case sensitivity) |
| `TestOutputFlag_RejectsEmpty` | `-o ""` returns non-nil error (FR-002) |
| `TestTemplateFetch_RejectsJSONMode` | `template fetch -o json` returns non-nil error (FR-013) |

Tests use cobra's `ExecuteC` in-process invocation pattern (same as `cmd/root_logging_test.go` from #34).
