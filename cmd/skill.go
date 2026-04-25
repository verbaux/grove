package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// SkillContent is the embedded SKILL.md content. Set from main package at init.
var SkillContent string

type skillTarget struct {
	Name       string // short name: "claude", "codex"
	SkillsDir  string // parent skills directory: ~/.claude/skills
	ParentHint string // parent dir that must exist for autodetect: ~/.claude
}

func skillTargets() ([]skillTarget, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return []skillTarget{
		{
			Name:       "claude",
			SkillsDir:  filepath.Join(home, ".claude", "skills"),
			ParentHint: filepath.Join(home, ".claude"),
		},
		{
			Name:       "codex",
			SkillsDir:  filepath.Join(home, ".agents", "skills"),
			ParentHint: filepath.Join(home, ".agents"),
		},
	}, nil
}

var (
	skillTargetFlag string
	skillDirFlag    string
	skillForceFlag  bool
)

func init() {
	skillCmd.AddCommand(skillInstallCmd)
	skillCmd.AddCommand(skillUninstallCmd)
	skillCmd.AddCommand(skillPathCmd)

	skillInstallCmd.Flags().StringVar(&skillTargetFlag, "target", "", "install to specific agent: claude | codex")
	skillInstallCmd.Flags().StringVar(&skillDirFlag, "dir", "", "install into custom skills directory (overrides --target)")
	skillInstallCmd.Flags().BoolVarP(&skillForceFlag, "force", "f", false, "overwrite existing files without prompting")

	skillUninstallCmd.Flags().StringVar(&skillTargetFlag, "target", "", "uninstall from specific agent: claude | codex")
	skillUninstallCmd.Flags().StringVar(&skillDirFlag, "dir", "", "uninstall from custom skills directory (overrides --target)")
	skillUninstallCmd.Flags().BoolVarP(&skillForceFlag, "force", "f", false, "remove without prompting")

	rootCmd.AddCommand(skillCmd)
}

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage the grove agent skill",
	Long: `Install, uninstall, or inspect the grove agent skill.

The skill teaches AI coding agents (Claude Code, Codex CLI, etc.) to use grove
instead of raw git worktree commands. It follows the agentskills.io specification.`,
}

var skillInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the grove skill into agent skills directories",
	Long: `Install the grove skill into agent skills directories.

By default, installs into every detected agent skills directory. Use --target
to pick a specific agent, or --dir to install into a custom path.`,
	RunE: runSkillInstall,
}

var skillUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the grove skill from agent skills directories",
	RunE:  runSkillUninstall,
}

var skillPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the install paths for the grove skill",
	RunE:  runSkillPath,
}

// resolveSkillDirs returns the skill directories to act on, based on flags.
// Returns parent skills dirs (e.g. ~/.claude/skills), not the grove subdir.
func resolveSkillDirs(autodetect bool) ([]string, error) {
	if skillDirFlag != "" {
		return []string{skillDirFlag}, nil
	}

	targets, err := skillTargets()
	if err != nil {
		return nil, err
	}

	if skillTargetFlag != "" {
		for _, t := range targets {
			if t.Name == skillTargetFlag {
				return []string{t.SkillsDir}, nil
			}
		}
		names := make([]string, len(targets))
		for i, t := range targets {
			names[i] = t.Name
		}
		return nil, fmt.Errorf("unknown target %q (available: %s)", skillTargetFlag, strings.Join(names, ", "))
	}

	if !autodetect {
		return nil, nil
	}

	var dirs []string
	for _, t := range targets {
		if _, err := os.Stat(t.ParentHint); err == nil {
			dirs = append(dirs, t.SkillsDir)
		}
	}
	return dirs, nil
}

func runSkillInstall(cmd *cobra.Command, args []string) error {
	if SkillContent == "" {
		return fmt.Errorf("skill content not embedded — rebuild grove from source")
	}

	dirs, err := resolveSkillDirs(true)
	if err != nil {
		return err
	}

	if len(dirs) == 0 {
		targets, _ := skillTargets()
		fmt.Println("No agent skills directories detected. Tried:")
		for _, t := range targets {
			fmt.Printf("  %s → %s\n", t.Name, t.ParentHint)
		}
		fmt.Println()
		fmt.Println("Install into a custom path with: grove skill install --dir <path>")
		return nil
	}

	for _, skillsDir := range dirs {
		groveDir := filepath.Join(skillsDir, "grove")
		target := filepath.Join(groveDir, "SKILL.md")

		if _, err := os.Stat(target); err == nil && !skillForceFlag {
			answer := prompt(fmt.Sprintf("%s exists. Overwrite? [y/N]", target), "N")
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				fmt.Printf("  skipped %s\n", target)
				continue
			}
		}

		if err := os.MkdirAll(groveDir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", groveDir, err)
		}
		if err := os.WriteFile(target, []byte(SkillContent), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		fmt.Printf("  ✓ installed %s\n", target)
	}
	return nil
}

func runSkillUninstall(cmd *cobra.Command, args []string) error {
	dirs, err := resolveSkillDirs(true)
	if err != nil {
		return err
	}

	if len(dirs) == 0 {
		fmt.Println("No agent skills directories detected — nothing to uninstall.")
		return nil
	}

	for _, skillsDir := range dirs {
		groveDir := filepath.Join(skillsDir, "grove")
		if _, err := os.Stat(groveDir); os.IsNotExist(err) {
			continue
		}

		if !skillForceFlag {
			answer := prompt(fmt.Sprintf("Remove %s? [y/N]", groveDir), "N")
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				fmt.Printf("  skipped %s\n", groveDir)
				continue
			}
		}

		if err := os.RemoveAll(groveDir); err != nil {
			return fmt.Errorf("remove %s: %w", groveDir, err)
		}
		fmt.Printf("  ✓ removed %s\n", groveDir)
	}
	return nil
}

func runSkillPath(cmd *cobra.Command, args []string) error {
	targets, err := skillTargets()
	if err != nil {
		return err
	}

	for _, t := range targets {
		target := filepath.Join(t.SkillsDir, "grove", "SKILL.md")
		status := "(not installed)"
		if _, err := os.Stat(target); err == nil {
			status = "(installed)"
		} else if _, err := os.Stat(t.ParentHint); err == nil {
			status = "(detected, not installed)"
		}
		fmt.Printf("%-8s %s %s\n", t.Name, target, status)
	}
	return nil
}
