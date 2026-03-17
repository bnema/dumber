package webkit

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/require"
)

func TestEngine_UpdateSettings_WrongType(t *testing.T) {
	e := &Engine{}
	err := e.UpdateSettings(context.Background(), port.EngineSettingsUpdate{Raw: "wrong"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected *config.Config")
}

func TestEngine_UpdateSettings_NilSettings(t *testing.T) {
	// settings is nil — should not panic, just be a no-op.
	e := &Engine{}
	// Use a nil *config.Config to pass the type assertion.
	var cfg *config.Config
	err := e.UpdateSettings(context.Background(), port.EngineSettingsUpdate{Raw: cfg})
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

func TestEngine_Close_NilPool(t *testing.T) {
	e := &Engine{}
	err := e.Close()
	require.NoError(t, err)
}
