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

	// DataDir is the directory for persistent data (cookies, localStorage, etc.)
	DataDir string

	// CacheDir is the directory for HTTP cache
	CacheDir string
}

// GetDefaultConfig returns a Config with sensible defaults
func GetDefaultConfig() *Config {
	return &Config{
		UserAgent:            "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
		EnableJavaScript:     true,
		EnableWebGL:          true,
		EnableMediaStream:    true,
		HardwareAcceleration: true,
		DefaultFontSize:      16,
		MinimumFontSize:      8,
	}
}
