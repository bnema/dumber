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
}

// RenderingConfig controls hardware acceleration preferences.
// Mode accepts: "auto" (default), "gpu", or "cpu".
// DebugGPU enables compositing indicators if supported.
type RenderingConfig struct {
    Mode    string
    DebugGPU bool
}
