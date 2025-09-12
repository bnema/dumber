package browser

import (
	"database/sql"
	"embed"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/bnema/dumber/internal/app/api"
	"github.com/bnema/dumber/internal/app/control"
	"github.com/bnema/dumber/internal/app/environment"
	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/pkg/webkit"
	"github.com/bnema/dumber/services"
)

// BrowserApp represents the main browser application
type BrowserApp struct {
	version   string
	commit    string
	buildDate string
	assets    embed.FS

	// Core components
	config   *config.Config
	database *sql.DB
	queries  *db.Queries

	// Services
	parserService  *services.ParserService
	browserService *services.BrowserService

	// WebView and controllers
	webView              *webkit.WebView
	zoomController       *control.ZoomController
	navigationController *control.NavigationController
	clipboardController  *control.ClipboardController

	// Handlers
	schemeHandler   *api.SchemeHandler
	messageHandler  *messaging.Handler
	shortcutHandler *ShortcutHandler
}

// Run starts the browser application
func Run(assets embed.FS, version, commit, buildDate string) {
	log.Printf("Starting GUI mode (webkit_cgo=%v)", webkit.IsNativeAvailable())

	app := &BrowserApp{
		version:   version,
		commit:    commit,
		buildDate: buildDate,
		assets:    assets,
	}

	if err := app.Initialize(); err != nil {
		log.Printf("Failed to initialize browser: %v", err)
		if webkit.IsNativeAvailable() {
			runtime.UnlockOSThread()
		}
		os.Exit(1)
	}

	app.Run()
}

// Initialize sets up all browser components
func (app *BrowserApp) Initialize() error {
	// Setup media pipeline hardening
	environment.SetupMediaPipeline()

	// Initialize configuration
	if err := config.Init(); err != nil {
		return err
	}
	app.config = config.Get()

	// Apply environment configurations
	environment.ApplyCodecConfiguration(app.config.CodecPreferences)

	// Initialize logging
	if err := logging.Init(
		app.config.Logging.LogDir,
		app.config.Logging.Level,
		app.config.Logging.Format,
		app.config.Logging.EnableFileLog,
		app.config.Logging.MaxSize,
		app.config.Logging.MaxBackups,
		app.config.Logging.MaxAge,
		app.config.Logging.Compress,
	); err != nil {
		log.Printf("Warning: failed to initialize logging: %v", err)
	}

	// Setup crash handler
	logging.SetupCrashHandler()

	// Setup output capture if configured
	if err := app.setupOutputCapture(); err != nil {
		log.Printf("Warning: failed to setup output capture: %v", err)
	}

	// Initialize WebKit log capture if configured
	if app.config.Logging.CaptureCOutput {
		if err := webkit.InitWebKitLogCapture(); err != nil {
			log.Printf("Warning: failed to initialize WebKit log capture: %v", err)
		} else {
			defer webkit.StopWebKitLogCapture()
		}
	}

	log.Printf("Config initialized")

	// Detect keyboard layout
	environment.DetectAndSetKeyboardLocale()

	// Initialize database
	database, err := db.InitDB(app.config.Database.Path)
	if err != nil {
		return err
	}
	app.database = database
	app.queries = db.New(database)
	log.Printf("Database opened at %s", app.config.Database.Path)

	// Initialize services
	app.parserService = services.NewParserService(app.config, app.queries)
	app.browserService = services.NewBrowserService(app.config, app.queries)

	// Initialize handlers
	app.schemeHandler = api.NewSchemeHandler(app.assets, app.parserService, app.browserService)
	app.messageHandler = messaging.NewHandler(app.parserService, app.browserService)

	return nil
}

// Run executes the main browser loop
func (app *BrowserApp) Run() {
	defer app.cleanup()
	defer logging.SetupPanicRecovery()

	// Register custom scheme resolver
	webkit.SetURISchemeResolver(func(uri string) (string, []byte, bool) {
		return app.schemeHandler.Handle(uri, app.config)
	})

	// Create and setup WebView
	if err := app.createWebView(); err != nil {
		log.Printf("Warning: failed to create WebView: %v", err)
		return
	}

	// Handle browse command if present
	app.navigationController.HandleBrowseCommand()

	// Setup signal handling
	app.setupSignalHandling()

	// Run main loop
	app.runMainLoop()
}

// cleanup handles cleanup on shutdown
func (app *BrowserApp) cleanup() {
	if app.database != nil {
		if closeErr := app.database.Close(); closeErr != nil {
			log.Printf("Warning: failed to close database: %v", closeErr)
		}
	}
}

// setupOutputCapture initializes stdout/stderr capture if configured
func (app *BrowserApp) setupOutputCapture() error {
	if app.config.Logging.CaptureStdout || app.config.Logging.CaptureStderr {
		log.Printf("Warning: stdout/stderr capture is experimental and may interfere with normal operations")
		if logger := logging.GetLogger(); logger != nil {
			outputCapture := logging.NewOutputCapture(logger)
			if err := outputCapture.Start(); err != nil {
				return err
			}
			// Note: defer outputCapture.Stop() should be handled by the caller
		}
	}
	return nil
}

// setupSignalHandling configures graceful shutdown on signals
func (app *BrowserApp) setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Handle signals in a goroutine to quit the main loop
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v - shutting down gracefully", sig)
		webkit.QuitMainLoop()
	}()
}

// runMainLoop starts the appropriate main loop based on WebKit availability
func (app *BrowserApp) runMainLoop() {
	if webkit.IsNativeAvailable() {
		log.Printf("Entering GTK main loop…")
		webkit.RunMainLoop()
		log.Printf("GTK main loop exited")
	} else {
		log.Printf("Not entering GUI loop (non-CGO build)")
		// In non-CGO mode, just wait for signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		log.Printf("Received signal %v - exiting", sig)
	}
}
