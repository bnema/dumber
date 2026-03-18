package webkit

import "github.com/bnema/dumber/internal/application/port"

// cookiePolicy controls cookie acceptance behavior for the WebKit NetworkSession.
// This is a webkit-internal type; values match port.CookiePolicy string values.
type cookiePolicy = port.CookiePolicy

const (
	cookiePolicyAlways       cookiePolicy = port.CookiePolicyAlways
	cookiePolicyNoThirdParty cookiePolicy = port.CookiePolicyNoThirdParty
	cookiePolicyNever        cookiePolicy = port.CookiePolicyNever
)

// webKitContextOptions configures WebKitContext creation.
// This is a webkit-specific options struct that extends EngineOptions with
// WebKit-specific fields (e.g. ITPEnabled).
type webKitContextOptions struct {
	// DataDir is the directory for persistent data (cookies, storage, etc.).
	DataDir string

	// CacheDir is the directory for cache data.
	CacheDir string

	// CookiePolicy controls cookie acceptance behavior.
	// Empty value means runtime defaults are used.
	CookiePolicy cookiePolicy

	// ITPEnabled controls Intelligent Tracking Prevention for NetworkSession.
	// False means ITP remains disabled unless enabled explicitly.
	ITPEnabled bool

	// WebProcessMemory configures memory pressure for web processes.
	// nil means use WebKit defaults.
	WebProcessMemory *port.MemoryPressureConfig

	// NetworkProcessMemory configures memory pressure for the network process.
	// nil means use WebKit defaults.
	NetworkProcessMemory *port.MemoryPressureConfig
}

// IsWebProcessMemoryConfigured returns true if web process memory settings are configured.
func (o *webKitContextOptions) IsWebProcessMemoryConfigured() bool {
	if o == nil || o.WebProcessMemory == nil {
		return false
	}
	return o.WebProcessMemory.IsConfigured()
}

// IsNetworkProcessMemoryConfigured returns true if network process memory settings are configured.
func (o *webKitContextOptions) IsNetworkProcessMemoryConfigured() bool {
	if o == nil || o.NetworkProcessMemory == nil {
		return false
	}
	return o.NetworkProcessMemory.IsConfigured()
}
