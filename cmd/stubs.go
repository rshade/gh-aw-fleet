package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [repo]",
		Short: "Diff desired (fleet.json) vs actual state of a repo's workflows",
		RunE:  notImplemented("status"),
	}
}

func newAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <owner/repo>",
		Short: "Register a repo in fleet.json with a profile",
		RunE:  notImplemented("add"),
	}
}

func notImplemented(name string) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf("%s: not yet implemented", name)
	}
}
