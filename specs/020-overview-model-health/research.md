# Phase 0 Research: `overview` Model-Support Advisory

All spec Assumptions marked "planning-phase decision" are resolved below. No `NEEDS CLARIFICATION` markers remain.

## Decision 1 — Detection data source: failure-issue markers

**Decision**: Detect `model_not_supported` per repo by reading the repo's open **`agentic-workflows` failure issues** and matching the machine-readable marker `failure_categories` for `model_not_supported_error`. Do **not** extend the `gh aw logs --json` parse.

**Rationale**: Verified against a live failure issue (`rshade/finfocus#1349`, "[aw] Code Simplifier failed", state OPEN). Its trailing HTML comment is:

```html
<!-- gh-aw-failure-issue: true, workflow_id: code-simplifier, branch: main, failure_categories: agent_failure|model_not_supported_error -->
```

- The category token is **`model_not_supported_error`** (note the `_error` suffix — the issue's prose `failureClass=model_not_supported` is the runtime class, not the marker token). The parser matches the substring `model_not_supported` inside the pipe-delimited `failure_categories` value to be robust to either form.
- The decoded `logsRun` struct (`internal/fleet/consumption_logs.go`) exposes only `conclusion`, `aic`, and counts — **no failure category**. Extending it would require an upstream JSON field that does not exist today, so option 2 cannot satisfy FR-001 now.
- Failure issues carry `gh-aw-expires` (~7 days) and gh-aw auto-closes them, so **open-issue state is itself a bounded recency window** that aligns with `overview`'s trailing-7d default (spec Assumption on window/state semantics: "flagged reflects current open-issue state").

**Alternatives considered**:

- *Extend `gh aw logs --json`* — rejected: the current JSON schema carries no failure-category/model-detail field; relies on an unverified upstream shape.
- *Read per-run activation artifacts (`activation/models.json`, `awf-reflect.json`)* — rejected for v1: heavy new plumbing (download + unzip per run), out of proportion to a lightweight advisory.

**Marker-match rules (deterministic)**:

1. A body is a failure issue iff it contains `gh-aw-failure-issue: true`.
2. It is model-flagged iff its `failure_categories:` value (up to the next `,`/`-->`) contains the substring `model_not_supported`.
3. Fetch scope: `gh issue list --repo <repo> --label agentic-workflows --state open` (the `[aw] No-Op Runs` issue also carries this label but lacks the failure marker, so it is filtered out by rule 1).

## Decision 2 — Offending-model attribution: resolve the effective model from repo variables

**Decision** *(operator-selected)*: For **flagged repos only**, resolve the effective Copilot model and report it as the offending model:

```text
effective = vars.GH_AW_MODEL_AGENT_COPILOT
          || vars.GH_AW_DEFAULT_MODEL_COPILOT
          || ghAwBakedDefaultCopilotModel   // gh-aw's compiled-in default
```

read via `gh variable list --repo <repo> --json name,value` (seam `ghRepoVariablesAPI`).

**Rationale**: The failure-issue marker does **not** name the offending model, and the body's `400 The requested model is not supported.` has none; the `model: gpt-5-mini` in the body's "How to fix" block is a *suggested fix*, not the offending model — scraping it would mis-attribute. For a `model_not_supported` failure the *effective* `COPILOT_MODEL` is by definition the offending one, and gh-aw resolves it exactly as the fallback chain above (issue #201 Technical Notes: `COPILOT_MODEL: ${{ vars.GH_AW_MODEL_AGENT_COPILOT || vars.GH_AW_DEFAULT_MODEL_COPILOT || 'claude-sonnet-4.6' }}`). This actually names the model on the dashboard (SC-001) instead of always reporting "unknown".

**Cost / trade-off accepted**: one extra `gh variable list` call per *flagged* (already-broken) repo — negligible fan-out — plus a `ghAwBakedDefaultCopilotModel` constant that MUST track the fleet's `github/gh-aw` source pin. gh-aw changes this default across releases (it dropped `gpt-5-mini` in v0.81.5), so the constant's godoc records the pin it was verified against and flags it as a maintenance touch-point on gh-aw pin bumps.

**Fallback to unknown (FR-002 compliance)**: If `ghRepoVariablesAPI` errors for a flagged repo, the repo stays flagged (`status: affected`) with an **empty** offending-model set, rendered as `unknown`. Detection never fails the command because model *naming* failed.

**Known v1 limitation (documented, not a bug)**: repo-level variables + baked default resolve to one effective model per repo; a per-workflow frontmatter `model:` override (in the clone, not read here) is not consulted. The spec's "multiple offending models" edge case is still satisfied structurally (the field is a deduplicated, sorted set), and per-workflow resolution is a noted follow-on.

**Alternatives considered**:

- *Always report "unknown"* — rejected by operator: weakest outcome; the feature's title is literally "…failing on unsupported/deprecated Copilot models," so naming the model is the value.
- *Best-effort body regex* — rejected: matches nothing in today's gh-aw body and risks mis-attributing the "How to fix" suggestion.

## Decision 3 — Surfacing channels reuse existing seams

**Decision**: Reuse the three existing `overview` output channels; add no new table column.

- **Text (stdout)**: a `model-support:` line inside the affected repo's **per-repo detail block**. An affected repo is unhealthy by definition (its runs failed), so `needsOverviewDetail` already gates its detail block open — **no change to `needsOverviewDetail`**. Keeps the `REPO DRIFT RUNS FAIL NOOP HEALTH AIC COST` table layout byte-stable (spec Assumption).
- **Hint (stderr)**: one `DiagHint` per flagged repo, carrying the `fieldRepo` field so `splitOverviewDiags`/`emitOverviewDiagnostics` route and sort it through the existing hints channel exactly like `nilAICDiag`/`latestNoopDiagnostic`. Message names `GH_AW_DEFAULT_MODEL_COPILOT` (FR-004, SC-005).
- **JSON**: additive `model_support` pointer field (see Decision 4).

**Rationale**: Matches the spec's clarifications (both channels) and the established diagnostic-routing pattern. Reusing `DiagHint` avoids adding a `fleetdiag` code and the accompanying `splitOverviewDiags` hint-set entry; the machine-readable surface is the JSON field, not a diag code.

**Undetermined placement**: When the failure-issue read itself fails for a repo, its status is `undetermined`. Per FR-011 this appears in text **only inside a detail block that already renders for another reason** (drift / unhealthy / runs-unavailable) — `overview` never forces a new detail block for an undetermined status — and **always** in JSON. So a fully healthy, drift-aligned fleet stays unchanged (SC-003).

## Decision 4 — JSON shape: additive `model_support` pointer, mirroring `drift_detail`

**Decision**: Add a `ModelSupport` pointer field to `RepoOverview`, mirroring the existing `DriftDetail` pointer:

```go
ModelSupport *ModelSupport `json:"model_support,omitempty"` // mirrors DriftDetail *RepoStatus `json:"drift_detail,omitempty"`
```

Present only when the repo is `affected` or `undetermined`; **omitted entirely** when clean, so clean rows stay byte-identical to today (FR-005, FR-006, SC-004).

**Rationale**: The pointer-with-`omitempty` idiom is already the codebase's convention for "additive nested detail present only when non-clean" (`DriftDetail`). No `cmd.SchemaVersion` bump; no existing field renamed/removed/repurposed — exactly how AIC/Source were added (FR-006). The `nilAICDiag`/pointer patterns confirm the wire-contract discipline.

## Decision 5 — Exit code stays drift-gated

**Decision**: `overviewExitCode` is unchanged; it keys on `Total.Drifted + Total.Errored` only. A model-support failure never changes the exit code (FR-007, SC-003).

**Rationale**: Model failures are advisory like all run failures today. No new counter feeds the exit gate.

## Decision 6 — Determinism

**Decision**: Offending-model sets are deduplicated and sorted; the per-repo iteration in `Overview` is already over the sorted `overviewRepoNames`; hint diagnostics sort by `fieldRepo` in `splitOverviewDiags`. Fixtures drive all tests offline via the seams (FR-012, SC-002).

## Open items

None. All FRs map to a decision above; SC-006 fixtures (model-failure / unrelated-failure / healthy / undetermined) are enumerated in `quickstart.md` and `data-model.md`.
