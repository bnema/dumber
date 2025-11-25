# WebKit ES6 Module Loading Issue - Extension Popups

## Problem Summary

When clicking on uBlock Origin or Bitwarden extension icons, a new pane opens but shows only a colored background. The HTML loads correctly (including CSS and fonts), but **ES6 JavaScript modules are never loaded**.

The HTML contains `<script type="module">` tags with correct `dumb-extension://` URLs, but WebKit never makes network requests for those JavaScript files.

## Observed Behavior

1. **Scheme handler receives**: HTML, CSS, font files
2. **Scheme handler DOES NOT receive**: JavaScript file requests
3. **Network tab shows**: Zero JS requests, HTML/CSS/fonts only
4. **Console shows**: GUI scripts from main WebView running in extension popup (fixed with `IsExtensionWebView` flag)
5. **Visual result**: Extension popup body with loading class, blank/colored background

## Technical Context

### Architecture
- Extension popups use custom `dumb-extension://` URI scheme
- Scheme is registered via `webkit_web_context_register_uri_scheme()`
- Background contexts run in Goja (pure Go JS engine), not WebView
- Popups are WebKitGTK WebViews with `web-extension-mode=WEBKIT_WEB_EXTENSION_MODE_MANIFESTV2`

### What Works
- `browser.*` API injection via WebProcess extension at `document-loaded`
- Custom scheme handling for HTML, CSS, fonts, images
- Background context Goja runtime (ES6 modules load fine there)
- CORS registration on SecurityManager

## Attempted Solutions (All Failed)

### 1. CSP Configuration
- **Tried**: Adding default CSP `"script-src 'self'; object-src 'self';"`
- **Result**: No effect on module loading
- **Reason**: WebKit is not requesting JS files at all, so CSP never applies

### 2. Registering Scheme as "Local"
- **Tried**: `sm.RegisterURISchemeAsLocal("dumb-extension")`
- **Result**: No effect
- **Hypothesis**: Local schemes can load subresources like file://

### 3. Dedicated WebContext for Extension Popups
- **Tried**: Creating separate `WebContext` with scheme registered early
- **Result**: "Cannot register URI scheme dumb-extension more than once" error
- **Discovery**: Main browser context IS the default context

### 4. Removing `web-extension-mode` Property
- **Tried**: Creating WebView WITHOUT `web-extension-mode=ManifestV2`
- **Result**: GUI scripts from main browser were injected into popup
- **Fix**: Added `IsExtensionWebView` config flag to skip UserContentManager injection

### 5. Disabling CSP in WebView Creation
- **Tried**: Not setting `default-content-security-policy` property
- **Result**: No effect on module loading

### 6. ApplyURISchemesToDefaultContext
- **Tried**: Registering scheme handlers on `webkit_web_context_get_default()`
- **Result**: Error - scheme already registered (proves default == main context)

## Key Observations

1. **The scheme IS registered** on the WebContext used by extension WebViews
2. **HTML with `<script type="module">` tags IS parsed** by WebKit
3. **WebKit DOES NOT initiate requests** for the JavaScript modules
4. **This is NOT a CSP issue** - the requests never happen
5. **This is NOT a CORS issue** - the requests never happen

## Comparison with Epiphany

Epiphany (GNOME Web) successfully loads extension popups. Their approach:
- Uses `ephy-webextension://` scheme
- Registers scheme as secure and CORS-enabled
- Creates extension WebViews with `web-extension-mode=ManifestV2`
- Uses related-view for session sharing

We follow the same pattern but modules still don't load.

## Hypothesis

WebKit may have internal restrictions on ES6 module loading from custom URI schemes that are not documented. Possible factors:
1. Same-origin policy quirks with custom schemes
2. Module loading requires specific scheme capabilities not exposed via GTK bindings
3. Bug in WebKitGTK when combining custom schemes + extension mode

## Next Steps to Investigate

1. **Test with file:// scheme** - Do ES6 modules load from file:// URLs?
2. **Inspect WebKit source code** - Look at `ModuleFetcherBase` and related classes
3. **Test Epiphany with debug logs** - See if their JS requests go through scheme handler
4. **Try data: URLs for inline modules** - Bypass scheme loading entirely
5. **Contact WebKitGTK maintainers** - Ask about custom scheme + module loading

## Useful References

- WebKitGTK API: `webkit_security_manager_register_uri_scheme_as_*`
- Epiphany source: `embed/ephy-web-extension-manager.c`
- WebKit source: `Source/WebCore/loader/ModuleFetcherBase.cpp`

## Cleanup Status

Experimental code has been reverted. Kept changes:
- `IsExtensionWebView` flag (prevents GUI script injection)
- `isMessageHeadersValid` helper (fixes gotk4 binding bug)
- `GetContentSecurityPolicy()` usage (cleaner CSP handling)
