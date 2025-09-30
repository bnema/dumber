// workspace_state_tombstone.go - State tombstones for rollback capability inspired by Zellij
package browser

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bnema/dumber/pkg/webkit"
)

// StateTombstone captures the complete state of the workspace at a point in time
type StateTombstone struct {
	ID          string
	Timestamp   time.Time
	Operation   string
	TreeState   *TombstoneTreeSnapshot
	WidgetState *WidgetSnapshot
	FocusState  *FocusSnapshot
	StackState  *StackSnapshot
	Compressed  bool
	Size        int
}

// TombstoneTreeSnapshot captures the tree structure for state tombstones
type TombstoneTreeSnapshot struct {
	RootID      string
	MainPaneID  string
	Nodes       map[string]*NodeSnapshot
	ParentLinks map[string]string    // child_id -> parent_id
	ChildLinks  map[string][2]string // parent_id -> [left_id, right_id]
	NodeOrder   []string             // Traversal order for reconstruction
}

// NodeSnapshot captures the state of a single node
type NodeSnapshot struct {
	ID          string
	IsLeaf      bool
	IsStacked   bool
	IsPopup     bool
	WindowType  int // webkit.WindowType
	Orientation int // webkit.Orientation
	HasPane     bool
	PaneID      string
	StackInfo   *StackInfo
}

// StackInfo captures stack-specific state
type StackInfo struct {
	StackedPaneIDs   []string
	ActiveStackIndex int
}

// WidgetSnapshot captures widget state information
type WidgetSnapshot struct {
	Widgets     map[string]*WidgetInfo
	Allocations map[string]*AllocationInfo
	CSSClasses  map[string][]string
	Visibility  map[string]bool
	ParentChild map[string]string // widget_id -> parent_id
}

// WidgetInfo captures widget information
type WidgetInfo struct {
	ID      string
	Type    string
	Pointer uintptr
	IsValid bool
}

// AllocationInfo captures widget allocation
type AllocationInfo struct {
	X      int
	Y      int
	Width  int
	Height int
}

// FocusSnapshot captures focus state
type FocusSnapshot struct {
	ActiveNodeID  string
	LastFocusTime time.Time
	FocusSource   int // FocusSource
	HoverTokens   map[string]uintptr
}

// StackSnapshot captures all stack states
type StackSnapshot struct {
	StackContainers map[string]*StackContainerInfo
}

// StackContainerInfo captures stack container state
type StackContainerInfo struct {
	NodeID         string
	StackedPaneIDs []string
	ActiveIndex    int
	TitleBars      map[string]string // pane_id -> title
	Visibility     map[string]bool   // pane_id -> visible
}

var gobRegisterOnce sync.Once

func registerTombstoneGobTypes() {
	gobRegisterOnce.Do(func() {
		gob.Register(&TombstoneTreeSnapshot{})
		gob.Register(&NodeSnapshot{})
		gob.Register(&WidgetSnapshot{})
		gob.Register(&FocusSnapshot{})
		gob.Register(&StackSnapshot{})
		gob.Register(&StackInfo{})
		gob.Register(&WidgetInfo{})
		gob.Register(&AllocationInfo{})
		gob.Register(&StackContainerInfo{})
	})
}

// StateTombstoneManager manages state tombstones for rollback operations
type StateTombstoneManager struct {
	tombstones    map[string]*StateTombstone
	history       []*StateTombstone
	maxTombstones int
	maxHistory    int
	compress      bool
	mu            sync.RWMutex
	wm            *WorkspaceManager
}

// NewStateTombstoneManager creates a new state tombstone manager
func NewStateTombstoneManager(wm *WorkspaceManager) *StateTombstoneManager {
	// Ensure concrete tombstone types are available for gob encoding/decoding.
	registerTombstoneGobTypes()

	return &StateTombstoneManager{
		tombstones:    make(map[string]*StateTombstone),
		history:       make([]*StateTombstone, 0, 50),
		maxTombstones: 20,
		maxHistory:    50,
		compress:      true,
		wm:            wm,
	}
}

// CaptureState captures the current workspace state as a tombstone
func (stm *StateTombstoneManager) CaptureState(operation string) (*StateTombstone, error) {
	stm.mu.Lock()
	defer stm.mu.Unlock()

	tombstoneID := fmt.Sprintf("%s_%d", operation, time.Now().UnixNano())

	log.Printf("[tombstone] Capturing state for operation: %s", operation)

	// Capture tree state
	treeState, err := stm.captureTreeState()
	if err != nil {
		return nil, fmt.Errorf("failed to capture tree state: %w", err)
	}

	// Capture widget state
	widgetState, err := stm.captureWidgetState()
	if err != nil {
		return nil, fmt.Errorf("failed to capture widget state: %w", err)
	}

	// Capture focus state
	focusState := stm.captureFocusState()

	// Capture stack state
	stackState := stm.captureStackState()

	tombstone := &StateTombstone{
		ID:          tombstoneID,
		Timestamp:   time.Now(),
		Operation:   operation,
		TreeState:   treeState,
		WidgetState: widgetState,
		FocusState:  focusState,
		StackState:  stackState,
		Compressed:  false,
	}

	// Compress if enabled
	if stm.compress {
		if err := stm.compressTombstone(tombstone); err != nil {
			log.Printf("[tombstone] Failed to compress tombstone: %v", err)
		}
	}

	// Calculate size
	tombstone.Size = stm.calculateTombstoneSize(tombstone)

	// Store tombstone
	stm.tombstones[tombstoneID] = tombstone

	// Add to history
	stm.history = append(stm.history, tombstone)

	// Cleanup old tombstones
	stm.cleanupOldTombstones()

	log.Printf("[tombstone] Captured state: id=%s, size=%d bytes", tombstoneID, tombstone.Size)

	return tombstone, nil
}

// captureTreeState captures the current tree structure
func (stm *StateTombstoneManager) captureTreeState() (*TombstoneTreeSnapshot, error) {
	snapshot := &TombstoneTreeSnapshot{
		Nodes:       make(map[string]*NodeSnapshot),
		ParentLinks: make(map[string]string),
		ChildLinks:  make(map[string][2]string),
		NodeOrder:   make([]string, 0),
	}

	if stm.wm.root != nil {
		snapshot.RootID = stm.nodeID(stm.wm.root)
	}

	if stm.wm.mainPane != nil {
		snapshot.MainPaneID = stm.nodeID(stm.wm.mainPane)
	}

	// Traverse tree and capture all nodes
	if stm.wm.root != nil {
		stm.captureNodeRecursive(stm.wm.root, snapshot)
	}

	return snapshot, nil
}

// captureNodeRecursive recursively captures node information
func (stm *StateTombstoneManager) captureNodeRecursive(node *paneNode, snapshot *TombstoneTreeSnapshot) {
	if node == nil {
		return
	}

	nodeID := stm.nodeID(node)
	snapshot.NodeOrder = append(snapshot.NodeOrder, nodeID)

	// Capture node information
	nodeSnap := &NodeSnapshot{
		ID:          nodeID,
		IsLeaf:      node.isLeaf,
		IsStacked:   node.isStacked,
		IsPopup:     node.isPopup,
		WindowType:  int(node.windowType),
		Orientation: int(node.orientation),
	}

	// Capture pane information
	if node.pane != nil {
		nodeSnap.HasPane = true
		nodeSnap.PaneID = stm.paneID(node.pane)
	}

	// Capture stack information
	if node.isStacked {
		nodeSnap.StackInfo = &StackInfo{
			StackedPaneIDs:   make([]string, len(node.stackedPanes)),
			ActiveStackIndex: node.activeStackIndex,
		}

		for i, stackedPane := range node.stackedPanes {
			if stackedPane != nil {
				nodeSnap.StackInfo.StackedPaneIDs[i] = stm.nodeID(stackedPane)
			}
		}
	}

	snapshot.Nodes[nodeID] = nodeSnap

	// Capture parent-child relationships
	if node.parent != nil {
		snapshot.ParentLinks[nodeID] = stm.nodeID(node.parent)
	}

	if node.left != nil || node.right != nil {
		var leftID, rightID string
		if node.left != nil {
			leftID = stm.nodeID(node.left)
		}
		if node.right != nil {
			rightID = stm.nodeID(node.right)
		}
		snapshot.ChildLinks[nodeID] = [2]string{leftID, rightID}
	}

	// Recurse for children
	stm.captureNodeRecursive(node.left, snapshot)
	stm.captureNodeRecursive(node.right, snapshot)

	// Recurse for stacked panes
	if node.isStacked {
		for _, stackedPane := range node.stackedPanes {
			stm.captureNodeRecursive(stackedPane, snapshot)
		}
	}
}

// captureWidgetState captures widget state information
func (stm *StateTombstoneManager) captureWidgetState() (*WidgetSnapshot, error) {
	snapshot := &WidgetSnapshot{
		Widgets:     make(map[string]*WidgetInfo),
		Allocations: make(map[string]*AllocationInfo),
		CSSClasses:  make(map[string][]string),
		Visibility:  make(map[string]bool),
		ParentChild: make(map[string]string),
	}

	// Capture widget information from all nodes
	if stm.wm.root != nil {
		stm.captureWidgetRecursive(stm.wm.root, snapshot)
	}

	return snapshot, nil
}

// captureWidgetRecursive recursively captures widget information
func (stm *StateTombstoneManager) captureWidgetRecursive(node *paneNode, snapshot *WidgetSnapshot) {
	if node == nil {
		return
	}

	nodeID := stm.nodeID(node)

	// Capture container widget
	if node.container != nil {
		widgetID := fmt.Sprintf("%s_container", nodeID)
		snapshot.Widgets[widgetID] = &WidgetInfo{
			ID:      widgetID,
			Type:    "container",
			Pointer: node.container.Ptr(),
			IsValid: node.container.IsValid(),
		}

		// Capture allocation and visibility
		node.container.Execute(func(ptr uintptr) error {
			allocation := webkit.WidgetGetAllocation(ptr)
			snapshot.Allocations[widgetID] = &AllocationInfo{
				X:      allocation.X,
				Y:      allocation.Y,
				Width:  allocation.Width,
				Height: allocation.Height,
			}

			snapshot.Visibility[widgetID] = webkit.WidgetGetVisible(ptr)

			// Capture CSS classes
			cssClasses := stm.extractCSSClasses(ptr)
			if len(cssClasses) > 0 {
				snapshot.CSSClasses[widgetID] = cssClasses
			}

			return nil
		})
	}

	// Capture title bar widget
	if node.titleBar != nil {
		widgetID := fmt.Sprintf("%s_titlebar", nodeID)
		snapshot.Widgets[widgetID] = &WidgetInfo{
			ID:      widgetID,
			Type:    "titlebar",
			Pointer: node.titleBar.Ptr(),
			IsValid: node.titleBar.IsValid(),
		}

		node.titleBar.Execute(func(ptr uintptr) error {
			snapshot.Visibility[widgetID] = webkit.WidgetGetVisible(ptr)
			return nil
		})
	}

	// Capture stack wrapper widget
	if node.stackWrapper != nil {
		widgetID := fmt.Sprintf("%s_stackwrapper", nodeID)
		snapshot.Widgets[widgetID] = &WidgetInfo{
			ID:      widgetID,
			Type:    "stackwrapper",
			Pointer: node.stackWrapper.Ptr(),
			IsValid: node.stackWrapper.IsValid(),
		}
	}

	// Recurse for children
	stm.captureWidgetRecursive(node.left, snapshot)
	stm.captureWidgetRecursive(node.right, snapshot)

	// Recurse for stacked panes
	if node.isStacked {
		for _, stackedPane := range node.stackedPanes {
			stm.captureWidgetRecursive(stackedPane, snapshot)
		}
	}
}

// captureFocusState captures the current focus state
func (stm *StateTombstoneManager) captureFocusState() *FocusSnapshot {
	snapshot := &FocusSnapshot{
		HoverTokens: make(map[string]uintptr),
	}

	// Capture active node
	if activeNode := stm.wm.GetActiveNode(); activeNode != nil {
		snapshot.ActiveNodeID = stm.nodeID(activeNode)
	}

	// Capture focus timing information
	snapshot.LastFocusTime = stm.wm.lastFocusTime
	if stm.wm.lastFocusTarget != nil {
		// Additional focus target info could be captured here
	}

	// Capture hover tokens (simplified - in practice you'd need access to hover token map)
	// This would require extending the workspace manager to expose hover tokens

	return snapshot
}

// captureStackState captures all stack container states
func (stm *StateTombstoneManager) captureStackState() *StackSnapshot {
	snapshot := &StackSnapshot{
		StackContainers: make(map[string]*StackContainerInfo),
	}

	// Find all stacked nodes and capture their state
	if stm.wm.root != nil {
		stm.captureStackRecursive(stm.wm.root, snapshot)
	}

	return snapshot
}

// captureStackRecursive recursively captures stack states
func (stm *StateTombstoneManager) captureStackRecursive(node *paneNode, snapshot *StackSnapshot) {
	if node == nil {
		return
	}

	if node.isStacked {
		nodeID := stm.nodeID(node)
		stackInfo := &StackContainerInfo{
			NodeID:         nodeID,
			StackedPaneIDs: make([]string, len(node.stackedPanes)),
			ActiveIndex:    node.activeStackIndex,
			TitleBars:      make(map[string]string),
			Visibility:     make(map[string]bool),
		}

		// Capture stacked pane IDs
		for i, stackedPane := range node.stackedPanes {
			if stackedPane != nil {
				paneID := stm.nodeID(stackedPane)
				stackInfo.StackedPaneIDs[i] = paneID

				// Capture title and visibility
				if stackedPane.pane != nil && stackedPane.pane.webView != nil {
					title := stackedPane.pane.webView.GetTitle()
					stackInfo.TitleBars[paneID] = title
				}

				if stackedPane.container != nil {
					stackedPane.container.Execute(func(ptr uintptr) error {
						stackInfo.Visibility[paneID] = webkit.WidgetGetVisible(ptr)
						return nil
					})
				}
			}
		}

		snapshot.StackContainers[nodeID] = stackInfo
	}

	// Recurse for children
	stm.captureStackRecursive(node.left, snapshot)
	stm.captureStackRecursive(node.right, snapshot)

	// Recurse for stacked panes
	if node.isStacked {
		for _, stackedPane := range node.stackedPanes {
			stm.captureStackRecursive(stackedPane, snapshot)
		}
	}
}

// RestoreState restores the workspace to a previously captured state
func (stm *StateTombstoneManager) RestoreState(tombstoneID string) error {
	stm.mu.RLock()
	tombstone, exists := stm.tombstones[tombstoneID]
	stm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("tombstone %s not found", tombstoneID)
	}

	log.Printf("[tombstone] Restoring state from tombstone: %s (operation: %s)",
		tombstoneID, tombstone.Operation)

	// Decompress if needed
	if tombstone.Compressed {
		if err := stm.decompressTombstone(tombstone); err != nil {
			return fmt.Errorf("failed to decompress tombstone: %w", err)
		}
	}

	// This is a complex operation that would require:
	// 1. Destroying current tree structure
	// 2. Recreating nodes from snapshot
	// 3. Restoring widget hierarchy
	// 4. Restoring focus state
	// 5. Validating the restored state

	// For now, we'll implement a basic restoration framework
	return stm.performRestore(tombstone)
}

// performRestore performs the actual state restoration
func (stm *StateTombstoneManager) performRestore(tombstone *StateTombstone) error {
	// This is a placeholder for the complex restoration logic
	// In a full implementation, this would:

	// 1. Create a transaction for the entire restoration
	// 2. Clear current state while preserving essential references
	// 3. Recreate the tree structure from the snapshot
	// 4. Restore widget hierarchy and properties
	// 5. Restore focus and stack states
	// 6. Validate the restored state

	log.Printf("[tombstone] Restoration framework executed for tombstone %s", tombstone.ID)
	return fmt.Errorf("state restoration not yet fully implemented")
}

// Helper methods

// nodeID generates a unique ID for a pane node
func (stm *StateTombstoneManager) nodeID(node *paneNode) string {
	if node == nil {
		return ""
	}
	return fmt.Sprintf("node_%p", node)
}

// paneID generates a unique ID for a browser pane
func (stm *StateTombstoneManager) paneID(pane *BrowserPane) string {
	if pane == nil {
		return ""
	}
	return fmt.Sprintf("pane_%p", pane)
}

// extractCSSClasses extracts CSS classes from a widget (simplified)
func (stm *StateTombstoneManager) extractCSSClasses(ptr uintptr) []string {
	// This would need to be implemented using GTK introspection
	// to extract the actual CSS classes from the widget
	var classes []string

	// Check for known classes
	knownClasses := []string{
		basePaneClass,
		multiPaneClass,
		activePaneClass,
		outlinePaneClass,
		stackContainerClass,
	}

	for _, class := range knownClasses {
		if webkit.WidgetHasCSSClass(ptr, class) {
			classes = append(classes, class)
		}
	}

	return classes
}

// compressTombstone compresses a tombstone to save memory
func (stm *StateTombstoneManager) compressTombstone(tombstone *StateTombstone) error {
	if tombstone.Compressed {
		return nil
	}

	// Serialize the tombstone data
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)

	data := map[string]interface{}{
		"tree":   tombstone.TreeState,
		"widget": tombstone.WidgetState,
		"focus":  tombstone.FocusState,
		"stack":  tombstone.StackState,
	}

	if err := encoder.Encode(data); err != nil {
		return err
	}

	// Compress with gzip
	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	if _, err := gzipWriter.Write(buf.Bytes()); err != nil {
		return err
	}
	if err := gzipWriter.Close(); err != nil {
		return err
	}

	// Store compressed data (this is simplified - in practice you'd store the compressed data)
	tombstone.Compressed = true

	log.Printf("[tombstone] Compressed tombstone %s: %d -> %d bytes",
		tombstone.ID, buf.Len(), compressed.Len())

	return nil
}

// decompressTombstone decompresses a tombstone
func (stm *StateTombstoneManager) decompressTombstone(tombstone *StateTombstone) error {
	if !tombstone.Compressed {
		return nil
	}

	// This would decompress the data stored during compression
	tombstone.Compressed = false

	log.Printf("[tombstone] Decompressed tombstone %s", tombstone.ID)
	return nil
}

// calculateTombstoneSize estimates the size of a tombstone
func (stm *StateTombstoneManager) calculateTombstoneSize(tombstone *StateTombstone) int {
	// This is a rough estimate - in practice you'd calculate more precisely
	size := 0

	if tombstone.TreeState != nil {
		size += len(tombstone.TreeState.Nodes) * 100 // Rough estimate per node
	}

	if tombstone.WidgetState != nil {
		size += len(tombstone.WidgetState.Widgets) * 50 // Rough estimate per widget
	}

	return size
}

// cleanupOldTombstones removes old tombstones to manage memory
func (stm *StateTombstoneManager) cleanupOldTombstones() {
	// Remove excess tombstones
	if len(stm.tombstones) > stm.maxTombstones {
		// Remove oldest tombstones first
		oldestTime := time.Now()
		var oldestID string

		for id, tombstone := range stm.tombstones {
			if tombstone.Timestamp.Before(oldestTime) {
				oldestTime = tombstone.Timestamp
				oldestID = id
			}
		}

		if oldestID != "" {
			delete(stm.tombstones, oldestID)
			log.Printf("[tombstone] Removed old tombstone: %s", oldestID)
		}
	}

	// Clean up history
	if len(stm.history) > stm.maxHistory {
		stm.history = stm.history[len(stm.history)-stm.maxHistory:]
	}
}

// GetTombstoneStats returns statistics about tombstones
func (stm *StateTombstoneManager) GetTombstoneStats() map[string]interface{} {
	stm.mu.RLock()
	defer stm.mu.RUnlock()

	totalSize := 0
	compressedCount := 0

	for _, tombstone := range stm.tombstones {
		totalSize += tombstone.Size
		if tombstone.Compressed {
			compressedCount++
		}
	}

	return map[string]interface{}{
		"total_tombstones":    len(stm.tombstones),
		"history_length":      len(stm.history),
		"total_size_bytes":    totalSize,
		"compressed_count":    compressedCount,
		"compression_rate":    float64(compressedCount) / float64(max(len(stm.tombstones), 1)),
		"max_tombstones":      stm.maxTombstones,
		"compression_enabled": stm.compress,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
