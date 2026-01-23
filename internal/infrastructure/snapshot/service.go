package snapshot

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/logging"
)

// Service handles debounced session state snapshots.
type Service struct {
	snapshotUC *usecase.SnapshotSessionUseCase
	provider   port.TabListProvider
	interval   time.Duration

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
		intervalMs = 5000 // Default 5 seconds
	}
	return &Service{
		snapshotUC: snapshotUC,
		provider:   provider,
		interval:   time.Duration(intervalMs) * time.Millisecond,
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
	defer s.mu.Unlock()
	s.ready = true
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
		return nil
	}

	return s.snapshotUC.Execute(ctx, usecase.SnapshotInput{
		SessionID: sessionID,
		TabList:   tabList,
	})
}
