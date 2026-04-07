package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(detachCmd)
}

var detachCmd = &cobra.Command{
	Use:   "detach",
	Short: "Remove symlinks in the current worktree",
	Long: `Remove all symlinked directories in the current worktree so it
becomes fully independent. Run this from inside a worktree.

After detaching, install dependencies manually.`,
	RunE: runDetach,
}

func runDetach(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return err
	}

	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}

	s, err := state.Load(root)
	if err != nil {
		return err
	}

	var worktreePath string
	for _, entry := range s.Worktrees {
		resolved, err := filepath.EvalSymlinks(entry.Path)
		if err != nil {
			continue
		}
		if cwd == resolved || isSubdir(cwd, resolved) {
			worktreePath = resolved
			break
		}
	}

	if worktreePath == "" {
		return fmt.Errorf("not inside a worktree — run this from a grove-managed worktree directory")
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	if len(cfg.Symlink) == 0 {
		fmt.Println("No symlinks configured.")
		return nil
	}

	removed := 0
	for _, name := range cfg.Symlink {
		link := filepath.Join(worktreePath, name)
		fi, err := os.Lstat(link)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if err := os.Remove(link); err != nil {
			fmt.Printf("  ✗ could not remove %s: %v\n", name, err)
			continue
		}
		fmt.Printf("  ✓ removed symlink %s\n", name)
		removed++
	}

	if removed == 0 {
		fmt.Println("No symlinks found to remove.")
		return nil
	}

	fmt.Println()
	fmt.Println("Worktree detached. Install dependencies manually to continue.")
	return nil
}

func isSubdir(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && len(rel) > 0 && rel[0] != '.'
}
