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
	"github.com/bnema/puregotk/v4/glib"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	transcoderpkg "github.com/bnema/dumber/internal/infrastructure/transcoder"
	"github.com/bnema/dumber/internal/logging"
)

const puregoCEFInitTraceEnvVar = "PUREGO_CEF_INIT_TRACE"

// NewEngine initializes the CEF runtime and returns a ready-to-use Engine.
func NewEngine(
	ctx context.Context,
	opts port.EngineOptions,
	cfg RuntimeConfig,
	transcodingCfg TranscodingRuntimeConfig,
	audioFactory port.AudioOutputFactory,
	deps EngineDependencies,
) (*Engine, error) {
	logger := logging.FromContext(ctx)
	stateRoot := resolvedStateRoot(opts)
	cleanStaleSingletonLocks(logger, stateRoot)

	legacyCfg := config.CEFEngineConfig{
		CEFDir:                   cfg.CEFDir,
		LogFile:                  cfg.LogFile,
		LogSeverity:              cfg.LogSeverity,
		WindowlessFrameRate:      cfg.WindowlessFrameRate,
		EnableAudioHandler:       cfg.EnableAudioHandler,
		EnableContextMenuHandler: cfg.EnableContextMenuHandler,
		TraceHandlers:            cfg.TraceHandlers,
	}
	legacyTranscodingCfg := config.TranscodingConfig{
		Enabled:       transcodingCfg.Enabled,
		HWAccel:       transcodingCfg.HWAccel,
		MaxConcurrent: transcodingCfg.MaxConcurrent,
		Quality:       transcodingCfg.Quality,
	}
	windowlessFrameRate := legacyCfg.CEFWindowlessFrameRate()

	settings, err := prepareCEFSettings(opts, cfg, logger)
	if err != nil {
		return nil, err
	}

	// Inject --no-zygote temporarily for cef_initialize, then restore.
	// Safe: runs during single-threaded startup before concurrent goroutines.
	savedArgs := os.Args
	os.Args = appendIfMissing(os.Args, "--no-zygote")

	eng := &Engine{
		ctx: ctx,
	}

	logger.Info().
		Int32("windowless_frame_rate", windowlessFrameRate).
		Bool("external_begin_frame", externalBeginFrameEnabled()).
		Bool("trace_handlers", cfg.TraceHandlers).
		Bool("enable_audio_handler", cfg.EnableAudioHandler).
		Bool("enable_context_menu_handler", cfg.EnableContextMenuHandler).
		Msg("cef: configured engine")

	if err := initializeCEF(eng, settings, logger); err != nil {
		os.Args = savedArgs
		return nil, err
	}
	os.Args = savedArgs

	return wireEngine(ctx, eng, legacyCfg, legacyTranscodingCfg, windowlessFrameRate, audioFactory, logger)
}

func resolvedStateRoot(opts port.EngineOptions) string {
	switch {
	case opts.DataDir != "":
		return opts.DataDir
	case opts.CacheDir != "":
		return opts.CacheDir
	default:
		return defaultCEFUserDataDir()
	}
}

// prepareCEFSettings builds purecef.Settings from the engine config.
func prepareCEFSettings(opts port.EngineOptions, cfg RuntimeConfig, logger *zerolog.Logger) (purecef.Settings, error) {
	if opts.CookiePolicy != "" && opts.CookiePolicy != port.CookiePolicyAlways {
		return purecef.Settings{}, fmt.Errorf("%w: %s", ErrCookiePolicyUnsupported, opts.CookiePolicy)
	}

	legacyCfg := config.CEFEngineConfig{
		CEFDir:                   cfg.CEFDir,
		LogFile:                  cfg.LogFile,
		LogSeverity:              cfg.LogSeverity,
		WindowlessFrameRate:      cfg.WindowlessFrameRate,
		EnableAudioHandler:       cfg.EnableAudioHandler,
		EnableContextMenuHandler: cfg.EnableContextMenuHandler,
		TraceHandlers:            cfg.TraceHandlers,
	}

	settings := purecef.DefaultSettings()
	settings.MultiThreadedMessageLoop = true
	settings.ExternalMessagePump = false
	settings.RootCachePath = resolvedStateRoot(opts)
	if legacyCfg.CEFDir != "" {
		settings.CEFDir = legacyCfg.CEFDir
	}
	if runtimeLogFile, err := prepareCEFLogFile(legacyCfg); err != nil {
		logger.Warn().Err(err).Msg("cef: failed to prepare runtime log file")
	} else {
		settings.LogFile = runtimeLogFile
	}
	if bootstrapLogFile, err := prepareCEFInitTraceFile(legacyCfg); err != nil {
		logger.Warn().Err(err).Msg("cef: failed to prepare init trace file")
	} else if bootstrapLogFile != "" {
		settings.InitTraceFile = bootstrapLogFile
		logger.Info().
			Str("bootstrap_log_file", bootstrapLogFile).
			Str("env_var", puregoCEFInitTraceEnvVar).
			Msg("cef: init bootstrap diagnostics enabled")
	}
	if legacyCfg.LogSeverity != 0 {
		settings.LogSeverity = legacyCfg.LogSeverity
	}
	if helperPath := findHelperBinary(); helperPath != "" {
		settings.BrowserSubprocessPath = helperPath
		logger.Debug().Str("path", helperPath).Msg("cef: using subprocess helper")
	} else {
		logger.Warn().Msg("cef: subprocess helper not found, falling back to main binary")
	}
	return settings, nil
}

func defaultCEFUserDataDir() string {
	// Use Dumber's XDG helpers to get the config directory
	dirs, err := config.GetXDGDirs()
	if err != nil {
		// Fallback to HOME-based path if XDG resolution fails
		return filepath.Clean(filepath.Join(os.Getenv("HOME"), ".config", "cef_user_data"))
	}
	if os.Getenv("ENV") == "dev" {
		return filepath.Join(dirs.ConfigHome, "cef_user_data")
	}
	return filepath.Join(filepath.Dir(dirs.ConfigHome), "cef_user_data")
}

// initializeCEF calls cef_initialize with the App to register custom schemes
// and browser-process callbacks (OnBeforeCommandLineProcessing, etc.).
func initializeCEF(eng *Engine, settings purecef.Settings, logger *zerolog.Logger) error {
	app := newDumberApp(eng)
	logger.Debug().Msg("cef: calling InitWithApp")
	if err := purecef.InitWithApp(settings, app); err != nil {
		return fmt.Errorf("cef.InitWithApp: %w", err)
	}
	logger.Debug().Msg("cef: InitWithApp returned OK")
	return nil
}

// wireEngine creates GL loader, factory, pool, and scheme handler after CEF init.
func wireEngine(
	ctx context.Context, eng *Engine, cfg config.CEFEngineConfig, transcodingCfg config.TranscodingConfig,
	windowlessFrameRate int32, audioFactory port.AudioOutputFactory, logger *zerolog.Logger,
) (*Engine, error) {
	gl, err := newGLLoader()
	if err != nil {
		purecef.Shutdown()
		return nil, fmt.Errorf("GL loader: %w", err)
	}

	scale := detectHiDPIScale(logger)

	// Initialize the GPU transcoder when enabled in the loaded app config.
	var mediaTranscoder port.MediaTranscoder
	transcoderState := buildTranscoderStartupState(transcodingCfg)
	if transcodingCfg.Enabled {
		transcoderState.ProbeAttempted = true
		tc := transcoderpkg.New(transcodingCfg, logger)
		caps := tc.Capabilities()
		transcoderState.API = caps.API
		transcoderState.Encoders = append([]string(nil), caps.Encoders...)
		transcoderState.Decoders = append([]string(nil), caps.Decoders...)
		if tc.Available() {
			mediaTranscoder = tc
			transcoderState.Status = "available"
			logger.Info().
				Str("api", tc.Capabilities().API).
				Strs("encoders", tc.Capabilities().Encoders).
				Msg("cef: GPU transcoding available")
		} else {
			transcoderState.Status = "unavailable_no_compatible_gpu"
			logger.Warn().Msg("cef: GPU transcoding enabled but no compatible GPU found — feature disabled")
		}
	}

	factory := newWebViewFactory(eng, gl, webViewFactoryOptions{
		scale:                    scale,
		windowlessFrameRate:      windowlessFrameRate,
		enableContextMenuHandler: cfg.EnableContextMenuHandler,
		transcoder:               mediaTranscoder,
		audioOutputFactory:       audioFactory,
	})
	pool := newWebViewPool(factory)

	eng.gl = gl
	eng.factory = factory
	eng.pool = pool
	eng.contentInj = newContentInjector(eng, nil)
	eng.transcoderState = transcoderState

	messageRouter := NewMessageRouter(ctx)
	schemeHandler := newDumbSchemeHandler(ctx, messageRouter, mediaTranscoder)
	schemeHandler.setAssets(assets.WebUIAssets)

	// Bridge clipboard writes from CEF JS → GDK system clipboard.
	// The callback is invoked on the CEF IO thread, so we schedule the
	// GDK write on the GTK main loop via glib.IdleAddOnce.
	schemeHandler.onClipboardSet = func(text string) {
		fn := glib.SourceOnceFunc(func(_ uintptr) {
			if display := gdk.DisplayGetDefault(); display != nil {
				if cb := display.GetClipboard(); cb != nil {
					cb.SetText(text)
					logger.Debug().Msg("cef: clipboard set via GDK")
				} else {
					logger.Debug().Msg("cef: clipboard set failed — no GDK clipboard")
				}
			} else {
				logger.Debug().Msg("cef: clipboard set failed — no GDK display")
			}
		})
		glib.IdleAddOnce(&fn, 0)
	}

	schemeFactory := purecef.NewSchemeHandlerFactory(schemeHandler)

	result := purecef.RegisterSchemeHandlerFactory("dumb", "", schemeFactory)
	if result != 1 {
		logger.Error().Int32("result", result).Msg("cef: failed to register dumb:// scheme handler factory")
	} else {
		logger.Info().Msg("cef: registered dumb:// scheme handler factory")
	}

	result = purecef.RegisterSchemeHandlerFactory(actualInternalScheme, actualInternalHost, schemeFactory)
	if result != 1 {
		logger.Error().
			Int32("result", result).
			Str("origin", actualInternalOrigin).
			Msg("cef: failed to register internal https handler factory")
	} else {
		logger.Info().
			Str("origin", actualInternalOrigin).
			Msg("cef: registered internal https handler factory")
	}

	eng.messageRouter = messageRouter
	eng.schemeHandler = schemeHandler

	return eng, nil
}

func buildTranscoderStartupState(cfg config.TranscodingConfig) transcoderStartupState {
	hwaccel := cfg.HWAccel
	if hwaccel == "" {
		hwaccel = "auto"
	}

	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}

	quality := cfg.Quality
	if quality == "" {
		quality = "medium"
	}

	status := "disabled_in_config"
	if cfg.Enabled {
		status = "configured_enabled"
	}

	return transcoderStartupState{
		ConfigEnabled: cfg.Enabled,
		HWAccel:       hwaccel,
		MaxConcurrent: maxConcurrent,
		Quality:       quality,
		Status:        status,
	}
}

// getPrimaryMonitor returns the primary GDK monitor, or nil if unavailable.
func getPrimaryMonitor() *gdk.Monitor {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return nil
	}
	monitors := display.GetMonitors()
	if monitors == nil {
		return nil
	}
	obj := monitors.GetObject(0)
	if obj == nil {
		return nil
	}
	mon := &gdk.Monitor{}
	mon.SetGoPointer(obj.GoPointer())
	return mon
}

// detectHiDPIScale queries the primary GDK monitor for its scale factor.
func detectHiDPIScale(logger *zerolog.Logger) int32 {
	mon := getPrimaryMonitor()
	if mon == nil {
		logger.Debug().Msg("cef: no primary monitor, using scale=1")
		return 1
	}
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
func cleanStaleSingletonLocks(logger *zerolog.Logger, dir string) {
	if dir == "" {
		return
	}

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
	return envBoolEnabled(puregoCEFInitTraceEnvVar)
}
