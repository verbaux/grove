package cmd

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
)

func TestReviewBranchIsolatesForkPRs(t *testing.T) {
	fork := reviewBranch(prInfo{Number: 42, HeadRefName: "main", IsCrossRepository: true})
	if fork != "pr-42" {
		t.Errorf("fork PR branch = %q, want pr-42", fork)
	}
	same := reviewBranch(prInfo{Number: 7, HeadRefName: "feature/x"})
	if same != "feature/x" {
		t.Errorf("same-repo PR branch = %q, want feature/x", same)
	}
}

func TestSyncReviewBranchFastForwardsStaleBranch(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{WorktreeDir: "../", Symlink: []string{}})
	runGit := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("branch", "feature/stale")
	runGit("commit", "--allow-empty", "-m", "newer PR commit")
	tip := runGit("rev-parse", "HEAD")

	if _, err := captureStdout(t, func() error { return syncReviewBranch("feature/stale", "main") }); err != nil {
		t.Fatal(err)
	}
	if got := runGit("rev-parse", "feature/stale"); got != tip {
		t.Errorf("feature/stale = %s, want %s", got, tip)
	}
}
