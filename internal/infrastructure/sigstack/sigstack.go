// Package sigstack repairs signal handlers that native libraries install
// without SA_ONSTACK.
//
// Go requires every non-Go signal handler to use SA_ONSTACK. Without it the
// kernel writes the signal frame onto whatever stack the thread is using,
// which may be a small goroutine stack. When such a handler forwards the
// signal to Go's handler (as JavaScriptCore does for faults it does not own),
// the runtime aborts with "non-Go code set up signal handler without
// SA_ONSTACK flag" instead of turning a fault into a panic.
package sigstack

import (
	"fmt"

	"github.com/rs/zerolog"
)

const (
	sigHandlerDefault uintptr = 0 // SIG_DFL
	sigHandlerIgnore  uintptr = 1 // SIG_IGN
	saOnStackFlag     uint64  = 0x08000000
)

// Repaired describes a handler that received SA_ONSTACK.
type Repaired struct {
	Signal  int
	Name    string
	Handler uintptr
}

// Result lists repaired handlers and per-signal failures.
type Result struct {
	Repaired []Repaired
	Errors   []error
}

// needsOnStack reports whether a handler is a real function (not SIG_DFL or
// SIG_IGN) installed without SA_ONSTACK.
func needsOnStack(handler uintptr, flags uint64) bool {
	if handler == sigHandlerDefault || handler == sigHandlerIgnore {
		return false
	}
	return flags&saOnStackFlag == 0
}

// Log reports a repair result. source names the library that installed the
// handlers (for example "cef" or "webkit") and prefixes each message, which
// the stress harness matches; repairs are logged at level, so expected
// repairs need not warn.
func Log(logger *zerolog.Logger, source string, res Result, level zerolog.Level, debugWhenClean bool) {
	if logger == nil {
		return
	}
	for _, r := range res.Repaired {
		logger.WithLevel(level).
			Str("source", source).
			Int("signal", r.Signal).
			Str("signal_name", r.Name).
			Str("handler", fmt.Sprintf("%#x", r.Handler)).
			Msg(source + ": repaired signal handler missing SA_ONSTACK")
	}
	for _, err := range res.Errors {
		logger.Warn().Str("source", source).Err(err).Msg(source + ": signal handler SA_ONSTACK audit failed")
	}
	if debugWhenClean && len(res.Repaired) == 0 && len(res.Errors) == 0 {
		logger.Debug().Str("source", source).Msg(source + ": all signal handlers already use SA_ONSTACK")
	}
}
