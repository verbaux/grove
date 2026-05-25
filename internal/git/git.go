package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// run executes a git command and returns its stdout.
// All git operations go through this — one place to debug if something breaks.
func run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Worktree holds info about a single worktree from `git worktree list`.
type Worktree struct {
	Path   string
	Branch string
	IsMain bool
}

// AddWorktree creates a new worktree. If the branch doesn't exist, it creates it.
// `from` is the base branch/commit — if empty, uses current HEAD.
func AddWorktree(path, branch, from string) error {
	// Make path absolute so git doesn't get confused by relative paths
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	if branchExists(branch) {
		_, err = run("worktree", "add", absPath, branch)
	} else if from != "" {
		_, err = run("worktree", "add", "-b", branch, absPath, from)
	} else {
		_, err = run("worktree", "add", "-b", branch, absPath)
	}
	return err
}

// RemoveWorktree removes a worktree by path.
// Pass force=true to remove even if there are uncommitted changes.
func RemoveWorktree(path string, force bool) error {
	if force {
		_, err := run("worktree", "remove", "--force", path)
		return err
	}
	_, err := run("worktree", "remove", path)
	return err
}

// PruneWorktrees cleans up stale worktree references.
func PruneWorktrees() error {
	_, err := run("worktree", "prune")
	return err
}

// ListWorktrees parses output of `git worktree list` into structured data.
// The first entry is always the main worktree.
func ListWorktrees() ([]Worktree, error) {
	// --porcelain gives machine-readable output, one key-value pair per line,
	// worktrees separated by blank lines.
	out, err := run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []Worktree
	var current Worktree
	var currentHead string
	var detached bool

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			currentHead = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "detached":
			detached = true
		case line == "":
			// Blank line = end of one worktree entry
			if current.Path != "" {
				if detached && current.Branch == "" {
					// Show short commit hash so the user knows where they are
					if len(currentHead) >= 7 {
						current.Branch = "(detached " + currentHead[:7] + ")"
					} else {
						current.Branch = "(detached)"
					}
				}
				worktrees = append(worktrees, current)
			}
			current = Worktree{}
			currentHead = ""
			detached = false
		}
	}
	// Last entry (no trailing blank line)
	if current.Path != "" {
		if detached && current.Branch == "" {
			if len(currentHead) >= 7 {
				current.Branch = "(detached " + currentHead[:7] + ")"
			} else {
				current.Branch = "(detached)"
			}
		}
		worktrees = append(worktrees, current)
	}

	// First worktree in git's output is always the main one
	if len(worktrees) > 0 {
		worktrees[0].IsMain = true
	}

	return worktrees, nil
}

// Status returns a short status summary for a worktree path.
// Returns "clean" or a breakdown like "2 staged, 1 modified, 3 untracked".
func Status(worktreePath string) (string, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git status in %s: %w", worktreePath, err)
	}

	// git status --porcelain: each line starts with two status chars XY.
	// X = staging area, Y = working tree.
	// "??" = untracked file.
	// We split on newlines and skip empty lines — do NOT TrimSpace on the whole
	// output, as leading spaces in lines like " M file.txt" are meaningful status chars.
	var staged, modified, untracked int
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		if x == '?' && y == '?' {
			untracked++
			continue
		}
		if x != ' ' {
			staged++
		}
		if y != ' ' {
			modified++
		}
	}

	if staged == 0 && modified == 0 && untracked == 0 {
		return "clean", nil
	}

	var parts []string
	if staged > 0 {
		parts = append(parts, fmt.Sprintf("%d staged", staged))
	}
	if modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", modified))
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", untracked))
	}
	return strings.Join(parts, ", "), nil
}

// Fetch fetches a refspec from a remote.
func Fetch(remote, refspec string) error {
	_, err := run("fetch", remote, refspec)
	return err
}

// DefaultBranch returns the repository's default branch (e.g. "main").
// Prefers the remote's HEAD (origin/HEAD); falls back to the main worktree's
// branch. Returns "" if neither can be determined.
func DefaultBranch() string {
	if out, err := run("symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimPrefix(out, "origin/")
	}
	worktrees, err := ListWorktrees()
	if err == nil && len(worktrees) > 0 {
		return worktrees[0].Branch
	}
	return ""
}

// IsMerged reports whether branch has already been merged into base.
// It detects both regular/fast-forward merges (branch tip is an ancestor of
// base) and squash merges (branch's cumulative diff already lives in base, but
// the tip is not an ancestor).
func IsMerged(branch, base string) (bool, error) {
	if branch == "" || base == "" {
		return false, nil
	}

	// Regular/fast-forward merge: branch tip is reachable from base.
	if _, err := run("merge-base", "--is-ancestor", branch, base); err == nil {
		return true, nil
	}

	// Squash merge: synthesize a commit holding branch's tree on top of the
	// merge-base, then ask `git cherry` whether base already contains an
	// equivalent patch. The synthetic commit is dangling and never referenced;
	// git gc reclaims it.
	mergeBase, err := run("merge-base", base, branch)
	if err != nil {
		// Unrelated histories — not merged.
		return false, nil
	}
	tree, err := run("rev-parse", branch+"^{tree}")
	if err != nil {
		return false, err
	}
	synthetic, err := run("commit-tree", tree, "-p", mergeBase, "-m", "grove-prune-check")
	if err != nil {
		return false, err
	}
	cherry, err := run("cherry", base, synthetic)
	if err != nil {
		return false, nil
	}
	cherry = strings.TrimSpace(cherry)
	if cherry == "" {
		// Nothing diverged from base — the ancestor check above already covers
		// true merges, so an empty diff here means no unique work to compare.
		return false, nil
	}
	// Each line is "- <sha>" (patch present in base) or "+ <sha>" (absent).
	// Fully merged means every line is "-".
	for _, line := range strings.Split(cherry, "\n") {
		if !strings.HasPrefix(line, "-") {
			return false, nil
		}
	}
	return true, nil
}

func branchExists(branch string) bool {
	_, err := run("rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}
