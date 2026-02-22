package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all worktrees",
	Long:  "Show a table of all active worktrees with their branch, path, and git status.",
	RunE:  runList,
}

type row struct {
	name   string
	branch string
	path   string
	status string
	isMain bool
}

func runList(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	worktrees, err := git.ListWorktrees()
	if err != nil {
		return err
	}

	s, err := state.Load(root)
	if err != nil {
		return err
	}

	// Build a path → alias map from state for quick lookup.
	pathToAlias := make(map[string]string)
	for alias, entry := range s.Worktrees {
		pathToAlias[entry.Path] = alias
	}

	var rows []row
	for _, wt := range worktrees {
		name := pathToAlias[wt.Path]
		if name == "" {
			if wt.IsMain {
				name = "main"
			} else {
				name = "?"
			}
		}

		status, err := git.Status(wt.Path)
		if err != nil {
			status = "unknown"
		}

		rows = append(rows, row{
			name:   name,
			branch: wt.Branch,
			path:   wt.Path,
			status: status,
			isMain: wt.IsMain,
		})
	}

	if len(rows) == 0 {
		fmt.Println("No worktrees found.")
		return nil
	}

	fmt.Println(renderTable(rows))
	return nil
}

func renderTable(rows []row) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("241"))
	mainStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))   // blue
	nameStyle := lipgloss.NewStyle().Bold(true)
	cleanStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("34"))  // green
	dirtyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // orange

	// Calculate column widths dynamically based on content.
	nameW := len("NAME")
	branchW := len("BRANCH")
	pathW := len("PATH")

	for _, r := range rows {
		if len(r.name) > nameW {
			nameW = len(r.name)
		}
		if len(r.branch) > branchW {
			branchW = len(r.branch)
		}
		if len(r.path) > pathW {
			pathW = len(r.path)
		}
	}

	pad := func(s string, w int) string {
		return s + strings.Repeat(" ", w-len(s)+2)
	}

	var sb strings.Builder

	sb.WriteString(
		header.Render(pad("NAME", nameW)) +
			header.Render(pad("BRANCH", branchW)) +
			header.Render(pad("PATH", pathW)) +
			header.Render("STATUS") + "\n",
	)

	for _, r := range rows {
		statusStr := "✓ clean"
		statusRendered := cleanStyle.Render(statusStr)
		if r.status != "clean" {
			statusStr = r.status
			statusRendered = dirtyStyle.Render(statusStr)
		}

		name := nameStyle.Render(pad(r.name, nameW))
		if r.isMain {
			name = mainStyle.Render(pad(r.name, nameW))
		}

		sb.WriteString(
			name +
				pad(r.branch, branchW) +
				pad(r.path, pathW) +
				statusRendered + "\n",
		)
	}

	return sb.String()
}
