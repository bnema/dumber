package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/bnema/dumber/internal/config"
	"github.com/spf13/cobra"
)

// NewConfigCmd creates the config command
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage dumber configuration",
		Long:  `Open the configuration file in your editor or print its path.`,
		RunE:  openConfig,
	}

	cmd.Flags().Bool("path", false, "Print the full path of the config file")

	return cmd
}

// openConfig opens the config file in the user's editor or prints its path
func openConfig(cmd *cobra.Command, _ []string) error {
	configPath, err := config.GetConfigFile()
	if err != nil {
		return fmt.Errorf("failed to get config file path: %w", err)
	}

	// If --path flag is set, just print the path
	printPath, _ := cmd.Flags().GetBool("path")
	if printPath {
		fmt.Println(configPath)
		return nil
	}

	// Get editor from environment (prefer $VISUAL, fallback to $EDITOR)
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		return fmt.Errorf("no editor defined: set $VISUAL or $EDITOR environment variable")
	}

	// Open the config file in the editor
	editorCmd := exec.Command(editor, configPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	return nil
}
