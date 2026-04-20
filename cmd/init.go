package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

	copyDirsInput := prompt(
		"Directories to copy as build cache (comma-separated, leave empty for none) []",
		"",
	)
	cfg.CopyDirs = splitAndTrim(copyDirsInput)

	portRangeInput := prompt(
		fmt.Sprintf("Port range for worktrees (format min-max) [%d-%d]", config.DefaultPortMin, config.DefaultPortMax),
		fmt.Sprintf("%d-%d", config.DefaultPortMin, config.DefaultPortMax),
	)
	if pr, err := parsePortRange(portRangeInput); err == nil {
		if pr.Min != config.DefaultPortMin || pr.Max != config.DefaultPortMax {
			cfg.PortRange = &pr
		}
	} else {
		fmt.Printf("  Warning: %v — using defaults\n", err)
	}

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
	if len(cfg.CopyDirs) > 0 {
		fmt.Printf("  Copy dirs:    %s\n", strings.Join(cfg.CopyDirs, ", "))
	}
	if cfg.PortRange != nil {
		fmt.Printf("  Port range:   %d-%d\n", cfg.PortRange.Min, cfg.PortRange.Max)
	}
	if cfg.AfterCreate != "" {
		fmt.Printf("  After create: %s\n", cfg.AfterCreate)
	}
	agentsAnswer := prompt("Create AGENTS.md with AI coding agent instructions? [Y/n]", "y")
	if strings.ToLower(agentsAnswer) != "n" {
		if err := writeAgentsMd(cwd); err != nil {
			fmt.Printf("  Warning: could not write AGENTS.md: %v\n", err)
		} else {
			fmt.Println("  Created AGENTS.md")
		}
	}

	fmt.Println()
	fmt.Println("Next: grove create <branch>")

	return nil
}

func writeAgentsMd(dir string) error {
	path := filepath.Join(dir, "AGENTS.md")

	existing, err := os.ReadFile(path)
	if err == nil {
		if strings.Contains(string(existing), "## Grove") {
			return nil
		}
		return os.WriteFile(path, append(existing, []byte("\n"+groveAgentsSection)...), 0644)
	}

	return os.WriteFile(path, []byte("# AI Coding Agent Instructions\n\n"+groveAgentsSection), 0644)
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

// parsePortRange parses "min-max" into a PortRange.
func parsePortRange(s string) (config.PortRange, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return config.PortRange{}, fmt.Errorf("invalid port range %q, expected min-max", s)
	}
	minV, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	maxV, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return config.PortRange{}, fmt.Errorf("invalid port range %q", s)
	}
	if minV <= 0 || maxV < minV {
		return config.PortRange{}, fmt.Errorf("invalid port range %q", s)
	}
	return config.PortRange{Min: minV, Max: maxV}, nil
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
