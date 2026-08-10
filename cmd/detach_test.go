package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	s, err = state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok = s.Get("detach-globs")
	if !ok || !entry.Detached {
		t.Fatalf("detached state entry = %+v", entry)
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

func TestDetachRecordsModeWhenNoSymlinksRemain(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
	})
	createName, createFrom, createDetach, createJSON = "detach-migration", "", false, false
	t.Cleanup(func() { createName = "" })
	if err := runCreate(createCmd, []string{"feature/detach-migration"}); err != nil {
		t.Fatal(err)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := s.Get("detach-migration")
	if !ok {
		t.Fatal("missing detach-migration state entry")
	}
	t.Cleanup(func() {
		_ = os.Chdir(dir)
		cleanupWorktree(t, dir, entry.Path)
	})
	if err := os.Chdir(entry.Path); err != nil {
		t.Fatal(err)
	}
	if err := runDetach(detachCmd, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	s, err = state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok = s.Get("detach-migration")
	if !ok || !entry.Detached {
		t.Fatalf("migration did not record detached mode: %+v", entry)
	}
}

func TestDetachDoesNotRecordModeWhenAnySymlinkRemovalFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove files from a read-only directory")
	}
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{"node_modules", "packages/ui/node_modules"},
	})
	for _, name := range []string{"node_modules", "packages/ui/node_modules"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	createName, createFrom, createDetach, createJSON = "detach-failure", "", false, false
	t.Cleanup(func() { createName = "" })
	if err := runCreate(createCmd, []string{"feature/detach-failure"}); err != nil {
		t.Fatal(err)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := s.Get("detach-failure")
	if !ok {
		t.Fatal("missing detach-failure state entry")
	}
	t.Cleanup(func() {
		_ = os.Chdir(dir)
		cleanupWorktree(t, dir, entry.Path)
	})
	if err := os.Chdir(entry.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(entry.Path, 0555); err != nil {
		t.Fatal(err)
	}
	err = runDetach(detachCmd, nil)
	if chmodErr := os.Chmod(entry.Path, 0755); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	if err == nil {
		t.Fatal("expected symlink removal failure")
	}
	for _, want := range []string{"1 symlink removal(s) failed after 1 succeeded", "detached mode was not recorded", "grove sync detach-failure"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("detach error %q does not contain %q", err, want)
		}
	}
	if _, err := os.Lstat(filepath.Join(entry.Path, "node_modules")); err != nil {
		t.Fatalf("expected failed root symlink removal: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(entry.Path, "packages/ui/node_modules")); !os.IsNotExist(err) {
		t.Fatalf("expected nested symlink to be removed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	s, err = state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok = s.Get("detach-failure")
	if !ok || entry.Detached {
		t.Fatalf("failed detach changed state: %+v", entry)
	}
}
