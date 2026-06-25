package ui

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/gtk"
)

func (a *App) createBrowserWindow(ctx context.Context, initialURL string) (*browserWindow, error) {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("url_host", logging.SafeURLHost(initialURL)).
		Bool("using_factory", a.browserWindowFactory != nil).
		Int("window_count_before", len(a.browserWindows)).
		Msg("ui: create browser window started")

	if a.browserWindowFactory != nil {
		created, err := a.browserWindowFactory(ctx, initialURL)
		if err != nil {
			log.Warn().Err(err).
				Str("url_host", logging.SafeURLHost(initialURL)).
				Msg("ui: browser window factory failed")
			return nil, err
		}
		created.ensureTabs()
		a.wireBrowserWindowActivationTracking(created)
		created.initChrome(ctx, a)
		return created, nil
	}

	runtimeCfg := a.runtimeConfigSnapshot().UI
	mainWindow, err := window.New(ctx, a.gtkApp, runtimeCfg.Workspace.TabBarPosition)
	if err != nil {
		log.Warn().Err(err).
			Str("url_host", logging.SafeURLHost(initialURL)).
			Msg("ui: GTK browser window shell creation failed")
		return nil, err
	}
	browserWindow := &browserWindow{
		id:         a.generateWindowID(),
		initialURL: initialURL,
		tabs:       entity.NewTabList(),
		mainWindow: mainWindow,
	}
	a.wireBrowserWindowActivationTracking(browserWindow)
	if a.mainWindow == nil {
		a.mainWindow = mainWindow
	}

	closeRequestCb := func(_ gtk.Window) bool {
		log.Info().Msg("browser window close requested")
		a.removeBrowserWindow(browserWindow.id)
		return false
	}
	mainWindow.Window().ConnectCloseRequest(&closeRequestCb)

	a.initBrowserWindowOverlays(mainWindow, browserWindow, runtimeCfg)
	a.wireBrowserWindowPermissionIndicator(browserWindow)
	if a.kbDispatcher != nil {
		a.initBrowserWindowInput(ctx, browserWindow)
	}
	if a.tabCoord != nil {
		a.wireBrowserWindowTabBar(ctx, browserWindow)
	}
	browserWindow.initChrome(ctx, a)

	// Apply GTK CSS styling from theme manager.
	if a.deps == nil || a.deps.Theme == nil {
		return browserWindow, nil
	}
	if display := mainWindow.Window().GetDisplay(); display != nil {
		a.deps.Theme.ApplyToDisplay(ctx, display)
	}
	log.Debug().
		Str("window_id", browserWindow.id).
		Str("url_host", logging.SafeURLHost(initialURL)).
		Msg("ui: create browser window completed")
	return browserWindow, nil
}

func (a *App) openFreshWindow(ctx context.Context, url string) error {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("url_host", logging.SafeURLHost(url)).
		Int("window_count_before", len(a.browserWindows)).
		Bool("has_tab_coord", a.tabCoord != nil).
		Bool("has_tabs_uc", a.tabsUC != nil).
		Msg("ui: open fresh window started")

	created, err := a.createBrowserWindow(ctx, url)
	if err != nil {
		log.Warn().Err(err).
			Str("url_host", logging.SafeURLHost(url)).
			Int("window_count_after", len(a.browserWindows)).
			Msg("ui: open fresh window could not create browser window")
		return err
	}
	log.Debug().
		Str("window_id", created.id).
		Str("url_host", logging.SafeURLHost(url)).
		Msg("ui: open fresh window created browser window shell")
	a.registerBrowserWindow(created)
	log.Debug().
		Str("window_id", created.id).
		Int("window_count_after_register", len(a.browserWindows)).
		Msg("ui: open fresh window registered browser window")

	if len(a.browserWindows) == 1 && (url == "" || (a.deps != nil && a.deps.RestoreSessionID != "")) {
		log.Debug().Str("window_id", created.id).Msg("ui: activating first browser window without initial tab")
		a.activateBrowserWindow(created)
		return nil
	}

	var openErr error
	path := "tabs_uc"
	if a.tabCoord != nil {
		path = "tab_coord"
		log.Debug().Str("window_id", created.id).Msg("ui: creating initial tab for fresh window via tab coordinator")
		openErr = a.openFreshWindowWithTabCoord(ctx, url, created)
	} else if a.tabsUC == nil {
		log.Debug().Str("window_id", created.id).Msg("ui: activating fresh browser window without tab use case")
		a.activateBrowserWindow(created)
		return nil
	} else {
		log.Debug().Str("window_id", created.id).Msg("ui: creating initial tab for fresh window via tabs use case")
		openErr = a.openFreshWindowWithTabsUC(ctx, url, created)
	}
	if openErr != nil {
		log.Warn().Err(openErr).
			Str("window_id", created.id).
			Str("path", path).
			Int("window_count_after", len(a.browserWindows)).
			Msg("ui: open fresh window initial tab creation failed")
		return openErr
	}
	log.Debug().
		Str("window_id", created.id).
		Str("path", path).
		Msg("ui: activating fresh browser window after tab creation")
	a.activateBrowserWindow(created)
	return nil
}

func (a *App) openFreshWindowWithTabCoord(ctx context.Context, url string, created *browserWindow) error {
	log := logging.FromContext(ctx)
	previousTarget := a.tabCoord.CurrentTarget()
	target := a.ensureTabTargetForBrowserWindow(created)
	a.tabCoord.SetCurrentTarget(target)
	defer func() {
		a.tabCoord.SetCurrentTarget(previousTarget)
	}()

	log.Debug().
		Str("window_id", created.id).
		Str("url_host", logging.SafeURLHost(url)).
		Msg("ui: tab coordinator create started for fresh window")
	createdTab, createErr := a.tabCoord.Create(ctx, target, url)
	if createErr != nil {
		log.Warn().Err(createErr).
			Str("window_id", created.id).
			Msg("ui: tab coordinator create failed for fresh window; rolling back window")
		a.removeBrowserWindow(created.id)
		if created.mainWindow != nil {
			created.mainWindow.Destroy()
		}
		return createErr
	}
	log.Debug().
		Str("window_id", created.id).
		Str("tab_id", string(createdTab.ID)).
		Msg("ui: tab coordinator create completed for fresh window")
	if a.workspaceViews[createdTab.ID] == nil {
		log.Warn().
			Str("window_id", created.id).
			Str("tab_id", string(createdTab.ID)).
			Msg("ui: fresh window tab has no workspace view; rolling back window")
		if createdTabs := a.tabListForBrowserWindow(created); createdTabs != nil {
			createdTabs.Remove(createdTab.ID)
		}
		if created.mainWindow != nil {
			if tabBar := created.mainWindow.TabBar(); tabBar != nil {
				tabBar.RemoveTab(createdTab.ID)
				if activeTab := a.activeTabForBrowserWindow(created); activeTab != nil {
					tabBar.SetActive(activeTab.ID)
				}
			}
		}
		a.removeBrowserWindow(created.id)
		if created.mainWindow != nil {
			created.mainWindow.Destroy()
		}
		return fmt.Errorf("workspace view not created for tab %s", createdTab.ID)
	}
	a.setBrowserWindowForTab(createdTab.ID, created)
	// Tab was already created in the per-window TabList via tabCoord.Create;
	// active state is managed by TabList.SetActive.
	if created.mainWindow != nil {
		created.mainWindow.Show()
	}
	return nil
}

func (a *App) openFreshWindowWithTabsUC(ctx context.Context, url string, created *browserWindow) error {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("window_id", created.id).
		Str("url_host", logging.SafeURLHost(url)).
		Msg("ui: tabs use case create started for fresh window")
	output, err := a.tabsUC.Create(ctx, usecase.CreateTabInput{
		TabList:    a.tabs,
		InitialURL: url,
	})
	if err != nil {
		log.Warn().Err(err).
			Str("window_id", created.id).
			Msg("ui: tabs use case create failed for fresh window; rolling back window")
		a.removeBrowserWindow(created.id)
		if created.mainWindow != nil {
			created.mainWindow.Destroy()
		}
		return err
	}
	log.Debug().
		Str("window_id", created.id).
		Str("tab_id", string(output.Tab.ID)).
		Msg("ui: tabs use case create completed for fresh window")
	a.setBrowserWindowForTab(output.Tab.ID, created)
	// Mirror the tabsUC-created tab into the new browser window's tab list.
	// This path uses the tabs usecase directly instead of tabCoord.Create,
	// so the per-window TabList must be populated explicitly.
	a.ensureTabListForBrowserWindow(created).Add(output.Tab)
	if a.widgetFactory != nil || a.workspaceViewCreateOverride != nil {
		a.createWorkspaceView(ctx, output.Tab)
		a.switchWorkspaceView(ctx, output.Tab.ID)
	}
	if a.workspaceViews[output.Tab.ID] == nil {
		log.Warn().
			Str("window_id", created.id).
			Str("tab_id", string(output.Tab.ID)).
			Msg("ui: fresh window tabs use case tab has no workspace view; rolling back window")
		if createdTabs := a.tabListForBrowserWindow(created); createdTabs != nil {
			createdTabs.Remove(output.Tab.ID)
		}
		if a.tabs != nil {
			a.tabs.Remove(output.Tab.ID)
		}
		if created.mainWindow != nil {
			if tabBar := created.mainWindow.TabBar(); tabBar != nil {
				tabBar.RemoveTab(output.Tab.ID)
				if activeTab := a.activeTabForBrowserWindow(created); activeTab != nil {
					tabBar.SetActive(activeTab.ID)
				}
			}
		}
		a.removeBrowserWindow(created.id)
		if created.mainWindow != nil {
			created.mainWindow.Destroy()
		}
		return fmt.Errorf("workspace view not created for tab %s", output.Tab.ID)
	}
	if created.mainWindow != nil {
		created.mainWindow.Show()
	}
	return nil
}
