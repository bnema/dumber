package desktop

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/bnema/dumber/internal/application/port"
)

// ErrBrowserLaunchUnconfirmed reports that a running browser instance may have
// received the request, but the caller could not confirm that in time.
var ErrBrowserLaunchUnconfirmed = errors.New("browser launch could not be confirmed")

// BrowserLauncher opens URLs in a relay-first dumber browser instance.
type BrowserLauncher struct {
	relay                 port.BrowserLaunchRelay
	resolveExecutablePath func() (string, error)
	startDetachedProcess  func(*exec.Cmd) error
}

// NewBrowserLauncher creates a browser launcher that prefers relay handoff.
func NewBrowserLauncher(relay port.BrowserLaunchRelay) *BrowserLauncher {
	return &BrowserLauncher{
		relay:                 relay,
		resolveExecutablePath: getExecutablePath,
		startDetachedProcess:  startDetachedProcess,
	}
}

// LaunchURL forwards the URL to a running instance when possible, otherwise spawns a new one.
func (l *BrowserLauncher) LaunchURL(ctx context.Context, url string) error {
	if l == nil {
		return errors.New("browser launcher is unavailable")
	}

	if l.relay != nil {
		delivered, err := l.relay.DeliverOpenFreshWindow(ctx, url)
		if err != nil {
			if delivered && errors.Is(err, ErrBrowserLaunchRelayUnconfirmed) {
				return ErrBrowserLaunchUnconfirmed
			}
			return fmt.Errorf("forward dumber browser URL: %w", err)
		}
		if delivered {
			return nil
		}
	}

	if err := launchBrowserBrowseURL(url, l.resolveExecutablePath, l.startDetachedProcess); err != nil {
		return fmt.Errorf("launch dumber browse: %w", err)
	}
	return nil
}
