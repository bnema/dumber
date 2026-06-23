package bootstrap

import (
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func EngineSettingsPayloadFromConfig(cfg *config.Config) port.EngineSettingsPayload {
	if cfg == nil {
		return port.EngineSettingsPayload{}
	}
	return port.EngineSettingsPayload{
		DefaultUIScale: cfg.DefaultUIScale,
		WebContent: port.EngineWebContentSettingsPayload{
			SansFont:                  cfg.Appearance.SansFont,
			SerifFont:                 cfg.Appearance.SerifFont,
			MonospaceFont:             cfg.Appearance.MonospaceFont,
			DefaultFontSize:           cfg.Appearance.DefaultFontSize,
			EnableDevTools:            cfg.Debug.EnableDevTools,
			CaptureConsole:            cfg.Logging.CaptureConsole,
			DrawCompositingIndicators: cfg.Engine.WebKit.DrawCompositingIndicators,
			HardwareDecoding:          engineHardwareDecodingModeFromConfig(cfg.Media.HardwareDecodingMode),
		},
	}
}

func engineHardwareDecodingModeFromConfig(mode config.HardwareDecodingMode) port.EngineHardwareDecodingMode {
	switch mode {
	case config.HardwareDecodingForce:
		return port.EngineHardwareDecodingForce
	case config.HardwareDecodingDisable:
		return port.EngineHardwareDecodingDisable
	default:
		return port.EngineHardwareDecodingAuto
	}
}
