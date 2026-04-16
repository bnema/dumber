package cef

import (
	"path/filepath"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

// SessionSpawnEnvironment exposes the engine-specific launch environment needed
// when restoring a session into a freshly spawned browser process.
type SessionSpawnEnvironment struct{}

func (SessionSpawnEnvironment) RootCacheEnvVar() string {
	return CEFRootCachePathEnvVar
}

func (SessionSpawnEnvironment) SessionRootCachePath(sessionID entity.SessionID) string {
	return filepath.Join(DefaultCEFUserDataDir(), "sessions", string(sessionID))
}

var _ port.SessionSpawnEnvironment = SessionSpawnEnvironment{}
