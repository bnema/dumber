// Package cmd provides Cobra CLI commands for dumber.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/cli"
	"github.com/bnema/dumber/internal/domain/build"
)

var (
	app       *cli.App
	buildInfo build.Info
	rootCmd   = &cobra.Command{
		Use:   "dumber",
		Short: "A minimal web browser",
		Long:  `Dumber is a minimal, keyboard-driven web browser built with GTK4 and WebKitGTK.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Skip initialization for help commands
			if cmd.Name() == "help" || cmd.Name() == "completion" {
				return nil
			}

			var err error
			app, err = cli.NewApp()
			if err != nil {
				return fmt.Errorf("initialize app: %w", err)
			}
			// Set build info from main.go
			app.BuildInfo = buildInfo
			return nil
		},
		PersistentPostRun: func(_ *cobra.Command, _ []string) {
			if app != nil {
				_ = app.Close()
			}
		},
	}
)

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// GetApp returns the initialized app (for use by subcommands).
func GetApp() *cli.App {
	return app
}

// browseCmd is a placeholder for help - actual execution is in main.go
var browseCmd = &cobra.Command{
	Use:   "browse [url]",
	Short: "Launch the graphical browser",
	Long: `Launch the GTK4 graphical browser.

If a URL is provided, navigate to it. Otherwise, open the homepage.

Examples:
  dumber browse                  # Open browser to homepage
  dumber browse example.com      # Open browser to URL`,
		Run: func(_ *cobra.Command, _ []string) {
		// This is handled by main.go before cobra runs
	},
}

func init() {
	rootCmd.AddCommand(browseCmd)
}

// SetBuildInfo sets the build information (called from main.go before Execute).
func SetBuildInfo(info build.Info) {
	buildInfo = info
}
