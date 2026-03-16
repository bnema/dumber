package content

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// EnsureWebView acquires or reuses a WebView for the given pane.
func (c *Coordinator) EnsureWebView(ctx context.Context, paneID entity.PaneID) (port.WebView, error) {
	log := logging.FromContext(ctx)

	if wv := c.getWebViewLocked(paneID); wv != nil && !wv.IsDestroyed() {
		return wv, nil
	}

	if c.pool == nil {
		return nil, fmt.Errorf("webview pool not configured")
	}

	// Mark tab_created on first webview (first tab)
	if c.webViewCount() == 0 {
		logging.Trace().Mark("tab_created")
	}

	wv, err := c.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	logging.Trace().Mark("webview_acquired")

	c.setWebViewLocked(paneID, wv)
	c.setupWebViewCallbacks(ctx, paneID, wv)

	log.Debug().Str("pane_id", string(paneID)).Msg("webview acquired for pane")
	return wv, nil
}

// ReleaseWebView returns the WebView for a pane to the pool.
func (c *Coordinator) ReleaseWebView(ctx context.Context, paneID entity.PaneID) {
	log := logging.FromContext(ctx)

	wv := c.deleteWebViewLocked(paneID)
	if wv == nil {
		return
	}
	c.clearPendingAppearance(paneID)

	// CRITICAL: If this webview was inhibiting idle (fullscreen or audio playing),
	// we must release the inhibition before destroying the webview.
	// Otherwise the D-Bus inhibit request stays active forever.
	if c.idleInhibitor != nil {
		if wv.IsFullscreen() {
			log.Debug().Str("pane_id", string(paneID)).Msg("releasing idle inhibition (was fullscreen)")
			if err := c.idleInhibitor.Uninhibit(ctx); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to uninhibit idle on release (fullscreen)")
			}
		}
		if wv.IsPlayingAudio() {
			log.Debug().Str("pane_id", string(paneID)).Msg("releasing idle inhibition (was playing audio)")
			if err := c.idleInhibitor.Uninhibit(ctx); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to uninhibit idle on release (audio)")
			}
		}
	}

	// Clean up title tracking
	c.titleMu.Lock()
	delete(c.paneTitles, paneID)
	c.titleMu.Unlock()

	// Clean up navigation origin tracking
	c.navOriginMu.Lock()
	delete(c.navOrigins, paneID)
	c.navOriginMu.Unlock()

	if c.pool != nil {
		c.pool.Release(wv)
	} else {
		wv.Destroy()
	}

	log.Debug().Str("pane_id", string(paneID)).Msg("webview released")
}

// AttachToWorkspace ensures each pane in the workspace has a WebView widget attached.
func (c *Coordinator) AttachToWorkspace(ctx context.Context, ws *entity.Workspace, wsView *component.WorkspaceView) {
	log := logging.FromContext(ctx)

	if ws == nil || wsView == nil || c.widgetFactory == nil {
		return
	}

	for _, pane := range ws.AllPanes() {
		if pane == nil {
			continue
		}

		wv, err := c.EnsureWebView(ctx, pane.ID)
		if err != nil {
			log.Warn().Err(err).Str("pane_id", string(pane.ID)).Msg("failed to ensure webview for pane")
			continue
		}

		// Load the pane's URI if set and different from current
		if pane.URI != "" && pane.URI != wv.URI() {
			if err := wv.LoadURI(ctx, pane.URI); err != nil {
				log.Warn().Err(err).Str("pane_id", string(pane.ID)).Str("uri", pane.URI).Msg("failed to load pane URI")
			}
		}

		widget := c.WrapWidget(ctx, wv)
		if widget == nil {
			continue
		}

		if err := wsView.SetWebViewWidget(pane.ID, widget); err != nil {
			log.Warn().Err(err).Str("pane_id", string(pane.ID)).Msg("failed to attach webview widget")
		}
		logging.Trace().Mark("webview_attached")
	}
}

// WrapWidget converts a WebView to a layout.Widget for embedding.
// It also attaches gesture handlers for mouse button navigation.
func (c *Coordinator) WrapWidget(ctx context.Context, wv port.WebView) layout.Widget {
	log := logging.FromContext(ctx)

	if wv == nil || c.widgetFactory == nil {
		log.Debug().Msg("cannot wrap nil webview or factory")
		return nil
	}

	// Use NativeWidgetProvider interface for GTK embedding (engine-agnostic)
	nwp, ok := wv.(port.NativeWidgetProvider)
	if !ok {
		log.Debug().Msg("webview does not support widget embedding")
		return nil
	}

	ptr := nwp.NativeWidget()
	if ptr == 0 {
		return nil
	}

	gtkWidget := &gtk.Widget{}
	gtkWidget.Ptr = ptr
	widget := c.widgetFactory.WrapWidget(gtkWidget)

	// Attach gesture handler for mouse button 8/9 navigation
	if widget != nil {
		gestureHandler := input.NewGestureHandler(ctx)
		// Pass WebView directly to preserve user gesture context (like Epiphany)
		if nav, ok := wv.(input.DirectNavigator); ok {
			gestureHandler.SetNavigator(nav)
		}
		// Keep callback as fallback
		if c.gestureActionHandler != nil {
			gestureHandler.SetOnAction(c.gestureActionHandler)
		}
		gestureHandler.AttachTo(widget.GtkWidget())
		log.Debug().Msg("gesture handler attached to webview with direct navigator")
	}

	return widget
}

// ActiveWebView returns the WebView for the active pane.
func (c *Coordinator) ActiveWebView(ctx context.Context) port.WebView {
	log := logging.FromContext(ctx)

	if paneID, ok := c.activePaneOverrideID(); ok {
		wv := c.getWebViewLocked(paneID)
		if wv != nil {
			return wv
		}
		log.Debug().Str("pane_id", string(paneID)).Msg("active pane override has no webview, falling back")
	}
	if c.getActiveWS == nil {
		log.Debug().Msg("no active workspace resolver")
		return nil
	}

	ws, _ := c.getActiveWS()
	if ws == nil {
		log.Debug().Msg("no active workspace")
		return nil
	}

	pane := ws.ActivePane()
	if pane == nil || pane.Pane == nil {
		log.Debug().Msg("no active pane")
		return nil
	}

	return c.getWebViewLocked(pane.Pane.ID)
}

// GetWebView returns the WebView for a specific pane.
func (c *Coordinator) GetWebView(paneID entity.PaneID) port.WebView {
	return c.getWebViewLocked(paneID)
}

// RegisterPopupWebView registers a popup WebView that was created externally.
// This is used when popup tabs are created and the WebView needs to be tracked.
func (c *Coordinator) RegisterPopupWebView(paneID entity.PaneID, wv port.WebView) {
	if wv != nil && paneID != "" {
		c.setWebViewLocked(paneID, wv)
	}
}
