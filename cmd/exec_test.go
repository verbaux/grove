package cmd

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func TestExecTargetSetsWorktreeEnv(t *testing.T) {
	dir := setupIntegrationRepo(t, config.Config{WorktreeDir: "../", Prefix: "ex", Symlink: []string{}})
	createName, createFrom, createDetach, createJSON = "", "", false, false
	if err := runCreate(createCmd, []string{"feature/run"}); err != nil {
		t.Fatal(err)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := s.Get("run")
	t.Setenv("GROVE_PORT", "1")
	t.Setenv("GROVE_KEEP", "yes")

	path, env, err := execTarget(dir, "run")
	if err != nil {
		t.Fatal(err)
	}
	if path != entry.Path {
		t.Fatalf("path = %q, want %q", path, entry.Path)
	}
	grove := groveVars(env)
	want := []string{
		"GROVE_ALIAS=run",
		"GROVE_BRANCH=feature/run",
		"GROVE_KEEP=yes",
		"GROVE_PATH=" + entry.Path,
		fmt.Sprintf("GROVE_PORT=%d", entry.Port),
	}
	if !slices.Equal(grove, want) {
		t.Fatalf("GROVE_* env = %v, want %v", grove, want)
	}

	_, env, err = execTarget(dir, "1")
	if err != nil {
		t.Fatal(err)
	}
	if got := groveVars(env); !slices.Equal(got, []string{"GROVE_BRANCH=main", "GROVE_KEEP=yes", "GROVE_PATH=" + dir}) {
		t.Fatalf("main worktree GROVE_* env = %v", got)
	}

	if _, _, err := execTarget(dir, "missing"); err == nil {
		t.Fatal("expected error for unknown worktree")
	}
}

func groveVars(env []string) []string {
	var out []string
	for _, kv := range env {
		if strings.HasPrefix(kv, "GROVE_") {
			out = append(out, kv)
		}
	}
	slices.Sort(out)
	return out
}
