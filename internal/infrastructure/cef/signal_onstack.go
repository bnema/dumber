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

// signalHandlerState is the subset of a kernel sigaction needed to decide
// whether a handler must be repaired.
type signalHandlerState struct {
	Signal  int
	Handler uintptr
	Flags   uint64
}

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

// signalsNeedingOnStack returns the states whose handler is a real function
// (not SIG_DFL/SIG_IGN) installed without SA_ONSTACK. Go requires non-Go
// handlers to run on the alternate signal stack; otherwise the kernel writes
// the signal frame onto a small goroutine stack and corrupts memory.
func signalsNeedingOnStack(states []signalHandlerState) []signalHandlerState {
	var out []signalHandlerState
	for _, st := range states {
		if st.Handler == sigHandlerDefault || st.Handler == sigHandlerIgnore {
			continue
		}
		if st.Flags&saOnStackFlag != 0 {
			continue
		}
		out = append(out, signalHandlerState{Signal: st.Signal, Handler: st.Handler, Flags: st.Flags | saOnStackFlag})
	}
	return out
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
