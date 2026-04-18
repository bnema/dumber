package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	audiofactory "github.com/bnema/dumber/internal/infrastructure/audio/factory"
	"github.com/bnema/dumber/internal/infrastructure/cef"
	clipboardinfra "github.com/bnema/dumber/internal/infrastructure/clipboard"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/infrastructure/handlers"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/bnema/dumber/internal/infrastructure/transcoder"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/ui/theme"
	"github.com/rs/zerolog"
)

// EngineInput holds the input for BuildEngine.
type EngineInput struct {
	Ctx            context.Context
	Config         *config.Config
	RuntimeProfile runtimeprofile.Profile
	ThemeManager   *theme.Manager
	ColorResolver  port.ColorSchemeResolver
	Logger         zerolog.Logger
}

// BuildEngine constructs a port.Engine for the engine type specified in cfg.Engine.Type.
func BuildEngine(input EngineInput) (port.Engine, error) {
	cfg := input.Config
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
			CEFDir:              cfg.Engine.CEF.CEFDir,
			LogFile:             cfg.Engine.CEF.LogFile,
			LogSeverity:         cfg.Engine.CEF.LogSeverity,
			WindowlessFrameRate: cfg.Engine.CEF.WindowlessFrameRate,
			EnableAudioHandler:  cfg.Engine.CEF.EnableAudioHandler,
			TraceHandlers:       cfg.Engine.CEF.TraceHandlers,
		}
		transcodingCfg := cef.TranscodingRuntimeConfig{
			Enabled:       cfg.Transcoding.Enabled,
			HWAccel:       cfg.Transcoding.HWAccel,
			MaxConcurrent: cfg.Transcoding.MaxConcurrent,
			Quality:       cfg.Transcoding.Quality,
		}
		surveyor := env.NewHardwareSurveyor()
		buildConfigPayload := func(cfgf func() *config.Config) func() ([]byte, error) {
			return func() ([]byte, error) {
				cfg := cfgf()
				var hw *port.HardwareInfo
				if surveyor != nil {
					survey := surveyor.Survey(context.Background())
					hw = &survey
				}
				return json.Marshal(config.BuildWebUIConfigPayload(cfg, hw))
			}
		}
		deps := cef.EngineDependencies{
			RegisterHandlers:           handlers.RegisterAll,
			RegisterAccentHandlers:     handlers.RegisterAccentHandlers,
			CurrentConfigPayload:       buildConfigPayload(config.Get),
			DefaultConfigPayload:       buildConfigPayload(config.DefaultConfig),
			ContextMenuBuilder:         contextMenuBuilder,
			ContextMenuExecutorFactory: contextMenuExecutorFactory,
			Clipboard:                  clipboardinfra.New(),
			ImageDataResolver:          webkit.NewContextMenuResolver(),
			MediaClassifier: cef.MediaClassifier{
				IsProprietaryVideoMIME:     transcoder.IsProprietaryVideoMIME,
				IsOpenVideoMIME:            transcoder.IsOpenVideoMIME,
				IsStreamingManifestMIME:    transcoder.IsStreamingManifestMIME,
				IsStreamingManifestURL:     transcoder.IsStreamingManifestURL,
				IsEagerTranscodeURL:        transcoder.IsEagerTranscodeURL,
				ParseSyntheticTranscodeURL: transcoder.ParseSyntheticTranscodeURL,
			},
		}
		audioFactory := audiofactory.NewAudioOutputFactory()
		return cef.NewEngine(input.Ctx, opts, profile, cefCfg, transcodingCfg, audioFactory, deps)
	default:
		return nil, fmt.Errorf("unknown engine type: %q", engineType)
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
