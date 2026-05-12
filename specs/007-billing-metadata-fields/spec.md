# Feature Specification: Billing Metadata Fields

**Feature Branch**: `007-billing-metadata-fields`
**Created**: 2026-05-10
**Status**: Draft
**Input**: User description: "Add two paired optional metadata fields to the fleet schema — Profile.Tier (advisory cost-tier label on profile definitions) and RepoSpec.CostCenter (free-form budget-attribution string on repo entries) — and surface both in `gh-aw-fleet list` output (text and JSON modes). Both are additive, both honor existing schema-version contracts, neither carries enforcement. They are prerequisite metadata for the future `gh-aw-fleet consumption` subcommand's `--by tier` and `--by cost-center` grouping keys. Tracks GitHub issues #54 (tier) and #55 (cost_center)."

## Clarifications

### Session 2026-05-10

- Q: Should the tool give `cost_center` any special handling beyond ordinary string fields (e.g., warn or block when it appears in the public `fleet.json` rather than the private `fleet.local.json`)? → A: No special treatment — cost_center is an ordinary string field; operators self-police via the existing public/private file split.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Cost-aware profile selection (Priority: P1)

A fleet operator preparing for the 2026-06-01 transition to usage-based Copilot billing wants to scan their fleet at a glance and immediately see which repos run cheap profiles versus expensive ones. Today they must cross-reference the workflow roster of each profile against their mental model of which workflows invoke expensive models — there is no first-class cost signal in the fleet configuration or in any tool output. With this feature, operators annotate each profile definition with an honest cost-tier label (e.g., `minimal`, `standard`, `premium`) and read the label back from the existing fleet listing command without any new flags.

**Why this priority**: The 2026-06-01 billing transition makes profile choice an explicit cost statement, not just a feature bundle. Operators who cannot see cost tier in their tooling fly blind into the new billing model. Surfacing tier in the listing command is the smallest behavior change that closes that visibility gap.

**Independent Test**: Annotate a profile in the example fleet configuration with a tier value, run the listing command, and confirm the tier appears unambiguously associated with that profile in both text and JSON output modes. Existing fleet configurations without tier annotations continue to load and list correctly with no migration step.

**Acceptance Scenarios**:

1. **Given** a fleet configuration where one profile declares `tier: "standard"`, **When** the operator runs the listing command in text mode, **Then** the output displays the tier value such that it is unambiguously associated with that profile name for each repo using the profile.
2. **Given** the same configuration, **When** the operator runs the listing command in JSON output mode, **Then** each row includes a structured mapping from profile name to tier value.
3. **Given** an existing fleet configuration with no tier annotations on any profile, **When** the operator runs the listing command in either mode, **Then** the command succeeds with no errors and the output reflects the absence of tier values gracefully — an empty mapping (not null) in JSON, no garbage characters in text mode.

---

### User Story 2 - Budget attribution per repo (Priority: P1)

A fleet operator at an organization that uses GitHub's cost-center budget controls — introduced with the same usage-based-billing rollout — wants each repo entry in their fleet configuration to declare which cost center its agentic-workflow consumption should attribute to. They use this attribution today as living documentation (so reviewers know which team a repo charges to) and as preparation for a future cross-fleet consumption rollup that will group spend by cost center.

**Why this priority**: Same billing-transition motivation as Story 1. The field is also a hard prerequisite for the planned cross-fleet consumption command's group-by-cost-center grouping key. Without this field, the future consumption command cannot group spend along the dimension operators care most about. P1 because the work is paired with Story 1 — same schema file, same listing surface, same release.

**Independent Test**: Set a cost-center value on a repo entry in the example fleet, run the listing command, and confirm the value renders in both text and JSON output modes. Confirm that repos without a cost-center value still appear correctly with the existing unset-string placeholder.

**Acceptance Scenarios**:

1. **Given** a repo entry annotated with `cost_center: "platform-eng"`, **When** the operator runs the listing command in text mode, **Then** a cost-center column displays `platform-eng` for that repo.
2. **Given** the same annotation, **When** the operator runs the listing command in JSON output mode, **Then** the row for that repo includes a `cost_center` field carrying the value `platform-eng`.
3. **Given** a repo entry with no cost-center declared, **When** the operator runs the listing command in text mode, **Then** the cost-center column displays the existing placeholder used for other unset string fields (the dash character).
4. **Given** the same repo entry, **When** the operator runs the listing command in JSON output mode, **Then** the row includes a `cost_center` field carrying the empty string — the field is always present.

---

### Edge Cases

- A fleet configuration with no tier or cost-center annotations on any profile or repo loads, validates, and produces listing output that is identical to today's for all non-new columns. The two new fields are purely additive.
- A profile declares a tier value outside the recommended `minimal | standard | premium` vocabulary. The value is preserved verbatim and surfaced as-is; nothing rejects it.
- A repo declares `cost_center: ""` (explicit empty string). The value is treated as equivalent to unset. On the on-disk `RepoSpec`, the serialized form drops the field via `omitempty`; on the JSON envelope's `ListRow.cost_center`, the field is always emitted as the empty string per FR-008. The text-mode listing command renders the unset placeholder.
- A repo carries multiple profiles, each with its own tier value. The listing command surfaces the per-profile mapping rather than collapsing the tiers into a single conflated value or dropping all but one.
- The public example fleet configuration and the canonical default-profile fixture must remain byte-identical for the default profile (existing project hard invariant). Both files MUST receive matching tier annotations in the same change.
- A private local-override configuration adds tier or cost-center values that do not exist in the public example. The existing precedence rules merge the override on top of the base configuration; new fields participate in the merge the same way every other optional field already does.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Profile definitions in the fleet configuration MUST accept an optional advisory `tier` string field.
- **FR-002**: Repo entries in the fleet configuration MUST accept an optional `cost_center` string field.
- **FR-003**: Both new fields MUST be additive — existing fleet configurations that do not declare either field MUST continue to load, validate, and list with no migration step and no error.
- **FR-004**: Neither field addition MUST cause a bump of the on-disk fleet configuration schema version. Both are additive on the schema contract.
- **FR-005**: The fleet listing command's default text output MUST display the tier value for each profile a repo uses, such that each tier is unambiguously associated with the profile it belongs to. Association is by position within parallel rendered slices — the i-th element of the tier rendering corresponds to the i-th profile name in the same row. The exact column shape is pinned by `contracts/list-text-output.md`.
- **FR-006**: The fleet listing command's default text output MUST display the cost-center value as part of each row, using the existing unset-string placeholder when the value is empty.
- **FR-007**: The fleet listing command's JSON output mode MUST include, for each row, a mapping from profile name to tier value. The mapping MUST be an empty mapping (not null) when no profiles in the row have tiers set.
- **FR-008**: The fleet listing command's JSON output mode MUST include, for each row, a `cost_center` string field that is always present, carrying the empty string when unset.
- **FR-009**: The JSON output envelope's schema version MUST remain unchanged — both field additions are additive on the envelope contract.
- **FR-010**: The system MUST NOT enforce any closed set of values for the tier field. Operator-chosen values outside the recommended vocabulary MUST be preserved and surfaced verbatim.
- **FR-011**: The system MUST NOT validate the cost-center value against any external registry. The field is a free-form label whose accuracy is the operator's concern.
- **FR-012**: The canonical default-profile fixture and the public example fleet configuration MUST both receive tier annotations in the same change, preserving the byte-identical-mirror invariant for the default profile.
- **FR-013**: Every profile shipped in the public example fleet configuration MUST receive an honest tier annotation — no profile may ship with the tier field missing or assigned a placeholder such as `unknown`.
- **FR-014**: The fleet listing command MUST remain a read-only operation. Neither field addition introduces any new mutating side effect on the listing command's behavior.
- **FR-015**: Operator-facing documentation — the project's primary agent-guidance file and the fleet onboarding / profile-build skills — MUST be updated to describe both fields' semantics: advisory labels, no enforcement, and their role as future group-by keys for the planned consumption command.
- **FR-016**: The system MUST NOT apply any special validation, warning, or blocking behavior to the `cost_center` field based on which configuration file it appears in. The field is treated identically to other free-form string fields; sensitivity classification is delegated to the existing public-versus-private file split that already governs every other repo-level field.

### Key Entities *(include if feature involves data)*

- **Profile**: A named bundle of workflows pulled from one or more upstream sources, used as the unit of fleet-wide assignment. Gains an optional advisory `tier` label that signals cost framing without enforcing it.
- **RepoSpec**: The desired-state record for a single target repository, listing which profiles it uses and overrides specific to that repo. Gains an optional free-form `cost_center` label for budget attribution.
- **ListRow**: One row of the fleet listing command's structured output (in both text and JSON modes), representing a single repo's resolved state. Gains a per-profile tier mapping and a cost-center string.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can identify any profile's cost tier in under 5 seconds by reading the fleet listing command's text output, without cross-referencing workflow rosters or any other document.
- **SC-002**: An operator can identify any repo's cost-center attribution in under 5 seconds via the same listing command — no separate query, no separate file.
- **SC-003**: 100% of existing fleet configurations (those that do not yet carry tier or cost-center fields) continue to load and produce listing output that matches today's output for every column that existed before the change.
- **SC-004**: 100% of profiles in the public example fleet configuration ship with tier annotations on the first release of the feature — no profile ships with a missing tier.
- **SC-005**: A consumer of the JSON output mode can deserialize the new fields using only the documented additive contract — no schema-version negotiation is required, and no consumer needs to special-case the absence of the new fields.
- **SC-006**: Round-trip serialization (load → save) of a fleet configuration with both new fields populated produces a byte-identical configuration file, including for configurations where either or both fields are unset.

## Assumptions

- The recommended `minimal | standard | premium` vocabulary for tier values is intentionally advisory. Tightening to a closed enum is a deliberate follow-up candidate once operators have used the field in practice and reported drift problems.
- Cost-center attribution is per-repo, not per-profile, because GitHub's billing model attaches cost centers to repos and users — not to workflow definitions. A future per-profile default could be added without rewriting this work.
- The public example fleet configuration leaves its sole repo entry (`rshade/gh-aw-fleet`) without a cost-center value so the public example demonstrates the unset case explicitly. Real fleets populate `cost_center` via their private local-override configuration.
- The work ships in a single bundled change rather than two sequenced changes because both field additions touch the same schema file, the same listing-command output surface, and the same future downstream consumer (the planned consumption command). The plan phase MAY revisit this if it surfaces a concrete sequencing concern.
- Tier values for the existing opt-in `*-plus` profiles in the example fleet are assigned by honest cost framing — agentic, PR-generating profiles receive `premium`; event-scoped, dormant-when-idle profiles receive `standard`; the foundational `default` profile receives `standard`. The exact per-profile assignment is an implementation decision resolved at plan time.
- The configuration loader's existing precedence rules between the public example fleet file and the private local-override file extend to the new fields without modification — tier and cost-center merge by the same per-key rule as every other field.
- No closed-enum validation, no upstream-registry lookup, and no policy enforcement (such as a per-repo `max_tier` guard rail) is in scope. All of those are deliberate follow-up candidates filed against the future consumption-command epic.
- Both fields are independent of the existing dry-run-by-default invariant — the listing command stays read-only and gains no new mutating behavior.
