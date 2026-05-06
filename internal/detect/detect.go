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
	detectTurbo,
	detectDirenv,
	detectMise,
	detectPython,
	detectCargo,
	detectGradle,
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
