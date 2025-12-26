package dispatcher

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

// KeyboardDispatcher routes keyboard actions to appropriate coordinators.
type KeyboardDispatcher struct {
	tabCoord       *coordinator.TabCoordinator
	wsCoord        *coordinator.WorkspaceCoordinator
	navCoord       *coordinator.NavigationCoordinator
	zoomUC         *usecase.ManageZoomUseCase
	copyURLUC      *usecase.CopyURLUseCase
	actionHandlers map[input.Action]func(ctx context.Context) error
	onQuit         func()
	onFindOpen     func(ctx context.Context) error
	onFindNext     func(ctx context.Context) error
	onFindPrev     func(ctx context.Context) error
	onFindClose    func(ctx context.Context) error
	onSessionOpen  func(ctx context.Context) error
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

	dispatcher := &KeyboardDispatcher{
		tabCoord:  tabCoord,
		wsCoord:   wsCoord,
		navCoord:  navCoord,
		zoomUC:    zoomUC,
		copyURLUC: copyURLUC,
	}
	dispatcher.initActionHandlers()
	return dispatcher
}

// SetOnQuit sets the callback for quit action.
func (d *KeyboardDispatcher) SetOnQuit(fn func()) {
	d.onQuit = fn
}

// SetOnFindOpen sets the callback for opening the find bar.
func (d *KeyboardDispatcher) SetOnFindOpen(fn func(ctx context.Context) error) {
	d.onFindOpen = fn
}

// SetOnFindNext sets the callback for the next match action.
func (d *KeyboardDispatcher) SetOnFindNext(fn func(ctx context.Context) error) {
	d.onFindNext = fn
}

// SetOnFindPrev sets the callback for the previous match action.
func (d *KeyboardDispatcher) SetOnFindPrev(fn func(ctx context.Context) error) {
	d.onFindPrev = fn
}

// SetOnFindClose sets the callback for closing the find bar.
func (d *KeyboardDispatcher) SetOnFindClose(fn func(ctx context.Context) error) {
	d.onFindClose = fn
}

// SetOnSessionOpen sets the callback for opening the session manager.
func (d *KeyboardDispatcher) SetOnSessionOpen(fn func(ctx context.Context) error) {
	d.onSessionOpen = fn
}

func (d *KeyboardDispatcher) initActionHandlers() {
	const (
		firstTabIndex   = 0
		secondTabIndex  = 1
		thirdTabIndex   = 2
		fourthTabIndex  = 3
		fifthTabIndex   = 4
		sixthTabIndex   = 5
		seventhTabIndex = 6
		eighthTabIndex  = 7
		ninthTabIndex   = 8
		tenthTabIndex   = 9
	)
	d.actionHandlers = map[input.Action]func(ctx context.Context) error{
		// Tab actions
		input.ActionNewTab: func(ctx context.Context) error {
			cfg := config.Get()
			_, err := d.tabCoord.Create(ctx, domainurl.Normalize(cfg.Defaults.NewPaneURL))
			return err
		},
		input.ActionCloseTab:         d.tabCoord.Close,
		input.ActionNextTab:          d.tabCoord.SwitchNext,
		input.ActionPreviousTab:      d.tabCoord.SwitchPrev,
		input.ActionSwitchLastTab:    d.tabCoord.SwitchToLastActive,
		input.ActionSwitchTabIndex1:  func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, firstTabIndex) },
		input.ActionSwitchTabIndex2:  func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, secondTabIndex) },
		input.ActionSwitchTabIndex3:  func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, thirdTabIndex) },
		input.ActionSwitchTabIndex4:  func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, fourthTabIndex) },
		input.ActionSwitchTabIndex5:  func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, fifthTabIndex) },
		input.ActionSwitchTabIndex6:  func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, sixthTabIndex) },
		input.ActionSwitchTabIndex7:  func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, seventhTabIndex) },
		input.ActionSwitchTabIndex8:  func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, eighthTabIndex) },
		input.ActionSwitchTabIndex9:  func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, ninthTabIndex) },
		input.ActionSwitchTabIndex10: func(ctx context.Context) error { return d.tabCoord.SwitchByIndex(ctx, tenthTabIndex) },
		input.ActionRenameTab: func(ctx context.Context) error {
			return d.logNoop(ctx, "rename tab action (not yet implemented)")
		},
		// Pane actions
		input.ActionSplitRight: func(ctx context.Context) error { return d.wsCoord.Split(ctx, usecase.SplitRight) },
		input.ActionSplitLeft:  func(ctx context.Context) error { return d.wsCoord.Split(ctx, usecase.SplitLeft) },
		input.ActionSplitUp:    func(ctx context.Context) error { return d.wsCoord.Split(ctx, usecase.SplitUp) },
		input.ActionSplitDown:  func(ctx context.Context) error { return d.wsCoord.Split(ctx, usecase.SplitDown) },
		input.ActionClosePane:  d.wsCoord.ClosePane,
		input.ActionStackPane:  d.wsCoord.StackPane,
		input.ActionFocusRight: func(ctx context.Context) error { return d.wsCoord.FocusPane(ctx, usecase.NavRight) },
		input.ActionFocusLeft:  func(ctx context.Context) error { return d.wsCoord.FocusPane(ctx, usecase.NavLeft) },
		input.ActionFocusUp:    func(ctx context.Context) error { return d.wsCoord.FocusPane(ctx, usecase.NavUp) },
		input.ActionFocusDown:  func(ctx context.Context) error { return d.wsCoord.FocusPane(ctx, usecase.NavDown) },
		// Resize actions
		input.ActionResizeIncreaseLeft:  func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeIncreaseLeft) },
		input.ActionResizeIncreaseRight: func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeIncreaseRight) },
		input.ActionResizeIncreaseUp:    func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeIncreaseUp) },
		input.ActionResizeIncreaseDown:  func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeIncreaseDown) },
		input.ActionResizeDecreaseLeft:  func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeDecreaseLeft) },
		input.ActionResizeDecreaseRight: func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeDecreaseRight) },
		input.ActionResizeDecreaseUp:    func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeDecreaseUp) },
		input.ActionResizeDecreaseDown:  func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeDecreaseDown) },
		input.ActionResizeIncrease:      func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeIncrease) },
		input.ActionResizeDecrease:      func(ctx context.Context) error { return d.wsCoord.Resize(ctx, usecase.ResizeDecrease) },
		// Stack navigation
		input.ActionStackNavUp:   func(ctx context.Context) error { return d.wsCoord.NavigateStack(ctx, "up") },
		input.ActionStackNavDown: func(ctx context.Context) error { return d.wsCoord.NavigateStack(ctx, "down") },
		// Navigation
		input.ActionGoBack:     d.navCoord.GoBack,
		input.ActionGoForward:  d.navCoord.GoForward,
		input.ActionReload:     d.navCoord.Reload,
		input.ActionHardReload: d.navCoord.HardReload,
		// Zoom actions
		input.ActionZoomIn:    func(ctx context.Context) error { return d.handleZoom(ctx, "in") },
		input.ActionZoomOut:   func(ctx context.Context) error { return d.handleZoom(ctx, "out") },
		input.ActionZoomReset: func(ctx context.Context) error { return d.handleZoom(ctx, "reset") },
		// UI
		input.ActionOpenOmnibox:  d.navCoord.OpenOmnibox,
		input.ActionOpenFind:     d.handleFindOpen,
		input.ActionFindNext:     d.handleFindNext,
		input.ActionFindPrev:     d.handleFindPrev,
		input.ActionCloseFind:    d.handleFindClose,
		input.ActionOpenDevTools: d.navCoord.OpenDevTools,
		input.ActionToggleFullscreen: func(ctx context.Context) error {
			return d.logNoop(ctx, "toggle fullscreen action (not yet implemented)")
		},
		// Clipboard
		input.ActionCopyURL: d.handleCopyURL,
		// Session management
		input.ActionOpenSessionManager: d.handleSessionOpen,
		// Application
		input.ActionQuit: d.handleQuit,
	}
}

// Dispatch routes a keyboard action to the appropriate coordinator.
func (d *KeyboardDispatcher) Dispatch(ctx context.Context, action input.Action) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("action", string(action)).Msg("dispatching keyboard action")

	if handler, ok := d.actionHandlers[action]; ok {
		return handler(ctx)
	}

	log.Warn().Str("action", string(action)).Msg("unhandled keyboard action")
	return nil
}

func (d *KeyboardDispatcher) handleFindOpen(ctx context.Context) error {
	if d.onFindOpen != nil {
		return d.onFindOpen(ctx)
	}
	logging.FromContext(ctx).Debug().Msg("find open action (no handler)")
	return nil
}

func (d *KeyboardDispatcher) handleFindNext(ctx context.Context) error {
	if d.onFindNext != nil {
		return d.onFindNext(ctx)
	}
	logging.FromContext(ctx).Debug().Msg("find next action (no handler)")
	return nil
}

func (d *KeyboardDispatcher) handleFindPrev(ctx context.Context) error {
	if d.onFindPrev != nil {
		return d.onFindPrev(ctx)
	}
	logging.FromContext(ctx).Debug().Msg("find prev action (no handler)")
	return nil
}

func (d *KeyboardDispatcher) handleFindClose(ctx context.Context) error {
	if d.onFindClose != nil {
		return d.onFindClose(ctx)
	}
	logging.FromContext(ctx).Debug().Msg("find close action (no handler)")
	return nil
}

func (d *KeyboardDispatcher) handleSessionOpen(ctx context.Context) error {
	if d.onSessionOpen != nil {
		return d.onSessionOpen(ctx)
	}
	logging.FromContext(ctx).Debug().Msg("session open action (no handler)")
	return nil
}

func (d *KeyboardDispatcher) logNoop(ctx context.Context, message string) error {
	if ctx == nil {
		return fmt.Errorf("missing context")
	}
	logging.FromContext(ctx).Debug().Msg(message)
	return nil
}

func (d *KeyboardDispatcher) handleQuit(ctx context.Context) error {
	if d.onQuit != nil {
		d.onQuit()
	}
	if ctx == nil {
		return fmt.Errorf("missing context")
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
		cb := glib.SourceFunc(func(_ uintptr) bool {
			d.wsCoord.ShowToastOnActivePane(ctx, "URL copied", component.ToastSuccess)
			return false
		})
		glib.IdleAdd(&cb, 0)
	}()

	return nil
}
