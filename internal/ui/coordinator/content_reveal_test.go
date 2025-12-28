package coordinator

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
)

// TestPendingReveal tests the pending reveal state management.
// These are internal package tests that verify the state transitions
// without needing to mock WebKit dependencies.

func TestMarkPendingReveal_SetsState(t *testing.T) {
	c := &ContentCoordinator{
		pendingReveal: make(map[entity.PaneID]bool),
	}
	paneID := entity.PaneID("pane-1")

	c.markPendingReveal(paneID)

	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	assert.True(t, c.pendingReveal[paneID])
}

func TestClearPendingReveal_RemovesState(t *testing.T) {
	c := &ContentCoordinator{
		pendingReveal: make(map[entity.PaneID]bool),
	}
	paneID := entity.PaneID("pane-1")

	// First mark as pending
	c.markPendingReveal(paneID)

	// Then clear
	c.clearPendingReveal(paneID)

	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	assert.False(t, c.pendingReveal[paneID])
}

func TestRevealIfPending_NotPending_DoesNothing(t *testing.T) {
	callbackCalled := false
	c := &ContentCoordinator{
		pendingReveal: make(map[entity.PaneID]bool),
		webViews:      make(map[entity.PaneID]*webkit.WebView),
		onWebViewShown: func(paneID entity.PaneID) {
			callbackCalled = true
		},
	}
	paneID := entity.PaneID("pane-1")

	// Call reveal without marking pending first
	c.revealIfPending(context.Background(), paneID, "http://example.com", "test")

	assert.False(t, callbackCalled, "callback should not be called when not pending")
}

func TestRevealIfPending_ClearsStateOnReveal(t *testing.T) {
	c := &ContentCoordinator{
		pendingReveal: make(map[entity.PaneID]bool),
		webViews:      make(map[entity.PaneID]*webkit.WebView),
	}
	paneID := entity.PaneID("pane-1")

	// Mark as pending
	c.markPendingReveal(paneID)

	// Reveal (will return early since webview is nil, but should still clear state)
	c.revealIfPending(context.Background(), paneID, "http://example.com", "test")

	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	assert.False(t, c.pendingReveal[paneID], "pending state should be cleared after reveal attempt")
}

func TestRevealIfPending_OnlyRevealsOnce(t *testing.T) {
	revealCount := 0
	c := &ContentCoordinator{
		pendingReveal: make(map[entity.PaneID]bool),
		webViews:      make(map[entity.PaneID]*webkit.WebView),
		onWebViewShown: func(paneID entity.PaneID) {
			revealCount++
		},
	}
	paneID := entity.PaneID("pane-1")

	// Mark as pending
	c.markPendingReveal(paneID)

	// Call reveal multiple times
	c.revealIfPending(context.Background(), paneID, "", "first")
	c.revealIfPending(context.Background(), paneID, "", "second")
	c.revealIfPending(context.Background(), paneID, "", "third")

	// Callback should not be called since webview is nil, but state should be cleared
	assert.Equal(t, 0, revealCount, "callback not called when webview is nil")

	// Verify state is cleared
	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	assert.False(t, c.pendingReveal[paneID])
}

func TestPendingReveal_ConcurrentAccess(t *testing.T) {
	c := &ContentCoordinator{
		pendingReveal: make(map[entity.PaneID]bool),
		webViews:      make(map[entity.PaneID]*webkit.WebView),
	}

	var wg sync.WaitGroup
	paneIDs := []entity.PaneID{"pane-1", "pane-2", "pane-3", "pane-4", "pane-5"}

	// Concurrently mark and clear panes
	for i := 0; i < 100; i++ {
		for _, paneID := range paneIDs {
			wg.Add(3)

			go func(id entity.PaneID) {
				defer wg.Done()
				c.markPendingReveal(id)
			}(paneID)

			go func(id entity.PaneID) {
				defer wg.Done()
				c.clearPendingReveal(id)
			}(paneID)

			go func(id entity.PaneID) {
				defer wg.Done()
				c.revealIfPending(context.Background(), id, "", "concurrent")
			}(paneID)
		}
	}

	wg.Wait()
	// Test passes if no race conditions occur
}

func TestMarkPendingReveal_MultiplePanes(t *testing.T) {
	c := &ContentCoordinator{
		pendingReveal: make(map[entity.PaneID]bool),
	}

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")

	c.markPendingReveal(pane1)
	c.markPendingReveal(pane2)

	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	assert.True(t, c.pendingReveal[pane1])
	assert.True(t, c.pendingReveal[pane2])
}

func TestClearPendingReveal_OnlyAffectsTargetPane(t *testing.T) {
	c := &ContentCoordinator{
		pendingReveal: make(map[entity.PaneID]bool),
	}

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")

	c.markPendingReveal(pane1)
	c.markPendingReveal(pane2)
	c.clearPendingReveal(pane1)

	c.revealMu.Lock()
	defer c.revealMu.Unlock()
	assert.False(t, c.pendingReveal[pane1], "pane1 should be cleared")
	assert.True(t, c.pendingReveal[pane2], "pane2 should still be pending")
}
