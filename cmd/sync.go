package cmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	zlog "github.com/rs/zerolog/log"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

func newSyncCmd(flagDir *string) *cobra.Command {
	var (
		flagApply   bool
		flagPrune   bool
		flagForce   bool
		flagWorkDir string
	)
	cmd := &cobra.Command{
		Use:   "sync <repo>",
		Short: "Reconcile a repo to match its declared profile (add missing, flag drift)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]
			if flagPrune && !flagApply {
				return errors.New("--prune requires --apply")
			}
			cfg, err := fleet.LoadConfig(*flagDir)
			if err != nil {
				return err
			}
			if _, ok := cfg.Repos[repo]; !ok {
				return fmt.Errorf("repo %q not tracked in %s", repo, fleet.ConfigFile)
			}
			opts := fleet.SyncOpts{
				Apply:   flagApply,
				Prune:   flagPrune,
				Force:   flagForce,
				WorkDir: flagWorkDir,
			}
			res, syncErr := fleet.Sync(cmd.Context(), cfg, repo, opts)
			printSync(cmd, res, flagApply, flagPrune)
			return syncErr
		},
	}
	cmd.Flags().BoolVar(&flagApply, "apply", false,
		"Add missing workflows and optionally prune drift (default is dry-run)")
	cmd.Flags().BoolVar(&flagPrune, "prune", false,
		"Delete drift workflow files (requires --apply)")
	cmd.Flags().BoolVar(&flagForce, "force", false,
		"Overwrite existing workflow files (passes --force to gh aw add)")
	cmd.Flags().StringVar(&flagWorkDir, "work-dir", "",
		"Existing clone to sync (skips git clone + auto-cleanup)")
	return cmd
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

	printSyncDrift(w, res)
	printSyncMissing(w, res)
	printSyncExpected(w, res)
	printSyncPruned(w, res)
	printSyncDeploy(w, res)
	printSyncPreflight(w, res)

	emitSyncWarnings(res)
	if len(res.Drift) > 0 && !prune {
		fmt.Fprintln(w, "\n  Run with --prune --apply to remove drift workflows (this is destructive).")
	}

	if !apply {
		fmt.Fprintln(w, "\nRe-run with --apply to add missing workflows and (with --prune) remove drift.")
	}
}

func emitSyncWarnings(res *fleet.SyncResult) {
	if res == nil || len(res.Drift) == 0 {
		return
	}
	zlog.Warn().
		Str("repo", res.Repo).
		Strs("drift", res.Drift).
		Msg("Drift detected: workflows on disk not declared in fleet.json")
}

func printSyncDrift(w io.Writer, res *fleet.SyncResult) {
	fmt.Fprintf(w, "  drift:    %d\n", len(res.Drift))
	for _, d := range res.Drift {
		fmt.Fprintf(w, "    ~ %s (unexpected, not in fleet.json or extra_workflows)\n", d)
	}
}

func printSyncMissing(w io.Writer, res *fleet.SyncResult) {
	fmt.Fprintf(w, "  missing:  %d\n", len(res.Missing))
	for _, m := range res.Missing {
		fmt.Fprintf(w, "    + %s (will be deployed)\n", m)
	}
}

func printSyncExpected(w io.Writer, res *fleet.SyncResult) {
	fmt.Fprintf(w, "  expected: %d\n", len(res.Expected))
	for _, e := range res.Expected {
		fmt.Fprintf(w, "    = %s (already present)\n", e)
	}
}

func printSyncPruned(w io.Writer, res *fleet.SyncResult) {
	if len(res.Pruned) == 0 {
		return
	}
	fmt.Fprintf(w, "  pruned:   %d\n", len(res.Pruned))
	for _, p := range res.Pruned {
		fmt.Fprintf(w, "    - %s (removed)\n", p)
	}
}

func printSyncDeploy(w io.Writer, res *fleet.SyncResult) {
	if res.Deploy == nil {
		return
	}
	fmt.Fprintf(w, "  deploy:   %d added", len(res.Deploy.Added))
	if res.Deploy.PRURL != "" {
		fmt.Fprintf(w, " → %s", res.Deploy.PRURL)
	}
	fmt.Fprintln(w)
}

func printSyncPreflight(w io.Writer, res *fleet.SyncResult) {
	if res.DeployPreflight == nil || len(res.DeployPreflight.Failed) == 0 {
		return
	}
	fmt.Fprintf(w, "  would fail: %d\n", len(res.DeployPreflight.Failed))
	errs := make([]string, 0, len(res.DeployPreflight.Failed))
	for _, f := range res.DeployPreflight.Failed {
		fmt.Fprintf(w, "    ! %s\n      %s\n", f.Name, f.Error)
		errs = append(errs, f.Error)
	}
	hints := fleet.CollectHints(errs...)
	for _, h := range hints {
		fmt.Fprintf(w, "  hint: %s\n", h)
	}
	emitHints(res.Repo, hints)
}
