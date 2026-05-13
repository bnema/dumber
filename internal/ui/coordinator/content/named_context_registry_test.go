package content

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
)

func TestNamedBrowsingContextRegistry_LookupReusesWithinSameWindow(t *testing.T) {
	t.Parallel()

	reg := newNamedBrowsingContextRegistry()
	paneID := entity.PaneID("pane-1")
	webViewID := port.WebViewID(41)
	reg.Register("window-1", "shared-pane", paneID, webViewID)

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().ID().Return(webViewID).Once()

	state, gotWV, ok := reg.Lookup(
		"window-1",
		"shared-pane",
		func(gotPaneID entity.PaneID) port.WebView {
			assert.Equal(t, paneID, gotPaneID)
			return wv
		},
		func(gotPaneID entity.PaneID) (string, bool) {
			assert.Equal(t, paneID, gotPaneID)
			return "window-1", true
		},
	)

	assert.True(t, ok)
	assert.Equal(t, paneID, state.PaneID)
	assert.Same(t, wv, gotWV)
}

func TestNamedBrowsingContextRegistry_LookupIsolatedAcrossWindows(t *testing.T) {
	t.Parallel()

	reg := newNamedBrowsingContextRegistry()
	reg.Register("window-1", "shared-pane", entity.PaneID("pane-1"), port.WebViewID(41))

	_, _, ok := reg.Lookup(
		"window-2",
		"shared-pane",
		func(entity.PaneID) port.WebView { return nil },
		func(entity.PaneID) (string, bool) { return "window-2", true },
	)

	assert.False(t, ok)
}

func TestNamedBrowsingContextRegistry_DropsStaleWindowOwnershipOnLookup(t *testing.T) {
	t.Parallel()

	reg := newNamedBrowsingContextRegistry()
	paneID := entity.PaneID("pane-1")
	reg.Register("window-1", "shared-pane", paneID, port.WebViewID(41))

	_, _, ok := reg.Lookup(
		"window-1",
		"shared-pane",
		func(entity.PaneID) port.WebView { return nil },
		func(entity.PaneID) (string, bool) { return "window-2", true },
	)
	assert.False(t, ok)

	_, _, ok = reg.Lookup(
		"window-1",
		"shared-pane",
		func(entity.PaneID) port.WebView { return nil },
		func(entity.PaneID) (string, bool) { return "window-1", true },
	)
	assert.False(t, ok)
}

func TestNamedBrowsingContextRegistry_LookupDoesNotDeleteReRegisteredEntryAfterWindowMismatch(t *testing.T) {
	t.Parallel()

	reg := newNamedBrowsingContextRegistry()
	oldPaneID := entity.PaneID("pane-old")
	newPaneID := entity.PaneID("pane-new")
	newWebViewID := port.WebViewID(42)
	reg.Register("window-1", "shared-pane", oldPaneID, port.WebViewID(41))

	_, _, ok := reg.Lookup(
		"window-1",
		"shared-pane",
		func(entity.PaneID) port.WebView { return nil },
		func(gotPaneID entity.PaneID) (string, bool) {
			assert.Equal(t, oldPaneID, gotPaneID)
			reg.Register("window-1", "shared-pane", newPaneID, newWebViewID)
			return "window-2", true
		},
	)
	assert.False(t, ok)

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().ID().Return(newWebViewID).Once()

	state, gotWV, ok := reg.Lookup(
		"window-1",
		"shared-pane",
		func(gotPaneID entity.PaneID) port.WebView {
			assert.Equal(t, newPaneID, gotPaneID)
			return wv
		},
		func(gotPaneID entity.PaneID) (string, bool) {
			assert.Equal(t, newPaneID, gotPaneID)
			return "window-1", true
		},
	)
	assert.True(t, ok)
	assert.Equal(t, newPaneID, state.PaneID)
	assert.Same(t, wv, gotWV)
}

func TestNamedBrowsingContextRegistry_LookupDoesNotDeleteReRegisteredEntryAfterStaleWebView(t *testing.T) {
	t.Parallel()

	reg := newNamedBrowsingContextRegistry()
	oldPaneID := entity.PaneID("pane-old")
	newPaneID := entity.PaneID("pane-new")
	newWebViewID := port.WebViewID(42)
	reg.Register("window-1", "shared-pane", oldPaneID, port.WebViewID(41))

	_, _, ok := reg.Lookup(
		"window-1",
		"shared-pane",
		func(gotPaneID entity.PaneID) port.WebView {
			assert.Equal(t, oldPaneID, gotPaneID)
			reg.Register("window-1", "shared-pane", newPaneID, newWebViewID)
			return nil
		},
		func(gotPaneID entity.PaneID) (string, bool) {
			assert.Equal(t, oldPaneID, gotPaneID)
			return "window-1", true
		},
	)
	assert.False(t, ok)

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false).Once()
	wv.EXPECT().ID().Return(newWebViewID).Once()

	state, gotWV, ok := reg.Lookup(
		"window-1",
		"shared-pane",
		func(gotPaneID entity.PaneID) port.WebView {
			assert.Equal(t, newPaneID, gotPaneID)
			return wv
		},
		func(gotPaneID entity.PaneID) (string, bool) {
			assert.Equal(t, newPaneID, gotPaneID)
			return "window-1", true
		},
	)
	assert.True(t, ok)
	assert.Equal(t, newPaneID, state.PaneID)
	assert.Same(t, wv, gotWV)
}
