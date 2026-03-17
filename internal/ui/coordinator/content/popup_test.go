package content

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

// ---------------------------------------------------------------------------
// DetectPopupType
// ---------------------------------------------------------------------------

func TestDetectPopupType_BlankIsTab(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PopupTypeTab, DetectPopupType("_blank"))
}

func TestDetectPopupType_EmptyIsPopup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PopupTypePopup, DetectPopupType(""))
}

func TestDetectPopupType_NamedFrameIsPopup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PopupTypePopup, DetectPopupType("myFrame"))
}

// ---------------------------------------------------------------------------
// PopupType.String()
// ---------------------------------------------------------------------------

func TestPopupTypeString_Tab(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tab", PopupTypeTab.String())
}

func TestPopupTypeString_Popup(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "popup", PopupTypePopup.String())
}

func TestPopupTypeString_Unknown(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unknown", PopupType(99).String())
}

// ---------------------------------------------------------------------------
// GetBehavior
// ---------------------------------------------------------------------------

func TestGetBehavior_NilConfigReturnsSplit(t *testing.T) {
	t.Parallel()

	assert.Equal(t, config.PopupBehaviorSplit, GetBehavior(PopupTypeTab, nil))
	assert.Equal(t, config.PopupBehaviorSplit, GetBehavior(PopupTypePopup, nil))
}

func TestGetBehavior_TabPopup_DefaultConfig(t *testing.T) {
	t.Parallel()

	// BlankTargetBehavior is empty → falls through to default "stacked"
	cfg := &config.PopupBehaviorConfig{}
	assert.Equal(t, config.PopupBehaviorStacked, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_SplitConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.PopupBehaviorConfig{BlankTargetBehavior: "split"}
	assert.Equal(t, config.PopupBehaviorSplit, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_StackedConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.PopupBehaviorConfig{BlankTargetBehavior: "stacked"}
	assert.Equal(t, config.PopupBehaviorStacked, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_TabPopup_TabbedConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.PopupBehaviorConfig{BlankTargetBehavior: "tabbed"}
	assert.Equal(t, config.PopupBehaviorTabbed, GetBehavior(PopupTypeTab, cfg))
}

func TestGetBehavior_JSPopup_UsesBehaviorField(t *testing.T) {
	t.Parallel()

	cfg := &config.PopupBehaviorConfig{Behavior: config.PopupBehaviorWindowed}
	assert.Equal(t, config.PopupBehaviorWindowed, GetBehavior(PopupTypePopup, cfg))
}

func TestGetBehavior_JSPopup_DefaultConfig(t *testing.T) {
	t.Parallel()

	// Behavior zero-value is empty string; GetBehavior returns it as-is.
	cfg := &config.PopupBehaviorConfig{}
	assert.Equal(t, config.PopupBehavior(""), GetBehavior(PopupTypePopup, cfg))
}

func TestGetBehavior_TabPopup_BlocksWhenOpenInNewPaneFalse(t *testing.T) {
	t.Parallel()

	// GetBehavior itself does not honor OpenInNewPane — that is enforced by
	// handlePopupCreate. GetBehavior still returns the configured value.
	cfg := &config.PopupBehaviorConfig{
		OpenInNewPane:       false,
		BlankTargetBehavior: "split",
	}
	assert.Equal(t, config.PopupBehaviorSplit, GetBehavior(PopupTypeTab, cfg))
}

// ---------------------------------------------------------------------------
// trackOAuthPopup / capturePopupOAuthState
// ---------------------------------------------------------------------------

func TestTrackOAuthPopup_StoresState(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		webViews:   make(map[entity.PaneID]port.WebView),
		popupOAuth: make(map[port.WebViewID]*popupOAuthState),
	}

	popupID := port.WebViewID(1)
	parentPaneID := entity.PaneID("parent-pane")

	c.trackOAuthPopup(popupID, parentPaneID)

	c.popupMu.RLock()
	state, ok := c.popupOAuth[popupID]
	c.popupMu.RUnlock()

	assert.True(t, ok, "oauth state should be tracked after trackOAuthPopup")
	assert.Equal(t, parentPaneID, state.ParentPaneID)
	assert.False(t, state.Seen, "Seen should be false before capturing callback URI")
}

func TestCapturePopupOAuthState_SuccessCallback(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		webViews:   make(map[entity.PaneID]port.WebView),
		popupOAuth: make(map[port.WebViewID]*popupOAuthState),
	}

	popupID := port.WebViewID(2)
	c.trackOAuthPopup(popupID, entity.PaneID("parent"))
	c.capturePopupOAuthState(popupID, "https://app.example.com/callback?code=abc123")

	c.popupMu.RLock()
	state := c.popupOAuth[popupID]
	c.popupMu.RUnlock()

	assert.True(t, state.Seen)
	assert.True(t, state.Success)
	assert.False(t, state.Error)
	assert.Equal(t, "https://app.example.com/callback?code=abc123", state.CallbackURI)
}

func TestCapturePopupOAuthState_ErrorCallback(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		webViews:   make(map[entity.PaneID]port.WebView),
		popupOAuth: make(map[port.WebViewID]*popupOAuthState),
	}

	popupID := port.WebViewID(3)
	c.trackOAuthPopup(popupID, entity.PaneID("parent"))
	c.capturePopupOAuthState(popupID, "https://app.example.com/callback?error=access_denied")

	c.popupMu.RLock()
	state := c.popupOAuth[popupID]
	c.popupMu.RUnlock()

	assert.True(t, state.Seen)
	assert.False(t, state.Success)
	assert.True(t, state.Error)
}

func TestCapturePopupOAuthState_UnknownPopupIsNoop(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		webViews:   make(map[entity.PaneID]port.WebView),
		popupOAuth: make(map[port.WebViewID]*popupOAuthState),
	}

	// Should not panic when no tracking entry exists.
	c.capturePopupOAuthState(port.WebViewID(999), "https://example.com/callback?code=x")

	c.popupMu.RLock()
	_, ok := c.popupOAuth[port.WebViewID(999)]
	c.popupMu.RUnlock()

	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// SetPopupConfig
// ---------------------------------------------------------------------------

func TestSetPopupConfig_SetsFields(t *testing.T) {
	t.Parallel()

	c := &Coordinator{}

	factory := &stubFactory{}
	cfg := &config.PopupBehaviorConfig{Behavior: config.PopupBehaviorSplit}
	genID := func() string { return "test-id" }

	c.SetPopupConfig(factory, cfg, genID)

	assert.Equal(t, factory, c.factory)
	assert.Equal(t, cfg, c.popupConfig)
	assert.NotNil(t, c.generateID)
	assert.Equal(t, "test-id", c.generateID())
}

func TestSetPopupConfig_NilConfigAllowed(t *testing.T) {
	t.Parallel()

	c := &Coordinator{}
	c.SetPopupConfig(nil, nil, nil)

	assert.Nil(t, c.factory)
	assert.Nil(t, c.popupConfig)
	assert.Nil(t, c.generateID)
}

// stubFactory satisfies port.WebViewFactory without importing the mocks package.
type stubFactory struct{}

func (s *stubFactory) Create(_ context.Context) (port.WebView, error) {
	return nil, nil
}

func (s *stubFactory) CreateRelated(_ context.Context, _ port.WebViewID) (port.WebView, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Concurrent access to pendingPopups
// ---------------------------------------------------------------------------

func TestPendingPopups_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		pendingPopups: make(map[port.WebViewID]*PendingPopup),
	}

	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	// Writers
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			popupID := port.WebViewID(id)
			c.popupMu.Lock()
			c.pendingPopups[popupID] = &PendingPopup{
				FrameName: "_blank",
				PopupType: PopupTypeTab,
			}
			c.popupMu.Unlock()
		}(i)
	}

	// Readers
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			popupID := port.WebViewID(id)
			c.popupMu.RLock()
			_ = c.pendingPopups[popupID]
			c.popupMu.RUnlock()
		}(i)
	}

	wg.Wait()

	c.popupMu.RLock()
	count := len(c.pendingPopups)
	c.popupMu.RUnlock()

	assert.Equal(t, workers, count, "each writer uses a unique key, so all inserts should be present")
}

func TestPendingPopups_ConcurrentDeleteAndRead(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		pendingPopups: make(map[port.WebViewID]*PendingPopup),
	}

	// Pre-populate
	for i := 0; i < 10; i++ {
		c.pendingPopups[port.WebViewID(i)] = &PendingPopup{PopupType: PopupTypePopup}
	}

	var wg sync.WaitGroup
	wg.Add(20)

	// Deleters
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer wg.Done()
			c.popupMu.Lock()
			delete(c.pendingPopups, port.WebViewID(id))
			c.popupMu.Unlock()
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer wg.Done()
			c.popupMu.RLock()
			_ = c.pendingPopups[port.WebViewID(id)]
			c.popupMu.RUnlock()
		}(i)
	}

	wg.Wait()

	c.popupMu.RLock()
	defer c.popupMu.RUnlock()
	assert.Empty(t, c.pendingPopups, "all preloaded popups should have been deleted")
}
