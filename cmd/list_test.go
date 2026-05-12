package cmd

import (
	"reflect"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

// TestTiersForRow exercises the pure tiersForRow helper without a tabwriter
// dependency. The cases mirror data-model.md §Rendering examples — including
// the special "all positions would be '-'" case where the function returns an
// empty slice so %v formats as [] (matching the existing slice-empty
// convention used for Excluded / Extra).
func TestTiersForRow(t *testing.T) {
	defs := map[string]fleet.Profile{
		"default":       {Tier: "standard"},
		"quality-plus":  {Tier: "premium"},
		"custom-legacy": {}, // no tier
	}

	cases := []struct {
		name     string
		profiles []string
		want     []string
	}{
		{
			name:     "one tiered profile",
			profiles: []string{"default"},
			want:     []string{"standard"},
		},
		{
			name:     "two tiered profiles",
			profiles: []string{"default", "quality-plus"},
			want:     []string{"standard", "premium"},
		},
		{
			name:     "mixed tiered and untiered",
			profiles: []string{"default", "custom-legacy", "quality-plus"},
			want:     []string{"standard", "-", "premium"},
		},
		{
			name:     "single untiered profile renders empty slice",
			profiles: []string{"custom-legacy"},
			want:     []string{},
		},
		{
			name:     "all untiered renders empty slice not all-dashes",
			profiles: []string{"custom-legacy", "custom-legacy"},
			want:     []string{},
		},
		{
			name:     "empty profiles list renders empty slice",
			profiles: []string{},
			want:     []string{},
		},
		{
			name:     "unknown profile name is treated as untiered",
			profiles: []string{"default", "no-such-profile"},
			want:     []string{"standard", "-"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tiersForRow(tc.profiles, defs)
			if got == nil {
				t.Fatalf("tiersForRow returned nil; want non-nil slice")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("tiersForRow(%v) = %v; want %v", tc.profiles, got, tc.want)
			}
		})
	}
}

func TestTiersCellForRowPreservesFreeFormBoundaries(t *testing.T) {
	defs := map[string]fleet.Profile{
		"review":  {Tier: "premium review"},
		"default": {Tier: "standard"},
	}

	got := tiersCellForRow([]string{"review", "default"}, defs)
	want := `["premium review" "standard"]`
	if got != want {
		t.Errorf("tiersCellForRow() = %q; want %q", got, want)
	}
}
