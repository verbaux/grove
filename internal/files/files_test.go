package files

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindEnvFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a realistic project structure
	touch(t, dir, ".env")
	touch(t, dir, ".env.local")
	touch(t, dir, ".env.development")
	touch(t, dir, "README.md")
	touch(t, dir, "packages/api/.env")
	touch(t, dir, "node_modules/some-pkg/.env") // should be skipped
	touch(t, dir, ".git/config")                // should be skipped

	found, err := FindEnvFiles(dir)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		".env":              true,
		".env.local":        true,
		".env.development":  true,
		"packages/api/.env": true,
	}

	if len(found) != len(want) {
		t.Errorf("found %d files, want %d: %v", len(found), len(want), found)
	}

	for _, f := range found {
		// Normalize to forward slashes for comparison on any OS
		f = filepath.ToSlash(f)
		if !want[f] {
			t.Errorf("unexpected file: %q", f)
		}
	}
}

func TestCopyEnvFiles(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.WriteFile(filepath.Join(src, ".env"), []byte("PORT=3000\n"), 0644)
	touch(t, src, ".env.local")
	touch(t, src, "packages/api/.env")

	copied, err := CopyEnvFiles(src, dst)
	if err != nil {
		t.Fatal("CopyEnvFiles failed:", err)
	}

	if len(copied) != 3 {
		t.Fatalf("expected 3 copied, got %d", len(copied))
	}

	// Verify content was actually copied, not just an empty file
	data, err := os.ReadFile(filepath.Join(dst, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "PORT=3000\n" {
		t.Errorf(".env content = %q, want %q", string(data), "PORT=3000\n")
	}
}

func TestCopyMissingEnvFilesDoesNotOverwrite(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("source\n"), 0644); err != nil {
		t.Fatal(err)
	}
	touch(t, src, "packages/api/.env")
	if err := os.WriteFile(filepath.Join(dst, ".env"), []byte("local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	copied, err := CopyMissingEnvFiles(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) != 1 || filepath.ToSlash(copied[0]) != "packages/api/.env" {
		t.Fatalf("copied = %v, want only packages/api/.env", copied)
	}

	data, err := os.ReadFile(filepath.Join(dst, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "local\n" {
		t.Fatalf(".env was overwritten: %q", string(data))
	}
}

func TestSymlink(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.MkdirAll(filepath.Join(src, "node_modules"), 0755)

	created, err := Symlink(src, dst, "node_modules")
	if err != nil {
		t.Fatal("Symlink failed:", err)
	}
	if !created {
		t.Error("expected created=true")
	}

	info, err := os.Lstat(filepath.Join(dst, "node_modules"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected a symlink")
	}
}

func TestSymlinkIdempotent(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.MkdirAll(filepath.Join(src, "node_modules"), 0755)

	Symlink(src, dst, "node_modules")

	created, err := Symlink(src, dst, "node_modules")
	if err != nil {
		t.Fatal("second Symlink call should not error:", err)
	}
	if created {
		t.Error("expected created=false on second call")
	}
}

func TestSymlinkSkipsMissingSrc(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	created, err := Symlink(src, dst, "node_modules")
	if err != nil {
		t.Fatal("Symlink should not error for missing src:", err)
	}
	if created {
		t.Error("expected created=false when src missing")
	}
}

func TestSymlinkConflict(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	os.MkdirAll(filepath.Join(src, "node_modules"), 0755)
	// Create a real directory at dst — not a symlink
	os.MkdirAll(filepath.Join(dst, "node_modules"), 0755)

	_, err := Symlink(src, dst, "node_modules")
	if err == nil {
		t.Fatal("expected error when dst exists as a real directory")
	}
	if !errors.Is(err, ErrSymlinkDestinationConflict) {
		t.Fatalf("expected ErrSymlinkDestinationConflict, got %v", err)
	}
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copied")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(src, "sub"), 0755)
	os.WriteFile(filepath.Join(src, "sub", "nested.txt"), []byte("world"), 0644)

	copied, err := CopyDir(src, dst)
	if err != nil {
		t.Fatal("CopyDir failed:", err)
	}
	if !copied {
		t.Error("expected copied=true")
	}

	data, err := os.ReadFile(filepath.Join(dst, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("file.txt = %q, want %q", string(data), "hello")
	}

	data, err = os.ReadFile(filepath.Join(dst, "sub", "nested.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "world" {
		t.Errorf("sub/nested.txt = %q, want %q", string(data), "world")
	}
}

func TestCopyDirSkipsMissingSrc(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "copied")

	copied, err := CopyDir("/nonexistent/path", dst)
	if err != nil {
		t.Fatal("CopyDir should not error for missing src:", err)
	}
	if copied {
		t.Error("expected copied=false when src missing")
	}
}

func TestCopyDirIsIndependent(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copied")

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("original"), 0644)

	CopyDir(src, dst)

	os.WriteFile(filepath.Join(src, "file.txt"), []byte("modified"), 0644)

	data, _ := os.ReadFile(filepath.Join(dst, "file.txt"))
	if string(data) != "original" {
		t.Error("copy should be independent of source")
	}
}

// touch creates a file (and any needed parent dirs) with empty content.
func touch(t *testing.T, dir string, rel string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
}
