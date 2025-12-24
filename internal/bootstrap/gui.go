// Package bootstrap provides initialization utilities for the browser.
package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/colorscheme"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/deps"
	"github.com/bnema/dumber/internal/infrastructure/media"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/theme"
	sqlite3 "github.com/ncruces/go-sqlite3"
)

// DatabaseResult holds database connection and cleanup function.
type DatabaseResult struct {
	DB      *sql.DB
	Cleanup func()
}

// ParallelInitResult holds the results of parallel initialization phase.
type ParallelInitResult struct {
	DataDir         string
	CacheDir        string
	ThemeManager    *theme.Manager
	ColorResolver   port.ColorSchemeResolver
	AdwaitaDetector *colorscheme.AdwaitaDetector
	Duration        time.Duration
}

// DeferredInitResult holds results from deferred initialization checks.
type DeferredInitResult struct {
	RuntimeErr error
	MediaErr   error
	SQLiteErr  error
	Duration   time.Duration
}

// ParallelInitInput holds the input for parallel initialization.
type ParallelInitInput struct {
	Ctx    context.Context
	Config *config.Config
}

// DeferredInitInput holds the input for deferred initialization.
type DeferredInitInput struct {
	Ctx    context.Context
	Config *config.Config
}

// RuntimeRequirementsError contains details about missing runtime dependencies.
type RuntimeRequirementsError struct {
	Checks []usecase.RuntimeDependencyStatus
}

func (*RuntimeRequirementsError) Error() string {
	return "runtime requirements not met"
}

// LogDetails logs detailed information about missing dependencies.
func (e *RuntimeRequirementsError) LogDetails(ctx context.Context) {
	log := logging.FromContext(ctx)
	for _, c := range e.Checks {
		if c.Installed {
			log.Error().
				Str("dependency", c.PkgConfigName).
				Str("have", c.Version).
				Str("need", c.RequiredVersion).
				Bool("ok", c.MeetsRequirement).
				Msg("runtime dependency")
		} else {
			log.Error().
				Str("dependency", c.PkgConfigName).
				Str("need", c.RequiredVersion).
				Msg("runtime dependency missing")
		}
	}
}

// RunParallelInit runs the essential parallel initialization phase.
// This includes directory resolution, color scheme resolver creation, and theme creation.
// Returns the first fatal error encountered, or nil with the results.
func RunParallelInit(input ParallelInitInput) (*ParallelInitResult, error) {
	var (
		dataDir, cacheDir string
		dirsErr           error
		wg                sync.WaitGroup
	)

	start := time.Now()

	// Create color scheme resolver with initial detectors.
	// AdwaitaDetector is created but NOT marked available yet (requires adw.Init()).
	configAdapter := colorscheme.NewConfigAdapter(input.Config)
	resolver := colorscheme.NewResolver(configAdapter)

	// Register detectors that are available before GTK init
	resolver.RegisterDetector(colorscheme.NewEnvDetector())
	resolver.RegisterDetector(colorscheme.NewGsettingsDetector())

	// Create adwaita detector (will be marked available after adw.Init())
	adwaitaDetector := colorscheme.NewAdwaitaDetector()
	resolver.RegisterDetector(adwaitaDetector)

	wg.Add(2)

	// Resolve directories
	go func() {
		defer wg.Done()
		dataDir, cacheDir, dirsErr = resolveWebKitDirs()
	}()

	// Theme manager (CPU-bound, no I/O)
	var themeManager *theme.Manager
	go func() {
		defer wg.Done()
		themeManager = theme.NewManager(input.Ctx, input.Config, resolver)
	}()

	wg.Wait()
	duration := time.Since(start)

	// Return first fatal error
	if dirsErr != nil {
		return nil, fmt.Errorf("resolve directories: %w", dirsErr)
	}

	return &ParallelInitResult{
		DataDir:         dataDir,
		CacheDir:        cacheDir,
		ThemeManager:    themeManager,
		ColorResolver:   resolver,
		AdwaitaDetector: adwaitaDetector,
		Duration:        duration,
	}, nil
}

// RunDeferredInit runs deferred initialization checks off the critical path.
// This includes SQLite WASM precompile, runtime requirements, and media checks.
func RunDeferredInit(input DeferredInitInput) DeferredInitResult {
	var (
		runtimeErr error
		mediaErr   error
		sqliteErr  error
		wg         sync.WaitGroup
	)

	start := time.Now()
	wg.Add(3)

	go func() {
		defer wg.Done()
		sqliteErr = sqlite3.Initialize()
	}()

	go func() {
		defer wg.Done()
		runtimeErr = CheckRuntimeRequirements(input.Ctx, input.Config)
	}()

	go func() {
		defer wg.Done()
		mediaErr = CheckMediaRequirements(input.Ctx, input.Config)
	}()

	wg.Wait()

	return DeferredInitResult{
		RuntimeErr: runtimeErr,
		MediaErr:   mediaErr,
		SQLiteErr:  sqliteErr,
		Duration:   time.Since(start),
	}
}

// CheckRuntimeRequirements verifies WebKitGTK and other runtime dependencies.
// Returns error if requirements are not met; caller should log details and exit.
func CheckRuntimeRequirements(ctx context.Context, cfg *config.Config) error {
	probe := deps.NewPkgConfigProbe()
	checkRuntimeUC := usecase.NewCheckRuntimeDependenciesUseCase(probe)
	runtimeOut, err := checkRuntimeUC.Execute(ctx, usecase.CheckRuntimeDependenciesInput{
		Prefix: cfg.Runtime.Prefix,
	})
	if err != nil {
		return fmt.Errorf("runtime check execution failed: %w", err)
	}
	if runtimeOut.OK {
		return nil
	}
	return &RuntimeRequirementsError{Checks: runtimeOut.Checks}
}

// CheckMediaRequirements verifies GStreamer and media codec availability.
// Returns error if media subsystem initialization fails.
func CheckMediaRequirements(ctx context.Context, cfg *config.Config) error {
	mediaDiagAdapter := media.New()
	checkMediaUC := usecase.NewCheckMediaUseCase(mediaDiagAdapter)
	if _, err := checkMediaUC.Execute(ctx, usecase.CheckMediaInput{
		ShowDiagnostics: cfg.Media.ShowDiagnosticsOnStartup,
	}); err != nil {
		return fmt.Errorf("media check failed: %w", err)
	}
	return nil
}

// resolveWebKitDirs resolves and creates the data and cache directories for WebKit.
func resolveWebKitDirs() (dataDir, cacheDir string, err error) {
	const cacheDirPerm = 0o755
	dataDir, err = config.GetDataDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve data directory: %w", err)
	}
	stateDir, err := config.GetStateDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve state directory: %w", err)
	}
	cacheDir = filepath.Join(stateDir, "webkit-cache")
	if mkErr := os.MkdirAll(cacheDir, cacheDirPerm); mkErr != nil {
		return "", "", fmt.Errorf("create cache directory %s: %w", cacheDir, mkErr)
	}
	return dataDir, cacheDir, nil
}

// OpenDatabase opens and initializes the SQLite database using XDG paths.
// Returns error if database cannot be opened or migrations fail.
func OpenDatabase(ctx context.Context) (*DatabaseResult, error) {
	dbPath, err := config.GetDatabaseFile()
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	db, err := sqlite.NewConnection(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("initialize database at %s: %w", dbPath, err)
	}
	return &DatabaseResult{
		DB: db,
		Cleanup: func() {
			_ = sqlite.Close(db)
		},
	}, nil
}

// CreateLazyDatabase creates a lazy database provider that defers initialization.
// The database is initialized on first access, allowing the UI to render faster.
//
// Currently unused: the application uses OpenDatabase with RunParallelDBWebKit for
// eager initialization. This function is kept for potential future use when lazy
// initialization past first paint becomes beneficial.
func CreateLazyDatabase() (*sqlite.LazyDB, error) {
	dbPath, err := config.GetDatabaseFile()
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	return sqlite.NewLazyDB(dbPath), nil
}

// ParallelDBWebKitResult holds results from parallel DB and WebKit initialization.
type ParallelDBWebKitResult struct {
	DB        *sql.DB
	DBCleanup func()
	Stack     WebKitStack
}

// ParallelDBWebKitInput holds inputs for parallel DB and WebKit initialization.
type ParallelDBWebKitInput struct {
	Ctx           context.Context
	Config        *config.Config
	DataDir       string // For WebKit context
	CacheDir      string // For WebKit cache
	ThemeManager  *theme.Manager
	ColorResolver port.ColorSchemeResolver
}

// Note: Database path is resolved via config.GetDatabaseFile() internally.

// dbInitResult holds the result of database initialization for channel communication.
type dbInitResult struct {
	db      *sql.DB
	cleanup func()
	err     error
}

// RunParallelDBWebKit initializes database in background while WebKit runs on main thread.
// Database init happens in goroutine (pure Go/WASM), WebKit must stay on main thread (GTK).
// This saves ~150ms by overlapping DB migrations with WebKit context creation.
func RunParallelDBWebKit(input ParallelDBWebKitInput) (*ParallelDBWebKitResult, error) {
	log := logging.FromContext(input.Ctx)

	dbCh := make(chan dbInitResult, 1)

	// Database initialization in background (WASM is thread-safe)
	go func() {
		dbResult, err := OpenDatabase(input.Ctx)
		if err != nil {
			dbCh <- dbInitResult{err: err}
			return
		}
		dbCh <- dbInitResult{
			db:      dbResult.DB,
			cleanup: dbResult.Cleanup,
		}
	}()

	// WebKit stack on main thread (GTK requirement)
	stack := BuildWebKitStack(WebKitStackInput{
		Ctx:           input.Ctx,
		Config:        input.Config,
		DataDir:       input.DataDir,
		CacheDir:      input.CacheDir,
		ThemeManager:  input.ThemeManager,
		ColorResolver: input.ColorResolver,
		Logger:        *log,
	})

	// Wait for database
	dbRes := <-dbCh

	if dbRes.err != nil {
		return nil, fmt.Errorf("database initialization: %w", dbRes.err)
	}

	return &ParallelDBWebKitResult{
		DB:        dbRes.db,
		DBCleanup: dbRes.cleanup,
		Stack:     stack,
	}, nil
}
