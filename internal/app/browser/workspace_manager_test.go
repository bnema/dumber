package browser

import (
	"testing"

	"github.com/bnema/dumber/pkg/webkit"
)

func newTestWorkspaceManager(t *testing.T) *WorkspaceManager {
	t.Helper()
	webkit.ResetWidgetStubsForTesting()

	rootView, err := webkit.NewWebView(&webkit.Config{})
	if err != nil {
		t.Fatalf("failed to create root webview: %v", err)
	}

	rootPane := &BrowserPane{webView: rootView}

	app := &BrowserApp{}
	app.panes = []*BrowserPane{rootPane}
	app.activePane = rootPane
	app.webView = rootView

	wm := NewWorkspaceManager(app, rootPane)
	wm.createWebViewFn = func() (*webkit.WebView, error) {
		return webkit.NewWebView(&webkit.Config{})
	}
	wm.createPaneFn = func(view *webkit.WebView) (*BrowserPane, error) {
		return &BrowserPane{webView: view}, nil
	}

	return wm
}

func TestSplitNodeCreatesExpectedTree(t *testing.T) {
	cases := []struct {
		name              string
		direction         string
		expectOrientation webkit.Orientation
		existingIsStart   bool
	}{
		{name: "Right", direction: "right", expectOrientation: webkit.OrientationHorizontal, existingIsStart: true},
		{name: "Left", direction: "left", expectOrientation: webkit.OrientationHorizontal, existingIsStart: false},
		{name: "Up", direction: "up", expectOrientation: webkit.OrientationVertical, existingIsStart: false},
		{name: "Down", direction: "down", expectOrientation: webkit.OrientationVertical, existingIsStart: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wm := newTestWorkspaceManager(t)
			original := wm.active

			newLeaf, err := wm.splitNode(original, tc.direction)
			if err != nil {
				t.Fatalf("splitNode(%s) returned error: %v", tc.direction, err)
			}

			parent := newLeaf.parent
			if parent == nil {
				t.Fatalf("expected new leaf to have parent")
			}

			if parent.orientation != tc.expectOrientation {
				t.Fatalf("expected orientation %v got %v", tc.expectOrientation, parent.orientation)
			}

			if tc.existingIsStart {
				if parent.left != original {
					t.Fatalf("expected original pane to remain as start child")
				}
				if parent.right != newLeaf {
					t.Fatalf("expected new pane to be end child")
				}
			} else {
				if parent.left != newLeaf {
					t.Fatalf("expected new pane to be start child")
				}
				if parent.right != original {
					t.Fatalf("expected original pane to be end child")
				}
			}

			if wm.root != parent {
				t.Fatalf("expected parent to become new root")
			}

			if len(wm.app.panes) != 2 {
				t.Fatalf("expected 2 panes registered, got %d", len(wm.app.panes))
			}

			mapped := wm.viewToNode[newLeaf.pane.webView]
			if mapped != newLeaf {
				t.Fatalf("expected new webview to map to new leaf")
			}
		})
	}
}

func TestClosePanePromotesSibling(t *testing.T) {
	wm := newTestWorkspaceManager(t)
	original := wm.active

	newLeaf, err := wm.splitNode(original, "right")
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if err := wm.closePane(newLeaf); err != nil {
		t.Fatalf("closePane failed: %v", err)
	}

	if wm.root != original {
		t.Fatalf("expected original leaf to become root after closing sibling")
	}

	if original.parent != nil {
		t.Fatalf("expected original leaf to have no parent after promotion")
	}

	if len(wm.app.panes) != 1 {
		t.Fatalf("expected single pane remaining, got %d", len(wm.app.panes))
	}

	if wm.app.panes[0] != original.pane {
		t.Fatalf("expected remaining pane to match original")
	}

	if wm.active != original {
		t.Fatalf("expected focus to move to promoted pane")
	}
}

func TestFocusNeighborWithBounds(t *testing.T) {
	wm := newTestWorkspaceManager(t)
	left := wm.active

	right, err := wm.splitNode(left, "right")
	if err != nil {
		t.Fatalf("split failed: %v", err)
	}

	if right == nil {
		t.Fatalf("expected new right leaf")
	}

	// Provide geometry hints for focus calculations.
	webkit.SetWidgetBoundsForTesting(left.container, webkit.WidgetBounds{X: 0, Y: 0, Width: 100, Height: 100})
	webkit.SetWidgetBoundsForTesting(right.container, webkit.WidgetBounds{X: 120, Y: 0, Width: 100, Height: 100})

	if !wm.FocusNeighbor("left") {
		t.Fatalf("expected focus neighbor left to succeed")
	}

	if wm.active != left {
		t.Fatalf("expected focus to move to left pane")
	}

	// Vertical split from the left pane to test up/down logic.
	bottom, err := wm.splitNode(left, "down")
	if err != nil {
		t.Fatalf("vertical split failed: %v", err)
	}

	top := bottom.parent.left

	webkit.SetWidgetBoundsForTesting(top.container, webkit.WidgetBounds{X: 0, Y: 0, Width: 100, Height: 90})
	webkit.SetWidgetBoundsForTesting(bottom.container, webkit.WidgetBounds{X: 0, Y: 120, Width: 100, Height: 90})

	// Active pane should be bottom after split. Move focus up.
	if wm.active != bottom {
		t.Fatalf("expected bottom pane to be active after split")
	}

	if !wm.FocusNeighbor("up") {
		t.Fatalf("expected focus neighbor up to succeed")
	}

	if wm.active != top {
		t.Fatalf("expected focus to move to top pane")
	}
}

func TestFocusNeighborWithoutPeer(t *testing.T) {
	wm := newTestWorkspaceManager(t)
	if wm.FocusNeighbor("left") {
		t.Fatalf("expected focus move to fail when only one pane exists")
	}
}
