package fleet

import (
	"context"
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
	Repo            string        // repo owner/name
	CloneDir        string        // where the clone lives
	Missing         []string      // workflow names not yet deployed
	Drift           []string      // workflow files present but not declared
	Expected        []string      // workflow names matching fleet.json (informational)
	Deploy          *DeployResult // set iff Apply==true and Missing/Prune required action
	Pruned          []string      // files removed when --prune --apply
	DeployPreflight *DeployResult // set iff Apply==false to surface compilation failures
}

// Sync reconciles one repo's .github/workflows/ to match its declared profile(s).
// Detects missing, drift, and expected workflows, then optionally deploys or prunes.
func Sync(ctx context.Context, cfg *Config, repo string, opts SyncOpts) (*SyncResult, error) {
	resolved, err := cfg.ResolveRepoWorkflows(repo)
	if err != nil {
		return nil, err
	}

	res := &SyncResult{Repo: repo}

	// Clone the repo.
	res.CloneDir, err = prepareClone(ctx, repo, opts.WorkDir)
	if err != nil {
		return res, err
	}
	if opts.WorkDir == "" && !opts.Apply {
		defer os.RemoveAll(res.CloneDir)
	}

	// Ensure gh aw init has run.
	_, err = ensureInit(ctx, res.CloneDir)
	if err != nil {
		return res, fmt.Errorf("gh aw init: %w", err)
	}

	// Build set of desired workflows (non-local ones).
	desiredNames := map[string]bool{}
	extraNames := map[string]bool{}
	for _, w := range resolved {
		desiredNames[w.Name] = true
		if w.Extra {
			extraNames[w.Name] = true
		}
	}

	// Scan .github/workflows/ for actual .md files.
	workflowsDir := filepath.Join(res.CloneDir, ".github", "workflows")
	var onDisk []string
	if entries, err := os.ReadDir(workflowsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(e.Name(), ".md") {
				name := strings.TrimSuffix(e.Name(), ".md")
				onDisk = append(onDisk, name)
			}
		}
	}

	// Compute missing and drift.
	onDiskSet := make(map[string]bool)
	for _, name := range onDisk {
		onDiskSet[name] = true
	}

	for name := range desiredNames {
		if !onDiskSet[name] {
			res.Missing = append(res.Missing, name)
		} else {
			res.Expected = append(res.Expected, name)
		}
	}

	for name := range onDiskSet {
		// Skip copilot-setup-steps (reserved).
		if name == "copilot-setup-steps" {
			continue
		}
		// Skip if it's in extra workflows (operator can manage it manually).
		if extraNames[name] {
			continue
		}
		// If not in desired, it's drift.
		if !desiredNames[name] {
			res.Drift = append(res.Drift, name)
		}
	}

	// If prune flag set without apply, error early.
	if opts.Prune && !opts.Apply {
		return res, fmt.Errorf("--prune requires --apply")
	}

	// If apply is set, call Deploy to add missing workflows.
	// If prune is also set, delete drift files before Deploy.
	if opts.Apply {
		if opts.Prune && len(res.Drift) > 0 {
			for _, name := range res.Drift {
				path := filepath.Join(workflowsDir, name+".md")
				if err := os.Remove(path); err != nil {
					return res, fmt.Errorf("remove drift file %s: %w", name, err)
				}
				res.Pruned = append(res.Pruned, name)
			}
			// Stage the deletions.
			if err := gitCmd(ctx, res.CloneDir, "add", ".github/"); err != nil {
				return res, fmt.Errorf("git add after prune: %w", err)
			}
		}

		if len(res.Missing) > 0 {
			// Call Deploy with the clone directory to add missing workflows.
			// Deploy will handle branching, commits, and PR.
			deployOpts := DeployOpts{
				Apply:   true,
				Force:   opts.Force,
				WorkDir: res.CloneDir,
			}
			res.Deploy, err = Deploy(ctx, cfg, repo, deployOpts)
			if err != nil {
				return res, err
			}
		} else if opts.Prune && len(res.Pruned) > 0 {
			// If we pruned files but have no missing workflows, we still need to
			// commit and push the deletions.
			if err := gitCmd(ctx, res.CloneDir, "commit", "-m", fmt.Sprintf("ci(workflows): remove %d drift workflows", len(res.Pruned))); err != nil {
				return res, fmt.Errorf("git commit prune: %w", err)
			}
			if err := gitCmd(ctx, res.CloneDir, "push", "-u", "origin", "HEAD"); err != nil {
				return res, fmt.Errorf("git push prune: %w", err)
			}
		}
	} else if len(res.Missing) > 0 {
		// Dry-run pre-flight: call Deploy with Apply=false to check compilation.
		deployOpts := DeployOpts{
			Apply:   false,
			Force:   opts.Force,
			WorkDir: res.CloneDir,
		}
		res.DeployPreflight, err = Deploy(ctx, cfg, repo, deployOpts)
		if err != nil {
			return res, err
		}
	}

	return res, nil
}
