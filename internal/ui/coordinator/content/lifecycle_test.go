package content

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

// newMinimalCoordinator returns a Coordinator with the maps required by
// ReleaseWebView initialized so that calls to clearPendingAppearance,
// titleMu / navOriginMu locks do not panic.
func newMinimalCoordinator() *Coordinator {
	return &Coordinator{
		webViews:   make(map[entity.PaneID]port.WebView),
		paneTitles: make(map[entity.PaneID]string),
		navOrigins: make(map[entity.PaneID]string),
	}
}

// ---------------------------------------------------------------------------
// EnsureWebView
// ---------------------------------------------------------------------------

func TestLifecycle_EnsureWebView_ReusesExistingNonDestroyed(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().IsDestroyed().Return(false)

	pool := mocks.NewMockWebViewPool(t)
	// pool.Acquire must NOT be called

	c := newMinimalCoordinator()
	c.pool = pool
	c.webViews[entity.PaneID("pane-1")] = wv

	got, err := c.EnsureWebView(context.Background(), "pane-1")

	require.NoError(t, err)
	assert.Equal(t, wv, got)
}

func TestLifecycle_EnsureWebView_AcquiresFromPoolWhenNoneExists(t *testing.T) {
	t.Parallel()

	newWV := mocks.NewMockWebView(t)
	newWV.EXPECT().SetCallbacks(mock.Anything).Maybe()
	newWV.EXPECT().Generation().Return(uint64(0)).Maybe()

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Acquire(mock.Anything).Return(newWV, nil)

	c := newMinimalCoordinator()
	c.pool = pool

	got, err := c.EnsureWebView(context.Background(), "pane-1")

	require.NoError(t, err)
	assert.Equal(t, newWV, got)
	assert.Equal(t, newWV, c.webViews[entity.PaneID("pane-1")])
}

func TestLifecycle_EnsureWebView_AcquiresFromPoolWhenExistingIsDestroyed(t *testing.T) {
	t.Parallel()

	oldWV := mocks.NewMockWebView(t)
	oldWV.EXPECT().IsDestroyed().Return(true)

	newWV := mocks.NewMockWebView(t)
	newWV.EXPECT().SetCallbacks(mock.Anything).Maybe()
	newWV.EXPECT().Generation().Return(uint64(0)).Maybe()

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Acquire(mock.Anything).Return(newWV, nil)

	c := newMinimalCoordinator()
	c.pool = pool
	c.webViews[entity.PaneID("pane-1")] = oldWV

	got, err := c.EnsureWebView(context.Background(), "pane-1")

	require.NoError(t, err)
	assert.Equal(t, newWV, got)
}

func TestLifecycle_EnsureWebView_ErrorWhenPoolIsNil(t *testing.T) {
	t.Parallel()

	c := newMinimalCoordinator()
	// pool remains nil

	_, err := c.EnsureWebView(context.Background(), "pane-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "webview pool not configured")
}

func TestLifecycle_EnsureWebView_ErrorWhenAcquireFails(t *testing.T) {
	t.Parallel()

	acquireErr := errors.New("pool exhausted")

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Acquire(mock.Anything).Return(nil, acquireErr)

	c := newMinimalCoordinator()
	c.pool = pool

	_, err := c.EnsureWebView(context.Background(), "pane-1")

	require.Error(t, err)
	assert.Equal(t, acquireErr, err)
}

// ---------------------------------------------------------------------------
// ReleaseWebView
// ---------------------------------------------------------------------------

func TestLifecycle_ReleaseWebView_ReleasesToPool(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	// idleInhibitor is nil so these won't be called
	wv.EXPECT().IsFullscreen().Return(false).Maybe()
	wv.EXPECT().IsPlayingAudio().Return(false).Maybe()

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Release(wv)

	c := newMinimalCoordinator()
	c.pool = pool
	c.webViews[entity.PaneID("pane-1")] = wv

	c.ReleaseWebView(context.Background(), "pane-1")

	_, exists := c.webViews[entity.PaneID("pane-1")]
	assert.False(t, exists, "webview should be removed from map after release")
}

func TestLifecycle_ReleaseWebView_NoopForUnknownPane(t *testing.T) {
	t.Parallel()

	pool := mocks.NewMockWebViewPool(t)
	// pool.Release must NOT be called

	c := newMinimalCoordinator()
	c.pool = pool

	// Should not panic
	c.ReleaseWebView(context.Background(), "unknown-pane")
}

func TestLifecycle_ReleaseWebView_UninhibitsIdleIfFullscreen(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().IsFullscreen().Return(true)
	wv.EXPECT().IsPlayingAudio().Return(false)

	inhibitor := mocks.NewMockIdleInhibitor(t)
	inhibitor.EXPECT().Uninhibit(mock.Anything).Return(nil)

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Release(wv)

	c := newMinimalCoordinator()
	c.pool = pool
	c.idleInhibitor = inhibitor
	c.webViews[entity.PaneID("pane-1")] = wv

	c.ReleaseWebView(context.Background(), "pane-1")
}

func TestLifecycle_ReleaseWebView_UninhibitsIdleIfPlayingAudio(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	wv.EXPECT().IsFullscreen().Return(false)
	wv.EXPECT().IsPlayingAudio().Return(true)

	inhibitor := mocks.NewMockIdleInhibitor(t)
	inhibitor.EXPECT().Uninhibit(mock.Anything).Return(nil)

	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Release(wv)

	c := newMinimalCoordinator()
	c.pool = pool
	c.idleInhibitor = inhibitor
	c.webViews[entity.PaneID("pane-1")] = wv

	c.ReleaseWebView(context.Background(), "pane-1")
}

func TestLifecycle_ReleaseWebView_DestroysWebViewWhenPoolIsNil(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)
	// idleInhibitor is nil so these won't be called
	wv.EXPECT().IsFullscreen().Return(false).Maybe()
	wv.EXPECT().IsPlayingAudio().Return(false).Maybe()
	wv.EXPECT().Destroy()

	c := newMinimalCoordinator()
	// pool remains nil
	c.webViews[entity.PaneID("pane-1")] = wv

	c.ReleaseWebView(context.Background(), "pane-1")
}

// ---------------------------------------------------------------------------
// GetWebView / RegisterPopupWebView
// ---------------------------------------------------------------------------

func TestLifecycle_GetWebView_ReturnsRegisteredWebView(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)

	c := newMinimalCoordinator()
	c.webViews[entity.PaneID("pane-1")] = wv

	got := c.GetWebView("pane-1")

	assert.Equal(t, wv, got)
}

func TestLifecycle_RegisterPopupWebView_StoresWebView(t *testing.T) {
	t.Parallel()

	wv := mocks.NewMockWebView(t)

	c := newMinimalCoordinator()

	c.RegisterPopupWebView("popup-1", wv)

	assert.Equal(t, wv, c.GetWebView("popup-1"))
}

func TestLifecycle_RegisterPopupWebView_IgnoresNil(t *testing.T) {
	t.Parallel()

	c := newMinimalCoordinator()

	// Should not panic
	c.RegisterPopupWebView("popup-1", nil)

	assert.Nil(t, c.GetWebView("popup-1"))
}
