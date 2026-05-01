package cef

import (
	"sync/atomic"
	"time"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/logging"
)

const (
	cefHeartbeatInterval    = 1 * time.Second
	cefHeartbeatLogEvery    = 10 * time.Second
	cefHeartbeatStopTimeout = 2 * time.Second
)

type cefThreadHeartbeatState struct {
	seq            atomic.Uint64
	lastPostUnixNS atomic.Int64
	lastAckUnixNS  atomic.Int64
	lastLatencyNS  atomic.Int64
	lastPostResult atomic.Int32
	lastLogUnixNS  atomic.Int64
	inFlight       atomic.Bool
}

func (s *cefThreadHeartbeatState) snapshot(now time.Time) cefHeartbeatSnapshot {
	if s == nil {
		return cefHeartbeatSnapshot{}
	}
	lastPost := unixNSTime(s.lastPostUnixNS.Load())
	lastAck := unixNSTime(s.lastAckUnixNS.Load())
	snap := cefHeartbeatSnapshot{
		Seq:          s.seq.Load(),
		PostResult:   s.lastPostResult.Load(),
		Latency:      time.Duration(s.lastLatencyNS.Load()),
		LastPostTime: lastPost,
		LastAckTime:  lastAck,
		InFlight:     s.inFlight.Load(),
	}
	if !lastPost.IsZero() {
		snap.LastPostAge = now.Sub(lastPost)
	}
	if !lastAck.IsZero() {
		snap.LastAckAge = now.Sub(lastAck)
	}
	return snap
}

type cefHeartbeatSnapshot struct {
	Seq          uint64
	PostResult   int32
	Latency      time.Duration
	LastPostTime time.Time
	LastAckTime  time.Time
	LastPostAge  time.Duration
	LastAckAge   time.Duration
	InFlight     bool
}

func (e *Engine) startCEFHeartbeat() {
	if e == nil || e.ctx == nil || e.cefHeartbeatStop != nil {
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	e.cefHeartbeatStop = stop
	e.cefHeartbeatDone = done
	go e.cefHeartbeatLoop(stop, done)
}

func (e *Engine) stopCEFHeartbeat() {
	if e == nil || e.cefHeartbeatStop == nil {
		return
	}
	stop := e.cefHeartbeatStop
	done := e.cefHeartbeatDone
	e.cefHeartbeatStop = nil
	e.cefHeartbeatDone = nil
	close(stop)
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(cefHeartbeatStopTimeout):
		if e.ctx != nil {
			logging.FromContext(e.ctx).Warn().Msg("cef: heartbeat stop timed out")
		}
	}
}

func (e *Engine) cefHeartbeatLoop(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(cefHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			e.postCEFThreadHeartbeat("ui", purecef.ThreadIDTidUi, &e.cefUIHeartbeat, now)
			e.postCEFThreadHeartbeat("io", purecef.ThreadIDTidIo, &e.cefIOHeartbeat, now)
		}
	}
}

func (e *Engine) postCEFThreadHeartbeat(name string, threadID purecef.ThreadID, state *cefThreadHeartbeatState, now time.Time) {
	if e == nil || state == nil {
		return
	}
	seq := state.seq.Add(1)
	postedAt := now
	state.lastPostUnixNS.Store(postedAt.UnixNano())
	state.inFlight.Store(true)
	task := cefNewTask(cefTaskFunc(func() {
		ackedAt := time.Now()
		state.lastAckUnixNS.Store(ackedAt.UnixNano())
		state.lastLatencyNS.Store(ackedAt.Sub(postedAt).Nanoseconds())
		state.inFlight.Store(false)
	}))
	if task == nil {
		state.lastPostResult.Store(0)
		state.inFlight.Store(false)
		return
	}
	result := cefPostTask(threadID, task)
	state.lastPostResult.Store(result)
	if result != 1 {
		state.inFlight.Store(false)
	}
	lastLog := unixNSTime(state.lastLogUnixNS.Load())
	if result != 1 || lastLog.IsZero() || now.Sub(lastLog) >= cefHeartbeatLogEvery {
		state.lastLogUnixNS.Store(now.UnixNano())
		snap := state.snapshot(now)
		logging.FromContext(e.ctx).Debug().
			Str("thread", name).
			Uint64("seq", seq).
			Int32("post_result", result).
			Int64("last_ack_age_ms", durationMillis(snap.LastAckAge)).
			Int64("latency_ms", durationMillis(snap.Latency)).
			Bool("in_flight", snap.InFlight).
			Msg("cef: thread heartbeat")
	}
}

func (e *Engine) cefHeartbeatSnapshots(now time.Time) (cefHeartbeatSnapshot, cefHeartbeatSnapshot) {
	if e == nil {
		return cefHeartbeatSnapshot{}, cefHeartbeatSnapshot{}
	}
	return e.cefUIHeartbeat.snapshot(now), e.cefIOHeartbeat.snapshot(now)
}

func (wv *WebView) cefHeartbeatSnapshots(now time.Time) (cefHeartbeatSnapshot, cefHeartbeatSnapshot) {
	if wv == nil || wv.engine == nil {
		return cefHeartbeatSnapshot{}, cefHeartbeatSnapshot{}
	}
	return wv.engine.cefHeartbeatSnapshots(now)
}

func unixNSTime(ns int64) time.Time {
	if ns <= 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

func durationMillis(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return d.Milliseconds()
}
