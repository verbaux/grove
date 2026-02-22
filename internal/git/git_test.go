package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupTestRepo creates a real git repo in a temp directory with one commit.
// Tests need a real repo because we're testing actual git commands.
func setupTestRepo(t *testing.T) string {
	t.Helper() // marks this as a helper — errors point to the calling test, not here

	// filepath.EvalSymlinks resolves macOS /tmp → /private/var/folders/... difference.
	// Without this, git returns the real path but t.TempDir() returns the symlinked one.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %s", args, string(out))
		}
	}

	// git commands in tests need to run from the repo directory.
	// t.Cleanup restores the original cwd after the test — like afterEach in Jest.
	// Without this, Chdir would affect all subsequent tests in the package.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestListWorktrees(t *testing.T) {
	dir := setupTestRepo(t)

	worktrees, err := ListWorktrees()
	if err != nil {
		t.Fatal("ListWorktrees failed:", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(worktrees))
	}
	if worktrees[0].Path != dir {
		t.Errorf("path = %q, want %q", worktrees[0].Path, dir)
	}
	if !worktrees[0].IsMain {
		t.Error("expected first worktree to be main")
	}
}

func TestAddAndRemoveWorktree(t *testing.T) {
	setupTestRepo(t)

	wtPath := filepath.Join(t.TempDir(), "test-worktree")

	if err := AddWorktree(wtPath, "test-branch", ""); err != nil {
		t.Fatal("AddWorktree failed:", err)
	}

	worktrees, err := ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(worktrees))
	}
	if worktrees[1].Branch != "test-branch" {
		t.Errorf("branch = %q, want %q", worktrees[1].Branch, "test-branch")
	}

	if err := RemoveWorktree(wtPath, false); err != nil {
		t.Fatal("RemoveWorktree failed:", err)
	}

	worktrees, err = ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("expected 1 worktree after remove, got %d", len(worktrees))
	}
}

func TestStatusClean(t *testing.T) {
	dir := setupTestRepo(t)

	status, err := Status(dir)
	if err != nil {
		t.Fatal("Status failed:", err)
	}
	if status != "clean" {
		t.Errorf("status = %q, want %q", status, "clean")
	}
}

// gitIn runs a git command with its working directory set to dir.
// Fails the test immediately if the command errors — use this for test setup, not assertions.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %s", args, out)
	}
}

func TestStatusModified(t *testing.T) {
	dir := setupTestRepo(t)

	// Create, commit, then modify a tracked file.
	file := filepath.Join(dir, "tracked.txt")
	if err := os.WriteFile(file, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "tracked.txt")
	gitIn(t, dir, "commit", "-m", "add tracked")
	if err := os.WriteFile(file, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err := Status(dir)
	if err != nil {
		t.Fatal("Status failed:", err)
	}
	if status != "1 modified" {
		t.Errorf("status = %q, want %q", status, "1 modified")
	}
}

func TestStatusUntracked(t *testing.T) {
	dir := setupTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err := Status(dir)
	if err != nil {
		t.Fatal("Status failed:", err)
	}
	if status != "1 untracked" {
		t.Errorf("status = %q, want %q", status, "1 untracked")
	}
}

func TestStatusStaged(t *testing.T) {
	dir := setupTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "staged.txt")

	status, err := Status(dir)
	if err != nil {
		t.Fatal("Status failed:", err)
	}
	if status != "1 staged" {
		t.Errorf("status = %q, want %q", status, "1 staged")
	}
}
