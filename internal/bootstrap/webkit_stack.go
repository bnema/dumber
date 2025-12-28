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
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/rs/zerolog"
)

type WebKitStack struct {
	Context       *webkit.WebKitContext
	Settings      *webkit.SettingsManager
	Injector      *webkit.ContentInjector
	MessageRouter *webkit.MessageRouter
	Pool          *webkit.WebViewPool
	FilterManager *filtering.Manager
}

func BuildWebKitStack(
	ctx context.Context,
	cfg *config.Config,
	dataDir string,
	cacheDir string,
	themeManager *theme.Manager,
	logger zerolog.Logger,
) WebKitStack {
	// CRITICAL: Configure rendering environment BEFORE WebKit/GTK initialization.
	// Environment variables must be set before GTK/WebKit/GStreamer initializes.
	renderEnv := env.NewManager()
	gpuVendor := renderEnv.DetectGPUVendor(ctx)
	renderSettings := port.RenderingEnvSettings{
		// GStreamer settings
		ForceVSync:          cfg.Media.ForceVSync,
		GLRenderingMode:     string(cfg.Media.GLRenderingMode),
		GStreamerDebugLevel: cfg.Media.GStreamerDebugLevel,
		VideoBufferSizeMB:   cfg.Media.VideoBufferSizeMB,
		QueueBufferTimeSec:  cfg.Media.QueueBufferTimeSec,

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
	}
	if err := renderEnv.ApplyEnvironment(ctx, renderSettings); err != nil {
		logger.Warn().Err(err).Msg("failed to apply rendering environment")
	}
	logger.Info().
		Str("gpu", string(gpuVendor)).
		Interface("vars", renderEnv.GetAppliedVars()).
		Msg("rendering environment configured")

	wkCtx, err := webkit.NewWebKitContext(ctx, dataDir, cacheDir)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize WebKit context")
	}

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

	schemeHandler := webkit.NewDumbSchemeHandler(ctx)
	schemeHandler.SetAssets(assets.WebUIAssets)
	schemeHandler.RegisterWithContext(wkCtx)

	settings := webkit.NewSettingsManager(ctx, cfg)
	injector := webkit.NewContentInjector(themeManager.PrefersDark())

	// Set background color on injector for early CSS injection (prevents white flash)
	injector.SetBackgroundColor(themeManager.GetCurrentPalette().Background)

	prepareThemeUC := usecase.NewPrepareWebUIThemeUseCase(injector)
	themeCSSText := themeManager.GetWebUIThemeCSS()
	if err := prepareThemeUC.Execute(ctx, usecase.PrepareWebUIThemeInput{CSSVars: themeCSSText}); err != nil {
		logger.Warn().Err(err).Msg("failed to prepare WebUI theme CSS")
	}

	messageRouter := webkit.NewMessageRouter(ctx)
	poolCfg := webkit.DefaultPoolConfig()
	// Override prewarm count from config if set
	if cfg.Performance.WebViewPoolPrewarmCount > 0 {
		poolCfg.PrewarmCount = cfg.Performance.WebViewPoolPrewarmCount
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

	return WebKitStack{
		Context:       wkCtx,
		Settings:      settings,
		Injector:      injector,
		MessageRouter: messageRouter,
		Pool:          pool,
		FilterManager: filterManager,
	}
}
