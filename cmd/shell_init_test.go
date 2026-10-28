package cmd

import (
	"strings"
	"testing"
)

func TestGcdSnippet(t *testing.T) {
	tests := map[string]string{
		"bash":       `gcd() { cd "$(grove cd "$@")"; }`,
		"zsh":        `gcd() { cd "$(grove cd "$@")"; }`,
		"fish":       `set -l dir (grove cd $argv); and test -n "$dir"; and cd $dir`,
		"powershell": "function gcd { $dir = grove cd @args; if ($LASTEXITCODE -eq 0 -and $dir) { Set-Location $dir } }",
	}
	for shell, want := range tests {
		got := gcdSnippet(shell)
		if !strings.Contains(got, want) {
			t.Errorf("gcdSnippet(%q) = %q, want it to contain %q", shell, got, want)
		}
		if !strings.HasPrefix(got, "\n") || !strings.HasSuffix(got, "\n") {
			t.Errorf("gcdSnippet(%q) should be newline-padded, got %q", shell, got)
		}
	}
}
