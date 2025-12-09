package webkit

/*
#cgo pkg-config: webkitgtk-6.0
#include <webkit/webkit.h>
*/
import "C"
import (
	"fmt"
	"unsafe"

	"github.com/bnema/dumber/internal/logging"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/core/gextras"

// RefUserContentFilter is now a no-op as we rely on Go GC and persistent wrappers
func RefUserContentFilter(filter *webkit.UserContentFilter) {
}

// UnrefUserContentFilter is now a no-op as we rely on Go GC and persistent wrappers
func UnrefUserContentFilter(filter *webkit.UserContentFilter) {
}
