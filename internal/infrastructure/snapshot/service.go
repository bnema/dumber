package snapshot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

const (
	defaultSnapshotIntervalMs = 5000
	maxFKRetries              = 3
	fkRetryDelay              = 100 * time.Millisecond
)

// Compile-time interface check.
var _ port.SnapshotService = (*Service)(nil)

// Compile-time drain boundary check.
var _ port.PersistenceDrain = (*Service)(nil)

// Service handles debounced session state snapshots.
type Service struct {
	snapshotUC *usecase.SnapshotSessionUseCase
	provider   port.WindowStateProvider
	interval   time.Duration
	retries    int
	retryDelay time.Duration

	mu       sync.Mutex
	timer    *time.Timer
	timerGen uint64
	// stopped latches Stop: no new debounce work schedules afterwards.
	stopped bool
	dirty   bool
	ready   bool // true when session is persisted to DB and snapshots can be saved
	ctx     context.Context
	cancel  context.CancelFunc

	// Drain boundary for the residency quiescence contract. inflight
	// counts an executing capture/database save; lastErr records the
	// terminal failure of the most recent save while dirty is preserved
	// for reporting and later retry. Failed writes are settled-but-failed:
	// the drain terminates instead of looping forever on dirty.
	inflight  atomic.Int32
	lastErr   error
	onSettled []func()
}

// NewService creates a new snapshot service.
func NewService(
	snapshotUC *usecase.SnapshotSessionUseCase,
	provider port.WindowStateProvider,
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

// Stop stops the service and saves final state, then joins an
// already-running save within a bounded wait so shutdown observes the
// drain barrier instead of racing it. The wait never blocks forever.
// Latching stopped also prevents any later debounce from scheduling work
// on the dead service.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.stopped = true
	s.mu.Unlock()

	// Final save on shutdown
	err := s.SaveNow(ctx)
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if waitErr := s.WaitSettled(drainCtx); waitErr != nil {
		logging.FromContext(ctx).Warn().Err(waitErr).Msg("snapshot drain did not settle before shutdown")
	}
	return err
}

// MarkDirty signals that state has changed.
// Debounces saves to avoid excessive DB writes. It is a no-op after Stop
// so shutdown never resurrects debounce work on the dead service.
func (s *Service) MarkDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.dirty = true

	// Reset or create timer
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timerGen++
	gen := s.timerGen
	s.timer = time.AfterFunc(s.interval, func() {
		s.mu.Lock()
		// Retire the owning timer as it fires: a non-nil timer means
		// debounce work is outstanding, so leaving it set would strand
		// the drain after the save settles. The generation check keeps
		// a superseded timer from clearing its replacement, and a
		// superseded or stopped timer saves nothing: the replacement
		// timer or Stop's own final save owns that work.
		if s.timerGen != gen || s.stopped {
			if s.timerGen == gen {
				s.timer = nil
			}
			s.mu.Unlock()
			return
		}
		s.timer = nil
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
	// Claim the dirty work atomically: concurrent timer and SaveNow paths
	// serialize here, so exactly one save runs and shutdown joins rather
	// than duplicates the active save.
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	// Only clear dirty when we're actually going to save
	s.dirty = false
	s.lastErr = nil
	s.inflight.Add(1)
	s.mu.Unlock()

	defer func() {
		s.inflight.Add(-1)
		s.notifyIfSettled()
	}()

	sessionID := s.provider.GetSessionID()

	if sessionID == "" {
		s.MarkDirty()
		return nil
	}

	windows, activeWindowIndex := s.provider.GetWindowSnapshotState()
	// A nil window list with no active index means the snapshot is truly unavailable,
	// not merely empty. Keep the session dirty so a later capture can retry.
	if windows == nil && activeWindowIndex < 0 {
		s.MarkDirty()
		logging.FromContext(ctx).Warn().Msg("window snapshot unavailable; keeping session snapshot dirty")
		return nil
	}
	if windows == nil {
		windows = make([]entity.WindowTabListState, 0)
	}
	input := usecase.SnapshotInput{
		SessionID:         sessionID,
		Windows:           windows,
		ActiveWindowIndex: activeWindowIndex,
	}
	// single window with WindowID == "" is legacy v1 sentinel; populate TabList and clear Windows/ActiveWindowIndex.
	if len(windows) == 1 && windows[0].WindowID == "" {
		input.Windows = nil
		input.ActiveWindowIndex = 0
		input.TabList = windows[0].Tabs
	}

	if err := s.executeWithRetry(ctx, input); err != nil {
		s.markDirty()
		s.setTerminalError(err)
		return err
	}

	return nil
}

// Active reports whether snapshot work is outstanding: pending debounce,
// an armed timer, or an in-flight capture/database save. A terminally
// failed save is settled-but-failed: dirty is preserved for reporting and
// later retry while lastErr records it, so the drain always terminates and
// never loops forever merely because dirty remains set.
func (s *Service) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight.Load() > 0 {
		return true
	}
	if s.timer != nil {
		return true
	}
	if !s.dirty {
		return false
	}
	return s.lastErr == nil
}

// LastError returns the terminal error of the most recent save, or nil.
// Dirty is preserved alongside it for reporting and later retry.
func (s *Service) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// OnSettled registers a callback invoked when the service reaches a
// settled state after a save completes. Callbacks run on the completing
// goroutine and must be non-blocking; the UI wraps them with GTK dispatch.
// There is no unsubscribe; services are process-lifetime owners.
func (s *Service) OnSettled(fn func()) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSettled = append(s.onSettled, fn)
}

// WaitSettled blocks until no snapshot work is outstanding or ctx ends.
// It reports ctx.Err() on expiry so shutdown never waits forever. It must
// never be called on the GTK thread when capture dispatches there.
func (s *Service) WaitSettled(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !s.Active() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Service) setTerminalError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err
}

func (s *Service) notifyIfSettled() {
	if s.Active() {
		return
	}
	s.mu.Lock()
	callbacks := append([]func(){}, s.onSettled...)
	s.mu.Unlock()
	for _, fn := range callbacks {
		fn()
	}
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
