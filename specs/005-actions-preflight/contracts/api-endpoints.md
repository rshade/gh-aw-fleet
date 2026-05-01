# Contract: GitHub API endpoints consumed by the preflight

**Feature**: `005-actions-preflight` | **Date**: 2026-04-29

This contract documents the GitHub API endpoints consumed by the new `checkActionsSettings` function and the response shapes the preflight depends on. Both endpoints are read-only (no `PATCH`/`PUT`/`POST`/`DELETE`); FR-012 enforces this.

---

## Endpoint 1: `/repos/{owner}/{repo}/actions/permissions`

### Request

- **Method**: `GET`
- **Path**: `/repos/{owner}/{repo}/actions/permissions`
- **Caller**: `ghAPIJSON(ctx, fmt.Sprintf("/repos/%s/actions/permissions", repo))`
- **Auth**: Whatever scope the user's `gh auth status` token carries — the preflight does not require any specific scope. 403 on insufficient scope is gracefully handled (R3).

### Response — happy path

200 OK with body shape:

```json
{
  "enabled": true,
  "allowed_actions": "all"
}
```

The preflight reads exactly one field: `enabled`. Type-asserted as `bool`. Other fields are ignored.

### Response — disabled-Actions case

200 OK with:

```json
{
  "enabled": false
}
```

(Note: the `allowed_actions` field is documented as omitted by GitHub when `enabled: false`, but the preflight does not depend on its presence.)

### Response — error cases (all map to "indeterminate, skip")

| HTTP status | Cause | Preflight behavior |
|---|---|---|
| 200 + missing `enabled` field | API drift, partial response | Skip silently; debug log: `reason=missing_field:enabled`. |
| 200 + `enabled` is non-bool | API drift | Skip silently; debug log: `reason=type_mismatch:enabled`. |
| 200 + non-object body (string, array, null) | API drift | Skip silently; debug log: `reason=non_object_response`. |
| 401 | Token expired | Skip silently; debug log: `reason=http_401`. |
| 403 | Token lacks `repo`/`admin:repo` scope on a private repo, or org policy denies access | Skip silently; debug log: `reason=http_403`. |
| 404 | Repo doesn't exist or `gh` cannot see it | Skip silently; debug log: `reason=http_404`. (The deploy itself will fail later with its own 404 hint.) |
| 5xx | GitHub-side outage | Skip silently; debug log: `reason=http_5xx`. |
| Network error | Transport-layer failure | Skip silently; debug log: `reason=transport_error`. |
| Malformed JSON | API bug | Skip silently; debug log: `reason=malformed_json`. |

In every error case, the preflight **must not** panic, **must not** return an error to the caller, and **must not** emit a warning. The deploy proceeds as if the check did not happen.

---

## Endpoint 2: `/repos/{owner}/{repo}/actions/permissions/workflow`

### Request

- **Method**: `GET`
- **Path**: `/repos/{owner}/{repo}/actions/permissions/workflow`
- **Caller**: `ghAPIJSON(ctx, fmt.Sprintf("/repos/%s/actions/permissions/workflow", repo))`
- **Auth**: Same as endpoint 1.

### Response — happy path (write)

200 OK:

```json
{
  "default_workflow_permissions": "write",
  "can_approve_pull_request_reviews": false
}
```

The preflight reads exactly one field: `default_workflow_permissions`. Type-asserted as `string`. **`can_approve_pull_request_reviews` MUST be ignored** (FR-002): a `write` value combined with approval-disabled is still write-permitted for the purposes of this preflight.

### Response — read-only case

200 OK:

```json
{
  "default_workflow_permissions": "read",
  "can_approve_pull_request_reviews": false
}
```

### Field value space

| Value | Preflight returns |
|---|---|
| `"write"` | `false` (no warning) |
| `"read"` | `true` (warning fires) |
| Anything else (future GitHub addition, e.g., `"none"`) | Treat as indeterminate (skip silently). Preflight returns `false`; debug log: `reason=unknown_value:<value>`. |
| Field absent | Indeterminate. Skip silently; debug log: `reason=missing_field:default_workflow_permissions`. |
| Field is non-string | Indeterminate. Skip silently; debug log: `reason=type_mismatch:default_workflow_permissions`. |

### Response — error cases

Identical handling to endpoint 1 (same error → same skip → same debug log shape, with `endpoint` field naming this URL).

---

## Idempotence and rate cost

Both endpoints are idempotent (safe to call repeatedly without side effects). At one call each per `Deploy()` invocation, against typical fleet sizes (≤10 repos × occasional deploys), the rate-limit cost is negligible against the authenticated 5000 req/hr ceiling.

If a future feature fans `Deploy()` across all repos in a fleet, the per-deploy rate cost grows linearly. At fleet sizes ≥ 1000 repos this would matter, but no fleet that size exists in any current scope. No caching is needed (the Actions setting state can change at any time; cache invalidation rules are not worth the design surface).

---

## API version pinning

The preflight uses whatever default API version `gh api` resolves at call time. GitHub's REST API is generally backward-compatible within a major version. If a future version renames `enabled` or `default_workflow_permissions`, the preflight degrades gracefully (per the missing-field handling in R3 and clarification Q3): the warning stops firing, but the deploy continues to work. A subsequent feature spec would update the field-read logic.

No `Accept: application/vnd.github.v3+json` header is set explicitly — `gh api` injects an appropriate default. The preflight does not depend on a specific `Accept` value.

---

## Side-effect guarantees

- **No state-changing requests** (FR-012). The preflight emits only `GET`. No `PATCH`/`PUT`/`POST`/`DELETE` against any Actions-settings endpoint.
- **No retries** in the preflight code itself. `gh api` may have its own retry behavior; the preflight does not add a retry layer. A transient failure that surfaces as a network error becomes "indeterminate, skip" — the operator can re-run the deploy if they want a second opinion.
- **No write to disk**. The preflight writes nothing to `~/.config/gh-aw-fleet`, the work-dir, or any other on-disk location. State is transient on `DeployResult`.

---

## Observability contract

When either endpoint is queried, regardless of outcome, **exactly one** structured log line is emitted at one of these levels:

| Outcome | zerolog level | Fields |
|---|---|---|
| Healthy (200 + healthy value) | None (no log) | — |
| Positive finding (200 + disqualifying value) | None at the check site (the Warn comes from `emitDeployWarnings` later) | — |
| Indeterminate (any error path) | `Debug` | `repo`, `endpoint`, `reason` |

This means at default `--log-level info`, a healthy fleet sees **zero** Actions-settings log output. At `--log-level debug`, every call emits at most one line. The contract intentionally avoids "info" level for skip events — they are not actionable for a typical operator and would clutter normal output.
