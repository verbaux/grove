package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func setupStalePrune(t *testing.T, branches ...string) (string, map[string]string) {
	t.Helper()
	repo := setupIntegrationRepo(t, config.Config{Prefix: "testproject"})
	origin := filepath.Join(t.TempDir(), "origin.git")
	gitInRepo(t, repo, "init", "--bare", origin)
	gitInRepo(t, repo, "remote", "add", "origin", origin)
	gitInRepo(t, repo, "push", "origin", "main")
	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, branch := range branches {
		path := filepath.Join(t.TempDir(), branch)
		addWorktreeWithCommit(t, repo, branch, path)
		if err := s.Add(branch, branch, path, 0); err != nil {
			t.Fatal(err)
		}
		paths[branch] = path
	}
	if err := state.Save(repo, s); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s' \"$GROVE_TEST_PRS\"\nprintf '%s' \"$GROVE_TEST_GH_STDERR\" >&2\nexit \"$GROVE_TEST_GH_EXIT\"\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GROVE_TEST_PRS", "[]")
	t.Setenv("GROVE_TEST_GH_EXIT", "0")
	t.Setenv("GROVE_TEST_GH_STDERR", "")
	if err := pruneCmd.Flags().Set("stale", "true"); err != nil {
		t.Fatal(err)
	}
	pruneBase, pruneYes, pruneForce = "", false, false
	pruneDryRun, pruneJSON, pruneIncludeProtected, pruneDeleteBranch = true, true, false, false
	t.Cleanup(func() {
		_ = pruneCmd.Flags().Set("stale", "false")
		pruneBase, pruneYes, pruneForce = "", false, false
		pruneDryRun, pruneJSON, pruneIncludeProtected, pruneDeleteBranch = false, false, false, false
	})
	return repo, paths
}

func TestPruneStalePreviewAndRemoval(t *testing.T) {
	repo, paths := setupStalePrune(t, "stale", "published", "open-pr", "pr-42", "protected", "dirty")
	gitInRepo(t, repo, "push", "origin", "published")
	gitInRepo(t, repo, "update-ref", "-d", "refs/remotes/origin/published")
	// A deleted origin branch must be seen as absent without fetch --prune.
	gitInRepo(t, repo, "push", "origin", "stale")
	gitInRepo(t, repo, "push", "origin", "--delete", "stale")
	gitInRepo(t, repo, "update-ref", "refs/remotes/origin/stale", "refs/heads/stale")
	t.Setenv("GROVE_TEST_PRS", `[{"headRefName":"open-pr","number":7},{"headRefName":"fork-feature","number":42}]`)
	t.Setenv("GROVE_TEST_GH_STDERR", "warning: gh update available\n")
	if err := state.Update(repo, func(s *state.State) error {
		entry := s.Worktrees["protected"]
		entry.Protected = true
		s.Worktrees["protected"] = entry
		return nil
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
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 2 || result.Candidates[0].Alias != "dirty" || result.Candidates[1].Alias != "stale" {
		t.Fatalf("unexpected stale candidates: %s", out)
	}
	for _, candidate := range result.Candidates {
		if candidate.Reason == "" {
			t.Fatal("candidate must explain why it is stale")
		}
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preview removed %s: %v", path, err)
		}
	}
	pruneDryRun, pruneJSON, pruneYes, pruneDeleteBranch = false, false, true, true
	if _, err := captureStdout(t, func() error { return runPrune(pruneCmd, nil) }); err != nil {
		t.Fatal(err)
	}
	s, err := state.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	for branch, path := range paths {
		_, statErr := os.Stat(path)
		_, managed := s.Get(branch)
		if branch == "stale" {
			if !os.IsNotExist(statErr) || managed {
				t.Fatalf("stale worktree was not removed: stat=%v managed=%v", statErr, managed)
			}
		} else if statErr != nil || !managed {
			t.Fatalf("must retain %s: stat=%v managed=%v", branch, statErr, managed)
		}
	}
	if !git.LocalBranchExists("stale") {
		t.Fatal("--delete-branch without --force discarded unmerged commits")
	}
}

func TestPruneStaleCheckFailureNeverRemoves(t *testing.T) {
	for _, scenario := range []string{"origin unavailable", "origin missing", "gh failure", "malformed JSON", "null JSON", "invalid PR", "truncated PR list"} {
		t.Run(scenario, func(t *testing.T) {
			repo, paths := setupStalePrune(t, "stale")
			switch scenario {
			case "origin unavailable":
				gitInRepo(t, repo, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
			case "origin missing":
				gitInRepo(t, repo, "remote", "remove", "origin")
			case "gh failure":
				t.Setenv("GROVE_TEST_GH_EXIT", "1")
				t.Setenv("GROVE_TEST_GH_STDERR", "To authenticate, run: gh auth login\n")
			case "malformed JSON":
				t.Setenv("GROVE_TEST_PRS", "garbage")
			case "null JSON":
				t.Setenv("GROVE_TEST_PRS", "null")
			case "invalid PR":
				t.Setenv("GROVE_TEST_PRS", `[{}]`)
			case "truncated PR list":
				pr := `{"headRefName":"open-pr","number":7}`
				t.Setenv("GROVE_TEST_PRS", "["+strings.Repeat(pr+",", 9999)+pr+"]")
			}
			pruneDryRun, pruneJSON, pruneYes = false, false, true
			_, err := captureStdout(t, func() error { return runPrune(pruneCmd, nil) })
			if err == nil || !strings.Contains(err.Error(), "stale") {
				t.Fatalf("expected stale check error, got %v", err)
			}
			if scenario == "gh failure" && !strings.Contains(err.Error(), "gh auth login") {
				t.Fatalf("gh error must include authentication guidance from stderr: %v", err)
			}
			if _, err := os.Stat(paths["stale"]); err != nil {
				t.Fatalf("failed verification removed worktree: %v", err)
			}
			s, err := state.Load(repo)
			if err != nil || len(s.Worktrees) != 1 {
				t.Fatalf("failed verification changed state: %+v, %v", s, err)
			}
		})
	}
}

func TestPruneStaleForceIncludesProtectedAndDeletesBranch(t *testing.T) {
	repo, paths := setupStalePrune(t, "protected")
	if err := state.Update(repo, func(s *state.State) error {
		entry := s.Worktrees["protected"]
		entry.Protected = true
		s.Worktrees["protected"] = entry
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths["protected"], "untracked.txt"), []byte("discard"), 0644); err != nil {
		t.Fatal(err)
	}
	pruneDryRun, pruneJSON = false, false
	pruneYes, pruneForce, pruneIncludeProtected, pruneDeleteBranch = true, true, true, true
	if _, err := captureStdout(t, func() error { return runPrune(pruneCmd, nil) }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths["protected"]); !os.IsNotExist(err) {
		t.Fatalf("explicit force and include-protected should remove worktree: %v", err)
	}
	if git.LocalBranchExists("protected") {
		t.Fatal("--force --delete-branch should delete unmerged branch after removal")
	}
	s, err := state.Load(repo)
	if err != nil || len(s.Worktrees) != 0 {
		t.Fatalf("removal should update state: %+v, %v", s, err)
	}
}
