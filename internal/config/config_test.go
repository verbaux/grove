package config

import (
	"os"
	"path/filepath"
	"testing"
)

// In Go, tests live next to the code they test (same package).
// Every test function starts with Test and takes *testing.T.
// t.Fatal = stop this test immediately (like throwing in JS).
// t.Errorf = log failure but keep going (like console.error + a flag).

func TestSaveAndLoad(t *testing.T) {
	// t.TempDir() creates a temp directory that's auto-cleaned after the test.
	// Like creating a temp folder in beforeEach and deleting in afterEach.
	dir := t.TempDir()

	want := Config{
		WorktreeDir: "../",
		Prefix:      "myproject",
		Symlink:     []string{"node_modules", ".yarn"},
		AfterCreate: "make setup",
	}

	if err := Save(dir, want); err != nil {
		t.Fatal("Save failed:", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatal("Load failed:", err)
	}

	// Go doesn't have deep equality built into == for slices,
	// so we compare field by field.
	if got.WorktreeDir != want.WorktreeDir {
		t.Errorf("WorktreeDir = %q, want %q", got.WorktreeDir, want.WorktreeDir)
	}
	if got.Prefix != want.Prefix {
		t.Errorf("Prefix = %q, want %q", got.Prefix, want.Prefix)
	}
	if got.AfterCreate != want.AfterCreate {
		t.Errorf("AfterCreate = %q, want %q", got.AfterCreate, want.AfterCreate)
	}
	if len(got.Symlink) != len(want.Symlink) {
		t.Fatalf("Symlink length = %d, want %d", len(got.Symlink), len(want.Symlink))
	}
	for i := range want.Symlink {
		if got.Symlink[i] != want.Symlink[i] {
			t.Errorf("Symlink[%d] = %q, want %q", i, got.Symlink[i], want.Symlink[i])
		}
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error when .groverc.json is missing")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	os.WriteFile(path, []byte("{bad json}"), 0644)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
