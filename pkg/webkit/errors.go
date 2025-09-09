// Package webkit provides a WebKit2GTK-backed browser view and window.
package webkit

import "errors"

// ErrNotImplemented is returned by stub implementations pending WebKit2GTK migration.
var ErrNotImplemented = errors.New("webkit: not implemented")
