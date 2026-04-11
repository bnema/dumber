package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
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
			RegisterHandlers:       handlers.RegisterAll,
			RegisterAccentHandlers: handlers.RegisterAccentHandlers,
			CurrentConfigPayload:   buildConfigPayload(config.Get),
			DefaultConfigPayload:   buildConfigPayload(config.DefaultConfig),
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
