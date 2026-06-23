package port

import (
	"reflect"
	"testing"
)

func TestEngineSettingsPayloadContainsRuntimeWebContentFields(t *testing.T) {
	payloadType := reflect.TypeOf(EngineSettingsPayload{})
	for _, field := range []string{"DefaultUIScale", "WebContent"} {
		if _, ok := payloadType.FieldByName(field); !ok {
			t.Fatalf("EngineSettingsPayload missing %s", field)
		}
	}
	webContentType := reflect.TypeOf(EngineWebContentSettingsPayload{})
	for _, field := range []string{
		"SansFont",
		"SerifFont",
		"MonospaceFont",
		"DefaultFontSize",
		"EnableDevTools",
		"CaptureConsole",
		"DrawCompositingIndicators",
		"HardwareDecoding",
	} {
		if _, ok := webContentType.FieldByName(field); !ok {
			t.Fatalf("EngineWebContentSettingsPayload missing %s", field)
		}
	}
}

func TestEngineSettingsUpdateHasNoRawBridge(t *testing.T) {
	updateType := reflect.TypeOf(EngineSettingsUpdate{})
	if _, ok := updateType.FieldByName("Raw"); ok {
		t.Fatal("EngineSettingsUpdate must not expose Raw legacy bridge")
	}
}
