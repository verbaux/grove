package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const FileName = ".groverc.json"

const schemaURLFmt = "https://raw.githubusercontent.com/verbaux/grove/%s/groverc.schema.json"

var releaseTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// SchemaURL returns the JSON schema location for .groverc.json, written by
// grove init as the "$schema" field so editors offer validation and
// autocomplete. Release builds (vX.Y.Z) pin to that tag so the schema matches
// the binary; dev builds fall back to main.
func SchemaURL(version string) string {
	ref := "main"
	if releaseTag.MatchString(version) {
		ref = version
	}
	return fmt.Sprintf(schemaURLFmt, ref)
}

// PortRange defines the allowed port allocation range.
type PortRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// HookCommands is a list of shell commands run by lifecycle hooks.
// JSON accepts either a single string or an array.
type HookCommands []string

func (h *HookCommands) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*h = nil
		} else {
			*h = HookCommands{s}
		}
		return nil
	}
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return errors.New("hook command must be a string or array of strings")
	}
	*h = HookCommands(arr)
	return nil
}

func (h HookCommands) MarshalJSON() ([]byte, error) {
	if len(h) == 0 {
		return []byte(`""`), nil
	}
	if len(h) == 1 {
		return json.Marshal(h[0])
	}
	return json.Marshal([]string(h))
}

// Config maps directly to .groverc.json.
type Config struct {
	Schema              string       `json:"$schema,omitempty"`
	WorktreeDir         string       `json:"worktreeDir"`
	Prefix              string       `json:"prefix"`
	Symlink             []string     `json:"symlink"`
	CopyDirs            []string     `json:"copyDirs,omitempty"`
	AfterCreate         HookCommands `json:"afterCreate"`
	AfterDetachedCreate HookCommands `json:"afterDetachedCreate,omitempty"`
	BeforeRemove        HookCommands `json:"beforeRemove,omitempty"`
	AfterRemove         HookCommands `json:"afterRemove,omitempty"`
	PortRange           *PortRange   `json:"portRange,omitempty"`
	Editor              string       `json:"editor,omitempty"`
}

type setupFingerprint struct {
	WorktreeDir         string   `json:"worktreeDir"`
	Prefix              string   `json:"prefix"`
	Symlink             []string `json:"symlink"`
	CopyDirs            []string `json:"copyDirs"`
	AfterCreate         []string `json:"afterCreate"`
	AfterDetachedCreate []string `json:"afterDetachedCreate"`
	BeforeRemove        []string `json:"beforeRemove"`
	AfterRemove         []string `json:"afterRemove"`
	PortMin             int      `json:"portMin"`
	PortMax             int      `json:"portMax"`
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

// SetupHash returns a stable hash of the config fields that affect worktree
// setup. Non-setup metadata like $schema and editor is intentionally ignored.
func SetupHash(c Config) (string, error) {
	minPort, maxPort := c.ResolvedPortRange()
	fp := setupFingerprint{
		WorktreeDir:         c.WorktreeDir,
		Prefix:              c.Prefix,
		Symlink:             copyStringSlice(c.Symlink),
		CopyDirs:            copyStringSlice(c.CopyDirs),
		AfterCreate:         copyStringSlice([]string(c.AfterCreate)),
		AfterDetachedCreate: copyStringSlice([]string(c.AfterDetachedCreate)),
		BeforeRemove:        copyStringSlice([]string(c.BeforeRemove)),
		AfterRemove:         copyStringSlice([]string(c.AfterRemove)),
		PortMin:             minPort,
		PortMax:             maxPort,
	}
	data, err := json.Marshal(fp)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func copyStringSlice(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
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
