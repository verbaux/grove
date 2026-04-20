package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func TestDiagStatePathsAllExist(t *testing.T) {
	dir := t.TempDir()
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Branch: "feature/a", Path: dir, Port: 3001, Created: time.Now()},
	}}
	diags := diagStatePaths(s)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok diag, got %v", diags)
	}
}

func TestDiagStatePathsMissing(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Branch: "x", Path: "/nonexistent/path/definitely/missing", Port: 3001, Created: time.Now()},
	}}
	diags := diagStatePaths(s)
	hasError := false
	for _, d := range diags {
		if d.Level == levelError {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected error diag for missing path, got %v", diags)
	}
}

func TestDiagStatePathsEmpty(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{}}
	diags := diagStatePaths(s)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok diag for empty state, got %v", diags)
	}
}

func TestDiagOrphansNone(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Branch: "a", Path: "/tmp/a"},
	}}
	wts := []git.Worktree{
		{Path: "/tmp/main", IsMain: true, Branch: "main"},
		{Path: "/tmp/a", Branch: "a"},
	}
	diags := diagOrphans(s, wts)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok, got %v", diags)
	}
}

func TestDiagOrphansDetected(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{}}
	wts := []git.Worktree{
		{Path: "/tmp/main", IsMain: true, Branch: "main"},
		{Path: "/tmp/orphan", Branch: "feature/orphan"},
	}
	diags := diagOrphans(s, wts)
	hasWarn := false
	for _, d := range diags {
		if d.Level == levelWarn {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected warn for orphan, got %v", diags)
	}
}

func TestDiagPortCollisions(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: "/tmp/a", Port: 3001},
		"b": {Path: "/tmp/b", Port: 3001},
	}}
	diags := diagPortCollisions(s)
	hasError := false
	for _, d := range diags {
		if d.Level == levelError {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected error for port collision, got %v", diags)
	}
}

func TestDiagPortCollisionsNone(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: "/tmp/a", Port: 3001},
		"b": {Path: "/tmp/b", Port: 3002},
	}}
	diags := diagPortCollisions(s)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok, got %v", diags)
	}
}

func TestDiagConfigRangeInvalid(t *testing.T) {
	cfg := config.Config{PortRange: &config.PortRange{Min: 5000, Max: 4000}}
	diags := diagConfigRange(cfg)
	if len(diags) != 1 || diags[0].Level != levelError {
		t.Errorf("expected error for invalid range, got %v", diags)
	}
}

func TestDiagConfigRangeValid(t *testing.T) {
	cfg := config.Config{PortRange: &config.PortRange{Min: 3001, Max: 3999}}
	diags := diagConfigRange(cfg)
	if len(diags) != 0 {
		t.Errorf("expected no diags for valid range, got %v", diags)
	}
}

func TestDiagSymlinksBroken(t *testing.T) {
	wtDir := t.TempDir()
	link := filepath.Join(wtDir, "node_modules")
	if err := os.Symlink("/nonexistent/target/missing", link); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Symlink: []string{"node_modules"}}
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: wtDir, Branch: "a"},
	}}
	diags := diagSymlinks(wtDir, cfg, s)
	hasError := false
	for _, d := range diags {
		if d.Level == levelError {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected error for broken symlink, got %v", diags)
	}
}

func TestDiagSymlinksHealthy(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "node_modules")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	wtDir := t.TempDir()
	link := filepath.Join(wtDir, "node_modules")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Symlink: []string{"node_modules"}}
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: wtDir, Branch: "a"},
	}}
	diags := diagSymlinks(root, cfg, s)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok, got %v", diags)
	}
}
