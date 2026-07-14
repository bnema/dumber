package logging

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// StartupTrace records the single truthful path from process entry to the first
// GTK presentation. It deliberately rejects unknown, duplicate, and out of
// order milestones: accepting a convenient timestamp would make the result
// unsuitable for cold-start measurement.
type StartupTrace struct {
	mu               sync.Mutex
	now              func() time.Time
	t0               time.Time
	milestones       []Milestone
	logger           *zerolog.Logger
	buffered         []Milestone
	backend          string
	incompleteReason string
	summaryEmitted   bool
}

// Milestone represents a timing checkpoint measured from process entry.
type Milestone struct {
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

var (
	globalTrace     *StartupTrace
	globalTraceMu   sync.Mutex
	globalTraceOnce sync.Once
)

func newStartupTrace(now func() time.Time) *StartupTrace {
	start := now()
	return &StartupTrace{
		now:        now,
		t0:         start,
		milestones: make([]Milestone, 0, len(startupMilestoneOrder)),
		buffered:   make([]Milestone, 0, len(startupMilestoneOrder)),
	}
}

// InitStartupTrace must be called at the first instruction in main. logLevel is
// retained for source compatibility; this measurement is always collected so
// its final structured summary is available at normal log level.
func InitStartupTrace(_ string) {
	globalTraceOnce.Do(func() {
		trace := newStartupTrace(time.Now)
		globalTraceMu.Lock()
		globalTrace = trace
		globalTraceMu.Unlock()
	})
}

// Trace returns the process startup trace. Callers before initialization get a
// harmless trace which rejects all transitions.
func Trace() *StartupTrace {
	globalTraceMu.Lock()
	defer globalTraceMu.Unlock()
	if globalTrace == nil {
		return &StartupTrace{}
	}
	return globalTrace
}

// SetLogger makes buffered milestones available to the process logger.
func (st *StartupTrace) SetLogger(logger *zerolog.Logger) {
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

// UpdateLogger switches the destination without replaying events. Replaying
// would violate the one-shot property in session logs.
func (st *StartupTrace) UpdateLogger(logger *zerolog.Logger) { st.SetLogger(logger) }

// SetBackend records the selected presentation backend for the final summary.
func (st *StartupTrace) SetBackend(backend string) {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.backend = backend
}

// SetIncompleteReason records why a trace cannot reach a valid DMABUF result.
func (st *StartupTrace) SetIncompleteReason(reason string) {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.incompleteReason == "" {
		st.incompleteReason = reason
	}
}

// Mark accepts exactly the next non-presentation startup milestone and returns
// whether it was recorded. first_gtk_presentation is reserved for the GTK
// frame-clock after-paint hook through MarkGTKAfterPaint.
func (st *StartupTrace) Mark(name string) bool {
	if name == "first_gtk_presentation" {
		return false
	}
	return st.mark(name)
}

// MarkGTKAfterPaint records the final milestone. It is called only from the
// upstream CEF-to-GTK bridge's first frame-clock after-paint callback.
func (st *StartupTrace) MarkGTKAfterPaint() bool {
	return st.mark("first_gtk_presentation")
}

// mark accepts the next milestone and is safe for callbacks from CEF and GTK
// threads.
func (st *StartupTrace) mark(name string) bool {
	if st == nil || st.now == nil {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.milestones) >= len(startupMilestoneOrder) || name != startupMilestoneOrder[len(st.milestones)] {
		return false
	}

	now := st.now()
	milestone := Milestone{Name: name, Elapsed: now.Sub(st.t0)}
	// Published fields are integer milliseconds. Compute delta on that same
	// scale so delta_ms always exactly reconstructs t_ms in collected evidence.
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

func (st *StartupTrace) emitMilestone(m Milestone) {
	if st.logger == nil {
		return
	}
	st.logger.Debug().
		Str("milestone", m.Name).
		Int64("t_ms", m.Elapsed.Milliseconds()).
		Int64("delta_ms", m.Delta.Milliseconds()).
		Msg("startup_trace: milestone")
}

func (st *StartupTrace) emitSummaryLocked() {
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

// Finish is retained as a harmless compatibility no-op. The only valid source
// for first_gtk_presentation is the upstream GTK frame-clock after-paint hook.
func (*StartupTrace) Finish() {}

func (st *StartupTrace) Enabled() bool { return st != nil && st.now != nil }

func (st *StartupTrace) TotalElapsed() time.Duration {
	if st == nil || st.now == nil {
		return 0
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.now().Sub(st.t0)
}
