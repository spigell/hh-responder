package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Actual version can be specified in build command
	version = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
