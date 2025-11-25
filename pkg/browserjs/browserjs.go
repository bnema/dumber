// Package browserjs provides browser-like JavaScript APIs for Go JS runtimes.
// It implements standard web platform APIs (timers, fetch, encoding, DOM shims, etc.)
// that can be used with Sobek or other JavaScript engines.
//
// This package is designed to be extracted as a standalone SDK for building
// browser-like JavaScript environments in Go.
package browserjs

import (
	"net/http"
	"time"

	"github.com/grafana/sobek"
)

// Environment provides browser-like APIs for a JavaScript runtime.
type Environment struct {
	vm      *sobek.Runtime
	options Options

	// Submodules (lazily initialized)
	timers      *TimerManager
	encoding    *EncodingManager
	url         *URLManager
	fetch       *FetchManager
	dom         *DOMManager
	events      *EventManager
	css         *CSSManager
	console     *ConsoleManager
	navigator   *NavigatorManager
	performance *PerformanceManager
	webAPIs     *WebAPIsManager
}

// Options configures the browser environment.
type Options struct {
	// HTTPClient for fetch/XHR requests. If nil, uses http.DefaultClient.
	HTTPClient HTTPDoer

	// TaskQueue receives callbacks for async operations (timers, fetch, etc.).
	// If nil, async callbacks run directly (not recommended for production).
	TaskQueue chan func()

	// Logger receives console output. If nil, logs are discarded.
	Logger Logger

	// Clock provides time operations. If nil, uses real time.
	Clock Clock

	// RandomSource provides cryptographic randomness. If nil, uses crypto/rand.
	RandomSource RandomSource

	// StartTime is used for performance.now(). If zero, uses time.Now() at creation.
	StartTime time.Time

	// Origin is the origin for the window object (e.g., "https://example.com" or "null").
	Origin string

	// LocalStorage provides persistent localStorage backend. If nil, uses in-memory storage.
	LocalStorage LocalStorageBackend
}

// New creates a new browser environment for the given Sobek runtime.
func New(vm *sobek.Runtime, opts Options) *Environment {
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	if opts.StartTime.IsZero() {
		opts.StartTime = time.Now()
	}
	if opts.Clock == nil {
		opts.Clock = &realClock{}
	}

	return &Environment{
		vm:      vm,
		options: opts,
	}
}

// VM returns the underlying Sobek runtime.
func (e *Environment) VM() *sobek.Runtime {
	return e.vm
}

// Options returns the environment options.
func (e *Environment) Options() Options {
	return e.options
}

// InstallGlobalRefs sets up self, window, globalThis to reference the global object.
func (e *Environment) InstallGlobalRefs() error {
	global := e.vm.GlobalObject()
	e.vm.Set("self", global)
	e.vm.Set("window", global)
	e.vm.Set("globalThis", global)
	return nil
}

// InstallTimers installs setTimeout, clearTimeout, setInterval, clearInterval, queueMicrotask.
func (e *Environment) InstallTimers() error {
	if e.timers == nil {
		e.timers = NewTimerManager(e.vm, e.options.TaskQueue, e.options.Clock)
	}
	return e.timers.Install()
}

// InstallEncoding installs TextEncoder, TextDecoder, atob, btoa.
func (e *Environment) InstallEncoding() error {
	if e.encoding == nil {
		e.encoding = NewEncodingManager(e.vm)
	}
	return e.encoding.Install()
}

// InstallURL installs URL and URLSearchParams.
func (e *Environment) InstallURL() error {
	if e.url == nil {
		e.url = NewURLManager(e.vm)
	}
	return e.url.Install()
}

// InstallFetch installs fetch, Headers, Request, Response.
func (e *Environment) InstallFetch() error {
	if e.fetch == nil {
		e.fetch = NewFetchManager(e.vm, e.options.HTTPClient, e.options.TaskQueue)
	}
	return e.fetch.Install()
}

// RegisterFetchHandler adds a custom URL handler for fetch (e.g., extension:// URLs).
func (e *Environment) RegisterFetchHandler(handler FetchHandler) {
	if e.fetch == nil {
		e.fetch = NewFetchManager(e.vm, e.options.HTTPClient, e.options.TaskQueue)
	}
	e.fetch.RegisterHandler(handler)
}

// InstallDOMConstructors installs DOM class constructors (Node, Element, Blob, etc.).
func (e *Environment) InstallDOMConstructors() error {
	if e.dom == nil {
		e.dom = NewDOMManager(e.vm, e.options.TaskQueue)
	}
	return e.dom.InstallConstructors()
}

// InstallEvents installs Event constructors and EventTarget.
func (e *Environment) InstallEvents() error {
	if e.events == nil {
		e.events = NewEventManager(e.vm, e.options.StartTime)
	}
	return e.events.Install()
}

// InstallCSS installs the CSS global object.
func (e *Environment) InstallCSS() error {
	if e.css == nil {
		e.css = NewCSSManager(e.vm)
	}
	return e.css.Install()
}

// InstallConsole installs the console object.
func (e *Environment) InstallConsole() error {
	if e.console == nil {
		e.console = NewConsoleManager(e.vm, e.options.Logger, e.options.StartTime)
	}
	return e.console.Install()
}

// InstallNavigator installs the navigator object.
func (e *Environment) InstallNavigator() error {
	if e.navigator == nil {
		e.navigator = NewNavigatorManager(e.vm, e.options.TaskQueue)
	}
	return e.navigator.Install()
}

// InstallPerformance installs the performance object.
func (e *Environment) InstallPerformance() error {
	if e.performance == nil {
		e.performance = NewPerformanceManager(e.vm, e.options.StartTime)
	}
	return e.performance.Install()
}

// InstallWebAPIs installs additional web APIs (XMLHttpRequest, Storage, etc.).
func (e *Environment) InstallWebAPIs() error {
	if e.webAPIs == nil {
		e.webAPIs = NewWebAPIsManager(e.vm, e.options.TaskQueue, e.options.HTTPClient, e.options.StartTime, e.options.Origin, e.options.LocalStorage)
	}
	return e.webAPIs.Install()
}

// InstallAll installs all available browser APIs.
func (e *Environment) InstallAll() error {
	installers := []func() error{
		e.InstallGlobalRefs,
		e.InstallTimers,
		e.InstallEncoding,
		e.InstallURL,
		e.InstallFetch,
		e.InstallDOMConstructors,
		e.InstallEvents,
		e.InstallCSS,
		e.InstallConsole,
		e.InstallNavigator,
		e.InstallPerformance,
		e.InstallWebAPIs,
	}

	for _, install := range installers {
		if err := install(); err != nil {
			return err
		}
	}
	return nil
}

// Cleanup releases resources (stops timers, etc.).
func (e *Environment) Cleanup() {
	if e.timers != nil {
		e.timers.Cleanup()
	}
}
