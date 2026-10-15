package ui

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	contentcoord "github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const relayTestURL = "https://claude.com/cai/login/app-google-auth?open_in_browser=1&hop_nonce=XYZ"

type relayReuseHarness struct {
	app     *App
	tabs    *entity.TabList
	window  *browserWindow
	webview map[entity.PaneID]*recordingWebView
	clock   *time.Time
}

// newRelayReuseHarness builds an App with one window holding one tab, using the
// tabbed external-link behavior. New tabs get a recording webview registered so
// reuse can navigate them.
func newRelayReuseHarness(t *testing.T) *relayReuseHarness {
	t.Helper()
	ctx := context.Background()

	original := entity.NewTab("tab-1", "ws-1", entity.NewPane("pane-1"))
	tabs := entity.NewTabList()
	tabs.Add(original)
	window := &browserWindow{id: "focused", tabs: tabs}

	ids := []string{"tab-2", "ws-2", "pane-2", "tab-3", "ws-3", "pane-3"}
	index := 0
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	h := &relayReuseHarness{tabs: tabs, window: window, webview: map[entity.PaneID]*recordingWebView{}, clock: &now}

	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	app := &App{
		runtimeConfig: runtimeConfigStateForTest(&config.Config{Workspace: entity.WorkspaceConfig{ExternalLinks: entity.ExternalLinksConfig{Behavior: entity.ExternalLinkBehaviorTabbed}}}),
		tabsUC:        usecase.NewManageTabsUseCase(func() string { id := ids[index]; index++; return id }, nil),
		browserWindows: map[string]*browserWindow{
			window.id: window,
		},
		browserWindowOrder:  []string{window.id},
		lastFocusedWindowID: window.id,
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{},
		windowForTab:        map[entity.TabID]*browserWindow{},
		contentCoord:        contentCoord,
		navCoord:            coordinator.NewNavigationCoordinator(ctx, nil, contentCoord),
	}
	app.externalRelayPanes.now = func() time.Time { return *h.clock }
	app.tabCoord = coordinator.NewTabCoordinator(ctx, coordinator.TabCoordinatorConfig{TabsUC: app.tabsUC})
	app.tabCoord.SetOnTabCreated(func(_ context.Context, _ coordinator.TabTarget, tab *entity.Tab) {
		app.workspaceViews[tab.ID] = &component.WorkspaceView{}
		app.setBrowserWindowForTab(tab.ID, window)
		paneID := tab.Workspace.ActivePane().Pane.ID
		wv := &recordingWebView{id: port.WebViewID(100 + len(h.webview))}
		h.webview[paneID] = wv
		contentCoord.RegisterPopupWebView(paneID, wv)
	})
	h.app = app
	return h
}

func TestApp_OpenExternalURLReusesRecentRelayPane(t *testing.T) {
	ctx := context.Background()
	h := newRelayReuseHarness(t)

	require.NoError(t, h.app.OpenExternalURL(ctx, relayTestURL))
	require.Len(t, h.tabs.Tabs, 2)
	relayTab := h.tabs.ActiveTab()
	relayPaneID := relayTab.Workspace.ActivePane().Pane.ID
	assert.False(t, h.webview[relayPaneID].loadURICalled)

	// The user moves on to another tab, then the app retries the same URL.
	h.tabs.SetActive("tab-1")
	*h.clock = h.clock.Add(30 * time.Second)

	require.NoError(t, h.app.OpenExternalURL(ctx, relayTestURL))

	assert.Len(t, h.tabs.Tabs, 2, "no new tab must be created")
	assert.Equal(t, relayTab.ID, h.tabs.ActiveTabID, "relay tab must be focused again")
	assert.True(t, h.webview[relayPaneID].loadURICalled)
	assert.Equal(t, relayTestURL, h.webview[relayPaneID].loadURILastURI)
}

func TestApp_OpenExternalURLCreatesNewPaneAfterReuseWindowExpires(t *testing.T) {
	ctx := context.Background()
	h := newRelayReuseHarness(t)

	require.NoError(t, h.app.OpenExternalURL(ctx, relayTestURL))
	require.Len(t, h.tabs.Tabs, 2)

	*h.clock = h.clock.Add(domainurl.RelayReuseWindow + time.Second)
	require.NoError(t, h.app.OpenExternalURL(ctx, relayTestURL))

	assert.Len(t, h.tabs.Tabs, 3)
}

func TestApp_OpenExternalURLCreatesNewPaneForDifferentURL(t *testing.T) {
	ctx := context.Background()
	h := newRelayReuseHarness(t)

	require.NoError(t, h.app.OpenExternalURL(ctx, relayTestURL))
	require.NoError(t, h.app.OpenExternalURL(ctx, relayTestURL+"2"))

	assert.Len(t, h.tabs.Tabs, 3)
}

func TestApp_OpenExternalURLCreatesNewPaneWhenRelayPaneWasClosed(t *testing.T) {
	ctx := context.Background()
	h := newRelayReuseHarness(t)

	require.NoError(t, h.app.OpenExternalURL(ctx, relayTestURL))
	relayTab := h.tabs.ActiveTab()
	h.tabs.Remove(relayTab.ID)
	require.Len(t, h.tabs.Tabs, 1)

	require.NoError(t, h.app.OpenExternalURL(ctx, relayTestURL))

	assert.Len(t, h.tabs.Tabs, 2)
	assert.NotEqual(t, relayTab.ID, h.tabs.ActiveTabID)
	assert.Len(t, h.app.externalRelayPanes.entries, 1, "stale entry replaced by the new pane")
}

func TestRelayPaneRegistry_PrunesExpiredAndMissingPanes(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	reg := relayPaneRegistry{now: func() time.Time { return now }}
	reg.record("https://a.example/", "pane-a")
	reg.record("https://b.example/", "pane-b")

	exists := func(id entity.PaneID) bool { return id != "pane-b" }
	_, ok := reg.lookup("https://b.example/", exists)
	assert.False(t, ok)
	assert.NotContains(t, reg.entries, "https://b.example/")

	id, ok := reg.lookup("https://a.example/", exists)
	assert.True(t, ok)
	assert.Equal(t, entity.PaneID("pane-a"), id)

	now = now.Add(domainurl.RelayReuseWindow + time.Second)
	_, ok = reg.lookup("https://a.example/", exists)
	assert.False(t, ok)
	assert.Empty(t, reg.entries)
}
