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

// SchemaURL is the canonical location of the JSON schema for .groverc.json.
// grove init writes it as the "$schema" field so editors offer validation
// and autocomplete.
const SchemaURL = "https://raw.githubusercontent.com/verbaux/grove/main/groverc.schema.json"

// PortRange defines the allowed port allocation range.
type PortRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// AfterCreate is a list of shell commands run after worktree setup.
// JSON accepts either a single string (legacy) or an array.
type AfterCreate []string

func (a *AfterCreate) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*a = nil
		} else {
			*a = AfterCreate{s}
		}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return errors.New("afterCreate must be a string or array of strings")
	}
	*a = AfterCreate(arr)
	return nil
}

func (a AfterCreate) MarshalJSON() ([]byte, error) {
	if len(a) == 0 {
		return []byte(`""`), nil
	}
	if len(a) == 1 {
		return json.Marshal(a[0])
	}
	return json.Marshal([]string(a))
}

// Config maps directly to .groverc.json.
type Config struct {
	Schema              string      `json:"$schema,omitempty"`
	WorktreeDir         string      `json:"worktreeDir"`
	Prefix              string      `json:"prefix"`
	Symlink             []string    `json:"symlink"`
	CopyDirs            []string    `json:"copyDirs,omitempty"`
	AfterCreate         AfterCreate `json:"afterCreate"`
	AfterDetachedCreate AfterCreate `json:"afterDetachedCreate,omitempty"`
	PortRange           *PortRange  `json:"portRange,omitempty"`
	Editor              string      `json:"editor,omitempty"`
}

// DefaultPortMin, DefaultPortMax — fallback range when cfg.PortRange is nil.
const (
	DefaultPortMin = 3001
	DefaultPortMax = 3999
)

// ResolvedPortRange returns the configured range or defaults.
func (c Config) ResolvedPortRange() (int, int) {
	if c.PortRange != nil && c.PortRange.Min > 0 && c.PortRange.Max >= c.PortRange.Min {
		return c.PortRange.Min, c.PortRange.Max
	}
	return DefaultPortMin, DefaultPortMax
}

// Default returns a config with sensible defaults.
// Prefix is empty here — grove init will set it to the current folder name.
func Default() Config {
	return Config{
		WorktreeDir: "../",
		Prefix:      "",
		Symlink:     []string{"node_modules"},
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
