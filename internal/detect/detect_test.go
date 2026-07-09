package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0755); err != nil {
		t.Fatal(err)
	}
}

func hasSuggestion(sugs []Suggestion, kind Kind, value string) bool {
	for _, s := range sugs {
		if s.Kind == kind && s.Value == value {
			return true
		}
	}
	return false
}

func TestAnalyzeEmpty(t *testing.T) {
	dir := t.TempDir()
	if got := Analyze(dir); len(got) != 0 {
		t.Errorf("expected no suggestions for empty repo, got %+v", got)
	}
}

func TestDetectHuskyV9Gitignored(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, dir, ".husky")
	mkdir(t, dir, ".husky/_")
	writeFile(t, dir, ".gitignore", "node_modules\n.husky/_\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindSymlink, ".husky/_") {
		t.Errorf("expected husky symlink suggestion, got %+v", sugs)
	}
}

func TestDetectHuskyByHookSource(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, dir, ".husky")
	mkdir(t, dir, ".husky/_")
	writeFile(t, dir, ".husky/pre-commit", `#!/usr/bin/env sh
. "$(dirname -- "$0")/_/husky.sh"

npm test
`)

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindSymlink, ".husky/_") {
		t.Errorf("expected husky symlink suggestion via hook source, got %+v", sugs)
	}
}

func TestDetectHuskyFallsBackToAfterCreateWhenUnderscoreMissing(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, dir, ".husky")
	writeFile(t, dir, ".gitignore", ".husky/_\n")

	sugs := Analyze(dir)
	if hasSuggestion(sugs, KindSymlink, ".husky/_") {
		t.Errorf("did not expect symlink suggestion when .husky/_ missing, got %+v", sugs)
	}
	if !hasSuggestion(sugs, KindAfterCreate, "npx husky") {
		t.Errorf("expected npx husky afterCreate fallback, got %+v", sugs)
	}
}

func TestDetectHuskyAbsentNoSuggestion(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, dir, ".husky")
	writeFile(t, dir, ".husky/pre-commit", "echo standalone\n")

	sugs := Analyze(dir)
	if hasSuggestion(sugs, KindSymlink, ".husky/_") {
		t.Errorf("did not expect husky symlink suggestion, got %+v", sugs)
	}
}

func TestDetectNodeModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	mkdir(t, dir, "node_modules")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindSymlink, "node_modules") {
		t.Errorf("expected node_modules symlink suggestion, got %+v", sugs)
	}
}

func TestDetectNodeModulesSkippedWhenMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")

	sugs := Analyze(dir)
	if hasSuggestion(sugs, KindSymlink, "node_modules") {
		t.Errorf("did not expect node_modules symlink without existing dir, got %+v", sugs)
	}
}

func TestDetectPackageManagerPnpm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "pnpm-lock.yaml", "")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "pnpm install") {
		t.Errorf("expected pnpm install afterCreate, got %+v", sugs)
	}
}

func TestDetectPackageManagerPriority(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "pnpm-lock.yaml", "")
	writeFile(t, dir, "package-lock.json", "")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "pnpm install") {
		t.Errorf("expected pnpm install (priority over npm), got %+v", sugs)
	}
	if hasSuggestion(sugs, KindAfterCreate, "npm install") {
		t.Errorf("did not expect npm install when pnpm-lock.yaml present, got %+v", sugs)
	}
}

func TestDetectPackageManagerFieldWinsOverLockfile(t *testing.T) {
	dir := t.TempDir()
	// package.json declares yarn but only npm lockfile exists.
	writeFile(t, dir, "package.json", `{"packageManager":"yarn@4.0.0"}`)
	writeFile(t, dir, "package-lock.json", "")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "yarn install") {
		t.Errorf("expected yarn install from packageManager field, got %+v", sugs)
	}
	if hasSuggestion(sugs, KindAfterCreate, "npm install") {
		t.Errorf("did not expect npm install when packageManager=yarn, got %+v", sugs)
	}
}

func TestDetectPackageManagerFieldUnknownFallsBackToLockfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"packageManager":"weirdpm@1"}`)
	writeFile(t, dir, "package-lock.json", "")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "npm install") {
		t.Errorf("expected fallback to npm install for unknown packageManager, got %+v", sugs)
	}
}

func TestReadPackageManagerFieldStripsVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"packageManager":"pnpm@9.1.0+sha512.abc"}`)

	name, ok := readPackageManagerField(filepath.Join(dir, "package.json"))
	if !ok {
		t.Fatalf("expected packageManager field to be parsed")
	}
	if name != "pnpm" {
		t.Errorf("expected name=pnpm, got %q", name)
	}
}

func TestDetectNext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "next.config.mjs", "export default {}\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindCopyDir, ".next/cache") {
		t.Errorf("expected .next/cache copyDir, got %+v", sugs)
	}
}

func TestDetectTurbo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "turbo.json", "{}")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindCopyDir, ".turbo") {
		t.Errorf("expected .turbo copyDir, got %+v", sugs)
	}
}

func TestDetectDockerCompose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "compose.yaml", "services: {}\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "docker compose pull") {
		t.Errorf("expected docker compose pull, got %+v", sugs)
	}
}

func TestDetectViteFallbackInstall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "vite.config.ts", "export default {}\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "npm install") {
		t.Errorf("expected npm install fallback for Vite project, got %+v", sugs)
	}
}

func TestDetectViteSkipsFallbackWhenLockfilePresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "vite.config.ts", "export default {}\n")
	writeFile(t, dir, "pnpm-lock.yaml", "")

	sugs := Analyze(dir)
	if hasSuggestion(sugs, KindAfterCreate, "npm install") {
		t.Errorf("did not expect npm install fallback when package manager detection applies, got %+v", sugs)
	}
	if !hasSuggestion(sugs, KindAfterCreate, "pnpm install") {
		t.Errorf("expected pnpm install from lockfile, got %+v", sugs)
	}
}

func TestDetectRemixFallbackInstall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "remix.config.js", "module.exports = {}\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "npm install") {
		t.Errorf("expected npm install fallback for Remix project, got %+v", sugs)
	}
}

func TestDetectSvelteKit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", "{}")
	writeFile(t, dir, "svelte.config.js", "export default {}\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "npm install") {
		t.Errorf("expected npm install fallback for SvelteKit project, got %+v", sugs)
	}
	if !hasSuggestion(sugs, KindCopyDir, ".svelte-kit") {
		t.Errorf("expected .svelte-kit copyDir, got %+v", sugs)
	}
}

func TestDetectGoModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/app\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "go mod download") {
		t.Errorf("expected go mod download, got %+v", sugs)
	}
}

func TestDetectBundler(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Gemfile.lock", "")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "bundle install") {
		t.Errorf("expected bundle install, got %+v", sugs)
	}
}

func TestDetectComposer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.lock", "")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "composer install") {
		t.Errorf("expected composer install, got %+v", sugs)
	}
}

func TestDetectMakeSetupTarget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "setup:\n\t./script/setup\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "make setup") {
		t.Errorf("expected make setup, got %+v", sugs)
	}
}

func TestDetectMakefileSkipsWithoutSetupTarget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "test:\n\tgo test ./...\n")

	sugs := Analyze(dir)
	if hasSuggestion(sugs, KindAfterCreate, "make setup") {
		t.Errorf("did not expect make setup without explicit setup target, got %+v", sugs)
	}
}

func TestDetectDirenv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".envrc", "export FOO=bar\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "direnv allow") {
		t.Errorf("expected direnv allow afterCreate, got %+v", sugs)
	}
}

func TestDetectMiseToolVersions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".tool-versions", "nodejs 20.0.0\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "mise install") {
		t.Errorf("expected mise install afterCreate, got %+v", sugs)
	}
}

func TestDetectMiseConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".mise.toml", "[tools]\nnode = \"20\"\n")

	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "mise install") {
		t.Errorf("expected mise install afterCreate via .mise.toml, got %+v", sugs)
	}
}

func TestDetectPythonUv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "uv.lock", "")
	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "uv sync") {
		t.Errorf("expected uv sync, got %+v", sugs)
	}
}

func TestDetectPythonPriority(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "uv.lock", "")
	writeFile(t, dir, "poetry.lock", "")
	writeFile(t, dir, "requirements.txt", "")
	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "uv sync") {
		t.Errorf("expected uv sync (priority), got %+v", sugs)
	}
	if hasSuggestion(sugs, KindAfterCreate, "poetry install") {
		t.Errorf("did not expect poetry install when uv.lock present, got %+v", sugs)
	}
}

func TestDetectPythonRequirementsFallback(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "requirements.txt", "")
	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "pip install -r requirements.txt") {
		t.Errorf("expected pip install fallback, got %+v", sugs)
	}
}

func TestDetectCargo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\nname=\"x\"\n")
	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "cargo fetch") {
		t.Errorf("expected cargo fetch, got %+v", sugs)
	}
}

func TestDetectGradle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "build.gradle.kts", "")
	writeFile(t, dir, "gradlew", "#!/bin/sh\n")
	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "./gradlew --no-daemon dependencies") {
		t.Errorf("expected gradlew dependencies, got %+v", sugs)
	}
}

func TestDetectGradleSkippedWithoutWrapper(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "build.gradle.kts", "")
	sugs := Analyze(dir)
	if hasSuggestion(sugs, KindAfterCreate, "./gradlew --no-daemon dependencies") {
		t.Errorf("did not expect gradle suggestion without gradlew, got %+v", sugs)
	}
}

func TestAdaptReroutesInstallWhenNodeModulesSymlinked(t *testing.T) {
	in := []Suggestion{
		{Kind: KindAfterCreate, Value: "yarn install", Reason: "lockfile", LinkedSymlink: "node_modules"},
		{Kind: KindAfterCreate, Value: "direnv allow", Reason: "envrc"},
		{Kind: KindSymlink, Value: ".husky/_", Reason: "husky"},
	}

	got := Adapt([]string{"node_modules", ".husky/_"}, in)

	if got[0].Kind != KindAfterDetachedCreate {
		t.Errorf("expected install rerouted to afterDetachedCreate, got %s", got[0].Kind)
	}
	if got[0].Value != "yarn install" {
		t.Errorf("expected value preserved, got %q", got[0].Value)
	}
	if got[1].Kind != KindAfterCreate {
		t.Errorf("direnv (no LinkedSymlink) must stay afterCreate, got %s", got[1].Kind)
	}
	if got[2].Kind != KindSymlink {
		t.Errorf("symlink suggestion must stay symlink, got %s", got[2].Kind)
	}
}

func TestAdaptKeepsInstallAfterCreateWhenSymlinkAbsent(t *testing.T) {
	in := []Suggestion{
		{Kind: KindAfterCreate, Value: "yarn install", LinkedSymlink: "node_modules"},
	}
	got := Adapt(nil, in)
	if got[0].Kind != KindAfterCreate {
		t.Errorf("expected afterCreate when no symlink, got %s", got[0].Kind)
	}
}

func TestAdaptDoesNotDoubleRouteAlreadyDetached(t *testing.T) {
	in := []Suggestion{
		{Kind: KindAfterDetachedCreate, Value: "yarn install", LinkedSymlink: "node_modules"},
	}
	got := Adapt([]string{"node_modules"}, in)
	if got[0].Kind != KindAfterDetachedCreate {
		t.Errorf("expected afterDetachedCreate preserved, got %s", got[0].Kind)
	}
}

func TestAdaptReasonAnnotated(t *testing.T) {
	in := []Suggestion{
		{Kind: KindAfterCreate, Value: "yarn install", Reason: "lockfile present", LinkedSymlink: "node_modules"},
	}
	got := Adapt([]string{"node_modules"}, in)
	if !strings.Contains(got[0].Reason, "afterDetachedCreate") || !strings.Contains(got[0].Reason, "--detach") {
		t.Errorf("expected reason annotation about routing, got %q", got[0].Reason)
	}
}

func TestGitignoreHasIgnoresCommentsAndPrefixes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".gitignore", "# comment\n/.husky/_/\nnode_modules\n")

	if !gitignoreHas(dir, ".husky/_") {
		t.Errorf("expected gitignoreHas to match .husky/_ with leading / and trailing /")
	}
	if !gitignoreHas(dir, "node_modules") {
		t.Errorf("expected gitignoreHas to match node_modules")
	}
	if gitignoreHas(dir, "dist") {
		t.Errorf("did not expect gitignoreHas to match dist")
	}
}
