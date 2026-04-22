package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

func newUpgradeCmd(flagDir *string) *cobra.Command {
	var (
		flagApply   bool
		flagAudit   bool
		flagMajor   bool
		flagForce   bool
		flagWorkDir string
		flagAll     bool
	)
	cmd := &cobra.Command{
		Use:   "upgrade [repo|--all]",
		Short: "Bump profile pin + run gh aw upgrade + update across repos",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return errors.New("upgrade: at most one positional argument (repo name)")
			}
			if flagAll && len(args) > 0 {
				return errors.New("upgrade: cannot specify both --all and a repo name")
			}
			if !flagAll && len(args) == 0 {
				return errors.New("upgrade: specify either a repo name or --all")
			}
			if flagWorkDir != "" && flagAll {
				return errors.New("upgrade: --work-dir cannot be used with --all")
			}

			cfg, err := fleet.LoadConfig(*flagDir)
			if err != nil {
				return err
			}

			opts := fleet.UpgradeOpts{
				Apply:   flagApply,
				Audit:   flagAudit,
				Major:   flagMajor,
				Force:   flagForce,
				WorkDir: flagWorkDir,
			}

			if flagAll {
				results, allErr := fleet.UpgradeAll(cmd.Context(), cfg, opts)
				printUpgradeAll(cmd, results, flagAudit)
				return allErr
			}

			repo := args[0]
			if _, ok := cfg.Repos[repo]; !ok {
				return fmt.Errorf("repo %q not tracked in %s", repo, fleet.ConfigFile)
			}

			res, upErr := fleet.Upgrade(cmd.Context(), cfg, repo, opts)
			printUpgrade(cmd, res)
			return upErr
		},
	}
	cmd.Flags().BoolVar(&flagApply, "apply", false,
		"Actually commit + push + open PR (default is dry-run)")
	cmd.Flags().BoolVar(&flagAudit, "audit", false,
		"Only run gh aw upgrade --audit, skip upgrade")
	cmd.Flags().BoolVar(&flagMajor, "major", false,
		"Allow major-version bumps for tag pins (passes --major to gh aw update)")
	cmd.Flags().BoolVar(&flagForce, "force", false,
		"Update even if no changes detected (passes --force to gh aw update)")
	cmd.Flags().StringVar(&flagWorkDir, "work-dir", "",
		"Existing clone to upgrade in (skips git clone + auto-cleanup)")
	cmd.Flags().BoolVar(&flagAll, "all", false,
		"Upgrade all repos in fleet.json")
	return cmd
}

func printUpgrade(cmd *cobra.Command, res *fleet.UpgradeResult) {
	if res == nil {
		return
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s (clone: %s)\n", res.Repo, res.CloneDir)

	if len(res.Conflicts) > 0 {
		fmt.Fprintf(w, "  CONFLICTS: %d file(s) need manual merge\n", len(res.Conflicts))
		for _, c := range res.Conflicts {
			fmt.Fprintf(w, "    ! %s\n", c)
		}
		fmt.Fprintln(w, "  Clone dir preserved for manual review.")
		return
	}

	if res.NoChanges {
		fmt.Fprintln(w, "  no changes needed")
		return
	}

	fmt.Fprintf(w, "  changed: %d file(s)\n", len(res.ChangedFiles))
	for _, f := range res.ChangedFiles {
		fmt.Fprintf(w, "    ~ %s\n", f)
	}

	hints := fleet.CollectHints(res.OutputLog)
	for _, h := range hints {
		fmt.Fprintf(w, "  hint: %s\n", h)
	}
	emitHints(res.Repo, hints)

	if res.BranchPushed != "" {
		fmt.Fprintf(w, "  pushed:  %s\n", res.BranchPushed)
	}
	if res.PRURL != "" {
		fmt.Fprintf(w, "  PR:      %s\n", res.PRURL)
	}
}

func printUpgradeAll(cmd *cobra.Command, results []*fleet.UpgradeResult, audit bool) {
	w := cmd.OutOrStdout()
	if audit {
		printUpgradeAudit(w, results)
		return
	}
	printUpgradeSummary(w, results)
}

func printUpgradeAudit(w io.Writer, results []*fleet.UpgradeResult) {
	fmt.Fprintln(w, "Audit results:")
	for _, res := range results {
		if res == nil || res.AuditJSON == nil {
			continue
		}
		var auditData map[string]any
		if err := json.Unmarshal(res.AuditJSON, &auditData); err != nil {
			fmt.Fprintf(w, "  %s: error parsing audit JSON\n", res.Repo)
			continue
		}
		fmt.Fprintf(w, "  %s: %s\n", res.Repo, auditStatus(auditData))
	}
}

func auditStatus(auditData map[string]any) string {
	issues, ok := auditData["issues"]
	if !ok {
		return "OK"
	}
	count, countOK := issues.(float64)
	if !countOK || count <= 0 {
		return "OK"
	}
	return fmt.Sprintf("%d issues", int(count))
}

func printUpgradeSummary(w io.Writer, results []*fleet.UpgradeResult) {
	fmt.Fprintln(w, "Upgrade results:")
	for _, res := range results {
		if res == nil {
			continue
		}
		fmt.Fprintf(w, "  %s: %s\n", res.Repo, upgradeStatus(res))
	}
}

func upgradeStatus(res *fleet.UpgradeResult) string {
	switch {
	case len(res.Conflicts) > 0:
		return fmt.Sprintf("CONFLICTS (%d)", len(res.Conflicts))
	case res.NoChanges:
		return "no changes"
	case res.BranchPushed != "":
		return fmt.Sprintf("PR (%s)", res.PRURL)
	default:
		return "OK"
	}
}
