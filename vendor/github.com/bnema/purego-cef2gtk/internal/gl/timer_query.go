package gl

import "time"

const (
	TimeElapsed          uint32 = 0x88BF
	QueryResult          uint32 = 0x8866
	QueryResultAvailable uint32 = 0x8867
)

type TimerQuery uint32

type TimerQueryDriver interface {
	TimerQuerySupported() bool
	GenQueries(n int32, ids *uint32)
	DeleteQueries(n int32, ids *uint32)
	BeginQuery(target uint32, id uint32)
	EndQuery(target uint32)
	GetQueryObjectuiv(id uint32, pname uint32, params *uint32)
	GetQueryObjectui64v(id uint32, pname uint32, params *uint64)
}

// TimerQueryRecorder tracks asynchronous GL timer queries. It is not safe for
// concurrent use; callers must serialize all methods on the owning GL context
// thread.
type TimerQueryRecorder struct {
	driver  TimerQueryDriver
	pending []TimerQuery
}

func NewTimerQueryRecorder(driver TimerQueryDriver) *TimerQueryRecorder {
	if driver == nil || !driver.TimerQuerySupported() {
		return &TimerQueryRecorder{}
	}
	return &TimerQueryRecorder{driver: driver}
}

func (r *TimerQueryRecorder) Supported() bool {
	return r != nil && r.driver != nil && r.driver.TimerQuerySupported()
}

func (r *TimerQueryRecorder) Begin() (TimerQuery, bool) {
	if !r.Supported() {
		return 0, false
	}
	var id uint32
	r.driver.GenQueries(1, &id)
	if id == 0 {
		return 0, false
	}
	r.driver.BeginQuery(TimeElapsed, id)
	return TimerQuery(id), true
}

func (r *TimerQueryRecorder) End(q TimerQuery) {
	if !r.Supported() || q == 0 {
		return
	}
	r.driver.EndQuery(TimeElapsed)
	r.pending = append(r.pending, q)
}

func (r *TimerQueryRecorder) Collect() []time.Duration {
	if !r.Supported() || len(r.pending) == 0 {
		return nil
	}
	ready := make([]time.Duration, 0, len(r.pending))
	remaining := r.pending[:0]
	for _, q := range r.pending {
		var available uint32
		r.driver.GetQueryObjectuiv(uint32(q), QueryResultAvailable, &available)
		if available == 0 {
			remaining = append(remaining, q)
			continue
		}
		var ns uint64
		r.driver.GetQueryObjectui64v(uint32(q), QueryResult, &ns)
		id := uint32(q)
		r.driver.DeleteQueries(1, &id)
		ready = append(ready, time.Duration(ns))
	}
	r.pending = remaining
	return ready
}

func (r *TimerQueryRecorder) Close() {
	if !r.Supported() {
		r.pending = nil
		return
	}
	for _, q := range r.pending {
		id := uint32(q)
		r.driver.DeleteQueries(1, &id)
	}
	r.pending = nil
}
