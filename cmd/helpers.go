package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
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
	if alias == "" {
		return fmt.Errorf("alias cannot be empty")
	}
	if strings.TrimSpace(alias) != alias {
		return fmt.Errorf("alias %q cannot have leading or trailing whitespace", alias)
	}
	if alias == "." || alias == ".." {
		return fmt.Errorf("alias %q is not allowed", alias)
	}
	if strings.ContainsAny(alias, `/\`) {
		return fmt.Errorf("alias %q cannot contain path separators", alias)
	}
	if strings.IndexFunc(alias, unicode.IsControl) >= 0 {
		return fmt.Errorf("alias %q cannot contain control characters", alias)
	}
	if isNumericAlias(alias) {
		return fmt.Errorf("alias %q is not allowed — numeric-only names are reserved for index-based access (grove cd 3)", alias)
	}
	return nil
}

// updateManagedState applies a mutation to the latest on-disk state and, after
// a successful save, refreshes the caller's snapshot. Callers can perform slow
// setup work before this short transaction without overwriting other commands.
func updateManagedState(root string, snapshot *state.State, mutate func(*state.State) error) error {
	var updated state.State
	if err := state.Update(root, func(latest *state.State) error {
		if err := mutate(latest); err != nil {
			return err
		}
		updated = *latest
		return nil
	}); err != nil {
		return err
	}
	if snapshot != nil {
		*snapshot = updated
	}
	return nil
}

// worktreeRow holds display info for a single worktree in the list.
type worktreeRow struct {
	Index     int
	Name      string
	Branch    string
	Path      string
	Port      int
	Protected bool
	Note      string
	Status    string
	IsMain    bool
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
	pathToPort := make(map[string]int)
	pathToProtected := make(map[string]bool)
	pathToNote := make(map[string]string)
	for alias, entry := range s.Worktrees {
		pathToAlias[entry.Path] = alias
		pathToPort[entry.Path] = entry.Port
		pathToProtected[entry.Path] = entry.Protected
		pathToNote[entry.Path] = entry.Note
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
			Index:     i + 1,
			Name:      name,
			Branch:    wt.Branch,
			Path:      wt.Path,
			Port:      pathToPort[wt.Path],
			Protected: pathToProtected[wt.Path],
			Note:      pathToNote[wt.Path],
			Status:    status,
			IsMain:    wt.IsMain,
		})
	}

	return rows, nil
}

func displayNote(note string) string {
	if note == "" {
		return "-"
	}
	return terminalSafe(note)
}

// terminalSafe replaces control characters in values originating outside the
// current command invocation before they are rendered in a terminal.
func terminalSafe(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, value)
}

func completeAliases(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	root, err := config.FindRoot(cwd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	s, err := state.Load(root)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	aliases := make([]string, 0, len(s.Worktrees))
	for alias := range s.Worktrees {
		aliases = append(aliases, alias)
	}
	return aliases, cobra.ShellCompDirectiveNoFileComp
}

func completeOrphans(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	root, err := config.FindRoot(cwd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	s, err := state.Load(root)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	orphans, err := findOrphans(s)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	branches := make([]string, 0, len(orphans))
	for _, o := range orphans {
		branches = append(branches, o.Branch)
	}
	return branches, cobra.ShellCompDirectiveNoFileComp
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
	Alias     string
	Path      string
	Branch    string
	Protected bool
	InState   bool
	IsMain    bool
}

// resolveWorktree resolves a worktree reference to a worktree. query may be an
// alias, an index from `grove list`, a branch name, a path, or empty for the
// interactive picker. A nil result with nil error means nothing matched (or the
// picker was cancelled).
func resolveWorktree(root, query string) (*resolvedWorktree, error) {
	s, err := state.Load(root)
	if err != nil {
		return nil, err
	}

	// Empty: interactive picker.
	if query == "" {
		path, err := pickWorktree(root)
		if err != nil {
			return nil, err
		}
		if path == "" {
			return nil, nil // cancelled
		}
		return resolveByPath(path, s)
	}

	// Numeric: index from `grove list`. Numeric aliases are reserved, so this
	// is unambiguous.
	if idx, err := strconv.Atoi(query); err == nil {
		rows, err := buildWorktreeRows(root)
		if err != nil {
			return nil, err
		}
		if idx < 1 || idx > len(rows) {
			return nil, fmt.Errorf("index %d out of range — run 'grove list' to see available worktrees (1–%d)", idx, len(rows))
		}
		return rowToResolved(rows[idx-1], s), nil
	}

	// 1. Alias
	if entry, ok := s.Get(query); ok {
		return &resolvedWorktree{
			Alias: query, Path: entry.Path, Branch: entry.Branch, Protected: entry.Protected, InState: true,
		}, nil
	}

	// 2. Branch name in state
	for alias, entry := range s.Worktrees {
		if entry.Branch == query {
			return &resolvedWorktree{
				Alias: alias, Path: entry.Path, Branch: entry.Branch, Protected: entry.Protected, InState: true,
			}, nil
		}
	}

	// 3. Path in state
	for alias, entry := range s.Worktrees {
		if entry.Path == query {
			return &resolvedWorktree{
				Alias: alias, Path: entry.Path, Branch: entry.Branch, Protected: entry.Protected, InState: true,
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

// rowToResolved builds a resolvedWorktree from a list row, marking state membership.
func rowToResolved(r worktreeRow, s state.State) *resolvedWorktree {
	res := &resolvedWorktree{Path: r.Path, Branch: r.Branch, Protected: r.Protected, IsMain: r.IsMain}
	for alias, entry := range s.Worktrees {
		if entry.Path == r.Path {
			res.Alias, res.InState = alias, true
			break
		}
	}
	return res
}

// resolveByPath maps a worktree path (e.g. from the picker) to a resolvedWorktree.
func resolveByPath(path string, s state.State) (*resolvedWorktree, error) {
	for alias, entry := range s.Worktrees {
		if entry.Path == path {
			return &resolvedWorktree{Alias: alias, Path: entry.Path, Branch: entry.Branch, Protected: entry.Protected, InState: true}, nil
		}
	}
	worktrees, err := git.ListWorktrees()
	if err != nil {
		return nil, err
	}
	for _, wt := range worktrees {
		if wt.Path == path {
			return &resolvedWorktree{Path: wt.Path, Branch: wt.Branch, IsMain: wt.IsMain}, nil
		}
	}
	return nil, fmt.Errorf("worktree not found at %s", path)
}
