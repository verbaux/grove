package detect

import (
	"strings"
	"testing"
)

func cmakeSuggestions(sugs []Suggestion) []string {
	var out []string
	for _, s := range sugs {
		if s.Kind == KindAfterCreate && strings.HasPrefix(s.Value, "cmake ") {
			out = append(out, s.Value)
		}
	}
	return out
}

func TestDetectCMakeSinglePreset(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "CMakePresets.json", `{"version":3,"configurePresets":[
		{"name":"base","hidden":true},
		{"name":"dev","inherits":"base"}
	]}`)
	got := cmakeSuggestions(Analyze(dir))
	if len(got) != 1 || got[0] != "cmake --preset dev" {
		t.Errorf("got %v, want [cmake --preset dev]", got)
	}
}

func TestDetectCMakePrefersDefaultPreset(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "CMakePresets.json", `{"version":3,"configurePresets":[
		{"name":"release"},
		{"name":"default"}
	]}`)
	sugs := Analyze(dir)
	if !hasSuggestion(sugs, KindAfterCreate, "cmake --preset default") {
		t.Fatalf("expected default preset, got %+v", sugs)
	}
	for _, s := range sugs {
		if s.Value == "cmake --preset default" && !strings.Contains(s.Reason, "of 2 configure presets") {
			t.Errorf("reason should mention the other presets, got %q", s.Reason)
		}
	}
}

func TestDetectCMakeSkipsConditionalPresets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "CMakePresets.json", `{"version":3,"configurePresets":[
		{"name":"win-base","hidden":true,"condition":{"type":"equals","lhs":"${hostSystemName}","rhs":"Windows"}},
		{"name":"win","inherits":["win-base"]},
		{"name":"mac","condition":{"type":"equals","lhs":"${hostSystemName}","rhs":"Darwin"}},
		{"name":"from-include","inherits":"defined-elsewhere"},
		{"name":"plain","condition":null}
	]}`)
	got := cmakeSuggestions(Analyze(dir))
	if len(got) != 1 || got[0] != "cmake --preset plain" {
		t.Errorf("got %v, want [cmake --preset plain]", got)
	}
}

func TestDetectCMakeQuotesUnsafePresetName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "CMakePresets.json", `{"version":3,"configurePresets":[{"name":"my preset's"}]}`)
	got := cmakeSuggestions(Analyze(dir))
	want := `cmake --preset 'my preset'\''s'`
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %v, want [%s]", got, want)
	}
}

func TestDetectCMakeNoSuggestion(t *testing.T) {
	cases := map[string]string{
		"only hidden":  `{"configurePresets":[{"name":"base","hidden":true}]}`,
		"no presets":   `{"version":3}`,
		"invalid json": `{"configurePresets":`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "CMakePresets.json", body)
			if got := cmakeSuggestions(Analyze(dir)); len(got) != 0 {
				t.Errorf("expected no cmake suggestion, got %v", got)
			}
		})
	}
}

func TestDetectCMakeIgnoresUserPresets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "CMakeLists.txt", "project(x)\n")
	writeFile(t, dir, "CMakeUserPresets.json", `{"configurePresets":[{"name":"mine"}]}`)
	if got := cmakeSuggestions(Analyze(dir)); len(got) != 0 {
		t.Errorf("expected no suggestion from CMakeUserPresets.json, got %v", got)
	}
}
