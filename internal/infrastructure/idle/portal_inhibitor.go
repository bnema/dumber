// Package idle provides system idle/screensaver inhibition via XDG Desktop Portal.
package idle

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/godbus/dbus/v5"
)

const (
	portalDest      = "org.freedesktop.portal.Desktop"
	portalPath      = "/org/freedesktop/portal/desktop"
	portalInterface = "org.freedesktop.portal.Inhibit"
	requestIface    = "org.freedesktop.portal.Request"

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
type PortalInhibitor struct {
	conn        *dbus.Conn
	requestPath dbus.ObjectPath // Active inhibit request handle
	refcount    int
	supported   bool
	mu          sync.Mutex
}

// NewPortalInhibitor creates a new portal-based idle inhibitor.
// Returns a functional inhibitor even if D-Bus is unavailable (graceful degradation).
func NewPortalInhibitor(ctx context.Context) *PortalInhibitor {
	log := logging.FromContext(ctx)

	inhibitor := &PortalInhibitor{
		supported: false,
	}

	// Connect to session bus
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		log.Debug().Err(err).Msg("idle inhibitor: cannot connect to D-Bus session bus")
		return inhibitor
	}
	inhibitor.conn = conn

	// Check if portal is available
	obj := conn.Object(portalDest, portalPath)
	var version uint32
	err = obj.Call("org.freedesktop.DBus.Properties.Get", 0,
		portalInterface, "version").Store(&version)
	if err != nil {
		log.Debug().Err(err).Msg("idle inhibitor: portal not available")
		return inhibitor
	}

	inhibitor.supported = true
	log.Debug().Uint32("version", version).Msg("idle inhibitor: portal available")

	return inhibitor
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

	// First inhibit request
	if !p.supported || p.conn == nil {
		log.Debug().Msg("idle inhibitor: not supported, skipping")
		return nil
	}

	// Call portal Inhibit method
	// Inhibit(window: s, flags: u, options: a{sv}) -> handle: o
	obj := p.conn.Object(portalDest, portalPath)

	options := map[string]dbus.Variant{
		"reason": dbus.MakeVariant(reason),
	}

	var handlePath dbus.ObjectPath
	err := obj.Call(portalInterface+".Inhibit", 0,
		"",                           // window identifier (empty for non-sandboxed)
		uint32(flagIdle|flagSuspend), // inhibit idle and suspend
		options,
	).Store(&handlePath)

	if err != nil {
		p.refcount--
		log.Warn().Err(err).Msg("idle inhibitor: failed to inhibit")
		return fmt.Errorf("portal inhibit: %w", err)
	}

	p.requestPath = handlePath
	log.Info().
		Str("handle", string(handlePath)).
		Str("reason", reason).
		Msg("idle inhibitor: activated")

	return nil
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
	if p.refcount > 0 {
		return nil
	}

	// Release the inhibition
	if !p.supported || p.conn == nil || p.requestPath == "" {
		return nil
	}

	// Close the request to release inhibition
	obj := p.conn.Object(portalDest, p.requestPath)
	err := obj.Call(requestIface+".Close", 0).Err

	if err != nil {
		log.Warn().Err(err).Msg("idle inhibitor: failed to close request")
		// Don't return error - the request may have already been closed
	} else {
		log.Info().Msg("idle inhibitor: deactivated")
	}

	p.requestPath = ""
	return nil
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

	// Release any active inhibition
	if p.conn != nil && p.requestPath != "" {
		obj := p.conn.Object(portalDest, p.requestPath)
		_ = obj.Call(requestIface+".Close", 0).Err
		p.requestPath = ""
	}

	p.refcount = 0

	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		return err
	}

	return nil
}
