package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
	"github.com/spf13/cobra"
)

var (
	flagApplyUpgrade   bool
	flagAudit          bool
	flagMajor          bool
	flagForceUpgrade   bool
	flagWorkDirUpgrade string
	flagAll            bool
)

func init() {
	upgradeCmd.RunE = runUpgrade
	upgradeCmd.Flags().BoolVar(&flagApplyUpgrade, "apply", false, "Actually commit + push + open PR (default is dry-run)")
	upgradeCmd.Flags().BoolVar(&flagAudit, "audit", false, "Only run gh aw upgrade --audit, skip upgrade")
	upgradeCmd.Flags().BoolVar(&flagMajor, "major", false, "Allow major-version bumps for tag pins (passes --major to gh aw update)")
	upgradeCmd.Flags().BoolVar(&flagForceUpgrade, "force", false, "Update even if no changes detected (passes --force to gh aw update)")
	upgradeCmd.Flags().StringVar(&flagWorkDirUpgrade, "work-dir", "", "Existing clone to upgrade in (skips git clone + auto-cleanup)")
	upgradeCmd.Flags().BoolVar(&flagAll, "all", false, "Upgrade all repos in fleet.json")
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("upgrade: at most one positional argument (repo name)")
	}

	if flagAll && len(args) > 0 {
		return fmt.Errorf("upgrade: cannot specify both --all and a repo name")
	}
	if !flagAll && len(args) == 0 {
		return fmt.Errorf("upgrade: specify either a repo name or --all")
	}
	if flagWorkDirUpgrade != "" && flagAll {
		return fmt.Errorf("upgrade: --work-dir cannot be used with --all")
	}

	cfg, err := fleet.LoadConfig(flagDir)
	if err != nil {
		return err
	}

	opts := fleet.UpgradeOpts{
		Apply:   flagApplyUpgrade,
		Audit:   flagAudit,
		Major:   flagMajor,
		Force:   flagForceUpgrade,
		WorkDir: flagWorkDirUpgrade,
	}

	if flagAll {
		results, err := fleet.UpgradeAll(cmd.Context(), cfg, opts)
		printUpgradeAll(cmd, results, flagAudit)
		return err
	}

	repo := args[0]
	if _, ok := cfg.Repos[repo]; !ok {
		return fmt.Errorf("repo %q not tracked in %s", repo, fleet.ConfigFile)
	}

	res, err := fleet.Upgrade(cmd.Context(), cfg, repo, opts)
	printUpgrade(cmd, res)
	return err
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

	for _, h := range fleet.CollectHints(res.OutputLog) {
		fmt.Fprintf(w, "  hint: %s\n", h)
	}

	if res.BranchPushed != "" {
		fmt.Fprintf(w, "  pushed:  %s\n", res.BranchPushed)
	}
	if res.PRURL != "" {
		fmt.Fprintf(w, "  PR:      %s\n", res.PRURL)
	}
}

func printUpgradeAll(cmd *cobra.Command, results []*fleet.UpgradeResult, audit bool) {
	if audit {
		w := cmd.OutOrStdout()
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
			status := "OK"
			if issues, ok := auditData["issues"]; ok {
				if count, ok := issues.(float64); ok && count > 0 {
					status = fmt.Sprintf("%d issues", int(count))
				}
			}
			fmt.Fprintf(w, "  %s: %s\n", res.Repo, status)
		}
		return
	}

	w := cmd.OutOrStdout()
	fmt.Fprintln(w, "Upgrade results:")
	for _, res := range results {
		if res == nil {
			continue
		}
		status := "OK"
		if len(res.Conflicts) > 0 {
			status = fmt.Sprintf("CONFLICTS (%d)", len(res.Conflicts))
		} else if res.NoChanges {
			status = "no changes"
		} else if res.BranchPushed != "" {
			status = fmt.Sprintf("PR (%s)", res.PRURL)
		}
		fmt.Fprintf(w, "  %s: %s\n", res.Repo, status)
	}
}
