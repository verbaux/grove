package cmd

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func TestNoteSetShowAndClear(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	worktreePath := t.TempDir()
	if err := state.Save(root, state.State{Worktrees: map[string]state.WorktreeEntry{
		"auth": {Branch: "feature/auth", Path: worktreePath},
	}}); err != nil {
		t.Fatal(err)
	}
	noteClear = false
	t.Cleanup(func() { noteClear = false })

	setOut, err := captureStdout(t, func() error {
		return runNote(noteCmd, []string{"auth", "waiting", "for API review"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(setOut, `Worktree "auth" note updated.`) {
		t.Fatalf("unexpected set output: %q", setOut)
	}
	loaded, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := loaded.Get("auth")
	if entry.Note != "waiting for API review" {
		t.Fatalf("note = %q, want trimmed joined text", entry.Note)
	}

	showOut, err := captureStdout(t, func() error {
		return runNote(noteCmd, []string{"auth"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if showOut != "waiting for API review\n" {
		t.Fatalf("show output = %q", showOut)
	}

	noteClear = true
	clearOut, err := captureStdout(t, func() error {
		return runNote(noteCmd, []string{"auth"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clearOut, `Worktree "auth" note cleared.`) {
		t.Fatalf("unexpected clear output: %q", clearOut)
	}
	loaded, err = state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ = loaded.Get("auth")
	if entry.Note != "" {
		t.Fatalf("cleared note = %q", entry.Note)
	}
}

func TestNoteValidation(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	if err := state.Save(root, state.State{Worktrees: map[string]state.WorktreeEntry{
		"auth": {Branch: "feature/auth", Path: t.TempDir()},
	}}); err != nil {
		t.Fatal(err)
	}
	noteClear = false
	t.Cleanup(func() { noteClear = false })

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "empty", text: "   ", want: "note cannot be empty"},
		{name: "multiline", text: "line one\nline two", want: "single line"},
		{name: "too long", text: strings.Repeat("é", 201), want: "at most 200 characters"},
		{name: "invalid UTF-8", text: string([]byte{0xff}), want: "valid UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "too long" && utf8.RuneCountInString(tt.text) != 201 {
				t.Fatal("test fixture must be 201 Unicode characters")
			}
			err := runNote(noteCmd, []string{"auth", tt.text})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runNote error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNoteRejectsMainAndOrphanWorktrees(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	noteClear = false
	t.Cleanup(func() { noteClear = false })

	if err := runNote(noteCmd, []string{"1", "main note"}); err == nil || !strings.Contains(err.Error(), "main worktree") {
		t.Fatalf("main note error = %v", err)
	}

	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	orphanPath := filepath.Join(parent, "orphan")
	if err := git.AddWorktree(orphanPath, "feature/note-orphan", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = git.RemoveWorktree(orphanPath, true) })
	if err := runNote(noteCmd, []string{orphanPath, "orphan note"}); err == nil || !strings.Contains(err.Error(), "adopt") {
		t.Fatalf("orphan note error = %v", err)
	}
	_ = root
}

func TestUpdateNoteRejectsReusedAlias(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{})
	original := &resolvedWorktree{
		Alias: "auth", Branch: "feature/auth", Path: "/tmp/auth", InState: true,
	}
	if err := state.Update(root, func(latest *state.State) error {
		return latest.Add("auth", "feature/replacement", "/tmp/replacement", 3001)
	}); err != nil {
		t.Fatal(err)
	}

	err := updateNote(root, original, "do not apply")
	if err == nil || !strings.Contains(err.Error(), "changed while updating note") {
		t.Fatalf("updateNote error = %v, want reused alias rejection", err)
	}
	loaded, loadErr := state.Load(root)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	entry, _ := loaded.Get("auth")
	if entry.Note != "" {
		t.Fatalf("replacement entry note changed to %q", entry.Note)
	}
}
