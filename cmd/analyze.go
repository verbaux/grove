package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/detect"
)

var (
	analyzeJSON   bool
	analyzeApply  bool
	analyzeDryRun bool
	analyzeYes    bool
	analyzeClean  bool
)

func init() {
	rootCmd.AddCommand(analyzeCmd)
	analyzeCmd.Flags().BoolVar(&analyzeJSON, "json", false, "Emit suggestions as JSON for scripting")
	analyzeCmd.Flags().BoolVar(&analyzeApply, "apply", false, "Merge pending suggestions into .groverc.json")
	analyzeCmd.Flags().BoolVar(&analyzeDryRun, "dry-run", false, "With --apply, print resulting config without writing")
	analyzeCmd.Flags().BoolVar(&analyzeYes, "yes", false, "With --apply, skip confirmation prompt")
	analyzeCmd.Flags().BoolVar(&analyzeClean, "clean", false, "Include stale symlink/copyDir entries (paths missing in main repo) for removal")
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze the project and suggest .groverc.json additions",
	Long: `Scan the current project for known framework signals (husky, package
managers, Next.js, Turbo, ...) and print suggested additions to .groverc.json.

Suggestions already present in the loaded config are filtered out.
Output is non-mutating: review and apply manually, or rerun 'grove init'.`,
	RunE: runAnalyze,
}

// StalePath is a symlink or copyDir entry whose target no longer exists in the main repo.
type StalePath struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	root := cwd
	cfg := config.Config{}
	hasConfig := false
	if r, err := config.FindRoot(cwd); err == nil {
		root = r
		if c, err := config.Load(root); err == nil {
			cfg = c
			hasConfig = true
		}
	}

	all := detect.Adapt(cfg.Symlink, detect.Analyze(root))
	pending := pendingSuggestions(cfg, all)
	var stale []StalePath
	if analyzeClean {
		stale = findStalePaths(root, cfg)
	}

	if analyzeApply {
		if !hasConfig {
			return fmt.Errorf("no .groverc.json found at %s — run 'grove init' first", root)
		}
		return applyChanges(root, cfg, pending, stale)
	}

	if analyzeJSON {
		return printAnalyzeJSON(all, pending, stale)
	}
	printAnalyzeHuman(root, cfg, all, pending, stale)
	return nil
}

func applyChanges(root string, cfg config.Config, pending []detect.Suggestion, stale []StalePath) error {
	if len(pending) == 0 && len(stale) == 0 {
		fmt.Println("nothing to apply — no pending additions or stale entries")
		return nil
	}

	merged := mergeSuggestions(cfg, pending)
	merged = removeStalePaths(merged, stale)

	if len(pending) > 0 {
		fmt.Println("Pending additions:")
		for _, s := range pending {
			fmt.Printf("  + %s %q\n", s.Kind, s.Value)
		}
	}
	if len(stale) > 0 {
		if len(pending) > 0 {
			fmt.Println()
		}
		fmt.Println("Stale entries to remove:")
		for _, s := range stale {
			fmt.Printf("  - %s %q\n", s.Kind, s.Value)
		}
	}

	if analyzeDryRun {
		fmt.Println()
		fmt.Println("--- resulting .groverc.json (dry-run, not written) ---")
		data, err := json.MarshalIndent(merged, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if !analyzeYes {
		fmt.Println()
		answer := strings.ToLower(prompt(fmt.Sprintf("Write %d addition(s) and %d removal(s) to .groverc.json? [y/N]", len(pending), len(stale)), "n"))
		if answer != "y" {
			fmt.Println("aborted")
			return nil
		}
	}

	if err := config.Save(root, merged); err != nil {
		return err
	}
	fmt.Printf("updated %s\n", filepath.Join(root, config.FileName))
	return nil
}

func findStalePaths(root string, cfg config.Config) []StalePath {
	var out []StalePath
	check := func(kind, name string) {
		if name == "" {
			return
		}
		if _, err := os.Stat(filepath.Join(root, name)); os.IsNotExist(err) {
			out = append(out, StalePath{Kind: kind, Value: name})
		}
	}
	for _, name := range cfg.Symlink {
		check("symlink", name)
	}
	for _, name := range cfg.CopyDirs {
		check("copyDir", name)
	}
	return out
}

func removeStalePaths(cfg config.Config, stale []StalePath) config.Config {
	if len(stale) == 0 {
		return cfg
	}
	dropSymlink := map[string]bool{}
	dropCopy := map[string]bool{}
	for _, s := range stale {
		switch s.Kind {
		case "symlink":
			dropSymlink[s.Value] = true
		case "copyDir":
			dropCopy[s.Value] = true
		}
	}
	out := cfg
	out.Symlink = filterStrings(cfg.Symlink, dropSymlink)
	out.CopyDirs = filterStrings(cfg.CopyDirs, dropCopy)
	return out
}

func filterStrings(in []string, drop map[string]bool) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !drop[v] {
			out = append(out, v)
		}
	}
	return out
}

func mergeSuggestions(cfg config.Config, pending []detect.Suggestion) config.Config {
	out := cfg
	out.Symlink = append([]string(nil), cfg.Symlink...)
	out.CopyDirs = append([]string(nil), cfg.CopyDirs...)
	out.AfterCreate = append(config.AfterCreate(nil), cfg.AfterCreate...)
	out.AfterDetachedCreate = append(config.AfterCreate(nil), cfg.AfterDetachedCreate...)
	for _, s := range pending {
		switch s.Kind {
		case detect.KindSymlink:
			out.Symlink = append(out.Symlink, s.Value)
		case detect.KindCopyDir:
			out.CopyDirs = append(out.CopyDirs, s.Value)
		case detect.KindAfterCreate:
			out.AfterCreate = append(out.AfterCreate, s.Value)
		case detect.KindAfterDetachedCreate:
			out.AfterDetachedCreate = append(out.AfterDetachedCreate, s.Value)
		}
	}
	return out
}

func pendingSuggestions(cfg config.Config, all []detect.Suggestion) []detect.Suggestion {
	out := make([]detect.Suggestion, 0, len(all))
	for _, s := range all {
		if !configCoversSuggestion(cfg, s) {
			out = append(out, s)
		}
	}
	return groupByKind(out)
}

// groupByKind reorders suggestions so output reads symlinks → copyDirs → afterCreate.
// Stable ordering preserves rule order within each kind.
func groupByKind(sugs []detect.Suggestion) []detect.Suggestion {
	out := append([]detect.Suggestion(nil), sugs...)
	rank := map[detect.Kind]int{
		detect.KindSymlink:             0,
		detect.KindCopyDir:             1,
		detect.KindAfterCreate:         2,
		detect.KindAfterDetachedCreate: 3,
	}
	sort.SliceStable(out, func(i, j int) bool {
		return rank[out[i].Kind] < rank[out[j].Kind]
	})
	return out
}

func printAnalyzeHuman(root string, cfg config.Config, all, pending []detect.Suggestion, stale []StalePath) {
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	add := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	rm := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	ok := lipgloss.NewStyle().Foreground(lipgloss.Color("34"))

	fmt.Println(hint.Render("Project: ") + root)
	fmt.Println()

	if len(all) == 0 && len(stale) == 0 {
		fmt.Println(ok.Render("  ✓ ") + "no detectable conventions and no stale entries — nothing to suggest")
		return
	}

	if len(pending) == 0 && len(stale) == 0 {
		fmt.Println(ok.Render("  ✓ ") + ".groverc.json already covers all detected conventions")
		return
	}

	if len(pending) > 0 {
		fmt.Println("Suggested additions:")
		for _, s := range pending {
			fmt.Printf("%s %s %q\n", add.Render("  +"), s.Kind, s.Value)
			fmt.Println(hint.Render("      reason: " + s.Reason))
		}
	}

	if len(stale) > 0 {
		if len(pending) > 0 {
			fmt.Println()
		}
		fmt.Println("Stale entries (no target in main repo):")
		for _, s := range stale {
			fmt.Printf("%s %s %q\n", rm.Render("  -"), s.Kind, s.Value)
		}
	}

	fmt.Println()
	if analyzeClean {
		fmt.Println(hint.Render("Apply with: grove analyze --apply --clean"))
	} else {
		fmt.Println(hint.Render("Apply with: grove analyze --apply  (add --clean to remove stale entries too)"))
	}

	if covered := len(all) - len(pending); covered > 0 {
		fmt.Println()
		fmt.Printf("(%d suggestion(s) already covered by current config)\n", covered)
	}
	_ = cfg
}

func printAnalyzeJSON(all, pending []detect.Suggestion, stale []StalePath) error {
	payload := struct {
		All     []detect.Suggestion `json:"all"`
		Pending []detect.Suggestion `json:"pending"`
		Stale   []StalePath         `json:"stale,omitempty"`
	}{All: all, Pending: pending, Stale: stale}
	return json.NewEncoder(os.Stdout).Encode(payload)
}
