package cmd

import (
	"fmt"
	"os"
	"regexp"
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
	pruneDeleteBranch     bool
	pruneStale            bool
	pruneReviews          bool
)

var reviewAliasPattern = regexp.MustCompile(`^pr-[0-9]+$`)

func init() {
	rootCmd.AddCommand(pruneCmd)
	pruneCmd.Flags().BoolVar(&pruneForce, "force", false, "remove despite uncommitted changes or initialized submodules")
	pruneCmd.Flags().StringVar(&pruneBase, "base", "", "branch to check merges against (default: baseBranch from .groverc.json, else the auto-detected default branch)")
	pruneCmd.Flags().BoolVarP(&pruneYes, "yes", "y", false, "skip the confirmation prompt")
	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "show what would be removed without removing worktrees")
	pruneCmd.Flags().BoolVar(&pruneJSON, "json", false, "print dry-run result as JSON")
	pruneCmd.Flags().BoolVar(&pruneIncludeProtected, "include-protected", false, "include protected worktrees")
	pruneCmd.Flags().BoolVar(&pruneDeleteBranch, "delete-branch", false, "delete local branches of removed worktrees")
	pruneCmd.Flags().BoolVar(&pruneStale, "stale", false, "remove worktrees with neither an origin branch nor an open GitHub PR (requires gh)")
	pruneCmd.Flags().BoolVar(&pruneReviews, "reviews", false, "remove worktrees created by grove review, merged or not")
}

type pruneCandidate struct {
	Alias     string `json:"alias"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
	Port      int    `json:"-"`
	Protected bool   `json:"protected,omitempty"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
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
	Short: "Remove merged worktrees, or stale or review worktrees with --stale or --reviews",
	Long: `By default, remove grove-managed worktrees whose branch has already been merged into the
base branch: --base, else baseBranch from .groverc.json, else the auto-detected
default branch.

Detects both regular and squash merges. Merges are checked against the LOCAL
base branch, so pull it first to see PRs merged on the remote.

With --stale, instead select worktrees with neither a branch on origin nor an
open PR in origin's GitHub repository, even if unmerged. Requires authenticated
gh and network access. All remote checks must succeed before removal begins.
This can include unpublished local work: preview with --dry-run first.
With --stale --delete-branch, unmerged branches are kept unless --force is set.

With --reviews, instead select every worktree created by grove review, whether
or not its PR is merged. A worktree counts when grove review saved its PR
number, or when its alias is pr-<number>. No network access is needed.
--reviews cannot be combined with --stale or --base.
With --reviews --delete-branch, unmerged branches are kept unless --force is set.

Shows what will be removed and asks for confirmation. Use --yes to skip the
prompt and --force to remove worktrees with uncommitted changes.`,
	RunE: runPrune,
}

func runPrune(cmd *cobra.Command, args []string) error {
	if pruneJSON && !pruneDryRun {
		return fmt.Errorf("--json currently requires --dry-run")
	}
	if pruneReviews && (pruneStale || pruneBase != "") {
		return fmt.Errorf("--reviews cannot be combined with --stale or --base")
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
		base = resolvedBaseBranch(cfg)
	}
	if base == "" && !pruneReviews {
		return fmt.Errorf("could not determine base branch — pass --base <branch>")
	}

	if len(s.Worktrees) == 0 {
		if pruneJSON {
			return printJSON(pruneJSONResult{Base: base, DryRun: true, Candidates: []pruneCandidate{}, Skipped: []pruneSkipped{}})
		}
		fmt.Println("No managed worktrees to prune.")
		return nil
	}

	selection := fmt.Sprintf("Merged into %q", base)
	noCandidates := fmt.Sprintf("No worktrees merged into %q.", base)
	worktreeKind := "merged"
	var originHeads, openPRBranches map[string]bool
	defaultBranch := git.DefaultBranch()
	if pruneStale {
		if defaultBranch == "" {
			return fmt.Errorf("stale check: could not determine default branch")
		}
		originHeads, openPRBranches, err = stalePruneBranches(root)
		if err != nil {
			return err
		}
		selection = "No origin branch and no open PR"
		noCandidates = "No stale worktrees."
		worktreeKind = "stale"
	}
	if pruneReviews {
		selection = "Review worktrees"
		noCandidates = "No review worktrees."
		worktreeKind = "review"
	}
	// The merge check already proved merged branches safe to delete. The other
	// modes select unmerged work too, so only --force discards those commits.
	keepUnmergedBranches := (pruneStale || pruneReviews) && !pruneForce

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
		if entry.Branch == base || (pruneStale && entry.Branch == defaultBranch) {
			continue
		}
		reason := ""
		if pruneReviews {
			reason = reviewReason(alias, entry)
			if reason == "" {
				continue
			}
		} else if pruneStale {
			if originHeads[entry.Branch] || openPRBranches[entry.Branch] {
				continue
			}
			reason = "no origin branch and no open PR"
		} else {
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
		}
		if entry.Protected && !pruneIncludeProtected {
			skipped = append(skipped, pruneSkipped{Alias: alias, Branch: entry.Branch, Reason: "protected"})
			if !pruneJSON {
				fmt.Fprintf(os.Stderr, "  skipping protected worktree %s — pass --include-protected to remove\n", alias)
			}
			continue
		}
		status := worktreeStatus(entry.Path)
		merged = append(merged, pruneCandidate{Alias: alias, Branch: entry.Branch, Path: entry.Path, Port: entry.Port, Protected: entry.Protected, Status: status, Reason: reason})
		if hasUncommittedWork(status) {
			dirty = append(dirty, fmt.Sprintf("  %s (%s)", alias, status))
		}
	}

	if pruneJSON {
		return printJSON(pruneJSONResult{Base: base, DryRun: true, Candidates: merged, Skipped: skipped})
	}

	if pruneDryRun {
		if len(merged) == 0 {
			fmt.Println(noCandidates)
			return nil
		}
		fmt.Printf("%s — would remove:\n", selection)
		for _, wt := range merged {
			if pruneDeleteBranch {
				if keepUnmergedBranches {
					fmt.Printf("  %s → %s (branch %s will be deleted only if merged)\n", wt.Alias, wt.Path, wt.Branch)
				} else {
					fmt.Printf("  %s → %s (branch %s will be deleted)\n", wt.Alias, wt.Path, wt.Branch)
				}
			} else {
				fmt.Printf("  %s → %s\n", wt.Alias, wt.Path)
			}
		}
		return nil
	}

	if len(merged) == 0 {
		fmt.Println(noCandidates)
		return nil
	}

	// Non-interactive runs never silently discard uncommitted work: without
	// --force, drop dirty worktrees from the removal set.
	if pruneYes && len(dirty) > 0 && !pruneForce {
		var clean []pruneCandidate
		for _, wt := range merged {
			if !hasUncommittedWork(wt.Status) {
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

	fmt.Printf("%s — will remove:\n", selection)
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
	var removalFailures []error
	for _, wt := range merged {
		force, err := forceForRemoval(wt.Path, wt.Status, pruneForce, dirtyConfirmed)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  failed to remove %q: %v\n", wt.Alias, err)
			removalFailures = append(removalFailures, err)
			continue
		}
		ok, err := removeManagedWorktree(root, cfg, &s, managedRemoveTarget{
			Alias:            wt.Alias,
			Branch:           wt.Branch,
			Path:             wt.Path,
			Port:             wt.Port,
			IncludeProtected: pruneIncludeProtected,
		}, force)
		if ok && pruneDeleteBranch {
			deleteBranchAfterRemoval(wt.Branch, defaultBranch, !keepUnmergedBranches)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "  failed to remove %q: %v\n", wt.Alias, err)
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

	fmt.Printf("\nRemoved %d of %d %s worktree(s).\n", removed, len(merged), worktreeKind)
	return newBatchRemovalError(removalFailures)
}

// reviewReason explains why a worktree counts as created by grove review, or
// returns "" when it does not. An entry without a PR number still counts when
// its alias is pr-<number>, the alias grove review assigns by default.
func reviewReason(alias string, entry state.WorktreeEntry) string {
	if entry.PR > 0 {
		return fmt.Sprintf("review of PR #%d", entry.PR)
	}
	if reviewAliasPattern.MatchString(alias) {
		return "alias matches pr-<number>"
	}
	return ""
}
