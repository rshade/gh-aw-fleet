package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [repo]",
	Short: "Diff desired (fleet.json) vs actual state of a repo's workflows",
	RunE:  notImplemented("status"),
}

var addCmd = &cobra.Command{
	Use:   "add <owner/repo>",
	Short: "Register a repo in fleet.json with a profile",
	RunE:  notImplemented("add"),
}

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage the upstream template catalog (templates.json)",
}

var templateFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Refresh templates.json from gh-aw and agentics; Claude-classify new entries",
	RunE:  notImplemented("template fetch"),
}

var deployCmd = &cobra.Command{
	Use:   "deploy <repo>",
	Short: "Apply the declared workflow set to a repo via gh aw add + PR",
}

var syncCmd = &cobra.Command{
	Use:   "sync <repo>",
	Short: "Reconcile a repo to match its declared profile (add missing, flag drift)",
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [repo|--all]",
	Short: "Bump profile pin + run gh aw upgrade + update across repos",
	RunE:  notImplemented("upgrade"),
}

func init() {
	templateCmd.AddCommand(templateFetchCmd)
}

func notImplemented(name string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("%s: not yet implemented", name)
	}
}
