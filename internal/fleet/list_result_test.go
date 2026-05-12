package fleet

import (
	"encoding/json"
	"sort"
	"strings"
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

// TestBuildListResult_ProfileTiers covers FR-007: ProfileTiers reflects
// per-profile Tier values for the row, and is an empty map (never nil) when
// no profile in the row has a tier.
func TestBuildListResult_ProfileTiers(t *testing.T) {
	cfg := &Config{
		Defaults: Defaults{Engine: "copilot"},
		Profiles: map[string]Profile{
			"default":       {Tier: "standard", Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
			"security-plus": {Tier: "premium", Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
			"legacy":        {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
		},
		Repos: map[string]RepoSpec{
			"o/single-tiered":    {Profiles: []string{"default"}},
			"o/two-mixed":        {Profiles: []string{"default", "legacy"}},
			"o/two-both-tiered":  {Profiles: []string{"default", "security-plus"}},
			"o/all-untiered":     {Profiles: []string{"legacy"}},
			"o/two-all-untiered": {Profiles: []string{"legacy", "legacy"}},
		},
	}

	res, err := BuildListResult(cfg)
	if err != nil {
		t.Fatalf("BuildListResult: %v", err)
	}
	by := make(map[string]ListRow, len(res.Repos))
	for _, r := range res.Repos {
		by[r.Repo] = r
	}

	cases := []struct {
		repo string
		want map[string]string
	}{
		{"o/single-tiered", map[string]string{"default": "standard"}},
		{"o/two-mixed", map[string]string{"default": "standard"}},
		{"o/two-both-tiered", map[string]string{"default": "standard", "security-plus": "premium"}},
		{"o/all-untiered", map[string]string{}},
		{"o/two-all-untiered", map[string]string{}},
	}
	for _, tc := range cases {
		row, ok := by[tc.repo]
		if !ok {
			t.Errorf("missing row for %q", tc.repo)
			continue
		}
		if row.ProfileTiers == nil {
			t.Errorf("%s: ProfileTiers is nil; want non-nil map (FR-007 invariant)", tc.repo)
			continue
		}
		if len(row.ProfileTiers) != len(tc.want) {
			t.Errorf("%s: ProfileTiers = %v; want %v", tc.repo, row.ProfileTiers, tc.want)
			continue
		}
		for k, v := range tc.want {
			if got := row.ProfileTiers[k]; got != v {
				t.Errorf("%s: ProfileTiers[%q] = %q; want %q", tc.repo, k, got, v)
			}
		}
	}
}

// TestListRow_ProfileTiersEmptyMapMarshalsAsObject covers the FR-007 edge case
// the plan explicitly calls out: an initialized-empty (non-nil) ProfileTiers
// must marshal to "profile_tiers":{}, never "profile_tiers":null.
func TestListRow_ProfileTiersEmptyMapMarshalsAsObject(t *testing.T) {
	row := ListRow{
		Repo:         "o/r",
		Profiles:     []string{},
		ProfileTiers: map[string]string{},
		Workflows:    []string{},
		Excluded:     []string{},
		Extra:        []string{},
	}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, `"profile_tiers":{}`) {
		t.Errorf("marshaled output missing \"profile_tiers\":{}: %s", out)
	}
	if strings.Contains(out, `"profile_tiers":null`) {
		t.Errorf("marshaled output contains forbidden \"profile_tiers\":null: %s", out)
	}
}

// TestBuildListResult_CostCenter covers FR-008: ListRow.CostCenter copies
// RepoSpec.CostCenter verbatim, including the empty-string case when the
// annotation is unset.
func TestBuildListResult_CostCenter(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]Profile{
			"p": {Sources: map[string]SourcePin{}, Workflows: []ProfileWorkflow{}},
		},
		Repos: map[string]RepoSpec{
			"o/with-cc":    {Profiles: []string{"p"}, CostCenter: "platform-eng"},
			"o/without-cc": {Profiles: []string{"p"}},
		},
	}
	res, err := BuildListResult(cfg)
	if err != nil {
		t.Fatalf("BuildListResult: %v", err)
	}
	got := map[string]string{}
	for _, r := range res.Repos {
		got[r.Repo] = r.CostCenter
	}
	if got["o/with-cc"] != "platform-eng" {
		t.Errorf("CostCenter for o/with-cc = %q; want %q", got["o/with-cc"], "platform-eng")
	}
	if got["o/without-cc"] != "" {
		t.Errorf("CostCenter for o/without-cc = %q; want empty string", got["o/without-cc"])
	}
}

// TestListRow_CostCenterAlwaysEmitted covers FR-008: cost_center is always
// present in the JSON envelope, even when unset — never omitted.
func TestListRow_CostCenterAlwaysEmitted(t *testing.T) {
	row := ListRow{
		Repo:         "o/r",
		Profiles:     []string{},
		ProfileTiers: map[string]string{},
		Workflows:    []string{},
		Excluded:     []string{},
		Extra:        []string{},
		CostCenter:   "",
	}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, `"cost_center":""`) {
		t.Errorf("marshaled output missing \"cost_center\":\"\": %s", out)
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
