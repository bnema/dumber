package snapshot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/logging"
)

const (
	defaultSnapshotIntervalMs = 5000
	maxFKRetries              = 3
	fkRetryDelay              = 100 * time.Millisecond
)

// Service handles debounced session state snapshots.
type Service struct {
	snapshotUC *usecase.SnapshotSessionUseCase
	provider   port.TabListProvider
	interval   time.Duration
	retries    int
	retryDelay time.Duration

	mu     sync.Mutex
	timer  *time.Timer
	dirty  bool
	ready  bool // true when session is persisted to DB and snapshots can be saved
	ctx    context.Context
	cancel context.CancelFunc
}

// NewService creates a new snapshot service.
func NewService(
	snapshotUC *usecase.SnapshotSessionUseCase,
	provider port.TabListProvider,
	intervalMs int,
) *Service {
	if intervalMs <= 0 {
		intervalMs = defaultSnapshotIntervalMs
	}
	return &Service{
		snapshotUC: snapshotUC,
		provider:   provider,
		interval:   time.Duration(intervalMs) * time.Millisecond,
		retries:    maxFKRetries,
		retryDelay: fkRetryDelay,
	}
}

// Start begins watching for dirty state.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ctx, s.cancel = context.WithCancel(ctx)
	logging.FromContext(ctx).Debug().Dur("interval", s.interval).Msg("snapshot service started")
}

// SetReady marks the service as ready to save snapshots.
// Call this after the session has been persisted to the database
// to avoid FK constraint violations.
func (s *Service) SetReady() {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.ready = true
	dirty := s.dirty
	ctx := s.ctx
	s.mu.Unlock()

	if !dirty || ctx == nil {
		return
	}

	go func() {
		if err := s.saveSnapshot(ctx); err != nil {
			logging.FromContext(ctx).Error().Err(err).Msg("failed to save pending session snapshot after ready")
		}
	}()
}

// Stop stops the service and saves final state.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()

	// Final save on shutdown
	return s.SaveNow(ctx)
}

// MarkDirty signals that state has changed.
// Debounces saves to avoid excessive DB writes.
func (s *Service) MarkDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dirty = true

	// Reset or create timer
	if s.timer != nil {
		s.timer.Stop()
	}

	s.timer = time.AfterFunc(s.interval, func() {
		s.mu.Lock()
		ctx := s.ctx
		s.mu.Unlock()

		if ctx == nil {
			return
		}

		if err := s.saveSnapshot(ctx); err != nil {
			logging.FromContext(ctx).Error().Err(err).Msg("failed to save session snapshot")
		}
	})
}

// SaveNow forces immediate save (for shutdown).
func (s *Service) SaveNow(ctx context.Context) error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	dirty := s.dirty
	s.mu.Unlock()

	if !dirty {
		return nil
	}

	return s.saveSnapshot(ctx)
}

func (s *Service) saveSnapshot(ctx context.Context) error {
	s.mu.Lock()
	ready := s.ready
	if !ready {
		// Don't clear dirty if not ready - keep pending snapshot for later
		s.mu.Unlock()
		return nil
	}
	// Only clear dirty when we're actually going to save
	s.dirty = false
	s.mu.Unlock()

	tabList := s.provider.GetTabList()
	sessionID := s.provider.GetSessionID()

	if sessionID == "" {
		s.markDirty()
		return nil
	}

	if err := s.executeWithRetry(ctx, usecase.SnapshotInput{
		SessionID: sessionID,
		TabList:   tabList,
	}); err != nil {
		s.markDirty()
		return err
	}

	return nil
}

func (s *Service) executeWithRetry(ctx context.Context, input usecase.SnapshotInput) error {
	log := logging.FromContext(ctx)
	maxAttempts := s.retries + 1

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := s.snapshotUC.Execute(ctx, input); err != nil {
			lastErr = err
			if !isTransientFKError(err) || attempt == maxAttempts {
				if attempt == maxAttempts && isTransientFKError(err) {
					log.Error().
						Err(err).
						Str("session_id", string(input.SessionID)).
						Int("attempts", maxAttempts).
						Msg("snapshot save retries exhausted after transient fk violation")
				}
				return err
			}

			log.Warn().
				Err(err).
				Str("session_id", string(input.SessionID)).
				Int("attempt", attempt).
				Int("max_attempts", maxAttempts).
				Dur("retry_delay", s.retryDelay).
				Msg("transient fk violation while saving snapshot; retrying")

			if waitErr := waitForRetry(ctx, s.retryDelay); waitErr != nil {
				return fmt.Errorf("waiting to retry snapshot save: %w", waitErr)
			}
			continue
		}

		return nil
	}

	return lastErr
}

func (s *Service) markDirty() {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isTransientFKError(err error) bool {
	// SQLite-specific transient FK detection:
	// we match case-insensitive "foreign key" text from wrapped driver errors.
	// Message wording may vary across SQLite versions/wrappers; adjust this check
	// if transient FK retries stop matching expected failures in production logs.
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "foreign key")
}
