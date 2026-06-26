package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetXDGDirs_ProdPreservesLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ENV", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	dirs, err := GetXDGDirs()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", appName), dirs.ConfigHome)
	require.Equal(t, filepath.Join(home, ".local", "share", appName), dirs.DataHome)
	require.Equal(t, filepath.Join(home, ".local", "state", appName), dirs.StateHome)
	require.Equal(t, filepath.Join(home, ".cache", appName), dirs.CacheHome)
}

func TestGetXDGDirs_DevUsesSharedSandboxLayout(t *testing.T) {
	wd := t.TempDir()
	t.Chdir(wd)

	t.Setenv("ENV", "dev")
	runtimeDir, err := os.MkdirTemp("", "dbr-")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(runtimeDir)
	})
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	dirs, err := GetXDGDirs()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(wd, ".dev", appName, "config"), dirs.ConfigHome)
	require.Equal(t, filepath.Join(wd, ".dev", appName, "data"), dirs.DataHome)
	require.Equal(t, filepath.Join(wd, ".dev", appName, "state"), dirs.StateHome)
	require.Equal(t, filepath.Join(wd, ".dev", appName, "cache"), dirs.CacheHome)

	logDir, err := GetLogDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(wd, ".dev", appName, "logs"), logDir)
}
