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

A fully unfeatured unbloated browser for tiling WMs, built with GTK4. CEF is the default backend; WebKitGTK is available as a fallback.

Features:
  - Wayland native (Sway, Hyprland, River, Niri, etc.)
  - Tabs and workspaces with split or stacked panes
  - Keyboard-driven workflow inspired by Zellij
  - GPU rendering with automatic VA-API/VDPAU detection
  - Built-in ad blocking (UBlock-based network + cosmetic filtering)
  - Launcher integration (rofi/fuzzel) with favicons
  - Search shortcuts via bangs (!g, !gi, etc.)
  - Session management with auto-save and restore

Use 'dumber browse' to launch the graphical browser, 'dumber omnibox' to open
the standalone launcher overlay, or explore the subcommands for CLI-based
operations like history search and session management.`,
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
	Args:  cobra.MaximumNArgs(1),
	Long: `Launch the GTK4 graphical browser.

If a URL is provided, navigate to it. Otherwise, open the homepage.

With --instance <name> (CEF only), Dumber opens or focuses one dedicated
window inside the current CEF profile host. Named instances share cookies,
tokens, local storage, configuration, favorites and history by default.

Use --profile <name> for a separate persistent CEF profile and host process.
Use --ephemeral for a fresh temporary CEF profile removed when its host exits.
--profile and --ephemeral are mutually exclusive.

Examples:
  dumber browse                                      # Open browser normally
  dumber browse example.com                          # Open browser to URL
  dumber browse --instance scratch example.com       # Dedicated shared-profile window
  dumber browse --instance work --profile work       # Isolated persistent profile
  dumber browse --instance private --ephemeral       # Temporary isolated profile`,
	PreRunE: func(command *cobra.Command, _ []string) error {
		instance, _ := command.Flags().GetString("instance")
		profile, _ := command.Flags().GetString("profile")
		ephemeral, _ := command.Flags().GetBool("ephemeral")
		if command.Flags().Changed("instance") && instance == "" {
			return fmt.Errorf("--instance must not be empty")
		}
		if command.Flags().Changed("profile") && profile == "" {
			return fmt.Errorf("--profile must not be empty")
		}
		if profile != "" && ephemeral {
			return fmt.Errorf("--profile and --ephemeral are mutually exclusive")
		}
		return nil
	},
	Run: func(_ *cobra.Command, _ []string) {
		// Valid GUI launches are handled by main.go before Cobra runs.
	},
}

func init() {
	browseCmd.Flags().AddFlagSet(browseFlags())
	rootCmd.AddCommand(browseCmd)
}

// SetBuildInfo sets the build information (called from main.go before Execute).
func SetBuildInfo(info build.Info) {
	buildInfo = info
}
