package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOriginHeadsSSHDiagnostics(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "successful warning"
		if fail {
			name = "failed transport"
		}
		t.Run(name, func(t *testing.T) {
			repo := setupTestRepo(t)
			const remoteURL = "ssh://git@origin.test/repository"
			for _, args := range [][]string{{"branch", "published"}, {"remote", "add", "origin", remoteURL}} {
				if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
					t.Fatalf("git %v: %s: %v", args, out, err)
				}
			}
			ssh := filepath.Join(t.TempDir(), "ssh")
			script := `#!/bin/sh
echo 'Warning: Permanently added origin.test to known hosts.' >&2
if [ "$GROVE_TEST_SSH_FAIL" = 1 ]; then
  echo 'test transport denied' >&2
  exit 1
fi
exec git-upload-pack "$GROVE_TEST_ORIGIN"
`
			if err := os.WriteFile(ssh, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GIT_SSH_COMMAND", "'"+strings.ReplaceAll(ssh, "'", "'\\''")+"'")
			t.Setenv("GIT_SSH_VARIANT", "simple")
			t.Setenv("GROVE_TEST_ORIGIN", repo)
			t.Setenv("GROVE_TEST_SSH_FAIL", "0")
			if fail {
				t.Setenv("GROVE_TEST_SSH_FAIL", "1")
			}
			url, heads, err := OriginHeads(repo)
			if fail {
				if err == nil || !strings.Contains(err.Error(), "test transport denied") {
					t.Fatalf("transport failure must retain stderr: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("successful SSH warning must not prevent branch lookup: %v", err)
			}
			if url != remoteURL || !heads["published"] || heads["missing"] {
				t.Fatalf("unexpected origin branches: url=%q heads=%v", url, heads)
			}
		})
	}
}
