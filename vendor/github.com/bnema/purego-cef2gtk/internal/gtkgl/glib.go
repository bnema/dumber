package gtkgl

import (
	"fmt"

	"github.com/bnema/puregotk/v4/glib"
)

func glibErrorMessage(err *glib.Error) string {
	if err == nil {
		return "unknown GTK/GLib error"
	}
	return fmt.Sprintf("domain=%d code=%d message=%q", err.Domain, err.Code, err.MessageGo())
}
