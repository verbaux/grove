package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func TestRenameMovesStandardWorktreeAndPreservesMetadata(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "", "", false, false
	if err := runCreate(createCmd, []string{"feature/auth"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	beforeState, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	before, ok := beforeState.Get("auth")
	if !ok {
		t.Fatal("expected auth state entry")
	}
	oldPath := before.Path
	newPath := filepath.Join(filepath.Dir(root), "testproject-login")
	t.Cleanup(func() {
		_ = git.RemoveWorktree(oldPath, true)
		_ = git.RemoveWorktree(newPath, true)
	})

	renameJSON = false
	if err := runRename(renameCmd, []string{"auth", "login"}); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path should be gone, stat error = %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new path should exist: %v", err)
	}

	afterState, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := afterState.Get("auth"); ok {
		t.Fatal("old alias should be removed from state")
	}
	after, ok := afterState.Get("login")
	if !ok {
		t.Fatal("new alias should be present in state")
	}
	want := before
	want.Path = newPath
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("renamed entry = %+v, want %+v", after, want)
	}
}

func TestRenameRollsBackMoveWhenStateSaveFails(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "", "", false, false
	if err := runCreate(createCmd, []string{"feature/rollback"}); err != nil {
		t.Fatal(err)
	}
	s, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.Get("rollback")
	oldPath := before.Path
	newPath := filepath.Join(filepath.Dir(root), "testproject-restored")
	t.Cleanup(func() {
		_ = git.RemoveWorktree(oldPath, true)
		_ = git.RemoveWorktree(newPath, true)
	})

	originalSave := renameSaveState
	renameSaveState = func(string, state.State) error { return errors.New("forced save failure") }
	t.Cleanup(func() { renameSaveState = originalSave })

	err = runRename(renameCmd, []string{"rollback", "restored"})
	if err == nil || !strings.Contains(err.Error(), "forced save failure") {
		t.Fatalf("rename error = %v, want forced save failure", err)
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old path should be restored: %v", err)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatalf("new path should be removed by rollback, stat error = %v", err)
	}

	onDisk, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := onDisk.Get("rollback"); !ok {
		t.Fatal("old alias should remain on disk")
	}
	if _, ok := onDisk.Get("restored"); ok {
		t.Fatal("new alias should not be saved")
	}
}

func TestRenameJSONReportsMove(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "", "", false, false
	if err := runCreate(createCmd, []string{"feature/json-old"}); err != nil {
		t.Fatal(err)
	}
	s, _ := state.Load(root)
	before, _ := s.Get("json-old")
	newPath := filepath.Join(filepath.Dir(root), "testproject-json-new")
	t.Cleanup(func() {
		_ = git.RemoveWorktree(before.Path, true)
		_ = git.RemoveWorktree(newPath, true)
	})

	renameJSON = true
	t.Cleanup(func() { renameJSON = false })
	out, err := captureStdout(t, func() error {
		return runRename(renameCmd, []string{"json-old", "json-new"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var result renameResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if result.OldAlias != "json-old" || result.Alias != "json-new" || !result.Moved {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.OldPath != before.Path || result.Path != newPath || result.Port != before.Port {
		t.Fatalf("unexpected paths or port: %+v", result)
	}
}

func TestRenameRejectsMainWorktree(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())

	err := runRename(renameCmd, []string{"1", "renamed-main"})
	if err == nil || !strings.Contains(err.Error(), "main worktree") {
		t.Fatalf("rename error = %v, want main worktree refusal", err)
	}
	if _, statErr := os.Stat(root); statErr != nil {
		t.Fatalf("main worktree should remain: %v", statErr)
	}
}

func TestRenameRejectsOrphanWorktree(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	orphanPath := filepath.Join(filepath.Dir(root), "orphan-worktree")
	if err := git.AddWorktree(orphanPath, "feature/orphan", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = git.RemoveWorktree(orphanPath, true) })

	err := runRename(renameCmd, []string{"feature/orphan", "managed"})
	if err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("rename error = %v, want unmanaged refusal", err)
	}
	if _, statErr := os.Stat(orphanPath); statErr != nil {
		t.Fatalf("orphan path should remain: %v", statErr)
	}
}

func TestRenameRejectsExistingAlias(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "", "", false, false
	if err := runCreate(createCmd, []string{"feature/one"}); err != nil {
		t.Fatal(err)
	}
	if err := runCreate(createCmd, []string{"feature/two"}); err != nil {
		t.Fatal(err)
	}
	s, _ := state.Load(root)
	one, _ := s.Get("one")
	two, _ := s.Get("two")
	t.Cleanup(func() {
		_ = git.RemoveWorktree(one.Path, true)
		_ = git.RemoveWorktree(two.Path, true)
	})

	err := runRename(renameCmd, []string{"one", "two"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("rename error = %v, want duplicate alias error", err)
	}
	after, _ := state.Load(root)
	if got, _ := after.Get("one"); !reflect.DeepEqual(got, one) {
		t.Fatalf("source changed: %+v", got)
	}
	if got, _ := after.Get("two"); !reflect.DeepEqual(got, two) {
		t.Fatalf("destination changed: %+v", got)
	}
}

func TestRenameRejectsNumericAlias(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "", "", false, false
	if err := runCreate(createCmd, []string{"feature/named"}); err != nil {
		t.Fatal(err)
	}
	s, _ := state.Load(root)
	entry, _ := s.Get("named")
	t.Cleanup(func() { _ = git.RemoveWorktree(entry.Path, true) })

	err := runRename(renameCmd, []string{"named", "42"})
	if err == nil || !strings.Contains(err.Error(), "numeric-only") {
		t.Fatalf("rename error = %v, want numeric alias error", err)
	}
	after, _ := state.Load(root)
	if got, _ := after.Get("named"); !reflect.DeepEqual(got, entry) {
		t.Fatalf("entry changed: %+v", got)
	}
}

func TestRenameKeepsCustomWorktreePath(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	customPath := filepath.Join(filepath.Dir(root), "hand-placed-worktree")
	if err := git.AddWorktree(customPath, "feature/custom", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = git.RemoveWorktree(customPath, true) })

	s, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("custom", "feature/custom", customPath, 3555); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(root, s); err != nil {
		t.Fatal(err)
	}

	renameJSON = false
	if err := runRename(renameCmd, []string{"custom", "placed"}); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if _, err := os.Stat(customPath); err != nil {
		t.Fatalf("custom path should remain in place: %v", err)
	}
	afterState, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := afterState.Get("placed")
	if !ok {
		t.Fatal("new alias should be present in state")
	}
	if entry.Path != customPath {
		t.Fatalf("custom path changed to %q, want %q", entry.Path, customPath)
	}
	if entry.Port != 3555 {
		t.Fatalf("port changed to %d, want 3555", entry.Port)
	}
}
