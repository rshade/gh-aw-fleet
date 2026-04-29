# Contract: `status` JSON Envelope

**Feature**: `004-status-drift-detection` | **Date**: 2026-04-28

This document is the JSON wire contract for `gh-aw-fleet status -o json`. It extends the envelope contract from spec 003 (`003-cli-output-json`) — the top-level keys (`schema_version`, `command`, `repo`, `apply`, `result`, `warnings`, `hints`) are inherited verbatim. This document specifies the `command` value, the `result` shape (StatusResult), and the failure semantics specific to status.

---

## Envelope

The envelope shape, key order, and emission rules are inherited from spec 003 / `cmd/output.go:29` (`Envelope` struct). Status fills it as follows:

```json
{
  "schema_version": 1,
  "command": "status",
  "repo": "",
  "apply": false,
  "result": { "repos": [...] },
  "warnings": [],
  "hints": []
}
```

| Field | Type | Status's value |
|---|---|---|
| `schema_version` | integer | `1` (unchanged from spec 003 — status is additive). |
| `command` | string | `"status"` (literal, lowercase). |
| `repo` | string | The single-repo positional arg if supplied (`"owner/name"`); empty string `""` for fleet-wide runs. |
| `apply` | boolean | Always `false`. Status is read-only. |
| `result` | object \| null | A `StatusResult` object on success; `null` on pre-result failure (see below). |
| `warnings` | array of `Diagnostic` | Empty `[]` when no warnings. Populated for empty fleet, redirected repos, etc. |
| `hints` | array of `Diagnostic` | Empty `[]` when no hints. Populated for repo inaccessibility, rate limits, etc. |

---

## `result.StatusResult`

```json
{
  "repos": [ /* RepoStatus objects */ ]
}
```

- `repos`: array of `RepoStatus`. MUST always be present. Empty fleet → `[]` (with a warning in `warnings[]`).
- Order: alphabetical by `repo` field.

---

## `result.repos[i].RepoStatus`

```json
{
  "repo": "rshade/gh-aw-fleet",
  "drift_state": "aligned",
  "missing": [],
  "extra": [],
  "drifted": [],
  "unpinned": [],
  "error_message": ""
}
```

| Field | Type | Constraints |
|---|---|---|
| `repo` | string | Canonical name from `fleet.json` (`owner/name`). |
| `drift_state` | string | One of: `"aligned"`, `"drifted"`, `"errored"`. Closed set; consumers gating on this MAY assume no other values. |
| `missing` | array of string | Workflow names (no `.md` extension) declared in fleet.json but absent from the repo. Always present — empty `[]` (never `null`). |
| `extra` | array of string | Workflow names present in the repo's `.github/workflows/*.md` listing AND with parseable `source:` frontmatter AND not declared in fleet.json. Always present. |
| `drifted` | array of object | One `WorkflowDrift` per drifted workflow. See below. Always present. |
| `unpinned` | array of string | Workflow names declared in fleet.json AND present in the repo BUT lacking parseable `source:` frontmatter (missing field, malformed YAML, non-string value). Always present. |
| `error_message` | string | Empty `""` unless `drift_state == "errored"`. When errored, contains a one-line human-readable description (e.g., `"HTTP 404: repository not found or inaccessible"`). |

**State derivation (informative summary — canonical rule lives in `data-model.md` §Type 3):**

```text
if error_message != "":      drift_state = "errored"
elif missing+extra+drifted+unpinned all empty:  drift_state = "aligned"
else:                        drift_state = "drifted"
```

A workflow appears in **at most one** of the four drift slices. `aligned` is implicit (no slice for "aligned workflows" — operators count via the absence). When the canonical rule and this summary disagree, `data-model.md` wins.

---

## `result.repos[i].drifted[j].WorkflowDrift`

```json
{
  "name": "audit",
  "desired_ref": "v0.68.3",
  "actual_ref": "v0.67.0"
}
```

| Field | Type | Constraints |
|---|---|---|
| `name` | string | Workflow basename without `.md` (e.g., `audit`, not `audit.md`). |
| `desired_ref` | string | Literal ref string from fleet.json's profile source pin (e.g., `v0.68.3`, `main`, `abc123`). |
| `actual_ref` | string | Literal ref segment of the installed `source:` frontmatter (the part after `@`). May be a tag, branch, or SHA. |

**Comparison semantics**: Strict string equality (per clarification 3 / FR-004). `desired_ref == actual_ref` for a workflow means it's `aligned` and MUST NOT appear here. The follow-up issue **#62** tracks the SHA-resolution variant if false positives become operator pain.

---

## `Diagnostic` (warnings[] and hints[] entries)

Inherited verbatim from spec 003 / `internal/fleet/diagnostics.go:13`:

```json
{
  "code": "rate_limited",
  "message": "GitHub API rate limit exceeded. Wait until the limit resets, or rotate to a different token.",
  "fields": { "repo": "rshade/private-thing" }
}
```

| Field | Type | Constraints |
|---|---|---|
| `code` | string | Stable snake_case identifier. Status's emitted codes: `repo_inaccessible`, `rate_limited`, `network_unreachable`, `empty_fleet`, plus any matched by the existing `hints` table (`http_404`, `unknown_property`, etc.). |
| `message` | string | Human-readable. |
| `fields` | object \| absent | Optional structured context. Status populates `fields.repo` for per-repo failures by constructing the `Diagnostic` directly at the call site (`Fields: {"repo": <owner/name>}`). `CollectHintDiagnostics` is reserved for substring-matched hints derived from `gh api` output — it always emits `Fields: {"hint": <message>}` and does NOT accept augmentation; status does not route per-repo errors through it. |

---

## Emitted diagnostic codes (status-specific)

| Code | Class | Trigger |
|---|---|---|
| `repo_inaccessible` | hint | `gh api /repos/<owner>/<name>/contents/...` returns HTTP 404 or 403 against the repo root. |
| `rate_limited` | hint | Substring match `"API rate limit exceeded"` in `gh api` stderr. Header-based detection (`X-RateLimit-Remaining: 0`) is NOT used in this release — the existing `ghAPIRaw`/`ghAPIJSON` wrappers do not expose response headers. |
| `network_unreachable` | hint | `gh api` exec failure with substring `"Could not resolve host"` or similar network-class message. *(Optional in v1; the generic `HintFromError` fallback adequately covers this.)* |
| `empty_fleet` | warning | The loaded fleet config has zero repos. |

> **Note**: Redirect detection (`redirect_followed`) is deferred to a follow-up issue. See spec Edge Cases — surfacing the rename requires an extra `/repos/<owner>/<name>` call per repo plus surfacing the canonical name from the API response, neither of which the current fetch wrappers expose.

---

## Pre-result failure envelope

When the command fails before it can construct a `StatusResult` (config parse error, missing tool, repo arg not in fleet, fleet config not loadable), JSON mode still emits exactly one envelope on stdout — `result: null`, the cause in `hints[]` (via `preResultFailureEnvelope` from `cmd/output.go:137`), and exit code non-zero:

```json
{
  "schema_version": 1,
  "command": "status",
  "repo": "rshade/gh-aw-fleet",
  "apply": false,
  "result": null,
  "warnings": [],
  "hints": [
    {
      "code": "hint",
      "message": "repo \"rshade/gh-aw-fleet\" is not declared in fleet config",
      "fields": { "hint": "repo \"rshade/gh-aw-fleet\" is not declared in fleet config" }
    }
  ]
}
```

The `repo` field reflects what the user passed (so they can correlate); it MAY be empty if the failure happened before argument parsing. The `apply` field is always `false` for status.

---

## Stability guarantees

What this contract promises NOT to change without bumping `schema_version`:

- Top-level envelope keys (`schema_version`, `command`, `repo`, `apply`, `result`, `warnings`, `hints`).
- The closed set of `drift_state` values: `aligned`, `drifted`, `errored`.
- The four drift slice fields on `RepoStatus`: `missing`, `extra`, `drifted`, `unpinned`.
- The three fields on `WorkflowDrift`: `name`, `desired_ref`, `actual_ref`.
- Empty-array semantics: every slice MUST serialize as `[]`, never `null`.
- The presence (always) of `error_message` (empty when not errored).

What this contract permits to evolve **without** a `schema_version` bump (additive changes):

- New fields with `omitempty` JSON tags appended to existing structs.
- New diagnostic codes added to the codes table (but always with `code` strings; consumers should not assume the closed set unless gating on a specific code).
- New top-level fields on `StatusResult` (e.g., aggregate counts) added with `omitempty`.

What this contract forbids:

- Renaming any specified field.
- Changing the type of any specified field.
- Removing any specified field.
- Introducing a fourth `drift_state` value without a major version bump.
- Stringifying a nested object (`audit_json` style mistake from a different envelope — not applicable here, but the principle stands).
