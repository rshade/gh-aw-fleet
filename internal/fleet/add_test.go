package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantOut    string
		wantErr    bool
		wantErrSub string
	}{
		{name: "simple lowercase", in: "rshade/gh-aw-fleet", wantOut: "rshade/gh-aw-fleet"},
		{name: "dots allowed", in: "acme/foo.bar", wantOut: "acme/foo.bar"},
		{name: "underscores allowed", in: "acme/foo_bar", wantOut: "acme/foo_bar"},
		{name: "uppercase normalized", in: "Owner/Repo", wantOut: "owner/repo"},
		{name: "mixed case normalized", in: "GitHub/gh-AW", wantOut: "github/gh-aw"},
		{name: "leading/trailing whitespace trimmed", in: "  rshade/foo  ", wantOut: "rshade/foo"},

		// Error cases — each error must contain "owner/repo" as a valid example.
		{name: "empty", in: "", wantErr: true, wantErrSub: "owner/repo"},
		{name: "whitespace only", in: "   ", wantErr: true, wantErrSub: "owner/repo"},
		{name: "no slash", in: "justaword", wantErr: true, wantErrSub: "owner/repo"},
		{name: "too many slashes", in: "owner/repo/extra", wantErr: true, wantErrSub: "owner/repo"},
		{name: "empty owner half", in: "/repo", wantErr: true, wantErrSub: "owner/repo"},
		{name: "empty repo half", in: "owner/", wantErr: true, wantErrSub: "owner/repo"},
		{name: "whitespace owner half", in: "  /repo", wantErr: true, wantErrSub: "owner/repo"},
		{name: "whitespace repo half", in: "owner/  ", wantErr: true, wantErrSub: "owner/repo"},
		{name: "invalid char in owner", in: "owner!/repo", wantErr: true, wantErrSub: "owner/repo"},
		{name: "space inside half", in: "owner name/repo", wantErr: true, wantErrSub: "owner/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateSlug(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got output %q", tt.in, got)
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error %q does not contain expected substring %q", err.Error(), tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.in, err)
			}
			if got != tt.wantOut {
				t.Errorf("ValidateSlug(%q) = %q, want %q", tt.in, got, tt.wantOut)
			}
		})
	}
}

func TestBuildMinimalLocalConfig(t *testing.T) {
	spec := RepoSpec{Profiles: []string{"default"}}
	cfg := BuildMinimalLocalConfig("rshade/foo", spec)

	if cfg.Version != SchemaVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, SchemaVersion)
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("Repos has %d entries, want 1", len(cfg.Repos))
	}
	if _, ok := cfg.Repos["rshade/foo"]; !ok {
		t.Errorf("Repos does not contain rshade/foo; got %+v", cfg.Repos)
	}

	// Round-trip through JSON and assert exactly two top-level keys.
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if unmarshalErr := json.Unmarshal(data, &raw); unmarshalErr != nil {
		t.Fatalf("json.Unmarshal: %v", unmarshalErr)
	}
	gotKeys := make([]string, 0, len(raw))
	for k := range raw {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	wantKeys := []string{"repos", "version"}
	if !slices.Equal(gotKeys, wantKeys) {
		t.Errorf(
			"top-level keys = %v, want exactly %v (no defaults, no profiles per FR-015)",
			gotKeys, wantKeys,
		)
	}
	var repos map[string]RepoSpec
	if err := json.Unmarshal(raw["repos"], &repos); err != nil {
		t.Fatalf("unmarshal repos: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("repos has %d entries, want 1", len(repos))
	}
}

// seedBaseConfigWithDefaultProfile writes a fleet.json to dir with a single
// "default" profile that contains two workflows from a fake "upstream/x"
// source. Returns the loaded (merged) Config, matching what LoadConfig
// would produce for that state.
func seedBaseConfigWithDefaultProfile(t *testing.T, dir string) *Config {
	t.Helper()
	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"upstream/x": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "workflow-a", Source: "upstream/x"},
					{Name: "workflow-b", Source: "upstream/x"},
				},
			},
			"experimental": {
				Sources:   map[string]SourcePin{"upstream/x": {Ref: "main"}},
				Workflows: []ProfileWorkflow{{Name: "workflow-c", Source: "upstream/x"}},
			},
		},
		Repos: map[string]RepoSpec{},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal base: %v", err)
	}
	basePath := filepath.Join(dir, ConfigFile)
	if writeErr := os.WriteFile(basePath, append(data, '\n'), 0o600); writeErr != nil {
		t.Fatalf("seed fleet.json: %v", writeErr)
	}
	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig after seed: %v", err)
	}
	return loaded
}

func TestAdd_DryRun_HappyPath(t *testing.T) {
	dir := t.TempDir()
	cfg := seedBaseConfigWithDefaultProfile(t, dir)

	opts := AddOptions{
		Repo:     "rshade/new-repo",
		Profiles: []string{"default"},
		Dir:      dir,
	}
	res, err := Add(cfg, opts)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.Repo != "rshade/new-repo" {
		t.Errorf("Repo = %q, want rshade/new-repo", res.Repo)
	}
	if !slices.Equal(res.Profiles, []string{"default"}) {
		t.Errorf("Profiles = %v, want [default]", res.Profiles)
	}
	if len(res.Resolved) != 2 {
		t.Errorf("Resolved count = %d, want 2", len(res.Resolved))
	}
	if res.WroteLocal {
		t.Errorf("WroteLocal = true, want false for dry-run")
	}
	// Assert no fleet.local.json was created in the temp dir.
	if _, err := os.Stat(filepath.Join(dir, LocalConfigFile)); !os.IsNotExist(err) {
		t.Errorf("fleet.local.json exists after dry-run: stat err=%v", err)
	}
}

func TestAdd_Apply_HappyPath(t *testing.T) {
	dir := t.TempDir()
	cfg := seedBaseConfigWithDefaultProfile(t, dir)

	basePath := filepath.Join(dir, ConfigFile)
	baseBefore, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read fleet.json: %v", err)
	}

	opts := AddOptions{
		Repo:      "rshade/new-repo",
		Profiles:  []string{"default"},
		Apply:     true,
		Confirmed: true,
		Dir:       dir,
	}
	res, err := Add(cfg, opts)
	if err != nil {
		t.Fatalf("Add --apply: %v", err)
	}
	if !res.WroteLocal {
		t.Errorf("WroteLocal = false, want true")
	}

	localPath := filepath.Join(dir, LocalConfigFile)
	localData, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read fleet.local.json: %v", err)
	}
	wantMinimal, err := json.MarshalIndent(
		BuildMinimalLocalConfig("rshade/new-repo", RepoSpec{Profiles: []string{"default"}}),
		"", "  ",
	)
	if err != nil {
		t.Fatalf("marshal expected minimal: %v", err)
	}
	if strings.TrimRight(string(localData), "\n") != string(wantMinimal) {
		t.Errorf(
			"fleet.local.json contents differ from BuildMinimalLocalConfig output\ngot:\n%s\nwant:\n%s",
			localData, wantMinimal,
		)
	}

	baseAfter, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("re-read fleet.json: %v", err)
	}
	if string(baseBefore) != string(baseAfter) {
		t.Errorf("fleet.json was modified by Add --apply")
	}
}

func TestAdd_Apply_SynthesizesLocalFromJSON(t *testing.T) {
	dir := t.TempDir()
	cfg := seedBaseConfigWithDefaultProfile(t, dir)

	// Sanity: fleet.local.json does not exist yet.
	if _, err := os.Stat(filepath.Join(dir, LocalConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("precondition failed: fleet.local.json already exists")
	}

	opts := AddOptions{
		Repo:      "rshade/new-repo",
		Profiles:  []string{"default"},
		Apply:     true,
		Confirmed: true,
		Dir:       dir,
	}
	res, err := Add(cfg, opts)
	if err != nil {
		t.Fatalf("Add --apply: %v", err)
	}
	if !res.SynthesizedLocal {
		t.Errorf("SynthesizedLocal = false, want true when fleet.local.json did not exist")
	}
	if _, err := os.Stat(filepath.Join(dir, LocalConfigFile)); err != nil {
		t.Errorf("fleet.local.json was not created: %v", err)
	}
}

func TestAdd_DuplicateRepo_NamesSourceFile(t *testing.T) {
	tests := []struct {
		name         string
		inBase       bool
		inLocal      bool
		wantContains []string
	}{
		{
			name:         "exists in fleet.json only",
			inBase:       true,
			wantContains: []string{"rshade/existing", ConfigFile},
		},
		{
			name:         "exists in fleet.local.json only",
			inLocal:      true,
			wantContains: []string{"rshade/existing", LocalConfigFile},
		},
		{
			name:         "exists in both",
			inBase:       true,
			inLocal:      true,
			wantContains: []string{"rshade/existing", ConfigFile, LocalConfigFile},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			base := &Config{
				Version: SchemaVersion,
				Profiles: map[string]Profile{
					"default": {
						Sources:   map[string]SourcePin{"upstream/x": {Ref: "v1.0.0"}},
						Workflows: []ProfileWorkflow{{Name: "workflow-a", Source: "upstream/x"}},
					},
				},
				Repos: map[string]RepoSpec{},
			}
			if tt.inBase {
				base.Repos["rshade/existing"] = RepoSpec{Profiles: []string{"default"}}
			}
			baseData, _ := json.MarshalIndent(base, "", "  ")
			if err := os.WriteFile(filepath.Join(dir, ConfigFile), append(baseData, '\n'), 0o600); err != nil {
				t.Fatalf("seed fleet.json: %v", err)
			}

			if tt.inLocal {
				local := &Config{
					Version: SchemaVersion,
					Repos:   map[string]RepoSpec{"rshade/existing": {Profiles: []string{"default"}}},
				}
				localData, _ := json.MarshalIndent(local, "", "  ")
				localPath := filepath.Join(dir, LocalConfigFile)
				if err := os.WriteFile(localPath, append(localData, '\n'), 0o600); err != nil {
					t.Fatalf("seed fleet.local.json: %v", err)
				}
			}

			cfg, err := LoadConfig(dir)
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}

			opts := AddOptions{
				Repo:     "rshade/existing",
				Profiles: []string{"default"},
				Dir:      dir,
			}
			_, addErr := Add(cfg, opts)
			if addErr == nil {
				t.Fatal("Add returned nil error for duplicate repo")
			}
			msg := addErr.Error()
			for _, sub := range tt.wantContains {
				if !strings.Contains(msg, sub) {
					t.Errorf("error %q does not contain %q", msg, sub)
				}
			}
		})
	}
}

func TestAdd_UnknownProfile_ListsAvailable(t *testing.T) {
	dir := t.TempDir()
	cfg := seedBaseConfigWithDefaultProfile(t, dir)

	opts := AddOptions{
		Repo:     "rshade/foo",
		Profiles: []string{"nonexistent"},
		Dir:      dir,
	}
	_, err := Add(cfg, opts)
	if err == nil {
		t.Fatal("Add returned nil error for unknown profile")
	}
	msg := err.Error()
	for _, sub := range []string{"nonexistent", "default", "experimental"} {
		if !strings.Contains(msg, sub) {
			t.Errorf("error %q does not contain %q", msg, sub)
		}
	}
	// Profile list must be alphabetically sorted. "default" < "experimental".
	if strings.Index(msg, "default") > strings.Index(msg, "experimental") {
		t.Errorf("profile list not alphabetically sorted in %q", msg)
	}
}

func TestParseExtraWorkflowSpec(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    ExtraWorkflow
		wantErr bool
	}{
		{
			name: "bare name → local",
			in:   "my-local-thing",
			want: ExtraWorkflow{Name: "my-local-thing", Source: "local"},
		},
		{
			name: "3-part agentics with ref",
			in:   "githubnext/agentics/security-guardian@v0.4.1",
			want: ExtraWorkflow{
				Name:   "security-guardian",
				Source: "githubnext/agentics",
				Ref:    "v0.4.1",
			},
		},
		{
			name: "4-part gh-aw with ref",
			in:   "github/gh-aw/.github/workflows/custom.md@v0.68.3",
			want: ExtraWorkflow{
				Name:   "custom",
				Source: "github/gh-aw",
				Ref:    "v0.68.3",
				Path:   ".github/workflows/custom.md",
			},
		},

		// Error cases.
		{name: "3-part without ref", in: "owner/repo/name", wantErr: true},
		{name: "owner/repo alone", in: "owner/repo", wantErr: true},
		{name: "empty ref after @", in: "owner/repo/name@", wantErr: true},
		{name: "malformed path prefix", in: "owner/repo/foo/bar.md@v1", wantErr: true},
		{name: "empty string", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExtraWorkflowSpec(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %+v", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseExtraWorkflowSpec(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestAdd_UnknownEngine(t *testing.T) {
	dir := t.TempDir()
	cfg := seedBaseConfigWithDefaultProfile(t, dir)

	opts := AddOptions{
		Repo:     "rshade/foo",
		Profiles: []string{"default"},
		Engine:   "fictional-engine",
		Dir:      dir,
	}
	_, err := Add(cfg, opts)
	if err == nil {
		t.Fatal("Add returned nil error for unknown engine")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fictional-engine") {
		t.Errorf("error %q does not name rejected engine", msg)
	}
	for _, engine := range []string{"claude", "codex", "copilot", "gemini"} {
		if !strings.Contains(msg, engine) {
			t.Errorf("error %q does not list %q", msg, engine)
		}
	}
	// Engine list must be alphabetically sorted.
	last := -1
	for _, engine := range []string{"claude", "codex", "copilot", "gemini"} {
		idx := strings.Index(msg, engine)
		if idx <= last {
			t.Errorf("engine list not alphabetically sorted in %q (found %q at %d, prev %d)", msg, engine, idx, last)
		}
		last = idx
	}
}

func TestAdd_CustomizationFlags_WriteCorrectSpec(t *testing.T) {
	dir := t.TempDir()
	cfg := seedBaseConfigWithDefaultProfile(t, dir)

	opts := AddOptions{
		Repo:     "rshade/foo",
		Profiles: []string{"default"},
		Engine:   "claude",
		Excludes: []string{"workflow-a"},
		ExtraWorkflows: []string{
			"my-local",
			"githubnext/agentics/security-guardian@v0.4.1",
		},
		Apply:     true,
		Confirmed: true,
		Dir:       dir,
	}
	res, err := Add(cfg, opts)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !res.WroteLocal {
		t.Fatal("WroteLocal = false")
	}

	data, err := os.ReadFile(filepath.Join(dir, LocalConfigFile))
	if err != nil {
		t.Fatalf("read fleet.local.json: %v", err)
	}
	var written Config
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("parse fleet.local.json: %v", err)
	}
	spec, ok := written.Repos["rshade/foo"]
	if !ok {
		t.Fatal("rshade/foo missing from written file")
	}
	if spec.Engine != "claude" {
		t.Errorf("Engine = %q, want claude", spec.Engine)
	}
	if !slices.Equal(spec.ExcludeFromProfiles, []string{"workflow-a"}) {
		t.Errorf("ExcludeFromProfiles = %v, want [workflow-a]", spec.ExcludeFromProfiles)
	}
	if len(spec.ExtraWorkflows) != 2 {
		t.Fatalf("ExtraWorkflows len = %d, want 2", len(spec.ExtraWorkflows))
	}
	if spec.ExtraWorkflows[0].Source != "local" || spec.ExtraWorkflows[0].Name != "my-local" {
		t.Errorf("ExtraWorkflows[0] = %+v, want local/my-local", spec.ExtraWorkflows[0])
	}
	if spec.ExtraWorkflows[1].Source != "githubnext/agentics" ||
		spec.ExtraWorkflows[1].Ref != "v0.4.1" ||
		spec.ExtraWorkflows[1].Name != "security-guardian" {
		t.Errorf("ExtraWorkflows[1] = %+v", spec.ExtraWorkflows[1])
	}
}

func TestAdd_Warnings(t *testing.T) {
	t.Run("no-op exclude", func(t *testing.T) {
		dir := t.TempDir()
		cfg := seedBaseConfigWithDefaultProfile(t, dir)

		opts := AddOptions{
			Repo:     "rshade/foo",
			Profiles: []string{"default"},
			Excludes: []string{"does-not-exist"},
			Dir:      dir,
		}
		res, err := Add(cfg, opts)
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		if len(res.Warnings) == 0 {
			t.Fatal("expected a warning for no-op exclude")
		}
		found := false
		for _, w := range res.Warnings {
			if strings.Contains(w, "does-not-exist") {
				found = true
			}
		}
		if !found {
			t.Errorf("warnings %v do not mention the no-op exclude name", res.Warnings)
		}
	})

	t.Run("shadowed extra", func(t *testing.T) {
		dir := t.TempDir()
		cfg := seedBaseConfigWithDefaultProfile(t, dir)

		opts := AddOptions{
			Repo:           "rshade/foo",
			Profiles:       []string{"default"},
			ExtraWorkflows: []string{"workflow-a"}, // already in default
			Dir:            dir,
		}
		res, err := Add(cfg, opts)
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		found := false
		for _, w := range res.Warnings {
			if strings.Contains(w, "workflow-a") && strings.Contains(w, "shadow") {
				found = true
			}
		}
		if !found {
			t.Errorf("warnings %v do not mention the shadowed extra", res.Warnings)
		}
	})

	t.Run("zero resolved workflows", func(t *testing.T) {
		dir := t.TempDir()
		cfg := seedBaseConfigWithDefaultProfile(t, dir)

		opts := AddOptions{
			Repo:     "rshade/foo",
			Profiles: []string{"default"},
			Excludes: []string{"workflow-a", "workflow-b"},
			Dir:      dir,
		}
		res, err := Add(cfg, opts)
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		if len(res.Resolved) != 0 {
			t.Fatalf("expected 0 resolved workflows, got %d", len(res.Resolved))
		}
		found := false
		for _, w := range res.Warnings {
			if strings.Contains(w, "zero") || strings.Contains(w, "no workflows") {
				found = true
			}
		}
		if !found {
			t.Errorf("warnings %v do not mention zero-resolved condition", res.Warnings)
		}
	})
}

func TestSaveLocalConfig_WritesLocalFile(t *testing.T) {
	dir := t.TempDir()

	// Seed a fleet.json so we can assert it is NOT touched.
	baseCfg := &Config{
		Version:  SchemaVersion,
		Profiles: map[string]Profile{"default": {}},
	}
	baseData, err := json.MarshalIndent(baseCfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal base: %v", err)
	}
	basePath := filepath.Join(dir, ConfigFile)
	if writeErr := os.WriteFile(basePath, append(baseData, '\n'), 0o600); writeErr != nil {
		t.Fatalf("seed fleet.json: %v", writeErr)
	}
	baseBefore, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read fleet.json: %v", err)
	}

	localCfg := BuildMinimalLocalConfig("rshade/foo", RepoSpec{Profiles: []string{"default"}})
	if err := SaveLocalConfig(dir, localCfg); err != nil {
		t.Fatalf("SaveLocalConfig: %v", err)
	}

	localPath := filepath.Join(dir, LocalConfigFile)
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("fleet.local.json not created: %v", err)
	}

	localData, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read fleet.local.json: %v", err)
	}
	var roundTrip Config
	if err := json.Unmarshal(localData, &roundTrip); err != nil {
		t.Fatalf("parse fleet.local.json: %v", err)
	}
	if _, ok := roundTrip.Repos["rshade/foo"]; !ok {
		t.Errorf("round-tripped fleet.local.json missing rshade/foo: %+v", roundTrip.Repos)
	}

	baseAfter, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("re-read fleet.json: %v", err)
	}
	if string(baseBefore) != string(baseAfter) {
		t.Errorf("fleet.json was modified by SaveLocalConfig\nbefore: %s\nafter:  %s", baseBefore, baseAfter)
	}
}
