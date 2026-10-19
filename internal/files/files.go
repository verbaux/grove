package files

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrSymlinkDestinationConflict indicates that symlink destination already exists
// and is not a symlink.
var ErrSymlinkDestinationConflict = errors.New("symlink destination already exists and is not a symlink")

// ErrUnsafeSymlinkPath indicates that a configured symlink path escapes the
// source or destination root.
var ErrUnsafeSymlinkPath = errors.New("symlink path escapes its configured root")

// SymlinkConflictError adds path context to ErrSymlinkDestinationConflict.
type SymlinkConflictError struct {
	Name string
}

func (e SymlinkConflictError) Error() string {
	return fmt.Sprintf("cannot symlink %s: destination already exists and is not a symlink", e.Name)
}

func (e SymlinkConflictError) Unwrap() error {
	return ErrSymlinkDestinationConflict
}

// skipDirs are directories we never recurse into when searching for .env files.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"dist":         true,
	".next":        true,
	"build":        true,
}

// FindEnvFiles walks srcDir recursively and returns all .env* file paths.
// Paths are relative to srcDir, so they can be replicated in the destination.
func FindEnvFiles(srcDir string) ([]string, error) {
	var found []string

	err := filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip blacklisted directories entirely — don't recurse into them.
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		// Other worktrees can live under the project when worktreeDir points
		// inside it; their .env files belong to them, not to the source.
		if d.IsDir() && path != srcDir && isLinkedWorktree(path) {
			return filepath.SkipDir
		}

		if !d.IsDir() && isEnvFile(d.Name()) {
			// Store relative path so we can recreate the same structure in the destination.
			rel, err := filepath.Rel(srcDir, path)
			if err != nil {
				return err
			}
			found = append(found, rel)
		}

		return nil
	})

	return found, err
}

// CopyEnvFiles copies all .env* files from srcDir to dstDir,
// preserving the directory structure.
func CopyEnvFiles(srcDir, dstDir string) ([]string, error) {
	files, err := FindEnvFiles(srcDir)
	if err != nil {
		return nil, err
	}

	var copied []string
	for _, rel := range files {
		src := filepath.Join(srcDir, rel)
		dst := filepath.Join(dstDir, rel)

		if err := copyFile(src, dst); err != nil {
			return copied, err
		}
		copied = append(copied, rel)
	}

	return copied, nil
}

// CopyMissingEnvFiles copies .env* files that do not already exist in dstDir.
// Existing destination files are left untouched.
func CopyMissingEnvFiles(srcDir, dstDir string) ([]string, error) {
	files, err := FindEnvFiles(srcDir)
	if err != nil {
		return nil, err
	}

	var copied []string
	for _, rel := range files {
		dst := filepath.Join(dstDir, rel)
		if _, err := os.Stat(dst); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return copied, err
		}

		src := filepath.Join(srcDir, rel)
		if err := copyFile(src, dst); err != nil {
			return copied, err
		}
		copied = append(copied, rel)
	}

	return copied, nil
}

// Symlink creates a symlink at dstDir/name pointing to srcDir/name.
// Returns (true, nil) if the symlink was created.
// Returns (false, nil) if src doesn't exist — caller can decide whether to warn.
// Returns (false, err) if dst already exists but is not a symlink (conflict).
func Symlink(srcDir, dstDir, name string) (bool, error) {
	src := filepath.Join(srcDir, name)
	dst := filepath.Join(dstDir, name)

	if info, err := os.Lstat(dst); err == nil {
		// dst exists — only ok if it's already a symlink (idempotent)
		if info.Mode()&os.ModeSymlink != 0 {
			return false, nil
		}
		return false, SymlinkConflictError{Name: name}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	// src doesn't exist — skip silently (e.g. node_modules not yet installed)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return false, err
	}
	return true, os.Symlink(src, dst)
}

// RepairBrokenSymlink replaces an existing broken symlink with one pointing
// to srcDir/name. Healthy symlinks and missing destinations are left alone.
// A real destination is never overwritten, and a missing canonical source
// leaves the broken link unchanged.
func RepairBrokenSymlink(srcDir, dstDir, name string) (bool, error) {
	src, err := joinWithinRoot(srcDir, name)
	if err != nil {
		return false, err
	}
	dst, err := joinWithinRoot(dstDir, name)
	if err != nil {
		return false, err
	}

	info, err := os.Lstat(dst)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, SymlinkConflictError{Name: name}
	}
	if err := ensureParentWithinRoot(dstDir, dst); err != nil {
		return false, err
	}

	originalTarget, err := os.Readlink(dst)
	if err != nil {
		return false, err
	}
	resolvedTarget := originalTarget
	if !filepath.IsAbs(resolvedTarget) {
		resolvedTarget = filepath.Join(filepath.Dir(dst), resolvedTarget)
	}
	if _, err := os.Stat(resolvedTarget); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if _, err := os.Stat(src); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	// Recheck immediately before removal so a concurrent healthy replacement
	// or real destination is not overwritten.
	info, err = os.Lstat(dst)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, SymlinkConflictError{Name: name}
	}
	currentTarget, err := os.Readlink(dst)
	if err != nil {
		return false, err
	}
	currentResolved := currentTarget
	if !filepath.IsAbs(currentResolved) {
		currentResolved = filepath.Join(filepath.Dir(dst), currentResolved)
	}
	if _, err := os.Stat(currentResolved); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if err := os.Remove(dst); err != nil {
		return false, err
	}
	if err := os.Symlink(src, dst); err != nil {
		// The original link carried no reachable data, but restore it when the
		// destination is still free so a failed repair is non-destructive.
		if _, statErr := os.Lstat(dst); os.IsNotExist(statErr) {
			_ = os.Symlink(currentTarget, dst)
		}
		return false, err
	}
	return true, nil
}

func joinWithinRoot(root, name string) (string, error) {
	if name == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("%w: %q", ErrUnsafeSymlinkPath, name)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, name)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrUnsafeSymlinkPath, name)
	}
	return path, nil
}

func ensureParentWithinRoot(root, path string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedParent)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q", ErrUnsafeSymlinkPath, path)
	}
	return nil
}

// CopyDir recursively copies a directory from src to dst.
// Returns (true, nil) if copied, (false, nil) if src doesn't exist.
func CopyDir(src, dst string) (bool, error) {
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return true, copyFile(src, dst)
	}
	// WalkDir does not follow a symlinked root, so resolve it first.
	src, err = filepath.EvalSymlinks(src)
	if err != nil {
		return false, err
	}

	err = filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)

		switch {
		case d.Type()&fs.ModeSymlink != 0:
			// Recreate links as-is, like `cp -R`; relative links (node_modules/.bin,
			// pnpm stores) keep resolving inside the copy.
			return copySymlink(path, target)
		case d.IsDir():
			return os.MkdirAll(target, 0755)
		case !d.Type().IsRegular():
			// Sockets, FIFOs and devices cannot be copied meaningfully.
			return nil
		}
		return copyFile(path, target)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.Symlink(target, dst)
}

// copyFile copies a single file from src to dst, creating parent directories as needed.
// The destination gets the source permission bits, so secrets stay private and
// executables stay executable.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	perm := info.Mode().Perm()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	// Chmod covers umask and destinations that already existed.
	if err := out.Chmod(perm); err != nil {
		out.Close()
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// isLinkedWorktree reports whether dir is a git linked worktree: its .git is a
// file pointing into <git-common-dir>/worktrees/. Submodules also use a .git
// file, but theirs points into modules/.
func isLinkedWorktree(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return false
	}
	gitdir = filepath.ToSlash(strings.TrimSpace(gitdir))
	return strings.Contains(gitdir, "/worktrees/")
}

func isEnvFile(name string) bool {
	return strings.HasPrefix(name, ".env")
}
