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
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("%s version: %s\n", app, version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
