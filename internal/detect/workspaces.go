package detect

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// IsJSWorkspace reports whether root declares JS workspace packages in
// pnpm-workspace.yaml, package.json "workspaces", or lerna.json.
//
// Every package manager links workspace packages into node_modules with
// relative symlinks: pnpm in each package's node_modules, npm, yarn, bun and
// hoisted pnpm in the root one. A node_modules symlinked from the main
// checkout therefore resolves workspace packages to the main checkout's
// sources, so a workspace needs its own install per worktree.
func IsJSWorkspace(root string) bool {
	for _, p := range workspacePatterns(root) {
		p = strings.TrimSpace(p)
		if p != "" && !strings.HasPrefix(p, "!") {
			return true
		}
	}
	return false
}

// workspacePatterns collects workspace package globs from every supported
// manifest. Lerna's implicit "packages/*" default is not assumed.
func workspacePatterns(root string) []string {
	var out []string
	out = append(out, readPnpmWorkspacePackages(filepath.Join(root, "pnpm-workspace.yaml"))...)
	out = append(out, readPackageJSONWorkspaces(filepath.Join(root, "package.json"))...)
	out = append(out, readLernaPackages(filepath.Join(root, "lerna.json"))...)
	return out
}

// readPnpmWorkspacePackages reads the top-level "packages" list from
// pnpm-workspace.yaml. It handles block lists, indented or not, and
// single-line flow lists, without a full YAML parser.
func readPnpmWorkspacePackages(p string) []string {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []string
	inPackages := false
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := scan.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// YAML lets a block list under a top-level key start in column 0.
		isItem := strings.HasPrefix(trimmed, "-")
		topLevel := line[0] != ' ' && line[0] != '\t' && !isItem
		if topLevel {
			rest, ok := strings.CutPrefix(trimmed, "packages:")
			inPackages = ok
			if ok {
				out = append(out, parseYAMLFlowList(rest)...)
			}
			continue
		}
		if !inPackages {
			continue
		}
		if item, ok := strings.CutPrefix(trimmed, "-"); ok {
			if v := yamlScalar(item); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// parseYAMLFlowList parses `[a, 'b', "c"]`. Anything else yields nil.
func parseYAMLFlowList(s string) []string {
	s = stripYAMLComment(strings.TrimSpace(s))
	inner, ok := strings.CutPrefix(s, "[")
	if !ok {
		return nil
	}
	inner, ok = strings.CutSuffix(inner, "]")
	if !ok {
		return nil
	}
	var out []string
	for _, item := range strings.Split(inner, ",") {
		if v := yamlScalar(item); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// yamlScalar unquotes a plain, single-quoted, or double-quoted scalar and
// drops a trailing comment outside the quotes.
func yamlScalar(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if q := s[0]; q == '\'' || q == '"' {
		if end := strings.IndexByte(s[1:], q); end >= 0 {
			return s[1 : end+1]
		}
		return ""
	}
	return strings.TrimSpace(stripYAMLComment(s))
}

func stripYAMLComment(s string) string {
	if i := strings.Index(s, " #"); i >= 0 {
		return s[:i]
	}
	return s
}

// readPackageJSONWorkspaces reads "workspaces" as either an array (npm, yarn,
// bun) or an object with a "packages" array (yarn classic).
func readPackageJSONWorkspaces(p string) []string {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var pkg struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || len(pkg.Workspaces) == 0 {
		return nil
	}
	var list []string
	if err := json.Unmarshal(pkg.Workspaces, &list); err == nil {
		return list
	}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(pkg.Workspaces, &obj); err == nil {
		return obj.Packages
	}
	return nil
}

func readLernaPackages(p string) []string {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var lerna struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(data, &lerna); err != nil {
		return nil
	}
	return lerna.Packages
}
