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
	query := args[0]

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

	resolved, err := resolveWorktree(query, s)
	if err != nil {
		return err
	}
	if resolved == nil {
		return fmt.Errorf("no worktree matching %q — run 'grove list' to see available worktrees", query)
	}

	label := resolved.Alias
	if label == "" {
		label = resolved.Branch
	}

	// If the path no longer exists on disk, the worktree was removed manually.
	// Skip git commands and just clean up state.
	if _, err := os.Stat(resolved.Path); os.IsNotExist(err) {
		fmt.Printf("Worktree path %s no longer exists, cleaning up state.\n", resolved.Path)
	} else {
		status, err := git.Status(resolved.Path)
		if err != nil {
			return err
		}

		force := removeForce
		if status != "clean" && !removeForce {
			fmt.Printf("Worktree %q has %s.\n", label, status)
			answer := prompt("Remove anyway? [y/N]", "n")
			if answer != "y" && answer != "Y" {
				fmt.Println("Aborted.")
				return nil
			}
			force = true
		}

		if err := git.RemoveWorktree(resolved.Path, force); err != nil {
			return err
		}
		fmt.Printf("  ✓ removed worktree at %s\n", resolved.Path)
	}

	if resolved.InState {
		if err := s.Remove(resolved.Alias); err != nil {
			return err
		}
		if err := state.Save(root, s); err != nil {
			return err
		}
	}

	if err := git.PruneWorktrees(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: git worktree prune failed: %v\n", err)
	}

	fmt.Printf("Worktree %q removed.\n", label)
	return nil
}
