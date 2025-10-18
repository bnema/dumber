package main

import (
	"embed"
	"log"
	"os"
	"runtime"

	"github.com/bnema/dumber/internal/app/browser"
	"github.com/bnema/dumber/internal/app/cli"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
)

//go:embed assets
var assets embed.FS

// Build information set via ldflags
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	// Setup basic crash handling early (before any potential crashes)
	logging.SetupCrashHandler()

	// Check if we should run the CLI mode
	if shouldRunCLI() {
		cli.Execute(version, commit, buildDate)
		return
	}

	// Initialize logging early for GUI mode using default config
	cfg := config.New()
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
		log.Printf("Warning: failed to initialize logging: %v", err)
	}

	log.Printf("Starting dumber browser v%s (commit: %s, built: %s)", version, commit, buildDate)

	// GTK requires all UI calls to run on the main OS thread
	if webkit.IsNativeAvailable() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}

	// Otherwise run the GUI browser
	browser.Run(assets, version, commit, buildDate)
}

// shouldRunCLI determines if we should run in CLI mode based on arguments
func shouldRunCLI() bool {
	// No args = show CLI help with available commands
	if len(os.Args) <= 1 {
		return true
	}

	// Check for GUI-specific flags
	for _, arg := range os.Args[1:] {
		if arg == "--gui" || arg == "-g" {
			return false // Explicit GUI mode
		}
	}

	// Check for browse command - this should open GUI in direct navigation mode
	if len(os.Args) >= 2 && os.Args[1] == "browse" {
		return false // Browse command uses GUI mode but navigates directly
	}

	// Any other arguments mean CLI mode
	return true
}
