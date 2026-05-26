package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
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
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeAliases,
	RunE:              runCd,
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

	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}

	resolved, err := resolveWorktree(root, arg)
	if err != nil {
		return err
	}
	if resolved == nil {
		if arg == "" {
			return nil // picker cancelled
		}
		return fmt.Errorf("no worktree matching %q — run 'grove list' to see available worktrees", arg)
	}

	fmt.Println(resolved.Path)
	return nil
}

// pickWorktree shows the interactive fuzzy picker and returns the selected
// worktree path. An empty path with nil error means the picker was cancelled.
func pickWorktree(root string) (string, error) {
	rows, err := buildWorktreeRows(root)
	if err != nil {
		return "", err
	}

	if len(rows) == 0 {
		return "", fmt.Errorf("no worktrees found — create one with 'grove create'")
	}

	m, err := tea.NewProgram(newPicker(rows), tea.WithOutput(os.Stderr)).Run()
	if err != nil {
		return "", err
	}

	p := m.(pickerModel)
	if p.quitted || p.selected == "" {
		return "", nil
	}

	return p.selected, nil
}
