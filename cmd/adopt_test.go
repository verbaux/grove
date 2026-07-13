package cmd

import (
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
	path := t.TempDir() + "/adopted"
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
