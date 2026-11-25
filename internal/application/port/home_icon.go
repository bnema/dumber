package port

import (
	"context"
	"errors"
	"time"
)

// ErrHomeIconNotFound reports that no usable icon exists for a home service.
// Callers show a generic icon.
var ErrHomeIconNotFound = errors.New("home service icon not found")

// HomeIconImage is raw icon data with its sniffed MIME type.
type HomeIconImage struct {
	URL         string
	Bytes       []byte
	ContentType string
}

// HomeIconSource discovers and loads icons for home services configured by
// the user. It may reach private network addresses because service URLs come
// from trusted user config. It must never be used for page-supplied URLs.
type HomeIconSource interface {
	// DiscoverIconURLs returns candidate icon URLs for serviceURL, best first.
	// Private network addresses are allowed only on serviceURL's host:port.
	DiscoverIconURLs(ctx context.Context, serviceURL string) ([]string, error)
	// FetchIcon downloads one icon URL found for serviceURL. Private network
	// addresses are allowed only on serviceURL's own host:port.
	FetchIcon(ctx context.Context, serviceURL, iconURL string) (*HomeIconImage, error)
	// ReadLocalIcon reads a local override icon from an absolute path.
	ReadLocalIcon(ctx context.Context, path string) (*HomeIconImage, error)
}

// HomeIconCache stores normalized home service icons, separate from the
// browsing favicon cache.
type HomeIconCache interface {
	// Get returns the cached PNG and when it was stored.
	Get(ctx context.Context, serviceURL string) (png []byte, storedAt time.Time, ok bool)
	Put(ctx context.Context, serviceURL string, png []byte) error
}

// HomeIconResolver is the boundary-safe API used by engine scheme handlers to
// serve dumb://api/home-icon. The page sends only a service id; the
// implementation resolves the service from home.toml.
type HomeIconResolver interface {
	// ResolveHomeIconByID returns a PNG for the service, or an error wrapping
	// ErrHomeIconNotFound for unknown ids, invalid ids, or missing icons.
	ResolveHomeIconByID(ctx context.Context, serviceID string) (png []byte, contentType string, err error)
}

// HomeIconResolverSetter is an optional engine capability that wires the
// home icon resolver to internal systemview pages.
type HomeIconResolverSetter interface {
	SetHomeIconResolver(resolver HomeIconResolver)
}
