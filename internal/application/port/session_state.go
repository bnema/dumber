package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// TabListProvider provides access to the current tab list state.
// Implemented by the UI layer to allow the snapshot service to read state.
//
// Deprecated: use WindowStateProvider for v2 window-scoped snapshots.
type TabListProvider interface {
	// GetTabList returns the current tab list state.
	GetTabList() *entity.TabList
	// GetSessionID returns the current session ID.
	GetSessionID() entity.SessionID
}

// WindowStateProvider provides window-scoped browser state for v2 snapshots.
// Implemented by the UI layer and consumed by the snapshot service.
type WindowStateProvider interface {
	// GetWindowSnapshotState returns the ordered window tab lists and active window index atomically.
	GetWindowSnapshotState() ([]entity.WindowTabListState, int)
	// GetWindowTabLists returns the ordered list of windows with their tab lists.
	// For consistent reads alongside GetActiveWindowIndex, prefer GetWindowSnapshotState.
	GetWindowTabLists() []entity.WindowTabListState
	// GetActiveWindowIndex returns the index of the currently focused window.
	// For consistent reads alongside GetWindowTabLists, prefer GetWindowSnapshotState.
	GetActiveWindowIndex() int
	// GetSessionID returns the current session ID.
	GetSessionID() entity.SessionID
}

// SessionSpawner spawns a new dumber instance for resurrection.
type SessionSpawner interface {
	// SpawnWithSession starts a new dumber instance to restore a session.
	SpawnWithSession(sessionID entity.SessionID) error
}

// SessionSpawnEnvironment provides engine-specific environment overrides for
// spawned restore-session processes. Implementations are optional: engines that
// do not need extra launch environment can leave this nil.
type SessionSpawnEnvironment interface {
	// RootCacheEnvVar returns the environment variable used to override the
	// engine's root cache/data directory.
	RootCacheEnvVar() string
	// SessionRootCachePath returns the engine-specific root cache/data path for
	// the provided restored session.
	SessionRootCachePath(sessionID entity.SessionID) string
}

// SnapshotService manages debounced session-state snapshots.
type SnapshotService interface {
	// Start begins the snapshot service with the given context.
	Start(ctx context.Context)
	// Stop stops the service and saves the final state.
	Stop(ctx context.Context) error
	// SetReady marks the service as ready to save snapshots.
	// Call this after the session has been persisted to the database.
	SetReady()
	// MarkDirty signals that session state has changed and should be saved.
	MarkDirty()
	// SaveNow forces an immediate save (e.g. for shutdown).
	SaveNow(ctx context.Context) error
}
