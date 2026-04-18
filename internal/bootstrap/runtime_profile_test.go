package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/stretchr/testify/require"
)

func TestResolveRuntimeProfile_UsesConfigEngineResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Engine.Type = "webkit"
	t.Setenv("DUMBER_ENGINE", "cef")
	t.Setenv("ENV", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	profile, err := ResolveRuntimeProfile(cfg)
	require.NoError(t, err)
	require.Equal(t, "cef", profile.Engine)
}

func TestResolveRuntimeProfile_ProdPreservesSharedPaths(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Engine.Type = config.EngineTypeWebKit
	t.Setenv("ENV", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	profile, err := ResolveRuntimeProfile(cfg)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", "dumber"), profile.Shared.ConfigDir)
	require.Equal(t, filepath.Join(home, ".local", "share", "dumber"), profile.Shared.DataDir)
	require.Equal(t, filepath.Join(home, ".local", "state", "dumber"), profile.Shared.StateDir)
	require.Equal(t, filepath.Join(home, ".cache", "dumber"), profile.Shared.CacheDir)
}

func TestResolveRuntimeProfile_DevUsesEngineSpecificNamespaces(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Engine.Type = config.EngineTypeCEF

	wd := t.TempDir()
	t.Chdir(wd)

	t.Setenv("ENV", "dev")
	profile, err := ResolveRuntimeProfile(cfg)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(wd, ".dev", "dumber", "state"), profile.Shared.StateDir)
	require.Equal(t, filepath.Join(wd, ".dev", "dumber", "engines", "cef", "data"), profile.CEFUserDataDir())
	require.Equal(t, filepath.Join(wd, ".dev", "dumber", "runtime", "cef", "browser-launch.sock"), profile.IPC.BrowserLaunchSocket)
}

func TestResolveXDGRuntimeDir_UsesSharedRuntimeRootInDev(t *testing.T) {
	profile := runtimeprofile.Profile{
		Mode: runtimeprofile.ModeDev,
		Shared: runtimeprofile.SharedPaths{
			RootDir:  "/tmp/project/.dev/dumber",
			StateDir: "/tmp/project/.dev/dumber/state",
		},
	}

	require.Equal(t, "/tmp/project/.dev/dumber/runtime", ResolveXDGRuntimeDir(profile))
}

func TestResolveXDGRuntimeDir_UsesStateRuntimeRootInProd(t *testing.T) {
	profile := runtimeprofile.Profile{
		Mode: runtimeprofile.ModeProd,
		Shared: runtimeprofile.SharedPaths{
			StateDir: "/tmp/dumber-state/dumber",
		},
	}

	require.Equal(t, "/tmp/dumber-state/dumber/runtime", ResolveXDGRuntimeDir(profile))
}
