package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/verbaux/grove/internal/config"
	"github.com/verbaux/grove/internal/templates"
)

func init() {
	rootCmd.AddCommand(templateCmd)
	templateCmd.AddCommand(templateSaveCmd)
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateApplyCmd)
	templateCmd.AddCommand(templateDeleteCmd)

	templateApplyCmd.Flags().BoolP("force", "f", false, "Overwrite existing .groverc.json without prompting")
}

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage reusable .groverc.json templates",
	Long: `Save, list, and apply reusable templates of .groverc.json.

Templates are stored in $XDG_CONFIG_HOME/grove/templates (or ~/.config/grove/templates)
and can be shared across projects.`,
}

var templateSaveCmd = &cobra.Command{
	Use:   "save <name>",
	Short: "Save the current .groverc.json as a template",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateSave,
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved templates",
	RunE:  runTemplateList,
}

var templateShowCmd = &cobra.Command{
	Use:               "show <name>",
	Short:             "Print a template's contents",
	Args:              cobra.ExactArgs(1),
	RunE:              runTemplateShow,
	ValidArgsFunction: completeTemplates,
}

var templateApplyCmd = &cobra.Command{
	Use:               "apply <name>",
	Short:             "Copy a template to ./.groverc.json in the current directory",
	Args:              cobra.ExactArgs(1),
	RunE:              runTemplateApply,
	ValidArgsFunction: completeTemplates,
}

var templateDeleteCmd = &cobra.Command{
	Use:               "delete <name>",
	Short:             "Delete a saved template",
	Args:              cobra.ExactArgs(1),
	RunE:              runTemplateDelete,
	ValidArgsFunction: completeTemplates,
}

func runTemplateSave(cmd *cobra.Command, args []string) error {
	name := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := config.FindRoot(cwd)
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	if templates.Exists(name) {
		answer := prompt(fmt.Sprintf("Template %q already exists. Overwrite? [y/N]", name), "n")
		if strings.ToLower(answer) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	if err := templates.Save(name, cfg); err != nil {
		return err
	}
	fmt.Printf("Saved template %q\n", name)
	return nil
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	names, err := templates.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Println("No templates saved.")
		fmt.Println("  Save one: grove template save <name>")
		return nil
	}
	dir, _ := templates.Dir()
	fmt.Printf("Templates in %s:\n", dir)
	for _, n := range names {
		fmt.Printf("  %s\n", n)
	}
	return nil
}

func runTemplateShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	cfg, err := templates.Load(name)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func runTemplateApply(cmd *cobra.Command, args []string) error {
	name := args[0]
	force, _ := cmd.Flags().GetBool("force")

	cfg, err := templates.Load(name)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if _, err := os.Stat(cwd + "/" + config.FileName); err == nil && !force {
		answer := prompt(".groverc.json already exists in this directory. Overwrite? [y/N]", "n")
		if strings.ToLower(answer) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	if err := config.Save(cwd, cfg); err != nil {
		return err
	}
	fmt.Printf("Applied template %q — .groverc.json written\n", name)
	return nil
}

func runTemplateDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := templates.Delete(name); err != nil {
		return err
	}
	fmt.Printf("Deleted template %q\n", name)
	return nil
}

func completeTemplates(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names, err := templates.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
