package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/detect"
	"github.com/verbaux/grove/internal/files"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "print diagnostics as JSON")
	doctorCmd.Flags().BoolVar(&doctorFixMode, "fix", false, "repair safe, unambiguous local issues")
}

var (
	doctorJSON    bool
	doctorFixMode bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose grove configuration and worktree health",
	Long: `Run a set of checks against the current project's grove configuration,
state, and worktrees. Reports problems with config validity, missing paths,
orphan worktrees, broken symlinks, port collisions, and required tools.

With --fix, Grove repairs safe, unambiguous local issues and then runs the
diagnostics again. It never removes a worktree.

Exit code is non-zero if any final error-level issues are found.`,
	RunE: runDoctor,
}

type diagLevel string

const (
	levelOK    diagLevel = "ok"
	levelWarn  diagLevel = "warn"
	levelError diagLevel = "error"
)

type diagnostic struct {
	Level   diagLevel `json:"level"`
	Message string    `json:"message"`
}

type doctorJSONResult struct {
	OK          bool         `json:"ok"`
	Diagnostics []diagnostic `json:"diagnostics"`
	Fixes       []doctorFix  `json:"fixes,omitempty"`
}

type doctorFix struct {
	Action  string `json:"action"`
	Target  string `json:"target"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	var diags []diagnostic

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		diags = append(diags, diagnostic{levelError, fmt.Sprintf("no .groverc.json found: %v", err)})
		if doctorJSON {
			_ = printJSON(doctorResult(diags))
		} else {
			printDiag(diags[0])
		}
		return err
	}
	diags = append(diags, diagnostic{levelOK, fmt.Sprintf("project root: %s", root)})

	cfg, err := config.Load(root)
	if err != nil {
		diags = append(diags, diagnostic{levelError, fmt.Sprintf("config: %v", err)})
		if doctorJSON {
			_ = printJSON(doctorResult(diags))
		} else {
			printDiagnostics(diags)
		}
		return err
	}
	diags = append(diags, diagnostic{levelOK, ".groverc.json valid"})

	diags = append(diags, diagConfigRange(cfg)...)
	symlinkPaths, pathDiags := expandDoctorPaths(root, cfg)

	s, err := state.Load(root)
	if err != nil {
		diags = append(diags, diagnostic{levelError, fmt.Sprintf("state: %v", err)})
		if doctorJSON {
			_ = printJSON(doctorResult(diags))
		} else {
			printDiagnostics(diags)
		}
		return err
	}

	worktrees, wtErr := git.ListWorktrees()
	if wtErr != nil {
		diags = append(diags, diagnostic{levelError, fmt.Sprintf("git worktree list: %v", wtErr)})
	}

	var fixes []doctorFix
	if doctorFixMode {
		staleFixes, fixDiags := fixStaleState(root, s)
		fixes = append(fixes, staleFixes...)
		diags = append(diags, fixDiags...)

		s, err = state.Load(root)
		if err != nil {
			diags = append(diags, diagnostic{levelError, fmt.Sprintf("state after fixes: %v", err)})
			return printDoctorOutcome(diags, fixes, err)
		}
		if wtErr == nil {
			worktrees, wtErr = git.ListWorktrees()
			if wtErr != nil {
				diags = append(diags, diagnostic{levelError, fmt.Sprintf("git worktree list after fixes: %v", wtErr)})
			} else {
				orphanFixes, orphanDiags := fixOrphanWorktrees(root, cfg, s, worktrees)
				fixes = append(fixes, orphanFixes...)
				diags = append(diags, orphanDiags...)

				s, err = state.Load(root)
				if err != nil {
					diags = append(diags, diagnostic{levelError, fmt.Sprintf("state after orphan fixes: %v", err)})
					return printDoctorOutcome(diags, fixes, err)
				}
				worktrees, wtErr = git.ListWorktrees()
				if wtErr != nil {
					diags = append(diags, diagnostic{levelError, fmt.Sprintf("git worktree list after orphan fixes: %v", wtErr)})
				}
			}
		}

		symlinkFixes, symlinkDiags := fixBrokenSymlinks(root, symlinkPaths, s)
		fixes = append(fixes, symlinkFixes...)
		diags = append(diags, symlinkDiags...)
	}

	diags = append(diags, diagStatePaths(s)...)
	if wtErr == nil {
		diags = append(diags, diagOrphans(s, worktrees)...)
	}
	diags = append(diags, diagConfigDrift(cfg, s)...)
	diags = append(diags, diagSymlinks(symlinkPaths, s)...)
	diags = append(diags, diagPortCollisions(s)...)
	diags = append(diags, pathDiags...)
	diags = append(diags, diagSuggestions(root, cfg)...)
	diags = append(diags, diagGhCLI())

	if doctorJSON {
		result := doctorResult(diags)
		result.Fixes = fixes
		if err := printJSON(result); err != nil {
			return err
		}
	} else {
		printDoctorFixes(fixes)
		printDiagnostics(diags)
	}

	for _, d := range diags {
		if d.Level == levelError {
			return errors.New("grove doctor found errors")
		}
	}
	return nil
}

func fixStaleState(root string, snapshot state.State) ([]doctorFix, []diagnostic) {
	candidates := make([]string, 0)
	paths := make(map[string]string)
	for alias, entry := range snapshot.Worktrees {
		if _, err := os.Stat(entry.Path); errors.Is(err, os.ErrNotExist) {
			candidates = append(candidates, alias)
			paths[alias] = entry.Path
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Strings(candidates)

	var removed []string
	skipped := make(map[string]string)
	err := state.Update(root, func(latest *state.State) error {
		for _, alias := range candidates {
			entry, ok := latest.Get(alias)
			if !ok {
				skipped[alias] = "state entry was already removed"
				continue
			}
			if entry.Path != paths[alias] {
				skipped[alias] = "state entry changed before repair"
				continue
			}
			if _, statErr := os.Stat(entry.Path); !errors.Is(statErr, os.ErrNotExist) {
				skipped[alias] = "worktree path exists again"
				continue
			}
			if err := latest.Remove(alias); err != nil {
				return err
			}
			removed = append(removed, alias)
		}
		return nil
	})
	if err != nil {
		fixes := make([]doctorFix, 0, len(candidates))
		for _, alias := range candidates {
			fixes = append(fixes, doctorFix{
				Action: "remove-stale-state", Target: alias, Status: "failed", Message: err.Error(),
			})
		}
		return fixes, []diagnostic{{levelError, fmt.Sprintf("fix stale state: %v", err)}}
	}

	fixes := make([]doctorFix, 0, len(candidates))
	removedSet := make(map[string]bool, len(removed))
	for _, alias := range removed {
		removedSet[alias] = true
	}
	for _, alias := range candidates {
		if removedSet[alias] {
			fixes = append(fixes, doctorFix{
				Action: "remove-stale-state", Target: alias, Status: "fixed",
				Message: fmt.Sprintf("removed stale state entry %q", alias),
			})
			continue
		}
		fixes = append(fixes, doctorFix{
			Action: "remove-stale-state", Target: alias, Status: "skipped", Message: skipped[alias],
		})
	}
	return fixes, nil
}

type brokenSymlinkCandidate struct {
	alias  string
	branch string
	path   string
	name   string
	link   string
}

func fixBrokenSymlinks(root string, symlinks []string, snapshot state.State) ([]doctorFix, []diagnostic) {
	aliases := make([]string, 0, len(snapshot.Worktrees))
	for alias := range snapshot.Worktrees {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	var candidates []brokenSymlinkCandidate
	seen := make(map[string]bool)
	for _, alias := range aliases {
		entry := snapshot.Worktrees[alias]
		for _, name := range symlinks {
			link := filepath.Join(entry.Path, name)
			if seen[link] {
				continue
			}
			seen[link] = true
			info, err := os.Lstat(link)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			target, err := os.Readlink(link)
			if err != nil {
				continue
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(link), target)
			}
			if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
				continue
			}
			candidates = append(candidates, brokenSymlinkCandidate{
				alias: alias, branch: entry.Branch, path: entry.Path, name: name, link: link,
			})
		}
	}

	fixes := make([]doctorFix, 0, len(candidates))
	var diags []diagnostic
	for _, candidate := range candidates {
		latest, err := state.Load(root)
		if err != nil {
			fixes = append(fixes, doctorFix{
				Action: "repair-symlink", Target: candidate.link, Status: "failed", Message: err.Error(),
			})
			diags = append(diags, diagnostic{levelError, fmt.Sprintf("fix symlink %s: %v", candidate.link, err)})
			continue
		}
		entry, ok := latest.Get(candidate.alias)
		if !ok || entry.Path != candidate.path || entry.Branch != candidate.branch {
			fixes = append(fixes, doctorFix{
				Action: "repair-symlink", Target: candidate.link, Status: "skipped",
				Message: "state entry changed before repair",
			})
			continue
		}

		repaired, err := files.RepairBrokenSymlink(root, candidate.path, candidate.name)
		if err != nil {
			fixes = append(fixes, doctorFix{
				Action: "repair-symlink", Target: candidate.link, Status: "failed", Message: err.Error(),
			})
			diags = append(diags, diagnostic{levelError, fmt.Sprintf("fix symlink %s: %v", candidate.link, err)})
			continue
		}
		if repaired {
			fixes = append(fixes, doctorFix{
				Action: "repair-symlink", Target: candidate.link, Status: "fixed",
				Message: fmt.Sprintf("repaired symlink to %s", filepath.Join(root, candidate.name)),
			})
			continue
		}

		message := "symlink is no longer broken"
		if _, err := os.Stat(filepath.Join(root, candidate.name)); errors.Is(err, os.ErrNotExist) {
			message = "canonical target is missing in the main worktree"
		}
		fixes = append(fixes, doctorFix{
			Action: "repair-symlink", Target: candidate.link, Status: "skipped", Message: message,
		})
	}
	return fixes, diags
}

func printDoctorOutcome(diags []diagnostic, fixes []doctorFix, cause error) error {
	if doctorJSON {
		result := doctorResult(diags)
		result.Fixes = fixes
		_ = printJSON(result)
	} else {
		printDoctorFixes(fixes)
		printDiagnostics(diags)
	}
	return cause
}

func doctorResult(diags []diagnostic) doctorJSONResult {
	ok := true
	for _, d := range diags {
		if d.Level == levelError {
			ok = false
			break
		}
	}
	return doctorJSONResult{OK: ok, Diagnostics: diags}
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

func diagConfigDrift(cfg config.Config, s state.State) []diagnostic {
	if len(s.Worktrees) == 0 {
		return nil
	}

	currentHash, err := config.SetupHash(cfg)
	if err != nil {
		return []diagnostic{{levelError, fmt.Sprintf("config hash: %v", err)}}
	}

	var out []diagnostic
	var checked int
	for alias, entry := range s.Worktrees {
		if entry.ConfigHash == "" {
			out = append(out, diagnostic{
				Level:   levelWarn,
				Message: fmt.Sprintf("unknown config version for %q — recreate or sync this worktree to track config drift", alias),
			})
			continue
		}
		checked++
		if entry.ConfigHash != currentHash {
			out = append(out, diagnostic{
				Level:   levelWarn,
				Message: fmt.Sprintf("config drift for %q — worktree was created with an older .groverc.json setup hash", alias),
			})
		}
	}
	if len(out) == 0 && checked > 0 {
		out = append(out, diagnostic{levelOK, "no config drift"})
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

func diagSymlinks(symlinks []string, s state.State) []diagnostic {
	var out []diagnostic
	checked := 0
	broken := 0
	for _, entry := range s.Worktrees {
		for _, name := range symlinks {
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

func expandDoctorPaths(root string, cfg config.Config) ([]string, []diagnostic) {
	var symlinkPaths []string
	var out []diagnostic
	appendWarnings := func(field string, warnings []config.PathExpansionWarning) {
		for _, warning := range warnings {
			if warning.Path == "" {
				if !config.HasGlobMeta(warning.Pattern) {
					full := filepath.Join(root, warning.Pattern)
					message := fmt.Sprintf("%s %q has no source in main repo: %s", field, warning.Pattern, full)
					if field == "symlink" {
						message = fmt.Sprintf("symlink %q has no target in main repo: %s — symlinks to it will break", warning.Pattern, full)
					}
					out = append(out, diagnostic{Level: levelWarn, Message: message})
					continue
				}
				out = append(out, diagnostic{
					Level:   levelWarn,
					Message: fmt.Sprintf("%s pattern %q %s in main repo", field, warning.Pattern, warning.Reason),
				})
				continue
			}
			out = append(out, diagnostic{
				Level: levelWarn,
				Message: fmt.Sprintf("%s %q matched by %q: %s", field, warning.Path,
					warning.Pattern, warning.Reason),
			})
		}
	}
	symlinks, err := config.ExpandSymlink(root, cfg)
	if err != nil {
		out = append(out, diagnostic{levelError, fmt.Sprintf("symlink: %v", err)})
	} else {
		symlinkPaths = inspectionPaths(symlinks)
		appendWarnings("symlink", symlinks.Warnings)
	}
	copyDirs, err := config.ExpandCopyDirs(root, cfg)
	if err != nil {
		out = append(out, diagnostic{levelError, fmt.Sprintf("copyDir: %v", err)})
	} else {
		appendWarnings("copyDir", copyDirs.Warnings)
	}
	return symlinkPaths, out
}

func diagSuggestions(root string, cfg config.Config) []diagnostic {
	sugs := detect.Adapt(cfg.Symlink, detect.Analyze(root))
	var out []diagnostic
	for _, s := range sugs {
		if configHasSuggestion(cfg, s) {
			continue
		}
		out = append(out, diagnostic{
			Level:   levelWarn,
			Message: fmt.Sprintf("suggested %s %q — %s", s.Kind, s.Value, s.Reason),
		})
	}
	return out
}

func configHasSuggestion(cfg config.Config, s detect.Suggestion) bool {
	return configCoversSuggestion(cfg, s)
}

func diagGhCLI() diagnostic {
	if _, err := exec.LookPath("gh"); err != nil {
		return diagnostic{levelWarn, "gh CLI not found — 'grove review' will not work"}
	}
	return diagnostic{levelOK, "gh CLI available"}
}

func printDiag(d diagnostic) {
	ok := lipgloss.NewStyle().Foreground(lipgloss.Color("34"))    // green
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

func printDoctorFixes(fixes []doctorFix) {
	if len(fixes) == 0 {
		return
	}
	fmt.Println("Fixes:")
	for _, fix := range fixes {
		fmt.Printf("  %s %s: %s\n", fix.Status, fix.Action, fix.Message)
	}
	fmt.Println()
}
