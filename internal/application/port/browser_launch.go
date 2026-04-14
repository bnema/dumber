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
	DeliverOpenFreshWindow(ctx context.Context, url string) (bool, error)
	Listen(ctx context.Context, opener BrowserWindowOpener) (io.Closer, error)
}
