package fleet

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// withInteractivePrompt drives the security-findings prompt for placement
// tests: it forces an interactive stdout and feeds promptStdin from stdin,
// discarding prompt output. All three seams are restored on cleanup. These
// tests mutate package-level seams, so they must not run with t.Parallel.
func withInteractivePrompt(t *testing.T, stdin string) {
	t.Helper()
	origTTY := stdoutIsTerminal
	origIn := promptStdin
	origOut := promptStdout
	t.Cleanup(func() {
		stdoutIsTerminal = origTTY
		promptStdin = origIn
		promptStdout = origOut
	})
	stdoutIsTerminal = func() bool { return true }
	promptStdin = strings.NewReader(stdin)
	promptStdout = io.Discard
}

// withPromptStdinUnread forces a TTY but installs a stdin reader that fails the
// test if it is ever read — proving a code path proceeded without prompting.
func withPromptStdinUnread(t *testing.T, isTTY bool) {
	t.Helper()
	origTTY := stdoutIsTerminal
	origIn := promptStdin
	origOut := promptStdout
	t.Cleanup(func() {
		stdoutIsTerminal = origTTY
		promptStdin = origIn
		promptStdout = origOut
	})
	stdoutIsTerminal = func() bool { return isTTY }
	promptStdin = failOnReadReader{t}
	promptStdout = io.Discard
}

type failOnReadReader struct{ t *testing.T }

func (r failOnReadReader) Read([]byte) (int, error) {
	r.t.Error("stdin read on a path that must not prompt")
	return 0, io.EOF
}

// stubCleanActionlint installs a no-op `actionlint` binary at the front of PATH
// so the actionlint scanner resolves a binary and emits nothing, instead of the
// `actionlint:not-installed` INFO finding it returns when the binary is absent.
// Without this, the "zero findings" placement tests pass only on hosts that
// happen to have actionlint installed (dev machines) and fail in CI, where the
// graceful-degradation INFO finding is non-empty and — correctly, per FR-015 —
// fires the confirmation prompt. The stub exits 0 with empty output, which the
// scanner treats as "no diagnostics," making the baseline deterministic in both
// environments. Mirrors the fake-gh fixture idiom; safe to combine because the
// stub binary name does not collide.
func stubCleanActionlint(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "actionlint"), []byte(script), 0o755); err != nil {
		t.Fatalf("write stub actionlint: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func gitOutput(t *testing.T, dir string, arg ...string) string {
	t.Helper()
	cmd := exec.Command("git", arg...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v in %s: %v", arg, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// newPromptDeployFixture wires the strict-gate fixtures (a remote seeded with a
// HIGH finding, a fake gh, healthy Actions APIs, a stubbed gh aw version) and
// returns the config + repo for a deploy that reaches the commit boundary with
// findings present but compile-strict off.
func newPromptDeployFixture(t *testing.T, repo string) *Config {
	t.Helper()
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	_ = installFakeGhForSync(t, remote)
	installHealthyDeployAPIs(t, repo)
	stubGhAwVersionForStrictTests(t)
	return strictGateTestConfig(repo)
}

// TestDeployPromptDeclineAbortsBeforePR is the MVP guarantee: an interactive
// apply with findings and "n" returns a typed decline, makes no remote change,
// and preserves the clone — all before createDeployPR runs.
func TestDeployPromptDeclineAbortsBeforePR(t *testing.T) {
	repo := "rshade/prompt-deploy-decline"
	cfg := newPromptDeployFixture(t, repo)
	withInteractivePrompt(t, "n\n")

	res, err := Deploy(context.Background(), cfg, repo, DeployOpts{Apply: true})
	if !IsOperatorDeclinedError(err) {
		t.Fatalf("err = %T %[1]v; want *OperatorDeclinedError", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	if res.BranchPushed != "" || res.PRURL != "" {
		t.Fatalf("BranchPushed/PRURL = %q/%q; want no mutation on decline", res.BranchPushed, res.PRURL)
	}
	if _, statErr := os.Stat(res.CloneDir); statErr != nil {
		t.Fatalf("clone not preserved on decline: %v", statErr)
	}
	var declined *OperatorDeclinedError
	if errors.As(err, &declined) {
		if declined.Findings == 0 {
			t.Error("decline error should carry a positive finding count")
		}
		if declined.Repo != repo {
			t.Errorf("declined.Repo = %q; want %q", declined.Repo, repo)
		}
	}
}

// TestDeployPromptAcceptProceedsToPR proves "y" passes the prompt and reaches
// createDeployPR (which fails at the fake gh pr create) — so the prompt is
// reached before the commit boundary, with findings visible pre-commit.
func TestDeployPromptAcceptProceedsToPR(t *testing.T) {
	repo := "rshade/prompt-deploy-accept"
	cfg := newPromptDeployFixture(t, repo)
	withInteractivePrompt(t, "y\n")

	res, err := Deploy(context.Background(), cfg, repo, DeployOpts{Apply: true})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if err == nil || !strings.Contains(err.Error(), "gh pr create") {
		t.Fatalf("err = %v; want gh pr create failure after accept (proves prompt passed)", err)
	}
}

// TestDeployDryRunNeverPrompts proves --apply absence skips the prompt even with
// findings and queued "n": a dry-run returns nil, never a decline.
func TestDeployDryRunNeverPrompts(t *testing.T) {
	repo := "rshade/prompt-deploy-dryrun"
	cfg := newPromptDeployFixture(t, repo)
	withPromptStdinUnread(t, true)

	res, err := Deploy(context.Background(), cfg, repo, DeployOpts{Apply: false})
	if err != nil {
		t.Fatalf("dry-run err = %v; want nil (no prompt)", err)
	}
	if res != nil && res.CloneDir != "" {
		// dry-run on a tmp clone auto-cleans; nothing to remove if already gone.
		_ = os.RemoveAll(res.CloneDir)
	}
}

// TestDeployYesBypassesPromptWithoutReadingStdin proves --yes (SecurityOpts.Yes)
// skips the prompt without consulting stdin even at a TTY with findings.
func TestDeployYesBypassesPromptWithoutReadingStdin(t *testing.T) {
	repo := "rshade/prompt-deploy-yes"
	cfg := newPromptDeployFixture(t, repo)
	withPromptStdinUnread(t, true)

	res, err := Deploy(context.Background(), cfg, repo, DeployOpts{
		Apply:    true,
		Security: SecurityOpts{Yes: true},
	})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if err == nil || !strings.Contains(err.Error(), "gh pr create") {
		t.Fatalf("err = %v; want gh pr create (proceeded past skipped prompt)", err)
	}
	if IsOperatorDeclinedError(err) {
		t.Fatal("--yes must not produce an operator-decline error")
	}
}

// TestDeployNonTTYProceedsWithoutReadingStdin proves a non-interactive apply
// with findings proceeds without reading stdin (FR-009) — nothing hangs.
func TestDeployNonTTYProceedsWithoutReadingStdin(t *testing.T) {
	repo := "rshade/prompt-deploy-nontty"
	cfg := newPromptDeployFixture(t, repo)
	withPromptStdinUnread(t, false)

	res, err := Deploy(context.Background(), cfg, repo, DeployOpts{Apply: true})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if err == nil || !strings.Contains(err.Error(), "gh pr create") {
		t.Fatalf("err = %v; want gh pr create (non-TTY proceeded)", err)
	}
}

// TestDeployPushGateResumePromptDeclineBeforeAmend proves a push-gate
// --work-dir resume asks before the compile/manifest refresh that may amend
// the saved local commit.
func TestDeployPushGateResumePromptDeclineBeforeAmend(t *testing.T) {
	repo := "rshade/prompt-deploy-resume-push-decline"
	dir := newTestRepo(t, func(d string) {
		gitInDir(d, "checkout", "-b", "fleet/deploy-test")
		seedHighSecurityWorkflow(d)
	})
	withInteractivePrompt(t, "n\n")
	installHealthyDeployAPIs(t, repo)

	origExists := ghAPIExists
	t.Cleanup(func() { ghAPIExists = origExists })
	ghAPIExists = func(_ context.Context, _ string) bool { return false }

	seams := &compileStrictSeams{
		helpOut:    "  --strict  enable strict validation\n",
		versionOut: "v0.79.2",
	}
	installCompileStrictSeams(t, seams)

	cfg := strictGateTestConfig(repo)
	cfg.Repos[repo] = RepoSpec{Profiles: []string{"default"}, CompileStrict: boolPtr(true)}
	beforeHead := gitOutput(t, dir, "rev-parse", "HEAD")

	res, err := Deploy(context.Background(), cfg, repo, DeployOpts{
		Apply:   true,
		WorkDir: dir,
	})
	if res != nil && res.CloneDir != dir {
		t.Fatalf("CloneDir = %q; want resumed work-dir %q", res.CloneDir, dir)
	}
	if !IsOperatorDeclinedError(err) {
		t.Fatalf("err = %T %[1]v; want *OperatorDeclinedError", err)
	}
	if seams.compileCalls != 0 || seams.versionCalls != 0 {
		t.Fatalf(
			"compile/version calls = %d/%d; want 0/0 before declined prompt",
			seams.compileCalls, seams.versionCalls,
		)
	}
	if afterHead := gitOutput(t, dir, "rev-parse", "HEAD"); afterHead != beforeHead {
		t.Fatalf("HEAD = %s; want unchanged %s after decline", afterHead, beforeHead)
	}
	if _, statErr := os.Stat(filepath.Join(dir, FleetManifestPath)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest stat err = %v; want absent before accepted resume refresh", statErr)
	}
}

// TestDeployZeroFindingsNoPrompt proves a clean repo (no findings) never prompts
// even with a TTY and queued "n", and prBody omits the Security Findings section.
func TestDeployZeroFindingsNoPrompt(t *testing.T) {
	repo := "rshade/prompt-deploy-clean"
	remote := newTestRepo(t, nil) // no high-security workflow → zero findings
	_ = installFakeGhForSync(t, remote)
	installHealthyDeployAPIs(t, repo)
	stubGhAwVersionForStrictTests(t)
	stubCleanActionlint(t)          // zero findings regardless of host actionlint
	withInteractivePrompt(t, "n\n") // queued decline must be ignored

	res, err := Deploy(context.Background(), strictGateTestConfig(repo), repo, DeployOpts{Apply: true})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if IsOperatorDeclinedError(err) {
		t.Fatal("zero findings must not fire the prompt")
	}
	if err == nil || !strings.Contains(err.Error(), "gh pr create") {
		t.Fatalf("err = %v; want gh pr create (clean repo proceeded)", err)
	}
	if strings.Contains(prBody(res, repo, 0), "## Security Findings") {
		t.Error("zero findings must not render a ## Security Findings PR section")
	}
}

// TestDeployPRBodyKeepsSecurityFindingsUnderYes proves --yes does not suppress
// the PR-body Security Findings section (surface coexistence, FR-008).
func TestDeployPRBodyKeepsSecurityFindingsUnderYes(t *testing.T) {
	res := &DeployResult{
		Repo:             "rshade/example",
		SecurityFindings: oneHighFinding(),
	}
	body := prBody(res, res.Repo, 0)
	if !strings.Contains(body, "## Security Findings") {
		t.Errorf("prBody missing ## Security Findings section:\n%s", body)
	}
}

// --- sync placement (T013) ---

// TestSyncAddPathPromptsOnceDecline proves the sync add path inherits Deploy's
// single prompt: a decline surfaces as one *OperatorDeclinedError.
func TestSyncAddPathPromptsOnceDecline(t *testing.T) {
	repo := "rshade/prompt-sync-add-decline"
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	_ = installFakeGhForSync(t, remote)
	installHealthyDeployAPIs(t, repo)
	withInteractivePrompt(t, "n\n")

	res, err := Sync(context.Background(), strictGateMissingWorkflowConfig(repo), repo, SyncOpts{Apply: true})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if !IsOperatorDeclinedError(err) {
		t.Fatalf("err = %T %[1]v; want *OperatorDeclinedError via delegated Deploy", err)
	}
}

// TestSyncAddPathAcceptProceeds proves a single "y" passes the delegated Deploy
// prompt and there is no second sync-level prompt (it would read EOF and decline
// otherwise) — the run proceeds to the fake gh pr create failure.
func TestSyncAddPathAcceptProceeds(t *testing.T) {
	repo := "rshade/prompt-sync-add-accept"
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	_ = installFakeGhForSync(t, remote)
	installHealthyDeployAPIs(t, repo)
	withInteractivePrompt(t, "y\n")

	res, err := Sync(context.Background(), strictGateMissingWorkflowConfig(repo), repo, SyncOpts{Apply: true})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if IsOperatorDeclinedError(err) {
		t.Fatal("single y should pass; a second prompt would have declined at EOF")
	}
	if err == nil || !strings.Contains(err.Error(), "gh pr create") {
		t.Fatalf("err = %v; want gh pr create after accept", err)
	}
}

// TestSyncPruneOnlyDropsStaleFindingsBeforePrompt proves prune-only sync
// recomputes findings after deleting drift workflows, so warnings that existed
// only in deleted files cannot trigger the confirmation prompt.
func TestSyncPruneOnlyDropsStaleFindingsBeforePrompt(t *testing.T) {
	repo := "rshade/prompt-sync-prune-stale"
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	_ = installFakeGhForSync(t, remote)
	stubCleanActionlint(t) // pruned repo is finding-free regardless of host actionlint
	withPromptStdinUnread(t, true)

	res, err := Sync(context.Background(), strictGateTestConfig(repo), repo, SyncOpts{
		Apply: true,
		Prune: true,
	})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if IsOperatorDeclinedError(err) {
		t.Fatalf("err = %T %[1]v; want no prompt for findings removed by prune", err)
	}
	if err != nil && !strings.Contains(err.Error(), "git push prune") {
		t.Fatalf("err = %v; want nil or later git push prune failure", err)
	}
	if res == nil {
		t.Fatal("Sync returned nil result")
	}
	if !slices.Contains(res.Pruned, "strict-high") {
		t.Fatalf("Pruned = %v, want strict-high", res.Pruned)
	}
	if len(res.SecurityFindings) != 0 {
		t.Fatalf("SecurityFindings = %#v; want empty after pruning stale finding", res.SecurityFindings)
	}
}

// TestSyncPruneOnlyPromptDeclineBeforeCommit proves the prune-only path prompts
// before commitAndPushPrune when findings remain after pruning, and a decline
// aborts.
func TestSyncPruneOnlyPromptDeclineBeforeCommit(t *testing.T) {
	repo := "rshade/prompt-sync-prune-decline"
	remote := newTestRepo(t, func(dir string) {
		seedDeclaredHighWorkflow(dir)
		seedDriftedWorkflow(dir)
	})
	_ = installFakeGhForSync(t, remote)
	withInteractivePrompt(t, "n\n")

	res, err := Sync(context.Background(), strictGateMissingWorkflowConfig(repo), repo, SyncOpts{
		Apply: true,
		Prune: true,
	})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if !IsOperatorDeclinedError(err) {
		t.Fatalf("err = %T %[1]v; want *OperatorDeclinedError before prune commit", err)
	}
}

func seedDeclaredHighWorkflow(dir string) {
	wf := filepath.Join(dir, ".github", "workflows", "ci-doctor.md")
	if err := os.MkdirAll(filepath.Dir(wf), 0o755); err != nil {
		panic(err)
	}
	content := `---
source: githubnext/agentics/ci-doctor@v1.0.0
on:
  schedule:
    - cron: "0 0 * * *"
permissions: write-all
---
`
	if err := os.WriteFile(wf, []byte(content), 0o644); err != nil {
		panic(err)
	}
	gitInDir(dir, "add", ".github/workflows/ci-doctor.md")
	gitInDir(dir, "commit", "-m", "seed declared high workflow")
}

// TestSyncCleanPathNoPrompt proves a fully in-state sync (nothing to add or
// prune) never prompts, even with a TTY.
func TestSyncCleanPathNoPrompt(t *testing.T) {
	repo := "rshade/prompt-sync-clean"
	remote := newTestRepo(t, seedDeclaredWorkflow)
	_ = installFakeGhForSync(t, remote)
	withPromptStdinUnread(t, true)

	res, err := Sync(context.Background(), strictGateMissingWorkflowConfig(repo), repo, SyncOpts{Apply: true})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if err != nil {
		t.Fatalf("clean sync err = %v; want nil (no prompt)", err)
	}
}

// --- upgrade placement (T014) ---

// newPromptUpgradeFixture wires the upgrade fake-gh + a HIGH-finding remote and
// returns the config for an apply that reaches the PR boundary with findings.
func newPromptUpgradeFixture(t *testing.T, repo string) *Config {
	t.Helper()
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	installFakeGhForUpgrade(t, remote)
	stubGhAwVersionForStrictTests(t)
	return strictGateTestConfig(repo)
}

// TestUpgradePromptDeclineAbortsBeforePR proves an apply upgrade with findings
// and "n" aborts before createUpgradePR with a typed decline.
func TestUpgradePromptDeclineAbortsBeforePR(t *testing.T) {
	repo := "rshade/prompt-upgrade-decline"
	cfg := newPromptUpgradeFixture(t, repo)
	withInteractivePrompt(t, "n\n")

	res, err := Upgrade(context.Background(), cfg, repo, UpgradeOpts{Apply: true})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if !IsOperatorDeclinedError(err) {
		t.Fatalf("err = %T %[1]v; want *OperatorDeclinedError", err)
	}
	if res.BranchPushed != "" || res.PRURL != "" {
		t.Fatalf("BranchPushed/PRURL = %q/%q; want no mutation on decline", res.BranchPushed, res.PRURL)
	}
}

// TestUpgradePromptAcceptProceedsToPR proves "y" passes the prompt and reaches
// createUpgradePR (fake gh pr create failure).
func TestUpgradePromptAcceptProceedsToPR(t *testing.T) {
	repo := "rshade/prompt-upgrade-accept"
	cfg := newPromptUpgradeFixture(t, repo)
	withInteractivePrompt(t, "y\n")

	res, err := Upgrade(context.Background(), cfg, repo, UpgradeOpts{Apply: true})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if err == nil || !strings.Contains(err.Error(), "gh pr create") {
		t.Fatalf("err = %v; want gh pr create after accept", err)
	}
}

// TestUpgradeDryRunNeverPrompts proves a dry-run upgrade never prompts even with
// findings and queued "n".
func TestUpgradeDryRunNeverPrompts(t *testing.T) {
	repo := "rshade/prompt-upgrade-dryrun"
	cfg := newPromptUpgradeFixture(t, repo)
	withPromptStdinUnread(t, true)

	res, err := Upgrade(context.Background(), cfg, repo, UpgradeOpts{Apply: false})
	if res != nil && res.CloneDir != "" {
		_ = os.RemoveAll(res.CloneDir)
	}
	if err != nil {
		t.Fatalf("dry-run err = %v; want nil (no prompt)", err)
	}
}

// TestUpgradeAllDeclineFailsFast proves a decline on the first repo halts the
// batch (fail-fast) with a single result and a typed decline.
func TestUpgradeAllDeclineFailsFast(t *testing.T) {
	remote := newTestRepo(t, seedHighSecurityWorkflow)
	installFakeGhForUpgrade(t, remote)
	stubGhAwVersionForStrictTests(t)
	cfg := strictGateTestConfig("rshade/prompt-upgrade-all-one")
	cfg.Repos["rshade/prompt-upgrade-all-two"] = RepoSpec{Profiles: []string{"default"}, CompileStrict: boolPtr(false)}
	withInteractivePrompt(t, "n\n")

	results, err := UpgradeAll(context.Background(), cfg, UpgradeOpts{Apply: true})
	for _, r := range results {
		if r != nil && r.CloneDir != "" {
			t.Cleanup(func() { _ = os.RemoveAll(r.CloneDir) })
		}
	}
	if !IsOperatorDeclinedError(err) {
		t.Fatalf("err = %T %[1]v; want *OperatorDeclinedError", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d; want fail-fast after first decline", len(results))
	}
}

// TestUpgradeNonTTYProceedsWithoutReadingStdin proves a non-interactive upgrade
// apply with findings proceeds without reading stdin (FR-009).
func TestUpgradeNonTTYProceedsWithoutReadingStdin(t *testing.T) {
	repo := "rshade/prompt-upgrade-nontty"
	cfg := newPromptUpgradeFixture(t, repo)
	withPromptStdinUnread(t, false)

	res, err := Upgrade(context.Background(), cfg, repo, UpgradeOpts{Apply: true})
	if res != nil && res.CloneDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(res.CloneDir) })
	}
	if err == nil || !strings.Contains(err.Error(), "gh pr create") {
		t.Fatalf("err = %v; want gh pr create (non-TTY proceeded)", err)
	}
}
