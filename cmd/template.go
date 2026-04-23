package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

func newTemplateCmd(flagDir *string) *cobra.Command {
	tpl := &cobra.Command{
		Use:   "template",
		Short: "Manage the upstream template catalog (templates.json)",
	}
	tpl.AddCommand(newTemplateFetchCmd(flagDir))
	return tpl
}

func newTemplateFetchCmd(flagDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "fetch",
		Short: "Refresh templates.json from gh-aw and agentics; Claude-classify new entries",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := rejectJSONMode(cmd, "template fetch"); err != nil {
				return err
			}
			return runTemplateFetch(cmd, *flagDir)
		},
	}
}

func runTemplateFetch(cmd *cobra.Command, dir string) error {
	cfg, err := fleet.LoadConfig(dir)
	if err != nil {
		return err
	}
	prev, err := fleet.LoadTemplates(dir)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = cmd.Root().Context()
	}
	next, results, err := fleet.FetchAll(ctx, cfg, prev)
	if err != nil {
		return err
	}
	if saveErr := fleet.SaveTemplates(dir, next); saveErr != nil {
		return saveErr
	}

	w := cmd.OutOrStdout()
	total := 0
	for _, r := range results {
		total += len(r.Added) + len(r.Changed) + len(r.Unchanged) + len(r.Removed)
	}
	fmt.Fprintf(w, "Fetched %d workflows across %d sources. Catalog written to %s/%s.\n\n",
		total, len(results), dir, fleet.TemplatesFile)
	for _, r := range results {
		fmt.Fprintf(w, "%s @ %s\n", r.Source, r.Ref)
		fmt.Fprintf(w, "  added:     %v\n", nilAsDash(r.Added))
		fmt.Fprintf(w, "  changed:   %v\n", nilAsDash(r.Changed))
		fmt.Fprintf(w, "  removed:   %v\n", nilAsDash(r.Removed))
		fmt.Fprintf(w, "  unchanged: %d\n\n", len(r.Unchanged))
	}
	if hasChanges(results) {
		fmt.Fprintln(w, "Tip: ask Claude to review the new/changed workflows by pointing at templates.json.")
	}
	return nil
}

func nilAsDash[T any](s []T) any {
	if len(s) == 0 {
		return "-"
	}
	return s
}

func hasChanges(rs []fleet.FetchResult) bool {
	for _, r := range rs {
		if len(r.Added)+len(r.Changed)+len(r.Removed) > 0 {
			return true
		}
	}
	return false
}
