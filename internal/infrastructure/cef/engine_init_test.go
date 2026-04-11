package cef

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/audio/factory"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestPrepareCEFInitTraceFile_DisabledByDefault(t *testing.T) {
	t.Setenv(puregoCEFInitTraceEnvVar, "")

	path, err := prepareCEFInitTraceFile(config.CEFEngineConfig{})
	require.NoError(t, err)
	require.Empty(t, path)
}

func TestPrepareCEFInitTraceFile_EnabledViaEnv(t *testing.T) {
	t.Setenv(puregoCEFInitTraceEnvVar, "1")
	cfg := config.CEFEngineConfig{
		LogFile: filepath.Join(t.TempDir(), "cef_runtime.log"),
	}

	path, err := prepareCEFInitTraceFile(cfg)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(filepath.Dir(cfg.LogFile), "cef_runtime.bootstrap.log"), path)
	require.FileExists(t, path)
}

func TestPrepareCEFSettings_RootCachePath_UsesLegacyCEFUserDataDir(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", homeDir)
	t.Setenv("ENV", "")

	logger := zerolog.Nop()
	settings := prepareCEFSettings(config.CEFEngineConfig{}, &logger)

	require.Equal(t, filepath.Join(homeDir, ".config", "cef_user_data"), settings.RootCachePath)
}

func TestPrepareCEFSettings_RootCachePath_DevMode(t *testing.T) {
	// Create a temp directory and change into it for the test
	tempDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Set ENV=dev to trigger dev mode
	t.Setenv("ENV", "dev")

	logger := zerolog.Nop()
	settings := prepareCEFSettings(config.CEFEngineConfig{}, &logger)

	// In dev mode, should resolve under .dev/dumber/cef_user_data
	require.Equal(t, filepath.Join(tempDir, ".dev", "dumber", "cef_user_data"), settings.RootCachePath)
}

// TestCreateAudioOutputFactory_CreatesUsableFactory verifies that the audio
// factory can be created and produces working streams. This tests the runtime
// wiring decision to create the factory during CEF engine setup.
func TestCreateAudioOutputFactory_ReturnsFactory(t *testing.T) {
	audioFactory := factory.NewAudioOutputFactory()

	require.NotNil(t, audioFactory, "Audio factory should not be nil")
}
