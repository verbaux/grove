package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a real git repo in a temp directory with one commit.
// Tests need a real repo because we're testing actual git commands.
func setupTestRepo(t *testing.T) string {
	t.Helper() // marks this as a helper — errors point to the calling test, not here

	// filepath.EvalSymlinks resolves macOS /tmp → /private/var/folders/... difference.
	// Without this, git returns the real path but t.TempDir() returns the symlinked one.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %s", args, string(out))
		}
	}

	// git commands in tests need to run from the repo directory.
	// t.Cleanup restores the original cwd after the test — like afterEach in Jest.
	// Without this, Chdir would affect all subsequent tests in the package.
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

func TestListWorktrees(t *testing.T) {
	dir := setupTestRepo(t)

	worktrees, err := ListWorktrees()
	if err != nil {
		t.Fatal("ListWorktrees failed:", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(worktrees))
	}
	if worktrees[0].Path != dir {
		t.Errorf("path = %q, want %q", worktrees[0].Path, dir)
	}
	if !worktrees[0].IsMain {
		t.Error("expected first worktree to be main")
	}
}

func TestAddAndRemoveWorktree(t *testing.T) {
	setupTestRepo(t)

	wtPath := filepath.Join(t.TempDir(), "test-worktree")

	if err := AddWorktree(wtPath, "test-branch", ""); err != nil {
		t.Fatal("AddWorktree failed:", err)
	}

	worktrees, err := ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(worktrees))
	}
	if worktrees[1].Branch != "test-branch" {
		t.Errorf("branch = %q, want %q", worktrees[1].Branch, "test-branch")
	}

	if err := RemoveWorktree(wtPath, false); err != nil {
		t.Fatal("RemoveWorktree failed:", err)
	}

	worktrees, err = ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("expected 1 worktree after remove, got %d", len(worktrees))
	}
}

func TestRemoveWorktreeWithInitializedSubmodule(t *testing.T) {
	repo := setupTestRepo(t)
	submodule := setupTestRepo(t)
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	gitIn(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", submodule, "vendor/example")
	gitIn(t, repo, "commit", "-am", "add submodule")

	wtPath := filepath.Join(t.TempDir(), "submodule-worktree")
	if err := AddWorktree(wtPath, "submodule-branch", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveWorktree(wtPath, true) })
	gitIn(t, wtPath, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")
	if _, err := os.Stat(filepath.Join(wtPath, ".gitmodules")); err != nil {
		t.Fatalf("test worktree has no submodule configuration: %v", err)
	}
	hasSubmodules, err := HasSubmodulesBlockingRemoval(wtPath)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSubmodules {
		t.Fatal("initialized submodule was not detected")
	}

	submodulePath := filepath.Join(wtPath, "vendor/example")
	if err := os.WriteFile(filepath.Join(submodulePath, "local.txt"), []byte("local commit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, submodulePath, "add", "local.txt")
	gitIn(t, submodulePath, "commit", "-m", "local submodule commit")
	localCommit := gitOutput(t, submodulePath, "rev-parse", "HEAD")
	submoduleGitDir := gitOutput(t, submodulePath, "rev-parse", "--absolute-git-dir")
	gitIn(t, wtPath, "add", "vendor/example")
	gitIn(t, wtPath, "commit", "-m", "record local submodule commit")
	gitIn(t, wtPath, "submodule", "deinit", "-f", "vendor/example")

	hasSubmodules, err = HasSubmodulesBlockingRemoval(wtPath)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSubmodules {
		t.Fatal("retained submodule repository was not detected after deinit")
	}
	if err := MoveWorktree(wtPath, wtPath+"-moved"); !errors.Is(err, ErrSubmoduleMoveUnsupported) || !strings.Contains(err.Error(), "git worktree move") {
		t.Fatalf("MoveWorktree error = %v, want sentinel and original Git error", err)
	}

	err = RemoveWorktree(wtPath, false)
	if !errors.Is(err, ErrSubmoduleForceRequired) {
		t.Fatalf("RemoveWorktree error = %v, want force guidance", err)
	}
	catFile := exec.Command("git", "--git-dir", submoduleGitDir, "cat-file", "-t", localCommit)
	catFile.Dir = wtPath
	out, catErr := catFile.CombinedOutput()
	if catErr != nil || strings.TrimSpace(string(out)) != "commit" {
		t.Fatalf("local submodule commit was lost after refused removal: %v: %s", catErr, out)
	}

	if err := RemoveWorktree(wtPath, true); err != nil {
		t.Fatalf("forced RemoveWorktree failed: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree path still exists: %v", err)
	}
	worktrees, err := ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(worktrees) != 1 {
		t.Fatalf("expected only main worktree, got %+v", worktrees)
	}
}

func TestStatusRespectsIgnoredDirtySubmodule(t *testing.T) {
	repo := setupTestRepo(t)
	submodule := setupTestRepo(t)
	if err := os.WriteFile(filepath.Join(submodule, "tracked.txt"), []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, submodule, "add", "tracked.txt")
	gitIn(t, submodule, "commit", "-m", "add tracked file")
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	gitIn(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", submodule, "vendor/example")
	gitIn(t, repo, "config", "-f", ".gitmodules", "submodule.vendor/example.ignore", "all")
	gitIn(t, repo, "add", ".gitmodules")
	gitIn(t, repo, "commit", "-m", "add ignored submodule")

	wtPath := filepath.Join(t.TempDir(), "ignored-submodule-worktree")
	if err := AddWorktree(wtPath, "ignored-submodule-branch", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveWorktree(wtPath, true) })
	gitIn(t, wtPath, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")
	if err := os.WriteFile(filepath.Join(wtPath, "vendor/example/tracked.txt"), []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "vendor/example/untracked.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err := Status(wtPath)
	if err != nil {
		t.Fatal(err)
	}
	if status != "clean" {
		t.Fatalf("Status = %q, want clean because submodule ignore=all", status)
	}
	if err := RemoveWorktree(wtPath, false); !errors.Is(err, ErrSubmoduleForceRequired) {
		t.Fatalf("RemoveWorktree error = %v, want force guidance", err)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "vendor/example/tracked.txt")); err != nil {
		t.Fatalf("dirty submodule content was removed: %v", err)
	}
}

func TestRecognizesSubmoduleWorktreeRemoveError(t *testing.T) {
	err := errors.New("fatal: working trees containing submodules cannot be moved or removed")
	if !isSubmoduleWorktreeOperationError(err) {
		t.Fatal("expected submodule worktree error to be recognized")
	}
	if isSubmoduleWorktreeOperationError(errors.New("fatal: permission denied")) {
		t.Fatal("unrelated error must not enable destructive fallback")
	}
}

func TestHasSubmodulesBlockingRemovalReportsUnknownWithGitlinkMissingMapping(t *testing.T) {
	repo := setupTestRepo(t)
	nested := filepath.Join(repo, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, nested, "init")
	gitIn(t, nested, "config", "user.email", "test@test.com")
	gitIn(t, nested, "config", "user.name", "Test")
	gitIn(t, nested, "commit", "--allow-empty", "-m", "nested")
	gitIn(t, repo, "add", "nested")
	gitIn(t, repo, "commit", "-m", "add gitlink without mapping")

	blocking, err := HasSubmodulesBlockingRemoval(repo)
	if !errors.Is(err, ErrSubmoduleSafetyUnknown) {
		t.Fatalf("error = %v, want ErrSubmoduleSafetyUnknown", err)
	}
	if blocking {
		t.Fatal("inconclusive scan must not report a definite submodule result")
	}
	if !strings.Contains(err.Error(), "submodule status --recursive") {
		t.Fatalf("error does not retain the failing Git command: %v", err)
	}
	if err.Error() == ErrSubmoduleSafetyUnknown.Error() {
		t.Fatalf("error discarded the underlying Git failure: %v", err)
	}
}

func TestMoveWorktree(t *testing.T) {
	setupTestRepo(t)

	parent := t.TempDir()
	oldPath := filepath.Join(parent, "old-worktree")
	newPath := filepath.Join(parent, "new-worktree")
	if err := AddWorktree(oldPath, "move-branch", ""); err != nil {
		t.Fatal("AddWorktree failed:", err)
	}
	t.Cleanup(func() {
		_ = RemoveWorktree(oldPath, true)
		_ = RemoveWorktree(newPath, true)
	})

	if err := MoveWorktree(oldPath, newPath); err != nil {
		t.Fatal("MoveWorktree failed:", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old path should not exist after move: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new path should exist after move: %v", err)
	}
	resolvedNewPath, err := filepath.EvalSymlinks(newPath)
	if err != nil {
		t.Fatal(err)
	}

	worktrees, err := ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, wt := range worktrees {
		if wt.Path == resolvedNewPath && wt.Branch == "move-branch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("moved worktree not found in git worktree list: %+v", worktrees)
	}
}

func TestStatusClean(t *testing.T) {
	dir := setupTestRepo(t)

	status, err := Status(dir)
	if err != nil {
		t.Fatal("Status failed:", err)
	}
	if status != "clean" {
		t.Errorf("status = %q, want %q", status, "clean")
	}
}

func TestAheadBehind(t *testing.T) {
	dir := setupTestRepo(t)
	base := "main"
	if _, err := run("rev-parse", "--verify", "refs/heads/main"); err != nil {
		base = "master"
	}
	gitIn(t, dir, "checkout", "-b", "feature/ahead")
	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "feature.txt")
	gitIn(t, dir, "commit", "-m", "feature")

	ahead, behind, err := AheadBehind("feature/ahead", base)
	if err != nil {
		t.Fatal(err)
	}
	if ahead != 1 || behind != 0 {
		t.Fatalf("AheadBehind = ahead %d behind %d, want ahead 1 behind 0", ahead, behind)
	}
}

// gitIn runs a git command with its working directory set to dir.
// Fails the test immediately if the command errors — use this for test setup, not assertions.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %s", args, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %s", args, out)
	}
	return strings.TrimSpace(string(out))
}

func TestStatusModified(t *testing.T) {
	dir := setupTestRepo(t)

	// Create, commit, then modify a tracked file.
	file := filepath.Join(dir, "tracked.txt")
	if err := os.WriteFile(file, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "tracked.txt")
	gitIn(t, dir, "commit", "-m", "add tracked")
	if err := os.WriteFile(file, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err := Status(dir)
	if err != nil {
		t.Fatal("Status failed:", err)
	}
	if status != "1 modified" {
		t.Errorf("status = %q, want %q", status, "1 modified")
	}
}

func TestStatusUntracked(t *testing.T) {
	dir := setupTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	status, err := Status(dir)
	if err != nil {
		t.Fatal("Status failed:", err)
	}
	if status != "1 untracked" {
		t.Errorf("status = %q, want %q", status, "1 untracked")
	}
}

func TestStatusStaged(t *testing.T) {
	dir := setupTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "staged.txt")

	status, err := Status(dir)
	if err != nil {
		t.Fatal("Status failed:", err)
	}
	if status != "1 staged" {
		t.Errorf("status = %q, want %q", status, "1 staged")
	}
}

// commitFile writes a file, stages it, and commits — returns nothing, fails on error.
func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", name)
	gitIn(t, dir, "commit", "-m", msg)
}

// currentBranch returns the checked-out branch name — git's default branch
// for a fresh repo varies (master vs main) by version, so tests read it.
func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD failed: %v", err)
	}
	return string(out[:len(out)-1])
}

func TestIsMergedRegularMerge(t *testing.T) {
	dir := setupTestRepo(t)
	base := currentBranch(t, dir)

	gitIn(t, dir, "checkout", "-b", "feature")
	commitFile(t, dir, "feature.txt", "work", "feature work")
	gitIn(t, dir, "checkout", base)
	gitIn(t, dir, "merge", "--no-ff", "feature", "-m", "merge feature")

	merged, err := IsMerged("feature", base)
	if err != nil {
		t.Fatal("IsMerged failed:", err)
	}
	if !merged {
		t.Error("expected feature to be detected as merged")
	}
}

func TestIsMergedSquashMerge(t *testing.T) {
	dir := setupTestRepo(t)
	base := currentBranch(t, dir)

	gitIn(t, dir, "checkout", "-b", "feature")
	commitFile(t, dir, "a.txt", "one", "commit a")
	commitFile(t, dir, "b.txt", "two", "commit b")
	gitIn(t, dir, "checkout", base)
	// Squash merge: apply the branch's combined changes as a single commit
	// whose history does NOT include the feature tips.
	gitIn(t, dir, "merge", "--squash", "feature")
	gitIn(t, dir, "commit", "-m", "squashed feature")

	merged, err := IsMerged("feature", base)
	if err != nil {
		t.Fatal("IsMerged failed:", err)
	}
	if !merged {
		t.Error("expected squash-merged feature to be detected as merged")
	}
}

func TestIsMergedUnstartedBranch(t *testing.T) {
	dir := setupTestRepo(t)
	base := currentBranch(t, dir)

	// Create a branch but commit nothing on it, then advance base.
	// The branch tip is contained in base (ahead 0) but sits on the trunk —
	// it is unstarted, not merged, and must not be pruned.
	gitIn(t, dir, "branch", "feature")
	commitFile(t, dir, "base.txt", "more", "base advances")

	merged, err := IsMerged("feature", base)
	if err != nil {
		t.Fatal("IsMerged failed:", err)
	}
	if merged {
		t.Error("expected unstarted branch (no commits ahead) to be not merged")
	}
}

func TestIsMergedNotMerged(t *testing.T) {
	dir := setupTestRepo(t)
	base := currentBranch(t, dir)

	gitIn(t, dir, "checkout", "-b", "feature")
	commitFile(t, dir, "feature.txt", "work", "feature work")
	gitIn(t, dir, "checkout", base)

	merged, err := IsMerged("feature", base)
	if err != nil {
		t.Fatal("IsMerged failed:", err)
	}
	if merged {
		t.Error("expected un-merged feature to be detected as not merged")
	}
}

func TestDefaultBranch(t *testing.T) {
	dir := setupTestRepo(t)

	// No remote configured — falls back to the main worktree branch.
	if got := DefaultBranch(); got != currentBranch(t, dir) {
		t.Errorf("DefaultBranch() = %q, want %q", got, currentBranch(t, dir))
	}
}
