package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func TestDetachExpandsGlobPatterns(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{"apps/*/node_modules"},
	})
	for _, name := range []string{"apps/web/node_modules", "apps/api/node_modules"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	createName, createFrom, createDetach, createJSON = "detach-globs", "", false, false
	t.Cleanup(func() { createName, detachCopy = "", false })
	if err := runCreate(createCmd, []string{"feature/detach-globs"}); err != nil {
		t.Fatal(err)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := s.Get("detach-globs")
	if !ok {
		t.Fatal("missing detach-globs state entry")
	}
	if err := os.Chdir(entry.Path); err != nil {
		t.Fatal(err)
	}
	detachCopy = true
	if err := runDetach(detachCmd, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"apps/web/node_modules", "apps/api/node_modules"} {
		info, err := os.Lstat(filepath.Join(entry.Path, name))
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s was not detached into a real directory", name)
		}
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	remove := exec.Command("git", "worktree", "remove", "--force", entry.Path)
	remove.Dir = dir
	if out, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("cleanup failed: %s", out)
	}
}
