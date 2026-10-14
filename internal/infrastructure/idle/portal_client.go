package idle

import (
	"fmt"
	"strings"

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

// portalClient is the XDG Desktop Portal Inhibit API used by PortalInhibitor.
type portalClient interface {
	// Version returns the Inhibit portal version; it fails when no portal
	// answers.
	Version() (uint32, error)
	// Inhibit asks the portal to inhibit idle and suspend and returns the
	// request handle.
	Inhibit(reason string) (string, error)
	// CloseRequest ends the inhibit request with the given handle.
	CloseRequest(handle string) error
	// Close releases the bus connection.
	Close() error
}

// zerobusPortal implements portalClient over a zerobus connection.
// It is not safe for concurrent use.
type zerobusPortal struct {
	conn *zerobus.Conn
}

func (z *zerobusPortal) Version() (uint32, error) {
	e := z.conn.NewCall(portalDest, portalPath, propertiesIface, "Get", "ss")
	e.Str(portalInterface)
	e.Str("version")
	m, err := z.conn.Call()
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

// Inhibit calls Inhibit(window: s, flags: u, options: a{sv}) -> handle: o.
func (z *zerobusPortal) Inhibit(reason string) (string, error) {
	e := z.conn.NewCall(portalDest, portalPath, portalInterface, "Inhibit", "sua{sv}")
	e.Str("")                        // window identifier (empty for non-sandboxed)
	e.Uint32(flagIdle | flagSuspend) // inhibit idle and suspend
	options := e.BeginArray('{')
	e.Struct()
	e.Str("reason")
	e.Variant("s")
	e.Str(reason)
	e.EndArray(options)

	m, err := z.conn.Call()
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

func (z *zerobusPortal) CloseRequest(handle string) error {
	z.conn.NewCall(portalDest, handle, requestIface, "Close", "")
	_, err := z.conn.Call()
	return err
}

func (z *zerobusPortal) Close() error {
	return z.conn.Close()
}
