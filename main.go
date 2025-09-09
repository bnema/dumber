package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/bnema/dumber/internal/cli"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/services"
)

// Embed the frontend assets
//
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
		runCLI()
		return
	}

	// Otherwise run the GUI browser
	runBrowser()
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

// runCLI executes the CLI functionality using the existing CLI package
func runCLI() {
	// Initialize configuration system
	if err := config.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing configuration: %v\n", err)
		os.Exit(1)
	}

	// Start configuration watching for live reload
	if err := config.Watch(); err != nil {
		// Don't exit on watch error, just warn
		fmt.Fprintf(os.Stderr, "Warning: failed to start config watching: %v\n", err)
	}

	rootCmd := cli.NewRootCmd(version, commit, buildDate)

	// Handle dmenu flag at the root level for direct integration
	if len(os.Args) > 1 && os.Args[1] == "--dmenu" {
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
			os.Exit(1)
		}
		return
	}

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runBrowser executes the browser GUI functionality
func runBrowser() {
	// Initialize configuration
	if err := config.Init(); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}
	cfg := config.Get()

	// Initialize database
	database, err := db.InitDB(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	queries := db.New(database)

	// Initialize services
	parserService := services.NewParserService(cfg, queries)
	browserService := services.NewBrowserService(cfg, queries)
	configService := services.NewConfigService(cfg, "")

	// Load the injectable script into the browser service
	if err := browserService.LoadInjectableScript(assets); err != nil {
		log.Printf("Warning: Failed to load injectable script: %v", err)
	}

	// Check if this is a browse command with URL
	var directURL string
	var initialZoom float64 = 1.0
	if len(os.Args) >= 3 && os.Args[1] == "browse" {
		// Parse the URL using our parser service
		ctx := context.Background()
		result, err := parserService.ParseInput(ctx, os.Args[2])
		if err != nil {
			log.Printf("Error parsing URL: %v", err)
		} else {
			directURL = result.URL
			// Record the visit
			browserService.Navigate(ctx, directURL)
			// Load saved zoom level for the destination domain to apply natively
			if z, zerr := browserService.GetZoomLevel(ctx, directURL); zerr == nil && z > 0 {
				initialZoom = z
			}
		}
	}

	// Create application with correct v3-alpha options
	app := application.New(application.Options{
		Name:        "dumber-browser",
		Description: "A smart URL launcher and browser with learning behavior",
		LogLevel:    slog.LevelInfo,

		Services: []application.Service{
			application.NewService(parserService),
			application.NewService(browserService),
			application.NewService(configService),
		},

		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},

		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Always start with the browser UI, never directly navigate to external URLs
	startURL := "/"

	// Create the main window
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Dumber Browser",
		URL:   startURL,
		Zoom:  initialZoom,
		// Ensure the window can take full available space in tilers like Hyprland
		// by avoiding a too-small max size hint from GTK. Also start maximised.
		StartState:    application.WindowStateMaximised,
		DisableResize: false,
		MinWidth:      800,
		MinHeight:     600,
		// Use large max size hints so compositor isn't constrained by logical monitor size
		MaxWidth:  10000,
		MaxHeight: 10000,
		// Enable devtools and add native keybindings so they work on any page
		DevToolsEnabled: true,
		KeyBindings: map[string]func(window application.Window){
			// DevTools toggle (native; works on any page)
			"F12": func(w application.Window) { w.OpenDevTools() },
			// Native zoom shortcuts so they work on any origin
			// Note: current implementation does not persist zoom changes made via these shortcuts.
			// Zoom In variants
			"cmdorctrl+plus":       func(w application.Window) { w.ZoomIn() },
			"cmdorctrl+=":          func(w application.Window) { w.ZoomIn() },
			"cmdorctrl+shift+=":    func(w application.Window) { w.ZoomIn() },
			"cmdorctrl+shift+plus": func(w application.Window) { w.ZoomIn() },
			"cmdorctrl+-":          func(w application.Window) { w.ZoomOut() },
			"cmdorctrl+0":          func(w application.Window) { w.ZoomReset() },
		},
	})

	// Connect the browser service to the window for title updates and script injection
	browserService.SetWindowTitleUpdater(window)
	browserService.SetScriptInjector(window)

	// Show the window
	window.Show()

	// Set up navigation monitoring and script injection
	// We'll inject the script periodically to ensure it's available on all pages
	go func() {
		ctx := context.Background()
		
		// Initial injection after a short delay to let the page load
		time.Sleep(1 * time.Second)
		if err := browserService.InjectControlScript(ctx); err != nil {
			log.Printf("Initial script injection failed: %v", err)
		}
		
		// More aggressive re-injection to handle navigation and CSP restrictions
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				// Try to inject script - this will be ignored if already injected
				if err := browserService.InjectControlScript(ctx); err != nil {
					// Silently fail - this is expected on some pages with strict CSP
					continue
				}
			}
		}
	}()

	// Run the application (blocking call)
	err = app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
