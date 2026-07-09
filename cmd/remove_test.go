package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func baseConfig() config.Config {
	return config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
	}
}

// nonMainIndex returns the 1-based list index and alias of the single
// non-main worktree in the repo at root.
func nonMainIndex(t *testing.T, root string) (string, string) {
	t.Helper()
	rows, err := buildWorktreeRows(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if !r.IsMain {
			return strconv.Itoa(r.Index), r.Name
		}
	}
	t.Fatal("no non-main worktree found")
	return "", ""
}

func TestRemoveRunsLifecycleHooks(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		BeforeRemove: config.HookCommands{
			`printf "before:%s:%s:%s:%s" "$GROVE_ALIAS" "$GROVE_BRANCH" "$GROVE_PORT" "$(pwd)" > "$(dirname "$GROVE_PATH")/before-$GROVE_ALIAS.txt"`,
		},
		AfterRemove: config.HookCommands{
			`printf "after:%s:%s:%s:%s" "$GROVE_ALIAS" "$GROVE_BRANCH" "$GROVE_PORT" "$(pwd)" > "$(dirname "$GROVE_PATH")/after-$GROVE_ALIAS.txt"`,
		},
	})
	createName, createFrom, createDetach = "", "", false
	if err := runCreate(createCmd, []string{"feature/hooked-remove"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	idx, alias := nonMainIndex(t, dir)
	resolved, _ := resolveWorktree(dir, idx)
	path := resolved.Path

	removeForce = false
	if err := runRemove(removeCmd, []string{idx}); err != nil {
		t.Fatalf("remove: %v", err)
	}

	before, err := os.ReadFile(filepath.Join(filepath.Dir(path), "before-"+alias+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(before); !strings.HasPrefix(got, "before:"+alias+":feature/hooked-remove:") || !strings.HasSuffix(got, ":"+path) {
		t.Fatalf("unexpected beforeRemove output: %q", got)
	}

	after, err := os.ReadFile(filepath.Join(filepath.Dir(path), "after-"+alias+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(after); !strings.HasPrefix(got, "after:"+alias+":feature/hooked-remove:") || !strings.HasSuffix(got, ":"+dir) {
		t.Fatalf("unexpected afterRemove output: %q", got)
	}
}

func TestBeforeRemoveFailureBlocksRemoval(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir:  "../",
		Prefix:       "testproject",
		Symlink:      []string{},
		BeforeRemove: config.HookCommands{"exit 7"},
	})
	createName, createFrom, createDetach = "", "", false
	if err := runCreate(createCmd, []string{"feature/keep-me"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	idx, alias := nonMainIndex(t, dir)
	resolved, _ := resolveWorktree(dir, idx)
	path := resolved.Path

	removeForce = false
	err := runRemove(removeCmd, []string{idx})
	if err == nil {
		t.Fatal("expected beforeRemove failure")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("worktree should still exist after beforeRemove failure: %v", statErr)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(alias); !ok {
		t.Fatalf("state should still contain alias %q", alias)
	}

	if err := config.Save(dir, baseConfig()); err != nil {
		t.Fatal(err)
	}
	cleanupWorktree(t, dir, path)
}

func TestResolveWorktreeByIndexMatchesAlias(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach = "", "", false
	if err := runCreate(createCmd, []string{"feature/idx"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	idx, alias := nonMainIndex(t, dir)

	byIdx, err := resolveWorktree(dir, idx)
	if err != nil || byIdx == nil {
		t.Fatalf("resolve by index %q: %v (nil=%v)", idx, err, byIdx == nil)
	}
	byAlias, err := resolveWorktree(dir, alias)
	if err != nil || byAlias == nil {
		t.Fatalf("resolve by alias %q: %v", alias, err)
	}
	if byIdx.Path != byAlias.Path {
		t.Errorf("index path %q != alias path %q", byIdx.Path, byAlias.Path)
	}

	cleanupWorktree(t, dir, byIdx.Path)
}

func TestResolveWorktreeIndexOneIsMain(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())

	res, err := resolveWorktree(dir, "1")
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.IsMain {
		t.Fatalf("index 1 should resolve to the main worktree, got %+v", res)
	}
}

func TestResolveWorktreeIndexOutOfRange(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())

	if _, err := resolveWorktree(dir, "99"); err == nil {
		t.Fatal("expected out-of-range index to error")
	}
}

func TestRemoveByIndex(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach = "", "", false
	if err := runCreate(createCmd, []string{"feature/rm-idx"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	idx, alias := nonMainIndex(t, dir)
	resolved, _ := resolveWorktree(dir, idx)
	path := resolved.Path

	removeForce = false
	if err := runRemove(removeCmd, []string{idx}); err != nil {
		t.Fatalf("remove by index: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree path still exists: %s", path)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(alias); ok {
		t.Errorf("state still has alias %q after remove", alias)
	}
}

func TestRemoveRefusesMainWorktree(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())

	removeForce = false
	err := runRemove(removeCmd, []string{"1"})
	if err == nil {
		t.Fatal("expected remove of main worktree (index 1) to be refused")
	}

	// Main worktree must still exist.
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Errorf("main worktree dir disturbed: %v", statErr)
	}
}

func cleanupWorktree(t *testing.T, root, path string) {
	t.Helper()
	removeForce = true
	if err := runRemove(removeCmd, []string{path}); err != nil {
		t.Logf("cleanup remove %s: %v", path, err)
	}
}
