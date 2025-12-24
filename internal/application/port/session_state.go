package port

import "github.com/bnema/dumber/internal/domain/entity"

// TabListProvider provides access to the current tab list state.
// Implemented by the UI layer to allow the snapshot service to read state.
type TabListProvider interface {
	// GetTabList returns the current tab list state.
	GetTabList() *entity.TabList
	// GetSessionID returns the current session ID.
	GetSessionID() entity.SessionID
}

// SessionSpawner spawns a new dumber instance for resurrection.
type SessionSpawner interface {
	// SpawnWithSession starts a new dumber instance to restore a session.
	SpawnWithSession(sessionID entity.SessionID) error
}
