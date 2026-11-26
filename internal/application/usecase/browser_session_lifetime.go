package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/domain/repository"
	"bnema.dev/dumber/internal/logging"
)

// BrowserSessionLifetimeUseCase records a browser session together with its lease.
// While the session row is active, its lease stays held, so other instances can
// tell a live session from a crashed one.
type BrowserSessionLifetimeUseCase struct {
	sessionRepo repository.SessionRepository
	leases      port.SessionLeaseStore
}

// NewBrowserSessionLifetimeUseCase creates the use case. A nil leases store
// records sessions without a lease; liveness then relies on process probing.
func NewBrowserSessionLifetimeUseCase(
	sessionRepo repository.SessionRepository,
	leases port.SessionLeaseStore,
) *BrowserSessionLifetimeUseCase {
	return &BrowserSessionLifetimeUseCase{sessionRepo: sessionRepo, leases: leases}
}

// BrowserSessionLease is the lease a running session holds until End.
type BrowserSessionLease struct {
	closer io.Closer
}

// Begin takes the session's lease, then saves the session, so a visible active
// row always has a held lease while its process runs. A failed lease is logged
// and the session is saved without one.
func (uc *BrowserSessionLifetimeUseCase) Begin(ctx context.Context, session *entity.Session) (*BrowserSessionLease, error) {
	if session == nil {
		return nil, errors.New("session required")
	}
	log := logging.FromContext(ctx)
	lease := &BrowserSessionLease{}
	if uc.leases != nil {
		closer, err := uc.leases.Acquire(ctx, session.ID)
		if err != nil {
			log.Warn().Err(err).Str("session_id", string(session.ID)).
				Msg("failed to acquire session lease; liveness falls back to process probing")
		} else {
			lease.closer = closer
		}
	}
	if err := uc.sessionRepo.Save(ctx, session); err != nil {
		uc.release(ctx, session.ID, lease, true)
		return nil, fmt.Errorf("save session: %w", err)
	}
	return lease, nil
}

// End marks the session ended and releases its lease. The lease file is removed
// only once the row is ended: if marking fails, the released file stays and the
// next instance reads the lease as free and ends the session.
func (uc *BrowserSessionLifetimeUseCase) End(
	ctx context.Context,
	id entity.SessionID,
	lease *BrowserSessionLease,
	endedAt time.Time,
) error {
	if endedAt.IsZero() {
		endedAt = time.Now()
	}
	endErr := uc.sessionRepo.MarkEnded(ctx, id, endedAt)
	uc.release(ctx, id, lease, endErr == nil)
	if endErr != nil {
		return fmt.Errorf("mark session ended: %w", endErr)
	}
	logging.FromContext(ctx).Info().Str("session_id", string(id)).Msg("session ended")
	return nil
}

func (uc *BrowserSessionLifetimeUseCase) release(ctx context.Context, id entity.SessionID, lease *BrowserSessionLease, remove bool) {
	log := logging.FromContext(ctx)
	if lease == nil || lease.closer == nil {
		return
	}
	if err := lease.closer.Close(); err != nil {
		log.Warn().Err(err).Str("session_id", string(id)).Msg("failed to release session lease")
	}
	lease.closer = nil
	if remove && uc.leases != nil {
		if err := uc.leases.Remove(ctx, id); err != nil {
			log.Warn().Err(err).Str("session_id", string(id)).Msg("failed to remove session lease")
		}
	}
}
