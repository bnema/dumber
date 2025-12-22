package bootstrap

import (
	"context"
	"path/filepath"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/filtering"
	"github.com/bnema/dumber/internal/infrastructure/media"
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
	// CRITICAL: Configure GStreamer environment BEFORE WebKit/GTK initialization.
	// Environment variables must be set before GStreamer initializes its pipelines.
	gstEnv := media.NewEnvManager()
	gpuVendor := gstEnv.DetectGPUVendor(ctx)
	gstSettings := port.GStreamerEnvSettings{
		ForceVSync:          cfg.Media.ForceVSync,
		GLRenderingMode:     string(cfg.Media.GLRenderingMode),
		GStreamerDebugLevel: cfg.Media.GStreamerDebugLevel,
		VideoBufferSizeMB:   cfg.Media.VideoBufferSizeMB,
		QueueBufferTimeSec:  cfg.Media.QueueBufferTimeSec,
	}
	if err := gstEnv.ApplyEnvironment(ctx, gstSettings); err != nil {
		logger.Warn().Err(err).Msg("failed to apply gstreamer environment")
	}
	logger.Info().
		Str("gpu", string(gpuVendor)).
		Interface("vars", gstEnv.GetAppliedVars()).
		Msg("gstreamer environment configured")

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

	prepareThemeUC := usecase.NewPrepareWebUIThemeUseCase(injector)
	themeCSSText := themeManager.GetWebUIThemeCSS()
	if err := prepareThemeUC.Execute(ctx, usecase.PrepareWebUIThemeInput{CSSVars: themeCSSText}); err != nil {
		logger.Warn().Err(err).Msg("failed to prepare WebUI theme CSS")
	}

	messageRouter := webkit.NewMessageRouter(ctx)
	poolCfg := webkit.DefaultPoolConfig()
	pool := webkit.NewWebViewPool(ctx, wkCtx, settings, poolCfg, injector, messageRouter)

	if filterManager != nil {
		pool.SetFilterApplier(filterManager)
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
