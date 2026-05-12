package fleet

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestLoadConfig_HuJsonSyntax confirms HuJson syntax (line + block comments,
// trailing commas) round-trips to the same Go struct as the equivalent
// plain-JSON file.
func TestLoadConfig_HuJsonSyntax(t *testing.T) {
	plain := `{
  "version": 1,
  "profiles": {
    "default": {
      "sources": {"upstream/x": {"ref": "v1.0.0"}},
      "workflows": [{"name": "wf-a", "source": "upstream/x"}]
    }
  },
  "repos": {}
}
`
	hujsonStyle := `{
  // Schema version pinned to v1 until we cut over to v2.
  "version": 1,
  "profiles": {
    "default": {
      /* upstream/x is the canonical workflow source */
      "sources": {"upstream/x": {"ref": "v1.0.0"}},
      "workflows": [
        {"name": "wf-a", "source": "upstream/x"}, // primary workflow
      ],
    },
  },
  "repos": {},
}
`

	dirPlain := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirPlain, ConfigFile), []byte(plain), 0o600); err != nil {
		t.Fatalf("seed plain: %v", err)
	}
	plainCfg, err := LoadConfig(dirPlain)
	if err != nil {
		t.Fatalf("LoadConfig(plain): %v", err)
	}

	dirHujson := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirHujson, "fleet.hujson"), []byte(hujsonStyle), 0o600); err != nil {
		t.Fatalf("seed hujson: %v", err)
	}
	hujsonCfg, err := LoadConfig(dirHujson)
	if err != nil {
		t.Fatalf("LoadConfig(hujson): %v", err)
	}

	plainCfg.LoadedFrom = ""
	hujsonCfg.LoadedFrom = ""
	if !reflect.DeepEqual(plainCfg, hujsonCfg) {
		t.Errorf("HuJson and plain JSON parsed to different Configs:\nplain:  %+v\nhujson: %+v", plainCfg, hujsonCfg)
	}
}

// TestLoadConfig_HujsonExtensionWins confirms .hujson is preferred when only
// it exists, and that LoadedFrom names the actual file.
func TestLoadConfig_HujsonExtensionWins(t *testing.T) {
	dir := t.TempDir()
	body := `{"version": 1, "repos": {}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "fleet.hujson"), []byte(body), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !strings.HasSuffix(cfg.LoadedFrom, "fleet.hujson") {
		t.Errorf("LoadedFrom = %q, want suffix fleet.hujson", cfg.LoadedFrom)
	}
}

// TestLoadConfig_BothExtensionsError confirms an ambiguous-files setup is
// rejected at load time rather than silently picking one.
func TestLoadConfig_BothExtensionsError(t *testing.T) {
	dir := t.TempDir()
	body := `{"version": 1, "repos": {}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(body), 0o600); err != nil {
		t.Fatalf("seed json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fleet.hujson"), []byte(body), 0o600); err != nil {
		t.Fatalf("seed hujson: %v", err)
	}
	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("LoadConfig: want error for both files present, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ambiguous") {
		t.Errorf("error %q does not mention ambiguity", msg)
	}
	if !strings.Contains(msg, "fleet.json") || !strings.Contains(msg, "fleet.hujson") {
		t.Errorf("error %q does not name both files", msg)
	}
}

// TestLoadTemplates_HuJsonSyntax confirms templates also accept HuJson syntax.
func TestLoadTemplates_HuJsonSyntax(t *testing.T) {
	dir := t.TempDir()
	body := `{
  // catalog of upstream workflows
  "version": 1,
  "fetched_at": "2026-05-10T00:00:00Z",
  "sources": {},
  "evaluations": {
    "noisy-wf": {"verdict": "skip", "reason": "too chatty"},
  },
}
`
	if err := os.WriteFile(filepath.Join(dir, "templates.hujson"), []byte(body), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tpl, err := LoadTemplates(dir)
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	if tpl.Version != 1 {
		t.Errorf("version = %d, want 1", tpl.Version)
	}
	if _, ok := tpl.Evaluations["noisy-wf"]; !ok {
		t.Errorf("evaluations missing noisy-wf entry: %+v", tpl.Evaluations)
	}
}

// TestProbeConfigPath_Behavior covers the three branches of probeConfigPath
// directly so failures pinpoint the helper rather than a higher-level caller.
func TestProbeConfigPath_Behavior(t *testing.T) {
	t.Run("neither exists", func(t *testing.T) {
		dir := t.TempDir()
		path, exists, err := probeConfigPath(dir, configBase)
		if err != nil {
			t.Fatalf("probe: %v", err)
		}
		if exists {
			t.Errorf("exists = true, want false")
		}
		if !strings.HasSuffix(path, ".json") {
			t.Errorf("default path %q, want .json suffix", path)
		}
	})
	t.Run("only json exists", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, ConfigFile), []byte("{}"), 0o600)
		path, exists, err := probeConfigPath(dir, configBase)
		if err != nil || !exists || !strings.HasSuffix(path, ".json") {
			t.Errorf("got (%q, %v, %v); want .json+exists+nil", path, exists, err)
		}
	})
	t.Run("only hujson exists", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "fleet.hujson"), []byte("{}"), 0o600)
		path, exists, err := probeConfigPath(dir, configBase)
		if err != nil || !exists || !strings.HasSuffix(path, ".hujson") {
			t.Errorf("got (%q, %v, %v); want .hujson+exists+nil", path, exists, err)
		}
	})
}

// TestSaveTemplates_PreservesEvaluationsComments confirms that calling
// SaveTemplates against an existing templates.json with operator-authored
// comments under /evaluations leaves those comments intact while still
// updating /sources and /fetched_at.
func TestSaveTemplates_PreservesEvaluationsComments(t *testing.T) {
	dir := t.TempDir()
	const sentinel = "// SENTINEL: keep this comment across SaveTemplates"
	original := `{
  "version": 1,
  "fetched_at": "2025-01-01T00:00:00Z",
  "sources": {
    "upstream/old": {"ref_fetched": "main", "workflows": []}
  },
  "evaluations": {
    ` + sentinel + `
    "noisy-wf": {"verdict": "skip", "reason": "too chatty"}
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, TemplatesFile), []byte(original), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	loaded, err := LoadTemplates(dir)
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	loaded.Sources = map[string]TemplateSource{
		"upstream/new": {RefFetched: "v2.0.0", Workflows: nil},
	}

	if err := SaveTemplates(dir, loaded); err != nil {
		t.Fatalf("SaveTemplates: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(dir, TemplatesFile))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	out := string(written)
	if !strings.Contains(out, sentinel) {
		t.Errorf("SENTINEL comment was lost; output:\n%s", out)
	}
	if !strings.Contains(out, "upstream/new") {
		t.Errorf("new source not written; output:\n%s", out)
	}
	if strings.Contains(out, "upstream/old") {
		t.Errorf("old source should have been replaced; output:\n%s", out)
	}
}

// TestAdd_Apply_PreservesExistingReposAndComments is the regression test for
// the latent overwrite bug: a second `Add` against a fleet.local.json
// containing prior repos and operator comments must keep both.
func TestAdd_Apply_PreservesExistingReposAndComments(t *testing.T) {
	dir := t.TempDir()
	_ = seedBaseConfigWithDefaultProfile(t, dir)

	const sentinel = "// SENTINEL: pin chosen because v1.0.1 broke webhook handling"
	existingLocal := `{
  "version": 1,
  "repos": {
    ` + sentinel + `
    "rshade/first": {"profiles": ["default"]}
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, LocalConfigFile), []byte(existingLocal), 0o600); err != nil {
		t.Fatalf("seed local: %v", err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	res, err := Add(cfg, AddOptions{
		Repo:      "rshade/second",
		Profiles:  []string{"default"},
		Apply:     true,
		Confirmed: true,
		Dir:       dir,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !res.WroteLocal {
		t.Errorf("WroteLocal = false, want true")
	}

	written, err := os.ReadFile(filepath.Join(dir, LocalConfigFile))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	out := string(written)
	if !strings.Contains(out, "rshade/first") {
		t.Errorf("prior repo rshade/first was dropped; output:\n%s", out)
	}
	if !strings.Contains(out, "rshade/second") {
		t.Errorf("new repo rshade/second was not written; output:\n%s", out)
	}
	if !strings.Contains(out, sentinel) {
		t.Errorf("SENTINEL comment was lost; output:\n%s", out)
	}

	reloaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig after Add: %v", err)
	}
	if _, ok := reloaded.Repos["rshade/first"]; !ok {
		t.Errorf("rshade/first missing after reload")
	}
	if _, ok := reloaded.Repos["rshade/second"]; !ok {
		t.Errorf("rshade/second missing after reload")
	}
}

// TestBillingMetadata_RoundTrip covers SC-006: a Config carrying both
// Profile.Tier and RepoSpec.CostCenter round-trips through json.Marshal →
// json.Unmarshal with deep equality preserved. Also exercises the
// omitempty contract: a profile/repo with no annotation marshals without
// the corresponding key and re-loads to the zero value.
func TestBillingMetadata_RoundTrip(t *testing.T) {
	cfg := Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"tiered": {
				Tier:      "premium",
				Sources:   map[string]SourcePin{"u/x": {Ref: "v1"}},
				Workflows: []ProfileWorkflow{},
			},
			"untiered": {
				Sources:   map[string]SourcePin{"u/x": {Ref: "v1"}},
				Workflows: []ProfileWorkflow{},
			},
		},
		Repos: map[string]RepoSpec{
			"o/with-cc":    {Profiles: []string{"tiered"}, CostCenter: "platform-eng"},
			"o/without-cc": {Profiles: []string{"untiered"}},
		},
	}

	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	out := string(data)
	if !strings.Contains(out, `"tier":"premium"`) {
		t.Errorf("Marshal output missing tiered profile tier: %s", out)
	}
	if !strings.Contains(out, `"cost_center":"platform-eng"`) {
		t.Errorf("Marshal output missing cost_center for o/with-cc: %s", out)
	}
	if strings.Contains(out, `"tier":""`) {
		t.Errorf("Marshal emitted empty-string tier (omitempty should drop it): %s", out)
	}
	if strings.Contains(out, `"cost_center":""`) {
		t.Errorf("Marshal emitted empty-string cost_center (omitempty should drop it): %s", out)
	}

	var back Config
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(cfg.Profiles, back.Profiles) {
		t.Errorf("Profiles round-trip mismatch:\n  cfg:  %+v\n  back: %+v", cfg.Profiles, back.Profiles)
	}
	if !reflect.DeepEqual(cfg.Repos, back.Repos) {
		t.Errorf("Repos round-trip mismatch:\n  cfg:  %+v\n  back: %+v", cfg.Repos, back.Repos)
	}
}

// TestProbeConfigPath_AmbiguousReturnsTypedError confirms the ambiguous-files
// case surfaces an error rather than silently returning one of the paths.
func TestProbeConfigPath_AmbiguousReturnsTypedError(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, ConfigFile), []byte("{}"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "fleet.hujson"), []byte("{}"), 0o600)
	_, _, err := probeConfigPath(dir, configBase)
	if err == nil {
		t.Fatal("want error for both files present, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("err = %v, want substring 'ambiguous'", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("err should not classify as not-exist: %v", err)
	}
}
