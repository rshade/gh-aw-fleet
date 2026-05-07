# Contract: `security.Finding`

**Phase**: 1 (Design & Contracts) | **Spec**: [../spec.md](../spec.md) | **Date**: 2026-04-30

## Purpose

Defines the rich internal type that every scanner emits and that flows across all three output surfaces (stderr, JSON envelope, PR body). This is the contract the rest of the codebase consumes: any breaking change here is a breaking change to consumers of `DeployResult.SecurityFindings`.

## Type

```go
package security

type Finding struct {
    RuleID   string   // Namespaced rule identifier; required, non-empty.
    Severity Severity // One of SeverityInfo / SeverityLow / SeverityMedium / SeverityHigh.
    File     string   // Relative path within the work-dir clone; required for non-INFO findings.
    Line     int      // 1-indexed source line; 0 means "no specific line" (INFO findings).
    Message  string   // Human-readable; for credential findings MUST contain "<redacted>" not the matched literal.
    Remedy   string   // Single-sentence operator guidance; required.
}
```

## Construction contract (per scanner)

### gitleaks adapter (HIGH)

```go
finding := Finding{
    RuleID:   "gitleaks:" + gleak.RuleID,
    Severity: SeverityHigh,
    File:     filepath.Join(".github/workflows", workflowName + ".md"),
    Line:     gleak.StartLine,
    Message:  fmt.Sprintf("%s (<redacted>)", gleak.Description),
    Remedy:   "Rotate the credential. Remove from source. Use the engine.env / GitHub Actions secrets mechanism to inject at runtime.",
}
```

**MUST NOT** read or propagate `gleak.Secret` (the matched literal). FR-008a invariant. Test enforced.

### structural scanner: `fleet.permissions.write-on-schedule` (HIGH)

```go
Finding{
    RuleID:   "fleet.permissions.write-on-schedule",
    Severity: SeverityHigh,
    File:     workflowRelPath,
    Line:     permissionsBlockLine,
    Message:  fmt.Sprintf("Workflow has %s permissions and a %s trigger", scopeList, triggerName),
    Remedy:   "Schedule-triggered workflows with write permissions are the operational shape of a supply-chain compromise. Restrict permissions to read-only or remove the schedule trigger.",
}
```

### structural scanner: `fleet.safe-outputs.draft-false` (MEDIUM)

```go
Finding{
    RuleID:   "fleet.safe-outputs.draft-false",
    Severity: SeverityMedium,
    File:     workflowRelPath,
    Line:     draftLine,
    Message:  "safe-outputs.create-pull-request.draft is set to false",
    Remedy:   "Use draft: true so PRs require human approval before transitioning to non-draft.",
}
```

### structural scanner: `fleet.safe-outputs.missing-protected-files` (MEDIUM)

```go
Finding{
    RuleID:   "fleet.safe-outputs.missing-protected-files",
    Severity: SeverityMedium,
    File:     workflowRelPath,
    Line:     createPRBlockLine,
    Message:  "safe-outputs.create-pull-request block has no protected-files key",
    Remedy:   "Add a protected-files list to safe-outputs.create-pull-request to prevent the agent from modifying sensitive paths.",
}
```

### structural scanner: `fleet.engine.env.non-allowlist` (HIGH or INFO)

HIGH form (engine known, secret not in allowlist):

```go
Finding{
    RuleID:   "fleet.engine.env.non-allowlist",
    Severity: SeverityHigh,
    File:     workflowRelPath,
    Line:     envKeyLine,
    Message:  fmt.Sprintf("engine.env.%s references secret %q which is not in the %s engine allowlist (ADR-26919)", envKey, secretName, engineID),
    Remedy:   "Either remove the engine.env entry or add the secret to the engine's ADR-26919 allowlist upstream.",
}
```

INFO form (engine missing/unknown — FR-018):

```go
Finding{
    RuleID:   "fleet.engine.env.non-allowlist",
    Severity: SeverityInfo,
    File:     workflowRelPath,
    Line:     0,
    Message:  fmt.Sprintf("engine.env.non-allowlist rule skipped: engine %q is missing or not recognized", engineID),
    Remedy:   "Add an explicit `engine: <id>` to the workflow frontmatter so the rule can evaluate, or this is expected if the workflow does not target a known engine.",
}
```

### structural scanner: `fleet.repo-memory.main-branch` (HIGH)

```go
Finding{
    RuleID:   "fleet.repo-memory.main-branch",
    Severity: SeverityHigh,
    File:     workflowRelPath,
    Line:     branchNameLine,
    Message:  fmt.Sprintf("repo-memory.branch-name is %q, which is the default branch", branchName),
    Remedy:   "Set repo-memory.branch-name to a dedicated branch (e.g. agent-memory). The agent must not write to the default branch.",
}
```

### structural scanner: `fleet.mcp.non-standard-server` (HIGH)

```go
Finding{
    RuleID:   "fleet.mcp.non-standard-server",
    Severity: SeverityHigh,
    File:     workflowRelPath,
    Line:     mcpEntryLine,
    Message:  fmt.Sprintf("MCP server entry references host %q, outside the v1 allowlist {github.com, githubusercontent.com, raw.githubusercontent.com}", host),
    Remedy:   "Verify the MCP server's provenance. v1 allowlists only GitHub-served hosts to mitigate npm/registry typosquat risk. A future fleet.json allowlist extension will allow per-fleet opt-in.",
}
```

### structural scanner: malformed-frontmatter skip (INFO)

```go
Finding{
    RuleID:   "fleet.frontmatter.parse-error",
    Severity: SeverityInfo,
    File:     workflowRelPath,
    Line:     0,
    Message:  fmt.Sprintf("frontmatter could not be parsed: %v; structural rules skipped for this workflow", err),
    Remedy:   "Review the workflow's YAML frontmatter for syntax errors.",
}
```

### actionlint adapter: error (HIGH)

```go
Finding{
    RuleID:   "actionlint:" + diag.Kind,
    Severity: SeverityHigh,
    File:     lockYAMLRelPath,
    Line:     diag.Line,
    Message:  diag.Message,
    Remedy:   "Fix the workflow YAML or update the source markdown that compiled to this lock file. See actionlint documentation for rule details.",
}
```

### actionlint adapter: warning (MEDIUM)

Identical shape but `Severity: SeverityMedium`.

### actionlint adapter: missing binary (INFO, single Finding)

```go
Finding{
    RuleID:   "actionlint:not-installed",
    Severity: SeverityInfo,
    File:     "",      // No specific file
    Line:     0,
    Message:  "actionlint binary not found in PATH; compiled-YAML lint scanner skipped",
    Remedy:   "Install actionlint (https://github.com/rhysd/actionlint) for compiled-workflow validation. The fleet runs without it — this is graceful degradation.",
}
```

## Sort contract

```go
sort.SliceStable(findings, func(i, j int) bool {
    if findings[i].Severity != findings[j].Severity {
        return findings[i].Severity > findings[j].Severity   // descending
    }
    if findings[i].File != findings[j].File {
        return findings[i].File < findings[j].File           // ascending
    }
    return findings[i].Line < findings[j].Line               // ascending
})
```

`sort.SliceStable` is required (FR-011, SC-006) — `sort.Slice` does not guarantee stable order for equal-key elements.

## Projection contract: `Finding.ToDiagnostic`

```go
func (f Finding) ToDiagnostic() fleet.Diagnostic {
    return fleet.Diagnostic{
        Code:    diagCodeForRuleID(f.RuleID),  // see contracts/scanner-interface.md
        Message: f.Message,
        Fields: map[string]any{
            "severity": f.Severity.String(),    // "INFO" | "LOW" | "MEDIUM" | "HIGH"
            "rule_id":  f.RuleID,
            "file":     f.File,
            "line":     f.Line,
            "remedy":   f.Remedy,
        },
    }
}
```

The `Diagnostic.Fields` map carries everything `Finding` has that doesn't fit `Diagnostic`'s `Code`/`Message` slots. Consumers using `jq` or programmatic envelope readers can filter on `severity`, `rule_id`, `file`, etc.

## Test obligations

Every scanner adapter MUST have at least:

- One positive-case fixture (rule fires, asserts every Finding field).
- One negative-case fixture (clean input, asserts zero findings of that rule).
- For credential adapter only: a "redaction is enforced" subtest that introduces a known fake secret and asserts `!strings.Contains(finding.Message, fakeSecret)` AND `strings.Contains(finding.Message, "<redacted>")`.
- For structural rules with INFO fallback (engine.env, malformed frontmatter): a fixture that triggers the INFO path and asserts `finding.Severity == SeverityInfo`.
