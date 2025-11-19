package cli

import (
	"fmt"
	"os"

	"github.com/bnema/dumber/internal/webext"
	"github.com/spf13/cobra"
)

func newExtensionsUpdateCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "update [id]",
		Short: "Check for updates (bundled uBlock Origin and user extensions with update URLs)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			targetID := ""
			if len(args) == 1 {
				targetID = args[0]
			}

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

			if targetID == "" || targetID == webext.UBOExtensionID() {
				if err := manager.EnsureUBlockOrigin(); err != nil {
					return fmt.Errorf("failed to update uBlock Origin: %w", err)
				}
				fmt.Println("Checked uBlock Origin")
				if apply {
					fmt.Println("uBlock Origin updated if newer version exists.")
				}
			}

			if targetID != "" && targetID != webext.UBOExtensionID() {
				fmt.Printf("Update check for user extensions is not implemented yet (requested %s)\n", targetID)
				return nil
			}

			if targetID == "" {
				fmt.Println("User extension update checks not implemented yet.")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&apply, "apply", false, "Apply available updates (uBlock Origin)")
	return cmd
}
