package usecase

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// CleanupSessionsUseCase handles automatic cleanup of old exited sessions.
type CleanupSessionsUseCase struct {
	sessionRepo  repository.SessionRepository
	processProbe port.SessionProcessProbe
}

// NewCleanupSessionsUseCase creates a new CleanupSessionsUseCase.
func NewCleanupSessionsUseCase(
	sessionRepo repository.SessionRepository,
	processProbe port.SessionProcessProbe,
) *CleanupSessionsUseCase {
	return &CleanupSessionsUseCase{
		sessionRepo:  sessionRepo,
		processProbe: processProbe,
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

type ReconcileActiveBrowserSessionsInput struct {
	CurrentSessionID entity.SessionID
	RecentLimit      int
	EndedAt          time.Time
}

type ReconcileActiveBrowserSessionsOutput struct {
	EndedDeadSessions int64
}

// ReconcileActiveBrowserSessions ends only active browser sessions whose recorded process is proven dead.
func (uc *CleanupSessionsUseCase) ReconcileActiveBrowserSessions(
	ctx context.Context,
	input ReconcileActiveBrowserSessionsInput,
) (ReconcileActiveBrowserSessionsOutput, error) {
	log := logging.FromContext(ctx)
	output := ReconcileActiveBrowserSessionsOutput{}
	if uc.processProbe == nil {
		return output, nil
	}
	if input.RecentLimit <= 0 {
		input.RecentLimit = 20
	}
	endedAt := input.EndedAt
	if endedAt.IsZero() {
		endedAt = time.Now()
	}

	recent, err := uc.sessionRepo.GetRecent(ctx, input.RecentLimit)
	if err != nil {
		return output, err
	}
	for _, s := range recent {
		if s == nil || s.ID == input.CurrentSessionID || s.Type != entity.SessionTypeBrowser || !s.IsActive() {
			continue
		}
		if s.ProcessID == nil {
			log.Debug().Str("session_id", string(s.ID)).Msg("skipping active session without process id")
			continue
		}
		alive, probeErr := uc.processProbe.IsProcessAlive(ctx, *s.ProcessID)
		if probeErr != nil {
			log.Warn().Err(probeErr).Str("session_id", string(s.ID)).Int("pid", *s.ProcessID).Msg("failed to probe session process")
			continue
		}
		if alive {
			continue
		}
		if err := uc.sessionRepo.MarkEnded(ctx, s.ID, endedAt); err != nil {
			return output, err
		}
		log.Info().Str("session_id", string(s.ID)).Int("pid", *s.ProcessID).Msg("ended dead browser session")
		output.EndedDeadSessions++
	}
	return output, nil
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
