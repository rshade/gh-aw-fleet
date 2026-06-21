package fleet_test

import (
	"fmt"
	"testing"

	"github.com/rshade/gh-aw-fleet/pkg/fleet"
)

func TestExternalConsumer(t *testing.T) {
	if fleet.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d; want 1", fleet.SchemaVersion)
	}

	compileStrict := true
	cfg := &fleet.Config{
		Version: fleet.SchemaVersion,
		Defaults: fleet.Defaults{
			Engine: "copilot",
		},
		Profiles: map[string]fleet.Profile{
			"default": {
				Description: "Baseline workflows",
				Tier:        "standard",
				Sources: map[string]fleet.SourcePin{
					"githubnext/agentics": {Ref: "main"},
				},
				Workflows: []fleet.ProfileWorkflow{
					{
						Name:   "ci-doctor",
						Source: "githubnext/agentics",
						Path:   "workflows/ci-doctor.md",
					},
				},
			},
		},
		Repos: map[string]fleet.RepoSpec{
			"acme/widgets": {
				Profiles:      []string{"default"},
				CostCenter:    "platform",
				Engine:        "claude",
				CompileStrict: &compileStrict,
				ExtraWorkflows: []fleet.ExtraWorkflow{
					{
						Name:   "local-check",
						Source: "local",
						Path:   ".github/workflows/local-check.md",
					},
				},
				ExcludeFromProfiles: []string{"mergefest"},
				Overrides: map[string]string{
					"ci-doctor": ".github/workflows/ci-doctor.md",
				},
			},
		},
	}

	if got := cfg.EffectiveEngine("acme/widgets"); got != "claude" {
		t.Errorf("EffectiveEngine override = %q; want %q", got, "claude")
	}
	if got := cfg.EffectiveEngine("acme/api"); got != "copilot" {
		t.Errorf("EffectiveEngine default = %q; want %q", got, "copilot")
	}
}

func ExampleConfig_EffectiveEngine() {
	cfg := &fleet.Config{
		Defaults: fleet.Defaults{Engine: "copilot"},
		Repos: map[string]fleet.RepoSpec{
			"acme/widgets": {Engine: "claude"},
		},
	}

	fmt.Println(cfg.EffectiveEngine("acme/widgets"))
	fmt.Println(cfg.EffectiveEngine("acme/api"))

	// Output:
	// claude
	// copilot
}
