package cmd

import (
	"fmt"
	"strconv"

	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

// isNumericAlias returns true if the alias is purely numeric (e.g. "3", "42").
// Numeric-only aliases are reserved for index-based access in `grove cd`.
func isNumericAlias(alias string) bool {
	_, err := strconv.Atoi(alias)
	return err == nil
}

// validateAlias checks that an alias is valid for use as a worktree name.
func validateAlias(alias string) error {
	if isNumericAlias(alias) {
		return fmt.Errorf("alias %q is not allowed — numeric-only names are reserved for index-based access (grove cd 3)", alias)
	}
	return nil
}

// worktreeRow holds display info for a single worktree in the list.
type worktreeRow struct {
	Index  int
	Name   string
	Branch string
	Path   string
	Status string
	IsMain bool
}

// buildWorktreeRows builds an ordered list of worktree rows.
// The order matches `git worktree list` so that numbering is stable
// and consistent between `grove list` and `grove cd`.
func buildWorktreeRows(root string) ([]worktreeRow, error) {
	worktrees, err := git.ListWorktrees()
	if err != nil {
		return nil, err
	}

	s, err := state.Load(root)
	if err != nil {
		return nil, err
	}

	pathToAlias := make(map[string]string)
	for alias, entry := range s.Worktrees {
		pathToAlias[entry.Path] = alias
	}

	var rows []worktreeRow
	for i, wt := range worktrees {
		name := pathToAlias[wt.Path]
		if name == "" {
			if wt.IsMain {
				name = "main"
			} else {
				name = "?"
			}
		}

		status, err := git.Status(wt.Path)
		if err != nil {
			status = "unknown"
		}

		rows = append(rows, worktreeRow{
			Index:  i + 1,
			Name:   name,
			Branch: wt.Branch,
			Path:   wt.Path,
			Status: status,
			IsMain: wt.IsMain,
		})
	}

	return rows, nil
}

type orphanWorktree struct {
	Path   string
	Branch string
}

// findOrphans returns worktrees that git knows about but Grove doesn't track.
func findOrphans(s state.State) ([]orphanWorktree, error) {
	worktrees, err := git.ListWorktrees()
	if err != nil {
		return nil, err
	}

	tracked := make(map[string]bool)
	for _, entry := range s.Worktrees {
		tracked[entry.Path] = true
	}

	var orphans []orphanWorktree
	for _, wt := range worktrees {
		if wt.IsMain {
			continue
		}
		if !tracked[wt.Path] {
			orphans = append(orphans, orphanWorktree{Path: wt.Path, Branch: wt.Branch})
		}
	}
	return orphans, nil
}

type resolvedWorktree struct {
	Alias   string
	Path    string
	Branch  string
	InState bool
}

// resolveWorktree tries to find a worktree by alias, branch name, or path.
// Returns nil if nothing matches.
func resolveWorktree(query string, s state.State) (*resolvedWorktree, error) {
	// 1. Alias
	if entry, ok := s.Get(query); ok {
		return &resolvedWorktree{
			Alias: query, Path: entry.Path, Branch: entry.Branch, InState: true,
		}, nil
	}

	// 2. Branch name in state
	for alias, entry := range s.Worktrees {
		if entry.Branch == query {
			return &resolvedWorktree{
				Alias: alias, Path: entry.Path, Branch: entry.Branch, InState: true,
			}, nil
		}
	}

	// 3. Path in state
	for alias, entry := range s.Worktrees {
		if entry.Path == query {
			return &resolvedWorktree{
				Alias: alias, Path: entry.Path, Branch: entry.Branch, InState: true,
			}, nil
		}
	}

	// 4. Orphan worktree (git knows, grove doesn't)
	worktrees, err := git.ListWorktrees()
	if err != nil {
		return nil, err
	}
	for _, wt := range worktrees {
		if wt.IsMain {
			continue
		}
		if wt.Branch == query || wt.Path == query {
			return &resolvedWorktree{
				Path: wt.Path, Branch: wt.Branch, InState: false,
			}, nil
		}
	}

	return nil, nil
}
