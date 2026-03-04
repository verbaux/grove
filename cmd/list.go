package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
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

func runList(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}

	rows, err := buildWorktreeRows(root)
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		fmt.Println("No worktrees found.")
		return nil
	}

	fmt.Println(renderTable(rows))
	return nil
}

func renderTable(rows []worktreeRow) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("241"))
	idxStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	mainStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33"))   // blue
	nameStyle := lipgloss.NewStyle().Bold(true)
	cleanStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("34"))  // green
	dirtyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // orange

	// Calculate column widths dynamically based on content.
	idxW := len("#")
	nameW := len("NAME")
	branchW := len("BRANCH")
	pathW := len("PATH")

	for _, r := range rows {
		w := len(fmt.Sprintf("%d", r.Index))
		if w > idxW {
			idxW = w
		}
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
		if len(r.Branch) > branchW {
			branchW = len(r.Branch)
		}
		if len(r.Path) > pathW {
			pathW = len(r.Path)
		}
	}

	pad := func(s string, w int) string {
		return s + strings.Repeat(" ", w-len(s)+2)
	}

	var sb strings.Builder

	sb.WriteString(
		header.Render(pad("#", idxW)) +
			header.Render(pad("NAME", nameW)) +
			header.Render(pad("BRANCH", branchW)) +
			header.Render(pad("PATH", pathW)) +
			header.Render("STATUS") + "\n",
	)

	for _, r := range rows {
		statusStr := "✓ clean"
		statusRendered := cleanStyle.Render(statusStr)
		if r.Status != "clean" {
			statusStr = r.Status
			statusRendered = dirtyStyle.Render(statusStr)
		}

		name := nameStyle.Render(pad(r.Name, nameW))
		if r.IsMain {
			name = mainStyle.Render(pad(r.Name, nameW))
		}

		idx := idxStyle.Render(pad(fmt.Sprintf("%d", r.Index), idxW))

		sb.WriteString(
			idx +
				name +
				pad(r.Branch, branchW) +
				pad(r.Path, pathW) +
				statusRendered + "\n",
		)
	}

	return sb.String()
}
