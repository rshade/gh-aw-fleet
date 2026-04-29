# Phase 1 Data Model: `status` Subcommand for Drift Detection

**Feature**: `004-status-drift-detection` | **Date**: 2026-04-28 | **Plan**: [plan.md](./plan.md)

This document defines the Go types introduced by this feature, their JSON serialization shape, and their lifecycle. All entities are pure data carriers — there is no persistent storage, no state machine, and no relational model. Every type lives in `internal/fleet/status.go` except where noted.

---

## Type 1: `StatusOpts` (input)

```go
// StatusOpts controls a single Status() invocation.
type StatusOpts struct {
    Repo string // optional: when non-empty, query only this repo (must be declared in cfg);
                // when empty, query every repo in cfg.
}
```

**Lifecycle**: Constructed by `cmd/status.go` from positional arguments and passed to `fleet.Status(ctx, cfg, opts)` once. Never persisted, never mutated after construction.

**Validation rules**:

- If `Repo != ""`, the value MUST appear in `cfg.Repos` (the merged fleet.json + fleet.local.json). Validation happens in `Status()` before any `gh api` call (FR-008 — "MUST first verify the repo is declared … MUST NOT issue any GitHub API calls" if not). This validation surfaces via the third (`error`) return of `Status()` — see Type 2 lifecycle.
- An empty `Repo` triggers the fleet-wide path (every repo in `cfg`).
- A `cfg.ResolveRepoWorkflows(repo)` failure during orchestration (e.g., a profile reference is broken or names an undefined source) is surfaced as a per-repo `RepoStatus{DriftState: "errored", ErrorMessage: <wrapped err>}` plus a sibling `Diagnostic`, NOT a top-level `Status()` error. Per-repo isolation (FR-009) extends to config-resolution failures so one broken profile reference does not abort the rest of the fleet-wide run.

**No JSON serialization** — this is a Go-only input type, never crosses the wire.

---

## Type 2: `StatusResult` (top-level result)

```go
// StatusResult is the result payload embedded in the JSON envelope's `result`
// field for the `status` command. Single field today; left as a struct so
// future additions (e.g., aggregate counts, run timing) are non-breaking.
type StatusResult struct {
    Repos []RepoStatus `json:"repos"`
}
```

**Lifecycle**: Constructed once per `Status()` call. Workers append a `RepoStatus` to a results channel; the orchestrator collects them, sorts by `Repo` (alphabetic for deterministic output and stable test fixtures), and assigns to `StatusResult.Repos`.

`Status()` returns `(*StatusResult, []Diagnostic, error)`:

1. `*StatusResult` is the wire payload defined here.
2. `[]Diagnostic` carries per-repo `errored` diagnostics (one per failed repo, `Code: DiagRepoInaccessible` or `DiagRateLimited`, `Fields: {"repo": <owner/name>}`) PLUS fleet-wide warnings (e.g., a `Diagnostic{Code: "empty_fleet"}` when `cfg.Repos` is empty). `cmd/status.go` splits this slice into the envelope's `warnings[]` vs `hints[]` by `Code`. Keeping diagnostics OUT of `StatusResult` itself preserves the contract that `result` carries only the wire payload — diagnostics belong in the envelope's sibling slots, not nested inside `result`.
3. `error` is reserved for setup-time failures (config load, single-repo arg validation per FR-008) that prevent constructing a `StatusResult` at all. Per-repo failures NEVER surface here — they go to `RepoStatus.ErrorMessage` AND the second return.

**Validation/normalization rules**:

- `Repos` MUST always serialize as a JSON array (never `null`). The envelope writer's `initSlices` helper enforces this at the write boundary (FR-015, R4).
- Order: alphabetical by `Repo` field. Determinism matters for snapshot-style tests and for stable `jq` queries that index into the array.
- An empty fleet → `Repos: []` and a warning in the envelope's `warnings[]` (per spec Edge Cases — "fleet.json declares zero repos"). The warning is emitted as a `Diagnostic{Code: "empty_fleet"}` in `Status()`'s second return value.

---

## Type 3: `RepoStatus` (per-repo drift report)

```go
// RepoStatus is the drift report for a single repository. Exactly one
// RepoStatus is emitted per repo queried (success or failure). Categories
// are mutually descriptive: a workflow appears in at most one of
// missing / extra / drifted / unpinned per RepoStatus.
type RepoStatus struct {
    Repo         string           `json:"repo"`           // owner/name
    DriftState   string           `json:"drift_state"`    // aligned | drifted | errored
    Missing      []string         `json:"missing"`        // declared workflow names absent from repo
    Extra        []string         `json:"extra"`          // gh-aw-managed workflows present but undeclared
    Drifted      []WorkflowDrift  `json:"drifted"`        // present-but-different-ref workflows
    Unpinned     []string         `json:"unpinned"`       // present but lacking parseable source: frontmatter
    ErrorMessage string           `json:"error_message"`  // empty unless drift_state == "errored"
}
```

**Lifecycle**: Constructed by a worker goroutine after it has fetched a repo's directory listing and (per declared workflow) the workflow body. The worker assembles the four category slices, computes `DriftState` from them, and emits the struct on the results channel. The orchestrator does not mutate it after receipt.

**Drift state derivation rules** (per FR-006, clarification 4):

```text
drift_state == "errored"  ⟺  ErrorMessage != ""  (fetch failed for any reason)
drift_state == "aligned"  ⟺  ErrorMessage == "" AND len(Missing)+len(Extra)+len(Drifted)+len(Unpinned) == 0
drift_state == "drifted"  ⟺  otherwise (any non-empty drift category, no error)
```

**Validation rules**:

- All four slice fields (`Missing`, `Extra`, `Drifted`, `Unpinned`) MUST serialize as `[]` (never `null`) when empty (FR-015).
- `ErrorMessage` is empty string `""` (not omitted) unless `DriftState == "errored"`. Conversely, when `DriftState == "errored"`, the four slice fields are still emitted but typically empty (the per-repo failure short-circuits drift computation).
- `Repo` is the canonical name from fleet.json (e.g., `rshade/gh-aw-fleet`). A redirected repo on GitHub still uses the fleet.json name; the redirect surfaces as a separate entry in the envelope's `warnings[]` (spec Edge Cases — repo redirected on GitHub).
- Within each slice, entries are sorted: alphabetically by workflow name for `Missing` / `Extra` / `Unpinned`; alphabetically by `WorkflowDrift.Name` for `Drifted`. Determinism matters for tests and for diff-friendly output.

**Mutual exclusivity rules** (a single workflow cannot simultaneously be in two buckets):

| Workflow's actual state | Bucket |
|---|---|
| Declared in fleet.json AND not present in repo | `Missing` |
| Declared AND present AND `source:` frontmatter parseable AND ref matches | (aligned — appears in NO bucket) |
| Declared AND present AND `source:` parseable AND ref differs | `Drifted` |
| Declared AND present AND `source:` missing/malformed | `Unpinned` |
| Not declared AND present AND `source:` parseable | `Extra` |
| Not declared AND present AND `source:` missing/malformed | (ignored — not gh-aw managed; spec Edge Cases) |

---

## Type 4: `WorkflowDrift` (single drifted workflow)

```go
// WorkflowDrift describes one workflow whose installed source ref differs
// from what fleet.json declares. Used inside RepoStatus.Drifted.
type WorkflowDrift struct {
    Name        string `json:"name"`         // workflow basename, no .md suffix
    DesiredRef  string `json:"desired_ref"`  // ref string from fleet.json (e.g., "v0.68.3")
    ActualRef   string `json:"actual_ref"`   // ref string from installed source: frontmatter (e.g., "v0.67.0", "abc123def", "main")
}
```

**Lifecycle**: Constructed during per-workflow comparison inside a worker. Each appearance under `RepoStatus.Drifted` represents one workflow.

**Validation rules**:

- `Name` is the workflow basename without `.md` (e.g., `audit`, not `audit.md`). Matches the convention used by `TemplateWorkflow.Name` in `internal/fleet/schema.go`.
- `DesiredRef` and `ActualRef` are LITERAL strings — no normalization, no SHA resolution (per clarification 3, FR-004).
- `DesiredRef == ActualRef` should never appear here: if the strings are equal the workflow is aligned and should not be in the `Drifted` slice. The orchestrator's drift computation enforces this; tests assert it.

---

## Type 5: Internal worker types (not exported, not in JSON)

```go
// statusJob is one work unit dequeued by a worker. Lives in the buffered jobs
// channel; never observed outside the worker pool.
type statusJob struct {
    repo     string                   // owner/name
    declared []ResolvedWorkflow       // pre-computed by cfg.ResolveRepoWorkflows(repo) before fan-out
}

// statusFetcher is the seam between Status() and the gh api primitives, used
// to inject fakes in tests. Production binding: a thin wrapper over ghAPIRaw
// and ghAPIJSON that returns parsed structures.
type statusFetcher interface {
    listWorkflowsDir(ctx context.Context, repo string) ([]string, error)
    fetchWorkflowBody(ctx context.Context, repo, file string) (string, error)
}

// statusWorkerPoolSize is the number of concurrent repo workers (clarification 1: 4–8).
const statusWorkerPoolSize = 6
```

**Lifecycle**: `statusJob`s are constructed by the orchestrator *before* fan-out (one per repo to query) and pushed onto the jobs channel; workers consume them. `statusFetcher` is a single instance shared across workers (its methods are stateless w.r.t. `gh api` and safe for concurrent use; `exec.Command` in `ghAPIRaw` is goroutine-safe per Go runtime semantics).

**Why a fetcher interface**: Lets `internal/fleet/status_test.go` inject a fake fetcher with deterministic returns, isolating drift-computation tests from network. Without the interface, status_test would either hit real GitHub (flaky) or test only the trivial sub-functions.

---

## Cross-reference: Reused existing types

These existing types are consumed by status without modification:

| Existing type | Source file | Status's use |
|---|---|---|
| `Config` | `internal/fleet/load.go` | Input — read-only, passed to `Status()`. |
| `Repo` | `internal/fleet/schema.go` | Iterated to build `statusJob`s (fleet-wide path). |
| `ResolvedWorkflow` | `internal/fleet/schema.go` | The "declared" set per repo; status calls `cfg.ResolveRepoWorkflows(repo)` to get this. Carries `Name`, source repo, and ref. |
| `Diagnostic` | `internal/fleet/diagnostics.go` | Emitted in envelope's `warnings[]` and `hints[]`. |
| `Envelope` | `cmd/output.go` | The JSON wrapper. Status fills `Result: *StatusResult`, `Apply: false`, `Command: "status"`. |

---

## Schema versioning

Status does **not** introduce a new `schema_version`. The envelope's `cmd.SchemaVersion = 1` (defined in `cmd/output.go:17`) carries through. Status registers itself as another value of the envelope's `command` field (`"status"`), which is an additive change — does not bump the version per the documented rule ("Bumped only on breaking changes to envelope or result struct field shapes").

If a future revision adds a top-level field to `StatusResult` (e.g., aggregate counts, run timing) that is `omitempty`, that's also additive — no version bump. A breaking change (renaming `drift_state`, removing a category, changing a field's type) would bump the envelope's `schema_version` to `2` globally — a significant decision that would deserve its own spec round.
