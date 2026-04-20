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
	"github.com/verbaux/grove/internal/ports"
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

	alias := createName
	if alias == "" {
		alias = branchAlias(branch)
	}

	return doCreate(root, cfg, &s, branch, alias, createFrom)
}

// doCreate is the shared worktree creation logic used by both `grove create` and `grove review`.
func doCreate(root string, cfg config.Config, s *state.State, branch, alias, from string) error {
	if err := validateAlias(alias); err != nil {
		return err
	}

	if s.AliasExists(alias) {
		return fmt.Errorf("alias %q already exists — use --name to choose a different one", alias)
	}

	wtName := alias
	if cfg.Prefix != "" {
		wtName = cfg.Prefix + "-" + alias
	}
	worktreePath := filepath.Join(root, cfg.WorktreeDir, wtName)
	worktreePath, err := filepath.Abs(worktreePath)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(worktreePath)); err == nil {
		worktreePath = filepath.Join(resolved, filepath.Base(worktreePath))
	}

	portMin, portMax := cfg.ResolvedPortRange()
	used := make(map[int]bool)
	for _, e := range s.Worktrees {
		if e.Port != 0 {
			used[e.Port] = true
		}
	}
	port, err := ports.Allocate(alias, portMin, portMax, used)
	if err != nil {
		return fmt.Errorf("port allocation: %w", err)
	}

	fmt.Printf("Creating worktree for branch %q at %s\n", branch, worktreePath)

	if err := git.AddWorktree(worktreePath, branch, from); err != nil {
		return err
	}
	fmt.Println("  ✓ git worktree created")

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

	var copiedDirs []string
	for _, name := range cfg.CopyDirs {
		src := filepath.Join(root, name)
		dst := filepath.Join(worktreePath, name)
		copied, err := files.CopyDir(src, dst)
		if err != nil {
			setupErr = fmt.Errorf("copy %s: %w", name, err)
			return setupErr
		}
		if copied {
			copiedDirs = append(copiedDirs, name)
		}
	}
	if len(copiedDirs) > 0 {
		fmt.Printf("  ✓ copied %s\n", strings.Join(copiedDirs, ", "))
	}

	groveEnv := []string{
		"GROVE_ALIAS=" + alias,
		"GROVE_BRANCH=" + branch,
		"GROVE_PATH=" + worktreePath,
		fmt.Sprintf("GROVE_PORT=%d", port),
	}

	if cfg.AfterCreate != "" {
		fmt.Printf("  running: %s\n", cfg.AfterCreate)
		if err := runShell(cfg.AfterCreate, worktreePath, groveEnv); err != nil {
			setupErr = fmt.Errorf("afterCreate command failed: %w", err)
			return setupErr
		}
		fmt.Println("  ✓ afterCreate done")
	}

	if err := s.Add(alias, branch, worktreePath, port); err != nil {
		setupErr = err
		return setupErr
	}
	if err := state.Save(root, *s); err != nil {
		setupErr = err
		return setupErr
	}

	fmt.Println()
	fmt.Printf("Worktree %q ready (port %d).\n", alias, port)
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
// extraEnv is appended to os.Environ so callers can expose additional variables.
func runShell(command, dir string, extraEnv []string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd.Run()
}
