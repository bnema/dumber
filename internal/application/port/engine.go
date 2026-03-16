// Package port defines application-layer interfaces for external capabilities.
// Ports abstract infrastructure concerns, allowing the application layer to
// remain independent of specific implementations (WebKit, GTK, etc.).
package port

import "context"

// CookiePolicy controls cookie acceptance behavior for the engine's network session.
type CookiePolicy string

const (
	// CookiePolicyAlways accepts all cookies.
	CookiePolicyAlways CookiePolicy = "always"
	// CookiePolicyNoThirdParty rejects third-party cookies.
	CookiePolicyNoThirdParty CookiePolicy = "no_third_party"
	// CookiePolicyNever rejects all cookies.
	CookiePolicyNever CookiePolicy = "never"
)

// MemoryPressureConfig holds memory pressure settings for an engine process.
// This is a value type (no infrastructure dependencies) co-located with the
// interfaces that use it (MemoryPressureApplier, EngineOptions).
// Zero values mean "use engine defaults".
type MemoryPressureConfig struct {
	// MemoryLimitMB sets the memory limit in megabytes.
	// 0 means unset (uses engine default).
	MemoryLimitMB int

	// PollIntervalSec sets the interval for memory usage checks.
	// 0 means unset (uses engine default: 30 seconds).
	PollIntervalSec float64

	// ConservativeThreshold sets threshold for conservative memory release.
	// Must be in (0, 1). 0 means unset (uses engine default: 0.33).
	ConservativeThreshold float64

	// StrictThreshold sets threshold for strict memory release.
	// Must be in (0, 1). 0 means unset (uses engine default: 0.5).
	StrictThreshold float64
}

// IsConfigured returns true if any memory pressure setting is configured.
func (c *MemoryPressureConfig) IsConfigured() bool {
	if c == nil {
		return false
	}
	return c.MemoryLimitMB > 0 ||
		c.PollIntervalSec > 0 ||
		c.ConservativeThreshold > 0 ||
		c.StrictThreshold > 0
}

// MemoryPressureApplier applies memory pressure settings to engine processes.
// This interface abstracts engine-specific memory pressure configuration
// to allow testing without engine dependencies.
type MemoryPressureApplier interface {
	// ApplyNetworkProcessSettings applies memory pressure settings to the network process.
	// Must be called BEFORE creating any NetworkSession.
	ApplyNetworkProcessSettings(ctx context.Context, cfg *MemoryPressureConfig) error

	// ApplyWebProcessSettings applies memory pressure settings to web processes.
	// Returns an opaque settings object that should be passed to context creation.
	// Returns nil if no settings are configured.
	ApplyWebProcessSettings(ctx context.Context, cfg *MemoryPressureConfig) (any, error)
}

// EngineOptions configures engine initialization.
type EngineOptions struct {
	// DataDir is the directory for persistent data (cookies, storage, etc.).
	DataDir string

	// CacheDir is the directory for cache data.
	CacheDir string

	// CookiePolicy controls cookie acceptance behavior.
	// Empty value means runtime defaults are used.
	CookiePolicy CookiePolicy

	// WebProcessMemory configures memory pressure for web processes.
	// nil means use engine defaults.
	WebProcessMemory *MemoryPressureConfig

	// NetworkProcessMemory configures memory pressure for the network process.
	// nil means use engine defaults.
	NetworkProcessMemory *MemoryPressureConfig
}

// Engine is the top-level interface for a browser engine implementation.
// It provides access to all engine subsystems and manages the lifecycle
// of the underlying browser context.
//
// Engines are fully initialized by their constructor (e.g., webkit.NewEngine).
// There is no separate Init step.
type Engine interface {
	// Factory returns the WebViewFactory for creating new WebView instances.
	Factory() WebViewFactory

	// Pool returns the WebViewPool for acquiring pre-warmed WebView instances.
	Pool() WebViewPool

	// SchemeHandler returns the SchemeHandler for registering custom URI schemes.
	SchemeHandler() SchemeHandler

	// ContentInjector returns the ContentInjector for injecting scripts and styles.
	ContentInjector() ContentInjector

	// MessageRouter returns the MessageRouter for JS-to-Go message passing.
	MessageRouter() MessageRouter

	// SettingsApplier returns the SettingsApplier for applying browser settings.
	SettingsApplier() SettingsApplier

	// FilterApplier returns the FilterApplier for applying content filters.
	// Returns nil if content filtering is not supported by this engine.
	FilterApplier() FilterApplier

	// FaviconDatabase returns the FaviconDatabase for async favicon lookups.
	FaviconDatabase() FaviconDatabase

	// InternalSchemePath returns the URI scheme used for internal app resources.
	InternalSchemePath() string

	// Close releases all resources held by the engine.
	Close() error
}

// SchemeHandler defines the port interface for registering custom URI schemes.
// Implementations intercept navigation to registered schemes and serve content
// from application code.
type SchemeHandler interface {
	// RegisterScheme registers a handler for the given URI scheme.
	// The handler receives the full URI and returns the response body,
	// MIME type, and any error.
	RegisterScheme(scheme string, handler func(uri string) ([]byte, string, error))
}

// MessageRouter defines the port interface for bidirectional JS-to-Go messaging.
// It allows JavaScript running in a WebView to invoke named Go handlers,
// and allows Go code to post messages back to a specific WebView.
type MessageRouter interface {
	// RegisterHandler registers a named message handler callable from JavaScript.
	// The handler receives a JSON-encoded message string and returns a
	// JSON-encoded response string.
	RegisterHandler(name string, handler func(message string) (string, error))

	// PostMessage sends a message to the JavaScript context of the given WebView.
	PostMessage(webviewID WebViewID, message string) error
}

// SettingsApplier defines the port interface for applying browser settings to WebViews.
// Implementations apply engine-specific settings (security, features, etc.) uniformly
// across a set of WebView instances.
type SettingsApplier interface {
	// ApplyToAll applies settings to all provided WebView instances.
	ApplyToAll(ctx context.Context, webviews []WebView)
}

// FilterApplier defines the port interface for applying content filters to WebViews.
// Implementations configure content blocking rules on a set of WebView instances.
// This interface is optional; Engine.FilterApplier() returns nil if not supported.
type FilterApplier interface {
	// ApplyToAll applies content filters to all provided WebView instances.
	ApplyToAll(ctx context.Context, webviews []WebView)
}

// FaviconDatabase defines the port interface for async favicon lookups.
// Implementations retrieve favicons from an engine-managed database and
// deliver them via callback on the main thread.
type FaviconDatabase interface {
	// GetFaviconAsync retrieves the favicon for the given page URL asynchronously.
	// The callback is invoked with the favicon Texture when available.
	GetFaviconAsync(pageURL string, callback func(Texture))
}

// ScriptRefresher defines the port interface for refreshing injected scripts
// across multiple WebView instances (e.g., after a settings change).
type ScriptRefresher interface {
	// RefreshAll re-injects scripts into all provided WebView instances.
	RefreshAll(ctx context.Context, webviews []WebView)
}

// NativeWidgetProvider is an optional capability for WebViews that can provide
// a native GTK widget pointer for embedding into the UI layout.
type NativeWidgetProvider interface {
	// NativeWidget returns the underlying native widget pointer.
	NativeWidget() uintptr
}
