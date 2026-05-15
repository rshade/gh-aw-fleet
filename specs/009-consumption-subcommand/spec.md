# Feature Specification: Consumption Subcommand

**Feature Branch**: `009-consumption-subcommand`
**Created**: 2026-05-13
**Status**: Draft
**Input**: User description: "Add a new read-only subcommand `gh-aw-fleet consumption` that aggregates per-repo `api-consumption-report` outputs across the fleet, with three temporal modes (`--latest`, `--trailing <Nd>`, `--since <date>`) and a grouping selector (`--by repo|profile|cost-center|workflow`). Two-layer fetch: discovery via fleet-repo discussions filtered by category and HTML-comment tracker marker; data via run-artifact JSON. No prose parsing. Tracks GitHub issue #57; depends on the api-consumption-report workflow being deployed to fleet repos (separate issue), and on the already-shipped `Profile.Tier` and `RepoSpec.CostCenter` metadata fields from 007-billing-metadata-fields."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Fleet-wide consumption rollup at a glance (Priority: P1)

A fleet operator preparing for or already operating under usage-based Copilot billing wants to see, in one command, how their fleet is consuming API budget — without clicking through ten separate per-repo discussions every morning. Each repo in the fleet that has the upstream `api-consumption-report` workflow installed posts its own daily discussion summarizing its consumption. Today there is no single place to read those summaries together; cross-repo trends, hotspots, and noisy neighbors are invisible until the operator manually visits every repo. The new subcommand reads each repo's most-recent consumption report and prints a unified table grouped by repo, so the operator gets a fleet-wide view in one breath.

**Why this priority**: This is the entire reason the feature exists. Even with no temporal filtering and no alternative grouping axis, a single-command repo-by-repo rollup of recent consumption is the minimum viable product — it eliminates the "click through ten discussions" toil that motivated the request. Every later capability is a refinement of this view.

**Independent Test**: Configure a small fleet of two or three repos that already publish daily consumption reports, run the new subcommand with no flags, and confirm a tabular output appears with one row per repo summarizing its most-recent reported consumption. The command must succeed even when one repo has never published a report (that repo is surfaced as a visible diagnostic note rather than a silent omission).

**Acceptance Scenarios**:

1. **Given** a fleet of three repos all publishing daily consumption reports, **When** the operator runs the consumption subcommand in default mode, **Then** the output displays exactly three rows — one per repo — each showing the consumption volume drawn from that repo's most-recent valid report.
2. **Given** a fleet of three repos where one has never published a consumption report, **When** the operator runs the consumption subcommand, **Then** the output displays two rows (for the publishing repos) and a visible warning identifying the third repo as having no available reports, distinguishing this case from a zero-consumption row.
3. **Given** a fleet of three repos where one repo's most-recent report is still marked as in-progress (the report run has not yet finished), **When** the operator runs the consumption subcommand in default mode, **Then** the operator sees a warning that the latest report for that repo is not finalized — in default mode the in-progress report is skipped rather than partially included, with a fallback suggestion to widen the temporal window.

---

### User Story 2 - Cost-aware audit across a trailing window or since a fixed date (Priority: P2)

After landing the daily snapshot view, the same operator wants to ask retrospective questions: "what did the fleet consume over the last seven days?" or "since we onboarded the new team's three repos on 2026-04-01, how has the rollup changed?" Daily snapshots cannot answer these on their own — the operator needs to widen the temporal window. The subcommand offers two extra temporal modes: a trailing-window mode that selects all reports within the last N days, and a since-date mode that selects all reports from a fixed calendar date forward. Both modes sum across the selected window so that per-row totals reflect cumulative consumption rather than a single day.

**Why this priority**: Once the P1 single-snapshot view exists, this is the next natural question every operator asks, and the issue's design directly anticipates it. P2 because the snapshot view alone delivers real value, while the temporal-window modes are additive refinements that build on the same discovery and aggregation foundation.

**Independent Test**: With a fleet that has been publishing daily reports for at least a week, invoke the consumption subcommand with the trailing-window mode set to seven days, then again with a since-date mode set to a date five days ago. Confirm the rows sum the appropriate number of days of consumption in each mode, and that the two modes are mutually exclusive at the command-line level (the command refuses to run if more than one temporal-mode flag is provided).

**Acceptance Scenarios**:

1. **Given** a fleet where every repo has published seven daily reports, **When** the operator runs the consumption subcommand with the trailing-window mode set to seven days, **Then** each row's consumption totals reflect the sum of all seven reports for that repo (not the most-recent single day).
2. **Given** the same fleet, **When** the operator runs the consumption subcommand with a since-date mode set to three days ago, **Then** each row's consumption totals reflect the sum of the three calendar days from that date forward, and reports older than the cutoff date are excluded.
3. **Given** any fleet, **When** the operator passes more than one of the latest, trailing, and since flags in the same invocation, **Then** the command exits with a non-zero status and prints a clear error explaining the three modes are mutually exclusive.
4. **Given** a since-date mode set to a date beyond the run-log retention window for one of the fleet's repos (older than the platform's roughly 90-day retention), **When** the operator runs the consumption subcommand, **Then** the affected days surface as a visible diagnostic warning identifying the retention boundary as the cause — not a hard error, since the discovery layer still found the discussion but the underlying run artifacts have been garbage-collected.

---

### User Story 3 - Group-by-axis to expose cost concentration by profile or cost center (Priority: P2)

The repo-by-repo view from Story 1 surfaces individual hotspots, but operators frequently want to slice the fleet along the axes they actually budget against: profile (cost-tier identity), cost center (budget attribution), or workflow (engineering-driven hotspots). The existing fleet configuration already declares each repo's profiles, each profile's cost tier, and each repo's cost center. The new subcommand exposes a single grouping selector that pivots the same set of consumption reports into one of four views: by repo, by profile, by cost center, or by workflow. A repo that participates in two profiles contributes its full consumption to both profile groups; this additive double-counting is the explicit semantic, surfaced as an assumption.

**Why this priority**: Group-by-axis is the feature that makes the subcommand useful at the budget meeting, not just the engineering standup. P2 because Stories 1 and 2 must be present to make grouping meaningful — you cannot group what you cannot first roll up. The four axes share a common aggregation core, so they ship together.

**Independent Test**: With a fleet containing at least two profiles and at least two cost centers, run the consumption subcommand with the group-by flag set successively to each of the four axes (repo, profile, cost center, workflow). Confirm each invocation produces a distinct rollup view of the same underlying reports, and that an invalid group-by value is rejected with a clear error message naming the four valid options.

**Acceptance Scenarios**:

1. **Given** a fleet of four repos split across two cost-tier profiles named `standard` and `premium`, **When** the operator runs the consumption subcommand grouped by profile, **Then** the output displays exactly two rows — one labeled `standard` summing consumption across all repos in that profile, one labeled `premium` summing consumption across all repos in that profile.
2. **Given** a fleet where two repos declare `cost_center: "platform-eng"` and one declares `cost_center: "data-platform"`, **When** the operator runs the consumption subcommand grouped by cost center, **Then** the output displays exactly two rows keyed by cost-center value with summed totals, and repos lacking a cost-center declaration appear under a distinguishable bucket rather than being silently dropped.
3. **Given** a fleet of three repos collectively running ten distinct workflows, **When** the operator runs the consumption subcommand grouped by workflow, **Then** the output pivots away from per-repo rows and instead lists one row per distinct workflow name with summed consumption across every repo that runs it.
4. **Given** any fleet, **When** the operator passes a group-by value that is not one of the four supported axes, **Then** the command exits with a non-zero status and the error message enumerates the four valid values.
5. **Given** a repo declared to participate in two profiles where one profile is `standard` and the other is `premium`, **When** the operator runs the consumption subcommand grouped by profile, **Then** that repo's consumption appears in both the `standard` and `premium` rows — additive double-counting is the intended semantic.

---

### User Story 4 - Top-burner spotlight for hotspot triage (Priority: P3)

Once the operator can see the fleet's rollup grouped any way they want, the next question is "which specific workflows are driving the bulk of the consumption?" Each per-repo consumption report already carries a per-workflow breakdown; the new subcommand surfaces the highest-cost individual workflow rows as a footer block on the main output. Operators use this to prioritize which workflow to throttle, swap to a cheaper model, or remove from a profile.

**Why this priority**: A useful enhancement on top of the grouping work, but not the entry point — the P1/P2 rollups are already actionable for budget visibility and trend-watching. P3 because the top-burner block is a secondary visualization layered onto an already-complete primary view; it should ship but it is not gating.

**Independent Test**: With a fleet where one specific workflow demonstrably consumes more API budget than any other workflow across all repos, run the consumption subcommand in any grouping mode and confirm the highest-consuming workflows appear in a distinct footer block ordered from highest to lowest. The footer should highlight an honest ten entries when the fleet has at least ten distinct workflows, and gracefully present fewer rows when fewer distinct workflows exist.

**Acceptance Scenarios**:

1. **Given** a fleet of three repos collectively running fifteen distinct workflows of varying consumption levels, **When** the operator runs the consumption subcommand in any mode, **Then** a clearly labeled footer block appears listing the top ten workflows ranked from highest to lowest consumption.
2. **Given** a fleet running only three distinct workflows total, **When** the operator runs the consumption subcommand, **Then** the footer block displays all three workflows ranked, not an empty block padded to ten rows or a block that misrepresents three workflows as ten.
3. **Given** any fleet, **When** the consumption reports include the upstream Copilot-credit cost attribution field with non-empty values, **Then** the ranking still uses API-call volume as the primary ordering key (the cost field is informational while it remains an unstable upstream signal — see Assumptions), but the cost is surfaced alongside each row when present.

---

### Edge Cases

- A repo declared in the fleet configuration has no consumption discussions yet — the workflow was deployed but has not run, or the repo is freshly onboarded. The subcommand surfaces a diagnostic warning identifying the repo by name and explaining that no reports were discovered; this is distinct from a row reporting zero consumption.
- A repo's most-recent discovered report references a workflow run whose artifacts have already been garbage-collected by the platform's run-log retention window. The discovery succeeded (the discussion still exists) but the data layer cannot fetch the artifacts. This surfaces as a visible diagnostic warning per repo, never as a hard command failure.
- Every discovered report for a repo is either in-progress or beyond its declared expiry timestamp. In default snapshot mode, the row is omitted and a diagnostic suggests widening the temporal window. In trailing-window or since-date mode, in-progress reports are included but flagged with a diagnostic; expired reports are still excluded.
- The upstream consumption report payload includes a Copilot-credit cost field, but the value is the literal zero rather than absent. The system treats any non-positive cost value as equivalent to absent for the purpose of cost-aware ranking and output suppression, so a stray zero from the upstream engine does not pollute downstream cost displays.
- The grouping axis is cost-center and one or more repos have no cost-center value declared. Those repos do not silently disappear — their consumption rolls up under a distinguishable bucket that operators can recognize as the "unset" bucket, and the result table makes the bucket boundary visible.
- A repo participates in multiple profiles. When grouping by profile, the repo's consumption appears additively under every profile it participates in. The doc-visible Assumptions and the result presentation both make this additive-double-count behavior explicit so operators reading a profile-grouped total do not silently misinterpret a sum.
- The fleet configuration on disk has been updated between consumption-report runs, so a discussion published last week refers to a profile or cost-center value that no longer matches the repo's current configuration. The subcommand joins consumption reports against current fleet configuration — historical configuration drift is not a tracked dimension; this is documented as an assumption.
- The discovery query returns zero matching discussions across the entire fleet (e.g., the upstream workflow has not been deployed anywhere yet). The command exits successfully with an empty result table and a top-level diagnostic explaining that no reports were found, including a pointer to the prerequisite workflow installation.
- The discovered discussion body's HTML-comment markers fail to match the expected parsing pattern — usually because the upstream report format has changed. The per-report parse failure surfaces as a diagnostic warning for that report rather than aborting the rollup, so a single drifted upstream format does not block the whole fleet view.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST expose a new read-only subcommand named `consumption` that aggregates the fleet's existing per-repo consumption reports into a single output view.
- **FR-002**: The subcommand MUST NOT mutate any local or remote state — no pushes, no commits, no writes to any configuration file. It is a pure read command in the same category as the fleet listing command.
- **FR-003**: The subcommand MUST support three mutually exclusive temporal modes: a default snapshot mode that selects the single most-recent valid report per repo, a trailing-window mode that selects all valid reports within the last N days, and a since-date mode that selects all valid reports from a fixed calendar date forward.
- **FR-004**: The system MUST reject invocations that combine more than one temporal-mode flag, exiting with a non-zero status and a clear message naming the three modes.
- **FR-005**: The subcommand MUST support a grouping selector with exactly four valid axes — repo, profile, cost-center, and workflow — and MUST reject any other value with an error message enumerating the four valid options.
- **FR-006**: The discovery layer MUST locate consumption reports by querying each fleet repo's discussion category and filtering on a stable HTML-comment tracker marker embedded in the discussion body. The marker — not the human-readable prose of the report — is the parsing contract.
- **FR-007**: The data layer MUST fetch consumption attribution from the structured artifacts attached to the workflow run that each discovered report references, not from any rendered markdown table in the discussion body.
- **FR-008**: For each discovered discussion, the system MUST extract at minimum: the underlying workflow run identifier, the report date, the report's declared expiry timestamp, and an in-progress indicator. All four are sourced from stable markers in the discussion body, not from prose.
- **FR-009**: When the workflow run an artifacts fetch targets has been garbage-collected (typically beyond the platform's roughly 90-day run-log retention window), the system MUST surface the affected repo as a diagnostic warning rather than aborting the rollup with a hard error.
- **FR-010**: When the discovery layer finds zero reports for a repo, the system MUST surface that repo by name as a diagnostic warning rather than silently omitting it from the rollup.
- **FR-011**: When the only available reports for a repo in default snapshot mode are in-progress or expired, the system MUST omit that repo's row and surface a diagnostic warning suggesting the trailing-window mode as an alternative.
- **FR-012**: In trailing-window and since-date modes, in-progress reports MUST be included in the rollup, but each included in-progress report MUST surface a diagnostic warning so the operator knows the row contains partial data.
- **FR-013**: In trailing-window and since-date modes, expired reports MUST be excluded from the rollup, irrespective of in-progress status.
- **FR-014**: The grouping selector when set to profile MUST attribute a repo's consumption additively to every profile the repo participates in. The system MUST document this additive double-counting behavior in user-facing assistance.
- **FR-015**: The grouping selector when set to cost-center MUST attribute consumption to the repo's declared cost-center value (introduced by the 007-billing-metadata-fields work), MUST aggregate repos that share a cost-center value under one row, and MUST surface repos with no declared cost-center value under a distinguishable unset bucket rather than silently dropping them.
- **FR-016**: The grouping selector when set to workflow MUST pivot the result rows away from per-repo and per-profile aggregation and instead key each row on a distinct workflow name, summing consumption across every repo that runs that workflow.
- **FR-017**: The system MUST surface the highest-consuming workflows as a clearly labeled secondary footer block alongside the primary grouped output. The footer block MUST display at most ten rows, ordered by consumption volume from highest to lowest, and MUST gracefully present fewer rows when fewer distinct workflows exist in the dataset.
- **FR-018**: The system MUST parse the upstream Copilot-credit cost attribution field when present in the consumption-report artifact payload. The value MUST be carried through the rollup and surfaced in output when populated, and MUST be omitted from output when absent. Non-positive numeric values MUST be treated as equivalent to absent.
- **FR-019**: The system MUST support both the default human-readable text output mode and the existing JSON envelope output mode that other read-only fleet subcommands already honor. The JSON envelope's schema version MUST NOT bump — the new subcommand is additive on the envelope contract.
- **FR-020**: The system MUST emit diagnostic messages — warnings, hints, and other operator-facing notes — through the project's existing structured-warnings mechanism so that warnings appear both in human-readable output and in the JSON envelope's warnings collection.
- **FR-021**: The system MUST NOT introduce any new third-party runtime dependency to satisfy this feature (Constitution Principle I); it MUST rely on the project's existing standard-library tooling and shell-out patterns for any external calls.
- **FR-022**: The system MUST NOT cache consumption-report fetch results to disk between invocations. Each invocation re-discovers and re-fetches; this preserves the fleet's stateless operating model and avoids stale-cache failure modes.
- **FR-023**: The system MUST NOT enforce any spending cap, budget alarm, or hard limit on observed consumption. The subcommand is read-only; budget enforcement is delegated to the platform's first-party spending controls.
- **FR-024**: The system MUST NOT parse, infer, or surface long-trend totals beyond what the platform's run-log retention window can faithfully back with raw artifacts. The discussion's own prose summary MUST NOT be used as a source of truth for any reported number.
- **FR-025**: The diagnostic warning surfaced by the existing billing-quota hint mechanism (the cross-repo forward-reference target identified in the originating issue) MUST be authored so that operators encountering an upstream rate-limit or billing-quota error are pointed at the new subcommand for cross-repo attribution.
- **FR-026**: The system MUST consult the project's existing diagnostic-hint scanner so that recognizable error patterns in subprocess output (e.g., HTTP 404 on the discussion query for a repo without discussions enabled, HTTP 403 on the actions-runs query under tightened permissions) surface actionable remediations to the operator.
- **FR-027**: Operator-facing documentation — the project's primary agent-guidance file, the relevant skill describing the budget-review flow, and the subcommand's own usage text — MUST describe the additive double-counting semantic, the cost-field nil-until-populated status, and the no-retention-cache assumption.

### Key Entities *(include if feature involves data)*

- **Consumption Report**: A single per-repo per-day attribution record, conceptually one row of cross-fleet input. Carries at minimum the source repo identifier, the report date, the underlying workflow run identifier (so the data layer can locate the structured artifacts), and the report's published expiry timestamp. Optionally carries an in-progress indicator. The discussion is the discovery handle; the structured artifacts referenced by the discussion are the authoritative data source. The discussion's rendered markdown prose is not a parsing target.
- **Consumption Group**: One row in the primary grouped output. Keyed by a value drawn from the chosen grouping axis (repo, profile, cost-center, or workflow). Summed across the temporal window: API call volume, safe-output write volume, and — when the upstream cost field is populated — Copilot-credit cost. Records the count of reports that contributed to the sum so operators can spot sparse-coverage groups.
- **Workflow Breakdown Row**: One row in the per-workflow detail (used both as the workflow-grouping axis output and as the source for the top-burner footer block). Keyed on workflow name. Carries the workflow's run count over the temporal window, its API call volume, its average run duration, and optionally the upstream cost field.
- **Result Envelope**: The top-level output container. Records which configuration file the rollup was loaded from, which temporal mode was applied, the primary grouped rows, and the top-burner footer rows. Honors the existing JSON envelope shape used by other read-only fleet subcommands so that downstream tooling does not need new schema knowledge.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A fleet operator can obtain a fleet-wide consumption rollup in a single command invocation, eliminating the need to manually visit each repo's daily discussion page. For a fleet of ten repos, the operator-facing decision time drops from on the order of several minutes of clicking and mental arithmetic to a single screen of output.
- **SC-002**: For any fleet whose repos are actively publishing daily consumption reports, an operator running the default snapshot mode can read the highest-consuming repos directly from the first screen of output without needing to invoke a second command or sort manually.
- **SC-003**: Repos that fail to contribute to the rollup — because their reports are absent, expired, in-progress, or beyond the platform's retention window — are identified by name in the operator-visible diagnostics so the operator can distinguish "no data available" from "consumption is genuinely zero".
- **SC-004**: An operator who has a budget-attribution question phrased in terms of cost center or profile (e.g., "what did the platform-eng cost center consume last week?") can answer it by passing a single grouping flag, with no preprocessing of the underlying consumption reports.
- **SC-005**: The subcommand MUST add no new third-party runtime dependency (Constitution Principle I). The implementation relies exclusively on the project's existing standard-library and shell-out infrastructure.
- **SC-006**: The JSON envelope's schema version MUST remain unchanged after this feature ships. Downstream consumers of the envelope MUST NOT need to update their schema knowledge to read consumption output; the new result type is additive.
- **SC-007**: The full quality gate (formatting, vetting, linting, and the full test suite as exercised by the project's CI target) MUST pass cleanly with the new subcommand and all associated test fixtures included. No test in the new subcommand's package MUST require live network access — all external calls MUST be mockable via the project's existing shell-out wrapper, and all parser fixtures MUST be captured representative payloads, not live fetches.
- **SC-008**: An operator encountering an upstream rate-limit or billing-quota error in any existing fleet command (deploy, sync, upgrade) MUST see a diagnostic hint that points them at the new subcommand for cross-repo attribution, closing the loop between "I hit a budget wall" and "here is where I look to understand why".

## Assumptions

- The upstream `api-consumption-report` workflow is already deployed to the fleet repos that will be surveyed. This subcommand consumes the workflow's output; it does not deploy or configure the workflow. A separate roadmap issue tracks adding the workflow to a fleet profile so this assumption is operationally satisfied.
- The fleet configuration already carries cost-center declarations on each repo entry where the cost-center grouping axis is expected to be useful. The cost-center field and the related profile-tier field shipped previously as part of the 007-billing-metadata-fields work; this subcommand reads them as already-present optional fields.
- The upstream consumption-report payload's Copilot-credit cost field is not yet documented as stable. The subcommand parses it where present but does not treat it as load-bearing. A separate tracking issue captures the upstream stabilization work; once it lands, the cost field can be relied upon as a ranking key. Until then, ranking is by API-call volume.
- The platform's run-log retention window is approximately ninety days from the run's completion. Beyond this window, the underlying structured artifacts that back each consumption report are no longer fetchable, even when the discussion that referenced them still exists. The subcommand handles this gracefully via diagnostic warnings rather than hard errors, but it does not work around the retention limit (no caching, no historical archival).
- The fleet configuration's profile and cost-center mappings reflect current intent, not historical state. When the subcommand joins past consumption reports to today's fleet configuration, drift between when a report was published and the current configuration is not tracked. Operators who require strict historical-attribution semantics must pin the inputs upstream.
- A repo's participation in multiple profiles is rare but legitimate. The additive double-counting semantic for the profile grouping axis is the explicit chosen behavior — it is more useful than collapsing the consumption into a single arbitrary profile or refusing to aggregate. The user-facing assistance documents this so summed profile totals are not mistaken for fleet totals.
- Discovery and data fetching against the platform API are serial by default. If rate-limit budget or latency becomes a concern at fleet sizes the project does not yet target, parallelism is deferred to a follow-up. Caching is explicitly out of scope (stateless reads only).
- Operator-facing diagnostics are routed through the project's existing structured-warnings mechanism. New diagnostic codes for this feature are not required; existing categories (hint, warning, error) cover the surface area.
