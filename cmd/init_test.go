package cmd

import (
	"bufio"
	"reflect"
	"strings"
	"testing"

	"github.com/verbaux/grove/internal/config"
)

func TestParsePortRange(t *testing.T) {
	valid := map[string][2]int{
		"3001-3999":   {3001, 3999},
		" 1 - 65535 ": {1, 65535},
		"8080-8080":   {8080, 8080},
	}
	for input, want := range valid {
		got, err := parsePortRange(input)
		if err != nil || got.Min != want[0] || got.Max != want[1] {
			t.Errorf("parsePortRange(%q) = %+v, %v; want %v", input, got, err, want)
		}
	}
	for _, input := range []string{"", "3000", "a-b", "0-10", "20-10", "60000-70000", "1-65536"} {
		if got, err := parsePortRange(input); err == nil {
			t.Errorf("parsePortRange(%q) = %+v, want error", input, got)
		}
	}
}

func TestApplyDetectPromptsRoutesInstallByAcceptedSymlink(t *testing.T) {
	tests := []struct {
		name         string
		answers      string
		wantSymlink  []string
		wantAfter    config.HookCommands
		wantDetached config.HookCommands
	}{
		{"symlink accepted", "y\ny\n", []string{"node_modules"}, nil, config.HookCommands{"pnpm install"}},
		{"symlink declined", "n\ny\n", nil, config.HookCommands{"pnpm install"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeNodeProject(t)
			reader = bufio.NewReader(strings.NewReader(tt.answers))
			t.Cleanup(func() { reader = nil })

			got := applyDetectPrompts(root, config.Config{})

			if !reflect.DeepEqual(got.Symlink, tt.wantSymlink) ||
				!reflect.DeepEqual(got.AfterCreate, tt.wantAfter) ||
				!reflect.DeepEqual(got.AfterDetachedCreate, tt.wantDetached) {
				t.Errorf("got symlink=%v afterCreate=%v afterDetachedCreate=%v", got.Symlink, got.AfterCreate, got.AfterDetachedCreate)
			}
		})
	}
}
