// Package root handles command line parameters
package root

import (
	"os"

	"github.com/spf13/cobra"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "adkgo",
	Short: "CLI tool for use with ADK-GO",
	Long:  `adkgo is a CLI tool which allows developer to quickly deploy and test an agentic application`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	err := RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
