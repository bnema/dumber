package webkit

import (
	"os"
	"runtime"
	"strconv"

	"bnema.dev/puregotk/v4/javascriptcore"
	"golang.org/x/sys/unix"

	"bnema.dev/dumber/internal/infrastructure/sigstack"
)

// installJSCSignalHandlersOnStack makes JavaScriptCore install its UI-process
// signal handlers now, then adds SA_ONSTACK to them.
//
// JSC installs its SIGSEGV/SIGBUS handler (WTF::SignalHandlers::finalize) once,
// the first time a JS global context is created in the process; WebKit does
// that during the first page load. The handler is installed with SA_SIGINFO
// only and forwards faults it does not own straight to the previous handler,
// which is Go's. Without SA_ONSTACK that call runs Go's handler off the signal
// stack, so any fault in the UI process (including a recoverable Go nil
// dereference in a WebKit callback) aborts with "non-Go code set up signal
// handler without SA_ONSTACK flag" instead of panicking.
//
// Creating a throwaway context triggers the one-time install at a known point
// so a single repair covers it.
//
// The thread suspend/resume signal (JSC_SIGNAL_FOR_GC, SIGUSR1 by default) is
// left alone: WTF's handler reads the interrupted thread's registers and
// backs off when it runs on an alternate stack, so SA_ONSTACK would stop the
// JSC garbage collector from ever suspending a thread.
func installJSCSignalHandlersOnStack() sigstack.Result {
	runtime.LockOSThread()
	if jsc := javascriptcore.NewContext(); jsc != nil {
		jsc.Unref()
	}
	runtime.UnlockOSThread()
	return sigstack.Repair(jscSuspendResumeSignal(os.Getenv("JSC_SIGNAL_FOR_GC")))
}

// jscSuspendResumeSignal mirrors WTF's parsing of JSC_SIGNAL_FOR_GC
// (sscanf "%d", default SIGUSR1).
func jscSuspendResumeSignal(env string) int {
	if v, err := strconv.Atoi(leadingInt(env)); err == nil {
		return v
	}
	return int(unix.SIGUSR1)
}

// leadingInt returns the optional sign and decimal digits at the start of s,
// after leading spaces, like sscanf's %d.
func leadingInt(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
		i++
	}
	start := i
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[start:i]
}
