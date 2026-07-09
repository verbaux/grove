package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func TestPruneJSONDryRun(t *testing.T) {
	repo := setupIntegrationRepo(t, config.Config{Prefix: "testproject"})

	wtRoot := t.TempDir()
	mergedPath := filepath.Join(wtRoot, "merged")
	addWorktreeWithCommit(t, repo, "merged-json", mergedPath)
	gitInRepo(t, repo, "merge", "--no-ff", "merged-json", "-m", "merge merged-json")

	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("merged-json", "merged-json", mergedPath, 0); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(repo, s); err != nil {
		t.Fatal(err)
	}

	pruneForce = false
	pruneBase = ""
	pruneYes = false
	pruneDryRun = true
	pruneJSON = true
	t.Cleanup(func() {
		pruneDryRun = false
		pruneJSON = false
	})

	out, err := captureStdout(t, func() error {
		return runPrune(pruneCmd, nil)
	})
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Base       string `json:"base"`
		DryRun     bool   `json:"dryRun"`
		Candidates []struct {
			Alias  string `json:"alias"`
			Branch string `json:"branch"`
			Path   string `json:"path"`
			Status string `json:"status"`
		} `json:"candidates"`
		Skipped []struct {
			Alias  string `json:"alias"`
			Branch string `json:"branch"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("prune --json --dry-run output is not valid JSON: %v\n%s", err, out)
	}
	if !result.DryRun || result.Base != "main" || len(result.Candidates) != 1 {
		t.Fatalf("unexpected prune JSON result: %+v", result)
	}
	if result.Candidates[0].Alias != "merged-json" || result.Candidates[0].Status == "" {
		t.Fatalf("unexpected candidate: %+v", result.Candidates[0])
	}
	if result.Skipped == nil {
		t.Fatalf("expected skipped to be a JSON array, got nil")
	}
	if _, err := os.Stat(mergedPath); err != nil {
		t.Fatalf("dry-run should not remove worktree: %v", err)
	}
}

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
