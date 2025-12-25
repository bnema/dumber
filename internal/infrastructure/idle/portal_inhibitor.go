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
	conn            *dbus.Conn
	requestPath     dbus.ObjectPath // Active inhibit request handle
	refcount        int
	supported       bool
	requestComplete bool // True if the portal sent a Response signal (request no longer exists)
	mu              sync.Mutex
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
	p.requestComplete = false

	// Subscribe to the Response signal to know when the inhibition ends
	// Some portals complete the request immediately with a Response signal
	go p.watchForResponse(ctx, handlePath)

	log.Info().
		Str("handle", string(handlePath)).
		Str("reason", reason).
		Msg("idle inhibitor: activated")

	return nil
}

// watchForResponse monitors for the Response signal on the request object.
// Some portals (particularly GNOME) complete the Inhibit request immediately,
// which removes the Request object. We need to track this to avoid calling
// Close on a non-existent object.
func (p *PortalInhibitor) watchForResponse(ctx context.Context, handlePath dbus.ObjectPath) {
	log := logging.FromContext(ctx)

	if p.conn == nil {
		return
	}

	// Add match rule for the Response signal on this specific request
	matchRule := fmt.Sprintf(
		"type='signal',interface='%s',member='Response',path='%s'",
		requestIface, handlePath,
	)

	if err := p.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		log.Debug().Err(err).Msg("idle inhibitor: failed to add signal match")
		return
	}

	// Create a channel to receive signals
	signals := make(chan *dbus.Signal, 1)
	p.conn.Signal(signals)

	defer func() {
		p.conn.RemoveSignal(signals)
		_ = p.conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, matchRule).Err
	}()

	// Wait for either a Response signal or context cancellation
	for {
		select {
		case sig := <-signals:
			if sig == nil {
				return
			}
			if sig.Path == handlePath && sig.Name == requestIface+".Response" {
				p.mu.Lock()
				p.requestComplete = true
				p.mu.Unlock()
				log.Debug().
					Str("handle", string(handlePath)).
					Msg("idle inhibitor: request completed by portal")
				return
			}
		case <-ctx.Done():
			return
		}
	}
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

	// If the portal already completed the request with a Response signal,
	// the Request object no longer exists - don't try to close it
	if p.requestComplete {
		log.Info().Msg("idle inhibitor: deactivated (completed by portal)")
		p.requestPath = ""
		return nil
	}

	// Close the request to release inhibition
	obj := p.conn.Object(portalDest, p.requestPath)
	_ = obj.Call(requestIface+".Close", 0).Err

	log.Info().Msg("idle inhibitor: deactivated")
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

	// Release any active inhibition (only if not already completed by portal)
	if p.conn != nil && p.requestPath != "" && !p.requestComplete {
		obj := p.conn.Object(portalDest, p.requestPath)
		_ = obj.Call(requestIface+".Close", 0).Err
	}
	p.requestPath = ""
	p.requestComplete = false
	p.refcount = 0

	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		return err
	}

	return nil
}
