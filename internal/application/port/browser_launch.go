package port

import (
	"context"
	"io"
)

// BrowserWindowOpener opens browser URLs received through the launch relay.
type BrowserWindowOpener interface {
	OpenExternalURL(ctx context.Context, url string) error
	OpenFreshWindow(ctx context.Context, url string) error
}

// BrowserLaunchRelay delivers browser URL launch requests.
type BrowserLaunchRelay interface {
	// DeliverOpenExternalURL attempts to deliver a request to open an external URL.
	// The bool reports whether the relay accepted the request; false means the
	// caller should treat it as undelivered and try another path.
	// An error may still be returned with delivered=true when the relay accepted
	// the request but could not confirm completion before the caller timed out.
	DeliverOpenExternalURL(ctx context.Context, url string) (bool, error)
	// DeliverOpenFreshWindow attempts to deliver a request that must open in a
	// new browser window, independently of external-link workspace settings.
	DeliverOpenFreshWindow(ctx context.Context, url string) (bool, error)
	Listen(ctx context.Context, opener BrowserWindowOpener) (io.Closer, error)
}

// AlreadyRunningAppRelaunchHandlerSetter configures a handler for CEF relaunches.
type AlreadyRunningAppRelaunchHandlerSetter interface {
	SetAlreadyRunningAppRelaunchHandler(func(string))
}
