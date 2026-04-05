package bootstrap

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/cef"
	"github.com/bnema/dumber/internal/infrastructure/config"
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
		cefCfg := cfg.Engine.CEF
		return cef.NewEngine(input.Ctx, cefCfg, cfg.Transcoding)
	default:
		return nil, fmt.Errorf("unknown engine type: %q", engineType)
	}
}
