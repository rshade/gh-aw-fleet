# Phase 1 Data Model: Dependabot Config Conflict Scanner

This feature introduces **no persistent storage** and **no new wire types**. The
"data model" is (a) the minimal in-memory shape the scanner unmarshals from a
Dependabot config and (b) the mapping from detected gaps onto the existing
`security.Finding` type. Nothing here changes the JSON output envelope schema.

## Entity 1 — Dependabot configuration (parsed, read-only)

The discovered config file is read and `yaml.Unmarshal`ed into a minimal struct that
captures only the fields detection reads. Unknown fields are ignored (default
`yaml.v3` behavior — do **not** enable `KnownFields`).

`dependabotConfig` (the document root):

| Field (YAML key) | Parsed type | Used by | Notes |
|------------------|-------------|---------|-------|
| `version` | `int` | (none) | Captured for completeness; detection does not gate on it. |
| `updates` | `[]dependabotUpdate` | conflict rule | The list of update entries; each `github-actions` entry is evaluated independently. |

`dependabotUpdate` (one element of `updates`):

| Field (YAML key) | Parsed type | Used by | Notes |
|------------------|-------------|---------|-------|
| `package-ecosystem` | `string` | gate | Only entries equal to `github-actions` are evaluated. |
| `directory` | `string` | finding label | Names the entry in the finding `Message`; classic single-dir form. |
| `directories` | `[]string` | finding label | Newer multi-directory/glob form; used for the label when `directory` is absent. |
| `open-pull-requests-limit` | `*int` | protection | Pointer to distinguish absent from `0`; `0` ⇒ entry cannot open bump PRs ⇒ protected. |
| `ignore` | `[]dependabotIgnore` | protection | Substring `gh-aw` in any `dependency-name` ⇒ protected. |

`dependabotIgnore` (one element of an entry's `ignore`):

| Field (YAML key) | Parsed type | Used by | Notes |
|------------------|-------------|---------|-------|
| `dependency-name` | `string` | protection | Substring `gh-aw` ⇒ entry protected. Dependabot supports `*` wildcards here. |
| `versions` | `[]string` | (none) | Part of the schema; not read by detection. |

**Parsing notes**:
- A YAML *syntax* failure short-circuits to the malformed-config `INFO` finding (see
  Finding mapping below). A structurally-valid file that simply has no
  `github-actions` entry is **not** malformed — it produces no finding (FR-005).
- `package-ecosystem` is compared exactly to the literal `github-actions` (the
  canonical Dependabot ecosystem identifier). No normalization is needed.

## Entity 2 — Conflict rule (compile-time constant, ONE instance)

Unlike the Renovate sibling (two rules), Dependabot has exactly **one** conflict rule
— Dependabot cannot ignore by file glob, so there is no `*.lock.yml`-exclusion analog
(research.md Decision 5). The rule is expressed in code as the detection function plus
the canonical remediation block.

| Attribute | Rule (only) |
|-----------|-------------|
| Identity | "gh-aw action family not ignored on a github-actions entry" |
| Gate | update entry with `package-ecosystem: github-actions` |
| Marker substring | `gh-aw` (lineage covering all three identifiers) |
| Protected when | any `ignore[].dependency-name` contains `gh-aw`, **or** `open-pull-requests-limit == 0` |
| Granularity | per unprotected `github-actions` entry (one finding each) |
| Rule ID (proposed) | `fleet.dependabot.gh-aw-actions-not-ignored` |
| Severity | `LOW` |
| Remedy | canonical `ignore:` block + name-only caveat (research.md Decision 7) |

## Entity 3 — Finding (existing type, reused — `security.Finding`)

No change to the type. Field population for this scanner:

| `Finding` field | Value for the conflict | Value for malformed config |
|-----------------|------------------------|----------------------------|
| `RuleID` | `fleet.dependabot.gh-aw-actions-not-ignored` | `fleet.dependabot.parse-error` |
| `Severity` | `SeverityLow` | `SeverityInfo` |
| `File` | the discovered config path, clone-relative, slash form (`.github/dependabot.yml` or `.yaml`) | same discovered path |
| `Line` | `0` (no line localization in v1) | `0` |
| `Message` | states the `github-actions` entry (by `directory`) does not ignore the gh-aw family and why it matters | "Dependabot config could not be parsed: <err>; conflict checks skipped for this file" |
| `Remedy` | canonical `ignore:` block + the name-only caveat (copy-pasteable) | "Review the Dependabot config for YAML syntax errors." |

> Embedding the entry's `directory` in `Message` keeps multiple per-entry findings
> distinct under `Run`'s stable sort (which orders by severity, file, line, ruleID,
> then message) even though they share `File`, `Line`, and `RuleID`.

### State / flow

```text
probe .github/dependabot.yml then .github/dependabot.yaml (Decision 1)
  └─ none found ─────────────────────────────────► return [] (no findings)         [FR-005]
  └─ found path P, read bytes
       └─ yaml.Unmarshal fails (syntax) ──────────► [INFO parse-error @ P]          [FR-006]
       └─ parsed OK
            └─ for each updates[] entry where package-ecosystem == "github-actions":
                 ├─ open-pull-requests-limit == 0 ─► protected, skip
                 ├─ any ignore[].dependency-name contains "gh-aw" ─► protected, skip
                 └─ else ──────────────────────────► append LOW gh-aw-actions-not-ignored @ P (dir)  [FR-003]
            └─ no github-actions entry at all ─────► return [] (no findings)        [FR-005]
```

One unprotected `github-actions` entry ⇒ one finding (SC-001). A protected entry, a
gomod-only config, or no config ⇒ zero (SC-002/SC-003/SC-004). Two unprotected
`github-actions` entries ⇒ two findings (per-entry, Decision 5). The scanner never
returns an error and never mutates a file (FR-007/FR-009); it conforms to
`security.Scanner` and is invoked by `security.Run` via `defaultScanners()` — so
findings sort and surface alongside the other scanners' on all three output surfaces
with no caller changes.

## New stable identifiers introduced

| Kind | Identifier | Location |
|------|-----------|----------|
| Rule ID | `fleet.dependabot.gh-aw-actions-not-ignored` | `constants.go` |
| Rule ID | `fleet.dependabot.parse-error` | `constants.go` |
| Rule-ID prefix | `fleet.dependabot.` (`rulePrefixDependabot`) | `constants.go` |
| Diag code | `security_dependabot` (`DiagSecurityDependabot`) | `fleetdiag/diag.go` + mirrored in `diagnostics.go` |

**Diag-code granularity decision**: one diag code (`security_dependabot`) covers the
whole rule family, consistent with the existing convention ("Security_* entries are
one code per rule family; per-rule granularity rides on `Diagnostic.Fields["rule_id"]`"
— `fleetdiag/diag.go`). `diagCodeForRuleID` maps any `fleet.dependabot.`-prefixed rule
ID to `DiagSecurityDependabot`.
