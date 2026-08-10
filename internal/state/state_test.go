package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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

func TestSetDetachedPersists(t *testing.T) {
	dir := t.TempDir()
	s := State{Worktrees: map[string]WorktreeEntry{}}
	if err := s.Add("auth", "feature/auth", "/tmp/project-auth", 3001); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDetached("auth", true); err != nil {
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
	if !ok || !entry.Detached {
		t.Fatalf("loaded detached entry = %+v", entry)
	}
}

func TestSetNotePersistsAndClears(t *testing.T) {
	dir := t.TempDir()
	s := State{Worktrees: map[string]WorktreeEntry{}}
	if err := s.Add("auth", "feature/auth", "/tmp/project-auth", 3001); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNote("auth", "waiting for API review"); err != nil {
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
	if !ok || entry.Note != "waiting for API review" {
		t.Fatalf("loaded note = %q, want persisted note", entry.Note)
	}
	if err := loaded.SetNote("auth", ""); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, loaded); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, stateDir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"note"`) {
		t.Fatalf("cleared note should be omitted from state JSON: %s", raw)
	}
}

func TestAddWithConfigHashPersists(t *testing.T) {
	dir := t.TempDir()
	s := State{Worktrees: map[string]WorktreeEntry{}}
	if err := s.AddWithConfigHash("auth", "feature/auth", "/tmp/project-auth", 3001, "sha256:abc123", true); err != nil {
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
	if !entry.Detached {
		t.Fatal("detached setup mode was not persisted")
	}
}

func TestRenamePreservesEntryMetadata(t *testing.T) {
	created := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	original := WorktreeEntry{
		Branch:     "feature/auth",
		Path:       "/tmp/project-auth",
		Port:       3487,
		Protected:  true,
		Detached:   true,
		Note:       "keep until launch",
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

func TestUpdateSerializesConcurrentWriters(t *testing.T) {
	dir := t.TempDir()

	var active atomic.Int32
	var maxActive atomic.Int32
	const writers = 8

	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs <- Update(dir, func(s *State) error {
				current := active.Add(1)
				for {
					maximum := maxActive.Load()
					if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
						break
					}
				}
				defer active.Add(-1)

				time.Sleep(10 * time.Millisecond)
				alias := fmt.Sprintf("writer-%d", i)
				return s.Add(alias, "feature/"+alias, "/tmp/"+alias, 3001+i)
			})
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}
	}
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("concurrent callbacks = %d, want 1", got)
	}

	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(s.Worktrees); got != writers {
		t.Fatalf("worktrees = %d, want %d", got, writers)
	}
}

func TestUpdateDoesNotSaveWhenCallbackFails(t *testing.T) {
	dir := t.TempDir()
	original := State{Worktrees: map[string]WorktreeEntry{
		"auth": {Branch: "feature/auth", Path: "/tmp/auth", Port: 3001},
	}}
	if err := Save(dir, original); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("mutation failed")
	err := Update(dir, func(s *State) error {
		delete(s.Worktrees, "auth")
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Update error = %v, want %v", err, wantErr)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, original) {
		t.Fatalf("state changed after callback failure: %+v", loaded)
	}
}

func TestUpdateTimesOutWhenStateLockIsHeld(t *testing.T) {
	dir := t.TempDir()
	lockDir := filepath.Join(dir, stateDir)
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		t.Fatal(err)
	}

	lock, err := acquireLock(filepath.Join(lockDir, lockFileName), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()

	err = updateWithTimeout(dir, 25*time.Millisecond, func(*State) error {
		t.Fatal("callback must not run without the lock")
		return nil
	})
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("Update error = %v, want ErrLockTimeout", err)
	}
}
