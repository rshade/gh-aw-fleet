package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/rshade/gh-aw-fleet/internal/fleet"
)

func newAddCmd(flagDir *string) *cobra.Command {
	var (
		flagProfiles []string
		flagEngine   string
		flagExcludes []string
		flagExtras   []string
		flagApply    bool
		flagYes      bool
	)
	cmd := &cobra.Command{
		Use:   "add <owner/repo>",
		Short: "Register a repo in fleet.local.json with a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, err := fleet.ValidateSlug(args[0])
			if err != nil {
				return err
			}

			confirmed, err := resolveConfirmation(cmd, flagApply, flagYes)
			if err != nil {
				return err
			}

			cfg, err := fleet.LoadConfig(*flagDir)
			if err != nil {
				return err
			}

			opts := fleet.AddOptions{
				Repo:           slug,
				Profiles:       flagProfiles,
				Engine:         flagEngine,
				Excludes:       flagExcludes,
				ExtraWorkflows: flagExtras,
				Apply:          flagApply,
				Confirmed:      confirmed,
				Dir:            *flagDir,
			}
			res, addErr := fleet.Add(cfg, opts)
			if addErr != nil {
				return addErr
			}
			printAdd(cmd, res)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&flagProfiles, "profile", nil,
		"Profile name(s) to assign (repeatable or comma-separated); required")
	cmd.Flags().StringVar(&flagEngine, "engine", "",
		"Engine override (e.g. claude, copilot); validated against EngineSecrets")
	cmd.Flags().StringArrayVar(&flagExcludes, "exclude", nil,
		"Workflow name to exclude from selected profiles (repeatable)")
	cmd.Flags().StringArrayVar(&flagExtras, "extra-workflow", nil,
		"Extra workflow spec to add outside profiles (repeatable). "+
			"Accepts: name | owner/repo/name@ref | owner/repo/.github/workflows/name.md@ref")
	cmd.Flags().BoolVar(&flagApply, "apply", false,
		"Actually write fleet.local.json (default is dry-run)")
	cmd.Flags().BoolVar(&flagYes, "yes", false,
		"Confirm --apply without an interactive prompt (required in non-TTY)")
	_ = cmd.MarkFlagRequired("profile")
	return cmd
}

func resolveConfirmation(cmd *cobra.Command, apply, yes bool) (bool, error) {
	if !apply {
		if yes {
			fmt.Fprintln(cmd.ErrOrStderr(), "ignored: --yes has no effect without --apply")
		}
		return false, nil
	}
	if yes {
		return true, nil
	}
	fd := int(os.Stdin.Fd()) //nolint:gosec // fd numbers are small and always fit in int
	if !term.IsTerminal(fd) {
		return false, errors.New("--apply requires --yes in a non-interactive shell")
	}
	fmt.Fprint(cmd.ErrOrStderr(), "Write fleet.local.json? [y/N] ")
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	resp := strings.ToLower(strings.TrimSpace(line))
	if resp != "y" && resp != "yes" {
		return false, errors.New("aborted: re-run with --apply --yes to confirm")
	}
	return true, nil
}

func printAdd(cmd *cobra.Command, res *fleet.AddResult) {
	stderr := cmd.ErrOrStderr()
	stdout := cmd.OutOrStdout()
	verb := "would add"
	if res.WroteLocal {
		verb = "added"
		if res.SynthesizedLocal {
			fmt.Fprintln(stderr,
				"creating fleet.local.json (minimal; profiles/defaults still resolved from fleet.json)")
		}
	}
	fmt.Fprintf(stderr, "%s %s with profiles [%s] (%d workflows)\n",
		verb, res.Repo, strings.Join(res.Profiles, ","), len(res.Resolved))
	if res.Engine != "" {
		fmt.Fprintf(stderr, "engine override: %s\n", res.Engine)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "warning: %s\n", w)
	}
	for _, w := range res.Resolved {
		fmt.Fprintf(stdout, "- %s\n", w.Name)
	}
	if res.WroteLocal {
		fmt.Fprintf(stderr, "next: gh-aw-fleet deploy %s\n", res.Repo)
	} else {
		fmt.Fprintln(stderr, "next: re-run with --apply to persist")
	}
}
