package browser

import (
	"testing"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

func BenchmarkSplitNode(b *testing.B) {
	wm := newTestWorkspaceManagerWithMocks(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Reset to single node for each iteration
		wm = newTestWorkspaceManagerWithMocks(b)
		b.StartTimer()

		_, err := wm.splitNode(wm.root, "right")
		if err != nil {
			b.Fatalf("Split failed: %v", err)
		}
	}
}

func BenchmarkClosePane(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		wm := newTestWorkspaceManagerWithMocks(b)
		node, err := wm.splitNode(wm.root, "right")
		if err != nil {
			b.Fatalf("Setup split failed: %v", err)
		}
		b.StartTimer()

		err = wm.closePane(node)
		if err != nil {
			b.Fatalf("Close failed: %v", err)
		}
	}
}

func BenchmarkTreeTraversal(b *testing.B) {
	wm := newTestWorkspaceManagerWithMocks(b)

	// Build a complex tree with 15 leaves
	leaves := []*paneNode{wm.root}
	for i := 0; i < 14; i++ {
		if len(leaves) > 0 {
			// Split the first leaf
			target := leaves[0]
			leaves = leaves[1:] // Remove the target from leaves

			newNode, err := wm.splitNode(target, "right")
			if err != nil {
				b.Fatalf("Setup split %d failed: %v", i, err)
			}

			// Add both resulting leaves back
			leaves = append(leaves, target, newNode)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.collectLeaves()
	}
}

func BenchmarkCollectLeavesFrom(b *testing.B) {
	wm := newTestWorkspaceManagerWithMocks(b)

	// Build a balanced binary tree
	for i := 0; i < 10; i++ {
		leaves := wm.collectLeaves()
		if len(leaves) > 0 {
			_, err := wm.splitNode(leaves[0], "right")
			if err != nil {
				b.Fatalf("Setup split %d failed: %v", i, err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.collectLeavesFrom(wm.root)
	}
}

func BenchmarkLeftmostLeaf(b *testing.B) {
	wm := newTestWorkspaceManagerWithMocks(b)

	// Create a deep left-leaning tree
	current := wm.root
	for i := 0; i < 20; i++ {
		newNode, err := wm.splitNode(current, "left")
		if err != nil {
			b.Fatalf("Setup split %d failed: %v", i, err)
		}
		current = newNode
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.leftmostLeaf(wm.root)
	}
}

func BenchmarkFindReplacementRoot(b *testing.B) {
	wm := newTestWorkspaceManagerWithMocks(b)

	// Build a complex tree
	leaves := wm.collectLeaves()
	for i := 0; i < 15; i++ {
		if len(leaves) > 0 {
			target := leaves[i%len(leaves)]
			newNode, err := wm.splitNode(target, "right")
			if err != nil {
				continue
			}
			leaves = append(leaves, newNode)
		}
	}

	excludeNode := wm.root

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.findReplacementRoot(excludeNode)
	}
}

func BenchmarkFocusAdjacent(b *testing.B) {
	wm := newTestWorkspaceManagerWithMocks(b)

	// Create a 2x2 grid of panes with known geometry
	right, _ := wm.splitNode(wm.root, "right")
	bottom, _ := wm.splitNode(wm.root, "down")
	_, _ = wm.splitNode(right, "down")

	// Set up widget bounds for focus calculations
	webkit.SetWidgetBoundsForTesting(wm.root.container, webkit.WidgetBounds{X: 0, Y: 0, Width: 100, Height: 100})
	webkit.SetWidgetBoundsForTesting(right.container, webkit.WidgetBounds{X: 120, Y: 0, Width: 100, Height: 100})
	webkit.SetWidgetBoundsForTesting(bottom.container, webkit.WidgetBounds{X: 0, Y: 120, Width: 100, Height: 100})

	wm.active = wm.root

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.focusAdjacent("right")
		wm.focusAdjacent("left")
	}
}

func BenchmarkStructuralNeighbor(b *testing.B) {
	wm := newTestWorkspaceManagerWithMocks(b)

	// Create a complex nested structure
	right, _ := wm.splitNode(wm.root, "right")
	_, _ = wm.splitNode(right, "down")
	_, _ = wm.splitNode(wm.root, "down")

	// Set up geometry
	webkit.SetWidgetBoundsForTesting(wm.root.container, webkit.WidgetBounds{X: 0, Y: 0, Width: 100, Height: 100})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.structuralNeighbor(wm.root, "right")
		wm.structuralNeighbor(wm.root, "down")
	}
}

func BenchmarkClosestLeafFromSubtree(b *testing.B) {
	wm := newTestWorkspaceManagerWithMocks(b)

	// Build a subtree with multiple leaves
	right, _ := wm.splitNode(wm.root, "right")
	_, _ = wm.splitNode(right, "down")
	_, _ = wm.splitNode(right, "up")

	// Set up geometry for all nodes
	leaves := wm.collectLeavesFrom(right)
	for i, leaf := range leaves {
		webkit.SetWidgetBoundsForTesting(leaf.container, webkit.WidgetBounds{
			X: float64(i * 50), Y: 0, Width: 45, Height: 100,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wm.closestLeafFromSubtree(right, 25, 50, "right")
	}
}

func BenchmarkComplexTreeOperations(b *testing.B) {
	b.Run("SplitAndClose", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			wm := newTestWorkspaceManagerWithMocks(b)
			b.StartTimer()

			// Split multiple times then close in reverse order
			nodes := []*paneNode{wm.root}
			for j := 0; j < 5; j++ {
				newNode, err := wm.splitNode(nodes[len(nodes)-1], "right")
				if err != nil {
					b.Fatalf("Split failed: %v", err)
				}
				nodes = append(nodes, newNode)
			}

			// Close all but root
			for j := len(nodes) - 1; j > 0; j-- {
				err := wm.closePane(nodes[j])
				if err != nil {
					b.Fatalf("Close failed: %v", err)
				}
			}
		}
	})

	b.Run("BuildBalancedTree", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			wm := newTestWorkspaceManagerWithMocks(b)
			b.StartTimer()

			// Build a balanced tree by alternating split directions
			leaves := []*paneNode{wm.root}
			directions := []string{"right", "down", "left", "up"}

			for j := 0; j < 15 && len(leaves) > 0; j++ {
				target := leaves[j%len(leaves)]
				direction := directions[j%len(directions)]

				newNode, err := wm.splitNode(target, direction)
				if err != nil {
					continue
				}
				leaves = append(leaves, newNode)
			}
		}
	})
}

func BenchmarkMemoryOperations(b *testing.B) {
	b.Run("ViewToNodeMapping", func(b *testing.B) {
		wm := newTestWorkspaceManagerWithMocks(b)

		// Create many nodes to stress the map
		for i := 0; i < 100; i++ {
			leaves := wm.collectLeaves()
			if len(leaves) > 0 {
				_, _ = wm.splitNode(leaves[0], "right")
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Access all mappings
			for view, node := range wm.viewToNode {
				_ = view
				_ = node
			}
		}
	})

	b.Run("TreeWalkMemory", func(b *testing.B) {
		wm := newTestWorkspaceManagerWithMocks(b)

		// Build deep tree
		current := wm.root
		for i := 0; i < 50; i++ {
			newNode, err := wm.splitNode(current, "right")
			if err != nil {
				break
			}
			current = newNode
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Multiple tree operations that allocate
			leaves := wm.collectLeaves()
			_ = leaves
			countNodes(wm.root)
			wm.leftmostLeaf(wm.root)
		}
	})
}

// Helper function for benchmarks
func newTestWorkspaceManagerWithMocks(tb testing.TB) *WorkspaceManager {
	tb.Helper()
	webkit.ResetWidgetStubsForTesting()

	rootView, err := webkit.NewWebView(&webkit.Config{})
	if err != nil {
		tb.Fatalf("failed to create root webview: %v", err)
	}

	rootPane := &BrowserPane{
		webView: rootView,
		id:      "root",
	}

	app := &BrowserApp{
		panes:      []*BrowserPane{rootPane},
		activePane: rootPane,
		webView:    rootView,
	}

	wm := &WorkspaceManager{
		app:          app,
		window:       rootView.Window(),
		viewToNode:   make(map[*webkit.WebView]*paneNode),
		lastSplitMsg: make(map[*webkit.WebView]time.Time),
		lastExitMsg:  make(map[*webkit.WebView]time.Time),
	}

	wm.createWebViewFn = func() (*webkit.WebView, error) {
		return webkit.NewWebView(&webkit.Config{})
	}

	wm.createPaneFn = func(view *webkit.WebView) (*BrowserPane, error) {
		return &BrowserPane{
			webView: view,
			id:      "bench-pane",
		}, nil
	}

	root := &paneNode{
		pane:      rootPane,
		container: rootView.RootWidget(),
		isLeaf:    true,
	}
	wm.root = root
	wm.active = root
	wm.mainPane = root
	wm.viewToNode[rootView] = root

	return wm
}
