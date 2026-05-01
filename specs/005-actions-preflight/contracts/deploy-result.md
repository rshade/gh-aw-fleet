# Contract: `DeployResult` JSON envelope, post-extension

**Feature**: `005-actions-preflight` | **Date**: 2026-04-29

This contract documents the JSON output of `gh-aw-fleet deploy <repo> --output json` after this feature lands. It is a strict superset of the spec-003 envelope contract — every field that existed before continues to exist with identical semantics. New additions are flagged.

---

## Envelope shape (top level — unchanged)

```json
{
  "schema_version": 1,
  "command": "deploy",
  "repo": "alice/widgets",
  "apply": false,
  "result": { ... see below ... },
  "warnings": [ ... see below ... ],
  "hints": [ ... see below ... ]
}
```

`schema_version` remains `1` (clarification Q1 / research R8): the additions are strictly additive and no release has shipped since the schema was introduced, so a bump would only train consumers to over-react to additive changes.

---

## `result` object — additions

Two new boolean fields appear at the top level of `result`, after the existing `secret_key_url`:

```json
{
  "result": {
    "repo": "alice/widgets",
    "clone_dir": "/tmp/gh-aw-fleet-XXXXX",
    "added": [ ... ],
    "skipped": [ ... ],
    "failed": [ ... ],
    "init_was_run": false,
    "branch_pushed": "",
    "pr_url": "",
    "missing_secret": "",
    "secret_key_url": "",

    "actions_disabled": false,         // NEW — true iff Actions is disabled on the repo
    "workflow_token_read_only": false  // NEW — true iff GITHUB_TOKEN default permission is "read"
  }
}
```

**Field semantics**:

| Field | Type | Default | Meaning |
|---|---|---|---|
| `actions_disabled` | bool | `false` | `true` if and only if `gh api /repos/<repo>/actions/permissions` returned 200 with `{"enabled": false}`. Indeterminate responses (403, 5xx, network error, missing field) leave this `false`. |
| `workflow_token_read_only` | bool | `false` | `true` if and only if `gh api /repos/<repo>/actions/permissions/workflow` returned 200 with `{"default_workflow_permissions": "read"}`. Indeterminate responses leave this `false`. |

**Backward compatibility**: Pre-existing consumers reading `result.repo`, `result.added`, etc., continue to work unchanged. Consumers that exhaustively enumerate fields will see two new fields they don't recognize; standard JSON parsers ignore unknown fields by default. Go's `encoding/json` and `jq` both treat unknown fields as benign.

---

## `warnings[]` — additions

Two new `warning` entries can appear, in this fixed order when both are active:

```json
{
  "warnings": [
    {
      "code": "actions_disabled",
      "message": "GitHub Actions is disabled on alice/widgets — enable at https://github.com/alice/widgets/settings/actions",
      "fields": {
        "url": "https://github.com/alice/widgets/settings/actions"
      }
    },
    {
      "code": "workflow_token_read_only",
      "message": "GITHUB_TOKEN is read-only on alice/widgets — workflows that push commits or create reviews will fail; set \"Workflow permissions\" → \"Read and write permissions\" at https://github.com/alice/widgets/settings/actions",
      "fields": {
        "url": "https://github.com/alice/widgets/settings/actions"
      }
    },
    {
      "code": "missing_secret",
      "message": "Actions secret \"ANTHROPIC_API_KEY\" is not set on alice/widgets; workflows will fail until added (gh secret set ANTHROPIC_API_KEY --repo alice/widgets) — obtain the key at https://console.anthropic.com/settings/keys",
      "fields": {
        "secret": "ANTHROPIC_API_KEY",
        "url": "https://console.anthropic.com/settings/keys"
      }
    }
  ]
}
```

**Code stability** (per research R4):

| Code | Stability | Filter expression |
|---|---|---|
| `actions_disabled` | NEW — stable from feature ship | `jq -e '.warnings[] \| select(.code == "actions_disabled")'` |
| `workflow_token_read_only` | NEW — stable from feature ship | `jq -e '.warnings[] \| select(.code == "workflow_token_read_only")'` |
| `missing_secret` | unchanged | `jq -e '.warnings[] \| select(.code == "missing_secret")'` |

**Order invariant** (per clarification Q2 / FR-011): When multiple warnings fire, they appear in the order Actions → token → secret. Consumers should not depend on order *between* unrelated `warnings[]` entries (this is a per-feature stable order, not a global one).

---

## `hints[]` — unchanged

No new hint codes are added by this feature. The existing hint codes (`unknown_property`, `http_404`, `gpg_failure`, etc.) continue to surface from `CollectHintDiagnostics(failedErrors...)` exactly as before.

The two new findings are **warnings**, not **hints** — they describe a precondition the operator can fix proactively, not a failed operation that needs remediation. This matches the placement of `missing_secret` in `warnings[]` (cmd/deploy.go:142). Spec FR-010 and SC-007 use `warnings[]` consistently with this contract.

---

## Stability guarantees

| Property | Guarantee |
|---|---|
| `schema_version` | Stays `1`. Will increment only on a removed field, renamed field, or type-changed field. |
| `result.actions_disabled` field name | Stable. Consumers may key on this name. |
| `result.workflow_token_read_only` field name | Stable. Consumers may key on this name. |
| `warnings[].code` values | Stable. New codes may be added (additive); existing codes will not be renamed or removed without a `schema_version` bump. |
| Within-feature order of warnings | Stable. Actions → token → secret. |
| Between-feature order of warnings | Not guaranteed. Don't rely on `warnings[0]` being any specific code. |
| Empty `warnings[]` | Always serialized as `[]`, never `null`. |

---

## Worked examples

### Healthy repo (all preflight checks pass)

```json
{
  "schema_version": 1,
  "command": "deploy",
  "repo": "alice/widgets",
  "apply": false,
  "result": {
    "repo": "alice/widgets",
    "clone_dir": "/tmp/gh-aw-fleet-abc123",
    "added": [ ... ],
    "skipped": [],
    "failed": [],
    "init_was_run": false,
    "branch_pushed": "",
    "pr_url": "",
    "missing_secret": "",
    "secret_key_url": "",
    "actions_disabled": false,
    "workflow_token_read_only": false
  },
  "warnings": [],
  "hints": []
}
```

### Restricted-token CI session (`gh api` returns 403 on settings endpoints)

Same as healthy — fail-open per Q3. Both new fields stay `false`. Debug log records the skip but doesn't surface in the envelope.

### Both new findings active

```json
{
  "schema_version": 1,
  "command": "deploy",
  "repo": "alice/widgets",
  "apply": false,
  "result": {
    "repo": "alice/widgets",
    "actions_disabled": true,
    "workflow_token_read_only": true,
    "missing_secret": "",
    "secret_key_url": ""
  },
  "warnings": [
    { "code": "actions_disabled", "message": "...", "fields": { "url": "https://github.com/alice/widgets/settings/actions" } },
    { "code": "workflow_token_read_only", "message": "...", "fields": { "url": "https://github.com/alice/widgets/settings/actions" } }
  ],
  "hints": []
}
```

(Result trimmed for brevity; non-relevant fields elided.)
