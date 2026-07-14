// init.go is the composition root for the cef package. It wires
// the loader, CAPI bridge, and core Engine together.
//
// This file is handwritten — the rest of cef/ is generated.
//
// The hexagonal refactor removed the following hand-written APIs that are
// now provided (or intentionally dropped) by the generated public layer:
//   - NewKeyEvent, KeyEventSetType → use generated KeyEvent struct directly
//   - DefaultWindowInfo, DefaultBrowserSettings → use NewWindowInfo(), NewBrowserSettings()
//   - SetHandlerTraceEnabled, HandlerTraceEnabled → handler tracing removed
package cef

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"unsafe"

	"github.com/bnema/purego-cef/internal/capi"
	"github.com/bnema/purego-cef/internal/core"
	"github.com/bnema/purego-cef/internal/loader"
	portout "github.com/bnema/purego-cef/internal/ports/out"
)

var (
	openCEFLibrary           = loader.Open
	newCAPI                  = func(handle uintptr) portout.CAPI { return capi.NewBridge(handle) }
	processArgs              = func() []string { return os.Args }
	exitProcess              = os.Exit
	stderrWriter   io.Writer = os.Stderr
)

// Settings configures the CEF runtime.
//
// This is the user-facing settings type for Init/InitWithApp. Prefer it over
// the raw generated RawSettings struct unless you specifically need the exact
// CEF memory layout.
type Settings struct {
	CEFDir                     string
	LogSeverity                int32
	MultiThreadedMessageLoop   bool
	WindowlessRenderingEnabled bool
	ExternalMessagePump        bool
	NoSandbox                  bool
	BrowserSubprocessPath      string
	LogFile                    string
	CachePath                  string
	RootCachePath              string
}

func (s Settings) coreSettings() core.Settings {
	return core.Settings{
		CEFDir:                     s.CEFDir,
		LogSeverity:                s.LogSeverity,
		MultiThreadedMessageLoop:   s.MultiThreadedMessageLoop,
		WindowlessRenderingEnabled: s.WindowlessRenderingEnabled,
		ExternalMessagePump:        s.ExternalMessagePump,
		NoSandbox:                  s.NoSandbox,
		BrowserSubprocessPath:      s.BrowserSubprocessPath,
		LogFile:                    s.LogFile,
		CachePath:                  s.CachePath,
		RootCachePath:              s.RootCachePath,
	}
}

// DefaultSettings returns Settings suitable for off-screen rendering.
func DefaultSettings() Settings {
	return Settings{
		ExternalMessagePump:        true,
		WindowlessRenderingEnabled: true,
		NoSandbox:                  true,
	}
}

// Init loads the CEF library, registers all symbols, and initializes the runtime.
// It runs once via sync.Once — if initialization fails, subsequent calls return
// the cached error without retrying. This is by design: a failed CEF init leaves
// the process in an indeterminate state, so retrying would be unsafe.
func Init(settings Settings) error {
	return InitWithApp(settings, nil)
}

// InitWithApp loads the CEF library, registers all symbols, and initializes the
// runtime with a custom App handler. The App enables registration of custom
// schemes, browser process handlers, and subprocess apps.
// It runs once via sync.Once — if initialization fails, subsequent calls return
// the cached error without retrying.
func InitWithApp(settings Settings, app App) error {
	initOnce.Do(func() {
		handle, err := openCEFLibrary(settings.CEFDir)
		if err != nil {
			initErr = err
			return
		}
		api := newCAPI(handle)
		e := core.New(api)
		eng = e
		setCurrentRefManager(e.Refs())

		var appPtr unsafe.Pointer
		var wrapped App
		if !isNilImpl(app) {
			wrapped = NewApp(app)
			appPtr = extractRawPointer(wrapped)
		}
		initErr = e.InitWithApp(settings.coreSettings(), appPtr)
		runtime.KeepAlive(wrapped)
	})
	return initErr
}

// Shutdown releases all CEF resources.
func Shutdown() {
	if eng != nil {
		eng.Shutdown()
	}
}

// DoMessageLoopWork pumps the CEF message loop for one iteration.
func DoMessageLoopWork() {
	if eng != nil {
		eng.DoMessageLoopWork()
	}
}

// ExecuteSubprocess runs cef_execute_process using a lightweight path that
// binds only the symbols needed for subprocess detection.
//
// It returns executed=true when the current process was handled as a CEF helper
// subprocess. In that case exitCode should be used as the process exit status.
// When executed=false and err=nil, the caller should continue normal startup.
func ExecuteSubprocess() (executed bool, exitCode int, err error) {
	return ExecuteSubprocessWithApp(nil)
}

// ExecuteSubprocessWithApp is like ExecuteSubprocess but passes the given App
// to cef_execute_process. This enables custom render/browser-process handlers
// in helper subprocesses without calling cef_initialize first.
func ExecuteSubprocessWithApp(app App) (executed bool, exitCode int, err error) {
	handle, err := openCEFLibrary("")
	if err != nil {
		return false, 0, err
	}
	api := newCAPI(handle)
	// ExecuteProcess may invoke App callbacks that use generated helpers like
	// cefString/freeCefString/goStringUserfree, all of which route through eng.
	// Keep a lightweight subprocess engine installed for the full callback window,
	// matching the historical behavior from the pre-v0.10 path.
	subprocessEngine := core.New(api)
	subprocessRefs := subprocessEngine.Refs()
	registerRefManager(subprocessRefs)
	defer unregisterRefManager(subprocessRefs)

	previousEngine := eng
	eng = subprocessEngine
	defer func() {
		eng = previousEngine
	}()

	var appPtr unsafe.Pointer
	var wrapped App
	args := core.NewMainArgs(processArgs())
	var code int32
	withCurrentRefManager(subprocessRefs, func() {
		if !isNilImpl(app) {
			wrapped = NewApp(app)
			appPtr = extractRawPointer(wrapped)
		}
		code = api.ExecuteProcess(args.Ptr(), appPtr, nil)
	})
	runtime.KeepAlive(args)
	runtime.KeepAlive(wrapped)
	if code >= 0 {
		return true, int(code), nil
	}
	return false, 0, nil
}

// MaybeExitSubprocess is a convenience helper for main packages.
// Library code should usually prefer ExecuteSubprocess.
func MaybeExitSubprocess() {
	executed, exitCode, err := ExecuteSubprocess()
	if err != nil {
		_, _ = fmt.Fprintf(stderrWriter, "cef: ExecuteSubprocess: %v\n", err)
		return
	}
	if executed {
		exitProcess(exitCode)
	}
}
