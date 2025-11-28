package debugger

// ExpectedAPIs defines all WebExtension and Web APIs to check.
// Legend:
// - Required: true = critical for extensions like uBlock Origin
// - Status: "implemented", "stub", "missing" - documentation only
var ExpectedAPIs = []APIPath{
	// browser.storage.local
	{"browser.storage.local.get", "function", true, "implemented"},
	{"browser.storage.local.set", "function", true, "implemented"},
	{"browser.storage.local.remove", "function", true, "implemented"},
	{"browser.storage.local.clear", "function", false, "implemented"},
	{"browser.storage.local.getBytesInUse", "function", false, "implemented"},
	{"browser.storage.local.onChanged.addListener", "function", false, "implemented"},

	// browser.runtime
	{"browser.runtime.id", "string", true, "implemented"},
	{"browser.runtime.lastError", "object", false, "implemented"},
	{"browser.runtime.getManifest", "function", true, "implemented"},
	{"browser.runtime.getURL", "function", true, "implemented"},
	{"browser.runtime.sendMessage", "function", false, "stub"},
	{"browser.runtime.connect", "function", false, "implemented"},
	{"browser.runtime.onMessage.addListener", "function", true, "implemented"},
	{"browser.runtime.onConnect.addListener", "function", false, "implemented"},
	{"browser.runtime.getBrowserInfo", "function", true, "implemented"},
	{"browser.runtime.getPlatformInfo", "function", true, "implemented"},
	{"browser.runtime.openOptionsPage", "function", false, "stub"},

	// browser.tabs
	{"browser.tabs.query", "function", true, "implemented"},
	{"browser.tabs.get", "function", true, "implemented"},
	{"browser.tabs.getCurrent", "function", false, "implemented"},
	{"browser.tabs.create", "function", false, "stub"},
	{"browser.tabs.update", "function", false, "stub"},
	{"browser.tabs.remove", "function", false, "stub"},
	{"browser.tabs.sendMessage", "function", false, "stub"},
	{"browser.tabs.insertCSS", "function", false, "stub"},
	{"browser.tabs.removeCSS", "function", false, "stub"},
	{"browser.tabs.executeScript", "function", false, "stub"},
	{"browser.tabs.TAB_ID_NONE", "number", false, "implemented"},
	{"browser.tabs.reload", "function", true, "implemented"},
	{"browser.tabs.setZoom", "function", false, "stub"},
	{"browser.tabs.getZoom", "function", false, "stub"},

	// browser.cookies
	{"browser.cookies.get", "function", true, "implemented"},
	{"browser.cookies.set", "function", true, "implemented"},
	{"browser.cookies.remove", "function", true, "implemented"},
	{"browser.cookies.getAll", "function", true, "implemented"},
	{"browser.cookies.getAllCookieStores", "function", false, "implemented"},
	{"browser.cookies.onChanged.addListener", "function", false, "stub"},

	// browser.downloads
	{"browser.downloads.download", "function", false, "implemented"},
	{"browser.downloads.cancel", "function", false, "implemented"},
	{"browser.downloads.open", "function", false, "implemented"},
	{"browser.downloads.show", "function", false, "implemented"},
	{"browser.downloads.search", "function", false, "implemented"},
	{"browser.downloads.erase", "function", false, "implemented"},
	{"browser.downloads.removeFile", "function", false, "implemented"},
	{"browser.downloads.showDefaultFolder", "function", false, "implemented"},
	{"browser.downloads.onCreated.addListener", "function", false, "implemented"},
	{"browser.downloads.onChanged.addListener", "function", false, "implemented"},

	// browser.pageAction
	{"browser.pageAction.setIcon", "function", false, "implemented"},
	{"browser.pageAction.setTitle", "function", false, "implemented"},
	{"browser.pageAction.getTitle", "function", false, "implemented"},
	{"browser.pageAction.show", "function", false, "implemented"},
	{"browser.pageAction.hide", "function", false, "implemented"},
	{"browser.pageAction.onClicked.addListener", "function", false, "stub"},

	// browser.browserAction / browser.action
	{"browser.browserAction.setIcon", "function", false, "stub"},
	{"browser.browserAction.setTitle", "function", false, "stub"},
	{"browser.browserAction.setBadgeText", "function", false, "stub"},
	{"browser.browserAction.setBadgeBackgroundColor", "function", false, "stub"},
	{"browser.browserAction.setBadgeTextColor", "function", false, "stub"},
	{"browser.browserAction.getPopup", "function", false, "stub"},
	{"browser.browserAction.setPopup", "function", false, "stub"},
	{"browser.browserAction.onClicked.addListener", "function", false, "stub"},
	{"browser.action.setIcon", "function", false, "stub"}, // MV3 alias

	// browser.alarms
	{"browser.alarms.create", "function", false, "implemented"},
	{"browser.alarms.get", "function", false, "implemented"},
	{"browser.alarms.getAll", "function", false, "implemented"},
	{"browser.alarms.clear", "function", false, "implemented"},
	{"browser.alarms.clearAll", "function", false, "implemented"},
	{"browser.alarms.onAlarm.addListener", "function", false, "implemented"},

	// browser.webRequest
	{"browser.webRequest.onBeforeRequest.addListener", "function", true, "implemented"},
	{"browser.webRequest.onBeforeSendHeaders.addListener", "function", false, "implemented"},
	{"browser.webRequest.onHeadersReceived.addListener", "function", true, "implemented"},
	{"browser.webRequest.onCompleted.addListener", "function", false, "implemented"},
	{"browser.webRequest.onErrorOccurred.addListener", "function", false, "implemented"},
	{"browser.webRequest.handlerBehaviorChanged", "function", false, "implemented"},
	{"browser.webRequest.ResourceType", "object", false, "implemented"},

	// browser.webNavigation
	{"browser.webNavigation.getFrame", "function", false, "stub"},
	{"browser.webNavigation.getAllFrames", "function", false, "stub"},
	{"browser.webNavigation.onBeforeNavigate.addListener", "function", false, "stub"},
	{"browser.webNavigation.onCommitted.addListener", "function", false, "stub"},
	{"browser.webNavigation.onDOMContentLoaded.addListener", "function", false, "stub"},
	{"browser.webNavigation.onCompleted.addListener", "function", false, "stub"},

	// browser.contextMenus / browser.menus
	{"browser.contextMenus.create", "function", false, "stub"},
	{"browser.contextMenus.update", "function", false, "stub"},
	{"browser.contextMenus.remove", "function", false, "stub"},
	{"browser.contextMenus.removeAll", "function", false, "stub"},
	{"browser.contextMenus.onClicked.addListener", "function", false, "stub"},
	{"browser.menus.create", "function", false, "stub"}, // Firefox alias

	// browser.i18n
	{"browser.i18n.getMessage", "function", true, "implemented"},
	{"browser.i18n.getUILanguage", "function", false, "implemented"},
	{"browser.i18n.getAcceptLanguages", "function", false, "implemented"},
	{"browser.i18n.detectLanguage", "function", false, "stub"},

	// browser.notifications
	{"browser.notifications.create", "function", false, "stub"},
	{"browser.notifications.clear", "function", false, "stub"},
	{"browser.notifications.getAll", "function", false, "stub"},
	{"browser.notifications.update", "function", false, "stub"},
	{"browser.notifications.onClicked.addListener", "function", false, "stub"},
	{"browser.notifications.onClosed.addListener", "function", false, "stub"},

	// browser.commands
	{"browser.commands.getAll", "function", false, "stub"},
	{"browser.commands.reset", "function", false, "stub"},
	{"browser.commands.update", "function", false, "stub"},
	{"browser.commands.onCommand.addListener", "function", false, "stub"},

	// browser.permissions
	{"browser.permissions.getAll", "function", false, "implemented"},
	{"browser.permissions.contains", "function", false, "stub"},
	{"browser.permissions.request", "function", false, "stub"},
	{"browser.permissions.remove", "function", false, "stub"},

	// browser.extension
	{"browser.extension.getURL", "function", false, "implemented"},
	{"browser.extension.getBackgroundPage", "function", false, "stub"},
	{"browser.extension.getViews", "function", false, "stub"},
	{"browser.extension.isAllowedIncognitoAccess", "function", false, "stub"},
	{"browser.extension.isAllowedFileSchemeAccess", "function", false, "stub"},
	{"browser.extension.inIncognitoContext", "boolean", false, "implemented"},

	// browser.windows
	{"browser.windows.get", "function", false, "stub"},
	{"browser.windows.getCurrent", "function", false, "stub"},
	{"browser.windows.getLastFocused", "function", false, "stub"},
	{"browser.windows.getAll", "function", false, "stub"},
	{"browser.windows.create", "function", false, "stub"},
	{"browser.windows.update", "function", false, "stub"},
	{"browser.windows.remove", "function", false, "stub"},
	{"browser.windows.WINDOW_ID_NONE", "number", false, "implemented"},
	{"browser.windows.WINDOW_ID_CURRENT", "number", false, "implemented"},

	// browser.scripting (MV3)
	{"browser.scripting.executeScript", "function", false, "stub"},
	{"browser.scripting.insertCSS", "function", false, "stub"},
	{"browser.scripting.removeCSS", "function", false, "stub"},
	{"browser.scripting.registerContentScripts", "function", false, "stub"},
	{"browser.scripting.getRegisteredContentScripts", "function", false, "stub"},
	{"browser.scripting.unregisterContentScripts", "function", false, "stub"},

	// browser.idle
	{"browser.idle.queryState", "function", false, "stub"},
	{"browser.idle.setDetectionInterval", "function", false, "stub"},
	{"browser.idle.getAutoLockDelay", "function", false, "stub"},
	{"browser.idle.onStateChanged.addListener", "function", false, "stub"},

	// Web APIs (browserjs)
	// DOM & Parsing
	{"DOMParser", "function", true, "implemented"},
	{"XMLSerializer", "function", false, "implemented"},
	{"document", "object", false, "implemented"},

	// Binary & Files
	{"Blob", "function", true, "implemented"},
	{"File", "function", false, "implemented"},
	{"FileReader", "function", false, "implemented"},
	{"FormData", "function", true, "implemented"},

	// Fetch & XHR
	{"XMLHttpRequest", "function", true, "implemented"},
	{"fetch", "function", true, "implemented"},
	{"AbortController", "function", true, "implemented"},
	{"AbortSignal", "object", false, "implemented"},

	// Timers
	{"setTimeout", "function", true, "implemented"},
	{"setInterval", "function", true, "implemented"},
	{"clearTimeout", "function", true, "implemented"},
	{"clearInterval", "function", true, "implemented"},

	// Encoding
	{"TextEncoder", "function", true, "implemented"},
	{"TextDecoder", "function", true, "implemented"},
	{"atob", "function", true, "implemented"},
	{"btoa", "function", true, "implemented"},

	// URL
	{"URL", "function", true, "implemented"},
	{"URLSearchParams", "function", true, "implemented"},

	// Storage
	{"localStorage", "object", false, "implemented"},

	// Console
	{"console.log", "function", true, "implemented"},
	{"console.warn", "function", true, "implemented"},
	{"console.error", "function", true, "implemented"},
	{"console.info", "function", true, "implemented"},
	{"console.debug", "function", false, "implemented"},

	// Other
	{"structuredClone", "function", true, "implemented"},
	{"DOMException", "function", false, "implemented"},
	{"performance.now", "function", false, "implemented"},
	{"crypto.getRandomValues", "function", false, "implemented"},
	{"Intl.DateTimeFormat", "function", false, "implemented"},
}
