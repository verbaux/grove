package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/processes"
	"github.com/verbaux/grove/internal/state"
)

func TestBuildPSSortsAndClassifiesManagedWorktrees(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"zeta":     {Branch: "feature/zeta", Path: "/tmp/zeta"},
		"payments": {Branch: "feature/payments", Path: "/tmp/payments", Port: 3555},
		"auth":     {Branch: "feature/auth", Path: "/tmp/auth", Port: 3487},
	}}
	snapshot := processes.Snapshot{
		Source: "lsof",
		Listeners: map[int][]processes.Listener{
			3487: {{PID: 123, Command: "node"}},
		},
	}

	result := buildPS(s, snapshot)
	if result.Source != "lsof" || len(result.Worktrees) != 3 {
		t.Fatalf("result = %+v", result)
	}
	if result.Worktrees[0].Alias != "auth" || result.Worktrees[0].Status != "running" {
		t.Fatalf("auth row = %+v", result.Worktrees[0])
	}
	if result.Worktrees[1].Alias != "payments" || result.Worktrees[1].Status != "stopped" {
		t.Fatalf("payments row = %+v", result.Worktrees[1])
	}
	if result.Worktrees[2].Alias != "zeta" || result.Worktrees[2].Status != "unassigned" {
		t.Fatalf("zeta row = %+v", result.Worktrees[2])
	}
}

func TestPSJSONUsesStableResultShape(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"auth": {Branch: "feature/auth", Path: "/tmp/auth", Port: 3487},
	}}
	if err := state.Save(root, s); err != nil {
		t.Fatal(err)
	}

	psJSON = true
	t.Cleanup(func() { psJSON = false })
	out, err := captureStdout(t, func() error {
		return runPSWithFinder(func() (processes.Snapshot, error) {
			return processes.Snapshot{
				Source:    "lsof",
				Listeners: map[int][]processes.Listener{3487: {{PID: 123, Command: "node"}}},
			}, nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	var result psResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if result.Source != "lsof" || len(result.Worktrees) != 1 {
		t.Fatalf("result = %+v", result)
	}
	row := result.Worktrees[0]
	if row.Alias != "auth" || row.Status != "running" || len(row.Listeners) != 1 || row.Listeners[0].PID != 123 {
		t.Fatalf("row = %+v", row)
	}
}

func TestPSSkipsListenerScanWithoutManagedWorktrees(t *testing.T) {
	setupIntegrationRepo(t, baseConfig())
	called := false

	out, err := captureStdout(t, func() error {
		return runPSWithFinder(func() (processes.Snapshot, error) {
			called = true
			return processes.Snapshot{}, nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("listener finder should not run without managed worktrees")
	}
	if !strings.Contains(out, "No managed worktrees") {
		t.Fatalf("output = %q", out)
	}
}

func TestPSReturnsListenerDiscoveryError(t *testing.T) {
	root := setupIntegrationRepo(t, baseConfig())
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"auth": {Branch: "feature/auth", Path: "/tmp/auth", Port: 3487},
	}}
	if err := state.Save(root, s); err != nil {
		t.Fatal(err)
	}

	err := runPSWithFinder(func() (processes.Snapshot, error) {
		return processes.Snapshot{}, errors.New("discovery failed")
	})
	if err == nil || !strings.Contains(err.Error(), "discovery failed") {
		t.Fatalf("error = %v", err)
	}
}
