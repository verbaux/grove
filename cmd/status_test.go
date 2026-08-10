package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func TestStatusJSONReportsDirtyAndConfigDrift(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: nil,
	})

	createName = "status-drift"
	createFrom = ""
	createDetach = false
	createJSON = false
	t.Cleanup(func() { createName = "" })
	if err := runCreate(createCmd, []string{"feature/status-drift"}); err != nil {
		t.Fatal(err)
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := s.Get("status-drift")
	if !ok {
		t.Fatal("expected status-drift entry")
	}
	if err := os.WriteFile(filepath.Join(entry.Path, "dirty.txt"), []byte("dirty\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := config.Save(dir, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{"node_modules"},
		AfterCreate: nil,
	}); err != nil {
		t.Fatal(err)
	}

	statusJSON = true
	t.Cleanup(func() { statusJSON = false })
	out, err := captureStdout(t, func() error {
		return runStatus(statusCmd, nil)
	})
	if err != nil {
		t.Fatal(err)
	}

	var result statusJSONResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("status --json output is not valid JSON: %v\n%s", err, out)
	}
	if result.Base != "main" {
		t.Fatalf("Base = %q, want main", result.Base)
	}
	if result.Summary.Total != 1 || result.Summary.Dirty != 1 || result.Summary.ConfigDrift != 1 {
		t.Fatalf("unexpected summary: %+v", result.Summary)
	}
	if len(result.Worktrees) != 1 {
		t.Fatalf("expected one worktree, got %+v", result.Worktrees)
	}
	row := result.Worktrees[0]
	if row.Alias != "status-drift" {
		t.Fatalf("Alias = %q, want status-drift", row.Alias)
	}
	if row.WorktreeStatus != "1 untracked" {
		t.Fatalf("WorktreeStatus = %q, want 1 untracked", row.WorktreeStatus)
	}
	if row.ConfigStatus != "drift" {
		t.Fatalf("ConfigStatus = %q, want drift", row.ConfigStatus)
	}
	if row.PortStatus == "" || row.Freshness == "" {
		t.Fatalf("expected port and freshness status, got %+v", row)
	}
}

func TestStatusHumanNoManagedWorktrees(t *testing.T) {
	setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
	})

	statusJSON = false
	out, err := captureStdout(t, func() error {
		return runStatus(statusCmd, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "No managed worktrees.") {
		t.Fatalf("expected empty status message, got %q", out)
	}
}

func TestStatusHumanNoManagedWorktreesSuggestsDoctorFixForOrphans(t *testing.T) {
	out, err := captureStdout(t, func() error {
		printStatus(statusJSONResult{Summary: statusSummary{Orphans: 1}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "run 'grove adopt' or 'grove doctor --fix'") {
		t.Fatalf("expected actionable orphan repair hint, got %q", out)
	}
}

func TestStatusOutputsWorktreeNote(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "", "", false, false
	if err := runCreate(createCmd, []string{"feature/noted-status"}); err != nil {
		t.Fatal(err)
	}
	if err := state.Update(root, func(latest *state.State) error {
		if err := latest.SetNote("noted-status", "blocked by upstream"); err != nil {
			return err
		}
		return latest.SetDetached("noted-status", true)
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := loaded.Get("noted-status")
	t.Cleanup(func() { cleanupWorktree(t, root, entry.Path) })

	statusJSON = true
	t.Cleanup(func() { statusJSON = false })
	jsonOut, err := captureStdout(t, func() error { return runStatus(statusCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var result statusJSONResult
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("status JSON invalid: %v\n%s", err, jsonOut)
	}
	if len(result.Worktrees) != 1 || result.Worktrees[0].Note != "blocked by upstream" || !result.Worktrees[0].Detached {
		t.Fatalf("status JSON omitted note: %+v", result.Worktrees)
	}
	if result.Worktrees[0].SymlinkStatus != "detached" || result.Summary.SymlinkIssues != 0 {
		t.Fatalf("detached worktree reported symlink issues: %+v", result)
	}

	statusJSON = false
	humanOut, err := captureStdout(t, func() error { return runStatus(statusCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(humanOut, "NOTE") || !strings.Contains(humanOut, "blocked by upstream") || !strings.Contains(humanOut, "detached") {
		t.Fatalf("human status omitted note column: %q", humanOut)
	}
}

func TestStatusHelpersReportSymlinkAndPortIssues(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("/definitely/missing/target", filepath.Join(dir, "node_modules")); err != nil {
		t.Fatal(err)
	}

	if got := symlinkStatus(dir, []string{"node_modules", ".husky/_"}); got != "1 broken, 1 missing" {
		t.Fatalf("symlinkStatus = %q, want broken and missing counts", got)
	}
	if got := portStatus(3001, map[int]int{3001: 2}); got != "collision on 3001" {
		t.Fatalf("portStatus = %q, want collision on 3001", got)
	}
	if got := isDirtyStatus("stale"); got {
		t.Fatal("stale state should not count as dirty worktree changes")
	}
}
