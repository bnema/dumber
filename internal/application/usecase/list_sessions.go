package usecase

import (
	"context"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// ListSessionsUseCase handles listing sessions with their state information.
type ListSessionsUseCase struct {
	sessionRepo repository.SessionRepository
	stateRepo   repository.SessionStateRepository
	lockDir     string // Directory where lock files are stored
}

// NewListSessionsUseCase creates a new ListSessionsUseCase.
func NewListSessionsUseCase(
	sessionRepo repository.SessionRepository,
	stateRepo repository.SessionStateRepository,
	lockDir string,
) *ListSessionsUseCase {
	return &ListSessionsUseCase{
		sessionRepo: sessionRepo,
		stateRepo:   stateRepo,
		lockDir:     lockDir,
	}
}

// ListSessionsOutput contains the list of sessions with their info.
type ListSessionsOutput struct {
	Sessions []entity.SessionInfo
}

// Execute returns a list of sessions with their state information.
// Sessions are sorted with active first, then by date (most recent first).
func (uc *ListSessionsUseCase) Execute(ctx context.Context, currentSessionID entity.SessionID, limit int) (*ListSessionsOutput, error) {
	log := logging.FromContext(ctx)

	if limit <= 0 {
		limit = 50
	}

	sessions, err := uc.sessionRepo.GetRecent(ctx, limit)
	if err != nil {
		return nil, err
	}

	// Get all snapshots
	snapshots, err := uc.stateRepo.GetAllSnapshots(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get session snapshots")
		// Continue without snapshots
		snapshots = nil
	}

	// Build snapshot lookup map
	snapshotMap := make(map[entity.SessionID]*entity.SessionState)
	for _, s := range snapshots {
		snapshotMap[s.SessionID] = s
	}

	// Build session info list
	result := make([]entity.SessionInfo, 0, len(sessions))
	for _, session := range sessions {
		// Only include browser sessions
		if session.Type != entity.SessionTypeBrowser {
			continue
		}

		info := entity.SessionInfo{
			Session:   session,
			IsCurrent: session.ID == currentSessionID,
			IsActive:  session.IsActive() || uc.hasActiveLock(session.ID),
		}

		if state, ok := snapshotMap[session.ID]; ok {
			info.State = state
			info.TabCount = len(state.Tabs)
			info.PaneCount = state.CountPanes()
			info.UpdatedAt = state.SavedAt
		} else {
			info.UpdatedAt = session.StartedAt
		}

		result = append(result, info)
	}

	// Sort: current first, then active, then by date
	sortSessionInfos(result, currentSessionID)

	return &ListSessionsOutput{Sessions: result}, nil
}

// hasActiveLock checks if a session has an active lock file.
func (uc *ListSessionsUseCase) hasActiveLock(sessionID entity.SessionID) bool {
	return isSessionLocked(uc.lockDir, sessionID)
}

// sortSessionInfos sorts sessions by: current first, then active, then by UpdatedAt descending.
func sortSessionInfos(infos []entity.SessionInfo, currentID entity.SessionID) {
	// Simple bubble sort for small lists
	n := len(infos)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if shouldSwap(infos[j], infos[j+1], currentID) {
				infos[j], infos[j+1] = infos[j+1], infos[j]
			}
		}
	}
}

func shouldSwap(a, b entity.SessionInfo, currentID entity.SessionID) bool {
	// Current session always first
	if a.Session.ID == currentID {
		return false
	}
	if b.Session.ID == currentID {
		return true
	}

	// Active sessions before inactive
	if a.IsActive && !b.IsActive {
		return false
	}
	if !a.IsActive && b.IsActive {
		return true
	}

	// Most recent first
	return a.UpdatedAt.Before(b.UpdatedAt)
}

// GetSessionInfo returns info for a specific session.
func (uc *ListSessionsUseCase) GetSessionInfo(
	ctx context.Context,
	sessionID, currentSessionID entity.SessionID,
) (*entity.SessionInfo, error) {
	session, err := uc.sessionRepo.FindByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}

	info := &entity.SessionInfo{
		Session:   session,
		IsCurrent: session.ID == currentSessionID,
		IsActive:  session.IsActive() || uc.hasActiveLock(session.ID),
		UpdatedAt: session.StartedAt,
	}

	state, err := uc.stateRepo.GetSnapshot(ctx, sessionID)
	if err != nil {
		logging.FromContext(ctx).Warn().Err(err).Str("session_id", string(sessionID)).Msg("failed to get session state")
	}
	if state != nil {
		info.State = state
		info.TabCount = len(state.Tabs)
		info.PaneCount = state.CountPanes()
		info.UpdatedAt = state.SavedAt
	}

	return info, nil
}

const (
	hoursPerDay  = 24
	daysPerWeek  = 7
	maxIDDisplay = 20
)

// GetRelativeTime returns a human-readable relative time string.
func GetRelativeTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return formatDuration(mins, "m")
	case diff < hoursPerDay*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return formatDuration(hours, "h")
	case diff < daysPerWeek*hoursPerDay*time.Hour:
		days := int(diff.Hours() / hoursPerDay)
		if days == 1 {
			return "1d ago"
		}
		return formatDuration(days, "d")
	default:
		weeks := int(diff.Hours() / hoursPerDay / daysPerWeek)
		if weeks == 1 {
			return "1w ago"
		}
		return formatDuration(weeks, "w")
	}
}

func formatDuration(n int, unit string) string {
	return string(rune('0'+n/10)) + string(rune('0'+n%10)) + unit + " ago"
}
