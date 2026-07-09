// Package detect inspects a repository and proposes .groverc.json
// additions (symlink targets, copy dirs, afterCreate hooks).
//
// It only reads the filesystem. It never mutates config — callers
// (grove init, grove doctor) decide how to surface suggestions.
package detect

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Kind classifies a suggestion against a Config field.
type Kind string

const (
	KindSymlink             Kind = "symlink"
	KindCopyDir             Kind = "copyDir"
	KindAfterCreate         Kind = "afterCreate"
	KindAfterDetachedCreate Kind = "afterDetachedCreate"
)

// Suggestion is a single recommended config addition.
//
// LinkedSymlink names a symlink target whose presence in cfg.Symlink
// signals that this suggestion would conflict with shared state when
// run via afterCreate. Adapt() reroutes such suggestions to
// afterDetachedCreate so they only execute when the user explicitly
// asks for an independent worktree via `grove create --detach`.
//
// Empty LinkedSymlink means the suggestion is unaffected by the
// symlink set and stays in its original Kind.
type Suggestion struct {
	Kind          Kind   `json:"kind"`
	Value         string `json:"value"`
	Reason        string `json:"reason"`
	LinkedSymlink string `json:"linkedSymlink,omitempty"`
}

type rule func(root string) []Suggestion

var rules = []rule{
	detectHusky,
	detectNodeModules,
	detectPackageManager,
	detectNext,
	detectDockerCompose,
	detectVite,
	detectRemix,
	detectSvelteKit,
	detectTurbo,
	detectDirenv,
	detectMise,
	detectPython,
	detectGoModules,
	detectCargo,
	detectGradle,
	detectBundler,
	detectComposer,
	detectMakeSetup,
}

// Analyze walks the configured rule set and returns every suggestion that fires.
func Analyze(root string) []Suggestion {
	var out []Suggestion
	for _, r := range rules {
		out = append(out, r(root)...)
	}
	return out
}

// Adapt rewrites suggestions whose LinkedSymlink target is shared via
// cfg.Symlink: such installs would mutate the main worktree's state
// when run as afterCreate, so they are routed to afterDetachedCreate
// (which only runs with `grove create --detach`).
//
// Suggestions without LinkedSymlink, or whose target is not in the
// symlink set, are returned unchanged.
func Adapt(symlinks []string, sugs []Suggestion) []Suggestion {
	if len(sugs) == 0 {
		return nil
	}
	linked := make(map[string]bool, len(symlinks))
	for _, s := range symlinks {
		linked[s] = true
	}
	out := make([]Suggestion, len(sugs))
	for i, s := range sugs {
		if s.LinkedSymlink == "" || !linked[s.LinkedSymlink] || s.Kind != KindAfterCreate {
			out[i] = s
			continue
		}
		out[i] = Suggestion{
			Kind:          KindAfterDetachedCreate,
			Value:         s.Value,
			LinkedSymlink: s.LinkedSymlink,
			Reason: s.Reason +
				" (routed to afterDetachedCreate because " + s.LinkedSymlink +
				" is symlinked; runs only with `grove create --detach`)",
		}
	}
	return out
}

func detectHusky(root string) []Suggestion {
	if !dirExists(filepath.Join(root, ".husky")) {
		return nil
	}
	if !gitignoreHas(root, ".husky/_") && !huskyHooksReferenceUnderscore(root) {
		return nil
	}
	if dirExists(filepath.Join(root, ".husky", "_")) {
		return []Suggestion{{
			Kind:   KindSymlink,
			Value:  ".husky/_",
			Reason: "husky v9 layout detected and .husky/_ exists in main repo; symlink it so git hooks run in worktrees",
		}}
	}
	return []Suggestion{{
		Kind:   KindAfterCreate,
		Value:  "npx husky",
		Reason: "husky v9 layout detected but .husky/_ runtime is missing in main repo; regenerate it inside the worktree",
	}}
}

func detectNodeModules(root string) []Suggestion {
	if !fileExists(filepath.Join(root, "package.json")) {
		return nil
	}
	if !dirExists(filepath.Join(root, "node_modules")) {
		return nil
	}
	return []Suggestion{{
		Kind:   KindSymlink,
		Value:  "node_modules",
		Reason: "package.json + existing node_modules; symlink to skip reinstall per worktree",
	}}
}

func detectPackageManager(root string) []Suggestion {
	pkgPath := filepath.Join(root, "package.json")
	if name, ok := readPackageManagerField(pkgPath); ok {
		if cmd, ok := packageManagerInstall(name); ok {
			return []Suggestion{{
				Kind:          KindAfterCreate,
				Value:         cmd,
				Reason:        "package.json packageManager=" + name + " (declared field overrides lockfile detection)",
				LinkedSymlink: "node_modules",
			}}
		}
	}

	candidates := []struct {
		lock string
		cmd  string
	}{
		{"pnpm-lock.yaml", "pnpm install"},
		{"yarn.lock", "yarn install"},
		{"bun.lockb", "bun install"},
		{"bun.lock", "bun install"},
		{"package-lock.json", "npm install"},
	}
	for _, c := range candidates {
		if fileExists(filepath.Join(root, c.lock)) {
			return []Suggestion{{
				Kind:          KindAfterCreate,
				Value:         c.cmd,
				Reason:        c.lock + " present; install dependencies after worktree create",
				LinkedSymlink: "node_modules",
			}}
		}
	}
	return nil
}

// readPackageManagerField extracts the bare manager name from
// package.json's "packageManager" field, e.g. "pnpm@9.1.0" -> "pnpm".
func readPackageManagerField(pkgJSONPath string) (string, bool) {
	data, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		return "", false
	}
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", false
	}
	if pkg.PackageManager == "" {
		return "", false
	}
	name := pkg.PackageManager
	if i := strings.Index(name, "@"); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(name), true
}

func packageManagerInstall(name string) (string, bool) {
	switch name {
	case "pnpm":
		return "pnpm install", true
	case "yarn":
		return "yarn install", true
	case "bun":
		return "bun install", true
	case "npm":
		return "npm install", true
	}
	return "", false
}

func detectNext(root string) []Suggestion {
	for _, name := range []string{"next.config.js", "next.config.mjs", "next.config.ts", "next.config.cjs"} {
		if fileExists(filepath.Join(root, name)) {
			return []Suggestion{{
				Kind:   KindCopyDir,
				Value:  ".next/cache",
				Reason: "Next.js config detected; copy .next/cache for faster first build in worktree",
			}}
		}
	}
	return nil
}

func detectTurbo(root string) []Suggestion {
	if !fileExists(filepath.Join(root, "turbo.json")) {
		return nil
	}
	return []Suggestion{{
		Kind:   KindCopyDir,
		Value:  ".turbo",
		Reason: "turbo.json detected; copy .turbo cache for faster first task in worktree",
	}}
}

func detectDockerCompose(root string) []Suggestion {
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		if fileExists(filepath.Join(root, name)) {
			return []Suggestion{{
				Kind:   KindAfterCreate,
				Value:  "docker compose pull",
				Reason: name + " detected; pull declared images for the worktree without starting containers",
			}}
		}
	}
	return nil
}

func detectVite(root string) []Suggestion {
	if !anyFileExists(root, "vite.config.js", "vite.config.mjs", "vite.config.ts", "vite.config.cjs", "vite.config.mts", "vite.config.cts") {
		return nil
	}
	return nodeFrameworkInstallFallback(root, "Vite config detected")
}

func detectRemix(root string) []Suggestion {
	if !anyFileExists(root, "remix.config.js", "remix.config.mjs", "remix.config.ts", "remix.config.cjs") {
		return nil
	}
	return nodeFrameworkInstallFallback(root, "Remix config detected")
}

func detectSvelteKit(root string) []Suggestion {
	if !anyFileExists(root, "svelte.config.js", "svelte.config.mjs", "svelte.config.ts") {
		return nil
	}
	out := nodeFrameworkInstallFallback(root, "SvelteKit config detected")
	out = append(out, Suggestion{
		Kind:   KindCopyDir,
		Value:  ".svelte-kit",
		Reason: "SvelteKit config detected; copy .svelte-kit cache for faster first build in worktree",
	})
	return out
}

func nodeFrameworkInstallFallback(root, reason string) []Suggestion {
	if !fileExists(filepath.Join(root, "package.json")) || hasPackageManagerSignal(root) {
		return nil
	}
	return []Suggestion{{
		Kind:          KindAfterCreate,
		Value:         "npm install",
		Reason:        reason + " with package.json but no packageManager field or lockfile; install Node dependencies after worktree create",
		LinkedSymlink: "node_modules",
	}}
}

func hasPackageManagerSignal(root string) bool {
	if _, ok := readPackageManagerField(filepath.Join(root, "package.json")); ok {
		return true
	}
	return anyFileExists(root, "pnpm-lock.yaml", "yarn.lock", "bun.lockb", "bun.lock", "package-lock.json")
}

func detectDirenv(root string) []Suggestion {
	if !fileExists(filepath.Join(root, ".envrc")) {
		return nil
	}
	return []Suggestion{{
		Kind:   KindAfterCreate,
		Value:  "direnv allow",
		Reason: ".envrc detected; direnv requires explicit allow per directory, including new worktrees",
	}}
}

func detectPython(root string) []Suggestion {
	candidates := []struct {
		marker string
		cmd    string
	}{
		{"uv.lock", "uv sync"},
		{"poetry.lock", "poetry install"},
		{"Pipfile.lock", "pipenv install"},
		{"requirements.txt", "pip install -r requirements.txt"},
	}
	for _, c := range candidates {
		if fileExists(filepath.Join(root, c.marker)) {
			return []Suggestion{{
				Kind:          KindAfterCreate,
				Value:         c.cmd,
				Reason:        c.marker + " detected; install Python dependencies in worktree",
				LinkedSymlink: ".venv",
			}}
		}
	}
	return nil
}

func detectGoModules(root string) []Suggestion {
	if !fileExists(filepath.Join(root, "go.mod")) {
		return nil
	}
	return []Suggestion{{
		Kind:   KindAfterCreate,
		Value:  "go mod download",
		Reason: "go.mod detected; predownload Go modules into the module cache",
	}}
}

func detectCargo(root string) []Suggestion {
	if !fileExists(filepath.Join(root, "Cargo.toml")) {
		return nil
	}
	return []Suggestion{{
		Kind:          KindAfterCreate,
		Value:         "cargo fetch",
		Reason:        "Cargo.toml detected; predownload dependencies into the global cargo cache",
		LinkedSymlink: "target",
	}}
}

func detectGradle(root string) []Suggestion {
	hasBuildScript := fileExists(filepath.Join(root, "build.gradle")) ||
		fileExists(filepath.Join(root, "build.gradle.kts")) ||
		fileExists(filepath.Join(root, "settings.gradle")) ||
		fileExists(filepath.Join(root, "settings.gradle.kts"))
	if !hasBuildScript {
		return nil
	}
	if !fileExists(filepath.Join(root, "gradlew")) {
		return nil
	}
	return []Suggestion{{
		Kind:          KindAfterCreate,
		Value:         "./gradlew --no-daemon dependencies",
		Reason:        "Gradle wrapper + build script detected; prime the dependency cache for the worktree",
		LinkedSymlink: ".gradle",
	}}
}

func detectBundler(root string) []Suggestion {
	if !anyFileExists(root, "Gemfile.lock", "Gemfile") {
		return nil
	}
	return []Suggestion{{
		Kind:          KindAfterCreate,
		Value:         "bundle install",
		Reason:        "Gemfile detected; install Ruby gems after worktree create",
		LinkedSymlink: "vendor/bundle",
	}}
}

func detectComposer(root string) []Suggestion {
	if !anyFileExists(root, "composer.lock", "composer.json") {
		return nil
	}
	return []Suggestion{{
		Kind:          KindAfterCreate,
		Value:         "composer install",
		Reason:        "composer.json detected; install PHP dependencies after worktree create",
		LinkedSymlink: "vendor",
	}}
}

func detectMakeSetup(root string) []Suggestion {
	if !makefileHasTarget(root, "setup") {
		return nil
	}
	return []Suggestion{{
		Kind:   KindAfterCreate,
		Value:  "make setup",
		Reason: "Makefile has an explicit setup target; run it after worktree create",
	}}
}

func detectMise(root string) []Suggestion {
	for _, name := range []string{".mise.toml", "mise.toml", ".tool-versions"} {
		if fileExists(filepath.Join(root, name)) {
			return []Suggestion{{
				Kind:   KindAfterCreate,
				Value:  "mise install",
				Reason: name + " detected; install pinned tool versions in worktree",
			}}
		}
	}
	return nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func anyFileExists(root string, names ...string) bool {
	for _, name := range names {
		if fileExists(filepath.Join(root, name)) {
			return true
		}
	}
	return false
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// gitignoreHas reports whether root/.gitignore contains a non-comment line
// equal to needle (after trimming whitespace and a leading "/").
func gitignoreHas(root, needle string) bool {
	f, err := os.Open(filepath.Join(root, ".gitignore"))
	if err != nil {
		return false
	}
	defer f.Close()

	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "/")
		line = strings.TrimSuffix(line, "/")
		if line == needle || line == strings.TrimSuffix(needle, "/") {
			return true
		}
	}
	return false
}

// huskyHooksReferenceUnderscore reports whether any committed file under
// .husky/ (excluding the _ dir itself) sources husky.sh from the _ runtime dir.
func huskyHooksReferenceUnderscore(root string) bool {
	huskyDir := filepath.Join(root, ".husky")
	entries, err := os.ReadDir(huskyDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(huskyDir, e.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "husky.sh") {
			return true
		}
	}
	return false
}

func makefileHasTarget(root, target string) bool {
	var path string
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		p := filepath.Join(root, name)
		if fileExists(p) {
			path = p
			break
		}
	}
	if path == "" {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	prefix := target + ":"
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ".") {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
