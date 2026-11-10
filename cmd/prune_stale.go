package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/verbaux/grove/internal/git"
)

// stalePruneBranches completes all remote checks before any removal can begin.
// A failure or incomplete response must never become evidence of absence.
func stalePruneBranches(root string) (map[string]bool, map[string]bool, error) {
	if err := requireGH(); err != nil {
		return nil, nil, fmt.Errorf("stale check: %w", err)
	}
	remoteURL, heads, err := git.OriginHeads(root)
	if err != nil {
		return nil, nil, fmt.Errorf("stale check: could not read origin: %w", err)
	}
	// gh paginates up to this limit. Refuse a saturated response instead of
	// risking removal based on a truncated list of open pull requests.
	const limit = 10000
	command := exec.Command("gh", "pr", "list", "--repo", remoteURL,
		"--state", "open", "--limit", strconv.Itoa(limit), "--json", "headRefName,number")
	command.Dir = root
	out, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
				err = fmt.Errorf("%w: %s", err, stderr)
			}
		}
		return nil, nil, fmt.Errorf("stale check: could not list open PRs for origin: %w", err)
	}
	var prs []struct {
		HeadRefName string `json:"headRefName"`
		Number      int    `json:"number"`
	}
	if err := json.Unmarshal(out, &prs); err != nil || prs == nil {
		return nil, nil, fmt.Errorf("stale check: invalid open PR response")
	}
	if len(prs) >= limit {
		return nil, nil, fmt.Errorf("stale check: open PR list may be incomplete (limit %d)", limit)
	}
	openPRBranches := make(map[string]bool)
	for _, pr := range prs {
		if pr.HeadRefName == "" || pr.Number <= 0 {
			return nil, nil, fmt.Errorf("stale check: incomplete open PR response")
		}
		openPRBranches[pr.HeadRefName] = true
		// Fork PRs created by grove review use this local branch name.
		openPRBranches[fmt.Sprintf("pr-%d", pr.Number)] = true
	}
	return heads, openPRBranches, nil
}
