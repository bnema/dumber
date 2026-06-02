package cef

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

const puregoCEFInitTraceEnvVar = "PUREGO_CEF_INIT_TRACE"
const CEFRootCachePathEnvVar = "DUMBER_CEF_ROOT_CACHE_PATH"
const defaultCEFWindowlessFrameRate = 60

// RuntimePaths contains the concrete filesystem paths the CEF adapter needs.
type RuntimePaths struct {
	StateRoot     string
	LogFile       string
	ProfileLogDir string
}

// NewEngine initializes the CEF runtime and returns a ready-to-use Engine.
func NewEngine(
	ctx context.Context,
	opts port.EngineOptions,
	paths RuntimePaths,
	cfg RuntimeConfig,
	audioFactory port.AudioOutputFactory,
	deps EngineDependencies,
) (*Engine, error) {
	logger := logging.FromContext(ctx)
	stateRoot := resolvedStateRoot(paths.StateRoot, opts)
	cleanStaleSingletonLocks(logger, stateRoot)
	windowlessFrameRate := normalizedWindowlessFrameRate(cfg.WindowlessFrameRate)
	renderStackPlan, err := resolveCEFRenderStackPlan(cfg.RenderStack)
	if err != nil {
		return nil, err
	}
	cef2gtk.ConfigureRenderStackEnvironment(renderStackPlan)

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
		profileLogDir:          paths.ProfileLogDir,
		runtimeCEFDir:          settings.CEFDir,
		renderStackPlan:        renderStackPlan,
		applicationScale:       normalizedApplicationScale(cfg.ApplicationScale),
		registerHandlers:       deps.RegisterHandlers,
		registerAccentHandlers: deps.RegisterAccentHandlers,
		currentConfigPayload:   deps.CurrentConfigPayload,
		defaultConfigPayload:   deps.DefaultConfigPayload,
		ctxMenuBuilder:         deps.ContextMenuBuilder,
		ctxMenuExecutorFactory: deps.ContextMenuExecutorFactory,
		ctxMenuRenderer:        deps.ContextMenuRenderer,
		clipboard:              deps.Clipboard,
		resolver:               deps.ImageDataResolver,
	}

	logger.Info().
		Int32("windowless_frame_rate", windowlessFrameRate).
		Str("render_stack", string(renderStackPlan.Stack)).
		Str("render_backend", renderStackPlan.Backend.String()).
		Str("angle_backend", renderStackPlan.ANGLEBackend).
		Str("gsk_renderer", renderStackPlan.GSKRenderer).
		Bool("external_begin_frame", externalBeginFrameEnabled()).
		Bool("trace_handlers", cfg.TraceHandlers).
		Bool("enable_audio_handler", cfg.EnableAudioHandler).
		Float64("application_scale", eng.currentApplicationScale()).
		Msg("cef: configured engine")

	if err := initializeCEF(eng, settings, logger); err != nil {
		os.Args = savedArgs
		return nil, err
	}
	os.Args = savedArgs

	return wireEngine(
		ctx,
		eng,
		webViewFactoryOptions{
			adaptiveWindowlessFrameRate: cfg.AdaptiveWindowlessFrameRate,
			windowlessFrameRate:         windowlessFrameRate,
			windowlessFrameRateMax:      cfg.WindowlessFrameRateMax,
			inputConfig:                 cfg.Input,
			audioOutputFactory:          audioFactory,
		},
		logger,
		deps.CurrentConfigPayload,
		deps.DefaultConfigPayload,
	)
}

func normalizedApplicationScale(scale float64) float64 {
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale <= 0 {
		return 1
	}
	return scale
}

func normalizedWindowlessFrameRate(frameRate int32) int32 {
	if frameRate > 0 {
		return frameRate
	}
	return defaultCEFWindowlessFrameRate
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
	logger.Info().
		Str("settings_cef_dir", settings.CEFDir).
		Str("env_cef_dir", os.Getenv("CEF_DIR")).
		Msg("cef: runtime selection")
	logger.Debug().Msg("cef: calling InitWithApp")
	if err := purecef.InitWithApp(settings, app); err != nil {
		return fmt.Errorf("cef.InitWithApp: %w", err)
	}
	if libcefPath := loadedLibCEFPath(); libcefPath != "" {
		logger.Info().Str("libcef_path", libcefPath).Msg("cef: runtime library loaded")
	} else {
		logger.Warn().Msg("cef: runtime library loaded but libcef path was not found in /proc/self/maps")
	}
	logger.Debug().Msg("cef: InitWithApp returned OK")
	return nil
}

func loadedLibCEFPath() string {
	data, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return ""
	}
	return parseLoadedLibCEFPath(string(data))
}

func parseLoadedLibCEFPath(maps string) string {
	for _, line := range strings.Split(maps, "\n") {
		if !strings.Contains(line, "libcef.so") {
			continue
		}
		pathStart := strings.Index(line, "/")
		if pathStart < 0 {
			continue
		}
		return strings.TrimSpace(line[pathStart:])
	}
	return ""
}

// wireEngine creates factory, pool, and scheme handler after CEF init.
func wireEngine(
	ctx context.Context, eng *Engine,
	factoryOptions webViewFactoryOptions, logger *zerolog.Logger,
	currentConfigPayload func() ([]byte, error), defaultConfigPayload func() ([]byte, error),
) (*Engine, error) {
	eng.factory = newWebViewFactory(eng, factoryOptions)
	eng.pool = newWebViewPool(eng.factory)
	eng.contentInj = newContentInjector(eng, nil)

	messageRouter, schemeHandler, err := newEngineSchemeHandler(
		ctx,
		eng,
		logger,
		currentConfigPayload,
		defaultConfigPayload,
	)
	if err != nil {
		purecef.Shutdown()
		return nil, err
	}
	eng.messageRouter = messageRouter
	eng.schemeHandler = schemeHandler
	eng.startCEFHeartbeat()

	registerEngineSchemeFactories(logger, purecef.NewSchemeHandlerFactory(schemeHandler))
	return eng, nil
}

func newEngineSchemeHandler(
	ctx context.Context,
	eng *Engine,
	logger *zerolog.Logger,
	currentConfigPayload func() ([]byte, error),
	defaultConfigPayload func() ([]byte, error),
) (*MessageRouter, *dumbSchemeHandler, error) {
	messageRouter := NewMessageRouter(ctx)
	schemeHandler, err := newDumbSchemeHandler(
		ctx,
		messageRouter,
		currentConfigPayload,
		defaultConfigPayload,
	)
	if err != nil {
		return nil, nil, err
	}
	schemeHandler.setAssets(assets.WebUIAssets)
	schemeHandler.onClipboardSet = makeGTKClipboardSetter(logger)
	schemeHandler.onEditableFocus = eng.handleEditableFocusBridge
	schemeHandler.onPopupOpen = eng.handlePopupBridgeOpen
	schemeHandler.onPopupNavigate = eng.handlePopupBridgeNavigate
	schemeHandler.onPopupClose = eng.handlePopupBridgeClose
	schemeHandler.onPopupOpenerNavigate = eng.handlePopupOpenerNavigate
	schemeHandler.onPopupOpenerPostMessage = eng.handlePopupOpenerPostMessage
	schemeHandler.bridgeNonceValidator = eng.validateBridgeRequest
	return messageRouter, schemeHandler, nil
}

func makeGTKClipboardSetter(logger *zerolog.Logger) func(string) {
	// Bridge clipboard writes from page JS → GDK system clipboard.
	// The callback is invoked on the CEF IO thread, so schedule the actual GDK
	// write on the GTK main loop.
	return func(text string) {
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
}

func registerEngineSchemeFactories(logger *zerolog.Logger, schemeFactory purecef.SchemeHandlerFactory) {
	registerEngineSchemeFactory(logger, "dumb", "", schemeFactory)
	registerEngineSchemeFactory(logger, actualInternalScheme, actualInternalHost, schemeFactory)
}

func registerEngineSchemeFactory(
	logger *zerolog.Logger,
	scheme, host string,
	schemeFactory purecef.SchemeHandlerFactory,
) {
	result := purecef.RegisterSchemeHandlerFactory(scheme, host, schemeFactory)
	if scheme == "dumb" && host == "" {
		if result != 1 {
			logger.Error().Int32("result", result).Msg("cef: failed to register dumb:// scheme handler factory")
			return
		}
		logger.Info().Msg("cef: registered dumb:// scheme handler factory")
		return
	}
	if result != 1 {
		logger.Error().
			Int32("result", result).
			Str("origin", actualInternalOrigin).
			Msg("cef: failed to register internal https handler factory")
		return
	}
	logger.Info().
		Str("origin", actualInternalOrigin).
		Msg("cef: registered internal https handler factory")
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
