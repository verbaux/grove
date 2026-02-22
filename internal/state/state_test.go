package state

import (
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	s := State{Worktrees: map[string]WorktreeEntry{}}
	s.Add("auth", "feature/auth", "/tmp/project-auth")

	if err := Save(dir, s); err != nil {
		t.Fatal("Save failed:", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal("Load failed:", err)
	}

	entry, ok := loaded.Get("auth")
	if !ok {
		t.Fatal("expected 'auth' alias to exist")
	}
	if entry.Branch != "feature/auth" {
		t.Errorf("branch = %q, want %q", entry.Branch, "feature/auth")
	}
	if entry.Path != "/tmp/project-auth" {
		t.Errorf("path = %q, want %q", entry.Path, "/tmp/project-auth")
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()

	s, err := Load(dir)
	if err != nil {
		t.Fatal("Load should not error on missing file:", err)
	}
	if len(s.Worktrees) != 0 {
		t.Errorf("expected empty worktrees, got %d", len(s.Worktrees))
	}
}

func TestAddDuplicateAlias(t *testing.T) {
	s := State{Worktrees: map[string]WorktreeEntry{}}

	if err := s.Add("auth", "feature/auth", "/tmp/a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("auth", "feature/other", "/tmp/b"); err == nil {
		t.Fatal("expected error when adding duplicate alias")
	}
}

func TestRemove(t *testing.T) {
	s := State{Worktrees: map[string]WorktreeEntry{}}
	s.Add("auth", "feature/auth", "/tmp/a")

	if err := s.Remove("auth"); err != nil {
		t.Fatal("Remove failed:", err)
	}
	if s.AliasExists("auth") {
		t.Error("alias should not exist after remove")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	s := State{Worktrees: map[string]WorktreeEntry{}}

	if err := s.Remove("nope"); err == nil {
		t.Fatal("expected error when removing nonexistent alias")
	}
}
