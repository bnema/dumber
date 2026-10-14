// Package idle provides system idle/screensaver inhibition via XDG Desktop Portal.
package idle

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/zerobus"
)

// Compile-time interface check.
var _ port.IdleInhibitor = (*PortalInhibitor)(nil)

// PortalInhibitor implements idle inhibition using XDG Desktop Portal.
// This works on Wayland with any compositor (GNOME, KDE, sway, hyprland, etc.).
//
// Every portal call runs under mu because the client is not safe for
// concurrent use.
type PortalInhibitor struct {
	mu          sync.Mutex
	portal      portalClient // nil when the portal is unavailable or after Close
	requestPath string       // active inhibit request handle
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
	return newPortalInhibitor(ctx, &zerobusPortal{conn: conn})
}

// newPortalInhibitor takes ownership of portal and keeps it only if it
// answers.
func newPortalInhibitor(ctx context.Context, portal portalClient) *PortalInhibitor {
	log := logging.FromContext(ctx)

	version, err := portal.Version()
	if err != nil {
		log.Debug().Err(err).Msg("idle inhibitor: portal not available")
		_ = portal.Close()
		return &PortalInhibitor{}
	}

	log.Debug().Uint32("version", version).Msg("idle inhibitor: portal available")
	return &PortalInhibitor{portal: portal}
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

	if p.portal == nil {
		log.Debug().Msg("idle inhibitor: not supported, skipping")
		return nil
	}

	handle, err := p.portal.Inhibit(reason)
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
	if p.portal != nil && p.requestPath != "" {
		_ = p.portal.CloseRequest(p.requestPath)
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

	if p.portal == nil {
		return nil
	}
	err := p.portal.Close()
	p.portal = nil
	return err
}
