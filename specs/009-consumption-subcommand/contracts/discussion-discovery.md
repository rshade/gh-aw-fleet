# Contract: Discussion-discovery layer

**Feature**: 009-consumption-subcommand
**Plan**: [../plan.md](../plan.md) — Decision 1 (REST vs. GraphQL) and Decision 3 (body-marker parsing) in [../research.md](../research.md).
**Producer**: `discoverReports(ctx, repo string) ([]reportRef, error)` in `internal/fleet/consumption.go`.

## API call

```bash
gh api --paginate \
  -H "Accept: application/vnd.github+json" \
  "repos/{owner}/{name}/discussions"
```

— invoked via the package-level test-injection seam:

```go
//nolint:gochecknoglobals // test-injection seam for gh api discussion list
var ghDiscussionsAPI = func(ctx context.Context, repo string) ([]discussionJSON, error) {
    // Production impl: exec.CommandContext(ctx, "gh", "api", "--paginate", path).Output()
    // Mirrors internal/fleet/fetch.go:183 (ghAPIJSON).
}
```

Production decodes the paginated JSON stream into `[]discussionJSON`. Tests substitute a fixture-returning closure.

## Response shape consumed (subset)

```go
type discussionJSON struct {
    Number   int    `json:"number"`
    Title    string `json:"title"`
    Body     string `json:"body"`
    HTMLURL  string `json:"html_url"`
    Category struct {
        Slug string `json:"slug"`
    } `json:"category"`
}
```

Fields not listed (e.g. `user`, `reactions`, `comments`) are present in the raw response but ignored by this code path.

## Filter (after fetch)

A discussion is a consumption-report candidate iff **both**:

1. `discussion.Category.Slug == "audits"`
2. `strings.Contains(discussion.Body, "<!-- gh-aw-tracker-id: api-consumption-report-daily -->")`

Discussions that fail either check are silently dropped. No diagnostic — the same query response will contain unrelated audits-category discussions (security audits, etc.) that legitimately don't carry the marker.

## Body-marker extraction (after filter)

For each candidate, parse via package-scope compiled regexps:

```go
//nolint:gochecknoglobals // anchored marker regexps compiled once at init
var (
    runIDRe   = regexp.MustCompile(`/actions/runs/(\d+)/agentic_workflow`)
    expiresRe = regexp.MustCompile(`<!-- gh-aw-expires:\s*([^\s]+)\s*-->`)
)
```

- **RunID**: `runIDRe.FindStringSubmatch(body)[1]` → parse with `strconv.ParseInt(_, 10, 64)`. If no match: emit a `DiagHint` warning `"Discussion #{n} on {repo} contains no actions/runs/{id}/agentic_workflow link — skipping"`, drop the report.
- **Date**: parse from `discussion.Title` via `time.Parse("2006-01-02", strings.TrimSpace(extractDateToken(title)))`. The title may carry a prefix like `Daily consumption — ` or a suffix like ` (in-progress)`; `extractDateToken` returns the first `YYYY-MM-DD`-shaped substring or `""`. If empty / unparseable: emit a `DiagHint` warning `"Discussion #{n} on {repo} title %q does not contain a YYYY-MM-DD date — skipping"`, drop the report.
- **Expires**: `expiresRe.FindStringSubmatch(body)[1]` → parse with `time.Parse(time.RFC3339, _)`. If no match or unparseable: emit a `DiagHint` warning `"Discussion #{n} on {repo} contains no <!-- gh-aw-expires: ISO --> marker — treating as expired (cannot determine validity)"`, set `Expires` to zero time (which makes `now.After(expires)` true → row is excluded as expired). Soft-failure rather than hard-drop because the discussion may still be informational.
- **InProgress**: `strings.Contains(body, "🔄 in-progress")`. No diagnostic on miss — absence is the common case.

## Output type

```go
type reportRef struct {
    Repo       string    // pass-through from input parameter
    RunID      int64     // from runIDRe
    Date       time.Time // from title
    Expires    time.Time // from expiresRe, or zero on parse failure
    InProgress bool      // from "🔄 in-progress" substring
    URL        string    // discussion.HTMLURL — used in diagnostic copy
}
```

Returned: one `reportRef` per surviving discussion. The slice is sorted by `Date` descending — newest first — so `fetchLatest` mode can take `refs[0]` without re-sorting.

## Error handling

| Condition | Behavior |
|---|---|
| `gh` not installed / `gh api` exits non-zero with stderr `"Not Found"` | Returns `nil, err` wrapping `DiagHTTP404`. The caller emits a warning and continues to the next repo. |
| `gh api` returns valid JSON but the discussion array is empty | Returns `[]reportRef{}, nil`. The caller emits a `DiagHint` warning per FR-010 ("No consumption reports discovered for {repo}"). |
| Network unreachable | Returns `nil, err` wrapping `DiagNetworkUnreachable`. Caller emits the warning; the rollup continues with whichever repos succeeded. |
| Discussions disabled on the repo | `gh api` returns HTTP 410 / 404 with a body indicating discussions are disabled. Wrapped as `DiagHTTP404` per the existing hint table. |
| The repo has the workflow but it has never finished a run (no daily report ever published) | Returns `[]reportRef{}, nil`. Same as the empty-array case. |

## Mockability for tests

The injection seam `ghDiscussionsAPI` is set in production to the `exec.CommandContext` invocation. Tests:

```go
func TestDiscoverReports_HappyPath(t *testing.T) {
    body := testdata.MustRead(t, "consumption/discussion_valid.json")
    var payload []discussionJSON
    must.Unmarshal(t, body, &payload)

    old := ghDiscussionsAPI
    t.Cleanup(func() { ghDiscussionsAPI = old })
    ghDiscussionsAPI = func(ctx context.Context, repo string) ([]discussionJSON, error) {
        return payload, nil
    }
    // ... assert discoverReports("rshade/example") returns the expected refs
}
```

Fixtures under `internal/fleet/testdata/consumption/`:

- `discussion_valid.json` — full body with all four markers present
- `discussion_in_progress.json` — body contains `🔄 in-progress`
- `discussion_expired.json` — `<!-- gh-aw-expires: 2020-01-01T00:00:00Z -->` (a past date)
- `discussion_malformed.json` — missing the run-ID link; exercises soft-failure diagnostic
- `discussion_wrong_category.json` — category.slug is not "audits"; should be filtered out
- `discussion_no_tracker.json` — body lacks the tracker marker; should be filtered out
