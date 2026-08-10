package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/files"
	"github.com/verbaux/grove/internal/state"
)

var (
	syncHooks bool
	syncJSON  bool
)

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().BoolVar(&syncHooks, "hooks", false, "run afterCreate hooks after syncing files")
	syncCmd.Flags().BoolVar(&syncJSON, "json", false, "print sync result as JSON")
}

var syncCmd = &cobra.Command{
	Use:   "sync <name-or-number>",
	Short: "Sync a worktree with current .groverc.json setup",
	Long: `Bring a managed worktree up to the current .groverc.json setup.

Sync creates missing configured symlinks, copies missing .env* files, copies
new copyDirs that are absent in the worktree, and updates the stored config
hash. Hooks are skipped by default; pass --hooks to run afterCreate commands.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAliases,
	RunE:              runSync,
}

type syncResult struct {
	Alias             string   `json:"alias"`
	Branch            string   `json:"branch"`
	Path              string   `json:"path"`
	CopiedEnv         []string `json:"copiedEnv"`
	Symlinked         []string `json:"symlinked"`
	SkippedSymlinks   []string `json:"skippedSymlinks"`
	CopiedDirs        []string `json:"copiedDirs"`
	SkippedCopyDirs   []string `json:"skippedCopyDirs"`
	RanHooks          []string `json:"ranHooks"`
	UpdatedConfigHash bool     `json:"updatedConfigHash"`
}

func runSync(cmd *cobra.Command, args []string) error {
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
	resolved, err := resolveWorktree(root, args[0])
	if err != nil {
		return err
	}
	if resolved == nil {
		return fmt.Errorf("no worktree matching %q — run 'grove list' to see available worktrees", args[0])
	}
	if resolved.IsMain {
		return fmt.Errorf("refusing to sync the main worktree")
	}
	if !resolved.InState {
		return fmt.Errorf("worktree %q is not managed by Grove — run 'grove adopt' first", args[0])
	}

	entry, ok := s.Get(resolved.Alias)
	if !ok {
		return fmt.Errorf("state entry %q disappeared", resolved.Alias)
	}
	result, err := syncManagedWorktree(root, cfg, &s, resolved.Alias, entry, syncJSON)
	if err != nil {
		return err
	}
	if syncJSON {
		return printJSON(result)
	}
	printSyncResult(result)
	return nil
}

func syncManagedWorktree(root string, cfg config.Config, s *state.State, alias string, entry state.WorktreeEntry, quiet bool) (syncResult, error) {
	result := syncResult{
		Alias:           alias,
		Branch:          entry.Branch,
		Path:            entry.Path,
		CopiedEnv:       []string{},
		Symlinked:       []string{},
		SkippedSymlinks: []string{},
		CopiedDirs:      []string{},
		SkippedCopyDirs: []string{},
		RanHooks:        []string{},
	}

	if _, err := os.Stat(entry.Path); errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("worktree %q path missing: %s", alias, entry.Path)
	} else if err != nil {
		return result, err
	}
	symlinks, err := config.ExpandSymlink(root, cfg)
	if err != nil {
		return result, err
	}
	copyDirs, err := config.ExpandCopyDirs(root, cfg)
	if err != nil {
		return result, err
	}
	symlinkWarnings := actionableExpansionWarnings(symlinks.Warnings)
	copyDirWarnings := actionableExpansionWarnings(copyDirs.Warnings)
	result.SkippedSymlinks = append(result.SkippedSymlinks, expansionWarningValues(symlinkWarnings)...)
	result.SkippedCopyDirs = append(result.SkippedCopyDirs, expansionWarningValues(copyDirWarnings)...)
	if !quiet {
		printExpansionWarnings("symlink", symlinkWarnings)
		printExpansionWarnings("copyDir", copyDirWarnings)
	}

	copiedEnv, err := files.CopyMissingEnvFiles(root, entry.Path)
	if err != nil {
		return result, err
	}
	result.CopiedEnv = copiedEnv

	for _, name := range symlinks.Paths {
		created, err := files.Symlink(root, entry.Path, name)
		if err != nil {
			if errors.Is(err, files.ErrSymlinkDestinationConflict) {
				result.SkippedSymlinks = append(result.SkippedSymlinks, name)
				continue
			}
			return result, fmt.Errorf("symlink %s: %w", name, err)
		}
		if created {
			result.Symlinked = append(result.Symlinked, name)
		}
	}

	for _, name := range copyDirs.Paths {
		dst := filepath.Join(entry.Path, name)
		if _, err := os.Stat(dst); err == nil {
			result.SkippedCopyDirs = append(result.SkippedCopyDirs, name)
			continue
		} else if !os.IsNotExist(err) {
			return result, err
		}

		copied, err := files.CopyDir(filepath.Join(root, name), dst)
		if err != nil {
			return result, fmt.Errorf("copy %s: %w", name, err)
		}
		if copied {
			result.CopiedDirs = append(result.CopiedDirs, name)
		}
	}

	if syncHooks {
		hookStdout := io.Writer(os.Stdout)
		if quiet {
			hookStdout = os.Stderr
		}
		env := []string{
			"GROVE_ALIAS=" + alias,
			"GROVE_BRANCH=" + entry.Branch,
			"GROVE_PATH=" + entry.Path,
			fmt.Sprintf("GROVE_PORT=%d", entry.Port),
		}
		for i, command := range cfg.AfterCreate {
			if err := runShellIO(command, entry.Path, env, hookStdout, os.Stderr); err != nil {
				return result, fmt.Errorf("afterCreate command %d failed: %w", i+1, err)
			}
			result.RanHooks = append(result.RanHooks, command)
		}
	}

	configHash, err := config.SetupHash(cfg)
	if err != nil {
		return result, err
	}
	if entry.ConfigHash != configHash {
		if err := updateManagedState(root, s, func(latest *state.State) error {
			current, ok := latest.Get(alias)
			if !ok {
				return fmt.Errorf("state entry %q disappeared while syncing", alias)
			}
			if current.Path != entry.Path || current.Branch != entry.Branch {
				return fmt.Errorf("state entry %q changed while syncing — retry", alias)
			}
			current.ConfigHash = configHash
			latest.Worktrees[alias] = current
			return nil
		}); err != nil {
			return result, err
		}
		result.UpdatedConfigHash = true
	}

	return result, nil
}

func printSyncResult(result syncResult) {
	fmt.Printf("Synced %q.\n", result.Alias)
	printSyncList("copied env", result.CopiedEnv)
	printSyncList("symlinked", result.Symlinked)
	printSyncList("copied dirs", result.CopiedDirs)
	printSyncList("skipped symlinks", result.SkippedSymlinks)
	printSyncList("skipped copyDirs", result.SkippedCopyDirs)
	printSyncList("ran hooks", result.RanHooks)
	if result.UpdatedConfigHash {
		fmt.Println("  ✓ updated config hash")
	}
}

func printSyncList(label string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Printf("  ✓ %s: %s\n", label, strings.Join(values, ", "))
}
