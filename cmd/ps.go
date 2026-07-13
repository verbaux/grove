package cmd

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/processes"
	"github.com/verbaux/grove/internal/state"
)

var psJSON bool

func init() {
	rootCmd.AddCommand(psCmd)
	psCmd.Flags().BoolVar(&psJSON, "json", false, "output live worktree processes as JSON")
}

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "Show dev servers running on Grove-assigned ports",
	Long: `Show whether managed worktree ports have a local TCP listener.

Grove scans listening ports once using lsof, with netstat as a fallback.
This command is read-only and does not change port assignments or state.`,
	Args: cobra.NoArgs,
	RunE: runPS,
}

type psRow struct {
	Alias     string               `json:"alias"`
	Branch    string               `json:"branch"`
	Path      string               `json:"path"`
	Port      int                  `json:"port,omitempty"`
	Status    string               `json:"status"`
	Listeners []processes.Listener `json:"listeners"`
}

type psResult struct {
	Source    string  `json:"source,omitempty"`
	Worktrees []psRow `json:"worktrees"`
}

func runPS(_ *cobra.Command, _ []string) error {
	return runPSWithFinder(processes.FindListeners)
}

func runPSWithFinder(findListeners func() (processes.Snapshot, error)) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}
	s, err := state.Load(root)
	if err != nil {
		return err
	}
	if len(s.Worktrees) == 0 {
		result := psResult{Worktrees: []psRow{}}
		if psJSON {
			return printJSON(result)
		}
		fmt.Println("No managed worktrees.")
		return nil
	}

	snapshot, err := findListeners()
	if err != nil {
		return err
	}
	result := buildPS(s, snapshot)
	if psJSON {
		return printJSON(result)
	}
	fmt.Printf("Listener source: %s\n\n", terminalSafe(result.Source))
	fmt.Println(renderPSTable(result.Worktrees))
	return nil
}

func buildPS(s state.State, snapshot processes.Snapshot) psResult {
	aliases := make([]string, 0, len(s.Worktrees))
	for alias := range s.Worktrees {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	result := psResult{Source: snapshot.Source, Worktrees: make([]psRow, 0, len(aliases))}
	for _, alias := range aliases {
		entry := s.Worktrees[alias]
		listeners := snapshot.Listeners[entry.Port]
		if listeners == nil {
			listeners = []processes.Listener{}
		}
		status := "stopped"
		if entry.Port == 0 {
			status = "unassigned"
		} else if len(listeners) > 0 {
			status = "running"
		}
		result.Worktrees = append(result.Worktrees, psRow{
			Alias:     alias,
			Branch:    entry.Branch,
			Path:      entry.Path,
			Port:      entry.Port,
			Status:    status,
			Listeners: listeners,
		})
	}
	return result
}

func renderPSTable(rows []psRow) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("241"))
	running := lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	columns := []string{"ALIAS", "BRANCH", "PORT", "STATUS", "PID", "PROCESS"}
	widths := make([]int, len(columns))
	for i, column := range columns {
		widths[i] = len(column)
	}
	values := make([][]string, len(rows))
	for i, row := range rows {
		values[i] = psValues(row)
		for j, value := range values[i] {
			if len(value) > widths[j] {
				widths[j] = len(value)
			}
		}
	}
	pad := func(value string, width int) string {
		return value + strings.Repeat(" ", width-len(value)+2)
	}

	var output strings.Builder
	for i, column := range columns {
		output.WriteString(header.Render(pad(column, widths[i])))
	}
	output.WriteByte('\n')
	for i, row := range rows {
		for j, value := range values[i] {
			rendered := pad(value, widths[j])
			if j == 3 && row.Status == "running" {
				rendered = running.Render(rendered)
			} else if row.Status != "running" {
				rendered = dim.Render(rendered)
			}
			output.WriteString(rendered)
		}
		output.WriteByte('\n')
	}
	return output.String()
}

func psValues(row psRow) []string {
	port := "-"
	if row.Port != 0 {
		port = strconv.Itoa(row.Port)
	}
	var pids, commands []string
	for _, listener := range row.Listeners {
		if listener.PID != 0 {
			pids = append(pids, strconv.Itoa(listener.PID))
		}
		if listener.Command != "" {
			commands = append(commands, terminalSafe(listener.Command))
		}
	}
	return []string{
		terminalSafe(row.Alias),
		terminalSafe(row.Branch),
		port,
		row.Status,
		fallback(strings.Join(pids, ","), "-"),
		fallback(strings.Join(commands, ","), "-"),
	}
}
