package usecase

import (
	"context"

	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/domain/repository"
)

// ManageSessionUseCase reads session metadata. Browser sessions are recorded and
// ended by BrowserSessionLifetimeUseCase, which keeps their lease in step.
type ManageSessionUseCase struct {
	sessionRepo repository.SessionRepository
}

func NewManageSessionUseCase(sessionRepo repository.SessionRepository) *ManageSessionUseCase {
	return &ManageSessionUseCase{sessionRepo: sessionRepo}
}

func (uc *ManageSessionUseCase) GetActiveSession(ctx context.Context) (*entity.Session, error) {
	return uc.sessionRepo.GetActive(ctx)
}

func (uc *ManageSessionUseCase) GetRecentSessions(ctx context.Context, limit int) ([]*entity.Session, error) {
	return uc.sessionRepo.GetRecent(ctx, limit)
}
