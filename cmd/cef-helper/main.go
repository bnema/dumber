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

	infracef "github.com/bnema/dumber/internal/infrastructure/cef"
	cef "github.com/bnema/purego-cef/cef"
)

func main() {
	// Pin to the main OS thread — CEF requires it.
	runtime.LockOSThread()

	// cef_execute_process will handle the subprocess lifecycle and call
	// os.Exit internally if this is a valid CEF subprocess invocation.
	// We pass the lightweight subprocess app, which intentionally returns nil
	// from GetRenderProcessHandler(), so renderer-side bridges remain disabled
	// in helper subprocesses while the OSR startup regression is under repair.
	cef.MaybeExitSubprocessWithApp(infracef.NewSubprocessApp())

	// If we get here, something went wrong.
	os.Exit(1)
}
