package usecase

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// CleanupSessionsUseCase handles automatic cleanup of old exited sessions.
type CleanupSessionsUseCase struct {
	sessionRepo repository.SessionRepository
}

// NewCleanupSessionsUseCase creates a new CleanupSessionsUseCase.
func NewCleanupSessionsUseCase(sessionRepo repository.SessionRepository) *CleanupSessionsUseCase {
	return &CleanupSessionsUseCase{
		sessionRepo: sessionRepo,
	}
}

// CleanupSessionsInput contains the cleanup configuration.
type CleanupSessionsInput struct {
	// MaxExitedSessions is the maximum number of exited sessions to keep.
	// Sessions beyond this count will be deleted (oldest first).
	MaxExitedSessions int

	// MaxExitedSessionAgeDays is the maximum age in days for exited sessions.
	// Sessions older than this will be deleted.
	// A value of 0 disables age-based cleanup.
	MaxExitedSessionAgeDays int
}

// CleanupSessionsOutput contains the cleanup results.
type CleanupSessionsOutput struct {
	// DeletedByAge is the number of sessions deleted due to age.
	DeletedByAge int64

	// DeletedByCount is the number of sessions deleted due to count limit.
	DeletedByCount int64

	// TotalDeleted is the total number of sessions deleted.
	TotalDeleted int64
}

// Execute cleans up old exited sessions based on the provided configuration.
// It first deletes sessions older than MaxExitedSessionAgeDays,
// then enforces the MaxExitedSessions count limit.
func (uc *CleanupSessionsUseCase) Execute(ctx context.Context, input CleanupSessionsInput) (CleanupSessionsOutput, error) {
	log := logging.FromContext(ctx)
	output := CleanupSessionsOutput{}

	// 1. Delete by age first (if enabled)
	if input.MaxExitedSessionAgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -input.MaxExitedSessionAgeDays)
		deleted, err := uc.sessionRepo.DeleteExitedBefore(ctx, cutoff)
		if err != nil {
			log.Warn().Err(err).Msg("failed to delete sessions by age")
		} else {
			output.DeletedByAge = deleted
			if deleted > 0 {
				log.Info().
					Int64("deleted", deleted).
					Int("max_age_days", input.MaxExitedSessionAgeDays).
					Msg("cleaned up old sessions by age")
			}
		}
	}

	// 2. Enforce count limit on remaining sessions
	if input.MaxExitedSessions >= 0 {
		deleted, err := uc.sessionRepo.DeleteOldestExited(ctx, input.MaxExitedSessions)
		if err != nil {
			log.Warn().Err(err).Msg("failed to delete sessions by count")
		} else {
			output.DeletedByCount = deleted
			if deleted > 0 {
				log.Info().
					Int64("deleted", deleted).
					Int("max_count", input.MaxExitedSessions).
					Msg("cleaned up old sessions by count")
			}
		}
	}

	output.TotalDeleted = output.DeletedByAge + output.DeletedByCount

	if output.TotalDeleted > 0 {
		log.Info().
			Int64("total_deleted", output.TotalDeleted).
			Int64("by_age", output.DeletedByAge).
			Int64("by_count", output.DeletedByCount).
			Msg("session cleanup completed")
	}

	return output, nil
}
