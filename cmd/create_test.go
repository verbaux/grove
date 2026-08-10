package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func TestCreateJSON(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: nil,
	})

	createName = ""
	createFrom = ""
	createDetach = false
	createJSON = true
	t.Cleanup(func() { createJSON = false })

	out, err := captureStdout(t, func() error {
		return runCreate(createCmd, []string{"feature/json-output"})
	})
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Alias           string   `json:"alias"`
		Branch          string   `json:"branch"`
		Path            string   `json:"path"`
		Port            int      `json:"port"`
		Detached        bool     `json:"detached"`
		CopiedEnv       []string `json:"copiedEnv"`
		Symlinked       []string `json:"symlinked"`
		SkippedSymlinks []string `json:"skippedSymlinks"`
		CopiedDirs      []string `json:"copiedDirs"`
		SkippedCopyDirs []string `json:"skippedCopyDirs"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("create --json output is not valid JSON: %v\n%s", err, out)
	}
	if result.Alias != "json-output" || result.Branch != "feature/json-output" {
		t.Fatalf("unexpected JSON result: %+v", result)
	}
	if result.Path == "" || result.Port == 0 || result.Detached {
		t.Fatalf("expected path, port, and detached=false, got %+v", result)
	}
	if result.CopiedEnv == nil || result.Symlinked == nil || result.SkippedSymlinks == nil || result.CopiedDirs == nil || result.SkippedCopyDirs == nil {
		t.Fatalf("expected JSON arrays instead of null slices, got %+v", result)
	}

	remove := exec.Command("git", "worktree", "remove", "--force", filepath.Join(filepath.Dir(dir), "testproject-json-output"))
	remove.Dir = dir
	if out, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("cleanup failed: %s", out)
	}
}

func TestCreateStoresConfigHash(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: nil,
	})

	createName = ""
	createFrom = ""
	createDetach = false
	createJSON = false

	if err := runCreate(createCmd, []string{"feature/config-hash"}); err != nil {
		t.Fatal(err)
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := s.Get("config-hash")
	if !ok {
		t.Fatal("expected config-hash alias in state")
	}
	if entry.ConfigHash == "" {
		t.Fatal("expected config hash to be stored in state")
	}

	remove := exec.Command("git", "worktree", "remove", "--force", filepath.Join(filepath.Dir(dir), "testproject-config-hash"))
	remove.Dir = dir
	if out, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("cleanup failed: %s", out)
	}
}

// setupIntegrationRepo creates a real git repo with a .groverc.json and
// changes cwd into it. Returns the repo directory and a cleanup function.
func setupIntegrationRepo(t *testing.T, cfg config.Config) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}

	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestCreateRollbackOnAfterCreateFailure(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: config.HookCommands{"exit 1"}, // always fails
	})

	// Reset package-level flags so we get a clean state
	createName = ""
	createFrom = ""
	createDetach = false

	err := runCreate(createCmd, []string{"feature/rollback-test"})
	if err == nil {
		t.Fatal("expected runCreate to return error when afterCreate fails")
	}

	// The worktree directory must have been rolled back
	wtPath := filepath.Join(filepath.Dir(dir), "testproject-rollback-test")
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: worktree directory still exists at %s", wtPath)
	}

	// State must not have the alias
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.AliasExists("rollback-test") {
		t.Error("rollback failed: alias still present in state.json")
	}
}

func TestCreateRollbackOnStateSaveFailure(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		AfterCreate: nil,
	})

	createName = ""
	createFrom = ""
	createDetach = false

	// Make .grove directory a file so the state transaction cannot start.
	groveDir := filepath.Join(dir, ".grove")
	if err := os.WriteFile(groveDir, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	err := runCreate(createCmd, []string{"feature/state-fail"})
	if err == nil {
		t.Fatal("expected runCreate to return error when state save fails")
	}

	wtPath := filepath.Join(filepath.Dir(dir), "testproject-state-fail")
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("rollback failed: worktree directory still exists at %s", wtPath)
	}
}

func TestCreateSkipsSymlinkConflict(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{".yarn/cache"},
		AfterCreate: nil,
	})

	// Make .yarn/cache tracked so it exists in the new worktree checkout.
	cacheFile := filepath.Join(dir, ".yarn", "cache", "pkg.txt")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cacheFile, []byte("cached"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", ".yarn/cache/pkg.txt"},
		{"git", "commit", "-m", "track yarn cache"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}

	createName = ""
	createFrom = ""
	createDetach = false

	if err := runCreate(createCmd, []string{"feature/symlink-conflict"}); err != nil {
		t.Fatalf("expected create to succeed on symlink conflict, got: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(dir), "testproject-symlink-conflict")
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("expected worktree to exist, got: %v", err)
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.AliasExists("symlink-conflict") {
		t.Fatal("expected alias to be present in state")
	}

	cacheInfo, err := os.Lstat(filepath.Join(wtPath, ".yarn", "cache"))
	if err != nil {
		t.Fatal(err)
	}
	if cacheInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected .yarn/cache in worktree to remain a real directory")
	}

	// Cleanup successful worktree to avoid leaking dirs across test runs.
	remove := exec.Command("git", "worktree", "remove", "--force", wtPath)
	remove.Dir = dir
	if out, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("cleanup failed: %s", out)
	}
}

func TestCreateCopiesBuildCache(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		CopyDirs:    []string{".next", "dist"},
		AfterCreate: nil,
	})

	// Create build artifacts in main worktree
	nextDir := filepath.Join(dir, ".next", "cache")
	os.MkdirAll(nextDir, 0755)
	os.WriteFile(filepath.Join(nextDir, "build.json"), []byte(`{"version":1}`), 0644)

	distDir := filepath.Join(dir, "dist")
	os.MkdirAll(distDir, 0755)
	os.WriteFile(filepath.Join(distDir, "index.js"), []byte("console.log('hi')"), 0644)

	createName = ""
	createFrom = ""
	createDetach = false

	if err := runCreate(createCmd, []string{"feature/build-cache"}); err != nil {
		t.Fatalf("expected create to succeed, got: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(dir), "testproject-build-cache")

	// Verify .next/cache/build.json was copied
	data, err := os.ReadFile(filepath.Join(wtPath, ".next", "cache", "build.json"))
	if err != nil {
		t.Fatal("expected .next/cache/build.json to be copied:", err)
	}
	if string(data) != `{"version":1}` {
		t.Errorf("content = %q, want %q", string(data), `{"version":1}`)
	}

	// Verify dist/index.js was copied
	data, err = os.ReadFile(filepath.Join(wtPath, "dist", "index.js"))
	if err != nil {
		t.Fatal("expected dist/index.js to be copied:", err)
	}
	if string(data) != "console.log('hi')" {
		t.Errorf("content = %q, want %q", string(data), "console.log('hi')")
	}

	// Verify it's a copy, not a symlink
	info, err := os.Lstat(filepath.Join(wtPath, ".next"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected .next to be a real directory, not a symlink")
	}

	remove := exec.Command("git", "worktree", "remove", "--force", wtPath)
	remove.Dir = dir
	if out, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("cleanup failed: %s", out)
	}
}

func TestCreateDetachHonorsCopyDirsOnDetach(t *testing.T) {
	disabled := false
	for _, tc := range []struct {
		name             string
		copyDirsOnDetach *bool
		wantCopied       bool
	}{
		{name: "default", wantCopied: true},
		{name: "disabled", copyDirsOnDetach: &disabled, wantCopied: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupIntegrationRepo(t, config.Config{
				WorktreeDir:      "../",
				Prefix:           "testproject",
				Symlink:          []string{},
				CopyDirs:         []string{"dist"},
				CopyDirsOnDetach: tc.copyDirsOnDetach,
			})
			if err := os.MkdirAll(filepath.Join(dir, "dist"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "dist", "bundle.js"), []byte("built"), 0644); err != nil {
				t.Fatal(err)
			}

			createName, createFrom, createDetach = "detach-copy-"+tc.name, "", true
			t.Cleanup(func() { createName, createDetach = "", false })
			if err := runCreate(createCmd, []string{"feature/detach-copy-" + tc.name}); err != nil {
				t.Fatal(err)
			}

			wtPath := filepath.Join(filepath.Dir(dir), "testproject-detach-copy-"+tc.name)
			t.Cleanup(func() {
				remove := exec.Command("git", "worktree", "remove", "--force", wtPath)
				remove.Dir = dir
				_ = remove.Run()
			})
			_, err := os.Stat(filepath.Join(wtPath, "dist", "bundle.js"))
			if tc.wantCopied && err != nil {
				t.Fatalf("detached copy missing: %v", err)
			}
			if !tc.wantCopied && !os.IsNotExist(err) {
				t.Fatalf("copyDirs copied into detached worktree: %v", err)
			}
			s, err := state.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := s.Get("detach-copy-" + tc.name)
			if !ok || !entry.Detached {
				t.Fatalf("detached create state entry = %+v", entry)
			}
		})
	}
}

func TestCreateSkipsMissingCopyDirs(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{},
		CopyDirs:    []string{"nonexistent-dir"},
		AfterCreate: nil,
	})

	createName = ""
	createFrom = ""
	createDetach = false

	if err := runCreate(createCmd, []string{"feature/no-cache"}); err != nil {
		t.Fatalf("expected create to succeed with missing copyDirs, got: %v", err)
	}

	wtPath := filepath.Join(filepath.Dir(dir), "testproject-no-cache")

	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.AliasExists("no-cache") {
		t.Error("expected alias to be present in state")
	}

	remove := exec.Command("git", "worktree", "remove", "--force", wtPath)
	remove.Dir = dir
	if out, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("cleanup failed: %s", out)
	}
}

func TestCreateExpandsGlobPatternsAndReportsSkippedMatches(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{
		WorktreeDir: "../",
		Prefix:      "testproject",
		Symlink:     []string{"apps/*/node_modules", ".tool-versions"},
		CopyDirs:    []string{"packages/*/dist", "missing/*/dist", "pnpm-lock.yaml"},
	})
	for _, name := range []string{"apps/web/node_modules", "apps/api/node_modules", "packages/good/dist"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "packages", "good", "dist", "bundle.js"), []byte("built"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "packages", "bad"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "packages", "bad", "dist"), []byte("file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tool-versions"), []byte("nodejs 24"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte("lockfile"), 0644); err != nil {
		t.Fatal(err)
	}

	createName, createFrom, createDetach, createJSON = "glob-create", "", false, true
	t.Cleanup(func() { createName, createJSON = "", false })
	var out string
	stderr, err := captureStderr(t, func() error {
		var runErr error
		out, runErr = captureStdout(t, func() error {
			return runCreate(createCmd, []string{"feature/glob-create"})
		})
		return runErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Fatalf("quiet create wrote warnings to stderr: %q", stderr)
	}
	var result createResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("create --json output is not valid JSON: %v\n%s", err, out)
	}
	wantLinks := []string{".tool-versions", filepath.Join("apps", "api", "node_modules"), filepath.Join("apps", "web", "node_modules")}
	if !reflect.DeepEqual(result.Symlinked, wantLinks) {
		t.Fatalf("Symlinked = %v, want %v", result.Symlinked, wantLinks)
	}
	wantCopies := []string{filepath.Join("packages", "good", "dist"), "pnpm-lock.yaml"}
	if !reflect.DeepEqual(result.CopiedDirs, wantCopies) {
		t.Fatalf("CopiedDirs = %v", result.CopiedDirs)
	}
	wantSkipped := []string{filepath.Join("packages", "bad", "dist"), "missing/*/dist"}
	if !reflect.DeepEqual(result.SkippedCopyDirs, wantSkipped) {
		t.Fatalf("SkippedCopyDirs = %v, want %v", result.SkippedCopyDirs, wantSkipped)
	}

	for _, name := range wantLinks {
		info, err := os.Lstat(filepath.Join(result.Path, name))
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("expected %s to be a symlink: info=%v err=%v", name, info, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(result.Path, "packages", "good", "dist", "bundle.js"))
	if err != nil || string(data) != "built" {
		t.Fatalf("copied bundle = %q, err=%v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(result.Path, "pnpm-lock.yaml"))
	if err != nil || string(data) != "lockfile" {
		t.Fatalf("copied literal file = %q, err=%v", data, err)
	}

	remove := exec.Command("git", "worktree", "remove", "--force", result.Path)
	remove.Dir = dir
	if out, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("cleanup failed: %s", out)
	}
}

func TestCreatePreservesStateAddedAfterInitialLoad(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := state.Update(dir, func(latest *state.State) error {
		return latest.Add("payments", "feature/payments", "/tmp/payments", 0)
	}); err != nil {
		t.Fatal(err)
	}

	result, err := doCreate(dir, cfg, &stale, "feature/auth", "auth", "", false, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = git.RemoveWorktree(result.Path, true) })

	loaded, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Get("payments"); !ok {
		t.Fatal("concurrent payments entry was lost")
	}
	if _, ok := loaded.Get("auth"); !ok {
		t.Fatal("created auth entry is missing")
	}
}
