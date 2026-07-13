package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func TestSyncAppliesMissingSetupAndUpdatesConfigHash(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: nil,
	})
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("old=1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	createName = "sync-me"
	createFrom = ""
	createDetach = false
	createJSON = false
	t.Cleanup(func() { createName = "" })
	if err := runCreate(createCmd, []string{"feature/sync-me"}); err != nil {
		t.Fatal(err)
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := s.Get("sync-me")
	if !ok {
		t.Fatal("expected sync-me entry")
	}
	oldHash := entry.ConfigHash
	if oldHash == "" {
		t.Fatal("expected initial config hash")
	}
	if err := os.WriteFile(filepath.Join(entry.Path, ".env"), []byte("worktree=keep\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(dir, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".next", "cache"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".next", "cache", "data.txt"), []byte("cache\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subdir", ".env.local"), []byte("new=1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	newCfg := config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{"node_modules"},
		CopyDirs:    []string{".next/cache"},
		AfterCreate: nil,
	}
	if err := config.Save(dir, newCfg); err != nil {
		t.Fatal(err)
	}

	syncHooks = false
	syncJSON = true
	t.Cleanup(func() { syncJSON = false })
	out, err := captureStdout(t, func() error {
		return runSync(syncCmd, []string{"sync-me"})
	})
	if err != nil {
		t.Fatal(err)
	}

	var result syncResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("sync --json output is not valid JSON: %v\n%s", err, out)
	}
	if result.Alias != "sync-me" || !result.UpdatedConfigHash {
		t.Fatalf("unexpected sync result: %+v", result)
	}
	if len(result.Symlinked) != 1 || result.Symlinked[0] != "node_modules" {
		t.Fatalf("expected node_modules symlinked, got %+v", result.Symlinked)
	}
	if len(result.CopiedDirs) != 1 || result.CopiedDirs[0] != ".next/cache" {
		t.Fatalf("expected .next/cache copied, got %+v", result.CopiedDirs)
	}
	if len(result.CopiedEnv) != 1 || result.CopiedEnv[0] != filepath.Join("subdir", ".env.local") {
		t.Fatalf("expected only missing env copied, got %+v", result.CopiedEnv)
	}

	linkInfo, err := os.Lstat(filepath.Join(entry.Path, "node_modules"))
	if err != nil {
		t.Fatal(err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected node_modules to be a symlink")
	}
	if _, err := os.Stat(filepath.Join(entry.Path, ".next", "cache", "data.txt")); err != nil {
		t.Fatal(err)
	}
	envData, err := os.ReadFile(filepath.Join(entry.Path, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(envData) != "worktree=keep\n" {
		t.Fatalf(".env was overwritten: %q", string(envData))
	}

	s, err = state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok = s.Get("sync-me")
	if !ok {
		t.Fatal("expected sync-me entry after sync")
	}
	newHash, err := config.SetupHash(newCfg)
	if err != nil {
		t.Fatal(err)
	}
	if entry.ConfigHash != newHash || entry.ConfigHash == oldHash {
		t.Fatalf("ConfigHash = %q, want new hash %q", entry.ConfigHash, newHash)
	}
}

func TestSyncRunsHooksOnlyWhenRequested(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: nil,
	})

	createName = "sync-hooks"
	createFrom = ""
	createDetach = false
	createJSON = false
	t.Cleanup(func() { createName = "" })
	if err := runCreate(createCmd, []string{"feature/sync-hooks"}); err != nil {
		t.Fatal(err)
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := s.Get("sync-hooks")
	if !ok {
		t.Fatal("expected sync-hooks entry")
	}
	if err := config.Save(dir, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: config.HookCommands{"printf noisy; printf hook > hook.txt"},
	}); err != nil {
		t.Fatal(err)
	}

	syncHooks = false
	syncJSON = false
	if err := runSync(syncCmd, []string{"sync-hooks"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(entry.Path, "hook.txt")); !os.IsNotExist(err) {
		t.Fatalf("hook should not run without --hooks, stat err: %v", err)
	}

	syncHooks = true
	syncJSON = true
	t.Cleanup(func() {
		syncHooks = false
		syncJSON = false
	})
	out, err := captureStdout(t, func() error {
		return runSync(syncCmd, []string{"sync-hooks"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var result syncResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("sync --json output with hook stdout is not valid JSON: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(entry.Path, "hook.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hook" {
		t.Fatalf("hook.txt = %q, want hook", string(data))
	}
}

func TestSyncPreservesConcurrentProtectionChange(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "sync-lock", "", false, false
	t.Cleanup(func() { createName = "" })
	if err := runCreate(createCmd, []string{"feature/sync-lock"}); err != nil {
		t.Fatal(err)
	}

	stale, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := stale.Get("sync-lock")
	if !ok {
		t.Fatal("expected sync-lock entry")
	}
	t.Cleanup(func() { _ = git.RemoveWorktree(entry.Path, true) })

	if err := state.Update(dir, func(latest *state.State) error {
		return latest.SetProtected("sync-lock", true)
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterCreate = config.HookCommands{"true"}
	if _, err := syncManagedWorktree(dir, cfg, &stale, "sync-lock", entry, true); err != nil {
		t.Fatal(err)
	}

	loaded, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	updated, ok := loaded.Get("sync-lock")
	if !ok {
		t.Fatal("sync-lock entry is missing")
	}
	if !updated.Protected {
		t.Fatal("concurrent protection change was lost")
	}
}
