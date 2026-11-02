package detect

import "testing"

func TestIsJSWorkspace(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"pnpm block list", map[string]string{
			"pnpm-workspace.yaml": "# ws\npackages:\n  - 'apps/*'\n  - \"libs/core\" # core\ncatalog:\n  react: ^19\n",
		}, true},
		{"pnpm unindented block list", map[string]string{
			"pnpm-workspace.yaml": "packages:\n- backend\n- frontend/apps/*\n",
		}, true},
		{"pnpm flow list", map[string]string{
			"pnpm-workspace.yaml": "packages: ['packages/*', \"./libs/core/\"]\n",
		}, true},
		{"pnpm settings only", map[string]string{
			"pnpm-workspace.yaml": "nodeLinker: hoisted\nonlyBuiltDependencies:\n  - esbuild\n",
		}, false},
		{"pnpm only negations", map[string]string{
			"pnpm-workspace.yaml": "packages:\n  - '!legacy'\n",
		}, false},
		{"package.json array", map[string]string{
			"package.json": `{"workspaces":["packages/*"]}`,
		}, true},
		{"package.json object", map[string]string{
			"package.json": `{"workspaces":{"packages":["apps/*"],"nohoist":["**/x"]}}`,
		}, true},
		{"package.json invalid workspaces", map[string]string{
			"package.json": `{"workspaces": 42}`,
		}, false},
		{"package.json without workspaces", map[string]string{
			"package.json": `{"name":"single"}`,
		}, false},
		{"lerna", map[string]string{
			"lerna.json": `{"packages":["modules/*"]}`,
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tc.files {
				writeFile(t, dir, name, body)
			}
			if got := IsJSWorkspace(dir); got != tc.want {
				t.Errorf("IsJSWorkspace = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDetectNodeModulesSkippedInWorkspace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"root","packageManager":"pnpm@10.0.0"}`)
	writeFile(t, dir, "pnpm-workspace.yaml", "packages:\n  - apps/*\n")
	mkdir(t, dir, "node_modules")
	writeFile(t, dir, "apps/web/package.json", `{"name":"web"}`)
	mkdir(t, dir, "apps/web/node_modules")

	for _, s := range Analyze(dir) {
		if s.Kind == KindSymlink {
			t.Errorf("no symlink suggestion expected in a workspace, got %+v", s)
		}
	}
}
