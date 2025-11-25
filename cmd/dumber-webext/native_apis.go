package main

/*
#cgo pkg-config: javascriptcoregtk-6.0
#include <jsc/jsc.h>
#include <stdlib.h>

// Forward declaration of the Go callback
extern void dumberSendMessageCallback(char *functionName, void *functionArgs, void *resolveCallback, void *rejectCallback, void *userData);

// Wrapper to register the native function in JSC
static void register_send_message_function(JSCContext *context, void *userData) {
    JSCValue *function = jsc_value_new_function(
        context,
        NULL, // Anonymous function
        G_CALLBACK(dumberSendMessageCallback),
        userData,
        NULL, // No destroy notify (userData is managed by Go)
        G_TYPE_NONE, // No return value
        4, // 4 parameters
        G_TYPE_STRING,    // functionName
        JSC_TYPE_VALUE,   // functionArgs
        JSC_TYPE_VALUE,   // resolveCallback
        JSC_TYPE_VALUE    // rejectCallback
    );
    jsc_context_set_value(context, "_dumberSendMessage", function);
    g_object_unref(function);
}

// Wrapper to call a JSC callback with a string argument
static void call_jsc_callback_with_string(JSCValue *callback, const char *str) {
    (void)jsc_value_function_call(callback, G_TYPE_STRING, str, G_TYPE_NONE);
}

// Wrapper to call a JSC callback with a JSCValue argument
static void call_jsc_callback_with_value(JSCValue *callback, JSCValue *value) {
    (void)jsc_value_function_call(callback, JSC_TYPE_VALUE, value, G_TYPE_NONE);
}

// Wrapper for g_object_ref that returns the pointer
static void* ref_gobject(void *obj) {
    return g_object_ref(obj);
}

// Wrapper for g_object_unref
static void unref_gobject(void *obj) {
    g_object_unref(obj);
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
	"unsafe"

	"github.com/diamondburned/gotk4-webkitgtk/pkg/javascriptcore/v6"
	"github.com/diamondburned/gotk4-webkitgtk/pkg/webkitwebprocessextension/v6"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// extensionPageData holds metadata for an extension page
type extensionPageData struct {
	extensionID  string
	manifest     string // JSON
	translations string // JSON
	uiLanguage   string
}

// sendMessageData holds context for sending messages
type sendMessageData struct {
	page        *webkitwebprocessextension.WebPage
	extensionID string
}

// callbackData holds JavaScript callbacks for Promise resolution
type callbackData struct {
	resolveCallback unsafe.Pointer // *C.JSCValue (kept as Go pointer)
	rejectCallback  unsafe.Pointer // *C.JSCValue (kept as Go pointer)
}

// Global storage for message data to prevent GC
// Key: unique ID (uint64), Value: the msgData itself
var (
	messageDataStore   = make(map[uint64]*sendMessageData)
	messageDataStoreMu sync.Mutex
	messageDataNextID  uint64 = 1
)

//export dumberSendMessageCallback
func dumberSendMessageCallback(functionNameC *C.char, functionArgsC, resolveCallbackC, rejectCallbackC unsafe.Pointer, userDataC unsafe.Pointer) {
	functionName := C.GoString(functionNameC)

	// userDataC is actually a uint64 ID, not a pointer to sendMessageData
	msgDataID := uint64(uintptr(userDataC))

	// Look up the sendMessageData from the global store
	messageDataStoreMu.Lock()
	sendMsgData, ok := messageDataStore[msgDataID]
	messageDataStoreMu.Unlock()

	if !ok {
		log.Printf("[native-api] ERROR: Invalid message data ID: %d", msgDataID)
		return
	}

	// Convert JSCValue pointers to Go objects for validation
	functionArgs := (*C.JSCValue)(functionArgsC)
	resolveCallback := (*C.JSCValue)(resolveCallbackC)
	rejectCallback := (*C.JSCValue)(rejectCallbackC)

	// Validate callbacks
	if C.jsc_value_is_function(rejectCallback) == 0 {
		return // Can't reject without reject callback
	}

	if C.jsc_value_is_array(functionArgs) == 0 || C.jsc_value_is_function(resolveCallback) == 0 {
		rejectWithError(rejectCallback, "Invalid arguments: expected array and resolve function")
		return
	}

	// Convert arguments to JSON
	argsJSON := C.jsc_value_to_json(functionArgs, 0)
	defer C.free(unsafe.Pointer(argsJSON))

	// Create the API request payload
	type apiRequest struct {
		ExtensionID string          `json:"extensionId"`
		Function    string          `json:"function"`
		Args        json.RawMessage `json:"args"`
	}

	reqPayload := apiRequest{
		ExtensionID: sendMsgData.extensionID,
		Function:    functionName,
		Args:        json.RawMessage(C.GoString(argsJSON)),
	}

	payloadJSON, err := json.Marshal(reqPayload)
	if err != nil {
		rejectWithError(rejectCallback, fmt.Sprintf("Failed to marshal request: %v", err))
		return
	}

	// Create UserMessage
	variant := glib.NewVariantString(string(payloadJSON))
	message := webkitwebprocessextension.NewUserMessage("webext:api", variant)

	// Store callbacks for async response
	cbData := &callbackData{
		resolveCallback: C.ref_gobject(unsafe.Pointer(resolveCallback)),
		rejectCallback:  C.ref_gobject(unsafe.Pointer(rejectCallback)),
	}

	// Send message to UI process
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sendMsgData.page.SendMessageToView(ctx, message, func(res gio.AsyncResulter) {
		handleSendMessageResponse(sendMsgData.page, res, cbData)
	})
}

// handleSendMessageResponse processes the reply from the UI process
func handleSendMessageResponse(page *webkitwebprocessextension.WebPage, res gio.AsyncResulter, cbData *callbackData) {
	// CRITICAL: Unref must happen regardless of success/failure
	// But we must check if the object is still valid before calling into JSC
	defer func() {
		if cbData.resolveCallback != nil {
			C.unref_gobject(cbData.resolveCallback)
		}
		if cbData.rejectCallback != nil {
			C.unref_gobject(cbData.rejectCallback)
		}
	}()

	reply, err := page.SendMessageToViewFinish(res)
	if err != nil {
		log.Printf("[native-api] Failed to send message: %v", err)
		// Don't attempt to call JavaScript callbacks if the operation was cancelled
		// The cancellation likely means the WebView/context is being torn down
		// Calling into JSC at this point causes a segfault
		return
	}

	// Parse response
	params := reply.Parameters()
	if params == nil {
		resolveWithUndefined((*C.JSCValue)(cbData.resolveCallback))
		return
	}

	jsonStr := params.String()
	// Remove quotes if present (GVariant string format)
	if len(jsonStr) >= 2 && jsonStr[0] == '"' && jsonStr[len(jsonStr)-1] == '"' {
		jsonStr = jsonStr[1 : len(jsonStr)-1]
	}

	// Check if it's an error response
	var response struct {
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		rejectWithError((*C.JSCValue)(cbData.rejectCallback), fmt.Sprintf("Invalid response: %v", err))
		return
	}

	if response.Error != "" {
		rejectWithError((*C.JSCValue)(cbData.rejectCallback), response.Error)
		return
	}

	// Resolve with data
	resolveWithJSON((*C.JSCValue)(cbData.resolveCallback), string(response.Data))
}

// Helper functions to call JavaScript callbacks

func rejectWithError(rejectCallback *C.JSCValue, errorMsg string) {
	cError := C.CString(errorMsg)
	defer C.free(unsafe.Pointer(cError))
	C.call_jsc_callback_with_string(rejectCallback, cError)
}

func resolveWithUndefined(resolveCallback *C.JSCValue) {
	context := C.jsc_value_get_context(resolveCallback)
	undefinedVal := C.jsc_value_new_undefined(context)
	defer C.unref_gobject(unsafe.Pointer(undefinedVal))
	C.call_jsc_callback_with_value(resolveCallback, undefinedVal)
}

func resolveWithJSON(resolveCallback *C.JSCValue, jsonData string) {
	context := C.jsc_value_get_context(resolveCallback)

	// Handle empty data
	if jsonData == "" {
		resolveWithUndefined(resolveCallback)
		return
	}

	// Parse JSON into JSCValue
	cJSON := C.CString(jsonData)
	defer C.free(unsafe.Pointer(cJSON))
	value := C.jsc_value_new_from_json(context, cJSON)
	defer C.unref_gobject(unsafe.Pointer(value))

	C.call_jsc_callback_with_value(resolveCallback, value)
}

// installNativeBrowserAPIs injects browser.* APIs into an extension page's JavaScript context
// This is called for pages loaded via dumb-extension:// scheme
func installNativeBrowserAPIs(page *webkitwebprocessextension.WebPage, frame *webkitwebprocessextension.Frame, extData *extensionPageData) {
	if frame == nil {
		log.Printf("[native-api] No frame available")
		return
	}

	jsContext := frame.JsContext()
	if jsContext == nil {
		log.Printf("[native-api] No JS context available")
		return
	}

	// Inject extension metadata as global variables
	extIDValue := javascriptcore.NewValueString(jsContext, extData.extensionID)
	jsContext.SetValue("_dumberExtensionID", extIDValue)

	langValue := javascriptcore.NewValueString(jsContext, extData.uiLanguage)
	jsContext.SetValue("_dumberUILanguage", langValue)

	// Inject manifest
	if extData.manifest != "" {
		manifestValue := javascriptcore.NewValueFromJson(jsContext, extData.manifest)
		jsContext.SetValue("_dumberManifest", manifestValue)
	}

	// Inject translations
	if extData.translations != "" {
		translationsValue := javascriptcore.NewValueFromJson(jsContext, extData.translations)
		jsContext.SetValue("_dumberTranslations", translationsValue)
	}

	// CRITICAL: Register native function FIRST, before evaluating JavaScript
	// The JavaScript bridge expects window._dumberSendMessage to exist
	setupAPIMessageHandler(page, jsContext, extData.extensionID)

	// Now inject the browser API bridge JavaScript
	// It will find window._dumberSendMessage and create browser.* APIs
	bridgeJS := getBrowserAPIBridgeJS()
	result := jsContext.Evaluate(bridgeJS)
	if result != nil {
		if exception := jsContext.Exception(); exception != nil {
			log.Printf("[native-api] Failed to inject browser API bridge: %v", exception.String())
		} else {
			log.Printf("[native-api] Browser API bridge injected for extension %s", extData.extensionID)
		}
	}
}

// setupAPIMessageHandler registers a global JS function that sends messages to Go
func setupAPIMessageHandler(page *webkitwebprocessextension.WebPage, jsContext *javascriptcore.Context, extensionID string) {
	// Create sendMessageData for this page/extension
	msgData := &sendMessageData{
		page:        page,
		extensionID: extensionID,
	}

	// Generate a unique ID for this message data
	messageDataStoreMu.Lock()
	msgDataID := messageDataNextID
	messageDataNextID++
	// Store in global map to keep it alive and prevent GC
	messageDataStore[msgDataID] = msgData
	messageDataStoreMu.Unlock()

	// Get the underlying C JSCContext pointer
	cContext := (*C.JSCContext)(unsafe.Pointer(jsContext.Native()))

	// Pass the ID (as unsafe.Pointer) instead of the Go struct pointer
	// This avoids CGO's "Go pointer to Go pointer" restriction
	userDataID := unsafe.Pointer(uintptr(msgDataID))

	log.Printf("[native-api] About to register native function in JSC context for %s (msgDataID=%d)", extensionID, msgDataID)
	C.register_send_message_function(cContext, userDataID)
	log.Printf("[native-api] ✓ Successfully registered _dumberSendMessage function for extension %s (ID=%d)", extensionID, msgDataID)
}

// getBrowserAPIBridgeJS returns the JavaScript bridge for browser APIs
// This creates the browser.* namespace with full message-passing support
func getBrowserAPIBridgeJS() string {
	return `
(function() {
	'use strict';

	console.log('[dumber-api] Browser API bridge injection starting...');
	console.log('[dumber-api] window._dumberSendMessage exists:', typeof window._dumberSendMessage);
	console.log('[dumber-api] window.browser exists:', typeof window.browser);

	if (typeof window.browser !== 'undefined') {
		console.log('[dumber-api] Browser API already injected, skipping');
		return; // Already injected
	}

	// Helper function to send API calls to Go via the native bridge
	// Returns a Promise that resolves when the Go side responds
	function sendAPICall(functionName, ...args) {
		return new Promise((resolve, reject) => {
			if (typeof window._dumberSendMessage !== 'function') {
				reject(new Error('Native message bridge not available'));
				return;
			}

			// Call the native function registered by Go
			// It expects: (functionName, argsArray, resolveCallback, rejectCallback)
			window._dumberSendMessage(functionName, args, resolve, reject);
		});
	}

	// Keep track of ports and listeners
	const _dumberPorts = new Map();
	const _dumberRuntimeOnMessage = [];
	const _dumberRuntimeOnConnect = [];
	const _dumberStorageOnChanged = [];
	const _dumberWebRequestOnBeforeRequest = [];
	const _dumberWebRequestOnHeadersReceived = [];
	const _dumberWebRequestOnResponseStarted = [];
	const _dumberEventListeners = new Map(); // Map of event names to arrays of listeners

	// Simple event listener stub (like Epiphany's EphyEventListener)
	// Provides addListener/removeListener/hasListener without backend implementation
	class DumberEventListener {
		constructor(eventName) {
			this._eventName = eventName;
			this._listeners = [];
		}

		addListener(callback) {
			if (typeof callback === 'function' && !this._listeners.includes(callback)) {
				this._listeners.push(callback);
			}
		}

		removeListener(callback) {
			const idx = this._listeners.indexOf(callback);
			if (idx >= 0) {
				this._listeners.splice(idx, 1);
			}
		}

		hasListener(callback) {
			return this._listeners.includes(callback);
		}

		// Internal method to dispatch events (for future use)
		_dispatch(...args) {
			for (const listener of this._listeners) {
				try {
					listener(...args);
				} catch (e) {
					console.error('[dumber-api] Event listener error:', e);
				}
			}
		}
	}

	// Helper functions for generic event management
	function addEventListener(eventName, callback) {
		if (typeof callback !== 'function') return;
		if (!_dumberEventListeners.has(eventName)) {
			_dumberEventListeners.set(eventName, []);
		}
		const listeners = _dumberEventListeners.get(eventName);
		if (!listeners.includes(callback)) {
			listeners.push(callback);
		}
	}

	function removeEventListener(eventName, callback) {
		if (!_dumberEventListeners.has(eventName)) return;
		const listeners = _dumberEventListeners.get(eventName);
		const idx = listeners.indexOf(callback);
		if (idx >= 0) {
			listeners.splice(idx, 1);
		}
	}

	function hasEventListener(eventName, callback) {
		if (!_dumberEventListeners.has(eventName)) return false;
		return _dumberEventListeners.get(eventName).includes(callback);
	}

	function safeStringify(obj) {
		try {
			return JSON.stringify(obj);
		} catch (e) {
			return String(obj);
		}
	}

	function createPort(portId, name, sender) {
		const onMessageListeners = [];
		const onDisconnectListeners = [];
		const pendingMessages = [];

		const port = {
			name: name || '',
			sender: sender || null,
			_portId: portId || null,
			_disconnected: false,
			_pendingMessages: pendingMessages,

			postMessage(message) {
				if (this._disconnected) {
					return;
				}
				if (!this._portId) {
					this._pendingMessages.push(message);
					return;
				}
				sendAPICall('runtime.port.postMessage', { portId: this._portId, message })
					.catch(err => console.error('[dumber] port.postMessage failed:', err));
			},

			disconnect() {
				if (this._disconnected) {
					return;
				}
				if (this._portId) {
					sendAPICall('runtime.port.disconnect', { portId: this._portId })
						.catch(err => console.error('[dumber] port.disconnect failed:', err));
					_dumberPorts.delete(this._portId);
				}
				this._disconnected = true;
				onDisconnectListeners.forEach(fn => {
					try { fn(); } catch (e) { console.error(e); }
				});
			},

			onMessage: {
				addListener(fn) { if (typeof fn === 'function' && !onMessageListeners.includes(fn)) onMessageListeners.push(fn); },
				removeListener(fn) { const idx = onMessageListeners.indexOf(fn); if (idx >= 0) onMessageListeners.splice(idx, 1); },
				hasListener(fn) { return onMessageListeners.includes(fn); },
			},

			onDisconnect: {
				addListener(fn) { if (typeof fn === 'function' && !onDisconnectListeners.includes(fn)) onDisconnectListeners.push(fn); },
				removeListener(fn) { const idx = onDisconnectListeners.indexOf(fn); if (idx >= 0) onDisconnectListeners.splice(idx, 1); },
				hasListener(fn) { return onDisconnectListeners.includes(fn); },
			},

			_flushPending() {
				if (!this._portId || this._pendingMessages.length === 0) {
					return;
				}
				const msgs = this._pendingMessages.splice(0, this._pendingMessages.length);
				msgs.forEach(msg => this.postMessage(msg));
			},

			_deliverMessage(msg) {
				onMessageListeners.forEach(fn => {
					try { fn(msg, this); } catch (e) { console.error(e); }
				});
			},

			_markConnected(newPortId) {
				this._portId = newPortId;
				if (newPortId) {
					_dumberPorts.set(newPortId, this);
				}
				this._flushPending();
			},

			_remoteDisconnect() {
				if (this._disconnected) {
					return;
				}
				this._disconnected = true;
				if (this._portId) {
					_dumberPorts.delete(this._portId);
				}
				onDisconnectListeners.forEach(fn => {
					try { fn(); } catch (e) { console.error(e); }
				});
			},
		};

		if (portId) {
			_dumberPorts.set(portId, port);
		}

		return port;
	}

	function runRuntimeMessageListeners(message, sender) {
		_dumberRuntimeOnMessage.forEach(fn => {
			try {
				// sendResponse is not supported yet; provide noop for compatibility
				fn(message, sender, function noopSendResponse() {});
			} catch (e) {
				console.error(e);
			}
		});
	}

	function handleIncomingEvent(evt) {
		if (!evt || typeof evt !== 'object') {
			console.error('[dumber-api] handleIncomingEvent: invalid event', evt);
			return;
		}
		console.log('[dumber-api] handleIncomingEvent:', evt.type, 'portId:', evt.portId);

		switch (evt.type) {
	case 'runtime-message':
			console.log('[dumber-api] Handling runtime-message:', evt.message);
			runRuntimeMessageListeners(evt.message, evt.sender || null);
			break;
	case 'port-connect': {
			console.log('[dumber-api] Handling port-connect:', evt.portId, 'name:', evt.name, 'onConnect listeners:', _dumberRuntimeOnConnect.length);
			const port = createPort(evt.portId, evt.name || '', evt.sender || null);
			_dumberPorts.set(evt.portId, port);
			console.log('[dumber-api] Port created and registered:', evt.portId, 'total ports:', _dumberPorts.size);
			_dumberRuntimeOnConnect.forEach(fn => {
				try {
					console.log('[dumber-api] Calling onConnect listener with port:', evt.portId);
					fn(port);
				} catch (e) {
					console.error('[dumber-api] onConnect listener error:', e);
				}
			});
			console.log('[dumber-api] port-connect handled');
			break;
		}
	case 'port-message': {
			console.log('[dumber-api] Handling port-message:', evt.portId, 'payload:', evt.message);
			const port = _dumberPorts.get(evt.portId);
			if (port) {
				port._deliverMessage(evt.message);
			} else {
				console.warn('[dumber-api] Received port-message for unknown port:', evt.portId, evt);
			}
			break;
		}
	case 'port-disconnect': {
			console.log('[dumber-api] Handling port-disconnect:', evt.portId);
			const port = _dumberPorts.get(evt.portId);
			if (port) {
				port._remoteDisconnect();
			}
			break;
		}
	case 'webRequest:onBeforeRequest': {
			let response = {};
			for (const fn of _dumberWebRequestOnBeforeRequest) {
				try {
					const res = fn(evt.details);
					if (res !== undefined) {
						response = res;
					}
				} catch (e) {
					console.error('[dumber-api] webRequest listener error:', e);
				}
			}

			sendAPICall('webRequest.onBeforeRequest.reply', {
				requestId: evt.requestId,
				response
			}).catch(err => console.error('[dumber-api] webRequest reply failed:', err));
			break;
		}
	case 'webRequest:onHeadersReceived': {
			let response = {};
			for (const fn of _dumberWebRequestOnHeadersReceived) {
				try {
					const res = fn(evt.details);
					if (res !== undefined) {
						response = res;
					}
				} catch (e) {
					console.error('[dumber-api] webRequest.onHeadersReceived listener error:', e);
				}
			}

			sendAPICall('webRequest.onHeadersReceived.reply', {
				requestId: evt.requestId,
				response
			}).catch(err => console.error('[dumber-api] webRequest.onHeadersReceived reply failed:', err));
			break;
		}
	case 'webRequest:onResponseStarted': {
			for (const fn of _dumberWebRequestOnResponseStarted) {
				try {
					fn(evt.details);
				} catch (e) {
					console.error('[dumber-api] webRequest.onResponseStarted listener error:', e);
				}
			}
			// No response needed for onResponseStarted (it's not blocking)
			break;
		}
	case 'storage-change': {
			const changes = evt.changes || {};
			const areaName = evt.areaName || '';
			_dumberStorageOnChanged.forEach(fn => {
				try {
					fn(changes, areaName);
				} catch (e) {
					console.error('[dumber-api] storage.onChanged listener error:', e);
				}
			});
			break;
		}
	default: {
			const payload = safeStringify(evt);
			const typeLabel = evt && evt.type ? evt.type : '<missing>';
			console.warn('[dumber-api] Unknown event type ' + typeLabel + ' payload=' + payload);
			break;
		}
		}
	}

	// Create browser namespace
	window.browser = {
		runtime: {
			// Synchronous APIs (work with local data)
			getURL: function(path) {
				const extId = window._dumberExtensionID || 'unknown';
				return 'dumb-extension://' + extId + '/' + (path || '').replace(/^\//, '');
			},

			getManifest: function() {
				return window._dumberManifest || {name: 'Unknown', version: '0.0.0'};
			},

			getBrowserInfo: function() {
				return Promise.resolve({
					name: 'Dumber',
					vendor: 'Dumber Browser',
					version: '1.0.0'
				});
			},

			// Async APIs (via message passing)
			sendMessage: function(message, callback) {
				const promise = sendAPICall('runtime.sendMessage', {
					message,
					url: window.location && window.location.href ? window.location.href : ''
				});

				// Support both callback and promise styles
				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] runtime.sendMessage error:', err);
					});
				}

				return promise;
			},

			onMessage: {
				addListener: function(cb) {
					if (typeof cb === 'function' && !_dumberRuntimeOnMessage.includes(cb)) {
						_dumberRuntimeOnMessage.push(cb);
					}
				},
				removeListener: function(cb) {
					const idx = _dumberRuntimeOnMessage.indexOf(cb);
					if (idx >= 0) {
						_dumberRuntimeOnMessage.splice(idx, 1);
					}
				},
				hasListener: function(cb) { return _dumberRuntimeOnMessage.includes(cb); }
			},

			onConnect: {
				addListener: function(cb) {
					if (typeof cb === 'function' && !_dumberRuntimeOnConnect.includes(cb)) {
						_dumberRuntimeOnConnect.push(cb);
					}
				},
				removeListener: function(cb) {
					const idx = _dumberRuntimeOnConnect.indexOf(cb);
					if (idx >= 0) {
						_dumberRuntimeOnConnect.splice(idx, 1);
					}
				},
				hasListener: function(cb) { return _dumberRuntimeOnConnect.includes(cb); }
			},

			connect: function(connectInfo) {
				const info = connectInfo || {};
				const port = createPort(null, info.name || '', null);
				sendAPICall('runtime.connect', {
					name: info.name || '',
					url: window.location && window.location.href ? window.location.href : ''
				}).then(res => {
					if (res && res.portId) {
						port._markConnected(res.portId);
					} else {
						console.error('[dumber] runtime.connect returned no portId');
						port._remoteDisconnect();
					}
				}).catch(err => {
					console.error('[dumber] runtime.connect failed:', err);
					port._remoteDisconnect();
				});
				return port;
			},

			// Get platform information (OS, architecture)
			getPlatformInfo: function(callback) {
				const promise = sendAPICall('runtime.getPlatformInfo', null);
				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] runtime.getPlatformInfo error:', err);
					});
				}
				return promise;
			},

			// Open the extension's options page
			openOptionsPage: function(callback) {
				const promise = sendAPICall('runtime.openOptionsPage', null);
				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] runtime.openOptionsPage error:', err);
					});
				}
				return promise;
			},

			// onInstalled event (when extension is installed or updated)
			onInstalled: {
				addListener: function(callback) {
					addEventListener('runtime.onInstalled', callback);
				},
				removeListener: function(callback) {
					removeEventListener('runtime.onInstalled', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('runtime.onInstalled', callback);
				}
			},

			// onUpdateAvailable event (when a new version is available)
			onUpdateAvailable: {
				addListener: function(callback) {
					addEventListener('runtime.onUpdateAvailable', callback);
				},
				removeListener: function(callback) {
					removeEventListener('runtime.onUpdateAvailable', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('runtime.onUpdateAvailable', callback);
				}
			},

			// onMessageExternal event (when message is received from another extension)
			onMessageExternal: {
				addListener: function(callback) {
					addEventListener('runtime.onMessageExternal', callback);
				},
				removeListener: function(callback) {
					removeEventListener('runtime.onMessageExternal', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('runtime.onMessageExternal', callback);
				}
			},

			lastError: null
		},

		webRequest: {
			ResourceType: {
				MAIN_FRAME: 'main_frame',
				SUB_FRAME: 'sub_frame',
				STYLESHEET: 'stylesheet',
				SCRIPT: 'script',
				IMAGE: 'image',
				FONT: 'font',
				OBJECT: 'object',
				XMLHTTPREQUEST: 'xmlhttprequest',
				PING: 'ping',
				CSP_REPORT: 'csp_report',
				MEDIA: 'media',
				WEBSOCKET: 'websocket',
				WEBTRANSPORT: 'webtransport',
				OTHER: 'other'
			},
			MAX_HANDLER_BEHAVIOR_CHANGED_CALLS_PER_10_MINUTES: 20,
			onBeforeRequest: {
				addListener: function(cb, filter, extraInfoSpec) {
					if (typeof cb === 'function' && !_dumberWebRequestOnBeforeRequest.includes(cb)) {
						_dumberWebRequestOnBeforeRequest.push(cb);
					}

					const payload = { filter: filter || {} };
					sendAPICall('webRequest.onBeforeRequest.addListener', payload)
						.catch(err => console.error('[dumber] webRequest.onBeforeRequest.addListener failed:', err));
				},
				removeListener: function(cb) {
					const idx = _dumberWebRequestOnBeforeRequest.indexOf(cb);
					if (idx >= 0) {
						_dumberWebRequestOnBeforeRequest.splice(idx, 1);
					}
					sendAPICall('webRequest.onBeforeRequest.removeListener', {})
						.catch(err => console.error('[dumber] webRequest.onBeforeRequest.removeListener failed:', err));
				},
				hasListener: function(cb) {
					return _dumberWebRequestOnBeforeRequest.includes(cb);
				}
			},
			onHeadersReceived: {
				addListener: function(cb, filter, extraInfoSpec) {
					if (typeof cb === 'function' && !_dumberWebRequestOnHeadersReceived.includes(cb)) {
						_dumberWebRequestOnHeadersReceived.push(cb);
					}

					const payload = {
						filter: filter || {},
						extraInfoSpec: extraInfoSpec || []
					};
					sendAPICall('webRequest.onHeadersReceived.addListener', payload)
						.catch(err => console.error('[dumber] webRequest.onHeadersReceived.addListener failed:', err));
				},
				removeListener: function(cb) {
					const idx = _dumberWebRequestOnHeadersReceived.indexOf(cb);
					if (idx >= 0) {
						_dumberWebRequestOnHeadersReceived.splice(idx, 1);
					}
					sendAPICall('webRequest.onHeadersReceived.removeListener', {})
						.catch(err => console.error('[dumber] webRequest.onHeadersReceived.removeListener failed:', err));
				},
				hasListener: function(cb) {
					return _dumberWebRequestOnHeadersReceived.includes(cb);
				}
			},
			onResponseStarted: {
				addListener: function(cb, filter, extraInfoSpec) {
					if (typeof cb === 'function' && !_dumberWebRequestOnResponseStarted.includes(cb)) {
						_dumberWebRequestOnResponseStarted.push(cb);
					}

					const payload = {
						filter: filter || {},
						extraInfoSpec: extraInfoSpec || []
					};
					sendAPICall('webRequest.onResponseStarted.addListener', payload)
						.catch(err => console.error('[dumber] webRequest.onResponseStarted.addListener failed:', err));
				},
				removeListener: function(cb) {
					const idx = _dumberWebRequestOnResponseStarted.indexOf(cb);
					if (idx >= 0) {
						_dumberWebRequestOnResponseStarted.splice(idx, 1);
					}
					sendAPICall('webRequest.onResponseStarted.removeListener', {})
						.catch(err => console.error('[dumber] webRequest.onResponseStarted.removeListener failed:', err));
				},
				hasListener: function(cb) {
					return _dumberWebRequestOnResponseStarted.includes(cb);
				}
			},
			handlerBehaviorChanged: function() {
				sendAPICall('webRequest.handlerBehaviorChanged').catch(err => console.error('[dumber] webRequest.handlerBehaviorChanged failed:', err));
			}
		},

		i18n: {
			// Synchronous APIs (work with local data injected by Go)
			getMessage: function(messageName, substitutions) {
				const translations = window._dumberTranslations || {};
				const translation = translations[messageName];

				if (translation && translation.message) {
					let message = translation.message;

					// Handle placeholders: $1, $2, etc.
					if (substitutions) {
						const subs = Array.isArray(substitutions) ? substitutions : [substitutions];
						message = message.replace(/\$(\d+)/g, function(match, num) {
							const idx = parseInt(num) - 1;
							return idx < subs.length ? subs[idx] : match;
						});
					}

					return message;
				}

				return messageName; // Fallback to key name
			},

			getUILanguage: function() {
				return window._dumberUILanguage || navigator.language.split('-')[0] || 'en';
			}
		},

		// NOTE: browser.storage is NOT available in content scripts via this bridge
		// Content scripts must use runtime.sendMessage to communicate with the background
		// page for storage access. Background pages have direct Goja bindings for storage.
		//
		// Example for content scripts:
		//   browser.runtime.sendMessage({
		//     type: 'storage.get',
		//     keys: ['key1', 'key2']
		//   }).then(items => console.log(items));
		//
		// The background page would handle this message and access storage directly.
		storage: {
			// Storage onChanged events can still be received (dispatched from background)
			onChanged: {
				addListener: function(cb) {
					if (typeof cb === 'function' && !_dumberStorageOnChanged.includes(cb)) {
						_dumberStorageOnChanged.push(cb);
					}
				},
				removeListener: function(cb) {
					const idx = _dumberStorageOnChanged.indexOf(cb);
					if (idx >= 0) {
						_dumberStorageOnChanged.splice(idx, 1);
					}
				},
				hasListener: function(cb) { return _dumberStorageOnChanged.includes(cb); }
			}
		},

		tabs: {
			// Query tabs matching the given criteria (async via message passing)
			query: function(queryInfo, callback) {
				const promise = sendAPICall('tabs.query', queryInfo || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.query error:', err);
					});
				}

				return promise;
			},

			// Get a specific tab by ID (async via message passing)
			get: function(tabId, callback) {
				const promise = sendAPICall('tabs.get', tabId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.get error:', err);
					});
				}

				return promise;
			},

			// Get the current active tab (async via message passing)
			getCurrent: function(callback) {
				// For popup pages, query for the active tab in the current window
				const promise = sendAPICall('tabs.query', {active: true, currentWindow: true})
					.then(tabs => tabs && tabs.length > 0 ? tabs[0] : null);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.getCurrent error:', err);
					});
				}

				return promise;
			},

			// Create a new tab
			create: function(createProperties, callback) {
				const promise = sendAPICall('tabs.create', createProperties || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.create error:', err);
					});
				}

				return promise;
			},

			// Remove one or more tabs
			remove: function(tabIds, callback) {
				const promise = sendAPICall('tabs.remove', tabIds);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.remove error:', err);
					});
				}

				return promise;
			},

			// Update tab properties
			update: function(tabId, updateProperties, callback) {
				// Optional tabId parameter
				let args;
				if (typeof tabId === 'object') {
					// update(updateProperties, callback)
					callback = updateProperties;
					args = [tabId];
				} else {
					// update(tabId, updateProperties, callback)
					args = [tabId, updateProperties];
				}

				const promise = sendAPICall('tabs.update', args);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.update error:', err);
					});
				}

				return promise;
			},

			// Reload a tab
			reload: function(tabId, reloadProperties, callback) {
				// Optional parameters
				let args = [];
				if (typeof tabId === 'function') {
					// reload(callback)
					callback = tabId;
				} else if (typeof tabId === 'object') {
					// reload(reloadProperties, callback)
					callback = reloadProperties;
					args = [tabId];
				} else if (typeof reloadProperties === 'function') {
					// reload(tabId, callback)
					callback = reloadProperties;
					args = [tabId];
				} else {
					// reload(tabId, reloadProperties, callback)
					args = [tabId, reloadProperties];
				}

				const promise = sendAPICall('tabs.reload', args);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.reload error:', err);
					});
				}

				return promise;
			},

			// Execute JavaScript in a tab
			executeScript: function(tabId, details, callback) {
				// Optional tabId parameter
				let args;
				if (typeof tabId === 'object') {
					// executeScript(details, callback)
					callback = details;
					args = [tabId];
				} else {
					// executeScript(tabId, details, callback)
					args = [tabId, details];
				}

				const promise = sendAPICall('tabs.executeScript', args);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.executeScript error:', err);
					});
				}

				return promise;
			},

			// Insert CSS into a tab
			insertCSS: function(tabId, details, callback) {
				// Optional tabId parameter
				let args;
				if (typeof tabId === 'object') {
					// insertCSS(details, callback)
					callback = details;
					args = [tabId];
				} else {
					// insertCSS(tabId, details, callback)
					args = [tabId, details];
				}

				const promise = sendAPICall('tabs.insertCSS', args);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.insertCSS error:', err);
					});
				}

				return promise;
			},

			// Remove CSS from a tab
			removeCSS: function(tabId, details, callback) {
				// Optional tabId parameter
				let args;
				if (typeof tabId === 'object') {
					// removeCSS(details, callback)
					callback = details;
					args = [tabId];
				} else {
					// removeCSS(tabId, details, callback)
					args = [tabId, details];
				}

				const promise = sendAPICall('tabs.removeCSS', args);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.removeCSS error:', err);
					});
				}

				return promise;
			},

			// Get zoom level for a tab
			getZoom: function(tabId, callback) {
				// Optional tabId parameter
				const args = typeof tabId === 'function' ? [] : [tabId];
				if (typeof tabId === 'function') {
					callback = tabId;
				}

				const promise = sendAPICall('tabs.getZoom', args);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.getZoom error:', err);
					});
				}

				return promise;
			},

			// Set zoom level for a tab
			setZoom: function(tabId, zoomFactor, callback) {
				// Optional tabId parameter
				let args;
				if (typeof tabId === 'number' && typeof zoomFactor === 'number') {
					// setZoom(tabId, zoomFactor, callback)
					args = [tabId, zoomFactor];
				} else {
					// setZoom(zoomFactor, callback)
					callback = zoomFactor;
					args = [tabId];
				}

				const promise = sendAPICall('tabs.setZoom', args);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] tabs.setZoom error:', err);
					});
				}

				return promise;
			},

			// Tab event listeners
			onActivated: {
				addListener: function(callback) {
					addEventListener('tabs.onActivated', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onActivated', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onActivated', callback);
				}
			},

			onAttached: {
				addListener: function(callback) {
					addEventListener('tabs.onAttached', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onAttached', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onAttached', callback);
				}
			},

			onCreated: {
				addListener: function(callback) {
					addEventListener('tabs.onCreated', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onCreated', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onCreated', callback);
				}
			},

			onDetached: {
				addListener: function(callback) {
					addEventListener('tabs.onDetached', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onDetached', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onDetached', callback);
				}
			},

			onHighlighted: {
				addListener: function(callback) {
					addEventListener('tabs.onHighlighted', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onHighlighted', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onHighlighted', callback);
				}
			},

			onMoved: {
				addListener: function(callback) {
					addEventListener('tabs.onMoved', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onMoved', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onMoved', callback);
				}
			},

			onRemoved: {
				addListener: function(callback) {
					addEventListener('tabs.onRemoved', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onRemoved', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onRemoved', callback);
				}
			},

			onReplaced: {
				addListener: function(callback) {
					addEventListener('tabs.onReplaced', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onReplaced', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onReplaced', callback);
				}
			},

			onUpdated: {
				addListener: function(callback) {
					addEventListener('tabs.onUpdated', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onUpdated', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onUpdated', callback);
				}
			},

			onZoomChange: {
				addListener: function(callback) {
					addEventListener('tabs.onZoomChange', callback);
				},
				removeListener: function(callback) {
					removeEventListener('tabs.onZoomChange', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('tabs.onZoomChange', callback);
				}
			}
		},

		browserAction: {
			// Set the badge text for the browser action
			setBadgeText: function(details, callback) {
				const promise = sendAPICall('browserAction.setBadgeText', details || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] browserAction.setBadgeText error:', err);
					});
				}

				return promise;
			},

			// Set the badge background color for the browser action
			setBadgeBackgroundColor: function(details, callback) {
				const promise = sendAPICall('browserAction.setBadgeBackgroundColor', details || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] browserAction.setBadgeBackgroundColor error:', err);
					});
				}

				return promise;
			},

			// Browser action click event
			onClicked: {
				addListener: function(callback) {
					addEventListener('browserAction.onClicked', callback);
				},
				removeListener: function(callback) {
					removeEventListener('browserAction.onClicked', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('browserAction.onClicked', callback);
				}
			}
		},

		alarms: {
			// Create an alarm
			create: function(name, alarmInfo, callback) {
				// Handle optional name parameter
				let args;
				if (typeof name === 'object' && alarmInfo === undefined) {
					// name is actually alarmInfo, no name provided
					args = [name];
					callback = alarmInfo;
				} else if (typeof name === 'string') {
					args = [name, alarmInfo || {}];
				} else {
					args = [alarmInfo || {}];
				}

				const promise = sendAPICall('alarms.create', ...args);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] alarms.create error:', err);
					});
				}

				return promise;
			},

			// Get an alarm by name
			get: function(name, callback) {
				const promise = sendAPICall('alarms.get', name);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] alarms.get error:', err);
					});
				}

				return promise;
			},

			// Get all alarms
			getAll: function(callback) {
				const promise = sendAPICall('alarms.getAll');

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] alarms.getAll error:', err);
					});
				}

				return promise;
			},

			// Clear an alarm by name
			clear: function(name, callback) {
				const promise = sendAPICall('alarms.clear', name);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] alarms.clear error:', err);
					});
				}

				return promise;
			},

			// Clear all alarms
			clearAll: function(callback) {
				const promise = sendAPICall('alarms.clearAll');

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] alarms.clearAll error:', err);
					});
				}

				return promise;
			},

			// onAlarm event
			onAlarm: {
				addListener: function(callback) {
					addEventListener('alarms.onAlarm', callback);
				},
				removeListener: function(callback) {
					removeEventListener('alarms.onAlarm', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('alarms.onAlarm', callback);
				}
			}
		},

		menus: {
			// Create a menu item
			create: function(createProperties, callback) {
				const promise = sendAPICall('menus.create', createProperties || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] menus.create error:', err);
					});
				}

				return promise;
			},

			// Remove a menu item by ID
			remove: function(menuItemId, callback) {
				const promise = sendAPICall('menus.remove', menuItemId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] menus.remove error:', err);
					});
				}

				return promise;
			},

			// Remove all menu items
			removeAll: function(callback) {
				const promise = sendAPICall('menus.removeAll');

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] menus.removeAll error:', err);
					});
				}

				return promise;
			},

			// onClicked event (for when menu items are clicked)
			onClicked: {
				addListener: function(callback) {
					addEventListener('menus.onClicked', callback);
				},
				removeListener: function(callback) {
					removeEventListener('menus.onClicked', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('menus.onClicked', callback);
				}
			}
		},

		// contextMenus is an alias for menus (Firefox compatibility)
		get contextMenus() {
			return this.menus;
		},

		notifications: {
			// Create a notification
			create: function(notificationId, options, callback) {
				// Handle optional notificationId parameter
				let args;
				if (typeof notificationId === 'object' && options === undefined) {
					// notificationId is actually options, no ID provided
					args = [notificationId];
					callback = options;
				} else if (typeof notificationId === 'string') {
					args = [notificationId, options || {}];
				} else {
					args = [options || {}];
				}

				const promise = sendAPICall('notifications.create', ...args);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] notifications.create error:', err);
					});
				}

				return promise;
			},

			// Update a notification
			update: function(notificationId, options, callback) {
				const promise = sendAPICall('notifications.update', notificationId, options || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] notifications.update error:', err);
					});
				}

				return promise;
			},

			// Clear a notification
			clear: function(notificationId, callback) {
				const promise = sendAPICall('notifications.clear', notificationId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] notifications.clear error:', err);
					});
				}

				return promise;
			},

			// Get all notifications
			getAll: function(callback) {
				const promise = sendAPICall('notifications.getAll');

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] notifications.getAll error:', err);
					});
				}

				return promise;
			},

			// onClicked event (when notification is clicked)
			onClicked: {
				addListener: function(callback) {
					addEventListener('notifications.onClicked', callback);
				},
				removeListener: function(callback) {
					removeEventListener('notifications.onClicked', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('notifications.onClicked', callback);
				}
			},

			// onButtonClicked event (when notification button is clicked)
			onButtonClicked: {
				addListener: function(callback) {
					addEventListener('notifications.onButtonClicked', callback);
				},
				removeListener: function(callback) {
					removeEventListener('notifications.onButtonClicked', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('notifications.onButtonClicked', callback);
				}
			},

			// onClosed event (when notification is closed)
			onClosed: {
				addListener: function(callback) {
					addEventListener('notifications.onClosed', callback);
				},
				removeListener: function(callback) {
					removeEventListener('notifications.onClosed', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('notifications.onClosed', callback);
				}
			},

			// onShown event (when notification is shown)
			onShown: {
				addListener: function(callback) {
					addEventListener('notifications.onShown', callback);
				},
				removeListener: function(callback) {
					removeEventListener('notifications.onShown', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('notifications.onShown', callback);
				}
			}
		},

		downloads: {
			// Download a URL to a file
			download: function(options, callback) {
				const promise = sendAPICall('downloads.download', options || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] downloads.download error:', err);
					});
				}

				return promise;
			},

			// Cancel a download
			cancel: function(downloadId, callback) {
				const promise = sendAPICall('downloads.cancel', downloadId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] downloads.cancel error:', err);
					});
				}

				return promise;
			},

			// Open a downloaded file
			open: function(downloadId, callback) {
				const promise = sendAPICall('downloads.open', downloadId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] downloads.open error:', err);
					});
				}

				return promise;
			},

			// Show a downloaded file in file manager
			show: function(downloadId, callback) {
				const promise = sendAPICall('downloads.show', downloadId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] downloads.show error:', err);
					});
				}

				return promise;
			},

			// Remove the downloaded file from disk
			removeFile: function(downloadId, callback) {
				const promise = sendAPICall('downloads.removeFile', downloadId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] downloads.removeFile error:', err);
					});
				}

				return promise;
			},

			// Show the default downloads folder
			showDefaultFolder: function(callback) {
				const promise = sendAPICall('downloads.showDefaultFolder');

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] downloads.showDefaultFolder error:', err);
					});
				}

				return promise;
			},

			// Search for downloads
			search: function(query, callback) {
				const promise = sendAPICall('downloads.search', query || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] downloads.search error:', err);
					});
				}

				return promise;
			},

			// Erase downloads from history
			erase: function(query, callback) {
				const promise = sendAPICall('downloads.erase', query || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] downloads.erase error:', err);
					});
				}

				return promise;
			},

			// onCreated event (when download starts)
			onCreated: {
				addListener: function(callback) {
					addEventListener('downloads.onCreated', callback);
				},
				removeListener: function(callback) {
					removeEventListener('downloads.onCreated', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('downloads.onCreated', callback);
				}
			},

			// onChanged event (when download state changes)
			onChanged: {
				addListener: function(callback) {
					addEventListener('downloads.onChanged', callback);
				},
				removeListener: function(callback) {
					removeEventListener('downloads.onChanged', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('downloads.onChanged', callback);
				}
			},

			// onErased event (when download is removed from history)
			onErased: {
				addListener: function(callback) {
					addEventListener('downloads.onErased', callback);
				},
				removeListener: function(callback) {
					removeEventListener('downloads.onErased', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('downloads.onErased', callback);
				}
			}
		},

		commands: {
			// Get all registered commands
			getAll: function(callback) {
				const promise = sendAPICall('commands.getAll', null);
				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] commands.getAll error:', err);
					});
				}
				return promise;
			},

			// Reset a command to its default shortcut
			reset: function(name, callback) {
				const promise = sendAPICall('commands.reset', name);
				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] commands.reset error:', err);
					});
				}
				return promise;
			},

			// Update a command
			update: function(details, callback) {
				const promise = sendAPICall('commands.update', details || {});
				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] commands.update error:', err);
					});
				}
				return promise;
			},

			// Event listeners for command activation
			onCommand: {
				addListener: function(callback) {
					addEventListener('commands.onCommand', callback);
				},
				removeListener: function(callback) {
					removeEventListener('commands.onCommand', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('commands.onCommand', callback);
				}
			}
		},

		pageAction: {
			// Set the icon for a page action
			setIcon: function(details, callback) {
				const promise = sendAPICall('pageAction.setIcon', details || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] pageAction.setIcon error:', err);
					});
				}

				return promise;
			},

			// Set the title for a page action
			setTitle: function(details, callback) {
				const promise = sendAPICall('pageAction.setTitle', details || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] pageAction.setTitle error:', err);
					});
				}

				return promise;
			},

			// Get the title for a page action
			getTitle: function(details, callback) {
				const promise = sendAPICall('pageAction.getTitle', details || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] pageAction.getTitle error:', err);
					});
				}

				return promise;
			},

			// Show a page action
			show: function(tabId, callback) {
				const promise = sendAPICall('pageAction.show', tabId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] pageAction.show error:', err);
					});
				}

				return promise;
			},

			// Hide a page action
			hide: function(tabId, callback) {
				const promise = sendAPICall('pageAction.hide', tabId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] pageAction.hide error:', err);
					});
				}

				return promise;
			},

			// onClicked event (when page action is clicked)
			onClicked: {
				addListener: function(callback) {
					addEventListener('pageAction.onClicked', callback);
				},
				removeListener: function(callback) {
					removeEventListener('pageAction.onClicked', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('pageAction.onClicked', callback);
				}
			}
		},

		windows: {
			// Get details about a window
			get: function(windowId, getInfo, callback) {
				const promise = sendAPICall('windows.get', { windowId, getInfo: getInfo || {} });

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] windows.get error:', err);
					});
				}

				return promise;
			},

			// Get the current window
			getCurrent: function(getInfo, callback) {
				const promise = sendAPICall('windows.getCurrent', getInfo || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] windows.getCurrent error:', err);
					});
				}

				return promise;
			},

			// Get the last focused window
			getLastFocused: function(getInfo, callback) {
				const promise = sendAPICall('windows.getLastFocused', getInfo || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] windows.getLastFocused error:', err);
					});
				}

				return promise;
			},

			// Get all windows
			getAll: function(getInfo, callback) {
				const promise = sendAPICall('windows.getAll', getInfo || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] windows.getAll error:', err);
					});
				}

				return promise;
			},

			// Create a new window
			create: function(createData, callback) {
				const promise = sendAPICall('windows.create', createData || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] windows.create error:', err);
					});
				}

				return promise;
			},

			// Remove (close) a window
			remove: function(windowId, callback) {
				const promise = sendAPICall('windows.remove', windowId);

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] windows.remove error:', err);
					});
				}

				return promise;
			},

			// onCreated event (when window is created)
			onCreated: {
				addListener: function(callback) {
					addEventListener('windows.onCreated', callback);
				},
				removeListener: function(callback) {
					removeEventListener('windows.onCreated', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('windows.onCreated', callback);
				}
			},

			// onRemoved event (when window is removed)
			onRemoved: {
				addListener: function(callback) {
					addEventListener('windows.onRemoved', callback);
				},
				removeListener: function(callback) {
					removeEventListener('windows.onRemoved', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('windows.onRemoved', callback);
				}
			},

			// onFocusChanged event (when window focus changes)
			onFocusChanged: {
				addListener: function(callback) {
					addEventListener('windows.onFocusChanged', callback);
				},
				removeListener: function(callback) {
					removeEventListener('windows.onFocusChanged', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('windows.onFocusChanged', callback);
				}
			}
		},

		webNavigation: {
			// Get details about a specific frame
			getFrame: function(details, callback) {
				// Stub: return null frame (not implemented)
				const promise = Promise.resolve(null);
				if (callback) callback(null);
				return promise;
			},

			// Get all frames in a tab
			getAllFrames: function(details, callback) {
				// Stub: return empty array (not implemented)
				const promise = Promise.resolve([]);
				if (callback) callback([]);
				return promise;
			},

			// Events (stubs - listeners registered but never fired)
			onBeforeNavigate: {
				addListener: function(callback) { addEventListener('webNavigation.onBeforeNavigate', callback); },
				removeListener: function(callback) { removeEventListener('webNavigation.onBeforeNavigate', callback); },
				hasListener: function(callback) { return hasEventListener('webNavigation.onBeforeNavigate', callback); }
			},

			onCommitted: {
				addListener: function(callback) { addEventListener('webNavigation.onCommitted', callback); },
				removeListener: function(callback) { removeEventListener('webNavigation.onCommitted', callback); },
				hasListener: function(callback) { return hasEventListener('webNavigation.onCommitted', callback); }
			},

			onDOMContentLoaded: {
				addListener: function(callback) { addEventListener('webNavigation.onDOMContentLoaded', callback); },
				removeListener: function(callback) { removeEventListener('webNavigation.onDOMContentLoaded', callback); },
				hasListener: function(callback) { return hasEventListener('webNavigation.onDOMContentLoaded', callback); }
			},

			onCompleted: {
				addListener: function(callback) { addEventListener('webNavigation.onCompleted', callback); },
				removeListener: function(callback) { removeEventListener('webNavigation.onCompleted', callback); },
				hasListener: function(callback) { return hasEventListener('webNavigation.onCompleted', callback); }
			},

			onCreatedNavigationTarget: {
				addListener: function(callback) { addEventListener('webNavigation.onCreatedNavigationTarget', callback); },
				removeListener: function(callback) { removeEventListener('webNavigation.onCreatedNavigationTarget', callback); },
				hasListener: function(callback) { return hasEventListener('webNavigation.onCreatedNavigationTarget', callback); }
			}
		},

		cookies: {
			// Get a single cookie by name
			get: function(details, callback) {
				const promise = sendAPICall('cookies.get', details || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] cookies.get error:', err);
					});
				}

				return promise;
			},

			// Get all cookies matching the given filter
			getAll: function(details, callback) {
				const promise = sendAPICall('cookies.getAll', details || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] cookies.getAll error:', err);
					});
				}

				return promise;
			},

			// Set a cookie
			set: function(details, callback) {
				const promise = sendAPICall('cookies.set', details || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] cookies.set error:', err);
					});
				}

				return promise;
			},

			// Remove a cookie
			remove: function(details, callback) {
				const promise = sendAPICall('cookies.remove', details || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] cookies.remove error:', err);
					});
				}

				return promise;
			},

			// Get all cookie stores
			getAllCookieStores: function(callback) {
				const promise = sendAPICall('cookies.getAllCookieStores', {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] cookies.getAllCookieStores error:', err);
					});
				}

				return promise;
			},

			// onChanged event (when cookie is set or removed)
			onChanged: {
				addListener: function(callback) {
					addEventListener('cookies.onChanged', callback);
				},
				removeListener: function(callback) {
					removeEventListener('cookies.onChanged', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('cookies.onChanged', callback);
				}
			}
		},

		permissions: {
			// Check if extension has specific permissions
			contains: function(permissions, callback) {
				const promise = sendAPICall('permissions.contains', permissions || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] permissions.contains error:', err);
					});
				}

				return promise;
			},

			// Get all current permissions
			getAll: function(callback) {
				const promise = sendAPICall('permissions.getAll', {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] permissions.getAll error:', err);
					});
				}

				return promise;
			},

			// Request additional permissions
			request: function(permissions, callback) {
				const promise = sendAPICall('permissions.request', permissions || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] permissions.request error:', err);
					});
				}

				return promise;
			},

			// Remove permissions
			remove: function(permissions, callback) {
				const promise = sendAPICall('permissions.remove', permissions || {});

				if (callback) {
					promise.then(callback).catch(err => {
						console.error('[dumber] permissions.remove error:', err);
					});
				}

				return promise;
			},

			// onAdded event (when permissions are added)
			onAdded: {
				addListener: function(callback) {
					addEventListener('permissions.onAdded', callback);
				},
				removeListener: function(callback) {
					removeEventListener('permissions.onAdded', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('permissions.onAdded', callback);
				}
			},

			// onRemoved event (when permissions are removed)
			onRemoved: {
				addListener: function(callback) {
					addEventListener('permissions.onRemoved', callback);
				},
				removeListener: function(callback) {
					removeEventListener('permissions.onRemoved', callback);
				},
				hasListener: function(callback) {
					return hasEventListener('permissions.onRemoved', callback);
				}
			}
		}
	};

	// Create extension namespace (legacy alias to runtime)
	window.browser.extension = {
		getURL: window.browser.runtime.getURL,
		getManifest: window.browser.runtime.getManifest,
		lastError: null
	};

	// Chrome compatibility
	window.chrome = window.browser;

	// Handler for incoming native events (responses + port/runtime events)
	window.__dumberWebExtReceive = function(evt) {
		console.log('[dumber-api] __dumberWebExtReceive called with event:', evt);
		handleIncomingEvent(evt);
		console.log('[dumber-api] __dumberWebExtReceive completed');
	};

	// Polyfill requestIdleCallback if not available (WebKitGTK doesn't provide it in extension contexts)
	if (typeof window.requestIdleCallback === 'undefined') {
		window.requestIdleCallback = function(callback, options) {
			const start = Date.now();
			return setTimeout(function() {
				callback({
					didTimeout: false,
					timeRemaining: function() {
						return Math.max(0, 50 - (Date.now() - start));
					}
				});
			}, 1);
		};
		window.cancelIdleCallback = function(id) {
			clearTimeout(id);
		};
	}

	console.log('[dumber-api] ✓ Browser API bridge loaded successfully for extension:', window._dumberExtensionID);
	console.log('[dumber-api] browser object:', window.browser);
	console.log('[dumber-api] chrome object:', window.chrome);
})();
`
}
