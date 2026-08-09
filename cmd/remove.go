package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

var (
	removeForce            bool
	removeIncludeProtected bool
)

func init() {
	rootCmd.AddCommand(removeCmd)
	removeCmd.Flags().BoolVar(&removeForce, "force", false, "remove despite uncommitted changes or initialized submodules")
	removeCmd.Flags().BoolVar(&removeIncludeProtected, "include-protected", false, "allow removing protected worktrees")
}

var removeCmd = &cobra.Command{
	Use:   "remove <name-or-number>",
	Short: "Remove a worktree",
	Long: `Remove a worktree by alias or by its index number from 'grove list'.

Checks for uncommitted changes and asks for confirmation before removing.
Use --force to remove despite uncommitted changes or initialized submodule data.`,
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAliases,
	RunE:              runRemove,
}

func runRemove(cmd *cobra.Command, args []string) error {
	query := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	s, err := state.Load(root)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
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
		return fmt.Errorf("refusing to remove the main worktree")
	}

	label := resolved.Alias
	if label == "" {
		label = resolved.Branch
	}

	if resolved.InState {
		if resolved.Protected && !removeIncludeProtected {
			return fmt.Errorf("worktree %q is protected — pass --include-protected to remove it", resolved.Alias)
		}
		entry, ok := s.Get(resolved.Alias)
		if !ok {
			return fmt.Errorf("state entry %q disappeared", resolved.Alias)
		}
		_, err := removeManagedWorktree(root, cfg, &s, managedRemoveTarget{
			Alias:            resolved.Alias,
			Branch:           entry.Branch,
			Path:             entry.Path,
			Port:             entry.Port,
			IncludeProtected: removeIncludeProtected,
		}, removeForce)
		if err != nil {
			return err
		}
	} else if _, err := os.Stat(resolved.Path); os.IsNotExist(err) {
		fmt.Printf("Worktree path %s no longer exists.\n", resolved.Path)
	} else {
		status, err := git.Status(resolved.Path)
		if err != nil {
			return err
		}

		force := removeForce
		if status != "clean" && !removeForce {
			if err := ensureNoBlockingSubmodules(resolved.Path); err != nil {
				return err
			}
			fmt.Printf("Worktree %q has %s.\n", label, status)
			answer := prompt("Remove anyway? [y/N]", "n")
			if answer != "y" && answer != "Y" {
				fmt.Println("Aborted.")
				return nil
			}
			force = true
		}

		if err := git.RemoveWorktree(resolved.Path, force); err != nil {
			return err
		}
		fmt.Printf("  ✓ removed worktree at %s\n", resolved.Path)
	}

	if err := git.PruneWorktrees(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: git worktree prune failed: %v\n", err)
	}

	fmt.Printf("Worktree %q removed.\n", label)
	return nil
}

type managedRemoveTarget struct {
	Alias            string
	Branch           string
	Path             string
	Port             int
	IncludeProtected bool
}

func removeManagedWorktree(root string, cfg config.Config, s *state.State, wt managedRemoveTarget, force bool) (bool, error) {
	_, statErr := os.Stat(wt.Path)
	pathMissing := os.IsNotExist(statErr)

	if !pathMissing && !force {
		if err := ensureNoBlockingSubmodules(wt.Path); err != nil {
			return false, err
		}
	}

	env := groveHookEnv(wt)
	if !pathMissing {
		if err := runLifecycleHook("beforeRemove", cfg.BeforeRemove, wt.Path, env); err != nil {
			return false, err
		}
	}

	if err := updateManagedState(root, s, func(latest *state.State) error {
		current, ok := latest.Get(wt.Alias)
		if !ok {
			return fmt.Errorf("state entry %q disappeared while removing", wt.Alias)
		}
		if current.Path != wt.Path || current.Branch != wt.Branch {
			return fmt.Errorf("state entry %q changed while removing — retry", wt.Alias)
		}
		if current.Protected && !wt.IncludeProtected {
			return fmt.Errorf("worktree %q became protected while removing — pass --include-protected to remove it", wt.Alias)
		}
		if !pathMissing {
			if err := git.RemoveWorktree(wt.Path, force); err != nil {
				return err
			}
		}
		return latest.Remove(wt.Alias)
	}); err != nil {
		return false, err
	}

	if pathMissing {
		fmt.Printf("  ✓ cleaned stale entry %s (path no longer exists)\n", wt.Alias)
		return true, nil
	}
	if err := runLifecycleHook("afterRemove", cfg.AfterRemove, root, env); err != nil {
		return true, err
	}
	fmt.Printf("  ✓ removed %s\n", wt.Alias)
	return true, nil
}

func groveHookEnv(wt managedRemoveTarget) []string {
	return []string{
		"GROVE_ALIAS=" + wt.Alias,
		"GROVE_BRANCH=" + wt.Branch,
		"GROVE_PATH=" + wt.Path,
		fmt.Sprintf("GROVE_PORT=%d", wt.Port),
	}
}

func runLifecycleHook(name string, commands config.HookCommands, dir string, env []string) error {
	for i, command := range commands {
		fmt.Printf("  running %s [%d/%d]: %s\n", name, i+1, len(commands), command)
		if err := runShell(command, dir, env); err != nil {
			return fmt.Errorf("%s command %d failed: %w", name, i+1, err)
		}
	}
	return nil
}
