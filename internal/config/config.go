package config

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const FileName = ".groverc.json"

// Config maps directly to .groverc.json.
type Config struct {
	WorktreeDir string   `json:"worktreeDir"`
	Prefix      string   `json:"prefix"`
	Symlink     []string `json:"symlink"`
	AfterCreate string   `json:"afterCreate"`
}

// Default returns a config with sensible defaults.
// Prefix is empty here — grove init will set it to the current folder name.
func Default() Config {
	return Config{
		WorktreeDir: "../",
		Prefix:      "",
		Symlink:     []string{"node_modules"},
		AfterCreate: "",
	}
}

// Load reads .groverc.json from dir.
func Load(dir string) (Config, error) {
	path := filepath.Join(dir, FileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, errors.New("no .groverc.json found — run 'grove init' first")
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, errors.New(".groverc.json is not valid JSON: " + err.Error())
	}

	return cfg, nil
}

// FindRoot walks up from dir until it finds a directory containing .groverc.json.
// Like how git finds .git — you can run grove commands from any subdirectory.
func FindRoot(dir string) (string, error) {
	current := dir
	for {
		if _, err := os.Stat(filepath.Join(current, FileName)); err == nil {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Fallback: if we're inside a git worktree, the main repo root
	// may be a sibling directory rather than a parent. Ask git for
	// the common .git dir and check if its parent has .groverc.json.
	if root, err := findRootViaGit(dir); err == nil {
		return root, nil
	}

	return "", errors.New("no .groverc.json found — run 'grove init' first")
}

func findRootViaGit(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	root := filepath.Dir(gitDir)

	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(filepath.Join(root, FileName)); err != nil {
		return "", err
	}
	return root, nil
}

// Save writes config to .groverc.json in dir.
// 0644 = owner can read/write, everyone else can read.
func Save(dir string, cfg Config) error {
	path := filepath.Join(dir, FileName)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')

	return os.WriteFile(path, data, 0644)
}
