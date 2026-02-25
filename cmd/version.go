package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
// Example: go build -ldflags "-X github.com/verbaux/grove/cmd.Version=1.0.0"
var Version = "dev"

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the grove version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("grove %s\n", Version)
	},
}
