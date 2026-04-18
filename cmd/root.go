package cmd

import (
	"github.com/spf13/cobra"
)

var (
	flagDir string
)

var rootCmd = &cobra.Command{
	Use:   "gh-aw-fleet",
	Short: "Declarative fleet manager for GitHub Agentic Workflows",
	Long: `gh-aw-fleet manages a fleet of repositories that deploy GitHub Agentic
Workflows (gh-aw). It reconciles each repo toward the desired state
declared in fleet.json, using gh aw add/update/upgrade under the hood
and calling Claude when a deploy or merge needs judgment.`,
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagDir, "dir", ".", "Directory containing fleet.json")
	rootCmd.AddCommand(listCmd, statusCmd, addCmd, templateCmd, deployCmd, syncCmd, upgradeCmd)
}
