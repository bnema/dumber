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
	defer func() {
		// Release callback references
		C.unref_gobject(cbData.resolveCallback)
		C.unref_gobject(cbData.rejectCallback)
	}()

	reply, err := page.SendMessageToViewFinish(res)
	if err != nil {
		log.Printf("[native-api] Failed to send message: %v", err)
		rejectWithError((*C.JSCValue)(cbData.rejectCallback), err.Error())
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
	// Debug: Function entry
	debugMsg := fmt.Sprintf("[webext] WebProcess: installNativeBrowserAPIs called for %s", extData.extensionID)
	variant := glib.NewVariantString(debugMsg)
	msg := webkitwebprocessextension.NewUserMessage("debug:log", variant)
	page.SendMessageToView(context.Background(), msg, nil)

	if frame == nil {
		log.Printf("[native-api] No frame available")
		debugMsg := "[webext] WebProcess ERROR: frame is nil in installNativeBrowserAPIs"
		variant := glib.NewVariantString(debugMsg)
		msg := webkitwebprocessextension.NewUserMessage("debug:log", variant)
		page.SendMessageToView(context.Background(), msg, nil)
		return
	}

	jsContext := frame.JsContext()
	if jsContext == nil {
		log.Printf("[native-api] No JS context available")
		debugMsg := "[webext] WebProcess ERROR: JS context is nil"
		variant := glib.NewVariantString(debugMsg)
		msg := webkitwebprocessextension.NewUserMessage("debug:log", variant)
		page.SendMessageToView(context.Background(), msg, nil)
		return
	}

	debugMsg = "[webext] WebProcess: JS context obtained, injecting metadata"
	variant = glib.NewVariantString(debugMsg)
	msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
	page.SendMessageToView(context.Background(), msg, nil)

	// Inject extension metadata as global variables
	extIDValue := javascriptcore.NewValueString(jsContext, extData.extensionID)
	jsContext.SetValue("_dumberExtensionID", extIDValue)

	langValue := javascriptcore.NewValueString(jsContext, extData.uiLanguage)
	jsContext.SetValue("_dumberUILanguage", langValue)

	debugMsg = "[webext] WebProcess: Metadata injected, preparing manifest"
	variant = glib.NewVariantString(debugMsg)
	msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
	page.SendMessageToView(context.Background(), msg, nil)

	// Inject manifest
	if extData.manifest != "" {
		manifestValue := javascriptcore.NewValueFromJson(jsContext, extData.manifest)
		jsContext.SetValue("_dumberManifest", manifestValue)
		debugMsg = fmt.Sprintf("[webext] WebProcess: Manifest injected (%d bytes)", len(extData.manifest))
		variant = glib.NewVariantString(debugMsg)
		msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
		page.SendMessageToView(context.Background(), msg, nil)
	}

	// Inject translations
	if extData.translations != "" {
		translationsValue := javascriptcore.NewValueFromJson(jsContext, extData.translations)
		jsContext.SetValue("_dumberTranslations", translationsValue)
		debugMsg = fmt.Sprintf("[webext] WebProcess: Translations injected (%d bytes)", len(extData.translations))
		variant = glib.NewVariantString(debugMsg)
		msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
		page.SendMessageToView(context.Background(), msg, nil)
	}

	// CRITICAL: Register native function FIRST, before evaluating JavaScript
	// The JavaScript bridge expects window._dumberSendMessage to exist
	debugMsg = "[webext] WebProcess: Setting up API message handler (registering _dumberSendMessage)"
	variant = glib.NewVariantString(debugMsg)
	msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
	page.SendMessageToView(context.Background(), msg, nil)

	setupAPIMessageHandler(page, jsContext, extData.extensionID)

	// Now inject the browser API bridge JavaScript
	// It will find window._dumberSendMessage and create browser.* APIs
	debugMsg = "[webext] WebProcess: Evaluating browser API bridge JavaScript"
	variant = glib.NewVariantString(debugMsg)
	msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
	page.SendMessageToView(context.Background(), msg, nil)

	bridgeJS := getBrowserAPIBridgeJS()
	result := jsContext.Evaluate(bridgeJS)
	if result != nil {
		if exception := jsContext.Exception(); exception != nil {
			log.Printf("[native-api] Failed to inject browser API bridge: %v", exception.String())
			debugMsg = fmt.Sprintf("[webext] WebProcess ERROR: JS evaluation failed: %v", exception.String())
			variant = glib.NewVariantString(debugMsg)
			msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
			page.SendMessageToView(context.Background(), msg, nil)
		} else {
			log.Printf("[native-api] Browser API bridge injected for extension %s", extData.extensionID)
			debugMsg = fmt.Sprintf("[webext] WebProcess SUCCESS: Browser API bridge injected for %s", extData.extensionID)
			variant = glib.NewVariantString(debugMsg)
			msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
			page.SendMessageToView(context.Background(), msg, nil)
		}
	} else {
		debugMsg = "[webext] WebProcess ERROR: jsContext.Evaluate returned nil"
		variant = glib.NewVariantString(debugMsg)
		msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
		page.SendMessageToView(context.Background(), msg, nil)
	}

	debugMsg = fmt.Sprintf("[webext] WebProcess: installNativeBrowserAPIs completed for %s", extData.extensionID)
	variant = glib.NewVariantString(debugMsg)
	msg = webkitwebprocessextension.NewUserMessage("debug:log", variant)
	page.SendMessageToView(context.Background(), msg, nil)
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
					try { fn(msg); } catch (e) { console.error(e); }
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
			return;
		}
		switch (evt.type) {
		case 'runtime-message':
			runRuntimeMessageListeners(evt.message, evt.sender || null);
			break;
		case 'port-connect': {
			const port = createPort(evt.portId, evt.name || '', evt.sender || null);
			_dumberPorts.set(evt.portId, port);
			_dumberRuntimeOnConnect.forEach(fn => {
				try { fn(port); } catch (e) { console.error(e); }
			});
			break;
		}
		case 'port-message': {
			const port = _dumberPorts.get(evt.portId);
			if (port) {
				port._deliverMessage(evt.message);
			}
			break;
		}
		case 'port-disconnect': {
			const port = _dumberPorts.get(evt.portId);
			if (port) {
				port._remoteDisconnect();
			}
			break;
		}
		default:
			break;
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

			lastError: null
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

		storage: {
			local: {
				// All storage APIs are async and use message passing
				get: function(keys, callback) {
					const promise = sendAPICall('storage.local.get', keys);

					if (callback) {
						promise.then(callback).catch(err => {
							console.error('[dumber] storage.local.get error:', err);
						});
					}

					return promise;
				},

				set: function(items, callback) {
					const promise = sendAPICall('storage.local.set', items);

					if (callback) {
						promise.then(callback).catch(err => {
							console.error('[dumber] storage.local.set error:', err);
						});
					}

					return promise;
				},

				remove: function(keys, callback) {
					const promise = sendAPICall('storage.local.remove', keys);

					if (callback) {
						promise.then(callback).catch(err => {
							console.error('[dumber] storage.local.remove error:', err);
						});
					}

					return promise;
				},

				clear: function(callback) {
					const promise = sendAPICall('storage.local.clear');

					if (callback) {
						promise.then(callback).catch(err => {
							console.error('[dumber] storage.local.clear error:', err);
						});
					}

					return promise;
				}
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
		handleIncomingEvent(evt);
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
