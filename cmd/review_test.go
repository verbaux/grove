package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/verbaux/grove/internal/config"
)

func TestReviewBranchIsolatesForkPRs(t *testing.T) {
	fork := reviewBranch(prInfo{Number: 42, HeadRefName: "main", IsCrossRepository: true})
	if fork != "pr-42" {
		t.Errorf("fork PR branch = %q, want pr-42", fork)
	}
	same := reviewBranch(prInfo{Number: 7, HeadRefName: "feature/x"})
	if same != "feature/x" {
		t.Errorf("same-repo PR branch = %q, want feature/x", same)
	}
}

func TestSyncReviewBranchFastForwardsStaleBranch(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{WorktreeDir: "../", Symlink: []string{}})
	runGit := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("branch", "feature/stale")
	runGit("commit", "--allow-empty", "-m", "newer PR commit")
	tip := runGit("rev-parse", "HEAD")

	if _, err := captureStdout(t, func() error { return syncReviewBranch("feature/stale", "main") }); err != nil {
		t.Fatal(err)
	}
	if got := runGit("rev-parse", "feature/stale"); got != tip {
		t.Errorf("feature/stale = %s, want %s", got, tip)
	}
}

func TestReviewCheckoutJSON(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{WorktreeDir: "../", Prefix: "rv", Symlink: []string{}})
	for _, args := range [][]string{{"branch", "feature/pr"}, {"remote", "add", "origin", dir}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	bin := t.TempDir()
	gh := "#!/bin/sh\necho '{\"headRefName\":\"feature/pr\",\"number\":7,\"title\":\"T\",\"isCrossRepository\":false}'\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	reviewName, reviewJSON = "", true
	t.Cleanup(func() { reviewJSON = false })

	out, err := captureStdout(t, func() error { return runReviewCheckout("7") })
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Alias, Branch, Path string
		PR                  int
		Fork                bool
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("review --json stdout is not JSON: %v\n%s", err, out)
	}
	if result.Alias != "pr-7" || result.Branch != "feature/pr" || result.PR != 7 || result.Fork || result.Path == "" {
		t.Fatalf("unexpected review JSON: %+v", result)
	}
}

func TestRenderPRTableTruncatesLongUnicodeTitle(t *testing.T) {
	title := strings.Repeat("я", 80)
	output := ansi.Strip(renderPRTable([]prListItem{{
		Number:      1,
		Title:       title,
		HeadRefName: "feature",
	}}))

	if !utf8.ValidString(output) {
		t.Fatalf("rendered table is not valid UTF-8: %q", output)
	}
	if !strings.Contains(output, strings.Repeat("я", 59)+"…") {
		t.Errorf("rendered table does not truncate title to 60 cells with ellipsis: %q", output)
	}
}

func TestRenderPRTableSanitizesExternalValues(t *testing.T) {
	output := renderPRTable([]prListItem{{
		Number:      1,
		Title:       "title\x1b[31m",
		HeadRefName: "branch\x1b[0m",
	}})
	item := prListItem{Number: 2, Title: "title", HeadRefName: "branch"}
	item.Author.Login = "author\x1b[31m"
	output += renderPRTable([]prListItem{item})

	if strings.ContainsRune(output, '\x1b') {
		t.Errorf("rendered table contains raw ESC byte: %q", output)
	}
	if !strings.Contains(ansi.Strip(output), "title?[31m") || !strings.Contains(ansi.Strip(output), "author?[31m") || !strings.Contains(ansi.Strip(output), "branch?[0m") {
		t.Errorf("rendered table is missing sanitized external values: %q", ansi.Strip(output))
	}
}

func TestRenderPRTableAlignsAuthorAfterUnicodeTitles(t *testing.T) {
	items := []prListItem{
		{Number: 1, Title: "ASCII title"},
		{Number: 2, Title: "Кириллический заголовок"},
	}
	items[0].Author.Login = "alice"
	items[1].Author.Login = "боб"
	lines := strings.Split(strings.TrimSuffix(ansi.Strip(renderPRTable(items)), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("rendered %d lines, want header and two rows", len(lines))
	}

	starts := make([]int, 0, len(items))
	for i, item := range items {
		login := item.Author.Login
		index := strings.Index(lines[i+1], login)
		if index < 0 {
			t.Fatalf("row %d does not contain author %q: %q", i, login, lines[i+1])
		}
		starts = append(starts, lipgloss.Width(lines[i+1][:index]))
	}
	if starts[0] != starts[1] {
		t.Errorf("AUTHOR starts at display columns %v, want equal columns", starts)
	}
}
