# Feature Specification: Fleet Manifest — Deployed Version Tracking and Stale Init Refresh

**Feature Branch**: `011-fleet-manifest`
**Created**: 2026-06-11
**Status**: Draft
**Issue**: #114
**Input**: Leave a fleet-managed marker/manifest in each repo to track deployed gh-aw version + detect/refresh stale init artifacts

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Deploy Records Version Provenance (Priority: P1)

As a fleet operator running a deploy against a managed repo, I want the deploy to leave a
versioned record of what was deployed so I have a reliable audit trail of the fleet's
footprint in that repo.

**Why this priority**: Without this record, every subsequent operation (status checks,
sync, upgrades) has no baseline to compare against. This is the foundation all other
stories depend on. The concrete failure that motivated this issue — `rshade/finfocus`
silently running v0.68 init artifacts while the fleet advanced to v0.79 — stems directly
from this gap.

**Independent Test**: Run `deploy --apply` against a fresh repo. Verify that
`.github/aw/fleet-manifest.json` appears in the commit alongside the workflow files.
Read its contents: fleet identity, source version, CLI version, profiles, and timestamp
are all present and match the fleet's current configuration.

**Acceptance Scenarios**:

1. **Given** a managed repo with no existing manifest,
   **When** `deploy --apply` completes,
   **Then** `.github/aw/fleet-manifest.json` is committed in the same PR as the workflow changes, recording the fleet identifier, gh-aw source version, CLI version, active profiles for that repo, and the deployment timestamp.

2. **Given** a managed repo with an existing manifest at the current version,
   **When** `deploy --apply` runs again with no profile or version change,
   **Then** the manifest content is byte-identical to the previous commit — no new diff entry, no timestamp churn.

3. **Given** a managed repo with an existing manifest at an older version,
   **When** `deploy --apply` runs with the fleet pinned to a newer version,
   **Then** the manifest is updated to reflect the new version, profiles, and timestamp.

4. **Given** the deploy fails mid-pipeline after the manifest write but before the commit,
   **When** the operator re-invokes deploy with `--work-dir` pointing to the preserved clone,
   **Then** the manifest is not duplicated or corrupted; the resume path produces a clean commit.

---

### User Story 2 — Status Reports Version Drift Per Repo (Priority: P2)

As a fleet operator, I want `status` to show me which repos are behind the fleet's current
gh-aw pin, at current, or have never been fleet-deployed — so I can prioritize which repos
need attention before a runtime failure surfaces.

**Why this priority**: Detection is the prerequisite for remediation. Operators cannot act
on drift they cannot see. This story makes the invisible visible without requiring any
deployment action.

**Independent Test**: Configure a fleet with three repos — one never deployed (no
manifest), one with a manifest at an old version, one with a manifest at the current
version. Run `status`. Verify each repo shows the correct state label in both the text
output and `--output json`.

**Acceptance Scenarios**:

1. **Given** a repo with no `.github/aw/fleet-manifest.json`,
   **When** `status` is run,
   **Then** that repo is reported as `unmanaged` (never fleet-deployed).

2. **Given** a repo whose manifest records a gh-aw version older than the fleet's current pin,
   **When** `status` is run,
   **Then** that repo is reported as `behind` with the recorded version and the expected version both visible.

3. **Given** a repo whose manifest records the same gh-aw version as the fleet's current pin,
   **When** `status` is run,
   **Then** that repo is reported as `current`.

4. **Given** `status --output json` is run,
   **Then** each repo entry in the JSON envelope includes a `version_drift` field with the state (`behind` / `current` / `unmanaged`), the recorded version (or null if unmanaged), and the expected version.

---

### User Story 3 — Stale Init Artifacts Are Refreshed on Deploy/Sync (Priority: P2)

As a fleet operator running `deploy` or `sync` against a repo whose manifest version is
behind, I want the init artifacts (skills, agent files, `.github/mcp.json`) to be
automatically refreshed as part of that operation — so I do not need a separate manual
step to fix what was silently stale.

**Why this priority**: Detection alone is insufficient if the remediation path requires a
separate, error-prone manual intervention. The motivation from issue #114 is that the
stale-init problem persisted undetected across multiple fleet advances; when it is
detected (via the manifest version), the system should refresh automatically.

**Independent Test**: Take a repo initialized by an old gh-aw version (legacy init marker
present, no manifest, or manifest at old version). Run `deploy --apply`. Verify that
`gh aw init` is re-executed and the resulting PR contains updated init artifacts alongside
the workflow files.

**Acceptance Scenarios**:

1. **Given** a repo with a manifest version behind the fleet pin,
   **When** `deploy --apply` or `sync --apply` runs,
   **Then** the init process is re-executed to refresh init artifacts, and the resulting commit includes both updated workflows and updated init artifacts.

2. **Given** a repo with no manifest (legacy init marker only, unmanaged state),
   **When** `deploy --apply` runs,
   **Then** the init process is executed (not skipped), init artifacts are written, and the manifest is created recording the current version.

3. **Given** a repo with a manifest at the current version,
   **When** `deploy --apply` runs,
   **Then** init is NOT re-executed (no spurious init artifact churn).

4. **Given** the init refresh produces no file changes in the clone (already up-to-date by content),
   **When** the commit gate runs,
   **Then** no empty commit is created; the deploy proceeds without error.

---

### Edge Cases

- What happens when the manifest file exists but is malformed or unreadable?
  The system treats the repo as `unmanaged` for the purposes of version comparison and logs a warning; it does not fail the command.
- What happens when a repo participates in profiles with different gh-aw source pins?
  [NEEDS CLARIFICATION: see FR-003 — single version field vs. per-source map is an open design choice with scope implications]
- What happens when the CLI version used to compile differs from the source pin recorded in the fleet config?
  Both are recorded independently in the manifest; the drift comparison uses the source pin, not the CLI version.
- What happens when a deploy clone is missing the `.github/aw/` directory?
  The directory is created as part of the manifest write; the write should not fail on a missing parent directory.
- What happens when the fleet identifier changes (e.g., repo fork or rename)?
  The existing manifest is overwritten on the next deploy; no conflict is raised.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The fleet MUST write or update `.github/aw/fleet-manifest.json` in the
  target repo's clone as part of every `deploy --apply` run, committed in the same PR as
  workflow changes.

- **FR-002**: The manifest MUST record all of the following fields:
  - A `managed` flag indicating the repo is fleet-managed
  - The fleet's own identifier (repository slug)
  - The gh-aw source version (pin from the fleet config) used for this deploy
  - The gh-aw CLI version that compiled the locks and init artifacts
  - The list of profiles active for this repo at deploy time
  - The timestamp of the most recent content-changing deploy

- **FR-003**: The manifest MUST record the gh-aw version in a form that allows unambiguous
  comparison when a repo participates in multiple profiles.
  [NEEDS CLARIFICATION: should the manifest record a single resolved version (simpler, loses per-source detail) or a map of source → version (more precise, adds complexity)? Both are valid; the choice affects how "behind" is computed for multi-profile repos.]

- **FR-004**: Re-deploying the same fleet configuration MUST produce a manifest whose
  content is byte-identical to the previous deploy — no timestamp or other field change
  when nothing else has changed.

- **FR-005**: `status` MUST report a `version_drift` state for every repo in the fleet:
  `behind` (manifest version older than fleet pin), `current` (versions match), or
  `unmanaged` (no manifest present or manifest is unreadable).

- **FR-006**: `status --output json` MUST include `version_drift` per repo in the output
  envelope, with sub-fields: `state`, `recorded_version`, `expected_version`.

- **FR-007**: `deploy` and `sync` MUST re-execute the init process when the repo's manifest
  version is behind the fleet pin, replacing the prior behavior of unconditionally skipping
  init when a legacy init marker file was detected.

- **FR-008**: `deploy` and `sync` MUST NOT re-execute the init process when the manifest
  version matches the fleet pin, preventing spurious init artifact churn.

- **FR-009**: A malformed or missing manifest MUST be treated as `unmanaged` state; it
  MUST NOT cause `status`, `deploy`, or `sync` to fail with an error.

### Key Entities

- **Fleet Manifest**: A per-repo deployment record placed at `.github/aw/fleet-manifest.json`
  in the managed repo. Attributes: `managed` (bool), `fleet` (string — fleet repo slug),
  `gh_aw_version` (string — source pin or version map, see FR-003), `cli_version` (string),
  `profiles` (list of profile names), `deployed_at` (timestamp, updated only on content change).

- **Version Drift State**: A computed attribute for each repo in `status` output. Values:
  `behind` (manifest version < fleet pin), `current` (versions match), `unmanaged` (no
  manifest or unreadable). Used both in text output and JSON envelope.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After running `deploy --apply` on any managed repo, a fleet operator can
  identify the exact gh-aw version used for that deployment without inspecting git history
  or workflow file contents.

- **SC-002**: A fleet operator running `status` against a fleet of N repos receives a
  version-drift state for every repo in a single command invocation, with no manual
  per-repo inspection required.

- **SC-003**: Re-running `deploy --apply` on a repo already at the current version produces
  zero manifest-related lines in the resulting git diff (no churn).

- **SC-004**: A repo that was initialized by a legacy gh-aw version (v0.68–v0.78) and has
  advanced to v0.79+ in the fleet pin is fully refreshed — init artifacts and manifest
  written — within the same deploy that detects the stale state. No additional manual step
  is required.

- **SC-005**: `status --output json` output is parseable by downstream tooling; the
  `version_drift` field is present and populated for every repo entry, including repos with
  no manifest (state: `unmanaged`).

## Assumptions

- The fleet's `github/gh-aw` source pin is the authoritative version reference for drift
  comparison. CLI version is recorded separately for provenance but is not used for the
  drift check.
- The manifest file is written by the fleet tool only; operators are not expected to
  hand-edit it. No migration or conflict resolution is needed for operator edits.
- `deployed_at` is updated only when at least one other manifest field changes; a
  content-identical redeploy does not update the timestamp. This avoids the "timestamp
  churn" problem described in issue #114.
- Repos that have never been touched by the fleet produce an `unmanaged` state in `status`,
  not an error. The `unmanaged` state is informational, not actionable until an operator
  chooses to run deploy.
- The manifest write is atomic with the rest of the deploy commit; partial writes (manifest
  written but commit not yet made) are recoverable via the `--work-dir` resume path already
  in place for interrupted deploys.
- `sync --apply` follows the same manifest write + version-comparison logic as
  `deploy --apply`; the two commands share this behavior.
- When the init refresh produces no file-level changes (already up-to-date by content),
  the deploy does not fail or create an empty commit; it proceeds normally.
