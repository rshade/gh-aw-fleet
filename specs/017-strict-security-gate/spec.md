# Feature Specification: --strict security gate

**Feature Branch**: `017-strict-security-gate`
**Created**: 2026-06-22
**Status**: Draft
**Input**: GitHub issue #38 — "Add --strict flag: promote HIGH Layer 1
findings from advisory to blocking" (part of epic #36)

## User Scenarios & Testing *(mandatory)*

The Layer 1 security scanner already runs on every `deploy`, `sync`, and
`upgrade` and surfaces its findings on stderr, in the JSON envelope's
`warnings[]`, and in the PR body's `## Security Findings` section. Today every
finding is **advisory**: even a HIGH-severity finding (e.g. an embedded
credential or `permissions: write-all`) only makes the output louder — the
operation still opens a pull request that a human must remember to reject. This
feature adds an **opt-in gate** that converts HIGH findings from advisory to
blocking, so an operator (or a CI job) can treat a HIGH finding as a hard stop
without imposing that policy on the whole team by default.

### User Story 1 - Hard-gate a deploy on HIGH findings (Priority: P1)

A fleet operator runs `deploy`/`sync` (single repo) with `--strict`. If the
scanner reports any HIGH-severity Layer 1 finding for that repo, the operation
aborts before any commit/push/PR step and exits non-zero. No pull request is
created. Without `--strict`, the identical command continues and opens the PR
exactly as it does today.

**Why this priority**: This is the core value of the feature — turning the
scanner from a passive reporter into an enforceable gate. It is the minimum
viable slice: shipping just this delivers the "zero-tolerance on secret leaks /
dangerous workflow structure" outcome the issue is about.

**Independent Test**: Run `deploy <repo> --strict --apply` against a fixture
whose workflow contains a fake embedded secret (HIGH finding); assert a non-zero
exit and that no PR was created on the target. Re-run the same command without
`--strict` and assert the operation proceeds.

**Acceptance Scenarios**:

1. **Given** a repo whose scan yields at least one HIGH Layer 1 finding, **When**
   the operator runs `deploy --strict --apply`, **Then** the command exits
   non-zero, no commit/push/PR occurs, and the findings are still printed.
2. **Given** the same repo, **When** the operator runs `deploy --apply` without
   `--strict`, **Then** the command proceeds and opens the PR (current advisory
   behavior is preserved).
3. **Given** a repo whose scan yields no HIGH findings (only MEDIUM/LOW/INFO, or
   none), **When** the operator runs `deploy --strict --apply`, **Then** the
   command behaves identically to a non-strict run (PR opened, exit 0).

---

### User Story 2 - Gate without applying, for pre-merge CI (Priority: P2)

An operator (or CI pipeline) runs a dry-run (`upgrade --strict`, no `--apply`)
to catch HIGH findings before any change is staged. The gate still fires: the
command prints the findings and exits non-zero when HIGH findings exist, even
though no commit would have been made. This lets a CI job fail a pull-request
check purely on the advisory scan, without needing write access to push.

**Why this priority**: The gate is most useful in CI, where the common mode is
dry-run validation rather than `--apply`. Without this, `--strict` would only
protect the apply path and leave the cheaper, more frequent dry-run path
ungated.

**Independent Test**: Run `upgrade --strict` (dry-run) against a fleet/fixture
with a HIGH finding; assert non-zero exit and that the findings were printed,
with no branch or PR created.

**Acceptance Scenarios**:

1. **Given** a HIGH finding and a dry-run invocation, **When** the operator runs
   `upgrade --strict` (no `--apply`), **Then** the command exits non-zero after
   printing findings, and makes no commit/branch/PR.
2. **Given** a fleet-wide `upgrade --strict` across multiple repos where one repo
   has a HIGH finding, **When** the run reaches that repo, **Then** the run
   aborts there (consistent with the command's existing fail-fast error
   propagation) and exits non-zero.

---

### User Story 3 - Prompt-injection findings stay advisory (Priority: P3)

A repo's scan includes a Layer 3 / prompt-injection finding (rule ID prefixed
`promptinj:`), even at HIGH severity. Under `--strict`, that finding does
**not** block the operation — only Layer 1 HIGH findings block. Layer 3 findings
remain advisory in every mode.

**Why this priority**: This is a guardrail that protects operator trust in
`--strict`. Indirect prompt-injection detection is the documented blind spot of
every surveyed classifier (high false-positive rate in the wild). Gating on it
would produce frequent false aborts and erode confidence in the flag. This is a
forward-looking carve-out: no `promptinj:` rules exist in the scanner today, so
the behavior is "future Layer 3 rules are exempt from the gate by construction."

**Independent Test**: Construct a finding with rule ID `promptinj:*` at HIGH
severity and assert the gate does not block on it, while a non-`promptinj:` HIGH
finding in the same set does block.

**Acceptance Scenarios**:

1. **Given** a finding set containing only a HIGH `promptinj:`-prefixed finding,
   **When** `--strict` is in effect, **Then** the operation is NOT blocked.
2. **Given** a finding set containing a HIGH `promptinj:` finding AND a HIGH
   Layer 1 finding, **When** `--strict` is in effect, **Then** the operation IS
   blocked (the Layer 1 finding alone is sufficient).

---

### Edge Cases

- **No HIGH findings, only lower tiers**: any number of MEDIUM/LOW/INFO findings
  must never trigger the gate — the gate keys on the HIGH tier, not on count.
- **Strict with no findings at all**: the operation proceeds identically to a
  non-strict run; `--strict` is a no-op when the scan is clean.
- **Dry-run abort breadcrumb**: when the gate fires on a dry-run, the work-dir
  clone is still preserved with the findings breadcrumb so the operator can
  inspect it (matching the existing "clone dirs are breadcrumbs after failure"
  convention).
- **Naming collision with compile-strict**: the tool already has an automatic
  *compile-strict* policy (public repos are compiled with `gh aw compile
  --strict`). `--strict` here governs **only** the Layer 1 HIGH security gate; it
  must not change compile-strict behavior, and operator-facing help/docs must
  make the distinction clear so the two "strict" concepts are not conflated.
- **Findings rendered before abort**: the operator must see the findings on
  stderr before the gate aborts, so the non-zero exit is never an unexplained
  failure.
- **`--strict` on a non-mutating command**: `--strict` is meaningful only for
  `deploy`/`sync`/`upgrade`; it has no effect on read-only commands (`list`,
  `status`, `consumption`).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The `deploy`, `sync`, and `upgrade` commands MUST accept an opt-in
  `--strict` flag. Its default MUST be off, preserving today's advisory-only
  behavior for every existing invocation.
- **FR-002**: When `--strict` is set and at least one **HIGH-severity Layer 1**
  finding is present for the repo being processed, the operation MUST abort
  **before** any commit, push, or PR-creation step and MUST exit non-zero.
- **FR-003**: The gate MUST key on **severity tier, not finding count** — a
  single HIGH finding blocks; MEDIUM, LOW, and INFO findings MUST NOT block
  regardless of how many are present.
- **FR-004**: The gate MUST NOT block on Layer 3 / prompt-injection findings,
  identified by the `promptinj:` rule-ID prefix. Such findings remain advisory
  even at HIGH severity under `--strict`.
- **FR-005**: Without `--strict`, behavior MUST be unchanged from today: findings
  are printed/surfaced and the operation proceeds (advisory). `--strict` MUST be
  the only thing that converts a finding into a blocker.
- **FR-006**: The gate MUST apply in dry-run mode as well as `--apply` mode —
  `--strict` MUST exit non-zero when HIGH findings exist even though no commit
  would have been made.
- **FR-007**: When the gate aborts, the system MUST write a `findings.json` file
  (a JSON array of all findings from the run) into the work-dir clone, so the
  operator can inspect findings post-mortem.
- **FR-008**: The abort error message MUST be actionable: it MUST state the count
  of blocking HIGH findings and tell the operator how to unblock (re-run without
  `--strict` to proceed advisory-only, or fix the findings).
- **FR-009**: The findings MUST already be rendered to stderr at the point the
  gate aborts, so the operator sees what blocked the run.
- **FR-010**: For a fleet-wide `upgrade --strict` spanning multiple repos, the
  gate MUST integrate with the command's existing fail-fast error propagation —
  the first repo whose scan yields a blocking HIGH finding aborts the run and the
  command exits non-zero. This strict-mode fail-fast behavior also applies to
  JSON/NDJSON output mode; any already-emitted per-repo JSON records remain
  valid, but no additional repositories are processed after the strict failure.
- **FR-011**: `--strict` MUST govern only the Layer 1 HIGH security gate. It MUST
  NOT alter compile-strict behavior, the JSON output schema version, or any other
  gating.
- **FR-012**: `--strict` MUST be opt-in per invocation only; it MUST NOT be
  persisted to `fleet.json` / `fleet.local.json`, so the team-wide default
  remains advisory.
- **FR-013**: Enabling `--strict` MUST NOT change the set, content, ordering, or
  severity of findings produced by the scanner — it only changes whether a HIGH
  finding is treated as blocking.

### Key Entities *(include if feature involves data)*

- **Strict security gate decision**: the per-invocation evaluation that inspects
  the scan's findings and decides whether to block. Inputs: the findings list and
  whether `--strict` is active. Output: either "proceed" or "abort with a
  non-zero error and a `findings.json` breadcrumb."
- **Security options (per-invocation)**: a small grouping of security-related
  invocation flags passed into `deploy`/`sync`/`upgrade`, carrying at minimum the
  strict toggle, with room to grow (e.g. a future deep-scan toggle) without
  churning command signatures.
- **Finding / Severity (existing)**: each finding carries a namespaced rule ID
  and one of INFO/LOW/MEDIUM/HIGH severities. The gate consumes these; the
  `promptinj:` rule-ID prefix distinguishes Layer 3 findings that are exempt from
  the gate.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A `deploy --strict --apply` against a repo with at least one HIGH
  Layer 1 finding exits non-zero and creates **zero** pull requests; the same
  command without `--strict` creates the pull request.
- **SC-002**: For any finding set containing only MEDIUM/LOW/INFO findings — at
  any count — `--strict` never aborts the operation (0% block rate across all
  non-HIGH tiers).
- **SC-003**: A HIGH finding whose rule ID is prefixed `promptinj:` never aborts
  the operation under `--strict`, while a non-`promptinj:` HIGH finding in the
  same set always does.
- **SC-004**: After every strict abort, a `findings.json` file exists in the
  work-dir clone and contains every finding from that run.
- **SC-005**: Every strict-abort message states both the blocking HIGH-finding
  count and the unblock path, verifiable from the command's stderr output.
- **SC-006**: When no HIGH Layer 1 findings exist, a `--strict` run produces the
  same outcome as the equivalent non-strict run (same PR created or not, same
  exit status) — `--strict` is observably a no-op on clean scans.
- **SC-007**: A dry-run `upgrade --strict` (no `--apply`) over a fleet with a
  HIGH finding exits non-zero without creating any branch, commit, or PR.

## Assumptions

- **Layer 1 scanner is already present.** The scanner (`security.Run`) and the
  `Finding`/`Severity` types already exist and already attach findings to the
  deploy/sync/upgrade result structs. This feature adds only the gate; it does
  not build or modify the scanner. (The issue's "depends on the Layer 1 scanner
  issue" precondition is already satisfied in the codebase.)
- **Flag name is `--strict` as specified in the issue.** A naming collision
  exists with the tool's automatic compile-strict policy; this spec keeps
  `--strict` (the issue's explicit choice) and mitigates the collision via FR-011
  (strict governs only the security gate) plus help/doc disambiguation, rather
  than renaming. If the operator later prefers a disambiguated name, that is a
  deliberate change to make during planning — not an open question this spec
  leaves unresolved.
- **No `promptinj:` rules exist yet.** The Layer 3 carve-out (FR-004) is
  forward-looking; it is specified now so the gate is correct by construction
  when Layer 3 rules land in a later slice (the `--deep-scan` issue).
- **Multi-repo semantics mirror existing behavior.** `deploy`/`sync` are
  single-repo; fleet-wide `upgrade` already uses fail-fast error propagation,
  which the gate reuses (FR-010) rather than introducing collect-all-then-fail
  semantics.
- **No schema-version bump.** Adding the gate and the `findings.json` breadcrumb
  is additive and does not change the JSON output envelope's schema version or
  the on-disk `fleet.json` format.
- **The breadcrumb file is `findings.json` at the work-dir clone root**, a JSON
  array of the `Finding` shape, consistent with the existing convention that
  `/tmp/gh-aw-fleet-*` clone dirs are preserved for post-mortem after a failed
  `--apply`.

## Out of Scope

- A per-finding ignore/allow mechanism (e.g. a `.fleet-security-ignore` file).
  Possible follow-up if `--strict` proves noisy in practice.
- Promoting MEDIUM (or lower) findings to blocking — the severity-tier semantic
  is kept clean: HIGH blocks under strict, everything else stays advisory.
- The Layer 3 prompt-injection classifier itself — that is the separate
  `--deep-scan` issue. This feature only reserves the `promptinj:` carve-out.
- Renaming or restructuring the existing compile-strict policy.
