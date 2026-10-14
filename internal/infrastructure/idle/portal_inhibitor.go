// Package idle provides system idle/screensaver inhibition via XDG Desktop Portal.
package idle

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/zerobus"
)

const (
	portalDest      = "org.freedesktop.portal.Desktop"
	portalPath      = "/org/freedesktop/portal/desktop"
	portalInterface = "org.freedesktop.portal.Inhibit"
	requestIface    = "org.freedesktop.portal.Request"
	propertiesIface = "org.freedesktop.DBus.Properties"

	// Inhibit flags from portal spec
	flagLogout     = 1
	flagUserSwitch = 2
	flagSuspend    = 4
	flagIdle       = 8
)

// Compile-time interface check.
var _ port.IdleInhibitor = (*PortalInhibitor)(nil)

// PortalInhibitor implements idle inhibition using XDG Desktop Portal.
// This works on Wayland with any compositor (GNOME, KDE, sway, hyprland, etc.).
//
// A zerobus connection is not safe for concurrent use, so every bus call
// runs under mu. Calls drop the signals that arrive meanwhile.
type PortalInhibitor struct {
	mu          sync.Mutex
	conn        *zerobus.Conn // nil when the portal is unavailable or after Close
	requestPath string        // active inhibit request handle
	refcount    int
}

// NewPortalInhibitor creates a new portal-based idle inhibitor.
// Returns a functional inhibitor even if D-Bus is unavailable (graceful degradation).
func NewPortalInhibitor(ctx context.Context) *PortalInhibitor {
	conn, err := zerobus.SessionBus()
	if err != nil {
		logging.FromContext(ctx).Debug().Err(err).Msg("idle inhibitor: cannot connect to D-Bus session bus")
		return &PortalInhibitor{}
	}
	return newPortalInhibitor(ctx, conn)
}

// newPortalInhibitor takes ownership of conn and keeps it only if the portal
// answers on it.
func newPortalInhibitor(ctx context.Context, conn *zerobus.Conn) *PortalInhibitor {
	log := logging.FromContext(ctx)

	version, err := portalVersion(conn)
	if err != nil {
		log.Debug().Err(err).Msg("idle inhibitor: portal not available")
		_ = conn.Close()
		return &PortalInhibitor{}
	}

	log.Debug().Uint32("version", version).Msg("idle inhibitor: portal available")
	return &PortalInhibitor{conn: conn}
}

// portalVersion reads the version property of the Inhibit portal.
func portalVersion(conn *zerobus.Conn) (uint32, error) {
	e := conn.NewCall(portalDest, portalPath, propertiesIface, "Get", "ss")
	e.Str(portalInterface)
	e.Str("version")
	m, err := conn.Call()
	if err != nil {
		return 0, err
	}
	body := m.Body()
	if sig := body.Variant(); sig != "u" {
		return 0, fmt.Errorf("portal version has type %q, want \"u\"", sig)
	}
	version := body.Uint32()
	return version, body.Err()
}

// Inhibit increments the inhibit refcount. First call activates inhibition.
func (p *PortalInhibitor) Inhibit(ctx context.Context, reason string) error {
	log := logging.FromContext(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.refcount++
	log.Debug().Int("refcount", p.refcount).Msg("idle inhibitor: inhibit called")

	// Already inhibiting
	if p.refcount > 1 {
		return nil
	}

	if p.conn == nil {
		log.Debug().Msg("idle inhibitor: not supported, skipping")
		return nil
	}

	handle, err := p.callInhibit(reason)
	if err != nil {
		p.refcount--
		log.Warn().Err(err).Msg("idle inhibitor: failed to inhibit")
		return fmt.Errorf("portal inhibit: %w", err)
	}
	p.requestPath = handle

	log.Info().
		Str("handle", handle).
		Str("reason", reason).
		Msg("idle inhibitor: activated")

	return nil
}

// callInhibit calls Inhibit(window: s, flags: u, options: a{sv}) -> handle: o
// and returns the request handle.
func (p *PortalInhibitor) callInhibit(reason string) (string, error) {
	e := p.conn.NewCall(portalDest, portalPath, portalInterface, "Inhibit", "sua{sv}")
	e.Str("")                        // window identifier (empty for non-sandboxed)
	e.Uint32(flagIdle | flagSuspend) // inhibit idle and suspend
	options := e.BeginArray('{')
	e.Struct()
	e.Str("reason")
	e.Variant("s")
	e.Str(reason)
	e.EndArray(options)

	m, err := p.conn.Call()
	if err != nil {
		return "", err
	}
	body := m.Body()
	// The reply points into the receive buffer: copy the handle we keep.
	handle := strings.Clone(body.ObjectPath())
	if err := body.Err(); err != nil {
		return "", err
	}
	return handle, nil
}

// Uninhibit decrements the refcount. When zero, releases inhibition.
func (p *PortalInhibitor) Uninhibit(ctx context.Context) error {
	log := logging.FromContext(ctx)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.refcount <= 0 {
		return nil // Nothing to uninhibit
	}

	p.refcount--
	log.Debug().Int("refcount", p.refcount).Msg("idle inhibitor: uninhibit called")

	// Still have active inhibitors
	if p.refcount > 0 || p.requestPath == "" {
		return nil
	}

	p.closeRequest()
	log.Info().Msg("idle inhibitor: deactivated")
	return nil
}

// closeRequest releases the active inhibit request. A portal that already
// ended the request answers with an error, which is ignored.
func (p *PortalInhibitor) closeRequest() {
	if p.conn != nil && p.requestPath != "" {
		p.conn.NewCall(portalDest, p.requestPath, requestIface, "Close", "")
		_, _ = p.conn.Call()
	}
	p.requestPath = ""
}

// IsInhibited returns true if currently inhibiting idle.
func (p *PortalInhibitor) IsInhibited() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.refcount > 0
}

// Close releases D-Bus resources and any active inhibition.
func (p *PortalInhibitor) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closeRequest()
	p.refcount = 0

	if p.conn == nil {
		return nil
	}
	err := p.conn.Close()
	p.conn = nil
	return err
}
