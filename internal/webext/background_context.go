package webext

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/webext/api"
	"github.com/bnema/dumber/internal/webext/globals"
	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/grafana/sobek"
	"github.com/grafana/sobek/parser"
)

// BackgroundContext executes extension background scripts inside a Goja VM.
// All interaction with the VM happens on a single goroutine via the task queue.
type BackgroundContext struct {
	ext *Extension

	mu          sync.Mutex
	vm          *sobek.Runtime
	tasks       chan func()
	ctx         context.Context
	stop        context.CancelFunc
	browserGlob *globals.BrowserGlobals

	runtimeOnMessage *jsEvent
	runtimeOnConnect *jsEvent
	storageOnChanged *jsEvent

	webRequest struct {
		onBeforeRequest     *jsEvent
		onBeforeSendHeaders *jsEvent
		onHeadersReceived   *jsEvent
		onResponseStarted   *jsEvent
		onCompleted         *jsEvent
		onErrorOccurred     *jsEvent
	}

	ports map[string]*backgroundPort
	// connectExternal is set by the manager to allow background-initiated ports.
	connectExternal func(name string) (string, error)

	// i18n translations loaded at startup
	i18nMessages map[string]I18nMessage
	i18nLocale   string

	// alarms storage
	alarms      map[string]*Alarm
	alarmsEvent *jsEvent

	// pane provider for tabs API (set by manager)
	paneProvider PaneProvider
}

// Alarm represents a scheduled alarm
type Alarm struct {
	Name          string            `json:"name"`
	ScheduledTime float64           `json:"scheduledTime"` // Unix timestamp in ms
	PeriodMinutes float64           `json:"periodInMinutes,omitempty"`
	sourceHandle  glib.SourceHandle // GLib timeout source handle
	backstop      *time.Timer       // Go timer fallback in case GLib callbacks don't run
	fireToken     uint64            // Monotonic counter to dedupe concurrent timers
}

// PaneProvider interface for getting tab/pane data from workspace
type PaneProvider interface {
	GetAllPanes() []api.PaneInfo
	GetActivePane() *api.PaneInfo
}

// NewBackgroundContext builds a background context for an extension.
func NewBackgroundContext(ext *Extension) *BackgroundContext {
	return &BackgroundContext{
		ext:              ext,
		runtimeOnMessage: newJSEvent(),
		runtimeOnConnect: newJSEvent(),
		storageOnChanged: newJSEvent(),
		webRequest: struct {
			onBeforeRequest     *jsEvent
			onBeforeSendHeaders *jsEvent
			onHeadersReceived   *jsEvent
			onResponseStarted   *jsEvent
			onCompleted         *jsEvent
			onErrorOccurred     *jsEvent
		}{
			onBeforeRequest:     newJSEvent(),
			onBeforeSendHeaders: newJSEvent(),
			onHeadersReceived:   newJSEvent(),
			onResponseStarted:   newJSEvent(),
			onCompleted:         newJSEvent(),
			onErrorOccurred:     newJSEvent(),
		},
		ports:        make(map[string]*backgroundPort),
		alarms:       make(map[string]*Alarm),
		alarmsEvent:  newJSEvent(),
		i18nMessages: make(map[string]I18nMessage),
	}
}

// SetPaneProvider sets the provider for tab/pane data
func (bc *BackgroundContext) SetPaneProvider(provider PaneProvider) {
	bc.mu.Lock()
	bc.paneProvider = provider
	bc.mu.Unlock()
}

// toJSSenderValue converts a MessageSender into a JS-friendly object with WebExtension field names.
func toJSSenderValue(vm *sobek.Runtime, sender api.MessageSender) sobek.Value {
	m := map[string]interface{}{
		"id":           sender.ID,
		"url":          sender.URL,
		"frameId":      sender.FrameID,
		"tlsChannelId": sender.TLSChannelID,
	}

	if sender.Tab != nil {
		tab := map[string]interface{}{
			"id":       sender.Tab.ID,
			"index":    sender.Tab.Index,
			"windowId": sender.Tab.WindowID,
			"url":      sender.Tab.URL,
			"title":    sender.Tab.Title,
			"active":   sender.Tab.Active,
		}
		if sender.Tab.Favicon != "" {
			tab["favIconUrl"] = sender.Tab.Favicon
		}
		m["tab"] = tab
	}

	return vm.ToValue(m)
}

// Start boots the Goja VM and loads background scripts.
func (bc *BackgroundContext) Start() error {
	bc.mu.Lock()
	if bc.tasks != nil {
		bc.mu.Unlock()
		return nil
	}
	bc.tasks = make(chan func(), 64)
	bc.ctx, bc.stop = context.WithCancel(context.Background())
	bc.vm = sobek.New()

	// Disable source map loading to avoid errors when source maps don't exist
	bc.vm.SetParserOptions(parser.WithDisableSourceMaps)

	// Load i18n translations
	if translations, err := LoadTranslationsForExtension(bc.ext); err == nil {
		bc.i18nMessages = translations.Messages
		bc.i18nLocale = translations.Locale
		log.Printf("[webext/bg %s] Loaded %d i18n messages for locale %s", bc.ext.ID, len(bc.i18nMessages), bc.i18nLocale)
	}

	// Initialize browser globals with script loader and persistent localStorage
	log.Printf("[webext/bg %s] LocalStorage backend: %v", bc.ext.ID, bc.ext.LocalStorage != nil)
	bc.browserGlob = globals.New(bc.vm, bc.ext, bc.tasks, bc.ext.LocalStorage)
	bc.browserGlob.SetScriptLoader(func(scriptPath string) error {
		// This is called from a goroutine, so we need to execute on the VM goroutine
		errCh := make(chan error, 1)
		bc.tasks <- func() {
			errCh <- bc.loadScript(scriptPath)
		}
		return <-errCh
	})

	bc.mu.Unlock()

	go bc.loop()

	return bc.call(func() error {
		if err := bc.installGlobals(); err != nil {
			return err
		}
		return bc.loadBackgroundScripts()
	})
}

// Stop tears down the background context.
func (bc *BackgroundContext) Stop() {
	bc.mu.Lock()
	if bc.stop != nil {
		bc.stop()
	}
	closeChan := bc.tasks
	bc.tasks = nil
	if bc.browserGlob != nil {
		bc.browserGlob.Cleanup()
	}
	// Cancel all pending alarms
	for _, alarm := range bc.alarms {
		bc.stopAlarmTimersLocked(alarm)
	}
	bc.alarms = nil
	bc.mu.Unlock()

	if closeChan != nil {
		close(closeChan)
	}
}

// SetPortConnector registers a callback that can create background-initiated ports.
func (bc *BackgroundContext) SetPortConnector(fn func(name string) (string, error)) {
	bc.mu.Lock()
	bc.connectExternal = fn
	bc.mu.Unlock()
}

// DispatchRuntimeMessage delivers a runtime.onMessage payload to the background script.
func (bc *BackgroundContext) DispatchRuntimeMessage(sender api.MessageSender, message interface{}) (interface{}, error) {
	// Check if this is a storage operation request from a content script
	// Content scripts can't access storage directly, so they message the background
	if msgMap, ok := message.(map[string]interface{}); ok {
		if msgType, hasType := msgMap["type"].(string); hasType {
			switch msgType {
			case "storage.get":
				// Handle storage.local.get request
				keys := msgMap["keys"]
				return bc.ext.Storage.Local().Get(keys)
			case "storage.set":
				// Handle storage.local.set request
				if items, ok := msgMap["items"].(map[string]interface{}); ok {
					return nil, bc.ext.Storage.Local().Set(items)
				}
				return nil, fmt.Errorf("storage.set requires 'items' object")
			case "storage.remove":
				// Handle storage.local.remove request
				keys := msgMap["keys"]
				return nil, bc.ext.Storage.Local().Remove(keys)
			case "storage.clear":
				// Handle storage.local.clear request
				return nil, bc.ext.Storage.Local().Clear()
			}
			// If type is not a storage operation, fall through to normal message dispatch
		}
	}

	// Normal runtime.onMessage dispatch for extension-defined message handlers
	var resp interface{}
	var err error
	if callErr := bc.call(func() error {
		if bc.runtimeOnMessage == nil {
			return nil
		}

		vm := bc.vm
		respVal, listenerErr := bc.runtimeOnMessage.dispatchWithResponse(vm, vm.ToValue(message), toJSSenderValue(vm, sender))
		if listenerErr != nil {
			err = listenerErr
			return nil
		}
		if respVal != nil {
			if exportErr := vm.ExportTo(respVal, &resp); exportErr != nil {
				err = exportErr
			}
		}
		return nil
	}); callErr != nil {
		return nil, callErr
	}
	return resp, err
}

// ConnectPort creates a new port inside the background context and triggers runtime.onConnect.
func (bc *BackgroundContext) ConnectPort(desc api.PortDescriptor) error {
	return bc.call(func() error {
		vm := bc.vm
		port := newBackgroundPort(vm, desc, bc)
		bc.ports[desc.ID] = port

		if bc.runtimeOnConnect != nil {
			_, err := bc.runtimeOnConnect.dispatch(vm, port.object)
			if err != nil {
				return err
			}
		}
		// Flush pending messages queued before the port got an ID.
		port.flushPending()
		return nil
	})
}

// DeliverPortMessage injects a port message coming from an external view.
func (bc *BackgroundContext) DeliverPortMessage(portID string, message interface{}) error {
	return bc.call(func() error {
		port := bc.ports[portID]
		if port == nil {
			return fmt.Errorf("port not found: %s", portID)
		}
		port.deliverFromExternal(message)
		return nil
	})
}

// DisconnectPort tears down a port and notifies listeners.
func (bc *BackgroundContext) DisconnectPort(portID string) error {
	return bc.call(func() error {
		port := bc.ports[portID]
		if port == nil {
			return nil
		}
		port.disconnectInternal()
		delete(bc.ports, portID)
		return nil
	})
}

// NotifyStorageChange dispatches storage.onChanged to the background.
func (bc *BackgroundContext) NotifyStorageChange(changes map[string]api.StorageChange, areaName string) {
	_ = bc.call(func() error {
		if bc.storageOnChanged == nil {
			return nil
		}
		vm := bc.vm
		_, _ = bc.storageOnChanged.dispatch(vm, vm.ToValue(changes), vm.ToValue(areaName))
		return nil
	})
}

// DispatchWebRequestEvent forwards a webRequest event into the VM and returns the first blocking response.
func (bc *BackgroundContext) DispatchWebRequestEvent(event string, payload interface{}) (*api.BlockingResponse, error) {
	start := time.Now()

	var evt *jsEvent
	isBlocking := false
	switch event {
	case "onBeforeRequest":
		evt = bc.webRequest.onBeforeRequest
		isBlocking = true
	case "onBeforeSendHeaders":
		evt = bc.webRequest.onBeforeSendHeaders
		isBlocking = true
	case "onHeadersReceived":
		evt = bc.webRequest.onHeadersReceived
		isBlocking = true
	case "onResponseStarted":
		evt = bc.webRequest.onResponseStarted
	case "onCompleted":
		evt = bc.webRequest.onCompleted
	case "onErrorOccurred":
		evt = bc.webRequest.onErrorOccurred
	default:
		return nil, fmt.Errorf("unsupported webRequest event: %s", event)
	}

	// Fast path: no listeners registered, skip expensive channel dispatch
	if !evt.hasListeners() {
		return nil, nil
	}

	// Convert payload to map with JSON tag names (not Go field names)
	jsonStart := time.Now()
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal webRequest payload: %w", err)
	}
	var jsPayload map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &jsPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal webRequest payload: %w", err)
	}
	jsonTime := time.Since(jsonStart)

	var resp *api.BlockingResponse
	var vmTime time.Duration
	callErr := bc.call(func() error {
		vmStart := time.Now()
		vm := bc.vm
		ret, dispatchErr := evt.dispatchWithResponse(vm, vm.ToValue(jsPayload))
		if dispatchErr != nil {
			return dispatchErr
		}
		if ret != nil && isBlocking {
			var blockingResp api.BlockingResponse
			if exportErr := vm.ExportTo(ret, &blockingResp); exportErr != nil {
				return exportErr
			}
			resp = &blockingResp
		}
		vmTime = time.Since(vmStart)
		return nil
	})

	total := time.Since(start)
	// Always log timing for debugging
	log.Printf("[webRequest-timing] total=%v json=%v vm=%v queue=%v", total, jsonTime, vmTime, total-jsonTime-vmTime)

	return resp, callErr
}

func (bc *BackgroundContext) loop() {
	for task := range bc.tasks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[webext/bg] recovered panic: %v\n%s", r, debug.Stack())
				}
			}()
			task()
		}()
	}
}

const alarmBackstopSlack = 500 * time.Millisecond

// scheduleAlarmTimer sets up a one-shot GLib timeout and a Go timer backstop for an alarm.
// The Go timer ensures the alarm fires even if the GLib main loop doesn't deliver callbacks.
func (bc *BackgroundContext) scheduleAlarmTimer(alarm *Alarm, name string, delay time.Duration) {
	if delay < time.Millisecond {
		delay = time.Millisecond
	}

	// Prepare new token and next scheduled time
	bc.mu.Lock()
	if alarm.sourceHandle != 0 {
		glib.SourceRemove(alarm.sourceHandle)
		alarm.sourceHandle = 0
	}
	if alarm.backstop != nil {
		alarm.backstop.Stop()
		alarm.backstop = nil
	}
	alarm.fireToken++
	token := alarm.fireToken
	alarm.ScheduledTime = float64(time.Now().Add(delay).UnixMilli())
	tasks := bc.tasks
	bc.mu.Unlock()

	if tasks == nil {
		return
	}

	delayMs := uint(delay.Milliseconds())
	handle := glib.TimeoutAdd(delayMs, func() bool {
		bc.handleAlarmFire(alarm, name, token)
		return false // one-shot; repeating alarms reschedule themselves
	})

	// Go timer backstop in case GLib callbacks don't run
	backstopDelay := delay + alarmBackstopSlack
	backstop := time.AfterFunc(backstopDelay, func() {
		bc.handleAlarmFire(alarm, name, token)
	})
	bc.mu.Lock()
	alarm.sourceHandle = handle
	alarm.backstop = backstop
	bc.mu.Unlock()
}

// handleAlarmFire dispatches an alarm if it is still active and matches the current token.
func (bc *BackgroundContext) handleAlarmFire(alarm *Alarm, name string, token uint64) {
	bc.mu.Lock()
	if bc.tasks == nil {
		bc.mu.Unlock()
		return
	}
	current, exists := bc.alarms[name]
	if !exists || current != alarm || alarm.fireToken != token {
		bc.mu.Unlock()
		return // stale or cleared
	}

	// Stop timers to avoid duplicate delivery
	bc.stopAlarmTimersLocked(alarm)

	tasks := bc.tasks
	alarmsEvent := bc.alarmsEvent
	vm := bc.vm
	periodMinutes := alarm.PeriodMinutes
	bc.mu.Unlock()

	// Queue JS callback
	tasks <- func() {
		log.Printf("[webext/bg %s] Firing alarm '%s'", bc.ext.ID, name)
		if alarmsEvent != nil && vm != nil {
			if _, err := alarmsEvent.dispatch(vm, bc.alarmToJS(alarm)); err != nil {
				log.Printf("[webext/bg %s] Alarm '%s' callback error: %v", bc.ext.ID, name, err)
			}
		}
	}

	// Reschedule repeating alarms
	if periodMinutes > 0 {
		nextDelay := time.Duration(periodMinutes * float64(time.Minute))
		bc.scheduleAlarmTimer(alarm, name, nextDelay)
		return
	}

	// One-shot: remove from map
	bc.mu.Lock()
	delete(bc.alarms, name)
	bc.mu.Unlock()
}

// stopAlarmTimersLocked stops GLib and Go timers for an alarm. Caller must hold bc.mu.
func (bc *BackgroundContext) stopAlarmTimersLocked(alarm *Alarm) {
	if alarm == nil {
		return
	}
	if alarm.sourceHandle != 0 {
		glib.SourceRemove(alarm.sourceHandle)
		alarm.sourceHandle = 0
	}
	if alarm.backstop != nil {
		alarm.backstop.Stop()
		alarm.backstop = nil
	}
}

func (bc *BackgroundContext) stopAlarmTimers(alarm *Alarm) {
	bc.mu.Lock()
	bc.stopAlarmTimersLocked(alarm)
	bc.mu.Unlock()
}

func (bc *BackgroundContext) alarmToJS(alarm *Alarm) *sobek.Object {
	vm := bc.vm
	if vm == nil || alarm == nil {
		return nil
	}
	alarmObj := vm.NewObject()
	_ = alarmObj.Set("name", alarm.Name)
	_ = alarmObj.Set("scheduledTime", alarm.ScheduledTime)
	if alarm.PeriodMinutes > 0 {
		_ = alarmObj.Set("periodInMinutes", alarm.PeriodMinutes)
	}
	return alarmObj
}

// call schedules fn on the VM goroutine and waits for completion.
func (bc *BackgroundContext) call(fn func() error) error {
	bc.mu.Lock()
	tasks := bc.tasks
	ctx := bc.ctx
	bc.mu.Unlock()
	if tasks == nil {
		return fmt.Errorf("background context not started")
	}

	errCh := make(chan error, 1)
	select {
	case tasks <- func() { errCh <- fn() }:
	case <-ctx.Done():
		return fmt.Errorf("background context stopped")
	}

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return fmt.Errorf("background context stopped")
	}
}

func (bc *BackgroundContext) installGlobals() error {
	vm := bc.vm

	// Install browser-like globals (document, setTimeout, fetch, etc.)
	if bc.browserGlob != nil {
		if err := bc.browserGlob.Install(); err != nil {
			return fmt.Errorf("install browser globals: %w", err)
		}
	}

	// Console with extension ID prefix
	console := vm.NewObject()
	logFn := func(level string) func(call sobek.FunctionCall) sobek.Value {
		return func(call sobek.FunctionCall) sobek.Value {
			args := make([]interface{}, 0, len(call.Arguments))
			for _, arg := range call.Arguments {
				args = append(args, arg.Export())
			}
			log.Printf("[webext/bg %s] [%s] %v", bc.ext.ID, level, args)
			return sobek.Undefined()
		}
	}
	_ = console.Set("log", logFn("LOG"))
	_ = console.Set("info", logFn("INFO"))
	_ = console.Set("warn", logFn("WARN"))
	_ = console.Set("error", logFn("ERROR"))
	_ = console.Set("debug", logFn("DEBUG"))
	_ = console.Set("trace", logFn("TRACE"))
	vm.Set("console", console)

	browser := vm.NewObject()
	runtimeObj, err := bc.buildRuntimeObject()
	if err != nil {
		return err
	}
	storageObj, err := bc.buildStorageObject()
	if err != nil {
		return err
	}
	webReqObj, err := bc.buildWebRequestObject()
	if err != nil {
		return err
	}

	browserActionObj := bc.buildBrowserActionObject()
	tabsObj := bc.buildTabsObject()
	i18nObj := bc.buildI18nObject()
	alarmsObj := bc.buildAlarmsObject()
	scriptingObj := bc.buildScriptingObject()
	webNavObj := bc.buildWebNavigationObject()
	contextMenusObj := bc.buildContextMenusObject()
	idleObj := bc.buildIdleObject()
	notificationsObj := bc.buildNotificationsObject()
	commandsObj := bc.buildCommandsObject()
	permissionsObj := bc.buildPermissionsObject()
	extensionObj := bc.buildExtensionObject()
	windowsObj := bc.buildWindowsObject()

	_ = browser.Set("runtime", runtimeObj)
	_ = browser.Set("storage", storageObj)
	_ = browser.Set("webRequest", webReqObj)
	_ = browser.Set("browserAction", browserActionObj)
	_ = browser.Set("action", browserActionObj) // MV3 alias
	_ = browser.Set("tabs", tabsObj)
	_ = browser.Set("i18n", i18nObj)
	_ = browser.Set("alarms", alarmsObj)
	_ = browser.Set("scripting", scriptingObj)
	_ = browser.Set("webNavigation", webNavObj)
	_ = browser.Set("contextMenus", contextMenusObj)
	_ = browser.Set("menus", contextMenusObj) // Firefox alias
	_ = browser.Set("idle", idleObj)
	_ = browser.Set("notifications", notificationsObj)
	_ = browser.Set("commands", commandsObj)
	_ = browser.Set("permissions", permissionsObj)
	_ = browser.Set("extension", extensionObj)
	_ = browser.Set("windows", windowsObj)

	// Chrome aliases.
	vm.Set("browser", browser)
	vm.Set("chrome", browser)
	return nil
}

// buildBrowserActionObject creates stubs for browser.browserAction API.
func (bc *BackgroundContext) buildBrowserActionObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// setIcon - stub that returns a resolved promise
	_ = obj.Set("setIcon", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// setTitle - stub
	_ = obj.Set("setTitle", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// setBadgeText - stub
	_ = obj.Set("setBadgeText", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// setBadgeBackgroundColor - stub
	_ = obj.Set("setBadgeBackgroundColor", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// setBadgeTextColor - stub
	_ = obj.Set("setBadgeTextColor", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// getPopup - stub
	_ = obj.Set("getPopup", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue("")) }
		}()
		return vm.ToValue(promise)
	})

	// setPopup - stub
	_ = obj.Set("setPopup", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// onClicked event
	onClicked := vm.NewObject()
	_ = onClicked.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = onClicked.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = onClicked.Set("hasListener", func(sobek.FunctionCall) sobek.Value { return vm.ToValue(false) })
	_ = obj.Set("onClicked", onClicked)

	return obj
}

// buildTabsObject creates browser.tabs API with real pane data.
func (bc *BackgroundContext) buildTabsObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// Helper to convert PaneInfo to tab object
	paneToTab := func(pane api.PaneInfo) *sobek.Object {
		tab := vm.NewObject()
		_ = tab.Set("id", int(pane.ID))
		_ = tab.Set("index", pane.Index)
		_ = tab.Set("windowId", int(pane.WindowID))
		_ = tab.Set("active", pane.Active)
		_ = tab.Set("url", pane.URL)
		_ = tab.Set("title", pane.Title)
		_ = tab.Set("status", "complete") // Could track loading state
		_ = tab.Set("incognito", false)
		_ = tab.Set("pinned", false)
		return tab
	}

	// query - returns tabs matching query
	_ = obj.Set("query", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()

		// Extract query options BEFORE starting goroutine (call.Arguments not safe in goroutine)
		var queryActive, queryCurrentWindow *bool
		var queryWindowID int64 = -1

		if len(call.Arguments) > 0 && call.Arguments[0] != nil && !sobek.IsUndefined(call.Arguments[0]) {
			queryObj := call.Arguments[0].Export()
			if q, ok := queryObj.(map[string]interface{}); ok {
				if v, ok := q["active"].(bool); ok {
					queryActive = &v
				}
				if v, ok := q["currentWindow"].(bool); ok {
					queryCurrentWindow = &v
				}
				if v, ok := q["windowId"].(int64); ok {
					queryWindowID = v
				} else if v, ok := q["windowId"].(float64); ok {
					queryWindowID = int64(v)
				}
			}
		}

		go func() {
			bc.tasks <- func() {
				var results []interface{}

				bc.mu.Lock()
				provider := bc.paneProvider
				bc.mu.Unlock()

				if provider == nil {
					_ = resolve(vm.ToValue(results))
					return
				}

				panes := provider.GetAllPanes()
				active := provider.GetActivePane()

				// Get current window ID
				var currentWindowID uint64
				if active != nil {
					currentWindowID = active.WindowID
				}

				for _, pane := range panes {
					// Filter by active
					if queryActive != nil && *queryActive != pane.Active {
						continue
					}
					// Filter by currentWindow
					if queryCurrentWindow != nil && *queryCurrentWindow && pane.WindowID != currentWindowID {
						continue
					}
					// Filter by windowId
					if queryWindowID >= 0 && uint64(queryWindowID) != pane.WindowID {
						continue
					}
					results = append(results, paneToTab(pane))
				}

				_ = resolve(vm.ToValue(results))
			}
		}()
		return vm.ToValue(promise)
	})

	// get - returns tab by ID
	_ = obj.Set("get", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()

		// Extract tabID BEFORE goroutine (call.Arguments not safe in goroutine)
		var tabID int64 = -1
		if len(call.Arguments) > 0 && call.Arguments[0] != nil && !sobek.IsUndefined(call.Arguments[0]) {
			tabID = call.Arguments[0].ToInteger()
		}
		log.Printf("[tabs.get DEBUG] Called with tabID=%d", tabID)

		go func() {
			bc.tasks <- func() {
				if tabID < 0 {
					log.Printf("[tabs.get DEBUG] tabID < 0, returning undefined")
					_ = resolve(sobek.Undefined())
					return
				}

				bc.mu.Lock()
				provider := bc.paneProvider
				bc.mu.Unlock()

				if provider == nil {
					log.Printf("[tabs.get DEBUG] paneProvider is nil, returning undefined")
					_ = resolve(sobek.Undefined())
					return
				}

				allPanes := provider.GetAllPanes()
				log.Printf("[tabs.get DEBUG] Found %d panes", len(allPanes))
				for _, pane := range allPanes {
					log.Printf("[tabs.get DEBUG] Pane ID=%d URL=%q", pane.ID, pane.URL)
					if int64(pane.ID) == tabID {
						log.Printf("[tabs.get DEBUG] Found matching pane! Returning tab with URL=%q", pane.URL)
						_ = resolve(paneToTab(pane))
						return
					}
				}
				log.Printf("[tabs.get DEBUG] No pane found with ID=%d", tabID)
				_ = resolve(sobek.Undefined())
			}
		}()
		return vm.ToValue(promise)
	})

	// getCurrent - returns the current tab (active pane)
	_ = obj.Set("getCurrent", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				bc.mu.Lock()
				provider := bc.paneProvider
				bc.mu.Unlock()

				if provider == nil {
					_ = resolve(sobek.Undefined())
					return
				}

				active := provider.GetActivePane()
				if active != nil {
					_ = resolve(paneToTab(*active))
				} else {
					_ = resolve(sobek.Undefined())
				}
			}
		}()
		return vm.ToValue(promise)
	})

	// create - stub
	_ = obj.Set("create", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				tab := vm.NewObject()
				_ = tab.Set("id", 1)
				_ = tab.Set("url", "")
				_ = resolve(tab)
			}
		}()
		return vm.ToValue(promise)
	})

	// update - stub
	_ = obj.Set("update", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// remove - stub
	_ = obj.Set("remove", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// sendMessage - stub
	_ = obj.Set("sendMessage", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// insertCSS - stub
	_ = obj.Set("insertCSS", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// removeCSS - stub
	_ = obj.Set("removeCSS", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// executeScript - stub
	_ = obj.Set("executeScript", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue([]interface{}{})) }
		}()
		return vm.ToValue(promise)
	})

	// Constants
	_ = obj.Set("TAB_ID_NONE", -1)

	// Events
	for _, eventName := range []string{"onCreated", "onUpdated", "onRemoved", "onActivated", "onReplaced"} {
		evt := vm.NewObject()
		_ = evt.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = evt.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = evt.Set("hasListener", func(sobek.FunctionCall) sobek.Value { return vm.ToValue(false) })
		_ = obj.Set(eventName, evt)
	}

	return obj
}

// buildI18nObject creates browser.i18n API.
func (bc *BackgroundContext) buildI18nObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// getMessage - returns the message or the key if not found
	_ = obj.Set("getMessage", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		key := call.Arguments[0].String()

		// Handle predefined messages
		switch key {
		case "@@extension_id":
			return vm.ToValue(bc.ext.ID)
		case "@@ui_locale":
			return vm.ToValue(bc.i18nLocale)
		case "@@bidi_dir":
			return vm.ToValue("ltr")
		case "@@bidi_reversed_dir":
			return vm.ToValue("rtl")
		case "@@bidi_start_edge":
			return vm.ToValue("left")
		case "@@bidi_end_edge":
			return vm.ToValue("right")
		}

		// Look up in loaded messages
		msg, ok := bc.i18nMessages[key]
		if !ok {
			return vm.ToValue("") // Return empty string like Firefox
		}

		result := msg.Message

		// Handle substitutions ($1, $2, etc.)
		if len(call.Arguments) > 1 {
			var subs []string
			arg1 := call.Arguments[1].Export()
			switch v := arg1.(type) {
			case []interface{}:
				for _, s := range v {
					subs = append(subs, fmt.Sprintf("%v", s))
				}
			case string:
				subs = []string{v}
			}
			for i, sub := range subs {
				placeholder := fmt.Sprintf("$%d", i+1)
				result = strings.ReplaceAll(result, placeholder, sub)
			}
		}

		return vm.ToValue(result)
	})

	// getUILanguage
	_ = obj.Set("getUILanguage", func(sobek.FunctionCall) sobek.Value {
		return vm.ToValue(bc.i18nLocale)
	})

	// getAcceptLanguages
	_ = obj.Set("getAcceptLanguages", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				langs := []string{bc.i18nLocale}
				if bc.i18nLocale != "en" {
					langs = append(langs, "en")
				}
				_ = resolve(vm.ToValue(langs))
			}
		}()
		return vm.ToValue(promise)
	})

	// detectLanguage - stub
	_ = obj.Set("detectLanguage", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				result := vm.NewObject()
				_ = result.Set("isReliable", false)
				_ = result.Set("languages", []interface{}{})
				_ = resolve(result)
			}
		}()
		return vm.ToValue(promise)
	})

	return obj
}

// buildAlarmsObject creates browser.alarms API with Go timer implementation.
func (bc *BackgroundContext) buildAlarmsObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// create - schedules a new alarm
	_ = obj.Set("create", func(call sobek.FunctionCall) sobek.Value {
		var name string
		var alarmInfo map[string]interface{}

		// Parse arguments: create(name?, alarmInfo)
		if len(call.Arguments) >= 2 {
			name = call.Arguments[0].String()
			if err := vm.ExportTo(call.Arguments[1], &alarmInfo); err != nil {
				name = ""
				_ = vm.ExportTo(call.Arguments[0], &alarmInfo)
			}
		} else if len(call.Arguments) == 1 {
			_ = vm.ExportTo(call.Arguments[0], &alarmInfo)
		}

		// Default name
		if name == "" {
			name = ""
		}

		// Cancel existing alarm with same name
		bc.mu.Lock()
		if existing, ok := bc.alarms[name]; ok {
			if existing.sourceHandle != 0 {
				glib.SourceRemove(existing.sourceHandle)
			}
			delete(bc.alarms, name)
		}
		bc.mu.Unlock()

		// Parse alarm info - JS numbers may export as int64 or float64
		var delayMinutes, periodMinutes, when float64

		delayMinutes = toFloat64(alarmInfo["delayInMinutes"])
		periodMinutes = toFloat64(alarmInfo["periodInMinutes"])
		when = toFloat64(alarmInfo["when"])

		// Calculate delay
		var delay time.Duration
		now := time.Now()

		if when > 0 {
			// "when" is Unix timestamp in milliseconds
			targetTime := time.UnixMilli(int64(when))
			delay = time.Until(targetTime)
			if delay < 0 {
				delay = 0 // Fire immediately if time has passed
			}
		} else if delayMinutes > 0 {
			delay = time.Duration(delayMinutes * float64(time.Minute))
		} else if periodMinutes > 0 {
			delay = time.Duration(periodMinutes * float64(time.Minute))
		} else {
			// Default minimum delay (1 minute as per spec)
			delay = time.Minute
		}

		// Create alarm
		alarm := &Alarm{
			Name:          name,
			ScheduledTime: float64(now.Add(delay).UnixMilli()),
			PeriodMinutes: periodMinutes,
		}

		// Store alarm in map BEFORE starting timer to avoid race condition
		bc.mu.Lock()
		bc.alarms[name] = alarm
		bc.mu.Unlock()

		// Schedule timer with GLib (plus Go fallback)
		bc.scheduleAlarmTimer(alarm, name, delay)

		log.Printf("[webext/bg %s] Created alarm '%s' (delay: %v, period: %v min)", bc.ext.ID, name, delay, periodMinutes)
		return sobek.Undefined()
	})

	// get - returns alarm by name
	_ = obj.Set("get", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()

		// Extract name BEFORE goroutine (call.Arguments not safe in goroutine)
		var name string
		if len(call.Arguments) > 0 && call.Arguments[0] != nil && !sobek.IsUndefined(call.Arguments[0]) {
			name = call.Arguments[0].String()
		}

		go func() {
			bc.tasks <- func() {
				bc.mu.Lock()
				alarm, ok := bc.alarms[name]
				bc.mu.Unlock()

				if ok {
					_ = resolve(bc.alarmToJS(alarm))
				} else {
					_ = resolve(sobek.Undefined())
				}
			}
		}()
		return vm.ToValue(promise)
	})

	// getAll - returns all alarms
	_ = obj.Set("getAll", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				var results []interface{}

				bc.mu.Lock()
				for _, alarm := range bc.alarms {
					results = append(results, bc.alarmToJS(alarm))
				}
				bc.mu.Unlock()

				_ = resolve(vm.ToValue(results))
			}
		}()
		return vm.ToValue(promise)
	})

	// clear - cancels alarm by name
	_ = obj.Set("clear", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()

		// Extract name BEFORE goroutine (call.Arguments not safe in goroutine)
		var name string
		if len(call.Arguments) > 0 && call.Arguments[0] != nil && !sobek.IsUndefined(call.Arguments[0]) {
			name = call.Arguments[0].String()
		}

		go func() {
			bc.tasks <- func() {
				bc.mu.Lock()
				alarm, ok := bc.alarms[name]
				if ok {
					bc.stopAlarmTimersLocked(alarm)
					delete(bc.alarms, name)
				}
				bc.mu.Unlock()

				_ = resolve(vm.ToValue(ok))
			}
		}()
		return vm.ToValue(promise)
	})

	// clearAll - cancels all alarms
	_ = obj.Set("clearAll", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				bc.mu.Lock()
				for _, alarm := range bc.alarms {
					bc.stopAlarmTimersLocked(alarm)
				}
				bc.alarms = make(map[string]*Alarm)
				bc.mu.Unlock()

				_ = resolve(vm.ToValue(true))
			}
		}()
		return vm.ToValue(promise)
	})

	// onAlarm event
	onAlarm := vm.NewObject()
	_ = onAlarm.Set("addListener", func(call sobek.FunctionCall) sobek.Value {
		if bc.alarmsEvent != nil && len(call.Arguments) > 0 {
			_ = bc.alarmsEvent.add(vm, call.Arguments[0])
		}
		return sobek.Undefined()
	})
	_ = onAlarm.Set("removeListener", func(call sobek.FunctionCall) sobek.Value {
		if bc.alarmsEvent != nil && len(call.Arguments) > 0 {
			bc.alarmsEvent.remove(vm, call.Arguments[0])
		}
		return sobek.Undefined()
	})
	_ = onAlarm.Set("hasListener", func(call sobek.FunctionCall) sobek.Value {
		if bc.alarmsEvent == nil || len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		return vm.ToValue(bc.alarmsEvent.has(vm, call.Arguments[0]))
	})
	_ = obj.Set("onAlarm", onAlarm)

	return obj
}

// buildScriptingObject creates browser.scripting API stubs (MV3).
func (bc *BackgroundContext) buildScriptingObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// executeScript - stub
	_ = obj.Set("executeScript", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue([]interface{}{})) }
		}()
		return vm.ToValue(promise)
	})

	// insertCSS - stub
	_ = obj.Set("insertCSS", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// removeCSS - stub
	_ = obj.Set("removeCSS", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// registerContentScripts - stub
	_ = obj.Set("registerContentScripts", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// getRegisteredContentScripts - stub
	_ = obj.Set("getRegisteredContentScripts", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue([]interface{}{})) }
		}()
		return vm.ToValue(promise)
	})

	// unregisterContentScripts - stub
	_ = obj.Set("unregisterContentScripts", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	return obj
}

// buildWebNavigationObject creates stubs for browser.webNavigation API.
// This is needed by uBlock Origin for tab navigation tracking.
func (bc *BackgroundContext) buildWebNavigationObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// Event objects with addListener/removeListener/hasListener
	eventNames := []string{
		"onBeforeNavigate",
		"onCommitted",
		"onDOMContentLoaded",
		"onCompleted",
		"onErrorOccurred",
		"onCreatedNavigationTarget",
		"onReferenceFragmentUpdated",
		"onTabReplaced",
		"onHistoryStateUpdated",
	}

	for _, name := range eventNames {
		eventObj := vm.NewObject()
		_ = eventObj.Set("addListener", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		_ = eventObj.Set("removeListener", func(sobek.FunctionCall) sobek.Value {
			return sobek.Undefined()
		})
		_ = eventObj.Set("hasListener", func(sobek.FunctionCall) sobek.Value {
			return vm.ToValue(false)
		})
		_ = obj.Set(name, eventObj)
	}

	// getFrame - returns null frame info
	_ = obj.Set("getFrame", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Null()) }
		}()
		return vm.ToValue(promise)
	})

	// getAllFrames - returns empty array
	_ = obj.Set("getAllFrames", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue([]interface{}{})) }
		}()
		return vm.ToValue(promise)
	})

	return obj
}

// buildContextMenusObject creates browser.contextMenus API stubs.
func (bc *BackgroundContext) buildContextMenusObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()
	menuIDCounter := 0

	// create - returns an ID
	_ = obj.Set("create", func(call sobek.FunctionCall) sobek.Value {
		menuIDCounter++
		return vm.ToValue(menuIDCounter)
	})

	// update - stub
	_ = obj.Set("update", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// remove - stub
	_ = obj.Set("remove", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// removeAll - stub
	_ = obj.Set("removeAll", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// onClicked event
	onClicked := vm.NewObject()
	_ = onClicked.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = onClicked.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = onClicked.Set("hasListener", func(sobek.FunctionCall) sobek.Value { return vm.ToValue(false) })
	_ = obj.Set("onClicked", onClicked)

	// Constants for context types
	_ = obj.Set("ACTION_MENU_TOP_LEVEL_LIMIT", 6)

	return obj
}

// buildIdleObject creates browser.idle API stubs.
func (bc *BackgroundContext) buildIdleObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// queryState - always returns "active"
	_ = obj.Set("queryState", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue("active")) }
		}()
		return vm.ToValue(promise)
	})

	// setDetectionInterval - stub
	_ = obj.Set("setDetectionInterval", func(sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})

	// getAutoLockDelay - stub
	_ = obj.Set("getAutoLockDelay", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(0)) }
		}()
		return vm.ToValue(promise)
	})

	// onStateChanged event
	onStateChanged := vm.NewObject()
	_ = onStateChanged.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = onStateChanged.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = onStateChanged.Set("hasListener", func(sobek.FunctionCall) sobek.Value { return vm.ToValue(false) })
	_ = obj.Set("onStateChanged", onStateChanged)

	return obj
}

// buildNotificationsObject creates browser.notifications API stubs.
func (bc *BackgroundContext) buildNotificationsObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// create - stub
	_ = obj.Set("create", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		var notifID string
		if len(call.Arguments) > 0 {
			notifID = call.Arguments[0].String()
		} else {
			notifID = fmt.Sprintf("notif-%d", time.Now().UnixNano())
		}
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(notifID)) }
		}()
		return vm.ToValue(promise)
	})

	// clear - stub
	_ = obj.Set("clear", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(true)) }
		}()
		return vm.ToValue(promise)
	})

	// getAll - stub (empty)
	_ = obj.Set("getAll", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(map[string]interface{}{})) }
		}()
		return vm.ToValue(promise)
	})

	// update - stub
	_ = obj.Set("update", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(true)) }
		}()
		return vm.ToValue(promise)
	})

	// Events
	for _, eventName := range []string{"onClicked", "onButtonClicked", "onClosed", "onShown"} {
		evt := vm.NewObject()
		_ = evt.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = evt.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = evt.Set("hasListener", func(sobek.FunctionCall) sobek.Value { return vm.ToValue(false) })
		_ = obj.Set(eventName, evt)
	}

	return obj
}

// buildCommandsObject creates browser.commands API stubs.
func (bc *BackgroundContext) buildCommandsObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// getAll - returns manifest commands
	_ = obj.Set("getAll", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				// Return empty for now, could be enhanced to return actual manifest commands
				_ = resolve(vm.ToValue([]interface{}{}))
			}
		}()
		return vm.ToValue(promise)
	})

	// reset - stub
	_ = obj.Set("reset", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// update - stub
	_ = obj.Set("update", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// onCommand event
	onCommand := vm.NewObject()
	_ = onCommand.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = onCommand.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
	_ = onCommand.Set("hasListener", func(sobek.FunctionCall) sobek.Value { return vm.ToValue(false) })
	_ = obj.Set("onCommand", onCommand)

	return obj
}

// buildPermissionsObject creates browser.permissions API stubs.
func (bc *BackgroundContext) buildPermissionsObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// getAll - returns permissions from manifest
	_ = obj.Set("getAll", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				result := vm.NewObject()
				_ = result.Set("permissions", bc.ext.Manifest.Permissions)
				_ = result.Set("origins", []string{})
				_ = resolve(result)
			}
		}()
		return vm.ToValue(promise)
	})

	// contains - check if permission is granted
	_ = obj.Set("contains", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(true)) } // Assume all declared permissions are granted
		}()
		return vm.ToValue(promise)
	})

	// request - stub (always succeeds)
	_ = obj.Set("request", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(true)) }
		}()
		return vm.ToValue(promise)
	})

	// remove - stub
	_ = obj.Set("remove", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(true)) }
		}()
		return vm.ToValue(promise)
	})

	// Events
	for _, eventName := range []string{"onAdded", "onRemoved"} {
		evt := vm.NewObject()
		_ = evt.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = evt.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = evt.Set("hasListener", func(sobek.FunctionCall) sobek.Value { return vm.ToValue(false) })
		_ = obj.Set(eventName, evt)
	}

	return obj
}

// buildExtensionObject creates browser.extension API stubs (deprecated but still used).
func (bc *BackgroundContext) buildExtensionObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// getURL - same as runtime.getURL
	_ = obj.Set("getURL", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		path := call.Arguments[0].String()
		return vm.ToValue(fmt.Sprintf("dumb-extension://%s/%s", bc.ext.ID, path))
	})

	// getBackgroundPage - returns null (can't return window in Goja)
	_ = obj.Set("getBackgroundPage", func(sobek.FunctionCall) sobek.Value {
		return sobek.Null()
	})

	// getViews - returns empty array (no views in headless context)
	_ = obj.Set("getViews", func(sobek.FunctionCall) sobek.Value {
		return vm.ToValue([]interface{}{})
	})

	// isAllowedIncognitoAccess - returns false
	_ = obj.Set("isAllowedIncognitoAccess", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(false)) }
		}()
		return vm.ToValue(promise)
	})

	// isAllowedFileSchemeAccess - returns false
	_ = obj.Set("isAllowedFileSchemeAccess", func(sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(vm.ToValue(false)) }
		}()
		return vm.ToValue(promise)
	})

	// inIncognitoContext
	_ = obj.Set("inIncognitoContext", false)

	return obj
}

// buildWindowsObject creates browser.windows API stubs.
func (bc *BackgroundContext) buildWindowsObject() *sobek.Object {
	vm := bc.vm
	obj := vm.NewObject()

	// get - returns undefined
	_ = obj.Set("get", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// getCurrent - returns a minimal window object
	_ = obj.Set("getCurrent", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				win := vm.NewObject()
				_ = win.Set("id", 1)
				_ = win.Set("focused", true)
				_ = win.Set("alwaysOnTop", false)
				_ = win.Set("incognito", false)
				_ = win.Set("type", "normal")
				_ = win.Set("state", "normal")
				_ = resolve(win)
			}
		}()
		return vm.ToValue(promise)
	})

	// getLastFocused - same as getCurrent
	_ = obj.Set("getLastFocused", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				win := vm.NewObject()
				_ = win.Set("id", 1)
				_ = win.Set("focused", true)
				_ = win.Set("alwaysOnTop", false)
				_ = win.Set("incognito", false)
				_ = win.Set("type", "normal")
				_ = win.Set("state", "normal")
				_ = resolve(win)
			}
		}()
		return vm.ToValue(promise)
	})

	// getAll - returns single window
	_ = obj.Set("getAll", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				win := vm.NewObject()
				_ = win.Set("id", 1)
				_ = win.Set("focused", true)
				_ = win.Set("alwaysOnTop", false)
				_ = win.Set("incognito", false)
				_ = win.Set("type", "normal")
				_ = win.Set("state", "normal")
				_ = resolve(vm.ToValue([]interface{}{win}))
			}
		}()
		return vm.ToValue(promise)
	})

	// create - stub
	_ = obj.Set("create", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() {
				win := vm.NewObject()
				_ = win.Set("id", 2)
				_ = win.Set("focused", true)
				_ = resolve(win)
			}
		}()
		return vm.ToValue(promise)
	})

	// update - stub
	_ = obj.Set("update", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// remove - stub
	_ = obj.Set("remove", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			bc.tasks <- func() { _ = resolve(sobek.Undefined()) }
		}()
		return vm.ToValue(promise)
	})

	// Constants
	_ = obj.Set("WINDOW_ID_NONE", -1)
	_ = obj.Set("WINDOW_ID_CURRENT", -2)

	// Events
	for _, eventName := range []string{"onCreated", "onRemoved", "onFocusChanged"} {
		evt := vm.NewObject()
		_ = evt.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = evt.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = evt.Set("hasListener", func(sobek.FunctionCall) sobek.Value { return vm.ToValue(false) })
		_ = obj.Set(eventName, evt)
	}

	return obj
}

func (bc *BackgroundContext) buildRuntimeObject() (*sobek.Object, error) {
	vm := bc.vm
	obj := vm.NewObject()

	// Events with actual dispatch capability
	events := map[string]*jsEvent{
		"onMessage": bc.runtimeOnMessage,
		"onConnect": bc.runtimeOnConnect,
	}

	for name, evt := range events {
		eventObj := vm.NewObject()
		_ = eventObj.Set("addListener", func(call sobek.FunctionCall) sobek.Value {
			if evt == nil || len(call.Arguments) == 0 {
				return sobek.Undefined()
			}
			_ = evt.add(vm, call.Arguments[0])
			return sobek.Undefined()
		})
		_ = eventObj.Set("removeListener", func(call sobek.FunctionCall) sobek.Value {
			if evt == nil || len(call.Arguments) == 0 {
				return sobek.Undefined()
			}
			evt.remove(vm, call.Arguments[0])
			return sobek.Undefined()
		})
		_ = eventObj.Set("hasListener", func(call sobek.FunctionCall) sobek.Value {
			if evt == nil || len(call.Arguments) == 0 {
				return vm.ToValue(false)
			}
			return vm.ToValue(evt.has(vm, call.Arguments[0]))
		})
		_ = obj.Set(name, eventObj)
	}

	// Stub events (listen but never fire)
	stubEvents := []string{
		"onInstalled",
		"onStartup",
		"onSuspend",
		"onSuspendCanceled",
		"onUpdateAvailable",
		"onBrowserUpdateAvailable",
		"onConnectExternal",
		"onMessageExternal",
		"onRestartRequired",
	}
	for _, eventName := range stubEvents {
		eventObj := vm.NewObject()
		_ = eventObj.Set("addListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = eventObj.Set("removeListener", func(sobek.FunctionCall) sobek.Value { return sobek.Undefined() })
		_ = eventObj.Set("hasListener", func(sobek.FunctionCall) sobek.Value { return vm.ToValue(false) })
		_ = obj.Set(eventName, eventObj)
	}

	// runtime.id - extension ID
	_ = obj.Set("id", bc.ext.ID)

	// runtime.lastError - null (no error)
	_ = obj.Set("lastError", sobek.Null())

	_ = obj.Set("getManifest", func(sobek.FunctionCall) sobek.Value {
		if bc.ext.Manifest == nil {
			return sobek.Null()
		}
		// Marshal to JSON and back to map to get proper JSON field names
		// (Go struct field names don't match JSON keys expected by extensions)
		data, err := json.Marshal(bc.ext.Manifest)
		if err != nil {
			log.Printf("[webext/bg %s] getManifest marshal error: %v", bc.ext.ID, err)
			return sobek.Null()
		}
		var manifestMap map[string]interface{}
		if err := json.Unmarshal(data, &manifestMap); err != nil {
			log.Printf("[webext/bg %s] getManifest unmarshal error: %v", bc.ext.ID, err)
			return sobek.Null()
		}
		return vm.ToValue(manifestMap)
	})

	_ = obj.Set("getURL", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue("")
		}
		path := call.Arguments[0].String()
		return vm.ToValue(fmt.Sprintf("dumb-extension://%s/%s", bc.ext.ID, path))
	})

	_ = obj.Set("sendMessage", func(call sobek.FunctionCall) sobek.Value {
		// Background -> other contexts messaging is not implemented yet.
		log.Printf("[webext/bg %s] runtime.sendMessage stub called (ignored)", bc.ext.ID)
		return sobek.Undefined()
	})

	_ = obj.Set("connect", func(call sobek.FunctionCall) sobek.Value {
		if bc.connectExternal == nil {
			log.Printf("[webext/bg %s] runtime.connect stub called (no connector)", bc.ext.ID)
			return sobek.Undefined()
		}
		var name string
		if len(call.Arguments) > 0 {
			name = call.Arguments[0].String()
		}
		portID, err := bc.connectExternal(name)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		port := bc.ports[portID]
		if port == nil {
			return sobek.Undefined()
		}
		return port.object
	})

	return obj, nil
}

func (bc *BackgroundContext) buildStorageObject() (*sobek.Object, error) {
	vm := bc.vm
	obj := vm.NewObject()
	local := vm.NewObject()

	_ = local.Set("get", func(call sobek.FunctionCall) sobek.Value {
		if bc.ext.Storage == nil {
			return sobek.Undefined()
		}
		var keys interface{}
		if len(call.Arguments) > 0 {
			keys = call.Arguments[0].Export()
		}
		items, err := bc.ext.Storage.Local().Get(keys)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return vm.ToValue(items)
	})

	_ = local.Set("set", func(call sobek.FunctionCall) sobek.Value {
		if bc.ext.Storage == nil || len(call.Arguments) == 0 {
			return sobek.Undefined()
		}
		var payload map[string]interface{}
		if err := vm.ExportTo(call.Arguments[0], &payload); err != nil {
			panic(vm.ToValue(err.Error()))
		}
		if err := bc.ext.Storage.Local().Set(payload); err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return sobek.Undefined()
	})

	_ = local.Set("remove", func(call sobek.FunctionCall) sobek.Value {
		if bc.ext.Storage == nil || len(call.Arguments) == 0 {
			return sobek.Undefined()
		}
		keys := call.Arguments[0].Export()
		if err := bc.ext.Storage.Local().Remove(keys); err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return sobek.Undefined()
	})

	_ = local.Set("clear", func(sobek.FunctionCall) sobek.Value {
		if bc.ext.Storage == nil {
			return sobek.Undefined()
		}
		if err := bc.ext.Storage.Local().Clear(); err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return sobek.Undefined()
	})

	eventObj := vm.NewObject()
	_ = eventObj.Set("addListener", func(call sobek.FunctionCall) sobek.Value {
		if bc.storageOnChanged != nil && len(call.Arguments) > 0 {
			_ = bc.storageOnChanged.add(vm, call.Arguments[0])
		}
		return sobek.Undefined()
	})
	_ = eventObj.Set("removeListener", func(call sobek.FunctionCall) sobek.Value {
		if bc.storageOnChanged != nil && len(call.Arguments) > 0 {
			bc.storageOnChanged.remove(vm, call.Arguments[0])
		}
		return sobek.Undefined()
	})
	_ = eventObj.Set("hasListener", func(call sobek.FunctionCall) sobek.Value {
		if bc.storageOnChanged == nil || len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		return vm.ToValue(bc.storageOnChanged.has(vm, call.Arguments[0]))
	})

	_ = local.Set("onChanged", eventObj)
	_ = obj.Set("local", local)
	return obj, nil
}

func (bc *BackgroundContext) buildWebRequestObject() (*sobek.Object, error) {
	vm := bc.vm
	obj := vm.NewObject()

	events := map[string]*jsEvent{
		"onBeforeRequest":     bc.webRequest.onBeforeRequest,
		"onBeforeSendHeaders": bc.webRequest.onBeforeSendHeaders,
		"onHeadersReceived":   bc.webRequest.onHeadersReceived,
		"onResponseStarted":   bc.webRequest.onResponseStarted,
		"onCompleted":         bc.webRequest.onCompleted,
		"onErrorOccurred":     bc.webRequest.onErrorOccurred,
	}

	for name, evt := range events {
		eventObj := vm.NewObject()
		_ = eventObj.Set("addListener", func(target *jsEvent) func(call sobek.FunctionCall) sobek.Value {
			return func(call sobek.FunctionCall) sobek.Value {
				if target != nil && len(call.Arguments) > 0 {
					_ = target.add(vm, call.Arguments[0])
				}
				return sobek.Undefined()
			}
		}(evt))
		_ = eventObj.Set("removeListener", func(target *jsEvent) func(call sobek.FunctionCall) sobek.Value {
			return func(call sobek.FunctionCall) sobek.Value {
				if target != nil && len(call.Arguments) > 0 {
					target.remove(vm, call.Arguments[0])
				}
				return sobek.Undefined()
			}
		}(evt))
		_ = eventObj.Set("hasListener", func(target *jsEvent) func(call sobek.FunctionCall) sobek.Value {
			return func(call sobek.FunctionCall) sobek.Value {
				if target == nil || len(call.Arguments) == 0 {
					return vm.ToValue(false)
				}
				return vm.ToValue(target.has(vm, call.Arguments[0]))
			}
		}(evt))

		_ = obj.Set(name, eventObj)
	}

	return obj, nil
}

func (bc *BackgroundContext) loadBackgroundScripts() error {
	if bc.ext == nil || bc.ext.Manifest == nil || bc.ext.Manifest.Background == nil {
		return nil
	}

	bg := bc.ext.Manifest.Background

	// Collect scripts to load - either from manifest or by parsing background.page HTML
	var scripts []ScriptInfo

	if len(bg.Scripts) > 0 {
		// Use scripts directly from manifest (these are regular scripts, not modules)
		for _, s := range bg.Scripts {
			scripts = append(scripts, ScriptInfo{Path: s, IsModule: false})
		}
	} else if bg.Page != "" {
		// Parse background.page HTML to extract script sources (like Epiphany does)
		pagePath := filepath.Join(bc.ext.Path, filepath.Clean(bg.Page))
		extracted, err := extractScriptsFromHTML(pagePath)
		if err != nil {
			return fmt.Errorf("parse background page %s: %w", bg.Page, err)
		}
		scripts = extracted
	}

	if len(scripts) == 0 {
		log.Printf("[background] No scripts found for extension %s", bc.ext.ID)
		return nil
	}

	// Load regular scripts first, then modules
	for _, script := range scripts {
		if !script.IsModule {
			if err := bc.loadScript(script.Path); err != nil {
				return err
			}
		}
	}

	// Now load ES modules
	for _, script := range scripts {
		if script.IsModule {
			if err := bc.loadModule(script.Path); err != nil {
				return err
			}
		}
	}

	return nil
}

// loadScript loads and executes a regular (non-module) script.
func (bc *BackgroundContext) loadScript(scriptPath string) error {
	// Clean and resolve the path
	cleanPath := filepath.Clean(scriptPath)
	fullPath := filepath.Join(bc.ext.Path, cleanPath)

	// Set document.currentScript for this script
	if bc.browserGlob != nil {
		bc.browserGlob.SetCurrentScript(cleanPath)
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("load script %s: %w", scriptPath, err)
	}

	log.Printf("[background] Loading script: %s", scriptPath)

	if _, err := bc.vm.RunString(string(data)); err != nil {
		return fmt.Errorf("execute script %s: %w", scriptPath, err)
	}

	return nil
}

// loadModule loads and executes an ES module using Sobek's module support.
func (bc *BackgroundContext) loadModule(modulePath string) error {
	cleanPath := filepath.Clean(modulePath)
	fullPath := filepath.Join(bc.ext.Path, cleanPath)

	log.Printf("[background] Loading ES module: %s", modulePath)

	// Set document.currentScript for this module
	if bc.browserGlob != nil {
		bc.browserGlob.SetCurrentScript(cleanPath)
	}

	// Module cache to avoid re-parsing (path -> module)
	moduleCache := make(map[string]sobek.ModuleRecord)
	// Reverse lookup to get path from module record
	modulePaths := make(map[sobek.ModuleRecord]string)

	// Host resolve function for imports
	var hostResolveImportedModule func(referencingScriptOrModule interface{}, specifier string) (sobek.ModuleRecord, error)
	hostResolveImportedModule = func(referencingScriptOrModule interface{}, specifier string) (sobek.ModuleRecord, error) {
		// Resolve relative path
		var resolvedPath string
		if strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") {
			// Relative import - resolve against the referencing module's directory
			var baseDir string
			if referencingScriptOrModule != nil {
				// Look up the path from our reverse map
				if refPath, ok := modulePaths[referencingScriptOrModule.(sobek.ModuleRecord)]; ok {
					baseDir = filepath.Dir(refPath)
				}
			}
			if baseDir == "" {
				baseDir = filepath.Dir(cleanPath)
			}
			resolvedPath = filepath.Join(baseDir, specifier)
		} else {
			// Absolute or bare specifier
			resolvedPath = specifier
		}
		resolvedPath = filepath.Clean(resolvedPath)

		// Check cache
		if cached, ok := moduleCache[resolvedPath]; ok {
			return cached, nil
		}

		// Read module source
		moduleFullPath := filepath.Join(bc.ext.Path, resolvedPath)
		data, err := os.ReadFile(moduleFullPath)
		if err != nil {
			return nil, fmt.Errorf("module not found: %s", specifier)
		}

		// Parse module
		module, err := sobek.ParseModule(resolvedPath, string(data), hostResolveImportedModule)
		if err != nil {
			return nil, fmt.Errorf("parse module %s: %w", specifier, err)
		}

		moduleCache[resolvedPath] = module
		modulePaths[module] = resolvedPath
		return module, nil
	}

	// Read main module source
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("load module %s: %w", modulePath, err)
	}

	// Parse main module
	mainModule, err := sobek.ParseModule(cleanPath, string(data), hostResolveImportedModule)
	if err != nil {
		return fmt.Errorf("parse module %s: %w", modulePath, err)
	}
	moduleCache[cleanPath] = mainModule
	modulePaths[mainModule] = cleanPath

	// Link all modules
	if err := mainModule.Link(); err != nil {
		return fmt.Errorf("link module %s: %w", modulePath, err)
	}

	// Set up dynamic import handler
	bc.vm.SetImportModuleDynamically(func(referencingScriptOrModule interface{}, specifierValue sobek.Value, promiseCapability interface{}) {
		specifier := specifierValue.String()
		go func() {
			bc.tasks <- func() {
				module, err := hostResolveImportedModule(referencingScriptOrModule, specifier)
				bc.vm.FinishLoadingImportModule(referencingScriptOrModule, specifierValue, promiseCapability, module, err)
			}
		}()
	})

	// Evaluate the module
	promise := mainModule.Evaluate(bc.vm)

	// Process any pending promises/microtasks
	// Sobek modules return a Promise, we need to wait for it
	if promise.State() == sobek.PromiseStateRejected {
		result := promise.Result()
		if exc, ok := result.Export().(*sobek.Exception); ok {
			return fmt.Errorf("module evaluation failed %s: %s", modulePath, exc.String())
		}
		return fmt.Errorf("module evaluation failed %s: %v", modulePath, result.Export())
	}

	return nil
}

// ScriptInfo holds information about a script to load
type ScriptInfo struct {
	Path     string
	IsModule bool
}

// toFloat64 converts various numeric types to float64 (JS numbers may export as int64 or float64)
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	default:
		return 0
	}
}

// extractScriptsFromHTML parses an HTML file and extracts script src attributes.
// This follows Epiphany's approach: background.page HTML is parsed to find scripts.
func extractScriptsFromHTML(htmlPath string) ([]ScriptInfo, error) {
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		return nil, err
	}

	content := string(data)
	var scripts []ScriptInfo

	// Simple regex-free parser for <script src="..."> tags
	// Handles both single and double quotes
	for {
		idx := findScriptTag(content)
		if idx < 0 {
			break
		}
		content = content[idx+7:] // skip "<script"

		// Get the tag content before >
		tagEnd := findTagEnd(content)
		if tagEnd < 0 {
			break
		}
		tag := content[:tagEnd]

		src := extractSrcAttribute(content)
		if src != "" {
			isModule := isModuleScript(tag)
			scripts = append(scripts, ScriptInfo{Path: src, IsModule: isModule})
		}

		// Move past this script tag
		content = content[tagEnd:]
	}

	return scripts, nil
}

// isModuleScript checks if the script tag has type="module"
func isModuleScript(tag string) bool {
	// Look for type="module" or type='module'
	tagLower := strings.ToLower(tag)
	return strings.Contains(tagLower, `type="module"`) || strings.Contains(tagLower, `type='module'`)
}

func findScriptTag(s string) int {
	for i := 0; i < len(s)-7; i++ {
		if (s[i] == '<') &&
			(s[i+1] == 's' || s[i+1] == 'S') &&
			(s[i+2] == 'c' || s[i+2] == 'C') &&
			(s[i+3] == 'r' || s[i+3] == 'R') &&
			(s[i+4] == 'i' || s[i+4] == 'I') &&
			(s[i+5] == 'p' || s[i+5] == 'P') &&
			(s[i+6] == 't' || s[i+6] == 'T') {
			return i
		}
	}
	return -1
}

func extractSrcAttribute(s string) string {
	// Find src= within the tag (before >)
	tagEnd := findTagEnd(s)
	if tagEnd < 0 {
		return ""
	}
	tag := s[:tagEnd]

	srcIdx := -1
	for i := 0; i < len(tag)-4; i++ {
		if (tag[i] == 's' || tag[i] == 'S') &&
			(tag[i+1] == 'r' || tag[i+1] == 'R') &&
			(tag[i+2] == 'c' || tag[i+2] == 'C') &&
			tag[i+3] == '=' {
			srcIdx = i + 4
			break
		}
	}
	if srcIdx < 0 {
		return ""
	}

	// Skip whitespace
	for srcIdx < len(tag) && (tag[srcIdx] == ' ' || tag[srcIdx] == '\t') {
		srcIdx++
	}
	if srcIdx >= len(tag) {
		return ""
	}

	// Get quoted value
	quote := tag[srcIdx]
	if quote != '"' && quote != '\'' {
		return ""
	}
	srcIdx++

	endQuote := -1
	for i := srcIdx; i < len(tag); i++ {
		if tag[i] == quote {
			endQuote = i
			break
		}
	}
	if endQuote < 0 {
		return ""
	}

	return tag[srcIdx:endQuote]
}

func findTagEnd(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '>' {
			return i
		}
	}
	return -1
}

// jsEvent is a lightweight listener registry usable from Goja.
type jsEvent struct {
	mu        sync.Mutex
	listeners []sobek.Callable
	values    []sobek.Value
}

func newJSEvent() *jsEvent {
	return &jsEvent{
		listeners: make([]sobek.Callable, 0, 4),
		values:    make([]sobek.Value, 0, 4),
	}
}

func (e *jsEvent) add(vm *sobek.Runtime, fn sobek.Value) error {
	callable, ok := sobek.AssertFunction(fn)
	if !ok {
		return fmt.Errorf("listener is not callable")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = append(e.listeners, callable)
	e.values = append(e.values, fn)
	return nil
}

func (e *jsEvent) remove(vm *sobek.Runtime, fn sobek.Value) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, val := range e.values {
		if val.SameAs(fn) {
			e.listeners = append(e.listeners[:i], e.listeners[i+1:]...)
			e.values = append(e.values[:i], e.values[i+1:]...)
			break
		}
	}
}

func (e *jsEvent) has(vm *sobek.Runtime, fn sobek.Value) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, val := range e.values {
		if val.SameAs(fn) {
			return true
		}
	}
	return false
}

// hasListeners returns true if there are any registered listeners
func (e *jsEvent) hasListeners() bool {
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.listeners) > 0
}

func (e *jsEvent) dispatch(vm *sobek.Runtime, args ...sobek.Value) ([]sobek.Value, error) {
	e.mu.Lock()
	listeners := append([]sobek.Callable{}, e.listeners...)
	e.mu.Unlock()

	results := make([]sobek.Value, 0, len(listeners))
	for _, listener := range listeners {
		ret, err := listener(sobek.Undefined(), args...)
		if err != nil {
			return nil, err
		}
		results = append(results, ret)
	}
	return results, nil
}

func (e *jsEvent) dispatchWithResponse(vm *sobek.Runtime, args ...sobek.Value) (sobek.Value, error) {
	results, err := e.dispatch(vm, args...)
	if err != nil {
		return nil, err
	}
	for _, res := range results {
		if res != nil && res != sobek.Undefined() && res != sobek.Null() {
			return res, nil
		}
	}
	return nil, nil
}

type backgroundPort struct {
	vm           *sobek.Runtime
	desc         api.PortDescriptor
	object       *sobek.Object
	onMessage    *jsEvent
	onDisconnect *jsEvent
	pending      []interface{}
	bc           *BackgroundContext
	disconnected bool
}

func newBackgroundPort(vm *sobek.Runtime, desc api.PortDescriptor, bc *BackgroundContext) *backgroundPort {
	port := &backgroundPort{
		vm:           vm,
		desc:         desc,
		onMessage:    newJSEvent(),
		onDisconnect: newJSEvent(),
		pending:      make([]interface{}, 0),
		bc:           bc,
	}

	obj := vm.NewObject()
	_ = obj.Set("name", desc.Name)
	_ = obj.Set("sender", toJSSenderValue(vm, desc.Sender))

	_ = obj.Set("postMessage", func(call sobek.FunctionCall) sobek.Value {
		if port.disconnected {
			return sobek.Undefined()
		}
		var payload interface{}
		if len(call.Arguments) > 0 {
			payload = call.Arguments[0].Export()
		}
		if desc.ID == "" {
			port.pending = append(port.pending, payload)
			return sobek.Undefined()
		}
		if desc.Callbacks.OnMessage != nil {
			desc.Callbacks.OnMessage(payload)
		}
		return sobek.Undefined()
	})

	_ = obj.Set("disconnect", func(sobek.FunctionCall) sobek.Value {
		port.disconnectInternal()
		return sobek.Undefined()
	})

	onMsgObj := vm.NewObject()
	_ = onMsgObj.Set("addListener", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			_ = port.onMessage.add(vm, call.Arguments[0])
		}
		return sobek.Undefined()
	})
	_ = onMsgObj.Set("removeListener", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			port.onMessage.remove(vm, call.Arguments[0])
		}
		return sobek.Undefined()
	})
	_ = obj.Set("onMessage", onMsgObj)

	onDiscObj := vm.NewObject()
	_ = onDiscObj.Set("addListener", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			_ = port.onDisconnect.add(vm, call.Arguments[0])
		}
		return sobek.Undefined()
	})
	_ = onDiscObj.Set("removeListener", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			port.onDisconnect.remove(vm, call.Arguments[0])
		}
		return sobek.Undefined()
	})
	_ = obj.Set("onDisconnect", onDiscObj)

	port.object = obj
	return port
}

func (p *backgroundPort) flushPending() {
	if len(p.pending) == 0 || p.desc.Callbacks.OnMessage == nil {
		return
	}
	for _, msg := range p.pending {
		p.desc.Callbacks.OnMessage(msg)
	}
	p.pending = nil
}

func (p *backgroundPort) deliverFromExternal(message interface{}) {
	if p.disconnected {
		return
	}
	val := p.vm.ToValue(message)
	_, _ = p.onMessage.dispatch(p.vm, val, p.object)
}

func (p *backgroundPort) disconnectInternal() {
	if p.disconnected {
		return
	}
	p.disconnected = true
	if p.desc.Callbacks.OnDisconnect != nil {
		p.desc.Callbacks.OnDisconnect()
	}
	_, _ = p.onDisconnect.dispatch(p.vm, p.object)
}

// Utility used by tests to simulate onMessage.
func (bc *BackgroundContext) waitForReady(timeout time.Duration) error {
	return bc.call(func() error { return nil })
}
