package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// T008 — ParseTrailing + normalizeCost (helpers)
// ---------------------------------------------------------------------------

func TestParseTrailing(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"7d", 7, false},
		{"30d", 30, false},
		{"1d", 1, false},
		{"", 0, true},
		{"7", 0, true},
		{"7h", 0, true},
		{"7d ", 0, true},
		{" 7d", 0, true},
		{"0d", 0, true},
		{"-1d", 0, true},
		{"abc", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseTrailing(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseTrailing(%q): want error, got %d", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTrailing(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseTrailing(%q) = %d; want %d", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeCost(t *testing.T) {
	v := func(f float64) *float64 { return &f }
	cases := []struct {
		in   *float64
		want *float64
	}{
		{nil, nil},
		{v(0), nil},
		{v(-1.5), nil},
		{v(12.45), v(12.45)},
		{v(0.01), v(0.01)},
	}
	for _, tc := range cases {
		got := normalizeCost(tc.in)
		if tc.want == nil {
			if got != nil {
				t.Errorf("normalizeCost(%v) = %v; want nil", tc.in, *got)
			}
			continue
		}
		if got == nil {
			t.Errorf("normalizeCost(%v): got nil, want %v", *tc.in, *tc.want)
			continue
		}
		if *got != *tc.want {
			t.Errorf("normalizeCost(%v) = %v; want %v", *tc.in, *got, *tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// T009 — discoverReports (discussion fixtures)
// ---------------------------------------------------------------------------

func mustReadDiscussion(t *testing.T, name string) discussionJSON {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "consumption", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var d discussionJSON
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return d
}

func withDiscussionsStub(t *testing.T, stub func(ctx context.Context, repo string) ([]discussionJSON, error)) {
	t.Helper()
	prev := ghDiscussionsAPI
	t.Cleanup(func() { ghDiscussionsAPI = prev })
	ghDiscussionsAPI = stub
}

func TestDiscoverReports(t *testing.T) {
	valid := mustReadDiscussion(t, "discussion_valid.json")
	inProg := mustReadDiscussion(t, "discussion_in_progress.json")
	expired := mustReadDiscussion(t, "discussion_expired.json")
	malformed := mustReadDiscussion(t, "discussion_malformed.json")
	wrongCat := mustReadDiscussion(t, "discussion_wrong_category.json")
	noTracker := mustReadDiscussion(t, "discussion_no_tracker.json")

	cases := []struct {
		name             string
		payload          []discussionJSON
		wantRefs         int
		wantDiags        int
		wantInProgress   bool
		wantExpiredEarly bool
	}{
		{"valid-only", []discussionJSON{valid}, 1, 0, false, false},
		{"in-progress-only", []discussionJSON{inProg}, 1, 0, true, false},
		{"expired-only", []discussionJSON{expired}, 1, 0, false, true},
		{"malformed-only", []discussionJSON{malformed}, 0, 1, false, false},
		{"wrong-category-filtered", []discussionJSON{wrongCat}, 0, 0, false, false},
		{"no-tracker-filtered", []discussionJSON{noTracker}, 0, 0, false, false},
		{"mixed-sorted", []discussionJSON{expired, valid, inProg}, 3, 0, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withDiscussionsStub(t, func(_ context.Context, _ string) ([]discussionJSON, error) {
				return tc.payload, nil
			})
			refs, diags, err := discoverReports(context.Background(), "rshade/example")
			if err != nil {
				t.Fatalf("discoverReports: %v", err)
			}
			if len(refs) != tc.wantRefs {
				t.Fatalf("len(refs) = %d; want %d (refs=%+v)", len(refs), tc.wantRefs, refs)
			}
			if len(diags) != tc.wantDiags {
				t.Fatalf("len(diags) = %d; want %d (diags=%+v)", len(diags), tc.wantDiags, diags)
			}
			assertFirstRefShape(t, refs, tc.wantInProgress, tc.wantExpiredEarly)
			// Sort: descending by Date.
			for i := 1; i < len(refs); i++ {
				if refs[i].Date.After(refs[i-1].Date) {
					t.Errorf("refs not sorted descending by Date at index %d: %v then %v",
						i, refs[i-1].Date, refs[i].Date)
				}
			}
		})
	}
}

// assertFirstRefShape checks the InProgress + Expires expectations on the
// first ref when at least one ref survived the filter; no-ops otherwise.
func assertFirstRefShape(t *testing.T, refs []reportRef, wantInProgress, wantExpiredEarly bool) {
	t.Helper()
	if len(refs) == 0 {
		return
	}
	first := refs[0]
	if wantInProgress && !first.InProgress {
		t.Errorf("first ref InProgress = false; want true")
	}
	if wantExpiredEarly && first.Expires.IsZero() {
		t.Errorf("first ref Expires is zero; want a past date")
	}
	if wantExpiredEarly && first.Expires.After(time.Now()) {
		t.Errorf("first ref Expires = %v; want a past date", first.Expires)
	}
}

// ---------------------------------------------------------------------------
// T010 — fetchRunArtifacts (artifact fixtures)
// ---------------------------------------------------------------------------

func mustReadAWInfo(t *testing.T, name string) awInfoPayload {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "consumption", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var p awInfoPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return p
}

func mustReadRunSummary(t *testing.T, name string) runSummaryPayload {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "consumption", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var p runSummaryPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return p
}

func withArtifactStub(t *testing.T, awInfo awInfoPayload, run runSummaryPayload) {
	t.Helper()
	prev := ghRunArtifactAPI
	t.Cleanup(func() { ghRunArtifactAPI = prev })
	ghRunArtifactAPI = func(_ context.Context, _ string, _ int64) (artifactPayload, error) {
		return artifactPayload{AWInfo: awInfo, RunSummary: run}, nil
	}
}

func TestFetchRunArtifacts(t *testing.T) {
	ref := reportRef{Repo: "rshade/example", RunID: 1234, Date: time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)}

	t.Run("cost-present", func(t *testing.T) {
		withArtifactStub(t, mustReadAWInfo(t, "aw_info_cost_present.json"), mustReadRunSummary(t, "run_summary.json"))
		rep, err := fetchRunArtifacts(context.Background(), ref)
		if err != nil {
			t.Fatalf("fetchRunArtifacts: %v", err)
		}
		if rep.Cost == nil {
			t.Fatalf("Cost is nil; want 12.45")
		}
		if *rep.Cost != 12.45 {
			t.Errorf("Cost = %v; want 12.45", *rep.Cost)
		}
		if rep.GitHubAPICalls != 4827 {
			t.Errorf("GitHubAPICalls = %d; want 4827", rep.GitHubAPICalls)
		}
		if len(rep.PerWorkflow) != 3 {
			t.Errorf("len(PerWorkflow) = %d; want 3", len(rep.PerWorkflow))
		}
	})
	t.Run("cost-absent", func(t *testing.T) {
		withArtifactStub(t, mustReadAWInfo(t, "aw_info_cost_absent.json"), mustReadRunSummary(t, "run_summary.json"))
		rep, err := fetchRunArtifacts(context.Background(), ref)
		if err != nil {
			t.Fatalf("fetchRunArtifacts: %v", err)
		}
		if rep.Cost != nil {
			t.Errorf("Cost = %v; want nil", *rep.Cost)
		}
	})
	t.Run("cost-zero-becomes-nil", func(t *testing.T) {
		withArtifactStub(t, mustReadAWInfo(t, "aw_info_cost_zero.json"), mustReadRunSummary(t, "run_summary.json"))
		rep, err := fetchRunArtifacts(context.Background(), ref)
		if err != nil {
			t.Fatalf("fetchRunArtifacts: %v", err)
		}
		if rep.Cost != nil {
			t.Errorf("Cost = %v; want nil (Decision 6)", *rep.Cost)
		}
	})
	t.Run("cost-negative-becomes-nil", func(t *testing.T) {
		withArtifactStub(t, mustReadAWInfo(t, "aw_info_cost_negative.json"), mustReadRunSummary(t, "run_summary.json"))
		rep, err := fetchRunArtifacts(context.Background(), ref)
		if err != nil {
			t.Fatalf("fetchRunArtifacts: %v", err)
		}
		if rep.Cost != nil {
			t.Errorf("Cost = %v; want nil (Decision 6)", *rep.Cost)
		}
	})
	t.Run("run-summary-empty", func(t *testing.T) {
		withArtifactStub(t,
			mustReadAWInfo(t, "aw_info_cost_present.json"),
			mustReadRunSummary(t, "run_summary_empty.json"))
		rep, err := fetchRunArtifacts(context.Background(), ref)
		if err != nil {
			t.Fatalf("fetchRunArtifacts: %v", err)
		}
		if len(rep.PerWorkflow) != 0 {
			t.Errorf("len(PerWorkflow) = %d; want 0", len(rep.PerWorkflow))
		}
	})
}

// ---------------------------------------------------------------------------
// Test helpers — fake refs + multi-repo stubs for aggregation tests
// ---------------------------------------------------------------------------

// stubFleet wires ghDiscussionsAPI to return a synthetic reportRef per repo
// and ghRunArtifactAPI to return a synthetic artifactPayload per (repo,run).
// Refs and payloads come from callback closures so each test can shape its
// own fleet.
func stubFleet(
	t *testing.T,
	discussions func(repo string) []discussionJSON,
	artifacts func(repo string, runID int64) artifactPayload,
) {
	t.Helper()
	prevD := ghDiscussionsAPI
	prevA := ghRunArtifactAPI
	t.Cleanup(func() {
		ghDiscussionsAPI = prevD
		ghRunArtifactAPI = prevA
	})
	ghDiscussionsAPI = func(_ context.Context, repo string) ([]discussionJSON, error) {
		return discussions(repo), nil
	}
	ghRunArtifactAPI = func(_ context.Context, repo string, runID int64) (artifactPayload, error) {
		return artifacts(repo, runID), nil
	}
}

// synthDiscussion's fixed shape feeds the aggregation tests. The in-progress
// + window-filter logic is exercised directly elsewhere via reportRef literals
// (TestShouldIncludeReport_*), so this helper hard-codes the happy-path
// markers: audits category, tracker marker present, run-ID link, future
// expires, no in-progress flag.
const (
	synthDiscussionDate  = "2026-05-13"
	synthDiscussionRunID = 1000
)

func synthDiscussion() discussionJSON {
	const number = synthDiscussionRunID
	body := "Run: https://github.com/example/repo/actions/runs/" +
		strconv.Itoa(synthDiscussionRunID) + "/agentic_workflow\n" +
		consumptionTrackerMarker + "\n" +
		"<!-- gh-aw-expires: 2099-01-01T00:00:00Z -->\n"
	return discussionJSON{
		Number:  number,
		Title:   "Daily consumption — " + synthDiscussionDate,
		Body:    body,
		HTMLURL: "https://github.com/example/repo/discussions/" + strconv.Itoa(number),
		Category: struct {
			Slug string `json:"slug"`
		}{Slug: "audits"},
	}
}

// synthArtifact builds an artifactPayload with the given totals + per-workflow rows.
type synthWorkflow struct {
	Name        string
	Runs        int
	APICalls    int
	AvgDuration float64
	Cost        *float64
}

func synthArtifact(coreConsumed, safeOutputs int, cost *float64, workflows []synthWorkflow) artifactPayload {
	p := artifactPayload{}
	p.AWInfo.GithubRateLimitUsage.CoreConsumed = coreConsumed
	p.AWInfo.SafeOutputs.TotalCalls = safeOutputs
	p.AWInfo.Cost = cost
	for _, w := range workflows {
		p.RunSummary.Workflows = append(p.RunSummary.Workflows, struct {
			Name               string   `json:"name"`
			Runs               int      `json:"runs"`
			APICalls           int      `json:"api_calls"`
			AvgDurationSeconds float64  `json:"avg_duration_seconds"`
			Cost               *float64 `json:"cost,omitempty"`
		}{Name: w.Name, Runs: w.Runs, APICalls: w.APICalls, AvgDurationSeconds: w.AvgDuration, Cost: w.Cost})
	}
	return p
}

func fptr(f float64) *float64 { return &f }

// ---------------------------------------------------------------------------
// T013 — shouldIncludeReport, FetchLatest arm
// ---------------------------------------------------------------------------

func TestShouldIncludeReport_FetchLatest(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	mode := FetchMode{Kind: FetchLatest}
	future := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		ref         reportRef
		wantInclude bool
		wantDiag    bool
	}{
		{"valid", reportRef{Repo: "o/r", Date: now, Expires: future, InProgress: false}, true, false},
		{"in-progress", reportRef{Repo: "o/r", Date: now, Expires: future, InProgress: true}, false, true},
		{"expired", reportRef{Repo: "o/r", Date: now, Expires: past, InProgress: false}, false, false},
		{"expired-and-in-progress", reportRef{Repo: "o/r", Date: now, Expires: past, InProgress: true}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, diag := shouldIncludeReport(tc.ref, mode, now)
			if got != tc.wantInclude {
				t.Errorf("include = %v; want %v", got, tc.wantInclude)
			}
			if (diag != nil) != tc.wantDiag {
				t.Errorf("diag presence = %v; want %v (diag=%+v)", diag != nil, tc.wantDiag, diag)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T014 — AggregateConsumption, by=repo
// ---------------------------------------------------------------------------

func TestAggregateConsumption_GroupByRepo(t *testing.T) {
	cfg := &Config{
		LoadedFrom: "fleet.local.json",
		Profiles: map[string]Profile{
			"default": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
		},
		Repos: map[string]RepoSpec{
			"a/one":   {Profiles: []string{"default"}},
			"b/two":   {Profiles: []string{"default"}},
			"c/three": {Profiles: []string{"default"}},
		},
	}
	stubFleet(t,
		func(repo string) []discussionJSON {
			return []discussionJSON{synthDiscussion()}
		},
		func(repo string, _ int64) artifactPayload {
			switch repo {
			case "a/one":
				return synthArtifact(100, 5, fptr(1.0), []synthWorkflow{
					{Name: "wf-a", Runs: 1, APICalls: 100, AvgDuration: 10, Cost: fptr(1.0)},
				})
			case "b/two":
				return synthArtifact(200, 10, fptr(2.0), []synthWorkflow{
					{Name: "wf-b", Runs: 2, APICalls: 200, AvgDuration: 12, Cost: fptr(2.0)},
				})
			case "c/three":
				return synthArtifact(300, 15, nil, []synthWorkflow{
					{Name: "wf-c", Runs: 3, APICalls: 300, AvgDuration: 14},
				})
			}
			return artifactPayload{}
		},
	)
	res, _, err := AggregateConsumption(
		context.Background(),
		cfg,
		FetchMode{Kind: FetchLatest},
		GroupByRepo,
		SourceArtifacts,
	)
	if err != nil {
		t.Fatalf("AggregateConsumption: %v", err)
	}
	if len(res.Groups) != 3 {
		t.Fatalf("len(Groups) = %d; want 3", len(res.Groups))
	}
	// Sorted ascending by Key.
	if !(res.Groups[0].Key == "a/one" && res.Groups[1].Key == "b/two" && res.Groups[2].Key == "c/three") {
		t.Errorf("not sorted by key: %v %v %v", res.Groups[0].Key, res.Groups[1].Key, res.Groups[2].Key)
	}
	if res.Groups[0].GitHubAPICalls != 100 || res.Groups[0].SafeOutputCalls != 5 {
		t.Errorf("a/one totals = %d/%d; want 100/5", res.Groups[0].GitHubAPICalls, res.Groups[0].SafeOutputCalls)
	}
	if res.Groups[2].Cost != nil {
		t.Errorf("c/three Cost = %v; want nil (no upstream cost)", *res.Groups[2].Cost)
	}
	if res.Groups[0].ReportCount != 1 {
		t.Errorf("a/one ReportCount = %d; want 1", res.Groups[0].ReportCount)
	}
	if res.FetchMode != "latest" {
		t.Errorf("FetchMode = %q; want \"latest\"", res.FetchMode)
	}
	if res.GroupBy != "repo" {
		t.Errorf("GroupBy = %q; want \"repo\"", res.GroupBy)
	}
}

// ---------------------------------------------------------------------------
// T015 — JSON envelope shape
// ---------------------------------------------------------------------------

func TestConsumptionResult_JSONMarshal(t *testing.T) {
	res := &ConsumptionResult{
		LoadedFrom: "fleet.local.json",
		FetchMode:  "latest",
		GroupBy:    "repo",
		Groups: []ConsumptionGroup{
			{Key: "a/one", GitHubAPICalls: 100, SafeOutputCalls: 5, Cost: fptr(1.5), ReportCount: 1},
			{Key: "b/two", GitHubAPICalls: 200, SafeOutputCalls: 10, Cost: nil, ReportCount: 1},
		},
		TopBurners: []WorkflowConsumption{
			{Workflow: "wf", Runs: 5, APICalls: 200, AvgDurationS: 12.5, Cost: fptr(3.0)},
		},
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"loaded_from":"fleet.local.json"`,
		`"fetch_mode":"latest"`,
		`"group_by":"repo"`,
		`"groups":[`,
		`"top_burners":[`,
		`"cost":1.5`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q: %s", want, s)
		}
	}
	// Second group has nil cost — must be omitted entirely.
	idx := strings.Index(s, `"key":"b/two"`)
	if idx < 0 {
		t.Fatalf("did not find b/two row: %s", s)
	}
	rest := s[idx:]
	endIdx := strings.Index(rest, `}`)
	if endIdx < 0 {
		t.Fatalf("could not isolate b/two object: %s", rest)
	}
	row := rest[:endIdx]
	if strings.Contains(row, `"cost"`) {
		t.Errorf("b/two row contains \"cost\" key; want omitted (Decision 6): %s", row)
	}
}

// ---------------------------------------------------------------------------
// T020 — shouldIncludeReport, FetchTrailing arm
// ---------------------------------------------------------------------------

func TestShouldIncludeReport_FetchTrailing(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	mode := FetchMode{Kind: FetchTrailing, Days: 7}
	future := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	inWindow := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	outOfWindow := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		ref         reportRef
		wantInclude bool
		wantDiag    bool
	}{
		{"in-window-valid", reportRef{Date: inWindow, Expires: future, InProgress: false}, true, false},
		{"in-window-in-progress-warns", reportRef{Date: inWindow, Expires: future, InProgress: true}, true, true},
		{"in-window-expired", reportRef{Date: inWindow, Expires: past, InProgress: false}, false, false},
		{"outside-window", reportRef{Date: outOfWindow, Expires: future, InProgress: false}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, diag := shouldIncludeReport(tc.ref, mode, now)
			if got != tc.wantInclude {
				t.Errorf("include = %v; want %v", got, tc.wantInclude)
			}
			if (diag != nil) != tc.wantDiag {
				t.Errorf("diag presence = %v; want %v", diag != nil, tc.wantDiag)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T021 — shouldIncludeReport, FetchSince arm
// ---------------------------------------------------------------------------

func TestShouldIncludeReport_FetchSince(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	since := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	mode := FetchMode{Kind: FetchSince, Since: since}
	future := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	afterSince := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	beforeSince := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		ref         reportRef
		wantInclude bool
		wantDiag    bool
	}{
		{"after-since-valid", reportRef{Date: afterSince, Expires: future, InProgress: false}, true, false},
		{"after-since-in-progress-warns", reportRef{Date: afterSince, Expires: future, InProgress: true}, true, true},
		{"after-since-expired", reportRef{Date: afterSince, Expires: past, InProgress: false}, false, false},
		{"before-since", reportRef{Date: beforeSince, Expires: future, InProgress: false}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, diag := shouldIncludeReport(tc.ref, mode, now)
			if got != tc.wantInclude {
				t.Errorf("include = %v; want %v", got, tc.wantInclude)
			}
			if (diag != nil) != tc.wantDiag {
				t.Errorf("diag presence = %v; want %v", diag != nil, tc.wantDiag)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T027 — by profile (additive double-counting)
// ---------------------------------------------------------------------------

func TestAggregateConsumption_GroupByProfile(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]Profile{
			"standard": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
			"premium":  {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
		},
		Repos: map[string]RepoSpec{
			"a/std-only":  {Profiles: []string{"standard"}},
			"b/prem-only": {Profiles: []string{"premium"}},
			"c/both":      {Profiles: []string{"standard", "premium"}},
		},
	}
	stubFleet(t,
		func(_ string) []discussionJSON {
			return []discussionJSON{synthDiscussion()}
		},
		func(_ string, _ int64) artifactPayload {
			return synthArtifact(100, 5, fptr(1.0), []synthWorkflow{
				{Name: "wf", Runs: 1, APICalls: 100, AvgDuration: 10, Cost: fptr(1.0)},
			})
		},
	)
	res, _, err := AggregateConsumption(
		context.Background(),
		cfg,
		FetchMode{Kind: FetchLatest},
		GroupByProfile,
		SourceArtifacts,
	)
	if err != nil {
		t.Fatalf("AggregateConsumption: %v", err)
	}
	if len(res.Groups) != 2 {
		t.Fatalf("len(Groups) = %d; want 2", len(res.Groups))
	}
	byKey := map[string]ConsumptionGroup{}
	for _, g := range res.Groups {
		byKey[g.Key] = g
	}
	// Each profile collects 2 repos (std → a + c, premium → b + c).
	if byKey["standard"].ReportCount != 2 {
		t.Errorf("standard ReportCount = %d; want 2 (a/std-only + c/both)", byKey["standard"].ReportCount)
	}
	if byKey["premium"].ReportCount != 2 {
		t.Errorf("premium ReportCount = %d; want 2 (b/prem-only + c/both)", byKey["premium"].ReportCount)
	}
	if byKey["standard"].GitHubAPICalls != 200 {
		t.Errorf("standard api calls = %d; want 200", byKey["standard"].GitHubAPICalls)
	}
}

// ---------------------------------------------------------------------------
// T028 — by cost-center (<unset> bucket)
// ---------------------------------------------------------------------------

func TestAggregateConsumption_GroupByCostCenter(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]Profile{
			"p": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
		},
		Repos: map[string]RepoSpec{
			"a/platform1": {Profiles: []string{"p"}, CostCenter: "platform-eng"},
			"b/platform2": {Profiles: []string{"p"}, CostCenter: "platform-eng"},
			"c/data":      {Profiles: []string{"p"}, CostCenter: "data-platform"},
			"d/unset":     {Profiles: []string{"p"}},
		},
	}
	stubFleet(t,
		func(_ string) []discussionJSON {
			return []discussionJSON{synthDiscussion()}
		},
		func(_ string, _ int64) artifactPayload {
			return synthArtifact(100, 5, nil, []synthWorkflow{{Name: "wf", Runs: 1, APICalls: 100}})
		},
	)
	res, _, err := AggregateConsumption(
		context.Background(),
		cfg,
		FetchMode{Kind: FetchLatest},
		GroupByCostCenter,
		SourceArtifacts,
	)
	if err != nil {
		t.Fatalf("AggregateConsumption: %v", err)
	}
	if len(res.Groups) != 3 {
		t.Fatalf("len(Groups) = %d; want 3", len(res.Groups))
	}
	byKey := map[string]ConsumptionGroup{}
	for _, g := range res.Groups {
		byKey[g.Key] = g
	}
	if byKey["platform-eng"].ReportCount != 2 {
		t.Errorf("platform-eng ReportCount = %d; want 2", byKey["platform-eng"].ReportCount)
	}
	if byKey["data-platform"].ReportCount != 1 {
		t.Errorf("data-platform ReportCount = %d; want 1", byKey["data-platform"].ReportCount)
	}
	if byKey[unsetCostCenter].ReportCount != 1 {
		t.Errorf("<unset> ReportCount = %d; want 1", byKey[unsetCostCenter].ReportCount)
	}
}

// ---------------------------------------------------------------------------
// T029 — by workflow
// ---------------------------------------------------------------------------

func TestAggregateConsumption_GroupByWorkflow(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]Profile{
			"p": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
		},
		Repos: map[string]RepoSpec{
			"a/r1": {Profiles: []string{"p"}},
			"b/r2": {Profiles: []string{"p"}},
			"c/r3": {Profiles: []string{"p"}},
		},
	}
	stubFleet(t,
		func(_ string) []discussionJSON {
			return []discussionJSON{synthDiscussion()}
		},
		func(repo string, _ int64) artifactPayload {
			workflows := []synthWorkflow{
				{Name: "alpha", Runs: 1, APICalls: 100, Cost: fptr(1.0)},
				{Name: "beta", Runs: 2, APICalls: 200, Cost: fptr(2.0)},
				{Name: "gamma", Runs: 3, APICalls: 300},
			}
			return synthArtifact(600, 30, fptr(3.0), workflows)
		},
	)
	res, _, err := AggregateConsumption(
		context.Background(),
		cfg,
		FetchMode{Kind: FetchLatest},
		GroupByWorkflow,
		SourceArtifacts,
	)
	if err != nil {
		t.Fatalf("AggregateConsumption: %v", err)
	}
	if len(res.Groups) != 3 {
		t.Fatalf("len(Groups) = %d; want 3", len(res.Groups))
	}
	if !(res.Groups[0].Key == "alpha" && res.Groups[1].Key == "beta" && res.Groups[2].Key == "gamma") {
		t.Errorf("workflow groups not sorted alphabetically: %v %v %v",
			res.Groups[0].Key, res.Groups[1].Key, res.Groups[2].Key)
	}
	if res.Groups[0].GitHubAPICalls != 300 {
		t.Errorf("alpha api calls = %d; want 300 (100×3 repos)", res.Groups[0].GitHubAPICalls)
	}
	if res.Groups[2].Cost != nil {
		t.Errorf("gamma cost = %v; want nil (no upstream cost on the workflow)", *res.Groups[2].Cost)
	}
	for _, g := range res.Groups {
		if g.SafeOutputCalls != 0 {
			t.Errorf("workflow group %q SafeOutputCalls = %d; want 0 (no honest per-workflow attribution exists)",
				g.Key, g.SafeOutputCalls)
		}
	}
}

// ---------------------------------------------------------------------------
// T036 — top burners, fifteen workflows → cap at ten
// ---------------------------------------------------------------------------

func TestAggregateConsumption_TopBurnersFullTen(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]Profile{
			"p": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
		},
		Repos: map[string]RepoSpec{
			"a/r": {Profiles: []string{"p"}},
		},
	}
	stubFleet(t,
		func(_ string) []discussionJSON {
			return []discussionJSON{synthDiscussion()}
		},
		func(_ string, _ int64) artifactPayload {
			workflows := make([]synthWorkflow, 15)
			for i := range workflows {
				workflows[i] = synthWorkflow{
					Name:     fmt.Sprintf("wf-%02d", i),
					Runs:     i + 1,
					APICalls: 1000 * (i + 1),
				}
			}
			return synthArtifact(120000, 50, nil, workflows)
		},
	)
	res, _, err := AggregateConsumption(
		context.Background(),
		cfg,
		FetchMode{Kind: FetchLatest},
		GroupByRepo,
		SourceArtifacts,
	)
	if err != nil {
		t.Fatalf("AggregateConsumption: %v", err)
	}
	if len(res.TopBurners) != 10 {
		t.Fatalf("len(TopBurners) = %d; want 10", len(res.TopBurners))
	}
	if res.TopBurners[0].Workflow != "wf-14" {
		t.Errorf("first top burner = %q; want wf-14 (highest APICalls)", res.TopBurners[0].Workflow)
	}
	for i := 1; i < len(res.TopBurners); i++ {
		if res.TopBurners[i].APICalls > res.TopBurners[i-1].APICalls {
			t.Errorf("top burners not descending by APICalls at index %d: %d > %d",
				i, res.TopBurners[i].APICalls, res.TopBurners[i-1].APICalls)
		}
	}
}

// ---------------------------------------------------------------------------
// T037 — top burners, fewer than ten distinct workflows
// ---------------------------------------------------------------------------

func TestAggregateConsumption_TopBurnersFewer(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]Profile{
			"p": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
		},
		Repos: map[string]RepoSpec{
			"a/r": {Profiles: []string{"p"}},
		},
	}
	stubFleet(t,
		func(_ string) []discussionJSON {
			return []discussionJSON{synthDiscussion()}
		},
		func(_ string, _ int64) artifactPayload {
			return synthArtifact(500, 10, nil, []synthWorkflow{
				{Name: "alpha", Runs: 1, APICalls: 100},
				{Name: "beta", Runs: 2, APICalls: 200},
				{Name: "gamma", Runs: 3, APICalls: 300},
			})
		},
	)
	res, _, err := AggregateConsumption(
		context.Background(),
		cfg,
		FetchMode{Kind: FetchLatest},
		GroupByRepo,
		SourceArtifacts,
	)
	if err != nil {
		t.Fatalf("AggregateConsumption: %v", err)
	}
	if len(res.TopBurners) != 3 {
		t.Errorf("len(TopBurners) = %d; want 3 (no padding to 10)", len(res.TopBurners))
	}
}

// ---------------------------------------------------------------------------
// selectArtifactIDs — real gh-aw artifact names (activation / aw-info)
// ---------------------------------------------------------------------------

func TestSelectArtifactIDs(t *testing.T) {
	tests := []struct {
		name        string
		artifacts   []artifactRef
		wantAWInfo  int64
		wantRunSumm int64
	}{
		{
			name: "v5 activation + run_summary",
			artifacts: []artifactRef{
				{ID: 11, Name: "agent"},
				{ID: 12, Name: "activation"},
				{ID: 13, Name: "run_summary"},
			},
			wantAWInfo:  12,
			wantRunSumm: 13,
		},
		{
			name:        "legacy aw-info (dash)",
			artifacts:   []artifactRef{{ID: 20, Name: "aw-info"}},
			wantAWInfo:  20,
			wantRunSumm: 0,
		},
		{
			name: "activation wins over aw-info when both present",
			artifacts: []artifactRef{
				{ID: 31, Name: "aw-info"},
				{ID: 32, Name: "activation"},
			},
			wantAWInfo:  32,
			wantRunSumm: 0,
		},
		{
			name:        "underscore fallback still matched",
			artifacts:   []artifactRef{{ID: 40, Name: "aw_info"}},
			wantAWInfo:  40,
			wantRunSumm: 0,
		},
		{
			name: "no aw_info artifact present",
			artifacts: []artifactRef{
				{ID: 50, Name: "agent"},
				{ID: 51, Name: "detection"},
			},
			wantAWInfo:  0,
			wantRunSumm: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotAW, gotRS := selectArtifactIDs(tc.artifacts)
			if gotAW != tc.wantAWInfo {
				t.Errorf("awInfoID = %d; want %d", gotAW, tc.wantAWInfo)
			}
			if gotRS != tc.wantRunSumm {
				t.Errorf("runSummaryID = %d; want %d", gotRS, tc.wantRunSumm)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// nil-cost hint — Copilot AI-Credits model emits no USD cost
// ---------------------------------------------------------------------------

func hasNilCostHint(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Code == DiagHint && strings.Contains(d.Message, "Cost is unavailable for every report") {
			return true
		}
	}
	return false
}

func TestAggregateConsumption_NilCostHint(t *testing.T) {
	cfg := &Config{
		LoadedFrom: "fleet.local.json",
		Profiles:   map[string]Profile{"default": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}}},
		Repos:      map[string]RepoSpec{"a/one": {Profiles: []string{"default"}}},
	}

	t.Run("all-nil cost emits the hint", func(t *testing.T) {
		stubFleet(t,
			func(_ string) []discussionJSON { return []discussionJSON{synthDiscussion()} },
			func(_ string, _ int64) artifactPayload {
				return synthArtifact(100, 5, nil, []synthWorkflow{
					{Name: "wf", Runs: 1, APICalls: 100, AvgDuration: 9},
				})
			},
		)
		_, diags, err := AggregateConsumption(
			context.Background(),
			cfg,
			FetchMode{Kind: FetchLatest},
			GroupByRepo,
			SourceArtifacts,
		)
		if err != nil {
			t.Fatalf("AggregateConsumption: %v", err)
		}
		if !hasNilCostHint(diags) {
			t.Errorf("expected nil-cost hint when every group has nil cost; diags=%v", diags)
		}
	})

	t.Run("present cost suppresses the hint", func(t *testing.T) {
		stubFleet(t,
			func(_ string) []discussionJSON { return []discussionJSON{synthDiscussion()} },
			func(_ string, _ int64) artifactPayload {
				return synthArtifact(100, 5, fptr(1.25), []synthWorkflow{
					{Name: "wf", Runs: 1, APICalls: 100, AvgDuration: 9, Cost: fptr(1.25)},
				})
			},
		)
		_, diags, err := AggregateConsumption(
			context.Background(),
			cfg,
			FetchMode{Kind: FetchLatest},
			GroupByRepo,
			SourceArtifacts,
		)
		if err != nil {
			t.Fatalf("AggregateConsumption: %v", err)
		}
		if hasNilCostHint(diags) {
			t.Errorf("did not expect nil-cost hint when a group has positive cost; diags=%v", diags)
		}
	})

	t.Run("no reports does not emit the hint", func(t *testing.T) {
		stubFleet(t,
			func(_ string) []discussionJSON { return nil },
			func(_ string, _ int64) artifactPayload { return artifactPayload{} },
		)
		_, diags, err := AggregateConsumption(
			context.Background(),
			cfg,
			FetchMode{Kind: FetchLatest},
			GroupByRepo,
			SourceArtifacts,
		)
		if err != nil {
			t.Fatalf("AggregateConsumption: %v", err)
		}
		if hasNilCostHint(diags) {
			t.Errorf("did not expect nil-cost hint when no reports were discovered; diags=%v", diags)
		}
	})
}

// ---------------------------------------------------------------------------
// Logs source (#103) — gh aw logs --json AIC rollup
// ---------------------------------------------------------------------------

func TestParseSource(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    SourceKind
		wantErr bool
	}{
		{"logs", SourceLogs, false},
		{"artifacts", SourceArtifacts, false},
		{"bogus", 0, true},
	} {
		got, err := ParseSource(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseSource(%q): want error", tc.in)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParseSource(%q) = %v, %v", tc.in, got, err)
		}
	}
	if SourceLogs.String() != "logs" || SourceArtifacts.String() != "artifacts" {
		t.Errorf("String() mismatch: %q %q", SourceLogs.String(), SourceArtifacts.String())
	}
}

func TestLogsWindowArgs(t *testing.T) {
	since := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		mode FetchMode
		want string
	}{
		{"latest", FetchMode{Kind: FetchLatest}, "-c 5"},
		{"trailing", FetchMode{Kind: FetchTrailing, Days: 7}, "--start-date -7d -c 1000"},
		{"since", FetchMode{Kind: FetchSince, Since: since}, "--start-date 2026-04-01 -c 1000"},
	}
	for _, tc := range cases {
		got := strings.Join(logsWindowArgs(tc.mode), " ")
		if got != tc.want {
			t.Errorf("%s: logsWindowArgs = %q; want %q", tc.name, got, tc.want)
		}
	}
}

func mustReadLogsPayload(t *testing.T, name string) logsPayload {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "logs", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var p logsPayload
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return p
}

func withLogsStubs(
	t *testing.T,
	workflows func(repo string) ([]workflowRef, error),
	logs func(repo, workflow string, mode FetchMode) (logsPayload, error),
) {
	t.Helper()
	prevW, prevL, prevV := ghWorkflowsAPI, ghLogsAPI, ghAwVersion
	t.Cleanup(func() { ghWorkflowsAPI, ghLogsAPI, ghAwVersion = prevW, prevL, prevV })
	ghWorkflowsAPI = func(_ context.Context, repo string) ([]workflowRef, error) { return workflows(repo) }
	ghLogsAPI = func(_ context.Context, repo, workflow string, mode FetchMode) (logsPayload, error) {
		return logs(repo, workflow, mode)
	}
	ghAwVersion = func(_ context.Context) (string, error) { return logsSourceMinVersion, nil }
}

func hasNilAICHint(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Code == DiagHint && strings.Contains(d.Message, "No AI-credit data found") {
			return true
		}
	}
	return false
}

func TestAggregateConsumption_LogsSource(t *testing.T) {
	cfg := &Config{
		LoadedFrom: "fleet.local.json",
		Profiles:   map[string]Profile{"default": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}}},
		Repos:      map[string]RepoSpec{"a/one": {Profiles: []string{"default"}}},
	}
	// Very wide window so the 2026 fixture runs are always in-window.
	wide := FetchMode{Kind: FetchTrailing, Days: 100000}
	oneWorkflow := func(_ string) ([]workflowRef, error) {
		return []workflowRef{{Name: "WF One", Path: ".github/workflows/wf-one.lock.yml"}}, nil
	}

	t.Run("rich logs → AIC populated, COST = AIC*0.01", func(t *testing.T) {
		rich := mustReadLogsPayload(t, "logs_with_aic.json")
		withLogsStubs(t, oneWorkflow,
			func(_, _ string, _ FetchMode) (logsPayload, error) { return rich, nil })
		res, diags, err := AggregateConsumption(context.Background(), cfg, wide, GroupByRepo, SourceLogs)
		if err != nil {
			t.Fatalf("AggregateConsumption: %v", err)
		}
		if res.Source != "logs" {
			t.Errorf("Source = %q; want logs", res.Source)
		}
		if len(res.Groups) != 1 {
			t.Fatalf("groups = %d; want 1", len(res.Groups))
		}
		g := res.Groups[0]
		if g.AIC == nil {
			t.Fatal("AIC nil; want populated from logs_with_aic fixture")
		}
		// 125.75499 + 137.49432 + 260.16342 = 523.41273 (the failed run omits aic).
		if *g.AIC < 523.0 || *g.AIC > 524.0 {
			t.Errorf("AIC = %v; want ~523.41", *g.AIC)
		}
		if g.Cost == nil || *g.Cost != *g.AIC*aicToUSDRate {
			t.Errorf("Cost = %v; want AIC*%.2f = %v", g.Cost, aicToUSDRate, *g.AIC*aicToUSDRate)
		}
		if hasNilAICHint(diags) {
			t.Errorf("did not expect nil-AIC hint when AIC is populated; diags=%v", diags)
		}
	})

	t.Run("all-failure repo → AIC nil + hint", func(t *testing.T) {
		fail := mustReadLogsPayload(t, "logs_all_failures.json")
		withLogsStubs(t, oneWorkflow,
			func(_, _ string, _ FetchMode) (logsPayload, error) { return fail, nil })
		res, diags, err := AggregateConsumption(context.Background(), cfg, wide, GroupByRepo, SourceLogs)
		if err != nil {
			t.Fatalf("AggregateConsumption: %v", err)
		}
		if len(res.Groups) != 1 {
			t.Fatalf("groups = %d; want 1 (failed runs still produce a report)", len(res.Groups))
		}
		if res.Groups[0].AIC != nil {
			t.Errorf("AIC = %v; want nil for an all-failure repo", *res.Groups[0].AIC)
		}
		if !hasNilAICHint(diags) {
			t.Errorf("expected nil-AIC hint for an all-failure repo; diags=%v", diags)
		}
	})

	t.Run("per-workflow fan-out calls logs once per workflow", func(t *testing.T) {
		rich := mustReadLogsPayload(t, "logs_with_aic.json")
		var calls []string
		withLogsStubs(t,
			func(_ string) ([]workflowRef, error) {
				return []workflowRef{
					{Name: "Beta", Path: ".github/workflows/b.lock.yml"},
					{Name: "Alpha", Path: ".github/workflows/a.lock.yml"},
				}, nil
			},
			func(_, workflow string, _ FetchMode) (logsPayload, error) {
				calls = append(calls, workflow)
				return rich, nil
			})
		_, _, err := AggregateConsumption(context.Background(), cfg, wide, GroupByWorkflow, SourceLogs)
		if err != nil {
			t.Fatalf("AggregateConsumption: %v", err)
		}
		// Sorted fan-out: Alpha before Beta.
		if len(calls) != 2 || calls[0] != "Alpha" || calls[1] != "Beta" {
			t.Errorf("logs fan-out calls = %v; want [Alpha Beta]", calls)
		}
	})
}

func TestAggregateConsumption_LogsSourceRejectsOldGhAw(t *testing.T) {
	cfg := &Config{
		LoadedFrom: "fleet.local.json",
		Profiles:   map[string]Profile{"default": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}}},
		Repos:      map[string]RepoSpec{"a/one": {Profiles: []string{"default"}}},
	}
	prevW, prevL, prevV := ghWorkflowsAPI, ghLogsAPI, ghAwVersion
	t.Cleanup(func() { ghWorkflowsAPI, ghLogsAPI, ghAwVersion = prevW, prevL, prevV })
	var workflowCalls, logsCalls int
	ghAwVersion = func(_ context.Context) (string, error) { return "v0.77.5", nil }
	ghWorkflowsAPI = func(_ context.Context, _ string) ([]workflowRef, error) {
		workflowCalls++
		return nil, nil
	}
	ghLogsAPI = func(_ context.Context, _, _ string, _ FetchMode) (logsPayload, error) {
		logsCalls++
		return logsPayload{}, nil
	}

	res, diags, err := AggregateConsumption(
		context.Background(), cfg, FetchMode{Kind: FetchLatest}, GroupByRepo, SourceLogs,
	)
	if err == nil {
		t.Fatal("AggregateConsumption: err nil; want old gh-aw version error")
	}
	if res != nil {
		t.Fatalf("res = %#v; want nil when version gate fails", res)
	}
	if diags != nil {
		t.Fatalf("diags = %#v; want nil when version gate fails", diags)
	}
	for _, want := range []string{"v0.77.5", logsSourceMinVersion, "--source artifacts"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q; want substring %q", err.Error(), want)
		}
	}
	if workflowCalls != 0 || logsCalls != 0 {
		t.Errorf("version gate did not short-circuit: workflowCalls=%d logsCalls=%d", workflowCalls, logsCalls)
	}
}

func TestAggregateConsumption_LogsSourceSafeWritesUsesFilteredRuns(t *testing.T) {
	cfg := &Config{
		LoadedFrom: "fleet.local.json",
		Profiles:   map[string]Profile{"default": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}}},
		Repos:      map[string]RepoSpec{"a/one": {Profiles: []string{"default"}}},
	}
	oneWorkflow := func(_ string) ([]workflowRef, error) {
		return []workflowRef{{Name: "WF One", Path: ".github/workflows/wf-one.lock.yml"}}, nil
	}
	olderAIC, latestAIC := 1.0, 2.0
	payload := logsPayload{
		Summary: logsSummary{TotalSafeItems: 99},
		Runs: []logsRun{
			{
				RunID:          1,
				AIC:            &olderAIC,
				GitHubAPICalls: 7,
				SafeItemsCount: 40,
				ActionMinutes:  1,
				CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				RunID:          2,
				AIC:            &latestAIC,
				GitHubAPICalls: 3,
				SafeItemsCount: 2,
				ActionMinutes:  1,
				CreatedAt:      time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	withLogsStubs(t, oneWorkflow,
		func(_, _ string, _ FetchMode) (logsPayload, error) { return payload, nil })

	res, _, err := AggregateConsumption(
		context.Background(), cfg, FetchMode{Kind: FetchLatest}, GroupByRepo, SourceLogs,
	)
	if err != nil {
		t.Fatalf("AggregateConsumption: %v", err)
	}
	if len(res.Groups) != 1 {
		t.Fatalf("groups = %d; want 1", len(res.Groups))
	}
	g := res.Groups[0]
	if g.SafeOutputCalls != 2 {
		t.Errorf("SAFE_WRITES = %d; want 2 from latest run, not summary total 99", g.SafeOutputCalls)
	}
	if g.GitHubAPICalls != 3 {
		t.Errorf("API_CALLS = %d; want 3 from latest run", g.GitHubAPICalls)
	}
	if g.AIC == nil || *g.AIC != latestAIC {
		t.Errorf("AIC = %v; want %v from latest run", g.AIC, latestAIC)
	}
}

func TestCompareVersionTokens(t *testing.T) {
	cases := []struct {
		name    string
		a       string
		b       string
		want    int
		wantErr bool
	}{
		{"equal", "v0.79.2", "v0.79.2", 0, false},
		{"patch greater", "v0.79.3", "v0.79.2", 1, false},
		{"minor lower", "v0.77.5", "v0.79.2", -1, false},
		{"major greater", "v1.0.0", "v0.79.2", 1, false},
		{"invalid", "0.79.2", "v0.79.2", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compareVersionTokens(tc.a, tc.b)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("compareVersionTokens(%q, %q): err nil; want error", tc.a, tc.b)
				}
				return
			}
			if err != nil {
				t.Fatalf("compareVersionTokens(%q, %q): %v", tc.a, tc.b, err)
			}
			if got != tc.want {
				t.Errorf("compareVersionTokens(%q, %q) = %d; want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
