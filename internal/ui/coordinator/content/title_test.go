package content

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/domain/entity"
)

// TestGetTitle tests the GetTitle method for title state retrieval.

func TestGetTitle_ReturnsEmptyForUnknownPane(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		paneTitles: make(map[entity.PaneID]string),
	}

	title := c.GetTitle("unknown-pane")
	assert.Empty(t, title)
}

func TestGetTitle_ReturnsTrackedTitle(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{
		paneTitles: map[entity.PaneID]string{
			paneID: "My Page Title",
		},
	}

	title := c.GetTitle(paneID)
	assert.Equal(t, "My Page Title", title)
}

func TestGetTitle_DirectMapUpdate(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{
		paneTitles: make(map[entity.PaneID]string),
	}

	// Simulate the write that onTitleChanged performs
	c.titleMu.Lock()
	c.paneTitles[paneID] = "Updated Title"
	c.titleMu.Unlock()

	title := c.GetTitle(paneID)
	assert.Equal(t, "Updated Title", title)
}

func TestGetTitle_EmptyStringAfterZeroValue(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{
		paneTitles: map[entity.PaneID]string{
			paneID: "",
		},
	}

	title := c.GetTitle(paneID)
	assert.Empty(t, title)
}

// TestPaneTitles_ConcurrentAccess verifies that concurrent reads and writes to
// paneTitles do not race.
func TestPaneTitles_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		paneTitles: make(map[entity.PaneID]string),
	}

	paneIDs := []entity.PaneID{"pane-1", "pane-2", "pane-3"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		for _, paneID := range paneIDs {
			wg.Add(2)

			go func(id entity.PaneID) {
				defer wg.Done()
				c.titleMu.Lock()
				c.paneTitles[id] = "title for " + string(id)
				c.titleMu.Unlock()
			}(paneID)

			go func(id entity.PaneID) {
				defer wg.Done()
				_ = c.GetTitle(id)
			}(paneID)
		}
	}

	wg.Wait()
	// Test passes if no race conditions occur
}

func TestPaneTitles_MultiplepanesTrackedIndependently(t *testing.T) {
	t.Parallel()

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")

	c := &Coordinator{
		paneTitles: map[entity.PaneID]string{
			pane1: "Title One",
			pane2: "Title Two",
		},
	}

	assert.Equal(t, "Title One", c.GetTitle(pane1))
	assert.Equal(t, "Title Two", c.GetTitle(pane2))
}

// TestSetNavigationOrigin tests navigation origin storage and retrieval.

func TestSetNavigationOrigin_StoresOriginURL(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{
		navOrigins: make(map[entity.PaneID]string),
	}

	c.SetNavigationOrigin(paneID, "https://google.fr/search?q=test")

	c.navOriginMu.RLock()
	origin := c.navOrigins[paneID]
	c.navOriginMu.RUnlock()

	assert.Equal(t, "https://google.fr/search?q=test", origin)
}

func TestSetNavigationOrigin_OverwritesPreviousOrigin(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{
		navOrigins: make(map[entity.PaneID]string),
	}

	c.SetNavigationOrigin(paneID, "https://google.fr/")
	c.SetNavigationOrigin(paneID, "https://example.com/new")

	c.navOriginMu.RLock()
	origin := c.navOrigins[paneID]
	c.navOriginMu.RUnlock()

	assert.Equal(t, "https://example.com/new", origin)
}

func TestSetNavigationOrigin_IndependentPerPane(t *testing.T) {
	t.Parallel()

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")
	c := &Coordinator{
		navOrigins: make(map[entity.PaneID]string),
	}

	c.SetNavigationOrigin(pane1, "https://pane1.example.com/")
	c.SetNavigationOrigin(pane2, "https://pane2.example.com/")

	c.navOriginMu.RLock()
	origin1 := c.navOrigins[pane1]
	origin2 := c.navOrigins[pane2]
	c.navOriginMu.RUnlock()

	assert.Equal(t, "https://pane1.example.com/", origin1)
	assert.Equal(t, "https://pane2.example.com/", origin2)
}

func TestSetNavigationOrigin_EmptyOrigin(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{
		navOrigins: map[entity.PaneID]string{
			paneID: "https://old-origin.example.com/",
		},
	}

	c.SetNavigationOrigin(paneID, "")

	c.navOriginMu.RLock()
	origin := c.navOrigins[paneID]
	c.navOriginMu.RUnlock()

	assert.Empty(t, origin)
}

// TestNavOrigins_ConcurrentAccess verifies that concurrent reads and writes to
// navOrigins do not race.
func TestNavOrigins_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	c := &Coordinator{
		navOrigins: make(map[entity.PaneID]string),
	}

	paneIDs := []entity.PaneID{"pane-1", "pane-2", "pane-3"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		for _, paneID := range paneIDs {
			wg.Add(2)

			go func(id entity.PaneID) {
				defer wg.Done()
				c.SetNavigationOrigin(id, "https://origin-for-"+string(id)+".example.com/")
			}(paneID)

			go func(id entity.PaneID) {
				defer wg.Done()
				c.navOriginMu.RLock()
				_ = c.navOrigins[id]
				c.navOriginMu.RUnlock()
			}(paneID)
		}
	}

	wg.Wait()
	// Test passes if no race conditions occur
}

// TestQueueThemeApply tests the appearance queue management for theme applies.

func TestQueueThemeApply_TakeReturnsTrue(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{}

	c.queueThemeApply(paneID, true, "body { color: white; }")

	update, ok := c.takePendingThemeApply(paneID)
	assert.True(t, ok)
	assert.True(t, update.prefersDark)
	assert.Equal(t, "body { color: white; }", update.cssText)
}

func TestQueueThemeApply_SecondTakeReturnsFalse(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{}

	c.queueThemeApply(paneID, false, "body { color: black; }")

	_, firstOk := c.takePendingThemeApply(paneID)
	assert.True(t, firstOk)

	_, secondOk := c.takePendingThemeApply(paneID)
	assert.False(t, secondOk)
}

func TestQueueThemeApply_TakeWithoutQueue_ReturnsFalse(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{}

	_, ok := c.takePendingThemeApply(paneID)
	assert.False(t, ok)
}

func TestQueueThemeApply_MultiplePanesQueued_Independently(t *testing.T) {
	t.Parallel()

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")
	c := &Coordinator{}

	// pendingThemeUpdate is a single shared payload; queueThemeApply always
	// overwrites it with the latest call. Both panes are marked as pending,
	// but they share the last-written update.
	c.queueThemeApply(pane1, true, "shared-css")
	c.queueThemeApply(pane2, true, "shared-css")

	// Both panes should be independently takeable
	update1, ok1 := c.takePendingThemeApply(pane1)
	assert.True(t, ok1)
	assert.Equal(t, "shared-css", update1.cssText)

	// pane2 should still be queued after taking pane1
	update2, ok2 := c.takePendingThemeApply(pane2)
	assert.True(t, ok2)
	assert.Equal(t, "shared-css", update2.cssText)

	// Neither should be available after both are taken
	_, ok3 := c.takePendingThemeApply(pane1)
	assert.False(t, ok3)
	_, ok4 := c.takePendingThemeApply(pane2)
	assert.False(t, ok4)
}

func TestQueueThemeApply_LastWriteWins(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{}

	// Queue twice; the second call overwrites pendingThemeUpdate
	c.queueThemeApply(paneID, true, "first-css")
	c.queueThemeApply(paneID, false, "second-css")

	update, ok := c.takePendingThemeApply(paneID)
	assert.True(t, ok)
	assert.False(t, update.prefersDark)
	assert.Equal(t, "second-css", update.cssText)
}

func TestQueueThemeApply_HasPendingFlagClearedWhenAllTaken(t *testing.T) {
	t.Parallel()

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")
	c := &Coordinator{}

	c.queueThemeApply(pane1, true, "css")
	c.queueThemeApply(pane2, true, "css")

	_, _ = c.takePendingThemeApply(pane1)

	// hasPendingThemeUpdate should still be true since pane2 is pending
	c.appearanceMu.Lock()
	assert.True(t, c.hasPendingThemeUpdate)
	c.appearanceMu.Unlock()

	_, _ = c.takePendingThemeApply(pane2)

	// Now hasPendingThemeUpdate should be false
	c.appearanceMu.Lock()
	assert.False(t, c.hasPendingThemeUpdate)
	c.appearanceMu.Unlock()
}

// TestQueueScriptRefresh tests the appearance queue management for script refreshes.

func TestQueueScriptRefresh_TakeReturnsTrue(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{}

	c.queueScriptRefresh(paneID)

	ok := c.takePendingScriptRefresh(paneID)
	assert.True(t, ok)
}

func TestQueueScriptRefresh_SecondTakeReturnsFalse(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{}

	c.queueScriptRefresh(paneID)

	firstOk := c.takePendingScriptRefresh(paneID)
	assert.True(t, firstOk)

	secondOk := c.takePendingScriptRefresh(paneID)
	assert.False(t, secondOk)
}

func TestQueueScriptRefresh_TakeWithoutQueue_ReturnsFalse(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{}

	ok := c.takePendingScriptRefresh(paneID)
	assert.False(t, ok)
}

func TestQueueScriptRefresh_MultiplePanesQueuedIndependently(t *testing.T) {
	t.Parallel()

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")
	pane3 := entity.PaneID("pane-3")
	c := &Coordinator{}

	c.queueScriptRefresh(pane1)
	c.queueScriptRefresh(pane2)

	// pane3 was never queued
	assert.True(t, c.takePendingScriptRefresh(pane1))
	assert.True(t, c.takePendingScriptRefresh(pane2))
	assert.False(t, c.takePendingScriptRefresh(pane3))
}

// TestApplyPendingThemeUpdate tests the state management aspect of applyPendingThemeUpdate.

func TestApplyPendingThemeUpdate_ReturnsFalseWhenNothingQueued(t *testing.T) {
	t.Parallel()

	paneID := entity.PaneID("pane-1")
	c := &Coordinator{}

	// applyPendingThemeUpdate calls takePendingThemeApply; with nothing queued it
	// returns false immediately without touching the WebView.
	result := c.applyPendingThemeUpdate(context.TODO(), paneID, nil)
	assert.False(t, result)
}

// TestClearPendingAppearance tests that clearPendingAppearance removes both
// theme and script queues for the targeted pane only.
func TestClearPendingAppearance_ClearsBothQueuesForPane(t *testing.T) {
	t.Parallel()

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")
	c := &Coordinator{}

	c.queueThemeApply(pane1, true, "css")
	c.queueScriptRefresh(pane1)
	c.queueThemeApply(pane2, true, "css")
	c.queueScriptRefresh(pane2)

	c.clearPendingAppearance(pane1)

	// pane1 queues should be gone
	assert.False(t, c.takePendingScriptRefresh(pane1))
	_, themeOk1 := c.takePendingThemeApply(pane1)
	assert.False(t, themeOk1)

	// pane2 queues should still be present
	assert.True(t, c.takePendingScriptRefresh(pane2))
	_, themeOk2 := c.takePendingThemeApply(pane2)
	assert.True(t, themeOk2)
}
