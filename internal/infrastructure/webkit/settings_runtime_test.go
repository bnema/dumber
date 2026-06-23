package webkit

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
)

func TestSettingsManagerUpdateFromPayloadStoresTypedRuntimeSettings(t *testing.T) {
	sm := NewSettingsManager(context.Background(), port.EngineSettingsPayload{})
	payload := port.EngineSettingsPayload{
		WebContent: port.EngineWebContentSettingsPayload{
			SansFont:         "Inter",
			SerifFont:        "Literata",
			MonospaceFont:    "Fira Code",
			DefaultFontSize:  16,
			HardwareDecoding: port.EngineHardwareDecodingForce,
		},
	}

	sm.UpdateFromPayload(context.Background(), payload)

	got := sm.current()
	if got.WebContent.SansFont != "Inter" ||
		got.WebContent.SerifFont != "Literata" ||
		got.WebContent.MonospaceFont != "Fira Code" ||
		got.WebContent.DefaultFontSize != 16 ||
		got.WebContent.HardwareDecoding != port.EngineHardwareDecodingForce {
		t.Fatalf("stored payload=%#v", got)
	}
}
