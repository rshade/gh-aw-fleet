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

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

// UpgradeOpts controls upgrade behavior. Apply=false is dry-run; Audit=true
// runs `gh aw audit` instead of the upgrade pipeline; Major=true permits
// major-version source bumps; Force=true passes through to `gh aw upgrade`.
type UpgradeOpts struct {
	Apply    bool
	Audit    bool
	Major    bool
	Force    bool
	WorkDir  string       // optional existing clone.
	Security SecurityOpts // security policy for this invocation.
}

// UpgradeResult aggregates what happened for a single-repo upgrade. OutputLog
// captures combined stdout+stderr from gh aw upgrade/update so the diagnostics
// layer can extract actionable hints on failure.
type UpgradeResult struct {
	Repo         string          `json:"repo"`
	CloneDir     string          `json:"clone_dir"`
	UpgradeOK    bool            `json:"upgrade_ok"`
	UpdateOK     bool            `json:"update_ok"`
	ChangedFiles []string        `json:"changed_files"`
	Conflicts    []string        `json:"conflicts"`
	NoChanges    bool            `json:"no_changes"`
	BranchPushed string          `json:"branch_pushed"`
	PRURL        string          `json:"pr_url"`
	AuditJSON    json.RawMessage `json:"audit_json"`
	OutputLog    string          `json:"output_log"` // combined stdout+stderr from gh aw upgrade/update; used for hint extraction

	// InitWasRun is true when ensureInit ran `gh aw init` to refresh init
	// artifacts during this upgrade (the clone's fleet manifest recorded a
	// gh-aw version other than the fleet pin). False when init was already
	// current and skipped. Mirrors DeployResult.InitWasRun.
	InitWasRun bool `json:"init_was_run"`

	// SecurityFindings is the sorted output of security.Run after the
	// upgrade pipeline modifies workflow markdown. Findings are advisory:
	// they never block the upgrade. Surfaced on stderr (zerolog), in the
	// JSON envelope warnings[], and in the PR body's `## Security Findings`
	// section.
	SecurityFindings []security.Finding `json:"security_findings"`

	// CompileStrictApplied is true ONLY when `gh aw compile --strict` was
	// invoked AND exited 0 during this Upgrade run. False covers both
	// "resolver said skip" and "resolver said apply but probe/compile
	// aborted." Consumers MUST cross-reference CompileStrictSource to
	// disambiguate. In dry-run mode this field is always false even when
	// strict would apply on --apply; cross-reference CompileStrictEffective
	// for the would-apply intent.
	CompileStrictApplied bool `json:"compile_strict_applied"`

	// CompileStrictEffective is the resolver's effective verdict for this
	// repo, independent of whether `gh aw compile --strict` actually ran.
	// In dry-run mode this is the "would apply on --apply" signal; in
	// --apply mode it equals CompileStrictApplied unless probe/compile
	// aborted (in which case Effective=true and Applied=false). Empty
	// CompileStrictSource means the resolver never ran and this field MUST
	// be treated as not applicable.
	CompileStrictEffective bool `json:"compile_strict_effective"`

	// CompileStrictSource discriminates why CompileStrictApplied has its
	// value. One of "explicit" (operator override via RepoSpec.CompileStrict),
	// "auto-public", "auto-private", "auto-fallback", or "" (the resolver
	// never ran — early error path). Empty string is DISTINCT from the four
	// valid values; consumers MUST treat it as "not applicable to this
	// result."
	CompileStrictSource string `json:"compile_strict_source"`
}

// SetCompileStrictSource implements compileStrictResult.
func (r *UpgradeResult) SetCompileStrictSource(s string) { r.CompileStrictSource = s }

// SetCompileStrictApplied implements compileStrictResult.
func (r *UpgradeResult) SetCompileStrictApplied(b bool) { r.CompileStrictApplied = b }

// SetCompileStrictEffective implements compileStrictResult.
func (r *UpgradeResult) SetCompileStrictEffective(b bool) { r.CompileStrictEffective = b }

// WorkCloneDir implements compileStrictResult.
func (r *UpgradeResult) WorkCloneDir() string { return r.CloneDir }

// Upgrade runs the upgrade pipeline for a single repo.
func Upgrade(ctx context.Context, cfg *Config, repo string, opts UpgradeOpts) (*UpgradeResult, error) {
	res := &UpgradeResult{Repo: repo}
	var err error
	res.CloneDir, err = prepareClone(ctx, repo, opts.WorkDir)
	if err != nil {
		return res, err
	}
	cleanupClone := opts.WorkDir == "" && !opts.Apply
	defer func() {
		if cleanupClone {
			_ = os.RemoveAll(res.CloneDir)
		}
	}()

	if opts.Audit {
		return runAudit(ctx, res)
	}

	// Refresh init artifacts to the fleet's gh-aw version BEFORE recompiling
	// lock files, so a stale dispatcher/init layout (last deploy predates a pin
	// bump) doesn't outlive the upgrade. No-op when the manifest already records
	// the current version. Runs in dry-run too (mutates only the throwaway
	// clone), matching deploy/sync which init before their apply split.
	res.InitWasRun, err = ensureInit(ctx, res.CloneDir, resolvedGhAwPin(cfg, repo))
	if err != nil {
		return res, fmt.Errorf("gh aw init: %w", err)
	}

	if pipelineErr := runUpgradePipeline(ctx, res, opts); pipelineErr != nil {
		return res, pipelineErr
	}

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
	res.SecurityFindings = security.Run(ctx, res.CloneDir)
	if gateErr := evaluateStrictGatePreservingClone(
		repo, res.CloneDir, opts.Security, res.SecurityFindings, &cleanupClone,
	); gateErr != nil {
		return res, gateErr
	}
	if len(changed) == 0 {
		return finishNoChangeUpgrade(ctx, cfg, repo, res, opts)
	}

	if !opts.Apply {
		// Dry-run path: resolve and log the would-be policy without invoking
		// the probe or the compile subprocess (cli-semantics.md §Dry-run mode).
		effective, source := logCompileStrictResolution(ctx, cfg, repo)
		res.CompileStrictSource = source
		res.CompileStrictEffective = effective
		return res, nil
	}

	if compileErr := runCompileStrictIfNeeded(ctx, res, cfg, repo); compileErr != nil {
		return res, compileErr
	}

	// Record the fleet manifest so the recorded gh-aw version reflects this
	// upgrade. Written before createUpgradePR's `git add .github/` so the
	// manifest update rides in the PR; writeManifestIfNeeded suppresses churn
	// when already current.
	if manifestErr := writeDeployManifest(ctx, cfg, repo, res.CloneDir); manifestErr != nil {
		return res, manifestErr
	}

	return createUpgradePR(ctx, res)
}

func finishNoChangeUpgrade(
	ctx context.Context, cfg *Config, repo string, res *UpgradeResult, opts UpgradeOpts,
) (*UpgradeResult, error) {
	if !opts.Apply {
		res.NoChanges = true
		return res, nil
	}
	manifestBackfilled, manifestErr := backfillUpgradeManifest(ctx, cfg, repo, res)
	if manifestErr != nil {
		return res, manifestErr
	}
	if !manifestBackfilled {
		return res, nil
	}
	return createUpgradePR(ctx, res)
}

func runUpgradePipeline(ctx context.Context, res *UpgradeResult, opts UpgradeOpts) error {
	upgradeOut, err := runUpgrade(ctx, res.CloneDir)
	res.OutputLog += upgradeOut
	if err != nil {
		return fmt.Errorf("gh aw upgrade: %w", err)
	}
	res.UpgradeOK = true

	updateOut, err := runUpdate(ctx, res.CloneDir, opts.Major, opts.Force)
	res.OutputLog += updateOut
	if err != nil {
		return fmt.Errorf("gh aw update: %w", err)
	}
	res.UpdateOK = true
	return nil
}

func backfillUpgradeManifest(ctx context.Context, cfg *Config, repo string, res *UpgradeResult) (bool, error) {
	// Backfill the fleet manifest for legacy repos whose gh-aw init layout is
	// already current but predates manifest tracking. Without this, apply-mode
	// upgrades short-circuit as "no changes" and the repo remains unmanaged.
	if manifestErr := writeDeployManifest(ctx, cfg, repo, res.CloneDir); manifestErr != nil {
		return false, manifestErr
	}
	changed, err := getChangedFiles(ctx, res.CloneDir)
	if err != nil {
		return false, err
	}
	res.ChangedFiles = changed
	if len(changed) == 0 {
		res.NoChanges = true
		return false, nil
	}
	return true, nil
}

// createUpgradePR branches, commits, pushes, and opens a PR for an upgrade
// that has changed files staged in res.CloneDir.
func createUpgradePR(ctx context.Context, res *UpgradeResult) (*UpgradeResult, error) {
	branch := fmt.Sprintf("fleet/upgrade-%s", time.Now().UTC().Format("2006-01-02-150405"))
	if branchErr := gitCmd(ctx, res.CloneDir, "checkout", "-b", branch); branchErr != nil {
		return res, fmt.Errorf("create branch: %w", branchErr)
	}
	if addErr := gitCmd(ctx, res.CloneDir, addToken, ".github/"); addErr != nil {
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
	err := runLogged(cmd, "gh", "aw upgrade", map[string]string{fieldCloneDir: dir})
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
	err := runLogged(cmd, "gh", "aw update", map[string]string{fieldCloneDir: dir})
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
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseGitStatusPorcelain(string(out)), nil
}

func parseGitStatusPorcelain(out string) []string {
	const porcelainPrefixLen = 3 // XY + space status prefix
	var files []string
	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		if len(line) <= porcelainPrefixLen {
			continue
		}
		file := strings.TrimSpace(line[porcelainPrefixLen:])
		files = append(files, file)
	}
	return files
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
	if res.InitWasRun {
		b.WriteString("Init artifacts were refreshed to the current gh-aw version (`gh aw init`).\n\n")
	}
	if section := security.RenderPRSection(res.SecurityFindings); section != "" {
		b.WriteString(section)
		b.WriteString("\n")
	}
	if len(res.ChangedFiles) > 0 {
		b.WriteString("## Changed files\n\n")
		for _, f := range res.ChangedFiles {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
	}
	return b.String()
}
