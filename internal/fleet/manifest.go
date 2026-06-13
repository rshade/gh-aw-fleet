package fleet

// manifest.go defines the fleet-owned deployment record written into each managed
// repository at .github/aw/fleet-manifest.json. It provides types and helpers for
// writing the manifest during deploy, reading it during status, and comparing
// versions to detect stale init artifacts.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// FleetManifestPath is the repo-relative path of the fleet manifest file
// written into each managed repository on every deploy.
const FleetManifestPath = ".github/aw/fleet-manifest.json"

// fleetRepoSlug identifies this fleet tool's own repository. Embedded in every
// fleet manifest under the "fleet" key so operators can trace which tool wrote a
// given manifest. Unexported because it is a tool-identity constant, not a
// per-fleet-config value; promote to Config if multi-fleet deployments arise.
const fleetRepoSlug = "rshade/gh-aw-fleet"

// Version-drift state constants emitted on VersionDrift.State.
const (
	// VersionDriftBehind means the manifest records an older gh-aw version than
	// the fleet currently declares. Init artifacts may be stale; deploy/sync will
	// re-run gh aw init on the next apply.
	VersionDriftBehind = "behind"
	// VersionDriftCurrent means the manifest version matches the fleet's current
	// github/gh-aw source pin. No refresh needed.
	VersionDriftCurrent = "current"
	// VersionDriftUnmanaged means no manifest is present or the manifest is
	// unreadable. The repo has either never been fleet-deployed or was deployed
	// before this feature shipped.
	VersionDriftUnmanaged = "unmanaged"
)

// FleetManifest is the fleet-owned deployment record written into each managed
// repository at FleetManifestPath. It records the provenance of the most recent
// fleet-driven deploy so that status, sync, and deploy can detect and remediate
// version drift without cloning the target repo.
// alongside VersionDrift and other manifest-adjacent types in this package.
//
//nolint:revive // fleet.FleetManifest is intentional: "Manifest" alone is ambiguous
type FleetManifest struct {
	// Managed is always true. Its presence distinguishes fleet-written manifests
	// from any accidental file at the same path.
	Managed bool `json:"managed"`
	// Fleet is the fleet tool's own repository slug, e.g. "rshade/gh-aw-fleet".
	Fleet string `json:"fleet"`
	// GhAwVersion is the github/gh-aw SOURCE pin from fleet.json used for this
	// deploy — the ref of the first resolved github/gh-aw workflow (see research.md
	// R1 for the multi-profile resolution decision). Empty when the repo has no
	// github/gh-aw-sourced workflows declared.
	GhAwVersion string `json:"gh_aw_version"`
	// CLIVersion is the output of `gh aw --version` at deploy time. It is the
	// runtime-artifact provenance, distinct from the source pin, and is recorded
	// for diagnostics only — not used for drift comparison.
	CLIVersion string `json:"cli_version"`
	// Profiles is the sorted list of profile names active for this repo at deploy
	// time. Sorted to ensure stable content-equality checks across redeployes.
	Profiles []string `json:"profiles"`
	// DeployedAt is the RFC3339 UTC timestamp of the deploy that last changed
	// manifest content. It is NOT updated on same-version redeployes; see
	// writeManifestIfNeeded.
	DeployedAt time.Time `json:"deployed_at"`
}

// VersionDrift describes the version-drift state of a managed repo's init
// artifacts relative to the fleet's current github/gh-aw source pin.
type VersionDrift struct {
	// State is one of VersionDriftBehind, VersionDriftCurrent, VersionDriftUnmanaged.
	State string `json:"state"`
	// RecordedVersion is the GhAwVersion from the repo's manifest. Empty when
	// State == VersionDriftUnmanaged.
	RecordedVersion string `json:"recorded_version"`
	// ExpectedVersion is the fleet's current github/gh-aw pin for this repo.
	// Empty when the repo has no github/gh-aw-sourced workflows declared.
	ExpectedVersion string `json:"expected_version"`
}

// resolvedGhAwPin returns the github/gh-aw source pin (ref) for the given repo
// from the fleet config. It returns the Ref of the first resolved workflow whose
// Source is "github/gh-aw". Returns an empty string when the repo has no
// github/gh-aw-sourced workflows or when ResolveRepoWorkflows fails.
func resolvedGhAwPin(cfg *Config, repo string) string {
	workflows, err := cfg.ResolveRepoWorkflows(repo)
	if err != nil {
		return ""
	}
	for _, w := range workflows {
		if w.Source == sourceGitHubAW {
			return w.Ref
		}
	}
	return ""
}

// buildManifest constructs a FleetManifest for repo from the fleet config and
// the CLI version string. Profiles are sorted for stable equality checks.
// DeployedAt is set to time.Now().UTC(); callers use writeManifestIfNeeded to
// avoid writing when content has not changed.
func buildManifest(cfg *Config, repo, cliVersion string) *FleetManifest {
	spec := cfg.Repos[repo]
	profiles := make([]string, len(spec.Profiles))
	copy(profiles, spec.Profiles)
	slices.Sort(profiles)
	return &FleetManifest{
		Managed:     true,
		Fleet:       fleetRepoSlug,
		GhAwVersion: resolvedGhAwPin(cfg, repo),
		CLIVersion:  cliVersion,
		Profiles:    profiles,
		DeployedAt:  time.Now().UTC(),
	}
}

// manifestEqualExceptTime reports whether a and b are equal in every field
// except DeployedAt. Used by writeManifestIfNeeded to suppress same-version
// redeploy churn.
func manifestEqualExceptTime(a, b *FleetManifest) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Managed == b.Managed &&
		a.Fleet == b.Fleet &&
		a.GhAwVersion == b.GhAwVersion &&
		a.CLIVersion == b.CLIVersion &&
		slices.Equal(a.Profiles, b.Profiles)
}

// readManifestFromClone reads and parses the fleet manifest from a local clone
// directory. Returns (nil, nil) when the file does not exist or when the parsed
// manifest has Managed==false. Returns (nil, err) for I/O or JSON parse errors.
func readManifestFromClone(dir string) (*FleetManifest, error) {
	path := filepath.Join(dir, FleetManifestPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil //nolint:nilnil // ENOENT means "not yet deployed" — callers check m != nil
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m FleetManifest
	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if !m.Managed {
		return nil, nil //nolint:nilnil // managed:false means "not ours" — callers check m != nil
	}
	return &m, nil
}

// writeManifestToClone marshals m as indented JSON and writes it to
// FleetManifestPath under dir. Parent directories are created as needed.
// The file is written with mode 0600 and ends with a newline.
func writeManifestToClone(dir string, m *FleetManifest) error {
	path := filepath.Join(dir, FleetManifestPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')
	err = os.WriteFile(path, data, 0o600)
	if err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// writeManifestIfNeeded writes m to FleetManifestPath in dir only when the
// existing manifest (if any) differs from m in a field other than DeployedAt.
// Returns (true, nil) when the file was written, (false, nil) when the content
// was already current (same-version redeploy), and (false, err) on I/O failure.
func writeManifestIfNeeded(dir string, m *FleetManifest) (bool, error) {
	existing, err := readManifestFromClone(dir)
	if err == nil && existing != nil && manifestEqualExceptTime(existing, m) {
		return false, nil
	}
	return true, writeManifestToClone(dir, m)
}

// parseManifestJSON parses a raw JSON string returned by the GitHub contents
// API into a FleetManifest. Returns (nil, nil) for an empty body or when the
// parsed manifest has Managed==false (both are "not our manifest" cases, not
// errors). Returns (nil, err) for malformed JSON. Callers should check the
// returned pointer for nil. Used by the status command to read manifests
// without cloning.
func parseManifestJSON(body string) (*FleetManifest, error) {
	if body == "" {
		return nil, nil //nolint:nilnil // empty body means "no manifest fetched" — callers check m != nil
	}
	var m FleetManifest
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if !m.Managed {
		return nil, nil //nolint:nilnil // managed:false means "not ours" — callers check m != nil
	}
	return &m, nil
}

// computeVersionDrift returns the VersionDrift for a repo given its manifest
// and the fleet's current expected github/gh-aw version. A nil manifest
// produces VersionDriftUnmanaged. Non-matching versions produce
// VersionDriftBehind (note: "ahead" is not a distinct state — any mismatch
// triggers a refresh on the next deploy).
func computeVersionDrift(m *FleetManifest, expectedVersion string) *VersionDrift {
	if m == nil {
		return &VersionDrift{
			State:           VersionDriftUnmanaged,
			RecordedVersion: "",
			ExpectedVersion: expectedVersion,
		}
	}
	state := VersionDriftCurrent
	if m.GhAwVersion != expectedVersion {
		state = VersionDriftBehind
	}
	return &VersionDrift{
		State:           state,
		RecordedVersion: m.GhAwVersion,
		ExpectedVersion: expectedVersion,
	}
}
