# Native WebExtension plumbing (Epiphany-style)

Goal: drop the TS shim and expose real `browser/chrome` objects from a WebKit web process extension, routed through Go.

## Architecture (mirrors Epiphany)
- **Scheme handler (UI process):** register `dumb-extension://` + mark secure; serve resources from extension dir with web_accessible_resources checks. Reference: `src/webextension/ephy-web-extension-manager.c` `ephy_webextension_scheme_cb`.
- **Web process extension (.so):** loaded via `webkit_web_context_set_web_extensions_directory`. In C, build `browser/i18n/runtime/extension` objects with JSC and a single bridge function (`dumber_send_message`) that forwards to the UI via `WebKitUserMessage`. Reference: `embed/web-process-extension/ephy-webextension-common.c` + `webextensions-common.js`.
- **UI dispatcher (Go):** receive `WebKitUserMessage` and dispatch to per-API handlers (runtime/storage/tabs/etc.), returning JSON strings. Reference pattern: `src/webextension/api/*.c`.
- **Storage:** per-extension JSON on disk; hydrate into the web process on load; write-through on mutations.
- **Background/popup:** create hidden background WebView and popup WebView per extension, both loaded via `dumb-extension://{id}/...`.
- **Content scripts:** per-extension `WebKitScriptWorld`, inject minimal JS that forwards to native API; no TS polyfill.
- **i18n:** load `_locales/<locale>/messages.json` in Go, pick locale order (manifest default → navigator → language-only → en), pass resolved dict to web process for synchronous `i18n.getMessage`.

## Files to create
- `native/webext/dumber-webextension-common.c`: JSC wiring for `browser`, `i18n.getMessage`, `extension.getURL/Manifest`, bridge function.
- `native/webext/dumber-webprocess-extension.c`: entry point; hook page/world creation; expose API only on extension pages; set up message handlers.
- `native/webext/resources/webextensions-common.js`: small JS glue (ported from Epiphany) that forwards to native bridge.
- `internal/webext/native/dispatcher.go`: handles `WebKitUserMessage` from the web process; routes to Go handlers; serializes replies.
- `internal/webext/native/storage.go`: JSON-backed per-extension storage helpers.
- `internal/webext/native/i18n.go`: locale resolution + message dict loading.
- Build wiring to replace the embedded `assets/webext/dumber-webext.so` with a locally-built .so (fallback to embedded until build is ready).

## Integration steps (incremental)
1) Add web process extension C scaffolding and JS glue; ensure it compiles to `dumber-webext.so`.
2) In Go startup: call `EnsureWebExtSO`; set `webkit_web_context_set_web_extensions_directory`; register `WebKitUserContentManager::script-message-received` to dispatch.
3) Implement runtime/storage handlers in Go; ensure `i18n` uses real locale dicts.
4) Stop injecting `gui/src/injected/modules/webext-api.ts` on extension pages once native objects are present.
5) Expand API surface as needed (tabs, browserAction, contextMenus).

References in Epiphany tree:
- `embed/web-process-extension/ephy-webextension-common.c` (i18n/runtime/extension)
- `src/webextension/ephy-web-extension-manager.c` (scheme handler + dispatcher)
- `embed/web-process-extension/resources/webextensions-common.js` (JS glue)
