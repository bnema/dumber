package cef

import (
	"path/filepath"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
)

// SessionSpawnEnvironment exposes the engine-specific launch environment needed
// when restoring a session into a freshly spawned browser process.
type SessionSpawnEnvironment struct {
	profile runtimeprofile.Profile
}

func NewSessionSpawnEnvironment(profile runtimeprofile.Profile) SessionSpawnEnvironment {
	return SessionSpawnEnvironment{profile: profile}
}

func (SessionSpawnEnvironment) RootCacheEnvVar() string {
	return CEFRootCachePathEnvVar
}

func (e SessionSpawnEnvironment) SessionRootCachePath(sessionID entity.SessionID) string {
	return filepath.Join(e.profile.CEFUserDataDir(), "sessions", string(sessionID))
}

var _ port.SessionSpawnEnvironment = SessionSpawnEnvironment{}
