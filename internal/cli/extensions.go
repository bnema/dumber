package cli

import "github.com/spf13/cobra"

// NewExtensionsCmd creates the root extensions command.
func NewExtensionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extensions",
		Short: "Manage browser extensions",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Show help when no subcommand is provided
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newExtensionsListCmd(),
		newExtensionsEnableCmd(),
		newExtensionsDisableCmd(),
		newExtensionsAddCmd(),
		newExtensionsRemoveCmd(),
		newExtensionsUpdateCmd(),
	)

	return cmd
}
