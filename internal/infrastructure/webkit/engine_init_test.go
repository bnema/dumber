package webkit

import (
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/stretchr/testify/require"
)

func TestEngineBuildContextOptions_UsesResolvedWebKitPaths(t *testing.T) {
	profile := runtimeprofile.Profile{
		Mode:   runtimeprofile.ModeDev,
		Engine: "webkit",
		EnginePaths: runtimeprofile.EnginePaths{
			RootDir: filepath.Join(t.TempDir(), "engines", "webkit"),
		},
	}

	wkOpts := engineBuildContextOptions(port.EngineOptions{}, profile, WebKitEngineConfig{}, &config.ResolvedPerformanceSettings{})
	require.Equal(t, profile.WebKitDataDir(), wkOpts.DataDir)
	require.Equal(t, profile.WebKitCacheDir(), wkOpts.CacheDir)
}

func TestEngineBuildContextOptions_PrefersExplicitEngineOptionsDirs(t *testing.T) {
	profile := runtimeprofile.Profile{
		Mode:   runtimeprofile.ModeDev,
		Engine: "webkit",
		EnginePaths: runtimeprofile.EnginePaths{
			RootDir: filepath.Join(t.TempDir(), "engines", "webkit"),
		},
	}

	wkOpts := engineBuildContextOptions(port.EngineOptions{DataDir: "/tmp/data", CacheDir: "/tmp/cache"}, profile, WebKitEngineConfig{}, &config.ResolvedPerformanceSettings{})
	require.Equal(t, "/tmp/data", wkOpts.DataDir)
	require.Equal(t, "/tmp/cache", wkOpts.CacheDir)
}
