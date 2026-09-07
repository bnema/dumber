package cef

import (
	"sync"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivityTrackerStartsQuiescent(t *testing.T) {
	tracker := NewRuntimeActivityTracker()
	require.True(t, tracker.Snapshot().Quiescent())
}

func TestRuntimeActivityTrackerCreationLifecycle(t *testing.T) {
	tracker := NewRuntimeActivityTracker()
	tracker.NoteCreationAccepted()
	require.Equal(t, 1, tracker.Snapshot().PendingCreations)
	require.False(t, tracker.Snapshot().Quiescent())

	tracker.NoteCreationResolved()
	require.True(t, tracker.Snapshot().Quiescent())

	// Over-resolution clamps instead of going negative.
	tracker.NoteCreationResolved()
	require.Equal(t, 0, tracker.Snapshot().PendingCreations)
}

func TestRuntimeActivityTrackerViewLifecycle(t *testing.T) {
	tracker := NewRuntimeActivityTracker()
	tracker.NoteViewRegistered()
	tracker.NoteViewRegistered()
	require.Equal(t, 2, tracker.Snapshot().ActiveViews)

	tracker.NoteViewCloseStarted()
	snapshot := tracker.Snapshot()
	require.Equal(t, 1, snapshot.ActiveViews)
	require.Equal(t, 1, snapshot.PendingCleanup)
	require.False(t, snapshot.Quiescent())

	tracker.NoteCleanupCompleted()
	tracker.NoteViewCloseStarted()
	tracker.NoteCleanupCompleted()
	require.True(t, tracker.Snapshot().Quiescent())
}

func TestRuntimeActivityTrackerDownloads(t *testing.T) {
	tracker := NewRuntimeActivityTracker()

	// Same filename on two browsers are distinct owners.
	tracker.NoteDownloadStarted(7, 100)
	tracker.NoteDownloadStarted(9, 100)
	require.Equal(t, 2, tracker.Snapshot().ActiveDownloads)

	// Progress without a start never creates work.
	tracker.NoteDownloadProgress(7, 999)
	require.Equal(t, 2, tracker.Snapshot().ActiveDownloads)

	// Terminal resolves only the exact owner sharing the CEF download ID.
	tracker.NoteDownloadTerminal(7, 100)
	require.Equal(t, 1, tracker.Snapshot().ActiveDownloads)

	tracker.NoteDownloadTerminal(9, 100)
	require.Equal(t, 0, tracker.Snapshot().ActiveDownloads)

	// Duplicate terminals are idempotent; unknown owners change nothing.
	tracker.NoteDownloadTerminal(9, 100)
	tracker.NoteDownloadTerminal(7, 424242)
	require.True(t, tracker.Snapshot().Quiescent())
}

func TestRuntimeActivityTrackerDropBrowser(t *testing.T) {
	tracker := NewRuntimeActivityTracker()
	tracker.NoteDownloadStarted(7, 100)
	tracker.NoteDownloadStarted(9, 200)
	tracker.DropBrowser(7)
	snapshot := tracker.Snapshot()
	require.Equal(t, 1, snapshot.ActiveDownloads)

	tracker.NoteDownloadTerminal(9, 200)
	require.True(t, tracker.Snapshot().Quiescent())

	// Dropping an unknown browser notifies nobody and changes nothing.
	tracker.DropBrowser(12345)
	require.True(t, tracker.Snapshot().Quiescent())
}

func TestRuntimeActivityTrackerSubscribeSequenced(t *testing.T) {
	tracker := NewRuntimeActivityTracker()
	var mu sync.Mutex
	var seen []port.RuntimeActivitySnapshot
	unsubscribe := tracker.Subscribe(func(snapshot port.RuntimeActivitySnapshot) {
		mu.Lock()
		seen = append(seen, snapshot)
		mu.Unlock()
	})
	// Initial delivery is synchronous and quiescent.
	mu.Lock()
	require.Len(t, seen, 1)
	require.True(t, seen[0].Quiescent())
	mu.Unlock()

	tracker.NoteCreationAccepted()
	tracker.NoteCreationResolved()
	unsubscribe()
	tracker.NoteViewRegistered()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, seen, 3, "unsubscribe stops delivery")
	require.Equal(t, 1, seen[1].PendingCreations)
	require.True(t, seen[2].Quiescent())
}
