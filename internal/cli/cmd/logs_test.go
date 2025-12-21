package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/stretchr/testify/require"
)

type fakeSessionMgr struct {
	sessions []*entity.Session
	err      error
}

func (f *fakeSessionMgr) GetRecentSessions(_ context.Context, _ int) ([]*entity.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sessions, nil
}

func TestGetSessionsMerged_DBAndFS(t *testing.T) {
	logDir := t.TempDir()

	s1 := &entity.Session{ID: entity.SessionID("20251217_205106_a7b3"), Type: entity.SessionTypeBrowser, StartedAt: time.Date(2025, 12, 17, 20, 51, 6, 0, time.UTC)}
	s2 := &entity.Session{ID: entity.SessionID("20251218_205106_bbbb"), Type: entity.SessionTypeBrowser, StartedAt: time.Date(2025, 12, 18, 20, 51, 6, 0, time.UTC)}

	require.NoError(t, os.WriteFile(filepath.Join(logDir, logging.SessionFilename(string(s1.ID))), []byte("hi\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, logging.SessionFilename("legacy_only")), []byte("legacy\n"), 0o600))

	mgr := &fakeSessionMgr{sessions: []*entity.Session{s1, s2}}

	merged, err := getSessionsMerged(context.Background(), mgr, logDir)
	require.NoError(t, err)
	require.Len(t, merged, 3)

	byID := map[string]SessionInfo{}
	for _, s := range merged {
		byID[s.SessionID] = s
	}

	require.True(t, byID[string(s1.ID)].FromDB)
	require.True(t, byID[string(s2.ID)].FromDB)
	require.False(t, byID["legacy_only"].FromDB)
}

func TestGetSessionsMerged_MissingTableFallsBack(t *testing.T) {
	logDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(logDir, logging.SessionFilename("legacy_only")), []byte("legacy\n"), 0o600))

	mgr := &fakeSessionMgr{err: fmt.Errorf("no such table: sessions")}
	merged, err := getSessionsMerged(context.Background(), mgr, logDir)
	require.NoError(t, err)
	require.Len(t, merged, 1)
	require.Equal(t, "legacy_only", merged[0].SessionID)
}

func TestFindSession_DBShortIDMatch(t *testing.T) {
	logDir := t.TempDir()

	s1 := &entity.Session{ID: entity.SessionID("20251217_205106_a7b3"), Type: entity.SessionTypeBrowser, StartedAt: time.Date(2025, 12, 17, 20, 51, 6, 0, time.UTC)}
	mgr := &fakeSessionMgr{sessions: []*entity.Session{s1}}

	info, err := findSession(context.Background(), mgr, logDir, "a7b3")
	require.NoError(t, err)
	require.Equal(t, string(s1.ID), info.SessionID)
	require.Equal(t, "a7b3", info.ShortID)
}
