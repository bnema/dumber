package bootstrap

import (
	"context"
	"path/filepath"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/rs/zerolog"
)

// WebKitStackInput holds the input for BuildWebKitStack.
type WebKitStackInput struct {
	Ctx           context.Context
	Config        *config.Config
	DataDir       string
	CacheDir      string
	ThemeManager  *theme.Manager
	ColorResolver port.ColorSchemeResolver
	Logger        zerolog.Logger
}

type WebKitStack struct {
	Context       *webkit.WebKitContext
	Settings      *webkit.SettingsManager
	Injector      *webkit.ContentInjector
	MessageRouter *webkit.MessageRouter
	Pool          *webkit.WebViewPool
	FilterManager *filtering.Manager
}

func BuildWebKitStack(input WebKitStackInput) WebKitStack {
	ctx := input.Ctx
	cfg := input.Config
	dataDir := input.DataDir
	cacheDir := input.CacheDir
	themeManager := input.ThemeManager
	colorResolver := input.ColorResolver
	logger := input.Logger

	// Detect hardware for performance profile scaling
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

	// Resolve performance profile to get actual settings
	perfSettings := config.ResolvePerformanceProfile(&cfg.Performance, &hwInfo)
	logging.Trace().Mark("performance_profile")
	logger.Info().
		Str("profile", string(cfg.Performance.Profile)).
		Int("skia_cpu_threads", perfSettings.SkiaCPUPaintingThreads).
		Int("skia_gpu_threads", perfSettings.SkiaGPUPaintingThreads).
		Int("webview_pool_prewarm", perfSettings.WebViewPoolPrewarmCount).
		Msg("resolved performance profile")

	// Configure rendering environment (must happen before WebKit/GTK init)
	configureRenderingEnvironment(ctx, cfg, &perfSettings, logger)
	logging.Trace().Mark("render_env")

	// Build WebKitContext options with memory pressure settings
	wkOpts := port.WebKitContextOptions{
		DataDir:  dataDir,
		CacheDir: cacheDir,
	}

	// Configure web process memory pressure if any setting is configured
	if hasWebProcessMemoryConfig(&perfSettings) {
		wkOpts.WebProcessMemory = &port.MemoryPressureConfig{
			MemoryLimitMB:         perfSettings.WebProcessMemoryLimitMB,
			PollIntervalSec:       perfSettings.WebProcessMemoryPollIntervalSec,
			ConservativeThreshold: perfSettings.WebProcessMemoryConservativeThreshold,
			StrictThreshold:       perfSettings.WebProcessMemoryStrictThreshold,
		}
	}

	// Configure network process memory pressure if any setting is configured
	if hasNetworkProcessMemoryConfig(&perfSettings) {
		wkOpts.NetworkProcessMemory = &port.MemoryPressureConfig{
			MemoryLimitMB:         perfSettings.NetworkProcessMemoryLimitMB,
			PollIntervalSec:       perfSettings.NetworkProcessMemoryPollIntervalSec,
			ConservativeThreshold: perfSettings.NetworkProcessMemoryConservativeThreshold,
			StrictThreshold:       perfSettings.NetworkProcessMemoryStrictThreshold,
		}
	}

	wkCtx, err := webkit.NewWebKitContextWithOptions(ctx, wkOpts)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize WebKit context")
	}
	logging.Trace().Mark("webkit_context")

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
	} else {
		if err := filterManager.Initialize(ctx); err != nil {
			logger.Warn().Err(err).Msg("failed to initialize filters, will load async")
		}
	}
	logging.Trace().Mark("filter_manager")

	schemeHandler := webkit.NewDumbSchemeHandler(ctx)
	schemeHandler.SetAssets(assets.WebUIAssets)
	schemeHandler.RegisterWithContext(wkCtx)

	settings := webkit.NewSettingsManager(ctx, cfg)
	injector := webkit.NewContentInjector(colorResolver)

	// Set up auto-copy on selection config getter
	injector.SetAutoCopyConfigGetter(func() bool {
		return config.Get().Clipboard.AutoCopyOnSelection
	})

	prepareThemeUC := usecase.NewPrepareWebUIThemeUseCase(injector)
	themeCSSText := themeManager.GetWebUIThemeCSS()
	if err := prepareThemeUC.Execute(ctx, usecase.PrepareWebUIThemeInput{CSSVars: themeCSSText}); err != nil {
		logger.Warn().Err(err).Msg("failed to prepare WebUI theme CSS")
	}

	messageRouter := webkit.NewMessageRouter(ctx)
	logging.Trace().Mark("settings_manager")

	poolCfg := webkit.DefaultPoolConfig()
	// Override prewarm count from resolved profile settings
	if perfSettings.WebViewPoolPrewarmCount > 0 {
		poolCfg.PrewarmCount = perfSettings.WebViewPoolPrewarmCount
	}
	pool := webkit.NewWebViewPool(ctx, wkCtx, settings, poolCfg, injector, messageRouter)
	// Ensure prewarmed WebViews pick up the theme background color.
	bgR, bgG, bgB, bgA := themeManager.GetBackgroundRGBA()
	pool.SetBackgroundColor(bgR, bgG, bgB, bgA)

	if filterManager != nil {
		pool.SetFilterApplier(filterManager)
	}

	// Pre-create ONE WebView synchronously for instant first navigation.
	// This is the heaviest operation but happens before window is shown,
	// so users perceive it as part of the normal "loading" phase.
	if err := pool.PrewarmFirst(ctx); err != nil {
		logger.Warn().Err(err).Msg("failed to prewarm first webview, first tab may be slower")
	}
	logging.Trace().Mark("pool_prewarm_first")

	return WebKitStack{
		Context:       wkCtx,
		Settings:      settings,
		Injector:      injector,
		MessageRouter: messageRouter,
		Pool:          pool,
		FilterManager: filterManager,
	}
}

// hasWebProcessMemoryConfig returns true if any web process memory setting is configured.
func hasWebProcessMemoryConfig(p *config.ResolvedPerformanceSettings) bool {
	return p.WebProcessMemoryLimitMB > 0 ||
		p.WebProcessMemoryPollIntervalSec > 0 ||
		p.WebProcessMemoryConservativeThreshold > 0 ||
		p.WebProcessMemoryStrictThreshold > 0
}

// hasNetworkProcessMemoryConfig returns true if any network process memory setting is configured.
func hasNetworkProcessMemoryConfig(p *config.ResolvedPerformanceSettings) bool {
	return p.NetworkProcessMemoryLimitMB > 0 ||
		p.NetworkProcessMemoryPollIntervalSec > 0 ||
		p.NetworkProcessMemoryConservativeThreshold > 0 ||
		p.NetworkProcessMemoryStrictThreshold > 0
}

// configureRenderingEnvironment sets up the rendering environment before WebKit/GTK initialization.
// Environment variables must be set before GTK/WebKit/GStreamer initializes.
func configureRenderingEnvironment(
	ctx context.Context,
	cfg *config.Config,
	perfSettings *config.ResolvedPerformanceSettings,
	logger zerolog.Logger,
) {
	// Install GLib log handler FIRST if configured (must be before any GTK/GLib calls)
	if cfg.Logging.CaptureGTKLogs {
		enableDebug := cfg.Logging.Level == "debug" || cfg.Logging.Level == "trace"
		logging.InstallGLibLogHandler(ctx, logger, enableDebug)
	}

	renderEnv := env.NewManager()
	gpuVendor := renderEnv.DetectGPUVendor(ctx)
	renderSettings := port.RenderingEnvSettings{
		// GStreamer settings
		ForceVSync:          cfg.Media.ForceVSync,
		GLRenderingMode:     string(cfg.Media.GLRenderingMode),
		GStreamerDebugLevel: cfg.Media.GStreamerDebugLevel,

		// WebKit compositor settings
		DisableDMABufRenderer:  cfg.Rendering.DisableDMABufRenderer,
		ForceCompositingMode:   cfg.Rendering.ForceCompositingMode,
		DisableCompositingMode: cfg.Rendering.DisableCompositingMode,

		// GTK/GSK settings
		GSKRenderer:    string(cfg.Rendering.GSKRenderer),
		DisableMipmaps: cfg.Rendering.DisableMipmaps,
		PreferGL:       cfg.Rendering.PreferGL,

		// Debug settings
		ShowFPS:      cfg.Rendering.ShowFPS,
		SampleMemory: cfg.Rendering.SampleMemory,
		DebugFrames:  cfg.Rendering.DebugFrames,

		// Skia rendering thread settings (from resolved profile)
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
