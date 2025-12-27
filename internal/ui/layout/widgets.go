// Package layout provides GTK widget abstractions and layout management for the pane system.
// It defines interfaces that wrap GTK types, enabling unit testing without GTK runtime.
package layout

import (
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// Orientation represents the orientation for layout widgets.
type Orientation = gtk.Orientation

// Orientation constants matching GTK values.
const (
	OrientationHorizontal = gtk.OrientationHorizontalValue
	OrientationVertical   = gtk.OrientationVerticalValue
)

// Widget is the base interface that all GTK widgets implement.
// It provides common widget operations needed for layout management.
type Widget interface {
	// Visibility
	Show()
	Hide()
	SetVisible(visible bool)
	IsVisible() bool

	// Focus
	GrabFocus() bool
	HasFocus() bool
	SetCanFocus(canFocus bool)
	SetFocusOnClick(focusOnClick bool)

	// Pointer events
	SetCanTarget(canTarget bool) // If false, widget won't receive pointer events

	// Layout
	SetHexpand(expand bool)
	SetVexpand(expand bool)
	GetHexpand() bool
	GetVexpand() bool
	SetHalign(align gtk.Align)
	SetValign(align gtk.Align)
	SetSizeRequest(width, height int)

	// Geometry - for focus navigation
	GetAllocatedWidth() int
	GetAllocatedHeight() int
	// ComputePoint returns the position of this widget's origin (0,0) relative to
	// the target widget's coordinate space. If target is nil, returns position
	// relative to the native window. Returns ok=false if computation fails.
	ComputePoint(target Widget) (x, y float64, ok bool)

	// CSS styling
	AddCssClass(cssClass string)
	RemoveCssClass(cssClass string)
	HasCssClass(cssClass string) bool

	// Parent management
	Unparent()
	GetParent() Widget

	// GTK interop - returns the underlying GTK widget for embedding
	GtkWidget() *gtk.Widget

	// AddController adds an event controller to the widget
	AddController(controller *gtk.EventController)
}

// PanedWidget wraps gtk.Paned for creating split views.
// It manages two child widgets separated by a draggable divider.
type PanedWidget interface {
	Widget

	// Child management
	SetStartChild(child Widget)
	SetEndChild(child Widget)
	GetStartChild() Widget
	GetEndChild() Widget

	// Divider position
	SetPosition(position int)
	GetPosition() int

	// Resize behavior
	SetResizeStartChild(resize bool)
	SetResizeEndChild(resize bool)
	GetResizeStartChild() bool
	GetResizeEndChild() bool

	// Shrink behavior (whether child can be smaller than its minimum size)
	SetShrinkStartChild(shrink bool)
	SetShrinkEndChild(shrink bool)
	GetShrinkStartChild() bool
	GetShrinkEndChild() bool

	// Signals
	ConnectMap(callback func()) uint32
	ConnectNotifyPosition(callback func()) uint32

	// Tick callback for frame-based updates (returns signal ID)
	// Callback returns true to continue, false to stop
	AddTickCallback(callback func() bool) uint

	// Handle appearance
	SetWideHandle(wide bool)
	GetWideHandle() bool
}

// BoxWidget wraps gtk.Box for linear layouts.
// It arranges children in a single row or column.
type BoxWidget interface {
	Widget

	// Child management
	Append(child Widget)
	Prepend(child Widget)
	Remove(child Widget)
	InsertChildAfter(child Widget, sibling Widget)
	ReorderChildAfter(child Widget, sibling Widget)

	// Configuration
	SetHomogeneous(homogeneous bool)
	GetHomogeneous() bool
	SetSpacing(spacing int)
	GetSpacing() int
	SetOrientation(orientation Orientation)
	GetOrientation() Orientation
}

// OverlayWidget wraps gtk.Overlay for layered content.
// It displays overlay widgets on top of a main child widget.
type OverlayWidget interface {
	Widget

	// Main child
	SetChild(child Widget)
	GetChild() Widget

	// Overlay management
	AddOverlay(overlay Widget)
	RemoveOverlay(overlay Widget)

	// Overlay configuration
	SetClipOverlay(overlay Widget, clip bool)
	GetClipOverlay(overlay Widget) bool
	SetMeasureOverlay(overlay Widget, measure bool)
	GetMeasureOverlay(overlay Widget) bool
}

// EllipsizeMode represents pango ellipsize modes.
type EllipsizeMode = int

// Ellipsize mode constants.
const (
	EllipsizeNone   EllipsizeMode = 0
	EllipsizeStart  EllipsizeMode = 1
	EllipsizeMiddle EllipsizeMode = 2
	EllipsizeEnd    EllipsizeMode = 3
)

// LabelWidget wraps gtk.Label for text display.
type LabelWidget interface {
	Widget

	SetText(text string)
	GetText() string
	SetMarkup(markup string)
	SetEllipsize(mode EllipsizeMode)
	SetMaxWidthChars(nChars int)
	SetXalign(xalign float32)
}

// ButtonWidget wraps gtk.Button for clickable elements.
type ButtonWidget interface {
	Widget

	SetLabel(label string)
	GetLabel() string
	SetChild(child Widget)
	GetChild() Widget

	// Connect click handler, returns signal ID for disconnection
	ConnectClicked(callback func()) uint32
}

// Paintable represents a graphics texture that can be displayed in an image.
// This interface abstracts over gdk.Paintable/gdk.Texture.
type Paintable interface {
	GoPointer() uintptr
}

// ImageWidget wraps gtk.Image for displaying images.
type ImageWidget interface {
	Widget

	SetFromIconName(iconName string)
	SetFromFile(filename string)
	SetFromPaintable(paintable Paintable)
	SetPixelSize(pixelSize int)
	Clear()
}

// ProgressBarWidget wraps gtk.ProgressBar for displaying loading progress.
// Implementations may include smooth animation behavior.
type ProgressBarWidget interface {
	Widget

	// SetFraction sets the progress value (0.0 to 1.0).
	SetFraction(fraction float64)
	// GetFraction returns the current progress value.
	GetFraction() float64
}

// SpinnerWidget wraps gtk.Spinner for displaying indefinite loading.
type SpinnerWidget interface {
	Widget

	Start()
	Stop()
	SetSpinning(spinning bool)
	GetSpinning() bool
}

// WidgetFactory creates widget instances.
// This abstraction allows tests to inject mock factories.
type WidgetFactory interface {
	// Container widgets
	NewPaned(orientation Orientation) PanedWidget
	NewBox(orientation Orientation, spacing int) BoxWidget
	NewOverlay() OverlayWidget

	// Display widgets
	NewLabel(text string) LabelWidget
	NewButton() ButtonWidget
	NewImage() ImageWidget
	NewProgressBar() ProgressBarWidget
	NewSpinner() SpinnerWidget

	// Wrap existing GTK widget
	WrapWidget(w *gtk.Widget) Widget
}

// PaneRenderer creates pane view widgets from domain entities.
// This decouples the tree renderer from concrete PaneView implementation.
type PaneRenderer interface {
	// RenderPane creates a widget for a pane node.
	// The webViewWidget is the GTK widget from the WebView.
	RenderPane(paneID string, webViewWidget Widget) Widget
}
