package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/state"
)

func TestListOutputsWorktreeNoteWithoutChangingPlainMode(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	createName, createFrom, createDetach, createJSON = "", "", false, false
	if err := runCreate(createCmd, []string{"feature/noted-list"}); err != nil {
		t.Fatal(err)
	}
	if err := state.Update(root, func(latest *state.State) error {
		return latest.SetNote("noted-list", "keep through launch")
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := loaded.Get("noted-list")
	t.Cleanup(func() { cleanupWorktree(t, root, entry.Path) })

	if err := listCmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = listCmd.Flags().Set("json", "false")
		_ = listCmd.Flags().Set("plain", "false")
	})
	jsonOut, err := captureStdout(t, func() error { return runList(listCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Name string `json:"name"`
		Note string `json:"note"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &rows); err != nil {
		t.Fatalf("list JSON invalid: %v\n%s", err, jsonOut)
	}
	var found bool
	for _, row := range rows {
		if row.Name == "noted-list" {
			found = true
			if row.Note != "keep through launch" {
				t.Fatalf("list JSON note = %q", row.Note)
			}
		}
	}
	if !found {
		t.Fatalf("noted worktree missing from list JSON: %+v", rows)
	}

	if err := listCmd.Flags().Set("json", "false"); err != nil {
		t.Fatal(err)
	}
	humanOut, err := captureStdout(t, func() error { return runList(listCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(humanOut, "NOTE") || !strings.Contains(humanOut, "keep through launch") {
		t.Fatalf("human list omitted note column: %q", humanOut)
	}

	if err := listCmd.Flags().Set("plain", "true"); err != nil {
		t.Fatal(err)
	}
	plainOut, err := captureStdout(t, func() error { return runList(listCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}
	if plainOut != "noted-list\n" {
		t.Fatalf("plain list changed: %q", plainOut)
	}
}
