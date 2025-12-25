package usecase_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLastRestorableSessionUseCase_NoSessions(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return([]*entity.Session{}, nil)

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, "")

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: "current",
	})

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Empty(t, output.SessionID)
	assert.Nil(t, output.State)
}

func TestGetLastRestorableSessionUseCase_OnlyCurrentSession(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	currentID := entity.SessionID("20251225_120000_curr")
	endedAt := time.Now()

	sessions := []*entity.Session{
		{
			ID:        currentID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-1 * time.Hour),
			EndedAt:   &endedAt,
		},
	}

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return(sessions, nil)

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, "")

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: currentID,
	})

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Empty(t, output.SessionID)
}

func TestGetLastRestorableSessionUseCase_SkipsCLISessions(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	endedAt := time.Now()
	sessions := []*entity.Session{
		{
			ID:        "20251225_120000_cli1",
			Type:      entity.SessionTypeCLI, // Not a browser session
			StartedAt: time.Now().Add(-1 * time.Hour),
			EndedAt:   &endedAt,
		},
	}

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return(sessions, nil)

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, "")

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: "current",
	})

	require.NoError(t, err)
	assert.Empty(t, output.SessionID)
}

func TestGetLastRestorableSessionUseCase_SkipsActiveWithLock(t *testing.T) {
	ctx := testContext()

	// Create temp dir for lock files
	lockDir := t.TempDir()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	activeID := entity.SessionID("20251225_120000_actv")

	// Create lock file to simulate running session
	lockPath := filepath.Join(lockDir, "session_"+string(activeID)+".lock")
	f, err := os.Create(lockPath)
	require.NoError(t, err)
	f.Close()

	sessions := []*entity.Session{
		{
			ID:        activeID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-1 * time.Hour),
			EndedAt:   nil, // Active (no ended_at)
		},
	}

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return(sessions, nil)

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, lockDir)

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: "current",
	})

	require.NoError(t, err)
	assert.Empty(t, output.SessionID)
}

func TestGetLastRestorableSessionUseCase_SkipsNoSnapshot(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	endedAt := time.Now()
	sessionID := entity.SessionID("20251225_120000_nosn")

	sessions := []*entity.Session{
		{
			ID:        sessionID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-1 * time.Hour),
			EndedAt:   &endedAt,
		},
	}

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return(sessions, nil)
	stateRepo.EXPECT().GetSnapshot(ctx, sessionID).Return(nil, nil) // No snapshot

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, "")

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: "current",
	})

	require.NoError(t, err)
	assert.Empty(t, output.SessionID)
}

func TestGetLastRestorableSessionUseCase_SkipsEmptyTabs(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	endedAt := time.Now()
	sessionID := entity.SessionID("20251225_120000_emty")

	sessions := []*entity.Session{
		{
			ID:        sessionID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-1 * time.Hour),
			EndedAt:   &endedAt,
		},
	}

	emptyState := &entity.SessionState{
		Version:   entity.SessionStateVersion,
		SessionID: sessionID,
		Tabs:      []entity.TabSnapshot{}, // No tabs
		SavedAt:   time.Now(),
	}

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return(sessions, nil)
	stateRepo.EXPECT().GetSnapshot(ctx, sessionID).Return(emptyState, nil)

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, "")

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: "current",
	})

	require.NoError(t, err)
	assert.Empty(t, output.SessionID)
}

func TestGetLastRestorableSessionUseCase_ReturnsEndedSession(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	endedAt := time.Now()
	sessionID := entity.SessionID("20251225_120000_good")

	sessions := []*entity.Session{
		{
			ID:        sessionID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-1 * time.Hour),
			EndedAt:   &endedAt, // Gracefully ended
		},
	}

	validState := &entity.SessionState{
		Version:   entity.SessionStateVersion,
		SessionID: sessionID,
		Tabs: []entity.TabSnapshot{
			{
				ID:   "tab-1",
				Name: "Test Tab",
				Workspace: entity.WorkspaceSnapshot{
					Root: &entity.PaneNodeSnapshot{
						Pane: &entity.PaneSnapshot{
							ID:    "pane-1",
							URI:   "https://example.com",
							Title: "Example",
						},
					},
				},
			},
		},
		SavedAt: time.Now(),
	}

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return(sessions, nil)
	stateRepo.EXPECT().GetSnapshot(ctx, sessionID).Return(validState, nil)

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, "")

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: "current",
	})

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, sessionID, output.SessionID)
	assert.Equal(t, validState, output.State)
}

func TestGetLastRestorableSessionUseCase_ReturnsCrashedSession(t *testing.T) {
	ctx := testContext()

	// Empty lock dir (no lock files)
	lockDir := t.TempDir()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	sessionID := entity.SessionID("20251225_120000_crsh")

	// Session with no ended_at (crashed) but no lock file
	sessions := []*entity.Session{
		{
			ID:        sessionID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-1 * time.Hour),
			EndedAt:   nil, // Not ended - simulates crash
		},
	}

	validState := &entity.SessionState{
		Version:   entity.SessionStateVersion,
		SessionID: sessionID,
		Tabs: []entity.TabSnapshot{
			{
				ID:   "tab-1",
				Name: "Crashed Tab",
				Workspace: entity.WorkspaceSnapshot{
					Root: &entity.PaneNodeSnapshot{
						Pane: &entity.PaneSnapshot{
							ID:    "pane-1",
							URI:   "https://example.com",
							Title: "Example",
						},
					},
				},
			},
		},
		SavedAt: time.Now(),
	}

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return(sessions, nil)
	stateRepo.EXPECT().GetSnapshot(ctx, sessionID).Return(validState, nil)

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, lockDir)

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: "current",
	})

	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, sessionID, output.SessionID)
	assert.Equal(t, validState, output.State)
}

func TestGetLastRestorableSessionUseCase_ReturnsMostRecent(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	endedAt := time.Now()
	recentID := entity.SessionID("20251225_140000_new1")
	olderID := entity.SessionID("20251225_120000_old1")

	// Sessions ordered by started_at DESC (most recent first)
	sessions := []*entity.Session{
		{
			ID:        recentID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-1 * time.Hour),
			EndedAt:   &endedAt,
		},
		{
			ID:        olderID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-2 * time.Hour),
			EndedAt:   &endedAt,
		},
	}

	recentState := &entity.SessionState{
		Version:   entity.SessionStateVersion,
		SessionID: recentID,
		Tabs: []entity.TabSnapshot{
			{ID: "tab-1", Name: "Recent Tab"},
		},
		SavedAt: time.Now(),
	}

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return(sessions, nil)
	stateRepo.EXPECT().GetSnapshot(ctx, recentID).Return(recentState, nil)
	// Note: GetSnapshot for olderID should NOT be called since we return first match

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, "")

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: "current",
	})

	require.NoError(t, err)
	assert.Equal(t, recentID, output.SessionID)
}

func TestGetLastRestorableSessionUseCase_SkipsFirstIfNoSnapshot(t *testing.T) {
	ctx := testContext()

	sessionRepo := repomocks.NewMockSessionRepository(t)
	stateRepo := repomocks.NewMockSessionStateRepository(t)

	endedAt := time.Now()
	noSnapID := entity.SessionID("20251225_140000_nosn")
	goodID := entity.SessionID("20251225_120000_good")

	sessions := []*entity.Session{
		{
			ID:        noSnapID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-1 * time.Hour),
			EndedAt:   &endedAt,
		},
		{
			ID:        goodID,
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-2 * time.Hour),
			EndedAt:   &endedAt,
		},
	}

	goodState := &entity.SessionState{
		Version:   entity.SessionStateVersion,
		SessionID: goodID,
		Tabs: []entity.TabSnapshot{
			{ID: "tab-1", Name: "Good Tab"},
		},
		SavedAt: time.Now(),
	}

	sessionRepo.EXPECT().GetRecent(ctx, 10).Return(sessions, nil)
	stateRepo.EXPECT().GetSnapshot(ctx, noSnapID).Return(nil, nil) // No snapshot
	stateRepo.EXPECT().GetSnapshot(ctx, goodID).Return(goodState, nil)

	uc := usecase.NewGetLastRestorableSessionUseCase(sessionRepo, stateRepo, "")

	output, err := uc.Execute(ctx, usecase.GetLastRestorableSessionInput{
		ExcludeSessionID: "current",
	})

	require.NoError(t, err)
	assert.Equal(t, goodID, output.SessionID)
}
