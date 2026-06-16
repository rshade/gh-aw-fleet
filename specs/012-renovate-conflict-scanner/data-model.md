# Phase 1 Data Model: Renovate Config Conflict Scanner

This feature introduces **no persistent storage** and **no new wire types**. The
"data model" is (a) the minimal in-memory shape the scanner unmarshals from a
Renovate config and (b) the mapping from detected gaps onto the existing
`security.Finding` type. Nothing here changes the JSON output envelope schema.

## Entity 1 — Renovate configuration (parsed, read-only)

The discovered config file is standardized (`hujson.Standardize`) and unmarshaled
into a minimal struct that captures only the fields detection reads. Unknown fields
are ignored (standard `encoding/json` behavior).

| Field (JSON key) | Parsed type | Used by | Notes |
|------------------|-------------|---------|-------|
| `enabled` | `*bool` | Rule A & B | Root-level `false` ⇒ Renovate off ⇒ both rules satisfied. Pointer to distinguish absent from `false`. |
| `ignoreDeps` | `[]string` | Rule A | Substring `gh-aw-actions` in any entry ⇒ Rule A present. |
| `ignorePaths` | `[]string` | Rule B | Substring `.lock.yml` in any entry ⇒ Rule B present. |
| `packageRules` | `[]packageRule` | Rule A & B | Each evaluated for `enabled:false` + matching matcher. |

`packageRule` (one array element):

| Field (JSON key) | Parsed type | Used by | Notes |
|------------------|-------------|---------|-------|
| `enabled` | `*bool` | A & B | The disable signal; only `false` counts. |
| `matchPackageNames` | `[]string` | A | Substring `gh-aw-actions` ⇒ matches Rule A package set. |
| `matchPackagePatterns` | `[]string` | A | Deprecated equivalent; same substring check. |
| `matchPackagePrefixes` | `[]string` | A | Deprecated equivalent; same substring check. |
| `matchDepNames` | `[]string` | A | Equivalent; same substring check. |
| `matchDepPatterns` | `[]string` | A | Equivalent; same substring check. |
| `matchFileNames` | `[]string` | B | Substring `.lock.yml` ⇒ matches Rule B file set. |
| `matchPaths` | `[]string` | B | Deprecated predecessor of `matchFileNames`; same check. |

**Parsing notes**:
- Renovate matchers accept both a JSON array and (historically) a single string;
  the unmarshal target should tolerate both (custom `UnmarshalJSON` or
  `[]string`-or-`string` normalization) so a scalar matcher does not become a parse
  error that masquerades as a malformed config.
- A standardize/unmarshal failure is **not** modeled here — it short-circuits to the
  malformed-config `INFO` finding (see Finding mapping below).

## Entity 2 — Conflict rule (compile-time constant, two instances)

Each required policy is a static definition the scanner checks for. Not persisted;
expressed in code as the detection functions + the canonical remediation block.

| Attribute | Rule A | Rule B |
|-----------|--------|--------|
| Identity | "gh-aw-actions updates disabled" | "generated lock files excluded" |
| Marker substring | `gh-aw-actions` | `.lock.yml` |
| Where checked | package matchers (`enabled:false`), `ignoreDeps`, root `enabled:false` | file matchers (`enabled:false`), `ignorePaths`, root `enabled:false` |
| Rule ID (proposed) | `fleet.renovate.gh-aw-actions-not-disabled` | `fleet.renovate.lockfile-not-disabled` |
| Severity | `LOW` | `LOW` |
| Remedy | Rule A canonical block (research.md Decision 6) | Rule B canonical block (research.md Decision 6) |

## Entity 3 — Finding (existing type, reused — `security.Finding`)

No change to the type. Field population for this scanner:

| `Finding` field | Value for Rule A/B conflict | Value for malformed config |
|-----------------|------------------------------|----------------------------|
| `RuleID` | `fleet.renovate.gh-aw-actions-not-disabled` / `fleet.renovate.lockfile-not-disabled` | `fleet.renovate.parse-error` |
| `Severity` | `SeverityLow` | `SeverityInfo` |
| `File` | the discovered config path, clone-relative, slash form (e.g. `renovate.json`, `.github/renovate.json`) | same discovered path |
| `Line` | `0` (no line localization in v1) | `0` |
| `Message` | states which rule is missing and why it matters | "Renovate config could not be parsed: <err>; conflict checks skipped for this file" |
| `Remedy` | the canonical block for that rule (copy-pasteable) | "Review the Renovate config for JSON syntax errors." |

### State / flow

```text
probe config paths (Decision 1)
  └─ none found ────────────────────────────────► return [] (no findings)        [FR-005]
  └─ found path P, read bytes
       └─ hujson.Standardize + json.Unmarshal fails ─► [INFO parse-error @ P]     [FR-006]
       └─ parsed OK
            ├─ root enabled == false ─────────────► return [] (Renovate off)
            ├─ Rule A intent absent ──────────────► append LOW gh-aw-actions @ P  [FR-003]
            └─ Rule B intent absent ──────────────► append LOW lockfile @ P       [FR-004]
```

Both rules absent ⇒ two findings (SC-001). Both present (any recognized form) ⇒ zero
(SC-002). The scanner never returns an error and never mutates a file (FR-007/FR-009);
it conforms to `security.Scanner` and is invoked by `security.Run` via
`defaultScanners()` — so findings sort and surface alongside the other scanners' on
all three output surfaces with no caller changes.

## New stable identifiers introduced

| Kind | Identifier | Location |
|------|-----------|----------|
| Rule ID | `fleet.renovate.gh-aw-actions-not-disabled` | `constants.go` |
| Rule ID | `fleet.renovate.lockfile-not-disabled` | `constants.go` |
| Rule ID | `fleet.renovate.parse-error` | `constants.go` |
| Rule-ID prefix | `fleet.renovate.` (`rulePrefixRenovate`) | `constants.go` |
| Diag code | `security_renovate` (`DiagSecurityRenovate`) | `fleetdiag/diag.go` + mirrored in `diagnostics.go` |

**Diag-code granularity decision**: one diag code (`security_renovate`) covers the
whole rule family, consistent with the existing convention ("Security_* entries are
one code per rule family; per-rule granularity rides on `Diagnostic.Fields["rule_id"]`"
— `fleetdiag/diag.go`). `diagCodeForRuleID` maps any `fleet.renovate.`-prefixed rule
ID to `DiagSecurityRenovate`.
