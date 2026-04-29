# Phase 0 Research: `status` Subcommand for Drift Detection

**Feature**: `004-status-drift-detection` | **Date**: 2026-04-28 | **Plan**: [plan.md](./plan.md)

This document records the integration and design decisions consolidated during planning. There were **no `NEEDS CLARIFICATION` markers** carried over from the spec — the `/speckit.clarify` session resolved all four open questions (concurrency model, ref selection, drift comparison rule, missing-frontmatter schema) before this phase. The research below confirms each integration point against the existing codebase rather than choosing among unknowns.

---

## R1. Where does `gh aw` embed the source ref of an installed workflow?

**Decision**: Read from the workflow's markdown YAML frontmatter, `source:` key.

**Rationale**: `internal/fleet/deploy.go:635` already states this as an established invariant: "Each workflow is pinned via its frontmatter `source:` field." The existing `SplitFrontmatter` (in `internal/fleet/frontmatter.go:18`) and `ParseFrontmatter` (line 39) helpers parse this without modification. The spec's `source:` value follows the format `<owner>/<repo>/<path>@<ref>` (e.g., `github/gh-aw/.github/workflows/audit.md@v0.68.3` or `githubnext/agentics/workflows/ci-doctor.md@main`). For drift detection, the ref segment is the only relevant slice — but the entire string is captured into `actual_ref` for clarity in the `WorkflowDrift` JSON output.

**Alternatives considered**:

- **Read from `.lock.yml` comment markers**: The `.lock.yml` is generated output, not source-of-truth metadata. Its layout is owned by `gh aw` and could change between releases. The markdown frontmatter is what `gh aw add` and `gh aw upgrade` both write directly.
- **Wait for an upstream `gh aw query` API**: Would defer this feature indefinitely. The marker we need already exists.

---

## R2. How does `gh api .../contents/...` resolve the default branch?

**Decision**: Issue `gh api /repos/<owner>/<name>/contents/.github/workflows/<file>.md` with **no** `?ref=` parameter. GitHub's REST API resolves `contents` requests against the repo's `default_branch` field when ref is unset.

**Rationale**: This is documented GitHub REST behavior. It honors per-repo default-branch settings (`main`, `master`, `develop`, anything custom) without status having to detect it. Hard-coding `?ref=main` would break against repos with non-`main` defaults.

**Implementation note**: For the directory enumeration needed to find **extra** workflows (workflows present in the repo but not declared in fleet.json), status calls `gh api /repos/<owner>/<name>/contents/.github/workflows` (no `?ref=`) and iterates the returned array, keeping only entries whose `name` ends in `.md`. The existing `fetchSource` function in `internal/fleet/fetch.go:94` already demonstrates this pattern (with explicit `?ref=main` for catalog discovery — for status's use case, omitting `?ref=` is correct).

**Alternatives considered**:

- **Pass `?ref=main` explicitly**: Brittle for any repo whose default branch isn't `main`.
- **Fetch `default_branch` first via `/repos/<owner>/<name>`, then issue contents requests with explicit ref**: Adds one extra API call per repo for no functional gain.

---

## R3. Concurrency model for the multi-repo fan-out

**Decision**: Hand-rolled bounded worker pool using stdlib primitives (`sync.WaitGroup`, buffered channel as a job queue). Worker count: constant `statusWorkerPoolSize = 6`, the midpoint of the spec's 4–8 range. Each worker dequeues a repo, runs all that repo's workflow fetches **serially**, builds a `RepoStatus`, and pushes it to a results channel. Errors in one worker do not cancel siblings (per FR-009 — per-repo failure isolation).

**Rationale**:

- **No new dependency**: stdlib `sync.WaitGroup` + channels is sufficient. The project does not import `golang.org/x/sync/errgroup` and adding it for this feature would violate Constitution Principle I and SC-006 ("zero new third-party dependencies").
- **`errgroup` is the wrong shape anyway**: `errgroup.Group` cancels all sibling goroutines on first error. Status MUST collect every repo's report independently — a 404 on `repoA` cannot abort the in-flight fetch of `repoB`. Hand-rolling a `WaitGroup` keeps that semantic explicit.
- **Worker count rationale (6)**: Empirical fan-out work against GitHub's REST API tends to plateau between 4 and 10 concurrent connections per token (above that, server-side rate limiting kicks in or TCP/TLS handshake serialization dominates). 6 is the safe midpoint and matches the spec's clarification.
- **Serial within a repo**: Keeps per-repo error attribution deterministic — if workflow #3 of 5 fails, the prior two completed reports are already attached to the `RepoStatus` in order.

**Alternatives considered**:

- **`golang.org/x/sync/errgroup`**: New dependency; wrong cancellation semantics. Rejected.
- **`golang.org/x/sync/semaphore.Weighted`**: New dependency; same outcome as a buffered channel queue but with more API surface. Rejected.
- **Strictly serial (one repo at a time)**: Trivial to implement but fails SC-001 for fleets above ~10 repos. Rejected per clarification 1.
- **Fully unbounded parallelism (one goroutine per workflow)**: Highest throughput but unbounded — could exhaust GitHub's per-token rate limit (5000 req/hr) on a large fleet, and concurrency hazards multiply. Rejected per clarification 1.

---

## R4. JSON envelope integration

**Decision**: Status emits the existing JSON envelope from spec 003 (`schema_version`, `command`, `repo`, `apply: false`, `result`, `warnings[]`, `hints[]`) with `result` set to a new `StatusResult` struct containing a single `repos[]` array.  Multi-repo runs emit **one** envelope (single-document JSON), not NDJSON.

**Rationale**:

- Spec 003 (`003-cli-output-json`) already established the envelope contract and registered `cmd.SchemaVersion = 1`. Status is downstream — it does not bump the envelope schema; it just adds another command type.
- **Single envelope, not NDJSON**: Status is a snapshot, not a stream. Operators consume it as one document, often piping to `jq` for cross-repo predicates (e.g., `jq '.result.repos | map(select(.drift_state == "drifted"))'`). NDJSON would force consumers to assemble a top-level array themselves before such queries work. `upgrade --all` chose NDJSON because per-repo work is long-running and streaming progress matters; status is fast and homogeneous.
- The `command` field is the literal string `"status"`. The `repo` field is the single-repo argument (e.g., `"rshade/gh-aw-fleet"`) when status is invoked with one positional arg, or empty string `""` for fleet-wide runs. The `apply: false` is constant — status never mutates.

**Alternatives considered**:

- **NDJSON one-per-repo**: Mismatch with status's snapshot semantics; harder for cross-repo `jq` queries.
- **Bump `schema_version` to 2**: Unnecessary — adding a new command type does not change the envelope shape.

---

## R5. CollectHints integration (which patterns to add)

**Decision**: Audit the existing `hints` table in `internal/fleet/diagnostics.go:41` against the failure modes status will encounter. Add patterns for any uncovered failure class.

**Findings** (from reading `diagnostics.go`):

| Failure class | Existing pattern | Action for status |
|---|---|---|
| Workflow file `.md` not found (HTTP 404 on contents API) | `"HTTP 404"` → `DiagHTTP404`. Existing message references `gh aw` source paths. | **Reuse** — but the existing message text is `gh aw`-centric. For status's "workflow declared in fleet.json but missing in repo" case, this is *expected*, not an error — it goes into `RepoStatus.missing[]`, NOT `hints[]`. Only emit the hint when a 404 is unexpected (e.g., the repo itself returns 404, distinct from a missing workflow file). |
| Repo not found / private repo no access (HTTP 404 or 403 against the **repo root** rather than a workflow file) | None directly. The existing 404 hint matches but the message points operators at the wrong remediation. | **Add new hint**: pattern `"Not Found"` *combined with repo-level context* — but pattern matching on substring alone can't disambiguate workflow-404 from repo-404. **Implementation choice**: status disambiguates structurally (which API call returned the 404), not by hint pattern. Synthesize the right `Diagnostic` directly from status's call site (e.g., code `repo_inaccessible` with `fields: {repo: "..."}`) rather than running the message through `CollectHintDiagnostics`. |
| GitHub rate limit exceeded (HTTP 403 with `X-RateLimit-Remaining: 0`) | None. | **Add new hint pattern**: `"API rate limit exceeded"` → message advising the operator to wait or rotate tokens. New `Code: rate_limited` constant. |
| Network error (host unreachable, TLS failure) | None. | **Add new hint pattern**: `"Could not resolve host"` and/or `"connection refused"` → generic-ish "check network / VPN" message. New `Code: network_unreachable` constant. Optional — may defer if the existing free-form `HintFromError` covers the case adequately. |

**Implementation plan**: Add two new `Hint` entries to the `hints` slice (`API rate limit exceeded`, optionally `Could not resolve host`). Add two new `Diag*` constants (`DiagRateLimited`, optionally `DiagNetworkUnreachable`). Add a status-specific `Diagnostic` constructor for the structural cases (repo-level 404/403) that don't fit the substring-match model.

**Rationale**: Adding new hint patterns is a one-table edit per the diagnostics.go package comment ("Adding a hint pattern means touching one table"). Status's per-repo failure mode coverage is what justifies the additions — it raises the failure surface beyond what `deploy` / `sync` / `upgrade` already trigger.

**Alternatives considered**:

- **Reuse existing 404 hint as-is**: The current message references `gh aw` source path conventions, which is wrong remediation for "this repo doesn't exist or isn't accessible to your token."
- **Don't add new patterns; just emit `HintFromError`**: Loses the stable `Code` for downstream `jq` consumers (e.g., dashboard rules that filter `hints | map(select(.code == "rate_limited"))`).

---

## R6. Detecting "extra" workflows (in repo but not declared)

**Decision**: After fetching every declared workflow's `.md` for a repo, status issues **one additional** `gh api .../contents/.github/workflows` call to enumerate the directory listing. Compare directory entries against the declared set; any `.md` file present in the listing whose name (minus `.md`) is not in the declared set, AND which has a parseable `source:` frontmatter, is reported under `extra[]`. Files lacking `source:` frontmatter belong to `unpinned[]` (only when the workflow IS declared) — extra files without `source:` are ignored as "not gh-aw managed."

**Rationale**:

- Adds exactly one extra API call per repo (constant overhead). Per FR-018 the call count is O(N × M) where M is per-repo workflow count; this directory enumeration is one additional call per repo, well within bounds.
- The `source:` requirement filters out hand-written `ci.yml`-style workflows that aren't gh-aw managed (per spec Edge Cases — "Plain GitHub Actions workflows are out of scope").
- The directory listing is also cheaper than per-file probes: returns names in one round-trip.

**Implementation detail**: To keep the API count tight, status fetches the directory listing **first**, then for each declared name, looks up the listing entry and only fetches the `.md` body if the entry exists. Workflows declared but NOT in the listing go directly to `missing[]` with no additional API call.

**Alternatives considered**:

- **Skip extra detection** (only report missing/drifted/unpinned): Loses a real operator value — out-of-band `gh aw add` is exactly the case where status earns its keep.
- **Fetch every workflow body unconditionally**: Inflates API calls when workflows are missing.

---

## R7. Handling tag-vs-SHA equivalence (non-issue per clarification 3)

**Decision**: No SHA resolution. Strict string comparison only.

**Rationale**: Per clarification 3 of the spec. The follow-up tracking issue (filed as #62) covers the "what if false positives become real pain" question.

**Implementation note**: `actual_ref` is the literal substring after `@` in the `source:` frontmatter value. `desired_ref` is the literal value of the corresponding source's pin in fleet.json (`profiles.<name>.sources["github/gh-aw"]`, etc.). String inequality → drifted.

---

## R8. cmd/status.go ↔ internal/fleet/status.go split

**Decision**: Mirror the existing `cmd/deploy.go` ↔ `internal/fleet/deploy.go` split:

- `cmd/status.go`: Cobra command construction, flag wiring, output formatting (text tabwriter + JSON envelope dispatch), exit-code computation.
- `internal/fleet/status.go`: Pure logic — `Status(ctx context.Context, cfg *Config, opts StatusOpts) (*StatusResult, []Diagnostic, error)`, the worker pool, per-repo drift computation, all `gh api` calls. Three returns: (1) the wire payload, (2) per-repo `errored` diagnostics PLUS fleet-wide warnings (e.g., `empty_fleet`) — `cmd/status.go` splits this slice into the envelope's `warnings[]` vs `hints[]` by `Code`, (3) setup-time fatal errors only (config load failure, single-repo arg not in fleet — see FR-008). See data-model.md Type 2 for the lifecycle and the rationale for keeping diagnostics out of `StatusResult`.

**Rationale**: Established pattern — five existing commands follow it (`deploy`, `sync`, `upgrade`, `add`, `list`). Same-package placement of `status.go` next to `fetch.go` / `frontmatter.go` / `diagnostics.go` keeps the unexported `ghAPIRaw` / `ghAPIJSON` helpers reachable without an export-API surface change.

**Alternatives considered**:

- **Keep status logic in `cmd/status.go`**: Mixes presentation with logic; breaks the established split.
- **Promote `ghAPIRaw` / `ghAPIJSON` to exported `GhAPIRaw` / `GhAPIJSON`**: Pure surface bloat; nothing outside `internal/fleet/` needs them.

---

## R9. Spec 003 dependency

**Decision**: Status assumes spec 003 (`003-cli-output-json` — JSON envelope mode) has landed. The existing `cmd/output.go` infrastructure is consumed; no parallel envelope shim.

**Rationale**: Spec 003 is upstream in the dependency graph. Both the current branch and 003's branch coexist; the merge order is 003 → 004 (status) on `main`. If for some reason 003 doesn't land first, status's JSON path still works on a stub — the envelope shape is well-defined and a thin local shim could ship — but the cleaner outcome is sequential merge.

**Implementation note**: Verify `cmd/output.go` exposes the envelope-emit helper status will call (likely a function like `emitEnvelope(cmd *cobra.Command, command string, repo string, apply bool, result any, warnings []Diagnostic, hints []Diagnostic)`). If the helper signature is not yet stable, status's plan will need to refactor in parallel; otherwise it's a direct call.

---

## R10. Test fixtures and integration boundaries

**Decision**: Two layers of testing.

1. **Unit tests in `internal/fleet/status_test.go`**: Table-driven against in-memory inputs. The pure function under test is `computeDrift(declared map[string]ResolvedWorkflow, listing []string, fetched map[string]string) RepoStatus` (or similar) — given the desired set, the actual directory listing, and the actual fetched bodies (as strings, simulating `ghAPIRaw` returns), assert the four output buckets (`missing`, `extra`, `drifted`, `unpinned`) and the rolled-up `drift_state`. **No network. No goroutines. No `gh api`.** The worker pool is tested via the orchestrator function with a stubbed fetcher (an interface or function type so tests can inject a fake).

2. **Manual integration test (documented in PR)**: Run `gh-aw-fleet status` against `rshade/gh-aw-fleet` (the project's own fleet); compare against `gh-aw-fleet deploy --dry-run rshade/gh-aw-fleet` to verify drift parity. This is the spec's Testing Strategy line item 3.

**Rationale**: The constitution explicitly does not require unit tests, but the spec's P1 acceptance scenarios call for diff-logic verification, which is most cleanly done as a unit test (no flaky network, fast, deterministic). Integration is verified manually because mocking GitHub is not the test substrate the project values (per Constitution Principle II).

**Implementation hint for the test fixture shape**: Use Go fixtures, not files-on-disk. Each test case is a struct: `{name string, declared []ResolvedWorkflow, listing []string, fetchedBodies map[string]string, want RepoStatus}`. Run as `t.Run` subtests. Coverage targets: aligned, drifted (one workflow), missing (one workflow), extra (one workflow), unpinned (no `source:` field), unpinned (malformed YAML), errored (fetcher returns err). 7 cases minimum.
