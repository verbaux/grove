package cmd

import (
	"fmt"
	"os"
	"sort"
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
		fmt.Println("No managed worktrees to clean.")
		if orphanRemoved, err := cleanOrphans(s, cleanForce); err != nil {
			return err
		} else if orphanRemoved > 0 {
			fmt.Printf("Removed %d orphan worktree(s).\n", orphanRemoved)
		}
		return nil
	}

	type worktreeInfo struct {
		alias  string
		path   string
		status string
	}

	aliases := make([]string, 0, len(s.Worktrees))
	for alias := range s.Worktrees {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	var toRemove []worktreeInfo
	var dirty []string

	for _, alias := range aliases {
		entry := s.Worktrees[alias]
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

	// User confirmed removal of dirty worktrees — pass force to git.
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

	if err := git.PruneWorktrees(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: git worktree prune failed: %v\n", err)
	}

	fmt.Printf("\nRemoved %d of %d worktree(s).\n", removed, len(toRemove))

	// Phase 2: orphan worktrees (git knows, grove doesn't)
	if orphanRemoved, err := cleanOrphans(s, cleanForce); err != nil {
		return err
	} else if orphanRemoved > 0 {
		fmt.Printf("Removed %d orphan worktree(s).\n", orphanRemoved)
	}

	return nil
}

func cleanOrphans(s state.State, force bool) (int, error) {
	orphans, err := findOrphans(s)
	if err != nil {
		return 0, err
	}

	if len(orphans) == 0 {
		return 0, nil
	}

	fmt.Printf("\nFound %d orphan worktree(s) not managed by grove:\n", len(orphans))

	var dirty []string
	for _, o := range orphans {
		status, err := git.Status(o.Path)
		if err != nil {
			status = "unknown"
		}
		marker := ""
		if status != "clean" {
			marker = " (" + status + ")"
			dirty = append(dirty, o.Branch)
		}
		fmt.Printf("  %s → %s%s\n", o.Branch, o.Path, marker)
	}
	fmt.Println()

	if len(dirty) > 0 && !force {
		answer := prompt("Some orphan worktrees have changes. Remove all anyway? [y/N]", "n")
		if answer != "y" && answer != "Y" {
			fmt.Println("Skipped orphan cleanup.")
			return 0, nil
		}
		force = true
	} else {
		answer := prompt(fmt.Sprintf("Remove %d orphan worktree(s)? [y/N]", len(orphans)), "n")
		if answer != "y" && answer != "Y" {
			fmt.Println("Skipped orphan cleanup.")
			return 0, nil
		}
	}

	var removed int
	for _, o := range orphans {
		if err := git.RemoveWorktree(o.Path, force); err != nil {
			fmt.Printf("  failed to remove orphan %q: %v\n", o.Branch, err)
			continue
		}
		removed++
		fmt.Printf("  ✓ removed orphan %s\n", o.Branch)
	}

	return removed, nil
}
