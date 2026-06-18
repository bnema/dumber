package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeProcessProbe struct {
	alive map[int]bool
}

func (p fakeProcessProbe) IsProcessAlive(_ context.Context, pid int) (bool, error) {
	return p.alive[pid], nil
}

func TestRunSessionCleanup_DoesNotEndOtherActiveBrowserSessionsWithoutLivenessProof(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := repomocks.NewMockSessionRepository(t)
	cleanupUC := usecase.NewCleanupSessionsUseCase(repo, fakeProcessProbe{alive: map[int]bool{1234: true}})
	cfg := config.DefaultConfig()
	logDir := t.TempDir()

	now := time.Now().UTC()
	current := &entity.Session{
		ID:        entity.SessionID("current"),
		Type:      entity.SessionTypeBrowser,
		StartedAt: now,
	}
	livePID := 1234
	live := &entity.Session{
		ID:        entity.SessionID("live-other-process"),
		Type:      entity.SessionTypeBrowser,
		StartedAt: now.Add(-time.Hour),
		ProcessID: &livePID,
	}

	repo.EXPECT().GetRecent(mock.Anything, recentSessionsLimit).Return([]*entity.Session{current, live}, nil).Once()
	repo.EXPECT().DeleteExitedBefore(mock.Anything, mock.MatchedBy(func(time.Time) bool { return true })).Return(int64(0), nil).Once()
	repo.EXPECT().DeleteOldestExited(mock.Anything, cfg.Session.MaxExitedSessions).Return(int64(0), nil).Once()

	runSessionCleanup(ctx, cleanupUC, cfg, logDir, current.ID, nil)
}

func TestRunSessionCleanup_NoPanicOnNilSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := repomocks.NewMockSessionRepository(t)
	cleanupUC := usecase.NewCleanupSessionsUseCase(repo, fakeProcessProbe{})
	cfg := config.DefaultConfig()
	logDir := t.TempDir()

	repo.EXPECT().GetRecent(mock.Anything, recentSessionsLimit).Return([]*entity.Session{nil}, nil).Once()
	repo.EXPECT().DeleteExitedBefore(mock.Anything, mock.MatchedBy(func(time.Time) bool { return true })).Return(int64(0), nil).Once()
	repo.EXPECT().DeleteOldestExited(mock.Anything, cfg.Session.MaxExitedSessions).Return(int64(0), nil).Once()

	require.NotPanics(t, func() {
		runSessionCleanup(ctx, cleanupUC, cfg, logDir, entity.SessionID("current"), nil)
	})
}
