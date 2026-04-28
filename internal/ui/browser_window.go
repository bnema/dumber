package ui

import (
	"context"
	"sort"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/focus"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/glib"
)

type browserWindow struct {
	id                    string
	initialURL            string
	activeTabID           entity.TabID
	prevActiveTabID       entity.TabID // per-window previous active tab (for Alt+Tab behavior)
	mainWindow            *window.MainWindow
	appToaster            *component.Toaster
	modeToaster           *component.Toaster
	borderMgr             *focus.BorderManager
	sessionManager        *component.SessionManager
	tabPicker             *component.TabPicker
	tabPickerWidget       layout.Widget
	tabPickerPaneID       entity.PaneID
	insertAccentUC        *usecase.InsertAccentUseCase
	accentPicker          *component.AccentPicker
	keyboardHandler       *input.KeyboardHandler
	globalShortcutHandler *input.GlobalShortcutHandler
	permissionDialog      port.PermissionDialogPresenter
	webrtcIndicator       *component.WebRTCPermissionIndicator
}

func (bw *browserWindow) clearShellState() {
	if bw == nil {
		return
	}
	bw.appToaster = nil
	bw.modeToaster = nil
	bw.borderMgr = nil
	bw.sessionManager = nil
	bw.tabPicker = nil
	bw.tabPickerWidget = nil
	bw.tabPickerPaneID = ""
	bw.insertAccentUC = nil
	bw.accentPicker = nil
	bw.keyboardHandler = nil
	bw.globalShortcutHandler = nil
	bw.permissionDialog = nil
	bw.webrtcIndicator = nil
}

func (bw *browserWindow) initChrome(ctx context.Context, a *App) {
	if bw == nil || a == nil || a.widgetFactory == nil || bw.mainWindow == nil {
		return
	}

	bw.initToasterOverlay(a)
	bw.initBorderOverlay(a)
	bw.initAccentPicker(ctx, a)
	bw.initSessionManager(ctx, a)
	bw.initTabPicker(ctx, a)
}

func (bw *browserWindow) initToasterOverlay(a *App) {
	if bw == nil || a == nil || a.widgetFactory == nil || bw.mainWindow == nil {
		return
	}

	bw.appToaster = component.NewToaster(a.widgetFactory)
	if bw.appToaster != nil {
		if widget := bw.appToaster.Widget(); widget != nil {
			if gtkWidget := widget.GtkWidget(); gtkWidget != nil {
				bw.mainWindow.AddOverlay(gtkWidget)
			}
		}
	}

	bw.modeToaster = component.NewToaster(a.widgetFactory)
	if bw.modeToaster != nil {
		if widget := bw.modeToaster.Widget(); widget != nil {
			if gtkWidget := widget.GtkWidget(); gtkWidget != nil {
				bw.mainWindow.AddOverlay(gtkWidget)
			}
		}
	}
}

func (bw *browserWindow) initBorderOverlay(a *App) {
	if bw == nil || a == nil || a.widgetFactory == nil || bw.mainWindow == nil {
		return
	}

	bw.borderMgr = focus.NewBorderManager(a.widgetFactory)
	if bw.borderMgr == nil {
		return
	}
	if widget := bw.borderMgr.Widget(); widget != nil {
		if gtkWidget := widget.GtkWidget(); gtkWidget != nil {
			bw.mainWindow.AddOverlay(gtkWidget)
		}
	}
}

func (bw *browserWindow) initAccentPicker(ctx context.Context, a *App) {
	log := logging.FromContext(ctx)
	if bw == nil || a == nil || a.widgetFactory == nil || bw.mainWindow == nil || a.deps == nil {
		log.Debug().Msg("widget factory, deps, or main window not available, skipping accent picker")
		return
	}

	bw.accentPicker = component.NewAccentPicker(a.widgetFactory)
	if bw.accentPicker == nil {
		log.Warn().Msg("failed to create accent picker")
		return
	}
	if widget := bw.accentPicker.Widget(); widget != nil {
		if gtkWidget := widget.GtkWidget(); gtkWidget != nil {
			bw.mainWindow.AddOverlay(gtkWidget)
		}
	}

	a.accentFocusProvider = a.deps.AccentFocusProvider
	bw.insertAccentUC = usecase.NewInsertAccentUseCase(
		a.accentFocusProvider,
		bw.accentPicker,
		func(fn func()) {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				fn()
				return false
			})
			glib.IdleAdd(&cb, 0)
		},
	)

	log.Debug().Msg("accent picker initialized")
}

func (bw *browserWindow) initSessionManager(ctx context.Context, a *App) {
	log := logging.FromContext(ctx)
	if bw == nil || a == nil || a.deps == nil || a.deps.Config == nil || bw.mainWindow == nil {
		log.Debug().Msg("deps or main window not available, skipping session manager")
		return
	}

	var listSessionsUC *usecase.ListSessionsUseCase
	var deleteSessionUC *usecase.DeleteSessionUseCase
	if a.deps.SessionRepo != nil && a.deps.SessionStateRepo != nil {
		listSessionsUC = usecase.NewListSessionsUseCase(
			a.deps.SessionRepo,
			a.deps.SessionStateRepo,
		)
		deleteSessionUC = usecase.NewDeleteSessionUseCase(
			a.deps.SessionStateRepo,
			a.deps.SessionRepo,
		)
	}

	bw.sessionManager = component.NewSessionManager(ctx, component.SessionManagerConfig{
		ListSessionsUC:  listSessionsUC,
		DeleteSessionUC: deleteSessionUC,
		CurrentSession:  a.deps.CurrentSessionID,
		UIScale:         a.deps.Config.DefaultUIScale,
		OnClose: func() {
			log.Debug().Msg("session manager closed")
		},
		OnOpen: func(sessionID entity.SessionID) {
			log.Info().Str("session_id", string(sessionID)).Msg("session restoration requested")
			if spawner := a.deps.SessionSpawner; spawner == nil {
				log.Warn().Str("session_id", string(sessionID)).Msg("session spawner not available")
				return
			} else if err := spawner.SpawnWithSession(sessionID); err != nil {
				log.Error().Err(err).Str("session_id", string(sessionID)).Msg("failed to spawn session")
			}
		},
		OnToast: func(ctx context.Context, message string, level component.ToastLevel) {
			if bw.appToaster != nil {
				bw.appToaster.Show(ctx, message, level)
			}
		},
	})

	if bw.sessionManager == nil {
		log.Warn().Msg("failed to create session manager")
		return
	}
	if widget := bw.sessionManager.Widget(); widget != nil {
		bw.mainWindow.AddOverlay(widget)
	}
	log.Debug().Msg("session manager initialized")
}

func (bw *browserWindow) initTabPicker(ctx context.Context, a *App) {
	log := logging.FromContext(ctx)
	if bw == nil || a == nil || a.deps == nil || a.deps.Config == nil {
		log.Debug().Msg("deps/config not available, skipping tab picker")
		return
	}

	bw.tabPicker = component.NewTabPicker(ctx, component.TabPickerConfig{
		UIScale: a.deps.Config.DefaultUIScale,
		OnClose: func() {
			log.Debug().Msg("tab picker closed")
		},
		OnSelect: func(item component.TabPickerItem) {
			cb := glib.SourceFunc(func(_ uintptr) bool {
				var targetID entity.TabID
				if !item.IsNew {
					targetID = item.TabID
				}
				if err := a.moveActivePaneToTabFromBrowserWindow(ctx, bw, targetID); err != nil {
					log.Warn().Err(err).Msg("move pane to tab failed")
				}
				return false
			})
			glib.IdleAdd(&cb, 0)
		},
	})

	if bw.tabPicker == nil {
		log.Warn().Msg("failed to create tab picker")
		return
	}

	log.Debug().Msg("tab picker initialized")
}

func (a *App) registerBrowserWindow(bw *browserWindow) {
	if bw == nil {
		return
	}
	if a.browserWindows == nil {
		a.browserWindows = make(map[string]*browserWindow)
	}
	a.browserWindows[bw.id] = bw
}

func (a *App) removeBrowserWindow(id string) {
	if id == "" || a.browserWindows == nil {
		return
	}
	removed := a.browserWindows[id]
	wasMainWindow := removed != nil && removed.mainWindow == a.mainWindow
	ownedTabIDs := make([]entity.TabID, 0)
	for tabID, bw := range a.windowForTab {
		if bw != nil && bw.id == id {
			ownedTabIDs = append(ownedTabIDs, tabID)
		}
	}
	for _, tabID := range ownedTabIDs {
		a.releaseFloatingSessionsForTab(context.Background(), tabID)
		delete(a.workspaceViews, tabID)
		delete(a.windowForTab, tabID)
		if a.tabs != nil && a.tabs.Find(tabID) != nil {
			a.tabs.Remove(tabID)
		}
	}
	if removed != nil {
		removed.clearShellState()
	}
	delete(a.browserWindows, id)
	fallback := a.deterministicBrowserWindowFallback()
	if a.lastFocusedWindowID == id {
		if fallback != nil {
			a.lastFocusedWindowID = fallback.id
		} else {
			a.lastFocusedWindowID = ""
		}
	}
	if wasMainWindow {
		a.clearResizeModeBorder()
		if fallback != nil {
			a.activateBrowserWindow(fallback)
			return
		}
		a.lastFocusedWindowID = ""
		a.mainWindow = nil
		a.keyboardHandler = nil
		a.globalShortcutHandler = nil
		if a.tabCoord != nil {
			a.tabCoord.SetMainWindow(nil)
		}
	}
}

func (a *App) deterministicBrowserWindowFallback() *browserWindow {
	if a.browserWindows == nil {
		return nil
	}
	ids := make([]string, 0, len(a.browserWindows))
	for id, bw := range a.browserWindows {
		if id == "" || bw == nil || bw.mainWindow == nil {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return nil
	}
	return a.browserWindows[ids[0]]
}

func (a *App) OpenFreshWindow(ctx context.Context, url string) error {
	dispatch := a.dispatchOnMainThread
	if dispatch == nil {
		dispatch = func(fn func()) { fn() }
	}

	var openErr error
	dispatch(func() {
		openErr = a.openFreshWindow(ctx, url)
	})

	return openErr
}
