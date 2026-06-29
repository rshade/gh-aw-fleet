package cmd

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

// errOverviewDrift is wrapped by overview RunE when any queried repo is not aligned.
var errOverviewDrift = errors.New("overview: drift detected")

const (
	overviewDefaultTrailingDays = 7
	percentScale                = 100
	flagLatest                  = "latest"
	flagSince                   = "since"
	flagTrailing                = "trailing"
)

type overviewFlags struct {
	latest   bool
	trailing string
	since    string
}

// runOverview is a package-level seam so command tests can exercise rendering
// and flag behavior without live gh/gh-aw calls.
//
//nolint:gochecknoglobals // test-injection seam for fleet.Overview
var runOverview = fleet.Overview

// newOverviewCmd returns the cobra command for the overview subcommand.
func newOverviewCmd(flagDir *string) *cobra.Command {
	var flags overviewFlags

	cmd := &cobra.Command{
		Use:   commandOverview + " [repo...]",
		Short: "Display a joined dashboard of drift, health, and cost across the fleet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOverviewCmd(cmd, flagDir, &flags, args)
		},
	}
	cmd.Flags().BoolVar(&flags.latest, flagLatest, false, "Most-recent run per workflow (NOOP best-effort)")
	cmd.Flags().StringVar(&flags.trailing, flagTrailing, "", "Trailing window like 7d (default: 7d)")
	cmd.Flags().StringVar(&flags.since, flagSince, "", "Since date in YYYY-MM-DD")
	cmd.MarkFlagsMutuallyExclusive(flagLatest, flagTrailing, flagSince)
	return cmd
}

func runOverviewCmd(cmd *cobra.Command, flagDir *string, flags *overviewFlags, repos []string) error {
	jsonMode := outputMode(cmd) == outputJSON

	mode, err := buildOverviewFetchMode(flags)
	if err != nil {
		return overviewFailure(cmd, jsonMode, err)
	}

	cfg, err := fleet.LoadConfig(*flagDir)
	if err != nil {
		return overviewFailure(cmd, jsonMode, err)
	}
	if !jsonMode {
		fmt.Fprintf(cmd.ErrOrStderr(), "(loaded %s)\n", cfg.LoadedFrom)
	}

	cfg, err = fleet.ScopeToRepos(cfg, repos)
	if err != nil {
		return overviewFailure(cmd, jsonMode, err)
	}

	res, diags, err := runOverview(cmd.Context(), cfg, fleet.OverviewOpts{Mode: mode})
	if mode.Kind == fleet.FetchLatest {
		diags = append(diags, latestNoopDiagnostic())
	}
	if err != nil {
		return overviewFailure(cmd, jsonMode, err)
	}
	if res == nil {
		res = &fleet.OverviewResult{LoadedFrom: cfg.LoadedFrom}
	}
	if !jsonMode {
		fmt.Fprintf(cmd.ErrOrStderr(), "overview · window: %s\n", res.Window)
	}

	warnings, hints := splitOverviewDiags(diags)
	if jsonMode {
		if writeErr := writeEnvelope(cmd, commandOverview, "", false, res, warnings, hints); writeErr != nil {
			return writeErr
		}
		return overviewExitCode(res)
	}

	emitOverviewDiagnostics(warnings, hints)
	if renderErr := renderOverviewText(cmd, res); renderErr != nil {
		return renderErr
	}
	return overviewExitCode(res)
}

func overviewFailure(cmd *cobra.Command, jsonMode bool, err error) error {
	if jsonMode {
		return preResultFailureEnvelope(cmd, commandOverview, "", false, err)
	}
	return err
}

func buildOverviewFetchMode(flags *overviewFlags) (fleet.FetchMode, error) {
	return parseFetchMode(flags.latest, flags.trailing, flags.since,
		fleet.FetchMode{Kind: fleet.FetchTrailing, Days: overviewDefaultTrailingDays})
}

func latestNoopDiagnostic() fleet.Diagnostic {
	msg := "NOOP is best-effort in --latest mode because gh aw logs exposes mcp_tool_usage as an aggregate over the fetched runs, not per run."
	return fleet.Diagnostic{Code: fleet.DiagHint, Message: msg, Fields: map[string]any{"hint": msg}}
}

func splitOverviewDiags(diags []fleet.Diagnostic) ([]fleet.Diagnostic, []fleet.Diagnostic) {
	warnings := []fleet.Diagnostic{}
	hints := []fleet.Diagnostic{}
	for _, d := range diags {
		switch d.Code {
		case fleet.DiagHint,
			fleet.DiagRepoInaccessible,
			fleet.DiagRateLimited,
			fleet.DiagNetworkUnreachable,
			fleet.DiagBillingQuotaExceeded:
			hints = append(hints, d)
		default:
			warnings = append(warnings, d)
		}
	}
	sort.SliceStable(hints, func(i, j int) bool {
		ri, _ := hints[i].Fields[diagnosticFieldRepo].(string)
		rj, _ := hints[j].Fields[diagnosticFieldRepo].(string)
		return ri < rj
	})
	return warnings, hints
}

func emitOverviewDiagnostics(warnings, hints []fleet.Diagnostic) {
	for _, w := range warnings {
		log.Warn().Str("code", w.Code).Msg(w.Message)
	}
	for _, h := range hints {
		repo, _ := h.Fields[diagnosticFieldRepo].(string)
		emitHints(repo, []string{h.Message})
	}
}

func renderOverviewText(cmd *cobra.Command, res *fleet.OverviewResult) error {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, tabPadding, ' ', 0)
	fmt.Fprintln(tw, "REPO\tDRIFT\tRUNS\tFAIL\tNOOP\tHEALTH\tAIC\tCOST")
	for _, row := range res.Repos {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			row.Repo,
			row.DriftState,
			overviewCell(row.RunsAvailable, strconv.Itoa(row.Runs)),
			overviewCell(row.RunsAvailable, strconv.Itoa(row.Failures)),
			overviewCell(row.RunsAvailable, strconv.Itoa(row.NoOps)),
			overviewCell(row.RunsAvailable, formatHealthRate(row.HealthRate)),
			overviewCell(row.RunsAvailable, formatAIC(row.AIC)),
			overviewCell(row.RunsAvailable, formatCost(row.Cost)),
		)
	}
	if len(res.Repos) > 0 {
		fmt.Fprintln(tw, "------------------------------------------------------------------------")
		fmt.Fprintf(tw, "TOTAL\t\t%d\t%d\t%d\t%s\t%s\t%s\n",
			res.Total.Runs,
			res.Total.Failures,
			res.Total.NoOps,
			formatHealthRate(res.Total.HealthRate),
			formatAIC(res.Total.AIC),
			formatCost(res.Total.Cost),
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	for _, row := range res.Repos {
		if needsOverviewDetail(row) {
			printOverviewDetail(cmd, row)
		}
	}
	return nil
}

// overviewCell renders value when the repo's run data is available, else the
// "-" placeholder shared by every unavailable run-derived column.
func overviewCell(available bool, value string) string {
	if !available {
		return "-"
	}
	return value
}

func formatHealthRate(rate *float64) string {
	if rate == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", math.Round(*rate*percentScale))
}

func needsOverviewDetail(row fleet.RepoOverview) bool {
	if row.DriftState != fleet.DriftStateAligned || !row.RunsAvailable {
		return true
	}
	return row.Runs > 0 && row.HealthRate != nil && *row.HealthRate < 1
}

func printOverviewDetail(cmd *cobra.Command, row fleet.RepoOverview) {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "\n%s (%s):\n", row.Repo, overviewDetailState(row))
	printOverviewDriftDetail(cmd, row)
	printOverviewRunsDetail(cmd, row)
}

func overviewDetailState(row fleet.RepoOverview) string {
	switch {
	case row.DriftState != fleet.DriftStateAligned:
		return row.DriftState
	case !row.RunsAvailable:
		return "runs unavailable"
	default:
		return "unhealthy"
	}
}

func printOverviewDriftDetail(cmd *cobra.Command, row fleet.RepoOverview) {
	w := cmd.OutOrStdout()
	detail := row.DriftDetail
	if detail == nil {
		return
	}
	if detail.DriftState == fleet.DriftStateErrored {
		fmt.Fprintf(w, "  drift: %s\n", detail.ErrorMessage)
		return
	}
	if len(detail.Missing) > 0 {
		fmt.Fprintf(w, "  missing:  %d\n", len(detail.Missing))
		for _, name := range detail.Missing {
			fmt.Fprintf(w, "    - %s\n", name)
		}
	}
	if len(detail.Extra) > 0 {
		fmt.Fprintf(w, "  extra:    %d\n", len(detail.Extra))
		for _, name := range detail.Extra {
			fmt.Fprintf(w, "    - %s\n", name)
		}
	}
	if len(detail.Drifted) > 0 {
		fmt.Fprintf(w, "  drifted:  %d\n", len(detail.Drifted))
		for _, d := range detail.Drifted {
			fmt.Fprintf(w, "    - %s %s -> %s\n", d.Name, d.DesiredRef, d.ActualRef)
		}
	}
	if len(detail.Unpinned) > 0 {
		fmt.Fprintf(w, "  unpinned: %d\n", len(detail.Unpinned))
		for _, name := range detail.Unpinned {
			fmt.Fprintf(w, "    - %s\n", name)
		}
	}
}

func printOverviewRunsDetail(cmd *cobra.Command, row fleet.RepoOverview) {
	w := cmd.OutOrStdout()
	if !row.RunsAvailable {
		fmt.Fprintf(w, "  runs: unavailable - %s\n", row.RunsError)
		return
	}
	fmt.Fprintf(w, "  runs: %d (%d failed, %d no-op) · health %s · cost %s\n",
		row.Runs, row.Failures, row.NoOps, formatHealthRate(row.HealthRate), formatCost(row.Cost))
	if row.Runs > 0 && row.Failures == row.Runs && row.Cost == nil {
		fmt.Fprintln(w, "  -> all runs failed in this window; cost is blank because failed runs report no credits.")
	}
}

func overviewExitCode(res *fleet.OverviewResult) error {
	if res == nil {
		return nil
	}
	if res.Total.Drifted+res.Total.Errored > 0 {
		return newCommandExitError(errOverviewDrift, 1, true)
	}
	return nil
}
