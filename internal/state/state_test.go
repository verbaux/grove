package state

import (
	"reflect"
	"testing"
	"time"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	s := State{Worktrees: map[string]WorktreeEntry{}}
	s.Add("auth", "feature/auth", "/tmp/project-auth", 3001)

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
	if entry.Port != 3001 {
		t.Errorf("port = %d, want 3001", entry.Port)
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

	if err := s.Add("auth", "feature/auth", "/tmp/a", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("auth", "feature/other", "/tmp/b", 0); err == nil {
		t.Fatal("expected error when adding duplicate alias")
	}
}

func TestRemove(t *testing.T) {
	s := State{Worktrees: map[string]WorktreeEntry{}}
	s.Add("auth", "feature/auth", "/tmp/a", 0)

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

func TestSetProtectedPersists(t *testing.T) {
	dir := t.TempDir()
	s := State{Worktrees: map[string]WorktreeEntry{}}
	if err := s.Add("auth", "feature/auth", "/tmp/project-auth", 3001); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProtected("auth", true); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := loaded.Get("auth")
	if !ok {
		t.Fatal("expected auth entry")
	}
	if !entry.Protected {
		t.Fatal("expected auth to be protected")
	}
}

func TestAddWithConfigHashPersists(t *testing.T) {
	dir := t.TempDir()
	s := State{Worktrees: map[string]WorktreeEntry{}}
	if err := s.AddWithConfigHash("auth", "feature/auth", "/tmp/project-auth", 3001, "sha256:abc123"); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := loaded.Get("auth")
	if !ok {
		t.Fatal("expected auth entry")
	}
	if entry.ConfigHash != "sha256:abc123" {
		t.Fatalf("ConfigHash = %q, want sha256:abc123", entry.ConfigHash)
	}
}

func TestRenamePreservesEntryMetadata(t *testing.T) {
	created := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	original := WorktreeEntry{
		Branch:     "feature/auth",
		Path:       "/tmp/project-auth",
		Port:       3487,
		Protected:  true,
		ConfigHash: "sha256:abc123",
		Created:    created,
	}
	s := State{Worktrees: map[string]WorktreeEntry{"auth": original}}

	if err := s.Rename("auth", "login", "/tmp/project-login"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("auth"); ok {
		t.Fatal("old alias should be removed")
	}

	want := original
	want.Path = "/tmp/project-login"
	got, ok := s.Get("login")
	if !ok {
		t.Fatal("new alias should exist")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("renamed entry = %+v, want %+v", got, want)
	}
}

func TestRenameRejectsExistingAliasWithoutChangingState(t *testing.T) {
	auth := WorktreeEntry{Branch: "feature/auth", Path: "/tmp/project-auth"}
	payments := WorktreeEntry{Branch: "feature/payments", Path: "/tmp/project-payments"}
	s := State{Worktrees: map[string]WorktreeEntry{
		"auth":     auth,
		"payments": payments,
	}}

	if err := s.Rename("auth", "payments", "/tmp/project-payments"); err == nil {
		t.Fatal("expected duplicate alias error")
	}
	if got, _ := s.Get("auth"); !reflect.DeepEqual(got, auth) {
		t.Fatalf("source entry changed: %+v", got)
	}
	if got, _ := s.Get("payments"); !reflect.DeepEqual(got, payments) {
		t.Fatalf("destination entry changed: %+v", got)
	}
}
