package gtkgl

import (
	"errors"

	"github.com/bnema/puregotk/v4/glib"
)

// ErrIdleScheduleFailed is returned by RunOnGTKThreadSync when scheduling the
// idle callback fails (glib.IdleAddOnce returned source ID 0).
var ErrIdleScheduleFailed = errors.New("gtk idle schedule failed")

// idleAddOnce is the scheduling function used by RunOnGTKThreadSync. It is a
// package-level variable so that tests can inject a failing implementation.
var idleAddOnce = glib.IdleAddOnce

// RunOnGTKThreadSync runs fn on the default GTK main context and waits for it to
// finish. If the current thread already owns that context, fn runs inline.
// It returns ErrIdleScheduleFailed if the idle callback could not be scheduled.
func RunOnGTKThreadSync(fn func()) error {
	if fn == nil {
		return nil
	}
	ctx := glib.MainContextDefault()
	if ctx != nil && ctx.IsOwner() {
		fn()
		return nil
	}
	done := make(chan struct{})
	cb := glib.SourceOnceFunc(func(uintptr) {
		defer close(done)
		fn()
	})
	if id := idleAddOnce(&cb, 0); id == 0 {
		return ErrIdleScheduleFailed
	}
	<-done
	return nil
}
