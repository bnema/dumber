// Package globals provides browser-like APIs for extension background scripts.
// This includes DOM shims, timers, fetch, TextEncoder/TextDecoder, console, etc.
package globals

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/pkg/browserjs"
	"github.com/grafana/sobek"
)

// Extension interface for extension info needed by globals.
type Extension interface {
	GetID() string
	GetInstallDir() string
}

// extensionLogger implements browserjs.Logger with extension ID prefix.
type extensionLogger struct {
	extID string
}

func (l *extensionLogger) Log(level string, args ...any) {
	if len(args) > 0 {
		log.Printf("[%s] %s: %v", l.extID, strings.ToUpper(level), args[0])
	}
}

// BrowserGlobals provides browser-like APIs for extension background scripts.
type BrowserGlobals struct {
	vm  *sobek.Runtime
	ext Extension
	mu  sync.Mutex

	// Browser environment (pkg/browserjs) - handles all standard browser APIs
	browserEnv *browserjs.Environment

	// Current script being executed (for document.currentScript)
	currentScriptURL string
	currentScriptDir string

	// Script loader callback - called when a script element is appended
	scriptLoader func(path string) error
}

// New creates browser globals for an extension.
// localStorage is optional - if nil, an in-memory implementation is used.
func New(vm *sobek.Runtime, ext Extension, tasks chan func(), localStorage browserjs.LocalStorageBackend) *BrowserGlobals {
	extID := ""
	origin := "null"
	basePath := ""
	if ext != nil {
		extID = ext.GetID()
		origin = fmt.Sprintf("dumb-extension://%s", extID)
		basePath = ext.GetInstallDir()
	}

	// Create browserjs environment with extension-specific config
	browserEnv := browserjs.New(vm, browserjs.Options{
		TaskQueue:         tasks,
		StartTime:         time.Now(),
		HTTPClient:        &http.Client{Timeout: 30 * time.Second},
		Logger:            &extensionLogger{extID: extID},
		Origin:            origin,
		LocalStorage:      localStorage,
		ExtensionBasePath: basePath,
	})

	return &BrowserGlobals{
		vm:         vm,
		ext:        ext,
		browserEnv: browserEnv,
	}
}

// SetCurrentScript sets the currently executing script URL.
func (bg *BrowserGlobals) SetCurrentScript(scriptPath string) {
	if bg.ext != nil {
		bg.currentScriptURL = "dumb-extension://" + bg.ext.GetID() + "/" + scriptPath
		bg.currentScriptDir = getDir(scriptPath)
	}
}

// SetScriptLoader sets the callback used when scripts are dynamically loaded.
func (bg *BrowserGlobals) SetScriptLoader(loader func(path string) error) {
	bg.scriptLoader = loader
}

// Install installs all browser globals into the VM.
func (bg *BrowserGlobals) Install() error {
	// Install standard browser APIs from pkg/browserjs
	if err := bg.browserEnv.InstallGlobalRefs(); err != nil {
		return err
	}
	if err := bg.browserEnv.InstallTimers(); err != nil {
		return err
	}
	if err := bg.browserEnv.InstallEncoding(); err != nil {
		return err
	}
	if err := bg.browserEnv.InstallURL(); err != nil {
		return err
	}

	// Register extension URL handler before installing fetch
	bg.browserEnv.RegisterFetchHandler(bg.extensionFetchHandler)
	if err := bg.browserEnv.InstallFetch(); err != nil {
		return err
	}

	// Install remaining browser APIs from browserjs
	if err := bg.browserEnv.InstallDOMConstructors(); err != nil {
		return err
	}
	if err := bg.browserEnv.InstallEvents(); err != nil {
		return err
	}
	if err := bg.browserEnv.InstallCSS(); err != nil {
		return err
	}
	if err := bg.browserEnv.InstallConsole(); err != nil {
		return err
	}
	if err := bg.browserEnv.InstallNavigator(); err != nil {
		return err
	}
	if err := bg.browserEnv.InstallPerformance(); err != nil {
		return err
	}
	if err := bg.browserEnv.InstallWebAPIs(); err != nil {
		return err
	}

	// Install extension-specific APIs (document with script loading, crypto)
	installers := []func() error{
		bg.installDocument,
		bg.installCrypto,
	}

	for _, install := range installers {
		if err := install(); err != nil {
			return err
		}
	}

	return nil
}

// Cleanup stops all timers and intervals.
func (bg *BrowserGlobals) Cleanup() {
	if bg.browserEnv != nil {
		bg.browserEnv.Cleanup()
	}
}

// Helper to get directory from path
func getDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return ""
}

// extensionFetchHandler handles dumb-extension:// URLs for fetch.
func (bg *BrowserGlobals) extensionFetchHandler(url string) *browserjs.FetchResult {
	if !strings.HasPrefix(url, "dumb-extension://") {
		return nil // Fall through to normal HTTP fetch
	}

	parts := strings.SplitN(url, "/", 4)
	if len(parts) < 4 {
		log.Printf("[fetch] Invalid extension URL: %s", url)
		return &browserjs.FetchResult{Error: fmt.Errorf("invalid extension URL")}
	}
	path := parts[3]
	fullPath := bg.ext.GetInstallDir() + "/" + path

	log.Printf("[fetch] Loading extension file: %s", fullPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		log.Printf("[fetch] Error reading file %s: %v", fullPath, err)
		return &browserjs.FetchResult{Error: err}
	}

	log.Printf("[fetch] Loaded %d bytes from %s", len(data), path)
	return &browserjs.FetchResult{
		Body:       data,
		Status:     200,
		StatusText: "OK",
	}
}
