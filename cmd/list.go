package cmd

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

const tabPadding = 2

func newListCmd(flagDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tracked repos and their resolved workflow sets",
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonMode := outputMode(cmd) == outputJSON
			cfg, err := fleet.LoadConfig(*flagDir)
			if err != nil {
				if jsonMode {
					return preResultFailureEnvelope(cmd, "list", "", false, err)
				}
				return err
			}
			// Breadcrumb stays on stderr in both modes — text consumers expect it,
			// JSON consumers redirect stderr.
			fmt.Fprintf(cmd.ErrOrStderr(), "  (loaded %s)\n", cfg.LoadedFrom)

			if jsonMode {
				res, buildErr := fleet.BuildListResult(cfg)
				if buildErr != nil {
					return preResultFailureEnvelope(cmd, "list", "", false, buildErr)
				}
				return writeEnvelope(cmd, "list", "", false, res, nil, nil)
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, tabPadding, ' ', 0)
			fmt.Fprintln(tw, "REPO\tPROFILES\tENGINE\tWORKFLOWS\tEXCLUDED\tEXTRA")
			repos := make([]string, 0, len(cfg.Repos))
			for r := range cfg.Repos {
				repos = append(repos, r)
			}
			sort.Strings(repos)
			for _, r := range repos {
				spec := cfg.Repos[r]
				resolved, resolveErr := cfg.ResolveRepoWorkflows(r)
				if resolveErr != nil {
					return resolveErr
				}
				fmt.Fprintf(tw, "%s\t%v\t%s\t%d\t%v\t%d\n",
					r, spec.Profiles, orDash(cfg.EffectiveEngine(r)), len(resolved),
					orEmpty(spec.ExcludeFromProfiles), len(spec.ExtraWorkflows))
			}
			return tw.Flush()
		},
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
