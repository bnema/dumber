package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/cef"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
)

// NewSessionSpawner wires the session spawner with any engine-specific launch
// environment required by the current browser backend.
func NewSessionSpawner(ctx context.Context, profile runtimeprofile.Profile) port.SessionSpawner {
	if strings.HasPrefix(profile.InstanceAppIDSuffix, "ephemeral-") {
		return ephemeralSessionSpawner{}
	}
	var spawnEnv port.SessionSpawnEnvironment
	if profile.Engine == config.EngineTypeCEF {
		spawnEnv = cef.NewSessionSpawnEnvironment(profile.CEFUserDataDir())
	}
	return desktop.NewSessionSpawner(ctx, spawnEnv)
}

type ephemeralSessionSpawner struct{}

func (ephemeralSessionSpawner) SpawnWithSession(entity.SessionID) error {
	return fmt.Errorf("detached session restore is unavailable with --ephemeral; keep the session in the current window or use --profile")
}
