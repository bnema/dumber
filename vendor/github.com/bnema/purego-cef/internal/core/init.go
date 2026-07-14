// internal/core/init.go
package core

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/bnema/purego-cef/internal/capi"
)

// MainArgs holds command-line arguments for CEF. The backing byte buffers
// are kept alive for the lifetime of the value.
type MainArgs struct {
	raw  capi.CEFMainArgsT
	argv [][]byte
	ptrs []*byte
}

// NewMainArgs creates MainArgs from a string slice.
func NewMainArgs(args []string) *MainArgs {
	m := &MainArgs{}
	m.argv = make([][]byte, len(args))
	m.ptrs = make([]*byte, len(args))
	for i, s := range args {
		buf := make([]byte, len(s)+1)
		copy(buf, s)
		m.argv[i] = buf
		m.ptrs[i] = &buf[0]
	}
	m.raw.Argc = int32(len(args))
	if len(m.ptrs) > 0 {
		m.raw.Argv = uintptr(unsafe.Pointer(&m.ptrs[0]))
	}
	return m
}

// Ptr returns an unsafe.Pointer to the underlying C struct.
func (m *MainArgs) Ptr() unsafe.Pointer { return unsafe.Pointer(&m.raw) }

// Settings configures the CEF runtime.
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

// DefaultSettings returns Settings suitable for off-screen rendering.
func DefaultSettings() Settings {
	return Settings{
		ExternalMessagePump:        true,
		WindowlessRenderingEnabled: true,
		NoSandbox:                  true,
	}
}

// Init initializes the CEF runtime. Must be called after New().
func (e *Engine) Init(settings Settings) error {
	return e.InitWithApp(settings, nil)
}

// InitWithApp initializes with an optional App for process handlers.
func (e *Engine) InitWithApp(settings Settings, appPtr unsafe.Pointer) error {
	args := NewMainArgs(os.Args)

	csettings, cleanup := e.settingsToRaw(settings)
	defer cleanup()

	ok := e.capi.Initialize(args.Ptr(), unsafe.Pointer(&csettings), appPtr, nil)
	runtime.KeepAlive(args)
	if ok != 1 {
		return fmt.Errorf("cef: cef_initialize returned %d", ok)
	}
	e.initialized.Store(true)
	return nil
}

// Shutdown releases all CEF resources.
func (e *Engine) Shutdown() {
	if !e.initialized.Load() {
		return
	}
	e.capi.Shutdown()
	e.initialized.Store(false)
}

// DoMessageLoopWork pumps the CEF message loop for one iteration.
func (e *Engine) DoMessageLoopWork() {
	e.capi.DoMessageLoopWork()
}

// MaybeExitSubprocess calls cef_execute_process and exits if this is a subprocess.
func (e *Engine) MaybeExitSubprocess() {
	args := NewMainArgs(os.Args)
	code := e.capi.ExecuteProcess(args.Ptr(), nil, nil)
	runtime.KeepAlive(args)
	if code >= 0 {
		os.Exit(int(code))
	}
}

// settingsToRaw converts Settings to raw CEF settings struct bytes.
// Returns the raw struct and a cleanup function for string fields.
func (e *Engine) settingsToRaw(s Settings) (capi.CEFSettingsT, func()) {
	var c capi.CEFSettingsT
	c.Size = uintptr(unsafe.Sizeof(c))

	if s.NoSandbox {
		c.NoSandbox = 1
	}
	if s.MultiThreadedMessageLoop {
		c.MultiThreadedMessageLoop = 1
	}
	if s.ExternalMessagePump && !s.MultiThreadedMessageLoop {
		c.ExternalMessagePump = 1
	}
	if s.WindowlessRenderingEnabled {
		c.WindowlessRenderingEnabled = 1
	}
	c.LogSeverity = s.LogSeverity

	var cleanups []func()
	if s.BrowserSubprocessPath != "" {
		cs := e.CefString(s.BrowserSubprocessPath)
		c.BrowserSubprocessPath = cs
		cleanups = append(cleanups, func() { e.FreeCefString(&c.BrowserSubprocessPath) })
	}
	if s.LogFile != "" {
		cs := e.CefString(s.LogFile)
		c.LogFile = cs
		cleanups = append(cleanups, func() { e.FreeCefString(&c.LogFile) })
	}
	if s.CachePath != "" {
		cs := e.CefString(s.CachePath)
		c.CachePath = cs
		cleanups = append(cleanups, func() { e.FreeCefString(&c.CachePath) })
	}
	if s.RootCachePath != "" {
		cs := e.CefString(s.RootCachePath)
		c.RootCachePath = cs
		cleanups = append(cleanups, func() { e.FreeCefString(&c.RootCachePath) })
	}

	cleanup := func() {
		for _, fn := range cleanups {
			fn()
		}
	}
	return c, cleanup
}
