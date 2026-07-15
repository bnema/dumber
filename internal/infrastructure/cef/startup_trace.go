package cef

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// startupTrace records the accelerated CEF/DMABUF/GTK path from process entry
// to the first GTK presentation. It deliberately rejects unknown, duplicate,
// and out-of-order milestones so collected cold-start measurements remain
// truthful.
type startupTrace struct {
	mu               sync.Mutex
	now              func() time.Time
	t0               time.Time
	milestones       []startupMilestone
	logger           *zerolog.Logger
	buffered         []startupMilestone
	backend          string
	incompleteReason string
	summaryEmitted   bool
}

type startupMilestone struct {
	Name    string
	Elapsed time.Duration
	Delta   time.Duration
}

var startupMilestoneOrder = []string{
	"process_entry",
	"config_complete",
	"cef_library_load_begin",
	"cef_initialized",
	"browser_create_requested",
	"first_accelerated_paint_received",
	"first_dmabuf_texture_swap",
	"first_gtk_presentation",
}

var processStartupTrace struct {
	sync.Mutex
	trace *startupTrace
}

func newStartupTrace(now func() time.Time, processEntry time.Time) *startupTrace {
	return &startupTrace{
		now:        now,
		t0:         processEntry,
		milestones: make([]startupMilestone, 0, len(startupMilestoneOrder)),
		buffered:   make([]startupMilestone, 0, len(startupMilestoneOrder)),
	}
}

// ActivateStartupTrace starts the CEF-only first-presentation trace after GUI
// engine selection. The supplied timestamps are captured by the application at
// process entry and configuration completion, before CEF initialization.
func ActivateStartupTrace(processEntry, configComplete time.Time, logger *zerolog.Logger) {
	if processEntry.IsZero() || configComplete.IsZero() {
		return
	}

	processStartupTrace.Lock()
	defer processStartupTrace.Unlock()
	if processStartupTrace.trace != nil {
		return
	}

	trace := newStartupTrace(time.Now, processEntry)
	if !trace.markAt("process_entry", processEntry) || !trace.markAt("config_complete", configComplete) {
		return
	}
	trace.SetLogger(logger)
	processStartupTrace.trace = trace
}

func activeStartupTrace() *startupTrace {
	processStartupTrace.Lock()
	defer processStartupTrace.Unlock()
	return processStartupTrace.trace
}

func (st *startupTrace) SetLogger(logger *zerolog.Logger) {
	if st == nil || logger == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.logger = logger
	for _, milestone := range st.buffered {
		st.emitMilestone(milestone)
	}
	st.buffered = nil
	st.emitSummaryLocked()
}

func (st *startupTrace) SetBackend(backend string) {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.backend = backend
}

func (st *startupTrace) SetIncompleteReason(reason string) {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.incompleteReason == "" {
		st.incompleteReason = reason
	}
}

// Mark accepts exactly the next non-presentation startup milestone.
func (st *startupTrace) Mark(name string) bool {
	if name == "first_gtk_presentation" {
		return false
	}
	return st.markAt(name, st.currentTime())
}

// MarkGTKAfterPaint records the final milestone from the CEF-to-GTK bridge's
// first frame-clock after-paint callback.
func (st *startupTrace) MarkGTKAfterPaint() bool {
	return st.markAt("first_gtk_presentation", st.currentTime())
}

func (st *startupTrace) currentTime() time.Time {
	if st == nil || st.now == nil {
		return time.Time{}
	}
	return st.now()
}

func (st *startupTrace) markAt(name string, at time.Time) bool {
	if st == nil || st.now == nil || at.IsZero() {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if at.Before(st.t0) || len(st.milestones) >= len(startupMilestoneOrder) || name != startupMilestoneOrder[len(st.milestones)] {
		return false
	}

	milestone := startupMilestone{Name: name, Elapsed: at.Sub(st.t0)}
	previousMilliseconds := int64(0)
	if len(st.milestones) > 0 {
		previousMilliseconds = st.milestones[len(st.milestones)-1].Elapsed.Milliseconds()
	}
	milestone.Delta = time.Duration(milestone.Elapsed.Milliseconds()-previousMilliseconds) * time.Millisecond
	st.milestones = append(st.milestones, milestone)
	if st.logger == nil {
		st.buffered = append(st.buffered, milestone)
	} else {
		st.emitMilestone(milestone)
	}
	st.emitSummaryLocked()
	return true
}

func (st *startupTrace) emitMilestone(m startupMilestone) {
	if st.logger == nil {
		return
	}
	st.logger.Debug().
		Str("milestone", m.Name).
		Int64("t_ms", m.Elapsed.Milliseconds()).
		Int64("delta_ms", m.Delta.Milliseconds()).
		Msg("startup_trace: milestone")
}

func (st *startupTrace) emitSummaryLocked() {
	if st.summaryEmitted || st.logger == nil || len(st.milestones) != len(startupMilestoneOrder) {
		return
	}
	st.summaryEmitted = true
	st.logger.Info().
		Str("backend", st.backend).
		Str("incomplete_reason", st.incompleteReason).
		Int64("total_ms", st.milestones[len(st.milestones)-1].Elapsed.Milliseconds()).
		Interface("milestones", st.milestones).
		Msg("startup_trace: first presentation")
}

func (st *startupTrace) Enabled() bool { return st != nil && st.now != nil }

func (st *startupTrace) TotalElapsed() time.Duration {
	if st == nil || st.now == nil {
		return 0
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.now().Sub(st.t0)
}
