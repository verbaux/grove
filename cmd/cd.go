package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(cdCmd)
}

var cdCmd = &cobra.Command{
	Use:   "cd <name>",
	Short: "Print the path to a worktree",
	Long: `Print the path to a worktree so you can cd into it.

Usage:
  cd $(grove cd auth)

Or add a shell function (aliases can't take arguments):
  gcd() { cd "$(grove cd "$1")"; }`,
	Args: cobra.ExactArgs(1),
	RunE: runCd,
}

func runCd(cmd *cobra.Command, args []string) error {
	alias := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	s, err := state.Load(root)
	if err != nil {
		return err
	}

	entry, ok := s.Get(alias)
	if !ok {
		return fmt.Errorf("no worktree with alias %q — run 'grove list' to see available worktrees", alias)
	}

	// Print just the path, nothing else.
	// This output is captured by the shell: cd $(grove cd auth)
	fmt.Println(entry.Path)
	return nil
}
