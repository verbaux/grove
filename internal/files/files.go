package files

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrSymlinkDestinationConflict indicates that symlink destination already exists
// and is not a symlink.
var ErrSymlinkDestinationConflict = errors.New("symlink destination already exists and is not a symlink")

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
	src := filepath.Join(srcDir, name)
	dst := filepath.Join(dstDir, name)

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

	err = filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// copyFile copies a single file from src to dst, creating parent directories as needed.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func isEnvFile(name string) bool {
	return strings.HasPrefix(name, ".env")
}
