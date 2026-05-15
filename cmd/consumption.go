package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

// consumptionFlags holds the closure-captured flag values for the
// consumption subcommand. Mutually-exclusive temporal flags are gated by
// cobra's MarkFlagsMutuallyExclusive; --by validation runs inside RunE.
type consumptionFlags struct {
	latest   bool
	trailing string
	since    string
	by       string
}

// newConsumptionCmd builds the cobra command for `gh-aw-fleet consumption`.
// It aggregates per-repo api-consumption-report output across the fleet via
// the layers in internal/fleet/consumption.go.
func newConsumptionCmd(flagDir *string) *cobra.Command {
	var flags consumptionFlags

	cmd := &cobra.Command{
		Use:   commandConsumption,
		Short: "Aggregate api-consumption-report output across the fleet",
		Long: `Aggregate api-consumption-report output across the fleet.

Discovery: queries each repo's audits-category discussions and filters by
the api-consumption-report-daily tracker marker. Data: fetches the
referenced workflow run's aw_info.json and run_summary.json artifacts.

Temporal modes (mutually exclusive):
  --latest                        most-recent valid report per repo (default)
  --trailing Nd                   all reports in the trailing N-day window
  --since YYYY-MM-DD              all reports on or after the given date

Grouping axis:
  --by repo|profile|cost-center|workflow   (default: repo)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConsumption(cmd, flagDir, &flags)
		},
	}
	cmd.Flags().BoolVar(&flags.latest, "latest", true, "Most-recent valid report per repo (default)")
	cmd.Flags().StringVar(&flags.trailing, "trailing", "", "Trailing window like 7d")
	cmd.Flags().StringVar(&flags.since, "since", "", "Since date in YYYY-MM-DD")
	cmd.Flags().StringVar(&flags.by, "by", "repo", "Group-by axis: repo|profile|cost-center|workflow")
	cmd.MarkFlagsMutuallyExclusive("latest", "trailing", "since")
	return cmd
}

// runConsumption loads fleet config, builds the FetchMode from flags,
// invokes the aggregator, and writes either tabwriter text or the JSON
// envelope based on --output.
func runConsumption(cmd *cobra.Command, flagDir *string, flags *consumptionFlags) error {
	jsonMode := outputMode(cmd) == outputJSON

	by, err := fleet.ParseGroupBy(flags.by)
	if err != nil {
		if jsonMode {
			return preResultFailureEnvelope(cmd, commandConsumption, "", false, err)
		}
		return err
	}

	mode, err := buildFetchMode(flags)
	if err != nil {
		if jsonMode {
			return preResultFailureEnvelope(cmd, commandConsumption, "", false, err)
		}
		return err
	}

	cfg, err := fleet.LoadConfig(*flagDir)
	if err != nil {
		if jsonMode {
			return preResultFailureEnvelope(cmd, commandConsumption, "", false, err)
		}
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "  (loaded %s)\n", cfg.LoadedFrom)

	res, warnings, err := fleet.AggregateConsumption(cmd.Context(), cfg, mode, by)
	if err != nil {
		if jsonMode {
			return preResultFailureEnvelope(cmd, commandConsumption, "", false, err)
		}
		return err
	}

	if jsonMode {
		return writeEnvelope(cmd, commandConsumption, "", false, res, warnings, nil)
	}

	emitConsumptionWarnings(warnings)
	return renderConsumptionText(cmd, by, res)
}

// buildFetchMode translates the mutually-exclusive temporal flags into a
// FetchMode. Cobra has already enforced mutual exclusion; this only parses
// the values.
func buildFetchMode(flags *consumptionFlags) (fleet.FetchMode, error) {
	switch {
	case flags.trailing != "":
		n, err := fleet.ParseTrailing(flags.trailing)
		if err != nil {
			return fleet.FetchMode{}, err
		}
		return fleet.FetchMode{Kind: fleet.FetchTrailing, Days: n}, nil
	case flags.since != "":
		t, err := time.Parse("2006-01-02", flags.since)
		if err != nil {
			return fleet.FetchMode{}, fmt.Errorf("--since value %q invalid: expected YYYY-MM-DD", flags.since)
		}
		return fleet.FetchMode{Kind: fleet.FetchSince, Since: t.UTC()}, nil
	default:
		return fleet.FetchMode{Kind: fleet.FetchLatest}, nil
	}
}

// emitConsumptionWarnings routes diagnostic warnings through zerolog so they
// surface on stderr above the table, mirroring deploy/sync's warning style.
func emitConsumptionWarnings(warnings []fleet.Diagnostic) {
	for _, w := range warnings {
		log.Warn().Str("code", w.Code).Msg(w.Message)
	}
}

// renderConsumptionText renders the primary grouped table and the
// top-burners footer to cmd.OutOrStdout().
func renderConsumptionText(cmd *cobra.Command, by fleet.GroupByKind, res *fleet.ConsumptionResult) error {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, tabPadding, ' ', 0)
	fmt.Fprintf(tw, "%s\tAPI_CALLS\tSAFE_WRITES\tCOST\tREPORTS\n", byColumnHeader(by))
	for _, g := range res.Groups {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%d\n",
			g.Key, g.GitHubAPICalls, g.SafeOutputCalls, formatCost(g.Cost), g.ReportCount)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(res.TopBurners) == 0 {
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "TOP 10 BURNERS:")
	bt := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, tabPadding, ' ', 0)
	fmt.Fprintln(bt, "WORKFLOW\tRUNS\tAPI_CALLS\tAVG_DURATION\tCOST")
	for _, w := range res.TopBurners {
		fmt.Fprintf(bt, "%s\t%d\t%d\t%.1fs\t%s\n",
			w.Workflow, w.Runs, w.APICalls, w.AvgDurationS, formatCost(w.Cost))
	}
	return bt.Flush()
}

// byColumnHeader maps the --by axis to its uppercase key-column header.
func byColumnHeader(by fleet.GroupByKind) string {
	switch by {
	case fleet.GroupByProfile:
		return "PROFILE"
	case fleet.GroupByCostCenter:
		return "COST_CENTER"
	case fleet.GroupByWorkflow:
		return "WORKFLOW"
	case fleet.GroupByRepo:
		return "REPO"
	}
	return "REPO"
}

// formatCost renders a *float64 cost as $%.2f when populated, else "-".
func formatCost(c *float64) string {
	if c == nil {
		return "-"
	}
	return fmt.Sprintf("$%.2f", *c)
}
