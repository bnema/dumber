package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyDevelopmentEnvironment_DevIsolatesHomeAndXDGDirectories(t *testing.T) {
	worktree := t.TempDir()
	t.Chdir(worktree)
	t.Setenv("ENV", developmentEnvironmentName)
	t.Setenv("HOME", "/home/production")
	t.Setenv("XDG_CONFIG_HOME", "/production/config")
	t.Setenv("XDG_DATA_HOME", "/production/data")
	t.Setenv("XDG_STATE_HOME", "/production/state")
	t.Setenv("XDG_CACHE_HOME", "/production/cache")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")

	require.NoError(t, ApplyDevelopmentEnvironment())

	root := filepath.Join(worktree, ".dev", "dumber")
	want := map[string]string{
		"HOME":            filepath.Join(root, "home"),
		"XDG_CONFIG_HOME": filepath.Join(root, "config"),
		"XDG_DATA_HOME":   filepath.Join(root, "data"),
		"XDG_STATE_HOME":  filepath.Join(root, "state"),
		"XDG_CACHE_HOME":  filepath.Join(root, "cache"),
	}
	for name, path := range want {
		require.Equal(t, path, os.Getenv(name), name)
		info, err := os.Stat(path)
		require.NoError(t, err, name)
		require.True(t, info.IsDir(), name)
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm(), name)
	}
	require.Equal(t, "/run/user/1000", os.Getenv("XDG_RUNTIME_DIR"))
}

func TestApplyDevelopmentEnvironment_DevRejectsSandboxSymlink(t *testing.T) {
	worktree := t.TempDir()
	t.Chdir(worktree)
	t.Setenv("ENV", developmentEnvironmentName)
	t.Setenv("HOME", "/home/production")

	devDir := filepath.Join(worktree, ".dev")
	require.NoError(t, os.Mkdir(devDir, 0o755))
	target := t.TempDir()
	require.NoError(t, os.Symlink(target, filepath.Join(devDir, "dumber")))

	err := ApplyDevelopmentEnvironment()
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be a directory")
	require.Equal(t, "/home/production", os.Getenv("HOME"))
}

func TestApplyDevelopmentEnvironment_DevRejectsSandboxChildSymlinks(t *testing.T) {
	for _, child := range []string{"logs", "engines"} {
		t.Run(child, func(t *testing.T) {
			worktree := t.TempDir()
			t.Chdir(worktree)
			t.Setenv("ENV", developmentEnvironmentName)

			root := filepath.Join(worktree, ".dev", "dumber")
			require.NoError(t, os.MkdirAll(root, 0o700))
			require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, child)))

			err := ApplyDevelopmentEnvironment()
			require.Error(t, err)
			require.Contains(t, err.Error(), "must be a directory")
		})
	}
}

func TestApplyDevelopmentEnvironment_DevTightensExistingSandboxPermissions(t *testing.T) {
	worktree := t.TempDir()
	t.Chdir(worktree)
	t.Setenv("ENV", developmentEnvironmentName)

	root := filepath.Join(worktree, ".dev", "dumber")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.Chmod(root, 0o755))

	require.NoError(t, ApplyDevelopmentEnvironment())

	info, err := os.Stat(root)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestApplyDevelopmentEnvironment_ProductionPreservesEnvironment(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("HOME", "/home/production")
	t.Setenv("XDG_CONFIG_HOME", "/production/config")
	t.Setenv("XDG_DATA_HOME", "/production/data")
	t.Setenv("XDG_STATE_HOME", "/production/state")
	t.Setenv("XDG_CACHE_HOME", "/production/cache")

	require.NoError(t, ApplyDevelopmentEnvironment())

	require.Equal(t, "/home/production", os.Getenv("HOME"))
	require.Equal(t, "/production/config", os.Getenv("XDG_CONFIG_HOME"))
	require.Equal(t, "/production/data", os.Getenv("XDG_DATA_HOME"))
	require.Equal(t, "/production/state", os.Getenv("XDG_STATE_HOME"))
	require.Equal(t, "/production/cache", os.Getenv("XDG_CACHE_HOME"))
}
