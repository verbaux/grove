package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(execCmd)
}

var execCmd = &cobra.Command{
	Use:   "exec <name-or-number> -- <command> [args...]",
	Short: "Run a command inside a worktree",
	Long: `Run a command with the worktree as its working directory.

The command gets GROVE_PATH and GROVE_BRANCH, plus GROVE_ALIAS and GROVE_PORT
for managed worktrees, and replaces the grove process, so its exit code and
output pass through unchanged. It runs without a shell; use sh -c for pipes:

  grove exec auth -- npm test
  grove exec 2 -- sh -c 'npm run dev -- --port "$GROVE_PORT"'`,
	Args: func(cmd *cobra.Command, args []string) error {
		if cmd.ArgsLenAtDash() != 1 || len(args) < 2 {
			return fmt.Errorf("usage: grove exec <name-or-number> -- <command> [args...]")
		}
		return nil
	},
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return completeAliases(cmd, args, toComplete)
		}
		return nil, cobra.ShellCompDirectiveDefault // the command and its arguments
	},
	RunE: runExec,
}

func runExec(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	dir, env, err := execTarget(root, args[0])
	if err != nil {
		return err
	}
	// Chdir first so relative commands like ./script.sh resolve in the worktree.
	if err := os.Chdir(dir); err != nil {
		return err
	}
	bin, err := exec.LookPath(args[1])
	if err != nil {
		return err
	}
	// ponytail: syscall.Exec is unsupported on Windows, same as grove open;
	// switch to exec.Command with exit-code passthrough if Windows users need it.
	return syscall.Exec(bin, args[1:], env)
}

// execTarget resolves the worktree directory and the environment for grove exec.
func execTarget(root, query string) (string, []string, error) {
	resolved, err := resolveWorktree(root, query)
	if err != nil {
		return "", nil, err
	}
	if resolved == nil {
		return "", nil, fmt.Errorf("no worktree matching %q — run 'grove list' to see available worktrees", query)
	}

	grove := []string{"GROVE_PATH=" + resolved.Path, "GROVE_BRANCH=" + resolved.Branch}
	if resolved.InState {
		s, err := state.Load(root)
		if err != nil {
			return "", nil, err
		}
		if entry, ok := s.Get(resolved.Alias); ok {
			grove = append(grove, "GROVE_ALIAS="+resolved.Alias, fmt.Sprintf("GROVE_PORT=%d", entry.Port))
		}
	}

	// Drop inherited worktree variables so a nested call (e.g. from a hook)
	// cannot leak another worktree's values; duplicate keys resolve to the first.
	env := make([]string, 0, len(os.Environ())+len(grove))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "GROVE_PATH", "GROVE_BRANCH", "GROVE_ALIAS", "GROVE_PORT":
		default:
			env = append(env, kv)
		}
	}
	return resolved.Path, append(env, grove...), nil
}
