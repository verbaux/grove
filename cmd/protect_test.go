package cmd

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

func TestProtectAndUnprotectByIndex(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach = "", "", false
	if err := runCreate(createCmd, []string{"feature/protect-me"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	idx, alias := nonMainIndex(t, dir)
	if err := runProtect(protectCmd, []string{idx}); err != nil {
		t.Fatalf("protect: %v", err)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := s.Get(alias)
	if !ok || !entry.Protected {
		t.Fatalf("expected %q protected, got %+v", alias, entry)
	}

	if err := runUnprotect(unprotectCmd, []string{idx}); err != nil {
		t.Fatalf("unprotect: %v", err)
	}
	s, err = state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok = s.Get(alias)
	if !ok || entry.Protected {
		t.Fatalf("expected %q unprotected, got %+v", alias, entry)
	}

	cleanupWorktree(t, dir, entry.Path)
}

func TestRemoveRefusesProtectedWorktree(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach = "", "", false
	if err := runCreate(createCmd, []string{"feature/protected-remove"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	idx, alias := nonMainIndex(t, dir)
	resolved, _ := resolveWorktree(dir, idx)
	path := resolved.Path
	if err := runProtect(protectCmd, []string{idx}); err != nil {
		t.Fatal(err)
	}

	removeForce = true
	removeIncludeProtected = false
	t.Cleanup(func() {
		removeForce = false
		removeIncludeProtected = false
	})
	err := runRemove(removeCmd, []string{idx})
	if err == nil {
		t.Fatal("expected protected remove to fail")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("protected worktree should still exist: %v", statErr)
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if entry, ok := s.Get(alias); !ok || !entry.Protected {
		t.Fatalf("protected state should remain, got %+v", entry)
	}

	removeIncludeProtected = true
	if err := runRemove(removeCmd, []string{idx}); err != nil {
		t.Fatalf("remove with include protected: %v", err)
	}
}

func TestListJSONIncludesProtected(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach = "", "", false
	if err := runCreate(createCmd, []string{"feature/list-protected"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	idx, alias := nonMainIndex(t, dir)
	if err := runProtect(protectCmd, []string{idx}); err != nil {
		t.Fatal(err)
	}

	listCmd.Flags().Set("json", "true")
	t.Cleanup(func() { listCmd.Flags().Set("json", "false") })
	out, err := captureStdout(t, func() error {
		return runList(listCmd, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name      string `json:"name"`
		Protected bool   `json:"protected"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("list json invalid: %v\n%s", err, out)
	}
	found := false
	for _, row := range rows {
		if row.Name == alias {
			found = true
			if !row.Protected {
				t.Fatalf("expected protected=true for %+v", row)
			}
		}
	}
	if !found {
		t.Fatalf("did not find %q in list output: %+v", alias, rows)
	}

	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := s.Get(alias)
	if err := config.Save(dir, baseConfig()); err != nil {
		t.Fatal(err)
	}
	removeIncludeProtected = true
	cleanupWorktree(t, dir, entry.Path)
}

func TestProtectionRejectsReusedAlias(t *testing.T) {
	dir := setupIntegrationRepo(t, baseConfig())
	original := &resolvedWorktree{
		Alias: "auth", Branch: "feature/auth", Path: "/tmp/auth", InState: true,
	}
	if err := state.Update(dir, func(latest *state.State) error {
		return latest.Add("auth", "feature/replacement", "/tmp/replacement", 3001)
	}); err != nil {
		t.Fatal(err)
	}

	err := updateProtection(dir, original, true)
	if err == nil || !strings.Contains(err.Error(), "changed while updating protection") {
		t.Fatalf("updateProtection error = %v, want identity conflict", err)
	}

	loaded, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	replacement, ok := loaded.Get("auth")
	if !ok {
		t.Fatal("replacement entry is missing")
	}
	if replacement.Protected {
		t.Fatal("replacement worktree must not inherit protection intended for the old alias")
	}
}
