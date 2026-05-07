# Contract: `security.Scanner` interface and `security.Run` orchestration

**Phase**: 1 (Design & Contracts) | **Spec**: [../spec.md](../spec.md) | **Date**: 2026-04-30

## Purpose

Defines the public surface of `internal/fleet/security/` that the rest of the codebase calls. The single entry point `Run` is what `internal/fleet/deploy.go`/`sync.go`/`upgrade.go` invoke; the `Scanner` interface is the v1 boundary for adding new detectors in future iterations without changing the call site.

## Public surface

```go
package security

// Run scans every workflow in the work-dir clone and returns sorted findings.
// Returns nil (not an empty slice) when there is nothing to scan
// (e.g. cloneDir does not contain a .github/workflows directory).
//
// Run is non-fatal: scanner internal errors are logged at warn level via
// zerolog and the run continues with remaining scanners. Run never returns
// an error; failures surface as findings (INFO severity for skipped scanners).
//
// Engine resolution for the engine.env.non-allowlist rule (FR-018) is
// per-workflow: the structural scanner reads the `engine:` key from each
// workflow's frontmatter. There is no fleet-level engine parameter; a
// profile may legitimately mix engines across workflows.
func Run(ctx context.Context, cloneDir string) []Finding

// Scanner is the v1 detector interface. Implementations MUST tolerate
// missing or malformed input by emitting INFO findings rather than panicking
// or returning errors.
type Scanner interface {
    Scan(ctx context.Context, cloneDir string) []Finding
}

// RenderForStderr returns a multi-line plain-text rendering of findings,
// suitable for passing one line at a time to zerolog.Warn(). Empty string
// when len(findings) == 0.
func RenderForStderr(findings []Finding) string

// RenderForPRBody returns the markdown content of the `## Security Findings`
// section: a summary tally line followed by per-finding bullets in sorted
// order. Empty string when len(findings) == 0 — caller suppresses the heading
// when output is empty (FR-005).
func RenderForPRBody(findings []Finding) string

// Severity is a typed int. See contracts/finding.md for constants and ordering.
type Severity int
```

## `Run` algorithm contract

```text
1. Build []Scanner with three v1 entries (in this exact order):
     gitleaks, structural, actionlint
   gitleaks's *Detector is constructed exactly once, here.
   actionlint's exec.LookPath check happens during constructor; result
   captured for reuse.

2. For each scanner: call Scan(ctx, cloneDir).
   Wrap each Scan call in a defer/recover to convert panics into a single
   INFO finding ("scanner {name} panicked: {msg}") rather than aborting Run.
   Concatenate emitted findings into one combined slice.

3. Sort the combined slice using the contract in contracts/finding.md.

4. Return the sorted slice.
```

## `RenderForStderr` output shape

One line per finding. Format:

```text
[SEVERITY] rule_id  file:line  message
```

Concrete examples:

```text
[HIGH] gitleaks:aws-access-key  .github/workflows/foo.md:23  AWS Access Key (<redacted>)
[HIGH] fleet.permissions.write-on-schedule  .github/workflows/bar.md:5  Workflow has permissions: contents: write and on: schedule trigger
[INFO] actionlint:not-installed  -  actionlint binary not found in PATH; compiled-YAML lint scanner skipped
```

Note: when `Line == 0`, render as just `file` (no `:0` suffix). When both `File == ""` and `Line == 0`, render the file slot as `-`.

The caller (`cmd/deploy.go`'s `emitDeployWarnings`) splits on newline and calls `zerolog.Warn().Str("rule_id", f.RuleID).Msg(line)` per line, OR — preferred — iterates `[]Finding` directly and produces structured warnings without the intermediate string. `RenderForStderr` exists for tests and for future operators who want to compose the full block without the structured-logging detour.

## `RenderForPRBody` output shape

```markdown
**Summary**: 2 HIGH, 1 MEDIUM, 1 INFO

- **HIGH** `gitleaks:aws-access-key` — `.github/workflows/foo.md:23` — AWS Access Key (<redacted>) — Rotate the credential. Remove from source. Use the engine.env / GitHub Actions secrets mechanism to inject at runtime.
- **HIGH** `fleet.permissions.write-on-schedule` — `.github/workflows/bar.md:5` — Workflow has permissions: contents: write and on: schedule trigger — Schedule-triggered workflows with write permissions are the operational shape of a supply-chain compromise. Restrict permissions to read-only or remove the schedule trigger.
- **MEDIUM** `fleet.safe-outputs.draft-false` — `.github/workflows/baz.md:12` — safe-outputs.create-pull-request.draft is set to false — Use draft: true so PRs require human approval before transitioning to non-draft.
- **INFO** `actionlint:not-installed` — — actionlint binary not found in PATH; compiled-YAML lint scanner skipped — Install actionlint (https://github.com/rhysd/actionlint) for compiled-workflow validation. The fleet runs without it — this is graceful degradation.
```

Constraints:

- Summary line tallies severities present in the slice. Severities with zero count are OMITTED from the summary line (e.g. only HIGH findings → `**Summary**: 2 HIGH`).
- Severity order in summary line: HIGH, MEDIUM, LOW, INFO (descending).
- Per-finding bullets use the same sort order as the slice (severity desc → file asc → line asc).
- The `## Security Findings` heading is **NOT** part of `RenderForPRBody`'s output — `securityFindingsSection` in `internal/fleet/deploy.go` adds the heading. This separation lets tests assert content shape independently of placement.

## `securityFindingsSection` composer (in `internal/fleet/deploy.go`)

```go
// securityFindingsSection renders the "## Security Findings" PR-body section
// for the deploy result. Returns the empty string when there are no findings
// (caller suppresses the heading entirely per FR-005).
func securityFindingsSection(res *DeployResult) string {
    if len(res.SecurityFindings) == 0 {
        return ""
    }
    var b strings.Builder
    b.WriteString("## Security Findings\n\n")
    b.WriteString(security.RenderForPRBody(res.SecurityFindings))
    b.WriteString("\n")
    return b.String()
}
```

Called from `prBody` after `setupRequiredSection`. Same pattern in `sync.go`'s `syncPRBody` and `upgrade.go`'s `upgradePRBody` if those exist (or wherever each command composes its PR body).

## Diagnostic-code mapping (`diagCodeForRuleID`)

```go
// diagCodeForRuleID maps a Finding's RuleID to one of the nine new diagnostic
// constants in fleet/diagnostics.go (seven rule codes plus DiagSecurityActionlint
// and DiagSecurityFrontmatterParseError for the helper paths). Defensive
// fallback to DiagHint for unknown prefixes (should never happen given the
// rule table is closed).
func diagCodeForRuleID(ruleID string) string {
    switch {
    case strings.HasPrefix(ruleID, "gitleaks:"):
        return fleet.DiagSecurityCredential
    case ruleID == "fleet.permissions.write-on-schedule":
        return fleet.DiagSecurityWriteOnSchedule
    case ruleID == "fleet.safe-outputs.draft-false":
        return fleet.DiagSecurityDraftFalse
    case ruleID == "fleet.safe-outputs.missing-protected-files":
        return fleet.DiagSecurityMissingProtectedFiles
    case ruleID == "fleet.engine.env.non-allowlist":
        return fleet.DiagSecurityEngineEnvNonAllowlist
    case ruleID == "fleet.repo-memory.main-branch":
        return fleet.DiagSecurityRepoMemoryMain
    case ruleID == "fleet.mcp.non-standard-server":
        return fleet.DiagSecurityMCPNonStandardHost
    case strings.HasPrefix(ruleID, "actionlint:"):
        return fleet.DiagSecurityActionlint
    case ruleID == "fleet.frontmatter.parse-error":
        return fleet.DiagSecurityFrontmatterParseError
    default:
        return fleet.DiagHint
    }
}
```

(Note: `DiagSecurityActionlint` and `DiagSecurityFrontmatterParseError` round out the seven RULE codes to nine diagnostic constants. The research note listed seven; the contract is canonical at nine. plan.md and tasks.md (T005) are aligned to nine.)

## Backwards-compatibility contract

- Adding new diagnostic codes is **non-breaking**: the JSON envelope's `warnings[]` is open-ended; consumers gate on known codes and ignore unknown ones (existing project posture, e.g. cmd/output.go's existing pattern).
- Adding new severities (e.g. `SeverityCritical` in a future v2) **WOULD be a breaking change** to consumers that switch-statement on Severity values. Defer until needed; v1 reserves LOW and uses the other three.
- Renaming a `RuleID` is a breaking change for any consumer filtering on it (none in v1; flagged for future awareness).
- Changing the `## Security Findings` heading is a breaking change for any consumer parsing the PR body (none in v1; flagged for future awareness).
