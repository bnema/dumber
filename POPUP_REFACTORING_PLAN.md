# Popup Window Refactoring Plan

## Executive Summary

The current popup window implementation causes critical SIGSEGV crashes due to improper lifecycle management that conflicts with WebKitGTK's internal architecture. This document outlines the issues, root causes, and a comprehensive refactoring plan to align with WebKitGTK's expected signal-based popup lifecycle.

## Issues Faced

### 1. Critical Crash: SIGSEGV on Popup Close

**Symptom:**
```
signal 11 received but handler not on signal stack
fatal error: non-Go code set up signal handler without SA_ONSTACK flag
```

**Occurrences:**
- OAuth popup completion (Google, Notion login)
- Rapid popup open/close cycles (popup blocker detection)
- Any programmatic popup closure

**Impact:** Browser crash, data loss, poor user experience

### 2. GTK Bloom Filter Corruption

**Symptom:**
```
[workspace] Skipping CSS, pane 0xc00015c000 reparented 0.6ms ago
```

**Root Cause:** CSS class modifications during widget reparenting operations before GTK widget hierarchy stabilizes

**Attempted Fixes (all failed):**
- Boolean flag `pendingReparent` cleared too early
- Timestamp-based CSS blocking (150ms safety window)
- `StopLoading()` before close (broke OAuth callback)

### 3. WebKit/GTK Race Conditions

**Manifestations:**
- localStorage JS injection immediately before `ClosePane()` triggered WebKit JS engine during GTK reparenting
- Rapid popup opens (<1s) caused WebKit rendering pipeline initialization conflicts with GTK widget destruction
- WebKit signal handlers firing after GTK widgets destroyed

## Root Cause Analysis

### Architecture Violation: Bypassing WebKit Popup Lifecycle

**Current Implementation Flow:**
```
JavaScript window.open()
    ↓
main-world.js intercepts
    ↓
postMessage to Go
    ↓
messaging.Handler
    ↓
WorkspaceManager.handleIntentAsPopup()
    ↓
Manual WebView creation
    ↓
Manual pane insertion
    ↓
Manual close (ClosePane)
```

**Problem:** This completely bypasses WebKit's native popup management system.

### WebKitGTK's Expected Popup Lifecycle

**Reference:** `/usr/include/webkitgtk-6.0/webkit/WebKitWebView.h:243-259`

```c
struct _WebKitWebViewClass {
    // ... other signals ...

    GtkWidget *(* create)         (WebKitWebView          *web_view,
                                   WebKitNavigationAction *navigation_action);

    void       (* ready_to_show)  (WebKitWebView          *web_view);

    void       (* close)          (WebKitWebView          *web_view);

    // ... other signals ...
};
```

**WebKit's Lifecycle Contract:**

1. **`create` signal**: WebKit requests new WebView for popup
   - Application creates and configures new WebView
   - Application returns WebView to WebKit
   - **WebKit takes ownership of loading and content rendering**

2. **`ready-to-show` signal**: WebKit indicates popup is ready
   - WebKit has finished initial setup
   - Safe to show/position window/pane
   - Window properties (size, features) are available

3. **`close` signal**: WebKit requests popup closure
   - Emitted when JavaScript calls `window.close()`
   - Emitted when page navigates away from popup context
   - **Application should destroy widgets in response**

### Why Current Implementation Fails

1. **No signal connection**: Application never connects to WebKit's `create` signal
2. **Manual lifecycle**: Application creates WebViews independently, breaking WebKit's internal reference tracking
3. **Premature destruction**: `ClosePane()` destroys GTK widgets while WebKit still has:
   - Active signal handlers
   - Compositor threads running
   - Rendering pipeline active
   - JS engine references

4. **Race conditions**: JavaScript message handling races with WebKit's internal operations

## Reference Implementations

### Epiphany (GNOME Web Browser)

**File:** `/home/brice/dev/clone/epiphany/src/ephy-window.c`

**Key Implementation Points:**

1. **Signal Connection** (line 2636-2638):
```c
g_signal_connect_object (web_view, "create",
                         G_CALLBACK (create_web_view_cb),
                         window, 0);
```

2. **Create Handler** (lines 2034-2070):
```c
static WebKitWebView *
create_web_view_cb (WebKitWebView          *web_view,
                    WebKitNavigationAction *navigation_action,
                    EphyWindow             *window)
{
  EphyEmbed *embed;
  WebKitWebView *new_web_view;

  // Determine if popup should be new window or new tab
  if (new_windows_in_tabs_setting) {
    target_window = window;  // Same window (tab)
  } else {
    target_window = ephy_window_new ();  // New window
  }

  // Create new tab/window with parent relationship
  embed = ephy_shell_new_tab_full (..., web_view, ...);

  new_web_view = EPHY_GET_WEBKIT_WEB_VIEW_FROM_EMBED (embed);

  // Connect ready-to-show handler
  g_signal_connect_object (new_web_view, "ready-to-show",
                           G_CALLBACK (web_view_ready_cb),
                           web_view, 0);

  // Return new WebView to WebKit
  return new_web_view;
}
```

3. **Ready-to-Show Handler** (lines 2012-2031):
```c
static gboolean
web_view_ready_cb (WebKitWebView *web_view,
                   WebKitWebView *parent_web_view)
{
  EphyWindow *window;

  window = EPHY_WINDOW (gtk_widget_get_root (GTK_WIDGET (web_view)));

  // Apply window properties
  if (using_new_window) {
    ephy_window_configure_for_view (window, web_view);
  }

  // Show the window/tab
  gtk_widget_set_visible (GTK_WIDGET (window), TRUE);

  return TRUE;
}
```

4. **Close Handler**: Uses AdwTabView's built-in `close-page` signal (lines 3314-3360), which properly coordinates with WebKit

**Key Lessons from Epiphany:**
- ✅ Respects WebKit's signal-based lifecycle
- ✅ Creates WebView synchronously in `create` handler
- ✅ Returns WebView immediately to WebKit
- ✅ Defers visibility to `ready-to-show`
- ✅ Lets WebKit manage content loading
- ✅ Clean separation: positioning logic vs. lifecycle management

### WebKitGTK Documentation

**Reference:** https://webkitgtk.org/reference/webkitgtk/stable/signal.WebView.create.html

**Signal: `WebKitWebView::create`**

> This signal is emitted when a new WebKitWebView should be created for a popup window. The WebKitNavigationAction parameter contains information about the navigation action that triggered this signal. The signal handler should return the newly created WebKitWebView, or NULL if the popup window creation should be canceled.

**Signal: `WebKitWebView::ready-to-show`**

> Emitted after the WebKitWebView::create signal when the new view is ready to be shown. This signal allows you to show the new view and apply the window properties from webkit_web_view_get_window_properties() before the page is loaded.

**Signal: `WebKitWebView::close`**

> Emitted when the WebKitWebView is being closed by the web process. This signal is typically triggered by JavaScript window.close() calls. The signal handler should destroy the widget or otherwise handle the close request.

## Proposed Refactoring

### Phase 1: Implement Native WebKit Popup Signals

#### 1.1 Add CGO Bindings for Create Signal

**File:** `pkg/webkit/webview_popup.go` (new file)

```go
package webkit

/*
#cgo pkg-config: webkitgtk-6.0
#include <webkit/webkit.h>
#include <gtk/gtk.h>

// Forward declaration
extern WebKitWebView* goWebViewCreateCallback(WebKitWebView*, WebKitNavigationAction*, gpointer);

// C helper to connect create signal
static inline void connect_create_signal(WebKitWebView* view, gpointer user_data) {
    g_signal_connect(view, "create", G_CALLBACK(goWebViewCreateCallback), user_data);
}
*/
import "C"
import "unsafe"

// ConnectCreateSignal connects the WebView's create signal for popup handling
func (w *WebView) ConnectCreateSignal() {
    if w.view == nil {
        return
    }
    C.connect_create_signal((*C.WebKitWebView)(unsafe.Pointer(w.view.Native())), C.gpointer(unsafe.Pointer(w)))
}
```

**File:** `pkg/webkit/webview_popup_callback.c` (new file)

```c
#include <webkit/webkit.h>
#include "_cgo_export.h"

// CGO callback bridge for create signal
WebKitWebView* goWebViewCreateCallback(WebKitWebView* parent_view,
                                        WebKitNavigationAction* navigation_action,
                                        gpointer user_data) {
    // Call Go callback via CGO
    return handleWebViewCreate(user_data, navigation_action);
}
```

#### 1.2 Implement Go-side Create Handler

**File:** `pkg/webkit/webview.go` (additions)

```go
type WebView struct {
    // ... existing fields ...

    // Popup lifecycle handlers
    onPopupCreate       func(*NavigationAction) *WebView
    onReadyToShow       func()
}

//export handleWebViewCreate
func handleWebViewCreate(webViewPtr unsafe.Pointer, navActionPtr unsafe.Pointer) *C.WebKitWebView {
    // Recover WebView from pointer
    w := (*WebView)(webViewPtr)

    if w.onPopupCreate == nil {
        return nil  // No handler, cancel popup
    }

    // Parse navigation action
    navAction := parseNavigationAction(navActionPtr)

    // Call Go handler
    newWebView := w.onPopupCreate(navAction)

    if newWebView == nil {
        return nil
    }

    // Return WebKit's native view
    return (*C.WebKitWebView)(unsafe.Pointer(newWebView.view.Native()))
}

// RegisterPopupCreateHandler registers handler for WebKit's create signal
func (w *WebView) RegisterPopupCreateHandler(handler func(*NavigationAction) *WebView) {
    w.mu.Lock()
    defer w.mu.Unlock()
    w.onPopupCreate = handler
    w.ConnectCreateSignal()
}
```

#### 1.3 Connect Ready-to-Show and Close Signals

**File:** `pkg/webkit/webview.go` (setupEventHandlers addition)

```go
func (w *WebView) setupEventHandlers() {
    // ... existing handlers ...

    // Ready-to-show signal (already exists in gotk4 bindings)
    w.view.ConnectReadyToShow(func() {
        if w.onReadyToShow != nil {
            w.onReadyToShow()
        }
    })

    // Close signal (already connected, line 219-223)
    // Just ensure onClose is used for popup lifecycle
}
```

### Phase 2: Refactor Workspace Popup Management

#### 2.1 Remove JavaScript Interception for window.open

**File:** `gui/src/lib/main-world/window-intercept.ts`

**Change:** Remove `window.open` interception, let it reach WebKit natively

**Rationale:** WebKit's `create` signal will handle it properly

#### 2.2 Implement Workspace-Level Create Handler

**File:** `internal/app/browser/workspace_popup.go`

**Refactor:** Replace `handleIntentAsPopup()` with proper signal-based approach

```go
// setupPopupHandling connects WebView to workspace popup management
func (wm *WorkspaceManager) setupPopupHandling(webView *webkit.WebView, parentNode *paneNode) {
    webView.RegisterPopupCreateHandler(func(navAction *webkit.NavigationAction) *webkit.WebView {
        // Parse popup intent from navigation
        url := navAction.GetRequest().URI()
        features := navAction.GetWindowFeatures()

        // Determine popup type from config/URL
        popupType := wm.determinePopupType(url, features)

        // Create new WebView for popup
        popupView, err := wm.createWebViewFn()
        if err != nil {
            log.Printf("[workspace] Failed to create popup WebView: %v", err)
            return nil  // Cancels popup
        }

        // Create pane but DON'T insert yet (WebKit still loading)
        popupPane, err := wm.createPaneFn(popupView)
        if err != nil {
            log.Printf("[workspace] Failed to create popup pane: %v", err)
            return nil
        }

        // Store popup info for ready-to-show
        popupInfo := &pendingPopup{
            view:       popupView,
            pane:       popupPane,
            parentNode: parentNode,
            popupType:  popupType,
            url:        url,
            features:   features,
        }
        wm.pendingPopups[popupView.ID()] = popupInfo

        // Connect ready-to-show for this specific popup
        popupView.RegisterReadyToShowHandler(func() {
            wm.handlePopupReadyToShow(popupView.ID())
        })

        // Connect close handler
        popupView.RegisterCloseHandler(func() {
            wm.handlePopupClose(popupView.ID())
        })

        // Return to WebKit - it will manage loading
        return popupView
    })
}

// handlePopupReadyToShow inserts popup pane when WebKit says it's ready
func (wm *WorkspaceManager) handlePopupReadyToShow(popupID uint64) {
    info, ok := wm.pendingPopups[popupID]
    if !ok {
        return
    }
    delete(wm.pendingPopups, popupID)

    // NOW it's safe to insert into workspace
    switch info.popupType {
    case PopupTypeSplit:
        wm.insertPopupPane(info.parentNode, info.pane, "right")
    case PopupTypeStacked:
        wm.stackedPaneManager.StackPane(info.parentNode)
    case PopupTypeTabbed:
        // TODO: Implement tabbed popups
        wm.insertPopupPane(info.parentNode, info.pane, "right")
    }

    // Apply popup features (size, position, toolbar, etc.)
    wm.applyPopupFeatures(info.pane, info.features)
}

// handlePopupClose responds to WebKit's close signal
func (wm *WorkspaceManager) handlePopupClose(popupID uint64) {
    // Find pane by WebView ID
    var targetNode *paneNode
    for webView, node := range wm.viewToNode {
        if webView != nil && webView.ID() == popupID {
            targetNode = node
            break
        }
    }

    if targetNode == nil {
        return
    }

    // Clean close via existing ClosePane (but WebKit already stopped)
    if err := wm.ClosePane(targetNode); err != nil {
        log.Printf("[workspace] Error closing popup pane: %v", err)
    }
}
```

**Key Changes:**
- ✅ WebView created in `create` signal handler
- ✅ Returned immediately to WebKit
- ✅ Pane insertion deferred until `ready-to-show`
- ✅ Cleanup happens in response to `close` signal
- ✅ No manual lifecycle management

#### 2.3 Configuration-Based Popup Type

**File:** `internal/app/config/config.go`

```go
type PopupBehavior string

const (
    PopupBehaviorSplit   PopupBehavior = "split"
    PopupBehaviorStacked PopupBehavior = "stacked"
    PopupBehaviorTabbed  PopupBehavior = "tabbed"
    PopupBehaviorWindow  PopupBehavior = "window"  // Native popup window
)

type Config struct {
    // ... existing fields ...

    PopupBehavior PopupBehavior `json:"popup_behavior"`

    // Per-domain popup overrides
    PopupOverrides map[string]PopupBehavior `json:"popup_overrides"`
}
```

**Usage:**
```go
func (wm *WorkspaceManager) determinePopupType(url string, features *webkit.WindowFeatures) PopupType {
    // Check per-domain overrides
    domain := extractDomain(url)
    if override, ok := wm.app.config.PopupOverrides[domain]; ok {
        return popupBehaviorToType(override)
    }

    // Use global config
    return popupBehaviorToType(wm.app.config.PopupBehavior)
}
```

### Phase 3: Remove Temporary Workarounds

#### 3.1 Remove CSS Bloom Filter Workarounds

**Files to clean:**
- `internal/app/browser/workspace_types.go`: Remove `lastReparentTime`, `pendingHoverReattach`, `pendingFocusReattach`
- `internal/app/browser/workspace_utils.go`: Remove timestamp checking in `applyActivePaneBorder()`
- `internal/app/browser/workspace_pane_ops.go`: Remove timestamp setting in reparenting operations

**Rationale:** Proper WebKit lifecycle management eliminates the race conditions

#### 3.2 Remove LocalStorage Cleanup Workarounds

**File:** `internal/app/browser/workspace_manager.go`

**Remove:** All localStorage cleanup code in `close-popup` handler

**Rationale:**
- Popup's WebView will be destroyed cleanly by WebKit
- Parent-child communication should use `window.postMessage` instead of localStorage
- If localStorage is really needed, clean it in JavaScript before calling `window.close()`

#### 3.3 Remove JavaScript Message-Based Popup Flow

**Files:**
- `gui/src/lib/main-world/window-intercept.ts`: Remove `window.open` interception
- `internal/app/messaging/handler.go`: Remove `WindowIntent` and `create-pane` handling
- `internal/app/messaging/deduplicator.go`: Potentially remove (may still be useful for other message types)

### Phase 4: Testing & Validation

#### 4.1 Test Scenarios

1. **OAuth Flows:**
   - Google login
   - Notion login
   - GitHub OAuth

2. **Rapid Popup Cycles:**
   - Popup blocker detection
   - Sites that rapidly open/close popups

3. **Popup Features:**
   - Window.open with size specifications
   - Toolbar/menubar specifications
   - window.close() from JavaScript

4. **Configuration:**
   - Split popups
   - Stacked popups
   - Per-domain overrides

#### 4.2 Success Criteria

- ✅ Zero SIGSEGV crashes during popup lifecycle
- ✅ No GTK bloom filter corruption warnings
- ✅ OAuth flows complete successfully
- ✅ Popups open in configured layout (split/stacked/tabbed)
- ✅ Clean popup closure
- ✅ Parent-child relationship respected
- ✅ Window features honored

## Implementation Timeline

### Week 1: Foundation
- [ ] Implement CGO bindings for create signal (`webview_popup.go`, `webview_popup_callback.c`)
- [ ] Add Go-side create handler infrastructure
- [ ] Test basic popup creation

### Week 2: Workspace Integration
- [ ] Refactor workspace popup management
- [ ] Implement `setupPopupHandling()`, `handlePopupReadyToShow()`, `handlePopupClose()`
- [ ] Add popup type configuration
- [ ] Test split/stacked popup layouts

### Week 3: Cleanup & Testing
- [ ] Remove JavaScript interception
- [ ] Remove temporary workarounds
- [ ] Comprehensive testing of OAuth flows
- [ ] Performance testing

### Week 4: Polish & Documentation
- [ ] User documentation for popup configuration
- [ ] Code comments and architecture documentation
- [ ] Edge case testing
- [ ] Production readiness review

## Migration Path

### Backward Compatibility

During transition period, support both flows:

```go
func (wm *WorkspaceManager) setupPopupHandling(webView *webkit.WebView, parentNode *paneNode) {
    if wm.app.config.UseNativePopupLifecycle {
        // New signal-based approach
        webView.RegisterPopupCreateHandler(...)
    } else {
        // Legacy JavaScript interception (deprecated)
        // Keep for one release cycle
    }
}
```

Config flag: `"use_native_popup_lifecycle": true`

### Deprecation Notice

Add to CHANGELOG:
```
## v0.11.0 (Upcoming)

### BREAKING CHANGES
- Popup window lifecycle now uses WebKit native signals
- JavaScript window.open interception removed
- Configuration changes: `popup_behavior` replaces internal logic

### Migration Guide
- Update config.json with `popup_behavior` setting
- Remove any custom window.open interception scripts
- Test OAuth flows after upgrade
```

## References

### WebKitGTK Documentation
- Create Signal: https://webkitgtk.org/reference/webkitgtk/stable/signal.WebView.create.html
- Ready-to-Show Signal: https://webkitgtk.org/reference/webkitgtk/stable/signal.WebView.ready-to-show.html
- Close Signal: https://webkitgtk.org/reference/webkitgtk/stable/signal.WebView.close.html
- WebKitWebView Class: `/usr/include/webkitgtk-6.0/webkit/WebKitWebView.h:243-259`
- WebKitNavigationAction: https://webkitgtk.org/reference/webkitgtk/stable/class.NavigationAction.html
- WebKitWindowProperties: https://webkitgtk.org/reference/webkitgtk/stable/class.WindowProperties.html

### GTK4 Documentation
- GtkWidget Lifecycle: https://docs.gtk.org/gtk4/class.Widget.html
- Signal Handling: https://docs.gtk.org/gobject/concepts.html#signals

### Reference Implementation
- Epiphany Browser: `/home/brice/dev/clone/epiphany/src/ephy-window.c`
  - Create handler: lines 2034-2070
  - Ready-to-show handler: lines 2012-2031
  - Close handling: lines 3314-3360

### Related Issues
- Previous conversation summary: Session context showing attempted fixes
- GTK Bloom Filter: https://blogs.gnome.org/otte/2012/12/26/css-hacker-news/

## Risk Assessment

### High Risk
- CGO callback stability
- Thread safety in signal handlers
- Potential gotk4 binding gaps

### Medium Risk
- Configuration migration complexity
- User workflow disruption during transition
- Edge cases in popup features

### Low Risk
- Performance impact (should improve)
- Memory leaks (proper lifecycle should fix existing issues)

## Conclusion

This refactoring aligns dumb-browser with WebKitGTK's architecture and proven patterns from Epiphany. By respecting WebKit's signal-based popup lifecycle, we eliminate race conditions and crashes while maintaining the unique split/stacked/tabbed popup positioning features.

The key insight: **We control WHERE popups appear, WebKit controls WHEN and HOW they lifecycle.**
