package dispatcher

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/puregotk/v4/glib"
)

const (
	// historySystemViewURL is the full-page/systemview history surface.
	// It remains reachable by direct navigation (for example typing
	// dumb://history in the omnibox), but Ctrl+H no longer falls back to it.
	// Ctrl+H is reserved for the native GTK history sidebar only.
	historySystemViewURL   = "dumb://history"
	favoritesSystemViewURL = "dumb://favorites"
	configSystemViewURL    = "dumb://config"
)

type KeyboardActions struct {
	NewTab         func(context.Context) error
	CloseTab       func(context.Context) error
	NextTab        func(context.Context) error
	PreviousTab    func(context.Context) error
	SwitchLastTab  func(context.Context) error
	SwitchTabIndex func(context.Context, int) error
	ActiveWebView  func(context.Context) port.WebView
}

// KeyboardDispatcher routes keyboard actions to appropriate coordinators.
type KeyboardDispatcher struct {
	actions                  KeyboardActions
	wsCoord                  *coordinator.WorkspaceCoordinator
	navCoord                 *coordinator.NavigationCoordinator
	zoomUC                   *usecase.ManageZoomUseCase
	copyURLUC                *usecase.CopyURLUseCase
	actionHandlers           map[input.Action]func(ctx context.Context) error
	onQuit                   func()
	onFindOpen               func(ctx context.Context) error
	onFindNext               func(ctx context.Context) error
	onFindPrev               func(ctx context.Context) error
	onFindClose              func(ctx context.Context) error
	activePaneID             func(ctx context.Context) entity.PaneID
	onSessionOpen            func(ctx context.Context, paneID entity.PaneID) error
	onMovePaneToTab          func(ctx context.Context, paneID entity.PaneID) error
	onMovePaneToNext         func(ctx context.Context, paneID entity.PaneID) error
	onEjectPaneToWindow      func(ctx context.Context, paneID entity.PaneID) error
	onToggleHistorySidebar   func(ctx context.Context) error
	onToggleFavoritesSidebar func(ctx context.Context) error
	onToggleCurrentFavorite  func(ctx context.Context) error
	onToggleFloating         func(ctx context.Context) error
	onOpenFloating           func(ctx context.Context, target input.FloatingProfileTarget) error
}

// NewKeyboardDispatcher creates a new KeyboardDispatcher.
func NewKeyboardDispatcher(
	ctx context.Context,
	wsCoord *coordinator.WorkspaceCoordinator,
	navCoord *coordinator.NavigationCoordinator,
	zoomUC *usecase.ManageZoomUseCase,
	copyURLUC *usecase.CopyURLUseCase,
	actions KeyboardActions,
	activePaneID func(context.Context) entity.PaneID,
) *KeyboardDispatcher {
	log := logging.FromContext(ctx)
	log.Debug().Msg("creating keyboard dispatcher")

	dispatcher := &KeyboardDispatcher{
		actions:      actions,
		wsCoord:      wsCoord,
		navCoord:     navCoord,
		zoomUC:       zoomUC,
		copyURLUC:    copyURLUC,
		activePaneID: activePaneID,
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
func (d *KeyboardDispatcher) SetOnSessionOpen(fn func(ctx context.Context, paneID entity.PaneID) error) {
	d.onSessionOpen = fn
}

func (d *KeyboardDispatcher) SetOnMovePaneToTab(fn func(ctx context.Context, paneID entity.PaneID) error) {
	d.onMovePaneToTab = fn
}

func (d *KeyboardDispatcher) SetOnMovePaneToNextTab(fn func(ctx context.Context, paneID entity.PaneID) error) {
	d.onMovePaneToNext = fn
}

func (d *KeyboardDispatcher) SetOnEjectPaneToWindow(fn func(ctx context.Context, paneID entity.PaneID) error) {
	d.onEjectPaneToWindow = fn
}

func (d *KeyboardDispatcher) SetOnToggleHistorySidebar(fn func(ctx context.Context) error) {
	d.onToggleHistorySidebar = fn
}

func (d *KeyboardDispatcher) SetOnToggleFavoritesSidebar(fn func(ctx context.Context) error) {
	d.onToggleFavoritesSidebar = fn
}

func (d *KeyboardDispatcher) SetOnToggleCurrentPageFavorite(fn func(ctx context.Context) error) {
	d.onToggleCurrentFavorite = fn
}

func (d *KeyboardDispatcher) SetOnToggleFloatingPane(fn func(ctx context.Context) error) {
	d.onToggleFloating = fn
}

func (d *KeyboardDispatcher) SetOnOpenFloatingURL(fn func(ctx context.Context, url string) error) {
	if fn == nil {
		d.onOpenFloating = nil
		return
	}
	d.onOpenFloating = func(ctx context.Context, target input.FloatingProfileTarget) error {
		return fn(ctx, target.URL)
	}
}

func (d *KeyboardDispatcher) SetOnOpenFloatingTarget(fn func(ctx context.Context, target input.FloatingProfileTarget) error) {
	d.onOpenFloating = fn
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
		input.ActionNewTab:   func(ctx context.Context) error { return d.handleKeyboardAction(ctx, "new tab", d.actions.NewTab) },
		input.ActionCloseTab: func(ctx context.Context) error { return d.handleKeyboardAction(ctx, "close tab", d.actions.CloseTab) },
		input.ActionNextTab:  func(ctx context.Context) error { return d.handleKeyboardAction(ctx, "next tab", d.actions.NextTab) },
		input.ActionPreviousTab: func(ctx context.Context) error {
			return d.handleKeyboardAction(ctx, "previous tab", d.actions.PreviousTab)
		},
		input.ActionSwitchLastTab: func(ctx context.Context) error {
			return d.handleKeyboardAction(ctx, "switch last tab", d.actions.SwitchLastTab)
		},
		input.ActionSwitchTabIndex1:  func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, firstTabIndex) },
		input.ActionSwitchTabIndex2:  func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, secondTabIndex) },
		input.ActionSwitchTabIndex3:  func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, thirdTabIndex) },
		input.ActionSwitchTabIndex4:  func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, fourthTabIndex) },
		input.ActionSwitchTabIndex5:  func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, fifthTabIndex) },
		input.ActionSwitchTabIndex6:  func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, sixthTabIndex) },
		input.ActionSwitchTabIndex7:  func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, seventhTabIndex) },
		input.ActionSwitchTabIndex8:  func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, eighthTabIndex) },
		input.ActionSwitchTabIndex9:  func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, ninthTabIndex) },
		input.ActionSwitchTabIndex10: func(ctx context.Context) error { return d.handleSwitchTabIndex(ctx, tenthTabIndex) },
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
		input.ActionMovePaneToTab: func(ctx context.Context) error {
			return d.handleMovePaneToTab(ctx)
		},
		input.ActionMovePaneToNextTab: func(ctx context.Context) error {
			return d.handleMovePaneToNextTab(ctx)
		},
		input.ActionEjectPaneToWindow: func(ctx context.Context) error {
			return d.handleEjectPaneToWindow(ctx)
		},
		input.ActionConsumeOrExpelLeft: func(ctx context.Context) error {
			return d.wsCoord.ConsumeOrExpelPane(ctx, usecase.ConsumeOrExpelLeft)
		},
		input.ActionConsumeOrExpelRight: func(ctx context.Context) error {
			return d.wsCoord.ConsumeOrExpelPane(ctx, usecase.ConsumeOrExpelRight)
		},
		input.ActionConsumeOrExpelUp: func(ctx context.Context) error {
			return d.wsCoord.ConsumeOrExpelPane(ctx, usecase.ConsumeOrExpelUp)
		},
		input.ActionConsumeOrExpelDown: func(ctx context.Context) error {
			return d.wsCoord.ConsumeOrExpelPane(ctx, usecase.ConsumeOrExpelDown)
		},
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
		input.ActionGoBack:     d.handleGoBack,
		input.ActionGoForward:  d.handleGoForward,
		input.ActionReload:     d.handleReload,
		input.ActionHardReload: d.handleHardReload,
		input.ActionPrintPage:  d.handlePrintPage,
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
		input.ActionOpenDevTools: d.handleOpenDevTools,
		input.ActionToggleFloatingPane: func(ctx context.Context) error {
			if d.onToggleFloating != nil {
				return d.onToggleFloating(ctx)
			}
			return d.logNoop(ctx, "toggle floating pane action (no handler)")
		},
		// ActionToggleHistorySystemView (default Ctrl+H) is intentionally bound
		// to the native GTK history sidebar only. The dumb://history systemview
		// remains available through direct navigation, not shortcut fallback.
		input.ActionToggleHistorySystemView: func(ctx context.Context) error {
			if d.onToggleHistorySidebar == nil {
				return fmt.Errorf("history sidebar unavailable: toggle handler not wired")
			}
			return d.onToggleHistorySidebar(ctx)
		},
		input.ActionToggleFavoritesSystemView: func(ctx context.Context) error {
			if d.onToggleFavoritesSidebar == nil {
				return fmt.Errorf("favorites sidebar unavailable: toggle handler not wired")
			}
			return d.onToggleFavoritesSidebar(ctx)
		},
		input.ActionToggleCurrentPageFavorite: func(ctx context.Context) error {
			if d.onToggleCurrentFavorite == nil {
				return fmt.Errorf("favorites unavailable: toggle current page handler not wired")
			}
			return d.onToggleCurrentFavorite(ctx)
		},
		input.ActionToggleConfigSystemView: func(ctx context.Context) error {
			return d.wsCoord.ToggleSystemViewRight(ctx, configSystemViewURL)
		},
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

	if target, ok := input.ParseFloatingProfileTarget(action); ok {
		if d.onOpenFloating != nil {
			return d.onOpenFloating(ctx, target)
		}
		log.Debug().Str("url", target.URL).Msg("floating profile action ignored (no handler)")
		return nil
	}

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
		return d.onSessionOpen(ctx, d.activePaneID(ctx))
	}
	logging.FromContext(ctx).Debug().Msg("session open action (no handler)")
	return nil
}

func (d *KeyboardDispatcher) handleMovePaneToTab(ctx context.Context) error {
	if d.onMovePaneToTab != nil {
		return d.onMovePaneToTab(ctx, d.activePaneID(ctx))
	}
	logging.FromContext(ctx).Debug().Msg("move pane to tab action (no handler)")
	return nil
}

func (d *KeyboardDispatcher) handleMovePaneToNextTab(ctx context.Context) error {
	if d.onMovePaneToNext != nil {
		return d.onMovePaneToNext(ctx, d.activePaneID(ctx))
	}
	logging.FromContext(ctx).Debug().Msg("move pane to next tab action (no handler)")
	return nil
}

func (d *KeyboardDispatcher) handleEjectPaneToWindow(ctx context.Context) error {
	if d.onEjectPaneToWindow != nil {
		return d.onEjectPaneToWindow(ctx, d.activePaneID(ctx))
	}
	logging.FromContext(ctx).Debug().Msg("eject pane to window action (no handler)")
	return nil
}

func (d *KeyboardDispatcher) handleSwitchTabIndex(ctx context.Context, index int) error {
	if d.actions.SwitchTabIndex == nil {
		logging.FromContext(ctx).Debug().Int("index", index).Msg("switch tab index action ignored (no handler)")
		return nil
	}
	return d.actions.SwitchTabIndex(ctx, index)
}

func (d *KeyboardDispatcher) handleKeyboardAction(ctx context.Context, name string, fn func(context.Context) error) error {
	if fn == nil {
		logging.FromContext(ctx).Debug().Str("action", name).Msg("keyboard action ignored (no handler)")
		return nil
	}
	return fn(ctx)
}

func (d *KeyboardDispatcher) activeWebView(ctx context.Context) port.WebView {
	if d.actions.ActiveWebView == nil {
		logging.FromContext(ctx).Debug().Msg("active webview unavailable (no handler)")
		return nil
	}
	return d.actions.ActiveWebView(ctx)
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

func (d *KeyboardDispatcher) withActiveWebView(ctx context.Context, action string, fn func(port.WebView) error) error {
	wv := d.activeWebView(ctx)
	if wv == nil {
		logging.FromContext(ctx).Debug().Str("action", action).Msg("no active webview for keyboard action")
		return nil
	}
	return fn(wv)
}

func (d *KeyboardDispatcher) handleReload(ctx context.Context) error {
	return d.withActiveWebView(ctx, "reload", func(wv port.WebView) error {
		return d.navCoord.ReloadWebView(ctx, wv, false)
	})
}

func (d *KeyboardDispatcher) handleHardReload(ctx context.Context) error {
	return d.withActiveWebView(ctx, "hard reload", func(wv port.WebView) error {
		return d.navCoord.ReloadWebView(ctx, wv, true)
	})
}

func (d *KeyboardDispatcher) handleGoBack(ctx context.Context) error {
	return d.withActiveWebView(ctx, "go back", func(wv port.WebView) error {
		return d.navCoord.GoBackWebView(ctx, wv)
	})
}

func (d *KeyboardDispatcher) handleGoForward(ctx context.Context) error {
	return d.withActiveWebView(ctx, "go forward", func(wv port.WebView) error {
		return d.navCoord.GoForwardWebView(ctx, wv)
	})
}

func (d *KeyboardDispatcher) handlePrintPage(ctx context.Context) error {
	return d.withActiveWebView(ctx, "print page", func(wv port.WebView) error {
		return d.navCoord.PrintWebView(ctx, wv)
	})
}

func (d *KeyboardDispatcher) handleOpenDevTools(ctx context.Context) error {
	return d.withActiveWebView(ctx, "open devtools", func(wv port.WebView) error {
		return d.navCoord.OpenDevToolsWebView(ctx, wv)
	})
}

// handleZoom processes zoom in/out/reset actions for the active WebView.
func (d *KeyboardDispatcher) handleZoom(ctx context.Context, action string) error {
	log := logging.FromContext(ctx)

	if d.zoomUC == nil {
		log.Warn().Msg("zoom use case not available")
		return nil
	}

	wv := d.activeWebView(ctx)
	if wv == nil {
		log.Debug().Msg("no active webview for zoom")
		return nil
	}

	zoomKey, err := usecase.ExtractZoomKey(wv.URI())
	if err != nil {
		log.Debug().Str("uri", wv.URI()).Msg("cannot extract zoom key")
		return nil
	}

	current := wv.GetZoomLevel()
	var newZoom *entity.ZoomLevel

	switch action {
	case "in":
		newZoom, err = d.zoomUC.ZoomIn(ctx, zoomKey, current)
	case "out":
		newZoom, err = d.zoomUC.ZoomOut(ctx, zoomKey, current)
	case "reset":
		err = d.zoomUC.ResetZoom(ctx, zoomKey)
		if err == nil {
			newZoom = entity.NewZoomLevel(zoomKey, d.zoomUC.DefaultZoom())
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
			Str("zoom_key", zoomKey).
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

	wv := d.activeWebView(ctx)
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
