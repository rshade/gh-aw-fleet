# Contract: Dependabot Conflict Scanner

This CLI tool has no network API; its external contracts are (1) the
`security.Scanner` interface the new scanner implements, (2) the stable identifiers
(rule IDs, diag code) downstream agents gate on, and (3) the observable output across
the three finding surfaces. This document is the authoritative contract for all three.

## C1 — `security.Scanner` conformance

```go
// dependabotScanner implements security.Scanner.
// ctx is unused, so it takes the `_` name like the sibling scanners (renovate.go, gitleaks.go).
func (s *dependabotScanner) Scan(_ context.Context, cloneDir string) []security.Finding
```

Contract obligations (from the `Scanner` doc comment and FRs):

- **MUST NOT** modify any file in `cloneDir` (read-only — FR-009).
- **MUST NOT** panic or return an error on missing/malformed input; surface those as
  the absence of findings or a single `INFO` finding (FR-006). (`safeScan` will convert
  any escaped panic into `fleet.scanner.panic`, but this scanner must not rely on that
  net.)
- **MUST** return `nil`/empty when no Dependabot config is present (FR-005) **and**
  when a present config has no `github-actions` ecosystem entry (FR-005).
- Registered in `defaultScanners()` so `security.Run` includes it; ordering within the
  returned slice does not matter (Run sorts findings by severity/file/line/ruleID/
  message/remedy).

## C2 — Inputs → Outputs (behavioral contract)

| # | Clone state | Findings produced |
|---|-------------|-------------------|
| 1 | No `.github/dependabot.yml` / `.yaml` | none |
| 2 | Config present, no `github-actions` ecosystem entry (e.g. gomod-only) | none |
| 3 | `github-actions` entry, `ignore` covers gh-aw family (exact names or wildcard) | none |
| 4 | `github-actions` entry, `open-pull-requests-limit: 0` | none (cannot open bump PRs) |
| 5 | `github-actions` entry, no covering ignore and limit ≠ 0 (no ignore block, **or** an ignore that covers only unrelated deps like `actions/checkout`) | 1 × `LOW` `fleet.dependabot.gh-aw-actions-not-ignored` |
| 6 | Two `github-actions` entries, both unprotected | 2 × `LOW` (one per entry, labeled by `directory`) |
| 7 | Two `github-actions` entries, one protected + one not | 1 × `LOW` (the unprotected one) |
| 8 | Config present but unparseable YAML (syntax error) | 1 × `INFO` `fleet.dependabot.parse-error` |
| 9 | Both `.yml` and `.yaml` present | only the first per probe order (`.yml`) is inspected |

"Covers the gh-aw family" for rows 3–7 is defined in research.md Decision 4 (substring
`gh-aw` in any `ignore[].dependency-name`; or `open-pull-requests-limit: 0`).

## C3 — Stable identifiers (downstream gating contract)

| Identifier | Value | Stability |
|------------|-------|-----------|
| Rule ID (conflict) | `fleet.dependabot.gh-aw-actions-not-ignored` | stable; in `Finding.RuleID` and `Diagnostic.Fields["rule_id"]` |
| Rule ID (malformed) | `fleet.dependabot.parse-error` | stable |
| Diag code (family) | `security_dependabot` | stable; in JSON `warnings[].code` |

`diagCodeForRuleID` maps every `fleet.dependabot.`-prefixed rule ID to
`security_dependabot`. No `cmd.SchemaVersion` bump — the new findings/codes are
additive within the existing `warnings[]` contract (FR-014).

## C4 — Output surface contract (no caller changes)

Findings flow through the existing pipeline unchanged. The expected rendering of the
conflict finding on each surface:

**Stderr** (`RenderForStderr` / `emitSecurityFindingWarnings`):

```text
[LOW] fleet.dependabot.gh-aw-actions-not-ignored  .github/dependabot.yml  <message>
```

**JSON envelope** (`appendFindingDiagnostics` → `ToDiagnostic`), one `warnings[]` element:

```json
{
  "code": "security_dependabot",
  "message": "<message>",
  "fields": {
    "severity": "LOW",
    "rule_id": "fleet.dependabot.gh-aw-actions-not-ignored",
    "file": ".github/dependabot.yml",
    "line": 0,
    "remedy": "<canonical ignore block + name-only caveat>"
  }
}
```

**PR body** (`RenderPRSection` → `## Security Findings`), one bullet; the summary tally
includes a `LOW` count, e.g. `**Summary**: 1 LOW`:

```text
- **LOW** `fleet.dependabot.gh-aw-actions-not-ignored` — `.github/dependabot.yml` — <message> — <canonical ignore block + caveat>
```

## C5 — Remediation block contract (verbatim text)

The conflict finding's `Remedy` quotes the canonical `ignore:` block **and** the
name-only caveat so it is copy-pasteable and self-explanatory (FR-010 + FR-004). The
exact text is in [../research.md](../research.md) Decision 7. The `ignore:` block MUST
be reproduced so an operator can paste it under their existing `github-actions` entry,
and the caveat MUST state that Dependabot has no file-glob equivalent to the Renovate
lock-file exclusion (name-only protection).

## C6 — Non-contract (explicitly out of scope)

- Does **not** auto-apply or write the remediation (detect-and-warn only).
- Does **not** resolve organization-level Dependabot configuration.
- Does **not** verify Dependabot is enabled on the repo.
- Does **not** attempt a file-glob lock-file guard — Dependabot does not support one
  (the asymmetry); the scanner educates the operator instead (FR-004).
- Does **not** change deploy/sync/upgrade exit status (advisory — FR-007).
