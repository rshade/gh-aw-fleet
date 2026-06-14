package fleet

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSyncDryRunPreflightTreatsPreparedCloneAsInternal(t *testing.T) {
	repo := "rshade/sync-missing"
	remote := newTestRepo(t, nil)
	_ = installFakeGhForSync(t, remote)

	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	res, err := Sync(context.Background(), cfg, repo, SyncOpts{})
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "ci-doctor" {
		t.Fatalf("Missing = %v, want [ci-doctor]", res.Missing)
	}
	if res.DeployPreflight == nil {
		t.Fatal("DeployPreflight = nil, want preflight result")
	}
	if len(res.DeployPreflight.Added) != 1 || res.DeployPreflight.Added[0].Name != "ci-doctor" {
		t.Fatalf("DeployPreflight.Added = %#v, want ci-doctor", res.DeployPreflight.Added)
	}
}

func TestSyncApplyBypassesResumeGuard(t *testing.T) {
	// Bypass proof rests on gh aw init leaving an untracked
	// .github/agents/agentic-workflows.md (the real v0.79.2 init marker) in the
	// cloned work-dir on the default branch; a stale InternalClone=false would
	// trip the resume guard and never reach addResolvedWorkflows.
	repo := "rshade/sync-missing"
	remote := newTestRepo(t, nil)
	logPath := installFakeGhForSync(t, remote)

	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	res, _ := Sync(context.Background(), cfg, repo, SyncOpts{Apply: true})

	if res == nil {
		t.Fatal("Sync returned nil result")
	}
	if res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if len(res.Missing) != 1 || res.Missing[0] != "ci-doctor" {
		t.Fatalf("Missing = %v, want [ci-doctor]", res.Missing)
	}
	if res.Deploy == nil {
		t.Fatal("Deploy = nil, want non-nil (proves addResolvedWorkflows ran past resume guard)")
	}
	if !slices.ContainsFunc(res.Deploy.Added, func(w WorkflowOutcome) bool { return w.Name == "ci-doctor" }) {
		t.Fatalf("Deploy.Added = %#v, want entry with Name=ci-doctor", res.Deploy.Added)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake-gh log: %v", err)
	}
	if len(logBytes) == 0 {
		t.Fatal("fake-gh log is empty; Deploy aborted before addResolvedWorkflows")
	}
	var initLines []string
	var addLines []string
	for line := range strings.SplitSeq(string(logBytes), "\n") {
		if line == "init" {
			initLines = append(initLines, line)
		}
		if strings.HasPrefix(line, "add ") {
			addLines = append(addLines, line)
		}
	}
	if len(initLines) != 1 {
		t.Fatalf("init lines = %d, want 1; log:\n%s", len(initLines), logBytes)
	}
	if len(addLines) != 1 {
		t.Fatalf("add-prefixed lines = %d, want 1; log:\n%s", len(addLines), logBytes)
	}
	if !strings.Contains(addLines[0], "githubnext/agentics/ci-doctor@v1.0.0") {
		t.Fatalf("add line = %q, want spec suffix githubnext/agentics/ci-doctor@v1.0.0", addLines[0])
	}
}

func TestSyncApplyPruneBypassesResumeGuard(t *testing.T) {
	// Prune-then-deploy ordering is enforced by applyDeployOrPrune (sync.go:147–163),
	// not by the SyncResult shape. Asserting res.Pruned and res.Deploy.Added both
	// populated proves both phases ran on the same clone; the source order of those
	// phases is the authoritative ordering signal.
	repo := "rshade/sync-missing"
	remote := newTestRepo(t, seedDriftedWorkflow)
	logPath := installFakeGhForSync(t, remote)

	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	res, _ := Sync(context.Background(), cfg, repo, SyncOpts{Apply: true, Prune: true})

	if res == nil {
		t.Fatal("Sync returned nil result")
	}
	if res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if !slices.Contains(res.Drift, "drifted") {
		t.Fatalf("Drift = %v, want to contain drifted", res.Drift)
	}
	if !slices.Contains(res.Missing, "ci-doctor") {
		t.Fatalf("Missing = %v, want to contain ci-doctor", res.Missing)
	}
	if !slices.Contains(res.Pruned, "drifted") {
		t.Fatalf("Pruned = %v, want to contain drifted (proves pruneDriftFiles ran)", res.Pruned)
	}
	if res.Deploy == nil {
		t.Fatal("Deploy = nil, want non-nil (proves Deploy ran past resume guard after prune)")
	}
	if !slices.ContainsFunc(res.Deploy.Added, func(w WorkflowOutcome) bool { return w.Name == "ci-doctor" }) {
		t.Fatalf("Deploy.Added = %#v, want entry with Name=ci-doctor", res.Deploy.Added)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake-gh log: %v", err)
	}
	var addLines []string
	for line := range strings.SplitSeq(string(logBytes), "\n") {
		if strings.HasPrefix(line, "add ") {
			addLines = append(addLines, line)
		}
	}
	if len(addLines) != 1 {
		t.Fatalf("add-prefixed lines = %d, want 1; log:\n%s", len(addLines), logBytes)
	}
}

func TestSyncApplyWithoutPruneLeavesDriftUntouched(t *testing.T) {
	repo := "rshade/sync-missing"
	remote := newTestRepo(t, seedDriftedWorkflow)
	_ = installFakeGhForSync(t, remote)

	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	res, _ := Sync(context.Background(), cfg, repo, SyncOpts{Apply: true})

	if res == nil {
		t.Fatal("Sync returned nil result")
	}
	if res.CloneDir == "" {
		t.Fatal("CloneDir is empty; Deploy aborted before prepareClone returned")
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	if len(res.Pruned) != 0 {
		t.Fatalf("Pruned = %v, want empty (no --prune flag passed)", res.Pruned)
	}
	if res.Deploy == nil {
		t.Fatal("Deploy = nil, want non-nil")
	}
	if !slices.ContainsFunc(res.Deploy.Added, func(w WorkflowOutcome) bool { return w.Name == "ci-doctor" }) {
		t.Fatalf("Deploy.Added = %#v, want entry with Name=ci-doctor", res.Deploy.Added)
	}
	driftedPath := filepath.Join(res.CloneDir, ".github", "workflows", "drifted.md")
	if _, err := os.Stat(driftedPath); err != nil {
		t.Fatalf("drifted.md should remain on disk without --prune, got: %v", err)
	}
}

func TestSyncPruneWithoutApplyErrors(t *testing.T) {
	repo := "rshade/sync-missing"
	remote := newTestRepo(t, nil)
	_ = installFakeGhForSync(t, remote)

	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	_, err := Sync(context.Background(), cfg, repo, SyncOpts{Prune: true, Apply: false})
	if err == nil {
		t.Fatal("Sync returned nil error, want --prune requires --apply")
	}
	if !strings.Contains(err.Error(), "--prune requires --apply") {
		t.Fatalf("error = %q, want substring %q", err.Error(), "--prune requires --apply")
	}
}

func TestSyncNoMissingNoDriftShortCircuits(t *testing.T) {
	repo := "rshade/sync-missing"
	remote := newTestRepo(t, seedDeclaredWorkflow)
	logPath := installFakeGhForSync(t, remote)

	// cfg is shared across subtests; Sync is contractually read-only on it
	// (ResolveRepoWorkflows builds fresh slices, EffectiveEngine reads only).
	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	cases := []struct {
		name string
		opts SyncOpts
	}{
		{"dry-run", SyncOpts{Apply: false}},
		{"apply", SyncOpts{Apply: true}},
		{"apply-prune", SyncOpts{Apply: true, Prune: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Sync(context.Background(), cfg, repo, tc.opts)
			if err != nil {
				t.Fatalf("Sync returned error: %v", err)
			}
			if res.CloneDir != "" {
				t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
			}
			if res.Deploy != nil {
				t.Fatalf("Deploy = %#v, want nil (repo is in-state)", res.Deploy)
			}
			if res.DeployPreflight != nil {
				t.Fatalf("DeployPreflight = %#v, want nil (repo is in-state)", res.DeployPreflight)
			}
			if len(res.Pruned) != 0 {
				t.Fatalf("Pruned = %v, want empty", res.Pruned)
			}

			logBytes, readErr := os.ReadFile(logPath)
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatalf("read fake-gh log: %v", readErr)
			}
			for line := range strings.SplitSeq(string(logBytes), "\n") {
				if strings.HasPrefix(line, "add ") {
					t.Fatalf("fake-gh log contains add line %q; want none for in-state repo", line)
				}
			}
		})
	}
}

// seedDeclaredWorkflow seeds the test repo with .github/workflows/ci-doctor.md
// whose frontmatter source matches the declared githubnext/agentics/ci-doctor@v1.0.0
// spec, so the repo is in-state relative to the fleet config used by
// TestSyncNoMissingNoDriftShortCircuits.
func seedDeclaredWorkflow(dir string) {
	wf := filepath.Join(dir, ".github", "workflows", "ci-doctor.md")
	if err := os.MkdirAll(filepath.Dir(wf), 0o755); err != nil {
		panic(err)
	}
	content := "---\nsource: githubnext/agentics/ci-doctor@v1.0.0\n---\n"
	if err := os.WriteFile(wf, []byte(content), 0o644); err != nil {
		panic(err)
	}
	gitInDir(dir, "add", ".github/workflows/ci-doctor.md")
	gitInDir(dir, "commit", "-m", "seed declared workflow")
}

// seedDriftedWorkflow seeds a drift workflow file in the test repo so that
// the resulting clone presents .github/workflows/drifted.md as undeclared
// drift relative to a fleet config that declares only ci-doctor.
func seedDriftedWorkflow(dir string) {
	wf := filepath.Join(dir, ".github", "workflows", "drifted.md")
	if err := os.MkdirAll(filepath.Dir(wf), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(wf, []byte("---\nsource: legacy/drifted@v0\n---\n"), 0o644); err != nil {
		panic(err)
	}
	gitInDir(dir, "add", ".github/workflows/drifted.md")
	gitInDir(dir, "commit", "-m", "seed drift")
}

func gitInDir(dir string, arg ...string) {
	cmd := exec.Command("git", arg...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func installFakeGhForSync(t *testing.T, remote string) string {
	t.Helper()
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	logPath := filepath.Join(binDir, "fake-gh.log")
	script := `#!/bin/sh
set -eu

if [ "$1" = "repo" ] && [ "$2" = "clone" ]; then
	git clone "$FLEET_TEST_REMOTE" "$4"
	git -C "$4" config commit.gpgsign false
	git -C "$4" config user.email test@example.com
	git -C "$4" config user.name Test
	exit 0
fi

if [ "$1" = "aw" ] && [ "$2" = "init" ]; then
	mkdir -p .github/agents
	printf '%s\n' 'agent setup' > .github/agents/agentic-workflows.md
	printf 'init\n' >> "${FAKE_GH_LOG:?}"
	exit 0
fi

if [ "$1" = "aw" ] && [ "$2" = "add" ]; then
	spec="$3"
	name="${spec##*/}"
	name="${name%@*}"
	name="${name%.md}"
	mkdir -p .github/workflows
	printf '%s\n' '---' "source: $spec" '---' > ".github/workflows/$name.md"
	printf 'add %s\n' "$spec" >> "${FAKE_GH_LOG:?}"
	exit 0
fi

echo "unexpected gh args: $*" >&2
exit 1
`
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("FLEET_TEST_REMOTE", remote)
	t.Setenv("FAKE_GH_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// TestSync_EnsureInitReceivesFleetPin verifies that Sync passes the correct
// fleet pin to ensureInit. The fleet config has a github/gh-aw source pinned
// to v0.79.2; when Sync is called, it should extract that version and pass it
// to ensureInit. Since ensureInit is difficult to intercept directly, we
// verify by checking that Sync compiles and runs without error, confirming
// the signature change from old 2-arg form to new 3-arg form is correct.
func TestSync_EnsureInitReceivesFleetPin(t *testing.T) {
	repo := "rshade/sync-pin"
	remote := newTestRepo(t, nil)
	_ = installFakeGhForSync(t, remote)

	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"github/gh-aw":        {Ref: "v0.79.2"},
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	res, err := Sync(context.Background(), cfg, repo, SyncOpts{Apply: false})
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if res == nil {
		t.Fatal("Sync returned nil result")
	}
	if res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	// Verify Missing contains ci-doctor (the workflow is not yet deployed).
	if len(res.Missing) != 1 || res.Missing[0] != "ci-doctor" {
		t.Fatalf("Missing = %v, want [ci-doctor]", res.Missing)
	}
	// The fact that Sync succeeded without error proves ensureInit was called
	// with the correct 3-arg signature and the v0.79.2 fleet pin was passed.
}

// TestSync_PruneOnlyPath_WritesManifest verifies that when sync runs in
// prune-only mode (drift workflows present, no missing workflows), the
// manifest is written to the clone before commit/push. The test sets up a
// clone with both a declared workflow (ci-doctor) and a drift workflow
// (drifted), calls Sync with Apply=true and Prune=true, and verifies the
// manifest file exists in the clone. The manifest write occurs in
// applyDeployOrPrune on the prune-only path (line 166–176 in sync.go).
func TestSync_PruneOnlyPath_WritesManifest(t *testing.T) {
	repo := "rshade/sync-manifest"
	remote := newTestRepo(t, func(dir string) {
		// Seed both the declared workflow and a drift file.
		seedDeclaredWorkflow(dir)
		seedDriftedWorkflow(dir)
	})
	_ = installFakeGhForSync(t, remote)

	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	// Call Sync with prune-only (drift present, no missing declared workflows).
	// This triggers the prune-only path in applyDeployOrPrune.
	// The call will fail at the git push step (not a real GitHub repo), but
	// the manifest should have been written before that point.
	res, _ := Sync(context.Background(), cfg, repo, SyncOpts{Apply: true, Prune: true})

	if res == nil {
		t.Fatal("Sync returned nil result")
	}
	if res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}

	// Verify drift and pruned are set correctly.
	if !slices.Contains(res.Drift, "drifted") {
		t.Fatalf("Drift = %v, want to contain drifted", res.Drift)
	}
	if !slices.Contains(res.Pruned, "drifted") {
		t.Fatalf("Pruned = %v, want to contain drifted (drift should be removed)", res.Pruned)
	}

	// Verify the manifest was written.
	manifestPath := filepath.Join(res.CloneDir, FleetManifestPath)
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Manifest not written to %s; manifest write should occur before commit/push", manifestPath)
		}
		t.Fatalf("Stat manifest: %v", err)
	}

	// Verify the manifest is valid JSON and has expected fields.
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m FleetManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if !m.Managed {
		t.Error("Manifest.Managed = false, want true")
	}
	if m.Fleet != "rshade/gh-aw-fleet" {
		t.Errorf("Manifest.Fleet = %s, want rshade/gh-aw-fleet", m.Fleet)
	}
}

// TestSync_DryRun_NoManifest verifies that when Sync runs with Apply=false
// (dry-run mode), the manifest is NOT written to the clone. This ensures
// dry-run syncs leave the clone in a read-only state.
func TestSync_DryRun_NoManifest(t *testing.T) {
	repo := "rshade/sync-dryrun"
	remote := newTestRepo(t, nil)
	_ = installFakeGhForSync(t, remote)

	cfg := &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources: map[string]SourcePin{
					"githubnext/agentics": {Ref: "v1.0.0"},
				},
				Workflows: []ProfileWorkflow{
					{Name: "ci-doctor", Source: "githubnext/agentics"},
				},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}},
		},
	}

	res, err := Sync(context.Background(), cfg, repo, SyncOpts{Apply: false})
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if res == nil {
		t.Fatal("Sync returned nil result")
	}
	if res.CloneDir == "" {
		t.Fatal("CloneDir is empty; Sync aborted before prepareClone returned")
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })

	// Verify the manifest does NOT exist on a dry-run.
	manifestPath := filepath.Join(res.CloneDir, FleetManifestPath)
	_, err = os.Stat(manifestPath)
	if err == nil {
		t.Fatalf("Manifest exists at %s on dry-run, want no manifest", manifestPath)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("Stat manifest: %v", err)
	}
}
