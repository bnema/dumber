package layout

import (
	"sync"
)

// SplitView wraps a GTK Paned widget for creating split pane layouts.
// It manages two child widgets separated by a draggable divider.
type SplitView struct {
	paned       PanedWidget
	orientation Orientation
	startChild  Widget
	endChild    Widget
	ratio       float64 // 0.0-1.0, relative position of divider

	mu sync.RWMutex
}

// NewSplitView creates a new split view with the given orientation and children.
// The ratio determines the initial divider position (0.0-1.0).
func NewSplitView(factory WidgetFactory, orientation Orientation, startChild, endChild Widget, ratio float64) *SplitView {
	paned := factory.NewPaned(orientation)

	// Configure resize behavior - both children should resize
	paned.SetResizeStartChild(true)
	paned.SetResizeEndChild(true)

	// Prevent children from shrinking below minimum size
	paned.SetShrinkStartChild(false)
	paned.SetShrinkEndChild(false)

	sv := &SplitView{
		paned:       paned,
		orientation: orientation,
		startChild:  startChild,
		endChild:    endChild,
		ratio:       clampRatio(ratio),
	}

	// Set children
	if startChild != nil {
		paned.SetStartChild(startChild)
	}
	if endChild != nil {
		paned.SetEndChild(endChild)
	}

	// Apply ratio after widget is mapped (when we know the allocated size)
	paned.ConnectMap(func() {
		sv.ApplyRatio()
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

// ApplyRatio converts the ratio to a pixel position and applies it.
// This should be called after the widget has been mapped and has an allocated size.
func (sv *SplitView) ApplyRatio() {
	sv.mu.RLock()
	ratio := sv.ratio
	orientation := sv.orientation
	sv.mu.RUnlock()

	// Get the relevant dimension based on orientation
	var totalSize int
	if orientation == OrientationHorizontal {
		totalSize = sv.paned.GetAllocatedWidth()
	} else {
		totalSize = sv.paned.GetAllocatedHeight()
	}

	if totalSize <= 0 {
		return // Not yet allocated
	}

	// Calculate position from ratio
	position := int(float64(totalSize) * ratio)
	sv.paned.SetPosition(position)
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
