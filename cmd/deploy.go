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
			jsonMode := outputMode(cmd) == outputJSON
			cfg, err := fleet.LoadConfig(*flagDir)
			if err != nil {
				if jsonMode {
					return preResultFailureEnvelope(cmd, "deploy", repo, flagApply, err)
				}
				return err
			}
			if _, ok := cfg.Repos[repo]; !ok {
				notTrackedErr := fleet.ErrRepoNotTracked(repo, cfg.LoadedFrom)
				if jsonMode {
					return preResultFailureEnvelope(cmd, "deploy", repo, flagApply, notTrackedErr)
				}
				return notTrackedErr
			}
			opts := fleet.DeployOpts{
				Apply:   flagApply,
				Force:   flagForce,
				Branch:  flagBranch,
				PRTitle: flagPRTitle,
				WorkDir: flagWorkDir,
			}
			res, deployErr := fleet.Deploy(cmd.Context(), cfg, repo, opts)
			if jsonMode {
				return emitDeployEnvelope(cmd, repo, flagApply, res, deployErr)
			}
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
	msg := buildMissingSecretMessage(res)
	zlog.Warn().
		Str("repo", res.Repo).
		Str("secret", res.MissingSecret).
		Msg(msg)
}

// buildMissingSecretMessage produces the same human-readable text for both
// the stderr zerolog warning and the JSON envelope's warnings[] entry.
// Factored out so the two emission paths can never drift apart.
func buildMissingSecretMessage(res *fleet.DeployResult) string {
	msg := fmt.Sprintf(
		"Actions secret %q is not set on %s; workflows will fail until added (gh secret set %s --repo %s)",
		res.MissingSecret, res.Repo, res.MissingSecret, res.Repo,
	)
	if res.SecretKeyURL != "" {
		msg = fmt.Sprintf("%s — obtain the key at %s", msg, res.SecretKeyURL)
	}
	return msg
}

// emitDeployEnvelope writes the JSON envelope for a deploy invocation,
// dual-emitting warnings/hints to stderr (zerolog) per FR-011/FR-012 and
// embedding the structured equivalents in the envelope per FR-006/FR-009.
func emitDeployEnvelope(cmd *cobra.Command, repo string, apply bool, res *fleet.DeployResult, deployErr error) error {
	var warnings []fleet.Diagnostic
	var hints []fleet.Diagnostic

	if res != nil && res.MissingSecret != "" {
		emitDeployWarnings(res)
		warnings = append(warnings, fleet.Diagnostic{
			Code:    fleet.DiagMissingSecret,
			Message: buildMissingSecretMessage(res),
			Fields: map[string]any{
				"secret": res.MissingSecret,
				"url":    res.SecretKeyURL,
			},
		})
	}
	if res != nil && len(res.Failed) > 0 {
		errs := make([]string, 0, len(res.Failed))
		for _, f := range res.Failed {
			errs = append(errs, f.Error)
		}
		emitHints(res.Repo, fleet.CollectHints(errs...))
		hints = fleet.CollectHintDiagnostics(errs...)
	}
	hints = ensureFailureHint(hints, deployErr)

	if writeErr := writeEnvelope(cmd, "deploy", repo, apply, res, warnings, hints); writeErr != nil {
		return writeErr
	}
	return deployErr
}
