package fleet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestComputeDrift_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		repo          string
		declared      []ResolvedWorkflow
		listing       []string
		fetchedBodies map[string]string
		fetchErr      error
		want          RepoStatus
	}{
		{
			name: "aligned",
			repo: "rshade/r1",
			declared: []ResolvedWorkflow{
				{Name: "audit", Source: "githubnext/agentics", Ref: "v1.0"},
			},
			listing: []string{"audit.md"},
			fetchedBodies: map[string]string{
				"audit.md": frontmatterDoc("source: githubnext/agentics/audit@v1.0"),
			},
			want: RepoStatus{
				Repo:       "rshade/r1",
				DriftState: "aligned",
			},
		},
		{
			name: "missing",
			repo: "rshade/r2",
			declared: []ResolvedWorkflow{
				{Name: "audit", Source: "githubnext/agentics", Ref: "v1.0"},
			},
			listing: []string{},
			want: RepoStatus{
				Repo:       "rshade/r2",
				DriftState: "drifted",
				Missing:    []string{"audit"},
			},
		},
		{
			name: "drifted",
			repo: "rshade/r3",
			declared: []ResolvedWorkflow{
				{Name: "audit", Source: "githubnext/agentics", Ref: "v1.0"},
			},
			listing: []string{"audit.md"},
			fetchedBodies: map[string]string{
				"audit.md": frontmatterDoc("source: githubnext/agentics/audit@v0.9"),
			},
			want: RepoStatus{
				Repo:       "rshade/r3",
				DriftState: "drifted",
				Drifted: []WorkflowDrift{
					{Name: "audit", DesiredRef: "v1.0", ActualRef: "v0.9"},
				},
			},
		},
		{
			name:     "extra",
			repo:     "rshade/r4",
			declared: nil,
			listing:  []string{"stranger.md"},
			fetchedBodies: map[string]string{
				"stranger.md": frontmatterDoc("source: githubnext/agentics/stranger@main"),
			},
			want: RepoStatus{
				Repo:       "rshade/r4",
				DriftState: "drifted",
				Extra:      []string{"stranger"},
			},
		},
		{
			name: "unpinned_missing_source",
			repo: "rshade/r5",
			declared: []ResolvedWorkflow{
				{Name: "audit", Source: "githubnext/agentics", Ref: "v1.0"},
			},
			listing: []string{"audit.md"},
			fetchedBodies: map[string]string{
				"audit.md": frontmatterDoc("name: audit"),
			},
			want: RepoStatus{
				Repo:       "rshade/r5",
				DriftState: "drifted",
				Unpinned:   []string{"audit"},
			},
		},
		{
			name: "unpinned_malformed_yaml",
			repo: "rshade/r6",
			declared: []ResolvedWorkflow{
				{Name: "audit", Source: "githubnext/agentics", Ref: "v1.0"},
			},
			listing: []string{"audit.md"},
			fetchedBodies: map[string]string{
				"audit.md": frontmatterDoc("source: [unterminated"),
			},
			want: RepoStatus{
				Repo:       "rshade/r6",
				DriftState: "drifted",
				Unpinned:   []string{"audit"},
			},
		},
		{
			name:     "errored",
			repo:     "rshade/r7",
			fetchErr: errors.New("HTTP 404: not found"),
			want: RepoStatus{
				Repo:         "rshade/r7",
				DriftState:   "errored",
				ErrorMessage: "HTTP 404: not found",
			},
		},
		{
			name:    "extra_undeclared_no_source_ignored",
			repo:    "rshade/r8",
			listing: []string{"random.md"},
			fetchedBodies: map[string]string{
				"random.md": "# plain markdown, no frontmatter",
			},
			want: RepoStatus{
				Repo:       "rshade/r8",
				DriftState: "aligned",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDrift(tt.repo, tt.declared, tt.listing, tt.fetchedBodies, tt.fetchErr)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("computeDrift mismatch:\n got=%#v\nwant=%#v", got, tt.want)
			}
		})
	}
}

// recordingFetcher tracks every method invocation. T007 item (h) asserts
// the recorded calls contain ONLY listWorkflowsDir / fetchWorkflowBody —
// any future mutating method on statusFetcher would surface here.
type recordingFetcher struct {
	mu       sync.Mutex
	calls    []string
	listings map[string][]string
	bodies   map[string]map[string]string
	listErr  map[string]error

	inFlightRepos atomic.Int32
	maxConcurrent atomic.Int32

	repoOrderMu sync.Mutex
	repoOrder   map[string][]string
}

func (f *recordingFetcher) listWorkflowsDir(_ context.Context, repo string) ([]string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("list:%s", repo))
	f.mu.Unlock()
	now := f.inFlightRepos.Add(1)
	defer f.inFlightRepos.Add(-1)
	for {
		peak := f.maxConcurrent.Load()
		if now <= peak || f.maxConcurrent.CompareAndSwap(peak, now) {
			break
		}
	}
	// Sleep briefly so concurrent runs overlap measurably.
	time.Sleep(5 * time.Millisecond)
	if err, ok := f.listErr[repo]; ok && err != nil {
		return nil, err
	}
	return f.listings[repo], nil
}

func (f *recordingFetcher) fetchWorkflowBody(_ context.Context, repo, file string) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("fetch:%s/%s", repo, file))
	f.mu.Unlock()
	f.repoOrderMu.Lock()
	f.repoOrder[repo] = append(f.repoOrder[repo], file)
	f.repoOrderMu.Unlock()
	if m, ok := f.bodies[repo]; ok {
		if body, fileOK := m[file]; fileOK {
			return body, nil
		}
	}
	return "", nil
}

func newRecordingFetcher() *recordingFetcher {
	return &recordingFetcher{
		listings:  map[string][]string{},
		bodies:    map[string]map[string]string{},
		listErr:   map[string]error{},
		repoOrder: map[string][]string{},
	}
}

func TestStatus_FleetWide_AllStates(t *testing.T) {
	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "audit", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			"rshade/aligned": {Profiles: []string{"default"}},
			"rshade/drifted": {Profiles: []string{"default"}},
			"rshade/errored": {Profiles: []string{"default"}},
		},
	}

	fetcher := newRecordingFetcher()
	fetcher.listings = map[string][]string{
		"rshade/aligned": {"audit.md"},
		"rshade/drifted": {"audit.md"},
	}
	fetcher.bodies = map[string]map[string]string{
		"rshade/aligned": {
			"audit.md": frontmatterDoc("source: githubnext/agentics/audit@v1.0"),
		},
		"rshade/drifted": {
			"audit.md": frontmatterDoc("source: githubnext/agentics/audit@v0.9"),
		},
	}
	fetcher.listErr = map[string]error{
		"rshade/errored": errors.New("HTTP 404"),
	}

	cwdBefore, _ := os.ReadDir(".")
	res, diags, err := Status(context.Background(), cfg, StatusOpts{fetcher: fetcher})
	cwdAfter, _ := os.ReadDir(".")
	if err != nil {
		t.Fatalf("Status returned setup error: %v", err)
	}
	if res == nil {
		t.Fatal("Status returned nil result")
	}

	// (a)+(b): all repos present in result, sorted alphabetically.
	if len(res.Repos) != 3 {
		t.Fatalf("len(res.Repos) = %d; want 3", len(res.Repos))
	}
	wantOrder := []string{"rshade/aligned", "rshade/drifted", "rshade/errored"}
	for i, want := range wantOrder {
		if res.Repos[i].Repo != want {
			t.Errorf("res.Repos[%d].Repo = %q; want %q", i, res.Repos[i].Repo, want)
		}
	}

	// (c): per-repo errors do not abort siblings.
	byRepo := map[string]RepoStatus{}
	for _, r := range res.Repos {
		byRepo[r.Repo] = r
	}
	if byRepo["rshade/aligned"].DriftState != "aligned" {
		t.Errorf("aligned.DriftState = %q; want aligned", byRepo["rshade/aligned"].DriftState)
	}
	if byRepo["rshade/drifted"].DriftState != "drifted" {
		t.Errorf("drifted.DriftState = %q; want drifted", byRepo["rshade/drifted"].DriftState)
	}
	if byRepo["rshade/errored"].DriftState != "errored" {
		t.Errorf("errored.DriftState = %q; want errored", byRepo["rshade/errored"].DriftState)
	}
	if byRepo["rshade/errored"].ErrorMessage == "" {
		t.Error("errored.ErrorMessage should be non-empty")
	}

	// (g): errored repo produces a Diagnostic with DiagRepoInaccessible.
	var foundErrDiag bool
	for _, d := range diags {
		if d.Code == DiagRepoInaccessible && d.Fields["repo"] == "rshade/errored" {
			foundErrDiag = true
		}
	}
	if !foundErrDiag {
		t.Errorf("expected repo_inaccessible diagnostic for rshade/errored; got %#v", diags)
	}

	// (h): only listWorkflowsDir / fetchWorkflowBody were called.
	for _, call := range fetcher.calls {
		if !startsWithAny(call, "list:", "fetch:") {
			t.Errorf("unexpected call recorded: %q", call)
		}
	}

	// (h, cont.): cwd is unchanged before/after.
	if !sameDirEntries(cwdBefore, cwdAfter) {
		t.Error("Status() modified working directory contents")
	}

	// (d): worker pool concurrency bounded by statusWorkerPoolSize.
	if peak := fetcher.maxConcurrent.Load(); peak > int32(statusWorkerPoolSize) {
		t.Errorf("maxConcurrent = %d; want <= %d", peak, statusWorkerPoolSize)
	} else if peak < 2 {
		t.Errorf("maxConcurrent = %d; want >= 2 to prove repo work is parallel", peak)
	}
}

func TestStatus_PerRepoConfigResolveError(t *testing.T) {
	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "audit", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			"rshade/ok":     {Profiles: []string{"default"}},
			"rshade/broken": {Profiles: []string{"nonexistent"}},
		},
	}
	fetcher := newRecordingFetcher()
	fetcher.listings = map[string][]string{
		"rshade/ok": {"audit.md"},
	}
	fetcher.bodies = map[string]map[string]string{
		"rshade/ok": {
			"audit.md": frontmatterDoc("source: githubnext/agentics/audit@v1.0"),
		},
	}
	res, diags, err := Status(context.Background(), cfg, StatusOpts{fetcher: fetcher})
	if err != nil {
		t.Fatalf("Status returned setup error: %v", err)
	}
	if len(res.Repos) != 2 {
		t.Fatalf("len(res.Repos) = %d; want 2", len(res.Repos))
	}

	byRepo := map[string]RepoStatus{}
	for _, r := range res.Repos {
		byRepo[r.Repo] = r
	}
	if byRepo["rshade/ok"].DriftState != "aligned" {
		t.Errorf("ok.DriftState = %q; want aligned", byRepo["rshade/ok"].DriftState)
	}
	if byRepo["rshade/broken"].DriftState != "errored" {
		t.Errorf("broken.DriftState = %q; want errored", byRepo["rshade/broken"].DriftState)
	}

	// fetcher must not have been called for the broken repo (resolve fails first).
	for _, call := range fetcher.calls {
		if call == "list:rshade/broken" {
			t.Error("fetcher.listWorkflowsDir was called for broken repo (should short-circuit)")
		}
	}

	// Diagnostic surfaced for the broken repo.
	var foundDiag bool
	for _, d := range diags {
		if d.Fields["repo"] == "rshade/broken" {
			foundDiag = true
		}
	}
	if !foundDiag {
		t.Error("expected diagnostic for rshade/broken")
	}
}

func TestStatus_SingleRepo_InCfg(t *testing.T) {
	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0"},
				},
				Workflows: []ProfileWorkflow{{Name: "audit", Source: "githubnext/agentics"}},
			},
		},
		Repos: map[string]RepoSpec{
			"rshade/target": {Profiles: []string{"default"}},
			"rshade/other":  {Profiles: []string{"default"}},
		},
	}
	fetcher := newRecordingFetcher()
	fetcher.listings = map[string][]string{
		"rshade/target": {"audit.md"},
	}
	fetcher.bodies = map[string]map[string]string{
		"rshade/target": {
			"audit.md": frontmatterDoc("source: githubnext/agentics/audit@v1.0"),
		},
	}
	res, _, err := Status(context.Background(), cfg, StatusOpts{Repo: "rshade/target", fetcher: fetcher})
	if err != nil {
		t.Fatalf("Status returned err: %v", err)
	}
	if len(res.Repos) != 1 {
		t.Fatalf("len(res.Repos) = %d; want 1", len(res.Repos))
	}
	if res.Repos[0].Repo != "rshade/target" {
		t.Errorf("res.Repos[0].Repo = %q; want rshade/target", res.Repos[0].Repo)
	}
	// The other repo must not have been queried.
	for _, call := range fetcher.calls {
		if call == "list:rshade/other" {
			t.Error("fetcher contacted unrelated repo for single-repo invocation")
		}
	}
}

func TestStatus_SingleRepo_NotInCfg(t *testing.T) {
	cfg := &Config{
		Version: SchemaVersion,
		Repos:   map[string]RepoSpec{"rshade/target": {}},
	}
	fetcher := newRecordingFetcher()
	_, _, err := Status(context.Background(), cfg, StatusOpts{Repo: "some/unknown", fetcher: fetcher})
	if err == nil {
		t.Fatal("Status should have errored on unknown repo")
	}
	wantSubstr := `repo "some/unknown" is not declared in fleet config`
	if got := err.Error(); got != wantSubstr {
		t.Errorf("err = %q; want %q", got, wantSubstr)
	}
	if len(fetcher.calls) != 0 {
		t.Errorf("fetcher contacted GitHub %d times; want 0", len(fetcher.calls))
	}
}

func TestStatus_EmptyFleet_EmitsWarning(t *testing.T) {
	cfg := &Config{Version: SchemaVersion, Repos: map[string]RepoSpec{}}
	fetcher := newRecordingFetcher()
	res, diags, err := Status(context.Background(), cfg, StatusOpts{fetcher: fetcher})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(res.Repos) != 0 {
		t.Errorf("len(res.Repos) = %d; want 0", len(res.Repos))
	}
	var sawEmpty bool
	for _, d := range diags {
		if d.Code == DiagEmptyFleet {
			sawEmpty = true
		}
	}
	if !sawEmpty {
		t.Error("expected empty_fleet diagnostic for zero-repo fleet")
	}
}

func TestStatus_DiagnosticSorting(t *testing.T) {
	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0"},
				},
				Workflows: []ProfileWorkflow{{Name: "audit", Source: "githubnext/agentics"}},
			},
		},
		Repos: map[string]RepoSpec{
			"rshade/zeta":  {Profiles: []string{"default"}},
			"rshade/alpha": {Profiles: []string{"default"}},
		},
	}
	fetcher := newRecordingFetcher()
	fetcher.listErr = map[string]error{
		"rshade/zeta":  errors.New("HTTP 404"),
		"rshade/alpha": errors.New("HTTP 404"),
	}
	_, diags, err := Status(context.Background(), cfg, StatusOpts{fetcher: fetcher})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(diags) < 2 {
		t.Fatalf("len(diags) = %d; want >= 2", len(diags))
	}
	repos := make([]string, 0, len(diags))
	for _, d := range diags {
		if r, ok := d.Fields["repo"].(string); ok {
			repos = append(repos, r)
		}
	}
	if !sort.StringsAreSorted(repos) {
		t.Errorf("diagnostics not sorted by repo: %v", repos)
	}
}

func TestStatus_CtxCancel_StopsDequeuingAndReturnsCtxErr(t *testing.T) {
	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0"},
				},
				Workflows: []ProfileWorkflow{{Name: "audit", Source: "githubnext/agentics"}},
			},
		},
		Repos: map[string]RepoSpec{},
	}
	for i := range 50 {
		cfg.Repos[fmt.Sprintf("rshade/r%02d", i)] = RepoSpec{Profiles: []string{"default"}}
	}

	fetcher := newRecordingFetcher()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, _, err := Status(ctx, cfg, StatusOpts{fetcher: fetcher})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Status err = %v; want context.Canceled", err)
	}
	if res == nil {
		t.Fatal("Status returned nil result on cancel; want partial StatusResult")
	}
	if len(fetcher.calls) != 0 {
		t.Errorf("fetcher invoked %d times after pre-cancel; want 0 (workers must short-circuit before processJob)",
			len(fetcher.calls))
	}
}

func TestStatus_RateLimitedDiagnosticCode(t *testing.T) {
	cfg := &Config{
		Version: SchemaVersion,
		Repos: map[string]RepoSpec{
			"rshade/limited": {},
		},
	}
	fetcher := newRecordingFetcher()
	fetcher.listErr = map[string]error{
		"rshade/limited": errors.New("gh api: API rate limit exceeded for user 1234"),
	}
	_, diags, err := Status(context.Background(), cfg, StatusOpts{fetcher: fetcher})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	var found bool
	for _, d := range diags {
		if d.Code == DiagRateLimited {
			found = true
		}
	}
	if !found {
		t.Errorf("expected rate_limited diagnostic; got %#v", diags)
	}
}

func TestGHStatusFetcher_MissingWorkflowDirectoryIsEmpty(t *testing.T) {
	old := ghAPIJSON
	t.Cleanup(func() { ghAPIJSON = old })

	var calls []string
	ghAPIJSON = func(_ context.Context, path string) (any, error) {
		calls = append(calls, path)
		switch path {
		case "/repos/rshade/new/contents/.github/workflows":
			return nil, errors.New("gh api: Not Found (HTTP 404)")
		case "/repos/rshade/new":
			return map[string]any{"full_name": "rshade/new"}, nil
		default:
			return nil, fmt.Errorf("unexpected path %s", path)
		}
	}

	got, err := ghStatusFetcher{}.listWorkflowsDir(context.Background(), "rshade/new")
	if err != nil {
		t.Fatalf("listWorkflowsDir returned error: %v", err)
	}
	if got == nil {
		t.Fatal("listWorkflowsDir returned nil; want empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("listWorkflowsDir returned %v; want empty listing", got)
	}
	wantCalls := []string{
		"/repos/rshade/new/contents/.github/workflows",
		"/repos/rshade/new",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("ghAPIJSON calls = %v; want %v", calls, wantCalls)
	}
}

func TestGHStatusFetcher_Repo404RemainsError(t *testing.T) {
	old := ghAPIJSON
	t.Cleanup(func() { ghAPIJSON = old })

	ghAPIJSON = func(_ context.Context, path string) (any, error) {
		switch path {
		case "/repos/rshade/private/contents/.github/workflows":
			return nil, errors.New("gh api: Not Found (HTTP 404)")
		case "/repos/rshade/private":
			return nil, errors.New("gh api: Not Found (HTTP 404)")
		default:
			return nil, fmt.Errorf("unexpected path %s", path)
		}
	}

	_, err := ghStatusFetcher{}.listWorkflowsDir(context.Background(), "rshade/private")
	if err == nil {
		t.Fatal("listWorkflowsDir returned nil; want repo error")
	}
	if !strings.Contains(err.Error(), "list rshade/private workflows") {
		t.Fatalf("error = %q; want list context", err.Error())
	}
}

func frontmatterDoc(fm string) string {
	return "---\n" + fm + "\n---\n# body\n"
}

func startsWithAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

func sameDirEntries(a, b []os.DirEntry) bool {
	if len(a) != len(b) {
		return false
	}
	an := make([]string, len(a))
	bn := make([]string, len(b))
	for i := range a {
		an[i] = a[i].Name()
		bn[i] = b[i].Name()
	}
	sort.Strings(an)
	sort.Strings(bn)
	return reflect.DeepEqual(an, bn)
}
