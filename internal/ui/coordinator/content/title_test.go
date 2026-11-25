package content

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/ui/component"
)

func TestOnTitleChanged_DoesNotEmitWindowCallbackForBackgroundPane(t *testing.T) {
	t.Parallel()

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")
	root := &entity.PaneNode{ID: "root"}
	leaf1 := &entity.PaneNode{ID: string(pane1), Pane: entity.NewPane(pane1), Parent: root}
	leaf2 := &entity.PaneNode{ID: string(pane2), Pane: entity.NewPane(pane2), Parent: root}
	root.Children = []*entity.PaneNode{leaf1, leaf2}

	ws := &entity.Workspace{Root: root, ActivePaneID: pane1}
	var gotPaneID entity.PaneID
	var gotTitle string
	c := &Coordinator{
		paneTitles:  make(map[entity.PaneID]string),
		getActiveWS: func() (*entity.Workspace, *component.WorkspaceView) { return ws, nil },
		onWindowTitleChanged: func(paneID entity.PaneID, title string) {
			gotPaneID = paneID
			gotTitle = title
		},
	}

	c.onTitleChanged(context.Background(), pane2, "Pane Two")

	assert.Empty(t, gotPaneID)
	assert.Empty(t, gotTitle)
	assert.Equal(t, "Pane Two", c.GetTitle(pane2))
}

func TestOnTitleChanged_EmitsWindowCallbackForActivePane(t *testing.T) {
	t.Parallel()

	pane := entity.PaneID("pane-1")
	root := &entity.PaneNode{ID: "root"}
	leaf := &entity.PaneNode{ID: string(pane), Pane: entity.NewPane(pane), Parent: root}
	root.Children = []*entity.PaneNode{leaf}

	ws := &entity.Workspace{Root: root, ActivePaneID: pane}
	var gotPaneID entity.PaneID
	var gotTitle string
	c := &Coordinator{
		paneTitles:  make(map[entity.PaneID]string),
		getActiveWS: func() (*entity.Workspace, *component.WorkspaceView) { return ws, nil },
		onWindowTitleChanged: func(paneID entity.PaneID, title string) {
			gotPaneID = paneID
			gotTitle = title
		},
	}

	c.onTitleChanged(context.Background(), pane, "Pane One")

	assert.Equal(t, pane, gotPaneID)
	assert.Equal(t, "Pane One", gotTitle)
	assert.Equal(t, "Pane One", c.GetTitle(pane))
}

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
	for range 50 {
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
	for range 50 {
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

// TestClearPendingAppearance tests that clearPendingAppearance removes both
// system view and script queues for the targeted pane only.
func TestClearPendingAppearance_ClearsBothQueuesForPane(t *testing.T) {
	t.Parallel()

	pane1 := entity.PaneID("pane-1")
	pane2 := entity.PaneID("pane-2")
	c := &Coordinator{}

	c.markSystemViewPending(pane1)
	c.queueScriptRefresh(pane1)
	c.markSystemViewPending(pane2)
	c.queueScriptRefresh(pane2)

	c.clearPendingAppearance(pane1)

	// pane1 queues should be gone
	assert.False(t, c.takePendingScriptRefresh(pane1))
	assert.False(t, c.takeSystemViewPending(pane1))

	// pane2 queues should still be present
	assert.True(t, c.takePendingScriptRefresh(pane2))
	assert.True(t, c.takeSystemViewPending(pane2))
}
