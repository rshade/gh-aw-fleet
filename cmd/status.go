package cmd

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	zlog "github.com/rs/zerolog/log"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

// errStatusDrift is wrapped by status RunE when any queried repo is not
// aligned. The wrapper carries exit code 1 while telling main not to log this
// expected CI-gate result as a fatal internal command failure.
var errStatusDrift = errors.New("status: drift detected")

// statusTabPadding is the gutter width for the tabwriter columns; matches
// the deploy / sync output style.
const statusTabPadding = 2

func newStatusCmd(flagDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status [repo]",
		Short: "Diff desired (fleet.json) vs actual workflow refs across the fleet — read-only, no clones",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoArg := ""
			if len(args) == 1 {
				repoArg = args[0]
			}
			jsonMode := outputMode(cmd) == outputJSON

			cfg, err := fleet.LoadConfig(*flagDir)
			if err != nil {
				if jsonMode {
					return preResultFailureEnvelope(cmd, "status", repoArg, false, err)
				}
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "(loaded %s)\n", cfg.LoadedFrom)

			res, diags, statusErr := fleet.Status(cmd.Context(), cfg, fleet.StatusOpts{Repo: repoArg})
			if statusErr != nil {
				if jsonMode {
					return preResultFailureEnvelope(cmd, "status", repoArg, false, statusErr)
				}
				return statusErr
			}

			if jsonMode {
				return emitStatusEnvelope(cmd, repoArg, res, diags)
			}

			printStatus(cmd, res, diags)
			return statusExitCode(res)
		},
	}
}

// statusExitCode returns nil iff every queried repo is aligned. Any
// drift / errored / unpinned state surfaces as a silent exit-code error.
func statusExitCode(res *fleet.StatusResult) error {
	if res == nil {
		return nil
	}
	for _, r := range res.Repos {
		if r.DriftState != "aligned" {
			return newCommandExitError(errStatusDrift, 1, true)
		}
	}
	return nil
}

// printStatus renders the text-mode tabwriter summary plus per-repo
// detail blocks for any repo that is not aligned.
func printStatus(cmd *cobra.Command, res *fleet.StatusResult, diags []fleet.Diagnostic) {
	w := cmd.OutOrStdout()
	if res == nil {
		return
	}

	emitStatusWarnings(diags)

	tw := tabwriter.NewWriter(w, 0, 0, statusTabPadding, ' ', 0)
	fmt.Fprintln(tw, "REPO\tSTATE\tMISSING\tEXTRA\tDRIFTED\tUNPINNED")
	for _, r := range res.Repos {
		if r.DriftState == "errored" {
			fmt.Fprintf(tw, "%s\t%s\t-\t-\t-\t-\n", r.Repo, r.DriftState)
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\n",
			r.Repo, r.DriftState,
			len(r.Missing), len(r.Extra), len(r.Drifted), len(r.Unpinned),
		)
	}
	_ = tw.Flush()

	for _, r := range res.Repos {
		if r.DriftState == "aligned" {
			continue
		}
		printRepoDetail(w, r)
	}
}

func printRepoDetail(w io.Writer, r fleet.RepoStatus) {
	fmt.Fprintf(w, "\n%s (%s)\n", r.Repo, r.DriftState)
	if r.DriftState == "errored" {
		fmt.Fprintf(w, "  error: %s\n", r.ErrorMessage)
		return
	}
	if len(r.Missing) > 0 {
		fmt.Fprintf(w, "  missing:  %d\n", len(r.Missing))
		for _, name := range r.Missing {
			fmt.Fprintf(w, "    - %s\n", name)
		}
	}
	if len(r.Extra) > 0 {
		fmt.Fprintf(w, "  extra:    %d\n", len(r.Extra))
		for _, name := range r.Extra {
			fmt.Fprintf(w, "    - %s\n", name)
		}
	}
	if len(r.Drifted) > 0 {
		fmt.Fprintf(w, "  drifted:  %d\n", len(r.Drifted))
		for _, d := range r.Drifted {
			fmt.Fprintf(w, "    - %s %s → %s\n", d.Name, d.DesiredRef, d.ActualRef)
		}
	}
	if len(r.Unpinned) > 0 {
		fmt.Fprintf(w, "  unpinned: %d\n", len(r.Unpinned))
		for _, name := range r.Unpinned {
			fmt.Fprintf(w, "    - %s\n", name)
		}
	}
}

// emitStatusEnvelope writes the JSON envelope for a status invocation and
// returns the exit-code error (errStatusDrift on any non-aligned repo).
// Splits the diags slice into envelope warnings vs hints by code.
func emitStatusEnvelope(
	cmd *cobra.Command, repoArg string, res *fleet.StatusResult, diags []fleet.Diagnostic,
) error {
	warnings, hints := splitStatusDiags(diags)

	for _, h := range hints {
		repo, _ := h.Fields["repo"].(string)
		emitHints(repo, []string{h.Message})
	}
	logStatusWarnings(warnings)

	if writeErr := writeEnvelope(cmd, "status", repoArg, false, res, warnings, hints); writeErr != nil {
		return writeErr
	}
	return statusExitCode(res)
}

func emitStatusWarnings(diags []fleet.Diagnostic) {
	warnings, _ := splitStatusDiags(diags)
	logStatusWarnings(warnings)
}

func logStatusWarnings(warnings []fleet.Diagnostic) {
	for _, wn := range warnings {
		zlog.Warn().Str("code", wn.Code).Msg(wn.Message)
	}
}

// splitStatusDiags routes per-repo error diagnostics (DiagRepoInaccessible,
// DiagRateLimited, DiagNetworkUnreachable) to hints[] and fleet-wide
// notices (DiagEmptyFleet and future warning-class codes) to warnings[].
// Hints are sorted by repo for
// deterministic ordering across runs.
func splitStatusDiags(diags []fleet.Diagnostic) ([]fleet.Diagnostic, []fleet.Diagnostic) {
	warnings := []fleet.Diagnostic{}
	hints := []fleet.Diagnostic{}
	for _, d := range diags {
		switch d.Code {
		case fleet.DiagRepoInaccessible, fleet.DiagRateLimited, fleet.DiagNetworkUnreachable:
			hints = append(hints, d)
		default:
			warnings = append(warnings, d)
		}
	}
	sort.SliceStable(hints, func(i, j int) bool {
		ri, _ := hints[i].Fields["repo"].(string)
		rj, _ := hints[j].Fields["repo"].(string)
		return ri < rj
	})
	return warnings, hints
}
