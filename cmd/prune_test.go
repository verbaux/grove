package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
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

func TestPruneTextDryRunShowsBranchDeletion(t *testing.T) {
	repo := setupIntegrationRepo(t, config.Config{Prefix: "testproject"})
	mergedPath := filepath.Join(t.TempDir(), "sq")
	addWorktreeWithCommit(t, repo, "sq", mergedPath)
	gitInRepo(t, repo, "merge", "--no-ff", "sq", "-m", "merge sq")
	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("sq", "sq", mergedPath, 0); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(repo, s); err != nil {
		t.Fatal(err)
	}

	pruneBase, pruneYes, pruneForce = "", false, false
	pruneDryRun, pruneJSON, pruneIncludeProtected, pruneDeleteBranch = true, false, false, true
	t.Cleanup(func() {
		pruneBase, pruneYes, pruneForce = "", false, false
		pruneDryRun, pruneJSON, pruneIncludeProtected, pruneDeleteBranch = false, false, false, false
	})
	out, err := captureStdout(t, func() error { return runPrune(pruneCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	want := "  sq → " + mergedPath + " (branch sq will be deleted)"
	if !strings.Contains(out, want) {
		t.Errorf("prune dry-run output = %q, want line %q", out, want)
	}
	if !git.LocalBranchExists("sq") {
		t.Fatal("dry-run deleted branch")
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

func TestPruneDeleteBranchOnlyForRemovedWorktrees(t *testing.T) {
	repo := setupIntegrationRepo(t, config.Config{Prefix: "testproject"})
	wtRoot := t.TempDir()
	cleanPath := filepath.Join(wtRoot, "clean")
	protectedPath := filepath.Join(wtRoot, "protected")
	dirtyPath := filepath.Join(wtRoot, "dirty")
	addWorktreeWithCommit(t, repo, "clean-merged", cleanPath)
	addWorktreeWithCommit(t, repo, "protected-merged", protectedPath)
	addWorktreeWithCommit(t, repo, "dirty-merged", dirtyPath)
	for _, branch := range []string{"clean-merged", "protected-merged", "dirty-merged"} {
		gitInRepo(t, repo, "merge", "--no-ff", branch, "-m", "merge "+branch)
	}
	if err := os.WriteFile(filepath.Join(dirtyPath, "untracked.txt"), []byte("dirty\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	for alias, item := range map[string]struct{ branch, path string }{
		"clean":     {"clean-merged", cleanPath},
		"protected": {"protected-merged", protectedPath},
		"dirty":     {"dirty-merged", dirtyPath},
	} {
		if err := s.Add(alias, item.branch, item.path, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetProtected("protected", true); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(repo, s); err != nil {
		t.Fatal(err)
	}

	pruneBase, pruneYes, pruneForce = "", true, false
	pruneDryRun, pruneJSON, pruneIncludeProtected, pruneDeleteBranch = false, false, false, true
	t.Cleanup(func() {
		pruneBase, pruneYes, pruneForce = "", false, false
		pruneDryRun, pruneJSON, pruneIncludeProtected, pruneDeleteBranch = false, false, false, false
	})
	if err := runPrune(pruneCmd, nil); err != nil {
		t.Fatal(err)
	}

	if git.LocalBranchExists("clean-merged") {
		t.Fatal("branch for removed worktree still exists")
	}
	for _, branch := range []string{"protected-merged", "dirty-merged"} {
		if !git.LocalBranchExists(branch) {
			t.Errorf("branch %q for skipped worktree was deleted", branch)
		}
	}
	for _, path := range []string{cleanPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("removed worktree still exists at %s", path)
		}
	}
	for _, path := range []string{protectedPath, dirtyPath} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("skipped worktree missing at %s: %v", path, err)
		}
	}
}

func TestPruneSkipsProtectedUnlessIncluded(t *testing.T) {
	repo := setupIntegrationRepo(t, config.Config{Prefix: "testproject"})

	wtRoot := t.TempDir()
	protectedPath := filepath.Join(wtRoot, "protected")
	unprotectedPath := filepath.Join(wtRoot, "unprotected")

	addWorktreeWithCommit(t, repo, "protected-merged", protectedPath)
	addWorktreeWithCommit(t, repo, "unprotected-merged", unprotectedPath)
	gitInRepo(t, repo, "merge", "--no-ff", "protected-merged", "-m", "merge protected")
	gitInRepo(t, repo, "merge", "--no-ff", "unprotected-merged", "-m", "merge unprotected")

	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("protected", "protected-merged", protectedPath, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProtected("protected", true); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("unprotected", "unprotected-merged", unprotectedPath, 0); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(repo, s); err != nil {
		t.Fatal(err)
	}

	pruneForce = false
	pruneBase = ""
	pruneYes = true
	pruneDryRun = false
	pruneJSON = false
	pruneIncludeProtected = false
	t.Cleanup(func() { pruneIncludeProtected = false })

	if err := runPrune(pruneCmd, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(protectedPath); err != nil {
		t.Fatalf("protected worktree should remain: %v", err)
	}
	if _, err := os.Stat(unprotectedPath); !os.IsNotExist(err) {
		t.Fatalf("unprotected worktree should be removed, stat err=%v", err)
	}
	s, err = state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := s.Get("protected"); !ok || !entry.Protected {
		t.Fatalf("protected state should remain, got %+v", entry)
	}
	if s.AliasExists("unprotected") {
		t.Fatal("unprotected alias should be removed")
	}

	pruneIncludeProtected = true
	if err := runPrune(pruneCmd, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(protectedPath); !os.IsNotExist(err) {
		t.Fatalf("protected worktree should be removed with include-protected, stat err=%v", err)
	}
}

func TestPruneYesRemovesStaleMergedEntry(t *testing.T) {
	repo := setupIntegrationRepo(t, config.Config{Prefix: "testproject"})

	stalePath := filepath.Join(t.TempDir(), "stale")
	addWorktreeWithCommit(t, repo, "stale-feat", stalePath)
	gitInRepo(t, repo, "merge", "--no-ff", "stale-feat", "-m", "merge stale-feat")
	if err := os.RemoveAll(stalePath); err != nil {
		t.Fatal(err)
	}

	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("stale", "stale-feat", stalePath, 0); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(repo, s); err != nil {
		t.Fatal(err)
	}

	pruneForce, pruneBase, pruneYes = false, "", true
	t.Cleanup(func() { pruneYes = false })

	if _, err := captureStderr(t, func() error {
		_, runErr := captureStdout(t, func() error { return runPrune(pruneCmd, nil) })
		return runErr
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AliasExists("stale") {
		t.Error("stale merged entry was skipped as dirty instead of cleaned")
	}
}
