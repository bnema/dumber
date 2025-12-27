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

	"github.com/bnema/dumber/internal/application/usecase"
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
	DataDir      string
	CacheDir     string
	ThemeManager *theme.Manager
	Duration     time.Duration
}

// ParallelInitInput holds the input for parallel initialization.
type ParallelInitInput struct {
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

// RunParallelInit runs the parallel initialization phase.
// This includes directory resolution, runtime/media checks, and theme creation.
// Returns the first fatal error encountered, or nil with the results.
func RunParallelInit(input ParallelInitInput) (*ParallelInitResult, error) {
	var (
		dataDir, cacheDir string
		themeManager      *theme.Manager
		dirsErr           error
		runtimeErr        error
		mediaErr          error
		wg                sync.WaitGroup
	)

	start := time.Now()
	wg.Add(5)

	// Pre-initialize SQLite WASM runtime (expensive compilation)
	// This runs concurrently with other checks so it's hidden from critical path.
	go func() {
		defer wg.Done()
		_ = sqlite3.Initialize()
	}()

	// Resolve directories
	go func() {
		defer wg.Done()
		dataDir, cacheDir, dirsErr = resolveWebKitDirs()
	}()

	// Runtime checks (pkg-config subprocess)
	go func() {
		defer wg.Done()
		runtimeErr = CheckRuntimeRequirements(input.Ctx, input.Config)
	}()

	// Media checks (GStreamer subprocess)
	go func() {
		defer wg.Done()
		mediaErr = CheckMediaRequirements(input.Ctx, input.Config)
	}()

	// Theme manager (CPU-bound, no I/O)
	go func() {
		defer wg.Done()
		themeManager = theme.NewManager(input.Ctx, input.Config)
	}()

	wg.Wait()
	duration := time.Since(start)

	// Return first fatal error
	switch {
	case dirsErr != nil:
		return nil, fmt.Errorf("resolve directories: %w", dirsErr)
	case runtimeErr != nil:
		return nil, runtimeErr
	case mediaErr != nil:
		return nil, fmt.Errorf("media check: %w", mediaErr)
	}

	return &ParallelInitResult{
		DataDir:      dataDir,
		CacheDir:     cacheDir,
		ThemeManager: themeManager,
		Duration:     duration,
	}, nil
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
	Ctx          context.Context
	Config       *config.Config
	DataDir      string // For WebKit context
	CacheDir     string // For WebKit cache
	ThemeManager *theme.Manager
}

// Note: Database path is resolved via config.GetDatabaseFile() internally.

// RunParallelDBWebKit initializes database in background while WebKit runs on main thread.
// Database init happens in goroutine (pure Go/WASM), WebKit must stay on main thread (GTK).
// This saves ~150ms by overlapping DB migrations with WebKit context creation.
func RunParallelDBWebKit(input ParallelDBWebKitInput) (*ParallelDBWebKitResult, error) {
	log := logging.FromContext(input.Ctx)

	var (
		db        *sql.DB
		dbCleanup func()
		dbErr     error
		dbDone    = make(chan struct{})
	)

	// Database initialization in background (WASM is thread-safe)
	go func() {
		defer close(dbDone)
		dbResult, err := OpenDatabase(input.Ctx)
		if err != nil {
			dbErr = err
			return
		}
		db = dbResult.DB
		dbCleanup = dbResult.Cleanup
	}()

	// WebKit stack on main thread (GTK requirement)
	stack := BuildWebKitStack(input.Ctx, input.Config, input.DataDir, input.CacheDir, input.ThemeManager, *log)

	// Wait for database
	<-dbDone

	if dbErr != nil {
		return nil, fmt.Errorf("database initialization: %w", dbErr)
	}

	return &ParallelDBWebKitResult{
		DB:        db,
		DBCleanup: dbCleanup,
		Stack:     stack,
	}, nil
}
