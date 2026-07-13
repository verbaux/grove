package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output status as JSON")
}

var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show worktree health and freshness",
	Long: `Show a read-only daily status view for managed worktrees.

Includes dirty state, stale state paths, config drift, symlink health,
port assignment issues, orphan count, and branch freshness against the
local default branch.`,
	RunE: runStatus,
}

type statusSummary struct {
	Total          int `json:"total"`
	Dirty          int `json:"dirty"`
	Stale          int `json:"stale"`
	ConfigDrift    int `json:"configDrift"`
	SymlinkIssues  int `json:"symlinkIssues"`
	PortCollisions int `json:"portCollisions"`
	Orphans        int `json:"orphans"`
}

type statusRow struct {
	Alias          string `json:"alias"`
	Branch         string `json:"branch"`
	Path           string `json:"path"`
	Port           int    `json:"port,omitempty"`
	Protected      bool   `json:"protected,omitempty"`
	Note           string `json:"note,omitempty"`
	WorktreeStatus string `json:"worktreeStatus"`
	ConfigStatus   string `json:"configStatus"`
	SymlinkStatus  string `json:"symlinkStatus"`
	PortStatus     string `json:"portStatus"`
	Freshness      string `json:"freshness"`
}

type statusJSONResult struct {
	Base      string           `json:"base"`
	Summary   statusSummary    `json:"summary"`
	Worktrees []statusRow      `json:"worktrees"`
	Orphans   []orphanWorktree `json:"orphans"`
}

func runStatus(cmd *cobra.Command, args []string) error {
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

	result, err := buildStatus(root, cfg, s)
	if err != nil {
		return err
	}

	if statusJSON {
		return printJSON(result)
	}
	printStatus(result)
	return nil
}

func buildStatus(_ string, cfg config.Config, s state.State) (statusJSONResult, error) {
	base := git.DefaultBranch()
	orphans, err := findOrphans(s)
	if err != nil {
		return statusJSONResult{}, err
	}
	currentHash, err := config.SetupHash(cfg)
	if err != nil {
		return statusJSONResult{}, err
	}

	portCounts := make(map[int]int)
	for _, entry := range s.Worktrees {
		if entry.Port != 0 {
			portCounts[entry.Port]++
		}
	}

	aliases := make([]string, 0, len(s.Worktrees))
	for alias := range s.Worktrees {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	result := statusJSONResult{
		Base:      base,
		Summary:   statusSummary{Total: len(aliases), Orphans: len(orphans)},
		Worktrees: []statusRow{},
		Orphans:   orphans,
	}

	for _, alias := range aliases {
		entry := s.Worktrees[alias]
		row := statusRow{
			Alias:          alias,
			Branch:         entry.Branch,
			Path:           entry.Path,
			Port:           entry.Port,
			Protected:      entry.Protected,
			Note:           entry.Note,
			WorktreeStatus: worktreeStatus(entry.Path),
			ConfigStatus:   configStatus(entry.ConfigHash, currentHash),
			SymlinkStatus:  symlinkStatus(entry.Path, cfg),
			PortStatus:     portStatus(entry.Port, portCounts),
			Freshness:      freshnessStatus(entry.Branch, base),
		}
		if isDirtyStatus(row.WorktreeStatus) {
			result.Summary.Dirty++
		}
		if row.WorktreeStatus == "stale" {
			result.Summary.Stale++
		}
		if row.ConfigStatus == "drift" || row.ConfigStatus == "unknown" {
			result.Summary.ConfigDrift++
		}
		if row.SymlinkStatus != "ok" && row.SymlinkStatus != "none" && row.SymlinkStatus != "unknown" {
			result.Summary.SymlinkIssues++
		}
		if strings.HasPrefix(row.PortStatus, "collision") {
			result.Summary.PortCollisions++
		}
		result.Worktrees = append(result.Worktrees, row)
	}

	return result, nil
}

func isDirtyStatus(status string) bool {
	return status != "clean" && status != "stale" && status != "unknown"
}

func worktreeStatus(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "stale"
	}
	status, err := git.Status(path)
	if err != nil {
		return "unknown"
	}
	return status
}

func configStatus(storedHash, currentHash string) string {
	if storedHash == "" {
		return "unknown"
	}
	if storedHash != currentHash {
		return "drift"
	}
	return "ok"
}

func symlinkStatus(path string, cfg config.Config) string {
	if len(cfg.Symlink) == 0 {
		return "none"
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "unknown"
	}

	var missing, local, broken int
	for _, name := range cfg.Symlink {
		link := filepath.Join(path, name)
		info, err := os.Lstat(link)
		if errors.Is(err, os.ErrNotExist) {
			missing++
			continue
		}
		if err != nil {
			broken++
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			local++
			continue
		}
		target, err := os.Readlink(link)
		if err != nil {
			broken++
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(link), target)
		}
		if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
			broken++
		}
	}

	var parts []string
	if broken > 0 {
		parts = append(parts, fmt.Sprintf("%d broken", broken))
	}
	if missing > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", missing))
	}
	if local > 0 {
		parts = append(parts, fmt.Sprintf("%d local", local))
	}
	if len(parts) > 0 {
		return strings.Join(parts, ", ")
	}
	return "ok"
}

func portStatus(port int, counts map[int]int) string {
	if port == 0 {
		return "missing"
	}
	if counts[port] > 1 {
		return fmt.Sprintf("collision on %d", port)
	}
	return fmt.Sprintf("%d", port)
}

func freshnessStatus(branch, base string) string {
	if base == "" {
		return "unknown"
	}
	if branch == base {
		return "base"
	}
	if strings.HasPrefix(branch, "(") {
		return "detached"
	}
	ahead, behind, err := git.AheadBehind(branch, base)
	if err != nil {
		return "unknown"
	}
	switch {
	case ahead == 0 && behind == 0:
		return "up to date"
	case ahead == 0:
		return fmt.Sprintf("behind %d, no unique commits", behind)
	case behind == 0:
		return fmt.Sprintf("ahead %d", ahead)
	default:
		return fmt.Sprintf("ahead %d, behind %d", ahead, behind)
	}
}

func printStatus(result statusJSONResult) {
	if result.Summary.Total == 0 {
		fmt.Println("No managed worktrees.")
		if result.Summary.Orphans > 0 {
			fmt.Printf("%d orphan worktree(s) found — run 'grove adopt' or 'grove doctor --fix'.\n", result.Summary.Orphans)
		}
		return
	}

	fmt.Printf("Base: %s\n", fallback(result.Base, "unknown"))
	fmt.Printf("Managed: %d  Dirty: %d  Stale: %d  Config drift: %d  Symlink issues: %d  Port collisions: %d  Orphans: %d\n\n",
		result.Summary.Total,
		result.Summary.Dirty,
		result.Summary.Stale,
		result.Summary.ConfigDrift,
		result.Summary.SymlinkIssues,
		result.Summary.PortCollisions,
		result.Summary.Orphans,
	)
	fmt.Println(renderStatusTable(result.Worktrees))
}

func renderStatusTable(rows []statusRow) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("241"))
	nameStyle := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	cols := []string{"ALIAS", "BRANCH", "WORKTREE", "CONFIG", "SYMLINKS", "PORT", "FRESHNESS", "NOTE"}
	widths := make([]int, len(cols))
	for i, col := range cols {
		widths[i] = len(col)
	}
	for _, r := range rows {
		values := statusValues(r)
		for i, v := range values {
			if lipgloss.Width(v) > widths[i] {
				widths[i] = lipgloss.Width(v)
			}
		}
	}
	pad := func(s string, w int) string {
		return s + strings.Repeat(" ", w-lipgloss.Width(s)+2)
	}

	var sb strings.Builder
	for i, col := range cols {
		sb.WriteString(header.Render(pad(col, widths[i])))
	}
	sb.WriteByte('\n')
	for _, r := range rows {
		values := statusValues(r)
		values[0] = nameStyle.Render(pad(values[0], widths[0]))
		for i, v := range values {
			if i == 0 {
				sb.WriteString(v)
				continue
			}
			rendered := pad(v, widths[i])
			if v == "ok" || v == "clean" || v == "none" {
				rendered = dim.Render(rendered)
			}
			sb.WriteString(rendered)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func statusValues(r statusRow) []string {
	alias := r.Alias
	if r.Protected {
		alias += " (protected)"
	}
	return []string{
		alias,
		r.Branch,
		r.WorktreeStatus,
		r.ConfigStatus,
		r.SymlinkStatus,
		r.PortStatus,
		r.Freshness,
		displayNote(r.Note),
	}
}

func fallback(s, value string) string {
	if s == "" {
		return value
	}
	return s
}
