package fleet

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	zlog "github.com/rs/zerolog/log"

	"github.com/rshade/gh-aw-fleet/internal/fleet/security"
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
	engineCopilot: {secretCopilotGitHubToken, keyURLCopilot},
	engineClaude:  {secretAnthropicAPIKey, keyURLAnthropic},
	engineCodex:   {secretOpenAIAPIKey, keyURLOpenAI},
	engineGemini:  {secretGeminiAPIKey, keyURLGemini},
}

// DeployOpts controls deploy behavior.
type DeployOpts struct {
	Apply          bool // false = dry-run (default); true = commit + push + PR
	Force          bool // pass --force to gh aw add
	Branch         string
	PRTitle        string
	WorkDir        string // if set, use this clone path; otherwise tmp.
	InternalClone  bool   // WorkDir was prepared by this process; not a user resume request.
	InitAlreadyRan bool   // caller already ran gh aw init on WorkDir during this operation.
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

	// ActionsDisabled is true only when GitHub Actions is observably disabled
	// on the target repo (positive evidence: 200 OK + enabled=false). It
	// remains false for every indeterminate response (403/5xx/missing field/
	// network error) per the fail-open contract — false means "no warning,"
	// not "Actions is enabled."
	ActionsDisabled bool `json:"actions_disabled"`

	// WorkflowTokenReadOnly is true only on positive evidence: 200 OK +
	// default_workflow_permissions=="read". Same fail-open semantics as
	// ActionsDisabled — indeterminate responses leave it false.
	WorkflowTokenReadOnly bool `json:"workflow_token_read_only"`

	// CompileStrictApplied is true ONLY when `gh aw compile --strict` was
	// invoked AND exited 0 during this Deploy run. False covers both
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

	// SecurityFindings is the sorted output of security.Run; nil when the
	// scanner has not run (e.g. resume-from-work-dir paths that bypass
	// addResolvedWorkflows). Findings are advisory: they never block the
	// deploy. Surfaced on stderr (zerolog), in the JSON envelope
	// warnings[], and in the PR body's `## Security Findings` section.
	SecurityFindings []security.Finding `json:"security_findings"`
}

// WorkflowOutcome is one workflow's fate during a deploy.
type WorkflowOutcome struct {
	Name   string `json:"name"`
	Spec   string `json:"spec"`
	Reason string `json:"reason"`
	Error  string `json:"error"`
}

// ActionsSettingsURL returns the GitHub web URL where the operator can
// toggle "Actions permissions" and "Workflow permissions" for the given
// repo. Both preflight findings (Actions-disabled, workflow-token-read-only)
// share this URL because GitHub renders both controls on a single page.
// Embedded in stderr warnings, JSON envelope fields["url"], and PR-body
// sub-blocks so all three surfaces send the operator to the same place.
func ActionsSettingsURL(repo string) string {
	return fmt.Sprintf("https://github.com/%s/settings/actions", repo)
}

// BuildActionsDisabledMessage returns the single-line warning shown when
// the deploy target has GitHub Actions disabled. The trailing "enable at
// <URL>" suffix is part of the message contract — both the stderr warning
// and the JSON envelope's warnings[].message reuse this string so the
// operator sees the same actionable URL in every channel.
func BuildActionsDisabledMessage(repo string) string {
	return fmt.Sprintf(
		"GitHub Actions is disabled on %s — enable at %s",
		repo, ActionsSettingsURL(repo),
	)
}

// BuildWorkflowTokenReadOnlyMessage returns the single-line warning shown
// when the repo's default workflow token permission is "read." The
// consequence sentence ("workflows that push commits or create reviews
// will fail") is included in the message because operators may not
// understand why their write workflows fail until they see the concrete
// consequence spelled out. The settings control text is also part of the
// contract surface so stderr and JSON users know exactly which setting to
// change after following the URL.
func BuildWorkflowTokenReadOnlyMessage(repo string) string {
	return fmt.Sprintf(
		"GITHUB_TOKEN is read-only on %s — workflows that push commits or create reviews will fail; set \"Workflow permissions\" → \"Read and write permissions\" at %s",
		repo,
		ActionsSettingsURL(repo),
	)
}

// BuildMissingSecretMessage returns the single-line, human-readable warning
// shown when the engine secret is absent on the deploy target. Shared by
// the stderr (zerolog) emission and the JSON envelope's warnings[] entry.
// The PR body's setup-required section is rendered separately by
// setupRequiredSection from the same DeployResult fields, so all three
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

// setupRequiredSection renders the umbrella "## ⚠ Setup required" markdown
// block for the deploy PR body, with one sub-block per active preflight
// finding in fixed order: ActionsDisabled → WorkflowTokenReadOnly →
// MissingSecret. Returns the empty string when no findings are active so
// the caller suppresses the heading entirely. The fixed order matches the
// stderr warning order and the JSON envelope's warnings[] order, so all
// three surfaces describe the same findings in the same sequence.
func setupRequiredSection(res *DeployResult) string {
	if !res.ActionsDisabled && !res.WorkflowTokenReadOnly && res.MissingSecret == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## ⚠ Setup required\n\n")
	if res.ActionsDisabled {
		fmt.Fprintf(&b,
			"**GitHub Actions is disabled on `%s`.** "+
				"Workflows added in this PR will not run until Actions is enabled.\n\n",
			res.Repo,
		)
		fmt.Fprintf(&b, "Enable at: %s\n\n", ActionsSettingsURL(res.Repo))
	}
	if res.WorkflowTokenReadOnly {
		fmt.Fprintf(&b,
			"**Workflow token is read-only on `%s`.** "+
				"Agentic workflows that push commits or create reviews will fail.\n\n",
			res.Repo,
		)
		fmt.Fprintf(&b, "Fix at: %s\n\n", ActionsSettingsURL(res.Repo))
		b.WriteString("Set \"Workflow permissions\" → \"Read and write permissions\"\n\n")
	}
	if res.MissingSecret != "" {
		fmt.Fprintf(&b,
			"**Engine secret missing on `%s`.** The `%s` secret is not set. "+
				"Workflows added in this PR will fail until it is configured.\n\n",
			res.Repo, res.MissingSecret,
		)
		fmt.Fprintf(&b, "```sh\ngh secret set %s --repo %s\n```\n", res.MissingSecret, res.Repo)
		if res.SecretKeyURL != "" {
			fmt.Fprintf(&b, "\nObtain the key at: %s\n", res.SecretKeyURL)
		}
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

	if opts.WorkDir != "" && !opts.InternalClone {
		resumed, handled, resumeErr := handleWorkDirResume(ctx, cfg, repo, res, opts)
		if handled {
			return resumed, resumeErr
		}
		fmt.Fprintf(os.Stderr,
			"(--work-dir %s has no resume signals; running fresh pipeline)\n", opts.WorkDir)
	}

	fleetPin := resolvedGhAwPin(cfg, repo)
	if opts.InitAlreadyRan {
		res.InitWasRun = true
	} else {
		res.InitWasRun, err = ensureInit(ctx, res.CloneDir, fleetPin)
		if err != nil {
			return res, fmt.Errorf("gh aw init: %w", err)
		}
	}

	engine := cfg.EffectiveEngine(repo)
	addResolvedWorkflows(ctx, res, resolved, opts, engine)

	// Work around a gh aw add path bug (observed v0.79.2): a workflow that
	// imports a skill via a repo-relative path (e.g. ../skills/jqschema/SKILL.md)
	// has that skill materialized one level too deep under
	// .github/workflows/.github/ instead of .github/, so the import fails to
	// resolve and gh aw add's own compile aborts before writing the .lock.yml.
	// Relocate the tree, then recompile the affected workflows so their lock
	// files exist for the commit. Runs in dry-run too, so the dry-run reflects
	// reality.
	if err = fixMisplacedSkillImports(ctx, res, repo, engine); err != nil {
		return res, err
	}

	// Check that the engine secret exists on the target repo regardless of dry-run/apply.
	res.MissingSecret, res.SecretKeyURL = checkEngineSecret(ctx, repo, engine)
	res.ActionsDisabled, res.WorkflowTokenReadOnly = checkActionsSettings(ctx, repo)

	res.SecurityFindings = security.Run(ctx, res.CloneDir)

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

	if manifestErr := writeDeployManifest(ctx, cfg, repo, res.CloneDir); manifestErr != nil {
		return res, manifestErr
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
		res.ActionsDisabled, res.WorkflowTokenReadOnly = checkActionsSettings(ctx, repo)
		if !opts.Apply {
			effective, source := logCompileStrictResolution(ctx, cfg, repo)
			res.CompileStrictSource = source
			res.CompileStrictEffective = effective
			return res, true, nil
		}
		if compileErr := refreshResumeCompileStrict(ctx, cfg, repo, res); compileErr != nil {
			return res, true, compileErr
		}
		if manifestErr := writeDeployManifest(ctx, cfg, repo, res.CloneDir); manifestErr != nil {
			return res, true, manifestErr
		}
		out, prErr := createDeployPR(ctx, res, repo, opts, branch)
		return out, true, prErr
	}

	fmt.Fprintf(os.Stderr, "(resumed from --work-dir %s at push gate)\n", opts.WorkDir)
	engine := cfg.EffectiveEngine(repo)
	res.MissingSecret, res.SecretKeyURL = checkEngineSecret(ctx, repo, engine)
	res.ActionsDisabled, res.WorkflowTokenReadOnly = checkActionsSettings(ctx, repo)
	if !opts.Apply {
		// Dry-run resume reports the current policy without mutating the
		// preserved work-dir. Apply-mode push-gate resume re-runs the
		// compile path below before pushing the saved branch.
		effective, source := logCompileStrictResolution(ctx, cfg, repo)
		res.CompileStrictSource = source
		res.CompileStrictEffective = effective
		return res, true, nil
	}
	if compileErr := refreshResumePushCompileStrict(ctx, cfg, repo, res, branch); compileErr != nil {
		return res, true, compileErr
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
// has pending workflow changes in res.CloneDir. If resumeBranch is non-empty,
// the branch already exists and .github/ changes are restaged after resume
// safety checks.
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
	} else {
		if err := stageResumeGithub(ctx, res.CloneDir); err != nil {
			return res, err
		}
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

// refreshResumeCompileStrict re-runs the current compile-strict policy for a
// preserved work-dir. Strict-on policy uses `gh aw compile --strict`; strict-off
// policy uses a plain `gh aw compile` so a retry after an opt-out can replace
// lockfiles generated by an earlier strict run.
func refreshResumeCompileStrict(ctx context.Context, cfg *Config, repo string, res *DeployResult) error {
	if compileErr := runCompileStrictIfNeeded(ctx, res, cfg, repo); compileErr != nil {
		return compileErr
	}
	if res.CompileStrictEffective || res.CompileStrictSource == "" {
		return nil
	}
	compileOut, compileErr := runGhAwCompile(ctx, res.CloneDir)
	if compileErr != nil {
		trimmedOut := strings.TrimSpace(compileOut)
		if trimmedOut != "" {
			return fmt.Errorf("gh aw compile: %s: %w", trimmedOut, compileErr)
		}
		return fmt.Errorf("gh aw compile: %w", compileErr)
	}
	return nil
}

// refreshResumePushCompileStrict re-runs the current compile-strict policy
// and refreshes the fleet manifest before pushing a branch that was previously
// committed but not pushed. If either step changes .github/, the unpushed
// commit is amended so the pushed branch matches the reported policy and
// records the current manifest.
func refreshResumePushCompileStrict(
	ctx context.Context, cfg *Config, repo string, res *DeployResult, branch string,
) error {
	if compileErr := refreshResumeCompileStrict(ctx, cfg, repo, res); compileErr != nil {
		return compileErr
	}
	if manifestErr := writeDeployManifest(ctx, cfg, repo, res.CloneDir); manifestErr != nil {
		return manifestErr
	}
	changed, err := hasStagedOrUnstagedWorkflowChanges(ctx, res.CloneDir)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if stageErr := stageResumeGithub(ctx, res.CloneDir); stageErr != nil {
		return stageErr
	}
	if amendErr := gitCmdInteractive(ctx, res.CloneDir, "commit", "--amend", "--no-edit"); amendErr != nil {
		return fmt.Errorf(
			"git commit --amend: %w\n\nTo finish manually:\n  cd %s\n"+
				"  git commit --amend --no-edit\n"+
				"  git push -u origin %s",
			amendErr, res.CloneDir, branch,
		)
	}
	return nil
}

func writeDeployManifest(ctx context.Context, cfg *Config, repo, cloneDir string) error {
	cliVer, _ := ghAwVersion(ctx)
	m := buildManifest(cfg, repo, cliVer)
	if _, writeErr := writeManifestIfNeeded(cloneDir, m); writeErr != nil {
		return fmt.Errorf("write fleet manifest: %w", writeErr)
	}
	return nil
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
	fields := map[string]string{fieldRepo: repo, fieldCloneDir: dir}
	if runErr := runLogged(cmd, "gh", "repo clone", fields); runErr != nil {
		return dir, fmt.Errorf("gh repo clone %s: %w", repo, runErr)
	}
	return dir, nil
}

// ensureInit ensures the target clone has been initialized for gh-aw. It
// supersedes the former initMarkerPaths file-check: rather than asking "has
// init ever run?", it asks "has init run at the fleet's current gh-aw version?"
// If fleetGhAwVersion is non-empty and the clone's fleet manifest records that
// exact version, init is skipped — the artifacts are already current. Otherwise
// (no manifest, unreadable manifest, or version mismatch) gh aw init runs to
// refresh init artifacts to the current version.
func ensureInit(ctx context.Context, dir, fleetGhAwVersion string) (bool, error) {
	if fleetGhAwVersion != "" {
		m, _ := readManifestFromClone(dir)
		if m != nil && m.GhAwVersion == fleetGhAwVersion {
			return false, nil
		}
	}
	cmd := exec.CommandContext(ctx, "gh", "aw", "init")
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := runLogged(cmd, "gh", "aw init", map[string]string{fieldCloneDir: dir}); err != nil {
		return false, err
	}
	return true, nil
}

// fixMisplacedSkillImports relocates a mis-nested .github/workflows/.github
// tree up to .github/ and, when a relocation occurred, recompiles the
// just-added workflows so their lock files exist. No-op when nothing was
// mis-placed. See the call site in Deploy for the upstream gh aw add bug this
// works around.
func fixMisplacedSkillImports(ctx context.Context, res *DeployResult, repo, engine string) error {
	moved, err := relocateNestedDotGitHub(res.CloneDir)
	if err != nil {
		return fmt.Errorf("relocate nested .github: %w", err)
	}
	if !moved {
		return nil
	}
	zlog.Warn().
		Str("event", "relocated_nested_github").
		Str(fieldRepo, repo).
		Str(fieldCloneDir, res.CloneDir).
		Msg("moved mis-placed skill imports from .github/workflows/.github up to .github (gh aw add path bug); recompiling affected workflows")
	return recompileAddedWorkflows(ctx, res, engine)
}

// relocateNestedDotGitHub moves every file under
// <cloneDir>/.github/workflows/.github/ up to <cloneDir>/.github/, merging into
// existing directories, then removes the emptied nested tree. Returns whether
// any file was moved; a missing nested tree is a no-op returning false. Files
// are collected before any move so the walk is not mutated mid-iteration.
func relocateNestedDotGitHub(cloneDir string) (bool, error) {
	nested := filepath.Join(cloneDir, ".github", "workflows", ".github")
	info, err := os.Stat(nested)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	var files []string
	walkErr := filepath.WalkDir(nested, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if walkErr != nil {
		return false, walkErr
	}
	target := filepath.Join(cloneDir, ".github")
	for _, src := range files {
		rel, relErr := filepath.Rel(nested, src)
		if relErr != nil {
			return true, relErr
		}
		dst := filepath.Join(target, rel)
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o750); mkErr != nil {
			return true, mkErr
		}
		if rnErr := os.Rename(src, dst); rnErr != nil {
			return true, rnErr
		}
	}
	if rmErr := os.RemoveAll(nested); rmErr != nil {
		return len(files) > 0, rmErr
	}
	return len(files) > 0, nil
}

// recompileAddedWorkflows runs `gh aw compile` on each workflow this deploy
// added, regenerating the .lock.yml that gh aw add failed to write when a
// mis-placed skill import broke its in-process compile. It MUST forward the
// same engine override gh aw add used (--engine) — without it, compile re-reads
// the workflow's native frontmatter engine (e.g. `engine: claude`) and silently
// reverts the deploy's intended engine, producing a lock that demands the wrong
// secret at runtime.
func recompileAddedWorkflows(ctx context.Context, res *DeployResult, engine string) error {
	for _, w := range res.Added {
		mdPath := filepath.Join(".github", "workflows", w.Name+".md")
		args := []string{"aw", "compile", mdPath}
		if engine != "" {
			args = append(args, "--engine", engine)
		}
		cmd := exec.CommandContext(ctx, "gh", args...)
		cmd.Dir = res.CloneDir
		out, err := runLoggedCombined(cmd, "gh", "aw compile", map[string]string{fieldCloneDir: res.CloneDir})
		if err != nil {
			return fmt.Errorf("gh aw compile %s: %w: %s", mdPath, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func runAdd(ctx context.Context, dir, spec, engine string, force bool) (string, error) {
	args := []string{"aw", addToken, spec}
	if engine != "" {
		args = append(args, "--engine", engine)
	}
	if force {
		args = append(args, "--force")
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	out, err := runLoggedCombined(cmd, "gh", "aw add", map[string]string{fieldCloneDir: dir})
	return string(out), err
}

func gitCmd(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := runLoggedCombined(cmd, "git", subcommandLabel(args), map[string]string{fieldCloneDir: dir})
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
	if err := runLogged(cmd, "git", subcommandLabel(args), map[string]string{fieldCloneDir: dir}); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func ghPRCreate(ctx context.Context, dir, title, body string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--title", title, "--body", body)
	cmd.Dir = dir
	out, err := runLoggedOutput(cmd, "gh", "pr create", map[string]string{fieldCloneDir: dir})
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
	if err := gitCmd(ctx, dir, addToken, ".github/"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	return nil
}

// stageResumeGithub stages .github/ during a resumed deploy after confirming
// that no unrelated staged paths would be swept into the commit. Used after
// compile-strict reruns because they can regenerate lock files after the
// original resume signal was detected.
func stageResumeGithub(ctx context.Context, dir string) error {
	if err := assertResumeStagedScopedToGithub(ctx, dir); err != nil {
		return err
	}
	if err := gitCmd(ctx, dir, addToken, ".github/"); err != nil {
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
	return branch == branchMain || branch == branchMaster
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

// RepoVisibility returns the repo's `visibility` field via the package-level
// injection seam. Stable export so the `cmd/` layer can render the onboarding
// info line at `gh-aw-fleet add` time without reaching across packages for
// an unexported var. Tests in this package override the seam itself; tests in
// downstream packages should inject their own indirection.
func RepoVisibility(ctx context.Context, repo string) (string, error) {
	return ghRepoVisibility(ctx, repo)
}

// ghRepoVisibility returns the repo's `visibility` field from
// `gh api /repos/<owner>/<repo> --jq .visibility`. Returns the trimmed string
// ("public" | "private" | "internal" | …) or a wrapped error. Empty output
// and the literal jq sentinel "null" are treated as errors so the resolver
// folds into auto-fallback (strict ON) rather than auto-private (strict OFF)
// — preserves the fail-secure contract when GitHub omits the visibility field.
//
//nolint:gochecknoglobals // test-injection seam mirroring ghAPIExists/ghAPIJSON
var ghRepoVisibility = func(ctx context.Context, repo string) (string, error) {
	path := fmt.Sprintf("/repos/%s", repo)
	cmd := exec.CommandContext(ctx, "gh", "api", path, "--jq", ".visibility")
	out, err := cmd.Output()
	if err != nil {
		return "", ghErr(err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" || v == "null" {
		return "", fmt.Errorf("gh api %s --jq .visibility returned %q (missing/null field)", path, v)
	}
	return v, nil
}

// ghAwCompileHelp runs `gh aw compile --help` and returns combined stdout+stderr.
// Used to probe whether the locally-installed `gh aw` advertises the `--strict`
// flag (R2 in research.md).
//
//nolint:gochecknoglobals // test-injection seam mirroring ghAPIExists/ghAPIJSON
var ghAwCompileHelp = func(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "aw", "compile", "--help")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ghAwVersionRE extracts the first vMAJOR.MINOR.PATCH token from
// `gh aw --version` output. Used for diagnostic enrichment only — the gate
// is the flag probe (R3 in research.md).
var ghAwVersionRE = regexp.MustCompile(`v\d+\.\d+\.\d+`)

// ghAwVersion runs `gh aw --version` and returns the parsed semver token
// (e.g. "v0.72.1") or an empty string when parsing fails. The wrapped error
// is non-nil only on exec failure.
//
//nolint:gochecknoglobals // test-injection seam mirroring ghAPIExists/ghAPIJSON
var ghAwVersion = func(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "aw", "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return ghAwVersionRE.FindString(string(out)), nil
}

// runGhAwCompile invokes `gh aw compile` in dir, tee-ing combined stdout+stderr
// to the operator's stderr while also returning the buffered output for error
// messages. Used only by resume paths when the current policy is strict-off.
//
//nolint:gochecknoglobals // test-injection seam mirroring ghAPIExists/ghAPIJSON
var runGhAwCompile = func(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "aw", "compile")
	cmd.Dir = dir
	var buf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stderr, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	err := runLogged(cmd, "gh", "aw compile", map[string]string{fieldCloneDir: dir})
	return buf.String(), err
}

// runGhAwCompileStrict invokes `gh aw compile --strict` in dir, tee-ing
// combined stdout+stderr to the operator's stderr (so compile progress is
// visible) while also returning the buffered output for hint extraction. The
// returned error is non-nil only when the subprocess exits non-zero.
//
//nolint:gochecknoglobals // test-injection seam mirroring ghAPIExists/ghAPIJSON
var runGhAwCompileStrict = func(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "aw", "compile", "--strict")
	cmd.Dir = dir
	var buf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stderr, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	err := runLogged(cmd, "gh", "aw compile --strict", map[string]string{fieldCloneDir: dir})
	return buf.String(), err
}

// compileStrictResult is the minimum surface runCompileStrictIfNeeded needs
// from a Deploy- or Upgrade-result struct. Lets one helper service both
// pipelines without duplication. The accessor is named WorkCloneDir (rather
// than the natural CloneDir) because Go forbids method/field name collisions
// and both DeployResult and UpgradeResult already expose a CloneDir string
// field.
type compileStrictResult interface {
	SetCompileStrictSource(string)
	SetCompileStrictApplied(bool)
	SetCompileStrictEffective(bool)
	WorkCloneDir() string
}

// SetCompileStrictSource implements compileStrictResult.
func (r *DeployResult) SetCompileStrictSource(s string) { r.CompileStrictSource = s }

// SetCompileStrictApplied implements compileStrictResult.
func (r *DeployResult) SetCompileStrictApplied(b bool) { r.CompileStrictApplied = b }

// SetCompileStrictEffective implements compileStrictResult.
func (r *DeployResult) SetCompileStrictEffective(b bool) { r.CompileStrictEffective = b }

// WorkCloneDir implements compileStrictResult.
func (r *DeployResult) WorkCloneDir() string { return r.CloneDir }

// CompileStrictMinVersion is the minimum `gh aw` release this fleet
// supports. The probed `compile --strict` gate has shipped since v0.68.3,
// but the documented/CI floor is v0.79.2 (the FinOps baseline; see #108).
// Surfaced in CompileStrictError.Fields and in the operator-facing
// diagnostic message so consumers can gate on it.
const CompileStrictMinVersion = "v0.79.2"

// logCompileStrictResolution resolves the effective compile-strict policy
// for repo and emits the FR-006 info log line plus the FR-007 warn line on
// auto-fallback. Returns the resolver's effective verdict and source so
// callers can populate their own result fields. Used by every code path
// that needs the resolver outcome — including paths (dry-run, push-gate
// resume) that do NOT invoke the probe or compile subprocess. Centralizing
// the resolve+log keeps the FR-006/FR-007 log shape identical across call
// sites.
func logCompileStrictResolution(ctx context.Context, cfg *Config, repo string) (bool, string) {
	effective, source, reason := cfg.EffectiveCompileStrict(ctx, repo)
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
	return effective, source
}

// runCompileStrictIfNeeded resolves the effective compile-strict policy for
// repo via cfg, emits the FR-006 info log event, optionally emits the FR-007
// warn event on auto-fallback, then probes the local `gh aw compile --help`
// and invokes `gh aw compile --strict` in the result's clone directory when
// the resolver says ON. Failures from probe or compile produce a
// *CompileStrictError carrying the typed Code, ready-to-display Message,
// structured Fields, and the underlying error in Cause for errors.As. The
// work-dir clone is NOT deleted on failure — the caller's existing
// preservation behavior owns that contract (Constitution Principle III).
func runCompileStrictIfNeeded(ctx context.Context, res compileStrictResult, cfg *Config, repo string) error {
	effective, source := logCompileStrictResolution(ctx, cfg, repo)
	res.SetCompileStrictSource(source)
	res.SetCompileStrictEffective(effective)

	if !effective {
		return nil
	}

	helpOut, helpErr := ghAwCompileHelp(ctx)
	if helpErr != nil {
		trimmedHelp := strings.TrimSpace(helpOut)
		message := fmt.Sprintf("gh aw compile --help probe failed for %s: %s", repo, ghAwMissingHint)
		fields := map[string]any{fieldRepo: repo}
		if trimmedHelp != "" {
			fields["help_output"] = trimmedHelp
		}
		return &CompileStrictError{
			Code:    DiagGhAwMissing,
			Message: message,
			Fields:  fields,
			Cause:   helpErr,
		}
	}
	if !strings.Contains(helpOut, "--strict") {
		detected, _ := ghAwVersion(ctx)
		if detected == "" {
			detected = "(version unknown)"
		}
		message := fmt.Sprintf(
			"Local `gh aw` version is too old (detected %s; minimum %s) for %s. %s",
			detected, CompileStrictMinVersion, repo, ghAwTooOldHint,
		)
		return &CompileStrictError{
			Code:    DiagGhAwTooOld,
			Message: message,
			Fields: map[string]any{
				fieldRepo:          repo,
				"detected_version": detected,
				"minimum_version":  CompileStrictMinVersion,
			},
			Cause: fmt.Errorf(
				"gh aw is too old: %s detected, minimum %s required",
				detected, CompileStrictMinVersion,
			),
		}
	}

	cloneDir := res.WorkCloneDir()
	compileOut, compileErr := runGhAwCompileStrict(ctx, cloneDir)
	if compileErr != nil {
		trimmedOut := strings.TrimSpace(compileOut)
		message := fmt.Sprintf("`gh aw compile --strict` failed for %s. %s", repo, compileStrictFailedHint)
		fields := map[string]any{
			fieldRepo:     repo,
			fieldCloneDir: cloneDir,
		}
		if trimmedOut != "" {
			fields["compile_output"] = trimmedOut
		}
		return &CompileStrictError{
			Code:    DiagCompileStrictFailed,
			Message: message,
			Fields:  fields,
			Cause: fmt.Errorf(
				"gh aw compile --strict: %s: %w",
				trimmedOut, compileErr,
			),
		}
	}

	res.SetCompileStrictApplied(true)
	return nil
}

// CompileStrictError is the typed error returned from runCompileStrictIfNeeded
// when the probe or compile aborts. Carries the Diag* code (Code), an
// operator-facing message (Message), structured Fields for envelope
// consumers (per contracts/json-envelope.md), and the underlying error
// (Cause) for errors.As / errors.Is chain walking. Implements error so
// existing callers that just propagate `err` keep working unchanged; the
// cmd-layer envelope code uses errors.As to extract the typed payload.
type CompileStrictError struct {
	Code    string
	Message string
	Fields  map[string]any
	Cause   error
}

// Error implements error. Returns the operator-facing Message so existing
// %w / %v formatting at call sites produces the same human-readable output
// the cmd-layer surfaces. The Cause chain is reachable via Unwrap.
func (e *CompileStrictError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s", e.Message, e.Cause.Error())
	}
	return e.Message
}

// Unwrap returns the underlying cause so errors.Is / errors.As chains
// walk through to root causes (e.g. *exec.ExitError) when needed.
func (e *CompileStrictError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// checkActionsSettings queries the GitHub Actions repo-level permission
// endpoints and returns (actionsDisabled, workflowTokenReadOnly).
//
// Both booleans are true ONLY in response to positive evidence: a 200 OK
// with the disqualifying value present and well-typed. Every other path
// (HTTP 401/403/404/5xx, transport error, malformed JSON, missing field,
// wrong type, unknown enum value) is treated as indeterminate and returns
// false for the affected boolean — never an error to the caller. Fail-open
// is deliberate: a deploy run from a CI environment with a narrow-scoped
// token must complete cleanly with no Actions-settings warnings rather
// than failing or guessing.
//
// Each indeterminate path emits a single zlog.Debug() entry naming the
// repo, endpoint, and reason — invisible at the default --log-level info
// and useful only when an operator is debugging why an expected warning
// did not fire.
//
// The two endpoints are queried independently — neither short-circuits
// the other. The token endpoint's can_approve_pull_request_reviews field
// is intentionally ignored: a write token with PR-review approval disabled
// is still write-permitted for the purposes of this preflight; those two
// repo settings are orthogonal.
func checkActionsSettings(ctx context.Context, repo string) (bool, bool) {
	actionsDisabled := readActionsDisabled(ctx, repo)
	tokenReadOnly := readWorkflowTokenReadOnly(ctx, repo)
	return actionsDisabled, tokenReadOnly
}

// readActionsDisabled reads /repos/<repo>/actions/permissions and returns
// true only when the response carries a positively-typed enabled=false.
func readActionsDisabled(ctx context.Context, repo string) bool {
	path := fmt.Sprintf("/repos/%s/actions/permissions", repo)
	raw, err := ghAPIJSON(ctx, path)
	if err != nil {
		logActionsSettingsSkip(repo, path, classifyAPIError(err))
		return false
	}
	body, ok := raw.(map[string]any)
	if !ok {
		logActionsSettingsSkip(repo, path, "non_object_response")
		return false
	}
	v, ok := body["enabled"]
	if !ok {
		logActionsSettingsSkip(repo, path, "missing_field:enabled")
		return false
	}
	enabled, ok := v.(bool)
	if !ok {
		logActionsSettingsSkip(repo, path, "type_mismatch:enabled")
		return false
	}
	return !enabled
}

// readWorkflowTokenReadOnly reads /repos/<repo>/actions/permissions/workflow
// and returns true only when default_workflow_permissions is observably
// "read". The "write" value returns false; any future GitHub-introduced
// value (e.g., "none") is treated as indeterminate to preserve fail-open
// semantics — we only flag what we positively recognize as read-only.
func readWorkflowTokenReadOnly(ctx context.Context, repo string) bool {
	path := fmt.Sprintf("/repos/%s/actions/permissions/workflow", repo)
	raw, err := ghAPIJSON(ctx, path)
	if err != nil {
		logActionsSettingsSkip(repo, path, classifyAPIError(err))
		return false
	}
	body, ok := raw.(map[string]any)
	if !ok {
		logActionsSettingsSkip(repo, path, "non_object_response")
		return false
	}
	v, ok := body["default_workflow_permissions"]
	if !ok {
		logActionsSettingsSkip(repo, path, "missing_field:default_workflow_permissions")
		return false
	}
	perm, ok := v.(string)
	if !ok {
		logActionsSettingsSkip(repo, path, "type_mismatch:default_workflow_permissions")
		return false
	}
	switch perm {
	case workflowPermissionRead:
		return true
	case workflowPermissionWrite:
		return false
	default:
		logActionsSettingsSkip(repo, path, "unknown_value:"+perm)
		return false
	}
}

// logActionsSettingsSkip records a single Debug entry naming why a
// settings-endpoint read was treated as indeterminate. Hidden at default
// --log-level info; visible at debug.
func logActionsSettingsSkip(repo, endpoint, reason string) {
	zlog.Debug().
		Str(fieldRepo, repo).
		Str("endpoint", endpoint).
		Str("reason", reason).
		Msg("actions-settings preflight skipped")
}

// classifyAPIError maps a ghAPIJSON error into a stable, low-cardinality
// reason string for the debug log. The categories are coarse on purpose:
// debug consumers grep on these tokens to gate alerts, so the set must be
// small and stable across releases.
func classifyAPIError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "HTTP 401"):
		return "http_401"
	case strings.Contains(msg, "HTTP 403"):
		return "http_403"
	case strings.Contains(msg, "HTTP 404"):
		return "http_404"
	case strings.Contains(msg, "HTTP 5"):
		return "http_5xx"
	case strings.Contains(msg, "decode gh api response"):
		return "malformed_json"
	default:
		return "transport_error"
	}
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
	if section := setupRequiredSection(res); section != "" {
		b.WriteString(section)
		b.WriteString("\n")
	}
	if section := security.RenderPRSection(res.SecurityFindings); section != "" {
		b.WriteString(section)
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
