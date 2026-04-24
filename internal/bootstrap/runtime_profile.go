package bootstrap

import (
	"os"
	"path/filepath"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
)

// ResolveRuntimeProfile resolves the current mode+engine profile once in bootstrap.
func ResolveRuntimeProfile(cfg *config.Config) (runtimeprofile.Profile, error) {
	engine := config.EngineTypeWebKit
	if cfg != nil {
		engine = cfg.Engine.ResolveEngineType()
	}

	dirs, err := config.GetXDGDirs()
	if err != nil {
		return runtimeprofile.Profile{}, err
	}

	return runtimeprofile.Resolve(runtimeprofile.ResolveInput{
		Env:    os.Getenv,
		CWD:    os.Getwd,
		Engine: engine,
		Base: runtimeprofile.BasePaths{
			ConfigHome: dirs.ConfigHome,
			DataHome:   dirs.DataHome,
			StateHome:  dirs.StateHome,
			CacheHome:  dirs.CacheHome,
		},
	})
}

// ResolveXDGRuntimeDir returns the shared XDG runtime dir for the current mode.
func ResolveXDGRuntimeDir(profile runtimeprofile.Profile) string {
	if profile.Mode == runtimeprofile.ModeDev {
		if profile.Shared.RootDir != "" {
			return filepath.Join(profile.Shared.RootDir, "runtime")
		}
	}
	if profile.Shared.StateDir != "" {
		return filepath.Join(profile.Shared.StateDir, "runtime")
	}
	return ""
}
