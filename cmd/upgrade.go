package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

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
			jsonMode := outputMode(cmd) == outputJSON
			if err := validateUpgradeArgs(args, flagAll, flagWorkDir); err != nil {
				if jsonMode {
					repo := ""
					if len(args) > 0 {
						repo = args[0]
					}
					return preResultFailureEnvelope(cmd, "upgrade", repo, flagApply, err)
				}
				return err
			}
			cfg, err := fleet.LoadConfig(*flagDir)
			if err != nil {
				if jsonMode {
					repo := ""
					if len(args) > 0 {
						repo = args[0]
					}
					return preResultFailureEnvelope(cmd, "upgrade", repo, flagApply, err)
				}
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
				return runUpgradeAll(cmd, cfg, opts, flagApply, flagAudit, jsonMode)
			}
			return runUpgradeSingle(cmd, cfg, args[0], opts, flagApply, jsonMode)
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

// validateUpgradeArgs enforces the --all/positional arg contract. Extracted
// to keep the cobra RunE under the gocognit threshold.
func validateUpgradeArgs(args []string, all bool, workDir string) error {
	if len(args) > 1 {
		return errors.New("upgrade: at most one positional argument (repo name)")
	}
	if all && len(args) > 0 {
		return errors.New("upgrade: cannot specify both --all and a repo name")
	}
	if !all && len(args) == 0 {
		return errors.New("upgrade: specify either a repo name or --all")
	}
	if workDir != "" && all {
		return errors.New("upgrade: --work-dir cannot be used with --all")
	}
	return nil
}

// runUpgradeAll routes the --all path between text-mode (UpgradeAll batch +
// summary print) and JSON-mode (per-repo NDJSON via runUpgradeAllJSON).
func runUpgradeAll(
	cmd *cobra.Command, cfg *fleet.Config, opts fleet.UpgradeOpts, apply, audit, jsonMode bool,
) error {
	if jsonMode {
		return runUpgradeAllJSON(cmd, cfg, opts, apply)
	}
	results, allErr := fleet.UpgradeAll(cmd.Context(), cfg, opts)
	printUpgradeAll(cmd, results, audit)
	return allErr
}

// runUpgradeSingle routes the single-repo path. Validates the repo is tracked
// before invoking Upgrade; both modes get a structured rejection on miss.
func runUpgradeSingle(
	cmd *cobra.Command, cfg *fleet.Config, repo string, opts fleet.UpgradeOpts, apply, jsonMode bool,
) error {
	if _, ok := cfg.Repos[repo]; !ok {
		notTrackedErr := fmt.Errorf("repo %q not tracked in %s", repo, cfg.LoadedFrom)
		if jsonMode {
			return preResultFailureEnvelope(cmd, "upgrade", repo, apply, notTrackedErr)
		}
		return notTrackedErr
	}
	res, upErr := fleet.Upgrade(cmd.Context(), cfg, repo, opts)
	if jsonMode {
		return emitUpgradeEnvelope(cmd, repo, apply, res, upErr)
	}
	printUpgrade(cmd, res)
	return upErr
}

// emitUpgradeEnvelope writes a single-repo upgrade envelope. Hints come from
// res.OutputLog (the captured combined stdout+stderr of gh aw upgrade/update),
// matching the text-mode hint source at cmd/upgrade.go:110.
func emitUpgradeEnvelope(cmd *cobra.Command, repo string, apply bool, res *fleet.UpgradeResult, upErr error) error {
	var hints []fleet.Diagnostic
	if res != nil && res.OutputLog != "" {
		emitHints(res.Repo, fleet.CollectHints(res.OutputLog))
		hints = fleet.CollectHintDiagnostics(res.OutputLog)
	}
	hints = ensureFailureHint(hints, upErr)
	if writeErr := writeEnvelope(cmd, "upgrade", repo, apply, res, nil, hints); writeErr != nil {
		return writeErr
	}
	return upErr
}

// runUpgradeAllJSON emits one envelope per repo as each upgrade completes.
// Diverges from text-mode `UpgradeAll` (which short-circuits on first error)
// because the NDJSON contract requires every repo to surface.
//
// Future parallelization MUST serialize the writeEnvelope calls behind a
// mutex to prevent interleaved partial lines (contracts/ndjson.md).
func runUpgradeAllJSON(cmd *cobra.Command, cfg *fleet.Config, opts fleet.UpgradeOpts, apply bool) error {
	var firstErr error
	repos := make([]string, 0, len(cfg.Repos))
	for r := range cfg.Repos {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	for _, repo := range repos {
		res, upErr := fleet.Upgrade(cmd.Context(), cfg, repo, opts)
		if writeErr := emitUpgradeEnvelope(cmd, repo, apply, res, upErr); writeErr != nil && firstErr == nil {
			firstErr = writeErr
		}
		if upErr != nil && firstErr == nil {
			firstErr = upErr
		}
	}
	return firstErr
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
