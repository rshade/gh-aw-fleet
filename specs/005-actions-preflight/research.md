# Phase 0 Research: Deploy Preflight for Actions Enabled and Workflow Write Permissions

**Feature**: `005-actions-preflight` | **Date**: 2026-04-29 | **Plan**: [plan.md](./plan.md)

This document confirms each integration point against the existing codebase. There are **no `NEEDS CLARIFICATION` markers** carried over from the spec — `/speckit.clarify` resolved all four open questions before this phase. The research below validates the plan's stated assumptions about the codebase rather than choosing among unknowns.

---

## R1. Where do existing preflight findings flow through `Deploy()` and consumers?

**Decision**: Two boolean fields appended to `DeployResult` (`internal/fleet/deploy.go:40-52`), populated by a new `checkActionsSettings()` function called alongside `checkEngineSecret()` at every existing preflight call site.

**Rationale**: The codebase already has the exact precedent in `MissingSecret` / `SecretKeyURL`:

- `DeployResult` carries the finding (deploy.go:50-51).
- `Deploy()` calls `checkEngineSecret()` and assigns to `res.MissingSecret`, `res.SecretKeyURL` at three call sites (deploy.go:136, deploy.go:198, deploy.go:208 — main flow plus `handleWorkDirResume`'s commit-gate and push-gate resume points).
- `cmd/deploy.go:emitDeployWarnings()` at line 123 reads `res.MissingSecret` and emits a `zlog.Warn()` line.
- `cmd/deploy.go:emitDeployEnvelope()` at line 140 reads the same field and appends a structured `Diagnostic{Code: DiagMissingSecret, ...}` to the envelope's `warnings[]`.
- `internal/fleet/deploy.go:prBody()` at line 612 conditionally inserts `missingSecretPRSection(res)` near the top of the PR body.

The new feature mirrors this pattern exactly. The two new boolean fields default to `false` (consistent with Go zero-values), so existing JSON consumers continue to deserialize correctly without a `cmd.SchemaVersion` bump (clarification Q1).

**Alternatives considered**:

- **A separate `PreflightResult` substruct** to group the four fields: rejected as premature aggregation. The feature adds two booleans to a struct that already has nine fields; one more boolean each is below the noise floor. A substruct introduces a needless naming layer (`res.Preflight.ActionsDisabled` vs. `res.ActionsDisabled`) without making any consumer simpler.
- **Side-channel logging only** (no `DeployResult` field): rejected because the JSON envelope contract requires the finding to be machine-readable in `warnings[]`, and the PR body composer needs the same finding to render a setup-required block. Both consumers need a structured field, not a log line.

---

## R2. Which `gh api` wrapper supports the two new endpoints?

**Decision**: Use `ghAPIJSON` from `internal/fleet/fetch.go:183` — already a `var func(ctx, path) (any, error)` declared as a package-level variable (test seam) and already in use by `fetch.go` and `status.go`.

**Rationale**: Both endpoints (`/repos/<owner>/<repo>/actions/permissions` and `/repos/<owner>/<repo>/actions/permissions/workflow`) return JSON objects. `ghAPIJSON` returns `any`, which decodes to `map[string]any` for object responses. The check function performs a type assertion + field lookup:

```go
m, ok := raw.(map[string]any)
if !ok {
    return indeterminate // see R3
}
enabled, ok := m["enabled"].(bool)
if !ok {
    return indeterminate
}
```

Reusing `ghAPIJSON` means the test pattern from `TestCheckEngineSecret` (which overrides `ghAPIExists`) translates directly: `TestCheckActionsSettings` overrides `ghAPIJSON` with a closure returning a fixture `map[string]any` keyed by endpoint path. Clarification Q4 locked this in.

**Alternatives considered**:

- **Wrap `ghAPIJSON` in typed helpers** (`ghActionsPermissions(ctx, repo) (ActionsPermissions, error)`): rejected as Q4 option B. Adds a layer for a function that is called from exactly one site each. The typed-fixture upside is cosmetic; the test-fixture cost (build a struct vs. `map[string]any{"enabled": true}`) is a wash.
- **Introduce `GitHubClient` interface**: Q4 option C. Rejected — would require refactoring `checkEngineSecret`, `fetch.go`, and `status.go` to take the interface, multiplying the surface for a two-endpoint feature.
- **Use `ghAPIRaw` + `json.Unmarshal` into a typed struct**: would be marginally type-safer but breaks the seam pattern (`ghAPIRaw` is a regular `func`, not a `var func`). Tests would need to override `exec.Command`, which is the very abstraction the package-level var pattern exists to avoid.

---

## R3. How is "indeterminate" represented when the API call fails or returns an unexpected shape?

**Decision**: Indeterminate is **observable only via debug logs**; the two `DeployResult` boolean fields stay `false` (their zero value), which represents "no warning" for downstream consumers. A discriminated `Finding` enum is **not** introduced.

**Rationale**: The boolean field semantics are "should we warn the operator?" — not "what is the underlying setting?" An indeterminate response means "we don't know, fail open, don't warn" — which maps cleanly onto `false`. Adding a third state would force every consumer (`emitDeployWarnings`, `emitDeployEnvelope`, `setupRequiredSection`) to handle the third state explicitly, inflating the change for zero observable benefit.

The debug-log emission satisfies FR-007 / FR-008 ("a single debug-level log entry SHOULD record that the check was skipped"). It uses `zlog.Debug()` with structured fields:

```go
zlog.Debug().
    Str("repo", repo).
    Str("endpoint", "/repos/.../actions/permissions").
    Str("reason", "http_403"). // or "missing_field:enabled" or "transport_error"
    Msg("actions-settings preflight skipped")
```

Hidden at the default `info` log level; visible at `debug`. Clarification Q3 locked indeterminate to "skip silently, never assume defaults, never assume worst case."

**Alternatives considered**:

- **Tri-state field** (`type Finding int { Healthy, Misconfigured, Indeterminate }`): rejected per above — multiplies consumer complexity without surfacing useful information. The boolean false-as-"no warn" is the simplest contract.
- **Surface "could not determine" as a third warning type** (Q3 option D): rejected at clarification time. Generates noise on healthy-deploy + restricted-token CI flows — exactly the pattern this feature exists to support.

---

## R4. Where do new `Diagnostic` codes live?

**Decision**: Two new constants in `internal/fleet/diagnostics.go` joining the existing list (line 21 onward): `DiagActionsDisabled = "actions_disabled"` and `DiagWorkflowTokenReadOnly = "workflow_token_read_only"`.

**Rationale**: Snake-case, single-word-or-underscored, no namespace prefix — consistent with `DiagMissingSecret`, `DiagDriftDetected`, `DiagRepoInaccessible`. CI consumers gate on these codes via `jq -e '.warnings[] | select(.code == "actions_disabled")'`. Two codes (rather than one combined `repo_settings_misconfigured`) so consumers can filter and act on each finding independently — the clarification answer to Q1's discussion of stable-code-per-finding.

**Alternatives considered**:

- **Single combined code** `actions_settings_misconfigured`: rejected. Loses the ability to filter on Actions-disabled vs. read-only-token — the two have different remediations and different operators may care about different ones.
- **Namespaced codes** `deploy.actions_disabled`: rejected as inconsistent with the existing flat naming convention. If a future feature spec wants namespacing, that's a cross-cutting refactor of all 10 codes, not a one-off for this feature.

---

## R5. How do the two new findings appear in the PR body?

**Decision**: Replace `missingSecretPRSection(res)` with `setupRequiredSection(res)` — a composer that emits a single `## ⚠ Setup required` heading exactly once when *any* of (ActionsDisabled, WorkflowTokenReadOnly, MissingSecret) is set, with one sub-block per active finding in fixed order. When all three are absent, the function returns the empty string and `prBody` does not insert it.

**Rationale**: Clarification Q2 requires one umbrella section, not three parallel headings. The current `missingSecretPRSection` bakes the heading inside the function body (`b.WriteString("## ⚠ Setup required — Actions secret missing\n\n")` at deploy.go:86). Wrapping the existing function in an outer composer would either duplicate the heading or require post-hoc string editing. Renaming + restructuring is one small refactor; the existing test surface for `missingSecretPRSection` (if any — currently no direct test, only through `prBody` integration) will be reorganized into per-finding sub-section tests in `setupRequiredSection`.

Sub-block order (per Q2):

1. **GitHub Actions disabled** (browser-clickable fix; settings URL)
2. **Workflow token is read-only** (browser-clickable fix; settings URL + radio-button label)
3. **Engine secret missing** (paste-in-shell fix; `gh secret set ...`)

The browser-vs-shell ordering is intentional: settings toggles take a few seconds in the browser and are gating; the secret can be added in either order.

**Alternatives considered**:

- **Three independent composer functions called in sequence by `prBody`** with each emitting its own heading: rejected per Q2. Three headings in a PR body is visually noisy and operator scanning is harder.
- **Single composer function with an internal switch on which findings are active**: this is what we ship. It is the simplest function shape that satisfies the unified-section requirement.

---

## R6. Where does the new check get called inside `Deploy()`?

**Decision**: At every site where `checkEngineSecret(ctx, repo, engine)` is currently called — three sites in `internal/fleet/deploy.go` (line 136 main flow, lines 198 and 208 `handleWorkDirResume`'s commit-gate and push-gate resume paths). The new call replaces nothing; it stacks alongside the existing call:

```go
res.MissingSecret, res.SecretKeyURL = checkEngineSecret(ctx, repo, engine)
res.ActionsDisabled, res.WorkflowTokenReadOnly = checkActionsSettings(ctx, repo)
```

**Rationale**: `--work-dir` resume re-enters the preflight independently of the fresh-clone path. If we add the new check at only the main-flow site, a `deploy --apply --work-dir <clone>` resume would skip the warnings — invisible until an operator actually resumes a partially-failed deploy. The triplet placement matches the existing pattern exactly and ensures parity.

The two checks are **independent** (FR-014, clarification context). `checkActionsSettings` does not depend on the engine-secret result; an indeterminate engine-secret check does not skip the Actions check. The function returns two booleans regardless.

**Alternatives considered**:

- **Single combined `runPreflight()` function** that wraps both checks: rejected as a refactor outside this feature's scope. The existing call sites work; combining would touch every line that's also being touched by this feature, multiplying review surface for cosmetic gain.
- **Move to a new `preflight.go` file**: same rejection — premature factoring. Ten lines of preflight code does not warrant a file; if a future fourth preflight check arrives, that's the right time to extract.

---

## R7. Test injection mechanics for `ghAPIJSON`

**Decision**: Tests override `fetch.ghAPIJSON` (the package-level `var func` declared at `fetch.go:183`) with a closure that pattern-matches on the requested path and returns canned responses.

**Rationale**: Same shape as `TestCheckEngineSecret` overriding `ghAPIExists` (deploy_test.go:14):

```go
origGhAPIJSON := ghAPIJSON
t.Cleanup(func() { ghAPIJSON = origGhAPIJSON })
ghAPIJSON = func(_ context.Context, path string) (any, error) {
    switch path {
    case "/repos/acme/widgets/actions/permissions":
        return map[string]any{"enabled": true}, nil
    case "/repos/acme/widgets/actions/permissions/workflow":
        return map[string]any{"default_workflow_permissions": "write"}, nil
    }
    return nil, fmt.Errorf("unexpected path: %s", path)
}
```

Test cases for `TestCheckActionsSettings`: (a) healthy/healthy, (b) Actions-disabled, (c) read-only token, (d) both misconfigured, (e) 403 on Actions endpoint, (f) 403 on token endpoint, (g) missing field on Actions endpoint, (h) missing field on token endpoint, (i) network error. SC-006 mandates total runtime under one second for the suite — the in-memory closure pattern delivers this trivially.

**Alternatives considered**:

- **`httptest.NewServer` + reconfiguring `gh api` to point at it**: rejected as orthogonal to the package-level seam pattern. Would test integration with `gh` CLI rather than the unit under test.
- **Build tag for a "real-network" test**: deferred — spec's manual testing strategy (toggle Actions off on a test repo, observe the warning) is the integration test, run by the operator before merge.

---

## R8. Schema version policy for `cmd.SchemaVersion`

**Decision**: No bump. Adding optional boolean fields (default `false`) to `DeployResult` and adding new optional `warnings[].code` values is strictly additive; existing JSON consumers continue to deserialize correctly.

**Rationale**: Clarification Q1 locked this in, with the additional context that no release has shipped since the schema was introduced — there are no real-world consumers to break. Future versions of `cmd.SchemaVersion` are reserved for *removing* a field, *renaming* a field, or *changing* the type of an existing field. Adding fields is non-breaking by Go's `encoding/json` semantics: unknown fields on the consumer side are ignored, missing fields decode to zero values.

**Alternatives considered**: All three Q1 options (no bump / minor / major) were considered at clarification time; "no bump" won.

---

## Summary of validated assumptions

| Plan claim | Validated by | Result |
|---|---|---|
| `ghAPIJSON` is a swap-able var, not a regular func | grep on `internal/fleet/fetch.go:183` | ✅ Confirmed |
| `DeployResult` is the right place for new fields | inspect `internal/fleet/deploy.go:40-52` and consumers | ✅ Confirmed |
| `checkEngineSecret` has three call sites | grep `checkEngineSecret\(` in `internal/fleet/` | ✅ Three sites: lines 136, 198, 208 |
| `Diagnostic` constants live in one table | inspect `internal/fleet/diagnostics.go:21-32` | ✅ Confirmed; ten codes today |
| `prBody` inserts the setup-required section at one site | inspect `internal/fleet/deploy.go:606-650` | ✅ One call site at line 612 |
| Stderr warnings emit via `zlog.Warn()` | inspect `cmd/deploy.go:123-131` | ✅ Confirmed pattern |
| JSON envelope embeds Diagnostics in `warnings[]` | inspect `cmd/deploy.go:136-165` | ✅ Confirmed pattern |

No assumptions are unvalidated. Phase 1 may proceed.
