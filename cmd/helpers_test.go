package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func TestCompleteAliases(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
	})

	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"auth":     {Branch: "feature/auth", Path: filepath.Join(dir, "auth")},
		"payments": {Branch: "feature/payments", Path: filepath.Join(dir, "payments")},
	}}
	if err := state.Save(dir, s); err != nil {
		t.Fatal(err)
	}

	completions, directive := completeAliases(&cobra.Command{}, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(completions) != 2 {
		t.Fatalf("expected 2 completions, got %d: %v", len(completions), completions)
	}

	got := map[string]bool{}
	for _, c := range completions {
		got[c] = true
	}
	if !got["auth"] || !got["payments"] {
		t.Errorf("expected aliases [auth, payments], got %v", completions)
	}
}

func TestCompleteAliasesEmpty(t *testing.T) {
	setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
	})

	completions, directive := completeAliases(&cobra.Command{}, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(completions) != 0 {
		t.Errorf("expected 0 completions, got %v", completions)
	}
}

func TestCompleteOrphans(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
	})

	// Create a worktree directly with git (not grove) so it becomes an orphan.
	wtPath := filepath.Join(filepath.Dir(dir), "testproject-orphan")
	c := exec.Command("git", "worktree", "add", "-b", "feature/orphan", wtPath)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}
	t.Cleanup(func() {
		rm := exec.Command("git", "worktree", "remove", "--force", wtPath)
		rm.Dir = dir
		rm.CombinedOutput()
	})

	completions, directive := completeOrphans(&cobra.Command{}, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(completions) != 1 {
		t.Fatalf("expected 1 orphan completion, got %d: %v", len(completions), completions)
	}
	if completions[0] != "feature/orphan" {
		t.Errorf("expected orphan branch 'feature/orphan', got %q", completions[0])
	}
}

func TestCompleteOrphansIgnoresTracked(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
	})

	// Create a worktree via git, then register it in state (not an orphan).
	wtPath := filepath.Join(filepath.Dir(dir), "testproject-tracked")
	c := exec.Command("git", "worktree", "add", "-b", "feature/tracked", wtPath)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}
	t.Cleanup(func() {
		rm := exec.Command("git", "worktree", "remove", "--force", wtPath)
		rm.Dir = dir
		rm.CombinedOutput()
	})

	absWtPath, _ := filepath.EvalSymlinks(wtPath)
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"tracked": {Branch: "feature/tracked", Path: absWtPath},
	}}
	if err := state.Save(dir, s); err != nil {
		t.Fatal(err)
	}

	completions, _ := completeOrphans(&cobra.Command{}, nil, "")

	if len(completions) != 0 {
		t.Errorf("expected 0 orphan completions (worktree is tracked), got %v", completions)
	}
}

func TestCompleteAliasesNoRoot(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	completions, directive := completeAliases(&cobra.Command{}, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(completions) != 0 {
		t.Errorf("expected 0 completions when no grove root, got %v", completions)
	}
}
