package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [repo]",
		Short: "Diff desired (fleet.json) vs actual state of a repo's workflows",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := rejectJSONMode(cmd, "status"); err != nil {
				return err
			}
			return errors.New("status: not yet implemented")
		},
	}
}
