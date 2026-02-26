package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const stateDir = ".grove"
const fileName = "state.json"

// WorktreeEntry holds info about one grove-managed worktree.
type WorktreeEntry struct {
	Branch  string    `json:"branch"`
	Path    string    `json:"path"`
	Created time.Time `json:"created"`
}

// State is the top-level structure of .grove/state.json.
// The map key is the alias (e.g. "auth").
type State struct {
	Worktrees map[string]WorktreeEntry `json:"worktrees"`
}

// Load reads .grove/state.json from dir.
// If the file doesn't exist, returns an empty state (not an error).
// This is different from config.Load — missing state is normal (no worktrees yet).
func Load(dir string) (State, error) {
	path := filepath.Join(dir, stateDir, fileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{Worktrees: map[string]WorktreeEntry{}}, nil
		}
		return State{}, err
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, errors.New(".grove/state.json is not valid JSON: " + err.Error())
	}

	// Ensure map is initialized even if JSON had "worktrees": null
	if s.Worktrees == nil {
		s.Worktrees = map[string]WorktreeEntry{}
	}

	return s, nil
}

// Save writes state to .grove/state.json, creating the .grove directory if needed.
// Uses an atomic write (temp file + rename) so a concurrent reader never sees a partial file.
func Save(dir string, s State) error {
	dirPath := filepath.Join(dir, stateDir)

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// Write to a temp file in the same directory, then rename into place.
	// os.Rename is atomic on the same filesystem, so readers always see a complete file.
	tmp, err := os.CreateTemp(dirPath, "state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	if err := os.Rename(tmpName, filepath.Join(dirPath, fileName)); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// Add registers a new worktree alias. Returns an error if the alias is taken.
func (s *State) Add(alias, branch, path string) error {
	if _, exists := s.Worktrees[alias]; exists {
		return errors.New("alias \"" + alias + "\" already exists")
	}

	s.Worktrees[alias] = WorktreeEntry{
		Branch:  branch,
		Path:    path,
		Created: time.Now(),
	}
	return nil
}

// Remove deletes a worktree alias. Returns an error if the alias doesn't exist.
func (s *State) Remove(alias string) error {
	if _, exists := s.Worktrees[alias]; !exists {
		return errors.New("alias \"" + alias + "\" not found")
	}

	delete(s.Worktrees, alias)
	return nil
}

// Get looks up a worktree by alias.
// Returns the entry and true if found, zero value and false if not.
func (s *State) Get(alias string) (WorktreeEntry, bool) {
	entry, ok := s.Worktrees[alias]
	return entry, ok
}

// AliasExists checks if an alias is already taken.
func (s *State) AliasExists(alias string) bool {
	_, ok := s.Worktrees[alias]
	return ok
}
