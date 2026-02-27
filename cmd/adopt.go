package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
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
	Args: cobra.MaximumNArgs(1),
	RunE: runAdopt,
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

	if s.AliasExists(alias) {
		return fmt.Errorf("alias %q already exists — choose a different one", alias)
	}

	if err := s.Add(alias, target.Branch, target.Path); err != nil {
		return err
	}
	if err := state.Save(root, s); err != nil {
		return err
	}

	fmt.Printf("Worktree %q adopted (%s).\n", alias, target.Path)
	return nil
}
