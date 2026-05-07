# Phase 1 Data Model: Layer 1 Security Scanner

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-04-30

## Overview

The scanner has no persistent state. The "data model" describes the in-memory types that flow through one `security.Run` invocation: from raw workflow content on disk, through scanner adapters, into a sorted `[]Finding` on the result struct, then projected into `Diagnostic` entries for stderr/JSON envelope and into a markdown section for the PR body.

## Entity 1: `Severity` (typed int)

```go
type Severity int

const (
    SeverityInfo   Severity = 0
    SeverityLow    Severity = 1
    SeverityMedium Severity = 2
    SeverityHigh   Severity = 3
)

// String returns the severity's display name (UPPERCASE).
func (s Severity) String() string
```

| Value | Display | Used by v1? | Meaning |
|---|---|---|---|
| `SeverityInfo` | `INFO` | Yes | Scanner skip notes (missing actionlint, malformed frontmatter, unknown engine). Purely informational; never indicates a problem. |
| `SeverityLow` | `LOW` | **No (reserved)** | FR-015: model carries the slot for future detectors; no v1 rule emits LOW. |
| `SeverityMedium` | `MEDIUM` | Yes | Defensive-posture findings (`safe-outputs.draft-false`, `safe-outputs.missing-protected-files`, actionlint warnings). |
| `SeverityHigh` | `HIGH` | Yes | Security-impact findings (gitleaks matches, write-on-schedule, engine.env non-allowlist, repo-memory main, MCP non-standard host, actionlint errors). |

**Sort semantics**: Higher `Severity` int sorts first (descending). `sort.SliceStable` with `findings[i].Severity > findings[j].Severity` as the primary comparator.

## Entity 2: `Finding` (struct)

```go
type Finding struct {
    RuleID   string   // Namespaced; e.g. "gitleaks:aws-access-key" or "fleet.permissions.write-on-schedule"
    Severity Severity
    File     string   // Path within the work-dir clone, e.g. ".github/workflows/foo.md"
    Line     int      // 1-indexed; 0 means "no specific line" (rare; e.g. INFO scanner-skip findings)
    Message  string   // Human-readable; for credential findings, MUST contain "<redacted>" not the matched literal (FR-008a)
    Remedy   string   // Single-sentence guidance for the operator
}
```

### Validation rules

| Rule | Source | Enforcement |
|---|---|---|
| `RuleID` is non-empty | All scanners | Constructor of each scanner ensures this; tests assert it on every emitted Finding. |
| `RuleID` for credential scanner starts with `"gitleaks:"` | Adapter (R1) | gitleaks adapter prepends the prefix. |
| `RuleID` for structural rules starts with `"fleet."` | Adapter (R8) | rule table entries enforce this naming. |
| `RuleID` for actionlint findings starts with `"actionlint:"` | Adapter (R2) | actionlint adapter prepends the prefix. |
| `Severity == SeverityHigh` for all gitleaks findings | FR-012 | gitleaks adapter hardcodes HIGH. |
| `Message` for gitleaks findings does NOT contain the matched secret value | FR-008a | gitleaks adapter constructs Message from `f.Description + " (<redacted>)"`; never reads `f.Secret`. Test asserts `strings.Contains(msg, "<redacted>")` and `!strings.Contains(msg, knownFakeSecret)`. |
| `File` is a path relative to the work-dir clone root (not absolute) | All scanners | Adapters strip the clone-dir prefix; tests assert `!filepath.IsAbs(f.File)`. |
| `Line >= 0` | All scanners | INFO findings (no specific line) use `Line=0`; per-line findings use 1-indexed source lines. |

### State transitions

`Finding` is immutable after creation. There are no state transitions — the value is constructed by an adapter, sorted into the result slice, and read for output.

### Lifecycle

```text
[scanner adapter] → emits Finding
       ↓
[security.Run] → collects []Finding from all scanners → sorts (R6)
       ↓
[res.SecurityFindings] → stored on DeployResult / SyncResult / UpgradeResult
       ↓
       ├─→ cmd/*.go: emitDeployWarnings → zerolog.Warn per finding (stderr)
       ├─→ cmd/*.go: emitDeployEnvelope → ToDiagnostic() per finding → warnings[] (JSON)
       └─→ internal/fleet/deploy.go: securityFindingsSection(res) → PR body (only when --apply)
```

## Entity 3: `Scanner` (interface)

```go
type Scanner interface {
    // Scan walks the workflow content available on disk in the clone-dir,
    // returning zero or more findings. Implementations MUST NOT modify any
    // file. Implementations MUST tolerate missing or malformed input by
    // emitting an INFO finding rather than returning an error.
    Scan(ctx context.Context, cloneDir string) []Finding
}
```

### v1 implementations

| Type | Name | Source it reads | Severity emitted |
|---|---|---|---|
| `*gitleaksScanner` | `newGitleaksScanner()` | `.md` files under `<cloneDir>/.github/workflows/` | HIGH |
| `*structuralScanner` | `newStructuralScanner()` | YAML frontmatter of each `.md` file | HIGH or MEDIUM (per rule); INFO on malformed frontmatter or unknown engine |
| `*actionlintScanner` | `newActionlintScanner()` | `.lock.yml` files under `<cloneDir>/.github/workflows/` | HIGH (errors) or MEDIUM (warnings); INFO if binary missing |

### Adapter constructor responsibilities

- `newGitleaksScanner()` — constructs `*detect.Detector` once via `detect.NewDetectorDefaultConfig()` (or equivalent in pinned v8 API). Returns `&gitleaksScanner{detector: d}`. Constructor is called once per `security.Run` invocation; the same instance scans all workflows in that run (R1).
- `newActionlintScanner()` — calls `exec.LookPath("actionlint")`. If missing, returns a struct that emits one INFO Finding from `Scan` (graceful degradation, FR-007). If present, returns a struct that shells out per `.lock.yml` file.
- `newStructuralScanner()` — takes no engine argument. The `engine.env.non-allowlist` rule reads each workflow's `engine:` key from its own frontmatter (FR-018). If the workflow's frontmatter omits `engine:` or specifies an engine not in the ADR-26919 allowlist map, the rule emits one INFO finding for that workflow and skips itself for that workflow only.

## Entity 4: Rule (internal to `structuralScanner`)

```go
// rule is internal to package security; not exported.
type rule struct {
    ID          string                       // e.g. "fleet.permissions.write-on-schedule"
    Severity    Severity                     // HIGH or MEDIUM (no v1 rule is INFO/LOW)
    Description string                       // Used in Finding.Message
    Remedy      string                       // Used in Finding.Remedy
    Eval        func(fm map[string]any) []ruleHit
    // The engine.env.non-allowlist rule reads `engine:` from `fm` itself
    // (per-workflow); other rules ignore engine entirely.
}

type ruleHit struct {
    Line    int    // 1-indexed; 0 if not localizable
    Detail  string // Optional rule-specific detail (e.g. the offending host for MCP)
}
```

### v1 rule table (six rules)

| RuleID | Severity | When it fires |
|---|---|---|
| `fleet.permissions.write-on-schedule` | HIGH | Frontmatter has `permissions:` block with any `write` or `admin` scope AND `on:` includes `schedule` or `workflow_run`. |
| `fleet.safe-outputs.draft-false` | MEDIUM | Frontmatter has `safe-outputs.create-pull-request.draft: false`. |
| `fleet.safe-outputs.missing-protected-files` | MEDIUM | Frontmatter has `safe-outputs.create-pull-request` block AND that block has no `protected-files` key. |
| `fleet.engine.env.non-allowlist` | HIGH | Frontmatter has `engine.env.<KEY>: ${{ secrets.<NAME> }}` where `<NAME>` is not in the engine's ADR-26919 allowlist. **FR-018**: emits INFO instead and skips this rule for the workflow when engine is missing/unknown. |
| `fleet.repo-memory.main-branch` | HIGH | Frontmatter has `repo-memory.branch-name: main` or `master`. |
| `fleet.mcp.non-standard-server` | HIGH | Frontmatter has any MCP server entry whose host is outside the v1 allowlist `{github.com, githubusercontent.com, raw.githubusercontent.com}` (FR-019). |

## Entity 5: `DeployResult.SecurityFindings` (extension)

`internal/fleet/deploy.go` adds one new field:

```go
// SecurityFindings is the sorted output of security.Run; nil when the scanner
// has not run (e.g. resume-from-work-dir paths that bypass addResolvedWorkflows).
SecurityFindings []security.Finding `json:"security_findings,omitempty"`
```

`omitempty` keeps the existing JSON envelope output backwards-compatible when the slice is nil (no scanner activity).

Parallel field on `SyncResult` (in `internal/fleet/sync.go`) and `UpgradeResult` (in `internal/fleet/upgrade.go`).

## Entity 6: Cross-surface projection (Finding → Diagnostic)

For the JSON envelope's `warnings[]` array, each `Finding` projects into the existing `fleet.Diagnostic` shape:

```go
func (f Finding) ToDiagnostic() fleet.Diagnostic {
    return fleet.Diagnostic{
        Code:    diagCodeForRuleID(f.RuleID),
        Message: f.Message,
        Fields: map[string]any{
            "severity":  f.Severity.String(),
            "rule_id":   f.RuleID,
            "file":      f.File,
            "line":      f.Line,
            "remedy":    f.Remedy,
        },
    }
}
```

`diagCodeForRuleID` is a small lookup that maps rule-ID prefixes to the nine new diagnostic codes (seven rule codes plus `DiagSecurityActionlint` and `DiagSecurityFrontmatterParseError` for the helper paths). Unknown prefixes fall back to `DiagHint` (defensive — should never happen in practice, but keeps the projection total).

## Entity 7: ADR-26919 allowlist (static map)

```go
// adr26919Allowlist maps engine ID to the set of secret names the engine is
// allowed to reference via engine.env. ADR-26919 specifies that conformant
// codemods MUST call getSecretRequirementsForEngine(engine, includeSystemSecrets=false,
// includeOptional=false). The actual data lives in upstream
// github.com/github/gh-aw/pkg/constants/engine_constants.go (`EngineOptions`
// table) — pinned to commit SHA <SHA> of that file (NOT the ADR file, which
// does not transcribe the table).
// Drift is caught by TestADR26919AllowlistMatchesFixture against
// testdata/security/adr-26919-allowlist.json.
//
//nolint:gochecknoglobals // immutable allowlist table
var adr26919Allowlist = map[string]map[string]bool{
    "claude":   {"ANTHROPIC_API_KEY": true},
    "codex":    {"OPENAI_API_KEY": true, "CODEX_API_KEY": true},
    "copilot":  {"COPILOT_GITHUB_TOKEN": true},
    "gemini":   {"GEMINI_API_KEY": true},
    "opencode": {"COPILOT_GITHUB_TOKEN": true, "ANTHROPIC_API_KEY": true, "GEMINI_API_KEY": true},
    "crush":    {"COPILOT_GITHUB_TOKEN": true, "ANTHROPIC_API_KEY": true, "GEMINI_API_KEY": true},
}
```

The map is private to `internal/fleet/security/`. Its content is derived from upstream `pkg/constants/engine_constants.go`'s `EngineOptions` table (each engine's `SecretName` plus `AlternativeSecrets`), recorded in `testdata/security/adr-26919-allowlist.json`, then transcribed into the Go map. The drift-detection test asserts the JSON fixture and the Go map agree byte-for-byte after marshal-roundtrip; updating one without the other fails CI.

## Relationship diagram

```text
                ┌──────────────────────┐
                │  security.Run(ctx,   │
                │  cloneDir)           │
                └──────────┬───────────┘
                           │ instantiates
                ┌──────────┴────────────┐
                ▼          ▼            ▼
      ┌─────────────┐ ┌──────────┐ ┌──────────────┐
      │ gitleaks    │ │ structural│ │ actionlint   │
      │ Scanner     │ │ Scanner   │ │ Scanner      │
      └─────┬───────┘ └─────┬─────┘ └──────┬───────┘
            │ Scan          │ Scan        │ Scan
            ▼               ▼             ▼
      ┌──────────────────────────────────────────────┐
      │  []Finding  (concatenated, then sorted by R6)│
      └──────────────────┬───────────────────────────┘
                         │
                         ▼
      ┌──────────────────────────────────────────────┐
      │  DeployResult.SecurityFindings               │
      └──────────────────┬───────────────────────────┘
                         │
        ┌────────────────┼────────────────────────┐
        ▼                ▼                        ▼
  ┌──────────┐    ┌──────────────┐    ┌────────────────────────┐
  │ stderr   │    │ JSON envelope│    │ PR body                │
  │ (zerolog │    │ warnings[]   │    │ ## Security Findings   │
  │  .Warn)  │    │ (Diagnostic) │    │ section (--apply only) │
  └──────────┘    └──────────────┘    └────────────────────────┘
```

## Out-of-scope at v1

- **Persisted findings** — no DB, no baseline file, no `.gh-aw-fleet/security-baseline.json`.
- **Suppression mechanism** — no `# fleet:disable security_xxx` comments. Operators read findings; the system does not let them silence findings yet.
- **Custom rules / per-fleet rule packs** — the rule table is hardcoded for v1.
- **Cross-workflow rules** — every rule in v1 evaluates a single workflow. Cross-workflow patterns (e.g. "two workflows reference the same secret with conflicting permission shapes") are deferred to Layer 3.
- **Severity per-rule configurability** — every rule has a fixed severity. Operators cannot demote `permissions.write-on-schedule` from HIGH to MEDIUM.
