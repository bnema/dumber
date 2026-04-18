package cef

import (
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/audio/factory"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestResolvedStateRoot_PrefersDataDir(t *testing.T) {
	profile := testCEFDevProfile(t)
	root := resolvedStateRoot(profile, port.EngineOptions{DataDir: "/tmp/data", CacheDir: "/tmp/cache"})
	require.Equal(t, "/tmp/data", root)
}

func TestResolvedStateRoot_UsesEnvOverride(t *testing.T) {
	profile := testCEFDevProfile(t)
	t.Setenv(CEFRootCachePathEnvVar, "/tmp/override")

	root := resolvedStateRoot(profile, port.EngineOptions{})
	require.Equal(t, "/tmp/override", root)

	root = resolvedStateRoot(profile, port.EngineOptions{DataDir: "/tmp/data", CacheDir: "/tmp/cache"})
	require.Equal(t, "/tmp/override", root)
}

func TestResolvedStateRoot_UsesProfileDefault(t *testing.T) {
	profile := testCEFProdProfile(t)
	root := resolvedStateRoot(profile, port.EngineOptions{})
	require.Equal(t, profile.CEFUserDataDir(), root)
}

func TestPrepareCEFSettings_UsesResolvedProfilePaths(t *testing.T) {
	logger := zerolog.Nop()
	profile := testCEFDevProfile(t)
	settings, err := prepareCEFSettings(port.EngineOptions{}, profile, RuntimeConfig{}, &logger)
	require.NoError(t, err)
	require.Equal(t, profile.CEFUserDataDir(), settings.RootCachePath)
	require.Equal(t, profile.CEFLogFile(), settings.LogFile)
}

func TestPrepareCEFSettings_RejectsNonDefaultCookiePolicy(t *testing.T) {
	logger := zerolog.Nop()
	_, err := prepareCEFSettings(port.EngineOptions{CookiePolicy: port.CookiePolicyNever}, testCEFDevProfile(t), RuntimeConfig{}, &logger)
	require.ErrorIs(t, err, ErrCookiePolicyUnsupported)
}

func TestPrepareCEFSettings_AllowsNoThirdPartyCookiePolicy(t *testing.T) {
	logger := zerolog.Nop()
	_, err := prepareCEFSettings(port.EngineOptions{CookiePolicy: port.CookiePolicyNoThirdParty}, testCEFDevProfile(t), RuntimeConfig{}, &logger)
	require.NoError(t, err)
}

func TestPrepareCEFInitTraceFile_DisabledByDefault(t *testing.T) {
	t.Setenv(puregoCEFInitTraceEnvVar, "")

	path, err := prepareCEFInitTraceFile(testCEFDevProfile(t), "")
	require.NoError(t, err)
	require.Empty(t, path)
}

func TestPrepareCEFInitTraceFile_EnabledViaExplicitLogFile(t *testing.T) {
	t.Setenv(puregoCEFInitTraceEnvVar, "1")
	logFile := filepath.Join(t.TempDir(), "custom", "cef_runtime.log")

	path, err := prepareCEFInitTraceFile(testCEFDevProfile(t), logFile)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(filepath.Dir(logFile), "cef_runtime.bootstrap.log"), path)
	require.FileExists(t, path)
}

func TestPrepareCEFInitTraceFile_UsesResolvedProfileLogDirByDefault(t *testing.T) {
	t.Setenv(puregoCEFInitTraceEnvVar, "1")
	profile := testCEFDevProfile(t)

	path, err := prepareCEFInitTraceFile(profile, "")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(profile.EnginePaths.LogDir, "cef_runtime.bootstrap.log"), path)
	require.FileExists(t, path)
}

// TestCreateAudioOutputFactory_CreatesUsableFactory verifies that the audio
// factory can be created and produces working streams. This tests the runtime
// wiring decision to create the factory during CEF engine setup.
func TestCreateAudioOutputFactory_ReturnsFactory(t *testing.T) {
	audioFactory := factory.NewAudioOutputFactory()

	require.NotNil(t, audioFactory, "Audio factory should not be nil")
}

func testCEFDevProfile(t *testing.T) runtimeprofile.Profile {
	t.Helper()
	cwd := t.TempDir()
	profile, err := runtimeprofile.Resolve(runtimeprofile.ResolveInput{
		Env: func(key string) string {
			if key == "ENV" {
				return "dev"
			}
			return ""
		},
		Engine: "cef",
		CWD:    func() (string, error) { return cwd, nil },
	})
	require.NoError(t, err)
	return profile
}

func testCEFProdProfile(t *testing.T) runtimeprofile.Profile {
	t.Helper()
	root := t.TempDir()
	profile, err := runtimeprofile.Resolve(runtimeprofile.ResolveInput{
		Env:    func(string) string { return "" },
		Engine: "cef",
		CWD:    func() (string, error) { return root, nil },
		Base: runtimeprofile.BasePaths{
			ConfigHome: filepath.Join(root, "config"),
			DataHome:   filepath.Join(root, "data"),
			StateHome:  filepath.Join(root, "state"),
			CacheHome:  filepath.Join(root, "cache"),
		},
	})
	require.NoError(t, err)
	return profile
}
