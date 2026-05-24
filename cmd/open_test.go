package cmd

import (
	"testing"

	"github.com/verbaux/grove/internal/config"
)

func TestResolveEditor(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		cfg     string // editor field in .groverc.json; "" means no config file
		visual  string
		editor  string
		want    string
		wantErr bool
	}{
		{name: "flag wins over everything", flag: "code", cfg: "vim", visual: "nano", editor: "emacs", want: "code"},
		{name: "config wins over env", cfg: "vim", visual: "nano", editor: "emacs", want: "vim"},
		{name: "visual wins over editor", visual: "nano", editor: "emacs", want: "nano"},
		{name: "editor used last", editor: "emacs", want: "emacs"},
		{name: "nothing configured errors", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.cfg != "" {
				if err := config.Save(root, config.Config{Editor: tt.cfg}); err != nil {
					t.Fatalf("save config: %v", err)
				}
			}

			openEditor = tt.flag
			t.Cleanup(func() { openEditor = "" })
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)

			got, err := resolveEditor(root)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveEditor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"/home/dev/app":     `'/home/dev/app'`,
		"/path with spaces": `'/path with spaces'`,
		"/it's/a/path":      `'/it'\''s/a/path'`,
	}
	for in, want := range tests {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
