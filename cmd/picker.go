package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pickerModel struct {
	input    textinput.Model
	rows     []worktreeRow
	filtered []worktreeRow
	cursor   int
	selected string
	quitted  bool
}

func newPicker(rows []worktreeRow) pickerModel {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.Focus()

	return pickerModel{
		input:    ti,
		rows:     rows,
		filtered: rows,
	}
}

func (m pickerModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitted = true
			return m, tea.Quit
		case "enter":
			if len(m.filtered) > 0 {
				m.selected = m.filtered[m.cursor].Path
			}
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	m.filtered = filterRows(m.rows, m.input.Value())
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}

	return m, cmd
}

func (m pickerModel) View() string {
	var sb strings.Builder

	sb.WriteString(m.input.View())
	sb.WriteString("\n\n")

	if len(m.filtered) == 0 {
		sb.WriteString(dimStyle.Render("  no matches"))
		sb.WriteString("\n")
	} else {
		for i, r := range m.filtered {
			cursor := "  "
			if i == m.cursor {
				cursor = cursorStyle.Render("> ")
			}

			name := nameHighStyle.Render(r.Name)
			branch := dimStyle.Render(r.Branch)

			status := cleanMark
			if r.Status != "clean" {
				status = dirtyMark.Render("● " + r.Status)
			}

			sb.WriteString(fmt.Sprintf("%s%-14s %s  %s\n", cursor, name, branch, status))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("↑/↓ navigate • enter select • esc quit"))

	return sb.String()
}

func filterRows(rows []worktreeRow, query string) []worktreeRow {
	if query == "" {
		return rows
	}
	q := strings.ToLower(query)
	var out []worktreeRow
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Name), q) ||
			strings.Contains(strings.ToLower(r.Branch), q) {
			out = append(out, r)
		}
	}
	return out
}

var (
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	nameHighStyle = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	cleanMark    = dimStyle.Render("✓")
	dirtyMark    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)
