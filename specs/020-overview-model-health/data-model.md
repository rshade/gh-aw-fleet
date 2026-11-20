# Phase 1 Data Model: `overview` Model-Support Advisory

All types are transient (no persistence). New types live in `internal/fleet`; the JSON shapes are additive to the existing `overview` envelope payload.

## Entity: `ModelSupport` (new, exported)

The per-repo model-support advisory attached to a `RepoOverview` row.

| Field | Go type | JSON | Semantics |
|-------|---------|------|-----------|
| Status | `string` | `status` | One of `affected` \| `undetermined`. (`clean` is never serialized — the whole object is nil/omitted.) |
| OffendingModels | `[]string` | `offending_models,omitempty` | Offending model identifiers — at most one in v1 (the repo's resolved effective model). Empty ⇒ model unknown (rendered `unknown`). Populated from the effective-model resolution for `affected` repos. |

```go
// ModelSupport is the per-repo advisory reporting whether a repo's recent
// agentic runs failed because the effective Copilot model is unsupported or
// deprecated. It is additive to the RepoOverview row and mirrors the
// pointer-with-omitempty convention of DriftDetail.
type ModelSupport struct {
    // Status is ModelSupportAffected when a model_not_supported failure issue
    // is open, or ModelSupportUndetermined when the failure-issue signal could
    // not be read.
    Status string `json:"status"`
    // OffendingModels names the repo's effective Copilot model(s); the v1
    // collector resolves at most one per repo. Empty means the model could not
    // be named (report as unknown).
    OffendingModels []string `json:"offending_models,omitempty"`
}
```

**Status constants** (exported, package `fleet`; consumed by `cmd` for rendering and detail gating):

```go
const (
    ModelSupportAffected     = "affected"
    ModelSupportUndetermined = "undetermined"
)
```

**Validation / invariants**:

- Serialized only when `Status != ""` (i.e. affected or undetermined). A clean repo carries a nil `*ModelSupport`, so the field is absent (FR-005, SC-003/SC-004).
- `OffendingModels` carries at most one entry in v1, so output ordering is trivially deterministic (FR-012).
- `undetermined` rows always carry an empty `OffendingModels` (there is nothing to name when the signal is unavailable).

## Modified entity: `RepoOverview` (existing)

One additive field; every existing field unchanged (FR-006).

```go
type RepoOverview struct {
    // ... existing fields (Repo, DriftState, DriftDetail, Runs, Failures,
    //     NoOps, HealthRate, AIC, Cost, RunsAvailable, RunsError) unchanged ...

    // ModelSupport carries the model-support advisory when the repo is affected
    // or undetermined; nil (and omitted) when the repo has no model-support
    // problem, keeping clean rows byte-identical to prior output.
    ModelSupport *ModelSupport `json:"model_support,omitempty"`
}
```

`OverviewTotal` is **unchanged** — model-support is per-repo and advisory; no pooled counter feeds it or the exit gate (FR-007).

## Internal reduce type: `overviewModelHealth` (new, unexported)

Mirrors the existing `overviewHealth` reduce struct; produced by the model collector, consumed by `joinOverviewRow`.

```go
type overviewModelHealth struct {
    status         string // "", ModelSupportAffected, or ModelSupportUndetermined
    offendingModel string // "" ⇒ unknown
    controllingVar string // variable that sets offendingModel; "" ⇒ unknown
}
```

- `status == ""` ⇒ clean ⇒ `joinOverviewRow` leaves `RepoOverview.ModelSupport` nil.
- Otherwise `joinOverviewRow` sets `row.ModelSupport = &ModelSupport{Status: h.status}`, wrapping a non-empty `offendingModel` as a single-element `OffendingModels` list (the wire field stays a list; the collector resolves at most one model per repo in v1).

## Upstream signal types (new, unexported)

Decoded from the three new `gh` seams in `internal/fleet/model_signal.go`.

```go
// failureIssue is one open agentic-workflows failure issue for a repo.
type failureIssue struct {
    Number int    `json:"number"`
    Title  string `json:"title"`
    Body   string `json:"body"`
}

// ghFailureIssuesAPI lists a repo's open agentic-workflows failure issues.
// Package-level seam so tests run offline.
var ghFailureIssuesAPI = func(ctx context.Context, repo string) ([]failureIssue, error) { /* gh issue list */ }

// ghRepoVariablesAPI returns a repo's own Actions variables as name→value.
// Package-level seam; called only for flagged repos.
var ghRepoVariablesAPI = func(ctx context.Context, repo string) (map[string]string, error) { /* gh variable list */ }

// ghOrgVariablesAPI returns the organization variables shared with a repo;
// HTTP 422 (user-owned repo) is an empty set. Called only for flagged repos.
var ghOrgVariablesAPI = func(ctx context.Context, repo string) (map[string]string, error) { /* gh api .../organization-variables */ }
```

## Constants (new)

```go
// Copilot model-resolution variable names (gh-aw's COPILOT_MODEL fallback chain).
const (
    varAgentModelCopilot   = "GH_AW_MODEL_AGENT_COPILOT"
    varDefaultModelCopilot = "GH_AW_DEFAULT_MODEL_COPILOT"
)

// ghAwBakedDefaultCopilotModel is the model gh-aw compiles into COPILOT_MODEL
// when neither GH_AW_MODEL_AGENT_COPILOT nor GH_AW_DEFAULT_MODEL_COPILOT is set.
// It MUST track the fleet's github/gh-aw source pin — gh-aw changes this default
// across releases (e.g. it dropped gpt-5-mini in v0.81.5). Verified against the
// current fleet pin.
const ghAwBakedDefaultCopilotModel = "claude-sonnet-4.6"

// failure-issue marker tokens.
const (
    failureIssueMarker      = "gh-aw-failure-issue: true"
    failureCategoriesKey    = "failure_categories:"
    modelNotSupportedToken  = "model_not_supported"
)
```

## State transitions (per repo, per `overview` run)

```text
read open failure issues (ghFailureIssuesAPI)
      │
      ├─ error ─────────────────────────────► status = undetermined, models = []
      │
      └─ ok
           │
           ├─ no issue matches marker+category ─► status = "" (clean; ModelSupport nil)
           │
           └─ ≥1 issue matches
                   │
                   ▼  resolve effective model (repo vars merged over org vars, flagged repos only)
                   ├─ either read errors ──► status = affected, model = ""   (unknown)
                   └─ ok ──────────────────► status = affected, model = effective,
                                             controllingVar = the variable to set

ctx cancelled at any point ──► collector returns ctx.Err(); Overview fails (no partial rows)
```

## Fixture matrix (SC-006)

Offline fixtures exercised via the seams:

| Repo fixture | ghFailureIssuesAPI | ghRepoVariablesAPI | Expected `model_support` |
|--------------|--------------------|--------------------|--------------------------|
| model-failure, unset var | one issue with `…\|model_not_supported_error` | `{}` | `{status: affected, offending_models: [claude-sonnet-4.6]}` |
| model-failure, override set | same | `{GH_AW_DEFAULT_MODEL_COPILOT: gpt-5-mini}` | `{status: affected, offending_models: [gpt-5-mini]}` |
| unrelated failure | one issue with only `agent_failure` | (not called) | absent (nil) — no false positive (FR-009) |
| healthy | no failure issues | (not called) | absent (nil) — output unchanged (SC-003) |
| undetermined | ghFailureIssuesAPI returns error | (not called) | `{status: undetermined}` |
| var-resolve failure | model_not_supported issue | ghRepoVariablesAPI errors | `{status: affected}` (models omitted ⇒ unknown) |
| org agent var | model_not_supported issue | `{}`; org `{GH_AW_MODEL_AGENT_COPILOT: org-agent}` | `{status: affected, offending_models: [org-agent]}`; hint names `GH_AW_MODEL_AGENT_COPILOT` |
| repo over org | model_not_supported issue | `{GH_AW_MODEL_AGENT_COPILOT: repo-agent}`; org sets `org-agent` | `{status: affected, offending_models: [repo-agent]}` |
| org-read failure | model_not_supported issue | `{}`; ghOrgVariablesAPI errors | `{status: affected}` (models omitted ⇒ unknown) |
