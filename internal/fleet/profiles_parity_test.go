package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tailscale/hujson"
)

// TestProfilesDefaultParity enforces the CLAUDE.md invariant: the `default`
// profile in fleet.json must match profiles/default.json verbatim. Both
// files are passed through hujson.Standardize first so HuJson syntax in
// either file does not cause spurious drift; comparison is structural
// (Go map/slice equality after re-marshaling), so reordering of keys
// inside maps does not count as drift.
//
// When this test fails, choose one side as authoritative and align the
// other. Both files are operator-curated; neither is generated.
func TestProfilesDefaultParity(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot locate repo root for parity check: %v", err)
	}

	fleetData, err := os.ReadFile(filepath.Join(repoRoot, ConfigFile))
	if err != nil {
		t.Fatalf("read fleet.json: %v", err)
	}
	fleetData, err = hujson.Standardize(fleetData)
	if err != nil {
		t.Fatalf("standardize fleet.json: %v", err)
	}
	var fleetWrapper struct {
		Profiles map[string]json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(fleetData, &fleetWrapper); err != nil {
		t.Fatalf("parse fleet.json: %v", err)
	}
	defaultRaw, ok := fleetWrapper.Profiles["default"]
	if !ok {
		t.Fatal("fleet.json has no profiles.default")
	}

	profileData, err := os.ReadFile(filepath.Join(repoRoot, "profiles", "default.json"))
	if err != nil {
		t.Fatalf("read profiles/default.json: %v", err)
	}
	profileData, err = hujson.Standardize(profileData)
	if err != nil {
		t.Fatalf("standardize profiles/default.json: %v", err)
	}

	var fromFleet, fromFile any
	if err := json.Unmarshal(defaultRaw, &fromFleet); err != nil {
		t.Fatalf("re-parse fleet default: %v", err)
	}
	if err := json.Unmarshal(profileData, &fromFile); err != nil {
		t.Fatalf("re-parse profiles/default.json: %v", err)
	}

	if !reflect.DeepEqual(fromFleet, fromFile) {
		fleetPretty, _ := json.MarshalIndent(fromFleet, "", "  ")
		filePretty, _ := json.MarshalIndent(fromFile, "", "  ")
		t.Errorf(
			"fleet.json default profile differs from profiles/default.json.\n"+
				"--- fleet.json ---\n%s\n--- profiles/default.json ---\n%s",
			fleetPretty, filePretty,
		)
	}
}

// findRepoRoot walks up from the test working directory to locate the repo
// root (identified by go.mod). Returns an error if no go.mod is found
// before reaching the filesystem root.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
