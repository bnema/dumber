package webkit

// WindowType indicates how a new WebView should be treated
type WindowType int

const (
	// WindowTypeTab represents an independent WebView (future: tab)
	WindowTypeTab WindowType = iota
	// WindowTypePopup represents a related WebView (shares process/context)
	WindowTypePopup
	// WindowTypeUnknown indicates type not detected yet
	WindowTypeUnknown
)

// WindowFeatures describes the window features detected from WebKitWindowProperties
type WindowFeatures struct {
	Width              int
	Height             int
	ToolbarVisible     bool
	LocationbarVisible bool
	MenubarVisible     bool
	Resizable          bool
}

// Config holds WebView configuration
type Config struct {
	// UserAgent string for the WebView
	UserAgent string

	// EnableJavaScript controls JavaScript execution
	EnableJavaScript bool

	// EnableWebGL controls WebGL support
	EnableWebGL bool

	// EnableMediaStream controls media stream support
	EnableMediaStream bool

	// HardwareAcceleration controls GPU acceleration
	HardwareAcceleration bool

	// DefaultFontSize in pixels
	DefaultFontSize int

	// MinimumFontSize in pixels
	MinimumFontSize int

	// Performance optimizations
	// EnablePageCache enables back/forward cache for instant navigation
	EnablePageCache bool

	// EnableSmoothScrolling enables smooth scrolling animations
	EnableSmoothScrolling bool

	// DataDir is the directory for persistent data (cookies, localStorage, etc.)
	DataDir string

	// CacheDir is the directory for HTTP cache
	CacheDir string

	// AppearanceConfigJSON is the JSON string for appearance configuration
	// This will be injected at document-start via UserContentManager
	AppearanceConfigJSON string

	// CreateWindow controls whether to create a standalone GTK Window for this WebView
	// Set to false for WebViews that will be embedded in workspace panes
	CreateWindow bool
}

// GetDefaultConfig returns a Config with sensible defaults
func GetDefaultConfig() *Config {
	return &Config{
		UserAgent:                 "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15", // WebKitGTK default (same as Epiphany)
		EnableJavaScript:          true,
		EnableWebGL:               true,
		EnableMediaStream:         true,
		HardwareAcceleration:      true,
		DefaultFontSize:           16,
		MinimumFontSize:           8,
		EnablePageCache:       true, // Instant back/forward navigation
		EnableSmoothScrolling: true, // Better UX
		CreateWindow:          true, // Default to standalone window
	}
}
