package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/bnema/dumber/internal/application/port"
)

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
func (l *BrowserLauncher) LaunchURL(ctx context.Context, url string) {
	if l == nil {
		return
	}

	if l.relay != nil {
		delivered, err := l.relay.DeliverOpenFreshWindow(ctx, url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to forward dumber browser URL %q: %v\n", url, err)
		}
		if delivered {
			return
		}
	}

	launchBrowserBrowseURL(url, l.resolveExecutablePath, l.startDetachedProcess)
}
