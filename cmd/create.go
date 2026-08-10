package cmd

import (
	"errors"
	"fmt"
	"io"
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
	createName   string
	createFrom   string
	createDetach bool
	createJSON   bool
)

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().StringVar(&createName, "name", "", "alias for the worktree (default: last segment of branch name)")
	createCmd.Flags().StringVar(&createFrom, "from", "", "base branch or commit to create the new branch from")
	createCmd.Flags().BoolVar(&createDetach, "detach", false, "create worktree without symlinks (runs afterDetachedCreate before afterCreate)")
	createCmd.Flags().BoolVar(&createJSON, "json", false, "print created worktree details as JSON")
}

type createResult struct {
	Alias           string   `json:"alias"`
	Branch          string   `json:"branch"`
	Path            string   `json:"path"`
	Port            int      `json:"port"`
	Detached        bool     `json:"detached"`
	CopiedEnv       []string `json:"copiedEnv"`
	Symlinked       []string `json:"symlinked"`
	SkippedSymlinks []string `json:"skippedSymlinks"`
	CopiedDirs      []string `json:"copiedDirs"`
	SkippedCopyDirs []string `json:"skippedCopyDirs"`
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

With --detach, symlinks are skipped. If afterDetachedCreate is configured,
its commands run before afterCreate (useful for installing dependencies
locally instead of relying on symlinked node_modules).

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

	result, err := doCreate(root, cfg, &s, branch, alias, createFrom, createDetach, createJSON)
	if err != nil {
		return err
	}
	if createJSON {
		return printJSON(result)
	}
	return nil
}

// doCreate is the shared worktree creation logic used by both `grove create` and `grove review`.
// When detach is true, symlinks are skipped and afterDetachedCreate runs before afterCreate.
func doCreate(root string, cfg config.Config, s *state.State, branch, alias, from string, detach bool, quiet bool) (createResult, error) {
	result := createResult{
		Alias:           alias,
		Branch:          branch,
		Detached:        detach,
		CopiedEnv:       []string{},
		Symlinked:       []string{},
		SkippedSymlinks: []string{},
		CopiedDirs:      []string{},
		SkippedCopyDirs: []string{},
	}
	if err := validateAlias(alias); err != nil {
		return result, err
	}

	if s.AliasExists(alias) {
		return result, fmt.Errorf("alias %q already exists — use --name to choose a different one", alias)
	}

	worktreePath, err := configuredWorktreePath(root, cfg, alias)
	if err != nil {
		return result, err
	}
	result.Path = worktreePath

	portMin, portMax := cfg.ResolvedPortRange()
	used := make(map[int]bool)
	for _, e := range s.Worktrees {
		if e.Port != 0 {
			used[e.Port] = true
		}
	}
	port, err := ports.Allocate(alias, portMin, portMax, used)
	if err != nil {
		return result, fmt.Errorf("port allocation: %w", err)
	}
	result.Port = port

	symlinks, err := config.ExpandSymlink(root, cfg)
	if err != nil {
		return result, err
	}
	copyDirs := config.PathExpansion{}
	copyDirsEnabled := cfg.ShouldCopyDirs(detach)
	if copyDirsEnabled {
		copyDirs, err = config.ExpandCopyDirs(root, cfg)
		if err != nil {
			return result, err
		}
	}
	symlinkWarnings := actionableExpansionWarnings(symlinks.Warnings)
	copyDirWarnings := actionableExpansionWarnings(copyDirs.Warnings)
	result.SkippedSymlinks = expansionWarningValues(symlinkWarnings)
	result.SkippedCopyDirs = expansionWarningValues(copyDirWarnings)

	if !quiet {
		fmt.Printf("Creating worktree for branch %q at %s\n", branch, worktreePath)
		printExpansionWarnings("symlink", symlinkWarnings)
		printExpansionWarnings("copyDir", copyDirWarnings)
	}

	if err := git.AddWorktree(worktreePath, branch, from); err != nil {
		return result, err
	}
	if !quiet {
		fmt.Println("  ✓ git worktree created")
	}

	var setupErr error
	defer func() {
		if setupErr != nil {
			if !quiet {
				fmt.Printf("  rolling back: removing worktree at %s\n", worktreePath)
			}
			if rbErr := git.RemoveWorktree(worktreePath, true); rbErr != nil {
				fmt.Fprintf(os.Stderr, "  warning: rollback failed, manual cleanup needed: %v\n", rbErr)
			}
		}
	}()

	copied, err := files.CopyEnvFiles(root, worktreePath)
	if err != nil {
		setupErr = err
		return result, setupErr
	}
	if copied != nil {
		result.CopiedEnv = copied
	}
	if len(copied) > 0 && !quiet {
		fmt.Printf("  ✓ copied %d .env file(s)\n", len(copied))
	}

	if detach {
		if len(symlinks.Paths) > 0 && !quiet {
			fmt.Printf("  ⚠ skipping %d symlink(s) (--detach)\n", len(symlinks.Paths))
		}
		if !copyDirsEnabled && len(cfg.CopyDirs) > 0 && !quiet {
			fmt.Println("  ⚠ skipping configured copyDirs (--detach, copyDirsOnDetach=false)")
		}
	} else {
		var symlinked []string
		for _, name := range symlinks.Paths {
			created, err := files.Symlink(root, worktreePath, name)
			if err != nil {
				if errors.Is(err, files.ErrSymlinkDestinationConflict) {
					result.SkippedSymlinks = append(result.SkippedSymlinks, name)
					fmt.Fprintf(os.Stderr, "  warning: skipping symlink %s: %v\n", name, err)
					continue
				}
				setupErr = fmt.Errorf("symlink %s: %w", name, err)
				return result, setupErr
			}
			if created {
				symlinked = append(symlinked, name)
			}
		}
		if symlinked != nil {
			result.Symlinked = symlinked
		}
		if len(symlinked) > 0 && !quiet {
			fmt.Printf("  ✓ symlinked %s\n", strings.Join(symlinked, ", "))
		}
	}

	var copiedDirs []string
	for _, name := range copyDirs.Paths {
		src := filepath.Join(root, name)
		dst := filepath.Join(worktreePath, name)
		copied, err := files.CopyDir(src, dst)
		if err != nil {
			setupErr = fmt.Errorf("copy %s: %w", name, err)
			return result, setupErr
		}
		if copied {
			copiedDirs = append(copiedDirs, name)
		}
	}
	if copiedDirs != nil {
		result.CopiedDirs = copiedDirs
	}
	if len(copiedDirs) > 0 && !quiet {
		fmt.Printf("  ✓ copied %s\n", strings.Join(copiedDirs, ", "))
	}

	groveEnv := []string{
		"GROVE_ALIAS=" + alias,
		"GROVE_BRANCH=" + branch,
		"GROVE_PATH=" + worktreePath,
		fmt.Sprintf("GROVE_PORT=%d", port),
	}

	hookStdout := io.Writer(os.Stdout)
	if quiet {
		hookStdout = os.Stderr
	}

	if detach {
		for i, command := range cfg.AfterDetachedCreate {
			if !quiet {
				fmt.Printf("  running afterDetachedCreate [%d/%d]: %s\n", i+1, len(cfg.AfterDetachedCreate), command)
			}
			if err := runShellIO(command, worktreePath, groveEnv, hookStdout, os.Stderr); err != nil {
				setupErr = fmt.Errorf("afterDetachedCreate command %d failed: %w", i+1, err)
				return result, setupErr
			}
		}
		if len(cfg.AfterDetachedCreate) > 0 && !quiet {
			fmt.Println("  ✓ afterDetachedCreate done")
		}
	}

	for i, command := range cfg.AfterCreate {
		if !quiet {
			fmt.Printf("  running [%d/%d]: %s\n", i+1, len(cfg.AfterCreate), command)
		}
		if err := runShellIO(command, worktreePath, groveEnv, hookStdout, os.Stderr); err != nil {
			setupErr = fmt.Errorf("afterCreate command %d failed: %w", i+1, err)
			return result, setupErr
		}
	}
	if len(cfg.AfterCreate) > 0 && !quiet {
		fmt.Println("  ✓ afterCreate done")
	}

	configHash, err := config.SetupHash(cfg)
	if err != nil {
		setupErr = fmt.Errorf("config hash: %w", err)
		return result, setupErr
	}
	if err := updateManagedState(root, s, func(latest *state.State) error {
		if latest.AliasExists(alias) {
			return fmt.Errorf("alias %q was created by another Grove process", alias)
		}
		for otherAlias, entry := range latest.Worktrees {
			if entry.Port == port {
				return fmt.Errorf("port %d was assigned to worktree %q by another Grove process — retry create", port, otherAlias)
			}
		}
		return latest.AddWithConfigHash(alias, branch, worktreePath, port, configHash, detach)
	}); err != nil {
		setupErr = err
		return result, setupErr
	}

	if !quiet {
		fmt.Println()
		fmt.Printf("Worktree %q ready (port %d).\n", alias, port)
		fmt.Printf("  cd $(grove cd %s)\n", alias)
	}

	return result, nil
}

// configuredWorktreePath returns the canonical path Grove derives for an alias.
func configuredWorktreePath(root string, cfg config.Config, alias string) (string, error) {
	name := alias
	if cfg.Prefix != "" {
		name = cfg.Prefix + "-" + alias
	}
	path, err := filepath.Abs(filepath.Join(root, cfg.WorktreeDir, name))
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		path = filepath.Join(resolved, filepath.Base(path))
	}
	return path, nil
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
	return runShellIO(command, dir, extraEnv, os.Stdout, os.Stderr)
}

func runShellIO(command, dir string, extraEnv []string, stdout, stderr io.Writer) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd.Run()
}
