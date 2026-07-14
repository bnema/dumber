package cef2gtk

import (
	"sync"
	"time"
)

// DiagnosticEvent records notable rendering-path events and failures.
type DiagnosticEvent struct {
	Time    time.Time
	Kind    string
	Message string
}

const maxDiagnosticEvents = 256

// Diagnostics is a snapshot of View rendering diagnostics. Events contains the
// most recent diagnostic events, capped to avoid unbounded growth.
type Diagnostics struct {
	Backend                 string
	AcceleratedPaints       int
	AcceleratedPaintErrors  int
	StaleAcceleratedPaints  int
	UnsupportedPaints       int
	ImportFailures          int
	RenderFailures          int
	TexturesBuilt           int
	TextureBuildFailures    int
	FDDupFailures           int
	UnsupportedFormats      int
	PaintableSwaps          int
	PendingFrame            bool
	PendingScheduled        bool
	PendingAge              time.Duration
	PendingSourceID         uint
	PendingReschedules      int
	PendingScheduleFailures int
	PendingIdleCallbacks    int
	Events                  []DiagnosticEvent
}

type diagnosticsRecorder struct {
	mu        sync.Mutex
	d         Diagnostics
	eventHead int
	eventFull bool
}

func newDiagnosticsRecorder() *diagnosticsRecorder { return &diagnosticsRecorder{} }

func (r *diagnosticsRecorder) Snapshot() Diagnostics {
	if r == nil {
		return Diagnostics{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.d
	if r.eventFull && len(r.d.Events) == maxDiagnosticEvents {
		out.Events = make([]DiagnosticEvent, 0, maxDiagnosticEvents)
		out.Events = append(out.Events, r.d.Events[r.eventHead:]...)
		out.Events = append(out.Events, r.d.Events[:r.eventHead]...)
		return out
	}
	out.Events = append([]DiagnosticEvent(nil), r.d.Events...)
	return out
}

func (r *diagnosticsRecorder) RecordAcceleratedPaint() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.d.AcceleratedPaints++
	r.appendEventLocked(DiagnosticEvent{Time: time.Now(), Kind: "accelerated-paint"})
}

func (r *diagnosticsRecorder) RecordStaleAcceleratedPaint() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.d.StaleAcceleratedPaints++
	r.appendEventLocked(DiagnosticEvent{Time: time.Now(), Kind: "stale-accelerated-paint"})
}

func (r *diagnosticsRecorder) RecordImportFailure(err error) {
	if r == nil {
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.d.AcceleratedPaintErrors++
	r.d.ImportFailures++
	r.appendEventLocked(DiagnosticEvent{Time: time.Now(), Kind: "import-failure", Message: msg})
}

func (r *diagnosticsRecorder) RecordRenderFailure(err error) {
	if r == nil {
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.d.AcceleratedPaintErrors++
	r.d.RenderFailures++
	r.appendEventLocked(DiagnosticEvent{Time: time.Now(), Kind: "render-failure", Message: msg})
}

func (r *diagnosticsRecorder) RecordUnsupportedPaint() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.d.UnsupportedPaints++
	r.appendEventLocked(DiagnosticEvent{Time: time.Now(), Kind: "unsupported-paint"})
}

func (r *diagnosticsRecorder) appendEventLocked(event DiagnosticEvent) {
	if len(r.d.Events) < maxDiagnosticEvents {
		r.d.Events = append(r.d.Events, event)
		r.eventHead = len(r.d.Events) % maxDiagnosticEvents
		return
	}
	r.d.Events[r.eventHead] = event
	r.eventHead = (r.eventHead + 1) % maxDiagnosticEvents
	r.eventFull = true
}
