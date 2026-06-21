package fleet_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/rshade/gh-aw-fleet/pkg/fleet"
)

//nolint:gochecknoglobals // test flag registered at package scope so the standard test harness parses it
var updateGolden = flag.Bool("update", false, "update golden testdata")

func TestGoldenRoundTrip(t *testing.T) {
	root := filepath.Join("..", "..")
	inputPath := filepath.Join(root, "fleet.example.json")
	goldenPath := filepath.Join("testdata", "config.canonical.json")

	input, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read fleet example: %v", err)
	}

	var cfg fleet.Config
	if err := json.Unmarshal(input, &cfg); err != nil {
		t.Fatalf("unmarshal fleet example: %v", err)
	}
	cfg.LoadedFrom = "fleet.example.json"

	got, err := marshalCanonical(cfg)
	if err != nil {
		t.Fatalf("marshal canonical config: %v", err)
	}
	if bytes.Contains(got, []byte("LoadedFrom")) || bytes.Contains(got, []byte("loaded_from")) {
		t.Fatalf("serialized config includes LoadedFrom: %s", got)
	}

	if updateGoldenEnabled() {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("create testdata dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("canonical config mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestConfigJSONTags(t *testing.T) {
	mustMarshal := func(t *testing.T, v any) []byte {
		t.Helper()
		out, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return out
	}

	cfg := fleet.Config{
		Version:    fleet.SchemaVersion,
		Profiles:   map[string]fleet.Profile{},
		LoadedFrom: "fleet.local.json",
	}
	out := mustMarshal(t, cfg)

	if bytes.Contains(out, []byte("defaults")) {
		t.Errorf("zero Defaults field was serialized: %s", out)
	}
	if bytes.Contains(out, []byte("profiles")) {
		t.Errorf("empty Profiles map was serialized: %s", out)
	}
	if !bytes.Contains(out, []byte(`"repos":null`)) {
		t.Errorf("nil Repos field was omitted; got %s", out)
	}
	if bytes.Contains(out, []byte("LoadedFrom")) || bytes.Contains(out, []byte("loaded_from")) {
		t.Errorf("LoadedFrom field was serialized: %s", out)
	}

	repo := fleet.RepoSpec{
		Profiles:            []string{"default"},
		ExtraWorkflows:      []fleet.ExtraWorkflow{},
		ExcludeFromProfiles: []string{},
	}
	repoOut := mustMarshal(t, repo)
	if bytes.Contains(repoOut, []byte("extra")) {
		t.Errorf("empty ExtraWorkflows slice was serialized: %s", repoOut)
	}
	if bytes.Contains(repoOut, []byte("exclude")) {
		t.Errorf("empty ExcludeFromProfiles slice was serialized: %s", repoOut)
	}
}

func updateGoldenEnabled() bool {
	return *updateGolden
}

func marshalCanonical(cfg fleet.Config) ([]byte, error) {
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
