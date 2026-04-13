package cef

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/glib"

	"github.com/bnema/dumber/internal/logging"
)

const (
	resizeReconcileRetryDelay = 8 * time.Millisecond
	resizeReconcileWindow     = 64 * time.Millisecond
)

type resizeReconcileTimer interface {
	stop()
}

type resizeReconcileScheduler interface {
	afterFunc(delay time.Duration, fn func()) resizeReconcileTimer
}

type glibResizeTimer struct{ id uint }

func (t *glibResizeTimer) stop() {
	if t == nil || t.id == 0 {
		return
	}
	glib.SourceRemove(t.id)
	t.id = 0
}

type glibResizeScheduler struct{}

func (glibResizeScheduler) afterFunc(delay time.Duration, fn func()) resizeReconcileTimer {
	cb := glib.SourceFunc(func(_ uintptr) bool {
		if fn != nil {
			fn()
		}
		return false
	})
	return &glibResizeTimer{id: glib.TimeoutAdd(uint(delay.Milliseconds()), &cb, 0)}
}

type resizeReconciler struct {
	mu        sync.Mutex
	webviewID port.WebViewID
	ctx       context.Context
	scheduler resizeReconcileScheduler
	now       func() time.Time
	session   *resizeReconcileSession
}

type resizeReconcileSession struct {
	seq        uint64
	startedAt  time.Time
	retryCount int
	timer      resizeReconcileTimer
	hostGetter func() resizeNotifiableBrowserHost
	destroyed  func() bool
}

func newResizeReconciler(ctx context.Context, webviewID port.WebViewID) *resizeReconciler {
	return &resizeReconciler{
		webviewID: webviewID,
		ctx:       ctx,
		scheduler: glibResizeScheduler{},
		now:       time.Now,
	}
}

func (r *resizeReconciler) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *resizeReconciler) schedulerOrDefault() resizeReconcileScheduler {
	if r != nil && r.scheduler != nil {
		return r.scheduler
	}
	return glibResizeScheduler{}
}

func (r *resizeReconciler) start(seq uint64, hostGetter func() resizeNotifiableBrowserHost, destroyed func() bool) {
	if r == nil || seq == 0 || hostGetter == nil {
		return
	}
	if destroyed != nil && destroyed() {
		return
	}

	now := r.clock()
	var superseded *resizeReconcileSession

	r.mu.Lock()
	if r.session != nil {
		superseded = r.session
		if superseded.timer != nil {
			superseded.timer.stop()
		}
		r.session = nil
	}
	session := &resizeReconcileSession{
		seq:        seq,
		startedAt:  now,
		hostGetter: hostGetter,
		destroyed:  destroyed,
	}
	r.session = session
	r.mu.Unlock()

	if superseded != nil {
		r.logStopped("superseded", superseded.seq, superseded.retryCount, now.Sub(superseded.startedAt).Milliseconds())
	}
	r.logStarted(seq)
	r.schedule(session)
}

func (r *resizeReconciler) notePaint(seq uint64, sizeMatches bool) {
	if r == nil || seq == 0 || !sizeMatches {
		return
	}
	now := r.clock()

	r.mu.Lock()
	session := r.session
	if session == nil || session.seq != seq {
		r.mu.Unlock()
		return
	}
	if session.timer != nil {
		session.timer.stop()
		session.timer = nil
	}
	r.session = nil
	retryCount := session.retryCount
	elapsed := now.Sub(session.startedAt).Milliseconds()
	r.mu.Unlock()

	r.logStopped("paint-arrived", seq, retryCount, elapsed)
}

func (r *resizeReconciler) stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	session := r.session
	r.session = nil
	r.mu.Unlock()
	if session != nil && session.timer != nil {
		session.timer.stop()
	}
}

func (r *resizeReconciler) schedule(session *resizeReconcileSession) {
	if r == nil || session == nil {
		return
	}
	timer := r.schedulerOrDefault().afterFunc(resizeReconcileRetryDelay, func() {
		r.runTick(session)
	})
	r.mu.Lock()
	if r.session == session {
		session.timer = timer
	} else if timer != nil {
		timer.stop()
	}
	r.mu.Unlock()
}

func (r *resizeReconciler) runTick(session *resizeReconcileSession) {
	if r == nil || session == nil {
		return
	}
	now := r.clock()

	r.mu.Lock()
	if r.session != session {
		r.mu.Unlock()
		return
	}
	if session.destroyed != nil && session.destroyed() {
		r.session = nil
		r.mu.Unlock()
		return
	}
	host := session.hostGetter()
	if host == nil {
		r.session = nil
		r.mu.Unlock()
		return
	}
	elapsed := now.Sub(session.startedAt)
	if elapsed > resizeReconcileWindow {
		r.session = nil
		retryCount := session.retryCount
		r.mu.Unlock()
		r.logStopped("expired", session.seq, retryCount, elapsed.Milliseconds())
		return
	}
	session.retryCount++
	retryCount := session.retryCount
	seq := session.seq
	r.mu.Unlock()

	host.Invalidate(purecef.PaintElementTypePetView)
	r.logRetry(seq, retryCount, elapsed.Milliseconds())

	elapsedAfterInvalidate := r.clock().Sub(session.startedAt)
	r.mu.Lock()
	if r.session != session {
		r.mu.Unlock()
		return
	}
	if elapsedAfterInvalidate >= resizeReconcileWindow {
		r.session = nil
		r.mu.Unlock()
		r.logStopped("expired", seq, retryCount, elapsedAfterInvalidate.Milliseconds())
		return
	}
	newTimer := r.schedulerOrDefault().afterFunc(resizeReconcileRetryDelay, func() {
		r.runTick(session)
	})
	session.timer = newTimer
	r.mu.Unlock()
}

func (r *resizeReconciler) logStarted(seq uint64) {
	if r == nil || r.ctx == nil {
		return
	}
	logging.FromContext(r.ctx).Debug().
		Uint64("webview_id", uint64(r.webviewID)).
		Uint64("resize_seq", seq).
		Int("retry_count", 0).
		Int64("elapsed_ms", 0).
		Msg("cef: resize reconciliation started")
}

func (r *resizeReconciler) logRetry(seq uint64, retryCount int, elapsedMs int64) {
	if r == nil || r.ctx == nil {
		return
	}
	logging.FromContext(r.ctx).Debug().
		Uint64("webview_id", uint64(r.webviewID)).
		Uint64("resize_seq", seq).
		Int("retry_count", retryCount).
		Int64("elapsed_ms", elapsedMs).
		Msg("cef: resize reconciliation retry")
}

func (r *resizeReconciler) logStopped(reason string, seq uint64, retryCount int, elapsedMs int64) {
	if r == nil || r.ctx == nil {
		return
	}
	logging.FromContext(r.ctx).Debug().
		Uint64("webview_id", uint64(r.webviewID)).
		Uint64("resize_seq", seq).
		Int("retry_count", retryCount).
		Int64("elapsed_ms", elapsedMs).
		Str("reason", reason).
		Msg("cef: resize reconciliation stopped")
}
