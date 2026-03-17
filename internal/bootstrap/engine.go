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

		// Pre-build keybindings handler for handler registration.
		keybindingsHandler, err := buildKeybindingsHandler()
		if err != nil {
			return nil, fmt.Errorf("failed to build keybindings handler: %w", err)
		}

		// Pre-build config save function for handler registration.
		saveConfigFunc := buildSaveConfigFunc()

		return webkit.NewEngine(
			input.Ctx, cfg, opts, wkCfg,
			input.ThemeManager, input.ColorResolver, input.Logger,
			func(ctx context.Context, router *webkit.MessageRouter, deps port.HandlerDependencies) error {
				var historyUC port.HomepageHistory
				if deps.HistoryUC != nil {
					historyUC, _ = deps.HistoryUC.(port.HomepageHistory)
				}
				var favoritesUC port.HomepageFavorites
				if deps.FavoritesUC != nil {
					favoritesUC, _ = deps.FavoritesUC.(port.HomepageFavorites)
				}
				var autoCopyConfig port.AutoCopyConfig
				if deps.ConfigGetter != nil {
					autoCopyConfig, _ = deps.ConfigGetter().(port.AutoCopyConfig)
				}
				return handlers.RegisterAll(ctx, router, handlers.Config{
					HistoryUC:          historyUC,
					FavoritesUC:        favoritesUC,
					Clipboard:          deps.Clipboard,
					AutoCopyConfig:     autoCopyConfig,
					SaveConfig:         saveConfigFunc,
					KeybindingsHandler: keybindingsHandler,
					OnClipboardCopied:  deps.OnClipboardCopied,
				})
			},
			func(ctx context.Context, router *webkit.MessageRouter, handler any) error {
				accentHandler, ok := handler.(port.AccentKeyHandler)
				if !ok {
					return fmt.Errorf("handler does not implement port.AccentKeyHandler")
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

// buildKeybindingsHandler constructs the keybindings handler using the config manager.
func buildKeybindingsHandler() (*handlers.KeybindingsHandler, error) {
	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	gateway := config.NewKeybindingsGateway(mgr)

	return handlers.NewKeybindingsHandler(
		usecase.NewGetKeybindingsUseCase(gateway),
		usecase.NewSetKeybindingUseCase(gateway, gateway),
		usecase.NewResetKeybindingUseCase(gateway),
		usecase.NewResetAllKeybindingsUseCase(gateway),
	), nil
}

// buildSaveConfigFunc constructs the config save function using the config manager.
func buildSaveConfigFunc() func(context.Context, port.WebUIConfig) error {
	mgr := config.GetManager()
	if mgr == nil {
		// Return a function that errors — config manager may not be ready yet.
		return func(_ context.Context, _ port.WebUIConfig) error {
			return fmt.Errorf("config manager not initialized")
		}
	}
	uc := usecase.NewSaveWebUIConfigUseCase(config.NewWebUIConfigGateway(mgr))
	return uc.Execute
}
