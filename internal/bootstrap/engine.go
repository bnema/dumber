package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	audiofactory "github.com/bnema/dumber/internal/infrastructure/audio/factory"
	"github.com/bnema/dumber/internal/infrastructure/cef"
	clipboardinfra "github.com/bnema/dumber/internal/infrastructure/clipboard"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/infrastructure/gtkmenu"
	"github.com/bnema/dumber/internal/infrastructure/handlers"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/rs/zerolog"
)

// EngineInput holds the input for BuildEngine.
type EngineInput struct {
	Ctx                 context.Context
	Config              *config.Config
	RuntimeProfile      runtimeprofile.Profile
	ThemeManager        *theme.Manager
	ExternalThemeSource port.ExternalThemeSource
	ColorResolver       port.ColorSchemeResolver
	Logger              zerolog.Logger
}

// BuildEngine constructs a port.Engine for the engine type specified in cfg.Engine.Type.
func BuildEngine(input EngineInput) (port.Engine, error) {
	cfg := input.Config
	systemviewReader := config.NewSystemviewConfigReader(env.NewHardwareSurveyor())
	systemviewUC := usecase.NewReadSystemviewConfigUseCase(
		systemviewReader,
		usecase.WithSystemviewResolvedAppearance(
			input.ExternalThemeSource,
			input.ColorResolver,
			prefersDarkProvider(input.ThemeManager),
		),
	)
	buildConfigPayload := func(read func(context.Context) (dto.SystemviewConfigPayload, error)) func() ([]byte, error) {
		return func() ([]byte, error) {
			payload, err := read(input.Ctx)
			if err != nil {
				return nil, err
			}
			return json.Marshal(payload)
		}
	}
	currentConfigPayload := buildConfigPayload(systemviewUC.Current)
	defaultConfigPayload := buildConfigPayload(systemviewUC.Default)
	contextMenuBuilder := usecase.NewBuildContextMenuUseCase()
	contextMenuExecutorFactory := &usecase.ContextMenuActionExecutorFactory{}
	engineType := cfg.Engine.ResolveEngineType()
	switch engineType {
	case config.EngineTypeWebKit:
		profile, err := requireRuntimeProfile(input.RuntimeProfile, engineType)
		if err != nil {
			return nil, err
		}
		opts := port.EngineOptions{
			CookiePolicy: port.CookiePolicy(cfg.Engine.CookiePolicy),
		}
		wkCfg := webkit.EngineConfigFromConfig(cfg.Engine.WebKit)

		return webkit.NewEngine(
			input.Ctx, cfg, opts, profile, wkCfg,
			EngineSettingsPayloadFromConfig(cfg),
			currentConfigPayload, defaultConfigPayload,
			input.ThemeManager, input.ColorResolver,
			contextMenuBuilder, contextMenuExecutorFactory,
			input.Logger,
		)
	case config.EngineTypeCEF:
		profile, err := requireRuntimeProfile(input.RuntimeProfile, engineType)
		if err != nil {
			return nil, err
		}
		opts := port.EngineOptions{
			CookiePolicy: port.CookiePolicy(cfg.Engine.CookiePolicy),
		}
		cefCfg := cef.RuntimeConfig{
			CEFDir:                      cfg.Engine.CEF.CEFDir,
			LogFile:                     cfg.Engine.CEF.LogFile,
			LogSeverity:                 cfg.Engine.CEF.LogSeverity,
			RenderStack:                 string(cfg.Engine.CEF.CEFRenderStack()),
			AdaptiveWindowlessFrameRate: cfg.Engine.CEF.CEFAdaptiveWindowlessFrameRate(),
			WindowlessFrameRate:         cfg.Engine.CEF.WindowlessFrameRate,
			WindowlessFrameRateMax:      cfg.Engine.CEF.CEFWindowlessFrameRateMax(),
			Input: cef.RuntimeInputConfig{
				ScrollWheelMultiplier:              cfg.Engine.CEF.Input.ScrollWheelMultiplier,
				ScrollPreciseMultiplier:            cfg.Engine.CEF.Input.ScrollPreciseMultiplier,
				ScrollHorizontalMultiplier:         cfg.Engine.CEF.Input.ScrollHorizontalMultiplier,
				ScrollVerticalMultiplier:           cfg.Engine.CEF.Input.ScrollVerticalMultiplier,
				ScrollMaxDelta:                     cfg.Engine.CEF.Input.ScrollMaxDelta,
				TouchpadNavigationEnabled:          cfg.Engine.CEF.Input.TouchpadNavigationEnabled,
				TouchpadNavigationMinDelta:         cfg.Engine.CEF.Input.TouchpadNavigationMinDelta,
				TouchpadNavigationMaxVerticalRatio: cfg.Engine.CEF.Input.TouchpadNavigationMaxVerticalRatio,
			},
			EnableAudioHandler: cfg.Engine.CEF.EnableAudioHandler,
			TraceHandlers:      cfg.Engine.CEF.TraceHandlers,
			ApplicationScale:   cfg.DefaultUIScale,
		}
		deps := cef.EngineDependencies{
			RegisterHandlers:           handlers.RegisterAll,
			RegisterAccentHandlers:     handlers.RegisterAccentHandlers,
			CurrentConfigPayload:       currentConfigPayload,
			DefaultConfigPayload:       defaultConfigPayload,
			ContextMenuBuilder:         contextMenuBuilder,
			ContextMenuExecutorFactory: contextMenuExecutorFactory,
			ContextMenuRenderer:        gtkmenu.NewRenderer(nil),
			Clipboard:                  clipboardinfra.New(),
			ImageDataResolver:          webkit.NewContextMenuResolver(),
		}
		audioFactory := audiofactory.NewAudioOutputFactory()
		return cef.NewEngine(input.Ctx, opts, cef.RuntimePaths{
			StateRoot:     profile.CEFUserDataDir(),
			LogFile:       profile.CEFLogFile(),
			ProfileLogDir: profile.Shared.LogDir,
		}, cefCfg, audioFactory, deps)
	default:
		return nil, fmt.Errorf("unknown engine type: %q", engineType)
	}
}

func prefersDarkProvider(manager *theme.Manager) func() bool {
	return func() bool {
		if manager == nil {
			return true
		}
		return manager.PrefersDark()
	}
}

func requireRuntimeProfile(profile runtimeprofile.Profile, engineType string) (runtimeprofile.Profile, error) {
	if profile.Engine == "" {
		return runtimeprofile.Profile{}, fmt.Errorf("missing runtime profile for %s engine", engineType)
	}
	if profile.Engine != engineType {
		return runtimeprofile.Profile{}, fmt.Errorf("runtime profile engine %q does not match %q", profile.Engine, engineType)
	}
	return profile, nil
}
