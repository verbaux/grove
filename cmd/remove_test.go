package cmd

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func baseConfig() config.Config {
	return config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
	}
}

func TestForceForRemovalDoesNotForceCleanTargetAfterDirtyConfirmation(t *testing.T) {
	force, err := forceForRemoval("/unused", "clean", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if force {
		t.Fatal("clean target inherited force from another dirty worktree")
	}
}

func TestForceForRemovalAllowsExplicitForce(t *testing.T) {
	force, err := forceForRemoval("/unused", "clean", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !force {
		t.Fatal("explicit force was not preserved")
	}
}

func TestEnsureNoBlockingSubmodulesDoesNotDuplicateUnknownSafetyMessage(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	gitInRepo(t, nested, "init")
	gitInRepo(t, nested, "config", "user.email", "test@test.com")
	gitInRepo(t, nested, "config", "user.name", "Test")
	gitInRepo(t, nested, "commit", "--allow-empty", "-m", "nested")
	gitInRepo(t, root, "add", "nested")
	gitInRepo(t, root, "commit", "-m", "add gitlink without mapping")

	err := ensureNoBlockingSubmodules(root)
	if !errors.Is(err, git.ErrSubmoduleSafetyUnknown) {
		t.Fatalf("error = %v, want ErrSubmoduleSafetyUnknown", err)
	}
	if err.Error() != git.ErrSubmoduleSafetyUnknown.Error() {
		t.Fatalf("error was redundantly wrapped: %q", err)
	}
}

func TestForceForRemovalRequiresExplicitForceForInitializedSubmodule(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	submodule := t.TempDir()
	wtPath := filepath.Join(t.TempDir(), "linked-worktree")
	commands := []struct {
		dir  string
		args []string
	}{
		{submodule, []string{"init"}},
		{submodule, []string{"config", "user.email", "test@test.com"}},
		{submodule, []string{"config", "user.name", "Test"}},
		{submodule, []string{"commit", "--allow-empty", "-m", "initial"}},
		{root, []string{"-c", "protocol.file.allow=always", "submodule", "add", submodule, "vendor/example"}},
		{root, []string{"commit", "-am", "add submodule"}},
		{root, []string{"worktree", "add", "-b", "feature/submodule-policy", wtPath}},
		{wtPath, []string{"-c", "protocol.file.allow=always", "submodule", "update", "--init"}},
		{wtPath, []string{"submodule", "deinit", "-f", "vendor/example"}},
	}
	for _, command := range commands {
		cmd := exec.Command("git", command.args...)
		cmd.Dir = command.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", command.args, out)
		}
	}
	t.Cleanup(func() {
		cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
		cmd.Dir = root
		_ = cmd.Run()
	})

	force, err := forceForRemoval(wtPath, "1 modified", false, true)
	if err == nil || !strings.Contains(err.Error(), "rerun with --force") {
		t.Fatalf("error = %v, want explicit force guidance", err)
	}
	if force {
		t.Fatal("dirty confirmation implicitly forced initialized submodule removal")
	}
}

func TestBatchRemovalDoesNotApplyDirtyConfirmationToCleanSubmoduleWorktree(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"clean", func() error { return runClean(cleanCmd, nil) }},
		{"prune", func() error { return runPrune(pruneCmd, nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := setupIntegrationRepo(t, baseConfig())
			submodule := t.TempDir()
			gitInRepo(t, submodule, "init")
			gitInRepo(t, submodule, "config", "user.email", "test@test.com")
			gitInRepo(t, submodule, "config", "user.name", "Test")
			gitInRepo(t, submodule, "commit", "--allow-empty", "-m", "initial")
			gitInRepo(t, root, "-c", "protocol.file.allow=always", "submodule", "add", submodule, "vendor/example")
			gitInRepo(t, root, "commit", "-am", "add submodule")

			worktreeRoot := t.TempDir()
			dirtyPath := filepath.Join(worktreeRoot, "dirty")
			submodulePath := filepath.Join(worktreeRoot, "submodule")
			gitInRepo(t, root, "worktree", "add", "-b", "dirty-branch", dirtyPath)
			gitInRepo(t, root, "worktree", "add", "-b", "submodule-branch", submodulePath)
			if tc.name == "prune" {
				for _, item := range []struct{ path, file string }{{dirtyPath, "dirty-branch.txt"}, {submodulePath, "submodule-branch.txt"}} {
					if err := os.WriteFile(filepath.Join(item.path, item.file), []byte("merged\n"), 0644); err != nil {
						t.Fatal(err)
					}
					gitInRepo(t, item.path, "add", item.file)
					gitInRepo(t, item.path, "commit", "-m", "add "+item.file)
					gitInRepo(t, root, "merge", "--no-ff", filepath.Base(strings.TrimSuffix(item.file, ".txt")), "-m", "merge "+item.file)
				}
			}
			gitInRepo(t, submodulePath, "-c", "protocol.file.allow=always", "submodule", "update", "--init")
			if err := os.WriteFile(filepath.Join(dirtyPath, "dirty.txt"), []byte("dirty\n"), 0644); err != nil {
				t.Fatal(err)
			}

			s, err := state.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Add("dirty", "dirty-branch", dirtyPath, 0); err != nil {
				t.Fatal(err)
			}
			if err := s.Add("submodule", "submodule-branch", submodulePath, 0); err != nil {
				t.Fatal(err)
			}
			if err := state.Save(root, s); err != nil {
				t.Fatal(err)
			}

			reader = bufio.NewReader(strings.NewReader("y\n"))
			cleanForce, cleanIncludeProtected = false, false
			pruneForce, pruneBase, pruneYes, pruneDryRun, pruneJSON, pruneIncludeProtected = false, "", false, false, false, false
			t.Cleanup(func() { reader = nil })

			err = tc.run()
			if !errors.Is(err, git.ErrSubmoduleForceRequired) {
				t.Fatalf("error = %v, want clean submodule worktree refusal", err)
			}
			if _, err := os.Stat(dirtyPath); !os.IsNotExist(err) {
				t.Fatalf("confirmed dirty worktree was not removed: %v", err)
			}
			if _, err := os.Stat(submodulePath); err != nil {
				t.Fatalf("clean submodule worktree inherited force and was removed: %v", err)
			}
		})
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

func TestSubmodulePreflightBlocksBeforeRemoveHook(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		BeforeRemove: config.HookCommands{
			`touch "$(dirname "$GROVE_PATH")/before-submodule-marker"`,
		},
	})
	submodule := t.TempDir()
	commands := []struct {
		dir  string
		args []string
	}{
		{submodule, []string{"init"}},
		{submodule, []string{"config", "user.email", "test@test.com"}},
		{submodule, []string{"config", "user.name", "Test"}},
		{submodule, []string{"commit", "--allow-empty", "-m", "initial"}},
		{dir, []string{"-c", "protocol.file.allow=always", "submodule", "add", submodule, "vendor/example"}},
		{dir, []string{"commit", "-am", "add submodule"}},
	}
	for _, command := range commands {
		cmd := exec.Command("git", command.args...)
		cmd.Dir = command.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %s", command.args, out)
		}
	}

	createName, createFrom, createDetach = "submodule-hook", "", false
	if err := runCreate(createCmd, []string{"feature/submodule-hook"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { createName = "" })
	resolved, err := resolveWorktree(dir, "submodule-hook")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = git.RemoveWorktree(resolved.Path, true) })
	cmd := exec.Command("git", "-c", "protocol.file.allow=always", "submodule", "update", "--init")
	cmd.Dir = resolved.Path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initialize worktree submodule: %s", out)
	}

	removeForce = false
	err = runRemove(removeCmd, []string{"submodule-hook"})
	if !errors.Is(err, git.ErrSubmoduleForceRequired) {
		t.Fatalf("remove error = %v, want submodule force requirement", err)
	}
	marker := filepath.Join(filepath.Dir(resolved.Path), "before-submodule-marker")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("beforeRemove hook ran before submodule refusal: %v", err)
	}
	if _, err := os.Stat(resolved.Path); err != nil {
		t.Fatalf("worktree was removed after preflight refusal: %v", err)
	}
	loaded, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Get("submodule-hook"); !ok {
		t.Fatal("state entry was removed after preflight refusal")
	}
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

func TestRemovePreservesStateAddedAfterInitialLoad(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "remove-lock", "", false, false
	t.Cleanup(func() { createName = "" })
	if err := runCreate(createCmd, []string{"feature/remove-lock"}); err != nil {
		t.Fatal(err)
	}

	stale, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := stale.Get("remove-lock")
	if !ok {
		t.Fatal("expected remove-lock entry")
	}
	t.Cleanup(func() { _ = git.RemoveWorktree(entry.Path, true) })

	if err := state.Update(dir, func(latest *state.State) error {
		return latest.Add("payments", "feature/payments", "/tmp/payments", 0)
	}); err != nil {
		t.Fatal(err)
	}

	removed, err := removeManagedWorktree(dir, baseConfig(), &stale, managedRemoveTarget{
		Alias: "remove-lock", Branch: entry.Branch, Path: entry.Path, Port: entry.Port,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected worktree removal")
	}

	loaded, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Get("payments"); !ok {
		t.Fatal("concurrent payments entry was lost")
	}
	if _, ok := loaded.Get("remove-lock"); ok {
		t.Fatal("removed entry still exists")
	}
}

func TestRemoveRechecksConcurrentProtectionChange(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "protect-race", "", false, false
	t.Cleanup(func() { createName = "" })
	if err := runCreate(createCmd, []string{"feature/protect-race"}); err != nil {
		t.Fatal(err)
	}

	stale, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := stale.Get("protect-race")
	if !ok {
		t.Fatal("expected protect-race entry")
	}
	t.Cleanup(func() { _ = git.RemoveWorktree(entry.Path, true) })

	if err := state.Update(dir, func(latest *state.State) error {
		return latest.SetProtected("protect-race", true)
	}); err != nil {
		t.Fatal(err)
	}

	_, err = removeManagedWorktree(dir, baseConfig(), &stale, managedRemoveTarget{
		Alias: "protect-race", Branch: entry.Branch, Path: entry.Path, Port: entry.Port,
	}, true)
	if err == nil || !strings.Contains(err.Error(), "became protected") {
		t.Fatalf("remove error = %v, want concurrent protection error", err)
	}
	if _, err := os.Stat(entry.Path); err != nil {
		t.Fatalf("protected worktree should remain: %v", err)
	}
	loaded, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	protected, ok := loaded.Get("protect-race")
	if !ok || !protected.Protected {
		t.Fatalf("protected state should remain: %+v", protected)
	}
}

func cleanupWorktree(t *testing.T, root, path string) {
	t.Helper()
	removeForce = true
	if err := runRemove(removeCmd, []string{path}); err != nil {
		t.Logf("cleanup remove %s: %v", path, err)
	}
}
