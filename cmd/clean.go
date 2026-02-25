package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

var cleanForce bool

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().BoolVar(&cleanForce, "force", false, "remove even if worktrees have uncommitted changes")
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all worktrees except the main one",
	Long: `Remove all grove-managed worktrees, keeping the main working tree intact.

Shows a list of what will be removed and asks for confirmation.
Use --force to remove even if worktrees have uncommitted changes.`,
	RunE: runClean,
}

func runClean(cmd *cobra.Command, args []string) error {
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

	if len(s.Worktrees) == 0 {
		fmt.Println("No worktrees to clean.")
		return nil
	}

	type worktreeInfo struct {
		alias  string
		path   string
		status string
	}

	var toRemove []worktreeInfo
	var dirty []string

	for alias, entry := range s.Worktrees {
		status, err := git.Status(entry.Path)
		if err != nil {
			status = "unknown"
		}
		toRemove = append(toRemove, worktreeInfo{alias, entry.Path, status})
		if status != "clean" {
			dirty = append(dirty, fmt.Sprintf("  %s (%s)", alias, status))
		}
	}

	if len(dirty) > 0 && !cleanForce {
		fmt.Println("The following worktrees have uncommitted changes:")
		fmt.Println(strings.Join(dirty, "\n"))
		fmt.Println()
	}

	fmt.Println("Will remove:")
	for _, wt := range toRemove {
		fmt.Printf("  %s → %s\n", wt.alias, wt.path)
	}
	fmt.Println()

	if len(dirty) > 0 && !cleanForce {
		answer := prompt("Some worktrees have changes. Remove all anyway? [y/N]", "n")
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	} else {
		answer := prompt(fmt.Sprintf("Remove %d worktree(s)? [y/N]", len(toRemove)), "n")
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// User confirmed removal — force-remove dirty worktrees so git doesn't reject them.
	force := cleanForce
	if len(dirty) > 0 && !cleanForce {
		force = true
	}

	// If one removal fails, keep going — state stays consistent with what was actually removed.
	var removed int
	for _, wt := range toRemove {
		if _, err := os.Stat(wt.path); os.IsNotExist(err) {
			// Path already gone — just clean up state
			if err := s.Remove(wt.alias); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: could not remove alias %s from state: %v\n", wt.alias, err)
			}
			removed++
			fmt.Printf("  ✓ cleaned stale entry %s (path no longer exists)\n", wt.alias)
			continue
		}
		if err := git.RemoveWorktree(wt.path, force); err != nil {
			fmt.Printf("  failed to remove %q: %v\n", wt.alias, err)
			continue
		}
		if err := s.Remove(wt.alias); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not remove alias %s from state: %v\n", wt.alias, err)
		}
		removed++
		fmt.Printf("  ✓ removed %s\n", wt.alias)
	}

	if err := state.Save(root, s); err != nil {
		return err
	}

	git.PruneWorktrees()

	fmt.Printf("\nRemoved %d of %d worktree(s).\n", removed, len(toRemove))
	return nil
}
