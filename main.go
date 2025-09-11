package main

import (
	"embed"
	"os"
	"runtime"

	"github.com/bnema/dumber/internal/app/browser"
	"github.com/bnema/dumber/internal/app/cli"
	"github.com/bnema/dumber/pkg/webkit"
)

//go:embed frontend/dist
var assets embed.FS

// Build information set via ldflags
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	// Check if we should run the CLI mode
	if shouldRunCLI() {
		cli.Execute(version, commit, buildDate)
		return
	}

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
	// No args = GUI landing page mode
	if len(os.Args) <= 1 {
		return false
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