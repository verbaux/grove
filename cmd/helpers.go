package cmd

import (
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

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
