# Phase 1 Data Model: Deploy Preflight for Actions Enabled and Workflow Write Permissions

**Feature**: `005-actions-preflight` | **Date**: 2026-04-29 | **Plan**: [plan.md](./plan.md)

This feature introduces **no new types**. It extends one existing struct (`DeployResult`) and adds two stable string constants (diagnostic codes). The data flow is plain: two booleans are populated by a preflight function and consumed by three rendering surfaces (stderr warning, JSON envelope, PR body composer).

---

## Type 1: `DeployResult` (modified)

**File**: `internal/fleet/deploy.go` (existing, lines 40–52 today)

**Change**: Append two `bool` fields to the end of the struct, with explicit `json:` tags using the snake-case convention shared with the existing fields. Maintain field order: pre-existing fields first, then new fields.

```go
// DeployResult aggregates what happened for a single-repo deploy.
type DeployResult struct {
    Repo          string            `json:"repo"`
    CloneDir      string            `json:"clone_dir"`
    Added         []WorkflowOutcome `json:"added"`
    Skipped       []WorkflowOutcome `json:"skipped"`
    Failed        []WorkflowOutcome `json:"failed"`
    InitWasRun    bool              `json:"init_was_run"`
    BranchPushed  string            `json:"branch_pushed"`
    PRURL         string            `json:"pr_url"`
    MissingSecret string            `json:"missing_secret"` // non-empty if the engine secret is absent from the repo
    SecretKeyURL  string            `json:"secret_key_url"` // where to obtain the key for MissingSecret

    // Set true when the GitHub Actions repo-level setting `enabled` is false.
    // false is also the indeterminate value (403/5xx/missing-field response): we
    // do not warn unless we have positive evidence Actions is disabled.
    ActionsDisabled bool `json:"actions_disabled"`

    // Set true when the GitHub Actions workflow token's default permission is
    // "read" (i.e., GITHUB_TOKEN cannot push commits or create reviews on the
    // target repo). false is the indeterminate value (same fail-open semantics
    // as ActionsDisabled).
    WorkflowTokenReadOnly bool `json:"workflow_token_read_only"`
}
```

**JSON serialization**: Pre-existing consumers see two new fields with default `false`. Per clarification Q1 / R8, no `cmd.SchemaVersion` bump is required because the additions are strictly additive and no release has shipped since the schema was introduced.

**Lifecycle**: Constructed by `Deploy()` at line 109 (`res := &DeployResult{Repo: repo}`). Populated by `checkActionsSettings()` at three call sites — fresh-clone preflight, work-dir resume commit-gate, work-dir resume push-gate — alongside the existing `checkEngineSecret()` calls. Read by `printDeploy()`, `emitDeployWarnings()`, `emitDeployEnvelope()`, and `setupRequiredSection()` (the renamed PR-body composer).

**Invariants**:

- The two boolean fields are observably independent (FR-014). Setting `ActionsDisabled=true` does not imply or require `WorkflowTokenReadOnly=true`. The check function returns both, and either may be true alone.
- The fields are never `true` for indeterminate API responses. Only positive evidence (200 OK + the expected field present + the disqualifying value) sets a field to `true`.
- The fields are never reset to `false` after being set within a single `Deploy()` call. The check runs once per call site (matching `checkEngineSecret`'s existing one-call-per-site pattern).

---

## Type 2: New `Diagnostic` codes

**File**: `internal/fleet/diagnostics.go` (existing, lines 21–32 today)

**Change**: Add two new `const` entries to the existing block. Position adjacent to `DiagMissingSecret` (the closest semantic neighbor).

```go
const (
    DiagMissingSecret         = "missing_secret"
    DiagActionsDisabled       = "actions_disabled"        // NEW
    DiagWorkflowTokenReadOnly = "workflow_token_read_only" // NEW
    DiagDriftDetected         = "drift_detected"
    DiagHint                  = "hint"
    DiagUnknownProperty       = "unknown_property"
    DiagHTTP404               = "http_404"
    DiagGPGFailure            = "gpg_failure"
    DiagRateLimited           = "rate_limited"
    DiagRepoInaccessible      = "repo_inaccessible"
    DiagNetworkUnreachable    = "network_unreachable"
    DiagEmptyFleet            = "empty_fleet"
)
```

**Naming rationale**: Snake-case, single-or-double-word, no namespace prefix. Consistent with the existing 10 codes. Per R4, two distinct codes (rather than one combined `repo_settings_misconfigured`) so CI consumers can filter on each finding independently.

**JSON consumption**: Diagnostic values appear in the envelope's `warnings[].code` field. Example:

```json
{
  "schema_version": 1,
  "command": "deploy",
  "warnings": [
    {
      "code": "actions_disabled",
      "message": "GitHub Actions is disabled on alice/widgets — enable at https://github.com/alice/widgets/settings/actions",
      "fields": { "url": "https://github.com/alice/widgets/settings/actions" }
    },
    {
      "code": "workflow_token_read_only",
      "message": "GITHUB_TOKEN is read-only on alice/widgets — workflows that push commits or create reviews will fail; set \"Workflow permissions\" → \"Read and write permissions\" at https://github.com/alice/widgets/settings/actions",
      "fields": { "url": "https://github.com/alice/widgets/settings/actions" }
    }
  ]
}
```

---

## Function 1: `checkActionsSettings` (new)

**File**: `internal/fleet/deploy.go` (existing file, new function)

**Signature** (planned):

```go
// checkActionsSettings queries the GitHub Actions repo-level permission
// endpoints and returns (actionsDisabled, workflowTokenReadOnly).
// Indeterminate responses (403, 5xx, network error, malformed JSON, missing
// expected field) return (false, false) and emit a single zlog.Debug entry
// per skipped endpoint identifying the failure mode. Both endpoints are
// queried independently — neither short-circuits the other (FR-014).
func checkActionsSettings(ctx context.Context, repo string) (bool, bool)
```

**Behavior** (deterministic, per FR-001/FR-002/FR-007/FR-008/FR-014):

1. Query `/repos/<repo>/actions/permissions` via `ghAPIJSON`. On any error or non-`map[string]any` response, set the actions-disabled return to `false` and emit `zlog.Debug().Str("repo", repo).Str("endpoint", "/repos/.../actions/permissions").Str("reason", reason).Msg("actions-settings preflight skipped")`. On success, type-assert `map["enabled"].(bool)`; if absent or wrong type, treat as indeterminate. Otherwise, the return is `!enabled`.
2. Independently, query `/repos/<repo>/actions/permissions/workflow`. Same indeterminate handling. On success, type-assert `map["default_workflow_permissions"].(string)`; if absent or wrong type, treat as indeterminate. Otherwise, the return is `value == "read"`.
3. Return both booleans regardless of either one's outcome.

**Error semantics**: The function does not return an `error`. By design, every non-success path is treated as indeterminate-and-skip (clarification Q3, FR-008). An `error` return would push every consumer to handle "failed to check" differently from "no warning needed," which contradicts the fail-open contract.

**Test seam**: `ghAPIJSON` is a package-level `var func(...)` (declared in `fetch.go:183`). Tests override it per the `TestCheckEngineSecret` pattern.

---

## Function 2: `setupRequiredSection` (renamed and extended)

**File**: `internal/fleet/deploy.go` (replaces `missingSecretPRSection` at lines 79–98)

**Signature** (planned):

```go
// setupRequiredSection renders a single "## ⚠ Setup required" markdown block
// for the deploy PR body, with one sub-block per active preflight finding.
// Findings are emitted in fixed order: ActionsDisabled, WorkflowTokenReadOnly,
// MissingSecret. Returns the empty string when no findings are active, in
// which case the caller (prBody) does not insert the section heading.
func setupRequiredSection(res *DeployResult) string
```

**Output shape** (worked example with all three findings active):

```markdown
## ⚠ Setup required

**GitHub Actions is disabled on `alice/widgets`.** Workflows added in this PR will not run until Actions is enabled.

Enable at: https://github.com/alice/widgets/settings/actions

**Workflow token is read-only on `alice/widgets`.** Agentic workflows that push commits or create reviews will fail.

Fix at: https://github.com/alice/widgets/settings/actions

Set "Workflow permissions" → "Read and write permissions"

**Engine secret missing on `alice/widgets`.** The `ANTHROPIC_API_KEY` secret is not set. Workflows added in this PR will fail until it is configured.

```sh
gh secret set ANTHROPIC_API_KEY --repo alice/widgets
```

Obtain the key at: https://console.anthropic.com/settings/keys
```

When only one or two findings are active, the heading still appears once and only the active sub-blocks render. When all three are absent, the function returns `""`.

**Caller**: `prBody()` at `internal/fleet/deploy.go:612`. The current conditional `if res.MissingSecret != "" { b.WriteString(missingSecretPRSection(res)); ... }` becomes `if section := setupRequiredSection(res); section != "" { b.WriteString(section); ... }`.

**Backward compatibility**: `BuildMissingSecretMessage` is retained unchanged (it produces the single-line stderr warning, used by both `emitDeployWarnings` in `cmd/deploy.go` and `warnings[].message` in the JSON envelope). Two parallel functions are added: `BuildActionsDisabledMessage(repo string) string` and `BuildWorkflowTokenReadOnlyMessage(repo string) string`. Each produces a single-line warning with embedded settings URL.

---

## State transitions

This feature has none. The data is computed-once per `Deploy()` invocation, read by N consumers, and discarded when the function returns. There is no persistent state, no on-disk cache, no migration. Field defaults follow Go zero-values; that's the entire state machine.

---

## Validation rules

Pre-existing consumers of `DeployResult` accept the extended struct without code change because the new fields are strictly additive. No FR mandates a particular value combination — every combination of `(ActionsDisabled, WorkflowTokenReadOnly, MissingSecret)` is valid input to every consumer:

| ActionsDisabled | WorkflowTokenReadOnly | MissingSecret non-empty | Behavior |
|---|---|---|---|
| false | false | no  | No setup-required section, no warnings, no Diagnostic entries. |
| true  | false | no  | One sub-block (Actions); one warning; one Diagnostic. |
| false | true  | no  | One sub-block (token); one warning; one Diagnostic. |
| true  | true  | no  | Two sub-blocks (Actions, token); two warnings; two Diagnostics. |
| false | false | yes | One sub-block (secret); one warning; one Diagnostic. (Existing behavior preserved.) |
| true  | true  | yes | Three sub-blocks in fixed order; three warnings; three Diagnostics. |

The truth table is the validation rule: every cell is reachable, every cell is observable, every cell has the same downstream behavior shape (warning, Diagnostic, PR-body block).
