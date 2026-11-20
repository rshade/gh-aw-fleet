# Contract: Upstream Model-Failure Signals

Two read-only `gh` surfaces behind package-level injection seams (so tests run offline, mirroring `ghWorkflowsAPI`/`ghLogsAPI`/`ghDiscussionsAPI`).

## Seam A — `ghFailureIssuesAPI(ctx, repo) ([]failureIssue, error)`

**Command**:

```bash
gh issue list --repo <owner/repo> --label agentic-workflows --state open \
  --json number,title,body --limit 100
```

**Returns**: decoded `[]failureIssue{Number, Title, Body}`.

**Marker parsing** (pure, unit-testable — `parseModelFailure(body) bool`):

1. Body is a failure issue **iff** it contains `gh-aw-failure-issue: true`.
2. Locate the `failure_categories:` value (from the key up to the next `,` or `-->`).
3. Repo is model-flagged **iff** that value contains the substring `model_not_supported`.

Reference marker (verified live, `rshade/finfocus#1349`):

```html
<!-- gh-aw-failure-issue: true, workflow_id: code-simplifier, branch: main, failure_categories: agent_failure|model_not_supported_error -->
```

**Filtering**: The `[aw] No-Op Runs` issue also carries the `agentic-workflows` label but lacks the `gh-aw-failure-issue: true` marker, so rule 1 excludes it.

**Errors**: any non-zero `gh` exit or decode error → the repo's model-support status is `undetermined` (a soft `Diagnostic`, not a fatal error). Wrapped through `ghErr`/`%w` like the sibling seams.

## Seam B — `ghRepoVariablesAPI(ctx, repo) (map[string]string, error)`

Called **only for flagged repos** (gated to minimize I/O).

**Command**:

```bash
gh variable list --repo <owner/repo> --json name,value
```

(Equivalent `gh api repos/<repo>/actions/variables --paginate` is acceptable; the seam abstracts the choice.)

**Returns**: `map[string]string` of the repo's **own** (repository-level) variable name → value.

## Seam C — `ghOrgVariablesAPI(ctx, repo) (map[string]string, error)`

Called **only for flagged repos**, after Seam B.

**Command**:

```bash
gh api --paginate repos/<owner/repo>/actions/organization-variables --jq '.variables[] | {name, value}'
```

**Returns**: the organization variables shared with the repo, name → value. A user-owned repo has no organization; the endpoint answers `HTTP 422`, which the seam maps to an **empty** set, not an error.

**Merge** (pure — `mergeActionsVariables(org, repo)`): repository values override organization values of the same name, matching how GitHub builds the `vars` context. Environment-level variables are out of scope: compiled gh-aw lock files declare no `environment:`.

**Effective-model resolution** (pure, unit-testable — `resolveEffectiveModel(merged) (model, controllingVar)`):

```text
GH_AW_MODEL_AGENT_COPILOT  (if set and non-empty)
  else GH_AW_DEFAULT_MODEL_COPILOT  (if set and non-empty)
  else ghAwBakedDefaultCopilotModel  // "claude-sonnet-4.6" at current gh-aw pin
```

`controllingVar` is the variable an operator must set to change the model: `GH_AW_MODEL_AGENT_COPILOT` when it supplied the value, otherwise `GH_AW_DEFAULT_MODEL_COPILOT` (which also overrides the baked default). The remediation hint names it (§ Diagnostics emitted).

**Errors**: any non-zero `gh` exit (other than Seam C's `HTTP 422`) or decode error from Seam B **or** Seam C → repo stays `affected` with an **empty** offending-model set (reported as `unknown`); the flag is never dropped (FR-002). Resolving from one level alone could name the wrong model.

## Concurrency & determinism

- The model collector `collectOverviewModelHealth(ctx, repos, …)` runs as a **third** `wg.Go` in `Overview`, concurrent with the drift and health collectors, joined by the existing `wg.Wait()`.
- Within the collector, repos are processed in the sorted `overviewRepoNames` order; `ghRepoVariablesAPI` and `ghOrgVariablesAPI` are invoked lazily per flagged repo.
- The v1 collector resolves at most one offending model per repo, so ordering is trivial (FR-012); remediation hints sort by `fieldRepo` via `splitOverviewDiags`.
- Context cancellation is the collector's only error, returned like `collectOverviewHealth`'s; `Overview` fails rather than rendering unread repos as clean.

## Diagnostics emitted

| Condition | Code | Channel | Fields |
|-----------|------|---------|--------|
| Affected repo remediation | `DiagHint` (may use `newSoftDiagnostic(repo, msg)`) | hints (stderr) | `repo`, `hint` (names the controlling variable; always references `GH_AW_DEFAULT_MODEL_COPILOT`) |
| Failure-issue read failed | warnings diag: `withDiagnosticSignal(buildRepoErrorDiag(repo, msg), repo, overviewSignalModel)` | warnings (stderr) | `repo`, `signal` |

No new `fleetdiag` code is introduced. The **affected-repo remediation** reuses `DiagHint` (already in the hints set of `splitOverviewDiags`), matching `nilAICDiag`/`latestNoopDiagnostic`; the shape it needs is exactly what `newSoftDiagnostic(repo, msg)` returns (`Code: DiagHint`, `Fields: {hint, repo}`). The **undetermined-repo warning** must **not** use `newSoftDiagnostic` (that is `DiagHint`-coded and would land in the hints channel with `{hint, repo}` — wrong channel and wrong fields for this row); instead it reuses `buildRepoErrorDiag`'s existing repo-error classification via `withDiagnosticSignal`, exactly like the sibling runs-unavailable degradation (`overview.go:257`), so it routes to the warnings channel with `{repo, signal}` fields and never appears as a hint (contract §3).
