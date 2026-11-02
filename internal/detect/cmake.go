package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type cmakeConfigurePreset struct {
	Name      string          `json:"name"`
	Hidden    bool            `json:"hidden"`
	Inherits  json.RawMessage `json:"inherits"`
	Condition json.RawMessage `json:"condition"`
}

// shellSafeWord matches preset names that need no quoting in `sh -c`.
var shellSafeWord = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// detectCMakePresets suggests configuring the build from a committed
// CMakePresets.json. CMakeUserPresets.json is ignored because it is local to
// one checkout. Presets with a condition, own or inherited, are skipped: the
// detector cannot tell whether the condition holds on this machine.
func detectCMakePresets(root string) []Suggestion {
	presets := readCMakeConfigurePresets(filepath.Join(root, "CMakePresets.json"))
	byName := make(map[string]cmakeConfigurePreset, len(presets))
	for _, p := range presets {
		byName[p.Name] = p
	}
	var usable []string
	for _, p := range presets {
		if p.Name != "" && !p.Hidden && !cmakeHasCondition(p, byName, map[string]bool{}) {
			usable = append(usable, p.Name)
		}
	}
	if len(usable) == 0 {
		return nil
	}

	name := usable[0]
	for _, n := range usable {
		if n == "default" {
			name = n
			break
		}
	}
	reason := "CMakePresets.json detected; configure the build tree and compile_commands.json in the worktree"
	if len(usable) > 1 {
		reason += " (picked " + strconv.Quote(name) + " of " + strconv.Itoa(len(usable)) + " configure presets; edit to choose another)"
	}
	return []Suggestion{{
		Kind:   KindAfterCreate,
		Value:  "cmake --preset " + shellQuote(name),
		Reason: reason,
	}}
}

func readCMakeConfigurePresets(p string) []cmakeConfigurePreset {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var doc struct {
		ConfigurePresets []cmakeConfigurePreset `json:"configurePresets"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	return doc.ConfigurePresets
}

// cmakeHasCondition reports whether p or any preset it inherits from declares
// a condition. A null condition counts as none. Unknown parents are treated as
// conditional, because they may live in an included file.
func cmakeHasCondition(p cmakeConfigurePreset, byName map[string]cmakeConfigurePreset, seen map[string]bool) bool {
	if c := strings.TrimSpace(string(p.Condition)); c != "" && c != "null" {
		return true
	}
	if seen[p.Name] {
		return false
	}
	seen[p.Name] = true
	for _, parent := range cmakeInherits(p.Inherits) {
		pp, ok := byName[parent]
		if !ok || cmakeHasCondition(pp, byName, seen) {
			return true
		}
	}
	return false
}

// cmakeInherits decodes "inherits", which CMake allows as a string or a list.
func cmakeInherits(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return []string{one}
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many
	}
	return nil
}

func shellQuote(s string) string {
	if shellSafeWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
