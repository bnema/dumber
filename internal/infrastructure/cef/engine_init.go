package cef

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
)

// NewEngine initializes the CEF runtime and returns a ready-to-use Engine.
func NewEngine(ctx context.Context, cfg config.CEFEngineConfig) (*Engine, error) {
	logger := logging.FromContext(ctx)

	// Clean up stale CEF singleton locks from previous unclean shutdowns.
	// When the process is killed (Ctrl+C, SIGSEGV), CEF leaves SingletonLock
	// files that cause the next instance to crash on startup.
	cleanStaleSingletonLocks(logger)

	// Resolve pump mode from config (with defaults).
	multiThreaded := cfg.CEFMultiThreadedMessageLoop()
	manualPumpMs := cfg.CEFManualPumpIntervalMs()

	// 1. Initialize CEF.
	settings := purecef.DefaultSettings()
	settings.MultiThreadedMessageLoop = multiThreaded
	// External message pump is disabled: purego-cef has a bug where
	// OnScheduleMessagePumpWork is never called after cef_initialize
	// (double-wrapping of BrowserProcessHandler in NewApp).
	// When multi-threaded is false, we use a manual glib timer instead.
	settings.ExternalMessagePump = false
	if cfg.CEFDir != "" {
		settings.CEFDir = cfg.CEFDir
	}
	if cfg.LogSeverity != 0 {
		settings.LogSeverity = cfg.LogSeverity
	}

	// NOTE: MaybeExitSubprocess is called in main() before Cobra, so that
	// CEF subprocess args (--type=renderer) are handled before arg stripping.

	// Use a dedicated helper binary for CEF subprocesses (renderer, GPU,
	// utility). The main dumber binary has heavy Go initialization that
	// corrupts process state before CEF gets control. The helper binary
	// calls cef_execute_process immediately and exits cleanly.
	helperPath := findHelperBinary()
	if helperPath != "" {
		settings.BrowserSubprocessPath = helperPath
		logger.Debug().Str("path", helperPath).Msg("cef: using subprocess helper")
	} else {
		logger.Warn().Msg("cef: subprocess helper not found, falling back to main binary")
	}

	// Chromium flags injected temporarily into os.Args for cef_initialize,
	// then restored so GTK doesn't choke on unknown flags.
	//
	// --no-zygote: Disable the zygote process which uses fork(). Go's runtime
	//   does not survive fork cleanly, causing renderer crashes. Without zygote,
	//   CEF launches each subprocess as a fresh exec of the helper binary.
	//
	// Thread-safety: safe because this runs during single-threaded startup
	// before any concurrent goroutines are spawned.
	savedArgs := os.Args
	os.Args = appendIfMissing(os.Args, "--no-zygote")

	// 2. Build the engine early so the App can reference it for the pump callback.
	eng := &Engine{
		ctx:                      ctx,
		multiThreadedMessageLoop: multiThreaded,
		manualPumpInterval:       manualPumpMs,
	}

	logger.Info().
		Bool("multi_threaded_message_loop", multiThreaded).
		Int64("manual_pump_interval_ms", manualPumpMs).
		Msg("cef: configured message pump mode")

	// Create the CEF App with a BrowserProcessHandler that drives the
	// adaptive message pump via OnScheduleMessagePumpWork.
	app := newDumberApp(eng)

	logger.Debug().Msg("cef: calling InitWithApp")
	if err := purecef.InitWithApp(settings, app); err != nil {
		os.Args = savedArgs
		return nil, fmt.Errorf("cef.InitWithApp: %w", err)
	}
	os.Args = savedArgs
	logger.Debug().Msg("cef: InitWithApp returned OK")

	// 3. Load GL.
	gl, err := newGLLoader()
	if err != nil {
		purecef.Shutdown()
		return nil, fmt.Errorf("GL loader: %w", err)
	}

	// 4. Detect HiDPI scale from the primary monitor.
	scale := int32(1)
	if display := gdk.DisplayGetDefault(); display != nil {
		if monitors := display.GetMonitors(); monitors != nil {
			obj := monitors.GetObject(0)
			if obj != nil {
				mon := &gdk.Monitor{}
				mon.SetGoPointer(obj.GoPointer())
				if s := mon.GetScaleFactor(); s > 0 {
					scale = int32(s)
					logger.Info().Int32("scale", scale).Msg("cef: detected HiDPI scale from monitor")
				}
			}
		}
	}

	// 5. Create factory + pool and wire them into the engine.
	factory := newWebViewFactory(eng, gl, scale)
	pool := newWebViewPool(factory)

	eng.gl = gl
	eng.factory = factory
	eng.pool = pool

	return eng, nil
}

// appendIfMissing appends flag to args if it's not already present.
func appendIfMissing(args []string, flag string) []string {
	for _, a := range args {
		if a == flag {
			return args
		}
	}
	return append(args, flag)
}

// findHelperBinary looks for the cef-helper binary next to the running
// executable. Returns empty string if not found.
// cleanStaleSingletonLocks removes CEF's SingletonLock/Socket/Cookie files
// if the owning process is no longer running. CEF leaves these behind on
// unclean shutdown (SIGKILL, SIGSEGV) and the next instance crashes trying
// to connect to the dead process.
func cleanStaleSingletonLocks(logger *zerolog.Logger) {
	// CEF defaults to ~/.config/cef_user_data when no root_cache_path is set.
	dir := filepath.Join(os.Getenv("HOME"), ".config", "cef_user_data")

	lockPath := filepath.Join(dir, "SingletonLock")
	target, err := os.Readlink(lockPath)
	if err != nil {
		return // No lock file or not a symlink — nothing to clean.
	}

	// SingletonLock is a symlink to "hostname-pid".
	parts := strings.SplitN(target, "-", 2)
	if len(parts) != 2 {
		return
	}
	pid, err := strconv.Atoi(parts[1])
	if err != nil || pid <= 0 {
		return
	}

	// Check if the process is still alive.
	proc, err := os.FindProcess(pid)
	if err == nil {
		if err := proc.Signal(syscall.Signal(0)); err == nil {
			// Process is alive — don't touch the lock.
			return
		}
	}

	// Stale lock — remove singleton files.
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		p := filepath.Join(dir, name)
		if err := os.Remove(p); err == nil {
			logger.Info().Str("path", p).Msg("cef: removed stale singleton file")
		}
	}
}

func findHelperBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	helper := filepath.Join(dir, "cef-helper")
	if _, err := os.Stat(helper); err == nil {
		return helper
	}
	return ""
}
