// Package port defines interfaces for external dependencies.
package port

import "context"

// WebKitCookiePolicy controls cookie acceptance behavior for NetworkSession.
type WebKitCookiePolicy string

const (
	WebKitCookiePolicyAlways       WebKitCookiePolicy = "always"
	WebKitCookiePolicyNoThirdParty WebKitCookiePolicy = "no_third_party"
	WebKitCookiePolicyNever        WebKitCookiePolicy = "never"
)

// MemoryPressureConfig holds memory pressure settings for a WebKit process.
// Zero values mean "use WebKit defaults".
type MemoryPressureConfig struct {
	// MemoryLimitMB sets the memory limit in megabytes.
	// 0 means unset (uses WebKit default: system RAM capped at 3GB).
	MemoryLimitMB int

	// PollIntervalSec sets the interval for memory usage checks.
	// 0 means unset (uses WebKit default: 30 seconds).
	PollIntervalSec float64

	// ConservativeThreshold sets threshold for conservative memory release.
	// Must be in (0, 1). 0 means unset (uses WebKit default: 0.33).
	ConservativeThreshold float64

	// StrictThreshold sets threshold for strict memory release.
	// Must be in (0, 1). 0 means unset (uses WebKit default: 0.5).
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

// WebKitContextOptions configures WebKitContext creation.
type WebKitContextOptions struct {
	// DataDir is the directory for persistent data (cookies, storage, etc.).
	DataDir string

	// CacheDir is the directory for cache data.
	CacheDir string

	// CookiePolicy controls cookie acceptance behavior.
	CookiePolicy WebKitCookiePolicy

	// ITPEnabled controls Intelligent Tracking Prevention for NetworkSession.
	ITPEnabled bool

	// WebProcessMemory configures memory pressure for web processes.
	// nil means use WebKit defaults.
	WebProcessMemory *MemoryPressureConfig

	// NetworkProcessMemory configures memory pressure for the network process.
	// nil means use WebKit defaults.
	NetworkProcessMemory *MemoryPressureConfig
}

// IsWebProcessMemoryConfigured returns true if web process memory settings are configured.
func (o *WebKitContextOptions) IsWebProcessMemoryConfigured() bool {
	if o == nil {
		return false
	}
	return o.WebProcessMemory.IsConfigured()
}

// IsNetworkProcessMemoryConfigured returns true if network process memory settings are configured.
func (o *WebKitContextOptions) IsNetworkProcessMemoryConfigured() bool {
	if o == nil {
		return false
	}
	return o.NetworkProcessMemory.IsConfigured()
}

// MemoryPressureApplier applies memory pressure settings to WebKit processes.
// This interface abstracts the WebKit-specific memory pressure configuration
// to allow testing without WebKit dependencies.
type MemoryPressureApplier interface {
	// ApplyNetworkProcessSettings applies memory pressure settings to the network process.
	// Must be called BEFORE creating any NetworkSession.
	ApplyNetworkProcessSettings(ctx context.Context, cfg *MemoryPressureConfig) error

	// ApplyWebProcessSettings applies memory pressure settings to web processes.
	// Returns an opaque settings object that should be passed to WebContext creation.
	// Returns nil if no settings are configured.
	ApplyWebProcessSettings(ctx context.Context, cfg *MemoryPressureConfig) (any, error)
}

// WebKitContextProvider creates and manages WebKit contexts.
// This interface abstracts WebKit context creation for testability.
type WebKitContextProvider interface {
	// Initialize sets up the WebKit context with the given options.
	// Must be called before any other methods.
	Initialize(ctx context.Context, opts WebKitContextOptions) error

	// DataDir returns the data directory path.
	DataDir() string

	// CacheDir returns the cache directory path.
	CacheDir() string

	// PrefetchDNS prefetches DNS for the given hostname.
	PrefetchDNS(hostname string)

	// Close releases resources held by the context.
	Close() error
}
