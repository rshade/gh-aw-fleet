package cmd

import (
	"github.com/spf13/cobra"
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
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&flagDir, "dir", ".", "Directory containing fleet.json")
	root.AddCommand(
		newListCmd(&flagDir),
		newStatusCmd(),
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
