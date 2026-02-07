package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifySessionExitFromMarkers_CleanExit(t *testing.T) {
	lockDir := t.TempDir()
	sessionID := "clean-1"
	startedAt := time.Date(2026, 2, 7, 9, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(2 * time.Minute)

	require.NoError(t, writeStartupMarker(lockDir, sessionID, startedAt))
	require.NoError(t, writeShutdownMarker(lockDir, sessionID, endedAt))

	classification, err := ClassifySessionExitFromMarkers(lockDir, sessionID)
	require.NoError(t, err)
	assert.Equal(t, SessionExitCleanExit, classification.Class)
	assert.Equal(t, "marker-confirmed", classification.Inference)
	assert.Equal(t, "shutdown marker present", classification.Reason)
	require.NotNil(t, classification.StartupAt)
	assert.Equal(t, startedAt, *classification.StartupAt)
	require.NotNil(t, classification.ShutdownAt)
	assert.Equal(t, endedAt, *classification.ShutdownAt)
	assert.Nil(t, classification.AbruptDetectedAt)
}

func TestClassifySessionExitFromMarkers_AbruptMarker(t *testing.T) {
	lockDir := t.TempDir()
	sessionID := "abrupt-1"
	startedAt := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC)
	detectedAt := startedAt.Add(5 * time.Minute)

	require.NoError(t, writeStartupMarker(lockDir, sessionID, startedAt))
	require.NoError(t, os.WriteFile(
		abruptMarkerPath(lockDir, sessionID),
		[]byte("detected_at="+detectedAt.Format(time.RFC3339Nano)+"\n"),
		markerFilePerm,
	))

	classification, err := ClassifySessionExitFromMarkers(lockDir, sessionID)
	require.NoError(t, err)
	assert.Equal(t, SessionExitMainProcessCrashOrAbrupt, classification.Class)
	assert.Equal(t, "marker-confirmed", classification.Inference)
	assert.Equal(t, "abrupt marker present and no shutdown marker", classification.Reason)
	require.NotNil(t, classification.StartupAt)
	assert.Equal(t, startedAt, *classification.StartupAt)
	assert.Nil(t, classification.ShutdownAt)
	require.NotNil(t, classification.AbruptDetectedAt)
	assert.Equal(t, detectedAt, *classification.AbruptDetectedAt)
}

func TestClassifySessionExitFromMarkers_ExternalKillOrOOMInferred(t *testing.T) {
	lockDir := t.TempDir()
	sessionID := "inferred-1"
	startedAt := time.Date(2026, 2, 7, 11, 0, 0, 0, time.UTC)

	require.NoError(t, writeStartupMarker(lockDir, sessionID, startedAt))

	classification, err := ClassifySessionExitFromMarkers(lockDir, sessionID)
	require.NoError(t, err)
	assert.Equal(t, SessionExitExternalKillOrOOMInferred, classification.Class)
	assert.Equal(t, "best-effort", classification.Inference)
	assert.Equal(t, "startup marker present without shutdown/abrupt markers", classification.Reason)
	require.NotNil(t, classification.StartupAt)
	assert.Equal(t, startedAt, *classification.StartupAt)
	assert.Nil(t, classification.ShutdownAt)
	assert.Nil(t, classification.AbruptDetectedAt)
}

func TestClassifySessionExitFromMarkers_RequiresInputs(t *testing.T) {
	_, err := ClassifySessionExitFromMarkers("", "s1")
	require.Error(t, err)

	_, err = ClassifySessionExitFromMarkers(t.TempDir(), "")
	require.Error(t, err)
}

func TestBuildSessionExitReport(t *testing.T) {
	lockDir := t.TempDir()

	require.NoError(t, writeStartupMarker(lockDir, "clean-r", time.Date(2026, 2, 7, 8, 0, 0, 0, time.UTC)))
	require.NoError(t, writeShutdownMarker(lockDir, "clean-r", time.Date(2026, 2, 7, 8, 1, 0, 0, time.UTC)))

	require.NoError(t, writeStartupMarker(lockDir, "abrupt-r", time.Date(2026, 2, 7, 8, 5, 0, 0, time.UTC)))
	require.NoError(t, os.WriteFile(
		abruptMarkerPath(lockDir, "abrupt-r"),
		[]byte("detected_at=2026-02-07T08:08:00Z\n"),
		markerFilePerm,
	))

	require.NoError(t, writeStartupMarker(lockDir, "inferred-r", time.Date(2026, 2, 7, 8, 10, 0, 0, time.UTC)))

	// Unknown marker type should not be included.
	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "session_unknown.custom.marker"), []byte("x"), markerFilePerm))

	report, err := BuildSessionExitReport(lockDir)
	require.NoError(t, err)
	require.Len(t, report, 3)

	got := map[string]SessionExitClass{}
	for _, item := range report {
		got[item.SessionID] = item.Class
	}

	assert.Equal(t, SessionExitCleanExit, got["clean-r"])
	assert.Equal(t, SessionExitMainProcessCrashOrAbrupt, got["abrupt-r"])
	assert.Equal(t, SessionExitExternalKillOrOOMInferred, got["inferred-r"])
}
