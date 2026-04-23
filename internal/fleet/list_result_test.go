package fleet

import (
	"sort"
	"testing"
)

func TestBuildListResult_Shape(t *testing.T) {
	cfg := &Config{
		LoadedFrom: "fleet.local.json",
		Defaults:   Defaults{Engine: "copilot"},
		Profiles: map[string]Profile{
			"empty": {
				Sources:   map[string]SourcePin{},
				Workflows: []ProfileWorkflow{},
			},
		},
		Repos: map[string]RepoSpec{
			"z/last": {Profiles: []string{"empty"}},
			"a/first": {
				Profiles:            []string{"empty"},
				ExcludeFromProfiles: []string{"never"},
				ExtraWorkflows:      []ExtraWorkflow{{Name: "custom-extra", Source: SourceLocal}},
				Engine:              "claude",
			},
		},
	}

	res, err := BuildListResult(cfg)
	if err != nil {
		t.Fatalf("BuildListResult: %v", err)
	}
	if res.LoadedFrom != "fleet.local.json" {
		t.Errorf("LoadedFrom = %q; want fleet.local.json", res.LoadedFrom)
	}
	if len(res.Repos) != 2 {
		t.Fatalf("len(Repos) = %d; want 2", len(res.Repos))
	}
	// Sorted alphabetically by Repo.
	got := make([]string, len(res.Repos))
	for i, r := range res.Repos {
		got[i] = r.Repo
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("Repos not sorted: %v", got)
	}
	if got[0] != "a/first" || got[1] != "z/last" {
		t.Errorf("repo order = %v; want [a/first z/last]", got)
	}

	first := res.Repos[0]
	if first.Engine != "claude" {
		t.Errorf("a/first Engine = %q; want claude", first.Engine)
	}
	if len(first.Excluded) != 1 || first.Excluded[0] != "never" {
		t.Errorf("a/first Excluded = %v; want [never]", first.Excluded)
	}
	if len(first.Extra) != 1 || first.Extra[0] != "custom-extra" {
		t.Errorf("a/first Extra = %v; want [custom-extra]", first.Extra)
	}

	last := res.Repos[1]
	if last.Engine != "copilot" {
		t.Errorf("z/last Engine = %q; want copilot (from defaults)", last.Engine)
	}
	// All slice fields non-nil even when empty.
	for _, r := range res.Repos {
		if r.Profiles == nil {
			t.Errorf("repo %q Profiles is nil; want non-nil empty slice", r.Repo)
		}
		if r.Workflows == nil {
			t.Errorf("repo %q Workflows is nil; want non-nil empty slice", r.Repo)
		}
		if r.Excluded == nil {
			t.Errorf("repo %q Excluded is nil; want non-nil empty slice", r.Repo)
		}
		if r.Extra == nil {
			t.Errorf("repo %q Extra is nil; want non-nil empty slice", r.Repo)
		}
	}
}

func TestBuildListResult_EmptyConfig(t *testing.T) {
	cfg := &Config{LoadedFrom: ""}
	res, err := BuildListResult(cfg)
	if err != nil {
		t.Fatalf("BuildListResult: %v", err)
	}
	if res.Repos == nil {
		t.Error("Repos is nil; want non-nil empty slice")
	}
	if len(res.Repos) != 0 {
		t.Errorf("len(Repos) = %d; want 0", len(res.Repos))
	}
	if res.LoadedFrom != "" {
		t.Errorf("LoadedFrom = %q; want empty", res.LoadedFrom)
	}
}

func TestBuildListResult_EngineEmptyStringNotDash(t *testing.T) {
	// Engine empty when neither defaults nor per-repo override is set.
	cfg := &Config{
		Profiles: map[string]Profile{
			"p": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
		},
		Repos: map[string]RepoSpec{"x/y": {Profiles: []string{"p"}}},
	}
	res, err := BuildListResult(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Repos[0].Engine != "" {
		t.Errorf("Engine = %q; want empty string (NOT \"-\" placeholder)", res.Repos[0].Engine)
	}
}
