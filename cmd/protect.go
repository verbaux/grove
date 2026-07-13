package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(protectCmd)
	rootCmd.AddCommand(unprotectCmd)
}

var protectCmd = &cobra.Command{
	Use:               "protect <name-or-number>",
	Short:             "Protect a managed worktree from removal",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAliases,
	RunE:              runProtect,
}

var unprotectCmd = &cobra.Command{
	Use:               "unprotect <name-or-number>",
	Short:             "Remove protection from a managed worktree",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeAliases,
	RunE:              runUnprotect,
}

func runProtect(cmd *cobra.Command, args []string) error {
	return setProtection(args[0], true)
}

func runUnprotect(cmd *cobra.Command, args []string) error {
	return setProtection(args[0], false)
}

func setProtection(query string, protected bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := config.FindRoot(cwd)
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
		return fmt.Errorf("refusing to protect the main worktree")
	}
	if !resolved.InState {
		return fmt.Errorf("cannot protect orphan worktree %q — run 'grove adopt' first", query)
	}

	if err := updateProtection(root, resolved, protected); err != nil {
		return err
	}
	if protected {
		fmt.Printf("Worktree %q protected.\n", resolved.Alias)
	} else {
		fmt.Printf("Worktree %q unprotected.\n", resolved.Alias)
	}
	return nil
}

func updateProtection(root string, resolved *resolvedWorktree, protected bool) error {
	return state.Update(root, func(latest *state.State) error {
		current, ok := latest.Get(resolved.Alias)
		if !ok {
			return fmt.Errorf("state entry %q disappeared while updating protection", resolved.Alias)
		}
		if current.Path != resolved.Path || current.Branch != resolved.Branch {
			return fmt.Errorf("state entry %q changed while updating protection — retry", resolved.Alias)
		}
		return latest.SetProtected(resolved.Alias, protected)
	})
}
