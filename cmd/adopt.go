package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/ports"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(adoptCmd)
}

var adoptCmd = &cobra.Command{
	Use:   "adopt [branch-or-path]",
	Short: "Register an existing worktree with grove",
	Long: `Adopt a git worktree that was created outside of grove.

If there is only one orphan worktree, it will be selected automatically.
Otherwise, pass a branch name or path to identify which one to adopt.
You will be prompted for an alias (defaults to the branch name).`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeOrphans,
	RunE:              runAdopt,
}

func runAdopt(cmd *cobra.Command, args []string) error {
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

	orphans, err := findOrphans(s)
	if err != nil {
		return err
	}

	if len(orphans) == 0 {
		fmt.Println("No orphan worktrees found. All worktrees are tracked by grove.")
		return nil
	}

	var target orphanWorktree

	if len(args) == 1 {
		query := args[0]
		for _, o := range orphans {
			if o.Branch == query || o.Path == query {
				target = o
				break
			}
		}
		if target.Path == "" {
			fmt.Println("No orphan worktree matches that query. Available orphans:")
			for _, o := range orphans {
				fmt.Printf("  %s → %s\n", o.Branch, o.Path)
			}
			return nil
		}
	} else if len(orphans) == 1 {
		target = orphans[0]
		fmt.Printf("Found orphan worktree: %s (%s)\n", target.Branch, target.Path)
	} else {
		fmt.Println("Multiple orphan worktrees found:")
		for i, o := range orphans {
			fmt.Printf("  [%d] %s → %s\n", i+1, o.Branch, o.Path)
		}
		fmt.Println()
		answer := prompt("Which one? (number)", "")
		var idx int
		if _, err := fmt.Sscanf(answer, "%d", &idx); err != nil || idx < 1 || idx > len(orphans) {
			fmt.Println("Aborted.")
			return nil
		}
		target = orphans[idx-1]
	}

	defaultAlias := branchAlias(target.Branch)
	alias := prompt(fmt.Sprintf("Alias [%s]", defaultAlias), defaultAlias)
	alias = strings.TrimSpace(alias)

	if err := validateAlias(alias); err != nil {
		return err
	}

	if s.AliasExists(alias) {
		return fmt.Errorf("alias %q already exists — choose a different one", alias)
	}

	port, err := adoptWorktree(root, cfg, target, alias)
	if err != nil {
		return err
	}

	fmt.Printf("Worktree %q adopted (%s, port %d).\n", alias, target.Path, port)
	return nil
}

func adoptWorktree(root string, cfg config.Config, target orphanWorktree, alias string) (int, error) {
	if err := validateAlias(alias); err != nil {
		return 0, err
	}

	var port int
	if err := state.Update(root, func(latest *state.State) error {
		if latest.AliasExists(alias) {
			return fmt.Errorf("alias %q already exists — choose a different one", alias)
		}
		for existingAlias, entry := range latest.Worktrees {
			if entry.Path == target.Path {
				return fmt.Errorf("worktree path %s is already managed as %q", target.Path, existingAlias)
			}
		}

		portMin, portMax := cfg.ResolvedPortRange()
		used := make(map[int]bool)
		for _, entry := range latest.Worktrees {
			if entry.Port != 0 {
				used[entry.Port] = true
			}
		}
		allocated, err := ports.Allocate(alias, portMin, portMax, used)
		if err != nil {
			return fmt.Errorf("port allocation: %w", err)
		}
		port = allocated
		return latest.Add(alias, target.Branch, target.Path, port)
	}); err != nil {
		return 0, err
	}
	return port, nil
}
