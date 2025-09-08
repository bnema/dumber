package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/bnema/dumber/internal/cli"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/services"
)

// Embed the frontend assets
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

	// Check if this is a browse command with URL
	var directURL string
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

	// Determine the starting URL
	startURL := "/"
	if directURL != "" {
		startURL = directURL
	}

	// Create the main window
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Dumber Browser",
		Width:  1200,
		Height: 800,
		URL:    startURL,
	})
	
	// Show the window
	window.Show()

	// Run the application (blocking call)
	err = app.Run()
	if err != nil {
		log.Fatal(err)
	}
}