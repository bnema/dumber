package logging

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// CEFStartupTrace records the accelerated CEF/DMABUF/GTK path from process
// entry to the first GTK presentation. It deliberately rejects unknown,
// duplicate, and out-of-order milestones. Accepting a convenient timestamp
// would make the result unsuitable for cold-start measurement.
type CEFStartupTrace struct {
	mu               sync.Mutex
	now              func() time.Time
	t0               time.Time
	milestones       []CEFStartupMilestone
	logger           *zerolog.Logger
	buffered         []CEFStartupMilestone
	backend          string
	incompleteReason string
	summaryEmitted   bool
}

// CEFStartupMilestone represents a timing checkpoint from process entry.
type CEFStartupMilestone struct {
	Name    string
	Elapsed time.Duration
	Delta   time.Duration
}

var cefStartupMilestoneOrder = []string{
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
	globalCEFTrace     *CEFStartupTrace
	globalCEFTraceMu   sync.Mutex
	globalCEFTraceOnce sync.Once
)

func newCEFStartupTrace(now func() time.Time) *CEFStartupTrace {
	start := now()
	return &CEFStartupTrace{
		now:        now,
		t0:         start,
		milestones: make([]CEFStartupMilestone, 0, len(cefStartupMilestoneOrder)),
		buffered:   make([]CEFStartupMilestone, 0, len(cefStartupMilestoneOrder)),
	}
}

// NewCEFStartupTrace creates an isolated accelerated CEF/DMABUF/GTK trace.
// Production startup uses InitCEFStartupTrace and CEFTrace.
func NewCEFStartupTrace() *CEFStartupTrace { return newCEFStartupTrace(time.Now) }

// InitCEFStartupTrace begins the CEF/DMABUF/GTK measurement at process entry.
// It is intentionally safe to call again during GUI bootstrap.
func InitCEFStartupTrace() {
	globalCEFTraceOnce.Do(func() {
		trace := newCEFStartupTrace(time.Now)
		globalCEFTraceMu.Lock()
		globalCEFTrace = trace
		globalCEFTraceMu.Unlock()
	})
}

// CEFTrace returns the accelerated CEF/DMABUF/GTK trace. Callers before
// initialization get a harmless trace which rejects all transitions.
func CEFTrace() *CEFStartupTrace {
	globalCEFTraceMu.Lock()
	defer globalCEFTraceMu.Unlock()
	if globalCEFTrace == nil {
		return &CEFStartupTrace{}
	}
	return globalCEFTrace
}

// SetLogger makes buffered milestones available to the process logger.
func (st *CEFStartupTrace) SetLogger(logger *zerolog.Logger) {
	if st == nil || logger == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.logger = logger
	for _, milestone := range st.buffered {
		st.emitCEFStartupMilestone(milestone)
	}
	st.buffered = nil
	st.emitSummaryLocked()
}

// SetBackend records the selected presentation backend for the final summary.
func (st *CEFStartupTrace) SetBackend(backend string) {
	if st == nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.backend = backend
}

// SetIncompleteReason records why a trace cannot reach a valid DMABUF result.
func (st *CEFStartupTrace) SetIncompleteReason(reason string) {
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
func (st *CEFStartupTrace) Mark(name string) bool {
	if name == "first_gtk_presentation" {
		return false
	}
	return st.mark(name)
}

// MarkGTKAfterPaint records the final milestone. It is called only from the
// upstream CEF-to-GTK bridge's first frame-clock after-paint callback.
func (st *CEFStartupTrace) MarkGTKAfterPaint() bool {
	return st.mark("first_gtk_presentation")
}

// mark accepts the next milestone and is safe for callbacks from CEF and GTK
// threads.
func (st *CEFStartupTrace) mark(name string) bool {
	if st == nil || st.now == nil {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.milestones) >= len(cefStartupMilestoneOrder) || name != cefStartupMilestoneOrder[len(st.milestones)] {
		return false
	}

	now := st.now()
	milestone := CEFStartupMilestone{Name: name, Elapsed: now.Sub(st.t0)}
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
		st.emitCEFStartupMilestone(milestone)
	}
	st.emitSummaryLocked()
	return true
}

func (st *CEFStartupTrace) emitCEFStartupMilestone(m CEFStartupMilestone) {
	if st.logger == nil {
		return
	}
	st.logger.Debug().
		Str("milestone", m.Name).
		Int64("t_ms", m.Elapsed.Milliseconds()).
		Int64("delta_ms", m.Delta.Milliseconds()).
		Msg("startup_trace: milestone")
}

func (st *CEFStartupTrace) emitSummaryLocked() {
	if st.summaryEmitted || st.logger == nil || len(st.milestones) != len(cefStartupMilestoneOrder) {
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

func (st *CEFStartupTrace) Enabled() bool { return st != nil && st.now != nil }

func (st *CEFStartupTrace) TotalElapsed() time.Duration {
	if st == nil || st.now == nil {
		return 0
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.now().Sub(st.t0)
}
