package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	audiofactory "github.com/bnema/dumber/internal/infrastructure/audio/factory"
	"github.com/bnema/dumber/internal/infrastructure/cef"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/env"
	"github.com/bnema/dumber/internal/infrastructure/handlers"
	"github.com/bnema/dumber/internal/infrastructure/transcoder"
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
	systemviewReader := config.NewSystemviewConfigReader(env.NewHardwareSurveyor())
	systemviewUC := usecase.NewReadSystemviewConfigUseCase(systemviewReader)
	buildConfigPayload := func(read func(context.Context) (port.SystemviewConfigPayload, error)) func() ([]byte, error) {
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
	engineType := cfg.Engine.ResolveEngineType()
	switch engineType {
	case config.EngineTypeWebKit:
		opts := port.EngineOptions{
			DataDir:      input.DataDir,
			CacheDir:     input.CacheDir,
			CookiePolicy: port.CookiePolicy(cfg.Engine.CookiePolicy),
		}
		wkCfg := webkit.EngineConfigFromConfig(cfg.Engine.WebKit)

		return webkit.NewEngine(
			input.Ctx, cfg, opts, wkCfg,
			currentConfigPayload, defaultConfigPayload,
			input.ThemeManager, input.ColorResolver, input.Logger,
		)
	case config.EngineTypeCEF:
		opts := port.EngineOptions{
			DataDir:      input.DataDir,
			CacheDir:     input.CacheDir,
			CookiePolicy: port.CookiePolicy(cfg.Engine.CookiePolicy),
		}
		cefCfg := cef.RuntimeConfig{
			CEFDir:                   cfg.Engine.CEF.CEFDir,
			LogFile:                  cfg.Engine.CEF.LogFile,
			LogSeverity:              cfg.Engine.CEF.LogSeverity,
			WindowlessFrameRate:      cfg.Engine.CEF.WindowlessFrameRate,
			EnableAudioHandler:       cfg.Engine.CEF.EnableAudioHandler,
			EnableContextMenuHandler: cfg.Engine.CEF.EnableContextMenuHandler,
			TraceHandlers:            cfg.Engine.CEF.TraceHandlers,
		}
		transcodingCfg := cef.TranscodingRuntimeConfig{
			Enabled:       cfg.Transcoding.Enabled,
			HWAccel:       cfg.Transcoding.HWAccel,
			MaxConcurrent: cfg.Transcoding.MaxConcurrent,
			Quality:       cfg.Transcoding.Quality,
		}
		deps := cef.EngineDependencies{
			RegisterHandlers:       handlers.RegisterAll,
			RegisterAccentHandlers: handlers.RegisterAccentHandlers,
			CurrentConfigPayload:   currentConfigPayload,
			DefaultConfigPayload:   defaultConfigPayload,
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
		return cef.NewEngine(input.Ctx, opts, cefCfg, transcodingCfg, audioFactory, deps)
	default:
		return nil, fmt.Errorf("unknown engine type: %q", engineType)
	}
}
