# Contract: Renovate Conflict Scanner

This CLI tool has no network API; its external contracts are (1) the
`security.Scanner` interface the new scanner implements, (2) the stable identifiers
(rule IDs, diag code) downstream agents gate on, and (3) the observable output
across the three finding surfaces. This document is the authoritative contract for
all three.

## C1 — `security.Scanner` conformance

```go
// renovateScanner implements security.Scanner.
func (s *renovateScanner) Scan(ctx context.Context, cloneDir string) []security.Finding
```

Contract obligations (from the `Scanner` doc comment and FRs):

- **MUST NOT** modify any file in `cloneDir` (read-only — FR-009).
- **MUST NOT** panic or return an error on missing/malformed input; surface those as
  the absence of findings or a single `INFO` finding (FR-006). (`safeScan` will
  convert any escaped panic into `fleet.scanner.panic`, but this scanner must not
  rely on that net.)
- **MUST** return `nil`/empty when no Renovate config is present (FR-005).
- Registered in `defaultScanners()` so `security.Run` includes it; ordering within
  the returned slice does not matter (Run sorts findings by severity/file/line/ruleID).

## C2 — Inputs → Outputs (behavioral contract)

| # | Clone state | Findings produced |
|---|-------------|-------------------|
| 1 | No recognized Renovate config file | none |
| 2 | Config present, both rules present (any recognized form) | none |
| 3 | Config present, Rule A absent only | 1 × `LOW` `fleet.renovate.gh-aw-actions-not-disabled` |
| 4 | Config present, Rule B absent only | 1 × `LOW` `fleet.renovate.lockfile-not-disabled` |
| 5 | Config present, both absent | 2 × `LOW` (one per rule) |
| 6 | Config present, root `enabled: false` | none (Renovate disabled repo-wide) |
| 7 | Config present, JWCC comments / trailing commas, rules present | none (parsed via hujson) |
| 8 | Config present but unparseable (incl. full-JSON5-only syntax) | 1 × `INFO` `fleet.renovate.parse-error` |
| 9 | Multiple recognized config files present | only the first per probe order is inspected |

"Recognized form" for rules 2–6 is defined in research.md Decision 4 (substring
`gh-aw-actions` in a disabling package construct for Rule A; substring `.lock.yml`
in a disabling file construct for Rule B; `ignoreDeps`/`ignorePaths`/root-disable).

## C3 — Stable identifiers (downstream gating contract)

| Identifier | Value | Stability |
|------------|-------|-----------|
| Rule ID (Rule A) | `fleet.renovate.gh-aw-actions-not-disabled` | stable; in `Finding.RuleID` and `Diagnostic.Fields["rule_id"]` |
| Rule ID (Rule B) | `fleet.renovate.lockfile-not-disabled` | stable |
| Rule ID (malformed) | `fleet.renovate.parse-error` | stable |
| Diag code (family) | `security_renovate` | stable; in JSON `warnings[].code` |

`diagCodeForRuleID` maps every `fleet.renovate.`-prefixed rule ID to
`security_renovate`. No `cmd.SchemaVersion` bump — the new findings/codes are
additive within the existing `warnings[]` contract (FR-014).

## C4 — Output surface contract (no caller changes)

Findings flow through the existing pipeline unchanged. The expected rendering of a
Rule A conflict on each surface:

**Stderr** (`RenderForStderr` / `emitSecurityFindingWarnings`):

```text
[LOW] fleet.renovate.gh-aw-actions-not-disabled  renovate.json  <message>
```

**JSON envelope** (`appendFindingDiagnostics` → `ToDiagnostic`), one `warnings[]` element:

```json
{
  "code": "security_renovate",
  "message": "<message>",
  "fields": {
    "severity": "LOW",
    "rule_id": "fleet.renovate.gh-aw-actions-not-disabled",
    "file": "renovate.json",
    "line": 0,
    "remedy": "<canonical Rule A block>"
  }
}
```

**PR body** (`RenderPRSection` → `## Security Findings`), one bullet; the summary
tally now includes a `LOW` count, e.g. `**Summary**: 1 LOW`:

```text
- **LOW** `fleet.renovate.gh-aw-actions-not-disabled` — `renovate.json` — <message> — <canonical Rule A block>
```

## C5 — Remediation block contract (verbatim text)

Each conflict finding's `Remedy` quotes the matching canonical block so it is
copy-pasteable (FR-010). The exact text is in
[../research.md](../research.md) Decision 6 (Rule A and Rule B). The blocks MUST be
reproduced byte-for-byte (including the `description` strings) so an operator can
paste them directly into their `packageRules` array.

## C6 — Non-contract (explicitly out of scope)

- Does **not** auto-apply or write the remediation (detect-and-warn only).
- Does **not** resolve `extends`/preset references or `package.json` `renovate`.
- Does **not** verify Renovate is installed/enabled on the repo.
- Does **not** change deploy/sync/upgrade exit status (advisory — FR-007).
