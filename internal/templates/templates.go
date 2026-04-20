// Package templates stores and loads shared .groverc.json templates.
package templates

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/verbaux/grove/internal/config"
)

const ext = ".json"

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Dir returns the directory where templates are stored.
// Follows XDG Base Directory spec: $XDG_CONFIG_HOME/grove/templates,
// falling back to $HOME/.config/grove/templates.
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "grove", "templates"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "grove", "templates"), nil
}

func validateName(name string) error {
	if name == "" {
		return errors.New("template name cannot be empty")
	}
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid template name %q: only letters, digits, dash, underscore allowed", name)
	}
	return nil
}

func pathFor(name string) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+ext), nil
}

// Save writes cfg as a named template.
func Save(name string, cfg config.Config) error {
	p, err := pathFor(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(p, data, 0644)
}

// Load reads a named template.
func Load(name string) (config.Config, error) {
	p, err := pathFor(name)
	if err != nil {
		return config.Config{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.Config{}, fmt.Errorf("template %q not found", name)
		}
		return config.Config{}, err
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config.Config{}, fmt.Errorf("template %q: invalid JSON: %v", name, err)
	}
	return cfg, nil
}

// Exists reports whether a template with the given name exists.
func Exists(name string) bool {
	p, err := pathFor(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// List returns all template names, sorted.
func List() ([]string, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ext) {
			continue
		}
		names = append(names, strings.TrimSuffix(name, ext))
	}
	sort.Strings(names)
	return names, nil
}

// Delete removes a named template.
func Delete(name string) error {
	p, err := pathFor(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("template %q not found", name)
	}
	return os.Remove(p)
}
