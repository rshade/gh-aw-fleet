package fleet

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

const strictHighRuleID = "fleet.permissions.write-on-schedule"

func TestDeployStrictDryRunPreservesTempClone(t *testing.T) {
	repo := "rshade/strict-deploy-dry-run"
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	_ = installFakeGhForSync(t, remote)
	installHealthyDeployAPIs(t, repo)

	res, err := Deploy(context.Background(), strictGateTestConfig(repo), repo, DeployOpts{
		Security: SecurityOpts{Strict: true},
	})
	assertStrictSecurityError(t, err, repo)
	assertClonePreservedWithFindings(t, res.CloneDir)
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	if !hasStrictHighFinding(res.SecurityFindings) {
		t.Fatalf("SecurityFindings = %#v; want %s", res.SecurityFindings, strictHighRuleID)
	}
	if res.CompileStrictSource != "" {
		t.Fatalf("CompileStrictSource = %q; want empty because strict gate aborts first", res.CompileStrictSource)
	}
}

func TestDeployStrictApplyBlocksBeforeManifestAndPR(t *testing.T) {
	repo := "rshade/strict-deploy-apply"
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	_ = installFakeGhForSync(t, remote)
	installHealthyDeployAPIs(t, repo)
	stubGhAwVersionForStrictTests(t)

	res, err := Deploy(context.Background(), strictGateTestConfig(repo), repo, DeployOpts{
		Apply:    true,
		Security: SecurityOpts{Strict: true},
	})
	assertStrictSecurityError(t, err, repo)
	assertClonePreservedWithFindings(t, res.CloneDir)
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	if res.BranchPushed != "" || res.PRURL != "" {
		t.Fatalf("BranchPushed/PRURL = %q/%q; want no mutation", res.BranchPushed, res.PRURL)
	}
	if _, statErr := os.Stat(filepath.Join(res.CloneDir, FleetManifestPath)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest stat err = %v; want absent because strict gate aborts before manifest", statErr)
	}
}

func TestDeployNonStrictHighFindingRemainsAdvisory(t *testing.T) {
	repo := "rshade/strict-deploy-advisory"
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	_ = installFakeGhForSync(t, remote)
	installHealthyDeployAPIs(t, repo)
	stubGhAwVersionForStrictTests(t)

	res, err := Deploy(context.Background(), strictGateTestConfig(repo), repo, DeployOpts{Apply: true})
	if err == nil {
		t.Fatal("Deploy returned nil error; want later PR creation failure proving advisory flow proceeded")
	}
	if !strings.Contains(err.Error(), "gh pr create") {
		t.Fatalf("Deploy error = %v; want gh pr create after advisory finding", err)
	}
	if res == nil || res.CloneDir == "" {
		t.Fatal("Deploy returned no clone")
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	if !hasStrictHighFinding(res.SecurityFindings) {
		t.Fatalf("SecurityFindings = %#v; want %s", res.SecurityFindings, strictHighRuleID)
	}
	if _, statErr := os.Stat(filepath.Join(res.CloneDir, FleetManifestPath)); statErr != nil {
		t.Fatalf("manifest missing; non-strict advisory flow should reach manifest write: %v", statErr)
	}
}

func TestSyncStrictDelegatedDeployBlocksAndPreservesClone(t *testing.T) {
	repo := "rshade/strict-sync-missing"
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	_ = installFakeGhForSync(t, remote)
	installHealthyDeployAPIs(t, repo)

	res, err := Sync(context.Background(), strictGateMissingWorkflowConfig(repo), repo, SyncOpts{
		Security: SecurityOpts{Strict: true},
	})
	assertStrictSecurityError(t, err, repo)
	assertClonePreservedWithFindings(t, res.CloneDir)
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	if res.DeployPreflight == nil {
		t.Fatal("DeployPreflight = nil; want delegated Deploy strict result")
	}
	if !hasStrictHighFinding(res.SecurityFindings) {
		t.Fatalf("SecurityFindings = %#v; want %s", res.SecurityFindings, strictHighRuleID)
	}
}

func TestSyncStrictPruneOnlyBlocksBeforePruneCommit(t *testing.T) {
	repo := "rshade/strict-sync-prune"
	remote := newTestRepo(t, func(dir string) {
		seedDeclaredWorkflow(dir)
		seedHighSecurityWorkflow(dir)
	})
	_ = installFakeGhForSync(t, remote)

	res, err := Sync(context.Background(), strictGateMissingWorkflowConfig(repo), repo, SyncOpts{
		Apply:    true,
		Prune:    true,
		Security: SecurityOpts{Strict: true},
	})
	assertStrictSecurityError(t, err, repo)
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	if len(res.Pruned) != 0 {
		t.Fatalf("Pruned = %v; want empty because strict gate runs before prune", res.Pruned)
	}
	if !slices.Contains(res.Drift, "strict-high") {
		t.Fatalf("Drift = %v; want strict-high drift workflow", res.Drift)
	}
	if _, statErr := os.Stat(filepath.Join(res.CloneDir, ".github", "workflows", "strict-high.md")); statErr != nil {
		t.Fatalf("strict-high workflow missing; strict gate should abort before prune: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(res.CloneDir, FleetManifestPath)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest stat err = %v; want absent because strict gate aborts before manifest", statErr)
	}
}

func TestUpgradeStrictDryRunBlocksAndPreservesClone(t *testing.T) {
	repo := "rshade/strict-upgrade-dry-run"
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	installFakeGhForUpgrade(t, remote)

	res, err := Upgrade(context.Background(), strictGateTestConfig(repo), repo, UpgradeOpts{
		Security: SecurityOpts{Strict: true},
	})
	assertStrictSecurityError(t, err, repo)
	assertClonePreservedWithFindings(t, res.CloneDir)
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	if res.CompileStrictSource != "" || res.CompileStrictEffective || res.CompileStrictApplied {
		t.Fatalf("compile-strict fields changed before strict abort: %#v", res)
	}
}

func TestUpgradeStrictApplyBlocksBeforeManifestAndPR(t *testing.T) {
	repo := "rshade/strict-upgrade-apply"
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	installFakeGhForUpgrade(t, remote)
	stubGhAwVersionForStrictTests(t)

	res, err := Upgrade(context.Background(), strictGateTestConfig(repo), repo, UpgradeOpts{
		Apply:    true,
		Security: SecurityOpts{Strict: true},
	})
	assertStrictSecurityError(t, err, repo)
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	if res.BranchPushed != "" || res.PRURL != "" {
		t.Fatalf("BranchPushed/PRURL = %q/%q; want no mutation", res.BranchPushed, res.PRURL)
	}
	if _, statErr := os.Stat(filepath.Join(res.CloneDir, FleetManifestPath)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest stat err = %v; want absent because strict gate aborts before manifest", statErr)
	}
}

func TestUpgradeAllStrictFailsFast(t *testing.T) {
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	installFakeGhForUpgrade(t, remote)
	cfg := strictGateTestConfig("rshade/strict-upgrade-one")
	cfg.Repos["rshade/strict-upgrade-two"] = RepoSpec{Profiles: []string{"default"}, CompileStrict: boolPtr(false)}

	results, err := UpgradeAll(context.Background(), cfg, UpgradeOpts{Security: SecurityOpts{Strict: true}})
	assertStrictSecurityError(t, err, "")
	if len(results) != 1 {
		t.Fatalf("len(results) = %d; want fail-fast after first strict blocker", len(results))
	}
	if results[0] != nil && results[0].CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(results[0].CloneDir) })
	}
}

func strictGateTestConfig(repo string) *Config {
	return &Config{
		Version: SchemaVersion,
		Profiles: map[string]Profile{
			"default": {
				Sources:   map[string]SourcePin{},
				Workflows: []ProfileWorkflow{},
			},
		},
		Repos: map[string]RepoSpec{
			repo: {Profiles: []string{"default"}, CompileStrict: boolPtr(false)},
		},
	}
}

func strictGateMissingWorkflowConfig(repo string) *Config {
	cfg := strictGateTestConfig(repo)
	cfg.Profiles["default"] = Profile{
		Sources: map[string]SourcePin{
			"githubnext/agentics": {Ref: "v1.0.0"},
		},
		Workflows: []ProfileWorkflow{
			{Name: "ci-doctor", Source: "githubnext/agentics"},
		},
	}
	return cfg
}

func seedHighSecurityWorkflow(dir string) {
	wf := filepath.Join(dir, ".github", "workflows", "strict-high.md")
	if err := os.MkdirAll(filepath.Dir(wf), 0o755); err != nil {
		panic(err)
	}
	content := `---
on:
  schedule:
    - cron: "0 0 * * *"
permissions: write-all
---
`
	if err := os.WriteFile(wf, []byte(content), 0o644); err != nil {
		panic(err)
	}
	gitInDir(dir, "add", ".github/workflows/strict-high.md")
	gitInDir(dir, "commit", "-m", "seed strict high workflow")
}

func installHealthyDeployAPIs(t *testing.T, repo string) {
	t.Helper()
	withGhAPIJSON(t, map[string]fakeJSONResponse{
		"/repos/" + repo + "/actions/permissions": {
			body: map[string]any{"enabled": true},
		},
		"/repos/" + repo + "/actions/permissions/workflow": {
			body: map[string]any{"default_workflow_permissions": "write"},
		},
	})
}

func stubGhAwVersionForStrictTests(t *testing.T) {
	t.Helper()
	origVersion := ghAwVersion
	t.Cleanup(func() { ghAwVersion = origVersion })
	ghAwVersion = func(_ context.Context) (string, error) {
		return "v0.79.2", nil
	}
}

func assertStrictSecurityError(t *testing.T, err error, repo string) {
	t.Helper()
	var strictErr *StrictSecurityError
	if !errors.As(err, &strictErr) {
		t.Fatalf("error = %T %[1]v; want *StrictSecurityError", err)
	}
	if strictErr.BlockingCount != 1 {
		t.Fatalf("BlockingCount = %d; want 1", strictErr.BlockingCount)
	}
	if repo != "" && strictErr.Repo != repo {
		t.Fatalf("Repo = %q; want %q", strictErr.Repo, repo)
	}
}

func assertClonePreservedWithFindings(t *testing.T, cloneDir string) {
	t.Helper()
	if cloneDir == "" {
		t.Fatal("CloneDir is empty")
	}
	if _, statErr := os.Stat(cloneDir); statErr != nil {
		t.Fatalf("clone not preserved at %s: %v", cloneDir, statErr)
	}
	if _, statErr := os.Stat(filepath.Join(cloneDir, "findings.json")); statErr != nil {
		t.Fatalf("findings.json missing in preserved clone: %v", statErr)
	}
}

func hasStrictHighFinding(findings []security.Finding) bool {
	return slices.ContainsFunc(findings, func(finding security.Finding) bool {
		return finding.RuleID == strictHighRuleID
	})
}
