package bootstrap

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/infrastructure/webkit/handlers"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/rs/zerolog"
)

// EngineInput holds the input for BuildEngine.
type EngineInput struct {
	Ctx           context.Context
	Config        *config.Config
	DataDir       string
	CacheDir      string
	ThemeManager  *theme.Manager
	ColorResolver port.ColorSchemeResolver
	Logger        zerolog.Logger
}

// BuildEngine constructs a port.Engine for the engine type specified in cfg.Engine.Type.
// Currently only "webkit" is supported; "cef" and other types return an error.
func BuildEngine(input EngineInput) (port.Engine, error) {
	cfg := input.Config
	engineType := cfg.Engine.Type
	if engineType == "" {
		engineType = "webkit"
	}
	switch engineType {
	case "webkit":
		opts := port.EngineOptions{
			DataDir:      input.DataDir,
			CacheDir:     input.CacheDir,
			CookiePolicy: port.CookiePolicy(cfg.Engine.CookiePolicy),
		}
		wkCfg := webkit.EngineConfigFromConfig(cfg.Engine.WebKit)
		return webkit.NewEngine(
			input.Ctx, cfg, opts, wkCfg,
			input.ThemeManager, input.ColorResolver, input.Logger,
			func(ctx context.Context, router *webkit.MessageRouter, deps port.HandlerDependencies) error {
				var historyUC *usecase.SearchHistoryUseCase
				if deps.HistoryUC != nil {
					historyUC, _ = deps.HistoryUC.(*usecase.SearchHistoryUseCase)
				}
				var favoritesUC *usecase.ManageFavoritesUseCase
				if deps.FavoritesUC != nil {
					favoritesUC, _ = deps.FavoritesUC.(*usecase.ManageFavoritesUseCase)
				}
				var configGetter func() *config.Config
				if deps.ConfigGetter != nil {
					configGetter = func() *config.Config {
						raw := deps.ConfigGetter()
						if raw == nil {
							return nil
						}
						cfg, _ := raw.(*config.Config)
						return cfg
					}
				}
				return handlers.RegisterAll(ctx, router, handlers.Config{
					HistoryUC:         historyUC,
					FavoritesUC:       favoritesUC,
					Clipboard:         deps.Clipboard,
					ConfigGetter:      configGetter,
					OnClipboardCopied: deps.OnClipboardCopied,
				})
			},
			func(ctx context.Context, router *webkit.MessageRouter, handler any) error {
				accentHandler, ok := handler.(handlers.AccentKeyHandler)
				if !ok {
					return fmt.Errorf("handler does not implement AccentKeyHandler")
				}
				return handlers.RegisterAccentHandlers(ctx, router, accentHandler)
			},
		)
	case "cef":
		return nil, fmt.Errorf("CEF engine not yet implemented")
	default:
		return nil, fmt.Errorf("unknown engine type: %q", engineType)
	}
}
