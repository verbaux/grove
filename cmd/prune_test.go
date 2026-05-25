package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

// gitInRepo runs a git command in dir, failing the test on error.
func gitInRepo(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

// addWorktreeWithCommit creates a worktree on a new branch and commits a file in it.
func addWorktreeWithCommit(t *testing.T, repo, branch, wtPath string) {
	t.Helper()
	gitInRepo(t, repo, "worktree", "add", "-b", branch, wtPath)
	if err := os.WriteFile(filepath.Join(wtPath, branch+".txt"), []byte("work"), 0644); err != nil {
		t.Fatal(err)
	}
	gitInRepo(t, wtPath, "add", ".")
	gitInRepo(t, wtPath, "commit", "-m", branch+" work")
}

func TestPruneRemovesMergedKeepsUnmerged(t *testing.T) {
	repo := setupIntegrationRepo(t, config.Config{Prefix: "testproject"})

	wtRoot := t.TempDir()
	mergedPath := filepath.Join(wtRoot, "merged")
	unmergedPath := filepath.Join(wtRoot, "unmerged")

	addWorktreeWithCommit(t, repo, "merged-feat", mergedPath)
	addWorktreeWithCommit(t, repo, "unmerged-feat", unmergedPath)

	// Merge only merged-feat into main.
	gitInRepo(t, repo, "merge", "--no-ff", "merged-feat", "-m", "merge merged-feat")

	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("merged", "merged-feat", mergedPath, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("unmerged", "unmerged-feat", unmergedPath, 0); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(repo, s); err != nil {
		t.Fatal(err)
	}

	pruneForce = false
	pruneBase = ""
	pruneYes = true

	if err := runPrune(pruneCmd, nil); err != nil {
		t.Fatal("runPrune failed:", err)
	}

	if _, err := os.Stat(mergedPath); !os.IsNotExist(err) {
		t.Errorf("merged worktree still exists at %s", mergedPath)
	}
	if _, err := os.Stat(unmergedPath); err != nil {
		t.Errorf("unmerged worktree should still exist: %v", err)
	}

	s, err = state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if s.AliasExists("merged") {
		t.Error("merged alias should be gone from state")
	}
	if !s.AliasExists("unmerged") {
		t.Error("unmerged alias should remain in state")
	}
}
