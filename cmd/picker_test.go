package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPickWorktreeFailsFastWithoutTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	_, err = pickWorktree(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "pass an alias or index") {
		t.Fatalf("expected non-terminal error, got %v", err)
	}
}

func TestPickerViewAlignsBranchesForWideNames(t *testing.T) {
	rows := []worktreeRow{
		{Name: "auth", Branch: "feature/auth", Status: "clean"},
		{Name: "認証", Branch: "feature/fix", Status: "clean"},
	}
	view := newPicker(rows).View()

	columns := map[string]int{}
	for _, line := range strings.Split(view, "\n") {
		for _, branch := range []string{"feature/auth", "feature/fix"} {
			if i := strings.Index(line, branch); i >= 0 {
				columns[branch] = lipgloss.Width(line[:i])
			}
		}
	}
	if len(columns) != 2 {
		t.Fatalf("expected both branches in view, got %q", view)
	}
	if columns["feature/auth"] != columns["feature/fix"] {
		t.Errorf("branch columns differ: %v\n%s", columns, view)
	}
}
