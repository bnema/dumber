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

const puregoCEFInitTraceEnvVar = "PUREGO_CEF_INIT_TRACE"

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
	windowlessFrameRate := cfg.CEFWindowlessFrameRate()

	purecef.SetHandlerTraceEnabled(cfg.TraceHandlers)

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
	if runtimeLogFile, err := prepareCEFLogFile(cfg); err != nil {
		logger.Warn().Err(err).Msg("cef: failed to prepare runtime log file")
	} else {
		settings.LogFile = runtimeLogFile
	}
	if bootstrapLogFile, err := prepareCEFInitTraceFile(cfg); err != nil {
		logger.Warn().Err(err).Msg("cef: failed to prepare init trace file")
	} else if bootstrapLogFile != "" {
		settings.InitTraceFile = bootstrapLogFile
		logger.Info().
			Str("bootstrap_log_file", bootstrapLogFile).
			Str("env_var", puregoCEFInitTraceEnvVar).
			Msg("cef: init bootstrap diagnostics enabled")
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
		Int32("windowless_frame_rate", windowlessFrameRate).
		Bool("trace_handlers", cfg.TraceHandlers).
		Bool("enable_audio_handler", cfg.EnableAudioHandler).
		Bool("enable_context_menu_handler", cfg.EnableContextMenuHandler).
		Msg("cef: configured engine")

	if multiThreaded {
		// In multi-threaded message loop mode CEF owns the browser UI thread, so
		// Dumber does not rely on BrowserProcessHandler callbacks to drive the
		// message pump. Initializing without a CefApp avoids the current
		// purego-cef app bridge issue that causes cef_initialize() to fail early.
		logger.Debug().Msg("cef: calling Init")
		if err := purecef.Init(settings); err != nil {
			os.Args = savedArgs
			return nil, fmt.Errorf("cef.Init: %w", err)
		}
		os.Args = savedArgs
		logger.Debug().Msg("cef: Init returned OK")
	} else {
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
	}

	// 3. Load GL.
	gl, err := newGLLoader()
	if err != nil {
		purecef.Shutdown()
		return nil, fmt.Errorf("GL loader: %w", err)
	}

	// 4. Detect HiDPI scale from the primary monitor.
	scale := int32(1)
	if display := gdk.DisplayGetDefault(); display == nil {
		logger.Debug().Msg("cef: no GDK display available, using scale=1")
	} else if monitors := display.GetMonitors(); monitors == nil {
		logger.Debug().Msg("cef: no monitors found, using scale=1")
	} else if obj := monitors.GetObject(0); obj == nil {
		logger.Debug().Msg("cef: primary monitor not available, using scale=1")
	} else {
		mon := &gdk.Monitor{}
		mon.SetGoPointer(obj.GoPointer())
		if s := mon.GetScaleFactor(); s > 0 {
			scale = int32(s)
			logger.Info().Int32("scale", scale).Msg("cef: detected HiDPI scale from monitor")
		} else {
			logger.Debug().Msg("cef: monitor scale factor <= 0, using scale=1")
		}
	}

	// 5. Create factory + pool and wire them into the engine.
	factory := newWebViewFactory(eng, gl, webViewFactoryOptions{
		scale:                    scale,
		windowlessFrameRate:      windowlessFrameRate,
		enableAudioHandler:       cfg.EnableAudioHandler,
		enableContextMenuHandler: cfg.EnableContextMenuHandler,
	})
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

// cleanStaleSingletonLocks removes CEF's SingletonLock/Socket/Cookie files
// if the owning process is no longer running. CEF leaves these behind on
// unclean shutdown (SIGKILL, SIGSEGV) and the next instance crashes trying
// to connect to the dead process.
func cleanStaleSingletonLocks(logger *zerolog.Logger) {
	// CEF defaults to ~/.config/cef_user_data when no root_cache_path is set.
	dir := filepath.Clean(filepath.Join(os.Getenv("HOME"), ".config", "cef_user_data"))

	lockPath := filepath.Join(dir, "SingletonLock")
	target, err := os.Readlink(lockPath)
	if err != nil {
		return // No lock file or not a symlink — nothing to clean.
	}

	// SingletonLock is a symlink to "hostname-pid". Split from the right
	// because hostnames may contain hyphens.
	lastDash := strings.LastIndex(target, "-")
	if lastDash < 0 || lastDash == len(target)-1 {
		return
	}
	pid, err := strconv.Atoi(target[lastDash+1:])
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

// findHelperBinary looks for the cef-helper binary next to the running
// executable. Returns empty string if not found.
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

const (
	dirPerm  = 0o755
	filePerm = 0o644
)

func prepareCEFLogFile(cfg config.CEFEngineConfig) (string, error) {
	if cfg.LogFile != "" {
		runtimeLogFile := filepath.Clean(cfg.LogFile)
		if err := os.MkdirAll(filepath.Dir(runtimeLogFile), dirPerm); err != nil {
			return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(runtimeLogFile), err)
		}
		return runtimeLogFile, nil
	}

	// Dev-mode default: logs go into .dev/dumber/logs/ relative to CWD.
	// Production deployments should set cfg.LogFile explicitly.
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		return "", fmt.Errorf("getwd: %w", cwdErr)
	}
	runtimeLogFile := filepath.Join(cwd, ".dev", "dumber", "logs", "cef_runtime.log")
	if err := os.MkdirAll(filepath.Dir(runtimeLogFile), dirPerm); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(runtimeLogFile), err)
	}
	return runtimeLogFile, nil
}

func prepareCEFInitTraceFile(cfg config.CEFEngineConfig) (string, error) {
	if !puregoCEFInitTraceEnabled() {
		return "", nil
	}

	runtimeLogFile := cfg.LogFile
	if runtimeLogFile != "" {
		runtimeLogFile = filepath.Clean(runtimeLogFile)
	} else {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return "", fmt.Errorf("getwd: %w", cwdErr)
		}
		runtimeLogFile = filepath.Join(cwd, ".dev", "dumber", "logs", "cef_runtime.log")
	}
	bootstrapLogFile := strings.TrimSuffix(runtimeLogFile, filepath.Ext(runtimeLogFile)) + ".bootstrap.log"
	if err := os.MkdirAll(filepath.Dir(bootstrapLogFile), dirPerm); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(bootstrapLogFile), err)
	}
	if err := os.WriteFile(bootstrapLogFile, nil, filePerm); err != nil {
		return "", fmt.Errorf("reset %s: %w", bootstrapLogFile, err)
	}
	return bootstrapLogFile, nil
}

func puregoCEFInitTraceEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(puregoCEFInitTraceEnvVar))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
