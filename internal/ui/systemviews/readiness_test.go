package systemviews

// P4.2 readiness tests: the shell loading state clears after the initial
// mount and action binding, never gated on async history-data completion.
// Fatal mount/bind failures report to the shell while preserving the
// original error.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

// signalingDOM records readiness signals on top of the recording mount.
type signalingDOM struct {
	recordingDOM
	ready  bool
	failed string
}

func (d *signalingDOM) SignalReady() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ready = true
	return nil
}

func (d *signalingDOM) SignalError(message string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.failed = message
	return nil
}

func (d *signalingDOM) isReady() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ready
}

func (d *signalingDOM) failure() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.failed
}

// signalingActionDOM binds actions and signals readiness.
type signalingActionDOM struct {
	signalingDOM
	handler DOMActionHandler
}

func (d *signalingActionDOM) BindActions(handler DOMActionHandler) error {
	d.handler = handler
	return nil
}

func TestReadinessSignaledAfterMountAndBind(t *testing.T) {
	t.Parallel()

	dom := &signalingActionDOM{}
	app := NewApp(Dependencies{DOM: dom, LocationURI: "dumb://history"})
	require.NoError(t, app.Run())
	require.True(t, dom.isReady(), "readiness must fire after mount and binding")
	require.Empty(t, dom.failure())
}

func TestReadinessMountFailureSignalsError(t *testing.T) {
	t.Parallel()

	dom := &failingMountSignalingDOM{err: errors.New("mount blew up")}
	app := NewApp(Dependencies{DOM: dom, LocationURI: "dumb://history"})
	err := app.Run()
	require.ErrorIs(t, err, dom.err, "original mount error must propagate")
	require.Contains(t, dom.failure(), "mount blew up")
	require.False(t, dom.isReady())
}

// failingMountSignalingDOM fails Mount while recording signals.
type failingMountSignalingDOM struct {
	signalingDOM
	err error
}

func (d *failingMountSignalingDOM) Mount(string) error {
	return d.err
}

func TestReadinessBindFailureSignalsError(t *testing.T) {
	t.Parallel()

	dom := &failingBindSignalingDOM{err: errors.New("bind blew up")}
	app := NewApp(Dependencies{DOM: dom, LocationURI: "dumb://history"})
	err := app.Run()
	require.ErrorIs(t, err, dom.err, "original bind error must propagate")
	require.Contains(t, dom.failure(), "bind blew up")
	require.False(t, dom.isReady())
}

// failingBindSignalingDOM fails BindActions while recording signals.
type failingBindSignalingDOM struct {
	signalingDOM
	err error
}

func (d *failingBindSignalingDOM) BindActions(DOMActionHandler) error {
	return d.err
}

func TestReadinessWithoutSignalerKeepsLegacyBehavior(t *testing.T) {
	t.Parallel()

	dom := &recordingDOM{}
	app := NewApp(Dependencies{DOM: dom, LocationURI: "dumb://history"})
	require.NoError(t, app.Run())
	require.True(t, dom.Mounted())
}

func TestReadinessSignalFailurePropagates(t *testing.T) {
	t.Parallel()

	dom := &failingReadySignalingDOM{err: errors.New("signal blew up")}
	app := NewApp(Dependencies{DOM: dom, LocationURI: "dumb://history"})
	require.ErrorIs(t, app.Run(), dom.err)
}

// failingReadySignalingDOM binds fine but fails the readiness signal.
type failingReadySignalingDOM struct {
	signalingDOM
	err error
}

func (d *failingReadySignalingDOM) BindActions(DOMActionHandler) error {
	return nil
}

func (d *failingReadySignalingDOM) SignalReady() error {
	return d.err
}

func TestReadinessNotGatedOnAsyncHistoryData(t *testing.T) {
	t.Parallel()

	dom := &signalingActionDOM{signalingDOM: signalingDOM{recordingDOM: recordingDOM{mounts: make(chan string, 4)}}}
	history := &recordingHistoryService{
		entries:         []*entity.HistoryEntry{{ID: 1, URL: "https://example.com", Title: "Loaded entry"}},
		timelineStarted: make(chan struct{}),
		releaseTimeline: make(chan struct{}),
	}
	app := NewApp(Dependencies{DOM: dom, History: history, LocationURI: "dumb://history"})

	require.NoError(t, app.RunWithContext(context.Background()))
	select {
	case <-history.timelineStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("history timeline was not started asynchronously")
	}
	// The async hydration is still blocked, yet readiness already fired:
	// history-data completion is separate from shell readiness.
	require.True(t, dom.isReady(), "readiness must not wait for history data")
	close(history.releaseTimeline)
}
