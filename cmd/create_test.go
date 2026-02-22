package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

// setupIntegrationRepo creates a real git repo with a .groverc.json and
// changes cwd into it. Returns the repo directory and a cleanup function.
func setupIntegrationRepo(t *testing.T, cfg config.Config) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}

	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

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

func TestCreateRollbackOnAfterCreateFailure(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: "exit 1", // always fails
	})

	// Reset package-level flags so we get a clean state
	createName = ""
	createFrom = ""

	err := runCreate(createCmd, []string{"feature/rollback-test"})
	if err == nil {
		t.Fatal("expected runCreate to return error when afterCreate fails")
	}

	// The worktree directory must have been rolled back
	wtPath := filepath.Join(filepath.Dir(dir), "testproject-rollback-test")
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: worktree directory still exists at %s", wtPath)
	}

	// State must not have the alias
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.AliasExists("rollback-test") {
		t.Error("rollback failed: alias still present in state.json")
	}
}

func TestCreateRollbackOnStateSaveFailure(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: "",
	})

	createName = ""
	createFrom = ""

	// Make .grove directory a file so state.Save fails
	groveDir := filepath.Join(dir, ".grove")
	if err := os.WriteFile(groveDir, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	err := runCreate(createCmd, []string{"feature/state-fail"})
	if err == nil {
		t.Fatal("expected runCreate to return error when state save fails")
	}

	wtPath := filepath.Join(filepath.Dir(dir), "testproject-state-fail")
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: worktree directory still exists at %s", wtPath)
	}
}

func TestCreateSkipsSymlinkConflict(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{".yarn/cache"},
		AfterCreate: "",
	})

	// Make .yarn/cache tracked so it exists in the new worktree checkout.
	cacheFile := filepath.Join(dir, ".yarn", "cache", "pkg.txt")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte("cached"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", ".yarn/cache/pkg.txt"},
		{"git", "commit", "-m", "track yarn cache"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}

	createName = ""
	createFrom = ""

	if err := runCreate(createCmd, []string{"feature/symlink-conflict"}); err != nil {
		t.Fatalf("expected create to succeed on symlink conflict, got: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(dir), "testproject-symlink-conflict")
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("expected worktree to exist, got: %v", err)
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.AliasExists("symlink-conflict") {
		t.Fatal("expected alias to be present in state")
	}

	cacheInfo, err := os.Lstat(filepath.Join(wtPath, ".yarn", "cache"))
	if err != nil {
		t.Fatal(err)
	}
	if cacheInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected .yarn/cache in worktree to remain a real directory")
	}

	// Cleanup successful worktree to avoid leaking dirs across test runs.
	remove := exec.Command("git", "worktree", "remove", "--force", wtPath)
	remove.Dir = dir
	if out, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("cleanup failed: %s", out)
	}
}
