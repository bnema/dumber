package content

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/component"
)

func newRevealTestCoordinator() *Coordinator {
	return &Coordinator{
		webViews:       make(map[entity.PaneID]port.WebView),
		webViewPaneIDs: make(map[port.WebViewID]entity.PaneID),
		paneTitles:     make(map[entity.PaneID]string),
		navOrigins:     make(map[entity.PaneID]string),
	}
}

func revealTestWebView(t *testing.T, id port.WebViewID, generation uint64) *mocks.MockWebView {
	t.Helper()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(id).Maybe()
	wv.EXPECT().Generation().Return(generation).Maybe()
	return wv
}

func TestRevealState_IsBoundToCurrentWebViewAcrossReplacementPaths(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	first := revealTestWebView(t, 101, 1)
	// The pool may reuse the same native WebView ID; its reuse generation is
	// part of the presentation identity.
	directReplacement := revealTestWebView(t, 101, 2)
	popupReplacement := revealTestWebView(t, 103, 1)
	c := newRevealTestCoordinator()

	c.setWebViewLocked(paneID, first)
	firstIdentity, _ := identityForWebView(first)
	c.markWebViewRevealed(paneID, firstIdentity)
	assert.True(t, c.WebViewRevealed(paneID))

	// A direct replacement for the same pane must show the fresh loading state.
	c.setWebViewLocked(paneID, directReplacement)
	assert.False(t, c.WebViewRevealed(paneID))

	directReplacementIdentity, _ := identityForWebView(directReplacement)
	c.markWebViewRevealed(paneID, directReplacementIdentity)
	c.RegisterPopupWebView(paneID, popupReplacement)
	assert.False(t, c.WebViewRevealed(paneID), "popup registration must not inherit a prior WebView reveal")
}

func TestRevealState_ReleaseClearsStateWithoutWebViewMapping(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	wv := revealTestWebView(t, 101, 1)
	c := newRevealTestCoordinator()
	c.setWebViewLocked(paneID, wv)
	identity, _ := identityForWebView(wv)
	c.markWebViewRevealed(paneID, identity)

	// Model an earlier partial cleanup that lost the WebView mapping. Release
	// remains responsible for clearing presentation state on its early return.
	c.webViewsMu.Lock()
	delete(c.webViews, paneID)
	c.webViewsMu.Unlock()
	c.ReleaseWebView(context.Background(), paneID)

	c.revealMu.Lock()
	_, retained := c.revealedWebViews[paneID]
	c.revealMu.Unlock()
	assert.False(t, retained)
}

func TestRevealState_ReleaseMappedWebViewClearsRevealStateAndReturnsToPool(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	wv := revealTestWebView(t, 101, 1)
	pool := mocks.NewMockWebViewPool(t)
	pool.EXPECT().Release(wv).Once()
	c := newRevealTestCoordinator()
	c.pool = pool
	c.setWebViewLocked(paneID, wv)
	identity, _ := identityForWebView(wv)
	c.markWebViewRevealed(paneID, identity)

	c.ReleaseWebView(context.Background(), paneID)

	assert.Nil(t, c.GetWebView(paneID))
	assert.False(t, c.WebViewRevealed(paneID))
}

func TestRevealState_StaleCallbackCannotRevealReplacement(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	oldWV := revealTestWebView(t, 101, 1)
	newWV := revealTestWebView(t, 102, 1)
	oldWV.EXPECT().IsDestroyed().Return(false).Maybe()
	c := newRevealTestCoordinator()
	c.setWebViewLocked(paneID, oldWV)
	oldIdentity, _ := identityForWebView(oldWV)
	c.markPendingReveal(paneID, oldIdentity)
	c.setWebViewLocked(paneID, newWV)

	c.revealIfPending(context.Background(), paneID, oldWV, oldIdentity, "", "stale")
	assert.False(t, c.WebViewRevealed(paneID))
}

func TestRevealState_RevealedCurrentWebViewPersistsForWorkspaceRebuild(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	wv := revealTestWebView(t, 101, 1)
	c := newRevealTestCoordinator()
	c.setWebViewLocked(paneID, wv)
	identity, _ := identityForWebView(wv)
	c.markWebViewRevealed(paneID, identity)

	// AttachToWorkspace asks this identity-aware query when rebuilding a pane,
	// so the replacement PaneView hides its skeleton for this same WebView.
	assert.True(t, c.webViewRevealed(paneID, wv))

	replacement := revealTestWebView(t, 102, 1)
	assert.False(t, c.webViewRevealed(paneID, replacement))
}

func TestRevealCallback_CapturesGenerationBeforeSameObjectPoolReuse(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	wv := mocks.NewMockWebView(t)
	generation := uint64(1)
	wv.EXPECT().ID().Return(port.WebViewID(101)).Maybe()
	wv.EXPECT().Generation().RunAndReturn(func() uint64 { return generation }).Maybe()
	wv.EXPECT().IsDestroyed().Return(false).Maybe()
	var callbacks *port.WebViewCallbacks
	wv.EXPECT().SetCallbacks(mock.Anything).RunAndReturn(func(value *port.WebViewCallbacks) {
		callbacks = value
	}).Once()

	c := newRevealTestCoordinator()
	c.getActiveWS = func() (*entity.Workspace, *component.WorkspaceView) { return nil, nil }
	c.setWebViewLocked(paneID, wv)
	installedIdentity, _ := identityForWebView(wv)
	c.markPendingReveal(paneID, installedIdentity)
	c.setupWebViewCallbacks(context.Background(), paneID, wv)
	revealed := false
	c.setWebViewVisible = func(port.WebView) { revealed = true }

	// The callback was installed at generation 1. A pool reuse can advance the
	// same Go WebView object before its old callback is delivered.
	generation = 2
	callbacks.OnProgressChanged(1)

	assert.False(t, revealed, "the generation-1 callback must not reveal generation 2")
	assert.False(t, c.WebViewRevealed(paneID))
}

func TestRevealIfPending_SerializesReplacementUntilNativeVisibility(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	oldWV := revealTestWebView(t, 101, 1)
	oldWV.EXPECT().IsDestroyed().Return(false).Once()
	newWV := revealTestWebView(t, 102, 1)
	c := newRevealTestCoordinator()
	c.setWebViewLocked(paneID, oldWV)
	identity, _ := identityForWebView(oldWV)
	c.markPendingReveal(paneID, identity)

	nativeStarted := make(chan struct{})
	allowNativeReturn := make(chan struct{})
	c.setWebViewVisible = func(wv port.WebView) {
		assert.Same(t, oldWV, wv)
		close(nativeStarted)
		<-allowNativeReturn
	}
	revealed := make(chan struct{})
	go func() {
		c.revealIfPending(context.Background(), paneID, oldWV, identity, "", "test")
		close(revealed)
	}()
	<-nativeStarted

	replaced := make(chan struct{})
	go func() {
		c.setWebViewLocked(paneID, newWV)
		close(replaced)
	}()
	select {
	case <-replaced:
		t.Fatal("replacement interleaved before native visibility completed")
	default:
	}

	close(allowNativeReturn)
	<-revealed
	<-replaced
	assert.Same(t, newWV, c.GetWebView(paneID))
	assert.False(t, c.WebViewRevealed(paneID), "replacement must not inherit old reveal state")
}
