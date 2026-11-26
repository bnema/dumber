package usecase

import (
	"context"
	"time"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/domain/repository"
	"bnema.dev/dumber/internal/logging"
)

// CleanupSessionsUseCase handles automatic cleanup of old exited sessions.
type CleanupSessionsUseCase struct {
	sessionRepo  repository.SessionRepository
	processProbe port.SessionProcessProbe
	leases       port.SessionLeaseStore
}

// NewCleanupSessionsUseCase creates a new CleanupSessionsUseCase.
// leases decides liveness for sessions that hold one; processProbe covers older
// sessions recorded without a lease. Either may be nil.
func NewCleanupSessionsUseCase(
	sessionRepo repository.SessionRepository,
	processProbe port.SessionProcessProbe,
	leases port.SessionLeaseStore,
) *CleanupSessionsUseCase {
	return &CleanupSessionsUseCase{
		sessionRepo:  sessionRepo,
		processProbe: processProbe,
		leases:       leases,
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
}

type ReconcileActiveBrowserSessionsOutput struct {
	EndedDeadSessions int64
}

// pidReuseMargin absorbs clock and /proc rounding error before a PID holder that
// started after its session is treated as an unrelated process reusing the PID.
const pidReuseMargin = time.Minute

// sessionProcessRunning reports whether the session's own process may still hold its PID.
// A holder running another binary that started well after the session reuses the PID.
// A Dumber holder is kept whatever its start time, since a wall-clock step forward
// shifts the computed start time and must never end a live session.
func sessionProcessRunning(s *entity.Session, status port.SessionProcessStatus) bool {
	if !status.Running {
		return false
	}
	return status.SameExecutable || !status.StartedAt.After(s.StartedAt.Add(pidReuseMargin))
}

// ReconcileActiveBrowserSessions ends every active browser session whose process is gone.
// A session's lease answers exactly; sessions without one fall back to probing their PID.
func (uc *CleanupSessionsUseCase) ReconcileActiveBrowserSessions(
	ctx context.Context,
	input ReconcileActiveBrowserSessionsInput,
) (ReconcileActiveBrowserSessionsOutput, error) {
	log := logging.FromContext(ctx)
	output := ReconcileActiveBrowserSessionsOutput{}
	if uc.processProbe == nil && uc.leases == nil {
		return output, nil
	}
	active, err := uc.sessionRepo.ListActiveBrowser(ctx)
	if err != nil {
		return output, err
	}
	for _, s := range active {
		if s == nil || s.ID == input.CurrentSessionID {
			continue
		}
		if !uc.sessionProcessGone(ctx, s) {
			continue
		}
		// Ending at last activity keeps long-dead sessions from outranking recent
		// clean exits in the exited-session count cleanup.
		if err := uc.sessionRepo.MarkEndedAtLastActivity(ctx, s.ID); err != nil {
			return output, err
		}
		if uc.leases != nil {
			if err := uc.leases.Remove(ctx, s.ID); err != nil {
				log.Warn().Err(err).Str("session_id", string(s.ID)).Msg("failed to remove session lease")
			}
		}
		log.Info().Str("session_id", string(s.ID)).Msg("ended dead browser session")
		output.EndedDeadSessions++
	}
	return output, nil
}

// sessionProcessGone reports whether the session's process is proven gone. Any doubt keeps the session.
func (uc *CleanupSessionsUseCase) sessionProcessGone(ctx context.Context, s *entity.Session) bool {
	log := logging.FromContext(ctx).With().Str("session_id", string(s.ID)).Logger()
	if uc.leases != nil {
		state, err := uc.leases.Probe(ctx, s.ID)
		if err != nil {
			log.Warn().Err(err).Msg("failed to probe session lease")
			return false
		}
		switch state {
		case port.SessionLeaseHeld:
			return false
		case port.SessionLeaseFree:
			return true
		case port.SessionLeaseMissing:
		}
	}
	if uc.processProbe == nil {
		return false
	}
	if s.ProcessID == nil {
		log.Debug().Msg("skipping active session without lease or process id")
		return false
	}
	status, err := uc.processProbe.ProcessStatus(ctx, *s.ProcessID)
	if err != nil {
		log.Warn().Err(err).Int("pid", *s.ProcessID).Msg("failed to probe session process")
		return false
	}
	return !sessionProcessRunning(s, status)
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
