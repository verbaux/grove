package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/files"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

var (
	createName string
	createFrom string
)

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().StringVar(&createName, "name", "", "alias for the worktree (default: last segment of branch name)")
	createCmd.Flags().StringVar(&createFrom, "from", "", "base branch or commit to create the new branch from")
}

var createCmd = &cobra.Command{
	Use:   "create <branch>",
	Short: "Create a new worktree for a branch",
	Long: `Create a new git worktree for a branch and set it up automatically.

Grove will:
  - Create the worktree with git worktree add
  - Copy all .env* files found in the project
  - Create symlinks for configured directories (e.g. node_modules)
  - Run the afterCreate command if configured

The branch will be created if it doesn't already exist.`,
	Args: cobra.ExactArgs(1),
	RunE: runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
	branch := args[0]

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

	// Derive alias from branch name unless --name was provided.
	// "feature/auth" → "auth", "main" → "main"
	alias := createName
	if alias == "" {
		alias = branchAlias(branch)
	}

	if s.AliasExists(alias) {
		return fmt.Errorf("alias %q already exists — use --name to choose a different one", alias)
	}

	// Build the worktree path: worktreeDir + prefix + "-" + alias
	// e.g. "../" + "myproject" + "-" + "auth" → "../myproject-auth"
	// If prefix is empty, use just the alias to avoid a leading dash.
	wtName := alias
	if cfg.Prefix != "" {
		wtName = cfg.Prefix + "-" + alias
	}
	worktreePath := filepath.Join(root, cfg.WorktreeDir, wtName)
	worktreePath, err = filepath.Abs(worktreePath)
	if err != nil {
		return err
	}
	// EvalSymlinks resolves /tmp → /private/tmp on macOS so path lookups
	// match what git worktree list returns.
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(worktreePath)); err == nil {
		worktreePath = filepath.Join(resolved, filepath.Base(worktreePath))
	}

	fmt.Printf("Creating worktree for branch %q at %s\n", branch, worktreePath)

	if err := git.AddWorktree(worktreePath, branch, createFrom); err != nil {
		return err
	}
	fmt.Println("  ✓ git worktree created")

	// If any step after this fails, clean up the worktree so we don't leave
	// an orphaned directory that git knows about but grove doesn't.
	var setupErr error
	defer func() {
		if setupErr != nil {
			fmt.Printf("  rolling back: removing worktree at %s\n", worktreePath)
			if rbErr := git.RemoveWorktree(worktreePath, true); rbErr != nil {
				fmt.Fprintf(os.Stderr, "  warning: rollback failed, manual cleanup needed: %v\n", rbErr)
			}
		}
	}()

	copied, err := files.CopyEnvFiles(root, worktreePath)
	if err != nil {
		setupErr = err
		return setupErr
	}
	if len(copied) > 0 {
		fmt.Printf("  ✓ copied %d .env file(s)\n", len(copied))
	}

	var symlinked []string
	for _, name := range cfg.Symlink {
		created, err := files.Symlink(root, worktreePath, name)
		if err != nil {
			if errors.Is(err, files.ErrSymlinkDestinationConflict) {
				fmt.Fprintf(os.Stderr, "  warning: skipping symlink %s: %v\n", name, err)
				continue
			}
			setupErr = fmt.Errorf("symlink %s: %w", name, err)
			return setupErr
		}
		if created {
			symlinked = append(symlinked, name)
		}
	}
	if len(symlinked) > 0 {
		fmt.Printf("  ✓ symlinked %s\n", strings.Join(symlinked, ", "))
	}

	if cfg.AfterCreate != "" {
		fmt.Printf("  running: %s\n", cfg.AfterCreate)
		if err := runShell(cfg.AfterCreate, worktreePath); err != nil {
			setupErr = fmt.Errorf("afterCreate command failed: %w", err)
			return setupErr
		}
		fmt.Println("  ✓ afterCreate done")
	}

	if err := s.Add(alias, branch, worktreePath); err != nil {
		setupErr = err
		return setupErr
	}
	if err := state.Save(root, s); err != nil {
		setupErr = err
		return setupErr
	}

	fmt.Println()
	fmt.Printf("Worktree %q ready.\n", alias)
	fmt.Printf("  cd $(grove cd %s)\n", alias)

	return nil
}

// branchAlias returns the last segment of a branch name.
// "feature/auth" → "auth", "fix/some/deep" → "deep", "main" → "main"
func branchAlias(branch string) string {
	parts := strings.Split(branch, "/")
	return parts[len(parts)-1]
}

// runShell runs a command string in the given directory.
// Uses "sh -c" so the string can include pipes, env vars, etc.
func runShell(command, dir string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
