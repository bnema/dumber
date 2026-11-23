package cef

import (
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

// signalOnStackAuditInterval bounds how often the heartbeat re-checks signal
// handlers installed by CEF/Chromium after initialization.
const signalOnStackAuditInterval = 10 * time.Second

const (
	sigHandlerDefault uintptr = 0 // SIG_DFL
	sigHandlerIgnore  uintptr = 1 // SIG_IGN
	saOnStackFlag     uint64  = 0x08000000
)

// repairedSignal describes a handler that received SA_ONSTACK.
type repairedSignal struct {
	Signal  int
	Name    string
	Handler uintptr
}

// signalOnStackResult lists repaired handlers and per-signal failures.
type signalOnStackResult struct {
	Repaired []repairedSignal
	Errors   []error
}

// needsOnStack reports whether a handler is a real function (not SIG_DFL or
// SIG_IGN) installed without SA_ONSTACK. Go requires non-Go handlers to run on
// the alternate signal stack; otherwise the kernel writes the signal frame
// onto a small goroutine stack and corrupts memory.
func needsOnStack(handler uintptr, flags uint64) bool {
	if handler == sigHandlerDefault || handler == sigHandlerIgnore {
		return false
	}
	return flags&saOnStackFlag == 0
}

func logSignalOnStackRepair(logger *zerolog.Logger, res signalOnStackResult, debugWhenClean bool) {
	if logger == nil {
		return
	}
	for _, r := range res.Repaired {
		logger.Warn().
			Int("signal", r.Signal).
			Str("signal_name", r.Name).
			Str("handler", fmt.Sprintf("%#x", r.Handler)).
			Msg("cef: repaired signal handler missing SA_ONSTACK")
	}
	for _, err := range res.Errors {
		logger.Warn().Err(err).Msg("cef: signal handler SA_ONSTACK audit failed")
	}
	if debugWhenClean && len(res.Repaired) == 0 && len(res.Errors) == 0 {
		logger.Debug().Msg("cef: all signal handlers already use SA_ONSTACK")
	}
}
