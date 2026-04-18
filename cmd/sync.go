package cmd

import (
	"fmt"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
	"github.com/spf13/cobra"
)

var (
	flagSyncApply   bool
	flagSyncPrune   bool
	flagSyncForce   bool
	flagSyncWorkDir string
)

func init() {
	syncCmd.Args = cobra.ExactArgs(1)
	syncCmd.RunE = runSync
	syncCmd.Flags().BoolVar(&flagSyncApply, "apply", false, "Add missing workflows and optionally prune drift (default is dry-run)")
	syncCmd.Flags().BoolVar(&flagSyncPrune, "prune", false, "Delete drift workflow files (requires --apply)")
	syncCmd.Flags().BoolVar(&flagSyncForce, "force", false, "Overwrite existing workflow files (passes --force to gh aw add)")
	syncCmd.Flags().StringVar(&flagSyncWorkDir, "work-dir", "", "Existing clone to sync (skips git clone + auto-cleanup)")
}

func runSync(cmd *cobra.Command, args []string) error {
	repo := args[0]

	if flagSyncPrune && !flagSyncApply {
		return fmt.Errorf("--prune requires --apply")
	}

	cfg, err := fleet.LoadConfig(flagDir)
	if err != nil {
		return err
	}
	if _, ok := cfg.Repos[repo]; !ok {
		return fmt.Errorf("repo %q not tracked in %s", repo, fleet.ConfigFile)
	}

	opts := fleet.SyncOpts{
		Apply:   flagSyncApply,
		Prune:   flagSyncPrune,
		Force:   flagSyncForce,
		WorkDir: flagSyncWorkDir,
	}
	res, err := fleet.Sync(cmd.Context(), cfg, repo, opts)
	printSync(cmd, res, flagSyncApply, flagSyncPrune)
	return err
}

func printSync(cmd *cobra.Command, res *fleet.SyncResult, apply, prune bool) {
	if res == nil {
		return
	}
	w := cmd.OutOrStdout()
	mode := "DRY RUN"
	if apply {
		mode = "APPLIED"
	}
	fmt.Fprintf(w, "[%s] %s (clone: %s)\n", mode, res.Repo, res.CloneDir)

	fmt.Fprintf(w, "  drift:    %d\n", len(res.Drift))
	for _, d := range res.Drift {
		fmt.Fprintf(w, "    ~ %s (unexpected, not in fleet.json or extra_workflows)\n", d)
	}

	fmt.Fprintf(w, "  missing:  %d\n", len(res.Missing))
	for _, m := range res.Missing {
		fmt.Fprintf(w, "    + %s (will be deployed)\n", m)
	}

	fmt.Fprintf(w, "  expected: %d\n", len(res.Expected))
	for _, e := range res.Expected {
		fmt.Fprintf(w, "    = %s (already present)\n", e)
	}

	if len(res.Pruned) > 0 {
		fmt.Fprintf(w, "  pruned:   %d\n", len(res.Pruned))
		for _, p := range res.Pruned {
			fmt.Fprintf(w, "    - %s (removed)\n", p)
		}
	}

	if res.Deploy != nil {
		fmt.Fprintf(w, "  deploy:   %d added", len(res.Deploy.Added))
		if res.Deploy.PRURL != "" {
			fmt.Fprintf(w, " → %s", res.Deploy.PRURL)
		}
		fmt.Fprintln(w)
	}

	if res.DeployPreflight != nil && len(res.DeployPreflight.Failed) > 0 {
		fmt.Fprintf(w, "  would fail: %d\n", len(res.DeployPreflight.Failed))
		errs := make([]string, 0, len(res.DeployPreflight.Failed))
		for _, f := range res.DeployPreflight.Failed {
			fmt.Fprintf(w, "    ! %s\n      %s\n", f.Name, f.Error)
			errs = append(errs, f.Error)
		}
		for _, h := range fleet.CollectHints(errs...) {
			fmt.Fprintf(w, "  hint: %s\n", h)
		}
	}

	if len(res.Drift) > 0 {
		fmt.Fprintln(w, "\n⚠️  WARNING: Drift detected. Workflows on disk not declared in fleet.json.")
		if !prune {
			fmt.Fprintln(w, "  Run with --prune --apply to remove drift workflows (this is destructive).")
		}
	}

	if !apply {
		fmt.Fprintln(w, "\nRe-run with --apply to add missing workflows and (with --prune) remove drift.")
	}
}
