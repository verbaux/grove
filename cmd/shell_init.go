package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(shellInitCmd)
}

var shellInitCmd = &cobra.Command{
	Use:   "shell-init [bash|zsh|fish|powershell]",
	Short: "Print shell integration (completion + gcd helper) to eval",
	Long: `Print shell integration for grove: tab completion plus a "gcd" helper
that cds straight into a worktree (with the fuzzy picker when called with
no argument).

Add one line to your shell startup file:

  # ~/.zshrc or ~/.bashrc
  eval "$(grove shell-init zsh)"

  # ~/.config/fish/config.fish
  grove shell-init fish | source

Then: gcd <name>   (or gcd with no args for the picker)`,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := args[0]

		var err error
		switch shell {
		case "bash":
			err = rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			err = rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			err = rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			err = rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		if err != nil {
			return err
		}

		fmt.Fprint(os.Stdout, gcdSnippet(shell))
		return nil
	},
}

// gcdSnippet returns the shell-specific "gcd" helper function.
func gcdSnippet(shell string) string {
	switch shell {
	case "fish":
		return "\nfunction gcd\n    cd (grove cd $argv)\nend\n"
	case "powershell":
		return "\nfunction gcd { Set-Location (grove cd @args) }\n"
	default: // bash, zsh
		return "\ngcd() { cd \"$(grove cd \"$@\")\"; }\n"
	}
}
