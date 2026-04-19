package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/files"
	"github.com/verbaux/grove/internal/state"
)

var detachCopy bool

func init() {
	rootCmd.AddCommand(detachCmd)
	detachCmd.Flags().BoolVar(&detachCopy, "copy", false, "copy symlink targets before removing")
}

var detachCmd = &cobra.Command{
	Use:   "detach",
	Short: "Remove symlinks in the current worktree",
	Long: `Remove all symlinked directories in the current worktree so it
becomes fully independent.

By default, prompts whether to copy each symlink's contents.
Use --copy to copy all without prompting.

After detaching without copying, install dependencies manually.`,
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

		target, err := os.Readlink(link)
		if err != nil {
			fmt.Printf("  ✗ could not read symlink %s: %v\n", name, err)
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(link), target)
		}

		shouldCopy := detachCopy
		if !shouldCopy {
			answer := prompt(
				fmt.Sprintf("  %s → %s\n  Copy contents before removing? [y/N]", name, target),
				"n",
			)
			shouldCopy = strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
		}

		if err := os.Remove(link); err != nil {
			fmt.Printf("  ✗ could not remove %s: %v\n", name, err)
			continue
		}

		if shouldCopy {
			copied, err := files.CopyDir(target, link)
			if err != nil {
				fmt.Printf("  ✗ failed to copy %s: %v\n", name, err)
				removed++
				continue
			}
			if copied {
				fmt.Printf("  ✓ copied %s\n", name)
			}
		}

		fmt.Printf("  ✓ removed symlink %s\n", name)
		removed++
	}

	if removed == 0 {
		fmt.Println("No symlinks found to remove.")
		return nil
	}

	fmt.Println()
	fmt.Println("Worktree detached.")
	return nil
}

func isSubdir(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && len(rel) > 0 && rel[0] != '.'
}
