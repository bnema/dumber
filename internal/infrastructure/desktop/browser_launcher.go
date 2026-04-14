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

	resolveExecutablePath := l.resolveExecutablePath
	if resolveExecutablePath == nil {
		resolveExecutablePath = getExecutablePath
	}
	execPath, err := resolveExecutablePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve dumber executable for URL %q: %v\n", url, err)
		return
	}

	cmd := exec.Command(execPath, "browse", url)
	cmd.Env = sanitizedChildEnv(os.Environ())

	spawnDetachedProcess := l.startDetachedProcess
	if spawnDetachedProcess == nil {
		spawnDetachedProcess = startDetachedProcess
	}
	if err := spawnDetachedProcess(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "failed to launch dumber browse for URL %q: %v\n", url, err)
	}
}
