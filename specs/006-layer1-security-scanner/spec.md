# Feature Specification: Layer 1 Security Scanner

**Feature Branch**: `006-layer1-security-scanner`
**Created**: 2026-04-30
**Status**: Draft
**Input**: User description: "Layer 1 security scanner: secrets + compiled-YAML + fleet-structural rules (issue #37, part of epic #36)"

## Clarifications

### Session 2026-04-30

- Q: Should the credential scanner's matched literal appear in the finding's message text on stderr and in the PR body? → A: Redact the matched value in both surfaces — show only the rule ID, file:line, and a `<redacted>` placeholder; operators inspect the source file to confirm.
- Q: How should the `fleet.engine.env.non-allowlist` rule behave when a workflow's frontmatter omits the `engine:` key or specifies an engine the fleet doesn't recognize? → A: Emit a single INFO finding noting the engine could not be determined and the rule was skipped for that workflow (mirrors the FR-017 malformed-frontmatter and FR-007 missing-binary patterns).
- Q: What hosts are on the v1 `fleet.mcp.non-standard-server` allowlist? → A: GitHub hosts only (`github.com`, `githubusercontent.com`, `raw.githubusercontent.com`). npmjs.com and other ecosystems are deliberately excluded from v1 because npm has been an established typosquat / malware distribution vector for MCP-server packages, and allowlisting a known compromise channel collapses the rule's value. A `fleet.json`-level allowlist extension (so operators can opt in to additional trusted hosts per profile) is deferred to a future issue.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Catch embedded secrets in upstream markdown before commit (Priority: P1)

A fleet operator runs `gh-aw-fleet deploy <repo>` (with or without `--apply`). The tool fetches workflow markdown from upstream sources (`github/gh-aw`, `githubnext/agentics`, …) and prepares to commit it into the operator's target repo. Before that commit happens, the tool scans the fetched markdown for embedded credentials. If any high-confidence credential pattern is detected (cloud provider keys, generic API tokens, private keys, …), the operator sees a HIGH-severity finding on stderr identifying the file, line, rule, and remediation. The deploy still proceeds (advisory only) — but the operator now knows about the leak before they merge the PR.

**Why this priority**: Embedded credentials are the highest-impact failure mode this feature exists to catch. A leaked AWS key in upstream `agentics/main` commits to the operator's repo, then to git history, then becomes a credential rotation incident. This story alone delivers the feature's primary security value; everything else is incremental.

**Independent Test**: Run a `deploy` against a fixture repo using a profile whose source markdown contains an obvious fake credential (e.g. a string matching `AKIA[A-Z0-9]{16}`). Confirm a HIGH-severity finding is emitted to stderr identifying the file, line, rule ID, and remediation, AND that the deploy continues to completion (does not block).

**Acceptance Scenarios**:

1. **Given** a profile that pulls a workflow file containing an AWS-key-shaped string, **When** the operator runs `deploy` (dry-run), **Then** stderr contains a HIGH-severity finding naming the file, line number, rule identifier, and remediation, AND the dry-run output still prints normally.
2. **Given** a profile that pulls only clean workflows, **When** the operator runs `deploy` (dry-run), **Then** no security findings are emitted on stderr.
3. **Given** a profile with one workflow containing an embedded secret and ten clean workflows, **When** the operator runs `deploy` (dry-run), **Then** exactly one HIGH-severity finding is emitted, identifying only the offending workflow.

---

### User Story 2 - Catch fleet-structural anti-patterns in workflow frontmatter (Priority: P2)

A fleet operator runs `deploy`/`sync`/`upgrade`. The tool inspects the YAML frontmatter of each fetched workflow against a fixed set of fleet-specific rules: write/admin permissions on scheduled triggers, `safe-outputs.create-pull-request.draft: false`, `create-pull-request` blocks missing a `protected-files` key, `engine.env.*` keys referencing secrets outside the engine-specific allowlist, `repo-memory.branch-name` set to `main` or `master`, and MCP server entries pointing at non-standard hosts. Each match becomes a finding with severity HIGH (security-relevant) or MEDIUM (defensive-posture-relevant).

**Why this priority**: These patterns are the unique value the fleet adds over a generic GitHub Actions linter. A scheduled workflow with `permissions: contents: write` is the operational shape of a supply-chain compromise; relaxed `safe-outputs` is a defense-in-depth regression. Catching these requires fleet-specific knowledge no off-the-shelf scanner has.

**Independent Test**: Run `deploy` against a fixture with one workflow per anti-pattern. Confirm each rule fires exactly once with the expected severity, file path, and rule ID. Confirm a clean workflow produces zero findings.

**Acceptance Scenarios**:

1. **Given** a workflow declaring `on: schedule` and `permissions: contents: write`, **When** the operator runs `deploy`, **Then** a HIGH finding with rule `fleet.permissions.write-on-schedule` is emitted naming the file and line of the `permissions:` block.
2. **Given** a workflow declaring `safe-outputs.create-pull-request.draft: false`, **When** the operator runs `deploy`, **Then** a MEDIUM finding with rule `fleet.safe-outputs.draft-false` is emitted.
3. **Given** a workflow declaring `engine.env.MY_SECRET: ${{ secrets.MY_SECRET }}` for a secret not in the engine-specific allowlist, **When** the operator runs `deploy`, **Then** a HIGH finding with rule `fleet.engine.env.non-allowlist` is emitted.
4. **Given** a workflow declaring `repo-memory.branch-name: main`, **When** the operator runs `deploy`, **Then** a HIGH finding with rule `fleet.repo-memory.main-branch` is emitted.
5. **Given** a clean upstream agentics workflow (no anti-patterns), **When** the operator runs `deploy`, **Then** zero structural findings are emitted.

---

### User Story 3 - Surface findings in the opened PR body so reviewers see them at review time (Priority: P3)

A fleet operator runs `deploy --apply`, which opens a PR against the target repo. When findings exist, the PR body includes a `## Security Findings` section summarizing them: a severity-tallied summary line (e.g. `2 HIGH, 1 MEDIUM`), then per-finding details (rule, file, line, message, remediation). When no findings exist, the section is omitted entirely. Reviewers see the same information the operator saw on stderr, without having to dig through tooling output.

**Why this priority**: The stderr surface only helps the operator. Most fleet PRs are reviewed asynchronously by other engineers (or by the operator's future self). Embedding findings in the PR body collapses two channels into one and ensures the security signal cannot be lost between "operator ran the command" and "reviewer approves the PR."

**Independent Test**: Run `deploy --apply` against a sandbox repo with a workflow that triggers at least one finding. Inspect the opened PR. Confirm the body contains a `## Security Findings` section with a severity summary and per-finding details. Run again with a clean workflow set. Confirm the PR body contains no `## Security Findings` section.

**Acceptance Scenarios**:

1. **Given** a workflow set producing at least one finding, **When** the operator runs `deploy --apply` and a PR is opened, **Then** the PR body contains a `## Security Findings` section listing each finding with its severity, rule, file:line, and remediation.
2. **Given** a workflow set producing zero findings, **When** the operator runs `deploy --apply` and a PR is opened, **Then** the PR body contains no `## Security Findings` section (the section is fully omitted, not rendered with a "no findings" placeholder).
3. **Given** a workflow set producing findings of mixed severities, **When** the PR is opened, **Then** the section's summary line tallies findings by severity (e.g. `2 HIGH, 1 MEDIUM, 1 INFO`).

---

### User Story 4 - Catch compiled-workflow lint issues with graceful degradation (Priority: P4)

A fleet operator runs `deploy`/`sync`/`upgrade`. After the workflow markdown is compiled to a GitHub Actions YAML lock file (the artifact GitHub Actions actually executes), the tool runs an external workflow linter against it. Linter errors map to HIGH findings, warnings map to MEDIUM. If the external linter is not installed on the operator's machine, the run continues without failure — instead, a single INFO-severity finding is emitted explaining the scanner was skipped.

**Why this priority**: The compiled lock file is the ground truth GitHub Actions executes — markdown can compile to surprisingly different YAML, and a linter run on the lock file catches issues invisible at the markdown level. Lower priority than P1–P3 because (a) it duplicates protection that exists in many target repos' own CI, and (b) it depends on an external binary that isn't a hard fleet dependency.

**Independent Test**: With the external linter installed, run `deploy` against a fixture whose compiled lock file has an obvious lint issue (e.g. invalid action reference syntax) and confirm a HIGH/MEDIUM finding is emitted with the correct file and line. Then uninstall the linter (or alter PATH so it cannot be found) and re-run; confirm a single INFO finding explains the skip and the deploy continues.

**Acceptance Scenarios**:

1. **Given** the external workflow linter is installed and a workflow whose compiled lock file has a lint error, **When** the operator runs `deploy`, **Then** a HIGH finding is emitted naming the lock file, line, rule, and message.
2. **Given** the external workflow linter is NOT installed (binary missing from PATH), **When** the operator runs `deploy`, **Then** exactly one INFO-severity finding is emitted explaining the linter is unavailable, AND the run completes successfully without aborting.
3. **Given** the external linter is installed and the compiled lock file has no issues, **When** the operator runs `deploy`, **Then** no findings from the lock-file linter are emitted (other scanners' findings unaffected).

---

### Edge Cases

- **Workflow with malformed frontmatter**: structural rules cannot evaluate. System emits a single INFO finding for that file noting the parse failure, then skips structural rules for that file (other scanners still run).
- **Workflow with missing or unknown `engine:`**: the `fleet.engine.env.non-allowlist` rule cannot determine which allowlist applies. System emits a single INFO finding for that workflow noting the engine could not be determined and skips that rule only — other structural rules and other scanners continue to evaluate.
- **Empty workflow file**: zero findings, no error.
- **Workflow that triggers multiple rules at the same line** (e.g. an `engine.env` secret that is also a recognized credential pattern): each scanner's finding stands; the system does NOT deduplicate across scanners in v1 — operators see one finding per matching rule.
- **Profile with a large workflow set** (e.g. 30+ workflows): scanner overhead is bounded by one-time initialization plus near-linear per-workflow cost; total deploy runtime grows by less than a small constant overhead.
- **Findings are emitted in a stable, sorted order** — first by severity (HIGH → MEDIUM → LOW → INFO), then by file path, then by line number — so output is reproducible across runs.
- **Run with `--apply` produces an unsigned commit** (e.g. gpg failure): findings still emit on stderr; PR body assembly is skipped (no PR opened) — findings on stderr are sufficient for the operator to inspect before manually finishing the commit.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST scan upstream workflow markdown for embedded credentials before committing the workflow into a target repository.
- **FR-002**: System MUST scan compiled GitHub Actions YAML (the artifact GitHub Actions executes) for syntax and configuration issues before committing.
- **FR-003**: System MUST evaluate workflow frontmatter against a fixed set of fleet-specific structural rules (broad permissions on scheduled triggers, relaxed `safe-outputs`, missing `protected-files`, non-allowlist `engine.env` secret references, sensitive `repo-memory` branch names, non-standard MCP server hosts).
- **FR-004**: System MUST emit findings to stderr on every `deploy`, `sync`, and `upgrade` run regardless of whether `--apply` is used (advisory output is unconditional).
- **FR-005**: When `--apply` opens a PR, system MUST embed findings in the PR body as a `## Security Findings` section so reviewers see them at review time. The section MUST be omitted entirely when no findings exist.
- **FR-006**: System MUST NOT block `deploy`, `sync`, or `upgrade` based on findings in this iteration. All findings are advisory; blocking behavior (e.g. `--strict`) is explicitly future work and out of scope.
- **FR-007**: When an optional external scanner dependency (e.g. the workflow linter binary) is missing from PATH, system MUST continue running other scanners and emit a single INFO-severity finding explaining the skip.
- **FR-008**: Each finding MUST include severity (Info / Low / Medium / High), rule identifier, file path with line number, message, and remediation guidance.
- **FR-008a**: Findings produced by the embedded-credential scanner MUST NOT include the matched literal value in the finding's message, neither on stderr nor in the PR body. The message MUST identify the matched rule (e.g. `AWS Access Key`) and a `<redacted>` placeholder; operators inspect the source file at `file:line` to confirm the match. This applies symmetrically to both surfaces (no asymmetric leak channel).
- **FR-009**: System MUST classify findings into four severities (Info, Low, Medium, High) where High is the most operationally significant and Info is purely informational.
- **FR-010**: System MUST tally findings by severity in the PR body section's summary line (e.g. `2 HIGH, 1 MEDIUM`) when at least one finding exists.
- **FR-011**: System MUST emit findings in a stable, sorted order (severity desc → file path asc → line number asc) so output is reproducible across runs and diff-friendly.
- **FR-012**: System MUST classify any output from the embedded-credential scanner as HIGH severity (matches are high-confidence by construction).
- **FR-013**: System MUST classify the structural-rule outputs by the rule's published severity (HIGH for security-impact rules, MEDIUM for defensive-posture rules — see User Story 2 acceptance scenarios for the canonical mapping).
- **FR-014**: System MUST classify the compiled-YAML linter's outputs as HIGH for linter errors and MEDIUM for linter warnings.
- **FR-015**: System MUST NOT emit findings of severity LOW in v1; the LOW severity exists in the model but is reserved for future detectors (no v1 rule produces it).
- **FR-016**: Scanner overhead MUST NOT cause the run to fail or time out; if any individual scanner errors internally, system MUST log the error and continue with remaining scanners.
- **FR-017**: When workflow frontmatter cannot be parsed, system MUST emit a single INFO finding noting the parse failure for that file and skip structural-rule evaluation for that file only.
- **FR-018**: When evaluating the `fleet.engine.env.non-allowlist` rule, if a workflow's frontmatter omits the `engine:` key or specifies an engine the fleet does not recognize, system MUST emit a single INFO finding for that workflow noting the engine could not be determined and skip the rule for that workflow only. Other rules and other scanners MUST continue to evaluate normally.
- **FR-019**: The v1 `fleet.mcp.non-standard-server` allowlist MUST contain exactly three hosts: `github.com`, `githubusercontent.com`, `raw.githubusercontent.com`. Any MCP server entry referencing a host outside this list MUST produce a HIGH finding. The allowlist is intentionally conservative because npm and similar package ecosystems have been used as typosquat / malware distribution channels for MCP-server packages. Operator-extensible allowlists are out of scope for v1 and tracked as future work under the parent epic.

### Key Entities *(include if feature involves data)*

- **Finding**: A single security observation. Attributes: rule identifier (namespaced string, e.g. `gitleaks:aws-access-key` or `fleet.permissions.write-on-schedule`), severity (Info/Low/Medium/High), file path, line number, human-readable message, remediation guidance.
- **Scanner**: A detector that consumes workflow content (source markdown or compiled YAML) and emits zero or more findings. Three scanners exist in v1: an embedded-credential scanner, a compiled-YAML linter, and a fleet-structural rule engine.
- **Severity**: An ordinal classification (Info < Low < Medium < High) used for sorting findings and tallying the PR body summary.
- **Rule**: A named detection pattern owned by exactly one scanner (e.g. `fleet.permissions.write-on-schedule` belongs to the structural scanner; `gitleaks:aws-access-key` belongs to the embedded-credential scanner). Each rule has a fixed severity in v1.
- **Workflow Artifact**: The pair of (source markdown file, compiled YAML lock file) the fleet operates on. The credential scanner reads the markdown; the YAML linter reads the lock file; the structural scanner parses the markdown's frontmatter.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator running `deploy` against a workflow set that contains at least one of the four canonical anti-patterns (embedded credential, write permissions on a scheduled trigger, relaxed PR draft, non-allowlist `engine.env` secret reference) discovers the issue on stderr before committing — measured by 100% detection rate on the fixture suite covering each canonical anti-pattern.
- **SC-002**: A clean upstream workflow (e.g. an unmodified agentics workflow such as `ci-doctor.md`) produces zero findings — measured by a regression fixture with at least one real agentics workflow asserting an empty finding list. False-positive rate on the curated clean-fixture set is zero.
- **SC-003**: Scanner overhead does not extend a typical `deploy` run by more than a small bounded amount — measured by a benchmark on a 10-workflow profile showing scanner-attributable wall-clock time below a stated budget (target: under 2 seconds total scanner overhead for 10 workflows on commodity hardware).
- **SC-004**: Findings emitted on stderr correspond exactly to findings rendered in the opened PR body when `--apply` is used (same set, same rule IDs, same file:line references) — measured by an integration test that diffs the two surfaces and asserts equivalence.
- **SC-005**: When the optional external workflow linter is absent from PATH, the run completes successfully and reports the skip explicitly — measured by an integration test that strips the binary from PATH and asserts (a) the run exits zero and (b) exactly one INFO finding explains the skip.
- **SC-006**: Findings are reproducible across runs — measured by running the scanner twice against the same workflow set and asserting the finding list is byte-identical (after sorting normalization).
- **SC-007**: An operator reviewing a fleet PR can determine the security posture of the change without leaving the PR view — measured by a manual UX check: every finding visible on stderr also appears in the PR body with sufficient context (rule, file:line, remediation) to act on.

## Assumptions

- Fleet operators run `deploy`/`sync`/`upgrade` against repositories where they have permission to open PRs; the PR-body integration assumes a PR-creation pathway exists in the underlying flow.
- By the time scanners run, the fleet's working clone directory contains both source markdown (`.md`) and compiled artifacts (`.lock.yml` produced by the upstream compile step).
- Stderr is an acceptable channel for advisory output — operators routinely read stderr during fleet runs (precedent: existing diagnostic hints in `internal/fleet/diagnostics.go`).
- The set of fleet-structural rules at v1 is fixed and canonical (write-on-schedule, draft-false, missing-protected-files, non-allowlist engine.env, repo-memory main branch, non-standard MCP host); user-configurable rule packs are out of scope.
- The v1 MCP-host allowlist is intentionally conservative — only GitHub-served hosts. This will produce HIGH findings on common workflows that fetch MCP servers from npm or other ecosystems; that noise is accepted as the v1 cost of treating known supply-chain compromise channels as untrusted. Operators who need to opt in additional hosts (e.g. a vetted private registry) will rely on a configurable allowlist extension delivered in a future issue. SC-002's "zero false positives on the curated clean fixture set" applies only to fixtures whose MCP references are GitHub-hosted; npm-using workflows are tracked separately and excluded from the clean fixture set in v1.
- The default rule set of the embedded-credential scanner is sufficient for v1; custom rules tailored to agentic-workflow idioms are deferred.
- The `engine.env` allowlist is engine-dependent and tracks an upstream specification (`github/gh-aw` ADR-26919). The v1 implementation pins behavior to that specification with a regression test; specification drift is a known maintenance cost addressed by the test layer.
- Findings are not persisted across runs — each run produces a fresh finding list. There is no scanner cache, baseline file, or finding-suppression mechanism in v1.
- The PR-body section uses a stable header (`## Security Findings`) so consumers (humans or downstream tooling) can locate it reliably.
- Future blocking behavior (`--strict`), Layer 3 semantic detection (`--deep-scan`), interactive y/N prompts, and fleet-specific embedded-credential rule packs are explicitly future work tracked under the parent epic and are not part of this feature.
- Scanner failures are non-fatal: an internal error in one scanner does not abort the run nor prevent other scanners from emitting findings — the operator receives at least the advisory output the still-functional scanners can produce.
