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

// EngineSecretInfo holds the Actions secret name and where to obtain the key for a given engine.
// Kept in sync with github/gh-aw pkg/constants/engine_constants.go EngineOptions.
// Do NOT import github.com/github/gh-aw directly — its dependency tree is too large (TUI libs, SDKs, etc.).
type EngineSecretInfo struct {
	SecretName string
	KeyURL     string
}

// EngineSecrets maps engine name → secret metadata.
//
//nolint:gochecknoglobals // immutable engine→secret lookup; Go has no const map
var EngineSecrets = map[string]EngineSecretInfo{
	"copilot": {"COPILOT_GITHUB_TOKEN", "https://github.com/settings/personal-access-tokens/new"},
	"claude":  {"ANTHROPIC_API_KEY", "https://console.anthropic.com/settings/keys"},
	"codex":   {"OPENAI_API_KEY", "https://platform.openai.com/api-keys"},
	"gemini":  {"GEMINI_API_KEY", "https://aistudio.google.com/app/apikey"},
}

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
	Repo          string            `json:"repo"`
	CloneDir      string            `json:"clone_dir"`
	Added         []WorkflowOutcome `json:"added"`
	Skipped       []WorkflowOutcome `json:"skipped"`
	Failed        []WorkflowOutcome `json:"failed"`
	InitWasRun    bool              `json:"init_was_run"`
	BranchPushed  string            `json:"branch_pushed"`
	PRURL         string            `json:"pr_url"`
	MissingSecret string            `json:"missing_secret"` // non-empty if the engine secret is absent from the repo
	SecretKeyURL  string            `json:"secret_key_url"` // where to obtain the key for MissingSecret
}

// WorkflowOutcome is one workflow's fate during a deploy.
type WorkflowOutcome struct {
	Name   string `json:"name"`
	Spec   string `json:"spec"`
	Reason string `json:"reason"`
	Error  string `json:"error"`
}

// BuildMissingSecretMessage returns the single-line, human-readable warning
// shown when the engine secret is absent on the deploy target. Shared by
// the stderr (zerolog) emission and the JSON envelope's warnings[] entry.
// The PR body's setup-required section is rendered separately by
// missingSecretPRSection from the same DeployResult fields, so all three
// surfaces describe the same failure with the same fix.
func BuildMissingSecretMessage(res *DeployResult) string {
	msg := fmt.Sprintf(
		"Actions secret %q is not set on %s; workflows will fail until added (gh secret set %s --repo %s)",
		res.MissingSecret, res.Repo, res.MissingSecret, res.Repo,
	)
	if res.SecretKeyURL != "" {
		msg = fmt.Sprintf("%s — obtain the key at %s", msg, res.SecretKeyURL)
	}
	return msg
}

// missingSecretPRSection renders the markdown block surfaced near the top
// of a deploy PR body when DeployResult.MissingSecret is non-empty. Composes
// from the same DeployResult fields as BuildMissingSecretMessage so the PR
// body, stderr warning, and JSON envelope all describe the same failure
// with the same fix.
func missingSecretPRSection(res *DeployResult) string {
	var b strings.Builder
	b.WriteString("## ⚠ Setup required — Actions secret missing\n\n")
	fmt.Fprintf(&b,
		"The `%s` secret is not set on `%s`. "+
			"Workflows added in this PR will fail until it is configured.\n\n",
		res.MissingSecret, res.Repo,
	)
	b.WriteString("**To fix before or after merging:**\n\n")
	fmt.Fprintf(&b, "```sh\ngh secret set %s --repo %s\n```\n", res.MissingSecret, res.Repo)
	if res.SecretKeyURL != "" {
		fmt.Fprintf(&b, "\nObtain the key at: %s\n", res.SecretKeyURL)
	}
	return b.String()
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

	if opts.WorkDir != "" {
		resumed, handled, resumeErr := handleWorkDirResume(ctx, cfg, repo, res, opts)
		if handled {
			return resumed, resumeErr
		}
		fmt.Fprintf(os.Stderr,
			"(--work-dir %s has no resume signals; running fresh pipeline)\n", opts.WorkDir)
	}

	res.InitWasRun, err = ensureInit(ctx, res.CloneDir)
	if err != nil {
		return res, fmt.Errorf("gh aw init: %w", err)
	}

	engine := cfg.EffectiveEngine(repo)
	addResolvedWorkflows(ctx, res, resolved, opts, engine)

	// Check that the engine secret exists on the target repo regardless of dry-run/apply.
	res.MissingSecret, res.SecretKeyURL = checkEngineSecret(ctx, repo, engine)

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
	return createDeployPR(ctx, res, repo, opts, "")
}

// handleWorkDirResume tries to resume a prior --work-dir run at the commit
// gate (staged .github/ changes) or push gate (unpushed commits). Returns
// (result, handled, err): handled=true means the caller should return
// immediately with (result, err); handled=false means no resume signals
// were found and the caller should fall through to a fresh-pipeline deploy.
// Resume-only safety checks (refuse-on-default-branch, conflicting --branch)
// fire only when a resume action is about to occur, so a clean work-dir on
// main can still be used as a fresh-clone target.
func handleWorkDirResume(
	ctx context.Context, cfg *Config, repo string, res *DeployResult, opts DeployOpts,
) (*DeployResult, bool, error) {
	branch, err := gitCurrentBranch(ctx, res.CloneDir)
	if err != nil {
		return res, true, fmt.Errorf("work-dir %q is not a valid git repository: %w", opts.WorkDir, err)
	}

	staged, err := hasStagedOrUnstagedWorkflowChanges(ctx, res.CloneDir)
	if err != nil {
		return res, true, err
	}
	hasCommits, err := gitHasUnpushedCommits(ctx, res.CloneDir)
	if err != nil {
		return res, true, err
	}

	if !staged && !hasCommits {
		return res, false, nil
	}

	if isDefaultBranch(branch) {
		return res, true, fmt.Errorf(
			"work-dir is on default branch %q with pending changes; refusing to resume on a "+
				"protected branch. Switch to the deploy branch or omit --work-dir for a fresh clone",
			branch,
		)
	}
	if opts.Branch != "" && opts.Branch != branch {
		return res, true, fmt.Errorf(
			"--branch %q conflicts with current branch %q in work-dir; "+
				"omit --branch to resume on the existing branch",
			opts.Branch, branch,
		)
	}

	if staged {
		fmt.Fprintf(os.Stderr, "(resumed from --work-dir %s at commit gate)\n", opts.WorkDir)
		engine := cfg.EffectiveEngine(repo)
		res.MissingSecret, res.SecretKeyURL = checkEngineSecret(ctx, repo, engine)
		if !opts.Apply {
			return res, true, nil
		}
		out, prErr := createDeployPR(ctx, res, repo, opts, branch)
		return out, true, prErr
	}

	fmt.Fprintf(os.Stderr, "(resumed from --work-dir %s at push gate)\n", opts.WorkDir)
	engine := cfg.EffectiveEngine(repo)
	res.MissingSecret, res.SecretKeyURL = checkEngineSecret(ctx, repo, engine)
	if !opts.Apply {
		return res, true, nil
	}
	out, prErr := pushAndCreatePR(ctx, res, repo, opts, branch, nil)
	return out, true, prErr
}

// addResolvedWorkflows runs `gh aw add` for each resolved workflow, populating
// res.Added / res.Skipped / res.Failed.
func addResolvedWorkflows(
	ctx context.Context, res *DeployResult, resolved []ResolvedWorkflow, opts DeployOpts, engine string,
) {
	for _, w := range resolved {
		if w.Source == SourceLocal {
			continue
		}
		if fileExists(filepath.Join(res.CloneDir, ".github/workflows", w.Name+".md")) && !opts.Force {
			res.Skipped = append(res.Skipped, WorkflowOutcome{
				Name: w.Name, Spec: w.Spec(), Reason: "already present",
			})
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
}

// createDeployPR branches, commits, pushes, and opens a PR for a deploy that
// has pending workflow changes staged in res.CloneDir.
// If resumeBranch is non-empty, the branch already exists and changes are staged.
func createDeployPR(
	ctx context.Context, res *DeployResult, repo string, opts DeployOpts, resumeBranch string,
) (*DeployResult, error) {
	branch := opts.Branch
	if branch == "" {
		if resumeBranch != "" {
			branch = resumeBranch
		} else {
			branch = fmt.Sprintf("fleet/deploy-%s", time.Now().UTC().Format("2006-01-02-150405"))
		}
	}

	if resumeBranch == "" {
		if err := branchAndStageGithub(ctx, res.CloneDir, branch); err != nil {
			return res, err
		}
	} else if err := assertResumeStagedScopedToGithub(ctx, res.CloneDir); err != nil {
		return res, err
	}

	stagedNames, err := stagedWorkflowNames(ctx, res.CloneDir)
	if err != nil {
		return res, fmt.Errorf("git diff --cached: %w", err)
	}
	msg := commitMessage(res, stagedNames)
	if commitErr := gitCmdInteractive(ctx, res.CloneDir, "commit", "-m", msg); commitErr != nil {
		return res, fmt.Errorf(
			"git commit: %w\n\nTo finish manually:\n  cd %s\n"+
				"  git commit -m \"$(cat <<'EOF'\n%s\nEOF\n)\"\n"+
				"  git push -u origin %s",
			commitErr, res.CloneDir, msg, branch,
		)
	}

	return pushAndCreatePR(ctx, res, repo, opts, branch, stagedNames)
}

// pushAndCreatePR pushes the current branch to origin and opens a PR.
func pushAndCreatePR(
	ctx context.Context, res *DeployResult, repo string, opts DeployOpts,
	branch string, stagedNames []string,
) (*DeployResult, error) {
	if pushErr := gitCmd(ctx, res.CloneDir, "push", "-u", "origin", branch); pushErr != nil {
		return res, fmt.Errorf("git push: %w", pushErr)
	}
	res.BranchPushed = branch

	addedCount := len(res.Added)
	if addedCount == 0 {
		addedCount = len(stagedNames)
	}
	if addedCount == 0 {
		committed, committedErr := committedWorkflowNames(ctx, res.CloneDir)
		if committedErr != nil {
			return res, fmt.Errorf("git diff-tree HEAD: %w", committedErr)
		}
		addedCount = len(committed)
	}

	title := opts.PRTitle
	if title == "" {
		title = fmt.Sprintf("ci(workflows): add %d agentic workflows via gh-aw-fleet", addedCount)
	}
	prURL, prErr := ghPRCreate(ctx, res.CloneDir, title, prBody(res, repo, addedCount))
	if prErr != nil {
		return res, fmt.Errorf("gh pr create: %w", prErr)
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
	if runErr := runLogged(cmd, "gh", "repo clone", map[string]string{"repo": repo, "clone_dir": dir}); runErr != nil {
		return dir, fmt.Errorf("gh repo clone %s: %w", repo, runErr)
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
	if err := runLogged(cmd, "gh", "aw init", map[string]string{"clone_dir": dir}); err != nil {
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
	out, err := runLoggedCombined(cmd, "gh", "aw add", map[string]string{"clone_dir": dir})
	return string(out), err
}

func gitCmd(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := runLoggedCombined(cmd, "git", subcommandLabel(args), map[string]string{"clone_dir": dir})
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// gitCmdInteractive runs a git command with stdio wired to the terminal so
// that gpg-agent's pinentry prompt can reach the user. Use for git commit.
func gitCmdInteractive(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := runLogged(cmd, "git", subcommandLabel(args), map[string]string{"clone_dir": dir}); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func ghPRCreate(ctx context.Context, dir, title, body string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--title", title, "--body", body)
	cmd.Dir = dir
	out, err := runLoggedOutput(cmd, "gh", "pr create", map[string]string{"clone_dir": dir})
	if err != nil {
		return "", ghErr(err)
	}
	return strings.TrimSpace(string(out)), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// stagedWorkflowNames returns the names (without .md) of workflow markdown
// files staged in the index under .github/workflows/. Used to build accurate
// commit messages on resume, when res.Added may be empty.
func stagedWorkflowNames(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-only")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --cached: %w", err)
	}
	return filterWorkflowMarkdownNames(out), nil
}

// branchAndStageGithub creates the deploy branch and stages everything under
// .github/. Used on the fresh-deploy path of createDeployPR.
func branchAndStageGithub(ctx context.Context, dir, branch string) error {
	if err := gitCmd(ctx, dir, "checkout", "-b", branch); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}
	if err := gitCmd(ctx, dir, "add", ".github/"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	return nil
}

// assertResumeStagedScopedToGithub returns an error if the index in dir has
// any staged path outside .github/. Used at the resume commit gate to refuse
// committing unrelated edits a preserved work-dir might still hold.
func assertResumeStagedScopedToGithub(ctx context.Context, dir string) error {
	extras, err := stagedNonGithubPaths(ctx, dir)
	if err != nil {
		return err
	}
	if len(extras) > 0 {
		return fmt.Errorf(
			"refusing to resume: %d staged path(s) outside .github/ would be committed: %s. "+
				"Unstage them with `git restore --staged <path>` before retrying",
			len(extras), strings.Join(extras, ", "),
		)
	}
	return nil
}

// stagedNonGithubPaths returns paths in the index that are NOT under
// .github/. Used at the resume commit gate to refuse committing unrelated
// staged edits that may have been left in a preserved work-dir.
func stagedNonGithubPaths(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--name-only")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --cached: %w", err)
	}
	var paths []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, ".github/") {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// committedWorkflowNames returns the names (without .md) of workflow markdown
// files in the most recent commit. Used as a fallback for PR title/body when
// resuming after a manual commit.
func committedWorkflowNames(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff-tree: %w", err)
	}
	return filterWorkflowMarkdownNames(out), nil
}

// filterWorkflowMarkdownNames extracts basenames (without .md) of paths under
// .github/workflows/ from newline-delimited git output (one path per line).
func filterWorkflowMarkdownNames(out []byte) []string {
	var names []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, ".github/workflows/") && strings.HasSuffix(line, ".md") {
			names = append(names, strings.TrimSuffix(filepath.Base(line), ".md"))
		}
	}
	return names
}

// gitCurrentBranch returns the name of the current branch in dir.
func gitCurrentBranch(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// isDefaultBranch reports whether branch is a protected default branch name.
func isDefaultBranch(branch string) bool {
	return branch == "main" || branch == "master"
}

// gitHasUnpushedCommits reports whether the current HEAD has commits that are
// not present on any remote branch.
func gitHasUnpushedCommits(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "branch", "-r", "--contains", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git branch -r --contains HEAD: %w", err)
	}
	return len(strings.TrimSpace(string(out))) == 0, nil
}

// ghAPIExists returns true if `gh api <path>` returns success (exit 0).
// Package variable so tests can stub the subprocess.
//
//nolint:gochecknoglobals // test seam; tests override this to stub gh api calls
var ghAPIExists = func(ctx context.Context, path string) bool {
	return exec.CommandContext(ctx, "gh", "api", path).Run() == nil
}

// checkEngineSecret verifies the required engine secret exists at the repo or org level.
// Returns (secretName, keyURL) if missing at both levels, ("", "") if present at either
// level or the engine is unknown. A 404 from either endpoint is treated as "not found"
// (matches the personal-account case and the org-without-secret case).
func checkEngineSecret(ctx context.Context, repo, engine string) (string, string) {
	info, ok := EngineSecrets[engine]
	if !ok {
		return "", ""
	}
	if ghAPIExists(ctx, fmt.Sprintf("/repos/%s/actions/secrets/%s", repo, info.SecretName)) {
		return "", ""
	}
	org, _, _ := strings.Cut(repo, "/")
	if ghAPIExists(ctx, fmt.Sprintf("/orgs/%s/actions/secrets/%s", org, info.SecretName)) {
		return "", ""
	}
	return info.SecretName, info.KeyURL
}

// hasStagedOrUnstagedWorkflowChanges reports whether dir has staged or
// unstaged modifications under .github/. The pathspec scope matches the
// helper's name and the resume-gate semantics: only changes the deploy
// pipeline would commit (workflow markdown, agentic infrastructure) should
// trigger a resume. Unrelated edits in the work-dir do not.
func hasStagedOrUnstagedWorkflowChanges(ctx context.Context, dir string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "--", ".github/")
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
		if strings.HasPrefix(t, "✗") ||
			strings.HasPrefix(t, "error:") ||
			strings.Contains(strings.ToLower(t), "failed") {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 && len(lines) > 0 {
		return lines[len(lines)-1] + " (" + err.Error() + ")"
	}
	return strings.Join(keep, " | ")
}

// commitMessage returns a Conventional Commits-formatted message.
// stagedFallback is used when res.Added is empty (e.g. resume after partial apply).
func commitMessage(res *DeployResult, stagedFallback []string) string {
	names := make([]string, len(res.Added))
	for i, a := range res.Added {
		names[i] = a.Name
	}
	if len(names) == 0 {
		names = stagedFallback
	}
	subject := fmt.Sprintf("ci(workflows): add %d agentic workflows via gh-aw-fleet", len(names))
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

func prBody(res *DeployResult, repo string, addedCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"Deploys %d agentic workflows to `%s` via [gh-aw-fleet](https://github.com/rshade/gh-aw-fleet).\n\n",
		addedCount, repo,
	)
	if res.MissingSecret != "" {
		b.WriteString(missingSecretPRSection(res))
		b.WriteString("\n")
	}
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
	b.WriteString("\nEach workflow is pinned via its frontmatter `source:` field. " +
		"Use `gh aw update` to pull upstream changes.\n")
	return b.String()
}
