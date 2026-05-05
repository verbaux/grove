package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/detect"
)

func TestPendingSuggestionsFiltersCovered(t *testing.T) {
	all := []detect.Suggestion{
		{Kind: detect.KindSymlink, Value: "node_modules", Reason: "x"},
		{Kind: detect.KindSymlink, Value: ".husky/_", Reason: "y"},
		{Kind: detect.KindAfterCreate, Value: "pnpm install", Reason: "z"},
	}
	cfg := config.Config{
		Symlink:     []string{"node_modules"},
		AfterCreate: config.AfterCreate{"pnpm install --frozen-lockfile"},
	}
	got := pendingSuggestions(cfg, all)
	if len(got) != 1 {
		t.Fatalf("expected 1 pending suggestion (.husky/_), got %d: %+v", len(got), got)
	}
	if got[0].Value != ".husky/_" {
		t.Errorf("expected .husky/_ pending, got %+v", got[0])
	}
}

func TestPendingSuggestionsAllPendingWhenConfigEmpty(t *testing.T) {
	all := []detect.Suggestion{
		{Kind: detect.KindCopyDir, Value: ".turbo"},
		{Kind: detect.KindSymlink, Value: "node_modules"},
	}
	got := pendingSuggestions(config.Config{}, all)
	if len(got) != 2 {
		t.Errorf("expected all 2 pending for empty config, got %d", len(got))
	}
}

func TestPendingSuggestionsNoneWhenAllCovered(t *testing.T) {
	all := []detect.Suggestion{
		{Kind: detect.KindCopyDir, Value: ".next/cache"},
	}
	cfg := config.Config{CopyDirs: []string{".next/cache"}}
	got := pendingSuggestions(cfg, all)
	if len(got) != 0 {
		t.Errorf("expected no pending when covered, got %+v", got)
	}
}

func TestMergeSuggestionsAppendsByKind(t *testing.T) {
	cfg := config.Config{
		Symlink:     []string{"node_modules"},
		AfterCreate: config.AfterCreate{"npm install"},
	}
	pending := []detect.Suggestion{
		{Kind: detect.KindSymlink, Value: ".husky/_"},
		{Kind: detect.KindCopyDir, Value: ".turbo"},
		{Kind: detect.KindAfterCreate, Value: "direnv allow"},
	}

	got := mergeSuggestions(cfg, pending)

	want := config.Config{
		Symlink:     []string{"node_modules", ".husky/_"},
		CopyDirs:    []string{".turbo"},
		AfterCreate: config.AfterCreate{"npm install", "direnv allow"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merge mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestPendingSuggestionsGroupedByKind(t *testing.T) {
	all := []detect.Suggestion{
		{Kind: detect.KindAfterCreate, Value: "pnpm install"},
		{Kind: detect.KindSymlink, Value: "node_modules"},
		{Kind: detect.KindCopyDir, Value: ".turbo"},
		{Kind: detect.KindAfterCreate, Value: "direnv allow"},
		{Kind: detect.KindSymlink, Value: ".husky/_"},
	}
	got := pendingSuggestions(config.Config{}, all)

	wantKinds := []detect.Kind{
		detect.KindSymlink, detect.KindSymlink,
		detect.KindCopyDir,
		detect.KindAfterCreate, detect.KindAfterCreate,
	}
	if len(got) != len(wantKinds) {
		t.Fatalf("expected %d, got %d: %+v", len(wantKinds), len(got), got)
	}
	for i, k := range wantKinds {
		if got[i].Kind != k {
			t.Errorf("position %d: expected kind %s, got %s (%+v)", i, k, got[i].Kind, got[i])
		}
	}
}

func TestMergeSuggestionsLeavesCfgUntouched(t *testing.T) {
	cfg := config.Config{Symlink: []string{"node_modules"}}
	pending := []detect.Suggestion{
		{Kind: detect.KindSymlink, Value: ".husky/_"},
	}
	_ = mergeSuggestions(cfg, pending)
	if !reflect.DeepEqual(cfg.Symlink, []string{"node_modules"}) {
		t.Errorf("mergeSuggestions mutated input cfg.Symlink: %+v", cfg.Symlink)
	}
}

func TestFindStalePathsDetectsMissingTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Symlink:  []string{"node_modules", "missing-link"},
		CopyDirs: []string{"missing-cache"},
	}

	got := findStalePaths(root, cfg)

	want := []StalePath{
		{Kind: "symlink", Value: "missing-link"},
		{Kind: "copyDir", Value: "missing-cache"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stale mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestFindStalePathsEmptyWhenAllExist(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Symlink: []string{"node_modules"}}

	got := findStalePaths(root, cfg)
	if len(got) != 0 {
		t.Errorf("expected no stale paths, got %+v", got)
	}
}

func TestRemoveStalePathsFiltersByKind(t *testing.T) {
	cfg := config.Config{
		Symlink:  []string{"node_modules", "missing-link", ".husky/_"},
		CopyDirs: []string{".turbo", "missing-cache"},
	}
	stale := []StalePath{
		{Kind: "symlink", Value: "missing-link"},
		{Kind: "copyDir", Value: "missing-cache"},
	}

	got := removeStalePaths(cfg, stale)

	want := config.Config{
		Symlink:  []string{"node_modules", ".husky/_"},
		CopyDirs: []string{".turbo"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("remove mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func TestRemoveStalePathsNoopWhenEmpty(t *testing.T) {
	cfg := config.Config{Symlink: []string{"node_modules"}}
	got := removeStalePaths(cfg, nil)
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("expected unchanged cfg, got %+v", got)
	}
}
