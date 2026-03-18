// Package port defines application-layer interfaces for external capabilities.
// Ports abstract infrastructure concerns, allowing the application layer to
// remain independent of specific implementations (WebKit, GTK, etc.).
package port

import (
	"context"
)

// WebViewID uniquely identifies a WebView instance.
type WebViewID uint64

// LoadEvent represents page load state transitions.
type LoadEvent int

const (
	// LoadStarted indicates navigation has begun.
	LoadStarted LoadEvent = iota
	// LoadRedirected indicates a redirect occurred.
	LoadRedirected
	// LoadCommitted indicates content is being received.
	LoadCommitted
	// LoadFinished indicates the page has fully loaded.
	LoadFinished
)

// String returns a human-readable representation of the load event.
func (e LoadEvent) String() string {
	switch e {
	case LoadStarted:
		return "started"
	case LoadRedirected:
		return "redirected"
	case LoadCommitted:
		return "committed"
	case LoadFinished:
		return "finished"
	default:
		return "unknown"
	}
}

// WebViewState represents a snapshot of the current WebView state.
// This is an immutable struct that can be safely passed between components.
type WebViewState struct {
	URI       string
	Title     string
	IsLoading bool
	Progress  float64 // 0.0 to 1.0
	CanGoBack bool
	CanGoFwd  bool
	ZoomLevel float64
}

// WebProcessTerminationReason describes why the web process terminated.
type WebProcessTerminationReason int

const (
	WebProcessTerminationUnknown WebProcessTerminationReason = iota
	WebProcessTerminationCrashed
	WebProcessTerminationExceededMemory
	WebProcessTerminationByAPI
)

// PopupRequest contains metadata about a popup window request.
type PopupRequest struct {
	TargetURI     string
	FrameName     string // e.g., "_blank", custom name, or empty
	IsUserGesture bool
	ParentViewID  WebViewID
}

// Texture represents a graphics texture returned by the engine.
// GoPointer returns a native toolkit pointer (e.g. *gdk.Texture in GTK engines).
// Engine implementations are responsible for the concrete type.
type Texture interface {
	GoPointer() uintptr
}

// WebViewCallbacks defines callback handlers for WebView events.
// Implementations should invoke these on the main thread/goroutine.
type WebViewCallbacks struct {
	// OnLoadChanged is called when load state changes.
	OnLoadChanged func(event LoadEvent)
	// OnTitleChanged is called when the page title changes.
	OnTitleChanged func(title string)
	// OnURIChanged is called when the URI changes.
	OnURIChanged func(uri string)
	// OnProgressChanged is called during page load with progress 0.0-1.0.
	OnProgressChanged func(progress float64)
	// OnFaviconChanged is called when the page favicon changes.
	// The parameter is a *gdk.Texture (passed as Texture interface to avoid GTK import in port layer).
	OnFaviconChanged func(favicon Texture)
	// OnClose is called when the WebView requests to close.
	OnClose func()
	// OnCreate is called when a popup window is requested.
	// Return a WebView to allow the popup, or nil to block it.
	OnCreate func(request PopupRequest) WebView
	// OnReadyToShow is called when a popup WebView is ready to display.
	OnReadyToShow func()
	// OnLinkHover is called when hovering over a link, image, or media element.
	// The uri parameter contains the target URL, or empty string when leaving.
	OnLinkHover func(uri string)
	// OnWebProcessTerminated is called when the web process exits unexpectedly.
	OnWebProcessTerminated func(reason WebProcessTerminationReason, reasonLabel string, uri string)

	// OnPermissionRequest is called when a site requests permission (mic, camera, screen sharing).
	// Return true to indicate the request was handled. Call allow()/deny() to respond.
	// The permission types are passed as strings ("microphone", "camera", "display", etc.).
	OnPermissionRequest func(origin string, permTypes []string, allow, deny func()) bool

	// OnLinkMiddleClick is called when a link is middle-clicked.
	// Return true if handled (blocks default navigation).
	OnLinkMiddleClick func(uri string) bool

	// OnEnterFullscreen is called when the WebView requests fullscreen mode.
	// Return true to prevent fullscreen.
	OnEnterFullscreen func() bool

	// OnLeaveFullscreen is called when the WebView exits fullscreen mode.
	// Return true to prevent leaving fullscreen.
	OnLeaveFullscreen func() bool

	// OnAudioStateChanged is called when audio playback starts or stops.
	OnAudioStateChanged func(playing bool)
}

// FindOptions configures search behavior.
type FindOptions struct {
	CaseInsensitive bool
	AtWordStarts    bool
	WrapAround      bool
}

// FindController abstracts WebKit's FindController for clean architecture.
type FindController interface {
	Search(text string, options FindOptions, maxMatches uint)
	CountMatches(text string, options FindOptions, maxMatches uint)
	SearchNext()
	SearchPrevious()
	SearchFinish()
	GetSearchText() string

	// Signal connections
	OnFoundText(callback func(matchCount uint)) uint
	OnFailedToFindText(callback func()) uint
	OnCountedMatches(callback func(matchCount uint)) uint
	DisconnectSignal(id uint)
}

// WebView defines the port interface for browser view operations.
// This interface abstracts the underlying browser engine (WebKit, etc.)
// and exposes only the navigation and state capabilities needed by
// the application layer.
type WebView interface {
	// ID returns the unique identifier for this WebView.
	ID() WebViewID

	// --- Navigation ---

	// LoadURI navigates to the specified URI.
	LoadURI(ctx context.Context, uri string) error

	// LoadHTML loads HTML content with an optional base URI for relative links.
	LoadHTML(ctx context.Context, content, baseURI string) error

	// Reload reloads the current page.
	Reload(ctx context.Context) error

	// ReloadBypassCache reloads the current page, bypassing cache.
	ReloadBypassCache(ctx context.Context) error

	// Stop stops the current page load.
	Stop(ctx context.Context) error

	// GoBack navigates back in history.
	// Uses WebKit native navigation if available, falls back to JavaScript history.back() for SPA compatibility.
	GoBack(ctx context.Context) error

	// GoForward navigates forward in history.
	// Uses WebKit native navigation if available, falls back to JavaScript history.forward() for SPA compatibility.
	GoForward(ctx context.Context) error

	// --- State Queries ---

	// State returns the current WebView state as a snapshot.
	State() WebViewState

	// URI returns the current URI.
	URI() string

	// Title returns the current page title.
	Title() string

	// IsLoading returns true if a page is currently loading.
	IsLoading() bool

	// EstimatedProgress returns the load progress (0.0 to 1.0).
	EstimatedProgress() float64

	// CanGoBack returns true if back navigation is available.
	CanGoBack() bool

	// CanGoForward returns true if forward navigation is available.
	CanGoForward() bool

	// --- Zoom ---

	// SetZoomLevel sets the zoom level (1.0 = 100%).
	SetZoomLevel(ctx context.Context, level float64) error

	// GetZoomLevel returns the current zoom level.
	GetZoomLevel() float64

	// --- Find ---

	// GetFindController returns the find controller for text search.
	// Returns nil if find is not supported.
	GetFindController() FindController

	// --- Callbacks ---

	// SetCallbacks registers callback handlers for WebView events.
	// Pass nil to clear all callbacks.
	SetCallbacks(callbacks *WebViewCallbacks)

	// --- Appearance ---

	// RunJavaScript executes a script in the main world. Fire-and-forget.
	RunJavaScript(ctx context.Context, script string)

	// SetBackgroundColor sets the background color shown before content paints (0.0-1.0 RGBA).
	SetBackgroundColor(r, g, b, a float64)

	// ResetBackgroundToDefault resets the background to white (browser default).
	ResetBackgroundToDefault()

	// --- Favicon ---

	// Favicon returns the current page favicon, or nil if unavailable.
	Favicon() Texture

	// Generation returns a monotonic counter incremented on pool reuse.
	// Used to detect stale callbacks after a WebView is recycled.
	Generation() uint64

	// --- Media/Fullscreen State ---

	// IsFullscreen returns true if the WebView is currently in fullscreen mode.
	IsFullscreen() bool

	// IsPlayingAudio returns true if the WebView is currently playing audio.
	IsPlayingAudio() bool

	// --- Lifecycle ---

	// IsDestroyed returns true if the WebView has been destroyed.
	IsDestroyed() bool

	// Destroy releases all resources associated with this WebView.
	// After calling Destroy, the WebView should not be used.
	Destroy()
}

// WebViewPool defines the port interface for WebView pooling.
// Pools maintain pre-created WebViews for fast tab creation.
type WebViewPool interface {
	// Acquire obtains a WebView from the pool or creates a new one.
	// The context can be used for cancellation.
	Acquire(ctx context.Context) (WebView, error)

	// Release returns a WebView to the pool for reuse.
	// The WebView will be reset to a blank state.
	// If the pool is full, the WebView will be destroyed.
	Release(wv WebView)

	// Prewarm creates WebViews synchronously to populate the pool.
	// Pass count <= 0 to use the default prewarm count.
	Prewarm(count int)

	// PrewarmAsync schedules WebView creation on the toolkit idle loop,
	// avoiding blocking startup while still warming up WebViews.
	// Pass count <= 0 to use the default prewarm count.
	PrewarmAsync(ctx context.Context, count int)

	// Size returns the current number of available WebViews in the pool.
	Size() int

	// Close shuts down the pool and destroys all pooled WebViews.
	Close()
}

// WebViewFactory creates new WebView instances.
// This is used when direct WebView creation is needed without pooling.
type WebViewFactory interface {
	// Create creates a new WebView instance.
	Create(ctx context.Context) (WebView, error)

	// CreateRelated creates a WebView that shares session/cookies with parent.
	// This is required for popup windows to maintain authentication state.
	// Popup WebViews bypass the pool since they must be related to a specific parent.
	CreateRelated(ctx context.Context, parentID WebViewID) (WebView, error)
}

// DevToolsOpener is an optional capability for WebViews that support developer tools.
type DevToolsOpener interface {
	OpenDevTools()
}

// Printer is an optional capability for WebViews that support printing.
type Printer interface {
	PrintPage()
}

// PopupCapable is implemented by WebViews that support popup lifecycle callbacks.
// SetOnClose composes the provided function with any existing close handler so
// that multiple callers can each register a close callback without overwriting
// one another.
type PopupCapable interface {
	SetOnReadyToShow(fn func())
	SetOnClose(fn func())
	Show()
}

// OAuthCallbackCapable is implemented by WebViews that support OAuth auto-close callbacks.
// It is an optional capability; callers should type-assert port.WebView to this interface
// before use and degrade gracefully when it is absent.
type OAuthCallbackCapable interface {
	// AddCloseCallback registers fn to be called when the WebView is programmatically closed.
	// Multiple callbacks may be registered; each will be called in registration order.
	AddCloseCallback(fn func())
	// AddNavigationCallback registers fn to be called whenever the URI changes or a
	// committed load is detected. The uri parameter contains the new URL.
	// Multiple callbacks may be registered; each will be called in registration order.
	AddNavigationCallback(fn func(uri string))
	// Close triggers a programmatic close of the WebView (equivalent to window.close()).
	Close()
}

// TextInputTargetProvider is an optional capability for WebViews that support
// text input method integration.
type TextInputTargetProvider interface {
	TextInputTarget() TextInputTarget
}
