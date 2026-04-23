package fleet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SyncOpts controls sync behavior.
type SyncOpts struct {
	Apply   bool   // if true, call Deploy to add missing workflows
	Prune   bool   // if true and Apply=true, delete drift files before Deploy
	Force   bool   // passed through to Deploy
	WorkDir string // optional existing clone
}

// SyncResult aggregates what happened for a single-repo sync.
type SyncResult struct {
	Repo            string        `json:"repo"`             // repo owner/name
	CloneDir        string        `json:"clone_dir"`        // where the clone lives
	Missing         []string      `json:"missing"`          // workflow names not yet deployed
	Drift           []string      `json:"drift"`            // workflow files present but not declared
	Expected        []string      `json:"expected"`         // workflow names matching fleet.json (informational)
	Deploy          *DeployResult `json:"deploy"`           // set iff Apply==true and Missing/Prune required action
	Pruned          []string      `json:"pruned"`           // files removed when --prune --apply
	DeployPreflight *DeployResult `json:"deploy_preflight"` // set iff Apply==false to surface compilation failures
}

// Sync reconciles one repo's .github/workflows/ to match its declared profile(s).
// Detects missing, drift, and expected workflows, then optionally deploys or prunes.
func Sync(ctx context.Context, cfg *Config, repo string, opts SyncOpts) (*SyncResult, error) {
	resolved, err := cfg.ResolveRepoWorkflows(repo)
	if err != nil {
		return nil, err
	}

	res := &SyncResult{Repo: repo}

	res.CloneDir, err = prepareClone(ctx, repo, opts.WorkDir)
	if err != nil {
		return res, err
	}
	if opts.WorkDir == "" && !opts.Apply {
		defer os.RemoveAll(res.CloneDir)
	}

	if _, initErr := ensureInit(ctx, res.CloneDir); initErr != nil {
		return res, fmt.Errorf("gh aw init: %w", initErr)
	}

	workflowsDir := filepath.Join(res.CloneDir, ".github", "workflows")
	computeDriftAndMissing(res, resolved, workflowsDir)

	if opts.Prune && !opts.Apply {
		return res, errors.New("--prune requires --apply")
	}

	if opts.Apply {
		if applyErr := applyDeployOrPrune(ctx, cfg, repo, res, opts, workflowsDir); applyErr != nil {
			return res, applyErr
		}
	} else if len(res.Missing) > 0 {
		if preErr := runPreflight(ctx, cfg, repo, res, opts); preErr != nil {
			return res, preErr
		}
	}

	return res, nil
}

// computeDriftAndMissing scans the on-disk workflows dir and populates
// res.Missing, res.Expected, and res.Drift against the resolved desired set.
func computeDriftAndMissing(res *SyncResult, resolved []ResolvedWorkflow, workflowsDir string) {
	desiredNames := map[string]bool{}
	extraNames := map[string]bool{}
	for _, w := range resolved {
		desiredNames[w.Name] = true
		if w.Extra {
			extraNames[w.Name] = true
		}
	}

	onDiskSet := map[string]bool{}
	if entries, readErr := os.ReadDir(workflowsDir); readErr == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(e.Name(), ".md") {
				onDiskSet[strings.TrimSuffix(e.Name(), ".md")] = true
			}
		}
	}

	for name := range desiredNames {
		if onDiskSet[name] {
			res.Expected = append(res.Expected, name)
		} else {
			res.Missing = append(res.Missing, name)
		}
	}

	for name := range onDiskSet {
		if name == "copilot-setup-steps" || extraNames[name] || desiredNames[name] {
			continue
		}
		res.Drift = append(res.Drift, name)
	}
}

// applyDeployOrPrune runs the apply-side actions: prune drift files (if requested),
// then either delegate to Deploy (for missing workflows) or commit/push pruned
// deletions standalone.
func applyDeployOrPrune(
	ctx context.Context, cfg *Config, repo string, res *SyncResult, opts SyncOpts, workflowsDir string,
) error {
	if opts.Prune && len(res.Drift) > 0 {
		if pruneErr := pruneDriftFiles(ctx, res, workflowsDir); pruneErr != nil {
			return pruneErr
		}
	}

	if len(res.Missing) > 0 {
		deployOpts := DeployOpts{
			Apply:   true,
			Force:   opts.Force,
			WorkDir: res.CloneDir,
		}
		var deployErr error
		res.Deploy, deployErr = Deploy(ctx, cfg, repo, deployOpts)
		return deployErr
	}

	if opts.Prune && len(res.Pruned) > 0 {
		return commitAndPushPrune(ctx, res)
	}
	return nil
}

// pruneDriftFiles removes drift files from disk and stages the deletions.
func pruneDriftFiles(ctx context.Context, res *SyncResult, workflowsDir string) error {
	for _, name := range res.Drift {
		path := filepath.Join(workflowsDir, name+".md")
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("remove drift file %s: %w", name, removeErr)
		}
		res.Pruned = append(res.Pruned, name)
	}
	if stageErr := gitCmd(ctx, res.CloneDir, "add", ".github/"); stageErr != nil {
		return fmt.Errorf("git add after prune: %w", stageErr)
	}
	return nil
}

// commitAndPushPrune commits and pushes standalone prune deletions when there
// are no missing workflows to deploy.
func commitAndPushPrune(ctx context.Context, res *SyncResult) error {
	msg := fmt.Sprintf("ci(workflows): remove %d drift workflows", len(res.Pruned))
	if commitErr := gitCmd(ctx, res.CloneDir, "commit", "-m", msg); commitErr != nil {
		return fmt.Errorf("git commit prune: %w", commitErr)
	}
	if pushErr := gitCmd(ctx, res.CloneDir, "push", "-u", "origin", "HEAD"); pushErr != nil {
		return fmt.Errorf("git push prune: %w", pushErr)
	}
	return nil
}

// runPreflight calls Deploy with Apply=false to surface compilation failures
// for missing workflows during a dry-run sync.
func runPreflight(ctx context.Context, cfg *Config, repo string, res *SyncResult, opts SyncOpts) error {
	deployOpts := DeployOpts{
		Apply:   false,
		Force:   opts.Force,
		WorkDir: res.CloneDir,
	}
	var err error
	res.DeployPreflight, err = Deploy(ctx, cfg, repo, deployOpts)
	return err
}
