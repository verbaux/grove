package cmd

import (
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/state"
)

const maxNoteRunes = 200

var noteClear bool

func init() {
	rootCmd.AddCommand(noteCmd)
	noteCmd.Flags().BoolVar(&noteClear, "clear", false, "remove the worktree note")
}

var noteCmd = &cobra.Command{
	Use:   "note <name-or-number> [text]",
	Short: "Show or update a managed worktree note",
	Long: `Show, set, or clear the short local note attached to a managed worktree.

Notes are stored in .grove/state.json and shown by grove list and grove status.
Pass multiple words quoted or unquoted; use --clear to remove the note.`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeAliases,
	RunE:              runNote,
}

func runNote(_ *cobra.Command, args []string) error {
	if noteClear && len(args) > 1 {
		return fmt.Errorf("--clear cannot be combined with note text")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}
	resolved, err := resolveWorktree(root, args[0])
	if err != nil {
		return err
	}
	if resolved == nil {
		return fmt.Errorf("no worktree matching %q — run 'grove list' to see available worktrees", args[0])
	}
	if resolved.IsMain {
		return fmt.Errorf("refusing to add a note to the main worktree")
	}
	if !resolved.InState {
		return fmt.Errorf("cannot note orphan worktree %q — run 'grove adopt' first", args[0])
	}

	if len(args) == 1 && !noteClear {
		note, err := loadCurrentNote(root, resolved)
		if err != nil {
			return err
		}
		if note == "" {
			fmt.Printf("No note set for %q.\n", resolved.Alias)
			return nil
		}
		fmt.Println(note)
		return nil
	}

	note := ""
	if !noteClear {
		note, err = normalizeNote(strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
	}
	if err := updateNote(root, resolved, note); err != nil {
		return err
	}
	if noteClear {
		fmt.Printf("Worktree %q note cleared.\n", resolved.Alias)
	} else {
		fmt.Printf("Worktree %q note updated.\n", resolved.Alias)
	}
	return nil
}

func normalizeNote(note string) (string, error) {
	if !utf8.ValidString(note) {
		return "", fmt.Errorf("note must be valid UTF-8")
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return "", fmt.Errorf("note cannot be empty — use --clear to remove it")
	}
	if strings.ContainsAny(note, "\r\n") {
		return "", fmt.Errorf("note must be a single line")
	}
	if strings.IndexFunc(note, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("note cannot contain control characters")
	}
	if utf8.RuneCountInString(note) > maxNoteRunes {
		return "", fmt.Errorf("note must be at most %d characters", maxNoteRunes)
	}
	return note, nil
}

func loadCurrentNote(root string, resolved *resolvedWorktree) (string, error) {
	latest, err := state.Load(root)
	if err != nil {
		return "", err
	}
	entry, ok := latest.Get(resolved.Alias)
	if !ok {
		return "", fmt.Errorf("state entry %q disappeared while reading note", resolved.Alias)
	}
	if entry.Path != resolved.Path || entry.Branch != resolved.Branch {
		return "", fmt.Errorf("state entry %q changed while reading note — retry", resolved.Alias)
	}
	return entry.Note, nil
}

func updateNote(root string, resolved *resolvedWorktree, note string) error {
	return state.Update(root, func(latest *state.State) error {
		entry, ok := latest.Get(resolved.Alias)
		if !ok {
			return fmt.Errorf("state entry %q disappeared while updating note", resolved.Alias)
		}
		if entry.Path != resolved.Path || entry.Branch != resolved.Branch {
			return fmt.Errorf("state entry %q changed while updating note — retry", resolved.Alias)
		}
		return latest.SetNote(resolved.Alias, note)
	})
}
