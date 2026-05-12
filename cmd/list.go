package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

const tabPadding = 2

func newListCmd(flagDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   commandList,
		Short: "List tracked repos and their resolved workflow sets",
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonMode := outputMode(cmd) == outputJSON
			cfg, err := fleet.LoadConfig(*flagDir)
			if err != nil {
				if jsonMode {
					return preResultFailureEnvelope(cmd, commandList, "", false, err)
				}
				return err
			}
			// Breadcrumb stays on stderr in both modes — text consumers expect it,
			// JSON consumers redirect stderr.
			fmt.Fprintf(cmd.ErrOrStderr(), "  (loaded %s)\n", cfg.LoadedFrom)

			if jsonMode {
				res, buildErr := fleet.BuildListResult(cfg)
				if buildErr != nil {
					return preResultFailureEnvelope(cmd, commandList, "", false, buildErr)
				}
				return writeEnvelope(cmd, commandList, "", false, res, nil, nil)
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, tabPadding, ' ', 0)
			fmt.Fprintln(tw, "REPO\tPROFILES\tTIERS\tENGINE\tWORKFLOWS\tEXCLUDED\tEXTRA\tCOST_CENTER")
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
				fmt.Fprintf(tw, "%s\t%v\t%s\t%s\t%d\t%v\t%d\t%s\n",
					r,
					spec.Profiles,
					tiersCellForRow(spec.Profiles, cfg.Profiles),
					orDash(cfg.EffectiveEngine(r)),
					len(resolved),
					orEmpty(spec.ExcludeFromProfiles),
					len(spec.ExtraWorkflows),
					orDash(spec.CostCenter))
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

// tiersForRow derives the text-mode tiers values from fleet.ProfileTiersMap.
// Each position holds either the profile's Tier or "-" as a placeholder;
// when every slot would be "-", returns an empty slice so the cell renders as
// [] — matching the slice-empty convention used for Excluded and Extra, and
// avoiding the visually noisy "[- - -]" rendering for fleets that have not
// opted into tier annotations yet.
func tiersForRow(profiles []string, profileDefs map[string]fleet.Profile) []string {
	tiered := fleet.ProfileTiersMap(profiles, profileDefs)
	if len(tiered) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(profiles))
	for _, name := range profiles {
		if t, ok := tiered[name]; ok {
			out = append(out, t)
		} else {
			out = append(out, "-")
		}
	}
	return out
}

func tiersCellForRow(profiles []string, profileDefs map[string]fleet.Profile) string {
	tiers := tiersForRow(profiles, profileDefs)
	if len(tiers) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(tiers))
	for _, tier := range tiers {
		quoted = append(quoted, strconv.Quote(tier))
	}
	return "[" + strings.Join(quoted, " ") + "]"
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
