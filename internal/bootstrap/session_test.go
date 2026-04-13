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

func TestRunSessionCleanup_SkipsCurrentActiveSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := repomocks.NewMockSessionRepository(t)
	sessionUC := usecase.NewManageSessionUseCase(repo, nil)
	cleanupUC := usecase.NewCleanupSessionsUseCase(repo)
	cfg := config.DefaultConfig()

	now := time.Now().UTC()
	current := &entity.Session{
		ID:        entity.SessionID("current"),
		Type:      entity.SessionTypeBrowser,
		StartedAt: now,
	}
	stale := &entity.Session{
		ID:        entity.SessionID("stale"),
		Type:      entity.SessionTypeBrowser,
		StartedAt: now.Add(-time.Hour),
	}

	repo.EXPECT().GetRecent(mock.Anything, recentSessionsLimit).Return([]*entity.Session{current, stale}, nil).Once()
	repo.EXPECT().MarkEnded(mock.Anything, stale.ID, mock.MatchedBy(func(t time.Time) bool {
		diff := t.Sub(now)
		if diff < 0 {
			diff = -diff
		}
		return diff < 2*time.Second
	})).Return(nil).Once()
	repo.EXPECT().DeleteExitedBefore(mock.Anything, mock.MatchedBy(func(time.Time) bool { return true })).Return(int64(0), nil).Once()
	repo.EXPECT().DeleteOldestExited(mock.Anything, cfg.Session.MaxExitedSessions).Return(int64(0), nil).Once()

	runSessionCleanup(ctx, sessionUC, cleanupUC, cfg, current.ID, nil)
}

func TestRunSessionCleanup_NoPanicOnNilSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := repomocks.NewMockSessionRepository(t)
	sessionUC := usecase.NewManageSessionUseCase(repo, nil)
	cleanupUC := usecase.NewCleanupSessionsUseCase(repo)
	cfg := config.DefaultConfig()

	repo.EXPECT().GetRecent(mock.Anything, recentSessionsLimit).Return([]*entity.Session{nil}, nil).Once()
	repo.EXPECT().DeleteExitedBefore(mock.Anything, mock.MatchedBy(func(time.Time) bool { return true })).Return(int64(0), nil).Once()
	repo.EXPECT().DeleteOldestExited(mock.Anything, cfg.Session.MaxExitedSessions).Return(int64(0), nil).Once()

	require.NotPanics(t, func() {
		runSessionCleanup(ctx, sessionUC, cleanupUC, cfg, entity.SessionID("current"), nil)
	})
}
