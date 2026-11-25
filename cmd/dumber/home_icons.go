package main

import (
	"context"
	"path/filepath"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/usecase"
	"bnema.dev/dumber/internal/infrastructure/config"
	infrafavicon "bnema.dev/dumber/internal/infrastructure/favicon"
	"bnema.dev/dumber/internal/logging"
)

// wireHomeIcons serves home service icons to the home systemview through
// the engine's /api/home-icon endpoint and returns the use case, which also
// computes the page's icon versions.
func wireHomeIcons(
	ctx context.Context,
	engine port.Engine,
	homeConfig port.HomeConfigSource,
	configDir string,
) *usecase.HomeIconUseCase {
	var cache port.HomeIconCache
	if faviconCacheDir, err := config.GetFaviconCacheDir(); err != nil || faviconCacheDir == "" {
		logging.FromContext(ctx).Warn().Err(err).Msg("home icon cache disabled: no favicon cache dir")
	} else {
		cache = infrafavicon.NewHomeIconCache(filepath.Join(faviconCacheDir, "home"))
	}
	icons := usecase.NewHomeIconUseCase(usecase.HomeIconDeps{
		Source:    infrafavicon.NewHomeIconSource(),
		Converter: infrafavicon.NewImageConverter(),
		Cache:     cache,
		Config:    homeConfig,
		ConfigDir: configDir,
	})
	if setter, ok := engine.(port.HomeIconResolverSetter); ok {
		setter.SetHomeIconResolver(icons)
	}
	return icons
}
