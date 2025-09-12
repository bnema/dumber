package cli

import (
	"fmt"
	"os"

	"github.com/bnema/dumber/internal/cli"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
)

// Execute runs the CLI functionality using the existing CLI package
func Execute(version, commit, buildDate string) {
	// Initialize configuration system
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logging system for CLI mode (file logging only, no output capture)
	cfg := config.Get()
	if err := logging.Init(
		cfg.Logging.LogDir,
		cfg.Logging.Level,
		cfg.Logging.Format,
		cfg.Logging.EnableFileLog,
		cfg.Logging.MaxSize,
		cfg.Logging.MaxBackups,
		cfg.Logging.MaxAge,
		cfg.Logging.Compress,
	); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to initialize logging: %v\n", err)
	}

	// Start configuration watching for live reload
	if err := config.Watch(); err != nil {
		// Don't exit on watch error, just warn
		fmt.Fprintf(os.Stderr, "Warning: failed to start config watching: %v\n", err)
	}

	rootCmd := cli.NewRootCmd(version, commit, buildDate)

	// Handle dmenu flag at the root level for direct integration
	if len(os.Args) > 1 && os.Args[1] == "--dmenu" {
		handleDmenuMode()
		return
	}

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// handleDmenuMode processes dmenu-specific functionality
func handleDmenuMode() {
	// Create a temporary CLI instance to handle dmenu mode
	cliInstance, err := cli.NewCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing CLI: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := cliInstance.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", closeErr)
		}
	}()

	// Generate dmenu options
	dmenuCmd := cli.NewDmenuCmd()
	if err := dmenuCmd.RunE(dmenuCmd, []string{}); err != nil {
		fmt.Fprintf(os.Stderr, "Error in dmenu mode: %v\n", err)
		if closeErr := cliInstance.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", closeErr)
		}
		os.Exit(1)
	}
}
