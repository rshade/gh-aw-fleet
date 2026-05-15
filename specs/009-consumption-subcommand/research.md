# Research: Consumption Subcommand

**Feature**: 009-consumption-subcommand
**Plan**: [plan.md](./plan.md)
**Status**: Phase 0 — resolves all technical-context unknowns identified in plan.md.

## Scope

The spec.md had no `[NEEDS CLARIFICATION]` markers, but the implementation introduces three new external-facing concerns to this codebase: querying GitHub Discussions, fetching workflow-run artifact files, and parsing the upstream `api-consumption-report` payload. None of these are exercised by any existing fleet command. This document captures the decisions and the alternatives evaluated for each.

---

## Decision 1: Discussion discovery query — REST vs. GraphQL

**Decision**: Use the REST endpoint `gh api repos/{owner}/{repo}/discussions --paginate` and filter the response stream client-side in Go (`category.slug == "audits"` + body substring `<!-- gh-aw-tracker-id: api-consumption-report-daily -->`).

**Rationale**:

1. `gh api` with a REST path automatically inherits the authenticated token, pagination via `--paginate`, and the existing project pattern at `internal/fleet/fetch.go:184` (`ghAPIJSON`). The new injection seam is a one-line variable matching that existing one.
2. The GraphQL alternative (`gh api graphql -f query=...`) would let the server do the category filter, but it requires authoring a multi-line GraphQL query string, escaping `--jq`-style projection arguments, and (critically) the response shape is bespoke per query — every test fixture would need handcrafted GraphQL shapes. REST returns a stable, documented array-of-discussion shape that the upstream `api-consumption-report` workflow already emits against.
3. Client-side filtering keeps the parsing contract anchored on the HTML-comment marker (FR-006), which is the stable interface declared in the upstream workflow's YAML frontmatter. Server-side GraphQL filtering by body substring is not supported anyway, so the client filter is unavoidable; the only question is whether to also use GraphQL for category, which would split the filter across two layers without simplifying either.

**Alternatives considered**:

- *GraphQL query with `category: { slug: "audits" }` filter*: Rejected. Splits the filter across the network and the client without removing the body-marker scan.
- *Tracker-marker-as-URL-fragment search*: Rejected. The GitHub search API does not match inside HTML comments, so a substring search on the marker would not be reliable.
- *Single-page non-paginated REST*: Rejected. Daily reports accumulate; a fleet repo can easily have >30 discussions per page after a month. `--paginate` is the right default.

**Concrete shape consumed**: Each discussion JSON object carries at least `number`, `title`, `body`, `category.slug`, `html_url`, `created_at`, `updated_at`. The fleet code reads exactly these fields and ignores the rest — no struct over-fitting to upstream shape.

---

## Decision 2: Workflow-run artifact fetch — REST endpoint vs. `gh run download`

**Decision**: Use `gh api repos/{owner}/{repo}/actions/runs/{run_id}/artifacts` to list the artifacts attached to a run, then `gh api repos/{owner}/{repo}/actions/artifacts/{artifact_id}/zip` to fetch the artifact zip. Unzip in memory (Go stdlib `archive/zip` + `bytes.Reader`), extract `aw_info.json` and `run_summary.json`, decode each as JSON.

**Rationale**:

1. Same `gh api` pattern; same authentication; same injection seam. `gh run download` exists but is interactive (writes to disk under cwd) and is shaped for human use, not programmatic fanout. Driving it programmatically would require temp directories, working-directory hygiene, and cleanup — all costs that the direct API path avoids.
2. The two artifact files we need are small (KB range): a single zip per run, unzipped in-process, is cheap.
3. The 404 case (run garbage-collected after ~90 days) is uniformly surfaceable through `ghErr` and the existing `DiagHTTP404` hint, which gives us FR-009 (retention-bound graceful degradation) for free.

**Alternatives considered**:

- *`gh run download <id>` (uses `actions/artifacts/{id}/zip` under the hood anyway)*: Rejected for the cwd/temp-dir overhead.
- *Single API call to the run object with embedded artifact bytes*: No such REST endpoint exists. Artifacts must be listed then individually downloaded.
- *Direct download via the `archive_download_url` field*: Identical mechanics under the hood and equivalent for our needs; using the explicit `/zip` path makes the contract more readable. Keeping the documented endpoint.

**Concrete shape consumed**:

- `artifacts` list endpoint returns `{ artifacts: [{ id, name, archive_download_url }, ...] }`. We pick the artifact whose `name` matches a stable prefix (the upstream gh-aw engine names its info artifact predictably; see contracts/run-artifact-payload.md for the exact pattern).
- The zip contains exactly two JSON files we care about: `aw_info.json` (top-level rate-limit usage, run cost, per-workflow runs) and `run_summary.json` (workflow-level breakdown).

---

## Decision 3: Body-marker parsing — regex vs. structured frontmatter

**Decision**: Use small, focused, anchored `regexp.MustCompile` patterns at package scope for each marker:

- `runIDRe`: `/actions/runs/(\d+)/agentic_workflow` — captures the run ID from the body's standard agentic_workflow link
- `expiresRe`: `<!-- gh-aw-expires: ([^ ]+) -->` — captures the ISO timestamp
- `trackerRe`: `<!-- gh-aw-tracker-id: api-consumption-report-daily -->` — used as a substring presence check, not a capture
- In-progress indicator: substring match for `"🔄 in-progress"` in the body

Title parsing uses `time.Parse("2006-01-02", title)` after stripping any leading prefix the upstream workflow may add (we tolerate a leading title prefix and a trailing parenthetical because the upstream workflow's title format is not pinned).

**Rationale**:

1. The markers are the parsing contract (FR-006, FR-008). Regex matches the structure exactly and fails loudly when the upstream format drifts — the failure surfaces as a `diagnostic warning per report` (edge case in spec.md) rather than a silent miss.
2. Compiled once at package init; the per-call cost is negligible.
3. The four markers are visually independent and naturally documented as four package-level `var ...Re = regexp.MustCompile(...)` declarations. Bundling them into a single mega-pattern would compress them at the cost of readability and per-marker error messages.

**Alternatives considered**:

- *YAML frontmatter parser*: Rejected. The markers live in HTML comments inside the body, not in a frontmatter block. There is no frontmatter on a Discussion body.
- *`html` package DOM walk for HTML comments*: Rejected. HTML comments in markdown bodies are not real HTML comments after the rendering pipeline; they're inline markdown-passthrough text. The regex is honest about what we're matching.

---

## Decision 4: Trailing-window parser — `--trailing 7d` syntax

**Decision**: Accept exactly the `Nd` shape (positive integer followed by literal `d`), parsed via a single small regex `^(\d+)d$`. Days are the only supported unit; any other unit (`h`, `w`, `m`, plain `7`) returns an explicit error naming the accepted form.

**Rationale**:

1. The originating issue spec wrote `--trailing <Nd>` explicitly. Honoring exactly that surface keeps the CLI contract narrow.
2. Daily reports are the cadence of the upstream `api-consumption-report` workflow. Sub-day units (`12h`) would not align with any actual report's resolution; reports for the same day would either all be in-window or all out-of-window. Sub-day precision is meaningless.
3. Multi-day units like `2w` could be useful but are convertible to days (`14d`) by the operator without ambiguity. Supporting both forms invites confusion about the semantics of the week boundary.

**Alternatives considered**:

- *Go `time.ParseDuration` ("168h" for a week)*: Rejected. Operator-facing surface, "168h" is unfriendly.
- *Free-form date math ("last week", "Mon")*: Rejected. Implicit timezone and reference-date assumptions invite bugs.

---

## Decision 5: Multi-profile aggregation — additive double-counting vs. error vs. apportionment

**Decision**: Additive double-counting (FR-014). A repo declared in two profiles contributes its full consumption to both profile groups. Documented in the operator-facing usage text and in the rendered output.

**Rationale**:

1. The repo's consumption is real and undivided — apportioning a fraction to each profile would invent precision that doesn't exist (the consumption report does not know which profile drove which workflow's calls, only that the workflow ran on the repo).
2. Erroring on multi-profile repos would reject a legitimate fleet configuration (the existing `*-plus` profiles are explicitly designed to be layered onto `default` per-repo). The whole point of the layered-profile architecture is multi-profile membership.
3. Additive is the most useful operator-facing semantic at the budget meeting: it answers "what does the `premium` profile cost when everything that uses it runs?" — which is the question the budget owner asks.
4. The cost of double-counting is paid in operator interpretation: a sum across all profile groups will exceed the fleet's actual consumption. The documentation in FR-014 and the user-facing assistance handle this by stating it plainly. A summary footer-row could be added later if confusion proves real; not needed at v1.

**Alternatives considered**:

- *Error on multi-profile repos*: Rejected per (2) above.
- *Apportion proportionally by workflow count*: Rejected. Apportionment is fiction since consumption is per workflow-run, not per profile-membership, and workflows can belong to multiple profiles even within the same repo.
- *Pick the lexicographically-first profile*: Rejected. Arbitrary tie-break, no operator-meaningful semantic.

---

## Decision 6: Cost-field zero-vs-absent disambiguation

**Decision**: Treat any non-positive value of the upstream `cost` field as equivalent to absent (FR-018). Internally, `cost` is a `*float64` pointer; if the upstream JSON omits the field, the pointer is nil; if the field is present but the parsed value is ≤ 0, the pointer is nilled out before storing.

**Rationale**:

1. The upstream `cost` field is currently undocumented and unstable (Assumptions §3). A literal zero from the upstream engine is more likely a placeholder default than a "this run cost zero dollars" assertion. Treating zero as nil avoids polluting downstream cost displays with stray zeros that aren't real signal.
2. Negative values are nonsensical in a billing context — treating them as nil also handles them defensively.
3. The pointer-vs-nil distinction lets the JSON envelope omit the field via `omitempty` when absent and emit it when populated. Downstream consumers (jq filters in operator-authored dashboards) can `select(.cost != null)` without surfacing zero rows.

**Alternatives considered**:

- *Treat zero as a valid cost*: Rejected per (1) above; pollutes display.
- *Treat any value (including negative) as valid*: Rejected per (2); negative is nonsense.
- *Use `omitempty` on a float, not a pointer*: Rejected. `omitempty` on `float64` drops only the literal zero, not negatives or absent fields, and is ambiguous about whether zero meant "absent" or "really zero." The pointer makes the absent/present distinction explicit.

---

## Decision 7: Test-injection seams — package-level `var func` vs. interface

**Decision**: Use two package-level `var func(...)` injection seams, exactly mirroring the existing `internal/fleet/fetch.go:183` (`ghAPIJSON`):

```go
//nolint:gochecknoglobals // test-injection seam for gh api discussion list
var ghDiscussionsAPI = func(ctx context.Context, repo string) ([]discussionJSON, error) { ... }

//nolint:gochecknoglobals // test-injection seam for gh api run artifact fetch
var ghRunArtifactAPI = func(ctx context.Context, repo string, runID int64) (artifactPayload, error) { ... }
```

**Rationale**:

1. The codebase has already chosen this pattern at `internal/fleet/fetch.go:183` — adopting it preserves consistency. A test substitutes the function variable in `func TestXxx(t *testing.T) { old := ghDiscussionsAPI; ghDiscussionsAPI = fakeImpl; t.Cleanup(...) }`.
2. An interface-based seam (`type discussionFetcher interface { ... }` plus a constructor parameter) would require threading the dependency through every caller — for a single-package internal feature this is over-engineering.
3. The `//nolint:gochecknoglobals` directive is already accepted by the project's lint config for this exact pattern; reusing it is the path of least surprise.

**Alternatives considered**:

- *Interface + constructor injection*: Rejected for over-engineering as above.
- *`net/http/httptest` against a fake `gh` binary*: Rejected. Driving a real subprocess introduces flakiness without proving more than the function-variable seam.

---

## Decision 8: Diagnostic codes — re-use existing vs. add new

**Decision**: Re-use existing codes (`DiagHTTP404` for retention-expired artifacts, `DiagRateLimited` for upstream rate-limit during the rollup, `DiagBillingQuotaExceeded` if the rollup itself hits a quota wall) plus generic `DiagHint` for repo-level "no reports found" and "in-progress report skipped" cases. No new codes added.

**Rationale**:

1. The originating issue explicitly states "No new diagnostic codes needed; warnings come from `shouldIncludeReport` and artifact-fetch failures via `fleet.Diagnostic` in the existing envelope's `warnings[]` slot."
2. The existing `internal/fleet/diagnostics.go:64` `billingQuotaHint` already forward-references this subcommand by name in its remediation copy. After this slice ships, the copy reads literally true; no edit needed beyond a verification pass.
3. New diagnostic codes are downstream-observable contract; introducing them when existing codes already cover the failure shape would create unjustified surface area.

**Alternatives considered**:

- *Add `DiagConsumptionReportMissing`, `DiagConsumptionInProgress`, `DiagConsumptionRetentionExpired`*: Rejected. Per (3) — and the FR text in spec.md FR-009 / FR-010 / FR-011 / FR-012 emphasizes the *behavior* (warning surfaced) over the *code* (which specific code label).

---

## Decision 9: Output format — text tabwriter shape

**Decision**: Two stacked tabwriter tables. The primary table is keyed by the `--by` axis value (one column for the key, columns for API calls / safe-output writes / cost-when-present / report count). The secondary footer table (TOP 10 BURNERS) is keyed by workflow name and ordered descending by API calls (or cost when populated). Tables are separated by a blank line and a header line `TOP 10 BURNERS:`.

**Rationale**:

1. `cmd/list.go` already uses `text/tabwriter` with `tabPadding = 2`; matching that exactly minimizes surprise.
2. The two-table shape is what the upstream `api-consumption-report` itself renders in its discussion body — operators reading the discussion are already trained on this shape. Mirroring it gives them an immediate "this is what I expected" experience.
3. A single combined table would conflate two semantics (rollup vs. burner detail) and force narrower columns to fit both.

**Alternatives considered**:

- *Single combined table*: Rejected per (3) above.
- *JSON-by-default, text via `--output text` flag*: Rejected. The project's pattern is text-by-default with `--output json` available; reversing would be jarring.
- *Per-`--by` axis bespoke renderers*: Rejected for unnecessary code duplication. The four axes share one rendering core that differs only in the key column header.

---

## Decision 10: Story phasing — which user stories ship in v1 vs. follow-up

**Decision**: All four user stories from spec.md (P1 repo rollup, P2 temporal modes, P2 group-by axes, P3 top burners) ship together in v1. They share enough of the discovery + parse + aggregate core that splitting them across releases would mean shipping a discovery path with no useful output, then adding the consumption it was always meant to feed.

**Rationale**:

1. The spec's P1 alone delivers the headline value, but its implementation already requires the parser and the aggregator — adding the trailing-window filter (P2) and the group-by axis (P2) on top is a small additional lift, while removing those features after they're written has zero benefit.
2. The top-burner footer (P3) is a 10-line render on a slice that the aggregator already produces; gating it behind a follow-up release would be more PR overhead than the feature itself.
3. The spec's priority labels are guidance for *what would be MVP if we had to cut*, not what we ship in this PR. Nothing in the issue or the constitution suggests cutting.

**Alternatives considered**:

- *Ship P1 only; defer P2/P3*: Rejected per (1) above.
- *Ship P1+P2; defer P3*: Rejected per (2) above.

---

## Summary

All technical-context unknowns in plan.md are resolved. The implementation surface is:

1. Two new files under `internal/fleet/` — one source, one test, plus fixtures under `testdata/consumption/`.
2. One new file under `cmd/` — the cobra subcommand.
3. One-line edit to `cmd/root.go` — `AddCommand`.
4. Verification (not modification) of `internal/fleet/diagnostics.go:64` `billingQuotaHint` copy.
5. Three contracts (`consumption-output.json`, `consumption-text-output.md`, `discussion-discovery.md`, `run-artifact-payload.md`) documenting the boundaries.
6. Docs updates (`AGENTS.md` / `CLAUDE.md`).

No new third-party dependencies, no schema bumps, no caching, no live network in tests. Ready for Phase 1.
