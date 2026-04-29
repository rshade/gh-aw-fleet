// Package cmd wires the cobra command tree for gh-aw-fleet. Each subcommand
// is a small shell that parses flags, calls into internal/fleet for the
// actual work, and formats the result in text or JSON (see output.go).
package cmd

import (
	"github.com/spf13/cobra"

	logpkg "github.com/rshade/gh-aw-fleet/internal/log"
)

// NewRootCmd builds the root gh-aw-fleet command with all subcommands wired in.
// It owns flagDir, which subcommands read through closures over the returned pointer.
func NewRootCmd() *cobra.Command {
	var flagDir string

	root := &cobra.Command{
		Use:   "gh-aw-fleet",
		Short: "Declarative fleet manager for GitHub Agentic Workflows",
		Long: `gh-aw-fleet manages a fleet of repositories that deploy GitHub Agentic
Workflows (gh-aw). It reconciles each repo toward the desired state
declared in fleet.json, using gh aw add/update/upgrade under the hood
and calling Claude when a deploy or merge needs judgment.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			level, _ := cmd.Flags().GetString("log-level")
			format, _ := cmd.Flags().GetString("log-format")
			if err := logpkg.Configure(level, format); err != nil {
				return err
			}
			out, _ := cmd.Flags().GetString("output")
			return validateOutputMode(out)
		},
	}
	root.PersistentFlags().StringVar(&flagDir, "dir", ".", "Directory containing fleet.json")
	root.PersistentFlags().String("log-level", "info", "Log verbosity: trace|debug|info|warn|error")
	root.PersistentFlags().String("log-format", "console", "Log format: console|json")
	root.PersistentFlags().StringP("output", "o", "text", "Output format: text|json")
	root.AddCommand(
		newListCmd(&flagDir),
		newStatusCmd(&flagDir),
		newAddCmd(&flagDir),
		newTemplateCmd(&flagDir),
		newDeployCmd(&flagDir),
		newSyncCmd(&flagDir),
		newUpgradeCmd(&flagDir),
	)
	return root
}

// Execute runs the root command.
func Execute() error {
	return NewRootCmd().Execute()
}
