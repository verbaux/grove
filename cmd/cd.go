package cmd

import (
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
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
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: completeAliases,
	RunE: runCd,
}

func runCd(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return runPicker(root)
	}

	arg := args[0]

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

func runPicker(root string) error {
	rows, err := buildWorktreeRows(root)
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		return fmt.Errorf("no worktrees found — create one with 'grove create'")
	}

	m, err := tea.NewProgram(newPicker(rows), tea.WithOutput(os.Stderr)).Run()
	if err != nil {
		return err
	}

	p := m.(pickerModel)
	if p.quitted || p.selected == "" {
		return nil
	}

	fmt.Println(p.selected)
	return nil
}
