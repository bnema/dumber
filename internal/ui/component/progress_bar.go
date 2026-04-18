// Package component provides UI components for the browser.
package component

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/ui/layout"
)

const (
	// Animation step size - how much to increment per frame
	progressStep = 0.02
	// Animation interval in milliseconds
	progressIntervalMs = 16 // ~60fps
	// Timeout to auto-hide progress bar if stuck (30 seconds)
	progressTimeoutMs = 30000
)

// ProgressBar displays a slim loading progress indicator at the bottom of a pane.
// Uses native GtkProgressBar with "osd" styling for overlay appearance.
// Implements smooth animation by incrementing towards the target value.
// Includes a 30-second timeout to auto-hide if the page load stalls.
type ProgressBar struct {
	ctx         context.Context
	progressBar layout.ProgressBarWidget

	visible          bool
	currentValue     float64 // Current displayed value
	targetValue      float64 // Target value to animate towards
	animationTimer   uint    // Timer source ID for animation
	timeoutTimer     uint    // Timer source ID for auto-hide timeout
	lastShowAt       time.Time
	lastProgressAt   time.Time
	progressEventSeq uint64

	mu sync.Mutex
}

// NewProgressBar creates a new progress bar component using the widget factory.
func NewProgressBar(ctx context.Context, factory layout.WidgetFactory) *ProgressBar {
	progressBar := factory.NewProgressBar()

	// Add "osd" class for on-screen-display overlay styling (like Epiphany)
	progressBar.AddCssClass("osd")

	// Position at bottom, full width
	progressBar.SetValign(gtk.AlignEndValue)
	progressBar.SetHalign(gtk.AlignFillValue)
	progressBar.SetHexpand(true)

	// Set minimum size to prevent GTK warning about negative minimum width (-2)
	// The internal "progress" gizmo needs valid dimensions before realization
	// Using 0 for width (not -1) ensures GTK doesn't calculate negative sizes
	progressBar.SetSizeRequest(0, 4)

	// Don't intercept pointer events - let clicks pass through to WebView
	progressBar.SetCanTarget(false)
	progressBar.SetCanFocus(false)

	// Initialize fraction to 0
	progressBar.SetFraction(0)

	// Hidden by default
	progressBar.SetVisible(false)

	return &ProgressBar{
		ctx:          ctx,
		progressBar:  progressBar,
		visible:      false,
		currentValue: 0,
		targetValue:  0,
	}
}

// SetProgress sets the target progress value and starts smooth animation.
// progress should be between 0.0 and 1.0.
func (pb *ProgressBar) SetProgress(progress float64) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	incomingProgress := progress
	ctx := pb.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	logging.FromContext(ctx).
		Trace().
		Float64("incoming_progress", incomingProgress).
		Float64("current_value", pb.currentValue).
		Float64("target_value", pb.targetValue).
		Bool("visible", pb.visible).
		Bool("animation_running", pb.animationTimer != 0).
		Bool("timeout_running", pb.timeoutTimer != 0).
		Int64("since_show_ms", sinceTimeMs(pb.lastShowAt, now)).
		Int64("since_last_progress_ms", sinceTimeMs(pb.lastProgressAt, now)).
		Msg("setting progress")

	// Clamp progress to valid range
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}

	pb.targetValue = progress
	pb.lastProgressAt = now
	pb.progressEventSeq++
	if pb.visible {
		pb.resetTimeout()
	}

	// For large jumps (>0.3) or completion, update immediately
	diff := progress - pb.currentValue
	if diff > 0.3 || progress >= 1.0 {
		pb.currentValue = progress
		pb.progressBar.SetFraction(progress)
		return
	}

	// Start animation if not already running
	if pb.animationTimer == 0 {
		pb.startAnimation()
	}
}

// startAnimation begins the smooth progress animation.
// Must be called with lock held.
func (pb *ProgressBar) startAnimation() {
	cb := glib.SourceFunc(func(_ uintptr) bool {
		pb.mu.Lock()
		defer pb.mu.Unlock()

		// Check if we've reached the target
		if pb.currentValue >= pb.targetValue {
			pb.animationTimer = 0
			return false // Stop the timer
		}

		// Increment towards target
		pb.currentValue += progressStep
		if pb.currentValue > pb.targetValue {
			pb.currentValue = pb.targetValue
		}

		pb.progressBar.SetFraction(pb.currentValue)

		// Continue animation if not at target
		if pb.currentValue >= pb.targetValue {
			pb.animationTimer = 0 // Clear before GLib auto-removes
			return false
		}
		return true
	})

	pb.animationTimer = glib.TimeoutAdd(progressIntervalMs, &cb, 0)
	ctx := pb.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	logging.FromContext(ctx).
		Trace().
		Uint64("progress_event_seq", pb.progressEventSeq).
		Float64("current_value", pb.currentValue).
		Float64("target_value", pb.targetValue).
		Msg("progress bar animation started")
}

// initialProgressFraction is the fraction set when the progress bar first
// appears so the user gets immediate visual feedback that loading has begun.
// CEF may not fire OnLoadingProgressChange for hundreds of milliseconds
// during cross-site process swaps, leaving the bar at 0% (invisible).
const initialProgressFraction = 0.1

// Show makes the progress bar visible and starts the auto-hide timeout.
func (pb *ProgressBar) Show() {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	ctx := pb.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	logging.FromContext(ctx).
		Debug().
		Bool("visible", pb.visible).
		Float64("current_value", pb.currentValue).
		Float64("target_value", pb.targetValue).
		Msg("progress bar show before")

	if !pb.visible {
		pb.visible = true
		pb.lastShowAt = time.Now()
		// Set an initial non-zero fraction so the bar is visually noticeable
		// immediately. Without this, the bar is technically visible but
		// renders as empty (0% fill) until the first progress callback,
		// which in CEF can be delayed during cross-site process swaps.
		pb.currentValue = initialProgressFraction
		pb.targetValue = initialProgressFraction
		pb.progressBar.SetFraction(initialProgressFraction)
		pb.progressBar.SetVisible(true)
	}

	// Reset timeout timer on every Show call.
	pb.resetTimeout()

	logging.FromContext(ctx).
		Debug().
		Bool("visible", pb.visible).
		Float64("current_value", pb.currentValue).
		Float64("target_value", pb.targetValue).
		Msg("progress bar show after")
}

// resetTimeout cancels any existing timeout and starts a new one.
// Must be called with lock held.
func (pb *ProgressBar) resetTimeout() {
	pb.resetTimeoutAfter(time.Duration(progressTimeoutMs) * time.Millisecond)
}

// resetTimeoutAfter cancels any existing timeout and starts a new one after
// the requested delay. Must be called with lock held.
func (pb *ProgressBar) resetTimeoutAfter(delay time.Duration) {
	if delay <= 0 {
		delay = time.Millisecond
	}

	// Cancel existing timeout.
	if pb.timeoutTimer != 0 {
		glib.SourceRemove(pb.timeoutTimer)
		pb.timeoutTimer = 0
	}

	timeoutMs := int(delay / time.Millisecond)
	if delay%time.Millisecond != 0 {
		timeoutMs++
	}
	if timeoutMs < 1 {
		timeoutMs = 1
	}

	cb := glib.SourceFunc(func(_ uintptr) bool {
		pb.mu.Lock()
		defer pb.mu.Unlock()

		now := time.Now()
		remaining := pb.timeoutRemainingLocked(now)
		ctx := pb.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		logging.FromContext(ctx).
			Debug().
			Float64("current_value", pb.currentValue).
			Float64("target_value", pb.targetValue).
			Int64("since_show_ms", sinceTimeMs(pb.lastShowAt, now)).
			Int64("since_last_progress_ms", sinceTimeMs(pb.lastProgressAt, now)).
			Int64("timeout_remaining_ms", remaining.Milliseconds()).
			Msg("progress bar timeout fired")

		// Clear timer ID (timer is being removed).
		pb.timeoutTimer = 0

		if remaining > 0 {
			pb.resetTimeoutAfter(remaining)
			return false
		}

		// Hide the progress bar inline to avoid race condition.
		pb.hideInternal("timeout")
		return false // Don't repeat
	})

	pb.timeoutTimer = glib.TimeoutAdd(uint(timeoutMs), &cb, 0)
}

// Hide makes the progress bar invisible and resets state.
func (pb *ProgressBar) Hide() {
	pb.HideWithReason("manual")
}

// HideWithReason makes the progress bar invisible and records why it was hidden.
func (pb *ProgressBar) HideWithReason(reason string) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.hideInternal(reason)
}

// hideInternal performs the actual hide operation.
// Must be called with lock held.
func (pb *ProgressBar) hideInternal(reason string) {
	ctx := pb.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	logging.FromContext(ctx).
		Debug().
		Bool("visible", pb.visible).
		Str("reason", reason).
		Float64("current_value", pb.currentValue).
		Float64("target_value", pb.targetValue).
		Bool("animation_running", pb.animationTimer != 0).
		Bool("timeout_running", pb.timeoutTimer != 0).
		Int64("since_show_ms", sinceTimeMs(pb.lastShowAt, now)).
		Int64("since_last_progress_ms", sinceTimeMs(pb.lastProgressAt, now)).
		Msg("progress bar hide before")

	if pb.visible {
		pb.visible = false
		pb.progressBar.SetVisible(false)

		// Stop any running animation
		if pb.animationTimer != 0 {
			glib.SourceRemove(pb.animationTimer)
			pb.animationTimer = 0
		}

		// Stop timeout timer
		if pb.timeoutTimer != 0 {
			glib.SourceRemove(pb.timeoutTimer)
			pb.timeoutTimer = 0
		}

		// Reset values
		pb.currentValue = 0
		pb.targetValue = 0
		pb.lastShowAt = time.Time{}
		pb.lastProgressAt = time.Time{}
		pb.progressBar.SetFraction(0)
	}
}

func (pb *ProgressBar) timeoutRemainingLocked(now time.Time) time.Duration {
	lastActivityAt := pb.lastShowAt
	if pb.lastProgressAt.After(lastActivityAt) {
		lastActivityAt = pb.lastProgressAt
	}
	if lastActivityAt.IsZero() {
		return 0
	}

	remaining := time.Duration(progressTimeoutMs)*time.Millisecond - now.Sub(lastActivityAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func sinceTimeMs(start, now time.Time) int64 {
	if start.IsZero() {
		return -1
	}
	return now.Sub(start).Milliseconds()
}

// IsVisible returns whether the progress bar is currently visible.
func (pb *ProgressBar) IsVisible() bool {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return pb.visible
}

// Widget returns the underlying widget for embedding in overlays.
func (pb *ProgressBar) Widget() layout.Widget {
	return pb.progressBar
}
