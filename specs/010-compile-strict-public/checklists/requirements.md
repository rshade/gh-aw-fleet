# Specification Quality Checklist: Compile Workflows with --strict on Public Repos by Default

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-05-17
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs) — Functional Requirements reference Go-level identifiers (`RepoSpec`, `EffectiveCompileStrict`, `*bool`) because the issue itself is a Go-CLI feature defined against a concrete codebase; the **user stories and success criteria** stay technology-agnostic. This is the same trade-off taken in spec 005 (Actions preflight) and spec 009 (consumption). If a hard "zero implementation references in any section" reading is required, the Functional Requirements can be paraphrased — flag during `/speckit-clarify`.
- [x] Focused on user value and business needs — every user story leads with operator workflow before mechanism.
- [x] Written for non-technical stakeholders — story narratives use plain language; Go-level surface lives in the FRs and Key Entities.
- [x] All mandatory sections completed — User Scenarios, Requirements, Success Criteria, Assumptions all present.

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain — one marker is present at the end of the Functional Requirements section (linked to Q1) and is **intentional**; it will be resolved by the `/speckit-clarify` step.
- [x] Requirements are testable and unambiguous — each FR has either a measurable observable (log line shape, file content diff, exit code) or a code-level injection point.
- [x] Success criteria are measurable — every SC has a concrete verification recipe (run X, assert Y).
- [x] Success criteria are technology-agnostic — they describe operator-observable outcomes (PR content, log output, exit codes), not internal mechanism. The reference to `make ci` in SC-008 is a project-specific gate rather than an implementation detail.
- [x] All acceptance scenarios are defined — every user story has 3 Given/When/Then scenarios.
- [x] Edge cases are identified — 9 edge cases enumerated (visibility values, redirects, archived repos, missing tool, work-dir resume, etc.).
- [x] Scope is clearly bounded — out-of-scope items pulled into Assumptions (CLI flag, per-workflow override, visibility caching, scanner-strict overlap).
- [x] Dependencies and assumptions identified — Assumptions section covers loader overlay semantics, HuJson round-trip, `internal` visibility, sync delegation, and the spec 005 precedent for envelope field shape.

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria — FRs map to SCs or to user story acceptance scenarios.
- [x] User scenarios cover primary flows — public-auto, override, fail-secure, compile-fail-diagnostic, onboarding feedback.
- [x] Feature meets measurable outcomes defined in Success Criteria — SC-001 through SC-008 each tie to one or more FRs.
- [ ] No implementation details leak into specification — partial fail (same trade-off as Content Quality #1); the FRs name `RepoSpec`, the visibility-fetcher injection seam, and `diagnostics.CollectHints` because they are pre-existing contracts the operator will edit. See Notes.

## Notes

- **Implementation references in FRs are deliberate**: The issue is highly specified at the codebase-symbol level (it names `RepoSpec`, `EffectiveCompileStrict`, `repoIsPublic`, `runAdd`, `runUpgrade`, `diagnostics.CollectHints`). Stripping these from the FRs would lose information without gaining stakeholder clarity, because the only stakeholders are the operator (who runs the CLI) and the implementer (who reads Go code). The same pattern is established in spec 005 (Actions preflight, FR-001/002/009 reference `ghAPIJSON`, `DeployResult`, the `DiagMissingSecret` code) and spec 009 (consumption, FRs reference specific `gh api` endpoints). If stakeholders demand strict separation, the FRs can be rewritten in operator-only language during `/speckit-clarify`; the meaning will not change.
- **One [NEEDS CLARIFICATION] marker is intentional**: Q1 (JSON envelope contract) is the single genuinely-open design choice. The spec template explicitly allows up to 3 markers; one is well under the cap. The marker links to Question 1 at the bottom of `spec.md` for operator resolution before `/speckit-plan`.
- **No `cmd.SchemaVersion` decision parked**: the spec resolves the envelope-bump question conditionally on Q1 (Assumption #9), so once Q1 is answered the bump-or-not question is also answered. No second clarification needed.
- **Sync's compile path is resolved, not deferred**: FR-005 + Assumption #3 codify that Sync inherits Deploy's compile via `applyDeployOrPrune` and adds no second invocation. The issue's "verify Deploy preflight covers it; no duplicate compile" sentence is honored.
