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
	ctx, err := resolveRemoveContext(args[0])
	if err != nil {
		return err
	}
	removed, err := removeResolvedWorktree(&ctx)
	if err != nil || !removed {
		return err
	}

	if err := git.PruneWorktrees(); err != nil {
		fmt.Fprintf(os.Stderr, "  warning: git worktree prune failed: %v\n", err)
	}
	fmt.Printf("Worktree %q removed.\n", ctx.label)
	return nil
}

type removeContext struct {
	root   string
	config config.Config
	state  state.State
	target *resolvedWorktree
	label  string
}

func resolveRemoveContext(query string) (removeContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return removeContext{}, err
	}
	root, err := config.FindRoot(cwd)
	if err != nil {
		return removeContext{}, err
	}
	s, err := state.Load(root)
	if err != nil {
		return removeContext{}, err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return removeContext{}, err
	}
	target, err := resolveWorktree(root, query)
	if err != nil {
		return removeContext{}, err
	}
	if target == nil {
		return removeContext{}, fmt.Errorf("no worktree matching %q — run 'grove list' to see available worktrees", query)
	}
	if target.IsMain {
		return removeContext{}, fmt.Errorf("refusing to remove the main worktree")
	}
	label := target.Alias
	if label == "" {
		label = target.Branch
	}
	return removeContext{root: root, config: cfg, state: s, target: target, label: label}, nil
}

func removeResolvedWorktree(ctx *removeContext) (bool, error) {
	if ctx.target.InState {
		return removeManagedTarget(ctx)
	}
	return removeOrphanTarget(ctx.target, ctx.label)
}

func removeManagedTarget(ctx *removeContext) (bool, error) {
	if ctx.target.Protected && !removeIncludeProtected {
		return false, fmt.Errorf("worktree %q is protected — pass --include-protected to remove it", ctx.target.Alias)
	}
	entry, ok := ctx.state.Get(ctx.target.Alias)
	if !ok {
		return false, fmt.Errorf("state entry %q disappeared", ctx.target.Alias)
	}
	force := removeForce
	if _, statErr := os.Stat(entry.Path); statErr == nil {
		status, err := git.Status(entry.Path)
		if err != nil {
			return false, err
		}
		var proceed bool
		force, proceed, err = confirmDirtyRemoval(ctx.label, entry.Path, status, removeForce)
		if err != nil || !proceed {
			return false, err
		}
	} else if !os.IsNotExist(statErr) {
		return false, statErr
	}
	return removeManagedWorktree(ctx.root, ctx.config, &ctx.state, managedRemoveTarget{
		Alias:            ctx.target.Alias,
		Branch:           entry.Branch,
		Path:             entry.Path,
		Port:             entry.Port,
		IncludeProtected: removeIncludeProtected,
	}, force)
}

func removeOrphanTarget(target *resolvedWorktree, label string) (bool, error) {
	if _, statErr := os.Stat(target.Path); os.IsNotExist(statErr) {
		fmt.Printf("Worktree path %s no longer exists.\n", target.Path)
		return true, nil
	} else if statErr != nil {
		return false, statErr
	}
	status, err := git.Status(target.Path)
	if err != nil {
		return false, err
	}
	force, proceed, err := confirmDirtyRemoval(label, target.Path, status, removeForce)
	if err != nil || !proceed {
		return false, err
	}
	if err := git.RemoveWorktree(target.Path, force); err != nil {
		return false, err
	}
	fmt.Printf("  ✓ removed worktree at %s\n", target.Path)
	return true, nil
}

func confirmDirtyRemoval(label, path, status string, explicitForce bool) (force, proceed bool, err error) {
	if explicitForce {
		return true, true, nil
	}
	if status == "clean" {
		return false, true, nil
	}
	if err := ensureNoBlockingSubmodules(path); err != nil {
		return false, false, err
	}
	fmt.Printf("Worktree %q has %s.\n", label, status)
	answer, err := promptWithError("Remove anyway? [y/N]", "n")
	if err != nil {
		return false, false, fmt.Errorf("removal confirmation unavailable: %w — rerun with --force to remove non-interactively", err)
	}
	if answer != "y" && answer != "Y" {
		fmt.Println("Aborted.")
		return false, false, nil
	}
	return true, true, nil
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
