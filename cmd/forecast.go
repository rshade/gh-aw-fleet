package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

// forecastFlags holds the closure-captured flag values for the forecast subcommand.
type forecastFlags struct {
	period string
	by     string
}

// newForecastCmd builds the cobra command for `gh-aw-fleet forecast`.
// It fans out `gh aw forecast --json` across the fleet and aggregates
// per-workflow projections into a table or JSON envelope.
func newForecastCmd(flagDir *string) *cobra.Command {
	var flags forecastFlags

	cmd := &cobra.Command{
		Use:   commandForecast + " [repo...]",
		Short: "Project pre-spend AI-credit cost across the fleet",
		Long: `Project pre-spend AI-credit cost across the fleet.

Pass one or more repos (owner/name) to scope the forecast to just those repos;
with no args it covers the whole fleet.

Projection horizon:
  --period week|month    (default: week)

Grouping axis:
  --by repo|profile|cost-center|tier   (default: repo)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runForecast(cmd, flagDir, &flags, args)
		},
	}
	cmd.Flags().StringVar(&flags.period, "period", "week", "Projection horizon: week|month")
	cmd.Flags().StringVar(&flags.by, "by", "repo", "Group-by axis: repo|profile|cost-center|tier")
	return cmd
}

// runForecast is the entry point for the forecast subcommand.
func runForecast(cmd *cobra.Command, flagDir *string, flags *forecastFlags, repos []string) error {
	jsonMode := outputMode(cmd) == outputJSON

	period, err := fleet.ParsePeriod(flags.period)
	if err != nil {
		if jsonMode {
			return preResultFailureEnvelope(cmd, commandForecast, "", false, err)
		}
		return err
	}

	by, err := fleet.ParseForecastGroupBy(flags.by)
	if err != nil {
		if jsonMode {
			return preResultFailureEnvelope(cmd, commandForecast, "", false, err)
		}
		return err
	}

	cfg, err := fleet.LoadConfig(*flagDir)
	if err != nil {
		if jsonMode {
			return preResultFailureEnvelope(cmd, commandForecast, "", false, err)
		}
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "  (loaded %s)\n", cfg.LoadedFrom)

	cfg, err = fleet.ScopeToRepos(cfg, repos)
	if err != nil {
		if jsonMode {
			return preResultFailureEnvelope(cmd, commandForecast, "", false, err)
		}
		return err
	}

	res, warnings, err := fleet.AggregateForecast(cmd.Context(), cfg, period, by)
	if err != nil {
		if jsonMode {
			hints := ensureFailureHint(nil, err)
			if writeErr := writeEnvelope(cmd, commandForecast, "", false, res, warnings, hints); writeErr != nil {
				return writeErr
			}
			return err
		}
		emitConsumptionWarnings(warnings)
		return err
	}

	if jsonMode {
		return writeEnvelope(cmd, commandForecast, "", false, res, warnings, nil)
	}

	emitConsumptionWarnings(warnings)
	return renderForecastText(cmd, by, res)
}

// renderForecastText renders the forecast groups table to stdout.
func renderForecastText(cmd *cobra.Command, by fleet.ForecastGroupBy, res *fleet.ForecastResult) error {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, tabPadding, ' ', 0)
	header := fmt.Sprintf("%s\tPROJECTED_AIC\tPROJECTED_COST\tP10\tP50\tP90\tSAMPLED\tWORKFLOWS\n",
		forecastByColumnHeader(by))
	fmt.Fprint(tw, header)

	for _, g := range res.Groups {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
			g.Key,
			fmt.Sprintf("%.2f", g.ProjectedAIC),
			formatCost(g.ProjectedCostUSD),
			formatAIC(g.AICP10),
			formatAIC(g.AICP50),
			formatAIC(g.AICP90),
			g.SampledRuns,
			g.WorkflowCount,
		)
	}

	return tw.Flush()
}

// forecastByColumnHeader maps the --by axis to its uppercase key-column header.
func forecastByColumnHeader(by fleet.ForecastGroupBy) string {
	const repoColumn = "REPO"
	switch by {
	case fleet.ForecastByProfile:
		return "PROFILE"
	case fleet.ForecastByCostCenter:
		return "COST_CENTER"
	case fleet.ForecastByTier:
		return "TIER"
	case fleet.ForecastByRepo:
		return repoColumn
	}
	return repoColumn
}
