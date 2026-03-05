package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(cdCmd)
}

var cdCmd = &cobra.Command{
	Use:   "cd <name-or-number>",
	Short: "Print the path to a worktree",
	Long: `Print the path to a worktree so you can cd into it.

Accepts either a worktree alias or an index number from 'grove list'.

Usage:
  cd $(grove cd auth)
  cd $(grove cd 3)

Or add a shell function (aliases can't take arguments):
  gcd() { cd "$(grove cd "$1")"; }`,
	Args: cobra.ExactArgs(1),
	ValidArgsFunction: completeAliases,
	RunE: runCd,
}

func runCd(cmd *cobra.Command, args []string) error {
	arg := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	// If the argument is a number, resolve by index from the worktree list.
	if idx, err := strconv.Atoi(arg); err == nil {
		rows, err := buildWorktreeRows(root)
		if err != nil {
			return err
		}
		if idx < 1 || idx > len(rows) {
			return fmt.Errorf("index %d out of range — run 'grove list' to see available worktrees (1–%d)", idx, len(rows))
		}
		fmt.Println(rows[idx-1].Path)
		return nil
	}

	s, err := state.Load(root)
	if err != nil {
		return err
	}

	entry, ok := s.Get(arg)
	if !ok {
		return fmt.Errorf("no worktree with alias %q — run 'grove list' to see available worktrees", arg)
	}

	fmt.Println(entry.Path)
	return nil
}
