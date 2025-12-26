package layout

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/rs/zerolog"
)

// SplitView wraps a GTK Paned widget for creating split pane layouts.
// It manages two child widgets separated by a draggable divider.
type SplitView struct {
	paned       PanedWidget
	orientation Orientation
	startChild  Widget
	endChild    Widget
	ratio       float64 // 0.0-1.0, relative position of divider
	logger      zerolog.Logger

	onRatioChanged      func(ratio float64)
	pendingNotifyRatio  float64
	notifyDebounceTimer *time.Timer

	mu sync.RWMutex
}

// maxRetryFrames is the maximum number of frames to wait for allocation to stabilize.
// At 60fps, 120 frames = ~2 seconds timeout.
const maxRetryFrames = 120

const notifyPositionDebounceDelay = 100 * time.Millisecond

// NewSplitView creates a new split view with the given orientation and children.
// The ratio determines the initial divider position (0.0-1.0).
func NewSplitView(
	ctx context.Context, factory WidgetFactory, orientation Orientation,
	startChild, endChild Widget, ratio float64,
) *SplitView {
	log := logging.FromContext(ctx)

	paned := factory.NewPaned(orientation)

	// Configure resize behavior - both children should resize
	// Note: Do NOT set SetShrinkStartChild/SetShrinkEndChild to false
	// as that prevents GTK from shrinking children to fit the ratio
	paned.SetResizeStartChild(true)
	paned.SetResizeEndChild(true)
	paned.SetVisible(true)

	sv := &SplitView{
		paned:       paned,
		orientation: orientation,
		startChild:  startChild,
		endChild:    endChild,
		ratio:       clampRatio(ratio),
		logger:      log.With().Str("component", "split-view").Logger(),
	}

	// Set children
	if startChild != nil {
		startChild.SetVisible(true)
		paned.SetStartChild(startChild)
	}
	if endChild != nil {
		endChild.SetVisible(true)
		paned.SetEndChild(endChild)
	}

	paned.ConnectNotifyPosition(func() {
		position := paned.GetPosition()
		var totalSize int
		if orientation == OrientationHorizontal {
			totalSize = paned.GetAllocatedWidth()
		} else {
			totalSize = paned.GetAllocatedHeight()
		}
		if totalSize <= 0 {
			return
		}

		ratio := clampRatio(float64(position) / float64(totalSize))

		sv.mu.Lock()
		sv.ratio = ratio
		sv.pendingNotifyRatio = ratio
		onRatioChanged := sv.onRatioChanged

		if sv.notifyDebounceTimer != nil {
			sv.notifyDebounceTimer.Stop()
		}
		sv.notifyDebounceTimer = time.AfterFunc(notifyPositionDebounceDelay, func() {
			if onRatioChanged == nil {
				return
			}
			cb := glib.SourceFunc(func(_ uintptr) bool {
				sv.mu.RLock()
				pending := sv.pendingNotifyRatio
				sv.mu.RUnlock()
				onRatioChanged(pending)
				return false
			})
			glib.IdleAdd(&cb, 0)
		})

		sv.mu.Unlock()
	})

	// Try to apply ratio immediately (in case widget is already allocated)
	if sv.ApplyRatio() {
		return sv
	}

	// Apply ratio after widget is mapped (when we know the allocated size)
	paned.ConnectMap(func() {
		sv.ApplyRatio()
	})

	// Add tick callback to retry applying ratio every frame until successful.
	// This handles cases where allocation isn't ready even after Map signal.
	frames := 0
	paned.AddTickCallback(func() bool {
		frames++
		if sv.ApplyRatio() {
			sv.logger.Debug().
				Int("frames", frames).
				Float64("ratio", sv.GetRatio()).
				Msg("tick callback: ratio applied successfully")
			return false // Stop ticking - ratio applied successfully
		}
		if frames >= maxRetryFrames {
			sv.logger.Warn().
				Int("frames", frames).
				Msg("tick callback: timeout reached, allocation never stabilized")
			return false // Stop ticking - timeout reached
		}
		return true // Continue ticking
	})

	return sv
}

// SetRatio updates the divider position.
// The ratio is clamped to the range [0.0, 1.0].
// Note: This sets the position based on ratio; actual pixel position
// depends on the allocated size of the paned widget.
func (sv *SplitView) SetRatio(ratio float64) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	sv.ratio = clampRatio(ratio)
}

// GetRatio returns the current ratio setting.
func (sv *SplitView) GetRatio() float64 {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	return sv.ratio
}

func (sv *SplitView) SetOnRatioChanged(fn func(ratio float64)) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	sv.onRatioChanged = fn
}

// ApplyRatio converts the ratio to a pixel position and applies it.
// This should be called after the widget has been mapped and has an allocated size.
// Returns true if the ratio was successfully applied, false if the widget is not yet allocated.
func (sv *SplitView) ApplyRatio() bool {
	sv.mu.RLock()
	ratio := sv.ratio
	orientation := sv.orientation
	sv.mu.RUnlock()

	// Get the relevant dimension based on orientation
	var totalSize int
	var orientStr string
	if orientation == OrientationHorizontal {
		totalSize = sv.paned.GetAllocatedWidth()
		orientStr = "horizontal"
	} else {
		totalSize = sv.paned.GetAllocatedHeight()
		orientStr = "vertical"
	}

	if totalSize <= 0 {
		sv.logger.Debug().
			Str("orientation", orientStr).
			Int("total_size", totalSize).
			Msg("ApplyRatio: not yet allocated")
		return false // Not yet allocated
	}

	// Calculate position from ratio
	position := int(float64(totalSize) * ratio)
	sv.logger.Debug().
		Str("orientation", orientStr).
		Int("total_size", totalSize).
		Float64("ratio", ratio).
		Int("position", position).
		Msg("ApplyRatio: setting position")
	sv.paned.SetPosition(position)
	return true
}

// SetPosition sets the divider position in pixels.
func (sv *SplitView) SetPosition(position int) {
	sv.paned.SetPosition(position)
}

// GetPosition returns the current divider position in pixels.
func (sv *SplitView) GetPosition() int {
	return sv.paned.GetPosition()
}

// SwapStart replaces the start (left/top) child widget.
// The old child is unparented before the new child is set.
func (sv *SplitView) SwapStart(newWidget Widget) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	// Remove old child from paned
	if sv.startChild != nil {
		sv.paned.SetStartChild(nil)
	}

	sv.startChild = newWidget

	if newWidget != nil {
		sv.paned.SetStartChild(newWidget)
	}
}

// SwapEnd replaces the end (right/bottom) child widget.
// The old child is unparented before the new child is set.
func (sv *SplitView) SwapEnd(newWidget Widget) {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	// Remove old child from paned
	if sv.endChild != nil {
		sv.paned.SetEndChild(nil)
	}

	sv.endChild = newWidget

	if newWidget != nil {
		sv.paned.SetEndChild(newWidget)
	}
}

// StartChild returns the current start (left/top) child.
func (sv *SplitView) StartChild() Widget {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	return sv.startChild
}

// EndChild returns the current end (right/bottom) child.
func (sv *SplitView) EndChild() Widget {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	return sv.endChild
}

// Orientation returns the split orientation.
func (sv *SplitView) Orientation() Orientation {
	return sv.orientation
}

// SetWideHandle sets whether the handle has a wide appearance.
func (sv *SplitView) SetWideHandle(wide bool) {
	sv.paned.SetWideHandle(wide)
}

// Widget returns the underlying PanedWidget for embedding in containers.
func (sv *SplitView) Widget() Widget {
	return sv.paned
}

// Paned returns the underlying PanedWidget for direct access.
func (sv *SplitView) Paned() PanedWidget {
	return sv.paned
}

// clampRatio ensures the ratio is within [0.0, 1.0].
func clampRatio(ratio float64) float64 {
	if ratio < 0.0 {
		return 0.0
	}
	if ratio > 1.0 {
		return 1.0
	}
	return ratio
}
