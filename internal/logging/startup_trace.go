package logging

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// StartupTrace tracks cold start milestones from process launch to first paint.
// Thread-safe for use across goroutines.
// Enabled only when log level is debug or trace.
type StartupTrace struct {
	mu         sync.Mutex
	t0         time.Time
	milestones []Milestone
	enabled    bool
	logger     *zerolog.Logger
	buffered   []Milestone // milestones recorded before logger is set
	finished   bool
}

// Milestone represents a timing checkpoint during startup.
type Milestone struct {
	Name    string
	Elapsed time.Duration // time since t0
	Delta   time.Duration // time since previous milestone
}

var (
	globalTrace     *StartupTrace
	globalTraceMu   sync.Mutex
	globalTraceOnce sync.Once
)

// InitStartupTrace initializes the global startup trace.
// Call this as early as possible in main() to capture T0.
// The trace is enabled only if the given log level is debug or trace.
func InitStartupTrace(logLevel string) {
	globalTraceOnce.Do(func() {
		enabled := logLevel == "debug" || logLevel == "trace"
		globalTraceMu.Lock()
		globalTrace = &StartupTrace{
			t0:         time.Now(),
			milestones: make([]Milestone, 0, 32),
			enabled:    enabled,
			buffered:   make([]Milestone, 0, 8),
		}
		globalTraceMu.Unlock()

		if enabled {
			// Record process_start as first milestone
			globalTrace.Mark("process_start")
		}
	})
}

// Trace returns the global startup trace instance.
// Returns a no-op trace if not initialized or disabled.
func Trace() *StartupTrace {
	globalTraceMu.Lock()
	defer globalTraceMu.Unlock()
	if globalTrace == nil {
		// Return a disabled no-op trace
		return &StartupTrace{enabled: false}
	}
	return globalTrace
}

// SetLogger sets the logger for emitting milestone logs.
// Flushes any buffered milestones that were recorded before the logger was set.
func (st *StartupTrace) SetLogger(logger *zerolog.Logger) {
	if st == nil || !st.enabled {
		return
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	st.logger = logger

	// Flush buffered milestones
	for _, m := range st.buffered {
		st.emitMilestone(m)
	}
	st.buffered = nil
}

// UpdateLogger updates the logger and re-emits all milestones to the new logger.
// Call this after the session file logger is available to capture milestones in the log file.
func (st *StartupTrace) UpdateLogger(logger *zerolog.Logger) {
	if st == nil || !st.enabled {
		return
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	st.logger = logger

	// Re-emit all milestones to the new logger
	for _, m := range st.milestones {
		st.emitMilestone(m)
	}
}

// Mark records a milestone with the given name.
// Emits a debug log line showing elapsed time and delta from previous milestone.
func (st *StartupTrace) Mark(name string) {
	if st == nil || !st.enabled {
		return
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if st.finished {
		return
	}

	now := time.Now()
	elapsed := now.Sub(st.t0)

	var delta time.Duration
	if len(st.milestones) > 0 {
		delta = elapsed - st.milestones[len(st.milestones)-1].Elapsed
	}

	m := Milestone{
		Name:    name,
		Elapsed: elapsed,
		Delta:   delta,
	}
	st.milestones = append(st.milestones, m)

	if st.logger != nil {
		st.emitMilestone(m)
	} else {
		// Buffer until logger is set
		st.buffered = append(st.buffered, m)
	}
}

// emitMilestone logs a single milestone. Caller must hold mutex.
func (st *StartupTrace) emitMilestone(m Milestone) {
	if st.logger == nil {
		return
	}

	elapsedMs := m.Elapsed.Milliseconds()

	event := st.logger.Debug().
		Str("milestone", m.Name).
		Int64("t_ms", elapsedMs)

	if m.Delta > 0 {
		event = event.Int64("delta_ms", m.Delta.Milliseconds())
	}

	// Format: startup_trace: milestone_name (T+123ms, +45ms)
	if m.Delta > 0 {
		event.Msgf("startup_trace: %s (T+%dms, +%dms)", m.Name, elapsedMs, m.Delta.Milliseconds())
	} else {
		event.Msgf("startup_trace: %s (T+%dms)", m.Name, elapsedMs)
	}
}

// Finish marks the startup trace as complete and emits a summary.
// Called on first_paint milestone.
func (st *StartupTrace) Finish() {
	if st == nil || !st.enabled {
		return
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if st.finished {
		return
	}
	st.finished = true

	if st.logger == nil {
		return
	}

	// Build summary string
	total := time.Since(st.t0)
	var parts []string
	for _, m := range st.milestones {
		parts = append(parts, fmt.Sprintf("%s:%d", m.Name, m.Elapsed.Milliseconds()))
	}
	summary := strings.Join(parts, ",")

	st.logger.Info().
		Int64("total_ms", total.Milliseconds()).
		Str("milestones", summary).
		Msg("startup_trace: cold start complete")
}

// Enabled returns whether the trace is active.
func (st *StartupTrace) Enabled() bool {
	if st == nil {
		return false
	}
	return st.enabled
}

// TotalElapsed returns the total time since T0.
func (st *StartupTrace) TotalElapsed() time.Duration {
	if st == nil || !st.enabled {
		return 0
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	return time.Since(st.t0)
}
