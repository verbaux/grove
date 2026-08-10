package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

var (
	cleanForce            bool
	cleanIncludeProtected bool
)

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().BoolVar(&cleanForce, "force", false, "remove despite uncommitted changes or initialized submodules")
	cleanCmd.Flags().BoolVar(&cleanIncludeProtected, "include-protected", false, "include protected worktrees")
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all worktrees except the main one",
	Long: `Remove all grove-managed worktrees, keeping the main working tree intact.

Shows a list of what will be removed and asks for confirmation.
Use --force to remove despite uncommitted changes or initialized submodules.`,
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
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	if len(s.Worktrees) == 0 {
		fmt.Println("No managed worktrees to clean.")
		if _, err := cleanOrphans(s, cleanForce); err != nil {
			return err
		}
		return nil
	}

	type worktreeInfo struct {
		alias  string
		branch string
		path   string
		port   int
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
		if entry.Protected && !cleanIncludeProtected {
			fmt.Printf("  skipping protected worktree %s — pass --include-protected to remove\n", alias)
			continue
		}
		toRemove = append(toRemove, worktreeInfo{alias, entry.Branch, entry.Path, entry.Port, status})
		if status != "clean" {
			dirty = append(dirty, fmt.Sprintf("  %s (%s)", alias, status))
		}
	}
	if len(toRemove) == 0 {
		fmt.Println("No removable managed worktrees.")
		if _, err := cleanOrphans(s, cleanForce); err != nil {
			return err
		}
		return nil
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

	dirtyConfirmed := false
	if len(dirty) > 0 && !cleanForce {
		answer := prompt("Some worktrees have changes. Remove all anyway? [y/N]", "n")
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
		dirtyConfirmed = true
	} else {
		answer := prompt(fmt.Sprintf("Remove %d worktree(s)? [y/N]", len(toRemove)), "n")
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// If one removal fails, keep going — state stays consistent with what was actually removed.
	var removed int
	var removalFailures []error
	for _, wt := range toRemove {
		force, err := forceForRemoval(wt.path, wt.status, cleanForce, dirtyConfirmed)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  failed to remove %q: %v\n", wt.alias, err)
			removalFailures = append(removalFailures, err)
			continue
		}
		ok, err := removeManagedWorktree(root, cfg, &s, managedRemoveTarget{
			Alias:            wt.alias,
			Branch:           wt.branch,
			Path:             wt.path,
			Port:             wt.port,
			IncludeProtected: cleanIncludeProtected,
		}, force)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  failed to remove %q: %v\n", wt.alias, err)
			removalFailures = append(removalFailures, err)
			continue
		}
		if ok {
			removed++
		}
	}

	if err := git.PruneWorktrees(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: git worktree prune failed: %v\n", err)
	}

	fmt.Printf("\nRemoved %d of %d worktree(s).\n", removed, len(toRemove))

	// Phase 2: orphan worktrees (git knows, grove doesn't)
	if _, err := cleanOrphans(s, cleanForce); err != nil {
		var batchErr *batchRemovalError
		if !errors.As(err, &batchErr) {
			return err
		}
		removalFailures = append(removalFailures, batchErr.failures...)
	}

	return newBatchRemovalError(removalFailures)
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
	statuses := make(map[string]string, len(orphans))
	for _, o := range orphans {
		status, err := git.Status(o.Path)
		if err != nil {
			status = "unknown"
		}
		statuses[o.Path] = status
		marker := ""
		if status != "clean" {
			marker = " (" + status + ")"
			dirty = append(dirty, o.Branch)
		}
		fmt.Printf("  %s → %s%s\n", o.Branch, o.Path, marker)
	}
	fmt.Println()

	dirtyConfirmed := false
	if len(dirty) > 0 && !force {
		answer := prompt("Some orphan worktrees have changes. Remove all anyway? [y/N]", "n")
		if answer != "y" && answer != "Y" {
			fmt.Println("Skipped orphan cleanup.")
			return 0, nil
		}
		dirtyConfirmed = true
	} else {
		answer := prompt(fmt.Sprintf("Remove %d orphan worktree(s)? [y/N]", len(orphans)), "n")
		if answer != "y" && answer != "Y" {
			fmt.Println("Skipped orphan cleanup.")
			return 0, nil
		}
	}

	var removed int
	var removalFailures []error
	for _, o := range orphans {
		orphanForce, err := forceForRemoval(o.Path, statuses[o.Path], force, dirtyConfirmed)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  failed to remove orphan %q: %v\n", o.Branch, err)
			removalFailures = append(removalFailures, err)
			continue
		}
		if err := git.RemoveWorktree(o.Path, orphanForce); err != nil {
			fmt.Fprintf(os.Stderr, "  failed to remove orphan %q: %v\n", o.Branch, err)
			removalFailures = append(removalFailures, err)
			continue
		}
		removed++
		fmt.Printf("  ✓ removed orphan %s\n", o.Branch)
	}

	if removed > 0 {
		fmt.Printf("Removed %d orphan worktree(s).\n", removed)
	}
	return removed, newBatchRemovalError(removalFailures)
}
