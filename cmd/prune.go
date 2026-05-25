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

var (
	pruneForce bool
	pruneBase  string
	pruneYes   bool
)

func init() {
	rootCmd.AddCommand(pruneCmd)
	pruneCmd.Flags().BoolVar(&pruneForce, "force", false, "remove even if worktrees have uncommitted changes")
	pruneCmd.Flags().StringVar(&pruneBase, "base", "", "branch to check merges against (default: auto-detected default branch)")
	pruneCmd.Flags().BoolVarP(&pruneYes, "yes", "y", false, "skip the confirmation prompt")
}

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove worktrees whose branch is already merged",
	Long: `Remove grove-managed worktrees whose branch has already been merged into the
base branch (auto-detected, or set with --base).

Detects both regular and squash merges. Merges are checked against the LOCAL
base branch, so pull it first to see PRs merged on the remote.

Shows what will be removed and asks for confirmation. Use --yes to skip the
prompt and --force to remove worktrees with uncommitted changes.`,
	RunE: runPrune,
}

func runPrune(cmd *cobra.Command, args []string) error {
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

	base := pruneBase
	if base == "" {
		base = git.DefaultBranch()
	}
	if base == "" {
		return fmt.Errorf("could not determine base branch — pass --base <branch>")
	}

	if len(s.Worktrees) == 0 {
		fmt.Println("No managed worktrees to prune.")
		return nil
	}

	aliases := make([]string, 0, len(s.Worktrees))
	for alias := range s.Worktrees {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	type mergedInfo struct {
		alias  string
		path   string
		status string
	}

	var merged []mergedInfo
	var dirty []string

	for _, alias := range aliases {
		entry := s.Worktrees[alias]
		if entry.Branch == base {
			continue
		}
		isMerged, err := git.IsMerged(entry.Branch, base)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not check %q: %v\n", alias, err)
			continue
		}
		if !isMerged {
			continue
		}
		status, err := git.Status(entry.Path)
		if err != nil {
			status = "unknown"
		}
		merged = append(merged, mergedInfo{alias, entry.Path, status})
		if status != "clean" {
			dirty = append(dirty, fmt.Sprintf("  %s (%s)", alias, status))
		}
	}

	if len(merged) == 0 {
		fmt.Printf("No worktrees merged into %q.\n", base)
		return nil
	}

	// Non-interactive runs never silently discard uncommitted work: without
	// --force, drop dirty worktrees from the removal set.
	if pruneYes && len(dirty) > 0 && !pruneForce {
		var clean []mergedInfo
		for _, wt := range merged {
			if wt.status == "clean" {
				clean = append(clean, wt)
			} else {
				fmt.Fprintf(os.Stderr, "  skipping %s (%s) — pass --force to remove\n", wt.alias, wt.status)
			}
		}
		merged = clean
		dirty = nil
		if len(merged) == 0 {
			fmt.Println("Nothing to remove.")
			return nil
		}
	}

	fmt.Printf("Merged into %q — will remove:\n", base)
	for _, wt := range merged {
		fmt.Printf("  %s → %s\n", wt.alias, wt.path)
	}
	fmt.Println()

	if len(dirty) > 0 && !pruneForce {
		fmt.Println("The following worktrees have uncommitted changes:")
		fmt.Println(strings.Join(dirty, "\n"))
		fmt.Println()
	}

	if !pruneYes {
		var answer string
		if len(dirty) > 0 && !pruneForce {
			answer = prompt("Some worktrees have changes. Remove all anyway? [y/N]", "n")
		} else {
			answer = prompt(fmt.Sprintf("Remove %d worktree(s)? [y/N]", len(merged)), "n")
		}
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// User confirmed removal of dirty worktrees interactively — pass force to git.
	force := pruneForce
	if !pruneYes && len(dirty) > 0 {
		force = true
	}

	var removed int
	for _, wt := range merged {
		if _, err := os.Stat(wt.path); os.IsNotExist(err) {
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

	fmt.Printf("\nRemoved %d of %d merged worktree(s).\n", removed, len(merged))
	return nil
}
