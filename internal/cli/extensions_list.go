package cli

import (
	"fmt"
	"os"

	tuiext "github.com/bnema/dumber/internal/tui/extensions"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func newExtensionsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Interactive list of extensions",
		RunE: func(_ *cobra.Command, _ []string) error {
			cli, err := NewCLI()
			if err != nil {
				return fmt.Errorf("failed to initialize CLI: %w", err)
			}
			defer func() {
				if err := cli.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
				}
			}()

			manager, err := buildExtensionManager(cli)
			if err != nil {
				return err
			}

			model := tuiext.NewModel(manager)
			prog := tea.NewProgram(model, tea.WithAltScreen())
			if _, err := prog.Run(); err != nil {
				return fmt.Errorf("tui error: %w", err)
			}

			return nil
		},
	}

	return cmd
}
