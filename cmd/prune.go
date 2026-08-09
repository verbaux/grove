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
	pruneForce            bool
	pruneBase             string
	pruneYes              bool
	pruneDryRun           bool
	pruneJSON             bool
	pruneIncludeProtected bool
)

func init() {
	rootCmd.AddCommand(pruneCmd)
	pruneCmd.Flags().BoolVar(&pruneForce, "force", false, "remove despite uncommitted changes or initialized submodules")
	pruneCmd.Flags().StringVar(&pruneBase, "base", "", "branch to check merges against (default: auto-detected default branch)")
	pruneCmd.Flags().BoolVarP(&pruneYes, "yes", "y", false, "skip the confirmation prompt")
	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "show what would be removed without removing worktrees")
	pruneCmd.Flags().BoolVar(&pruneJSON, "json", false, "print dry-run result as JSON")
	pruneCmd.Flags().BoolVar(&pruneIncludeProtected, "include-protected", false, "include protected worktrees")
}

type pruneCandidate struct {
	Alias     string `json:"alias"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	Port      int    `json:"-"`
	Protected bool   `json:"protected,omitempty"`
	Status    string `json:"status"`
}

type pruneSkipped struct {
	Alias  string `json:"alias"`
	Branch string `json:"branch"`
	Reason string `json:"reason"`
}

type pruneJSONResult struct {
	Base       string           `json:"base"`
	DryRun     bool             `json:"dryRun"`
	Candidates []pruneCandidate `json:"candidates"`
	Skipped    []pruneSkipped   `json:"skipped"`
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
	if pruneJSON && !pruneDryRun {
		return fmt.Errorf("--json currently requires --dry-run")
	}

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

	base := pruneBase
	if base == "" {
		base = git.DefaultBranch()
	}
	if base == "" {
		return fmt.Errorf("could not determine base branch — pass --base <branch>")
	}

	if len(s.Worktrees) == 0 {
		if pruneJSON {
			return printJSON(pruneJSONResult{Base: base, DryRun: true, Candidates: []pruneCandidate{}, Skipped: []pruneSkipped{}})
		}
		fmt.Println("No managed worktrees to prune.")
		return nil
	}

	aliases := make([]string, 0, len(s.Worktrees))
	for alias := range s.Worktrees {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	merged := []pruneCandidate{}
	var dirty []string
	skipped := []pruneSkipped{}

	for _, alias := range aliases {
		entry := s.Worktrees[alias]
		if entry.Branch == base {
			continue
		}
		isMerged, err := git.IsMerged(entry.Branch, base)
		if err != nil {
			if pruneJSON {
				skipped = append(skipped, pruneSkipped{Alias: alias, Branch: entry.Branch, Reason: err.Error()})
			} else {
				fmt.Fprintf(os.Stderr, "  warning: could not check %q: %v\n", alias, err)
			}
			continue
		}
		if !isMerged {
			continue
		}
		if entry.Protected && !pruneIncludeProtected {
			skipped = append(skipped, pruneSkipped{Alias: alias, Branch: entry.Branch, Reason: "protected"})
			if !pruneJSON {
				fmt.Fprintf(os.Stderr, "  skipping protected worktree %s — pass --include-protected to remove\n", alias)
			}
			continue
		}
		status, err := git.Status(entry.Path)
		if err != nil {
			status = "unknown"
		}
		merged = append(merged, pruneCandidate{Alias: alias, Branch: entry.Branch, Path: entry.Path, Port: entry.Port, Protected: entry.Protected, Status: status})
		if status != "clean" {
			dirty = append(dirty, fmt.Sprintf("  %s (%s)", alias, status))
		}
	}

	if pruneJSON {
		return printJSON(pruneJSONResult{Base: base, DryRun: true, Candidates: merged, Skipped: skipped})
	}

	if pruneDryRun {
		if len(merged) == 0 {
			fmt.Printf("No worktrees merged into %q.\n", base)
			return nil
		}
		fmt.Printf("Merged into %q — would remove:\n", base)
		for _, wt := range merged {
			fmt.Printf("  %s → %s\n", wt.Alias, wt.Path)
		}
		return nil
	}

	if len(merged) == 0 {
		fmt.Printf("No worktrees merged into %q.\n", base)
		return nil
	}

	// Non-interactive runs never silently discard uncommitted work: without
	// --force, drop dirty worktrees from the removal set.
	if pruneYes && len(dirty) > 0 && !pruneForce {
		var clean []pruneCandidate
		for _, wt := range merged {
			if wt.Status == "clean" {
				clean = append(clean, wt)
			} else {
				fmt.Fprintf(os.Stderr, "  skipping %s (%s) — pass --force to remove\n", wt.Alias, wt.Status)
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
		fmt.Printf("  %s → %s\n", wt.Alias, wt.Path)
	}
	fmt.Println()

	if len(dirty) > 0 && !pruneForce {
		fmt.Println("The following worktrees have uncommitted changes:")
		fmt.Println(strings.Join(dirty, "\n"))
		fmt.Println()
	}

	dirtyConfirmed := false
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
		dirtyConfirmed = len(dirty) > 0 && !pruneForce
	}

	var removed int
	var removeErr error
	for _, wt := range merged {
		force, err := forceForRemoval(wt.Path, wt.Status, pruneForce, dirtyConfirmed)
		if err != nil {
			fmt.Printf("  failed to remove %q: %v\n", wt.Alias, err)
			if removeErr == nil {
				removeErr = err
			}
			continue
		}
		ok, err := removeManagedWorktree(root, cfg, &s, managedRemoveTarget{
			Alias:            wt.Alias,
			Branch:           wt.Branch,
			Path:             wt.Path,
			Port:             wt.Port,
			IncludeProtected: pruneIncludeProtected,
		}, force)
		if err != nil {
			fmt.Printf("  failed to remove %q: %v\n", wt.Alias, err)
			if removeErr == nil {
				removeErr = err
			}
			continue
		}
		if ok {
			removed++
		}
	}

	if err := git.PruneWorktrees(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: git worktree prune failed: %v\n", err)
	}

	fmt.Printf("\nRemoved %d of %d merged worktree(s).\n", removed, len(merged))
	return removeErr
}
