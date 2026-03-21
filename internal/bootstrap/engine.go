package bootstrap

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/cef"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/handlers"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
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

		// Pre-build keybinding use cases for handler registration.
		keybindingDeps, err := buildKeybindingDeps()
		if err != nil {
			return nil, fmt.Errorf("failed to build keybinding deps: %w", err)
		}

		// Pre-build config save function for handler registration.
		saveConfigFunc, err := buildSaveConfigFunc()
		if err != nil {
			return nil, fmt.Errorf("failed to build save config func: %w", err)
		}

		return webkit.NewEngine(
			input.Ctx, cfg, opts, wkCfg,
			input.ThemeManager, input.ColorResolver, input.Logger,
			func(ctx context.Context, router port.WebUIHandlerRouter, deps port.HandlerDependencies) error {
				// Inject pre-built dependencies that come from bootstrap, not from the engine.
				deps.SaveConfig = saveConfigFunc
				deps.KeybindingsGetter = keybindingDeps.KeybindingsGetter
				deps.KeybindingSetter = keybindingDeps.KeybindingSetter
				deps.KeybindingResetter = keybindingDeps.KeybindingResetter
				deps.AllKeybindingsResetter = keybindingDeps.AllKeybindingsResetter
				return handlers.RegisterAll(ctx, router, deps)
			},
			func(ctx context.Context, router port.WebUIHandlerRouter, handler port.AccentKeyHandler) error {
				return handlers.RegisterAccentHandlers(ctx, router, handler)
			},
		)
	case "cef":
		cefCfg := cfg.Engine.CEF
		return cef.NewEngine(input.Ctx, cefCfg)
	default:
		return nil, fmt.Errorf("unknown engine type: %q", engineType)
	}
}

// keybindingDeps holds keybinding use cases built at bootstrap time.
type keybindingDeps struct {
	KeybindingsGetter      port.KeybindingsGetter
	KeybindingSetter       port.KeybindingSetter
	KeybindingResetter     port.KeybindingResetter
	AllKeybindingsResetter port.AllKeybindingsResetter
}

// buildKeybindingDeps constructs keybinding use cases using the config manager.
func buildKeybindingDeps() (*keybindingDeps, error) {
	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	gateway := config.NewKeybindingsGateway(mgr)

	return &keybindingDeps{
		KeybindingsGetter:      usecase.NewGetKeybindingsUseCase(gateway),
		KeybindingSetter:       usecase.NewSetKeybindingUseCase(gateway, gateway),
		KeybindingResetter:     usecase.NewResetKeybindingUseCase(gateway),
		AllKeybindingsResetter: usecase.NewResetAllKeybindingsUseCase(gateway),
	}, nil
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
