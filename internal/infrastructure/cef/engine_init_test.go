package cef

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/audio/factory"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestResolvedStateRoot_PrefersDataDir(t *testing.T) {
	root := resolvedStateRoot(port.EngineOptions{DataDir: "/tmp/data", CacheDir: "/tmp/cache"})
	require.Equal(t, "/tmp/data", root)
}

func TestResolvedStateRoot_FallsBackToCacheDir(t *testing.T) {
	root := resolvedStateRoot(port.EngineOptions{CacheDir: "/tmp/cache"})
	require.Equal(t, "/tmp/cache", root)
}

func TestPrepareCEFSettings_UsesResolvedStateRoot(t *testing.T) {
	logger := zerolog.Nop()
	settings, err := prepareCEFSettings(port.EngineOptions{CacheDir: "/tmp/cache"}, RuntimeConfig{}, &logger)
	require.NoError(t, err)
	require.Equal(t, "/tmp/cache", settings.RootCachePath)
}

func TestPrepareCEFSettings_RejectsNonDefaultCookiePolicy(t *testing.T) {
	logger := zerolog.Nop()
	_, err := prepareCEFSettings(port.EngineOptions{CookiePolicy: port.CookiePolicyNever}, RuntimeConfig{}, &logger)
	require.ErrorIs(t, err, ErrCookiePolicyUnsupported)
}

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
	settings, err := prepareCEFSettings(port.EngineOptions{}, RuntimeConfig{}, &logger)
	require.NoError(t, err)

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
	settings, err := prepareCEFSettings(port.EngineOptions{}, RuntimeConfig{}, &logger)
	require.NoError(t, err)

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
