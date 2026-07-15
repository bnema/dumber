package layout

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/glib"
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
	idleAdd             func(*glib.SourceFunc, uintptr) uint
	notifyDebounceTimer *time.Timer
	notifyGeneration    uint64
	tickCallbackID      uint
	cleanedUp           bool

	hasAppliedRatio     bool
	suppressNotifyUntil time.Time

	mu sync.RWMutex
}

// maxRetryFrames is the maximum number of frames to wait for allocation to stabilize.
// At 60fps, 120 frames = ~2 seconds timeout.
const maxRetryFrames = 120

const notifyPositionDebounceDelay = 100 * time.Millisecond
const notifyPositionSuppressAfterApplyDelay = 50 * time.Millisecond
const splitAllocationSettledTolerance = 2

func childAllocation(widget Widget) (int, int) {
	if widget == nil {
		return 0, 0
	}
	return widget.GetAllocatedWidth(), widget.GetAllocatedHeight()
}

func splitAllocationsSettled(
	orientation Orientation, totalSize, targetPosition, actualPosition int,
	startWidth, startHeight, endWidth, endHeight int,
) bool {
	if absInt(actualPosition-targetPosition) > splitAllocationSettledTolerance {
		return false
	}

	if targetPosition <= 0 || targetPosition >= totalSize {
		return true
	}

	startSize, endSize := startWidth, endWidth
	if orientation == OrientationVertical {
		startSize, endSize = startHeight, endHeight
	}

	if startSize <= splitAllocationSettledTolerance || endSize <= splitAllocationSettledTolerance {
		return false
	}

	expectedStart := targetPosition
	expectedEnd := totalSize - targetPosition
	if absInt(startSize-expectedStart) > splitAllocationSettledTolerance {
		return false
	}
	if absInt(endSize-expectedEnd) > splitAllocationSettledTolerance {
		return false
	}

	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

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
		idleAdd:     glib.IdleAdd,
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

		now := time.Now()

		sv.mu.Lock()
		// Ignore early notifications before we've applied the snapshot ratio.
		// GTK often emits an initial notify::position (typically 50/50) during allocation.
		if !sv.hasAppliedRatio {
			sv.mu.Unlock()
			return
		}
		if !sv.suppressNotifyUntil.IsZero() && now.Before(sv.suppressNotifyUntil) {
			sv.mu.Unlock()
			return
		}
		sv.mu.Unlock()

		ratio := clampRatio(float64(position) / float64(totalSize))

		sv.mu.Lock()
		if sv.cleanedUp {
			sv.mu.Unlock()
			return
		}
		sv.ratio = ratio
		sv.notifyGeneration++
		generation := sv.notifyGeneration

		if sv.notifyDebounceTimer != nil {
			sv.notifyDebounceTimer.Stop()
		}
		sv.notifyDebounceTimer = time.AfterFunc(notifyPositionDebounceDelay, func() {
			sv.mu.RLock()
			valid := !sv.cleanedUp && generation == sv.notifyGeneration
			sv.mu.RUnlock()
			if !valid {
				return
			}

			cb := glib.SourceFunc(func(_ uintptr) bool {
				sv.mu.RLock()
				if sv.cleanedUp || generation != sv.notifyGeneration {
					sv.mu.RUnlock()
					return false
				}
				onRatioChanged := sv.onRatioChanged
				sv.mu.RUnlock()

				if onRatioChanged != nil {
					onRatioChanged(ratio)
				}
				return false
			})
			sv.idleAdd(&cb, 0)
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
	sv.tickCallbackID = paned.AddTickCallback(func() bool {
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
// Cleanup releases callbacks owned by this split. It is idempotent because
// tree rebuild and GTK teardown can both reach the same view.
func (sv *SplitView) Cleanup() {
	sv.mu.Lock()
	if sv.cleanedUp {
		sv.mu.Unlock()
		return
	}
	sv.cleanedUp = true
	sv.notifyGeneration++
	sv.onRatioChanged = nil
	if sv.notifyDebounceTimer != nil {
		sv.notifyDebounceTimer.Stop()
		sv.notifyDebounceTimer = nil
	}
	tickCallbackID := sv.tickCallbackID
	sv.tickCallbackID = 0
	sv.mu.Unlock()

	if tickCallbackID != 0 {
		sv.paned.RemoveTickCallback(tickCallbackID)
	}
}

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
	startChild := sv.startChild
	endChild := sv.endChild
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
	startWidthBefore, startHeightBefore := childAllocation(startChild)
	endWidthBefore, endHeightBefore := childAllocation(endChild)
	currentPositionBeforeSet := sv.paned.GetPosition()
	sv.logger.Debug().
		Str("orientation", orientStr).
		Int("total_size", totalSize).
		Float64("ratio", ratio).
		Int("position", position).
		Int("current_position_before_set", currentPositionBeforeSet).
		Int("start_child_alloc_width", startWidthBefore).
		Int("start_child_alloc_height", startHeightBefore).
		Int("end_child_alloc_width", endWidthBefore).
		Int("end_child_alloc_height", endHeightBefore).
		Msg("ApplyRatio: setting position")

	// Avoid treating programmatic SetPosition as a user resize.
	// We suppress notify::position briefly; GTK may emit during/after SetPosition.
	sv.mu.Lock()
	sv.suppressNotifyUntil = time.Now().Add(notifyPositionSuppressAfterApplyDelay)
	sv.mu.Unlock()

	sv.paned.SetPosition(position)

	startWidthAfter, startHeightAfter := childAllocation(startChild)
	endWidthAfter, endHeightAfter := childAllocation(endChild)
	actualPositionAfterSet := sv.paned.GetPosition()
	childAllocationsSettled := splitAllocationsSettled(
		orientation, totalSize, position, actualPositionAfterSet,
		startWidthAfter, startHeightAfter, endWidthAfter, endHeightAfter,
	)
	sv.logger.Debug().
		Str("orientation", orientStr).
		Int("total_size", totalSize).
		Int("target_position", position).
		Int("actual_position_after_set", actualPositionAfterSet).
		Int("start_child_alloc_width", startWidthAfter).
		Int("start_child_alloc_height", startHeightAfter).
		Int("end_child_alloc_width", endWidthAfter).
		Int("end_child_alloc_height", endHeightAfter).
		Bool("child_allocations_settled", childAllocationsSettled).
		Msg("ApplyRatio: position applied")

	if !childAllocationsSettled {
		return false
	}

	sv.mu.Lock()
	sv.hasAppliedRatio = true
	sv.mu.Unlock()

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
