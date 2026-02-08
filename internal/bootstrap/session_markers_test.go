package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionMarkers_WriteStartupAndShutdown(t *testing.T) {
	lockDir := t.TempDir()
	sessionID := "s123"
	startedAt := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(3 * time.Minute)

	err := writeStartupMarker(lockDir, sessionID, startedAt)
	require.NoError(t, err)

	startupContent, err := os.ReadFile(startupMarkerPath(lockDir, sessionID))
	require.NoError(t, err)
	assert.Contains(t, string(startupContent), startedAt.Format(time.RFC3339Nano))
	assert.Contains(t, string(startupContent), "pid=")
	assert.Contains(t, string(startupContent), "ppid=")

	err = writeShutdownMarker(lockDir, sessionID, endedAt)
	require.NoError(t, err)

	shutdownContent, err := os.ReadFile(shutdownMarkerPath(lockDir, sessionID))
	require.NoError(t, err)
	assert.Contains(t, string(shutdownContent), endedAt.Format(time.RFC3339Nano))
	assert.Contains(t, string(shutdownContent), "started_at="+startedAt.Format(time.RFC3339Nano))
	assert.Contains(t, string(shutdownContent), "pid=")
	assert.Contains(t, string(shutdownContent), "ppid=")
}

func TestSessionMarkers_MarkAbruptExits(t *testing.T) {
	lockDir := t.TempDir()
	detectedAt := time.Date(2026, 2, 7, 11, 0, 0, 0, time.UTC)

	require.NoError(t, writeStartupMarker(lockDir, "abrupt-a", detectedAt.Add(-5*time.Minute)))
	require.NoError(t, writeStartupMarker(lockDir, "clean-b", detectedAt.Add(-4*time.Minute)))
	require.NoError(t, writeShutdownMarker(lockDir, "clean-b", detectedAt.Add(-1*time.Minute)))

	abruptSessions, err := markAbruptExits(lockDir, detectedAt, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"abrupt-a"}, abruptSessions)

	abruptMarkerData, err := os.ReadFile(abruptMarkerPath(lockDir, "abrupt-a"))
	require.NoError(t, err)
	assert.Contains(t, string(abruptMarkerData), "detected_at="+detectedAt.Format(time.RFC3339Nano))
	assert.Contains(t, string(abruptMarkerData), "started_at=")
	assert.Contains(t, string(abruptMarkerData), "pid=")
	assert.Contains(t, string(abruptMarkerData), "ppid=")
	assert.FileExists(t, filepath.Join(lockDir, "session_clean-b.shutdown.marker"))
}

func TestSessionMarkers_MarkAbruptExitsIdempotent(t *testing.T) {
	lockDir := t.TempDir()
	detectedAt := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)

	require.NoError(t, writeStartupMarker(lockDir, "abrupt-c", detectedAt.Add(-10*time.Minute)))

	first, err := markAbruptExits(lockDir, detectedAt, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"abrupt-c"}, first)

	second, err := markAbruptExits(lockDir, detectedAt.Add(1*time.Minute), nil)
	require.NoError(t, err)
	assert.Empty(t, second)
}
