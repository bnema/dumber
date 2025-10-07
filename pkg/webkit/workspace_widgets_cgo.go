//go:build webkit_cgo

package webkit

/*
#cgo pkg-config: gtk4
#include <gtk/gtk.h>
#include <gtk/gtktestutils.h>
#include <gtk/gtkbox.h>
#include <gtk/gtkpaned.h>
#include <gdk/gdk.h>
#include <glib.h>
#include <glib-object.h>
#include <graphene.h>
#include <pthread.h>
#include <stdint.h>
#include <stdlib.h>

// Logging helper for diagnostics
#define WORKSPACE_LOG(fmt, ...) g_printerr("[workspace-cgo] " fmt "\n", ##__VA_ARGS__)

// Helper functions for widget type checking
static gboolean is_paned_widget(GtkWidget *widget) {
    return GTK_IS_PANED(widget);
}

static gboolean is_box_widget(GtkWidget *widget) {
    return GTK_IS_BOX(widget);
}

// Callback function for idle handling following the existing pattern
extern gboolean goIdleCallback(uintptr_t handle);
extern void goHoverCallback(uintptr_t handle);
extern void goFocusEnterCallback(uintptr_t handle);
extern void goFocusLeaveCallback(uintptr_t handle);
extern void goTitleBarClickCallback(uintptr_t handle);

// Main thread helpers implemented in thread_helpers.c
void store_main_thread_id();
int is_main_thread();
int iterate_main_loop();

static gboolean idle_callback_wrapper(gpointer data) {
    return goIdleCallback((uintptr_t)data);
}

static guint add_idle_callback(uintptr_t handle) {
    return g_idle_add_full(G_PRIORITY_DEFAULT_IDLE, idle_callback_wrapper, (gpointer)handle, NULL);
}

static gboolean remove_idle_callback(uintptr_t handle) {
    return g_idle_remove_by_data((gpointer)handle);
}

// Title bar click callback
static void title_bar_click_cb(GtkGestureClick* gesture, gint n_press, gdouble x, gdouble y, gpointer user_data) {
    (void)gesture; (void)n_press; (void)x; (void)y;
    uintptr_t handle = (uintptr_t)user_data;
    goTitleBarClickCallback(handle);
}

// Attach click handler to a widget (for title bars)
static void widget_attach_click_handler(GtkWidget* widget, uintptr_t handle) {
    if (!widget) {
        return;
    }
    GtkGesture* click = gtk_gesture_click_new();
    if (!click) {
        return;
    }
    g_signal_connect_data(click, "pressed", G_CALLBACK(title_bar_click_cb), (gpointer)handle, NULL, 0);
    gtk_widget_add_controller(widget, GTK_EVENT_CONTROLLER(click));
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

// Forward declarations for focus controller callbacks defined later in this file
static void focus_enter_cb(GtkEventControllerFocus* controller, gpointer user_data);
static void focus_leave_cb(GtkEventControllerFocus* controller, gpointer user_data);

typedef struct {
    GtkEventController* controller;
    GtkWidget* widget;
    gboolean is_attached;
} ControllerTracker;

static GHashTable* controller_registry = NULL;

static void init_controller_registry(void) {
    if (controller_registry == NULL) {
        controller_registry = g_hash_table_new_full(g_direct_hash, g_direct_equal, NULL, g_free);
    }
}

static void controller_on_widget_destroy(GtkWidget* widget, gpointer user_data) {
    (void)widget;
    ControllerTracker* tracker = (ControllerTracker*)user_data;
    if (!tracker) {
        return;
    }
    tracker->is_attached = FALSE;
}

static GtkEventController* widget_add_focus_controller(GtkWidget* widget, uintptr_t nodePtr) {
    if (!widget) {
        return NULL;
    }

    init_controller_registry();

    GtkEventController* focus = gtk_event_controller_focus_new();
    if (!focus) {
        return NULL;
    }

    gtk_event_controller_set_propagation_phase(focus, GTK_PHASE_CAPTURE);
    g_signal_connect(focus, "enter", G_CALLBACK(focus_enter_cb), (gpointer)nodePtr);
    g_signal_connect(focus, "leave", G_CALLBACK(focus_leave_cb), (gpointer)nodePtr);

    ControllerTracker* tracker = g_new0(ControllerTracker, 1);
    tracker->controller = focus;
    tracker->widget = widget;
    tracker->is_attached = TRUE;

    g_hash_table_insert(controller_registry, focus, tracker);

    g_signal_connect(widget, "destroy", G_CALLBACK(controller_on_widget_destroy), tracker);

    gtk_widget_add_controller(widget, focus);
    return focus;
}

static void widget_remove_focus_controller(GtkWidget* widget, GtkEventController* controller) {
    if (!controller_registry || !controller) {
        return;
    }

    ControllerTracker* tracker = g_hash_table_lookup(controller_registry, controller);
    if (!tracker) {
        return;
    }

    if (!tracker->is_attached) {
        return;
    }

    if (!GTK_IS_WIDGET(widget)) {
        tracker->is_attached = FALSE;
        return;
    }

    GtkWidget* owner = gtk_event_controller_get_widget(controller);
    if (GTK_IS_WIDGET(owner) && owner == widget) {
        gtk_widget_remove_controller(widget, controller);
    }

    tracker->is_attached = FALSE;

    // Remove from registry; GTK drops the final reference when removing controller
    g_hash_table_remove(controller_registry, controller);
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

// GtkBox helpers for stacked panes
static GtkWidget* box_new(GtkOrientation orientation, int spacing) {
    return gtk_box_new(orientation, spacing);
}

static void box_append(GtkWidget* box, GtkWidget* child) {
    if (!box || !child) return;
    gtk_box_append(GTK_BOX(box), child);
}

static void box_prepend(GtkWidget* box, GtkWidget* child) {
    if (!box || !child) return;
    gtk_box_prepend(GTK_BOX(box), child);
}

static void box_remove(GtkWidget* box, GtkWidget* child) {
    if (!box || !child) return;
    gtk_box_remove(GTK_BOX(box), child);
}

static void box_insert_child_after(GtkWidget* box, GtkWidget* child, GtkWidget* sibling) {
    if (!box || !child) return;
    gtk_box_insert_child_after(GTK_BOX(box), child, sibling);
}

// Widget visibility helpers for stacked panes
static void widget_set_visible(GtkWidget* widget, gboolean visible) {
    if (!widget) return;
    gtk_widget_set_visible(widget, visible);
}


static gboolean widget_get_visible(GtkWidget* widget) {
    if (!widget) return FALSE;
    return gtk_widget_get_visible(widget);
}

static void widget_hide(GtkWidget* widget) {
    if (!widget) return;
    gtk_widget_set_visible(widget, FALSE);
}

static void widget_show(GtkWidget* widget) {
    if (!widget) return;
    gtk_widget_set_visible(widget, TRUE);
}

// workspace_gtk_test_init removed - moved to test files only

// Label helpers for title bars
static GtkWidget* label_new(const char* text) {
    return gtk_label_new(text);
}

static void label_set_text(GtkWidget* label, const char* text) {
    if (!label || !text) return;
    gtk_label_set_text(GTK_LABEL(label), text);
}

static const char* label_get_text(GtkWidget* label) {
    if (!label) return NULL;
    return gtk_label_get_text(GTK_LABEL(label));
}

static void label_set_ellipsize(GtkWidget* label, int mode) {
    if (!label) return;
    gtk_label_set_ellipsize(GTK_LABEL(label), (PangoEllipsizeMode)mode);
}

static void label_set_max_width_chars(GtkWidget* label, int n_chars) {
    if (!label) return;
    gtk_label_set_max_width_chars(GTK_LABEL(label), n_chars);
}

// GTK4 Focus Controller helpers for focus state machine
static void focus_enter_cb(GtkEventControllerFocus* controller, gpointer user_data) {
    (void)controller;
    uintptr_t nodePtr = (uintptr_t)user_data;
    goFocusEnterCallback(nodePtr);
}

static void focus_leave_cb(GtkEventControllerFocus* controller, gpointer user_data) {
    (void)controller;
    uintptr_t nodePtr = (uintptr_t)user_data;
    goFocusLeaveCallback(nodePtr);
}

// Widget allocation helper using modern GTK4 API
static void widget_get_allocation_modern(GtkWidget* widget, int* x, int* y, int* width, int* height) {
    if (!widget || !x || !y || !width || !height) {
        *x = *y = *width = *height = 0;
        return;
    }

    graphene_rect_t bounds;
    // Compute bounds relative to the widget itself (pass widget as target)
    if (gtk_widget_compute_bounds(widget, widget, &bounds)) {
        // Extract integer coordinates from the graphene rect
        *x = (int)bounds.origin.x;
        *y = (int)bounds.origin.y;
        *width = (int)bounds.size.width;
        *height = (int)bounds.size.height;
    } else {
        // Fallback if compute_bounds fails
        *x = *y = 0;
        *width = gtk_widget_get_width(widget);
        *height = gtk_widget_get_height(widget);
    }
}

// GtkImage helpers for favicon display
static GtkWidget* image_new(void) {
    return gtk_image_new();
}

static GtkWidget* image_new_from_file(const char* filename) {
    if (!filename) return NULL;
    return gtk_image_new_from_file(filename);
}

static void image_set_from_file(GtkWidget* image, const char* filename) {
    if (!image || !filename) return;
    gtk_image_set_from_file(GTK_IMAGE(image), filename);
}

static void image_set_pixel_size(GtkWidget* image, int pixel_size) {
    if (!image) return;
    gtk_image_set_pixel_size(GTK_IMAGE(image), pixel_size);
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

// FocusCallbacks holds the enter and leave callbacks for focus events
type FocusCallbacks struct {
	OnEnter func()
	OnLeave func()
}

// WidgetAllocation represents a widget's allocation (position and size)
type WidgetAllocation struct {
	X      int
	Y      int
	Width  int
	Height int
}

var (
	hoverMu          sync.Mutex
	hoverCallbacks           = make(map[uintptr]func())
	hoverControllers         = make(map[uintptr]uintptr)
	nextHoverID      uintptr = 1
	// gtkTestInitOnce removed - no longer needed

	// Focus controller management
	focusMu          sync.Mutex
	focusCallbacks           = make(map[uintptr]FocusCallbacks)
	focusControllers         = make(map[uintptr]uintptr)
	nextFocusID      uintptr = 1

	// Title bar click management
	titleBarMu       sync.Mutex
	titleBarCallbacks = make(map[uintptr]func())
	nextTitleBarID   uintptr = 1

	mainThreadInitialized bool
)

// InitMainThread should be called once during gtk_init to store the main thread ID.
func InitMainThread() {
	if !mainThreadInitialized {
		C.store_main_thread_id()
		mainThreadInitialized = true
	}
}

// IsMainThread returns true if the current goroutine is running on the GTK main thread.
func IsMainThread() bool {
	if !mainThreadInitialized {
		return false
	}
	return C.is_main_thread() == 1
}

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

// IsPaned checks if a widget is a GtkPaned container.
func IsPaned(widget uintptr) bool {
	if widget == 0 {
		return false
	}
	return C.is_paned_widget((*C.GtkWidget)(unsafe.Pointer(widget))) != 0
}

// IsBox checks if a widget is a GtkBox container.
func IsBox(widget uintptr) bool {
	if widget == 0 {
		return false
	}
	return C.is_box_widget((*C.GtkWidget)(unsafe.Pointer(widget))) != 0
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

// PanedSetPosition sets the pixel position of the divider.
func PanedSetPosition(paned uintptr, pos int) {
	if paned == 0 {
		return
	}
	C.gtk_paned_set_position((*C.GtkPaned)(unsafe.Pointer(paned)), C.int(pos))
}

// PanedGetPosition gets the current position of the divider in a GtkPaned
func PanedGetPosition(paned uintptr) int {
	if paned == 0 {
		return 0
	}
	if !widgetIsValid(paned) {
		return 0
	}
	return int(C.gtk_paned_get_position((*C.GtkPaned)(unsafe.Pointer(paned))))
}

// WidgetUnparent detaches a widget from its current parent.
func WidgetUnparent(widget uintptr) {
	if ok := WidgetUnparentChecked(widget); !ok {
		log.Printf("[workspace] WidgetUnparent skipped for widget=%#x", widget)
	}
}

// WidgetUnparentChecked safely detaches a widget from its parent if still valid.
//
// Return semantics:
//   - Returns false if the widget pointer is zero or not a valid GTK widget (invalid input).
//   - Returns true if the widget has no parent (already detached).
//   - Returns true if the widget was successfully unparented (parent changed or is now nil).
//   - Returns false if the widget is still valid and the parent did not change (unparent failed).
//
// Callers can use the return value to distinguish between invalid input, already-detached widgets, and successful unparenting.
func WidgetUnparentChecked(widget uintptr) bool {
	if widget == 0 {
		log.Printf("[workspace] WidgetUnparentChecked: widget=0")
		return false
	}

	if !widgetIsValid(widget) {
		log.Printf("[workspace] WidgetUnparentChecked: widget=%#x invalid before unparent", widget)
		return false
	}

	parentBefore := WidgetGetParent(widget)
	if parentBefore == 0 {
		log.Printf("[workspace] WidgetUnparentChecked: widget=%#x has no parent", widget)
		return true
	}

	refBefore := widgetRefCount(widget)
	log.Printf("[workspace] WidgetUnparentChecked: widget=%#x parent=%#x ref_before=%d", widget, parentBefore, refBefore)

	C.gtk_widget_unparent((*C.GtkWidget)(unsafe.Pointer(widget)))

	if !widgetIsValid(widget) {
		log.Printf("[workspace] WidgetUnparentChecked: widget=%#x invalid after unparent", widget)
		return true
	}

	parentAfter := WidgetGetParent(widget)
	refAfter := widgetRefCount(widget)
	log.Printf("[workspace] WidgetUnparentChecked: widget=%#x parent_after=%#x ref_after=%d", widget, parentAfter, refAfter)

	return parentAfter == 0 || parentAfter != parentBefore
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

// WidgetSetSizeRequest sets the minimum size request for a widget.
// Use -1 for width or height to unset that dimension.
func WidgetSetSizeRequest(widget uintptr, width, height int) {
	if widget == 0 || !widgetIsValid(widget) {
		return
	}
	log.Printf("[workspace] WidgetSetSizeRequest widget=%#x width=%d height=%d", widget, width, height)
	C.gtk_widget_set_size_request((*C.GtkWidget)(unsafe.Pointer(widget)), C.int(width), C.int(height))
}

// WidgetResetSizeRequest clears explicit size constraints so GTK can recalculate allocation.
func WidgetResetSizeRequest(widget uintptr) {
	if widget == 0 || !widgetIsValid(widget) {
		return
	}
	C.gtk_widget_set_size_request((*C.GtkWidget)(unsafe.Pointer(widget)), C.int(-1), C.int(-1))
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

// WidgetSetFocusChild sets or clears the focus child of a widget.
// Pass 0 as child to clear the focus child (prevents GTK focus chain warnings during reparenting).
func WidgetSetFocusChild(widget uintptr, child uintptr) {
	if widget == 0 {
		return
	}
	if !widgetIsValid(widget) {
		log.Printf("[workspace] WidgetSetFocusChild skipped invalid widget=%#x", widget)
		return
	}
	var childPtr *C.GtkWidget
	if child != 0 {
		if !widgetIsValid(child) {
			log.Printf("[workspace] WidgetSetFocusChild skipped invalid child=%#x", child)
			return
		}
		childPtr = (*C.GtkWidget)(unsafe.Pointer(child))
	}
	C.gtk_widget_set_focus_child((*C.GtkWidget)(unsafe.Pointer(widget)), childPtr)
	log.Printf("[workspace] WidgetSetFocusChild widget=%#x child=%#x", widget, child)
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

// WidgetQueueResize forces GTK to recalculate the widget's layout and size allocation.
// This is useful when a widget's size requirements have changed (e.g., after adding children).
func WidgetQueueResize(widget uintptr) {
	if widget == 0 {
		return
	}
	if !widgetIsValid(widget) {
		log.Printf("[workspace] WidgetQueueResize skipped invalid widget=%#x", widget)
		return
	}
	log.Printf("[workspace] WidgetQueueResize widget=%#x", widget)
	C.gtk_widget_queue_resize((*C.GtkWidget)(unsafe.Pointer(widget)))
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

// WidgetAttachClickHandler attaches a click handler to any widget (for title bars)
func WidgetAttachClickHandler(widget uintptr, fn func()) uintptr {
	if widget == 0 || fn == nil || !widgetIsValid(widget) {
		return 0
	}
	titleBarMu.Lock()
	token := nextTitleBarID
	nextTitleBarID++
	titleBarCallbacks[token] = fn
	titleBarMu.Unlock()

	C.widget_attach_click_handler((*C.GtkWidget)(unsafe.Pointer(widget)), C.uintptr_t(token))

	return token
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
func IdleAdd(fn func() bool) uintptr {
	if fn == nil {
		return 0
	}
	id := nextIdleID
	nextIdleID++
	idleCallbacks[id] = fn
	if C.add_idle_callback(C.uintptr_t(id)) == 0 {
		delete(idleCallbacks, id)
		return 0
	}
	return id
}

// IdleRemove cancels a previously scheduled idle callback.
func IdleRemove(handle uintptr) {
	if handle == 0 {
		return
	}
	C.remove_idle_callback(C.uintptr_t(handle))
	delete(idleCallbacks, handle)
}

// IterateMainLoop processes a single GTK main loop iteration and reports if work was handled.
func IterateMainLoop() bool {
	return C.iterate_main_loop() == 1
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

//export goTitleBarClickCallback
func goTitleBarClickCallback(handle C.uintptr_t) {
	token := uintptr(handle)
	titleBarMu.Lock()
	fn := titleBarCallbacks[token]
	titleBarMu.Unlock()
	if fn != nil {
		fn()
	}
}

// GtkBox functions for stacked panes

// NewBox creates a new GtkBox widget with the specified orientation and spacing.
func NewBox(orientation Orientation, spacing int) uintptr {
	box := C.box_new(C.GtkOrientation(orientation), C.int(spacing))
	return uintptr(unsafe.Pointer(box))
}

// BoxAppend adds a child widget to the end of a GtkBox.
func BoxAppend(box uintptr, child uintptr) {
	if box == 0 || child == 0 {
		return
	}
	log.Printf("[workspace] BoxAppend box=%#x child=%#x", box, child)
	C.box_append((*C.GtkWidget)(unsafe.Pointer(box)), (*C.GtkWidget)(unsafe.Pointer(child)))
}

// BoxPrepend adds a child widget to the beginning of a GtkBox.
func BoxPrepend(box uintptr, child uintptr) {
	if box == 0 || child == 0 {
		return
	}
	log.Printf("[workspace] BoxPrepend box=%#x child=%#x", box, child)
	C.box_prepend((*C.GtkWidget)(unsafe.Pointer(box)), (*C.GtkWidget)(unsafe.Pointer(child)))
}

// BoxRemove removes a child widget from a GtkBox.
func BoxRemove(box uintptr, child uintptr) {
	if box == 0 || child == 0 {
		return
	}
	log.Printf("[workspace] BoxRemove box=%#x child=%#x", box, child)
	C.box_remove((*C.GtkWidget)(unsafe.Pointer(box)), (*C.GtkWidget)(unsafe.Pointer(child)))
}

// BoxInsertChildAfter inserts a child widget after a sibling in a GtkBox.
func BoxInsertChildAfter(box uintptr, child uintptr, sibling uintptr) {
	if box == 0 || child == 0 {
		return
	}
	log.Printf("[workspace] BoxInsertChildAfter box=%#x child=%#x sibling=%#x", box, child, sibling)
	C.box_insert_child_after((*C.GtkWidget)(unsafe.Pointer(box)), (*C.GtkWidget)(unsafe.Pointer(child)), (*C.GtkWidget)(unsafe.Pointer(sibling)))
}

// Widget visibility functions for stacked panes

// WidgetSetVisible sets the visibility state of a widget.
func WidgetSetVisible(widget uintptr, visible bool) {
	if widget == 0 {
		return
	}
	log.Printf("[workspace] WidgetSetVisible widget=%#x visible=%v", widget, visible)
	C.widget_set_visible((*C.GtkWidget)(unsafe.Pointer(widget)), goBool(visible))
}

// WidgetGetVisible returns the visibility state of a widget.
func WidgetGetVisible(widget uintptr) bool {
	if widget == 0 {
		return false
	}
	return C.widget_get_visible((*C.GtkWidget)(unsafe.Pointer(widget))) != 0
}

// WidgetHide hides a widget.
func WidgetHide(widget uintptr) {
	if widget == 0 {
		return
	}
	log.Printf("[workspace] WidgetHide widget=%#x", widget)
	C.widget_hide((*C.GtkWidget)(unsafe.Pointer(widget)))
}

// Label functions for title bars

// NewLabel creates a new GtkLabel widget with the specified text.
func NewLabel(text string) uintptr {
	cstr := C.CString(text)
	defer C.free(unsafe.Pointer(cstr))
	label := C.label_new(cstr)
	return uintptr(unsafe.Pointer(label))
}

// LabelSetText sets the text of a GtkLabel.
func LabelSetText(label uintptr, text string) {
	if label == 0 {
		return
	}
	cstr := C.CString(text)
	defer C.free(unsafe.Pointer(cstr))
	C.label_set_text((*C.GtkWidget)(unsafe.Pointer(label)), cstr)
}

// LabelGetText returns the text of a GtkLabel.
func LabelGetText(label uintptr) string {
	if label == 0 {
		return ""
	}
	cstr := C.label_get_text((*C.GtkWidget)(unsafe.Pointer(label)))
	if cstr == nil {
		return ""
	}
	return C.GoString(cstr)
}

// EllipsizeMode represents PangoEllipsizeMode values.
type EllipsizeMode int

const (
	EllipsizeNone   EllipsizeMode = 0 // PANGO_ELLIPSIZE_NONE
	EllipsizeStart  EllipsizeMode = 1 // PANGO_ELLIPSIZE_START
	EllipsizeMiddle EllipsizeMode = 2 // PANGO_ELLIPSIZE_MIDDLE
	EllipsizeEnd    EllipsizeMode = 3 // PANGO_ELLIPSIZE_END
)

// LabelSetEllipsize sets the ellipsization mode for a GtkLabel.
func LabelSetEllipsize(label uintptr, mode EllipsizeMode) {
	if label == 0 {
		return
	}
	C.label_set_ellipsize((*C.GtkWidget)(unsafe.Pointer(label)), C.int(mode))
}

// LabelSetMaxWidthChars sets the maximum width in characters for a GtkLabel.
func LabelSetMaxWidthChars(label uintptr, nChars int) {
	if label == 0 {
		return
	}
	C.label_set_max_width_chars((*C.GtkWidget)(unsafe.Pointer(label)), C.int(nChars))
}

// WidgetWaitForDraw is deprecated - GTK4 handles widget operations automatically
// This function is kept for compatibility but does nothing in production
func WidgetWaitForDraw(widget uintptr) {
	// GTK4 documentation states: never force a redraw
	// Widget operations are handled automatically by the GTK4 main loop
	// This function is a no-op in production code
}

// WidgetQueueDraw safely queues a redraw operation (GTK4 recommended approach)
func WidgetQueueDraw(widget uintptr) {
	if widget == 0 || !widgetIsValid(widget) {
		return
	}
	C.gtk_widget_queue_draw((*C.GtkWidget)(unsafe.Pointer(widget)))
}

// WidgetHasCSSClass checks if a widget has a specific CSS class
func WidgetHasCSSClass(widget uintptr, class string) bool {
	if widget == 0 || class == "" || !widgetIsValid(widget) {
		return false
	}
	cstr := C.CString(class)
	defer C.free(unsafe.Pointer(cstr))
	result := C.gtk_widget_has_css_class((*C.GtkWidget)(unsafe.Pointer(widget)), cstr)
	return result != 0
}

// Widget margin functions for creating visual borders

// WidgetSetMargin sets all margins (top, right, bottom, left) to the same value
func WidgetSetMargin(widget uintptr, margin int) {
	if widget == 0 || !widgetIsValid(widget) {
		return
	}
	w := (*C.GtkWidget)(unsafe.Pointer(widget))
	C.gtk_widget_set_margin_top(w, C.int(margin))
	C.gtk_widget_set_margin_bottom(w, C.int(margin))
	C.gtk_widget_set_margin_start(w, C.int(margin))
	C.gtk_widget_set_margin_end(w, C.int(margin))
}

// WidgetSetMarginTop sets the top margin
func WidgetSetMarginTop(widget uintptr, margin int) {
	if widget == 0 || !widgetIsValid(widget) {
		return
	}
	C.gtk_widget_set_margin_top((*C.GtkWidget)(unsafe.Pointer(widget)), C.int(margin))
}

// WidgetSetMarginBottom sets the bottom margin
func WidgetSetMarginBottom(widget uintptr, margin int) {
	if widget == 0 || !widgetIsValid(widget) {
		return
	}
	C.gtk_widget_set_margin_bottom((*C.GtkWidget)(unsafe.Pointer(widget)), C.int(margin))
}

// WidgetSetMarginStart sets the start (left in LTR) margin
func WidgetSetMarginStart(widget uintptr, margin int) {
	if widget == 0 || !widgetIsValid(widget) {
		return
	}
	C.gtk_widget_set_margin_start((*C.GtkWidget)(unsafe.Pointer(widget)), C.int(margin))
}

// WidgetSetMarginEnd sets the end (right in LTR) margin
func WidgetSetMarginEnd(widget uintptr, margin int) {
	if widget == 0 || !widgetIsValid(widget) {
		return
	}
	C.gtk_widget_set_margin_end((*C.GtkWidget)(unsafe.Pointer(widget)), C.int(margin))
}

// GTK4 Focus Controller functions for focus state machine

// WidgetAddFocusController registers focus callbacks for the given widget and
// returns a token that can be used to detach it later.
func WidgetAddFocusController(widget uintptr, onEnter, onLeave func()) uintptr {
	if widget == 0 || !widgetIsValid(widget) {
		return 0
	}

	focusMu.Lock()
	token := nextFocusID
	nextFocusID++

	callbacks := FocusCallbacks{
		OnEnter: onEnter,
		OnLeave: onLeave,
	}
	focusCallbacks[token] = callbacks
	focusMu.Unlock()

	controller := C.widget_add_focus_controller((*C.GtkWidget)(unsafe.Pointer(widget)), C.uintptr_t(token))
	if controller == nil {
		focusMu.Lock()
		delete(focusCallbacks, token)
		focusMu.Unlock()
		return 0
	}

	focusMu.Lock()
	focusControllers[token] = uintptr(unsafe.Pointer(controller))
	focusMu.Unlock()
	return token
}

// WidgetRemoveFocusController removes a previously registered focus controller.
func WidgetRemoveFocusController(widget uintptr, token uintptr) {
	if token == 0 {
		return
	}

	focusMu.Lock()
	controller := focusControllers[token]
	delete(focusCallbacks, token)
	delete(focusControllers, token)
	focusMu.Unlock()

	if controller == 0 {
		return
	}

	if widget == 0 || !widgetIsValid(widget) {
		return
	}

	C.widget_remove_focus_controller((*C.GtkWidget)(unsafe.Pointer(widget)), (*C.GtkEventController)(unsafe.Pointer(controller)))
}

//export goFocusEnterCallback
func goFocusEnterCallback(handle C.uintptr_t) {
	token := uintptr(handle)
	focusMu.Lock()
	callbacks := focusCallbacks[token]
	focusMu.Unlock()

	if callbacks.OnEnter != nil {
		callbacks.OnEnter()
	}
}

//export goFocusLeaveCallback
func goFocusLeaveCallback(handle C.uintptr_t) {
	token := uintptr(handle)
	focusMu.Lock()
	callbacks := focusCallbacks[token]
	focusMu.Unlock()

	if callbacks.OnLeave != nil {
		callbacks.OnLeave()
	}
}

// WidgetGetAllocation gets the bounds (position and size) of a widget using modern GTK4 API
func WidgetGetAllocation(widget uintptr) WidgetAllocation {
	var x, y, width, height C.int

	C.widget_get_allocation_modern(
		(*C.GtkWidget)(unsafe.Pointer(widget)),
		&x, &y, &width, &height,
	)

	return WidgetAllocation{
		X:      int(x),
		Y:      int(y),
		Width:  int(width),
		Height: int(height),
	}
}

// GtkImage functions for favicon display

// NewImage creates a new empty GtkImage widget.
func NewImage() uintptr {
	image := C.image_new()
	return uintptr(unsafe.Pointer(image))
}

// ImageNewFromFile creates a new GtkImage widget from a file path.
func ImageNewFromFile(filename string) uintptr {
	if filename == "" {
		return 0
	}
	cstr := C.CString(filename)
	defer C.free(unsafe.Pointer(cstr))
	image := C.image_new_from_file(cstr)
	return uintptr(unsafe.Pointer(image))
}

// ImageSetFromFile updates an existing GtkImage to display an image from a file.
func ImageSetFromFile(image uintptr, filename string) {
	if image == 0 || filename == "" {
		return
	}
	cstr := C.CString(filename)
	defer C.free(unsafe.Pointer(cstr))
	C.image_set_from_file((*C.GtkWidget)(unsafe.Pointer(image)), cstr)
}

// ImageSetPixelSize sets the pixel size for the image (used for scaling icons).
func ImageSetPixelSize(image uintptr, pixelSize int) {
	if image == 0 {
		return
	}
	C.image_set_pixel_size((*C.GtkWidget)(unsafe.Pointer(image)), C.int(pixelSize))
}
