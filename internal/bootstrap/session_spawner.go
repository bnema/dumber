package bootstrap

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/cef"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
)

// NewSessionSpawner wires the session spawner with any engine-specific launch
// environment required by the current browser backend.
func NewSessionSpawner(ctx context.Context, profile runtimeprofile.Profile) port.SessionSpawner {
	var spawnEnv port.SessionSpawnEnvironment
	if profile.Engine == config.EngineTypeCEF {
		spawnEnv = cef.NewSessionSpawnEnvironment(profile)
	}
	return desktop.NewSessionSpawner(ctx, spawnEnv)
}
