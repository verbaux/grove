package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func TestValidateAliasRejectsUnsafeNames(t *testing.T) {
	tests := []string{
		"",
		".",
		"..",
		"../escape",
		"nested/name",
		`nested\name`,
		" leading",
		"trailing ",
		"line\nbreak",
	}
	for _, alias := range tests {
		t.Run(strings.ReplaceAll(alias, "\n", `\n`), func(t *testing.T) {
			if err := validateAlias(alias); err == nil {
				t.Fatalf("validateAlias(%q) succeeded, want error", alias)
			}
		})
	}
}

func TestValidateAliasAcceptsFileNameSafeNames(t *testing.T) {
	for _, alias := range []string{"auth", "payment-redesign", "release.v2", "feature_42"} {
		if err := validateAlias(alias); err != nil {
			t.Fatalf("validateAlias(%q) = %v", alias, err)
		}
	}
}

func TestDisplayNoteSanitizesControlCharacters(t *testing.T) {
	if got := displayNote("waiting\x1b[31m\nreview"); got != "waiting?[31m?review" {
		t.Fatalf("displayNote() = %q", got)
	}
}

func TestCompleteAliases(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
	})

	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"auth":     {Branch: "feature/auth", Path: filepath.Join(dir, "auth")},
		"payments": {Branch: "feature/payments", Path: filepath.Join(dir, "payments")},
	}}
	if err := state.Save(dir, s); err != nil {
		t.Fatal(err)
	}

	completions, directive := completeAliases(&cobra.Command{}, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(completions) != 2 {
		t.Fatalf("expected 2 completions, got %d: %v", len(completions), completions)
	}

	got := map[string]bool{}
	for _, c := range completions {
		got[c] = true
	}
	if !got["auth"] || !got["payments"] {
		t.Errorf("expected aliases [auth, payments], got %v", completions)
	}
}

func TestCompleteAliasesEmpty(t *testing.T) {
	setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
	})

	completions, directive := completeAliases(&cobra.Command{}, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(completions) != 0 {
		t.Errorf("expected 0 completions, got %v", completions)
	}
}

func TestCompleteOrphans(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
	})

	// Create a worktree directly with git (not grove) so it becomes an orphan.
	wtPath := filepath.Join(filepath.Dir(dir), "testproject-orphan")
	c := exec.Command("git", "worktree", "add", "-b", "feature/orphan", wtPath)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}
	t.Cleanup(func() {
		rm := exec.Command("git", "worktree", "remove", "--force", wtPath)
		rm.Dir = dir
		rm.CombinedOutput()
	})

	completions, directive := completeOrphans(&cobra.Command{}, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(completions) != 1 {
		t.Fatalf("expected 1 orphan completion, got %d: %v", len(completions), completions)
	}
	if completions[0] != "feature/orphan" {
		t.Errorf("expected orphan branch 'feature/orphan', got %q", completions[0])
	}
}

func TestCompleteOrphansIgnoresTracked(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
	})

	// Create a worktree via git, then register it in state (not an orphan).
	wtPath := filepath.Join(filepath.Dir(dir), "testproject-tracked")
	c := exec.Command("git", "worktree", "add", "-b", "feature/tracked", wtPath)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}
	t.Cleanup(func() {
		rm := exec.Command("git", "worktree", "remove", "--force", wtPath)
		rm.Dir = dir
		rm.CombinedOutput()
	})

	absWtPath, _ := filepath.EvalSymlinks(wtPath)
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"tracked": {Branch: "feature/tracked", Path: absWtPath},
	}}
	if err := state.Save(dir, s); err != nil {
		t.Fatal(err)
	}

	completions, _ := completeOrphans(&cobra.Command{}, nil, "")

	if len(completions) != 0 {
		t.Errorf("expected 0 orphan completions (worktree is tracked), got %v", completions)
	}
}

func TestCompleteAliasesNoRoot(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(dir)

	completions, directive := completeAliases(&cobra.Command{}, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(completions) != 0 {
		t.Errorf("expected 0 completions when no grove root, got %v", completions)
	}
}

func TestResolveWorktreeManagedPathSpellings(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{WorktreeDir: "../", Prefix: "testproject"})
	wtPath := filepath.Join(filepath.Dir(root), "testproject-managed-path")
	addTestWorktree(t, root, wtPath, "feature/managed-path")
	if err := state.Save(root, state.State{Worktrees: map[string]state.WorktreeEntry{
		"managed-path": {Branch: "feature/managed-path", Path: wtPath},
	}}); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{"../testproject-managed-path", wtPath + string(os.PathSeparator)} {
		resolved, err := resolveWorktree(root, query)
		if err != nil || resolved == nil || resolved.Alias != "managed-path" {
			t.Fatalf("resolveWorktree(%q) = %#v, %v; want managed-path", query, resolved, err)
		}
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	})
	if err := os.Chdir(wtPath); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveWorktree(root, ".")
	if err != nil || resolved == nil || resolved.Alias != "managed-path" {
		t.Fatalf("resolveWorktree(\".\") = %#v, %v; want managed-path", resolved, err)
	}
}

func TestResolveWorktreeOrphanRelativePath(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{WorktreeDir: "../", Prefix: "testproject"})
	wtPath := filepath.Join(filepath.Dir(root), "testproject-orphan-path")
	addTestWorktree(t, root, wtPath, "feature/orphan-path")

	resolved, err := resolveWorktree(root, "../testproject-orphan-path")
	if err != nil || resolved == nil || resolved.InState || resolved.Branch != "feature/orphan-path" {
		t.Fatalf("resolveWorktree(relative orphan path) = %#v, %v", resolved, err)
	}
}

func TestResolveWorktreeStaleManagedRelativePath(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{WorktreeDir: "../", Prefix: "testproject"})
	wtPath := filepath.Join(filepath.Dir(root), "testproject-stale-path")
	addTestWorktree(t, root, wtPath, "feature/stale-path")
	if err := state.Save(root, state.State{Worktrees: map[string]state.WorktreeEntry{
		"stale-path": {Branch: "feature/stale-path", Path: wtPath},
	}}); err != nil {
		t.Fatal(err)
	}
	rm := exec.Command("git", "worktree", "remove", "--force", wtPath)
	rm.Dir = root
	if out, err := rm.CombinedOutput(); err != nil {
		t.Fatalf("git worktree remove: %s", out)
	}

	resolved, err := resolveWorktree(root, "../testproject-stale-path")
	if err != nil || resolved == nil || resolved.Alias != "stale-path" {
		t.Fatalf("resolveWorktree(relative stale path) = %#v, %v; want stale-path", resolved, err)
	}
}

func TestResolveWorktreeDoesNotResolveMainByPath(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{WorktreeDir: "../", Prefix: "testproject"})
	resolved, err := resolveWorktree(root, ".")
	if err != nil || resolved != nil {
		t.Fatalf("resolveWorktree(main path) = %#v, %v; want no match", resolved, err)
	}
}

func TestBuildWorktreeRowsStatusOrderAndErrors(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{WorktreeDir: "../", Prefix: "testproject"})
	trackedPath := filepath.Join(root, "tracked.txt")
	if err := os.WriteFile(trackedPath, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Commit .groverc.json too so the main worktree is clean without relying on a global gitignore.
	for _, args := range [][]string{{"git", "add", "-f", "tracked.txt", ".groverc.json"}, {"git", "commit", "-m", "add tracked file"}} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}

	paths := make([]string, 5)
	for i := range paths {
		paths[i] = filepath.Join(filepath.Dir(root), "testproject-row-"+strconv.Itoa(i))
		addTestWorktree(t, root, paths[i], "feature/row-"+strconv.Itoa(i))
	}
	if err := os.WriteFile(filepath.Join(paths[1], "untracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths[2], "tracked.txt"), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(paths[4]); err != nil {
		t.Fatalf("remove worktree directory: %v", err)
	}

	worktrees, err := git.ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := buildWorktreeRows(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(worktrees) {
		t.Fatalf("got %d rows, want %d worktrees", len(rows), len(worktrees))
	}
	wantStatus := map[string]string{
		root:     "clean",
		paths[0]: "clean",
		paths[1]: "1 untracked",
		paths[2]: "1 modified",
		paths[3]: "clean",
		paths[4]: "unknown",
	}
	for i, wt := range worktrees {
		if rows[i].Index != i+1 || rows[i].Path != wt.Path {
			t.Errorf("row %d = index %d path %q, want index %d path %q", i, rows[i].Index, rows[i].Path, i+1, wt.Path)
		}
		if rows[i].Status != wantStatus[wt.Path] {
			t.Errorf("row %d (%s) status = %q, want %q", i, wt.Path, rows[i].Status, wantStatus[wt.Path])
		}
	}
}

func addTestWorktree(t *testing.T, root, path, branch string) {
	t.Helper()
	cmd := exec.Command("git", "worktree", "add", "-b", branch, path)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %s", out)
	}
	t.Cleanup(func() {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		cmd := exec.Command("git", "worktree", "remove", "--force", path)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("git worktree remove: %s", out)
		}
	})
}
