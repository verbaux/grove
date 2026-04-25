package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillInstallIntoCustomDir(t *testing.T) {
	tmp := t.TempDir()
	resetSkillFlags(t)
	SkillContent = "fake skill content"
	t.Cleanup(func() { SkillContent = "" })

	skillDirFlag = tmp
	skillForceFlag = true

	if err := runSkillInstall(nil, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmp, "grove", "SKILL.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "fake skill content" {
		t.Fatalf("content mismatch: %q", string(got))
	}
}

func TestSkillInstallOverwriteWithForce(t *testing.T) {
	tmp := t.TempDir()
	resetSkillFlags(t)
	SkillContent = "new"
	t.Cleanup(func() { SkillContent = "" })

	// pre-existing file
	groveDir := filepath.Join(tmp, "grove")
	if err := os.MkdirAll(groveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(groveDir, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	skillDirFlag = tmp
	skillForceFlag = true

	if err := runSkillInstall(nil, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(groveDir, "SKILL.md"))
	if string(got) != "new" {
		t.Fatalf("expected overwrite, got %q", string(got))
	}
}

func TestSkillInstallRequiresEmbeddedContent(t *testing.T) {
	tmp := t.TempDir()
	resetSkillFlags(t)
	SkillContent = ""

	skillDirFlag = tmp
	skillForceFlag = true

	err := runSkillInstall(nil, nil)
	if err == nil {
		t.Fatal("expected error for empty SkillContent")
	}
}

func TestSkillInstallAutodetectWithHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	resetSkillFlags(t)
	SkillContent = "c"
	t.Cleanup(func() { SkillContent = "" })

	// create .claude parent hint but not skills dir yet
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	skillForceFlag = true

	if err := runSkillInstall(nil, nil); err != nil {
		t.Fatalf("install: %v", err)
	}

	target := filepath.Join(home, ".claude", "skills", "grove", "SKILL.md")
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected skill at %s: %v", target, err)
	}

	codexTarget := filepath.Join(home, ".agents", "skills", "grove", "SKILL.md")
	if _, err := os.Stat(codexTarget); !os.IsNotExist(err) {
		t.Fatalf("codex skill should not be installed (no ~/.agents): %v", err)
	}
}

func TestSkillUninstallRemovesDir(t *testing.T) {
	tmp := t.TempDir()
	resetSkillFlags(t)

	groveDir := filepath.Join(tmp, "grove")
	if err := os.MkdirAll(groveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(groveDir, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	skillDirFlag = tmp
	skillForceFlag = true

	if err := runSkillUninstall(nil, nil); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(groveDir); !os.IsNotExist(err) {
		t.Fatalf("expected %s removed, got err=%v", groveDir, err)
	}
}

func TestSkillInstallUnknownTarget(t *testing.T) {
	resetSkillFlags(t)
	skillTargetFlag = "bogus"

	_, err := resolveSkillDirs(true)
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func resetSkillFlags(t *testing.T) {
	t.Helper()
	prev := struct {
		target, dir string
		force       bool
	}{skillTargetFlag, skillDirFlag, skillForceFlag}
	skillTargetFlag = ""
	skillDirFlag = ""
	skillForceFlag = false
	t.Cleanup(func() {
		skillTargetFlag = prev.target
		skillDirFlag = prev.dir
		skillForceFlag = prev.force
	})
}
