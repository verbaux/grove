package cmd

import (
	"errors"
	"fmt"
	"sort"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

type orphanFixCandidate struct {
	target orphanWorktree
	alias  string
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
		candidates = append(candidates, orphanFixCandidate{
			target: orphanWorktree{Path: worktree.Path, Branch: worktree.Branch},
			alias:  alias,
		})
		aliasCounts[alias]++
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].target.Path < candidates[j].target.Path
	})

	fixes := make([]doctorFix, 0, len(candidates))
	var diags []diagnostic
	for _, candidate := range candidates {
		fix := doctorFix{Action: "adopt-orphan", Target: candidate.target.Path}
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
			if errors.Is(err, errAdoptTargetChanged) {
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

func managedAliasForPath(s state.State, path string) (string, bool) {
	for alias, entry := range s.Worktrees {
		if entry.Path == path {
			return alias, true
		}
	}
	return "", false
}
