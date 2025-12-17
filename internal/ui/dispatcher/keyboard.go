package dispatcher

import (
	"context"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

// KeyboardDispatcher routes keyboard actions to appropriate coordinators.
type KeyboardDispatcher struct {
	tabCoord  *coordinator.TabCoordinator
	wsCoord   *coordinator.WorkspaceCoordinator
	navCoord  *coordinator.NavigationCoordinator
	zoomUC    *usecase.ManageZoomUseCase
	copyURLUC *usecase.CopyURLUseCase
	onQuit    func()
}

// NewKeyboardDispatcher creates a new KeyboardDispatcher.
func NewKeyboardDispatcher(
	ctx context.Context,
	tabCoord *coordinator.TabCoordinator,
	wsCoord *coordinator.WorkspaceCoordinator,
	navCoord *coordinator.NavigationCoordinator,
	zoomUC *usecase.ManageZoomUseCase,
	copyURLUC *usecase.CopyURLUseCase,
) *KeyboardDispatcher {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating keyboard dispatcher")

	return &KeyboardDispatcher{
		tabCoord:  tabCoord,
		wsCoord:   wsCoord,
		navCoord:  navCoord,
		zoomUC:    zoomUC,
		copyURLUC: copyURLUC,
	}
}

// SetOnQuit sets the callback for quit action.
func (d *KeyboardDispatcher) SetOnQuit(fn func()) {
	d.onQuit = fn
}

// Dispatch routes a keyboard action to the appropriate coordinator.
func (d *KeyboardDispatcher) Dispatch(ctx context.Context, action input.Action) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("action", string(action)).Msg("dispatching keyboard action")

	switch action {
	// Tab actions
	case input.ActionNewTab:
		_, err := d.tabCoord.Create(ctx, "about:blank")
		return err
	case input.ActionCloseTab:
		return d.tabCoord.Close(ctx)
	case input.ActionNextTab:
		return d.tabCoord.SwitchNext(ctx)
	case input.ActionPreviousTab:
		return d.tabCoord.SwitchPrev(ctx)
	case input.ActionSwitchLastTab:
		return d.tabCoord.SwitchToLastActive(ctx)
	case input.ActionSwitchTabIndex1:
		return d.tabCoord.SwitchByIndex(ctx, 0)
	case input.ActionSwitchTabIndex2:
		return d.tabCoord.SwitchByIndex(ctx, 1)
	case input.ActionSwitchTabIndex3:
		return d.tabCoord.SwitchByIndex(ctx, 2)
	case input.ActionSwitchTabIndex4:
		return d.tabCoord.SwitchByIndex(ctx, 3)
	case input.ActionSwitchTabIndex5:
		return d.tabCoord.SwitchByIndex(ctx, 4)
	case input.ActionSwitchTabIndex6:
		return d.tabCoord.SwitchByIndex(ctx, 5)
	case input.ActionSwitchTabIndex7:
		return d.tabCoord.SwitchByIndex(ctx, 6)
	case input.ActionSwitchTabIndex8:
		return d.tabCoord.SwitchByIndex(ctx, 7)
	case input.ActionSwitchTabIndex9:
		return d.tabCoord.SwitchByIndex(ctx, 8)
	case input.ActionSwitchTabIndex10:
		return d.tabCoord.SwitchByIndex(ctx, 9)

	// Pane actions
	case input.ActionSplitRight:
		return d.wsCoord.Split(ctx, usecase.SplitRight)
	case input.ActionSplitLeft:
		return d.wsCoord.Split(ctx, usecase.SplitLeft)
	case input.ActionSplitUp:
		return d.wsCoord.Split(ctx, usecase.SplitUp)
	case input.ActionSplitDown:
		return d.wsCoord.Split(ctx, usecase.SplitDown)
	case input.ActionClosePane:
		return d.wsCoord.ClosePane(ctx)
	case input.ActionStackPane:
		return d.wsCoord.StackPane(ctx)
	case input.ActionFocusRight:
		return d.wsCoord.FocusPane(ctx, usecase.NavRight)
	case input.ActionFocusLeft:
		return d.wsCoord.FocusPane(ctx, usecase.NavLeft)
	case input.ActionFocusUp:
		return d.wsCoord.FocusPane(ctx, usecase.NavUp)
	case input.ActionFocusDown:
		return d.wsCoord.FocusPane(ctx, usecase.NavDown)

	// Stack navigation
	case input.ActionStackNavUp:
		return d.wsCoord.NavigateStack(ctx, "up")
	case input.ActionStackNavDown:
		return d.wsCoord.NavigateStack(ctx, "down")

	// Navigation
	case input.ActionGoBack:
		return d.navCoord.GoBack(ctx)
	case input.ActionGoForward:
		return d.navCoord.GoForward(ctx)
	case input.ActionReload:
		return d.navCoord.Reload(ctx)
	case input.ActionHardReload:
		return d.navCoord.HardReload(ctx)

	// Zoom actions
	case input.ActionZoomIn:
		return d.handleZoom(ctx, "in")
	case input.ActionZoomOut:
		return d.handleZoom(ctx, "out")
	case input.ActionZoomReset:
		return d.handleZoom(ctx, "reset")

	// UI
	case input.ActionOpenOmnibox:
		return d.navCoord.OpenOmnibox(ctx)
	case input.ActionOpenDevTools:
		return d.navCoord.OpenDevTools(ctx)
	case input.ActionToggleFullscreen:
		log.Debug().Msg("toggle fullscreen action (not yet implemented)")

	// Clipboard
	case input.ActionCopyURL:
		return d.handleCopyURL(ctx)

	// Application
	case input.ActionQuit:
		if d.onQuit != nil {
			d.onQuit()
		}

	default:
		log.Warn().Str("action", string(action)).Msg("unhandled keyboard action")
	}

	return nil
}

// handleZoom processes zoom in/out/reset actions for the active WebView.
func (d *KeyboardDispatcher) handleZoom(ctx context.Context, action string) error {
	log := logging.FromContext(ctx)

	if d.zoomUC == nil {
		log.Warn().Msg("zoom use case not available")
		return nil
	}

	wv := d.navCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Debug().Msg("no active webview for zoom")
		return nil
	}

	domain, err := usecase.ExtractDomain(wv.URI())
	if err != nil {
		log.Debug().Str("uri", wv.URI()).Msg("cannot extract domain for zoom")
		return nil
	}

	current := wv.GetZoomLevel()
	var newZoom *entity.ZoomLevel

	switch action {
	case "in":
		newZoom, err = d.zoomUC.ZoomIn(ctx, domain, current)
	case "out":
		newZoom, err = d.zoomUC.ZoomOut(ctx, domain, current)
	case "reset":
		err = d.zoomUC.ResetZoom(ctx, domain)
		if err == nil {
			newZoom = entity.NewZoomLevel(domain, d.zoomUC.DefaultZoom())
		}
	}

	if err != nil {
		log.Error().Err(err).Str("action", action).Msg("zoom action failed")
		return err
	}

	if newZoom != nil {
		if setErr := wv.SetZoomLevel(ctx, newZoom.ZoomFactor); setErr != nil {
			log.Error().Err(setErr).Float64("zoom", newZoom.ZoomFactor).Msg("failed to apply zoom")
			return setErr
		}

		// Notify omnibox to update zoom indicator
		d.navCoord.NotifyZoomChanged(ctx, newZoom.ZoomFactor)

		// Show zoom toast on the active pane
		zoomPercent := int(newZoom.ZoomFactor * 100)
		d.wsCoord.ShowZoomToast(ctx, zoomPercent)

		log.Debug().
			Str("domain", domain).
			Str("action", action).
			Float64("zoom", newZoom.ZoomFactor).
			Msg("zoom applied")
	}

	return nil
}

// handleCopyURL copies the active pane's URL to clipboard.
func (d *KeyboardDispatcher) handleCopyURL(ctx context.Context) error {
	log := logging.FromContext(ctx)

	if d.copyURLUC == nil {
		log.Warn().Msg("copy URL use case not available")
		return nil
	}

	wv := d.navCoord.ActiveWebView(ctx)
	if wv == nil {
		log.Debug().Msg("no active webview for copy URL")
		return nil
	}

	uri := wv.URI()
	if uri == "" {
		log.Debug().Msg("active webview has empty URI")
		return nil
	}

	// Copy URL in background goroutine
	go func() {
		if err := d.copyURLUC.Copy(ctx, uri); err != nil {
			log.Error().Err(err).Str("uri", uri).Msg("copy URL failed")
			return
		}

		// Show toast on GTK main thread
		var cb glib.SourceFunc
		cb = func(_ uintptr) bool {
			d.wsCoord.ShowToastOnActivePane(ctx, "URL copied", component.ToastSuccess)
			return false
		}
		glib.IdleAdd(&cb, 0)
	}()

	return nil
}
