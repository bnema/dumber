package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newExtensionsEnableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <id> [id...]",
		Short: "Enable one or more extensions",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
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

			for _, id := range args {
				if err := manager.Enable(id); err != nil {
					return err
				}
				fmt.Printf("Enabled %s\n", id)
			}
			return nil
		},
	}
	return cmd
}
