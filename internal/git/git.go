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

func branchExists(branch string) bool {
	_, err := run("rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}
