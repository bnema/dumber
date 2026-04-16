package bootstrap

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/cef"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
)

// NewSessionSpawner wires the session spawner with any engine-specific launch
// environment required by the current browser backend.
func NewSessionSpawner(ctx context.Context, cfg *config.Config) port.SessionSpawner {
	var spawnEnv port.SessionSpawnEnvironment
	if cfg != nil && cfg.Engine.ResolveEngineType() == config.EngineTypeCEF {
		spawnEnv = cef.SessionSpawnEnvironment{}
	}
	return desktop.NewSessionSpawner(ctx, spawnEnv)
}
