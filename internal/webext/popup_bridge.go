package webext

// PopupBridgeJS is injected into extension popups to provide browser.* APIs
// It communicates with Go via webkit.messageHandlers.webext.postMessage()
const PopupBridgeJS = `
(function() {
	'use strict';

	if (typeof browser !== 'undefined') return; // Already injected

	// Pending promises awaiting responses
	const pendingCalls = new Map();
	let callId = 0;

	// Send message to Go and return a promise
	function sendMessage(api, method, args) {
		return new Promise((resolve, reject) => {
			const id = ++callId;
			pendingCalls.set(id, { resolve, reject });

			try {
				webkit.messageHandlers.webext.postMessage(JSON.stringify({
					id: id,
					api: api,
					method: method,
					args: args || []
				}));
			} catch (e) {
				pendingCalls.delete(id);
				reject(new Error('Failed to send message: ' + e.message));
			}

			// Timeout after 30 seconds
			setTimeout(() => {
				if (pendingCalls.has(id)) {
					pendingCalls.delete(id);
					reject(new Error('Request timeout'));
				}
			}, 30000);
		});
	}

	// Handle responses from Go
	window.__dumberPopupResponse = function(response) {
		try {
			const data = typeof response === 'string' ? JSON.parse(response) : response;
			const pending = pendingCalls.get(data.id);
			if (pending) {
				pendingCalls.delete(data.id);
				if (data.error) {
					pending.reject(new Error(data.error));
				} else {
					pending.resolve(data.result);
				}
			}
		} catch (e) {
			console.error('[popup-bridge] Failed to handle response:', e);
		}
	};

	// Event emitter for onChanged, onMessage, etc.
	function createEventTarget() {
		const listeners = [];
		return {
			addListener: function(callback) {
				if (typeof callback === 'function') {
					listeners.push(callback);
				}
			},
			removeListener: function(callback) {
				const idx = listeners.indexOf(callback);
				if (idx >= 0) listeners.splice(idx, 1);
			},
			hasListener: function(callback) {
				return listeners.includes(callback);
			},
			hasListeners: function() {
				return listeners.length > 0;
			},
			_emit: function(...args) {
				listeners.forEach(cb => {
					try { cb(...args); } catch (e) { console.error('[popup-bridge] Event listener error:', e); }
				});
			}
		};
	}

	// browser.storage API
	function createStorageArea(areaName) {
		return {
			get: function(keys) {
				return sendMessage('storage', 'get', [areaName, keys]);
			},
			set: function(items) {
				return sendMessage('storage', 'set', [areaName, items]);
			},
			remove: function(keys) {
				return sendMessage('storage', 'remove', [areaName, keys]);
			},
			clear: function() {
				return sendMessage('storage', 'clear', [areaName]);
			},
			getBytesInUse: function(keys) {
				return sendMessage('storage', 'getBytesInUse', [areaName, keys]);
			}
		};
	}

	const storage = {
		local: createStorageArea('local'),
		sync: createStorageArea('sync'),
		session: createStorageArea('session'),
		managed: createStorageArea('managed'),
		onChanged: createEventTarget()
	};

	// browser.runtime API
	const runtime = {
		id: '', // Will be set by Go
		lastError: null,

		sendMessage: function(extensionId, message, options) {
			// Handle overloaded signatures
			if (typeof extensionId === 'object' && message === undefined) {
				message = extensionId;
				extensionId = undefined;
			}
			return sendMessage('runtime', 'sendMessage', [extensionId, message, options]);
		},

		getURL: function(path) {
			// Construct extension URL - extension ID should be set
			const base = 'dumber-extension://' + (runtime.id || 'unknown');
			if (!path) return base + '/';
			if (path.startsWith('/')) return base + path;
			return base + '/' + path;
		},

		getManifest: function() {
			// Return cached manifest or fetch it
			return sendMessage('runtime', 'getManifest', []);
		},

		getPlatformInfo: function() {
			return Promise.resolve({
				os: 'linux',
				arch: 'x86-64'
			});
		},

		getBrowserInfo: function() {
			return Promise.resolve({
				name: 'Dumber',
				vendor: 'Dumber Browser',
				version: '1.0.0',
				buildID: '20240101'
			});
		},

		openOptionsPage: function() {
			return sendMessage('runtime', 'openOptionsPage', []);
		},

		setUninstallURL: function(url) {
			return sendMessage('runtime', 'setUninstallURL', [url]);
		},

		connect: function(extensionId, connectInfo) {
			// Stub - port-based messaging not fully implemented
			console.warn('[popup-bridge] runtime.connect not fully implemented');
			return {
				name: connectInfo?.name || '',
				disconnect: function() {},
				postMessage: function(msg) {},
				onMessage: createEventTarget(),
				onDisconnect: createEventTarget()
			};
		},

		onMessage: createEventTarget(),
		onConnect: createEventTarget(),
		onInstalled: createEventTarget(),
		onStartup: createEventTarget(),
		onSuspend: createEventTarget(),
		onUpdateAvailable: createEventTarget()
	};

	// browser.tabs API
	const tabs = {
		query: function(queryInfo) {
			return sendMessage('tabs', 'query', [queryInfo]);
		},
		get: function(tabId) {
			return sendMessage('tabs', 'get', [tabId]);
		},
		getCurrent: function() {
			return sendMessage('tabs', 'getCurrent', []);
		},
		create: function(createProperties) {
			return sendMessage('tabs', 'create', [createProperties]);
		},
		update: function(tabId, updateProperties) {
			return sendMessage('tabs', 'update', [tabId, updateProperties]);
		},
		remove: function(tabIds) {
			return sendMessage('tabs', 'remove', [tabIds]);
		},
		sendMessage: function(tabId, message, options) {
			return sendMessage('tabs', 'sendMessage', [tabId, message, options]);
		},
		executeScript: function(tabId, details) {
			return sendMessage('tabs', 'executeScript', [tabId, details]);
		},
		insertCSS: function(tabId, details) {
			return sendMessage('tabs', 'insertCSS', [tabId, details]);
		},

		onCreated: createEventTarget(),
		onUpdated: createEventTarget(),
		onRemoved: createEventTarget(),
		onActivated: createEventTarget()
	};

	// browser.windows API
	const windows = {
		get: function(windowId, getInfo) {
			return sendMessage('windows', 'get', [windowId, getInfo]);
		},
		getCurrent: function(getInfo) {
			return sendMessage('windows', 'getCurrent', [getInfo]);
		},
		getLastFocused: function(getInfo) {
			return sendMessage('windows', 'getLastFocused', [getInfo]);
		},
		getAll: function(getInfo) {
			return sendMessage('windows', 'getAll', [getInfo]);
		},
		create: function(createData) {
			return sendMessage('windows', 'create', [createData]);
		},
		update: function(windowId, updateInfo) {
			return sendMessage('windows', 'update', [windowId, updateInfo]);
		},
		remove: function(windowId) {
			return sendMessage('windows', 'remove', [windowId]);
		},

		WINDOW_ID_NONE: -1,
		WINDOW_ID_CURRENT: -2,

		onCreated: createEventTarget(),
		onRemoved: createEventTarget(),
		onFocusChanged: createEventTarget()
	};

	// browser.i18n API
	const i18n = {
		getMessage: function(messageName, substitutions) {
			// Synchronous call - use cached messages or return key
			return sendMessage('i18n', 'getMessage', [messageName, substitutions]);
		},
		getUILanguage: function() {
			return navigator.language || 'en';
		},
		detectLanguage: function(text) {
			return sendMessage('i18n', 'detectLanguage', [text]);
		},
		getAcceptLanguages: function() {
			return Promise.resolve(navigator.languages || ['en']);
		}
	};

	// browser.browserAction/action API
	const browserAction = {
		setIcon: function(details) {
			return sendMessage('browserAction', 'setIcon', [details]);
		},
		setTitle: function(details) {
			return sendMessage('browserAction', 'setTitle', [details]);
		},
		getTitle: function(details) {
			return sendMessage('browserAction', 'getTitle', [details]);
		},
		setBadgeText: function(details) {
			return sendMessage('browserAction', 'setBadgeText', [details]);
		},
		getBadgeText: function(details) {
			return sendMessage('browserAction', 'getBadgeText', [details]);
		},
		setBadgeBackgroundColor: function(details) {
			return sendMessage('browserAction', 'setBadgeBackgroundColor', [details]);
		},
		getBadgeBackgroundColor: function(details) {
			return sendMessage('browserAction', 'getBadgeBackgroundColor', [details]);
		},
		setPopup: function(details) {
			return sendMessage('browserAction', 'setPopup', [details]);
		},
		getPopup: function(details) {
			return sendMessage('browserAction', 'getPopup', [details]);
		},
		openPopup: function() {
			return sendMessage('browserAction', 'openPopup', []);
		},
		enable: function(tabId) {
			return sendMessage('browserAction', 'enable', [tabId]);
		},
		disable: function(tabId) {
			return sendMessage('browserAction', 'disable', [tabId]);
		},

		onClicked: createEventTarget()
	};

	// browser.extension API (deprecated but still used)
	const extension = {
		getURL: function(path) {
			return runtime.getURL(path);
		},
		getBackgroundPage: function() {
			// Not supported in this architecture
			return Promise.resolve(null);
		},
		isAllowedIncognitoAccess: function() {
			return Promise.resolve(false);
		},
		isAllowedFileSchemeAccess: function() {
			return Promise.resolve(false);
		}
	};

	// browser.permissions API
	const permissions = {
		contains: function(permissions) {
			return sendMessage('permissions', 'contains', [permissions]);
		},
		getAll: function() {
			return sendMessage('permissions', 'getAll', []);
		},
		request: function(permissions) {
			return sendMessage('permissions', 'request', [permissions]);
		},
		remove: function(permissions) {
			return sendMessage('permissions', 'remove', [permissions]);
		},

		onAdded: createEventTarget(),
		onRemoved: createEventTarget()
	};

	// browser.alarms API
	const alarms = {
		create: function(name, alarmInfo) {
			if (typeof name === 'object') {
				alarmInfo = name;
				name = '';
			}
			return sendMessage('alarms', 'create', [name, alarmInfo]);
		},
		get: function(name) {
			return sendMessage('alarms', 'get', [name]);
		},
		getAll: function() {
			return sendMessage('alarms', 'getAll', []);
		},
		clear: function(name) {
			return sendMessage('alarms', 'clear', [name]);
		},
		clearAll: function() {
			return sendMessage('alarms', 'clearAll', []);
		},

		onAlarm: createEventTarget()
	};

	// browser.notifications API (stub)
	const notifications = {
		create: function(id, options) {
			return sendMessage('notifications', 'create', [id, options]);
		},
		update: function(id, options) {
			return sendMessage('notifications', 'update', [id, options]);
		},
		clear: function(id) {
			return sendMessage('notifications', 'clear', [id]);
		},
		getAll: function() {
			return sendMessage('notifications', 'getAll', []);
		},

		onClicked: createEventTarget(),
		onButtonClicked: createEventTarget(),
		onClosed: createEventTarget(),
		onShown: createEventTarget()
	};

	// browser.contextMenus API (stub)
	const contextMenus = {
		create: function(createProperties, callback) {
			// Stub
			if (callback) callback();
			return Math.random().toString(36).substr(2, 9);
		},
		update: function(id, updateProperties, callback) {
			if (callback) callback();
		},
		remove: function(menuItemId, callback) {
			if (callback) callback();
		},
		removeAll: function(callback) {
			if (callback) callback();
		},

		onClicked: createEventTarget()
	};

	// Assemble browser object
	const browser = {
		storage: storage,
		runtime: runtime,
		tabs: tabs,
		windows: windows,
		i18n: i18n,
		browserAction: browserAction,
		action: browserAction, // MV3 alias
		extension: extension,
		permissions: permissions,
		alarms: alarms,
		notifications: notifications,
		contextMenus: contextMenus,
		menus: contextMenus // Firefox alias
	};

	// Also provide chrome.* for compatibility
	window.browser = browser;
	window.chrome = browser;

	console.log('[popup-bridge] browser.* APIs injected');
})();
`
