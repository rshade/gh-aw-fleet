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

	zlog "github.com/rs/zerolog/log"

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
)

// UpgradeOpts controls upgrade behavior. Apply=false is dry-run; Audit=true
// runs `gh aw audit` instead of the upgrade pipeline; Major=true permits
// major-version source bumps; Force=true passes through to `gh aw upgrade`.
type UpgradeOpts struct {
	Apply   bool
	Audit   bool
	Major   bool
	Force   bool
	WorkDir string
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
	// disambiguate.
	CompileStrictApplied bool `json:"compile_strict_applied"`

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
	res.SecurityFindings = security.Run(ctx, res.CloneDir)
	if len(changed) == 0 {
		res.NoChanges = true
		return res, nil
	}

	if !opts.Apply {
		// Dry-run path: resolve and log the would-be policy without invoking
		// the probe or the compile subprocess (cli-semantics.md §Dry-run mode).
		effective, source, reason := cfg.EffectiveCompileStrict(ctx, repo)
		res.CompileStrictSource = source
		zlog.Info().
			Str("event", "compile_strict_resolved").
			Str(fieldRepo, repo).
			Bool("effective", effective).
			Str("source", source).
			Msg("compile-strict resolution")
		if source == CompileStrictSourceAutoFallback {
			zlog.Warn().
				Str("event", "compile_strict_lookup_failed").
				Str(fieldRepo, repo).
				Str("reason", reason).
				Msg("compile-strict visibility lookup failed; defaulting to strict ON")
		}
		return res, nil
	}

	if compileErr := runCompileStrictIfNeeded(ctx, res, cfg, repo); compileErr != nil {
		return res, compileErr
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
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
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
