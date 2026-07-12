package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

var renameJSON bool

func init() {
	rootCmd.AddCommand(renameCmd)
	renameCmd.Flags().BoolVar(&renameJSON, "json", false, "print renamed worktree details as JSON")
}

var renameCmd = &cobra.Command{
	Use:   "rename <name-or-number> <new-alias>",
	Short: "Rename a managed worktree alias",
	Long: `Rename a Grove-managed worktree alias.

Worktrees created at Grove's configured path are moved to the path derived
from the new alias. Adopted worktrees at custom paths remain in place. The
existing branch, port, protection, creation time, and config hash are kept.`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeAliases,
	RunE:              runRename,
}

type renameResult struct {
	OldAlias string `json:"oldAlias"`
	Alias    string `json:"alias"`
	Branch   string `json:"branch"`
	OldPath  string `json:"oldPath"`
	Path     string `json:"path"`
	Port     int    `json:"port,omitempty"`
	Moved    bool   `json:"moved"`
}

func runRename(_ *cobra.Command, args []string) error {
	return runRenameWithSave(args, state.Save)
}

func runRenameWithSave(args []string, saveState func(string, state.State) error) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	s, err := state.Load(root)
	if err != nil {
		return err
	}

	query, newAlias := args[0], args[1]
	if err := validateAlias(newAlias); err != nil {
		return err
	}
	resolved, err := resolveWorktree(root, query)
	if err != nil {
		return err
	}
	if resolved == nil {
		return fmt.Errorf("no worktree matching %q — run 'grove list' to see available worktrees", query)
	}
	if resolved.IsMain {
		return fmt.Errorf("refusing to rename the main worktree")
	}
	if !resolved.InState {
		return fmt.Errorf("worktree %q is not managed by Grove — run 'grove adopt' first", query)
	}
	if newAlias != resolved.Alias && s.AliasExists(newAlias) {
		return fmt.Errorf("alias %q already exists — choose a different one", newAlias)
	}

	entry, ok := s.Get(resolved.Alias)
	if !ok {
		return fmt.Errorf("state entry %q disappeared", resolved.Alias)
	}
	oldPath := entry.Path
	newPath := oldPath
	oldConfiguredPath, err := configuredWorktreePath(root, cfg, resolved.Alias)
	if err != nil {
		return err
	}
	if newAlias != resolved.Alias && filepath.Clean(oldPath) == filepath.Clean(oldConfiguredPath) {
		newPath, err = configuredWorktreePath(root, cfg, newAlias)
		if err != nil {
			return err
		}
	}

	if err := s.Rename(resolved.Alias, newAlias, newPath); err != nil {
		return err
	}
	moved := newPath != oldPath
	if moved {
		if err := git.MoveWorktree(oldPath, newPath); err != nil {
			return err
		}
	}
	if err := saveState(root, s); err != nil {
		if !moved {
			return fmt.Errorf("save state: %w", err)
		}
		if rollbackErr := git.MoveWorktree(newPath, oldPath); rollbackErr != nil {
			return fmt.Errorf("save state: %w; rollback worktree move: %v", err, rollbackErr)
		}
		return fmt.Errorf("save state: %w (worktree move rolled back)", err)
	}

	result := renameResult{
		OldAlias: resolved.Alias,
		Alias:    newAlias,
		Branch:   entry.Branch,
		OldPath:  oldPath,
		Path:     newPath,
		Port:     entry.Port,
		Moved:    moved,
	}
	if renameJSON {
		return printJSON(result)
	}
	if moved {
		fmt.Printf("Worktree %q renamed to %q and moved to %s.\n", resolved.Alias, newAlias, newPath)
	} else {
		fmt.Printf("Worktree %q renamed to %q.\n", resolved.Alias, newAlias)
	}
	return nil
}
