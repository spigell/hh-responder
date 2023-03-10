package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Actual version can be specified in build command.
var version = "unknown"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s version: %s\n", app, version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
