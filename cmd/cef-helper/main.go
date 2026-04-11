// cef-helper is a minimal subprocess binary for CEF.
//
// CEF spawns renderer, GPU, and utility subprocesses by re-executing a binary
// with --type=renderer/gpu-process/utility. In Go, the main dumber binary has
// heavy initialization (config, DB, GTK) that corrupts process state before
// CEF gets control. This dedicated helper binary calls cef_execute_process as
// the very first thing and exits, avoiding that problem.
//
// Usage: set Settings.BrowserSubprocessPath to the path of this binary.
package main

import (
	"os"
	"runtime"

	cef "github.com/bnema/purego-cef/cef"
)

func main() {
	// Pin to the main OS thread — CEF requires it.
	runtime.LockOSThread()

	// cef_execute_process will handle the subprocess lifecycle and call
	// os.Exit internally if this is a valid CEF subprocess invocation.
	// If it returns (code < 0), this is not a subprocess — shouldn't
	// happen in normal operation since CEF only launches this binary
	// with --type= flags.
	cef.MaybeExitSubprocess()

	// If we get here, something went wrong.
	os.Exit(1)
}
