package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestBuildManifest(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *Config
		repo         string
		cliVersion   string
		wantManaged  bool
		wantFleet    string
		wantVersion  string
		wantCLI      string
		wantProfiles []string
	}{
		{
			name: "repo with gh-aw source",
			cfg: &Config{
				Repos: map[string]RepoSpec{
					"owner/repo": {Profiles: []string{"default", "security"}},
				},
				Profiles: map[string]Profile{
					"default": {
						Sources: map[string]SourcePin{
							sourceGitHubAW: {Ref: "v0.79.2"},
						},
						Workflows: []ProfileWorkflow{
							{Name: "test", Source: sourceGitHubAW},
						},
					},
					"security": {
						Sources: map[string]SourcePin{
							sourceGitHubAW: {Ref: "v0.79.2"},
						},
						Workflows: []ProfileWorkflow{},
					},
				},
			},
			repo:         "owner/repo",
			cliVersion:   "0.79.2-test",
			wantManaged:  true,
			wantFleet:    fleetRepoSlug,
			wantVersion:  "v0.79.2",
			wantCLI:      "0.79.2-test",
			wantProfiles: []string{"default", "security"},
		},
		{
			name: "profiles sorted",
			cfg: &Config{
				Repos: map[string]RepoSpec{
					"owner/repo": {Profiles: []string{"z-profile", "a-profile", "m-profile"}},
				},
				Profiles: map[string]Profile{
					"z-profile": {
						Sources: map[string]SourcePin{
							sourceGitHubAW: {Ref: "v1.0.0"},
						},
						Workflows: []ProfileWorkflow{
							{Name: "w1", Source: sourceGitHubAW},
						},
					},
					"a-profile": {
						Sources: map[string]SourcePin{
							sourceGitHubAW: {Ref: "v1.0.0"},
						},
						Workflows: []ProfileWorkflow{},
					},
					"m-profile": {
						Sources: map[string]SourcePin{
							sourceGitHubAW: {Ref: "v1.0.0"},
						},
						Workflows: []ProfileWorkflow{},
					},
				},
			},
			repo:         "owner/repo",
			cliVersion:   "1.0.0",
			wantManaged:  true,
			wantFleet:    fleetRepoSlug,
			wantVersion:  "v1.0.0",
			wantCLI:      "1.0.0",
			wantProfiles: []string{"a-profile", "m-profile", "z-profile"},
		},
		{
			name: "no gh-aw source",
			cfg: &Config{
				Repos: map[string]RepoSpec{
					"owner/repo": {Profiles: []string{"custom"}},
				},
				Profiles: map[string]Profile{
					"custom": {
						Sources: map[string]SourcePin{
							"some/other": {Ref: "v1.2.3"},
						},
						Workflows: []ProfileWorkflow{
							{Name: "test", Source: "some/other"},
						},
					},
				},
			},
			repo:         "owner/repo",
			cliVersion:   "1.2.3",
			wantManaged:  true,
			wantFleet:    fleetRepoSlug,
			wantVersion:  "",
			wantCLI:      "1.2.3",
			wantProfiles: []string{"custom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := buildManifest(tt.cfg, tt.repo, tt.cliVersion)
			if m == nil {
				t.Fatal("buildManifest returned nil")
			}
			if m.Managed != tt.wantManaged {
				t.Errorf("Managed = %v, want %v", m.Managed, tt.wantManaged)
			}
			if m.Fleet != tt.wantFleet {
				t.Errorf("Fleet = %q, want %q", m.Fleet, tt.wantFleet)
			}
			if m.GhAwVersion != tt.wantVersion {
				t.Errorf("GhAwVersion = %q, want %q", m.GhAwVersion, tt.wantVersion)
			}
			if m.CLIVersion != tt.cliVersion {
				t.Errorf("CLIVersion = %q, want %q", m.CLIVersion, tt.cliVersion)
			}
			if !slices.Equal(m.Profiles, tt.wantProfiles) {
				t.Errorf("Profiles = %v, want %v", m.Profiles, tt.wantProfiles)
			}
			if m.DeployedAt.IsZero() {
				t.Error("DeployedAt is zero")
			}
			if m.DeployedAt.Location() != time.UTC {
				t.Errorf("DeployedAt not in UTC: %v", m.DeployedAt.Location())
			}
		})
	}
}

func TestManifestEqualExceptTime(t *testing.T) {
	base := &FleetManifest{
		Managed:     true,
		Fleet:       "owner/repo",
		GhAwVersion: "v1.0.0",
		CLIVersion:  "1.0.0",
		Profiles:    []string{"default", "security"},
		DeployedAt:  time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name string
		a    *FleetManifest
		b    *FleetManifest
		want bool
	}{
		{
			name: "identical except time",
			a:    base,
			b: &FleetManifest{
				Managed:     true,
				Fleet:       "owner/repo",
				GhAwVersion: "v1.0.0",
				CLIVersion:  "1.0.0",
				Profiles:    []string{"default", "security"},
				DeployedAt:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			want: true,
		},
		{
			name: "same instance",
			a:    base,
			b:    base,
			want: true,
		},
		{
			name: "different GhAwVersion",
			a:    base,
			b: &FleetManifest{
				Managed:     true,
				Fleet:       "owner/repo",
				GhAwVersion: "v1.0.1",
				CLIVersion:  "1.0.0",
				Profiles:    []string{"default", "security"},
				DeployedAt:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
		{
			name: "different CLIVersion",
			a:    base,
			b: &FleetManifest{
				Managed:     true,
				Fleet:       "owner/repo",
				GhAwVersion: "v1.0.0",
				CLIVersion:  "1.0.1",
				Profiles:    []string{"default", "security"},
				DeployedAt:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
		{
			name: "different Profiles",
			a:    base,
			b: &FleetManifest{
				Managed:     true,
				Fleet:       "owner/repo",
				GhAwVersion: "v1.0.0",
				CLIVersion:  "1.0.0",
				Profiles:    []string{"default", "docs"},
				DeployedAt:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
		{
			name: "different Fleet",
			a:    base,
			b: &FleetManifest{
				Managed:     true,
				Fleet:       "other/fleet",
				GhAwVersion: "v1.0.0",
				CLIVersion:  "1.0.0",
				Profiles:    []string{"default", "security"},
				DeployedAt:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
		{
			name: "different Managed",
			a:    base,
			b: &FleetManifest{
				Managed:     false,
				Fleet:       "owner/repo",
				GhAwVersion: "v1.0.0",
				CLIVersion:  "1.0.0",
				Profiles:    []string{"default", "security"},
				DeployedAt:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "a nil, b not",
			a:    nil,
			b:    base,
			want: false,
		},
		{
			name: "b nil, a not",
			a:    base,
			b:    nil,
			want: false,
		},
		{
			name: "empty profiles vs non-empty",
			a: &FleetManifest{
				Managed:     true,
				Fleet:       "owner/repo",
				GhAwVersion: "v1.0.0",
				CLIVersion:  "1.0.0",
				Profiles:    []string{},
				DeployedAt:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			b: &FleetManifest{
				Managed:     true,
				Fleet:       "owner/repo",
				GhAwVersion: "v1.0.0",
				CLIVersion:  "1.0.0",
				Profiles:    []string{"default"},
				DeployedAt:  time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manifestEqualExceptTime(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("manifestEqualExceptTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteManifestIfNeeded(t *testing.T) {
	m1 := &FleetManifest{
		Managed:     true,
		Fleet:       fleetRepoSlug,
		GhAwVersion: "v0.79.2",
		CLIVersion:  "0.79.2",
		Profiles:    []string{"default"},
		DeployedAt:  time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	m2 := &FleetManifest{
		Managed:     true,
		Fleet:       fleetRepoSlug,
		GhAwVersion: "v0.79.3",
		CLIVersion:  "0.79.3",
		Profiles:    []string{"default"},
		DeployedAt:  time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name           string
		writeFirst     *FleetManifest
		writeSecond    *FleetManifest
		wantFirstBool  bool
		wantFirstErr   bool
		wantSecondBool bool
		wantSecondErr  bool
	}{
		{
			name:          "first write",
			writeFirst:    m1,
			wantFirstBool: true,
			wantFirstErr:  false,
		},
		{
			name:           "same content redeploy",
			writeFirst:     m1,
			writeSecond:    m1,
			wantFirstBool:  true,
			wantFirstErr:   false,
			wantSecondBool: false,
			wantSecondErr:  false,
		},
		{
			name: "same content but different DeployedAt",
			writeFirst: &FleetManifest{
				Managed:     true,
				Fleet:       fleetRepoSlug,
				GhAwVersion: "v0.79.2",
				CLIVersion:  "0.79.2",
				Profiles:    []string{"default"},
				DeployedAt:  time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			},
			writeSecond: &FleetManifest{
				Managed:     true,
				Fleet:       fleetRepoSlug,
				GhAwVersion: "v0.79.2",
				CLIVersion:  "0.79.2",
				Profiles:    []string{"default"},
				DeployedAt:  time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC),
			},
			wantFirstBool:  true,
			wantFirstErr:   false,
			wantSecondBool: false,
			wantSecondErr:  false,
		},
		{
			name:           "different version",
			writeFirst:     m1,
			writeSecond:    m2,
			wantFirstBool:  true,
			wantFirstErr:   false,
			wantSecondBool: true,
			wantSecondErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			if tt.writeFirst != nil {
				gotBool, gotErr := writeManifestIfNeeded(dir, tt.writeFirst)
				if gotBool != tt.wantFirstBool {
					t.Errorf("first writeManifestIfNeeded bool = %v, want %v", gotBool, tt.wantFirstBool)
				}
				if (gotErr != nil) != tt.wantFirstErr {
					t.Errorf("first writeManifestIfNeeded err = %v, wantErr %v", gotErr, tt.wantFirstErr)
				}
			}

			if tt.writeSecond != nil {
				gotBool, gotErr := writeManifestIfNeeded(dir, tt.writeSecond)
				if gotBool != tt.wantSecondBool {
					t.Errorf("second writeManifestIfNeeded bool = %v, want %v", gotBool, tt.wantSecondBool)
				}
				if (gotErr != nil) != tt.wantSecondErr {
					t.Errorf("second writeManifestIfNeeded err = %v, wantErr %v", gotErr, tt.wantSecondErr)
				}
			}

			path := filepath.Join(dir, FleetManifestPath)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read manifest: %v", err)
			}

			var read FleetManifest
			if err := json.Unmarshal(data, &read); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if read.Managed != true {
				t.Error("manifest Managed should be true")
			}
		})
	}
}

func TestParseManifestJSON(t *testing.T) {
	validJSON := `{
		"managed": true,
		"fleet": "rshade/gh-aw-fleet",
		"gh_aw_version": "v0.79.2",
		"cli_version": "0.79.2",
		"profiles": ["default", "security"],
		"deployed_at": "2025-01-01T12:00:00Z"
	}`

	unmanagedJSON := `{
		"managed": false,
		"fleet": "rshade/gh-aw-fleet"
	}`

	tests := []struct {
		name        string
		body        string
		wantNil     bool
		wantErr     bool
		wantManaged bool
	}{
		{
			name:        "valid managed manifest",
			body:        validJSON,
			wantNil:     false,
			wantErr:     false,
			wantManaged: true,
		},
		{
			name:    "unmanaged manifest returns nil",
			body:    unmanagedJSON,
			wantNil: true,
			wantErr: false,
		},
		{
			name:    "empty string returns nil",
			body:    "",
			wantNil: true,
			wantErr: false,
		},
		{
			name:    "malformed JSON",
			body:    `{invalid json}`,
			wantNil: true,
			wantErr: true,
		},
		{
			name:    "incomplete JSON",
			body:    `{"managed": true`,
			wantNil: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := parseManifestJSON(tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseManifestJSON err = %v, wantErr %v", err, tt.wantErr)
			}
			if (m == nil) != tt.wantNil {
				t.Errorf("parseManifestJSON nil = %v, wantNil %v", m == nil, tt.wantNil)
			}
			if m != nil && m.Managed != tt.wantManaged {
				t.Errorf("Managed = %v, want %v", m.Managed, tt.wantManaged)
			}
		})
	}
}

func TestComputeVersionDrift(t *testing.T) {
	tests := []struct {
		name            string
		manifest        *FleetManifest
		expectedVersion string
		wantState       string
		wantRecorded    string
		wantExpected    string
	}{
		{
			name:            "nil manifest unmanaged",
			manifest:        nil,
			expectedVersion: "v0.79.2",
			wantState:       VersionDriftUnmanaged,
			wantRecorded:    "",
			wantExpected:    "v0.79.2",
		},
		{
			name: "matching versions current",
			manifest: &FleetManifest{
				Managed:     true,
				GhAwVersion: "v0.79.2",
			},
			expectedVersion: "v0.79.2",
			wantState:       VersionDriftCurrent,
			wantRecorded:    "v0.79.2",
			wantExpected:    "v0.79.2",
		},
		{
			name: "mismatching versions behind",
			manifest: &FleetManifest{
				Managed:     true,
				GhAwVersion: "v0.79.1",
			},
			expectedVersion: "v0.79.2",
			wantState:       VersionDriftBehind,
			wantRecorded:    "v0.79.1",
			wantExpected:    "v0.79.2",
		},
		{
			name: "empty manifest version",
			manifest: &FleetManifest{
				Managed:     true,
				GhAwVersion: "",
			},
			expectedVersion: "v0.79.2",
			wantState:       VersionDriftBehind,
			wantRecorded:    "",
			wantExpected:    "v0.79.2",
		},
		{
			name: "empty expected version",
			manifest: &FleetManifest{
				Managed:     true,
				GhAwVersion: "v0.79.2",
			},
			expectedVersion: "",
			wantState:       VersionDriftBehind,
			wantRecorded:    "v0.79.2",
			wantExpected:    "",
		},
		{
			name: "both empty",
			manifest: &FleetManifest{
				Managed:     true,
				GhAwVersion: "",
			},
			expectedVersion: "",
			wantState:       VersionDriftCurrent,
			wantRecorded:    "",
			wantExpected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drift := computeVersionDrift(tt.manifest, tt.expectedVersion)
			if drift == nil {
				t.Fatal("computeVersionDrift returned nil")
			}
			if drift.State != tt.wantState {
				t.Errorf("State = %q, want %q", drift.State, tt.wantState)
			}
			if drift.RecordedVersion != tt.wantRecorded {
				t.Errorf("RecordedVersion = %q, want %q", drift.RecordedVersion, tt.wantRecorded)
			}
			if drift.ExpectedVersion != tt.wantExpected {
				t.Errorf("ExpectedVersion = %q, want %q", drift.ExpectedVersion, tt.wantExpected)
			}
		})
	}
}

func TestResolvedGhAwPin(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		repo    string
		wantPin string
	}{
		{
			name: "repo with gh-aw source",
			cfg: &Config{
				Repos: map[string]RepoSpec{
					"owner/repo": {Profiles: []string{"default"}},
				},
				Profiles: map[string]Profile{
					"default": {
						Sources: map[string]SourcePin{
							sourceGitHubAW: {Ref: "v0.79.2"},
						},
						Workflows: []ProfileWorkflow{
							{Name: "test", Source: sourceGitHubAW},
						},
					},
				},
			},
			repo:    "owner/repo",
			wantPin: "v0.79.2",
		},
		{
			name: "repo with no gh-aw source",
			cfg: &Config{
				Repos: map[string]RepoSpec{
					"owner/repo": {Profiles: []string{"custom"}},
				},
				Profiles: map[string]Profile{
					"custom": {
						Sources: map[string]SourcePin{
							"other/source": {Ref: "v1.0.0"},
						},
						Workflows: []ProfileWorkflow{
							{Name: "test", Source: "other/source"},
						},
					},
				},
			},
			repo:    "owner/repo",
			wantPin: "",
		},
		{
			name: "repo not in config",
			cfg: &Config{
				Repos: map[string]RepoSpec{},
				Profiles: map[string]Profile{
					"default": {
						Sources: map[string]SourcePin{
							sourceGitHubAW: {Ref: "v0.79.2"},
						},
					},
				},
			},
			repo:    "unknown/repo",
			wantPin: "",
		},
		{
			name: "multiple profiles first has gh-aw",
			cfg: &Config{
				Repos: map[string]RepoSpec{
					"owner/repo": {Profiles: []string{"p1", "p2"}},
				},
				Profiles: map[string]Profile{
					"p1": {
						Sources: map[string]SourcePin{
							sourceGitHubAW: {Ref: "v0.79.2"},
						},
						Workflows: []ProfileWorkflow{
							{Name: "w1", Source: sourceGitHubAW},
						},
					},
					"p2": {
						Sources: map[string]SourcePin{
							sourceGitHubAW: {Ref: "v0.80.0"},
						},
						Workflows: []ProfileWorkflow{
							{Name: "w2", Source: sourceGitHubAW},
						},
					},
				},
			},
			repo:    "owner/repo",
			wantPin: "v0.79.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pin := resolvedGhAwPin(tt.cfg, tt.repo)
			if pin != tt.wantPin {
				t.Errorf("resolvedGhAwPin() = %q, want %q", pin, tt.wantPin)
			}
		})
	}
}
