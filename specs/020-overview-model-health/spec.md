# Feature Specification: Flag Repos Failing on Unsupported/Deprecated Copilot Models in `overview`

**Feature Branch**: `020-overview-model-health`  
**Created**: 2026-07-08  
**Status**: Draft  
**Input**: User description (GitHub issue #201): "overview: flag repos failing on unsupported/deprecated Copilot models"

## Clarifications

### Session 2026-07-08

- Q: How should the model-support flag (repo + offending model) and its `GH_AW_DEFAULT_MODEL_COPILOT` remediation appear in the human-readable output? → A: Both channels — the flag and offending model(s) render as lines in the affected repo's per-repo detail block (stdout); the remediation renders through the existing hints channel (stderr). A repo that fails because its effective model does not exist is unhealthy by definition, so it already trips the detail-block render gate; no new gating is required for affected repos.
- Q: What shape should the additive `overview --output json` model-support field take? → A: A single nested object `model_support` (serialized as a pointer with `omitempty`, mirroring `drift_detail`) carrying the status and the set of offending model identifiers. Present when the repo is affected or undetermined; omitted entirely when the repo is clean, so clean rows stay byte-identical to today's output.
- Q: Where should an "undetermined" model-support status appear in human-readable text? → A: Only inside a detail block that is already rendering for another reason (drift, unhealthy, or runs-unavailable); `overview` never forces a new detail block solely for an undetermined status. JSON always carries it (per FR-005). This keeps a fully healthy, drift-aligned fleet's text output unchanged (SC-003) and requires no change to `needsOverviewDetail` gating.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See which repos are failing on an unsupported model (Priority: P1)

As a fleet operator, when I run the read-only `overview` dashboard, I want it to tell me *which* repos have agentic workflows failing because their effective Copilot model is unsupported for that repo's subscription (or has been dropped by gh-aw), and *which* model is at fault — so I learn about it from the one dashboard I already check, instead of reverse-engineering it from per-run logs after every scheduled run has silently failed.

**Why this priority**: This is the whole point of the feature. Today the operator cannot distinguish a model failure from any other failure in `overview`; the failure is silent and fleet-wide (every repo on the same subscription fails the moment its own unrelated blockers clear). Surfacing the repo and the offending model is the minimum that delivers value and is independently shippable.

**Independent Test**: Run `overview` against a fleet fixture containing one repo whose recent agentic runs failed on an unsupported model. The affected repo is named in the output together with the offending model; repos without model failures are not flagged. Fully testable with fixtures, no live network.

**Acceptance Scenarios**:

1. **Given** a repo whose recent agentic runs failed because the effective model is unsupported, **When** the operator runs `overview`, **Then** the output identifies that repo as having a model-support problem and names the offending model.
2. **Given** a fleet where no repo has a model-support failure, **When** the operator runs `overview`, **Then** no model-support advisory appears and the dashboard is unchanged from today's behavior.
3. **Given** a repo that failed for reasons unrelated to the model (e.g., a compile/setup failure), **When** the operator runs `overview`, **Then** that repo is counted as a failure as it is today but is **not** flagged as a model-support problem (no false positive).

---

### User Story 2 - Get an actionable remediation without leaving the dashboard (Priority: P2)

As a fleet operator who has just been told repo X is failing on model Y, I want the advisory to tell me how to fix it, so I can act immediately without recalling the mechanism or searching prior incidents.

**Why this priority**: Detection without remediation still forces the operator to remember that the fix is an Actions variable. Bundling the remediation turns a diagnosis into a one-step fix. It builds directly on P1 and is a small, independently demonstrable increment.

**Independent Test**: In the same fixture as P1, confirm the output includes a remediation hint that points the operator at the mechanism for overriding the model (the `GH_AW_DEFAULT_MODEL_COPILOT` variable) for the flagged repo.

**Acceptance Scenarios**:

1. **Given** a repo flagged with a model-support failure, **When** the operator reads the `overview` output, **Then** a remediation hint names the `GH_AW_DEFAULT_MODEL_COPILOT` mechanism as the fix.
2. **Given** no repo is flagged, **When** the operator runs `overview`, **Then** no model remediation hint is emitted.

---

### User Story 3 - Consume the signal programmatically (Priority: P3)

As an operator building automation on top of `overview --output json`, I want the model-support status exposed as a machine-readable field per repo, so I can alert, filter, or gate on it without parsing human-readable text.

**Why this priority**: The primary human workflow (P1/P2) delivers value on its own; JSON consumption is an additive convenience for downstream tooling and can ship after the text path. It must be additive so existing JSON consumers are unaffected.

**Independent Test**: Run `overview --output json` against the P1 fixture and confirm each repo row carries a new field describing its model-support status (including the offending model when flagged), that the field is absent/empty for unaffected repos, and that no existing field or the envelope's schema version changed.

**Acceptance Scenarios**:

1. **Given** a repo flagged with a model-support failure, **When** the operator runs `overview --output json`, **Then** that repo's row carries the model-support status and the offending model in a new additive field.
2. **Given** an existing JSON consumer that ignores unknown fields, **When** it reads the new output, **Then** it continues to work because the envelope schema version is unchanged and no prior field was renamed, removed, or repurposed.

---

### Edge Cases

- **Model name not recoverable**: the failure signal exists but does not name the offending model. The repo is still flagged as having a model-support problem; the model is reported as unknown/unavailable rather than dropping the flag.
- **Multiple offending models**: different workflows in one repo fail on different unsupported models. The offending-model field is a deduplicated, deterministically-ordered set, so the output shape supports reporting several models. **v1 limitation**: detection resolves a single *effective* Copilot model per repo (from repo Actions variables plus the gh-aw baked default), not a per-workflow model, so in practice a repo reports one offending model even when distinct workflows failed on distinct models; per-workflow model attribution is a noted follow-on (see plan-phase research on offending-model attribution).
- **Mixed outcomes**: some runs in the window succeeded while others failed on the model. The repo is still flagged (the operator needs to know some scheduled runs are failing) while existing health counts continue to reflect the mix.
- **Detection source unavailable**: the per-repo signal that distinguishes model failures cannot be retrieved (e.g., API error, or the run-log fan-out was already reported as unavailable for that repo). The rest of `overview` still renders; the model-support status for that repo is reported as undetermined — always in JSON, and in human text only where a detail block already renders for another reason — and the overview does not error out solely because model detection failed.
- **Stale signal**: a repo was fixed but a residual failure indicator persists. Detection reflects whatever the chosen source reports for the active window/state; the operator can re-run after the fix clears (see Assumptions on window/state semantics). Note that the reported offending model is the repo's *current* effective model: if the operator has already corrected the model configuration but the failure indicator has not yet cleared, the named model reflects current config rather than the historical failing model. The flag and its named model clear together on the next run once the underlying indicator resolves.
- **Exit code with a model failure present**: a model-support failure exists but the fleet is otherwise drift-aligned. The command's exit code is unchanged (success) — model failures are advisory, exactly like other run failures today.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `overview` MUST detect, per repo in scope, whether the repo's recent agentic-workflow runs failed because the effective Copilot model is unsupported or deprecated, distinguishing this from run failures of other kinds.
- **FR-002**: When a repo is flagged, `overview` MUST report the offending model identifier(s); when the signal does not name a model, it MUST still flag the repo and report the model as unknown.
- **FR-003**: `overview` MUST surface the flag in the human-readable output as a text annotation in the affected repo's per-repo detail block (stdout) that names the repo (and the offending model) without requiring the operator to open per-run logs.
- **FR-004**: `overview` MUST emit a remediation hint through the existing hints channel (stderr) for each flagged repo that points at the `GH_AW_DEFAULT_MODEL_COPILOT` override mechanism.
- **FR-005**: `overview --output json` MUST expose the per-repo model-support status as a new **additive** nested object field named `model_support` (serialized as a pointer with `omitempty`, mirroring `drift_detail`) on the existing per-repo row, carrying the status and the set of offending model identifiers. The field MUST be present when the repo is affected or undetermined and omitted entirely when the repo has no model-support problem, so clean rows are byte-identical to today's output.
- **FR-006**: The addition MUST NOT bump the JSON output envelope's schema version and MUST NOT rename, remove, or repurpose any existing field (mirroring how AIC/Source were added additively).
- **FR-007**: The command's exit code MUST remain gated on drift only; a model-support failure MUST NOT change the exit code (it is advisory, like all run failures today).
- **FR-008**: `overview` MUST NOT emit any model-support advisory (text or hint) when no repo in scope has a model-support failure — a healthy fleet's output is unchanged.
- **FR-009**: A repo that fails for reasons unrelated to the model MUST NOT be flagged as a model-support problem (no false positives).
- **FR-010**: The detection MUST cover both causes with a single signal: a model unsupported for the repo's subscription and a model deprecated/dropped by gh-aw. The advisory need not distinguish which of the two caused a given failure.
- **FR-011**: When the model-support signal cannot be determined for a repo, `overview` MUST degrade gracefully — render the rest of the dashboard, report that repo's model-support status as undetermined, and not fail the whole command solely because model detection failed. The undetermined status MUST always appear in JSON (per FR-005); in human text it MUST appear only within a detail block that already renders for another reason (drift, unhealthy, or runs-unavailable). `overview` MUST NOT force a new detail block solely for an undetermined status, so a fully healthy, drift-aligned fleet's text output is unchanged.
- **FR-012**: Output that lists flagged repos and offending models MUST be deterministically ordered so runs are reproducible and fixture-testable.

### Key Entities *(include if data involved)*

- **Model-support status (per repo)**: an advisory attached to each `overview` repo row indicating whether that repo's recent agentic runs failed on an unsupported/deprecated model. Attributes: affected (yes / no / undetermined) and the set of offending model identifiers (possibly empty/unknown). In JSON it serializes as a nested `model_support` object (pointer + `omitempty`) present only when the repo is affected or undetermined. Additive to the existing repo row; does not replace drift or health status.
- **Model-failure signal (source)**: the per-repo evidence that a run failed specifically because of the model. It carries, at minimum, the fact of a model-support failure and, when available, the offending model name. The concrete source is an implementation decision for the planning phase (see Assumptions).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator running `overview` on a fleet that includes a repo failing on an unsupported model learns the affected repo and the offending model from the dashboard alone, opening **zero** per-run logs to diagnose it.
- **SC-002**: In fixture tests, repos with a model-support failure are flagged and repos without one are not — **100%** of the fixture cases classify correctly (no false positives, no missed model failures).
- **SC-003**: For a fully healthy, drift-aligned fleet, `overview` output is unchanged from today's behavior (no new lines, no schema-version change) and the exit code stays success.
- **SC-004**: Existing `overview --output json` consumers that read current fields continue to parse successfully with **no** changes, because the schema version is unchanged and only additive fields appear.
- **SC-005**: Every flagged repo in the output is paired with a remediation hint referencing `GH_AW_DEFAULT_MODEL_COPILOT`, so the operator can act without consulting prior incidents.
- **SC-006**: `make ci` passes (fmt, vet, lint, full test suite green) with fixtures covering at least: one model-failure repo, one unrelated-failure repo, one healthy repo, and one undetermined/signal-unavailable repo.

## Assumptions

- **Data source is a planning-phase decision.** The spec requires a per-repo signal that distinguishes model failures from other failures and names the offending model; it does not mandate *how*. The issue recommends parsing the auto-filed `agentic-workflows` failure issues for the `model_not_supported_error` category (already reachable via `gh`, needs no new run-log plumbing); the alternative is extending the existing `gh aw logs --json` parse if that JSON carries a failure-detail/model field. `/speckit-plan` selects between them; both satisfy every FR here.
- **Window/state semantics follow the chosen source.** If detection reads open failure issues, "flagged" reflects current open-issue state (persists until the issue clears); if it reads the run-log window, "flagged" reflects failures inside the active `overview` window. Either is acceptable for v1; the operator re-runs after remediation to confirm the flag clears. The default `overview` window remains trailing 7 days.
- **Surfacing style reuses existing channels.** The flag and offending model(s) render as lines in the existing per-repo detail block (stdout); the remediation renders through the existing hints channel (stderr) — not a new always-on table column — keeping the `REPO DRIFT RUNS FAIL NOOP HEALTH AIC COST` table layout stable. Because a repo whose effective model does not exist is unhealthy by definition, it already satisfies the detail block's render gate. This matches the acceptance criterion "annotates the affected repo (text)."
- **Single model-support class.** "Unsupported for the subscription" and "deprecated/dropped by gh-aw" both surface at runtime as the same model-not-supported failure and are treated as one advisory class; the feature does not attempt to sub-classify the root cause.
- **Read-only and no new persistence.** `overview` remains a pure read dashboard: no cache, no baseline, no on-disk state. The model-support status is transient on the result row, consistent with AIC/Cost.
- **No new third-party dependencies** are required (consistent with the project's dependency constitution); detection reuses surfaces already reachable via `gh`/`gh aw`.
- **Fleet scope is unchanged.** Detection runs only for repos already in `overview` scope (the scoped `cfg.Repos`), honoring any repo arguments passed to the command.

## Out of Scope

- **Proactive static advisory** — a scanner-style check of each repo's *configured* model against a known-deprecated list (à la the Renovate/Dependabot scanners). The effective model lives in an Actions **variable**, not the clone, so a clone-only scanner cannot see it, and it would require sourcing/maintaining a deprecated-model list. Noted as a follow-on (could live under the security-scanner epic, issue #36).
- **Auto-remediation** — the feature never sets `GH_AW_DEFAULT_MODEL_COPILOT` (or any variable) on the operator's behalf; it only advises.
- **Sub-classifying the failure** into "unsupported-for-subscription" vs "deprecated-by-gh-aw" — treated as one class (see Assumptions).
- **Changing exit-code semantics** — model failures remain advisory; the command stays drift-gated.
