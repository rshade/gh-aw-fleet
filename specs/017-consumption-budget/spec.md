# Feature Specification: Read-Only Over-Budget Highlighting in the Consumption Rollup

**Feature Branch**: `017-consumption-budget`
**Created**: 2026-06-22
**Status**: Draft
**Input**: User description: "feat(consumption): read-only over-budget highlighting in the rollup (--budget) — add an optional, read-only per-row AIC threshold to `gh-aw-fleet consumption` that highlights which rollup rows exceed a ceiling. Highlight, not enforce: no cap, no halt, no external alert, exit code unaffected by breaches. Additive `over_budget` JSON field, no schema bump. Tracks issue #129; reconciles with the 009 consumption spec's FR-023 (no alarm) via a decision record."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Flag over-threshold rows at a glance (Priority: P1)

A fleet operator runs the consumption rollup every morning to watch spend. Today the output is an unannotated table; to find the rows that are running hot, the operator has to read every row and compare each value against a ceiling held in their head. With this feature, the operator supplies a single per-row budget ceiling, and the rollup marks every row whose consumption exceeds that ceiling — so the hotspots jump out of the table without manual scanning.

**Why this priority**: This is the entire point of the feature — turning an unannotated table into one with an anomaly lens. Every other behavior (JSON field, sorting, footer annotation) layers on top of this core "mark the row" capability. If only this ships, the operator already gets the value: spend hotspots are visible at a glance.

**Independent Test**: Run the rollup against a fixture fleet whose groups have known consumption values, pass a budget ceiling that sits between two of those values, and confirm that exactly the rows above the ceiling are marked and the rows at or below it are not.

**Acceptance Scenarios**:

1. **Given** a fleet rollup where one group's consumption is above the supplied ceiling and another's is below it, **When** the operator runs the rollup with the budget ceiling, **Then** the over-ceiling group is visibly marked as over budget and the under-ceiling group is not.
2. **Given** the same fleet, **When** the operator runs the rollup with no budget ceiling supplied, **Then** the output is byte-for-byte identical to the rollup's pre-feature behavior (no marker, no extra column).
3. **Given** a budget ceiling equal to a group's exact consumption value, **When** the operator runs the rollup, **Then** that group is NOT marked (the threshold flags strictly-greater-than, not equal).

---

### User Story 2 - The threshold honors whichever grouping axis is active (Priority: P1)

The rollup already pivots across four grouping axes (by repo, by profile, by cost center, by workflow). An operator who is reviewing spend by cost center wants the budget ceiling applied to the cost-center rows; an operator reviewing by repo wants it applied to repo rows. The ceiling must be a per-row check against whatever axis the operator is currently viewing, not a single hard-coded axis.

**Why this priority**: P1 because a highlight that only worked on one axis would be a trap — the operator would silently get no annotation on three of four axes and misread that as "nothing is over budget." Correctness across all axes is part of the core promise, not an enhancement.

**Independent Test**: For each of the four grouping axes, run the rollup with a fixed budget ceiling against a fixture whose per-axis aggregates straddle that ceiling, and confirm the correct rows are marked on every axis.

**Acceptance Scenarios**:

1. **Given** a fleet grouped by cost center where one cost center's summed consumption exceeds the ceiling, **When** the operator runs the rollup by cost center with that ceiling, **Then** the over-ceiling cost-center row is marked.
2. **Given** the same ceiling and a repo that participates in two profiles such that one profile's additive total exceeds the ceiling, **When** the operator runs the rollup by profile, **Then** the over-ceiling profile row is marked according to its additive total (consistent with the existing double-counting semantic).
3. **Given** a fleet grouped by workflow, **When** the operator runs the rollup by workflow with a ceiling, **Then** the over-ceiling workflow rows are marked, including any that also appear in the top-burner footer block.

---

### User Story 3 - Machine-readable over-budget signal in JSON output (Priority: P2)

An operator (or downstream automation) consuming the rollup's structured output wants the over-budget determination available as data, not just as a visual marker in the table — so a dashboard, a report generator, or a notebook can read which groups breached the ceiling and echo back the ceiling that was applied.

**Why this priority**: P2 because the human-readable table (Stories 1–2) is already actionable on its own; the structured field is a useful additive surface for tooling but is not the entry point. It ships in the same slice because it reads the exact same per-group determination the table uses.

**Independent Test**: Run the rollup in structured-output mode with a budget ceiling and confirm each group carries a boolean over-budget indicator matching the table, that the supplied ceiling is echoed in the envelope, and that the envelope's schema version is unchanged from the pre-feature value.

**Acceptance Scenarios**:

1. **Given** structured-output mode and a supplied ceiling, **When** the operator runs the rollup, **Then** every group in the structured output carries an over-budget boolean whose value matches the table marker for that group, and the envelope echoes the supplied ceiling.
2. **Given** structured-output mode with no ceiling supplied, **When** the operator runs the rollup, **Then** the over-budget indicator and ceiling echo are omitted so the structured payload remains identical to the pre-feature behavior.
3. **Given** any combination of ceiling and grouping axis, **When** the operator runs the rollup in structured-output mode, **Then** the envelope's schema version is identical to its pre-feature value (the field is additive).

---

### Edge Cases

- **No data / nil consumption for a group**: A group whose consumption rolls up to an unknown value (e.g., a repo whose runs all failed, so the consumption metric is absent — the existing all-or-nothing nil-merge rule) MUST NOT be marked over budget. "Exceeds the ceiling" is undefined for an unknown value; the absence of data is not a breach. When a ceiling is supplied, such a group reports not-over-budget on both the table and the structured output.
- **Zero-consumption group**: A group with a genuine consumption of zero is never over a non-negative ceiling and is never marked.
- **Ceiling of zero**: A ceiling of zero is valid and flags every group with any positive consumption (strictly-greater-than zero). This is a legitimate "show me everything with any spend" mode, not an error.
- **Negative ceiling**: A negative ceiling is invalid input. The command rejects it with a usage error and a non-zero exit code. This is input validation and is categorically different from a budget breach (see the read-only / exit-code requirement below) — a malformed flag value is an operator error, while an over-budget row is a normal observation.
- **Non-numeric ceiling**: A ceiling that cannot be interpreted as a number is rejected with a usage error and a non-zero exit code.
- **Ceiling supplied with an empty fleet / no groups**: The rollup produces its normal empty result; there are simply no rows to evaluate, and the command still exits successfully.
- **Every row over budget**: All rows are marked; the command still exits successfully (a breach — even a fleet-wide one — never changes the exit code).
- **Top-burner footer interaction**: The per-workflow top-burner footer block, when present, is annotated on the same consumption metric as the rows, so an operator does not see a workflow marked in the body but unmarked in the footer (or vice versa).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The consumption subcommand MUST accept an optional per-row budget ceiling input expressed in the rollup's native billing unit (the same unit the rollup already sums and displays).
- **FR-002**: When a budget ceiling is supplied, the system MUST mark each output row whose aggregated consumption strictly exceeds the ceiling as "over budget," and MUST NOT mark rows whose consumption is equal to or below the ceiling.
- **FR-003**: The over-budget determination MUST be applied to whichever grouping axis the operator has selected (repo, profile, cost center, or workflow), evaluating each row against the ceiling using that row's aggregated consumption for the active axis.
- **FR-004**: For the profile grouping axis, the over-budget determination MUST use the same additive (double-counted) per-profile total the rollup already computes, so the highlight is consistent with the displayed value.
- **FR-005**: When a budget ceiling is supplied, the human-readable output MUST visually distinguish over-budget rows from compliant rows (for example, a dedicated indicator column and/or an inline marker) such that an operator can identify breaching rows without comparing numbers by hand.
- **FR-006**: When no budget ceiling is supplied, the human-readable output MUST be unchanged from its pre-feature form — no indicator column, no marker, no reordering.
- **FR-007**: When a budget ceiling is supplied, the structured (machine-readable) output MUST carry, per group, an additive boolean indicating whether that group is over budget, and MUST echo the supplied ceiling at the envelope level so consumers can see the basis of the determination.
- **FR-008**: The structured output's schema version MUST NOT change — every new field is additive, and any consumer that ignores the new fields MUST observe no behavioral change.
- **FR-009**: A group whose aggregated consumption is unknown/absent (the existing nil-consumption case) MUST be treated as not over budget on every output surface.
- **FR-010 (read-only / no enforcement)**: The feature MUST NOT cap, halt, gate, retry, or otherwise alter any fleet operation in response to a breach, and MUST NOT emit any external alert, notification, push, comment, or write of any kind. It only annotates the operator's own pulled output.
- **FR-011 (exit code unaffected)**: The command's exit code MUST be independent of how many rows breach the ceiling — any number of over-budget rows (including all rows) still exits successfully. Only malformed input (e.g., a negative or non-numeric ceiling) produces a non-zero exit, and that is input validation, not breach enforcement.
- **FR-012**: A ceiling of zero MUST be accepted and MUST flag every group with strictly-positive consumption; a negative or non-numeric ceiling MUST be rejected as invalid input with a clear, actionable message.
- **FR-013**: The over-budget evaluation MUST be a pure function of the already-aggregated rollup result — it MUST NOT trigger any additional data fetch, network call, or external read beyond what the rollup already performs, and MUST remain fully exercisable offline against captured fixtures.
- **FR-014 (reconciliation decision record)**: The project MUST record, under the existing consumption-subcommand specification, a short decision record that reconciles this feature with the prior requirement that the tool "MUST NOT enforce any spending cap, budget alarm, or hard limit." The record MUST articulate the distinction between *highlighting* (annotating read-only, operator-pulled output) and *alarming/enforcing* (pushing a signal outward or constraining behavior), and MUST affirm that this feature stays strictly on the highlight side of that line.
- **FR-015**: Operator-facing guidance for the budget-review flow MUST document the new ceiling input, its unit, the strictly-greater-than semantics, the nil-consumption exclusion, and the explicit statement that the highlight does not enforce or alert.

### Key Entities *(include if feature involves data)*

- **Budget Ceiling**: A single optional per-row consumption threshold supplied by the operator at invocation time, expressed in the rollup's native billing unit. Global to the invocation (one ceiling applied to every row of the active axis), not a per-axis or per-key map. Echoed back in structured output so consumers can see what basis produced the determinations.
- **Over-Budget Determination**: A per-group boolean derived purely from comparing the group's already-aggregated consumption against the supplied ceiling (strictly greater than → over budget; equal, below, or unknown → not over budget). When a ceiling is supplied, surfaced as a visual marker in human-readable output and as an additive boolean in structured output.
- **Consumption Group** (existing): One row of the rollup, keyed by the active grouping axis. This feature reads its existing aggregated consumption value; it does not change how groups are formed, summed, or ordered.
- **Result Envelope** (existing): The structured output container. When a ceiling is supplied, this feature adds the echoed ceiling and reads/propagates the per-group over-budget boolean, all additively, without altering the envelope's schema version.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator reviewing the rollup with a supplied ceiling can identify every over-budget row by the dedicated marker/column, with zero manual numeric comparison, on all four grouping axes.
- **SC-002**: For every grouping axis, the set of rows marked over budget exactly equals the set of rows whose aggregated consumption strictly exceeds the supplied ceiling — verified by table-driven tests over captured fixtures, with no false positives (including the nil-consumption and equal-to-ceiling cases) and no false negatives.
- **SC-003**: Running the rollup without a ceiling produces output identical to the pre-feature behavior in both human-readable and structured modes — confirming the feature is purely additive and opt-in.
- **SC-004**: The structured output's schema version is unchanged after this feature ships; a consumer that ignores the new fields observes no difference.
- **SC-005**: The command's exit status is invariant under the number of breaching rows — demonstrably 0 whether zero, some, or all rows are over budget — and is non-zero only for malformed ceiling input.
- **SC-006**: The feature adds no new third-party runtime dependency and introduces no new network or disk interaction; all new logic is exercised offline against the existing fixtures.
- **SC-007**: The full project quality gate (formatting, vetting, linting, and the complete test suite) passes cleanly with the new behavior and its fixtures included.
- **SC-008**: A reader of the consumption-subcommand specification can locate a decision record that unambiguously distinguishes this highlight feature from budget enforcement/alarming and confirms it does not contradict the prior "no alarm" requirement.

## Assumptions

- **Single global ceiling, not a per-axis map**: The feature ships one global ceiling applied uniformly to every row of the active axis. Per-axis or per-key ceilings (e.g., a different ceiling per cost center) are a possible future enhancement but are out of scope here; the issue explicitly recommends starting with a single global threshold.
- **Native billing unit, not a derived currency**: The ceiling is expressed in the rollup's native billing unit (the unit the rollup already sums and ranks on), because that is the authoritative unit and the displayed currency is a flat derivation of it. A currency-denominated ceiling, if ever desired, is a straightforward conversion of the native-unit ceiling and is not part of this slice.
- **Rows are annotated in place; order is not changed**: Over-budget rows keep their existing position in the output rather than being re-sorted to the top. The rollup already orders rows by consumption, so the highest spenders — and thus the breaching rows — already cluster near the top; an explicit re-sort is deferred as an optional future refinement and is not required for the at-a-glance value. (The issue raises sort-to-top as an open question; in-place annotation is the chosen default.)
- **Strictly-greater-than threshold semantics**: "Exceeds the ceiling" means strictly greater than the ceiling. A row exactly at the ceiling is within budget and is not marked. This matches the plain-language reading of "over budget."
- **The decision record can land in the same change as the feature**: The reconciliation decision record under the consumption-subcommand spec is treated as part of this feature's deliverable rather than a separate blocking prerequisite, since the record's content (highlight ≠ alarm) is fully determined by this feature's read-only design. (The issue raises whether the amendment must block; bundling it is the chosen approach.)
- **The nil-consumption rule is inherited, not redefined**: The existing all-or-nothing nil-merge behavior (a group with only failed runs rolls up to an unknown consumption value) is taken as-is; this feature only specifies that such a group is treated as not over budget.
- **Read-only category is inherited**: This feature stays in the same pure-read category as the existing rollup — no mutation of any local or remote state, consistent with the consumption subcommand's existing read-only requirement.
- **Documentation timing**: Operator-facing budget-review guidance is updated to describe the new ceiling input. If a separate drift fix to that guidance is in flight, this feature's documentation update is layered on top of the corrected baseline rather than forking it.

## Out of Scope

- Any form of enforcement: capping spend, halting or gating deploys/sync/upgrade, retrying, or constraining any fleet operation based on a breach.
- Any external signal: alerts, notifications, pushes, comments, issues, or any write to a remote or local store in response to a breach.
- Pre-spend forecasting / projection of future consumption (tracked separately).
- Runtime cap-hit diagnostics that react to a platform cap being hit during a run (tracked separately).
- Deploying or adopting an upstream cost-tracker workflow (a separate, intentionally-not-taken path).
- Per-axis or per-key ceiling maps, currency-denominated ceilings, and re-sorting breaching rows to the top — all possible future enhancements, none required here.
