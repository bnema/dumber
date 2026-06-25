// Package bootstrap provides initialization utilities for the browser.
package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/colorscheme"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/deps"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/infrastructure/externaltheme/noctalia"
	"github.com/bnema/dumber/internal/infrastructure/media"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/theme"
)

// DatabaseResult holds database connection and cleanup function.
type DatabaseResult struct {
	DB      *sql.DB
	Cleanup func()
}

// ParallelInitResult holds the results of parallel initialization phase.
type ParallelInitResult struct {
	RuntimeProfile       runtimeprofile.Profile
	ThemeManager         *theme.Manager
	ResolvedTheme        entity.ResolvedTheme
	ResolveThemeUC       *usecase.ResolveThemeUseCase
	ExternalThemeSource  port.ConfigurableExternalThemeSource
	ExternalThemeWatcher port.ExternalThemeWatcher
	ColorResolver        port.ColorSchemeResolver
	AdwaitaDetector      *colorscheme.AdwaitaDetector
	Duration             time.Duration
}

// DeferredInitResult holds results from deferred initialization checks.
type DeferredInitResult struct {
	RuntimeErr error
	MediaErr   error
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
		dirsErr  error
		themeErr error
		wg       sync.WaitGroup
	)

	profile, err := ResolveRuntimeProfile(input.Config)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime profile: %w", err)
	}

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
		dirsErr = resolveWebKitDirs(profile)
	}()

	// Theme manager. External theme file reads, when enabled, are non-fatal
	// and reported as theme warnings by the usecase.
	externalThemeSource := noctalia.NewFileSourceFromConfig(input.Config.Appearance.ExternalTheme)
	resolveThemeUC := usecase.NewResolveThemeUseCase(externalThemeSource)
	var themeManager *theme.Manager
	var resolvedTheme entity.ResolvedTheme
	go func() {
		defer wg.Done()
		preference := resolver.Resolve()
		resolved, err := resolveThemeUC.Execute(input.Ctx, usecase.ResolveThemeInputFromConfig(
			&input.Config.Appearance,
			input.Config.DefaultUIScale,
			&input.Config.Workspace.Styling,
			preference,
		))
		if err != nil {
			themeErr = err
			return
		}
		LogResolvedTheme(input.Ctx, resolved.Theme)
		resolvedTheme = resolved.Theme
		themeManager = theme.NewManager(input.Ctx, resolved.Theme)
	}()

	wg.Wait()
	duration := time.Since(start)

	// Return first fatal error
	if dirsErr != nil {
		return nil, fmt.Errorf("resolve directories: %w", dirsErr)
	}
	if themeErr != nil {
		return nil, fmt.Errorf("resolve theme: %w", themeErr)
	}

	return &ParallelInitResult{
		RuntimeProfile:       profile,
		ThemeManager:         themeManager,
		ResolvedTheme:        resolvedTheme,
		ResolveThemeUC:       resolveThemeUC,
		ExternalThemeSource:  externalThemeSource,
		ExternalThemeWatcher: noctalia.NewFileWatcher(),
		ColorResolver:        resolver,
		AdwaitaDetector:      adwaitaDetector,
		Duration:             duration,
	}, nil
}

// LogResolvedTheme writes a compact theme-source summary and non-fatal warnings.
func LogResolvedTheme(ctx context.Context, resolved entity.ResolvedTheme) {
	log := logging.FromContext(ctx)
	for _, warning := range resolved.Warnings {
		log.Warn().Str("field", warning.Field).Msg(warning.Message)
	}
	event := log.Info().
		Str("theme_source", string(resolved.ThemeSource.Kind)).
		Str("color_scheme_source", resolved.ColorSchemeSource).
		Bool("prefers_dark", resolved.PrefersDark)
	if resolved.ThemeSource.Provider != "" {
		event = event.Str("provider", resolved.ThemeSource.Provider)
	}
	if resolved.ThemeSource.LastGood {
		event = event.Bool("last_good", true)
	}
	event.Msg("theme resolved")
}

// RunDeferredInit runs deferred initialization checks off the critical path.
// This includes runtime requirements and media checks.
func RunDeferredInit(input DeferredInitInput) DeferredInitResult {
	var (
		runtimeErr error
		mediaErr   error
		wg         sync.WaitGroup
	)

	start := time.Now()
	wg.Add(2)

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
		Duration:   time.Since(start),
	}
}

// CheckRuntimeRequirements verifies GUI runtime dependencies currently covered by runtime checks.
// Returns error if requirements are not met; caller should log details and exit.
// Note: When running in a Flatpak sandbox, runtime checks are skipped because
// the Flatpak runtime provides all required libraries.
func CheckRuntimeRequirements(ctx context.Context, cfg *config.Config) error {
	// Skip pkg-config checks in Flatpak - the runtime provides all dependencies
	if env.IsFlatpak() {
		log := logging.FromContext(ctx)
		log.Debug().Msg("running in Flatpak sandbox, skipping runtime dependency checks")
		return nil
	}

	probe := deps.NewPkgConfigProbe()
	checkRuntimeUC := usecase.NewCheckRuntimeDependenciesUseCase(probe)
	runtimeOut, err := checkRuntimeUC.Execute(ctx, usecase.CheckRuntimeDependenciesInput{
		Prefix: cfg.Engine.WebKit.Prefix,
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
func resolveWebKitDirs(profile runtimeprofile.Profile) error {
	const dirPerm = 0o755
	dataDir := profile.WebKitDataDir()
	cacheDir := profile.WebKitCacheDir()
	if mkErr := os.MkdirAll(dataDir, dirPerm); mkErr != nil {
		return fmt.Errorf("create data directory %s: %w", dataDir, mkErr)
	}
	if mkErr := os.MkdirAll(cacheDir, dirPerm); mkErr != nil {
		return fmt.Errorf("create cache directory %s: %w", cacheDir, mkErr)
	}
	return nil
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
// Currently unused: the application uses OpenDatabase with RunParallelDBEngine for
// eager initialization. This function is kept for potential future use when lazy
// initialization past first paint becomes beneficial.
func CreateLazyDatabase() (*sqlite.LazyDB, error) {
	dbPath, err := config.GetDatabaseFile()
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	return sqlite.NewLazyDB(dbPath), nil
}

// ParallelDBEngineResult holds results from parallel DB and engine initialization.
type ParallelDBEngineResult struct {
	DB        *sql.DB
	DBCleanup func()
	Engine    port.Engine
}

// ParallelDBEngineInput holds inputs for parallel DB and engine initialization.
type ParallelDBEngineInput struct {
	Ctx                 context.Context
	Config              *config.Config
	RuntimeProfile      runtimeprofile.Profile
	ThemeManager        *theme.Manager
	ExternalThemeSource port.ExternalThemeSource
	ColorResolver       port.ColorSchemeResolver
}

// Note: Database path is resolved via config.GetDatabaseFile() internally.

// dbInitResult holds the result of database initialization for channel communication.
type dbInitResult struct {
	db      *sql.DB
	cleanup func()
	err     error
}

// RunParallelDBEngine initializes database in background while the engine runs on main thread.
// Database init happens in goroutine (pure Go/WASM), engine must stay on main thread (GTK).
// This saves ~150ms by overlapping DB migrations with engine context creation.
func RunParallelDBEngine(input ParallelDBEngineInput) (*ParallelDBEngineResult, error) {
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

	// Engine on main thread (GTK requirement)
	engine, err := BuildEngine(EngineInput{
		Ctx:                 input.Ctx,
		Config:              input.Config,
		RuntimeProfile:      input.RuntimeProfile,
		ThemeManager:        input.ThemeManager,
		ExternalThemeSource: input.ExternalThemeSource,
		ColorResolver:       input.ColorResolver,
		Logger:              *log,
	})
	if err != nil {
		dbRes := <-dbCh
		if dbRes.err == nil && dbRes.cleanup != nil {
			dbRes.cleanup()
		}
		return nil, fmt.Errorf("engine initialization: %w", err)
	}

	// Wait for database
	dbRes := <-dbCh

	if dbRes.err != nil {
		return nil, fmt.Errorf("database initialization: %w", dbRes.err)
	}

	return &ParallelDBEngineResult{
		DB:        dbRes.db,
		DBCleanup: dbRes.cleanup,
		Engine:    engine,
	}, nil
}
