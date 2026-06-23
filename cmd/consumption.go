package cmd

import (
	"fmt"
	"math"
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
	source   string
	budget   float64
}

// aggregateConsumption is a package-level seam (matching the ghLogsAPI /
// ghWorkflowsAPI convention in internal/fleet) so cmd tests can stub the
// rollup without live gh calls. Tests save/restore it; cmd tests do not run
// in parallel, so a package var is safe.
//
//nolint:gochecknoglobals // test-injection seam for fleet.AggregateConsumption
var aggregateConsumption = fleet.AggregateConsumption

// newConsumptionCmd builds the cobra command for `gh-aw-fleet consumption`.
// It aggregates per-repo api-consumption-report output across the fleet via
// the layers in internal/fleet/consumption.go.
func newConsumptionCmd(flagDir *string) *cobra.Command {
	var flags consumptionFlags

	cmd := &cobra.Command{
		Use:   commandConsumption + " [repo...]",
		Short: "Aggregate api-consumption-report output across the fleet",
		Long: `Aggregate api-consumption-report output across the fleet.

Pass one or more repos (owner/name) to scope the rollup to just those repos;
with no args it covers the whole fleet. Combine with --by workflow to drill
into a single repo's per-workflow spend.

Discovery and data depend on --source (see below). The default --source logs
enumerates each repo's agentic workflows via the Actions API and sums AI
credits from gh aw logs --json per workflow.

Temporal modes (mutually exclusive):
  --latest                        most-recent valid report per repo (default)
  --trailing Nd                   all reports in the trailing N-day window
  --since YYYY-MM-DD              all reports on or after the given date

Grouping axis:
  --by repo|profile|cost-center|workflow   (default: repo)

Data source:
  --source logs        (default) enumerate agentic workflows via the Actions
                       API; AI credits from gh aw logs --json per workflow.
                       Needs no deployed api-consumption-report workflow.
  --source artifacts   legacy: discover via audits-category discussions
                       filtered by the api-consumption-report-daily tracker
                       marker; data from the run's aw_info.json and
                       run_summary.json artifacts.

Budget highlighting:
  --budget AIC         mark rows whose AIC strictly exceeds this ceiling`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsumption(cmd, flagDir, &flags, args)
		},
	}
	cmd.Flags().BoolVar(&flags.latest, "latest", true, "Most-recent valid report per repo (default)")
	cmd.Flags().StringVar(&flags.trailing, "trailing", "", "Trailing window like 7d")
	cmd.Flags().StringVar(&flags.since, "since", "", "Since date in YYYY-MM-DD")
	cmd.Flags().StringVar(&flags.by, "by", "repo", "Group-by axis: repo|profile|cost-center|workflow")
	cmd.Flags().StringVar(&flags.source, "source", "logs", "Data source: logs|artifacts")
	cmd.Flags().Float64Var(&flags.budget, "budget", 0, "Highlight rows whose AIC strictly exceeds this ceiling")
	cmd.MarkFlagsMutuallyExclusive("latest", "trailing", "since")
	return cmd
}

// runConsumption loads fleet config, builds the FetchMode from flags,
// invokes the aggregator, and writes either tabwriter text or the JSON
// envelope based on --output.
func runConsumption(cmd *cobra.Command, flagDir *string, flags *consumptionFlags, repos []string) error {
	jsonMode := outputMode(cmd) == outputJSON

	by, err := fleet.ParseGroupBy(flags.by)
	if err != nil {
		return consumptionFailure(cmd, jsonMode, err)
	}

	source, err := fleet.ParseSource(flags.source)
	if err != nil {
		return consumptionFailure(cmd, jsonMode, err)
	}

	mode, err := buildFetchMode(flags)
	if err != nil {
		return consumptionFailure(cmd, jsonMode, err)
	}

	budget, err := buildBudget(cmd, flags)
	if err != nil {
		return consumptionFailure(cmd, jsonMode, err)
	}

	cfg, err := fleet.LoadConfig(*flagDir)
	if err != nil {
		return consumptionFailure(cmd, jsonMode, err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "  (loaded %s)\n", cfg.LoadedFrom)

	cfg, err = fleet.ScopeToRepos(cfg, repos)
	if err != nil {
		return consumptionFailure(cmd, jsonMode, err)
	}

	res, warnings, err := aggregateConsumption(cmd.Context(), cfg, mode, by, source)
	if err != nil {
		return consumptionFailure(cmd, jsonMode, err)
	}
	fleet.ApplyBudget(res, budget)

	if jsonMode {
		return writeEnvelope(cmd, commandConsumption, "", false, res, warnings, nil)
	}

	emitConsumptionWarnings(warnings)
	return renderConsumptionText(cmd, by, res)
}

func consumptionFailure(cmd *cobra.Command, jsonMode bool, err error) error {
	if jsonMode {
		return preResultFailureEnvelope(cmd, commandConsumption, "", false, err)
	}
	return err
}

// buildBudget translates the optional --budget flag into a pointer so zero can
// remain a valid ceiling while an absent flag stays nil.
func buildBudget(cmd *cobra.Command, flags *consumptionFlags) (*float64, error) {
	if !cmd.Flags().Changed("budget") {
		//nolint:nilnil // a nil ceiling is the valid "no --budget supplied" signal
		return nil, nil
	}
	if math.IsNaN(flags.budget) || math.IsInf(flags.budget, 0) {
		return nil, fmt.Errorf(
			"--budget value %g invalid: expected a finite AIC ceiling",
			flags.budget,
		)
	}
	if flags.budget < 0 {
		return nil, fmt.Errorf(
			"--budget value %g invalid: expected a non-negative AIC ceiling",
			flags.budget,
		)
	}
	v := flags.budget
	return &v, nil
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
	budgeted := res.Budget != nil
	// overColumn appends the trailing OVER column only when a budget is set,
	// so no-budget output stays byte-identical (no trailing tab).
	overColumn := func(over *bool) string {
		if !budgeted {
			return ""
		}
		return "\t" + overBudgetMarker(over)
	}
	overHeader := ""
	if budgeted {
		overHeader = "\tOVER"
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, tabPadding, ' ', 0)
	fmt.Fprintf(tw, "%s\tAPI_CALLS\tSAFE_WRITES\tAIC\tCOST\tREPORTS%s\n", byColumnHeader(by), overHeader)
	for _, g := range res.Groups {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%s\t%d%s\n",
			g.Key, g.GitHubAPICalls, g.SafeOutputCalls, formatAIC(g.AIC),
			formatCost(g.Cost), g.ReportCount, overColumn(g.OverBudget))
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
	fmt.Fprintf(bt, "WORKFLOW\tRUNS\tAPI_CALLS\tAVG_DURATION\tAIC\tCOST%s\n", overHeader)
	for _, w := range res.TopBurners {
		fmt.Fprintf(bt, "%s\t%d\t%d\t%.1fs\t%s\t%s%s\n",
			w.Workflow, w.Runs, w.APICalls, w.AvgDurationS,
			formatAIC(w.AIC), formatCost(w.Cost), overColumn(w.OverBudget))
	}
	return bt.Flush()
}

func overBudgetMarker(over *bool) string {
	if over != nil && *over {
		return "!"
	}
	return ""
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

// formatAIC renders an AI-credit total as %.2f when populated, else "-". No "$"
// prefix — AIC is credits, not dollars (the derived USD lives in the COST column).
func formatAIC(a *float64) string {
	if a == nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", *a)
}
