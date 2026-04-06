package cmd

import (
	"github.com/spf13/cobra"
)

var omniboxCmd = &cobra.Command{
	Use:   "omnibox",
	Short: "Launch the standalone omnibox overlay",
	Args:  cobra.NoArgs,
	Long: `Launch the GTK omnibox as a standalone surface.

On Wayland, dumber will enable layer-shell when available so the omnibox can
appear as its own overlay instead of inside the main browser window.`,
	Run: func(_ *cobra.Command, _ []string) {
		// Handled by cmd/dumber/main.go before Cobra executes.
	},
}

func init() {
	rootCmd.AddCommand(omniboxCmd)
}
