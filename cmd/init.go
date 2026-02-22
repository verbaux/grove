package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize grove in the current project",
	Long:  "Interactive wizard that creates .groverc.json with your preferences.",
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(cwd, config.FileName)); err == nil {
		fmt.Println(".groverc.json already exists in this directory.")
		answer := prompt("Overwrite? [y/N]", "n")
		if strings.ToLower(answer) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	cfg := config.Default()

	defaultPrefix := filepath.Base(cwd)
	cfg.Prefix = prompt(
		fmt.Sprintf("Prefix for worktree directories [%s]", defaultPrefix),
		defaultPrefix,
	)

	cfg.WorktreeDir = prompt(
		fmt.Sprintf("Where to place worktrees [%s]", cfg.WorktreeDir),
		cfg.WorktreeDir,
	)

	symlinkInput := prompt(
		fmt.Sprintf("Directories to symlink (comma-separated) [%s]", strings.Join(cfg.Symlink, ",")),
		strings.Join(cfg.Symlink, ","),
	)
	cfg.Symlink = splitAndTrim(symlinkInput)

	cfg.AfterCreate = prompt("Command to run after creating worktree (leave empty for none) []", "")

	if err := config.Save(cwd, cfg); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Created .groverc.json")
	fmt.Println()
	fmt.Printf("  Prefix:       %s\n", cfg.Prefix)
	fmt.Printf("  Worktree dir: %s\n", cfg.WorktreeDir)
	if len(cfg.Symlink) > 0 {
		fmt.Printf("  Symlink:      %s\n", strings.Join(cfg.Symlink, ", "))
	}
	if cfg.AfterCreate != "" {
		fmt.Printf("  After create: %s\n", cfg.AfterCreate)
	}
	fmt.Println()
	fmt.Println("Next: grove create <branch>")

	return nil
}

// prompt prints a question and reads one line from stdin.
// If the user presses Enter without typing, returns the default.
var reader *bufio.Reader

func prompt(question, defaultVal string) string {
	if reader == nil {
		reader = bufio.NewReader(os.Stdin)
	}

	fmt.Print(question + ": ")

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)

	if line == "" {
		return defaultVal
	}
	return line
}

// splitAndTrim splits a comma-separated string and trims whitespace from each part.
// "node_modules, .yarn" → ["node_modules", ".yarn"]
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
