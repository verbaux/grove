package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

// setupReviewPrune registers one worktree per alias, each on a branch named
// after the alias with one unmerged commit. prs maps an alias to the PR number
// saved in state; aliases missing from prs get no PR number.
func setupReviewPrune(t *testing.T, prs map[string]int, aliases ...string) (string, map[string]string) {
	t.Helper()
	repo := setupIntegrationRepo(t, config.Config{Prefix: "testproject"})
	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, alias := range aliases {
		path := filepath.Join(t.TempDir(), alias)
		addWorktreeWithCommit(t, repo, alias, path)
		if err := s.Add(alias, alias, path, 0); err != nil {
			t.Fatal(err)
		}
		if pr := prs[alias]; pr > 0 {
			if err := s.SetPR(alias, pr); err != nil {
				t.Fatal(err)
			}
		}
		paths[alias] = path
	}
	if err := state.Save(repo, s); err != nil {
		t.Fatal(err)
	}
	pruneReviews, pruneStale = true, false
	pruneBase, pruneYes, pruneForce = "", false, false
	pruneDryRun, pruneJSON, pruneIncludeProtected, pruneDeleteBranch = true, true, false, false
	t.Cleanup(func() {
		pruneReviews, pruneStale = false, false
		pruneBase, pruneYes, pruneForce = "", false, false
		pruneDryRun, pruneJSON, pruneIncludeProtected, pruneDeleteBranch = false, false, false, false
	})
	return repo, paths
}

func TestPruneReviewsSelectsOnlyReviewWorktrees(t *testing.T) {
	repo, paths := setupReviewPrune(t, map[string]int{"named": 5, "locked": 9, "dirty": 3},
		"pr-12", "named", "locked", "dirty", "feature", "pr-draft")
	if err := state.Update(repo, func(s *state.State) error {
		return s.SetProtected("locked", true)
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths["dirty"], "untracked.txt"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(t, func() error { return runPrune(pruneCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Candidates []struct{ Alias, Reason string }
		Skipped    []struct{ Alias, Reason string }
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("prune --reviews --json stdout is not JSON: %v\n%s", err, out)
	}
	wantReasons := map[string]string{
		"dirty": "review of PR #3",
		"named": "review of PR #5",
		"pr-12": "alias matches pr-<number>",
	}
	if len(result.Candidates) != len(wantReasons) {
		t.Fatalf("unexpected review candidates: %s", out)
	}
	for _, candidate := range result.Candidates {
		if want, ok := wantReasons[candidate.Alias]; !ok || candidate.Reason != want {
			t.Fatalf("candidate %q has reason %q, want %q", candidate.Alias, candidate.Reason, want)
		}
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Alias != "locked" || result.Skipped[0].Reason != "protected" {
		t.Fatalf("protected review worktree must be reported as skipped: %s", out)
	}
	for alias, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preview removed %s: %v", alias, err)
		}
	}

	pruneDryRun, pruneJSON, pruneYes, pruneDeleteBranch = false, false, true, true
	if _, err := captureStdout(t, func() error { return runPrune(pruneCmd, nil) }); err != nil {
		t.Fatal(err)
	}
	assertReviewPruneRemoved(t, repo, paths, "named", "pr-12")
	for _, branch := range []string{"named", "pr-12"} {
		if !git.LocalBranchExists(branch) {
			t.Fatalf("--delete-branch without --force discarded unmerged branch %q", branch)
		}
	}

	pruneForce = true
	if _, err := captureStdout(t, func() error { return runPrune(pruneCmd, nil) }); err != nil {
		t.Fatal(err)
	}
	assertReviewPruneRemoved(t, repo, paths, "named", "pr-12", "dirty")
	if git.LocalBranchExists("dirty") {
		t.Fatal("--force --delete-branch should delete the unmerged branch after removal")
	}
}

// assertReviewPruneRemoved fails unless exactly the removed aliases lost both
// their worktree directory and their state entry.
func assertReviewPruneRemoved(t *testing.T, repo string, paths map[string]string, removed ...string) {
	t.Helper()
	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	gone := map[string]bool{}
	for _, alias := range removed {
		gone[alias] = true
	}
	for alias, path := range paths {
		_, statErr := os.Stat(path)
		_, managed := s.Get(alias)
		if gone[alias] {
			if !os.IsNotExist(statErr) || managed {
				t.Fatalf("review worktree %q was not removed: stat=%v managed=%v", alias, statErr, managed)
			}
		} else if statErr != nil || !managed {
			t.Fatalf("must retain %q: stat=%v managed=%v", alias, statErr, managed)
		}
	}
}

func TestPruneReviewsRejectsOtherSelectionFlags(t *testing.T) {
	for name, set := range map[string]func(){
		"stale": func() { pruneStale = true },
		"base":  func() { pruneBase = "main" },
	} {
		t.Run(name, func(t *testing.T) {
			repo, paths := setupReviewPrune(t, nil, "pr-12")
			pruneDryRun, pruneJSON, pruneYes = false, false, true
			set()
			if _, err := captureStdout(t, func() error { return runPrune(pruneCmd, nil) }); err == nil {
				t.Fatalf("--reviews with --%s must fail", name)
			}
			assertReviewPruneRemoved(t, repo, paths)
		})
	}
}
