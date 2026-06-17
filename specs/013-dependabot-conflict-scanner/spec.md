# Feature Specification: Dependabot Config Conflict Scanner (Advisory)

**Feature Branch**: `013-dependabot-conflict-scanner`
**Created**: 2026-06-16
**Status**: Draft
**Input**: GitHub issue #101 — "feat(security): detect Dependabot configs that conflict with gh-aw-managed pins"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Warn when Dependabot could bump the compiler-coupled action (Priority: P1)

As a fleet operator running `deploy` (or `sync`/`upgrade`) against a managed repo,
I want to be warned when that repo's Dependabot configuration has a GitHub Actions
update entry that does **not** ignore the `gh-aw-actions` action family, so that I
can add the ignore rule before Dependabot independently bumps the action — opening a
hash-breaking pull request that rewrites the generated `*.lock.yml` files and
desyncs them from the compiler version baked into lock-file metadata.

**Why this priority**: This is the core motivator and it is **live right now** —
`rshade/finfocus#1246` is a Dependabot GitHub Actions pull request bumping
`github/gh-aw-actions` 0.77.4 → 0.78.3 and rewriting nine-plus `*.lock.yml` files
plus `copilot-setup-steps.yml`. The `gh-aw-actions` version is coupled to the
`gh aw` compiler version; the fleet manages it atomically via `gh aw upgrade`. An
out-of-band Dependabot bump silently breaks hash validation across the repo's
agentic workflows. Catching this at deploy time — before Dependabot ever opens its
first PR — is where the value lands.

**Independent Test**: Point the scanner at a clone whose Dependabot config contains
a `github-actions` ecosystem entry but no ignore rule covering the gh-aw action
family; confirm exactly one advisory finding is produced, that it names the gap, and
that it quotes a copy-pasteable ignore block the operator can drop into their config.
Delivers value on its own even if no other check is ever added.

**Acceptance Scenarios**:

1. **Given** a managed repo whose Dependabot config has a `github-actions` update
   entry with no `ignore` rule covering the gh-aw action family, **When** the
   operator runs a deploy, **Then** the run surfaces one advisory finding describing
   the missing rule and including the exact `ignore:` block to add.
2. **Given** a managed repo whose Dependabot config already ignores the gh-aw action
   family on its `github-actions` entry, **When** the operator runs a deploy,
   **Then** no Dependabot-related finding is produced.
3. **Given** the finding above, **When** the operator reads it on any output surface,
   **Then** the remediation text is complete enough to copy-paste without further
   lookup — and it explicitly states that Dependabot can only ignore by dependency
   *name* (no file-glob equivalent to the Renovate lock-file exclusion), so the
   lock files remain reachable if any action in them is independently named.

---

### User Story 2 - Stay silent and safe when there is nothing actionable (Priority: P2)

As a fleet operator, I want the scanner to make no noise and never block when a
managed repo has no Dependabot config, a Dependabot config with no `github-actions`
ecosystem entry (e.g. a Go-modules-only config like the fleet's own), an
already-correct config, or a config that cannot be parsed — so that adding this
check never turns a working deploy into a failed or noisy one.

**Why this priority**: Robustness and trust. An advisory scanner that false-alarms on
repos that don't manage GitHub Actions through Dependabot, or that aborts a deploy on
a malformed file, would be worse than not having the feature. This slice guarantees
the scanner is "quiet by default" and strictly non-blocking. It is secondary to the
core warning only because the warning is the value; this slice protects it.

**Independent Test**: Run the scanner against (a) a clone with no Dependabot config,
(b) a clone whose Dependabot config is Go-modules-only with no `github-actions`
entry, (c) a clone whose `github-actions` entry already ignores the gh-aw family, and
(d) a clone with deliberately malformed YAML; confirm (a)-(c) produce zero findings,
(d) produces at most one informational note, and none of the four blocks or fails the
operation.

**Acceptance Scenarios**:

1. **Given** a managed repo with no Dependabot config in any recognized location,
   **When** the operator runs a deploy, **Then** the scanner produces no finding and
   the deploy proceeds unchanged.
2. **Given** a managed repo whose Dependabot config has no `github-actions` update
   entry (e.g. it manages only Go modules), **When** the operator runs a deploy,
   **Then** the scanner produces no finding.
3. **Given** a managed repo whose Dependabot config cannot be parsed, **When** the
   operator runs a deploy, **Then** the scanner produces a single informational
   finding noting the config could not be read, and the deploy still proceeds.
4. **Given** any Dependabot finding at all, **When** it is produced, **Then** it never
   changes the success/failure outcome of the deploy/sync/upgrade operation.

---

### Edge Cases

- **Go-modules-only config (no `github-actions` ecosystem)**: a Dependabot config that
  manages only `gomod` (or any non-`github-actions` ecosystem) is not at risk and
  produces no finding. This is the shape of the fleet's own `.github/dependabot.yml`
  today.
- **Multiple `github-actions` entries**: a config with more than one `github-actions`
  update entry (distinct `directory` values) is evaluated per entry; each entry
  lacking the ignore rule yields its own finding identifying that entry's directory.
- **Partial / unrelated ignore present**: a `github-actions` entry whose `ignore`
  list covers other dependencies (e.g. `actions/checkout`) but not the gh-aw action
  family still yields a finding.
- **Wildcard ignore**: an `ignore` entry using a wildcard `dependency-name` (e.g.
  `github/gh-aw-actions*`) that subsumes the gh-aw action family is treated as
  "rule present." Note the family spans two distinct identifiers — the
  `github/gh-aw-actions` action repo and the separate `github/gh-aw/actions/setup-cli`
  path — and the scanner's intent matching keys on their shared `gh-aw` lineage so a
  reasonable wildcard counts as protection (favoring fewer false positives).
- **Ecosystem effectively disabled**: a `github-actions` entry that cannot open bump
  PRs because it sets its open-pull-request limit to zero makes the conflict
  impossible; the scanner treats the rule as satisfied and produces no finding.
- **Malformed YAML**: a config the parser cannot read yields at most one informational
  finding and never panics or blocks.
- **Config present but Dependabot not actually enabled** on the repo: out of the
  scanner's knowledge; it reports on file contents only.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST detect whether a managed repo's clone contains a Dependabot
  configuration by probing the two recognized locations (`.github/dependabot.yml` and
  `.github/dependabot.yaml`) and using the first one found.
- **FR-002**: System MUST parse the Dependabot configuration's standard update-entry
  structure, reading each entry's ecosystem identifier, target directory (or the newer
  `directories` form), open-pull-request limit, and ignore list; unrecognized fields
  MUST be ignored rather than treated as errors.
- **FR-003**: When the Dependabot config contains a `github-actions` ecosystem update
  entry whose ignore configuration does **not** cover the gh-aw action family, System
  MUST emit exactly one advisory finding **per such entry**, identifying the affected
  entry and including the exact `ignore:` block the operator should add.
- **FR-004**: Each conflict finding's remediation MUST explicitly state that Dependabot
  ignores only by dependency *name* and has **no file-glob equivalent** to the
  Renovate lock-file exclusion — so the protection guards dependency names, not the
  `*.lock.yml` files as files, and those files remain reachable by Dependabot if any
  action they reference is independently named.
- **FR-005**: When no Dependabot config is present in any recognized location, **or**
  when a present config contains no `github-actions` ecosystem update entry, System
  MUST emit no finding for this scanner.
- **FR-006**: When a Dependabot config is present but cannot be parsed, System MUST
  emit a single informational finding and MUST NOT panic, return an error, or block
  the operation.
- **FR-007**: Findings from this scanner MUST be advisory only — they MUST NOT block,
  fail, or otherwise change the success/failure outcome of a deploy, sync, or upgrade.
- **FR-008**: Findings MUST surface on every existing security-finding output surface:
  the operator's stderr stream, the structured JSON `warnings[]` envelope, and the
  pull request's security-findings section.
- **FR-009**: System MUST NOT modify the managed repo's Dependabot config or any other
  file in the clone; the scanner is strictly read-only.
- **FR-010**: Each conflict finding MUST include remediation text complete enough to
  resolve the gap by copy-paste, without requiring the operator to consult external
  documentation.
- **FR-011**: System MUST emit each Dependabot **conflict** finding at the `LOW`
  advisory severity tier — the same non-blocking level the sibling Renovate scanner
  uses for its config-gap recommendations. The malformed-config finding (FR-006)
  MUST be emitted at `INFO`, consistent with the existing convention for "could not
  read / parse" notices.
- **FR-012**: System MUST decide whether a `github-actions` entry is protected using an
  intent-based, equivalence-aware match: the entry is considered protected if its
  ignore configuration covers the gh-aw action family (matched by dependency name,
  whether expressed as the exact identifiers or a wildcard pattern that subsumes them),
  **or** if the entry cannot open bump PRs because its open-pull-request limit is set
  to zero. Findings fire only when the *intent to protect* is absent, not when an exact
  canonical block is absent, to avoid false positives on healthy configs that achieve
  the same protection with different syntax.
- **FR-013**: The Dependabot scanner MUST be registered alongside the existing security
  scanners and run in the same operations where security findings are already collected
  and surfaced (deploy, sync, and upgrade).
- **FR-014**: System MUST NOT introduce any new third-party dependency to deliver this
  scanner, MUST NOT change the JSON output envelope's schema version, and MUST register
  its diagnostic code additively within the existing diagnostics contract (the new
  findings are additive within the existing warnings contract).

### Key Entities *(include if feature involves data)*

- **Dependabot configuration**: the managed repo's `.github/dependabot.yml` /
  `.yaml` file, discovered by probing the two recognized locations. The unit of input
  the scanner reads; its body is a list of update entries.
- **GitHub Actions update entry**: one update entry whose ecosystem is
  `github-actions`. The unit the scanner evaluates — it carries a target directory and
  an optional ignore list. Each such entry is checked independently.
- **Conflict rule**: the single policy the fleet needs present on each
  `github-actions` entry — ignore updates to the gh-aw action family by dependency
  name. A missing rule on an entry maps to one finding. (Unlike the Renovate sibling,
  there is no second file-exclusion rule — Dependabot cannot ignore by file glob.)
- **Advisory finding**: a non-blocking record carrying the affected file/entry, a
  human-readable description of the gap, and copy-pasteable remediation that includes
  the name-only-protection caveat; flows through the existing finding pipeline to
  stderr, JSON warnings, and the PR body. Conflict findings carry `LOW` severity; the
  malformed-config note carries `INFO`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a managed repo whose Dependabot config has a `github-actions` entry
  with no ignore rule covering the gh-aw action family, the scanner produces exactly
  one advisory finding for that entry.
- **SC-002**: For a managed repo whose `github-actions` entry already ignores the gh-aw
  action family (by exact name or subsuming wildcard), the scanner produces zero
  findings.
- **SC-003**: For a managed repo whose Dependabot config has no `github-actions`
  ecosystem entry (e.g. a Go-modules-only config), the scanner produces zero findings.
- **SC-004**: For a managed repo with no Dependabot config, the scanner produces zero
  findings and the deploy/sync/upgrade outcome is identical to a run without this
  feature.
- **SC-005**: 100% of conflict findings include both a copy-pasteable `ignore:` block
  and the explicit caveat that Dependabot offers name-only protection with no
  file-glob lock-file equivalent.
- **SC-006**: No deploy, sync, or upgrade run is ever blocked or failed by this scanner
  across all tested inputs (correct ignore present, `github-actions` entry without
  ignore, Go-modules-only, absent, malformed).
- **SC-007**: A malformed Dependabot config never causes a crash or error exit; it
  yields at most one informational finding.
- **SC-008**: The change adds no new third-party dependency and leaves the JSON output
  envelope's schema version unchanged.

## Assumptions

- **Recognized locations only**: Dependabot reads its config exclusively from
  `.github/dependabot.yml` / `.yaml`; the scanner probes both, prefers the first match,
  and does not look elsewhere.
- **Standard current schema**: the scanner reads the standard current Dependabot
  config structure (a top-level list of update entries, each with an ecosystem,
  directory, and optional ignore list). A legacy/alternate-shaped file that exposes no
  `github-actions` update entry simply yields no finding.
- **Local file only**: the scanner inspects the discovered config file's contents only.
  It does not resolve organization-level Dependabot configuration or verify the repo's
  Dependabot activation state; a repo whose protection is inherited elsewhere may
  therefore receive an advisory finding.
- **Per-entry findings; single-entry is the common case**: a config with multiple
  `github-actions` entries is evaluated per entry, but the overwhelming common case
  (including the live `rshade/finfocus#1246`) is a single `github-actions` entry → one
  finding.
- **Name-only protection is the only mechanism Dependabot offers**: the scanner does
  not attempt to confirm the `*.lock.yml` files are safe; it surfaces the structural
  limitation in the finding text rather than trying to engineer a file-level guard
  Dependabot does not support.
- **Reuses existing parsing capability**: parsing the Dependabot YAML reuses YAML
  machinery the project already depends on; no new parsing dependency is required.
- **Fleet self-fix is parallel and optional**: the fleet's own `.github/dependabot.yml`
  is Go-modules-only today, so it is not at risk and is not part of this scanner's
  acceptance; adding parity protection there is a worthwhile optional cleanup.

## Out of Scope

- **Auto-patching** a managed repo's Dependabot config. Deferred as an explicit
  opt-in follow-up; it additionally needs a YAML comment-preserving writer the
  codebase lacks (the project's comment-preserving write machinery is JSON-only). This
  feature is detect-and-warn only.
- **A file-glob lock-file protection equivalent** to the Renovate sibling's Rule B —
  Dependabot does not support ignoring by file path, so no such guard can exist; the
  scanner educates the operator about this gap instead of trying to close it.
- **Resolving organization-level Dependabot configuration** to evaluate the effective
  config.
- **Verifying that Dependabot is actually enabled** on the managed repo; the scanner
  reports on config file contents, not on the repo's Dependabot activation state.
