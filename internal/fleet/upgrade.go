package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type UpgradeOpts struct {
	Apply   bool
	Audit   bool
	Major   bool
	Force   bool
	WorkDir string
}

type UpgradeResult struct {
	Repo         string
	CloneDir     string
	UpgradeOK    bool
	UpdateOK     bool
	ChangedFiles []string
	Conflicts    []string
	NoChanges    bool
	BranchPushed string
	PRURL        string
	AuditJSON    json.RawMessage
	OutputLog    string // combined stdout+stderr from gh aw upgrade/update; used for hint extraction
}

// Upgrade runs the upgrade pipeline for a single repo.
func Upgrade(ctx context.Context, _ *Config, repo string, opts UpgradeOpts) (*UpgradeResult, error) {
	res := &UpgradeResult{Repo: repo}
	var err error
	res.CloneDir, err = prepareClone(ctx, repo, opts.WorkDir)
	if err != nil {
		return res, err
	}
	if opts.WorkDir == "" && !opts.Apply {
		defer os.RemoveAll(res.CloneDir)
	}

	if opts.Audit {
		return runAudit(ctx, res)
	}

	upgradeOut, err := runUpgrade(ctx, res.CloneDir)
	res.OutputLog += upgradeOut
	if err != nil {
		return res, fmt.Errorf("gh aw upgrade: %w", err)
	}
	res.UpgradeOK = true

	updateOut, err := runUpdate(ctx, res.CloneDir, opts.Major, opts.Force)
	res.OutputLog += updateOut
	if err != nil {
		return res, fmt.Errorf("gh aw update: %w", err)
	}
	res.UpdateOK = true

	conflicts, err := checkConflicts(ctx, res.CloneDir)
	if err != nil {
		return res, err
	}
	res.Conflicts = conflicts
	if len(conflicts) > 0 {
		return res, nil
	}

	changed, err := getChangedFiles(ctx, res.CloneDir)
	if err != nil {
		return res, err
	}
	res.ChangedFiles = changed
	if len(changed) == 0 {
		res.NoChanges = true
		return res, nil
	}

	if !opts.Apply {
		return res, nil
	}

	return createUpgradePR(ctx, res)
}

// createUpgradePR branches, commits, pushes, and opens a PR for an upgrade
// that has changed files staged in res.CloneDir.
func createUpgradePR(ctx context.Context, res *UpgradeResult) (*UpgradeResult, error) {
	branch := fmt.Sprintf("fleet/upgrade-%s", time.Now().UTC().Format("2006-01-02-150405"))
	if branchErr := gitCmd(ctx, res.CloneDir, "checkout", "-b", branch); branchErr != nil {
		return res, fmt.Errorf("create branch: %w", branchErr)
	}
	if addErr := gitCmd(ctx, res.CloneDir, "add", ".github/"); addErr != nil {
		return res, fmt.Errorf("git add: %w", addErr)
	}
	msg := upgradeCommitMessage(res)
	if commitErr := gitCmd(ctx, res.CloneDir, "commit", "-m", msg); commitErr != nil {
		return res, fmt.Errorf("git commit: %w", commitErr)
	}
	if pushErr := gitCmd(ctx, res.CloneDir, "push", "-u", "origin", branch); pushErr != nil {
		return res, fmt.Errorf("git push: %w", pushErr)
	}
	res.BranchPushed = branch

	prURL, prErr := ghPRCreate(ctx, res.CloneDir, upgradeTitle(), upgradeBody(res))
	if prErr != nil {
		return res, fmt.Errorf("gh pr create: %w", prErr)
	}
	res.PRURL = prURL
	return res, nil
}

// UpgradeAll runs upgrade for all repos in the config.
func UpgradeAll(ctx context.Context, cfg *Config, opts UpgradeOpts) ([]*UpgradeResult, error) {
	var results []*UpgradeResult
	for repo := range cfg.Repos {
		res, err := Upgrade(ctx, cfg, repo, opts)
		results = append(results, res)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func runAudit(ctx context.Context, res *UpgradeResult) (*UpgradeResult, error) {
	cmd := exec.CommandContext(ctx, "gh", "aw", "upgrade", "--audit", "--json")
	cmd.Dir = res.CloneDir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return res, fmt.Errorf("gh aw upgrade --audit: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return res, fmt.Errorf("gh aw upgrade --audit: %w", err)
	}
	res.AuditJSON = out
	return res, nil
}

// runUpgrade and runUpdate tee their output to stderr (so the user sees
// progress) AND capture it into a buffer so the caller can scan for
// diagnostic patterns (e.g. "Unknown property: mount-as-clis").
func runUpgrade(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "aw", "upgrade")
	cmd.Dir = dir
	var buf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stderr, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	err := cmd.Run()
	return buf.String(), err
}

func runUpdate(ctx context.Context, dir string, major, force bool) (string, error) {
	args := []string{"aw", "update"}
	if major {
		args = append(args, "--major")
	}
	if force {
		args = append(args, "--force")
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	var buf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stderr, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	err := cmd.Run()
	return buf.String(), err
}

func checkConflicts(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var conflicts []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		status := line[:2]
		if status[0] == 'U' || status[1] == 'U' {
			file := strings.TrimSpace(line[3:])
			conflicts = append(conflicts, file)
		}
	}

	for _, file := range conflicts {
		if hasConflictMarkers(filepath.Join(dir, file)) {
			return conflicts, nil
		}
	}

	for _, f := range conflicts {
		path := filepath.Join(dir, f)
		content, _ := os.ReadFile(path)
		if strings.Contains(string(content), "<<<<<<<") {
			return conflicts, nil
		}
	}

	return conflicts, nil
}

func hasConflictMarkers(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), "<<<<<<<")
}

func getChangedFiles(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		file := strings.TrimSpace(line[3:])
		files = append(files, file)
	}
	return files, nil
}

func upgradeCommitMessage(res *UpgradeResult) string {
	changed := len(res.ChangedFiles)
	return fmt.Sprintf(
		"ci(workflows): upgrade agentic workflows (%d files changed)\n\nUpgraded via gh aw upgrade + update.\n",
		changed,
	)
}

func upgradeTitle() string {
	return "ci(workflows): upgrade agentic workflows"
}

func upgradeBody(res *UpgradeResult) string {
	var b strings.Builder
	b.WriteString("Upgrades agentic workflows via `gh aw upgrade` + `gh aw update`.\n\n")
	if len(res.ChangedFiles) > 0 {
		b.WriteString("## Changed files\n\n")
		for _, f := range res.ChangedFiles {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
	}
	return b.String()
}
