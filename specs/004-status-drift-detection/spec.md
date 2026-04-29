# Feature Specification: `status` Subcommand for Drift Detection

**Feature Branch**: `004-status-drift-detection`
**Created**: 2026-04-28
**Status**: Draft
**Input**: GitHub issue #10 — `feat(cmd): implement status [repo] subcommand for drift detection`

## Clarifications

### Session 2026-04-28

- Q: What concurrency model should the multi-repo fetch use? → A: Bounded parallel across repos with a worker pool of 4–8; serial workflow fetches within each repo.
- Q: Should `status` accept a `--ref <branch>` flag to check non-default branches (e.g., a PR branch before merge)? → A: No — default branch only in this release. Follow-up issue **#61** tracks demand for an opt-in `--ref` flag; if users ask for it, it can be added without breaking changes.
- Q: How should drift detection handle non-tag refs (commit SHAs, branch names) in the workflow's `source:` frontmatter? → A: Strict string comparison only. `actual_ref` holds the literal frontmatter string; no SHA resolution. Follow-up issue **#62** tracks whether string-comparison false positives (e.g., a SHA pin that points to the same content as the desired tag) become a real operator pain point — if so, opt-in SHA resolution can be added later.
- Q: How should the JSON schema represent workflows with missing or malformed `source:` frontmatter? → A: Add a fourth top-level category `unpinned[]` (string array of workflow names) to `RepoStatus`, alongside `missing`/`extra`/`drifted`. Keeps schema clean and gives downstream consumers a single `jq` predicate for the "needs investigation" set.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Fleet-wide drift summary without cloning (Priority: P1)

An operator managing 10+ repos runs `gh-aw-fleet status` and gets a per-repo summary that classifies each repo as either **aligned** (no drift), **drifted** (some workflow is missing, extra, or pinned to a different ref than fleet.json declares), or **errored** (the repo is inaccessible — 404, 403, network failure). The whole run completes in seconds, not minutes, because no repos are cloned. The exit code is `0` when every repo is aligned and `1` otherwise — making the command suitable as a CI gate before `deploy --apply`.

**Why this priority**: This is the motivating use case from the issue. Today, the only way to answer "anything to do across the fleet?" is `deploy --dry-run` (which clones every repo) or `upgrade --audit` (which also clones). For a 10-repo fleet, that's bandwidth-heavy and slow enough that operators skip the check and discover drift only when something fails downstream. A fast, read-only fleet-wide health check unblocks every subsequent workflow (deploy chains, dashboards, scheduled CI) and is independently valuable on its own — even without a single-repo drill-down or JSON output.

**Independent Test**: Run `gh-aw-fleet status` against a fleet whose state is partially drifted (one repo missing a declared workflow, one repo pinned to an old ref, one repo aligned). Assert the output identifies all three repos correctly, exits `1`, and completes without invoking `git clone` (verifiable via process tracing or by confirming `/tmp/gh-aw-fleet-*` directories are not created).

**Acceptance Scenarios**:

1. **Given** a fleet of three repos where two are aligned and one has a workflow pinned to an older ref than fleet.json declares, **When** the operator runs `gh-aw-fleet status`, **Then** the output reports two repos as `aligned` and one as `drifted` with the drifted workflow's old → new ref shown, and the process exits with code `1`.
2. **Given** a fleet of repos that are all fully aligned with fleet.json, **When** the operator runs `gh-aw-fleet status`, **Then** every repo is reported as `aligned`, no drift categories are populated, and the process exits with code `0`.
3. **Given** a repo declared in fleet.local.json that does not exist on GitHub or is inaccessible (404/403), **When** the operator runs `gh-aw-fleet status`, **Then** the unreachable repo is reported as `errored` with a clear per-repo message, the remaining repos still get a drift report, and the process exits with code `1`.
4. **Given** any `status` invocation, **When** the command runs, **Then** no `git clone`, `git fetch`, or local working directory is created — drift is computed entirely from `gh api` reads of the repo contents.

---

### User Story 2 - Targeted single-repo drift check (Priority: P2)

After the fleet-wide summary flags one repo as drifted, the operator drills in with `gh-aw-fleet status owner/repo` to get the same drift report scoped to one repo only — same three categories (Missing, Extra, Drifted), same exit code semantics, same no-clone guarantee. The single-repo invocation is the natural follow-up after P1 narrows the focus, and is also useful from CI when the operator already knows which repo to check.

**Why this priority**: Single-repo mode is a strict subset of the multi-repo case in implementation terms (filter the iteration to one repo), and is genuinely useful as a focused check, but the fleet-wide command is the primary value. Anyone who needs single-repo can grep the fleet-wide output as a workaround in the meantime; the inverse is not true.

**Independent Test**: Run `gh-aw-fleet status rshade/gh-aw-fleet` against a known drifted repo. Assert that exactly one repo's drift report is emitted, the drift categories are populated identically to the fleet-wide run for that repo, and unrelated repos in fleet.local.json are not queried (verifiable by `gh api` call count).

**Acceptance Scenarios**:

1. **Given** a repo with one missing workflow and one drifted (different-ref) workflow, **When** the operator runs `gh-aw-fleet status owner/repo`, **Then** the drift report identifies one workflow under `missing` and one under `drifted` (with old → new refs), and the process exits with code `1`.
2. **Given** an `owner/repo` argument that is not declared in fleet.local.json, **When** the operator runs `gh-aw-fleet status owner/repo`, **Then** the command errors with a clear message saying the repo is not in the fleet (exit non-zero), without any network calls to GitHub.
3. **Given** a single-repo run, **When** the command queries the repo, **Then** only that repo's contents are fetched (no fleet-wide iteration).

---

### User Story 3 - Machine-readable JSON output for CI and dashboards (Priority: P3)

A CI pipeline or dashboard ingests `gh-aw-fleet status -o json` and parses the structured output to drive visualizations (e.g., a Grafana panel of drift state) or chained automation (`gh-aw-fleet status -o json | jq -e '.result.repos | all(.drift_state == "aligned")' && gh-aw-fleet deploy --apply ...`). The JSON envelope follows the existing `--output json` contract (schema_version, command, repo, apply, result, warnings, hints — defined in spec 003) so downstream consumers reuse their existing envelope parsers.

**Why this priority**: JSON output is what makes the command CI-grade rather than terminal-only. But the human-readable output covers the interactive operator's needs on its own; JSON is additive value. Lower priority than getting drift detection right.

**Independent Test**: Run `gh-aw-fleet status -o json` against a known-drifted fleet. Assert the envelope passes `jq -e .schema_version`, that `result.repos[]` is an array with one entry per queried repo, each entry has stable `repo`, `drift_state`, `missing[]`, `extra[]`, `drifted[]`, `unpinned[]` keys, and that empty drift categories serialize as `[]` not `null`.

**Acceptance Scenarios**:

1. **Given** any `status` invocation with `-o json`, **When** the envelope is emitted, **Then** stdout contains exactly one JSON object parseable by `jq -e`, with `schema_version`, `command: "status"`, `apply: false`, `result.repos[]`, `warnings[]`, and `hints[]`. (Single envelope, not NDJSON — see Assumptions.)
2. **Given** a repo that returns 404 from `gh api` during status, **When** the envelope is emitted, **Then** the repo's entry in `result.repos[]` has `drift_state: "errored"` with the error reason in `error_message`, AND a corresponding entry appears in `hints[]` as a structured `Diagnostic` with `code: "repo_inaccessible"` and `fields.repo` set to the failing repo (constructed directly at the call site per FR-010 — substring matching cannot disambiguate a missing-workflow 404 from a missing-repo 404).
3. **Given** human-readable mode (`-o text` or omitted), **When** the command runs, **Then** the JSON path is not invoked and the human-readable table is byte-identical regardless of whether `-o json` ever existed in the codebase.
4. **Given** every empty drift category (no missing, no extra, no drifted) in any repo's report, **When** serialized to JSON, **Then** each empty category is the empty array `[]`, never `null`.

---

### Edge Cases

- **Repo declared in fleet but with no workflows yet deployed**: Every declared workflow shows up under `missing[]`; the repo is `drifted`; exit code `1`.
- **Repo with workflows that gh-aw didn't manage** (hand-written `.yml` with no paired `.md` source): Ignored entirely. Status only reports on workflows whose `.github/workflows/*.md` source file is present and parses as a gh-aw managed workflow (i.e., has a `source:` frontmatter field). Plain GitHub Actions workflows are out of scope.
- **Workflow whose `.md` exists but lacks a `source:` frontmatter field**: Reported under the dedicated category `unpinned[]`, contributing to `drift_state: "drifted"`. The operator can fix the unmanaged file via `deploy --apply --force`. Status MUST NOT silently treat such a workflow as aligned.
- **`source:` frontmatter present but malformed** (e.g., not a string, missing `@ref` segment, unparseable YAML): Same handling as missing `source:` — added to `unpinned[]`, contributing to `drift_state: "drifted"`. Surfaced explicitly, never silently ignored.
- **Workflow listed under `extra[]`**: Means the `.md` is present in the repo, has a parseable gh-aw `source:`, but the workflow name does not appear in any profile mapped to this repo in fleet.json. This is the case where someone manually `gh aw add`-ed a workflow outside fleet management.
- **Repo redirected on GitHub** (renamed): `gh api` follows the redirect and returns the new content; the status report uses the canonical name from fleet.json. Surfacing the redirect explicitly via a `warnings[]` entry is **deferred to a follow-up issue** — it requires an extra `/repos/<owner>/<name>` API call per repo to detect canonical-name mismatch, which the current `ghAPIRaw`/`ghAPIJSON` wrappers do not expose. Until then, status produces a correct drift report against the redirected repo without flagging the rename.
- **GitHub rate limit hit mid-run**: Whichever repos completed before the limit show their drift report; remaining repos show `errored` with a rate-limit-specific hint suggesting the operator wait or re-run with a different token. The process still emits a complete envelope and exits non-zero.
- **`fleet.json` declares zero repos**: Command emits an empty `result.repos[]`, exits `0`, with a warning in `warnings[]` noting the empty fleet.
- **`gh aw` not installed**: Status does not require `gh aw` (it only needs `gh` for the API). Confirm this in implementation; if any code path does invoke `gh aw`, fail with a clear hint pointing at installation docs.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The `status` subcommand MUST accept zero or one positional argument: zero means "all repos in the loaded fleet config"; one (`owner/repo`) means "that repo only".
- **FR-002**: Status MUST compute drift WITHOUT cloning any repo. All repo content reads MUST go through the existing `gh api` delegation pattern (the unexported helpers in `internal/fleet/fetch.go`, promoted as needed for reuse).
- **FR-003**: For every workflow declared in fleet.json for a given repo, status MUST fetch the corresponding `.github/workflows/<name>.md` from that repo via `gh api`, parse its YAML frontmatter, and read the `source:` field to determine the actual pinned ref.
- **FR-004**: Status MUST classify each declared workflow into exactly one of three buckets per repo: **Missing** (declared in fleet.json, not present in the repo), **Drifted** (present but pinned to a different ref than fleet.json declares), or **Aligned** (present and pinned to the matching ref). The "different ref" comparison MUST be a strict string equality test against the literal `source:` frontmatter value — no tag-to-SHA resolution, no semantic ref normalization. A SHA pin that resolves to the same content as the desired tag still counts as drifted because the recorded `source:` line will be rewritten by the next `gh aw update`.
- **FR-005**: Status MUST also identify **Extra** workflows: gh-aw managed `.md` files present in the repo (with a parseable `source:`) whose workflow name is not declared in fleet.json for that repo.
- **FR-006**: Each repo's report MUST roll up to a single `drift_state`: `aligned` (every category — `missing`, `extra`, `drifted`, `unpinned` — is empty), `drifted` (one or more present in any of those four categories), or `errored` (fetch failed for any reason). The full derivation rule (state-machine table) lives in `data-model.md` §Type 3 and is the canonical reference; downstream artifacts (contracts, this FR) summarize but do not redefine it.
- **FR-007**: When `status` is invoked without arguments, it MUST iterate every repo in the loaded fleet config (the merged `fleet.json` + `fleet.local.json` already produced by `LoadConfig`).
- **FR-008**: When `status` is invoked with an `owner/repo` argument, it MUST first verify the repo is declared in the loaded fleet config; if not, it MUST exit non-zero with a clear "repo not in fleet" error and MUST NOT issue any GitHub API calls.
- **FR-009**: A per-repo fetch failure (HTTP 404, 403, network error, malformed response) MUST NOT abort the multi-repo run. The failing repo MUST be reported as `errored` with a clear per-repo message, and the iteration MUST continue with the remaining repos.
- **FR-010**: When any per-repo fetch fails, the failure MUST be surfaced as a structured `Diagnostic` (defined in `internal/fleet/diagnostics.go`) with a stable `Code` (e.g., `repo_inaccessible`, `rate_limited`) and `Fields.repo` set to the failing repo. Repo-level structural errors are constructed directly at the call site rather than routed through `CollectHintDiagnostics`, because substring matching cannot disambiguate a missing-workflow 404 from a missing-repo 404. Pattern-matched hints from `gh api` stderr (e.g., `Unknown property:` and similar gh-aw-style diagnostic strings) MAY additionally be emitted via `CollectHintDiagnostics` for non-structural error classes — these complement, rather than replace, the structured per-repo diagnostic.
- **FR-011**: The process exit code MUST be `0` if and only if every queried repo's `drift_state` is `aligned`; otherwise it MUST be `1`. `errored` repos count toward non-zero exit.
- **FR-012**: Default human-readable output MUST be a per-repo table on stdout, showing each repo's `drift_state`, the count of items in each drift bucket, and (for drifted workflows) the old → new ref pairs. Exact formatting follows the project's tabwriter conventions used by `list` and `sync`.
- **FR-013**: Status MUST honor the persistent `-o` / `--output` flag introduced by spec 003. With `-o json`, status MUST emit a JSON envelope conforming to that envelope contract: `schema_version`, `command: "status"`, `repo` (the single-repo arg or empty for fleet-wide), `apply: false`, `result`, `warnings[]`, `hints[]`.
- **FR-014**: The `result` object for status MUST contain a `repos[]` array, each element a `RepoStatus` with stable JSON keys: `repo` (string), `drift_state` (string: `aligned`, `drifted`, `errored`), `missing[]` (string array of workflow names), `extra[]` (string array of workflow names), `drifted[]` (array of objects with `name`, `desired_ref`, `actual_ref`), `unpinned[]` (string array of workflow names whose installed `.md` lacks a parseable `source:` frontmatter), and `error_message` (string, empty unless `drift_state == "errored"`).
- **FR-015**: All array fields in the JSON output MUST serialize as `[]` (never `null`) when empty, per FR-009 in spec 003.
- **FR-016**: In JSON mode, every `errored` repo MUST also produce a hint in `hints[]` with a stable code (e.g., `repo_inaccessible`, `rate_limited`), and every warning condition (e.g., empty fleet) MUST appear in `warnings[]`. Redirected-repo detection is out of scope for this release (see Edge Cases — tracked in a follow-up issue).
- **FR-017**: Status MUST NOT mutate any local file, repo, or GitHub state. No writes, no PRs, no commits, no branches, no clones.
- **FR-018**: A run against an N-repo fleet MUST issue at most O(N × M) GitHub API calls where M is the number of declared workflows per repo (one fetch per workflow). The implementation MUST process repos through a bounded worker pool sized between 4 and 8 concurrent workers (configurable as an internal constant; not user-tunable in this release). Workflow fetches WITHIN a single repo MUST run serially to keep partial-failure ordering deterministic per repo. Errors in one worker MUST NOT cancel other workers; each repo's report is computed independently and the final exit code aggregates across all workers.
- **FR-019**: The human-readable output MUST be byte-identical regardless of whether the `-o json` codepath is exercised in the same process; the two output modes are disjoint.
- **FR-020**: Status MUST work against the existing two source layouts (`github/gh-aw` and `githubnext/agentics` as encoded in `SourceLayout`) without changes to fleet.json. Adding a third source MUST NOT require a status-specific code change beyond the existing `SourceLayout` extension point.

### Key Entities *(data)*

- **RepoStatus**: Drift report for one repo. Fields: `repo` (string — `owner/name`), `drift_state` (`aligned` | `drifted` | `errored`), `missing` (string array — declared workflow names not present in the repo), `extra` (string array — gh-aw managed workflow names present in the repo but not declared), `drifted` (array of `WorkflowDrift`), `unpinned` (string array — workflow names whose installed `.md` is present but lacks a parseable `source:` frontmatter), `error_message` (string — empty unless `drift_state == "errored"`).
- **WorkflowDrift**: A single workflow whose actual pinned ref does not match the desired ref. Fields: `name` (string — workflow basename without `.md`), `desired_ref` (string — what fleet.json says, e.g., `v0.68.3`), `actual_ref` (string — what the repo's `source:` frontmatter says, e.g., `v0.67.0`).
- **StatusResult**: The top-level result object inside the JSON envelope's `result` field. Fields: `repos` (array of `RepoStatus`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator running `gh-aw-fleet status` against a 10-repo fleet receives a complete drift report in under 20 seconds end-to-end on a typical broadband connection — at least 5× faster than `deploy --dry-run` against the same fleet, which clones every repo.
- **SC-002**: Per-repo runtime is under 2 seconds when the repo has 5 or fewer declared workflows, dominated by `gh api` round-trip latency.
- **SC-003**: Across 100 invocations against a fleet with mixed drift states (some aligned, some drifted, some inaccessible), zero clones are observed in the temp directory (`/tmp/gh-aw-fleet-*` is never created), confirming the no-clone guarantee.
- **SC-004**: An operator can chain `gh-aw-fleet status && gh-aw-fleet deploy --apply ...` in a CI script and have the chain abort cleanly when drift exists, without reading or parsing the human-readable output.
- **SC-005**: A CI pipeline consuming `gh-aw-fleet status -o json` extracts the count of drifted repos with a single `jq` expression (`jq '.result.repos | map(select(.drift_state == "drifted")) | length'`) — zero regex parsing required.
- **SC-006**: The full local gate (`make ci` — `fmt-check`, `vet`, `lint`, `test`) passes with the feature in place. No new third-party dependencies appear in `go.sum`.
- **SC-007**: A repo that becomes inaccessible mid-run (rate limit, transient 5xx) is reported as `errored` for that run only and recovers cleanly to its true drift state on the next invocation, with no cached/stale state persisted between runs.
- **SC-008** (design property, verified by code review rather than test): An operator who introduces a fourth source layout (e.g., `myorg/internal-workflows`) by extending `SourceLayout` and adding profile pins gets working `status` output for that source on the next run, with zero changes to status-specific code. Verification: a reviewer confirms `internal/fleet/status.go` consumes `ResolvedWorkflow.Spec()` and reads `source:` frontmatter generically — it does not enumerate or branch on source-repo names. (Adding a literal fourth `SourceLayout` entry just to test removal of it would not exercise meaningful behavior; this SC is a design constraint on the implementation surface, not a runtime measurement.)

## Assumptions

- **gh-aw embeds the source ref in the workflow's markdown frontmatter under `source:`.** Confirmed by `internal/fleet/deploy.go:635` ("Each workflow is pinned via its frontmatter `source:` field"). The existing `SplitFrontmatter` and `ParseFrontmatter` helpers in `internal/fleet/frontmatter.go` parse it. The issue's open question about whether the marker lives in the lock.yml or the markdown is resolved: **markdown frontmatter, not the lock file**.
- **Status reads from each repo's default branch** on GitHub — whatever `gh api /repos/<owner>/<repo>/contents/...` returns without an explicit `?ref=...` parameter. Operators care about what's actually deployed, which is what the default branch reflects. No `--ref` flag is provided in this release; a follow-up issue tracks demand-driven addition of an opt-in `--ref <branch>` flag for advanced workflows (PR pre-merge checks, maintenance-branch validation). Until that issue is resolved and the flag implemented, status against an arbitrary feature branch is out of scope.
- **Multi-repo JSON output uses a single envelope** with `result.repos[]` rather than NDJSON. Status is a snapshot/read-only command — the natural shape is one document, not a stream. This differs from `upgrade --all` (which emits NDJSON) because upgrade has long-running per-repo work that benefits from streaming progress, while status is fast and homogeneous.
- **Inaccessible repos count toward exit code `1`.** The issue's stated rule ("0 if no drift, 1 otherwise") is interpreted to mean: any non-clean state — drift OR error — is exit `1`. An operator running `status && deploy --apply` wants the chain to abort if any repo couldn't be queried; they need to fix that before deploying. Operators who want exit-code parity only on confirmed drift can filter `result.repos[]` in JSON mode.
- **Plain GitHub Actions workflows (not gh-aw managed) are ignored.** Status only reports on `.md` files in `.github/workflows/` whose YAML frontmatter has a `source:` field. A repo with a hand-written `ci.yml` that has no paired `.md` is not flagged as `extra` — that's just a regular Actions workflow outside fleet scope.
- **Workflows with malformed or missing `source:` frontmatter are surfaced explicitly** under the dedicated `unpinned[]` category in `RepoStatus`, contributing to `drift_state: "drifted"`. They are NOT silently aligned. This protects against false-clean reports when something went wrong during a previous deploy.
- **No new persistent state.** Status output goes to stdout, that's it. No cache, no state file, no comparison against a prior snapshot. Each run computes from scratch.
- **Status is unaffected by gpg signing or local git config.** It never invokes `git`, so the `git push` / commit-signing constraints in CLAUDE.md don't apply.
- **`status` honors the existing `--log-level` / `--log-format` flags** on the root command for stderr diagnostics, consistent with `deploy` / `sync` / `upgrade`. No new flags beyond `-o` / `--output` (already inherited from root).
- **The `cmd/stubs.go` entry for status is replaced**, not augmented. Once status is implemented, its constructor moves out of `stubs.go` into a dedicated `cmd/status.go`, matching the pattern set by `deploy`, `sync`, `upgrade`, `add`, etc.
