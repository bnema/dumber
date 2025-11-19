package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/config"
	"github.com/spf13/cobra"
)

func newExtensionsRemoveCmd() *cobra.Command {
	var keepData bool

	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove an extension",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			cli, err := NewCLI()
			if err != nil {
				return fmt.Errorf("failed to initialize CLI: %w", err)
			}
			defer cli.Close()

			manager, err := buildExtensionManager(cli)
			if err != nil {
				return err
			}

			ext, ok := manager.GetExtension(id)
			if !ok {
				return fmt.Errorf("extension not found: %s", id)
			}
			if ext.Bundled {
				return fmt.Errorf("cannot remove bundled extension: %s", id)
			}

			if err := os.RemoveAll(ext.Path); err != nil {
				return fmt.Errorf("failed to remove extension files: %w", err)
			}

			if !keepData {
				dataDir, err := config.GetDataDir()
				if err != nil {
					return fmt.Errorf("failed to get data dir: %w", err)
				}
				dataPath := filepath.Join(dataDir, "extension-data", id)
				_ = os.RemoveAll(dataPath)
			}

			if cli.Queries != nil {
				if err := cli.Queries.MarkExtensionDeleted(cmd.Context(), id); err != nil {
					return fmt.Errorf("failed to mark extension deleted: %w", err)
				}
			}

			fmt.Printf("Removed extension %s\n", id)
			return nil
		},
	}

	cmd.Flags().BoolVar(&keepData, "keep-data", false, "Keep extension data directory")

	return cmd
}
