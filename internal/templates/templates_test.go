package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verbaux/grove/internal/config"
)

func TestDirRespectsXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "grove", "templates")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDirFallbackHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	got, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "grove", "templates")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSaveAndLoad(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	cfg := config.Config{
		WorktreeDir: "../",
		Prefix:      "nextjs",
		Symlink:     []string{"node_modules"},
		AfterCreate: config.HookCommands{"npm ci"},
	}
	if err := Save("nextjs", cfg); err != nil {
		t.Fatal(err)
	}

	got, err := Load("nextjs")
	if err != nil {
		t.Fatal(err)
	}
	if got.Prefix != "nextjs" {
		t.Errorf("prefix = %q", got.Prefix)
	}
	if len(got.AfterCreate) != 1 || got.AfterCreate[0] != "npm ci" {
		t.Errorf("afterCreate = %v", got.AfterCreate)
	}
}

func TestLoadMissing(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	_, err := Load("nope")
	if err == nil {
		t.Fatal("expected error for missing template")
	}
}

func TestList(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if err := Save("a", config.Default()); err != nil {
		t.Fatal(err)
	}
	if err := Save("b", config.Default()); err != nil {
		t.Fatal(err)
	}

	names, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("got %v", names)
	}
	if names[0] != "a" || names[1] != "b" {
		t.Errorf("expected [a b] sorted, got %v", names)
	}
}

func TestListEmpty(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	names, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
}

func TestDelete(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if err := Save("a", config.Default()); err != nil {
		t.Fatal(err)
	}
	if err := Delete("a"); err != nil {
		t.Fatal(err)
	}
	if Exists("a") {
		t.Error("template still exists after delete")
	}
}

func TestDeleteMissing(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if err := Delete("nope"); err == nil {
		t.Fatal("expected error deleting missing template")
	}
}

func TestExists(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if Exists("a") {
		t.Error("should not exist yet")
	}
	if err := Save("a", config.Default()); err != nil {
		t.Fatal(err)
	}
	if !Exists("a") {
		t.Error("should exist after save")
	}
}

func TestValidateName(t *testing.T) {
	cases := map[string]bool{
		"nextjs":     true,
		"go-service": true,
		"my_tpl":     true,
		"":           false,
		"with/slash": false,
		"..":         false,
		"with space": false,
	}
	for name, want := range cases {
		err := validateName(name)
		if (err == nil) != want {
			t.Errorf("validateName(%q): ok=%v, want %v", name, err == nil, want)
		}
	}
}

func TestPathNotInEscapesDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	// Ensure saves only land in templates dir
	if err := Save("legit", config.Default()); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(xdg, "grove", "templates"))
	if len(entries) != 1 {
		t.Errorf("expected 1 file, got %d", len(entries))
	}
}
