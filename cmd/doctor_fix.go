package cmd

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

type orphanFixCandidate struct {
	target      orphanWorktree
	alias       string
	unavailable bool
}

func fixOrphanWorktrees(root string, cfg config.Config, snapshot state.State, worktrees []git.Worktree) ([]doctorFix, []diagnostic) {
	tracked := make(map[string]bool, len(snapshot.Worktrees))
	for _, entry := range snapshot.Worktrees {
		tracked[entry.Path] = true
	}

	var candidates []orphanFixCandidate
	aliasCounts := make(map[string]int)
	for _, worktree := range worktrees {
		if worktree.IsMain || tracked[worktree.Path] {
			continue
		}
		alias := branchAlias(worktree.Branch)
		unavailable := false
		if _, err := os.Stat(worktree.Path); errors.Is(err, os.ErrNotExist) {
			unavailable = true
		}
		candidates = append(candidates, orphanFixCandidate{
			target:      orphanWorktree{Path: worktree.Path, Branch: worktree.Branch},
			alias:       alias,
			unavailable: unavailable,
		})
		if !unavailable {
			aliasCounts[alias]++
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].target.Path < candidates[j].target.Path
	})

	fixes := make([]doctorFix, 0, len(candidates))
	var diags []diagnostic
	for _, candidate := range candidates {
		fix := doctorFix{Action: "adopt-orphan", Target: candidate.target.Path}
		if candidate.unavailable {
			fix.Status = "skipped"
			fix.Message = "worktree path no longer exists"
			fixes = append(fixes, fix)
			continue
		}
		if err := validateAlias(candidate.alias); err != nil {
			fix.Status = "skipped"
			fix.Message = fmt.Sprintf("default alias %q is unsafe: %v", candidate.alias, err)
			fixes = append(fixes, fix)
			continue
		}
		if count := aliasCounts[candidate.alias]; count > 1 {
			fix.Status = "skipped"
			fix.Message = fmt.Sprintf("default alias %q is ambiguous across %d orphans", candidate.alias, count)
			fixes = append(fixes, fix)
			continue
		}

		latest, err := state.Load(root)
		if err != nil {
			fix.Status = "failed"
			fix.Message = err.Error()
			fixes = append(fixes, fix)
			diags = append(diags, diagnostic{levelError, fmt.Sprintf("adopt orphan %s: %v", candidate.target.Path, err)})
			continue
		}
		if owner, ok := managedAliasForPath(latest, candidate.target.Path); ok {
			fix.Status = "skipped"
			fix.Message = fmt.Sprintf("worktree was already adopted as %q", owner)
			fixes = append(fixes, fix)
			continue
		}
		if latest.AliasExists(candidate.alias) {
			fix.Status = "skipped"
			fix.Message = fmt.Sprintf("default alias %q is already in use", candidate.alias)
			fixes = append(fixes, fix)
			continue
		}

		currentWorktrees, err := git.ListWorktrees()
		if err != nil {
			fix.Status = "failed"
			fix.Message = err.Error()
			fixes = append(fixes, fix)
			diags = append(diags, diagnostic{levelError, fmt.Sprintf("adopt orphan %s: %v", candidate.target.Path, err)})
			continue
		}
		if !containsOrphanWorktree(currentWorktrees, candidate.target) {
			fix.Status = "skipped"
			fix.Message = "worktree is no longer present in git worktree list"
			fixes = append(fixes, fix)
			continue
		}

		port, err := adoptWorktree(root, cfg, candidate.target, candidate.alias)
		if err != nil {
			if errors.Is(err, errAdoptTargetChanged) || errors.Is(err, errAdoptTargetUnavailable) {
				fix.Status = "skipped"
				fix.Message = err.Error()
				fixes = append(fixes, fix)
				continue
			}
			if current, loadErr := state.Load(root); loadErr == nil {
				if owner, ok := managedAliasForPath(current, candidate.target.Path); ok {
					fix.Status = "skipped"
					fix.Message = fmt.Sprintf("worktree was adopted concurrently as %q", owner)
					fixes = append(fixes, fix)
					continue
				}
				if current.AliasExists(candidate.alias) {
					fix.Status = "skipped"
					fix.Message = fmt.Sprintf("default alias %q became unavailable", candidate.alias)
					fixes = append(fixes, fix)
					continue
				}
			}
			fix.Status = "failed"
			fix.Message = err.Error()
			fixes = append(fixes, fix)
			diags = append(diags, diagnostic{levelError, fmt.Sprintf("adopt orphan %s: %v", candidate.target.Path, err)})
			continue
		}

		fix.Status = "fixed"
		fix.Message = fmt.Sprintf("adopted as %q with port %d", candidate.alias, port)
		fixes = append(fixes, fix)
	}
	return fixes, diags
}

// nonCanonicalStatePaths maps aliases whose stored path is a symlinked spelling
// of a git worktree on the same branch to the path git reports. Older Grove
// versions stored such paths when the worktree directory did not exist yet.
func nonCanonicalStatePaths(s state.State, worktrees []git.Worktree) map[string]string {
	byPath := make(map[string]git.Worktree, len(worktrees))
	for _, wt := range worktrees {
		byPath[wt.Path] = wt
	}
	tracked := make(map[string]bool, len(s.Worktrees))
	for _, entry := range s.Worktrees {
		tracked[entry.Path] = true
	}

	repairs := make(map[string]string)
	targets := make(map[string]int)
	for alias, entry := range s.Worktrees {
		if _, ok := byPath[entry.Path]; ok {
			continue
		}
		canonical := normalizeWorktreePath(entry.Path)
		wt, ok := byPath[canonical]
		if !ok || wt.IsMain || wt.Branch != entry.Branch || tracked[canonical] {
			continue
		}
		repairs[alias] = canonical
		targets[canonical]++
	}
	for alias, canonical := range repairs {
		if targets[canonical] > 1 {
			delete(repairs, alias)
		}
	}
	return repairs
}

func fixStatePaths(root string, snapshot state.State, worktrees []git.Worktree) ([]doctorFix, []diagnostic) {
	repairs := nonCanonicalStatePaths(snapshot, worktrees)
	if len(repairs) == 0 {
		return nil, nil
	}
	aliases := slices.Sorted(maps.Keys(repairs))

	var fixes []doctorFix
	err := state.Update(root, func(latest *state.State) error {
		fixes = fixes[:0]
		for _, alias := range aliases {
			fix := doctorFix{Action: "repair-state-path", Target: alias}
			entry, ok := latest.Get(alias)
			if !ok || entry.Path != snapshot.Worktrees[alias].Path {
				fix.Status = "skipped"
				fix.Message = "state entry changed before repair"
			} else if owner, taken := managedAliasForPath(*latest, repairs[alias]); taken {
				fix.Status = "skipped"
				fix.Message = fmt.Sprintf("path is already tracked as %q", owner)
			} else {
				entry.Path = repairs[alias]
				latest.Worktrees[alias] = entry
				fix.Status = "fixed"
				fix.Message = fmt.Sprintf("path set to %s", repairs[alias])
			}
			fixes = append(fixes, fix)
		}
		return nil
	})
	if err != nil {
		fixes = fixes[:0]
		for _, alias := range aliases {
			fixes = append(fixes, doctorFix{Action: "repair-state-path", Target: alias, Status: "failed", Message: err.Error()})
		}
		return fixes, []diagnostic{{levelError, fmt.Sprintf("repair state paths: %v", err)}}
	}
	return fixes, nil
}

func managedAliasForPath(s state.State, path string) (string, bool) {
	for alias, entry := range s.Worktrees {
		if entry.Path == path {
			return alias, true
		}
	}
	return "", false
}
