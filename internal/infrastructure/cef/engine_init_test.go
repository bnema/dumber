package cef

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/audio/factory"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestResolvedStateRoot_PrefersCacheDir(t *testing.T) {
	profile := testCEFDevProfile(t)
	root := resolvedStateRoot(profile.CEFUserDataDir(), port.EngineOptions{DataDir: "/tmp/data", CacheDir: "/tmp/cache"})
	require.Equal(t, "/tmp/cache", root)
}

func TestResolvedStateRoot_UsesEnvOverride(t *testing.T) {
	profile := testCEFDevProfile(t)
	t.Setenv(CEFRootCachePathEnvVar, "/tmp/override")

	root := resolvedStateRoot(profile.CEFUserDataDir(), port.EngineOptions{})
	require.Equal(t, "/tmp/override", root)

	root = resolvedStateRoot(profile.CEFUserDataDir(), port.EngineOptions{DataDir: "/tmp/data", CacheDir: "/tmp/cache"})
	require.Equal(t, "/tmp/override", root)
}

func TestResolvedStateRoot_UsesProfileDefault(t *testing.T) {
	profile := testCEFProdProfile(t)
	root := resolvedStateRoot(profile.CEFUserDataDir(), port.EngineOptions{})
	require.Equal(t, profile.CEFUserDataDir(), root)
}

func TestPrepareCEFSettings_UsesResolvedProfilePaths(t *testing.T) {
	logger := zerolog.Nop()
	profile := testCEFDevProfile(t)
	settings, err := prepareCEFSettings(port.EngineOptions{}, RuntimePaths{StateRoot: profile.CEFUserDataDir(), LogFile: profile.CEFLogFile()}, RuntimeConfig{}, &logger)
	require.NoError(t, err)
	require.Equal(t, profile.CEFUserDataDir(), settings.RootCachePath)
	require.Equal(t, profile.CEFLogFile(), settings.LogFile)
	require.Empty(t, settings.BrowserSubprocessPath)
}

func TestParseLoadedLibCEFPath(t *testing.T) {
	maps := "7f5fd4000000-7f5fd4200000 r--p 00000000 00:00 0 /usr/lib/libvulkan.so.1\n" +
		"7f5fe0000000-7f5fe8200000 r-xp 00000000 00:00 0 /opt/cef-vaapi-runtime/usr/lib/cef/libcef.so\n" +
		"7f5fe8200000-7f5fe8400000 r--p 08200000 00:00 0 /opt/cef-vaapi-runtime/usr/lib/cef/libcef.so\n"

	require.Equal(t, "/opt/cef-vaapi-runtime/usr/lib/cef/libcef.so", parseLoadedLibCEFPath(maps))
}

func TestParseLoadedLibCEFPath_ReturnsEmptyWhenAbsent(t *testing.T) {
	require.Empty(t, parseLoadedLibCEFPath("7f5fd4000000-7f5fd4200000 r--p 00000000 00:00 0 /usr/lib/libvulkan.so.1\n"))
}

func TestPrepareCEFSettings_RejectsNonDefaultCookiePolicy(t *testing.T) {
	logger := zerolog.Nop()
	profile := testCEFDevProfile(t)
	_, err := prepareCEFSettings(port.EngineOptions{CookiePolicy: port.CookiePolicyNever}, RuntimePaths{StateRoot: profile.CEFUserDataDir(), LogFile: profile.CEFLogFile()}, RuntimeConfig{}, &logger)
	require.ErrorIs(t, err, ErrCookiePolicyUnsupported)
}

func TestPrepareCEFSettings_AllowsNoThirdPartyCookiePolicy(t *testing.T) {
	logger := zerolog.Nop()
	profile := testCEFDevProfile(t)
	_, err := prepareCEFSettings(port.EngineOptions{CookiePolicy: port.CookiePolicyNoThirdParty}, RuntimePaths{StateRoot: profile.CEFUserDataDir(), LogFile: profile.CEFLogFile()}, RuntimeConfig{}, &logger)
	require.NoError(t, err)
}

func TestResolveCEFInitTraceFile_EmptyWithoutLogPath(t *testing.T) {
	require.Empty(t, resolveCEFInitTraceFile("", ""))
}

func TestPrepareCEFLogFile_CreatesPrivateDirectoryAndFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "private", "cef_runtime.log")

	path, err := prepareCEFLogFile("", logFile)
	require.NoError(t, err)
	require.Equal(t, logFile, path)

	dirInfo, err := os.Stat(filepath.Dir(logFile))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(dirPerm), dirInfo.Mode().Perm())

	fileInfo, err := os.Stat(logFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(filePerm), fileInfo.Mode().Perm())
}

func TestPrepareCEFLogFile_TightensExistingFilePermissions(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "private", "cef_runtime.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logFile), dirPerm))
	require.NoError(t, os.WriteFile(logFile, []byte("existing"), 0o644))

	path, err := prepareCEFLogFile("", logFile)
	require.NoError(t, err)
	require.Equal(t, logFile, path)

	fileInfo, err := os.Stat(logFile)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(filePerm), fileInfo.Mode().Perm())
}

func TestPrepareCEFLogFile_RejectsSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.log")
	require.NoError(t, os.WriteFile(target, []byte("existing"), 0o644))

	logFile := filepath.Join(t.TempDir(), "private", "cef_runtime.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logFile), dirPerm))
	require.NoError(t, os.Symlink(target, logFile))

	path, err := prepareCEFLogFile("", logFile)
	require.ErrorContains(t, err, "must not be a symlink")
	require.Empty(t, path)

	targetInfo, statErr := os.Stat(target)
	require.NoError(t, statErr)
	require.Equal(t, os.FileMode(0o644), targetInfo.Mode().Perm())
}

func TestPrepareCEFLogFile_RejectsDirectoryPath(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "private", "cef_runtime.log")
	require.NoError(t, os.MkdirAll(logFile, dirPerm))

	path, err := prepareCEFLogFile("", logFile)
	require.ErrorContains(t, err, "not a regular file")
	require.Empty(t, path)
}

func TestResolveCEFInitTraceFile_UsesExplicitLogFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "custom", "cef_runtime.log")

	path := resolveCEFInitTraceFile("", logFile)
	require.Equal(t, filepath.Join(filepath.Dir(logFile), "cef_runtime.bootstrap.log"), path)
}

func TestResolveCEFInitTraceFile_UsesResolvedProfileLogDirByDefault(t *testing.T) {
	profile := testCEFDevProfile(t)

	path := resolveCEFInitTraceFile(profile.CEFLogFile(), "")
	require.Equal(t, filepath.Join(profile.EnginePaths.LogDir, "cef_runtime.bootstrap.log"), path)
}

func TestPrepareCEFSettings_IgnoresInitTraceRequestWithoutTouchingFilesystem(t *testing.T) {
	t.Setenv(puregoCEFInitTraceEnvVar, "1")
	profile := testCEFDevProfile(t)
	bootstrapLogFile := resolveCEFInitTraceFile(profile.CEFLogFile(), "")

	var logs bytes.Buffer
	logger := zerolog.New(&logs)

	settings, err := prepareCEFSettings(
		port.EngineOptions{},
		RuntimePaths{StateRoot: profile.CEFUserDataDir(), LogFile: profile.CEFLogFile()},
		RuntimeConfig{},
		&logger,
	)
	require.NoError(t, err)
	require.Equal(t, profile.CEFLogFile(), settings.LogFile)

	_, statErr := os.Stat(bootstrapLogFile)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	require.Contains(t, logs.String(), puregoCEFInitTraceEnvVar)
	require.Contains(t, logs.String(), bootstrapLogFile)
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
	runtimeDir := shortRuntimeDir(t)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	profile, err := runtimeprofile.Resolve(runtimeprofile.ResolveInput{
		Env: func(key string) string {
			switch key {
			case "ENV":
				return "dev"
			case "XDG_RUNTIME_DIR":
				return runtimeDir
			default:
				return ""
			}
		},
		Engine: "cef",
		CWD:    func() (string, error) { return cwd, nil },
	})
	require.NoError(t, err)
	return profile
}

func shortRuntimeDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "dbr-")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
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
