package bootstrap

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func TestEngineSettingsPayloadFromConfigMapsRuntimeFields(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DefaultUIScale = 1.25
	cfg.Appearance.SansFont = "Inter"
	cfg.Appearance.SerifFont = "Literata"
	cfg.Appearance.MonospaceFont = "Fira Code"
	cfg.Appearance.DefaultFontSize = 17
	cfg.Debug.EnableDevTools = true
	cfg.Logging.CaptureConsole = true
	cfg.Engine.WebKit.DrawCompositingIndicators = true
	cfg.Media.HardwareDecodingMode = config.HardwareDecodingForce

	got := EngineSettingsPayloadFromConfig(cfg)

	if got.DefaultUIScale != 1.25 {
		t.Fatalf("DefaultUIScale=%v, want 1.25", got.DefaultUIScale)
	}
	if got.WebContent.SansFont != "Inter" ||
		got.WebContent.SerifFont != "Literata" ||
		got.WebContent.MonospaceFont != "Fira Code" ||
		got.WebContent.DefaultFontSize != 17 {
		t.Fatalf("font settings not mapped: %#v", got.WebContent)
	}
	if !got.WebContent.EnableDevTools ||
		!got.WebContent.CaptureConsole ||
		!got.WebContent.DrawCompositingIndicators {
		t.Fatalf("debug settings not mapped: %#v", got.WebContent)
	}
	if got.WebContent.HardwareDecoding != port.EngineHardwareDecodingForce {
		t.Fatalf("HardwareDecoding=%q, want %q", got.WebContent.HardwareDecoding, port.EngineHardwareDecodingForce)
	}
}

func TestEngineSettingsPayloadFromNilConfigReturnsZeroPayload(t *testing.T) {
	got := EngineSettingsPayloadFromConfig(nil)
	if got != (port.EngineSettingsPayload{}) {
		t.Fatalf("payload=%#v, want zero value", got)
	}
}

func TestEngineSettingsPayloadFromConfigMapsHardwareDecodingModes(t *testing.T) {
	tests := []struct {
		name string
		mode config.HardwareDecodingMode
		want port.EngineHardwareDecodingMode
	}{
		{
			name: "auto",
			mode: config.HardwareDecodingAuto,
			want: port.EngineHardwareDecodingAuto,
		},
		{
			name: "force",
			mode: config.HardwareDecodingForce,
			want: port.EngineHardwareDecodingForce,
		},
		{
			name: "disable",
			mode: config.HardwareDecodingDisable,
			want: port.EngineHardwareDecodingDisable,
		},
		{
			name: "unknown",
			mode: config.HardwareDecodingMode("surprise"),
			want: port.EngineHardwareDecodingAuto,
		},
		{
			name: "zero value",
			mode: "",
			want: port.EngineHardwareDecodingAuto,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Media.HardwareDecodingMode = tt.mode

			got := EngineSettingsPayloadFromConfig(cfg)

			if got.WebContent.HardwareDecoding != tt.want {
				t.Fatalf("HardwareDecoding=%q, want %q", got.WebContent.HardwareDecoding, tt.want)
			}
		})
	}
}
