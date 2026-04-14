package port

import (
	"context"
	"io"
)

// BrowserWindowOpener opens a fresh browser window.
type BrowserWindowOpener interface {
	OpenFreshWindow(ctx context.Context, url string) error
}

// BrowserLaunchRelay delivers fresh-window launch requests.
type BrowserLaunchRelay interface {
	// DeliverOpenFreshWindow attempts to deliver a request to open a fresh window.
	// The bool reports whether the relay accepted the request; false means the
	// caller should treat it as undelivered and try another path.
	DeliverOpenFreshWindow(ctx context.Context, url string) (bool, error)
	Listen(ctx context.Context, opener BrowserWindowOpener) (io.Closer, error)
}
