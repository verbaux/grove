package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
)

var openEditor string

func init() {
	openCmd.Flags().StringVar(&openEditor, "editor", "", "Editor command to use (overrides config and $VISUAL/$EDITOR)")
	rootCmd.AddCommand(openCmd)
}

var openCmd = &cobra.Command{
	Use:   "open [name-or-number]",
	Short: "Open a worktree in your editor",
	Long: `Open a worktree in your editor.

Resolves the worktree by alias or index (or shows an interactive picker when
no argument is given), then launches your editor in that directory.

Editor resolution order:
  --editor flag → "editor" in .groverc.json → $VISUAL → $EDITOR`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeAliases,
	RunE:              runOpen,
}

func runOpen(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}

	path, err := resolveWorktreePath(root, arg)
	if err != nil {
		return err
	}
	if path == "" {
		return nil // picker cancelled
	}

	editor, err := resolveEditor(root)
	if err != nil {
		return err
	}

	sh, err := exec.LookPath("sh")
	if err != nil {
		return err
	}

	// Replace the grove process with the editor so terminal editors
	// (vim, nvim) attach to the tty correctly. The editor string may
	// carry its own flags (e.g. "code -w"), so run it through sh.
	line := fmt.Sprintf("exec %s %s", editor, shellQuote(path))
	return syscall.Exec(sh, []string{"sh", "-c", line}, os.Environ())
}

// resolveEditor picks the editor command following the documented precedence.
func resolveEditor(root string) (string, error) {
	if openEditor != "" {
		return openEditor, nil
	}
	if cfg, err := config.Load(root); err == nil && cfg.Editor != "" {
		return cfg.Editor, nil
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v, nil
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e, nil
	}
	return "", fmt.Errorf(`no editor configured — set $EDITOR or $VISUAL, add "editor" to .groverc.json, or pass --editor`)
}

// shellQuote wraps s in single quotes, safe for use in an sh command line.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
