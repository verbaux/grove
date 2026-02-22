package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

var removeForce bool

func init() {
	rootCmd.AddCommand(removeCmd)
	removeCmd.Flags().BoolVar(&removeForce, "force", false, "remove even if there are uncommitted changes")
}

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a worktree",
	Long: `Remove a worktree by alias.

Checks for uncommitted changes and asks for confirmation before removing.
Use --force to skip the check.`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
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

	// If the path no longer exists on disk, the worktree was removed manually.
	// Skip git commands and just clean up state.
	if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
		fmt.Printf("Worktree path %s no longer exists, cleaning up state.\n", entry.Path)
	} else {
		status, err := git.Status(entry.Path)
		if err != nil {
			return err
		}

		if status != "clean" && !removeForce {
			fmt.Printf("Worktree %q has %s.\n", alias, status)
			answer := prompt("Remove anyway? [y/N]", "n")
			if answer != "y" && answer != "Y" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		if err := git.RemoveWorktree(entry.Path, removeForce); err != nil {
			return err
		}
		fmt.Printf("  ✓ removed worktree at %s\n", entry.Path)
	}

	if err := s.Remove(alias); err != nil {
		return err
	}
	if err := state.Save(root, s); err != nil {
		return err
	}

	git.PruneWorktrees()

	fmt.Printf("Worktree %q removed.\n", alias)
	return nil
}
