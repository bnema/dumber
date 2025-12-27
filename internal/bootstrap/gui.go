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
)

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
	wg.Add(4)

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

// OpenDatabase opens and initializes the SQLite database.
// Returns error if database cannot be opened or migrations fail.
func OpenDatabase(ctx context.Context, dataDir string) (*sql.DB, func(), error) {
	dbPath := filepath.Join(dataDir, "dumber.db")
	db, err := sqlite.NewConnection(ctx, dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize database at %s: %w", dbPath, err)
	}
	cleanup := func() {
		_ = sqlite.Close(db)
	}
	return db, cleanup, nil
}
