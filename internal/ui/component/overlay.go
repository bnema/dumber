package component

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// overlayState tracks the current overlay status for a WebView.
type overlayState struct {
	visible bool
	mode    string
}

// OverlayController controls the injected overlay UI (omnibox/find).
// It issues JavaScript calls into the WebView to show/hide the overlay.
type OverlayController struct {
	mu     sync.RWMutex
	states map[webkit.WebViewID]overlayState
}

// NewOverlayController creates a new overlay controller.
func NewOverlayController(ctx context.Context) *OverlayController {
	log := logging.FromContext(ctx)
	log.Debug().Msg("overlay controller initialized")
	return &OverlayController{
		states: make(map[webkit.WebViewID]overlayState),
	}
}

// Show opens the overlay in the given mode ("omnibox" default or "find").
// An optional query seeds the overlay input.
func (oc *OverlayController) Show(ctx context.Context, webviewID webkit.WebViewID, mode, query string) error {
	if mode == "" {
		mode = "omnibox"
	}

	log := logging.FromContext(ctx).With().
		Str("component", "overlay-controller").
		Uint64("webview_id", uint64(webviewID)).
		Str("mode", mode).
		Logger()

	modeJSON, err := json.Marshal(mode)
	if err != nil {
		return fmt.Errorf("marshal mode: %w", err)
	}
	queryJSON, err := json.Marshal(query)
	if err != nil {
		return fmt.Errorf("marshal query: %w", err)
	}

	script := fmt.Sprintf(`(function(){try{if(window.__dumber_omnibox&&typeof window.__dumber_omnibox.open==="function"){window.__dumber_omnibox.open(%s,%s);return;}if(typeof window.__dumber_toggle==="function"){window.__dumber_toggle();}else{console.warn("dumber overlay open: APIs unavailable");}}catch(e){console.error("dumber overlay open failed",e);}})();`, string(modeJSON), string(queryJSON))

	if err := oc.runJavaScript(ctx, webviewID, script); err != nil {
		log.Warn().Err(err).Msg("failed to show overlay")
		return err
	}

	oc.setState(webviewID, overlayState{visible: true, mode: mode})
	log.Debug().Msg("overlay show requested")
	return nil
}

// Hide closes the overlay if present.
func (oc *OverlayController) Hide(ctx context.Context, webviewID webkit.WebViewID) error {
	log := logging.FromContext(ctx).With().
		Str("component", "overlay-controller").
		Uint64("webview_id", uint64(webviewID)).
		Logger()

	script := `(function(){try{if(window.__dumber_omnibox&&typeof window.__dumber_omnibox.close==="function"){window.__dumber_omnibox.close();return;}if(typeof window.__dumber_find_close==="function"){window.__dumber_find_close();return;}if(typeof window.__dumber_toggle==="function"){window.__dumber_toggle();}else{console.warn("dumber overlay close: APIs unavailable");}}catch(e){console.error("dumber overlay close failed",e);}})();`

	if err := oc.runJavaScript(ctx, webviewID, script); err != nil {
		log.Warn().Err(err).Msg("failed to hide overlay")
		return err
	}

	oc.setState(webviewID, overlayState{visible: false})
	log.Debug().Msg("overlay hide requested")
	return nil
}

// Toggle toggles the overlay visibility.
func (oc *OverlayController) Toggle(ctx context.Context, webviewID webkit.WebViewID) error {
	log := logging.FromContext(ctx).With().
		Str("component", "overlay-controller").
		Uint64("webview_id", uint64(webviewID)).
		Logger()

	script := `(function(){try{if(typeof window.__dumber_toggle==="function"){window.__dumber_toggle();return;}if(window.__dumber_omnibox&&typeof window.__dumber_omnibox.toggle==="function"){window.__dumber_omnibox.toggle();return;}if(window.__dumber_omnibox&&typeof window.__dumber_omnibox.open==="function"){window.__dumber_omnibox.open("omnibox","");return;}console.warn("dumber overlay toggle: APIs unavailable");}catch(e){console.error("dumber overlay toggle failed",e);}})();`

	if err := oc.runJavaScript(ctx, webviewID, script); err != nil {
		log.Warn().Err(err).Msg("failed to toggle overlay")
		return err
	}

	// Flip current state best-effort
	current := oc.getState(webviewID)
	oc.setState(webviewID, overlayState{visible: !current.visible, mode: current.mode})
	log.Debug().Bool("visible", !current.visible).Msg("overlay toggle requested")
	return nil
}

// OpenFind opens the find-in-page overlay with an optional query.
func (oc *OverlayController) OpenFind(ctx context.Context, webviewID webkit.WebViewID, query string) error {
	log := logging.FromContext(ctx).With().
		Str("component", "overlay-controller").
		Uint64("webview_id", uint64(webviewID)).
		Logger()

	queryJSON, err := json.Marshal(query)
	if err != nil {
		return fmt.Errorf("marshal find query: %w", err)
	}

	script := fmt.Sprintf(`(function(){try{if(typeof window.__dumber_find_open==="function"){window.__dumber_find_open(%s);return;}if(window.__dumber_omnibox&&typeof window.__dumber_omnibox.open==="function"){window.__dumber_omnibox.open("find",%s);return;}if(typeof window.__dumber_toggle==="function"){window.__dumber_toggle();return;}console.warn("dumber find open: APIs unavailable");}catch(e){console.error("dumber find open failed",e);}})();`, string(queryJSON), string(queryJSON))

	if err := oc.runJavaScript(ctx, webviewID, script); err != nil {
		log.Warn().Err(err).Msg("failed to open find overlay")
		return err
	}

	oc.setState(webviewID, overlayState{visible: true, mode: "find"})
	log.Debug().Msg("find overlay requested")
	return nil
}

// State returns the tracked overlay state for the given WebView.
func (oc *OverlayController) State(webviewID webkit.WebViewID) overlayState {
	return oc.getState(webviewID)
}

func (oc *OverlayController) setState(webviewID webkit.WebViewID, state overlayState) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.states[webviewID] = state
}

func (oc *OverlayController) getState(webviewID webkit.WebViewID) overlayState {
	oc.mu.RLock()
	defer oc.mu.RUnlock()

	if state, ok := oc.states[webviewID]; ok {
		return state
	}
	return overlayState{}
}

// runJavaScript executes script in the isolated "dumber" world for the given WebView.
// This is fire-and-forget: errors are logged asynchronously by the WebView.
func (oc *OverlayController) runJavaScript(ctx context.Context, webviewID webkit.WebViewID, script string) error {
	log := logging.FromContext(ctx)
	if webviewID == 0 {
		log.Debug().Msg("runJavaScript called with webviewID=0")
		return fmt.Errorf("webview id is required for overlay command")
	}

	wv := webkit.LookupWebView(webviewID)
	if wv == nil {
		return fmt.Errorf("webview %d not found", webviewID)
	}

	// Run in the isolated "dumber" world where the GUI scripts are injected
	wv.RunJavaScript(ctx, script, webkit.ScriptWorldName)
	return nil
}
