//go:build !webkit_cgo

package webkit

import "sync"

// Orientation mirrors GtkOrientation in stub builds.
type Orientation int

const (
	OrientationHorizontal Orientation = 0
	OrientationVertical   Orientation = 1
)

// WidgetBounds mirrors the CGO struct for stub builds.
type WidgetBounds struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type widgetStub struct {
	startChild uintptr
	endChild   uintptr
	bounds     WidgetBounds
	hasBounds  bool
	hover      map[uintptr]func()
	// GTK4 lifecycle simulation
	parent      uintptr
	isDestroyed bool
	refCount    uint
	// GTK focus management simulation
	hasFocus     bool
	canFocus     bool
	focusedChild uintptr
}

var (
	widgetMu       sync.Mutex
	widgetState            = map[uintptr]*widgetStub{}
	nextWidgetID   uintptr = 1
	nextHoverToken uintptr = 1
	// GTK focus management simulation
	globalFocusedWidget uintptr
	focusWarningEnabled bool = true
	gtkTestInitOnce     sync.Once
)

func newWidgetHandle() uintptr {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	id := nextWidgetID
	nextWidgetID++
	widgetState[id] = &widgetStub{
		refCount: 1,    // GTK widgets start with ref count 1
		canFocus: true, // Most widgets can receive focus
	}
	return id
}

// NewTestWidget returns a unique widget handle for tests.
func NewTestWidget() uintptr { return newWidgetHandle() }

// SetWidgetBoundsForTesting assigns bounds for the widget in stub builds.
func SetWidgetBoundsForTesting(widget uintptr, bounds WidgetBounds) {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	stub, ok := widgetState[widget]
	if !ok {
		stub = &widgetStub{}
		widgetState[widget] = stub
	}
	stub.bounds = bounds
	stub.hasBounds = true
}

// ResetWidgetStubsForTesting clears widget stub state for deterministic tests.
func ResetWidgetStubsForTesting() {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	widgetState = map[uintptr]*widgetStub{}
	nextWidgetID = 1
	nextHoverToken = 1
}

func NewPaned(orientation Orientation) uintptr {
	return newWidgetHandle()
}

func PanedSetStartChild(paned uintptr, child uintptr) {
	widgetMu.Lock()
	defer widgetMu.Unlock()

	// GTK4 validation: paned must be a valid widget
	if paned == 0 {
		panic("GTK-CRITICAL simulation: gtk_widget_get_parent: assertion 'GTK_IS_WIDGET (widget)' failed - paned is NULL")
	}

	stub, ok := widgetState[paned]
	if !ok || stub.isDestroyed {
		panic("GTK-CRITICAL simulation: gtk_widget_get_parent: assertion 'GTK_IS_WIDGET (widget)' failed - paned is invalid")
	}

	// GTK4 validation: if child is not 0, it must be a valid widget
	if child != 0 {
		childStub, childExists := widgetState[child]
		if !childExists || childStub.isDestroyed {
			panic("GTK-CRITICAL simulation: gtk_widget_insert_before: assertion 'GTK_IS_WIDGET (widget)' failed - child is invalid")
		}
	}

	// GTK4 behavior: setting child to 0 automatically unparents the old child
	if stub.startChild != 0 && child == 0 {
		if oldChild, exists := widgetState[stub.startChild]; exists {
			oldChild.parent = 0 // Automatically unparent
		}
	}
	// Set new child and establish parent relationship
	stub.startChild = child
	if child != 0 {
		if childStub, exists := widgetState[child]; exists {
			// Simulate the exact GTK focus management issue from production
			// When reparenting a widget with focus hierarchy to a new parent,
			// GTK's focus management calls gtk_paned_set_focus_child with (nil)
			if focusWarningEnabled && (childStub.hasFocus || childStub.focusedChild != 0) {
				panic("GTK-WARNING simulation: Error finding last focus widget of GtkPaned, gtk_paned_set_focus_child was called on widget (nil) which is not child")
			}
			childStub.parent = paned
		}
	}
}

func PanedSetEndChild(paned uintptr, child uintptr) {
	widgetMu.Lock()
	defer widgetMu.Unlock()

	// GTK4 validation: paned must be a valid widget
	if paned == 0 {
		panic("GTK-CRITICAL simulation: gtk_widget_get_parent: assertion 'GTK_IS_WIDGET (widget)' failed - paned is NULL")
	}

	stub, ok := widgetState[paned]
	if !ok || stub.isDestroyed {
		panic("GTK-CRITICAL simulation: gtk_widget_get_parent: assertion 'GTK_IS_WIDGET (widget)' failed - paned is invalid")
	}

	// GTK4 validation: if child is not 0, it must be a valid widget
	if child != 0 {
		childStub, childExists := widgetState[child]
		if !childExists || childStub.isDestroyed {
			panic("GTK-CRITICAL simulation: gtk_widget_insert_before: assertion 'GTK_IS_WIDGET (widget)' failed - child is invalid")
		}
	}

	// GTK4 behavior: setting child to 0 automatically unparents the old child
	if stub.endChild != 0 && child == 0 {
		if oldChild, exists := widgetState[stub.endChild]; exists {
			oldChild.parent = 0 // Automatically unparent
		}
	}
	// Set new child and establish parent relationship
	stub.endChild = child
	if child != 0 {
		if childStub, exists := widgetState[child]; exists {
			// Simulate the exact GTK focus management issue from production
			// When reparenting a widget with focus hierarchy to a new parent,
			// GTK's focus management calls gtk_paned_set_focus_child with (nil)
			if focusWarningEnabled && (childStub.hasFocus || childStub.focusedChild != 0) {
				panic("GTK-WARNING simulation: Error finding last focus widget of GtkPaned, gtk_paned_set_focus_child was called on widget (nil) which is not child")
			}
			childStub.parent = paned
		}
	}
}

func PanedSetResizeStart(paned uintptr, resize bool) {}

func PanedSetResizeEnd(paned uintptr, resize bool) {}

// PanedSetPosition is a no-op in stub builds.
func PanedSetPosition(paned uintptr, pos int) {}

func WidgetUnparent(widget uintptr) {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	if stub, ok := widgetState[widget]; ok {
		// GTK4 CRITICAL simulation: unparenting widget with no parent is illegal
		if stub.parent == 0 {
			// In real GTK4, this would cause: gtk_widget_get_parent: assertion 'GTK_IS_WIDGET (widget)' failed
			// We'll simulate this by marking the widget as destroyed
			stub.isDestroyed = true
			panic("GTK-CRITICAL simulation: gtk_widget_unparent called on widget with no parent")
		}
		// Remove from parent's child list
		if parentStub, exists := widgetState[stub.parent]; exists {
			if parentStub.startChild == widget {
				parentStub.startChild = 0
			}
			if parentStub.endChild == widget {
				parentStub.endChild = 0
			}
		}
		stub.parent = 0
	}
}

func WidgetSetHExpand(widget uintptr, expand bool) {}

func WidgetSetVExpand(widget uintptr, expand bool) {}

func WidgetGetParent(widget uintptr) uintptr {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	if stub, ok := widgetState[widget]; ok && !stub.isDestroyed {
		return stub.parent
	}
	return 0
}

func WidgetShow(widget uintptr) {}

func WidgetGrabFocus(widget uintptr) {
	widgetMu.Lock()
	defer widgetMu.Unlock()

	if widget == 0 {
		return
	}

	stub, ok := widgetState[widget]
	if !ok || stub.isDestroyed || !stub.canFocus {
		return
	}

	// Clear previous focus
	if globalFocusedWidget != 0 {
		if prevStub, exists := widgetState[globalFocusedWidget]; exists {
			prevStub.hasFocus = false
		}
	}

	// Set new focus
	stub.hasFocus = true
	globalFocusedWidget = widget

	// Simulate focus in parent hierarchy - each parent should point to its direct focused child
	currentChild := widget
	parent := stub.parent
	for parent != 0 {
		if parentStub, exists := widgetState[parent]; exists {
			parentStub.focusedChild = currentChild // Direct child that contains focus
			currentChild = parent                  // Move up the hierarchy
			parent = parentStub.parent
		} else {
			break
		}
	}
}

func WidgetRef(widget uintptr) bool {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	_, ok := widgetState[widget]
	return ok
}

func WidgetUnref(widget uintptr) {}

func WidgetQueueAllocate(widget uintptr) {}

func WidgetRealizeInContainer(widget uintptr) {}

func WidgetHookDestroy(widget uintptr) {}

func WidgetIsValid(widget uintptr) bool {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	if widget == 0 {
		return false
	}
	if stub, ok := widgetState[widget]; ok {
		return !stub.isDestroyed
	}
	return false
}

func WidgetRefCount(widget uintptr) uint {
	if WidgetIsValid(widget) {
		return 1
	}
	return 0
}

func WidgetAddHoverHandler(widget uintptr, fn func()) uintptr {
	if widget == 0 || fn == nil {
		return 0
	}
	widgetMu.Lock()
	defer widgetMu.Unlock()
	stub, ok := widgetState[widget]
	if !ok {
		stub = &widgetStub{}
		widgetState[widget] = stub
	}
	if stub.hover == nil {
		stub.hover = make(map[uintptr]func())
	}
	token := nextHoverToken
	nextHoverToken++
	stub.hover[token] = fn
	return token
}

func WidgetRemoveHoverHandler(widget uintptr, token uintptr) {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	if stub, ok := widgetState[widget]; ok && stub.hover != nil {
		delete(stub.hover, token)
	}
}

func WidgetGetBounds(widget uintptr) (WidgetBounds, bool) {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	stub, ok := widgetState[widget]
	if !ok || !stub.hasBounds {
		return WidgetBounds{}, false
	}
	return stub.bounds, true
}

// WidgetWaitForDraw ensures widget operations are complete (GTK test pattern)
// This replaces the problematic IdleAdd pattern with proper widget synchronization
func WidgetWaitForDraw(widget uintptr) {
	gtkTestInitOnce.Do(func() {})
	widgetMu.Lock()
	defer widgetMu.Unlock()

	// Validate widget is still valid after any recent operations
	if widget != 0 {
		stub, ok := widgetState[widget]
		if !ok || stub.isDestroyed {
			panic("GTK-CRITICAL simulation: gtk_test_widget_wait_for_draw: widget is invalid")
		}
	}
	// In real GTK, this would wait for all pending draw operations to complete
	// In our stub, we just validate the widget state
}

// IdleAdd simulates GTK's idle callback system - DEPRECATED, use WidgetWaitForDraw instead
func IdleAdd(fn func() bool) {
	if fn != nil {
		fn()
	}
}

func AddCSSProvider(css string) {}

func WidgetAddCSSClass(widget uintptr, class string) {}

func WidgetRemoveCSSClass(widget uintptr, class string) {}

// SimulateFocusForTesting gives focus to a widget for test scenarios
func SimulateFocusForTesting(widget uintptr) {
	WidgetGrabFocus(widget)
}

// GtkBox functions for stacked panes (stub implementations)

func NewBox(orientation Orientation, spacing int) uintptr {
	return newWidgetHandle()
}

func BoxAppend(box uintptr, child uintptr) {
	widgetMu.Lock()
	defer widgetMu.Unlock()

	if box == 0 || child == 0 {
		return
	}

	boxStub, ok := widgetState[box]
	if !ok || boxStub.isDestroyed {
		panic("GTK-CRITICAL simulation: gtk_box_append: box is invalid")
	}

	childStub, ok := widgetState[child]
	if !ok || childStub.isDestroyed {
		panic("GTK-CRITICAL simulation: gtk_box_append: child is invalid")
	}

	if childStub.parent != 0 && childStub.parent != box {
		panic("GTK-CRITICAL simulation: gtk_box_append: child already has a different parent")
	}
	childStub.parent = box
}

func BoxPrepend(box uintptr, child uintptr) {
	BoxAppend(box, child)
}

func BoxRemove(box uintptr, child uintptr) {
	widgetMu.Lock()
	defer widgetMu.Unlock()

	if box == 0 || child == 0 {
		return
	}

	childStub, ok := widgetState[child]
	if !ok || childStub.isDestroyed {
		panic("GTK-CRITICAL simulation: gtk_box_remove: child is invalid")
	}

	if childStub.parent != box {
		panic("GTK-CRITICAL simulation: gtk_box_remove: child is not parented to the specified box")
	}

	childStub.parent = 0
}

func BoxInsertChildAfter(box uintptr, child uintptr, sibling uintptr) {
	BoxAppend(box, child)
}

// Widget visibility functions for stacked panes (stub implementations)

func WidgetSetVisible(widget uintptr, visible bool) {}

func WidgetGetVisible(widget uintptr) bool { return true }

func WidgetHide(widget uintptr) {}

// Label functions for title bars (stub implementations)

func NewLabel(text string) uintptr {
	return newWidgetHandle()
}

func LabelSetText(label uintptr, text string) {}

func LabelGetText(label uintptr) string { return "" }

// EllipsizeMode represents PangoEllipsizeMode values in stub builds.
type EllipsizeMode int

const (
	EllipsizeNone   EllipsizeMode = 0
	EllipsizeStart  EllipsizeMode = 1
	EllipsizeMiddle EllipsizeMode = 2
	EllipsizeEnd    EllipsizeMode = 3
)

func LabelSetEllipsize(label uintptr, mode EllipsizeMode) {}

func LabelSetMaxWidthChars(label uintptr, nChars int) {}

// SetTestContainer is a testing helper to set the container field for WebView in stub builds
func (w *WebView) SetTestContainer(container uintptr) {
	w.container = container
}
