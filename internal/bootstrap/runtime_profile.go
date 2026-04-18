package bootstrap

import (
	"os"

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
