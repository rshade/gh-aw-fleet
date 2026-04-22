package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	zlog "github.com/rs/zerolog/log"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

func newDeployCmd(flagDir *string) *cobra.Command {
	var (
		flagApply   bool
		flagForce   bool
		flagBranch  string
		flagPRTitle string
		flagWorkDir string
	)
	cmd := &cobra.Command{
		Use:   "deploy <repo>",
		Short: "Apply the declared workflow set to a repo via gh aw add + PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]
			cfg, err := fleet.LoadConfig(*flagDir)
			if err != nil {
				return err
			}
			if _, ok := cfg.Repos[repo]; !ok {
				return fmt.Errorf("repo %q not tracked in %s", repo, fleet.ConfigFile)
			}
			opts := fleet.DeployOpts{
				Apply:   flagApply,
				Force:   flagForce,
				Branch:  flagBranch,
				PRTitle: flagPRTitle,
				WorkDir: flagWorkDir,
			}
			res, deployErr := fleet.Deploy(cmd.Context(), cfg, repo, opts)
			printDeploy(cmd, res, flagApply)
			return deployErr
		},
	}
	cmd.Flags().BoolVar(&flagApply, "apply", false,
		"Actually commit + push + open PR (default is dry-run)")
	cmd.Flags().BoolVar(&flagForce, "force", false,
		"Overwrite existing workflow files (passes --force to gh aw add)")
	cmd.Flags().StringVar(&flagBranch, "branch", "",
		"Branch name for the deploy PR (default: fleet/deploy-<timestamp>)")
	cmd.Flags().StringVar(&flagPRTitle, "pr-title", "",
		"PR title (default: auto-generated)")
	cmd.Flags().StringVar(&flagWorkDir, "work-dir", "",
		"Existing clone to deploy into (skips git clone + auto-cleanup)")
	return cmd
}

func printDeploy(cmd *cobra.Command, res *fleet.DeployResult, apply bool) {
	if res == nil {
		return
	}
	w := cmd.OutOrStdout()
	mode := "DRY RUN"
	if apply {
		mode = "APPLIED"
	}
	fmt.Fprintf(w, "[%s] %s (clone: %s)\n", mode, res.Repo, res.CloneDir)
	if res.InitWasRun {
		fmt.Fprintln(w, "  gh aw init: ran (repo was not yet initialized)")
	}
	fmt.Fprintf(w, "  added:   %d\n", len(res.Added))
	for _, a := range res.Added {
		fmt.Fprintf(w, "    + %s  (%s)\n", a.Name, a.Spec)
	}
	if len(res.Skipped) > 0 {
		fmt.Fprintf(w, "  skipped: %d (already present)\n", len(res.Skipped))
		for _, s := range res.Skipped {
			fmt.Fprintf(w, "    = %s\n", s.Name)
		}
	}
	if len(res.Failed) > 0 {
		fmt.Fprintf(w, "  failed:  %d\n", len(res.Failed))
		errs := make([]string, 0, len(res.Failed))
		for _, f := range res.Failed {
			fmt.Fprintf(w, "    ! %s\n      %s\n", f.Name, f.Error)
			errs = append(errs, f.Error)
		}
		hints := fleet.CollectHints(errs...)
		for _, h := range hints {
			fmt.Fprintf(w, "  hint: %s\n", h)
		}
		emitHints(res.Repo, hints)
	}
	if res.BranchPushed != "" {
		fmt.Fprintf(w, "  pushed:  %s\n", res.BranchPushed)
	}
	if res.PRURL != "" {
		fmt.Fprintf(w, "  PR:      %s\n", res.PRURL)
	}
	emitDeployWarnings(res)
	if !apply {
		fmt.Fprintln(w, "\nRe-run with --apply to commit, push, and open the PR.")
	}
}

// emitDeployWarnings: the secret-key URL goes in the message text, not a
// structured field — URL fields would defeat log-greppable secret-hygiene.
func emitDeployWarnings(res *fleet.DeployResult) {
	if res == nil || res.MissingSecret == "" {
		return
	}
	msg := fmt.Sprintf(
		"Actions secret %q is not set on %s; workflows will fail until added (gh secret set %s --repo %s)",
		res.MissingSecret, res.Repo, res.MissingSecret, res.Repo,
	)
	if res.SecretKeyURL != "" {
		msg = fmt.Sprintf("%s — obtain the key at %s", msg, res.SecretKeyURL)
	}
	zlog.Warn().
		Str("repo", res.Repo).
		Str("secret", res.MissingSecret).
		Msg(msg)
}
