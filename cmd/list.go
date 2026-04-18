package cmd

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tracked repos and their resolved workflow sets",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := fleet.LoadConfig(flagDir)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStderr(), "  (loaded %s)\n", cfg.LoadedFrom)
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "REPO\tPROFILES\tENGINE\tWORKFLOWS\tEXCLUDED\tEXTRA")
		repos := make([]string, 0, len(cfg.Repos))
		for r := range cfg.Repos {
			repos = append(repos, r)
		}
		sort.Strings(repos)
		for _, r := range repos {
			spec := cfg.Repos[r]
			resolved, err := cfg.ResolveRepoWorkflows(r)
			if err != nil {
				return err
			}
			fmt.Fprintf(tw, "%s\t%v\t%s\t%d\t%v\t%d\n",
				r, spec.Profiles, orDash(cfg.EffectiveEngine(r)), len(resolved),
				orEmpty(spec.ExcludeFromProfiles), len(spec.ExtraWorkflows))
		}
		return tw.Flush()
	},
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
