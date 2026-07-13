package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func TestAdoptWorktreeRegistersPathAndAllocatesPort(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "adopted")
	if err := git.AddWorktree(path, "feature/adopted", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = git.RemoveWorktree(path, true) })
	target := orphanWorktree{Path: path, Branch: "feature/adopted"}

	port, err := adoptWorktree(root, cfg, target, "adopted")
	if err != nil {
		t.Fatal(err)
	}
	if port == 0 {
		t.Fatal("expected allocated port")
	}
	loaded, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := loaded.Get("adopted")
	if !ok || entry.Path != path || entry.Branch != "feature/adopted" || entry.Port != port {
		t.Fatalf("adopted entry = %+v", entry)
	}

	_, err = adoptWorktree(root, cfg, target, "duplicate")
	if err == nil || !strings.Contains(err.Error(), "already managed") {
		t.Fatalf("duplicate path error = %v", err)
	}
}

func TestAdoptWorktreeRejectsTargetMissingFromGit(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	target := orphanWorktree{Path: t.TempDir(), Branch: "feature/vanished"}

	_, err = adoptWorktree(root, cfg, target, "vanished")
	if err == nil || !strings.Contains(err.Error(), "no longer present") {
		t.Fatalf("adoptWorktree error = %v, want missing git worktree error", err)
	}
	loaded, loadErr := state.Load(root)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(loaded.Worktrees) != 0 {
		t.Fatalf("missing worktree was registered: %+v", loaded.Worktrees)
	}
}

func TestAdoptWorktreeRejectsMissingPathStillKnownToGit(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "vanished")
	if err := git.AddWorktree(path, "feature/vanished-path", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = git.PruneWorktrees() })
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}

	_, err = adoptWorktree(root, cfg, orphanWorktree{Path: path, Branch: "feature/vanished-path"}, "vanished-path")
	if !errors.Is(err, errAdoptTargetUnavailable) {
		t.Fatalf("adoptWorktree error = %v, want errAdoptTargetUnavailable", err)
	}
	loaded, loadErr := state.Load(root)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(loaded.Worktrees) != 0 {
		t.Fatalf("missing worktree was registered: %+v", loaded.Worktrees)
	}
}
