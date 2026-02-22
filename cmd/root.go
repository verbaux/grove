package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "grove",
	Short:         "Git worktree manager — work on multiple branches without the hassle",
	SilenceUsage:  true, // don't print usage on every error — errors speak for themselves
	SilenceErrors: true, // we handle error printing in Execute() to avoid duplicate output
	Long: `Grove automates the tedious parts of git worktree:
  - Copies .env files to new worktrees
  - Symlinks node_modules (no extra npm install)

Get started with: grove init`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
