package webkit

// Config holds initialization settings for the WebKit-based browser components.
// This will map to WebKit2GTK and GTK settings in the real implementation.
type Config struct {
	// InitialURL is the first URL to load when creating a WebView.
	InitialURL string

	// UserAgent allows overriding the default user agent string.
	UserAgent string

	// EnableDeveloperExtras enables devtools/inspector.
	EnableDeveloperExtras bool

	// ZoomDefault sets an initial zoom factor (1.0 = 100%).
	ZoomDefault float64

	// DataDir is the base directory for persistent website data (cookies, localStorage, etc.).
	DataDir string
	// CacheDir is the base directory for cache data.
	CacheDir string

	// Fonts apply to pages that don't specify fonts (browser defaults).
	DefaultSansFont      string
	DefaultSerifFont     string
	DefaultMonospaceFont string
	DefaultFontSize      int // CSS px (~points)

	// Rendering controls GPU/CPU selection and debug options
	Rendering RenderingConfig

	// VideoAcceleration controls video hardware acceleration settings
	VideoAcceleration VideoAccelerationConfig

	// Memory controls memory optimization settings
	Memory MemoryConfig

	// CodecPreferences for media playback
	CodecPreferences CodecPreferencesConfig
}

// CodecPreferencesConfig holds codec preferences for WebKit media playback
type CodecPreferencesConfig struct {
	PreferredCodecs           []string
	BlockedCodecs             []string
	ForceAV1                  bool
	CustomUserAgent           string
	DisableTwitchCodecControl bool
}

// RenderingConfig controls hardware acceleration preferences.
// Mode accepts: "auto" (default), "gpu", or "cpu".
// DebugGPU enables compositing indicators if supported.
type RenderingConfig struct {
	Mode     string
	DebugGPU bool
}

// VideoAccelerationConfig controls video hardware acceleration settings.
type VideoAccelerationConfig struct {
	EnableVAAPI      bool
	AutoDetectGPU    bool
	VAAPIDriverName  string
	EnableAllDrivers bool
	LegacyVAAPI      bool
}

// CacheModel represents WebKit cache model options for memory optimization.
type CacheModel string

const (
	CacheModelDocumentViewer    CacheModel = "document_viewer"     // Minimal caching, lowest memory
	CacheModelWebBrowser        CacheModel = "web_browser"         // Default, balanced caching
	CacheModelPrimaryWebBrowser CacheModel = "primary_web_browser" // Maximum caching, highest memory
)

// MemoryConfig controls memory optimization settings.
type MemoryConfig struct {
	// MemoryLimitMB sets the maximum memory limit per WebView in MB (0 = unlimited)
	MemoryLimitMB int

	// ConservativeThreshold triggers early memory cleanup (0.0-1.0, default 0.33)
	ConservativeThreshold float64

	// StrictThreshold triggers aggressive memory cleanup (0.0-1.0, default 0.5)
	StrictThreshold float64

	// KillThreshold kills processes at this memory usage (0.0-1.0, 0 = disabled)
	KillThreshold float64

	// PollIntervalSeconds sets how often to check memory usage (default 2.0)
	PollIntervalSeconds float64

	// CacheModel determines caching strategy (default: web_browser)
	CacheModel CacheModel

	// EnablePageCache enables back/forward page caching (default: true)
	EnablePageCache bool

	// EnableOfflineAppCache enables offline application caching (default: true)
	EnableOfflineAppCache bool

	// ProcessRecycleThreshold recycles WebView after N page loads (0 = disabled)
	ProcessRecycleThreshold int

	// EnableGCInterval enables periodic JS garbage collection in seconds (0 = disabled)
	EnableGCInterval int

	// EnableMemoryMonitoring logs memory usage and pressure events
	EnableMemoryMonitoring bool
}
