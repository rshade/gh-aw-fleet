package fleet

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DeployOpts controls deploy behavior.
type DeployOpts struct {
	Apply   bool // false = dry-run (default); true = commit + push + PR
	Force   bool // pass --force to gh aw add
	Branch  string
	PRTitle string
	WorkDir string // if set, use this clone path; otherwise tmp.
}

// DeployResult aggregates what happened for a single-repo deploy.
type DeployResult struct {
	Repo         string
	CloneDir     string
	Added        []WorkflowOutcome
	Skipped      []WorkflowOutcome
	Failed       []WorkflowOutcome
	InitWasRun   bool
	BranchPushed string
	PRURL        string
}

// WorkflowOutcome is one workflow's fate during a deploy.
type WorkflowOutcome struct {
	Name   string
	Spec   string
	Reason string
	Error  string
}

// Deploy runs the deploy pipeline for a single repo.
// Dry-run (Apply=false) stops after pre-flight and returns the plan.
// --apply extends through branch/commit/push/PR.
func Deploy(ctx context.Context, cfg *Config, repo string, opts DeployOpts) (*DeployResult, error) {
	resolved, err := cfg.ResolveRepoWorkflows(repo)
	if err != nil {
		return nil, err
	}

	res := &DeployResult{Repo: repo}
	res.CloneDir, err = prepareClone(ctx, repo, opts.WorkDir)
	if err != nil {
		return res, err
	}
	if opts.WorkDir == "" && !opts.Apply {
		defer os.RemoveAll(res.CloneDir)
	}

	res.InitWasRun, err = ensureInit(ctx, res.CloneDir)
	if err != nil {
		return res, fmt.Errorf("gh aw init: %w", err)
	}

	engine := cfg.EffectiveEngine(repo)
	for _, w := range resolved {
		if w.Source == "local" {
			continue
		}
		if fileExists(filepath.Join(res.CloneDir, ".github/workflows", w.Name+".md")) && !opts.Force {
			res.Skipped = append(res.Skipped, WorkflowOutcome{Name: w.Name, Spec: w.Spec(), Reason: "already present"})
			continue
		}
		out, addErr := runAdd(ctx, res.CloneDir, w.Spec(), engine, opts.Force)
		if addErr != nil {
			res.Failed = append(res.Failed, WorkflowOutcome{
				Name:  w.Name,
				Spec:  w.Spec(),
				Error: condense(out, addErr),
			})
			continue
		}
		res.Added = append(res.Added, WorkflowOutcome{Name: w.Name, Spec: w.Spec()})
	}

	if !opts.Apply {
		return res, nil
	}
	staged, err := hasStagedOrUnstagedWorkflowChanges(ctx, res.CloneDir)
	if err != nil {
		return res, err
	}
	if len(res.Added) == 0 && !staged {
		return res, nil
	}

	branch := opts.Branch
	if branch == "" {
		branch = fmt.Sprintf("fleet/deploy-%s", time.Now().UTC().Format("2006-01-02-150405"))
	}
	if err := gitCmd(ctx, res.CloneDir, "checkout", "-b", branch); err != nil {
		return res, fmt.Errorf("create branch: %w", err)
	}
	if err := gitCmd(ctx, res.CloneDir, "add", ".github/"); err != nil {
		return res, fmt.Errorf("git add: %w", err)
	}
	msg := commitMessage(res)
	if err := gitCmd(ctx, res.CloneDir, "commit", "-m", msg); err != nil {
		return res, fmt.Errorf("git commit: %w", err)
	}
	if err := gitCmd(ctx, res.CloneDir, "push", "-u", "origin", branch); err != nil {
		return res, fmt.Errorf("git push: %w", err)
	}
	res.BranchPushed = branch

	title := opts.PRTitle
	if title == "" {
		title = fmt.Sprintf("ci(workflows): add %d agentic workflows via gh-aw-fleet", len(res.Added))
	}
	prURL, err := ghPRCreate(ctx, res.CloneDir, title, prBody(res, repo))
	if err != nil {
		return res, fmt.Errorf("gh pr create: %w", err)
	}
	res.PRURL = prURL
	return res, nil
}

func prepareClone(ctx context.Context, repo, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	dir, err := os.MkdirTemp("", "gh-aw-fleet-"+strings.ReplaceAll(repo, "/", "-")+"-")
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "gh", "repo", "clone", repo, dir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return dir, fmt.Errorf("gh repo clone %s: %w", repo, err)
	}
	return dir, nil
}

func ensureInit(ctx context.Context, dir string) (bool, error) {
	if fileExists(filepath.Join(dir, ".github/agents/agentic-workflows.agent.md")) {
		return false, nil
	}
	cmd := exec.CommandContext(ctx, "gh", "aw", "init")
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return false, err
	}
	return true, nil
}

func runAdd(ctx context.Context, dir, spec, engine string, force bool) (string, error) {
	args := []string{"aw", "add", spec}
	if engine != "" {
		args = append(args, "--engine", engine)
	}
	if force {
		args = append(args, "--force")
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func gitCmd(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ghPRCreate(ctx context.Context, dir, title, body string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--title", title, "--body", body)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", ghErr(err)
	}
	return strings.TrimSpace(string(out)), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// hasStagedOrUnstagedWorkflowChanges checks if git status has any workflow changes.
// Returns true if .github/ has staged or unstaged modifications.
func hasStagedOrUnstagedWorkflowChanges(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return len(out) > 0, nil
}

func condense(out string, err error) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var keep []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "✗") || strings.HasPrefix(t, "error:") || strings.Contains(strings.ToLower(t), "failed") {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 && len(lines) > 0 {
		return lines[len(lines)-1] + " (" + err.Error() + ")"
	}
	return strings.Join(keep, " | ")
}

// commitMessage returns a Conventional Commits-formatted message:
// subject "ci(workflows): add N agentic workflows via gh-aw-fleet" +
// bullet-listed body. Subject stays under 72 chars for commitlint.
func commitMessage(res *DeployResult) string {
	names := make([]string, len(res.Added))
	for i, a := range res.Added {
		names[i] = a.Name
	}
	subject := fmt.Sprintf("ci(workflows): add %d agentic workflows via gh-aw-fleet", len(res.Added))
	body := fmt.Sprintf("Deployed via gh-aw-fleet:\n\n- %s\n", strings.Join(names, "\n- "))
	if len(res.Failed) > 0 {
		fn := make([]string, len(res.Failed))
		for i, f := range res.Failed {
			fn[i] = f.Name
		}
		body += fmt.Sprintf("\nDeferred (pre-flight failed):\n- %s\n", strings.Join(fn, "\n- "))
	}
	return fmt.Sprintf("%s\n\n%s", subject, body)
}

func prBody(res *DeployResult, repo string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Deploys %d agentic workflows to `%s` via [gh-aw-fleet](https://github.com/rshade/gh-aw-fleet).\n\n", len(res.Added), repo)
	if res.InitWasRun {
		b.WriteString("This repo was not yet initialized for gh-aw; `gh aw init` was run as part of this PR.\n\n")
	}
	b.WriteString("## Added\n\n")
	for _, a := range res.Added {
		fmt.Fprintf(&b, "- `%s`\n", a.Spec)
	}
	if len(res.Skipped) > 0 {
		b.WriteString("\n## Already present (skipped)\n\n")
		for _, s := range res.Skipped {
			fmt.Fprintf(&b, "- `%s` — %s\n", s.Name, s.Reason)
		}
	}
	if len(res.Failed) > 0 {
		b.WriteString("\n## Deferred — failed pre-flight\n\n")
		for _, f := range res.Failed {
			fmt.Fprintf(&b, "- `%s`: %s\n", f.Name, f.Error)
		}
	}
	b.WriteString("\nEach workflow is pinned via its frontmatter `source:` field. Use `gh aw update` to pull upstream changes.\n")
	return b.String()
}
