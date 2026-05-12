# Phase 0 Research: Billing Metadata Fields

**Feature**: 007-billing-metadata-fields
**Date**: 2026-05-10

## Scope

The spec (`spec.md`) carries no `[NEEDS CLARIFICATION]` markers and the single open question was resolved in the `/speckit-clarify` pass (recorded as FR-016: no special handling for `cost_center`). This document records the design decisions that *were* made — implicit in the spec or settled during the user-shaped prompt — so that a reviewer or downstream agent can trace each choice to its rationale without re-reading the conversation.

## Decisions

### Decision 1: Field names — `tier` and `cost_center`

**Decision**: Use JSON keys `tier` (snake-case singular) on `Profile`, and `cost_center` (snake-case, underscore-separated) on `RepoSpec`. Go field names `Tier` and `CostCenter`.

**Rationale**:

- `tier` matches the GitHub blog post terminology around usage-based billing ("cost tier") and aligns with the recommended vocabulary `minimal | standard | premium`.
- `cost_center` exactly matches GitHub's billing-controls language (cost-center budgets are a named GitHub concept introduced with the 2026-06-01 transition). Using the same term avoids translation friction when operators map fleet config to GitHub's billing UI.
- Both follow the existing snake-case convention in `fleet.json` (e.g., `loaded_from`, `safe_outputs`).

**Alternatives considered**:

- `cost-tier` (kebab-case): rejected because the rest of the JSON schema uses snake-case.
- `costCenter` (camelCase): rejected for the same reason.
- `budget` (semantic generalization): rejected because GitHub's term is "cost center"; using a synonym creates documentation drift.
- `tier_label` / `cost_center_label`: rejected as redundant (every JSON field is a label).

### Decision 2: Tier vocabulary is advisory

**Decision**: Ship `minimal | standard | premium` as the recommended vocabulary. No closed enum, no validation. Operator-chosen values outside the vocabulary are preserved verbatim (FR-010).

**Rationale**:

- Vocabularies tighten well; they don't loosen well. Shipping advisory leaves room to tighten later (after operators have used the field and reported drift) without breaking existing configs.
- Enforcement requires a closed-set definition. Choosing the right set is harder than ship-now-fix-later: an operator might want `dev | prod`, `cheap | expensive`, or org-specific tiers. The fleet tool has no authority to pick a global vocabulary.
- Consistent with the "no enforcement" theme already in the spec (Story 1 priority rationale, Assumptions).

**Alternatives considered**:

- Closed enum (`minimal` | `standard` | `premium`): rejected as above — shipping a closed enum on first release is premature commitment.
- Free-form with no recommended vocabulary: rejected because documentation needs *some* example values for operators to start from. The "recommended" framing in godoc satisfies that without enforcement.

### Decision 3: Tier exposed in JSON as per-profile map

**Decision**: `ListRow.ProfileTiers map[string]string`, keyed by profile name, value is the tier string. Empty map (not nil) when no profiles in the row have tiers set.

**Rationale**:

- A repo can carry multiple profiles (e.g., `["default", "security-plus"]`), each with its own tier. Surfacing a single conflated value would lose information; surfacing an array parallel to `Profiles` would couple ordering and risk drift when one is updated and the other isn't.
- The map structure is self-describing: a consumer iterating `Profiles` can look up each name in `ProfileTiers` and gracefully handle absence (which means "this profile has no tier set").
- Empty-but-not-nil mirrors the existing convention for `Profiles`, `Workflows`, `Excluded`, `Extra` in `ListRow` (per `list_result.go:74-79` `nonNilStrings` helper). Consistent contract surface for envelope consumers.

**Alternatives considered**:

- Single `Tier string` field at row level: rejected because a repo with two profiles each carrying a different tier cannot be represented.
- Parallel `Tiers []string` array: rejected for the ordering-drift risk.
- Embed tier inside the existing `Profiles []string` array (e.g., `"default:standard"`): rejected because consumers parsing the JSON envelope shouldn't have to split strings; that's a presentation concern, not a data concern.

### Decision 4: Cost-center exposed as plain string

**Decision**: `ListRow.CostCenter string` (always emitted in JSON, empty string when unset). Matches the existing `Engine` field's convention.

**Rationale**:

- Cost-center is per-repo by GitHub's billing model — repos and users attach to cost centers, not workflow definitions. A simple string field maps 1:1.
- Matching `Engine`'s "always-emitted empty string" pattern means JSON consumers don't need new handling: every existing tool that handles `Engine == ""` already has the right shape for `CostCenter == ""`.
- Documented in spec FR-008.

**Alternatives considered**:

- Pointer `*string` with `omitempty`: rejected because it complicates consumers (null vs unset distinction) and doesn't add information (the spec treats `""` and missing as equivalent per the edge-cases section).
- Per-profile cost-center map (parallel to ProfileTiers): rejected because GitHub's model is per-repo, not per-workflow.

### Decision 5: Text-mode renders tier in a parallel TIERS column; cost-center as a separate column

**Decision** (revised at `/speckit-plan` time, 2026-05-10, per operator preference):

- A new `TIERS` column is added between `PROFILES` and `ENGINE`. The cell is a bracketed slice whose positions correspond 1:1 with the same row's `PROFILES` cell — `PROFILES [default quality-plus]` pairs with `TIERS [standard premium]`. Profiles whose underlying definition carries no `tier` render as the empty-string slot (e.g., `PROFILES [default custom]` → `TIERS [standard ]`). When **no** profile in the row has a tier, the cell renders `[]` exactly like today's empty-slice convention.
- A new `COST_CENTER` column is appended to the right of the existing columns. Uses `orDash(s)` (`cmd/list.go:62`) for the unset case.

**Rationale**:

- **Greppable.** `gh-aw-fleet list | grep premium` returns hits in the tier column without partial-token noise. Inline `default:standard` would also match but the column position would not be predictable for column-aware tooling.
- **Familiar formatting.** Mirrors the existing `[default]` slice formatting style; one extra `tabwriter` column, no new format-string vocabulary.
- **Cheap to render.** `BuildListResult` already iterates `spec.Profiles` in order; emitting a parallel `[]string` of tiers in the same pass is constant-time per element.

**Trade-off accepted**: Position-based binding means a future change that sorts one column without sorting the other would silently misalign tier and profile. Mitigation — both slices are built in a single pass over `spec.Profiles`. A reviewer would catch any reorder that touched only one column.

**Alternatives considered**:

- **Inline `name:tier` inside the existing `PROFILES` cell** (e.g., `[default:standard, security-plus:premium]`). This was the original prior-pass design. Rejected at `/speckit-plan` time on operator preference for greppability and a predictable column position. The colon separator was unambiguous-in-practice but required a new helper (`renderProfilesWithTiers`) and broke the natural `%v`-on-`[]string` formatting symmetry.
- **Single conflated `TIER` column** (one value per row): rejected because a repo with two profiles each on a different tier cannot be represented.
- **Cost-center inline with the repo name**: rejected because `REPO` is a stable identifier scripts grep by; appending metadata there breaks them.

**Empty handling**:

- Row whose profiles have no tiers: `TIERS` cell renders `[]` (matches the existing slice-empty convention; FR-007 covers JSON's `{}`).
- `COST_CENTER` cell uses `orDash` — value or `-`, never empty.

### Decision 6: Default profile mirroring stays byte-identical

**Decision**: The `default` profile in `fleet.json` and the entire contents of `profiles/default.json` MUST receive the same `tier: "standard"` annotation in the same change, and the two files MUST remain byte-identical for the `default` profile per the existing AGENTS.md hard invariant.

**Rationale**:

- The invariant exists today; this work doesn't introduce it. The decision is to *honor* it, not to relax or tighten it.
- Tier on `default` is the simplest case: `standard` reflects honest cost framing (the foundational profile is mid-cost, not the cheapest available).
- For the `*-plus` profiles (only in `fleet.json`, not in `profiles/default.json`), tier assignments per honest cost framing:
  - `quality-plus`, `security-plus`, `docs-plus` → `premium` (PR-generating agentic loops).
  - `community-plus` → `standard` (event-scoped, dormant when idle).
  - `observability-plus` → `premium` (the api-consumption-report runs daily as an LLM workflow per the recent #56 shipping note).

**Alternatives considered**:

- Move tier to `fleet.json` only (skip `profiles/default.json`): rejected because the invariant doesn't permit this — the two must mirror for `default`.
- Assign every profile `standard` to defer the cost-framing choice: rejected because that would ship a misleading uniform label that operators would have to correct on first review.

### Decision 7: Single-PR vs paired-PR sequencing

**Decision**: Ship both fields in a single PR. The four implementation steps (schema, envelope, text rendering, fixtures+docs) can land as one to four commits within the PR per reviewer preference, but the review surface is one PR.

**Rationale**:

- Same files touched in both cases (`schema.go`, `list_result.go`, `cmd/list.go`).
- Same review concerns (advisory-only, no enforcement, additive on both schema-version contracts).
- Same downstream consumer (the planned `consumption` subcommand, #57).
- The spec's "Sequencing note" already calls for paired PR; plan re-affirms.

**Alternatives considered**:

- Two sequenced PRs (tier first, cost-center second): rejected because the second PR would re-touch every file the first touched, doubling review overhead with no isolation benefit.
- Two parallel PRs: rejected because file overlap creates a guaranteed merge conflict.

### Decision 8: No new third-party dependencies (for the billing-metadata slice)

**Decision**: This slice uses only the standard library and the already-approved direct dependencies (cobra, zerolog, yaml.v3, gitleaks/v8 — none of which are touched by this feature). The bundled HuJson migration (issue #73) is a separate slice and introduces its own direct dependency (`github.com/tailscale/hujson`) under the same constitutional carve-out — that addition is owned by issue #73's plan, not by this one.

**Rationale**:

- Constitution v1.1.0 § Third-Party Dependencies requires an amendment for any new direct dependency. None is justified for the billing-metadata work: the changes are pure-Go struct extensions and `encoding/json` (stdlib) handles serialization.
- The user prompt for the 007 slice explicitly states "No new third-party dependencies" as a constraint. The hujson addition was scoped, justified, and approved under a separate workstream (issue #73) before being bundled into the same PR.

**Alternatives considered**: none — no third-party need surfaced during design of the billing-metadata fields themselves.

## Notes for Phase 1

- The data-model section will document the three modified entities (`Profile`, `RepoSpec`, `ListRow`) and the new `ProfileTiers` map shape.
- The contracts section will provide a concrete envelope example and a text-mode rendering example.
- The quickstart will walk an operator through: setting tier on the `default` profile, setting cost-center on a private repo entry, viewing the result in text and JSON modes.
