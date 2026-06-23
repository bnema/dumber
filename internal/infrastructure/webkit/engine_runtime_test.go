package webkit

import (
	"context"
	"os"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

func TestEngine_UpdateSettings_AcceptsTypedSettingsPayload(t *testing.T) {
	e := &Engine{settings: NewSettingsManager(context.Background(), port.EngineSettingsPayload{})}

	err := e.UpdateSettings(context.Background(), port.EngineSettingsUpdate{
		Settings: port.EngineSettingsPayload{
			DefaultUIScale: 1.25,
			WebContent: port.EngineWebContentSettingsPayload{
				SansFont:                  "Inter",
				EnableDevTools:            true,
				CaptureConsole:            true,
				DrawCompositingIndicators: true,
				HardwareDecoding:          port.EngineHardwareDecodingDisable,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Inter", e.settings.current().WebContent.SansFont)
	require.True(t, e.settings.current().WebContent.EnableDevTools)
	require.True(t, e.settings.current().WebContent.CaptureConsole)
	require.True(t, e.settings.current().WebContent.DrawCompositingIndicators)
	require.Equal(t, port.EngineHardwareDecodingDisable, e.settings.current().WebContent.HardwareDecoding)
}

func TestEngine_UpdateSettings_NilSettings(t *testing.T) {
	// settings is nil — should not panic, just be a no-op.
	e := &Engine{}
	err := e.UpdateSettings(context.Background(), port.EngineSettingsUpdate{})
	require.NoError(t, err)
}

func TestEngine_RegisterHandlers_NilRouter(t *testing.T) {
	e := &Engine{}
	err := e.RegisterHandlers(context.Background(), port.HandlerDependencies{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "message router not initialized")
}

func TestEngine_RegisterAccentHandlers_NilRouter(t *testing.T) {
	e := &Engine{}
	err := e.RegisterAccentHandlers(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "message router not initialized")
}

func TestEngine_ConfigureDownloads_NilContext(t *testing.T) {
	e := &Engine{}
	err := e.ConfigureDownloads(context.Background(), "/tmp", nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "webkit context not initialized")
}

func TestNewDownloadHandler_NilPreparer_NoPanic(t *testing.T) {
	// After removing the panic, NewDownloadHandler with nil preparer should
	// not panic. The caller (ConfigureDownloads) is responsible for validation.
	require.NotPanics(t, func() {
		_ = NewDownloadHandler("/tmp", nil, nil)
	})
}

func TestEngine_Close_NilPool(t *testing.T) {
	e := &Engine{}
	err := e.Close()
	require.NoError(t, err)
}

func TestEngineConfigureContentInjectorAutoCopyGetterReadsCurrentPayload(t *testing.T) {
	settings := NewSettingsManager(context.Background(), port.EngineSettingsPayload{})
	injector := NewContentInjector(nil)

	engineConfigureContentInjectorRuntimeSettings(injector, settings)

	require.NotNil(t, injector.autoCopyConfigGetter)
	require.False(t, injector.autoCopyConfigGetter())

	settings.UpdateFromPayload(context.Background(), port.EngineSettingsPayload{
		WebContent: port.EngineWebContentSettingsPayload{
			AutoCopyOnSelection: true,
		},
	})

	require.True(t, injector.autoCopyConfigGetter())
}

func TestNewEngineAutoCopyGetterUsesRuntimeSettingsPayload(t *testing.T) {
	source, err := os.ReadFile("engine_init.go")
	require.NoError(t, err)
	globalGetter := "config." + "Get()" + ".Clipboard.AutoCopyOnSelection"
	require.NotContains(t, string(source), globalGetter)
	require.Contains(t, string(source), "settings.current().WebContent.AutoCopyOnSelection")
}
