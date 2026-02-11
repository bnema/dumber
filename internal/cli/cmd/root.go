// Package cmd provides Cobra CLI commands for dumber.
package cmd

import (
	"errors"
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
		Use:           "dumber",
		Short:         "A fully unfeatured unbloated browser for tiling WMs",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long: `Dumber - a dumb browser that works like your favorite terminal multiplexer.

A fully unfeatured unbloated browser for tiling WMs, built with GTK4 and WebKitGTK.

Features:
  - Wayland native (Sway, Hyprland, River, Niri, etc.)
  - Tabs and workspaces with split or stacked panes
  - Keyboard-driven workflow inspired by Zellij
  - GPU rendering with automatic VA-API/VDPAU detection
  - Built-in ad blocking (UBlock-based network + cosmetic filtering)
  - Launcher integration (rofi/fuzzel) with favicons
  - Search shortcuts via bangs (!g, !gi, etc.)
  - Session management with auto-save and restore

Use 'dumber browse' to launch the graphical browser, or explore the
subcommands for CLI-based operations like history search and session
management.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Skip initialization for commands that don't need app context
			switch cmd.Name() {
			case "help", "completion", "gen-docs":
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
		var printedErr *printedError
		if errors.As(err, &printedErr) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type printedError struct {
	err error
}

func (e *printedError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *printedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func wrapPrintedError(err error) error {
	if err == nil {
		return nil
	}
	return &printedError{err: err}
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
