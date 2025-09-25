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
}

var (
	widgetMu       sync.Mutex
	widgetState            = map[uintptr]*widgetStub{}
	nextWidgetID   uintptr = 1
	nextHoverToken uintptr = 1
)

func newWidgetHandle() uintptr {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	id := nextWidgetID
	nextWidgetID++
	widgetState[id] = &widgetStub{}
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
	if stub, ok := widgetState[paned]; ok {
		stub.startChild = child
	}
}

func PanedSetEndChild(paned uintptr, child uintptr) {
	widgetMu.Lock()
	defer widgetMu.Unlock()
	if stub, ok := widgetState[paned]; ok {
		stub.endChild = child
	}
}

func PanedSetResizeStart(paned uintptr, resize bool) {}

func PanedSetResizeEnd(paned uintptr, resize bool) {}

// PanedSetPosition is a no-op in stub builds.
func PanedSetPosition(paned uintptr, pos int) {}

func WidgetUnparent(widget uintptr) {}

func WidgetSetHExpand(widget uintptr, expand bool) {}

func WidgetSetVExpand(widget uintptr, expand bool) {}

func WidgetGetParent(widget uintptr) uintptr { return 0 }

func WidgetShow(widget uintptr) {}

func WidgetGrabFocus(widget uintptr) {}

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
	_, ok := widgetState[widget]
	return ok && widget != 0
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

func IdleAdd(fn func() bool) {
	if fn != nil {
		fn()
	}
}

func AddCSSProvider(css string) {}

func WidgetAddCSSClass(widget uintptr, class string) {}

func WidgetRemoveCSSClass(widget uintptr, class string) {}

// GtkBox functions for stacked panes (stub implementations)

func NewBox(orientation Orientation, spacing int) uintptr {
	return newWidgetHandle()
}

func BoxAppend(box uintptr, child uintptr) {}

func BoxPrepend(box uintptr, child uintptr) {}

func BoxRemove(box uintptr, child uintptr) {}

func BoxInsertChildAfter(box uintptr, child uintptr, sibling uintptr) {}

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
