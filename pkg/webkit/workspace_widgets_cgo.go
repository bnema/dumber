//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>
#include <gdk/gdk.h>
#include <glib.h>
#include <glib-object.h>
#include <graphene.h>
#include <stdint.h>
#include <stdlib.h>

// Logging helper for diagnostics
#define WORKSPACE_LOG(fmt, ...) g_printerr("[workspace-cgo] " fmt "\n", ##__VA_ARGS__)

// Callback function for idle handling following the existing pattern
extern gboolean goIdleCallback(uintptr_t handle);
extern void goHoverCallback(uintptr_t handle);

static gboolean idle_callback_wrapper(gpointer data) {
    return goIdleCallback((uintptr_t)data);
}

static void add_idle_callback(uintptr_t handle) {
    g_idle_add_full(G_PRIORITY_DEFAULT_IDLE, idle_callback_wrapper, (gpointer)handle, NULL);
}

static void motion_enter_cb(GtkEventControllerMotion* controller, gdouble x, gdouble y, gpointer user_data) {
    (void)controller; (void)x; (void)y;
    uintptr_t handle = (uintptr_t)user_data;
    goHoverCallback(handle);
}

static GtkEventController* widget_add_hover_controller(GtkWidget* widget, uintptr_t handle) {
    if (!widget) {
        return NULL;
    }
    GtkEventController* motion = gtk_event_controller_motion_new();
    if (!motion) {
        return NULL;
    }
    gtk_event_controller_set_propagation_phase(motion, GTK_PHASE_CAPTURE);
    g_signal_connect_data(motion, "enter", G_CALLBACK(motion_enter_cb), (gpointer)handle, NULL, 0);
    gtk_widget_add_controller(widget, motion);
    return motion;
}

static void widget_remove_controller(GtkWidget* widget, GtkEventController* controller) {
    if (!widget || !controller) {
        return;
    }
    gtk_widget_remove_controller(widget, controller);
}

static gboolean widget_get_bounds(GtkWidget* widget, double* x, double* y, double* width, double* height) {
    if (!widget || !x || !y || !width || !height) {
        return FALSE;
    }
    GtkWidget* root = GTK_WIDGET(gtk_widget_get_root(widget));
    if (!root) {
        return FALSE;
    }
    graphene_rect_t rect;
    if (!gtk_widget_compute_bounds(widget, root, &rect)) {
        return FALSE;
    }
    *x = rect.origin.x;
    *y = rect.origin.y;
    *width = rect.size.width;
    *height = rect.size.height;
    return TRUE;
}

// Widget validation helper to prevent crashes from invalid widget pointers
static gboolean is_valid_widget(gpointer widget) {
    return widget != NULL && GTK_IS_WIDGET(widget);
}

static GtkWidget* paned_get_start_child(GtkWidget* paned) {
    if (!paned) return NULL;
    return gtk_paned_get_start_child(GTK_PANED(paned));
}

static GtkWidget* paned_get_end_child(GtkWidget* paned) {
    if (!paned) return NULL;
    return gtk_paned_get_end_child(GTK_PANED(paned));
}

static void on_widget_destroyed(GtkWidget* widget, gpointer user_data) {
    (void)user_data;
    guint ref_count = widget ? G_OBJECT(widget)->ref_count : 0;
    WORKSPACE_LOG("widget destroyed: %p ref_count=%u", widget, ref_count);
}

static void hook_destroy_signal(GtkWidget* widget) {
    if (!widget) return;
    g_signal_connect(widget, "destroy", G_CALLBACK(on_widget_destroyed), NULL);
}

static unsigned int widget_get_ref_count(GtkWidget* widget) {
    if (!widget) {
        return 0;
    }
    return G_OBJECT(widget)->ref_count;
}

// Widget realization helpers for proper WebView reparenting
// Force widget realization and ensure it's ready for rendering
static void ensure_widget_realized(GtkWidget* widget) {
    if (!widget) return;

    // Make widget visible first
    gtk_widget_set_visible(widget, TRUE);

    // For WebKitWebView, we need to ensure the widget is properly sized
    gtk_widget_set_size_request(widget, 100, 100); // Minimum size to force allocation

    // Queue a redraw to force the rendering context to reinitialize
    gtk_widget_queue_draw(widget);
    WORKSPACE_LOG("ensure_widget_realized: widget=%p", widget);
}

static void add_css_provider(const char* css) {
    if (!css) {
        return;
    }
    GtkCssProvider* provider = gtk_css_provider_new();
    if (!provider) {
        return;
    }
    gtk_css_provider_load_from_string(provider, css);
    GdkDisplay* display = gdk_display_get_default();
    if (display) {
        gtk_style_context_add_provider_for_display(display, GTK_STYLE_PROVIDER(provider), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
    }
    g_object_unref(provider);
}

static void widget_add_css_class(GtkWidget* widget, const char* name) {
    if (!widget || !name) return;
    gtk_widget_add_css_class(widget, name);
}

static void widget_remove_css_class(GtkWidget* widget, const char* name) {
    if (!widget || !name) return;
    gtk_widget_remove_css_class(widget, name);
}
*/
import "C"

import (
	"log"
	"sync"
	"unsafe"
)

// WidgetBounds captures absolute widget bounds relative to the toplevel window.
type WidgetBounds struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

var (
	hoverMu          sync.Mutex
	hoverCallbacks           = make(map[uintptr]func())
	hoverControllers         = make(map[uintptr]uintptr)
	nextHoverID      uintptr = 1
)

func widgetIsValid(widget uintptr) bool {
	if widget == 0 {
		return false
	}
	return C.is_valid_widget(C.gpointer(unsafe.Pointer(widget))) != 0
}

func widgetRefCount(widget uintptr) uint {
	if widget == 0 || !widgetIsValid(widget) {
		return 0
	}
	return uint(C.widget_get_ref_count((*C.GtkWidget)(unsafe.Pointer(widget))))
}

// WidgetIsValid exposes GTK_IS_WIDGET checks for diagnostics in Go.
func WidgetIsValid(widget uintptr) bool {
	return widgetIsValid(widget)
}

// WidgetRefCount exposes the widget reference count for debugging.
func WidgetRefCount(widget uintptr) uint {
	return widgetRefCount(widget)
}

// WidgetHookDestroy attaches a destroy signal handler for diagnostics (debug builds).
func WidgetHookDestroy(widget uintptr) {
	if widget == 0 || !widgetIsValid(widget) {
		return
	}
	C.hook_destroy_signal((*C.GtkWidget)(unsafe.Pointer(widget)))
}

// AddCSSProvider installs application-level CSS rules.
func AddCSSProvider(css string) {
	if css == "" {
		return
	}
	cstr := C.CString(css)
	defer C.free(unsafe.Pointer(cstr))
	C.add_css_provider(cstr)
}

// WidgetAddCSSClass attaches a CSS class to a widget.
func WidgetAddCSSClass(widget uintptr, class string) {
	if widget == 0 || class == "" || !widgetIsValid(widget) {
		return
	}
	cstr := C.CString(class)
	defer C.free(unsafe.Pointer(cstr))
	C.widget_add_css_class((*C.GtkWidget)(unsafe.Pointer(widget)), cstr)
}

// WidgetRemoveCSSClass removes a CSS class from a widget.
func WidgetRemoveCSSClass(widget uintptr, class string) {
	if widget == 0 || class == "" || !widgetIsValid(widget) {
		return
	}
	cstr := C.CString(class)
	defer C.free(unsafe.Pointer(cstr))
	C.widget_remove_css_class((*C.GtkWidget)(unsafe.Pointer(widget)), cstr)
}

// Orientation mirrors GtkOrientation for workspace layout helpers.
type Orientation int

const (
	OrientationHorizontal Orientation = Orientation(C.GTK_ORIENTATION_HORIZONTAL)
	OrientationVertical   Orientation = Orientation(C.GTK_ORIENTATION_VERTICAL)
)

func goBool(b bool) C.gboolean {
	if b {
		return C.gboolean(1)
	}
	return C.gboolean(0)
}

// NewPaned constructs a GtkPaned widget for the given orientation.
func NewPaned(orientation Orientation) uintptr {
	paned := C.gtk_paned_new(C.GtkOrientation(orientation))
	return uintptr(unsafe.Pointer(paned))
}

// PanedSetStartChild assigns the start child of a GtkPaned.
func PanedSetStartChild(paned uintptr, child uintptr) {
	if child != 0 {
		// Check if widget already has a parent and force unparent if needed
		parent := C.gtk_widget_get_parent((*C.GtkWidget)(unsafe.Pointer(child)))
		if parent != nil {
			C.gtk_widget_unparent((*C.GtkWidget)(unsafe.Pointer(child)))
		}
	}
	C.gtk_paned_set_start_child((*C.GtkPaned)(unsafe.Pointer(paned)), (*C.GtkWidget)(unsafe.Pointer(child)))
}

// PanedSetEndChild assigns the end child of a GtkPaned.
func PanedSetEndChild(paned uintptr, child uintptr) {
	if child != 0 {
		// Check if widget already has a parent and force unparent if needed
		parent := C.gtk_widget_get_parent((*C.GtkWidget)(unsafe.Pointer(child)))
		if parent != nil {
			C.gtk_widget_unparent((*C.GtkWidget)(unsafe.Pointer(child)))
		}
	}
	C.gtk_paned_set_end_child((*C.GtkPaned)(unsafe.Pointer(paned)), (*C.GtkWidget)(unsafe.Pointer(child)))
}

// PanedGetStartChild returns the start child pointer for diagnostics.
func PanedGetStartChild(paned uintptr) uintptr {
	if paned == 0 {
		return 0
	}
	child := C.paned_get_start_child((*C.GtkWidget)(unsafe.Pointer(paned)))
	return uintptr(unsafe.Pointer(child))
}

// PanedGetEndChild returns the end child pointer for diagnostics.
func PanedGetEndChild(paned uintptr) uintptr {
	if paned == 0 {
		return 0
	}
	child := C.paned_get_end_child((*C.GtkWidget)(unsafe.Pointer(paned)))
	return uintptr(unsafe.Pointer(child))
}

// PanedSetResizeStart configures whether the start child should expand when the paned is resized.
func PanedSetResizeStart(paned uintptr, resize bool) {
	C.gtk_paned_set_resize_start_child((*C.GtkPaned)(unsafe.Pointer(paned)), goBool(resize))
}

// PanedSetResizeEnd configures whether the end child should expand when the paned is resized.
func PanedSetResizeEnd(paned uintptr, resize bool) {
	C.gtk_paned_set_resize_end_child((*C.GtkPaned)(unsafe.Pointer(paned)), goBool(resize))
}

// WidgetUnparent detaches a widget from its current parent.
func WidgetUnparent(widget uintptr) {
	if widget == 0 {
		return
	}
	if !widgetIsValid(widget) {
		log.Printf("[workspace] WidgetUnparent skipped invalid widget=%#x", widget)
		return
	}
	before := widgetRefCount(widget)
	log.Printf("[workspace] WidgetUnparent widget=%#x ref_before=%d", widget, before)
	C.gtk_widget_unparent((*C.GtkWidget)(unsafe.Pointer(widget)))
	if widgetIsValid(widget) {
		after := widgetRefCount(widget)
		log.Printf("[workspace] WidgetUnparent widget=%#x ref_after=%d", widget, after)
	} else {
		log.Printf("[workspace] WidgetUnparent widget=%#x now invalid", widget)
	}
}

// WidgetSetHExpand configures horizontal expand for a widget.
func WidgetSetHExpand(widget uintptr, expand bool) {
	if widget == 0 {
		return
	}
	log.Printf("[workspace] WidgetSetHExpand widget=%#x expand=%v", widget, expand)
	C.gtk_widget_set_hexpand((*C.GtkWidget)(unsafe.Pointer(widget)), goBool(expand))
}

func WidgetGetParent(widget uintptr) uintptr {
	if widget == 0 {
		log.Printf("[workspace] WidgetGetParent: widget=0")
		return 0
	}
	if !widgetIsValid(widget) {
		log.Printf("[workspace] WidgetGetParent: widget=%#x invalid", widget)
		return 0
	}
	parent := C.gtk_widget_get_parent((*C.GtkWidget)(unsafe.Pointer(widget)))
	// Reduced logging: only debug mode shows individual widget operations
	return uintptr(unsafe.Pointer(parent))
}

// WidgetSetVExpand configures vertical expand for a widget.
func WidgetSetVExpand(widget uintptr, expand bool) {
	if widget == 0 {
		return
	}
	log.Printf("[workspace] WidgetSetVExpand widget=%#x expand=%v", widget, expand)
	C.gtk_widget_set_vexpand((*C.GtkWidget)(unsafe.Pointer(widget)), goBool(expand))
}

// WidgetShow makes the widget visible.
func WidgetShow(widget uintptr) {
	if widget == 0 {
		return
	}
	log.Printf("[workspace] WidgetShow widget=%#x", widget)
	C.gtk_widget_set_visible((*C.GtkWidget)(unsafe.Pointer(widget)), C.gboolean(1))
}

// WidgetGrabFocus focuses the widget if possible.
func WidgetGrabFocus(widget uintptr) {
	if widget == 0 {
		return
	}
	if !widgetIsValid(widget) {
		log.Printf("[workspace] WidgetGrabFocus skipped invalid widget=%#x", widget)
		return
	}
	// Reduced logging: focus grab logged only in consolidated focus operation
	C.gtk_widget_grab_focus((*C.GtkWidget)(unsafe.Pointer(widget)))
}

// WidgetRef increments the reference count to keep the widget alive during reparenting.
func WidgetRef(widget uintptr) bool {
	if widget == 0 {
		return false
	}
	if !widgetIsValid(widget) {
		log.Printf("[workspace] WidgetRef skipped invalid widget=%#x", widget)
		return false
	}
	before := widgetRefCount(widget)
	log.Printf("[workspace] WidgetRef widget=%#x ref_before=%d", widget, before)
	C.g_object_ref(C.gpointer(unsafe.Pointer(widget)))
	after := widgetRefCount(widget)
	log.Printf("[workspace] WidgetRef widget=%#x ref_after=%d", widget, after)
	return true
}

// WidgetUnref releases a previously held widget reference.
func WidgetUnref(widget uintptr) {
	if widget == 0 {
		return
	}
	if !widgetIsValid(widget) {
		log.Printf("[workspace] WidgetUnref skipped invalid widget=%#x", widget)
		return
	}
	before := widgetRefCount(widget)
	log.Printf("[workspace] WidgetUnref widget=%#x ref_before=%d", widget, before)
	C.g_object_unref(C.gpointer(unsafe.Pointer(widget)))
	if widgetIsValid(widget) {
		after := widgetRefCount(widget)
		log.Printf("[workspace] WidgetUnref widget=%#x ref_after=%d", widget, after)
	} else {
		log.Printf("[workspace] WidgetUnref widget=%#x now invalid", widget)
	}
}

// WidgetQueueAllocate queues an allocation on the widget.
func WidgetQueueAllocate(widget uintptr) {
	if widget == 0 {
		return
	}
	log.Printf("[workspace] WidgetQueueAllocate widget=%#x", widget)
	C.gtk_widget_queue_allocate((*C.GtkWidget)(unsafe.Pointer(widget)))
}

// WidgetRealizeInContainer ensures a widget is properly realized within a container
func WidgetRealizeInContainer(widget uintptr) {
	if widget == 0 {
		return
	}
	// Reduced logging: widget realization logged only in consolidated focus operation
	C.ensure_widget_realized((*C.GtkWidget)(unsafe.Pointer(widget)))
}

// WidgetAddHoverHandler registers a hover callback for the given widget and
// returns a token that can be used to detach it later.
func WidgetAddHoverHandler(widget uintptr, fn func()) uintptr {
	if widget == 0 || fn == nil || !widgetIsValid(widget) {
		return 0
	}
	hoverMu.Lock()
	token := nextHoverID
	nextHoverID++
	hoverCallbacks[token] = fn
	hoverMu.Unlock()

	controller := C.widget_add_hover_controller((*C.GtkWidget)(unsafe.Pointer(widget)), C.uintptr_t(token))
	if controller == nil {
		hoverMu.Lock()
		delete(hoverCallbacks, token)
		hoverMu.Unlock()
		return 0
	}

	hoverMu.Lock()
	hoverControllers[token] = uintptr(unsafe.Pointer(controller))
	hoverMu.Unlock()
	return token
}

// WidgetRemoveHoverHandler removes a previously registered hover callback.
func WidgetRemoveHoverHandler(widget uintptr, token uintptr) {
	if token == 0 {
		return
	}

	hoverMu.Lock()
	controller := hoverControllers[token]
	delete(hoverCallbacks, token)
	delete(hoverControllers, token)
	hoverMu.Unlock()

	if controller == 0 {
		return
	}

	if widget == 0 || !widgetIsValid(widget) {
		return
	}

	C.widget_remove_controller((*C.GtkWidget)(unsafe.Pointer(widget)), (*C.GtkEventController)(unsafe.Pointer(controller)))
}

// WidgetGetBounds returns the absolute bounds of the widget relative to the root window.
func WidgetGetBounds(widget uintptr) (WidgetBounds, bool) {
	if widget == 0 || !widgetIsValid(widget) {
		return WidgetBounds{}, false
	}

	var x, y, width, height C.double
	if C.widget_get_bounds((*C.GtkWidget)(unsafe.Pointer(widget)), (*C.double)(&x), (*C.double)(&y), (*C.double)(&width), (*C.double)(&height)) == 0 {
		return WidgetBounds{}, false
	}

	return WidgetBounds{
		X:      float64(x),
		Y:      float64(y),
		Width:  float64(width),
		Height: float64(height),
	}, true
}

// Global storage for idle callbacks
var (
	idleCallbacks         = make(map[uintptr]func() bool)
	nextIdleID    uintptr = 1
)

// IdleAdd schedules a function to be called when the main loop is idle.
func IdleAdd(fn func() bool) {
	id := nextIdleID
	nextIdleID++
	idleCallbacks[id] = fn
	C.add_idle_callback(C.uintptr_t(id))
}

//export goIdleCallback
func goIdleCallback(handle uintptr) C.gboolean {
	if fn, ok := idleCallbacks[handle]; ok {
		delete(idleCallbacks, handle) // Clean up after calling
		if fn() {
			return C.gboolean(1) // G_SOURCE_CONTINUE
		}
	}
	return C.gboolean(0) // G_SOURCE_REMOVE
}

//export goHoverCallback
func goHoverCallback(handle C.uintptr_t) {
	token := uintptr(handle)
	hoverMu.Lock()
	fn := hoverCallbacks[token]
	hoverMu.Unlock()
	if fn != nil {
		fn()
	}
}
