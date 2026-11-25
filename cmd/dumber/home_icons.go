package main

import (
	"context"
	"path/filepath"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/usecase"
	"bnema.dev/dumber/internal/infrastructure/config"
	infrafavicon "bnema.dev/dumber/internal/infrastructure/favicon"
	"bnema.dev/dumber/internal/infrastructure/homeconfig"
	"bnema.dev/dumber/internal/logging"
)

// wireHomeIcons serves home service icons to the home systemview through
// the engine's /api/home-icon endpoint.
func wireHomeIcons(ctx context.Context, engine port.Engine, configDir string) {
	setter, ok := engine.(port.HomeIconResolverSetter)
	if !ok {
		return
	}
	var cache port.HomeIconCache
	if faviconCacheDir, err := config.GetFaviconCacheDir(); err != nil || faviconCacheDir == "" {
		logging.FromContext(ctx).Warn().Err(err).Msg("home icon cache disabled: no favicon cache dir")
	} else {
		cache = infrafavicon.NewHomeIconCache(filepath.Join(faviconCacheDir, "home"))
	}
	setter.SetHomeIconResolver(usecase.NewHomeIconUseCase(usecase.HomeIconDeps{
		Source:    infrafavicon.NewHomeIconSource(),
		Converter: infrafavicon.NewImageConverter(),
		Cache:     cache,
		Config:    homeconfig.New(configDir),
		ConfigDir: configDir,
	}))
}
