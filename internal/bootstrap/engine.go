package bootstrap

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/cef"
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
func BuildEngine(input EngineInput) (port.Engine, error) {
	cfg := input.Config
	engineType := cfg.Engine.ResolveEngineType()
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
		saveConfigFunc, err := buildSaveConfigFunc()
		if err != nil {
			return nil, fmt.Errorf("failed to build save config func: %w", err)
		}

		return webkit.NewEngine(
			input.Ctx, cfg, opts, wkCfg,
			input.ThemeManager, input.ColorResolver, input.Logger,
			func(ctx context.Context, router *webkit.MessageRouter, deps port.HandlerDependencies) error {
				return handlers.RegisterAll(ctx, router, handlers.Config{
					HistoryUC:          deps.HistoryUC,
					FavoritesUC:        deps.FavoritesUC,
					Clipboard:          deps.Clipboard,
					AutoCopyConfig:     deps.AutoCopyConfig,
					SaveConfig:         saveConfigFunc,
					KeybindingsHandler: keybindingsHandler,
					OnClipboardCopied:  deps.OnClipboardCopied,
				})
			},
			handlers.RegisterAccentHandlers,
		)
	case "cef":
		cefCfg := cfg.Engine.CEF
		return cef.NewEngine(input.Ctx, cefCfg)
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
// It returns an error immediately if the config manager is not initialized, consistent
// with buildKeybindingsHandler.
func buildSaveConfigFunc() (func(context.Context, port.WebUIConfig) error, error) {
	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}
	uc := usecase.NewSaveWebUIConfigUseCase(config.NewWebUIConfigGateway(mgr))
	return uc.Execute, nil
}
