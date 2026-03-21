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

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
)

const puregoCEFInitTraceEnvVar = "PUREGO_CEF_INIT_TRACE"

// NewEngine initializes the CEF runtime and returns a ready-to-use Engine.
func NewEngine(ctx context.Context, cfg config.CEFEngineConfig) (*Engine, error) {
	logger := logging.FromContext(ctx)
	cleanStaleSingletonLocks(logger)

	multiThreaded := cfg.CEFMultiThreadedMessageLoop()
	manualPumpMs := cfg.CEFManualPumpIntervalMs()
	windowlessFrameRate := cfg.CEFWindowlessFrameRate()
	purecef.SetHandlerTraceEnabled(cfg.TraceHandlers)

	settings := prepareCEFSettings(cfg, logger)

	// Inject --no-zygote temporarily for cef_initialize, then restore.
	savedArgs := os.Args
	os.Args = appendIfMissing(os.Args, "--no-zygote")

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

	if err := initializeCEF(eng, settings, multiThreaded, logger); err != nil {
		os.Args = savedArgs
		return nil, err
	}
	os.Args = savedArgs

	return wireEngine(ctx, eng, cfg, windowlessFrameRate, logger)
}

// prepareCEFSettings builds purecef.Settings from the engine config.
func prepareCEFSettings(cfg config.CEFEngineConfig, logger *zerolog.Logger) purecef.Settings {
	settings := purecef.DefaultSettings()
	settings.MultiThreadedMessageLoop = cfg.CEFMultiThreadedMessageLoop()
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
	if helperPath := findHelperBinary(); helperPath != "" {
		settings.BrowserSubprocessPath = helperPath
		logger.Debug().Str("path", helperPath).Msg("cef: using subprocess helper")
	} else {
		logger.Warn().Msg("cef: subprocess helper not found, falling back to main binary")
	}
	return settings
}

// initializeCEF calls cef_initialize with or without an App depending on pump mode.
func initializeCEF(eng *Engine, settings purecef.Settings, multiThreaded bool, logger *zerolog.Logger) error {
	if multiThreaded {
		logger.Debug().Msg("cef: calling Init")
		if err := purecef.Init(settings); err != nil {
			return fmt.Errorf("cef.Init: %w", err)
		}
		logger.Debug().Msg("cef: Init returned OK")
	} else {
		app := newDumberApp(eng)
		logger.Debug().Msg("cef: calling InitWithApp")
		if err := purecef.InitWithApp(settings, app); err != nil {
			return fmt.Errorf("cef.InitWithApp: %w", err)
		}
		logger.Debug().Msg("cef: InitWithApp returned OK")
	}
	return nil
}

// wireEngine creates GL loader, factory, pool, and scheme handler after CEF init.
func wireEngine(
	ctx context.Context, eng *Engine, cfg config.CEFEngineConfig,
	windowlessFrameRate int32, logger *zerolog.Logger,
) (*Engine, error) {
	gl, err := newGLLoader()
	if err != nil {
		purecef.Shutdown()
		return nil, fmt.Errorf("GL loader: %w", err)
	}

	scale := detectHiDPIScale(logger)

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
	eng.contentInj = newContentInjector(eng, nil)

	messageRouter := NewMessageRouter(ctx)
	schemeHandler := newDumbSchemeHandler(ctx, messageRouter)
	schemeHandler.setAssets(assets.WebUIAssets)

	result := purecef.RegisterSchemeHandlerFactory("dumb", "", purecef.NewSchemeHandlerFactory(schemeHandler))
	logger.Info().Int32("result", result).Msg("cef: registered dumb:// scheme handler factory")

	eng.messageRouter = messageRouter
	eng.schemeHandler = schemeHandler

	return eng, nil
}

// detectHiDPIScale queries the primary GDK monitor for its scale factor.
func detectHiDPIScale(logger *zerolog.Logger) int32 {
	display := gdk.DisplayGetDefault()
	if display == nil {
		logger.Debug().Msg("cef: no GDK display available, using scale=1")
		return 1
	}
	monitors := display.GetMonitors()
	if monitors == nil {
		logger.Debug().Msg("cef: no monitors found, using scale=1")
		return 1
	}
	obj := monitors.GetObject(0)
	if obj == nil {
		logger.Debug().Msg("cef: primary monitor not available, using scale=1")
		return 1
	}
	mon := &gdk.Monitor{}
	mon.SetGoPointer(obj.GoPointer())
	if s := mon.GetScaleFactor(); s > 0 {
		logger.Info().Int32("scale", int32(s)).Msg("cef: detected HiDPI scale from monitor")
		return int32(s)
	}
	logger.Debug().Msg("cef: monitor scale factor <= 0, using scale=1")
	return 1
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
