package content

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
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
	c.markWebViewRevealed(paneID, first)
	assert.True(t, c.WebViewRevealed(paneID))

	// A direct replacement for the same pane must show the fresh loading state.
	c.setWebViewLocked(paneID, directReplacement)
	assert.False(t, c.WebViewRevealed(paneID))

	c.markWebViewRevealed(paneID, directReplacement)
	c.RegisterPopupWebView(paneID, popupReplacement)
	assert.False(t, c.WebViewRevealed(paneID), "popup registration must not inherit a prior WebView reveal")
}

func TestRevealState_ReleaseClearsStateWithoutWebViewMapping(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	wv := revealTestWebView(t, 101, 1)
	c := newRevealTestCoordinator()
	c.setWebViewLocked(paneID, wv)
	c.markWebViewRevealed(paneID, wv)

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

func TestRevealState_StaleCallbackCannotRevealReplacement(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	oldWV := revealTestWebView(t, 101, 1)
	newWV := revealTestWebView(t, 102, 1)
	oldWV.EXPECT().IsDestroyed().Return(false).Maybe()
	c := newRevealTestCoordinator()
	c.setWebViewLocked(paneID, oldWV)
	c.markPendingReveal(paneID, oldWV)
	c.setWebViewLocked(paneID, newWV)

	c.revealIfPending(context.Background(), paneID, oldWV, "", "stale")
	assert.False(t, c.WebViewRevealed(paneID))
}

func TestRevealState_RevealedCurrentWebViewPersistsForWorkspaceRebuild(t *testing.T) {
	paneID := entity.PaneID("pane-1")
	wv := revealTestWebView(t, 101, 1)
	c := newRevealTestCoordinator()
	c.setWebViewLocked(paneID, wv)
	c.markWebViewRevealed(paneID, wv)

	// AttachToWorkspace asks this identity-aware query when rebuilding a pane,
	// so the replacement PaneView hides its skeleton for this same WebView.
	assert.True(t, c.WebViewRevealed(paneID))
}
