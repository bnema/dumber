package webext

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/bnema/dumber/internal/webext/api"
	"github.com/dop251/goja"
)

// BackgroundContext represents the JavaScript execution context for an extension's background page
type BackgroundContext struct {
	mu        sync.RWMutex
	vm        *goja.Runtime
	ext       *Extension
	running   bool
	listeners map[string][]goja.Callable // Event listeners by event name

	// Callbacks for async operations
	callbacks     map[string]goja.Callable
	callbackIDGen int
}

// NewBackgroundContext creates a new background context for an extension
func NewBackgroundContext(ext *Extension) *BackgroundContext {
	return &BackgroundContext{
		vm:        goja.New(),
		ext:       ext,
		listeners: make(map[string][]goja.Callable),
		callbacks: make(map[string]goja.Callable),
	}
}

// Start initializes and starts the background context
func (b *BackgroundContext) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return fmt.Errorf("background context already running")
	}

	// Setup global objects
	if err := b.setupGlobals(); err != nil {
		return fmt.Errorf("failed to setup globals: %w", err)
	}

	// Expose chrome.* APIs
	if err := b.exposeChromeAPIs(); err != nil {
		return fmt.Errorf("failed to expose chrome APIs: %w", err)
	}

	// Load and execute background scripts
	if b.ext.Manifest.Background != nil {
		for _, scriptPath := range b.ext.Manifest.Background.Scripts {
			if err := b.loadScript(scriptPath); err != nil {
				return fmt.Errorf("failed to load background script %s: %w", scriptPath, err)
			}
		}
	}

	b.running = true
	log.Printf("[background] Started background context for extension %s", b.ext.ID)
	return nil
}

// Stop stops the background context and cleans up resources
func (b *BackgroundContext) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return
	}

	b.running = false
	b.vm.ClearInterrupt()
	log.Printf("[background] Stopped background context for extension %s", b.ext.ID)
}

// setupGlobals sets up global JavaScript objects (console, setTimeout, etc.)
func (b *BackgroundContext) setupGlobals() error {
	// Console object
	console := b.vm.NewObject()
	console.Set("log", b.consoleLog)
	console.Set("error", b.consoleError)
	console.Set("warn", b.consoleWarn)
	console.Set("info", b.consoleLog)
	console.Set("debug", b.consoleLog)
	b.vm.Set("console", console)

	// Simple setTimeout/setInterval stubs (for now)
	// TODO: Implement proper timer support with goroutines
	b.vm.Set("setTimeout", b.setTimeout)
	b.vm.Set("setInterval", b.setInterval)
	b.vm.Set("clearTimeout", b.clearTimeout)
	b.vm.Set("clearInterval", b.clearInterval)

	return nil
}

// exposeChromeAPIs exposes the chrome.* WebExtension APIs to JavaScript
func (b *BackgroundContext) exposeChromeAPIs() error {
	chrome := b.vm.NewObject()

	// chrome.runtime
	runtime := b.createRuntimeAPI()
	chrome.Set("runtime", runtime)

	// chrome.storage
	storage := b.createStorageAPI()
	chrome.Set("storage", storage)

	// chrome.tabs
	tabs := b.createTabsAPI()
	chrome.Set("tabs", tabs)

	// chrome.webRequest
	webRequest := b.createWebRequestAPI()
	chrome.Set("webRequest", webRequest)

	b.vm.Set("chrome", chrome)

	// Also set 'browser' for Firefox compatibility
	b.vm.Set("browser", chrome)

	return nil
}

// createRuntimeAPI creates the chrome.runtime API object
func (b *BackgroundContext) createRuntimeAPI() *goja.Object {
	runtime := b.vm.NewObject()

	// chrome.runtime.sendMessage
	runtime.Set("sendMessage", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return b.vm.ToValue(nil)
		}

		message := call.Argument(0).Export()
		var callback goja.Callable

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				callback = fn
			}
		}

		// Handle message through extension's runtime API
		if b.ext.Runtime != nil {
			err := b.ext.Runtime.SendMessage(message, func(response interface{}) {
				if callback != nil {
					b.invokeCallback(callback, response)
				}
			})
			if err != nil {
				log.Printf("[background] SendMessage error: %v", err)
			}
		}

		return goja.Undefined()
	})

	// chrome.runtime.onMessage
	onMessage := b.vm.NewObject()
	onMessage.Set("addListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}

		if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
			b.addListener("runtime.onMessage", fn)
		}

		return goja.Undefined()
	})
	runtime.Set("onMessage", onMessage)

	// chrome.runtime.getURL
	runtime.Set("getURL", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return b.vm.ToValue("")
		}

		path := call.Argument(0).String()
		fullPath := filepath.Join(b.ext.Path, path)
		return b.vm.ToValue(fullPath)
	})

	// chrome.runtime.getManifest
	runtime.Set("getManifest", func(call goja.FunctionCall) goja.Value {
		return b.vm.ToValue(b.ext.Manifest)
	})

	// chrome.runtime.id
	runtime.Set("id", b.ext.ID)

	return runtime
}

// createStorageAPI creates the chrome.storage API object
func (b *BackgroundContext) createStorageAPI() *goja.Object {
	storage := b.vm.NewObject()

	// chrome.storage.local
	local := b.vm.NewObject()

	local.Set("get", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}

		keys := call.Argument(0).Export()
		var callback goja.Callable

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				callback = fn
			}
		}

		if b.ext.Storage != nil {
			go func() {
				var keySlice []string
				switch v := keys.(type) {
				case string:
					keySlice = []string{v}
				case []interface{}:
					for _, k := range v {
						if str, ok := k.(string); ok {
							keySlice = append(keySlice, str)
						}
					}
				}

				items, err := b.ext.Storage.Local().Get(keySlice)
				if err != nil {
					log.Printf("[background] Storage.get error: %v", err)
					items = make(map[string]interface{})
				}

				if callback != nil {
					b.invokeCallback(callback, items)
				}
			}()
		}

		return goja.Undefined()
	})

	local.Set("set", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}

		items := call.Argument(0).Export()
		var callback goja.Callable

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				callback = fn
			}
		}

		if b.ext.Storage != nil {
			go func() {
				itemsMap, ok := items.(map[string]interface{})
				if !ok {
					log.Printf("[background] Storage.set: items not a map")
					return
				}

				err := b.ext.Storage.Local().Set(itemsMap)
				if err != nil {
					log.Printf("[background] Storage.set error: %v", err)
				}

				if callback != nil {
					b.invokeCallback(callback, nil)
				}
			}()
		}

		return goja.Undefined()
	})

	local.Set("remove", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}

		keys := call.Argument(0).Export()
		var callback goja.Callable

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				callback = fn
			}
		}

		if b.ext.Storage != nil {
			go func() {
				var keySlice []string
				switch v := keys.(type) {
				case string:
					keySlice = []string{v}
				case []interface{}:
					for _, k := range v {
						if str, ok := k.(string); ok {
							keySlice = append(keySlice, str)
						}
					}
				}

				err := b.ext.Storage.Local().Remove(keySlice)
				if err != nil {
					log.Printf("[background] Storage.remove error: %v", err)
				}

				if callback != nil {
					b.invokeCallback(callback, nil)
				}
			}()
		}

		return goja.Undefined()
	})

	storage.Set("local", local)
	return storage
}

// createTabsAPI creates the chrome.tabs API object
func (b *BackgroundContext) createTabsAPI() *goja.Object {
	tabs := b.vm.NewObject()

	// chrome.tabs.query
	tabs.Set("query", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}

		_ = call.Argument(0).Export() // queryInfo - TODO: use for filtering
		var callback goja.Callable

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				callback = fn
			}
		}

		if b.ext.Tabs != nil {
			go func() {
				// TODO: Implement tab querying with queryInfo filter
				// For now, return empty array
				result := []interface{}{}

				if callback != nil {
					b.invokeCallback(callback, result)
				}
			}()
		}

		return goja.Undefined()
	})

	// chrome.tabs.sendMessage
	tabs.Set("sendMessage", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}

		tabID := call.Argument(0).Export()
		message := call.Argument(1).Export()
		var callback goja.Callable

		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			if fn, ok := goja.AssertFunction(call.Argument(2)); ok {
				callback = fn
			}
		}

		if b.ext.Tabs != nil {
			go func() {
				// TODO: Implement message sending to tab
				log.Printf("[background] tabs.sendMessage to %v: %v", tabID, message)

				if callback != nil {
					b.invokeCallback(callback, nil)
				}
			}()
		}

		return goja.Undefined()
	})

	return tabs
}

// createWebRequestAPI creates the chrome.webRequest API object
func (b *BackgroundContext) createWebRequestAPI() *goja.Object {
	webRequest := b.vm.NewObject()

	// chrome.webRequest.onBeforeRequest
	onBeforeRequest := b.vm.NewObject()
	onBeforeRequest.Set("addListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}

		listener, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			return goja.Undefined()
		}

		// Parse filter from second argument
		filterObj := call.Argument(1).Export()
		filter := &api.RequestFilter{}

		if filterMap, ok := filterObj.(map[string]interface{}); ok {
			if urls, ok := filterMap["urls"].([]interface{}); ok {
				for _, u := range urls {
					if urlStr, ok := u.(string); ok {
						filter.URLs = append(filter.URLs, urlStr)
					}
				}
			}
		}

		// Register the listener with the webRequest API
		b.addListener("webRequest.onBeforeRequest", listener)

		// TODO: Wire up to actual Manager's webRequest API
		log.Printf("[background] Registered webRequest.onBeforeRequest listener for %s", b.ext.ID)

		return goja.Undefined()
	})
	webRequest.Set("onBeforeRequest", onBeforeRequest)

	// chrome.webRequest.onBeforeSendHeaders
	onBeforeSendHeaders := b.vm.NewObject()
	onBeforeSendHeaders.Set("addListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}

		listener, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			return goja.Undefined()
		}

		b.addListener("webRequest.onBeforeSendHeaders", listener)
		log.Printf("[background] Registered webRequest.onBeforeSendHeaders listener for %s", b.ext.ID)

		return goja.Undefined()
	})
	webRequest.Set("onBeforeSendHeaders", onBeforeSendHeaders)

	// chrome.webRequest.onHeadersReceived
	onHeadersReceived := b.vm.NewObject()
	onHeadersReceived.Set("addListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}

		listener, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			return goja.Undefined()
		}

		b.addListener("webRequest.onHeadersReceived", listener)
		log.Printf("[background] Registered webRequest.onHeadersReceived listener for %s", b.ext.ID)

		return goja.Undefined()
	})
	webRequest.Set("onHeadersReceived", onHeadersReceived)

	return webRequest
}

// loadScript loads and executes a JavaScript file in the VM
func (b *BackgroundContext) loadScript(scriptPath string) error {
	fullPath := filepath.Join(b.ext.Path, scriptPath)

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("failed to read script: %w", err)
	}

	log.Printf("[background] Loading script %s for extension %s", scriptPath, b.ext.ID)

	_, err = b.vm.RunString(string(content))
	if err != nil {
		return fmt.Errorf("failed to execute script: %w", err)
	}

	return nil
}

// addListener adds an event listener
func (b *BackgroundContext) addListener(event string, fn goja.Callable) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.listeners[event] = append(b.listeners[event], fn)
}

// invokeCallback invokes a callback with the given arguments in the VM's event loop
func (b *BackgroundContext) invokeCallback(callback goja.Callable, args ...interface{}) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return
	}

	gojaArgs := make([]goja.Value, len(args))
	for i, arg := range args {
		gojaArgs[i] = b.vm.ToValue(arg)
	}

	_, err := callback(goja.Undefined(), gojaArgs...)
	if err != nil {
		log.Printf("[background] Callback error: %v", err)
	}
}

// Console methods
func (b *BackgroundContext) consoleLog(call goja.FunctionCall) goja.Value {
	args := make([]interface{}, len(call.Arguments))
	for i, arg := range call.Arguments {
		args[i] = arg.Export()
	}
	log.Printf("[%s] %v", b.ext.ID, args)
	return goja.Undefined()
}

func (b *BackgroundContext) consoleError(call goja.FunctionCall) goja.Value {
	args := make([]interface{}, len(call.Arguments))
	for i, arg := range call.Arguments {
		args[i] = arg.Export()
	}
	log.Printf("[%s] ERROR: %v", b.ext.ID, args)
	return goja.Undefined()
}

func (b *BackgroundContext) consoleWarn(call goja.FunctionCall) goja.Value {
	args := make([]interface{}, len(call.Arguments))
	for i, arg := range call.Arguments {
		args[i] = arg.Export()
	}
	log.Printf("[%s] WARN: %v", b.ext.ID, args)
	return goja.Undefined()
}

// Timer stubs (TODO: implement properly with goroutines)
func (b *BackgroundContext) setTimeout(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return b.vm.ToValue(0)
	}

	fn, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		return b.vm.ToValue(0)
	}

	// For now, just return a dummy timer ID
	// TODO: Implement proper timer with goroutine
	log.Printf("[background] setTimeout called (not yet implemented)")
	_ = fn

	return b.vm.ToValue(1)
}

func (b *BackgroundContext) setInterval(call goja.FunctionCall) goja.Value {
	log.Printf("[background] setInterval called (not yet implemented)")
	return b.vm.ToValue(1)
}

func (b *BackgroundContext) clearTimeout(call goja.FunctionCall) goja.Value {
	log.Printf("[background] clearTimeout called (not yet implemented)")
	return goja.Undefined()
}

func (b *BackgroundContext) clearInterval(call goja.FunctionCall) goja.Value {
	log.Printf("[background] clearInterval called (not yet implemented)")
	return goja.Undefined()
}
