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

func TestFindEnvFilesRootNamedLikeSkippedDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "build")
	touch(t, dir, ".env")
	touch(t, dir, "build/.env.ignored")

	found, err := FindEnvFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != ".env" {
		t.Errorf("expected [.env], got %v", found)
	}
}

func TestFindEnvFilesSkipsNestedWorktrees(t *testing.T) {
	dir := t.TempDir()

	touch(t, dir, ".env")
	// Linked worktree under the project (worktreeDir inside root).
	touch(t, dir, ".wt/feature/.env")
	if err := os.WriteFile(filepath.Join(dir, ".wt", "feature", ".git"), []byte("gitdir: /repo/.git/worktrees/feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Submodules also use a .git file but must still be searched.
	touch(t, dir, "libs/sub/.env")
	if err := os.WriteFile(filepath.Join(dir, "libs", "sub", ".git"), []byte("gitdir: ../../.git/modules/sub\n"), 0644); err != nil {
		t.Fatal(err)
	}

	found, err := FindEnvFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range found {
		got[filepath.ToSlash(f)] = true
	}
	if !got[".env"] || !got["libs/sub/.env"] {
		t.Errorf("missing expected env files, got %v", found)
	}
	if got[".wt/feature/.env"] {
		t.Errorf("env file from a nested worktree must be skipped, got %v", found)
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

func TestRepairBrokenSymlinkUsesCanonicalSource(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	canonical := filepath.Join(src, "node_modules")
	if err := os.Mkdir(canonical, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dst, "node_modules")
	if err := os.Symlink(filepath.Join(dst, "missing-target"), link); err != nil {
		t.Fatal(err)
	}

	repaired, err := RepairBrokenSymlink(src, dst, "node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if !repaired {
		t.Fatal("expected broken symlink to be repaired")
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if target != canonical {
		t.Fatalf("symlink target = %q, want %q", target, canonical)
	}
}

func TestRepairBrokenSymlinkLeavesHealthyLink(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	canonical := filepath.Join(src, "node_modules")
	if err := os.Mkdir(canonical, 0755); err != nil {
		t.Fatal(err)
	}
	otherTarget := t.TempDir()
	link := filepath.Join(dst, "node_modules")
	if err := os.Symlink(otherTarget, link); err != nil {
		t.Fatal(err)
	}

	repaired, err := RepairBrokenSymlink(src, dst, "node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if repaired {
		t.Fatal("healthy symlink must not be replaced")
	}
	target, _ := os.Readlink(link)
	if target != otherTarget {
		t.Fatalf("healthy symlink changed to %q", target)
	}
}

func TestRepairBrokenSymlinkLeavesLinkWhenCanonicalSourceMissing(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	originalTarget := filepath.Join(dst, "missing-target")
	link := filepath.Join(dst, "node_modules")
	if err := os.Symlink(originalTarget, link); err != nil {
		t.Fatal(err)
	}

	repaired, err := RepairBrokenSymlink(src, dst, "node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if repaired {
		t.Fatal("repair must be skipped without a canonical source")
	}
	target, _ := os.Readlink(link)
	if target != originalTarget {
		t.Fatalf("broken symlink changed to %q", target)
	}
}

func TestRepairBrokenSymlinkRefusesRealDestination(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	realDestination := filepath.Join(dst, "node_modules")
	if err := os.Mkdir(realDestination, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := RepairBrokenSymlink(src, dst, "node_modules")
	if !errors.Is(err, ErrSymlinkDestinationConflict) {
		t.Fatalf("RepairBrokenSymlink error = %v, want ErrSymlinkDestinationConflict", err)
	}
	if info, statErr := os.Stat(realDestination); statErr != nil || !info.IsDir() {
		t.Fatalf("real destination was changed: info=%v err=%v", info, statErr)
	}
}

func TestRepairBrokenSymlinkRejectsPathOutsideWorktree(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "main")
	dst := filepath.Join(base, "worktrees", "feature")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(base, "shared")
	if err := os.Mkdir(canonical, 0755); err != nil {
		t.Fatal(err)
	}
	escapedLink := filepath.Join(base, "worktrees", "shared")
	originalTarget := filepath.Join(base, "missing")
	if err := os.Symlink(originalTarget, escapedLink); err != nil {
		t.Fatal(err)
	}

	_, err := RepairBrokenSymlink(src, dst, "../shared")
	if !errors.Is(err, ErrUnsafeSymlinkPath) {
		t.Fatalf("RepairBrokenSymlink error = %v, want ErrUnsafeSymlinkPath", err)
	}
	target, readErr := os.Readlink(escapedLink)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if target != originalTarget {
		t.Fatalf("outside symlink changed to %q", target)
	}
}

func TestRepairBrokenSymlinkRejectsSymlinkedParentOutsideWorktree(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "main")
	dst := filepath.Join(base, "worktree")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(filepath.Join(src, "nested", "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dst, "nested")); err != nil {
		t.Fatal(err)
	}
	escapedLink := filepath.Join(outside, "node_modules")
	originalTarget := filepath.Join(base, "missing")
	if err := os.Symlink(originalTarget, escapedLink); err != nil {
		t.Fatal(err)
	}

	_, err := RepairBrokenSymlink(src, dst, filepath.Join("nested", "node_modules"))
	if !errors.Is(err, ErrUnsafeSymlinkPath) {
		t.Fatalf("RepairBrokenSymlink error = %v, want ErrUnsafeSymlinkPath", err)
	}
	target, readErr := os.Readlink(escapedLink)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if target != originalTarget {
		t.Fatalf("outside symlink changed to %q", target)
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

func TestCopyDirPreservesSymlinks(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "copied")

	touch(t, src, "pkg/bin/run")
	if err := os.Symlink("pkg", filepath.Join(src, "dirlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("pkg/bin/run", filepath.Join(src, "filelink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(src, "broken")); err != nil {
		t.Fatal(err)
	}

	if _, err := CopyDir(src, dst); err != nil {
		t.Fatal("CopyDir failed:", err)
	}

	for name, want := range map[string]string{"dirlink": "pkg", "filelink": "pkg/bin/run", "broken": "missing"} {
		got, err := os.Readlink(filepath.Join(dst, name))
		if err != nil {
			t.Fatalf("%s should be a symlink: %v", name, err)
		}
		if got != want {
			t.Errorf("%s → %q, want %q", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "dirlink", "bin", "run")); err != nil {
		t.Errorf("relative symlink should resolve inside the copy: %v", err)
	}
}

func TestCopyDirFollowsSymlinkedRoot(t *testing.T) {
	real := t.TempDir()
	touch(t, real, "file.txt")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copied")

	if _, err := CopyDir(link, dst); err != nil {
		t.Fatal("CopyDir failed:", err)
	}
	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("dst should be a real directory, got mode %v", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
		t.Error(err)
	}
}

func TestCopyPreservesFileMode(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(src, ".env"), 0600); err != nil {
		t.Fatal(err)
	}
	touch(t, src, "bin/run")
	if err := os.Chmod(filepath.Join(src, "bin", "run"), 0755); err != nil {
		t.Fatal(err)
	}

	envDst := t.TempDir()
	// A pre-existing destination must end up with the source mode too.
	if err := os.WriteFile(filepath.Join(envDst, ".env"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyEnvFiles(src, envDst); err != nil {
		t.Fatal(err)
	}
	dirDst := filepath.Join(t.TempDir(), "copied")
	if _, err := CopyDir(filepath.Join(src, "bin"), dirDst); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]os.FileMode{
		filepath.Join(envDst, ".env"): 0600,
		filepath.Join(dirDst, "run"):  0755,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %v, want %v", path, got, want)
		}
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
