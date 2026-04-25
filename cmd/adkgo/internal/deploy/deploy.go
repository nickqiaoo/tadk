// Package deploy allows to run deployment-related subcommands.
package deploy

import (
	"github.com/spf13/cobra"

	"github.com/nickqiaoo/tadk/cmd/adkgo/internal/root"
)

// DeployCmd represents the deploy command.
var DeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Makes deployment to various platforms easy",
	Long:  `Please see subcommands for details`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return nil
	},
}

func init() {
	root.RootCmd.AddCommand(DeployCmd)
}
