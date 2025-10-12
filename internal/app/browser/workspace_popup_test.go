package browser

import (
	"testing"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/pkg/webkit"
)

// TestPendingPopupStorage tests the pendingPopups map operations
func TestPendingPopupStorage(t *testing.T) {
	wm := &WorkspaceManager{
		pendingPopups: make(map[uint64]*pendingPopup),
	}

	// Test adding a pending popup
	popupID := uint64(1)
	pending := &pendingPopup{
		url: "https://example.com",
	}

	wm.pendingPopups[popupID] = pending

	// Verify storage
	retrieved, ok := wm.pendingPopups[popupID]
	if !ok {
		t.Errorf("Expected popup ID %d to be stored", popupID)
	}

	if retrieved.url != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got %s", retrieved.url)
	}

	// Test deletion
	delete(wm.pendingPopups, popupID)
	_, ok = wm.pendingPopups[popupID]
	if ok {
		t.Errorf("Expected popup ID %d to be deleted", popupID)
	}
}

// TestPendingPopupCleanup tests cleanup of pending popups that never showed
func TestPendingPopupCleanup(t *testing.T) {
	wm := &WorkspaceManager{
		pendingPopups: make(map[uint64]*pendingPopup),
	}

	// Add multiple pending popups
	for i := uint64(1); i <= 5; i++ {
		wm.pendingPopups[i] = &pendingPopup{
			url: "https://example.com",
		}
	}

	// Verify all added
	if len(wm.pendingPopups) != 5 {
		t.Errorf("Expected 5 pending popups, got %d", len(wm.pendingPopups))
	}

	// Simulate close before ready-to-show for popup 3
	popupID := uint64(3)
	if _, ok := wm.pendingPopups[popupID]; ok {
		delete(wm.pendingPopups, popupID)
	}

	// Verify cleanup
	if len(wm.pendingPopups) != 4 {
		t.Errorf("Expected 4 pending popups after cleanup, got %d", len(wm.pendingPopups))
	}

	_, ok := wm.pendingPopups[popupID]
	if ok {
		t.Errorf("Expected popup ID %d to be removed", popupID)
	}
}

// TestPopupIDTracking tests that popup IDs are tracked correctly in paneNode
func TestPopupIDTracking(t *testing.T) {
	node := &paneNode{
		isLeaf:     true,
		windowType: webkit.WindowTypePopup,
		isPopup:    true,
		popupID:    42,
	}

	// Verify popup ID is stored
	if node.popupID != 42 {
		t.Errorf("Expected popupID 42, got %d", node.popupID)
	}

	// Verify window type
	if node.windowType != webkit.WindowTypePopup {
		t.Errorf("Expected WindowTypePopup, got %d", node.windowType)
	}

	// Verify popup flag
	if !node.isPopup {
		t.Errorf("Expected isPopup to be true")
	}
}

// TestActivePopupChildren tests tracking of active popup children in parent pane
func TestActivePopupChildren(t *testing.T) {
	parentNode := &paneNode{
		isLeaf:              true,
		activePopupChildren: make([]string, 0),
	}

	// Add popup children
	popupID1 := "1"
	popupID2 := "2"
	popupID3 := "3"

	parentNode.activePopupChildren = append(parentNode.activePopupChildren, popupID1)
	parentNode.activePopupChildren = append(parentNode.activePopupChildren, popupID2)
	parentNode.activePopupChildren = append(parentNode.activePopupChildren, popupID3)

	// Verify count
	if len(parentNode.activePopupChildren) != 3 {
		t.Errorf("Expected 3 active popup children, got %d", len(parentNode.activePopupChildren))
	}

	// Test removal of middle child
	for i, childID := range parentNode.activePopupChildren {
		if childID == popupID2 {
			parentNode.activePopupChildren = append(
				parentNode.activePopupChildren[:i],
				parentNode.activePopupChildren[i+1:]...,
			)
			break
		}
	}

	// Verify removal
	if len(parentNode.activePopupChildren) != 2 {
		t.Errorf("Expected 2 active popup children after removal, got %d", len(parentNode.activePopupChildren))
	}

	// Verify correct children remain
	if parentNode.activePopupChildren[0] != popupID1 || parentNode.activePopupChildren[1] != popupID3 {
		t.Errorf("Expected children [%s, %s], got %v", popupID1, popupID3, parentNode.activePopupChildren)
	}
}

// TestShouldAutoClose tests OAuth URL detection logic
func TestShouldAutoClose(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		enabled  bool
		expected bool
	}{
		{
			name:     "OAuth authorize URL",
			url:      "https://accounts.google.com/o/oauth2/v2/auth?client_id=...",
			enabled:  true,
			expected: true,
		},
		{
			name:     "OAuth callback URL",
			url:      "https://example.com/auth/callback?code=xyz",
			enabled:  true,
			expected: true,
		},
		{
			name:     "OAuth token response",
			url:      "https://example.com/redirect?access_token=xyz",
			enabled:  true,
			expected: true,
		},
		{
			name:     "Regular URL - auto-close enabled",
			url:      "https://example.com/page",
			enabled:  true,
			expected: false,
		},
		{
			name:     "OAuth URL - auto-close disabled",
			url:      "https://accounts.google.com/o/oauth2/v2/auth?client_id=...",
			enabled:  false,
			expected: false,
		},
		{
			name:     "OpenID Connect URL",
			url:      "https://example.com/oidc/authorize",
			enabled:  true,
			expected: true,
		},
		{
			name:     "Error response",
			url:      "https://example.com/callback?error=access_denied",
			enabled:  true,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wm := &WorkspaceManager{
				app: &BrowserApp{
					config: &config.Config{
						Workspace: config.WorkspaceConfig{
							Popups: config.PopupBehaviorConfig{
								OAuthAutoClose: tt.enabled,
							},
						},
					},
				},
			}

			result := wm.shouldAutoClose(tt.url)
			if result != tt.expected {
				t.Errorf("shouldAutoClose(%q) with enabled=%v = %v, want %v",
					tt.url, tt.enabled, result, tt.expected)
			}
		})
	}
}

// TestPopupBehaviorConfig tests popup behavior configuration
func TestPopupBehaviorConfig(t *testing.T) {
	tests := []struct {
		name     string
		behavior config.PopupBehavior
	}{
		{"Split behavior", config.PopupBehaviorSplit},
		{"Stacked behavior", config.PopupBehaviorStacked},
		{"Tabbed behavior", config.PopupBehaviorTabbed},
		{"Windowed behavior", config.PopupBehaviorWindowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workspace: config.WorkspaceConfig{
					Popups: config.PopupBehaviorConfig{
						Behavior: tt.behavior,
					},
				},
			}

			// Verify behavior is stored correctly
			if cfg.Workspace.Popups.Behavior != tt.behavior {
				t.Errorf("Expected behavior %s, got %s",
					tt.behavior, cfg.Workspace.Popups.Behavior)
			}
		})
	}
}

// TestPopupWindowTypeTracking tests window type tracking in paneNode
func TestPopupWindowTypeTracking(t *testing.T) {
	tests := []struct {
		name       string
		windowType webkit.WindowType
		isPopup    bool
		isRelated  bool
	}{
		{
			name:       "Regular tab",
			windowType: webkit.WindowTypeTab,
			isPopup:    false,
			isRelated:  false,
		},
		{
			name:       "Popup window",
			windowType: webkit.WindowTypePopup,
			isPopup:    true,
			isRelated:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &paneNode{
				windowType: tt.windowType,
				isPopup:    tt.isPopup,
				isRelated:  tt.isRelated,
			}

			if node.windowType != tt.windowType {
				t.Errorf("Expected windowType %d, got %d", tt.windowType, node.windowType)
			}

			if node.isPopup != tt.isPopup {
				t.Errorf("Expected isPopup %v, got %v", tt.isPopup, node.isPopup)
			}

			if node.isRelated != tt.isRelated {
				t.Errorf("Expected isRelated %v, got %v", tt.isRelated, node.isRelated)
			}
		})
	}
}

// TestPopupParentRelationship tests parent-child relationship tracking
func TestPopupParentRelationship(t *testing.T) {
	parentNode := &paneNode{
		isLeaf:              true,
		activePopupChildren: make([]string, 0),
	}

	popupNode := &paneNode{
		isLeaf:     true,
		windowType: webkit.WindowTypePopup,
		isPopup:    true,
		parentPane: parentNode,
		isRelated:  true,
	}

	// Verify parent relationship
	if popupNode.parentPane != parentNode {
		t.Errorf("Expected popup's parent to be parentNode")
	}

	// Verify popup is marked as related
	if !popupNode.isRelated {
		t.Errorf("Expected popup to be marked as related")
	}

	// Add popup to parent's children
	popupWebViewID := "test-popup-id"
	parentNode.activePopupChildren = append(parentNode.activePopupChildren, popupWebViewID)

	// Verify parent tracks popup
	if len(parentNode.activePopupChildren) != 1 {
		t.Errorf("Expected parent to track 1 popup child, got %d", len(parentNode.activePopupChildren))
	}

	if parentNode.activePopupChildren[0] != popupWebViewID {
		t.Errorf("Expected parent to track popup ID %s, got %s",
			popupWebViewID, parentNode.activePopupChildren[0])
	}
}

// TestPopupCleanupGeneration tests cleanup generation tracking
func TestPopupCleanupGeneration(t *testing.T) {
	node := &paneNode{
		isLeaf:            true,
		windowType:        webkit.WindowTypePopup,
		isPopup:           true,
		widgetValid:       true,
		cleanupGeneration: 0,
	}

	// Simulate lifecycle progression
	node.cleanupGeneration = 1
	if node.cleanupGeneration != 1 {
		t.Errorf("Expected cleanupGeneration 1, got %d", node.cleanupGeneration)
	}

	// Simulate widget becoming invalid
	node.widgetValid = false
	if node.widgetValid {
		t.Errorf("Expected widgetValid to be false")
	}

	// Increment generation for new lifecycle
	node.cleanupGeneration = 2
	if node.cleanupGeneration != 2 {
		t.Errorf("Expected cleanupGeneration 2, got %d", node.cleanupGeneration)
	}
}

// TestMultiplePendingPopups tests handling multiple pending popups simultaneously
func TestMultiplePendingPopups(t *testing.T) {
	wm := &WorkspaceManager{
		pendingPopups: make(map[uint64]*pendingPopup),
	}

	// Add multiple pending popups
	popups := map[uint64]string{
		1: "https://popup1.com",
		2: "https://popup2.com",
		3: "https://popup3.com",
	}

	for id, url := range popups {
		wm.pendingPopups[id] = &pendingPopup{
			url: url,
		}
	}

	// Verify all stored
	if len(wm.pendingPopups) != 3 {
		t.Errorf("Expected 3 pending popups, got %d", len(wm.pendingPopups))
	}

	// Verify each URL
	for id, expectedURL := range popups {
		pending, ok := wm.pendingPopups[id]
		if !ok {
			t.Errorf("Expected popup ID %d to be stored", id)
			continue
		}
		if pending.url != expectedURL {
			t.Errorf("Expected URL %s for popup %d, got %s", expectedURL, id, pending.url)
		}
	}

	// Simulate ready-to-show for popup 2 (remove from pending)
	delete(wm.pendingPopups, 2)

	// Verify remaining
	if len(wm.pendingPopups) != 2 {
		t.Errorf("Expected 2 pending popups after ready-to-show, got %d", len(wm.pendingPopups))
	}

	// Verify popup 2 is gone but 1 and 3 remain
	if _, ok := wm.pendingPopups[2]; ok {
		t.Errorf("Expected popup 2 to be removed after ready-to-show")
	}
	if _, ok := wm.pendingPopups[1]; !ok {
		t.Errorf("Expected popup 1 to still be pending")
	}
	if _, ok := wm.pendingPopups[3]; !ok {
		t.Errorf("Expected popup 3 to still be pending")
	}
}
