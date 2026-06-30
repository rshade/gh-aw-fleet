# Phase 1 Data Model: Interactive security-finding prompt before commit

This feature adds no persistent data and no new wire/config schema. It adds one
options field, one pure decision function, one typed error, and one exported
render helper. All "entities" below are in-memory Go values consumed during a
single command invocation.

## Consumed (unchanged) types

### `security.Finding` (existing — `internal/fleet/security/finding.go`)

Consumed read-only. The prompt reads `Severity` for the summary tally and the
slice length for the fire/skip decision. No field is added, reordered, or
modified. Ordering of `[]security.Finding` is preserved.

### `security.Severity` (existing)

`SeverityHigh | SeverityMedium | SeverityLow | SeverityInfo`. The summary line
tallies these in HIGH→MEDIUM→LOW→INFO order, omitting zero counts (existing
`severityTally` behavior).

## Added / modified types

### `SecurityOpts.Yes` (modified — `internal/fleet/security_gate.go`)

```go
// SecurityOpts controls invocation-scoped security policy.
type SecurityOpts struct {
    Strict bool // block on HIGH non-promptinj security findings
    Yes    bool // skip the interactive findings confirmation prompt (does not suppress stderr/PR-body findings)
}
```

- **Field meaning**: `Yes` skips *only* the interactive confirmation. It does not
  affect the strict gate, the stderr findings warnings, or the PR-body section.
- **Lifecycle**: set from the `--yes` Cobra flag in `cmd/{deploy,sync,upgrade}.go`;
  threaded unchanged through `DeployOpts`/`SyncOpts`/`UpgradeOpts` and through
  sync's delegated `Deploy` call (so the "prompt once" delegation keeps working).
- **Not persisted**: never written to `fleet.json`/`fleet.local.json`; no schema
  bump.
- **Zero value**: `false` → prompt is eligible to fire (the default, gated by
  findings-present + interactive-stdout).

### `PromptUser` (new — `internal/fleet/security_prompt.go`)

```go
// PromptUser asks the operator to confirm proceeding despite findings.
// Returns (true, nil) automatically when: findings is empty, yes is true, or
// stdout is not a terminal. Otherwise it writes a one-line severity summary to
// out and reads a line from in: "y"/"Y"/"yes" → (true, nil); any other non-empty
// or empty line → (false, nil); a read error or EOF before an answer → (false, err).
func PromptUser(findings []security.Finding, yes bool, in io.Reader, out io.Writer) (proceed bool, err error)
```

Pure and fully injectable for tests. The stdout-TTY check is a package-level seam
(`stdoutIsTerminal`) so the non-TTY branch is unit-testable without a real
terminal.

**Decision table**:

| findings | yes | stdout TTY | input line | result |
|----------|-----|-----------|-----------|--------|
| empty | any | any | — | `(true, nil)` |
| present | true | any | — | `(true, nil)` |
| present | false | no | — | `(true, nil)` |
| present | false | yes | `y` / `Y` / `yes` | `(true, nil)` |
| present | false | yes | `n` / other / empty | `(false, nil)` |
| present | false | yes | EOF / read error | `(false, err)` |

### `stdoutIsTerminal` (new seam — `internal/fleet/security_prompt.go`)

```go
// stdoutIsTerminal reports whether stdout is an interactive character device.
// Overridable in tests. Stdlib only — mirrors cmd/add.go's isStdinTerminal.
var stdoutIsTerminal = func() bool {
    fi, err := os.Stdout.Stat()
    if err != nil {
        return false
    }
    return fi.Mode()&os.ModeCharDevice != 0
}
```

### `OperatorDeclinedError` (new — `internal/fleet/security_prompt.go`)

```go
// OperatorDeclinedError is returned when the operator declines the interactive
// security-findings confirmation (including empty input, EOF, or a read error).
type OperatorDeclinedError struct {
    Repo     string // repository the apply was aborted for
    Findings int    // number of findings shown at the prompt
    Cause    error  // non-nil for EOF / read-error declines; nil for an explicit "no"
}
```

- `Error()` → actionable message, e.g. `aborted by operator: <N> security
  finding(s) for <repo> not accepted; re-run with --yes to skip the prompt`.
- `Unwrap()` → `Cause` (so EOF/read errors remain inspectable).
- A package predicate `IsOperatorDeclinedError(err) bool` (via `errors.As`)
  mirrors `IsStrictSecurityError`, letting `cmd/output.go` print the decline
  cleanly and exit non-zero without hint-engine noise.

### `confirmSecurityFindings` (new internal wrapper — `internal/fleet/security_prompt.go`)

```go
// confirmSecurityFindings runs PromptUser with the production stdin/stdout seams
// and, on decline, preserves the clone and returns an *OperatorDeclinedError.
func confirmSecurityFindings(repo string, findings []security.Finding, opts SecurityOpts, cleanupClone *bool) error
```

- Returns `nil` when `PromptUser` reports proceed.
- On decline, sets `*cleanupClone = false` (mirrors `preserveCloneForStrictError`;
  on resume paths cleanup is already disabled, so the pointer may be nil) and
  returns the typed error. This is the function the apply boundaries call.

### `security.SeveritySummary` (new export — `internal/fleet/security/render.go`)

```go
// SeveritySummary returns "2 HIGH, 1 MEDIUM" — the per-severity tally used by
// both the PR-body summary and the interactive prompt. Empty string for no findings.
func SeveritySummary(findings []Finding) string
```

Thin exported wrapper over the existing unexported `severityTally`; no behavior
change to the PR-body rendering that already uses it.

## Relationships & flow

```text
cmd/{deploy,sync,upgrade}.go
  flag --yes ──► fleet.SecurityOpts{Strict, Yes}
                      │
                      ▼
internal/fleet/{deploy,sync,upgrade}.go  (apply path, per repo)
  security.Run ─► EvaluateStrictSecurityGate ─► [pending commit?] ─► confirmSecurityFindings
                                                                          │
                                   PromptUser(findings, opts.Yes, stdin, stdout)
                                     │ proceed                    │ decline
                                     ▼                            ▼
                         createDeployPR / createUpgradePR    *OperatorDeclinedError
                         / commitAndPushPrune / pushAndCreatePR   (clone preserved)
                                     │                            │
                                     ▼                            ▼
                         PR body includes ## Security Findings   cmd/output.go → clean non-zero exit
```

## Validation rules (from Functional Requirements)

- Prompt eligible **only** when `apply && len(findings) > 0 && !opts.Yes &&
  stdoutIsTerminal()` and a commit is pending (FR-001, FR-009, FR-010, FR-015).
- Prompt fires **after** the strict gate; a strict abort returns before the
  prompt (FR-011).
- Affirmative = `y` / `Y` / `yes` (case-insensitive, trimmed); empty = decline
  (FR-003, FR-004).
- Decline → no commit/push/PR, typed error, non-zero exit, clone preserved
  (FR-005).
- `--yes` skips the prompt but never suppresses stderr warnings or the PR-body
  section (FR-007, FR-008).
- `--output json` mode suppresses the prompt even when stdout is a TTY; the
  apply proceeds and stderr/PR-body surfaces remain (FR-018).
- Zero findings → no prompt, no PR-body section (FR-010).
