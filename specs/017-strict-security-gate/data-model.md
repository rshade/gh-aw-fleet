# Phase 1 Data Model: Strict Security Gate

This feature introduces no persistent fleet configuration and no new scanner output
type. It adds invocation-level options and one transient strict gate decision that
consumes existing `security.Finding` values.

## Entity 1 - Security options

`SecurityOpts` is an invocation-scoped grouping carried by `DeployOpts`, `SyncOpts`,
and `UpgradeOpts`.

| Field | Type | Validation | Notes |
|-------|------|------------|-------|
| `Strict` | `bool` | default false | When true, HIGH non-`promptinj:` findings block before mutation. |

Relationships:

- `DeployOpts.Security SecurityOpts`
- `SyncOpts.Security SecurityOpts`
- `UpgradeOpts.Security SecurityOpts`

Rules:

- Must not be read from or written to `fleet.json` / `fleet.local.json`.
- Must not alter scanner execution or finding content.
- Must not alter compile-strict resolution fields (`CompileStrictApplied`,
  `CompileStrictEffective`, `CompileStrictSource`).

## Entity 2 - Security finding (existing)

The gate consumes the existing `security.Finding` type:

| Field | Gate use |
|-------|----------|
| `RuleID` | Exempt when prefixed with `promptinj:`. |
| `Severity` | Blocks only when `SeverityHigh`. |
| `File` | Copied into `findings.json`; useful for operator inspection. |
| `Line` | Copied into `findings.json`; useful for operator inspection. |
| `Message` | Rendered by existing stderr/JSON surfaces and copied into breadcrumb. |
| `Remedy` | Rendered by existing stderr/JSON surfaces and copied into breadcrumb. |

Blocking predicate:

```text
Severity == HIGH AND RuleID does not start with "promptinj:"
```

Validation:

- One blocking finding is sufficient.
- MEDIUM, LOW, and INFO never block.
- HIGH `promptinj:` findings never block.
- The full finding set and ordering must remain exactly what `security.Run` returns.

## Entity 3 - Strict gate decision

A per-repo evaluation performed after scanner output is available.

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| `Strict` | `bool` | `SecurityOpts.Strict` | False means always proceed. |
| `Findings` | `[]security.Finding` | command result | All findings from current run. |
| `BlockingFindings` | `[]security.Finding` | derived | HIGH non-`promptinj:` findings. |
| `BreadcrumbPath` | `string` | derived from clone dir | `<clone>/findings.json` when blocked. |
| `Outcome` | enum-like | derived | `proceed` or `abort`. |

State flow:

```text
scanner did not run
  -> findings nil
  -> no strict decision possible; proceed

scanner ran clean
  -> findings []
  -> blocking []
  -> proceed

scanner returned lower-severity findings only
  -> blocking []
  -> proceed

scanner returned HIGH promptinj findings only
  -> blocking []
  -> proceed

scanner returned >=1 HIGH non-promptinj finding and strict=false
  -> advisory only
  -> proceed

scanner returned >=1 HIGH non-promptinj finding and strict=true
  -> write findings.json
  -> preserve clone
  -> abort with StrictSecurityError
```

## Entity 4 - Strict security error

A typed error returned when the gate blocks.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `Repo` | `string` | yes | Owner/name for actionable output. |
| `BlockingCount` | `int` | yes | Must be >=1. |
| `BlockingFindings` | `[]security.Finding` | yes | High non-`promptinj:` findings only. |
| `BreadcrumbPath` | `string` | yes | Path to written `findings.json`. |

Error message contract:

```text
strict security gate blocked <N> HIGH Layer 1 finding(s) for <repo>; fix the findings or re-run without --strict to proceed advisory-only (findings saved to <path>)
```

Validation:

- Must include blocking count.
- Must include unblock path: fix findings or re-run without `--strict`.
- Must include breadcrumb path when the file was written.
- Must wrap or clearly report breadcrumb write failures without hiding the blocking
  count.

## Entity 5 - Findings breadcrumb

`findings.json` is a transient failure artifact written at the clone root when
strict blocks.

| Attribute | Value |
|-----------|-------|
| Path | `<clone>/findings.json` |
| Format | JSON array |
| Element type | existing `security.Finding` JSON shape |
| Contents | all findings from the run, not just blockers |
| Lifetime | preserved with the work-dir clone after strict abort |

Example:

```json
[
  {
    "rule_id": "gitleaks:aws-access-key",
    "severity": 3,
    "file": ".github/workflows/daily-scan.md",
    "line": 12,
    "message": "AWS Access Key (<redacted>)",
    "remedy": "Rotate the credential. Remove from source. Use the engine.env / GitHub Actions secrets mechanism to inject at runtime."
  }
]
```

Note: `Severity` currently marshals as its numeric typed-int value in the existing
`Finding` JSON shape. The feature does not change that representation.
