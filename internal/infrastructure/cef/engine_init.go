package cef

import (
	"context"
	"errors"
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
const CEFRootCachePathEnvVar = "DUMBER_CEF_ROOT_CACHE_PATH"

// RuntimePaths contains the concrete filesystem paths the CEF adapter needs.
type RuntimePaths struct {
	StateRoot string
	LogFile   string
}

// NewEngine initializes the CEF runtime and returns a ready-to-use Engine.
func NewEngine(
	ctx context.Context,
	opts port.EngineOptions,
	paths RuntimePaths,
	cfg RuntimeConfig,
	transcodingCfg TranscodingRuntimeConfig,
	audioFactory port.AudioOutputFactory,
	deps EngineDependencies,
) (*Engine, error) {
	logger := logging.FromContext(ctx)
	deps.MediaClassifier = deps.MediaClassifier.normalize()
	stateRoot := resolvedStateRoot(paths.StateRoot, opts)
	cleanStaleSingletonLocks(logger, stateRoot)
	windowlessFrameRate := config.CEFEngineConfig{WindowlessFrameRate: cfg.WindowlessFrameRate}.CEFWindowlessFrameRate()

	settings, err := prepareCEFSettings(opts, paths, cfg, logger)
	if err != nil {
		return nil, err
	}

	// Inject --no-zygote temporarily for cef_initialize, then restore.
	// Safe: runs during single-threaded startup before concurrent goroutines.
	savedArgs := os.Args
	os.Args = appendIfMissing(os.Args, "--no-zygote")

	eng := &Engine{
		ctx:                    ctx,
		registerHandlers:       deps.RegisterHandlers,
		registerAccentHandlers: deps.RegisterAccentHandlers,
		currentConfigPayload:   deps.CurrentConfigPayload,
		defaultConfigPayload:   deps.DefaultConfigPayload,
		ctxMenuBuilder:         deps.ContextMenuBuilder,
		ctxMenuExecutorFactory: deps.ContextMenuExecutorFactory,
		clipboard:              deps.Clipboard,
		resolver:               deps.ImageDataResolver,
		mediaClassifier:        deps.MediaClassifier,
	}

	logger.Info().
		Int32("windowless_frame_rate", windowlessFrameRate).
		Bool("external_begin_frame", externalBeginFrameEnabled()).
		Bool("trace_handlers", cfg.TraceHandlers).
		Bool("enable_audio_handler", cfg.EnableAudioHandler).
		Msg("cef: configured engine")

	if err := initializeCEF(eng, settings, logger); err != nil {
		os.Args = savedArgs
		return nil, err
	}
	os.Args = savedArgs

	return wireEngine(
		ctx,
		eng,
		cfg,
		transcodingCfg,
		windowlessFrameRate,
		audioFactory,
		logger,
		deps.MediaClassifier,
		deps.CurrentConfigPayload,
		deps.DefaultConfigPayload,
	)
}

func resolvedStateRoot(defaultStateRoot string, opts port.EngineOptions) string {
	if root := os.Getenv(CEFRootCachePathEnvVar); root != "" {
		return root
	}
	if opts.CacheDir != "" {
		return opts.CacheDir
	}
	if opts.DataDir != "" {
		return opts.DataDir
	}
	return defaultStateRoot
}

// prepareCEFSettings builds purecef.Settings from the engine config.
func prepareCEFSettings(
	opts port.EngineOptions,
	paths RuntimePaths,
	cfg RuntimeConfig,
	logger *zerolog.Logger,
) (purecef.Settings, error) {
	switch opts.CookiePolicy {
	case "", port.CookiePolicyAlways:
	case port.CookiePolicyNoThirdParty:
		logger.Warn().
			Str("cookie_policy", string(opts.CookiePolicy)).
			Msg("cef: no_third_party cookie policy is currently treated as always")
	case port.CookiePolicyNever:
		return purecef.Settings{}, fmt.Errorf("%w: %s", ErrCookiePolicyUnsupported, opts.CookiePolicy)
	default:
		return purecef.Settings{}, fmt.Errorf("%w: %s", ErrCookiePolicyUnsupported, opts.CookiePolicy)
	}

	settings := purecef.DefaultSettings()
	settings.MultiThreadedMessageLoop = true
	settings.ExternalMessagePump = false
	settings.RootCachePath = resolvedStateRoot(paths.StateRoot, opts)
	if cfg.CEFDir != "" {
		settings.CEFDir = cfg.CEFDir
	}
	if runtimeLogFile, err := prepareCEFLogFile(paths.LogFile, cfg.LogFile); err != nil {
		logger.Warn().Err(err).Msg("cef: failed to prepare runtime log file")
	} else {
		settings.LogFile = runtimeLogFile
	}
	if puregoCEFInitTraceEnabled() {
		warn := logger.Warn().Str("env_var", puregoCEFInitTraceEnvVar)
		if bootstrapLogFile := resolveCEFInitTraceFile(paths.LogFile, cfg.LogFile); bootstrapLogFile != "" {
			warn = warn.Str("bootstrap_log_file", bootstrapLogFile)
		}
		warn.Msg("cef: init bootstrap diagnostics requested, but current purego-cef no longer consumes this trace file")
	}
	if cfg.LogSeverity != 0 {
		settings.LogSeverity = cfg.LogSeverity
	}
	return settings, nil
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
	ctx context.Context, eng *Engine, _ RuntimeConfig, transcodingCfg TranscodingRuntimeConfig,
	windowlessFrameRate int32, audioFactory port.AudioOutputFactory, logger *zerolog.Logger,
	mediaClassifier MediaClassifier,
	currentConfigPayload func() ([]byte, error), defaultConfigPayload func() ([]byte, error),
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
		tc := transcoderpkg.New(config.TranscodingConfig{
			Enabled:       transcodingCfg.Enabled,
			HWAccel:       transcodingCfg.HWAccel,
			MaxConcurrent: transcodingCfg.MaxConcurrent,
			Quality:       transcodingCfg.Quality,
		}, logger)
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
		scale:               scale,
		windowlessFrameRate: windowlessFrameRate,
		transcoder:          mediaTranscoder,
		mediaClassifier:     mediaClassifier,
		audioOutputFactory:  audioFactory,
	})
	pool := newWebViewPool(factory)

	eng.gl = gl
	eng.factory = factory
	eng.pool = pool
	eng.contentInj = newContentInjector(eng, nil)
	eng.transcoderState = transcoderState

	messageRouter := NewMessageRouter(ctx)
	schemeHandler, err := newDumbSchemeHandler(
		ctx,
		messageRouter,
		mediaTranscoder,
		currentConfigPayload,
		defaultConfigPayload,
	)
	if err != nil {
		purecef.Shutdown()
		return nil, err
	}
	schemeHandler.setAssets(assets.WebUIAssets)

	// Bridge clipboard writes from page JS → GDK system clipboard.
	// The callback is invoked on the CEF IO thread, so schedule the actual GDK
	// write on the GTK main loop.
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

	schemeHandler.onEditableFocus = eng.handleEditableFocusBridge
	schemeHandler.onPopupOpen = eng.handlePopupBridgeOpen
	schemeHandler.onPopupNavigate = eng.handlePopupBridgeNavigate
	schemeHandler.onPopupClose = eng.handlePopupBridgeClose
	schemeHandler.onPopupOpenerNavigate = eng.handlePopupOpenerNavigate
	schemeHandler.onPopupOpenerPostMessage = eng.handlePopupOpenerPostMessage
	schemeHandler.bridgeNonceValidator = eng.validateBridgeRequest

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

func buildTranscoderStartupState(cfg TranscodingRuntimeConfig) transcoderStartupState {
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

const (
	dirPerm  = 0o700
	filePerm = 0o600
)

func prepareCEFLogFile(defaultLogFile, logFile string) (string, error) {
	runtimeLogFile := resolveLogFile(defaultLogFile, logFile)
	if runtimeLogFile == "" {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(runtimeLogFile), dirPerm); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(runtimeLogFile), err)
	}

	file, err := openRegularLogFile(runtimeLogFile)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", runtimeLogFile, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("log path %s is not a regular file", runtimeLogFile)
	}
	if err := file.Chmod(filePerm); err != nil {
		return "", fmt.Errorf("chmod %s: %w", runtimeLogFile, err)
	}
	return runtimeLogFile, nil
}

func openRegularLogFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("lstat %s: %w", path, err)
		}
		file, createErr := openFileNoFollow(path, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY, filePerm)
		if createErr != nil {
			return nil, fmt.Errorf("create %s: %w", path, createErr)
		}
		return file, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("log path %s must not be a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("log path %s is not a regular file", path)
	}
	file, openErr := openFileNoFollow(path, syscall.O_WRONLY, 0)
	if openErr != nil {
		return nil, fmt.Errorf("open %s: %w", path, openErr)
	}
	return file, nil
}

func openFileNoFollow(path string, flags int, perm os.FileMode) (*os.File, error) {
	fd, err := syscall.Open(path, flags|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, uint32(perm))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func resolveCEFInitTraceFile(defaultLogFile, logFile string) string {
	runtimeLogFile := resolveLogFile(defaultLogFile, logFile)
	if runtimeLogFile == "" {
		return ""
	}
	return strings.TrimSuffix(runtimeLogFile, filepath.Ext(runtimeLogFile)) + ".bootstrap.log"
}

func resolveLogFile(defaultLogFile, logFile string) string {
	if strings.TrimSpace(logFile) != "" {
		return filepath.Clean(logFile)
	}
	if strings.TrimSpace(defaultLogFile) != "" {
		return filepath.Clean(defaultLogFile)
	}
	return ""
}

func puregoCEFInitTraceEnabled() bool {
	return envBoolEnabled(puregoCEFInitTraceEnvVar)
}
