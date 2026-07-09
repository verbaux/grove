package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// In Go, tests live next to the code they test (same package).
// Every test function starts with Test and takes *testing.T.
// t.Fatal = stop this test immediately (like throwing in JS).
// t.Errorf = log failure but keep going (like console.error + a flag).

func TestSaveAndLoad(t *testing.T) {
	// t.TempDir() creates a temp directory that's auto-cleaned after the test.
	// Like creating a temp folder in beforeEach and deleting in afterEach.
	dir := t.TempDir()

	want := Config{
		WorktreeDir: "../",
		Prefix:      "myproject",
		Symlink:     []string{"node_modules", ".yarn"},
		AfterCreate: HookCommands{"make setup"},
	}

	if err := Save(dir, want); err != nil {
		t.Fatal("Save failed:", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatal("Load failed:", err)
	}

	// Go doesn't have deep equality built into == for slices,
	// so we compare field by field.
	if got.WorktreeDir != want.WorktreeDir {
		t.Errorf("WorktreeDir = %q, want %q", got.WorktreeDir, want.WorktreeDir)
	}
	if got.Prefix != want.Prefix {
		t.Errorf("Prefix = %q, want %q", got.Prefix, want.Prefix)
	}
	if len(got.AfterCreate) != len(want.AfterCreate) {
		t.Fatalf("AfterCreate len = %d, want %d", len(got.AfterCreate), len(want.AfterCreate))
	}
	for i := range want.AfterCreate {
		if got.AfterCreate[i] != want.AfterCreate[i] {
			t.Errorf("AfterCreate[%d] = %q, want %q", i, got.AfterCreate[i], want.AfterCreate[i])
		}
	}
	if len(got.Symlink) != len(want.Symlink) {
		t.Fatalf("Symlink length = %d, want %d", len(got.Symlink), len(want.Symlink))
	}
	for i := range want.Symlink {
		if got.Symlink[i] != want.Symlink[i] {
			t.Errorf("Symlink[%d] = %q, want %q", i, got.Symlink[i], want.Symlink[i])
		}
	}
}

func TestLoadMissing(t *testing.T) {
	dir := t.TempDir()

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error when .groverc.json is missing")
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	os.WriteFile(path, []byte("{bad json}"), 0644)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFindRootWalkUp(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, Default()); err != nil {
		t.Fatal(err)
	}

	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}

	got, err := FindRoot(child)
	if err != nil {
		t.Fatal("FindRoot failed:", err)
	}

	// EvalSymlinks to normalize /tmp vs /private/tmp on macOS
	want, _ := filepath.EvalSymlinks(root)
	got, _ = filepath.EvalSymlinks(got)
	if got != want {
		t.Errorf("FindRoot = %q, want %q", got, want)
	}
}

func TestFindRootWorktree(t *testing.T) {
	root := t.TempDir()

	run := func(dir, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v failed: %s\n%s", name, args, err, out)
		}
	}

	run(root, "git", "init")
	run(root, "git", "config", "user.email", "test@test.com")
	run(root, "git", "config", "user.name", "Test")

	// Commit a dummy file so the repo has a HEAD commit
	os.WriteFile(filepath.Join(root, "README"), []byte("x"), 0644)
	run(root, "git", "add", ".")
	run(root, "git", "commit", "-m", "init")

	// Create the worktree BEFORE adding .groverc.json, so the worktree
	// won't have the config file (it's untracked in the main repo).
	wtDir := filepath.Join(t.TempDir(), "worktree")
	run(root, "git", "worktree", "add", wtDir, "-b", "test-branch")
	t.Cleanup(func() {
		cmd := exec.Command("git", "worktree", "remove", "--force", wtDir)
		cmd.Dir = root
		cmd.Run()
	})

	// Now place .groverc.json in the main repo (untracked)
	if err := Save(root, Default()); err != nil {
		t.Fatal(err)
	}

	got, err := FindRoot(wtDir)
	if err != nil {
		t.Fatal("FindRoot from worktree failed:", err)
	}

	want, _ := filepath.EvalSymlinks(root)
	got, _ = filepath.EvalSymlinks(got)
	if got != want {
		t.Errorf("FindRoot = %q, want %q", got, want)
	}
}

func TestFindRootNoConfig(t *testing.T) {
	dir := t.TempDir()

	_, err := FindRoot(dir)
	if err == nil {
		t.Fatal("expected error when no .groverc.json exists anywhere")
	}
}

func TestHookCommandsUnmarshalString(t *testing.T) {
	var a HookCommands
	if err := json.Unmarshal([]byte(`"npm install"`), &a); err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || a[0] != "npm install" {
		t.Errorf("got %v, want [npm install]", a)
	}
}

func TestHookCommandsUnmarshalArray(t *testing.T) {
	var a HookCommands
	if err := json.Unmarshal([]byte(`["npm ci","npm run build"]`), &a); err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 || a[0] != "npm ci" || a[1] != "npm run build" {
		t.Errorf("got %v, want [npm ci, npm run build]", a)
	}
}

func TestHookCommandsUnmarshalEmpty(t *testing.T) {
	var a HookCommands
	if err := json.Unmarshal([]byte(`""`), &a); err != nil {
		t.Fatal(err)
	}
	if len(a) != 0 {
		t.Errorf("empty string should yield empty, got %v", a)
	}
}

func TestHookCommandsMarshalSingle(t *testing.T) {
	a := HookCommands{"npm install"}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"npm install"` {
		t.Errorf("got %s, want \"npm install\"", data)
	}
}

func TestHookCommandsMarshalArray(t *testing.T) {
	a := HookCommands{"a", "b"}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `["a","b"]` {
		t.Errorf("got %s, want [a,b]", data)
	}
}

func TestHookCommandsMarshalEmpty(t *testing.T) {
	a := HookCommands{}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `""` {
		t.Errorf("got %s, want empty string", data)
	}
}

func TestConfigBackwardCompatString(t *testing.T) {
	dir := t.TempDir()
	raw := `{"worktreeDir":"../","prefix":"x","symlink":["node_modules"],"afterCreate":"npm install"}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AfterCreate) != 1 || got.AfterCreate[0] != "npm install" {
		t.Errorf("got %v", got.AfterCreate)
	}
}

func TestConfigArrayForm(t *testing.T) {
	dir := t.TempDir()
	raw := `{"worktreeDir":"../","prefix":"x","symlink":["node_modules"],"afterCreate":["npm ci","npm run build"]}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AfterCreate) != 2 {
		t.Fatalf("got %v", got.AfterCreate)
	}
}

func TestRemoveHooksLoadStringOrArray(t *testing.T) {
	dir := t.TempDir()
	raw := `{"worktreeDir":"../","prefix":"x","symlink":[],"beforeRemove":"npm run stop","afterRemove":["docker compose down","rm -rf /tmp/x"]}`
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.BeforeRemove) != 1 || got.BeforeRemove[0] != "npm run stop" {
		t.Fatalf("beforeRemove = %v", got.BeforeRemove)
	}
	if len(got.AfterRemove) != 2 || got.AfterRemove[0] != "docker compose down" || got.AfterRemove[1] != "rm -rf /tmp/x" {
		t.Fatalf("afterRemove = %v", got.AfterRemove)
	}
}
