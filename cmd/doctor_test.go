package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/git"
	"github.com/verbaux/grove/internal/state"
)

func TestDoctorJSON(t *testing.T) {
	setupIntegrationRepo(t, config.Config{})
	doctorJSON = true
	t.Cleanup(func() { doctorJSON = false })

	out, err := captureStdout(t, func() error {
		return runDoctor(doctorCmd, nil)
	})
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		OK          bool `json:"ok"`
		Diagnostics []struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("doctor --json output is not valid JSON: %v\n%s", err, out)
	}
	if !result.OK {
		t.Fatalf("expected doctor JSON ok=true, got %+v", result)
	}
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected diagnostics in JSON output")
	}
	if result.Diagnostics[0].Level == "" || result.Diagnostics[0].Message == "" {
		t.Fatalf("expected level and message in first diagnostic, got %+v", result.Diagnostics[0])
	}
}

func TestDoctorFixRemovesStaleStateAndReportsJSON(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{})
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"stale": {
			Branch:  "feature/stale",
			Path:    filepath.Join(root, "missing-worktree"),
			Port:    3123,
			Created: time.Now(),
		},
	}}
	if err := state.Save(root, s); err != nil {
		t.Fatal(err)
	}

	doctorJSON = true
	doctorFixMode = true
	t.Cleanup(func() {
		doctorJSON = false
		doctorFixMode = false
	})

	out, err := captureStdout(t, func() error {
		return runDoctor(doctorCmd, nil)
	})
	if err != nil {
		t.Fatal(err)
	}

	var result doctorJSONResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("doctor --fix --json output is not valid JSON: %v\n%s", err, out)
	}
	if !result.OK {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Fixes) != 1 {
		t.Fatalf("fixes = %+v, want one stale-state fix", result.Fixes)
	}
	fix := result.Fixes[0]
	if fix.Action != "remove-stale-state" || fix.Target != "stale" || fix.Status != "fixed" {
		t.Fatalf("fix = %+v", fix)
	}
	for _, diag := range result.Diagnostics {
		if strings.Contains(diag.Message, "stale state") {
			t.Fatalf("post-fix diagnostics still contain stale error: %+v", result.Diagnostics)
		}
	}

	loaded, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Get("stale"); ok {
		t.Fatal("stale state entry was not removed")
	}
}

func TestDoctorWithoutFixLeavesStaleState(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{})
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"stale": {Branch: "feature/stale", Path: filepath.Join(root, "missing-worktree")},
	}}
	if err := state.Save(root, s); err != nil {
		t.Fatal(err)
	}

	doctorJSON = true
	doctorFixMode = false
	t.Cleanup(func() { doctorJSON = false })
	if _, err := captureStdout(t, func() error { return runDoctor(doctorCmd, nil) }); err == nil {
		t.Fatal("doctor without --fix should still report stale state as an error")
	}

	loaded, err := state.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Get("stale"); !ok {
		t.Fatal("read-only doctor removed stale state")
	}
}

func TestDoctorFixRepairsBrokenSymlink(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{Symlink: []string{"node_modules"}})
	canonical := filepath.Join(root, "node_modules")
	if err := os.Mkdir(canonical, 0755); err != nil {
		t.Fatal(err)
	}
	worktree := t.TempDir()
	link := filepath.Join(worktree, "node_modules")
	if err := os.Symlink(filepath.Join(worktree, "missing-target"), link); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := config.SetupHash(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Save(root, state.State{Worktrees: map[string]state.WorktreeEntry{
		"auth": {Branch: "feature/auth", Path: worktree, ConfigHash: hash, Created: time.Now()},
	}}); err != nil {
		t.Fatal(err)
	}

	doctorJSON = true
	doctorFixMode = true
	t.Cleanup(func() {
		doctorJSON = false
		doctorFixMode = false
	})
	out, err := captureStdout(t, func() error { return runDoctor(doctorCmd, nil) })
	if err != nil {
		t.Fatal(err)
	}

	var result doctorJSONResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("doctor output is not JSON: %v\n%s", err, out)
	}
	if len(result.Fixes) != 1 {
		t.Fatalf("fixes = %+v, want one symlink repair", result.Fixes)
	}
	fix := result.Fixes[0]
	if fix.Action != "repair-symlink" || fix.Target != link || fix.Status != "fixed" {
		t.Fatalf("fix = %+v", fix)
	}
	for _, diag := range result.Diagnostics {
		if strings.Contains(diag.Message, "broken symlink") {
			t.Fatalf("post-fix diagnostics still contain broken symlink: %+v", result.Diagnostics)
		}
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if target != canonical {
		t.Fatalf("repaired target = %q, want %q", target, canonical)
	}
}

func TestDoctorFixSkipsBrokenSymlinkWithoutCanonicalTarget(t *testing.T) {
	root := setupIntegrationRepo(t, config.Config{Symlink: []string{"node_modules"}})
	worktree := t.TempDir()
	link := filepath.Join(worktree, "node_modules")
	originalTarget := filepath.Join(worktree, "missing-target")
	if err := os.Symlink(originalTarget, link); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(root, state.State{Worktrees: map[string]state.WorktreeEntry{
		"auth": {Branch: "feature/auth", Path: worktree, Created: time.Now()},
	}}); err != nil {
		t.Fatal(err)
	}

	doctorJSON = true
	doctorFixMode = true
	t.Cleanup(func() {
		doctorJSON = false
		doctorFixMode = false
	})
	out, runErr := captureStdout(t, func() error { return runDoctor(doctorCmd, nil) })
	if runErr == nil {
		t.Fatal("unrepairable broken symlink should keep doctor exit non-zero")
	}

	var result doctorJSONResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("doctor output is not JSON: %v\n%s", err, out)
	}
	if len(result.Fixes) != 1 || result.Fixes[0].Action != "repair-symlink" || result.Fixes[0].Status != "skipped" {
		t.Fatalf("fixes = %+v", result.Fixes)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if target != originalTarget {
		t.Fatalf("unrepairable link changed to %q", target)
	}
}

func TestDiagStatePathsAllExist(t *testing.T) {
	dir := t.TempDir()
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Branch: "feature/a", Path: dir, Port: 3001, Created: time.Now()},
	}}
	diags := diagStatePaths(s)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok diag, got %v", diags)
	}
}

func TestDiagStatePathsMissing(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Branch: "x", Path: "/nonexistent/path/definitely/missing", Port: 3001, Created: time.Now()},
	}}
	diags := diagStatePaths(s)
	hasError := false
	for _, d := range diags {
		if d.Level == levelError {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected error diag for missing path, got %v", diags)
	}
}

func TestDiagStatePathsEmpty(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{}}
	diags := diagStatePaths(s)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok diag for empty state, got %v", diags)
	}
}

func TestDiagOrphansNone(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Branch: "a", Path: "/tmp/a"},
	}}
	wts := []git.Worktree{
		{Path: "/tmp/main", IsMain: true, Branch: "main"},
		{Path: "/tmp/a", Branch: "a"},
	}
	diags := diagOrphans(s, wts)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok, got %v", diags)
	}
}

func TestDiagOrphansDetected(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{}}
	wts := []git.Worktree{
		{Path: "/tmp/main", IsMain: true, Branch: "main"},
		{Path: "/tmp/orphan", Branch: "feature/orphan"},
	}
	diags := diagOrphans(s, wts)
	hasWarn := false
	for _, d := range diags {
		if d.Level == levelWarn {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected warn for orphan, got %v", diags)
	}
}

func TestDiagPortCollisions(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: "/tmp/a", Port: 3001},
		"b": {Path: "/tmp/b", Port: 3001},
	}}
	diags := diagPortCollisions(s)
	hasError := false
	for _, d := range diags {
		if d.Level == levelError {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected error for port collision, got %v", diags)
	}
}

func TestDiagPortCollisionsNone(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: "/tmp/a", Port: 3001},
		"b": {Path: "/tmp/b", Port: 3002},
	}}
	diags := diagPortCollisions(s)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok, got %v", diags)
	}
}

func TestDiagConfigRangeInvalid(t *testing.T) {
	cfg := config.Config{PortRange: &config.PortRange{Min: 5000, Max: 4000}}
	diags := diagConfigRange(cfg)
	if len(diags) != 1 || diags[0].Level != levelError {
		t.Errorf("expected error for invalid range, got %v", diags)
	}
}

func TestDiagConfigRangeValid(t *testing.T) {
	cfg := config.Config{PortRange: &config.PortRange{Min: 3001, Max: 3999}}
	diags := diagConfigRange(cfg)
	if len(diags) != 0 {
		t.Errorf("expected no diags for valid range, got %v", diags)
	}
}

func TestDiagConfigDriftDetectsChangedConfig(t *testing.T) {
	oldCfg := config.Config{Symlink: []string{"node_modules"}}
	oldHash, err := config.SetupHash(oldCfg)
	if err != nil {
		t.Fatal(err)
	}
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: "/tmp/a", Branch: "feature/a", ConfigHash: oldHash},
	}}
	newCfg := config.Config{Symlink: []string{"node_modules", ".husky/_"}}

	diags := diagConfigDrift(newCfg, s)
	if len(diags) != 1 || diags[0].Level != levelWarn || !containsAll(diags[0].Message, "config drift", "a") {
		t.Fatalf("expected config drift warning, got %+v", diags)
	}
}

func TestDiagConfigDriftWarnsWhenHashMissing(t *testing.T) {
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: "/tmp/a", Branch: "feature/a"},
	}}
	diags := diagConfigDrift(config.Config{}, s)
	if len(diags) != 1 || diags[0].Level != levelWarn || !containsAll(diags[0].Message, "unknown config version", "a") {
		t.Fatalf("expected missing config hash warning, got %+v", diags)
	}
}

func TestDiagConfigDriftOKWhenHashesMatch(t *testing.T) {
	cfg := config.Config{Symlink: []string{"node_modules"}}
	hash, err := config.SetupHash(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: "/tmp/a", Branch: "feature/a", ConfigHash: hash},
		"b": {Path: "/tmp/b", Branch: "feature/b", ConfigHash: hash},
	}}
	diags := diagConfigDrift(cfg, s)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Fatalf("expected config drift ok, got %+v", diags)
	}
}

func TestDiagSymlinksBroken(t *testing.T) {
	wtDir := t.TempDir()
	link := filepath.Join(wtDir, "node_modules")
	if err := os.Symlink("/nonexistent/target/missing", link); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Symlink: []string{"node_modules"}}
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: wtDir, Branch: "a"},
	}}
	diags := diagSymlinks(wtDir, cfg, s)
	hasError := false
	for _, d := range diags {
		if d.Level == levelError {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected error for broken symlink, got %v", diags)
	}
}

func TestDiagConfigPathsWarnsOnMissingSymlink(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{Symlink: []string{"node_modules", ".husky/_"}}

	diags := diagConfigPaths(root, cfg)
	if len(diags) != 2 {
		t.Errorf("expected 2 warnings for missing targets, got %+v", diags)
	}
	for _, d := range diags {
		if d.Level != levelWarn {
			t.Errorf("expected warn, got %v", d)
		}
	}
}

func TestDiagConfigPathsSilentWhenTargetsExist(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".husky", "_"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Symlink: []string{"node_modules", ".husky/_"}}

	diags := diagConfigPaths(root, cfg)
	if len(diags) != 0 {
		t.Errorf("expected no warnings when targets exist, got %+v", diags)
	}
}

func TestDiagConfigPathsWarnsOnMissingCopyDir(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{CopyDirs: []string{".next/cache"}}

	diags := diagConfigPaths(root, cfg)
	if len(diags) != 1 || diags[0].Level != levelWarn {
		t.Errorf("expected 1 warn for missing copyDir, got %+v", diags)
	}
}

func TestDiagSuggestionsWarnsOnMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{}
	diags := diagSuggestions(root, cfg)

	wantSymlink, wantAfter := false, false
	for _, d := range diags {
		if d.Level != levelWarn {
			t.Errorf("expected warn level, got %v", d)
		}
		if filepath.Base("node_modules") != "node_modules" {
			continue
		}
		if containsAll(d.Message, "symlink", "node_modules") {
			wantSymlink = true
		}
		if containsAll(d.Message, "afterCreate", "pnpm install") {
			wantAfter = true
		}
	}
	if !wantSymlink {
		t.Errorf("expected node_modules symlink suggestion, got %+v", diags)
	}
	if !wantAfter {
		t.Errorf("expected pnpm install afterCreate suggestion, got %+v", diags)
	}
}

func TestDiagSuggestionsSilentWhenSatisfied(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		Symlink:             []string{"node_modules"},
		AfterDetachedCreate: config.HookCommands{"pnpm install"},
	}
	diags := diagSuggestions(root, cfg)
	if len(diags) != 0 {
		t.Errorf("expected no suggestions when config satisfies them (install routed to afterDetachedCreate because node_modules is symlinked), got %+v", diags)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func TestDiagSymlinksHealthy(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "node_modules")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	wtDir := t.TempDir()
	link := filepath.Join(wtDir, "node_modules")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Symlink: []string{"node_modules"}}
	s := state.State{Worktrees: map[string]state.WorktreeEntry{
		"a": {Path: wtDir, Branch: "a"},
	}}
	diags := diagSymlinks(root, cfg, s)
	if len(diags) != 1 || diags[0].Level != levelOK {
		t.Errorf("expected single ok, got %v", diags)
	}
}
