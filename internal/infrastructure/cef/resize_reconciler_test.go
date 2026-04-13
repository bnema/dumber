package cef

import (
	"context"
	"sync"
	"testing"
	"time"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"
)

type resizeHostRecorder struct {
	mu            sync.Mutex
	invalidations []purecef.PaintElementType
}

func (h *resizeHostRecorder) WasResized() {}

func (h *resizeHostRecorder) Invalidate(elementType purecef.PaintElementType) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.invalidations = append(h.invalidations, elementType)
}

func (h *resizeHostRecorder) invalidationCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.invalidations)
}

type fakeResizeTimer struct {
	id     uint
	cancel func()
}

func (t *fakeResizeTimer) stop() {
	if t != nil && t.cancel != nil {
		t.cancel()
	}
}

type fakeResizeScheduler struct {
	mu      sync.Mutex
	nextID  uint
	timers  map[uint]func()
	delays  []time.Duration
	stopped map[uint]bool
}

func newFakeResizeScheduler() *fakeResizeScheduler {
	return &fakeResizeScheduler{
		timers:  make(map[uint]func()),
		stopped: make(map[uint]bool),
	}
}

func (s *fakeResizeScheduler) afterFunc(delay time.Duration, fn func()) resizeReconcileTimer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	id := s.nextID
	s.timers[id] = fn
	s.delays = append(s.delays, delay)
	return &fakeResizeTimer{
		id: id,
		cancel: func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.stopped[id] = true
		},
	}
}

func (s *fakeResizeScheduler) fireLast() {
	s.mu.Lock()
	id := s.nextID
	fn := s.timers[id]
	stopped := s.stopped[id]
	s.mu.Unlock()
	if fn == nil || stopped {
		return
	}
	fn()
}

func (s *fakeResizeScheduler) delayCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.delays)
}

func TestResizeReconciler_SchedulesRetriesWhenNoPaintObserved(t *testing.T) {
	clock := time.Unix(0, 0)
	scheduler := newFakeResizeScheduler()
	host := &resizeHostRecorder{}
	rr := &resizeReconciler{
		ctx:       context.Background(),
		scheduler: scheduler,
		now:       func() time.Time { return clock },
	}

	rr.start(7, func() resizeNotifiableBrowserHost { return host }, func() bool { return false })
	require.Equal(t, 1, scheduler.delayCount())
	require.Equal(t, 0, host.invalidationCount())

	clock = clock.Add(8 * time.Millisecond)
	scheduler.fireLast()
	require.Equal(t, 1, host.invalidationCount())
	require.Equal(t, 2, scheduler.delayCount())
}

func TestResizeReconciler_StopsAfterMatchingPaintArrives(t *testing.T) {
	clock := time.Unix(0, 0)
	scheduler := newFakeResizeScheduler()
	host := &resizeHostRecorder{}
	rr := &resizeReconciler{
		ctx:       context.Background(),
		scheduler: scheduler,
		now:       func() time.Time { return clock },
	}

	rr.start(9, func() resizeNotifiableBrowserHost { return host }, func() bool { return false })
	rr.notePaint(9, true)

	clock = clock.Add(8 * time.Millisecond)
	scheduler.fireLast()
	require.Zero(t, host.invalidationCount())
	require.Equal(t, 1, scheduler.delayCount())
}

func TestResizeReconciler_StaleSizePaintDoesNotStopRetries(t *testing.T) {
	clock := time.Unix(0, 0)
	scheduler := newFakeResizeScheduler()
	host := &resizeHostRecorder{}
	rr := &resizeReconciler{
		ctx:       context.Background(),
		scheduler: scheduler,
		now:       func() time.Time { return clock },
	}

	rr.start(10, func() resizeNotifiableBrowserHost { return host }, func() bool { return false })
	rr.notePaint(10, false)

	clock = clock.Add(8 * time.Millisecond)
	scheduler.fireLast()
	require.Equal(t, 1, host.invalidationCount())
	require.Equal(t, 2, scheduler.delayCount())
}

func TestResizeReconciler_NewerResizeSupersedesOlderSession(t *testing.T) {
	clock := time.Unix(0, 0)
	scheduler := newFakeResizeScheduler()
	host := &resizeHostRecorder{}
	rr := &resizeReconciler{
		ctx:       context.Background(),
		scheduler: scheduler,
		now:       func() time.Time { return clock },
	}

	rr.start(1, func() resizeNotifiableBrowserHost { return host }, func() bool { return false })
	firstDelayCount := scheduler.delayCount()
	rr.start(2, func() resizeNotifiableBrowserHost { return host }, func() bool { return false })
	require.Equal(t, firstDelayCount+1, scheduler.delayCount())

	clock = clock.Add(8 * time.Millisecond)
	scheduler.fireLast()
	require.Equal(t, 1, host.invalidationCount())
}

func TestResizeReconciler_ExpiresAfter64ms(t *testing.T) {
	clock := time.Unix(0, 0)
	scheduler := newFakeResizeScheduler()
	host := &resizeHostRecorder{}
	rr := &resizeReconciler{
		ctx:       context.Background(),
		scheduler: scheduler,
		now:       func() time.Time { return clock },
	}

	rr.start(11, func() resizeNotifiableBrowserHost { return host }, func() bool { return false })
	for i := 0; i < 8; i++ {
		clock = clock.Add(8 * time.Millisecond)
		scheduler.fireLast()
	}

	require.Equal(t, 8, host.invalidationCount())
	require.Equal(t, 8, scheduler.delayCount())
	clock = clock.Add(8 * time.Millisecond)
	scheduler.fireLast()
	require.Equal(t, 8, host.invalidationCount())
}
