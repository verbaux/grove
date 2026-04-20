package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(doctorCmd)
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose grove configuration and worktree health",
	Long: `Run a set of checks against the current project's grove configuration,
state, and worktrees. Reports problems with config validity, missing paths,
orphan worktrees, broken symlinks, port collisions, and required tools.

Exit code is non-zero if any error-level issues are found.`,
	RunE: runDoctor,
}

type diagLevel string

const (
	levelOK    diagLevel = "ok"
	levelWarn  diagLevel = "warn"
	levelError diagLevel = "error"
)

type diagnostic struct {
	Level   diagLevel
	Message string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	var diags []diagnostic

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		printDiag(diagnostic{levelError, fmt.Sprintf("no .groverc.json found: %v", err)})
		return err
	}
	diags = append(diags, diagnostic{levelOK, fmt.Sprintf("project root: %s", root)})

	cfg, err := config.Load(root)
	if err != nil {
		diags = append(diags, diagnostic{levelError, fmt.Sprintf("config: %v", err)})
		printDiagnostics(diags)
		return err
	}
	diags = append(diags, diagnostic{levelOK, ".groverc.json valid"})

	diags = append(diags, diagConfigRange(cfg)...)

	s, err := state.Load(root)
	if err != nil {
		diags = append(diags, diagnostic{levelError, fmt.Sprintf("state: %v", err)})
		printDiagnostics(diags)
		return err
	}

	worktrees, wtErr := git.ListWorktrees()
	if wtErr != nil {
		diags = append(diags, diagnostic{levelError, fmt.Sprintf("git worktree list: %v", wtErr)})
	}

	diags = append(diags, diagStatePaths(s)...)
	if wtErr == nil {
		diags = append(diags, diagOrphans(s, worktrees)...)
	}
	diags = append(diags, diagSymlinks(root, cfg, s)...)
	diags = append(diags, diagPortCollisions(s)...)
	diags = append(diags, diagGhCLI())

	printDiagnostics(diags)

	for _, d := range diags {
		if d.Level == levelError {
			return errors.New("grove doctor found errors")
		}
	}
	return nil
}

func diagConfigRange(cfg config.Config) []diagnostic {
	var out []diagnostic
	if cfg.PortRange != nil {
		if cfg.PortRange.Min <= 0 || cfg.PortRange.Max < cfg.PortRange.Min {
			out = append(out, diagnostic{levelError, fmt.Sprintf("invalid portRange: min=%d max=%d", cfg.PortRange.Min, cfg.PortRange.Max)})
		}
	}
	return out
}

func diagStatePaths(s state.State) []diagnostic {
	var out []diagnostic
	missing := 0
	for alias, entry := range s.Worktrees {
		if _, err := os.Stat(entry.Path); errors.Is(err, os.ErrNotExist) {
			out = append(out, diagnostic{levelError, fmt.Sprintf("stale state: %q path missing: %s", alias, entry.Path)})
			missing++
		}
	}
	if missing == 0 && len(s.Worktrees) > 0 {
		out = append(out, diagnostic{levelOK, fmt.Sprintf("%d tracked worktree(s), all paths exist", len(s.Worktrees))})
	}
	if len(s.Worktrees) == 0 {
		out = append(out, diagnostic{levelOK, "no tracked worktrees"})
	}
	return out
}

func diagOrphans(s state.State, worktrees []git.Worktree) []diagnostic {
	tracked := make(map[string]bool)
	for _, e := range s.Worktrees {
		tracked[e.Path] = true
	}
	var out []diagnostic
	for _, wt := range worktrees {
		if wt.IsMain {
			continue
		}
		if !tracked[wt.Path] {
			out = append(out, diagnostic{levelWarn, fmt.Sprintf("orphan worktree: %s (branch %s) — run 'grove adopt'", wt.Path, wt.Branch)})
		}
	}
	if len(out) == 0 {
		out = append(out, diagnostic{levelOK, "no orphan worktrees"})
	}
	return out
}

func diagSymlinks(root string, cfg config.Config, s state.State) []diagnostic {
	var out []diagnostic
	checked := 0
	broken := 0
	for _, entry := range s.Worktrees {
		for _, name := range cfg.Symlink {
			link := filepath.Join(entry.Path, name)
			info, err := os.Lstat(link)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			checked++
			target, err := os.Readlink(link)
			if err != nil {
				out = append(out, diagnostic{levelWarn, fmt.Sprintf("cannot read symlink %s: %v", link, err)})
				broken++
				continue
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(link), target)
			}
			if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
				out = append(out, diagnostic{levelError, fmt.Sprintf("broken symlink: %s → %s", link, target)})
				broken++
			}
		}
	}
	if checked > 0 && broken == 0 {
		out = append(out, diagnostic{levelOK, fmt.Sprintf("%d symlink(s) checked, all healthy", checked)})
	}
	_ = root
	return out
}

func diagPortCollisions(s state.State) []diagnostic {
	var out []diagnostic
	portToAliases := make(map[int][]string)
	for alias, entry := range s.Worktrees {
		if entry.Port == 0 {
			continue
		}
		portToAliases[entry.Port] = append(portToAliases[entry.Port], alias)
	}
	collisions := 0
	for port, aliases := range portToAliases {
		if len(aliases) > 1 {
			out = append(out, diagnostic{levelError, fmt.Sprintf("port collision on %d: %v", port, aliases)})
			collisions++
		}
	}
	if collisions == 0 && len(portToAliases) > 0 {
		out = append(out, diagnostic{levelOK, "no port collisions"})
	}
	return out
}

func diagGhCLI() diagnostic {
	if _, err := exec.LookPath("gh"); err != nil {
		return diagnostic{levelWarn, "gh CLI not found — 'grove review' will not work"}
	}
	return diagnostic{levelOK, "gh CLI available"}
}

func printDiag(d diagnostic) {
	ok := lipgloss.NewStyle().Foreground(lipgloss.Color("34"))   // green
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // orange
	errS := lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red

	switch d.Level {
	case levelOK:
		fmt.Println(ok.Render("  ✓ ") + d.Message)
	case levelWarn:
		fmt.Println(warn.Render("  ⚠ ") + d.Message)
	case levelError:
		fmt.Println(errS.Render("  ✗ ") + d.Message)
	}
}

func printDiagnostics(diags []diagnostic) {
	for _, d := range diags {
		printDiag(d)
	}
	fmt.Println()
	var ok, warn, errCount int
	for _, d := range diags {
		switch d.Level {
		case levelOK:
			ok++
		case levelWarn:
			warn++
		case levelError:
			errCount++
		}
	}
	fmt.Printf("Summary: %d ok, %d warn, %d error\n", ok, warn, errCount)
}
