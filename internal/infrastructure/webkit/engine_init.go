package webkit

import (
	"context"
	"path/filepath"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/rs/zerolog"
)

// NewEngine creates and initializes all WebKit engine components.
// It absorbs the initialization logic previously found in bootstrap.BuildWebKitStack.
func NewEngine(
	ctx context.Context,
	cfg *config.Config,
	opts port.EngineOptions,
	wkCfg WebKitEngineConfig,
	themeManager *theme.Manager,
	colorResolver port.ColorSchemeResolver,
	logger zerolog.Logger,
) (*Engine, error) {
	// --- Hardware survey and performance profile resolution ---
	perfSettings := engineSurveyHardwareAndResolveProfile(ctx, cfg, logger)

	// --- Configure rendering environment (must be before GTK/WebKit init) ---
	engineConfigureRenderingEnvironment(ctx, cfg, wkCfg, &perfSettings, logger)
	logging.Trace().Mark("render_env")

	// --- Build webKitContextOptions from opts + wkCfg + perfSettings ---
	wkOpts := engineBuildContextOptions(opts, wkCfg, &perfSettings)
	logger.Info().
		Str("cookie_policy", string(wkOpts.CookiePolicy)).
		Bool("itp_enabled", wkOpts.ITPEnabled).
		Msg("webkit privacy configuration")

	wkCtx, err := NewWebKitContextWithOptions(ctx, wkOpts)
	if err != nil {
		return nil, err
	}
	logging.Trace().Mark("webkit_context")

	// --- Filter manager ---
	filterManager := engineInitFilterManager(ctx, cfg, opts.DataDir, logger)

	// --- Scheme handler ---
	schemeHandler := NewDumbSchemeHandler(ctx)
	schemeHandler.SetAssets(assets.WebUIAssets)
	schemeHandler.RegisterWithContext(wkCtx)

	// --- Settings, injector, message router ---
	settings := NewSettingsManager(ctx, cfg)
	injector := NewContentInjector(colorResolver)

	injector.SetAutoCopyConfigGetter(func() bool {
		return config.Get().Clipboard.AutoCopyOnSelection
	})

	prepareThemeUC := usecase.NewPrepareWebUIThemeUseCase(injector)
	themeCSSText := themeManager.GetWebUIThemeCSS()
	if err := prepareThemeUC.Execute(ctx, usecase.PrepareWebUIThemeInput{CSSVars: themeCSSText}); err != nil {
		logger.Warn().Err(err).Msg("failed to prepare WebUI theme CSS")
	}

	messageRouter := NewMessageRouter(ctx)
	logging.Trace().Mark("settings_manager")

	// --- WebView pool ---
	poolCfg := DefaultPoolConfig()
	if perfSettings.WebViewPoolPrewarmCount > 0 {
		poolCfg.PrewarmCount = perfSettings.WebViewPoolPrewarmCount
	}
	pool := NewWebViewPool(ctx, wkCtx, settings, poolCfg, injector, messageRouter)

	bgR, bgG, bgB, bgA := themeManager.GetBackgroundRGBA()
	pool.SetBackgroundColor(bgR, bgG, bgB, bgA)

	if filterManager != nil {
		pool.SetFilterApplier(filterManager)
	}

	if gdk.DisplayGetDefault() != nil {
		if err := pool.PrewarmFirst(ctx); err != nil {
			logger.Warn().Err(err).Msg("failed to prewarm first webview, first tab may be slower")
		}
	} else {
		logger.Warn().Msg("skipping first webview prewarm: no GDK display available yet")
	}
	logging.Trace().Mark("pool_prewarm_first")

	// --- WebView factory ---
	factory := NewWebViewFactory(wkCtx, settings, pool, injector, messageRouter)
	factory.SetBackgroundColor(bgR, bgG, bgB, bgA)
	if filterManager != nil {
		factory.SetFilterApplier(filterManager)
	}

	// --- Assemble Engine ---
	engine := &Engine{
		ctx:           ctx,
		wkCtx:         wkCtx,
		settings:      settings,
		injector:      injector,
		messageRouter: messageRouter,
		pool:          pool,
		factory:       factory,
		filterManager: filterManager,
		schemeHandler: schemeHandler,
		schemePath:    "dumb://",
		logger:        logger,
	}

	return engine, nil
}

// engineSurveyHardwareAndResolveProfile surveys hardware and resolves the performance profile.
func engineSurveyHardwareAndResolveProfile(
	ctx context.Context, cfg *config.Config, logger zerolog.Logger,
) config.ResolvedPerformanceSettings {
	hwSurveyor := env.NewHardwareSurveyor()
	hwInfo := hwSurveyor.Survey(ctx)
	logging.Trace().Mark("hardware_survey")
	logger.Info().
		Int("cpu_cores", hwInfo.CPUCores).
		Int("cpu_threads", hwInfo.CPUThreads).
		Uint64("ram_mb", hwInfo.TotalRAM/(1024*1024)).
		Str("gpu_vendor", string(hwInfo.GPUVendor)).
		Uint64("vram_mb", hwInfo.VRAM/(1024*1024)).
		Msg("hardware survey completed")

	perfCfg := config.PerformanceConfigFromEngine(&cfg.Engine)
	perfSettings := config.ResolvePerformanceProfile(&perfCfg, &hwInfo)
	logging.Trace().Mark("performance_profile")
	logger.Info().
		Str("profile", string(cfg.Engine.Profile)).
		Int("skia_cpu_threads", perfSettings.SkiaCPUPaintingThreads).
		Int("skia_gpu_threads", perfSettings.SkiaGPUPaintingThreads).
		Int("webview_pool_prewarm", perfSettings.WebViewPoolPrewarmCount).
		Msg("resolved performance profile")

	return perfSettings
}

// engineConfigureRenderingEnvironment sets up the rendering environment.
// Environment variables must be set before GTK/WebKit/GStreamer initializes.
func engineConfigureRenderingEnvironment(
	ctx context.Context,
	cfg *config.Config,
	wkCfg WebKitEngineConfig,
	perfSettings *config.ResolvedPerformanceSettings,
	logger zerolog.Logger,
) {
	// Install GLib log handler FIRST if configured (must be before any GTK/GLib calls).
	if cfg.Logging.CaptureGTKLogs {
		enableDebug := cfg.Logging.Level == "debug" || cfg.Logging.Level == "trace"
		logging.InstallGLibLogHandler(ctx, logger, enableDebug)
	}

	renderEnv := env.NewManager()
	gpuVendor := renderEnv.DetectGPUVendor(ctx)
	renderSettings := env.RenderingSettings{
		// GStreamer settings
		ForceVSync:          wkCfg.ForceVSync,
		GLRenderingMode:     wkCfg.GLRenderingMode,
		GStreamerDebugLevel: wkCfg.GStreamerDebugLevel,
		// WebKit compositor settings
		DisableDMABufRenderer:  wkCfg.DisableDMABufRenderer,
		ForceCompositingMode:   wkCfg.ForceCompositingMode,
		DisableCompositingMode: wkCfg.DisableCompositingMode,
		// GTK/GSK settings
		GSKRenderer:    wkCfg.GSKRenderer,
		DisableMipmaps: wkCfg.DisableMipmaps,
		PreferGL:       wkCfg.PreferGL,
		// Debug settings
		ShowFPS:      wkCfg.ShowFPS,
		SampleMemory: wkCfg.SampleMemory,
		DebugFrames:  wkCfg.DebugFrames,
		// Skia rendering thread settings (from resolved performance profile)
		SkiaCPUPaintingThreads: perfSettings.SkiaCPUPaintingThreads,
		SkiaGPUPaintingThreads: perfSettings.SkiaGPUPaintingThreads,
		SkiaEnableCPURendering: perfSettings.SkiaEnableCPURendering,
	}
	if err := renderEnv.ApplyEnvironment(ctx, renderSettings); err != nil {
		logger.Warn().Err(err).Msg("failed to apply rendering environment")
	}
	logger.Info().
		Str("gpu", string(gpuVendor)).
		Interface("vars", renderEnv.GetAppliedVars()).
		Msg("rendering environment configured")
}

// engineBuildContextOptions builds webKitContextOptions from EngineOptions, wkCfg and perfSettings.
func engineBuildContextOptions(
	opts port.EngineOptions,
	wkCfg WebKitEngineConfig,
	perfSettings *config.ResolvedPerformanceSettings,
) webKitContextOptions {
	cp := opts.CookiePolicy // empty preserves runtime default per port contract

	wkOpts := webKitContextOptions{
		DataDir:      opts.DataDir,
		CacheDir:     opts.CacheDir,
		CookiePolicy: cp,
		ITPEnabled:   wkCfg.ITPEnabled,
	}

	if opts.WebProcessMemory != nil {
		wkOpts.WebProcessMemory = opts.WebProcessMemory
	} else if engineHasWebProcessMemoryConfig(perfSettings) {
		wkOpts.WebProcessMemory = &port.MemoryPressureConfig{
			MemoryLimitMB:         perfSettings.WebProcessMemoryLimitMB,
			PollIntervalSec:       perfSettings.WebProcessMemoryPollIntervalSec,
			ConservativeThreshold: perfSettings.WebProcessMemoryConservativeThreshold,
			StrictThreshold:       perfSettings.WebProcessMemoryStrictThreshold,
		}
	}

	if opts.NetworkProcessMemory != nil {
		wkOpts.NetworkProcessMemory = opts.NetworkProcessMemory
	} else if engineHasNetworkProcessMemoryConfig(perfSettings) {
		wkOpts.NetworkProcessMemory = &port.MemoryPressureConfig{
			MemoryLimitMB:         perfSettings.NetworkProcessMemoryLimitMB,
			PollIntervalSec:       perfSettings.NetworkProcessMemoryPollIntervalSec,
			ConservativeThreshold: perfSettings.NetworkProcessMemoryConservativeThreshold,
			StrictThreshold:       perfSettings.NetworkProcessMemoryStrictThreshold,
		}
	}

	return wkOpts
}

// engineInitFilterManager creates and initializes the content filter manager.
func engineInitFilterManager(ctx context.Context, cfg *config.Config, dataDir string, logger zerolog.Logger) *filtering.Manager {
	filterStoreDir := filepath.Join(dataDir, "filters", "store")
	filterJSONDir := filepath.Join(dataDir, "filters", "json")
	filterManager, err := filtering.NewManager(filtering.ManagerConfig{
		StoreDir:   filterStoreDir,
		JSONDir:    filterJSONDir,
		Enabled:    cfg.ContentFiltering.Enabled,
		AutoUpdate: cfg.ContentFiltering.AutoUpdate,
	})
	if err != nil {
		logger.Warn().Err(err).Msg("failed to create filter manager, continuing without content filtering")
		return nil
	}
	if err := filterManager.Initialize(ctx); err != nil {
		logger.Warn().Err(err).Msg("failed to initialize filters, will load async")
	}
	logging.Trace().Mark("filter_manager")
	return filterManager
}

// engineHasWebProcessMemoryConfig returns true if any web process memory setting is configured.
func engineHasWebProcessMemoryConfig(p *config.ResolvedPerformanceSettings) bool {
	return p.WebProcessMemoryLimitMB > 0 ||
		p.WebProcessMemoryPollIntervalSec > 0 ||
		p.WebProcessMemoryConservativeThreshold > 0 ||
		p.WebProcessMemoryStrictThreshold > 0
}

// engineHasNetworkProcessMemoryConfig returns true if any network process memory setting is configured.
func engineHasNetworkProcessMemoryConfig(p *config.ResolvedPerformanceSettings) bool {
	return p.NetworkProcessMemoryLimitMB > 0 ||
		p.NetworkProcessMemoryPollIntervalSec > 0 ||
		p.NetworkProcessMemoryConservativeThreshold > 0 ||
		p.NetworkProcessMemoryStrictThreshold > 0
}
